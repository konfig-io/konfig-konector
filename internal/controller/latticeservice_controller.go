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
	"fmt"
	"time"

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

// requeueLatticePolling is the requeue interval while a VPC Lattice resource
// is being created or deleted asynchronously.
var requeueLatticePolling = ctrl.Result{RequeueAfter: 15 * time.Second}

// LatticeServiceAWSAPI is the subset of the VPC Lattice API used by this controller.
type LatticeServiceAWSAPI interface {
	GetService(ctx context.Context, params *awslattice.GetServiceInput, optFns ...func(*awslattice.Options)) (*awslattice.GetServiceOutput, error)
	CreateService(ctx context.Context, params *awslattice.CreateServiceInput, optFns ...func(*awslattice.Options)) (*awslattice.CreateServiceOutput, error)
	UpdateService(ctx context.Context, params *awslattice.UpdateServiceInput, optFns ...func(*awslattice.Options)) (*awslattice.UpdateServiceOutput, error)
	DeleteService(ctx context.Context, params *awslattice.DeleteServiceInput, optFns ...func(*awslattice.Options)) (*awslattice.DeleteServiceOutput, error)
	TagResource(ctx context.Context, params *awslattice.TagResourceInput, optFns ...func(*awslattice.Options)) (*awslattice.TagResourceOutput, error)
}

// LatticeServiceReconciler reconciles LatticeService objects.
type LatticeServiceReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	LatticeClient LatticeServiceAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=latticeservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=latticeservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=latticeservices/finalizers,verbs=update

func (r *LatticeServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.LatticeService{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !obj.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(obj, awsv1alpha1.FinalizerName) {
			if shouldAbandon(obj) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(obj, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, obj)
			}
			if err := r.deleteService(ctx, obj); err != nil {
				logger.Error(err, "failed to delete LatticeService")
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

	result, err := r.reconcileService(ctx, obj)
	if err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *LatticeServiceReconciler) reconcileService(ctx context.Context, obj *awsv1alpha1.LatticeService) (ctrl.Result, error) {
	if obj.Status.ID == "" {
		input := &awslattice.CreateServiceInput{
			Name:     aws.String(obj.Spec.Name),
			AuthType: latticetypes.AuthType(obj.Spec.AuthType),
			Tags:     obj.Spec.Tags,
		}
		if obj.Spec.CustomDomainName != "" {
			input.CustomDomainName = aws.String(obj.Spec.CustomDomainName)
		}
		if obj.Spec.CertificateARN != "" {
			input.CertificateArn = aws.String(obj.Spec.CertificateARN)
		}
		out, err := r.LatticeClient.CreateService(ctx, input)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("create service: %w", err)
		}
		obj.Status.ID = aws.ToString(out.Id)
		obj.Status.ARN = aws.ToString(out.Arn)
		obj.Status.Status = string(out.Status)
		if out.DnsEntry != nil {
			obj.Status.DNSName = aws.ToString(out.DnsEntry.DomainName)
		}
		if err := persistStatus(ctx, r.Client, obj); err != nil {
			return ctrl.Result{}, fmt.Errorf("persist service ID after create: %w", err)
		}
		_ = r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Creating", "Lattice service is being created")
		return requeueLatticePolling, nil
	}

	getOut, err := r.LatticeClient.GetService(ctx, &awslattice.GetServiceInput{
		ServiceIdentifier: aws.String(obj.Status.ID),
	})
	if latticehelper.IsNotFound(err) {
		// Deleted out of band — recreate on the next pass.
		obj.Status.ID = ""
		obj.Status.ARN = ""
		obj.Status.Status = ""
		return r.reconcileService(ctx, obj)
	}
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("get service: %w", err)
	}

	obj.Status.ARN = aws.ToString(getOut.Arn)
	obj.Status.Status = string(getOut.Status)
	if getOut.DnsEntry != nil {
		obj.Status.DNSName = aws.ToString(getOut.DnsEntry.DomainName)
	}

	if getOut.Status != latticetypes.ServiceStatusActive {
		_ = r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Creating", fmt.Sprintf("Lattice service status is %s", getOut.Status))
		return requeueLatticePolling, nil
	}

	if obj.Status.ObservedGeneration != obj.Generation {
		input := &awslattice.UpdateServiceInput{
			ServiceIdentifier: aws.String(obj.Status.ID),
		}
		if obj.Spec.AuthType != "" {
			input.AuthType = latticetypes.AuthType(obj.Spec.AuthType)
		}
		if obj.Spec.CertificateARN != "" {
			input.CertificateArn = aws.String(obj.Spec.CertificateARN)
		}
		if _, err := r.LatticeClient.UpdateService(ctx, input); err != nil {
			return ctrl.Result{}, fmt.Errorf("update service: %w", err)
		}
		if len(obj.Spec.Tags) > 0 {
			if _, err := r.LatticeClient.TagResource(ctx, &awslattice.TagResourceInput{
				ResourceArn: getOut.Arn,
				Tags:        obj.Spec.Tags,
			}); err != nil {
				return ctrl.Result{}, fmt.Errorf("tag service: %w", err)
			}
		}
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	if err := r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "LatticeService reconciled"); err != nil {
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *LatticeServiceReconciler) deleteService(ctx context.Context, obj *awsv1alpha1.LatticeService) error {
	if obj.Status.ID == "" {
		// VPC Lattice identifiers must be an ID or ARN; nothing to delete
		// without a persisted identifier.
		return nil
	}
	_, err := r.LatticeClient.DeleteService(ctx, &awslattice.DeleteServiceInput{
		ServiceIdentifier: aws.String(obj.Status.ID),
	})
	if latticehelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *LatticeServiceReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.LatticeService, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *LatticeServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.LatticeService{}).
		Complete(r)
}
