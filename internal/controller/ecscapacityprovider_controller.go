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
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	ecshelper "github.com/konfig-io/konfig-konector/internal/aws/ecs"
)

// ECSCapacityProviderReconciler reconciles ECSCapacityProvider objects.
type ECSCapacityProviderReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	ECSClient *awsecs.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=ecscapacityproviders,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ecscapacityproviders/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ecscapacityproviders/finalizers,verbs=update

func (r *ECSCapacityProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.ECSCapacityProvider{}
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
			if err := r.deleteCapacityProvider(ctx, obj); err != nil {
				logger.Error(err, "failed to delete ECSCapacityProvider")
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

	if err := r.reconcileCapacityProvider(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionECPCP(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *ECSCapacityProviderReconciler) reconcileCapacityProvider(ctx context.Context, obj *awsv1alpha1.ECSCapacityProvider) error {
	existing, err := r.describeCapacityProvider(ctx, obj.Spec.Name)
	if err != nil {
		return fmt.Errorf("describe ecs capacity provider: %w", err)
	}

	if existing != nil {
		obj.Status.CapacityProviderARN = aws.ToString(existing.CapacityProviderArn)
		obj.Status.ProviderStatus = string(existing.Status)

		updateInput := &awsecs.UpdateCapacityProviderInput{
			Name: aws.String(obj.Spec.Name),
			AutoScalingGroupProvider: &ecstypes.AutoScalingGroupProviderUpdate{
				ManagedScaling:               buildManagedScaling(obj.Spec.AutoScalingGroupProvider.ManagedScaling),
				ManagedTerminationProtection: ecstypes.ManagedTerminationProtection(obj.Spec.AutoScalingGroupProvider.ManagedTerminationProtection),
			},
		}
		_, err := r.ECSClient.UpdateCapacityProvider(ctx, updateInput)
		if err != nil {
			return fmt.Errorf("update ecs capacity provider: %w", err)
		}
	} else {
		tags := make([]ecstypes.Tag, 0, len(obj.Spec.Tags))
		for k, v := range obj.Spec.Tags {
			k, v := k, v
			tags = append(tags, ecstypes.Tag{Key: &k, Value: &v})
		}

		createInput := &awsecs.CreateCapacityProviderInput{
			Name: aws.String(obj.Spec.Name),
			AutoScalingGroupProvider: &ecstypes.AutoScalingGroupProvider{
				AutoScalingGroupArn:          aws.String(obj.Spec.AutoScalingGroupProvider.AutoScalingGroupARN),
				ManagedScaling:               buildManagedScaling(obj.Spec.AutoScalingGroupProvider.ManagedScaling),
				ManagedTerminationProtection: ecstypes.ManagedTerminationProtection(obj.Spec.AutoScalingGroupProvider.ManagedTerminationProtection),
			},
		}
		if len(tags) > 0 {
			createInput.Tags = tags
		}
		out, err := r.ECSClient.CreateCapacityProvider(ctx, createInput)
		if err != nil {
			return fmt.Errorf("create ecs capacity provider: %w", err)
		}
		if out.CapacityProvider != nil {
			obj.Status.CapacityProviderARN = aws.ToString(out.CapacityProvider.CapacityProviderArn)
			obj.Status.ProviderStatus = string(out.CapacityProvider.Status)
		}
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionECPCP(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "ECSCapacityProvider reconciled")
}

func buildManagedScaling(ms *awsv1alpha1.ECSManagedScaling) *ecstypes.ManagedScaling {
	if ms == nil {
		return nil
	}
	result := &ecstypes.ManagedScaling{
		InstanceWarmupPeriod:   ms.InstanceWarmupPeriod,
		MaximumScalingStepSize: ms.MaximumScalingStepSize,
		MinimumScalingStepSize: ms.MinimumScalingStepSize,
		TargetCapacity:         ms.TargetCapacity,
	}
	if ms.Status != "" {
		result.Status = ecstypes.ManagedScalingStatus(ms.Status)
	}
	return result
}

func (r *ECSCapacityProviderReconciler) describeCapacityProvider(ctx context.Context, name string) (*ecstypes.CapacityProvider, error) {
	out, err := r.ECSClient.DescribeCapacityProviders(ctx, &awsecs.DescribeCapacityProvidersInput{
		CapacityProviders: []string{name},
	})
	if err != nil {
		if ecshelper.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	for i := range out.CapacityProviders {
		if aws.ToString(out.CapacityProviders[i].Name) == name && out.CapacityProviders[i].Status != ecstypes.CapacityProviderStatusInactive {
			return &out.CapacityProviders[i], nil
		}
	}
	return nil, nil
}

func (r *ECSCapacityProviderReconciler) deleteCapacityProvider(ctx context.Context, obj *awsv1alpha1.ECSCapacityProvider) error {
	_, err := r.ECSClient.DeleteCapacityProvider(ctx, &awsecs.DeleteCapacityProviderInput{
		CapacityProvider: aws.String(obj.Spec.Name),
	})
	if ecshelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *ECSCapacityProviderReconciler) setConditionECPCP(ctx context.Context, obj *awsv1alpha1.ECSCapacityProvider, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *ECSCapacityProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.ECSCapacityProvider{}).
		Complete(r)
}
