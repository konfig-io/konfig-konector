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
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// S3BucketNotificationReconciler reconciles S3BucketNotification objects.
type S3BucketNotificationReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	S3Client *multi.S3
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=s3bucketnotifications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=s3bucketnotifications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=s3bucketnotifications/finalizers,verbs=update

func (r *S3BucketNotificationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.S3BucketNotification{}
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
			if err := r.deleteNotification(ctx, obj); err != nil {
				logger.Error(err, "failed to delete S3BucketNotification")
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

	if err := r.reconcileNotification(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionSBN(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *S3BucketNotificationReconciler) reconcileNotification(ctx context.Context, obj *awsv1alpha1.S3BucketNotification) error {
	config := s3types.NotificationConfiguration{}

	for _, lc := range obj.Spec.LambdaFunctionConfigurations {
		lc := lc
		item := s3types.LambdaFunctionConfiguration{
			LambdaFunctionArn: aws.String(lc.LambdaFunctionARN),
		}
		if lc.ID != "" {
			item.Id = aws.String(lc.ID)
		}
		for _, e := range lc.Events {
			item.Events = append(item.Events, s3types.Event(e))
		}
		config.LambdaFunctionConfigurations = append(config.LambdaFunctionConfigurations, item)
	}

	for _, tc := range obj.Spec.TopicConfigurations {
		tc := tc
		item := s3types.TopicConfiguration{
			TopicArn: aws.String(tc.TopicARN),
		}
		if tc.ID != "" {
			item.Id = aws.String(tc.ID)
		}
		for _, e := range tc.Events {
			item.Events = append(item.Events, s3types.Event(e))
		}
		config.TopicConfigurations = append(config.TopicConfigurations, item)
	}

	for _, qc := range obj.Spec.QueueConfigurations {
		qc := qc
		item := s3types.QueueConfiguration{
			QueueArn: aws.String(qc.QueueARN),
		}
		if qc.ID != "" {
			item.Id = aws.String(qc.ID)
		}
		for _, e := range qc.Events {
			item.Events = append(item.Events, s3types.Event(e))
		}
		config.QueueConfigurations = append(config.QueueConfigurations, item)
	}

	if obj.Spec.EventBridgeEnabled {
		config.EventBridgeConfiguration = &s3types.EventBridgeConfiguration{}
	}

	_, err := r.S3Client.PutBucketNotificationConfiguration(ctx, &awss3.PutBucketNotificationConfigurationInput{
		Bucket:                    aws.String(obj.Spec.BucketName),
		NotificationConfiguration: &config,
	})
	if err != nil {
		return fmt.Errorf("put bucket notification configuration: %w", err)
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionSBN(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "S3BucketNotification reconciled")
}

func (r *S3BucketNotificationReconciler) deleteNotification(ctx context.Context, obj *awsv1alpha1.S3BucketNotification) error {
	_, err := r.S3Client.PutBucketNotificationConfiguration(ctx, &awss3.PutBucketNotificationConfigurationInput{
		Bucket:                    aws.String(obj.Spec.BucketName),
		NotificationConfiguration: &s3types.NotificationConfiguration{},
	})
	if err != nil {
		return fmt.Errorf("clear bucket notification configuration: %w", err)
	}
	return nil
}

func (r *S3BucketNotificationReconciler) setConditionSBN(ctx context.Context, obj *awsv1alpha1.S3BucketNotification, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *S3BucketNotificationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.S3BucketNotification{}).
		Complete(r)
}
