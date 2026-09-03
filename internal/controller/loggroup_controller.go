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
	awslogs "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	logshelper "github.com/konfig-io/konfig-konector/internal/aws/cloudwatchlogs"
)

// LogGroupReconciler reconciles LogGroup objects.
type LogGroupReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	LogsClient *awslogs.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=loggroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=loggroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=loggroups/finalizers,verbs=update

func (r *LogGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	lg := &awsv1alpha1.LogGroup{}
	if err := r.Get(ctx, req.NamespacedName, lg); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !lg.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(lg, awsv1alpha1.FinalizerName) {
			if shouldAbandon(lg) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(lg, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, lg)
			}
			if err := r.deleteLogGroup(ctx, lg); err != nil {
				logger.Error(err, "failed to delete LogGroup")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(lg, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, lg)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(lg, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(lg, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, lg); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileLogGroup(ctx, lg); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionLG(ctx, lg, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *LogGroupReconciler) reconcileLogGroup(ctx context.Context, lg *awsv1alpha1.LogGroup) error {
	if lg.Status.ARN != "" {
		out, err := r.LogsClient.DescribeLogGroups(ctx, &awslogs.DescribeLogGroupsInput{
			LogGroupNamePrefix: aws.String(lg.Spec.LogGroupName),
		})
		if err != nil && !logshelper.IsNotFound(err) {
			return fmt.Errorf("describe log group: %w", err)
		}
		for _, g := range out.LogGroups {
			if aws.ToString(g.LogGroupName) == lg.Spec.LogGroupName {
				lg.Status.ObservedGeneration = lg.Generation
				now := metav1.Now()
				lg.Status.LastSyncTime = &now
				if lg.Spec.RetentionInDays > 0 {
					if _, err2 := r.LogsClient.PutRetentionPolicy(ctx, &awslogs.PutRetentionPolicyInput{
						LogGroupName:    aws.String(lg.Spec.LogGroupName),
						RetentionInDays: aws.Int32(lg.Spec.RetentionInDays),
					}); err2 != nil {
						return fmt.Errorf("put retention policy: %w", err2)
					}
				}
				return r.setConditionLG(ctx, lg, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "LogGroup reconciled")
			}
		}
		lg.Status.ARN = ""
	}

	input := &awslogs.CreateLogGroupInput{
		LogGroupName: aws.String(lg.Spec.LogGroupName),
	}
	if lg.Spec.KMSKeyARN != "" {
		input.KmsKeyId = aws.String(lg.Spec.KMSKeyARN)
	}
	if len(lg.Spec.Tags) > 0 {
		input.Tags = lg.Spec.Tags
	}

	if _, err := r.LogsClient.CreateLogGroup(ctx, input); err != nil {
		return fmt.Errorf("create log group: %w", err)
	}

	if lg.Spec.RetentionInDays > 0 {
		if _, err := r.LogsClient.PutRetentionPolicy(ctx, &awslogs.PutRetentionPolicyInput{
			LogGroupName:    aws.String(lg.Spec.LogGroupName),
			RetentionInDays: aws.Int32(lg.Spec.RetentionInDays),
		}); err != nil {
			return fmt.Errorf("put retention policy: %w", err)
		}
	}

	out, err := r.LogsClient.DescribeLogGroups(ctx, &awslogs.DescribeLogGroupsInput{
		LogGroupNamePrefix: aws.String(lg.Spec.LogGroupName),
	})
	if err != nil {
		return fmt.Errorf("describe log group after create: %w", err)
	}
	for _, g := range out.LogGroups {
		if aws.ToString(g.LogGroupName) == lg.Spec.LogGroupName {
			lg.Status.ARN = aws.ToString(g.LogGroupArn)
		}
	}

	lg.Status.ObservedGeneration = lg.Generation
	now := metav1.Now()
	lg.Status.LastSyncTime = &now
	return r.setConditionLG(ctx, lg, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "LogGroup created")
}

func (r *LogGroupReconciler) deleteLogGroup(ctx context.Context, lg *awsv1alpha1.LogGroup) error {
	if lg.Spec.LogGroupName == "" {
		return nil
	}
	_, err := r.LogsClient.DeleteLogGroup(ctx, &awslogs.DeleteLogGroupInput{
		LogGroupName: aws.String(lg.Spec.LogGroupName),
	})
	if logshelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *LogGroupReconciler) setConditionLG(ctx context.Context, lg *awsv1alpha1.LogGroup, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&lg.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: lg.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, lg); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *LogGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.LogGroup{}).
		Complete(r)
}
