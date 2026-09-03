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
	awsscheduler "github.com/aws/aws-sdk-go-v2/service/scheduler"
	schedulertypes "github.com/aws/aws-sdk-go-v2/service/scheduler/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	schedulerhelper "github.com/konfig-io/konfig-konector/internal/aws/scheduler"
)

// ScheduleAWSAPI is the subset of the EventBridge Scheduler API used by this
// controller.
type ScheduleAWSAPI interface {
	GetSchedule(ctx context.Context, params *awsscheduler.GetScheduleInput, optFns ...func(*awsscheduler.Options)) (*awsscheduler.GetScheduleOutput, error)
	CreateSchedule(ctx context.Context, params *awsscheduler.CreateScheduleInput, optFns ...func(*awsscheduler.Options)) (*awsscheduler.CreateScheduleOutput, error)
	UpdateSchedule(ctx context.Context, params *awsscheduler.UpdateScheduleInput, optFns ...func(*awsscheduler.Options)) (*awsscheduler.UpdateScheduleOutput, error)
	DeleteSchedule(ctx context.Context, params *awsscheduler.DeleteScheduleInput, optFns ...func(*awsscheduler.Options)) (*awsscheduler.DeleteScheduleOutput, error)
}

// ScheduleReconciler reconciles Schedule objects.
type ScheduleReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	SchedulerClient ScheduleAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=schedules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=schedules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=schedules/finalizers,verbs=update

func (r *ScheduleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	s := &awsv1alpha1.Schedule{}
	if err := r.Get(ctx, req.NamespacedName, s); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !s.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(s, awsv1alpha1.FinalizerName) {
			if shouldAbandon(s) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(s, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, s)
			}
			if err := r.deleteSchedule(ctx, s); err != nil {
				logger.Error(err, "failed to delete schedule")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(s, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, s)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(s, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(s, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, s); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileSchedule(ctx, s); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, s, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

// resolveGroupName resolves the schedule group name from the ref, or ""
// (default group) when no ref is set.
func (r *ScheduleReconciler) resolveGroupName(ctx context.Context, s *awsv1alpha1.Schedule) (string, error) {
	ref := s.Spec.GroupRef
	if ref == nil {
		return "", nil
	}
	if ref.GroupName != "" {
		return ref.GroupName, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("groupRef must specify either name or groupName")
	}
	sg := &awsv1alpha1.ScheduleGroup{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: s.Namespace}, sg); err != nil {
		return "", err
	}
	if sg.Status.ARN == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("ScheduleGroup %s/%s has no ARN yet", s.Namespace, ref.Name)}
	}
	return sg.Spec.Name, nil
}

func (r *ScheduleReconciler) buildTarget(ctx context.Context, s *awsv1alpha1.Schedule) (*schedulertypes.Target, error) {
	roleARN, err := resolveIAMRoleARN(ctx, r.Client, s.Namespace, s.Spec.Target.RoleRef)
	if err != nil {
		return nil, err
	}
	target := &schedulertypes.Target{
		Arn:     aws.String(s.Spec.Target.ARN),
		RoleArn: aws.String(roleARN),
	}
	if s.Spec.Target.Input != "" {
		target.Input = aws.String(s.Spec.Target.Input)
	}
	if rp := s.Spec.Target.RetryPolicy; rp != nil {
		target.RetryPolicy = &schedulertypes.RetryPolicy{
			MaximumRetryAttempts:     rp.MaximumRetryAttempts,
			MaximumEventAgeInSeconds: rp.MaximumEventAgeInSeconds,
		}
	}
	if s.Spec.Target.DeadLetterARN != "" {
		target.DeadLetterConfig = &schedulertypes.DeadLetterConfig{
			Arn: aws.String(s.Spec.Target.DeadLetterARN),
		}
	}
	return target, nil
}

func (r *ScheduleReconciler) flexibleTimeWindow(s *awsv1alpha1.Schedule) *schedulertypes.FlexibleTimeWindow {
	ftw := &schedulertypes.FlexibleTimeWindow{
		Mode: schedulertypes.FlexibleTimeWindowMode(s.Spec.FlexibleTimeWindow.Mode),
	}
	if s.Spec.FlexibleTimeWindow.MaximumWindowInMinutes != nil {
		ftw.MaximumWindowInMinutes = s.Spec.FlexibleTimeWindow.MaximumWindowInMinutes
	}
	return ftw
}

func (r *ScheduleReconciler) scheduleState(s *awsv1alpha1.Schedule) schedulertypes.ScheduleState {
	if s.Spec.State == "" {
		return schedulertypes.ScheduleStateEnabled
	}
	return schedulertypes.ScheduleState(s.Spec.State)
}

func (r *ScheduleReconciler) reconcileSchedule(ctx context.Context, s *awsv1alpha1.Schedule) error {
	groupName, err := r.resolveGroupName(ctx, s)
	if err != nil {
		return err
	}
	target, err := r.buildTarget(ctx, s)
	if err != nil {
		return err
	}

	getInput := &awsscheduler.GetScheduleInput{Name: aws.String(s.Spec.Name)}
	if groupName != "" {
		getInput.GroupName = aws.String(groupName)
	}
	_, err = r.SchedulerClient.GetSchedule(ctx, getInput)

	if schedulerhelper.IsNotFound(err) {
		createInput := &awsscheduler.CreateScheduleInput{
			Name:               aws.String(s.Spec.Name),
			ScheduleExpression: aws.String(s.Spec.ScheduleExpression),
			FlexibleTimeWindow: r.flexibleTimeWindow(s),
			Target:             target,
			State:              r.scheduleState(s),
		}
		if groupName != "" {
			createInput.GroupName = aws.String(groupName)
		}
		if s.Spec.Timezone != "" {
			createInput.ScheduleExpressionTimezone = aws.String(s.Spec.Timezone)
		}
		if s.Spec.Description != "" {
			createInput.Description = aws.String(s.Spec.Description)
		}
		createOut, err := r.SchedulerClient.CreateSchedule(ctx, createInput)
		if err != nil {
			return fmt.Errorf("create schedule: %w", err)
		}
		s.Status.ARN = aws.ToString(createOut.ScheduleArn)
		if err := persistStatus(ctx, r.Client, s); err != nil {
			return fmt.Errorf("persist schedule ARN after create: %w", err)
		}
	} else if err != nil {
		return err
	} else if s.Status.ObservedGeneration != s.Generation {
		// UpdateSchedule replaces all fields, so send the full desired state.
		updateInput := &awsscheduler.UpdateScheduleInput{
			Name:               aws.String(s.Spec.Name),
			ScheduleExpression: aws.String(s.Spec.ScheduleExpression),
			FlexibleTimeWindow: r.flexibleTimeWindow(s),
			Target:             target,
			State:              r.scheduleState(s),
		}
		if groupName != "" {
			updateInput.GroupName = aws.String(groupName)
		}
		if s.Spec.Timezone != "" {
			updateInput.ScheduleExpressionTimezone = aws.String(s.Spec.Timezone)
		}
		if s.Spec.Description != "" {
			updateInput.Description = aws.String(s.Spec.Description)
		}
		updateOut, err := r.SchedulerClient.UpdateSchedule(ctx, updateInput)
		if err != nil {
			return fmt.Errorf("update schedule: %w", err)
		}
		s.Status.ARN = aws.ToString(updateOut.ScheduleArn)
	}

	s.Status.ObservedGeneration = s.Generation
	now := metav1.Now()
	s.Status.LastSyncTime = &now
	return r.setCondition(ctx, s, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "schedule reconciled")
}

func (r *ScheduleReconciler) deleteSchedule(ctx context.Context, s *awsv1alpha1.Schedule) error {
	// The schedule is identified by name + group name, both derivable from spec.
	input := &awsscheduler.DeleteScheduleInput{Name: aws.String(s.Spec.Name)}
	if ref := s.Spec.GroupRef; ref != nil {
		if ref.GroupName != "" {
			input.GroupName = aws.String(ref.GroupName)
		} else if ref.Name != "" {
			sg := &awsv1alpha1.ScheduleGroup{}
			err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: s.Namespace}, sg)
			if err == nil {
				input.GroupName = aws.String(sg.Spec.Name)
			}
			// If the group CR is already gone, the group deletion cascades to
			// its schedules; try the default-group delete anyway below.
		}
	}
	_, err := r.SchedulerClient.DeleteSchedule(ctx, input)
	if schedulerhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *ScheduleReconciler) setCondition(ctx context.Context, s *awsv1alpha1.Schedule, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&s.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: s.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, s); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *ScheduleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.Schedule{}).
		Complete(r)
}
