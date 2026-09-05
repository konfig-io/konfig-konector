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
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// SubscriptionFilterReconciler reconciles SubscriptionFilter objects.
type SubscriptionFilterReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	LogsClient *multi.CloudWatchLogs
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=subscriptionfilters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=subscriptionfilters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=subscriptionfilters/finalizers,verbs=update

func (r *SubscriptionFilterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	sf := &awsv1alpha1.SubscriptionFilter{}
	if err := r.Get(ctx, req.NamespacedName, sf); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, sf); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !sf.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(sf, awsv1alpha1.FinalizerName) {
			if shouldAbandon(sf) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(sf, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, sf)
			}
			if err := r.deleteSubscriptionFilter(ctx, sf); err != nil {
				logger.Error(err, "failed to delete SubscriptionFilter")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(sf, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, sf)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(sf, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(sf, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, sf); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileSubscriptionFilter(ctx, sf); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionSF(ctx, sf, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *SubscriptionFilterReconciler) reconcileSubscriptionFilter(ctx context.Context, sf *awsv1alpha1.SubscriptionFilter) error {
	logGroupName, err := r.resolveLogGroupNameSF(ctx, sf.Spec.LogGroupRef, sf.Namespace)
	if err != nil {
		return err
	}

	input := &awslogs.PutSubscriptionFilterInput{
		LogGroupName:   aws.String(logGroupName),
		FilterName:     aws.String(sf.Spec.FilterName),
		FilterPattern:  aws.String(sf.Spec.FilterPattern),
		DestinationArn: aws.String(sf.Spec.DestinationARN),
	}
	if sf.Spec.RoleARN != "" {
		input.RoleArn = aws.String(sf.Spec.RoleARN)
	}

	_, err = r.LogsClient.PutSubscriptionFilter(ctx, input)
	if err != nil {
		return fmt.Errorf("put subscription filter: %w", err)
	}

	sf.Status.ObservedGeneration = sf.Generation
	now := metav1.Now()
	sf.Status.LastSyncTime = &now
	return r.setConditionSF(ctx, sf, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "SubscriptionFilter reconciled")
}

func (r *SubscriptionFilterReconciler) resolveLogGroupNameSF(ctx context.Context, ref awsv1alpha1.LogGroupRef, namespace string) (string, error) {
	if ref.LogGroupName != "" {
		return ref.LogGroupName, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("logGroupRef requires name or logGroupName")
	}
	lgCR := &awsv1alpha1.LogGroup{}
	if err := r.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: namespace}, lgCR); err != nil {
		return "", err
	}
	if lgCR.Status.ARN == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("LogGroup %s/%s has no ARN yet", namespace, ref.Name)}
	}
	return lgCR.Spec.LogGroupName, nil
}

func (r *SubscriptionFilterReconciler) deleteSubscriptionFilter(ctx context.Context, sf *awsv1alpha1.SubscriptionFilter) error {
	logGroupName, err := r.resolveLogGroupNameSF(ctx, sf.Spec.LogGroupRef, sf.Namespace)
	if err != nil {
		return nil
	}
	_, err = r.LogsClient.DeleteSubscriptionFilter(ctx, &awslogs.DeleteSubscriptionFilterInput{
		LogGroupName: aws.String(logGroupName),
		FilterName:   aws.String(sf.Spec.FilterName),
	})
	if logshelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *SubscriptionFilterReconciler) setConditionSF(ctx context.Context, sf *awsv1alpha1.SubscriptionFilter, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&sf.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: sf.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, sf); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *SubscriptionFilterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.SubscriptionFilter{}).
		Complete(r)
}
