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
	awsaas "github.com/aws/aws-sdk-go-v2/service/applicationautoscaling"
	aastypes "github.com/aws/aws-sdk-go-v2/service/applicationautoscaling/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	aashelper "github.com/konfig-io/konfig-konector/internal/aws/applicationautoscaling"
)

// AppScalingPolicyAWSAPI is the subset of the Application Auto Scaling API
// used by this controller.
type AppScalingPolicyAWSAPI interface {
	PutScalingPolicy(ctx context.Context, params *awsaas.PutScalingPolicyInput, optFns ...func(*awsaas.Options)) (*awsaas.PutScalingPolicyOutput, error)
	DeleteScalingPolicy(ctx context.Context, params *awsaas.DeleteScalingPolicyInput, optFns ...func(*awsaas.Options)) (*awsaas.DeleteScalingPolicyOutput, error)
}

// AppScalingPolicyReconciler reconciles AppScalingPolicy objects.
type AppScalingPolicyReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	AASClient AppScalingPolicyAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=appscalingpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=appscalingpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=appscalingpolicies/finalizers,verbs=update

func (r *AppScalingPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	sp := &awsv1alpha1.AppScalingPolicy{}
	if err := r.Get(ctx, req.NamespacedName, sp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, sp); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !sp.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(sp, awsv1alpha1.FinalizerName) {
			if shouldAbandon(sp) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(sp, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, sp)
			}
			if err := r.deletePolicy(ctx, sp); err != nil {
				logger.Error(err, "failed to delete scaling policy")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(sp, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, sp)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(sp, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(sp, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, sp); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcilePolicy(ctx, sp); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, sp, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

// resolveTarget returns the (serviceNamespace, resourceId, scalableDimension)
// triple, from the referenced ScalableTarget CR or the inline spec fields.
func (r *AppScalingPolicyReconciler) resolveTarget(ctx context.Context, sp *awsv1alpha1.AppScalingPolicy) (string, string, string, error) {
	if sp.Spec.TargetRef != nil {
		st := &awsv1alpha1.ScalableTarget{}
		if err := r.Get(ctx, k8stypes.NamespacedName{Name: sp.Spec.TargetRef.Name, Namespace: sp.Namespace}, st); err != nil {
			return "", "", "", err
		}
		if st.Status.ScalableTargetARN == "" {
			return "", "", "", &dependencyNotReady{msg: fmt.Sprintf("ScalableTarget %s/%s is not yet registered", sp.Namespace, sp.Spec.TargetRef.Name)}
		}
		return st.Spec.ServiceNamespace, st.Spec.ResourceID, st.Spec.ScalableDimension, nil
	}
	if sp.Spec.ServiceNamespace == "" || sp.Spec.ResourceID == "" || sp.Spec.ScalableDimension == "" {
		return "", "", "", fmt.Errorf("either targetRef or serviceNamespace/resourceId/scalableDimension must be set")
	}
	return sp.Spec.ServiceNamespace, sp.Spec.ResourceID, sp.Spec.ScalableDimension, nil
}

func (r *AppScalingPolicyReconciler) reconcilePolicy(ctx context.Context, sp *awsv1alpha1.AppScalingPolicy) error {
	ns, resourceID, dimension, err := r.resolveTarget(ctx, sp)
	if err != nil {
		return err
	}

	input := &awsaas.PutScalingPolicyInput{
		PolicyName:        aws.String(sp.Spec.PolicyName),
		ServiceNamespace:  aastypes.ServiceNamespace(ns),
		ResourceId:        aws.String(resourceID),
		ScalableDimension: aastypes.ScalableDimension(dimension),
		PolicyType:        aastypes.PolicyType(sp.Spec.PolicyType),
	}

	if tt := sp.Spec.TargetTrackingConfiguration; tt != nil {
		cfg := &aastypes.TargetTrackingScalingPolicyConfiguration{
			TargetValue: aws.Float64(tt.TargetValue),
			PredefinedMetricSpecification: &aastypes.PredefinedMetricSpecification{
				PredefinedMetricType: aastypes.MetricType(tt.PredefinedMetricType),
			},
		}
		if tt.ResourceLabel != "" {
			cfg.PredefinedMetricSpecification.ResourceLabel = aws.String(tt.ResourceLabel)
		}
		if tt.ScaleInCooldown != nil {
			cfg.ScaleInCooldown = tt.ScaleInCooldown
		}
		if tt.ScaleOutCooldown != nil {
			cfg.ScaleOutCooldown = tt.ScaleOutCooldown
		}
		if tt.DisableScaleIn {
			cfg.DisableScaleIn = aws.Bool(true)
		}
		input.TargetTrackingScalingPolicyConfiguration = cfg
	}

	if ss := sp.Spec.StepScalingConfiguration; ss != nil {
		cfg := &aastypes.StepScalingPolicyConfiguration{
			AdjustmentType: aastypes.AdjustmentType(ss.AdjustmentType),
		}
		if ss.Cooldown != nil {
			cfg.Cooldown = ss.Cooldown
		}
		if ss.MetricAggregationType != "" {
			cfg.MetricAggregationType = aastypes.MetricAggregationType(ss.MetricAggregationType)
		}
		if ss.MinAdjustmentMagnitude != nil {
			cfg.MinAdjustmentMagnitude = ss.MinAdjustmentMagnitude
		}
		for _, sa := range ss.StepAdjustments {
			step := aastypes.StepAdjustment{
				ScalingAdjustment: aws.Int32(sa.ScalingAdjustment),
			}
			if sa.MetricIntervalLowerBound != nil {
				step.MetricIntervalLowerBound = sa.MetricIntervalLowerBound
			}
			if sa.MetricIntervalUpperBound != nil {
				step.MetricIntervalUpperBound = sa.MetricIntervalUpperBound
			}
			cfg.StepAdjustments = append(cfg.StepAdjustments, step)
		}
		input.StepScalingPolicyConfiguration = cfg
	}

	// PutScalingPolicy is an idempotent upsert keyed on the policy name.
	out, err := r.AASClient.PutScalingPolicy(ctx, input)
	if err != nil {
		return fmt.Errorf("put scaling policy: %w", err)
	}
	sp.Status.PolicyARN = aws.ToString(out.PolicyARN)
	if err := persistStatus(ctx, r.Client, sp); err != nil {
		return fmt.Errorf("persist policy ARN after put: %w", err)
	}

	sp.Status.ObservedGeneration = sp.Generation
	now := metav1.Now()
	sp.Status.LastSyncTime = &now
	return r.setCondition(ctx, sp, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "scaling policy reconciled")
}

func (r *AppScalingPolicyReconciler) deletePolicy(ctx context.Context, sp *awsv1alpha1.AppScalingPolicy) error {
	ns, resourceID, dimension, err := r.resolveTarget(ctx, sp)
	if err != nil {
		// If the referenced ScalableTarget CR is already gone, AWS has (or
		// will) cascade-delete the policy with the target; nothing to do.
		var notReady *dependencyNotReady
		if apierrors.IsNotFound(err) || errors.As(err, &notReady) {
			return nil
		}
		return err
	}
	_, err = r.AASClient.DeleteScalingPolicy(ctx, &awsaas.DeleteScalingPolicyInput{
		PolicyName:        aws.String(sp.Spec.PolicyName),
		ServiceNamespace:  aastypes.ServiceNamespace(ns),
		ResourceId:        aws.String(resourceID),
		ScalableDimension: aastypes.ScalableDimension(dimension),
	})
	if aashelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *AppScalingPolicyReconciler) setCondition(ctx context.Context, sp *awsv1alpha1.AppScalingPolicy, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&sp.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: sp.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, sp); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *AppScalingPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.AppScalingPolicy{}).
		Complete(r)
}
