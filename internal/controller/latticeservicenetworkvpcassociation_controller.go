/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awslattice "github.com/aws/aws-sdk-go-v2/service/vpclattice"
	latticetypes "github.com/aws/aws-sdk-go-v2/service/vpclattice/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	latticehelper "github.com/konfig-io/konfig-konector/internal/aws/vpclattice"
)

// resolveLatticeServiceNetworkID resolves a LatticeServiceNetworkRef to a
// service network ID or ARN.
func resolveLatticeServiceNetworkID(ctx context.Context, c client.Client, namespace string, ref awsv1alpha1.LatticeServiceNetworkRef) (string, error) {
	if ref.ID != "" {
		return ref.ID, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("serviceNetworkRef requires name or id")
	}
	sn := &awsv1alpha1.LatticeServiceNetwork{}
	if err := c.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, sn); err != nil {
		return "", err
	}
	if sn.Status.ID == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("LatticeServiceNetwork %s/%s has no ID yet", namespace, ref.Name)}
	}
	return sn.Status.ID, nil
}

// resolveLatticeServiceID resolves a LatticeServiceRef to a service ID or ARN.
func resolveLatticeServiceID(ctx context.Context, c client.Client, namespace string, ref awsv1alpha1.LatticeServiceRef) (string, error) {
	if ref.ID != "" {
		return ref.ID, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("serviceRef requires name or id")
	}
	svc := &awsv1alpha1.LatticeService{}
	if err := c.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, svc); err != nil {
		return "", err
	}
	if svc.Status.ID == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("LatticeService %s/%s has no ID yet", namespace, ref.Name)}
	}
	return svc.Status.ID, nil
}

// LatticeServiceNetworkVpcAssociationAWSAPI is the subset of the VPC Lattice
// API used by this controller.
type LatticeServiceNetworkVpcAssociationAWSAPI interface {
	GetServiceNetworkVpcAssociation(ctx context.Context, params *awslattice.GetServiceNetworkVpcAssociationInput, optFns ...func(*awslattice.Options)) (*awslattice.GetServiceNetworkVpcAssociationOutput, error)
	CreateServiceNetworkVpcAssociation(ctx context.Context, params *awslattice.CreateServiceNetworkVpcAssociationInput, optFns ...func(*awslattice.Options)) (*awslattice.CreateServiceNetworkVpcAssociationOutput, error)
	DeleteServiceNetworkVpcAssociation(ctx context.Context, params *awslattice.DeleteServiceNetworkVpcAssociationInput, optFns ...func(*awslattice.Options)) (*awslattice.DeleteServiceNetworkVpcAssociationOutput, error)
}

// LatticeServiceNetworkVpcAssociationReconciler reconciles
// LatticeServiceNetworkVpcAssociation objects.
type LatticeServiceNetworkVpcAssociationReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	LatticeClient LatticeServiceNetworkVpcAssociationAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=latticeservicenetworkvpcassociations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=latticeservicenetworkvpcassociations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=latticeservicenetworkvpcassociations/finalizers,verbs=update

func (r *LatticeServiceNetworkVpcAssociationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.LatticeServiceNetworkVpcAssociation{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, obj); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !obj.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(obj, awsv1alpha1.FinalizerName) {
			if shouldAbandon(obj) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(obj, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, obj)
			}
			if err := r.deleteAssociation(ctx, obj); err != nil {
				logger.Error(err, "failed to delete LatticeServiceNetworkVpcAssociation")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(obj, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, obj)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(obj, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(obj, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, obj); err != nil {
			return ctrl.Result{}, err
		}
	}

	result, err := r.reconcileAssociation(ctx, obj)
	if err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *LatticeServiceNetworkVpcAssociationReconciler) reconcileAssociation(ctx context.Context, obj *awsv1alpha1.LatticeServiceNetworkVpcAssociation) (ctrl.Result, error) {
	if obj.Status.ID == "" {
		snID, err := resolveLatticeServiceNetworkID(ctx, r.Client, obj.Namespace, obj.Spec.ServiceNetworkRef)
		if err != nil {
			return ctrl.Result{}, err
		}
		vpcID, err := r.resolveVPCIDLatticeAssoc(ctx, obj.Namespace, &obj.Spec.VPCRef)
		if err != nil {
			return ctrl.Result{}, err
		}
		sgIDs, err := resolveSGIDs(ctx, r.Client, obj.Namespace, obj.Spec.SecurityGroupRefs)
		if err != nil {
			return ctrl.Result{}, err
		}
		out, err := r.LatticeClient.CreateServiceNetworkVpcAssociation(ctx, &awslattice.CreateServiceNetworkVpcAssociationInput{
			ServiceNetworkIdentifier: aws.String(snID),
			VpcIdentifier:            aws.String(vpcID),
			SecurityGroupIds:         sgIDs,
			Tags:                     obj.Spec.Tags,
		})
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("create service network VPC association: %w", err)
		}
		obj.Status.ID = aws.ToString(out.Id)
		obj.Status.ARN = aws.ToString(out.Arn)
		obj.Status.Status = string(out.Status)
		if err := persistStatus(ctx, r.Client, obj); err != nil {
			return ctrl.Result{}, fmt.Errorf("persist association ID after create: %w", err)
		}
		_ = r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Creating", "association is being created")
		return requeueLatticePolling, nil
	}

	getOut, err := r.LatticeClient.GetServiceNetworkVpcAssociation(ctx, &awslattice.GetServiceNetworkVpcAssociationInput{
		ServiceNetworkVpcAssociationIdentifier: aws.String(obj.Status.ID),
	})
	if latticehelper.IsNotFound(err) {
		obj.Status.ID = ""
		obj.Status.ARN = ""
		obj.Status.Status = ""
		return r.reconcileAssociation(ctx, obj)
	}
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("get service network VPC association: %w", err)
	}
	obj.Status.ARN = aws.ToString(getOut.Arn)
	obj.Status.Status = string(getOut.Status)

	if getOut.Status != latticetypes.ServiceNetworkVpcAssociationStatusActive {
		_ = r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Creating", fmt.Sprintf("association status is %s", getOut.Status))
		return requeueLatticePolling, nil
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	if err := r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "association active"); err != nil {
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *LatticeServiceNetworkVpcAssociationReconciler) resolveVPCIDLatticeAssoc(ctx context.Context, namespace string, ref *awsv1alpha1.VPCResourceRef) (string, error) {
	if ref.ID != "" {
		return ref.ID, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("vpcRef requires name or id")
	}
	vpc := &awsv1alpha1.VPC{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, vpc); err != nil {
		return "", err
	}
	if vpc.Status.VPCID == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("VPC %s/%s has no vpcId yet", namespace, ref.Name)}
	}
	return vpc.Status.VPCID, nil
}

func (r *LatticeServiceNetworkVpcAssociationReconciler) deleteAssociation(ctx context.Context, obj *awsv1alpha1.LatticeServiceNetworkVpcAssociation) error {
	if obj.Status.ID == "" {
		return nil
	}
	_, err := r.LatticeClient.DeleteServiceNetworkVpcAssociation(ctx, &awslattice.DeleteServiceNetworkVpcAssociationInput{
		ServiceNetworkVpcAssociationIdentifier: aws.String(obj.Status.ID),
	})
	if latticehelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *LatticeServiceNetworkVpcAssociationReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.LatticeServiceNetworkVpcAssociation, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&obj.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: obj.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *LatticeServiceNetworkVpcAssociationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.LatticeServiceNetworkVpcAssociation{}).
		Complete(r)
}
