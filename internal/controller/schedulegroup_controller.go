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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	schedulerhelper "github.com/konfig-io/konfig-konector/internal/aws/scheduler"
)

// ScheduleGroupAWSAPI is the subset of the EventBridge Scheduler API used by
// this controller.
type ScheduleGroupAWSAPI interface {
	GetScheduleGroup(ctx context.Context, params *awsscheduler.GetScheduleGroupInput, optFns ...func(*awsscheduler.Options)) (*awsscheduler.GetScheduleGroupOutput, error)
	CreateScheduleGroup(ctx context.Context, params *awsscheduler.CreateScheduleGroupInput, optFns ...func(*awsscheduler.Options)) (*awsscheduler.CreateScheduleGroupOutput, error)
	DeleteScheduleGroup(ctx context.Context, params *awsscheduler.DeleteScheduleGroupInput, optFns ...func(*awsscheduler.Options)) (*awsscheduler.DeleteScheduleGroupOutput, error)
}

// ScheduleGroupReconciler reconciles ScheduleGroup objects.
type ScheduleGroupReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	SchedulerClient ScheduleGroupAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=schedulegroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=schedulegroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=schedulegroups/finalizers,verbs=update

func (r *ScheduleGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	sg := &awsv1alpha1.ScheduleGroup{}
	if err := r.Get(ctx, req.NamespacedName, sg); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !sg.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(sg, awsv1alpha1.FinalizerName) {
			if shouldAbandon(sg) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(sg, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, sg)
			}
			if err := r.deleteGroup(ctx, sg); err != nil {
				logger.Error(err, "failed to delete schedule group")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(sg, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, sg)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(sg, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(sg, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, sg); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileGroup(ctx, sg); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, sg, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *ScheduleGroupReconciler) reconcileGroup(ctx context.Context, sg *awsv1alpha1.ScheduleGroup) error {
	getOut, err := r.SchedulerClient.GetScheduleGroup(ctx, &awsscheduler.GetScheduleGroupInput{
		Name: aws.String(sg.Spec.Name),
	})
	if schedulerhelper.IsNotFound(err) {
		var tags []schedulertypes.Tag
		for k, v := range sg.Spec.Tags {
			tags = append(tags, schedulertypes.Tag{Key: aws.String(k), Value: aws.String(v)})
		}
		createOut, err := r.SchedulerClient.CreateScheduleGroup(ctx, &awsscheduler.CreateScheduleGroupInput{
			Name: aws.String(sg.Spec.Name),
			Tags: tags,
		})
		if err != nil {
			return fmt.Errorf("create schedule group: %w", err)
		}
		sg.Status.ARN = aws.ToString(createOut.ScheduleGroupArn)
		if err := persistStatus(ctx, r.Client, sg); err != nil {
			return fmt.Errorf("persist schedule group ARN after create: %w", err)
		}
	} else if err != nil {
		return err
	} else {
		sg.Status.ARN = aws.ToString(getOut.Arn)
	}

	sg.Status.ObservedGeneration = sg.Generation
	now := metav1.Now()
	sg.Status.LastSyncTime = &now
	return r.setCondition(ctx, sg, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "schedule group reconciled")
}

func (r *ScheduleGroupReconciler) deleteGroup(ctx context.Context, sg *awsv1alpha1.ScheduleGroup) error {
	// The group is identified by its spec name; no status identifier needed.
	_, err := r.SchedulerClient.DeleteScheduleGroup(ctx, &awsscheduler.DeleteScheduleGroupInput{
		Name: aws.String(sg.Spec.Name),
	})
	if schedulerhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *ScheduleGroupReconciler) setCondition(ctx context.Context, sg *awsv1alpha1.ScheduleGroup, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&sg.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: sg.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, sg); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *ScheduleGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.ScheduleGroup{}).
		Complete(r)
}
