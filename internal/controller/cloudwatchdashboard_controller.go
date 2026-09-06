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

// CloudWatchDashboardReconciler reconciles CloudWatchDashboard objects.
type CloudWatchDashboardReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	CloudWatchClient *multi.CloudWatch
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudwatchdashboards,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudwatchdashboards/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudwatchdashboards/finalizers,verbs=update

func (r *CloudWatchDashboardReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.CloudWatchDashboard{}
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
			if err := r.deleteDashboard(ctx, obj); err != nil {
				logger.Error(err, "failed to delete CloudWatchDashboard")
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

	if err := r.reconcileDashboard(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionCWD(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *CloudWatchDashboardReconciler) reconcileDashboard(ctx context.Context, obj *awsv1alpha1.CloudWatchDashboard) error {
	out, err := r.CloudWatchClient.PutDashboard(ctx, &awscw.PutDashboardInput{
		DashboardName: aws.String(obj.Spec.DashboardName),
		DashboardBody: aws.String(obj.Spec.DashboardBody),
	})
	if err != nil {
		return fmt.Errorf("put cloudwatch dashboard: %w", err)
	}
	_ = out

	// GetDashboard to fetch the ARN.
	getOut, err := r.CloudWatchClient.GetDashboard(ctx, &awscw.GetDashboardInput{
		DashboardName: aws.String(obj.Spec.DashboardName),
	})
	if err == nil {
		obj.Status.DashboardARN = aws.ToString(getOut.DashboardArn)
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionCWD(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "CloudWatchDashboard reconciled")
}

func (r *CloudWatchDashboardReconciler) deleteDashboard(ctx context.Context, obj *awsv1alpha1.CloudWatchDashboard) error {
	_, err := r.CloudWatchClient.DeleteDashboards(ctx, &awscw.DeleteDashboardsInput{
		DashboardNames: []string{obj.Spec.DashboardName},
	})
	if cwhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *CloudWatchDashboardReconciler) setConditionCWD(ctx context.Context, obj *awsv1alpha1.CloudWatchDashboard, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *CloudWatchDashboardReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.CloudWatchDashboard{}).
		Complete(r)
}
