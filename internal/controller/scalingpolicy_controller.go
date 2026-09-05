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
	awsas "github.com/aws/aws-sdk-go-v2/service/autoscaling"
	astypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// ScalingPolicyReconciler reconciles ScalingPolicy objects.
type ScalingPolicyReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	AutoScalingClient *multi.AutoScaling
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=scalingpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=scalingpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=scalingpolicies/finalizers,verbs=update

func (r *ScalingPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	sp := &awsv1alpha1.ScalingPolicy{}
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
			if err := r.deleteScalingPolicy(ctx, sp); err != nil {
				logger.Error(err, "failed to delete ScalingPolicy")
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
	}

	if err := r.reconcileScalingPolicy(ctx, sp); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionSP(ctx, sp, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *ScalingPolicyReconciler) reconcileScalingPolicy(ctx context.Context, sp *awsv1alpha1.ScalingPolicy) error {
	asgName, err := r.resolveASGName(ctx, sp)
	if err != nil {
		return err
	}

	input := &awsas.PutScalingPolicyInput{
		AutoScalingGroupName: aws.String(asgName),
		PolicyName:           aws.String(sp.Spec.PolicyName),
		PolicyType:           aws.String(sp.Spec.PolicyType),
	}
	if sp.Spec.AdjustmentType != "" {
		input.AdjustmentType = aws.String(sp.Spec.AdjustmentType)
	}
	if sp.Spec.ScalingAdjustment != 0 {
		input.ScalingAdjustment = aws.Int32(sp.Spec.ScalingAdjustment)
	}
	if sp.Spec.Cooldown > 0 {
		input.Cooldown = aws.Int32(sp.Spec.Cooldown)
	}
	for _, s := range sp.Spec.StepAdjustments {
		sa := astypes.StepAdjustment{
			ScalingAdjustment: aws.Int32(s.ScalingAdjustment),
		}
		if s.MetricIntervalLowerBound != nil {
			sa.MetricIntervalLowerBound = s.MetricIntervalLowerBound
		}
		if s.MetricIntervalUpperBound != nil {
			sa.MetricIntervalUpperBound = s.MetricIntervalUpperBound
		}
		input.StepAdjustments = append(input.StepAdjustments, sa)
	}
	if ttc := sp.Spec.TargetTrackingConfiguration; ttc != nil {
		cfg := &astypes.TargetTrackingConfiguration{
			TargetValue:    aws.Float64(ttc.TargetValue),
			DisableScaleIn: aws.Bool(ttc.DisableScaleIn),
		}
		if ttc.PredefinedMetricType != "" {
			cfg.PredefinedMetricSpecification = &astypes.PredefinedMetricSpecification{
				PredefinedMetricType: astypes.MetricType(ttc.PredefinedMetricType),
			}
		}
		input.TargetTrackingConfiguration = cfg
	}

	out, err := r.AutoScalingClient.PutScalingPolicy(ctx, input)
	if err != nil {
		return fmt.Errorf("put scaling policy: %w", err)
	}

	sp.Status.PolicyARN = aws.ToString(out.PolicyARN)
	sp.Status.ObservedGeneration = sp.Generation
	now := metav1.Now()
	sp.Status.LastSyncTime = &now
	return r.setConditionSP(ctx, sp, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "ScalingPolicy reconciled")
}

func (r *ScalingPolicyReconciler) resolveASGName(ctx context.Context, sp *awsv1alpha1.ScalingPolicy) (string, error) {
	ref := sp.Spec.AutoScalingGroupRef
	if ref.Name == "" {
		return "", fmt.Errorf("autoScalingGroupRef.name is required")
	}
	asg := &awsv1alpha1.AutoScalingGroup{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: sp.Namespace}, asg); err != nil {
		return "", err
	}
	if asg.Spec.AutoScalingGroupName == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("AutoScalingGroup %s/%s has no name", sp.Namespace, ref.Name)}
	}
	return asg.Spec.AutoScalingGroupName, nil
}

func (r *ScalingPolicyReconciler) deleteScalingPolicy(ctx context.Context, sp *awsv1alpha1.ScalingPolicy) error {
	asgName, err := r.resolveASGName(ctx, sp)
	if err != nil {
		return nil // if dependency gone, nothing to delete
	}
	_, err = r.AutoScalingClient.DeletePolicy(ctx, &awsas.DeletePolicyInput{
		AutoScalingGroupName: aws.String(asgName),
		PolicyName:           aws.String(sp.Spec.PolicyName),
	})
	return err
}

func (r *ScalingPolicyReconciler) setConditionSP(ctx context.Context, sp *awsv1alpha1.ScalingPolicy, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *ScalingPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.ScalingPolicy{}).
		Complete(r)
}
