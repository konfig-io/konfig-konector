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

// LatticeServiceNetworkAWSAPI is the subset of the VPC Lattice API used by this controller.
type LatticeServiceNetworkAWSAPI interface {
	GetServiceNetwork(ctx context.Context, params *awslattice.GetServiceNetworkInput, optFns ...func(*awslattice.Options)) (*awslattice.GetServiceNetworkOutput, error)
	CreateServiceNetwork(ctx context.Context, params *awslattice.CreateServiceNetworkInput, optFns ...func(*awslattice.Options)) (*awslattice.CreateServiceNetworkOutput, error)
	UpdateServiceNetwork(ctx context.Context, params *awslattice.UpdateServiceNetworkInput, optFns ...func(*awslattice.Options)) (*awslattice.UpdateServiceNetworkOutput, error)
	DeleteServiceNetwork(ctx context.Context, params *awslattice.DeleteServiceNetworkInput, optFns ...func(*awslattice.Options)) (*awslattice.DeleteServiceNetworkOutput, error)
	TagResource(ctx context.Context, params *awslattice.TagResourceInput, optFns ...func(*awslattice.Options)) (*awslattice.TagResourceOutput, error)
}

// LatticeServiceNetworkReconciler reconciles LatticeServiceNetwork objects.
type LatticeServiceNetworkReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	LatticeClient LatticeServiceNetworkAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=latticeservicenetworks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=latticeservicenetworks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=latticeservicenetworks/finalizers,verbs=update

func (r *LatticeServiceNetworkReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.LatticeServiceNetwork{}
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
			if err := r.deleteServiceNetwork(ctx, obj); err != nil {
				logger.Error(err, "failed to delete LatticeServiceNetwork")
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
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileServiceNetwork(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *LatticeServiceNetworkReconciler) reconcileServiceNetwork(ctx context.Context, obj *awsv1alpha1.LatticeServiceNetwork) error {
	if obj.Status.ID == "" {
		out, err := r.LatticeClient.CreateServiceNetwork(ctx, &awslattice.CreateServiceNetworkInput{
			Name:     aws.String(obj.Spec.Name),
			AuthType: latticetypes.AuthType(obj.Spec.AuthType),
			Tags:     obj.Spec.Tags,
		})
		if err != nil {
			return fmt.Errorf("create service network: %w", err)
		}
		obj.Status.ID = aws.ToString(out.Id)
		obj.Status.ARN = aws.ToString(out.Arn)
		if err := persistStatus(ctx, r.Client, obj); err != nil {
			return fmt.Errorf("persist service network ID after create: %w", err)
		}
		obj.Status.ObservedGeneration = obj.Generation
		now := metav1.Now()
		obj.Status.LastSyncTime = &now
		return r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "LatticeServiceNetwork created")
	}

	getOut, err := r.LatticeClient.GetServiceNetwork(ctx, &awslattice.GetServiceNetworkInput{
		ServiceNetworkIdentifier: aws.String(obj.Status.ID),
	})
	if latticehelper.IsNotFound(err) {
		// Resource was deleted out of band — recreate on the next pass.
		obj.Status.ID = ""
		obj.Status.ARN = ""
		return r.reconcileServiceNetwork(ctx, obj)
	}
	if err != nil {
		return fmt.Errorf("get service network: %w", err)
	}
	obj.Status.ARN = aws.ToString(getOut.Arn)

	if obj.Status.ObservedGeneration != obj.Generation {
		if obj.Spec.AuthType != "" && string(getOut.AuthType) != obj.Spec.AuthType {
			if _, err := r.LatticeClient.UpdateServiceNetwork(ctx, &awslattice.UpdateServiceNetworkInput{
				ServiceNetworkIdentifier: aws.String(obj.Status.ID),
				AuthType:                 latticetypes.AuthType(obj.Spec.AuthType),
			}); err != nil {
				return fmt.Errorf("update service network: %w", err)
			}
		}
		if len(obj.Spec.Tags) > 0 {
			if _, err := r.LatticeClient.TagResource(ctx, &awslattice.TagResourceInput{
				ResourceArn: getOut.Arn,
				Tags:        obj.Spec.Tags,
			}); err != nil {
				return fmt.Errorf("tag service network: %w", err)
			}
		}
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "LatticeServiceNetwork reconciled")
}

func (r *LatticeServiceNetworkReconciler) deleteServiceNetwork(ctx context.Context, obj *awsv1alpha1.LatticeServiceNetwork) error {
	if obj.Status.ID == "" {
		// VPC Lattice identifiers must be an ID or ARN; without a persisted
		// identifier there is no unambiguous lookup to perform here.
		return nil
	}
	_, err := r.LatticeClient.DeleteServiceNetwork(ctx, &awslattice.DeleteServiceNetworkInput{
		ServiceNetworkIdentifier: aws.String(obj.Status.ID),
	})
	if latticehelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *LatticeServiceNetworkReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.LatticeServiceNetwork, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *LatticeServiceNetworkReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.LatticeServiceNetwork{}).
		Complete(r)
}
