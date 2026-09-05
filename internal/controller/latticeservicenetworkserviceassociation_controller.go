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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	latticehelper "github.com/konfig-io/konfig-konector/internal/aws/vpclattice"
)

// LatticeServiceNetworkServiceAssociationAWSAPI is the subset of the VPC
// Lattice API used by this controller.
type LatticeServiceNetworkServiceAssociationAWSAPI interface {
	GetServiceNetworkServiceAssociation(ctx context.Context, params *awslattice.GetServiceNetworkServiceAssociationInput, optFns ...func(*awslattice.Options)) (*awslattice.GetServiceNetworkServiceAssociationOutput, error)
	CreateServiceNetworkServiceAssociation(ctx context.Context, params *awslattice.CreateServiceNetworkServiceAssociationInput, optFns ...func(*awslattice.Options)) (*awslattice.CreateServiceNetworkServiceAssociationOutput, error)
	DeleteServiceNetworkServiceAssociation(ctx context.Context, params *awslattice.DeleteServiceNetworkServiceAssociationInput, optFns ...func(*awslattice.Options)) (*awslattice.DeleteServiceNetworkServiceAssociationOutput, error)
}

// LatticeServiceNetworkServiceAssociationReconciler reconciles
// LatticeServiceNetworkServiceAssociation objects.
type LatticeServiceNetworkServiceAssociationReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	LatticeClient LatticeServiceNetworkServiceAssociationAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=latticeservicenetworkserviceassociations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=latticeservicenetworkserviceassociations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=latticeservicenetworkserviceassociations/finalizers,verbs=update

func (r *LatticeServiceNetworkServiceAssociationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.LatticeServiceNetworkServiceAssociation{}
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
				logger.Error(err, "failed to delete LatticeServiceNetworkServiceAssociation")
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

func (r *LatticeServiceNetworkServiceAssociationReconciler) reconcileAssociation(ctx context.Context, obj *awsv1alpha1.LatticeServiceNetworkServiceAssociation) (ctrl.Result, error) {
	if obj.Status.ID == "" {
		snID, err := resolveLatticeServiceNetworkID(ctx, r.Client, obj.Namespace, obj.Spec.ServiceNetworkRef)
		if err != nil {
			return ctrl.Result{}, err
		}
		svcID, err := resolveLatticeServiceID(ctx, r.Client, obj.Namespace, obj.Spec.ServiceRef)
		if err != nil {
			return ctrl.Result{}, err
		}
		out, err := r.LatticeClient.CreateServiceNetworkServiceAssociation(ctx, &awslattice.CreateServiceNetworkServiceAssociationInput{
			ServiceNetworkIdentifier: aws.String(snID),
			ServiceIdentifier:        aws.String(svcID),
			Tags:                     obj.Spec.Tags,
		})
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("create service network service association: %w", err)
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

	getOut, err := r.LatticeClient.GetServiceNetworkServiceAssociation(ctx, &awslattice.GetServiceNetworkServiceAssociationInput{
		ServiceNetworkServiceAssociationIdentifier: aws.String(obj.Status.ID),
	})
	if latticehelper.IsNotFound(err) {
		obj.Status.ID = ""
		obj.Status.ARN = ""
		obj.Status.Status = ""
		return r.reconcileAssociation(ctx, obj)
	}
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("get service network service association: %w", err)
	}
	obj.Status.ARN = aws.ToString(getOut.Arn)
	obj.Status.Status = string(getOut.Status)

	if getOut.Status != latticetypes.ServiceNetworkServiceAssociationStatusActive {
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

func (r *LatticeServiceNetworkServiceAssociationReconciler) deleteAssociation(ctx context.Context, obj *awsv1alpha1.LatticeServiceNetworkServiceAssociation) error {
	if obj.Status.ID == "" {
		return nil
	}
	_, err := r.LatticeClient.DeleteServiceNetworkServiceAssociation(ctx, &awslattice.DeleteServiceNetworkServiceAssociationInput{
		ServiceNetworkServiceAssociationIdentifier: aws.String(obj.Status.ID),
	})
	if latticehelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *LatticeServiceNetworkServiceAssociationReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.LatticeServiceNetworkServiceAssociation, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *LatticeServiceNetworkServiceAssociationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.LatticeServiceNetworkServiceAssociation{}).
		Complete(r)
}
