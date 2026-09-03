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
	awsevents "github.com/aws/aws-sdk-go-v2/service/eventbridge"
	eventtypes "github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	ebhelper "github.com/konfig-io/konfig-konector/internal/aws/eventbridge"
)

const ecsScheduledTaskTargetID = "ecs-scheduled-task"

// ECSScheduledTaskReconciler reconciles ECSScheduledTask objects.
type ECSScheduledTaskReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	EventBridgeClient *awsevents.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=ecsscheduledtasks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ecsscheduledtasks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ecsscheduledtasks/finalizers,verbs=update

func (r *ECSScheduledTaskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.ECSScheduledTask{}
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
			if err := r.deleteScheduledTask(ctx, obj); err != nil {
				logger.Error(err, "failed to delete ECSScheduledTask")
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

	if err := r.reconcileScheduledTask(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionEST(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *ECSScheduledTaskReconciler) reconcileScheduledTask(ctx context.Context, obj *awsv1alpha1.ECSScheduledTask) error {
	putRuleInput := &awsevents.PutRuleInput{
		Name:               aws.String(obj.Spec.RuleName),
		ScheduleExpression: aws.String(obj.Spec.ScheduleExpression),
		State:              eventtypes.RuleStateEnabled,
	}
	if obj.Spec.Description != "" {
		putRuleInput.Description = aws.String(obj.Spec.Description)
	}
	if obj.Spec.EventBusName != "" {
		putRuleInput.EventBusName = aws.String(obj.Spec.EventBusName)
	}
	if len(obj.Spec.Tags) > 0 {
		for k, v := range obj.Spec.Tags {
			k, v := k, v
			putRuleInput.Tags = append(putRuleInput.Tags, eventtypes.Tag{Key: &k, Value: &v})
		}
	}

	ruleOut, err := r.EventBridgeClient.PutRule(ctx, putRuleInput)
	if err != nil {
		return fmt.Errorf("put eventbridge rule: %w", err)
	}
	obj.Status.RuleARN = aws.ToString(ruleOut.RuleArn)

	ecsParams := &eventtypes.EcsParameters{
		TaskDefinitionArn: aws.String(obj.Spec.Target.TaskDefinitionARN),
	}
	if obj.Spec.Target.TaskCount != nil {
		ecsParams.TaskCount = obj.Spec.Target.TaskCount
	}
	if obj.Spec.Target.LaunchType != "" {
		ecsParams.LaunchType = eventtypes.LaunchType(obj.Spec.Target.LaunchType)
	}
	if obj.Spec.Target.PlatformVersion != "" {
		ecsParams.PlatformVersion = aws.String(obj.Spec.Target.PlatformVersion)
	}
	if nc := obj.Spec.Target.NetworkConfiguration; nc != nil {
		assignIP := eventtypes.AssignPublicIpDisabled
		if nc.AssignPublicIP == "ENABLED" {
			assignIP = eventtypes.AssignPublicIpEnabled
		}
		ecsParams.NetworkConfiguration = &eventtypes.NetworkConfiguration{
			AwsvpcConfiguration: &eventtypes.AwsVpcConfiguration{
				Subnets:        nc.Subnets,
				SecurityGroups: nc.SecurityGroups,
				AssignPublicIp: assignIP,
			},
		}
	}

	targetID := ecsScheduledTaskTargetID
	if obj.Status.TargetID != "" {
		targetID = obj.Status.TargetID
	}

	putTargetsInput := &awsevents.PutTargetsInput{
		Rule: aws.String(obj.Spec.RuleName),
		Targets: []eventtypes.Target{
			{
				Id:            aws.String(targetID),
				Arn:           aws.String(obj.Spec.Target.ClusterARN),
				RoleArn:       aws.String(obj.Spec.RoleARN),
				EcsParameters: ecsParams,
			},
		},
	}
	if obj.Spec.EventBusName != "" {
		putTargetsInput.EventBusName = aws.String(obj.Spec.EventBusName)
	}

	_, err = r.EventBridgeClient.PutTargets(ctx, putTargetsInput)
	if err != nil {
		return fmt.Errorf("put eventbridge targets: %w", err)
	}
	obj.Status.TargetID = targetID

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionEST(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "ECSScheduledTask reconciled")
}

func (r *ECSScheduledTaskReconciler) deleteScheduledTask(ctx context.Context, obj *awsv1alpha1.ECSScheduledTask) error {
	targetID := obj.Status.TargetID
	if targetID == "" {
		targetID = ecsScheduledTaskTargetID
	}

	removeInput := &awsevents.RemoveTargetsInput{
		Rule: aws.String(obj.Spec.RuleName),
		Ids:  []string{targetID},
	}
	if obj.Spec.EventBusName != "" {
		removeInput.EventBusName = aws.String(obj.Spec.EventBusName)
	}
	_, err := r.EventBridgeClient.RemoveTargets(ctx, removeInput)
	if err != nil && !ebhelper.IsNotFound(err) {
		return fmt.Errorf("remove eventbridge targets: %w", err)
	}

	deleteInput := &awsevents.DeleteRuleInput{
		Name: aws.String(obj.Spec.RuleName),
	}
	if obj.Spec.EventBusName != "" {
		deleteInput.EventBusName = aws.String(obj.Spec.EventBusName)
	}
	_, err = r.EventBridgeClient.DeleteRule(ctx, deleteInput)
	if ebhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *ECSScheduledTaskReconciler) setConditionEST(ctx context.Context, obj *awsv1alpha1.ECSScheduledTask, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *ECSScheduledTaskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.ECSScheduledTask{}).
		Complete(r)
}
