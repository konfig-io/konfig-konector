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
	logstypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	logshelper "github.com/konfig-io/konfig-konector/internal/aws/cloudwatchlogs"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// MetricFilterReconciler reconciles MetricFilter objects.
type MetricFilterReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	LogsClient *multi.CloudWatchLogs
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=metricfilters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=metricfilters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=metricfilters/finalizers,verbs=update

func (r *MetricFilterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	mf := &awsv1alpha1.MetricFilter{}
	if err := r.Get(ctx, req.NamespacedName, mf); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, mf); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !mf.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(mf, awsv1alpha1.FinalizerName) {
			if shouldAbandon(mf) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(mf, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, mf)
			}
			if err := r.deleteMetricFilter(ctx, mf); err != nil {
				logger.Error(err, "failed to delete MetricFilter")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(mf, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, mf)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(mf, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(mf, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, mf); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileMetricFilter(ctx, mf); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionMF(ctx, mf, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *MetricFilterReconciler) reconcileMetricFilter(ctx context.Context, mf *awsv1alpha1.MetricFilter) error {
	logGroupName, err := r.resolveLogGroupName(ctx, mf.Spec.LogGroupRef, mf.Namespace)
	if err != nil {
		return err
	}

	transformations := make([]logstypes.MetricTransformation, 0, len(mf.Spec.MetricTransformations))
	for _, t := range mf.Spec.MetricTransformations {
		t := t
		mt := logstypes.MetricTransformation{
			MetricName:      aws.String(t.MetricName),
			MetricNamespace: aws.String(t.MetricNamespace),
			MetricValue:     aws.String(t.MetricValue),
		}
		if t.DefaultValue != nil {
			mt.DefaultValue = t.DefaultValue
		}
		transformations = append(transformations, mt)
	}

	_, err = r.LogsClient.PutMetricFilter(ctx, &awslogs.PutMetricFilterInput{
		LogGroupName:          aws.String(logGroupName),
		FilterName:            aws.String(mf.Spec.FilterName),
		FilterPattern:         aws.String(mf.Spec.FilterPattern),
		MetricTransformations: transformations,
	})
	if err != nil {
		return fmt.Errorf("put metric filter: %w", err)
	}

	mf.Status.ObservedGeneration = mf.Generation
	now := metav1.Now()
	mf.Status.LastSyncTime = &now
	return r.setConditionMF(ctx, mf, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "MetricFilter reconciled")
}

func (r *MetricFilterReconciler) resolveLogGroupName(ctx context.Context, ref awsv1alpha1.LogGroupRef, namespace string) (string, error) {
	if ref.LogGroupName != "" {
		return ref.LogGroupName, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("logGroupRef requires name or logGroupName")
	}
	lgCR := &awsv1alpha1.LogGroup{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, lgCR); err != nil {
		return "", err
	}
	if lgCR.Status.ARN == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("LogGroup %s/%s has no ARN yet", namespace, ref.Name)}
	}
	return lgCR.Spec.LogGroupName, nil
}

func (r *MetricFilterReconciler) deleteMetricFilter(ctx context.Context, mf *awsv1alpha1.MetricFilter) error {
	logGroupName, err := r.resolveLogGroupName(ctx, mf.Spec.LogGroupRef, mf.Namespace)
	if err != nil {
		return nil
	}
	_, err = r.LogsClient.DeleteMetricFilter(ctx, &awslogs.DeleteMetricFilterInput{
		LogGroupName: aws.String(logGroupName),
		FilterName:   aws.String(mf.Spec.FilterName),
	})
	if logshelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *MetricFilterReconciler) setConditionMF(ctx context.Context, mf *awsv1alpha1.MetricFilter, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&mf.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: mf.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, mf); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *MetricFilterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.MetricFilter{}).
		Complete(r)
}
