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
	awscw "github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	cwhelper "github.com/konfig-io/konfig-konector/internal/aws/cloudwatch"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// CloudWatchAlarmReconciler reconciles CloudWatchAlarm objects.
type CloudWatchAlarmReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	CloudWatchClient *multi.CloudWatch
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudwatchalarms,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudwatchalarms/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudwatchalarms/finalizers,verbs=update

func (r *CloudWatchAlarmReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.CloudWatchAlarm{}
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
			if err := r.deleteAlarm(ctx, obj); err != nil {
				logger.Error(err, "failed to delete CloudWatchAlarm")
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

	if err := r.reconcileAlarm(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionCWA(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *CloudWatchAlarmReconciler) reconcileAlarm(ctx context.Context, obj *awsv1alpha1.CloudWatchAlarm) error {
	dims := make([]cwtypes.Dimension, 0, len(obj.Spec.Dimensions))
	for _, d := range obj.Spec.Dimensions {
		d := d
		dims = append(dims, cwtypes.Dimension{Name: &d.Name, Value: &d.Value})
	}

	input := &awscw.PutMetricAlarmInput{
		AlarmName:          aws.String(obj.Spec.AlarmName),
		ComparisonOperator: cwtypes.ComparisonOperator(obj.Spec.ComparisonOperator),
		EvaluationPeriods:  aws.Int32(obj.Spec.EvaluationPeriods),
	}
	if obj.Spec.MetricName != "" {
		input.MetricName = aws.String(obj.Spec.MetricName)
	}
	if obj.Spec.Namespace != "" {
		input.Namespace = aws.String(obj.Spec.Namespace)
	}
	if obj.Spec.Statistic != "" {
		input.Statistic = cwtypes.Statistic(obj.Spec.Statistic)
	}
	if obj.Spec.Period != nil {
		input.Period = obj.Spec.Period
	}
	if obj.Spec.Threshold != nil {
		input.Threshold = obj.Spec.Threshold
	}
	if obj.Spec.AlarmDescription != "" {
		input.AlarmDescription = aws.String(obj.Spec.AlarmDescription)
	}
	if obj.Spec.ActionsEnabled != nil {
		input.ActionsEnabled = obj.Spec.ActionsEnabled
	}
	if len(obj.Spec.AlarmActions) > 0 {
		input.AlarmActions = obj.Spec.AlarmActions
	}
	if len(obj.Spec.OKActions) > 0 {
		input.OKActions = obj.Spec.OKActions
	}
	if len(obj.Spec.InsufficientDataActions) > 0 {
		input.InsufficientDataActions = obj.Spec.InsufficientDataActions
	}
	if len(dims) > 0 {
		input.Dimensions = dims
	}
	if obj.Spec.DatapointsToAlarm != nil {
		input.DatapointsToAlarm = obj.Spec.DatapointsToAlarm
	}
	if obj.Spec.TreatMissingData != "" {
		input.TreatMissingData = aws.String(obj.Spec.TreatMissingData)
	}
	if obj.Spec.Unit != "" {
		input.Unit = cwtypes.StandardUnit(obj.Spec.Unit)
	}
	if len(obj.Spec.Tags) > 0 {
		for k, v := range obj.Spec.Tags {
			k, v := k, v
			input.Tags = append(input.Tags, cwtypes.Tag{Key: &k, Value: &v})
		}
	}

	_, err := r.CloudWatchClient.PutMetricAlarm(ctx, input)
	if err != nil {
		return fmt.Errorf("put cloudwatch metric alarm: %w", err)
	}

	// Fetch the alarm ARN and state.
	descOut, err := r.CloudWatchClient.DescribeAlarms(ctx, &awscw.DescribeAlarmsInput{
		AlarmNames: []string{obj.Spec.AlarmName},
	})
	if err == nil && len(descOut.MetricAlarms) > 0 {
		obj.Status.AlarmARN = aws.ToString(descOut.MetricAlarms[0].AlarmArn)
		obj.Status.AlarmState = string(descOut.MetricAlarms[0].StateValue)
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionCWA(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "CloudWatchAlarm reconciled")
}

func (r *CloudWatchAlarmReconciler) deleteAlarm(ctx context.Context, obj *awsv1alpha1.CloudWatchAlarm) error {
	_, err := r.CloudWatchClient.DeleteAlarms(ctx, &awscw.DeleteAlarmsInput{
		AlarmNames: []string{obj.Spec.AlarmName},
	})
	if cwhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *CloudWatchAlarmReconciler) setConditionCWA(ctx context.Context, obj *awsv1alpha1.CloudWatchAlarm, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *CloudWatchAlarmReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.CloudWatchAlarm{}).
		Complete(r)
}
