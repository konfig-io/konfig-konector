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

// S3BucketReplicationReconciler reconciles S3BucketReplication objects.
type S3BucketReplicationReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	S3Client *multi.S3
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=s3bucketreplications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=s3bucketreplications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=s3bucketreplications/finalizers,verbs=update

func (r *S3BucketReplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.S3BucketReplication{}
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
			if err := r.deleteReplication(ctx, obj); err != nil {
				logger.Error(err, "failed to delete S3BucketReplication")
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

	if err := r.reconcileReplication(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionSBR(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *S3BucketReplicationReconciler) reconcileReplication(ctx context.Context, obj *awsv1alpha1.S3BucketReplication) error {
	rules := make([]s3types.ReplicationRule, 0, len(obj.Spec.Rules))
	for _, rule := range obj.Spec.Rules {
		rule := rule
		rr := s3types.ReplicationRule{
			Status: s3types.ReplicationRuleStatus(rule.Status),
			Destination: &s3types.Destination{
				Bucket: aws.String(rule.Destination.Bucket),
			},
		}
		if rule.ID != "" {
			rr.ID = aws.String(rule.ID)
		}
		if rule.Prefix != "" {
			rr.Filter = &s3types.ReplicationRuleFilter{Prefix: aws.String(rule.Prefix)}
		}
		if rule.Priority != nil {
			rr.Priority = rule.Priority
		}
		if rule.Destination.StorageClass != "" {
			rr.Destination.StorageClass = s3types.StorageClass(rule.Destination.StorageClass)
		}
		if rule.Destination.Account != "" {
			rr.Destination.Account = aws.String(rule.Destination.Account)
		}
		rules = append(rules, rr)
	}

	_, err := r.S3Client.PutBucketReplication(ctx, &awss3.PutBucketReplicationInput{
		Bucket: aws.String(obj.Spec.BucketName),
		ReplicationConfiguration: &s3types.ReplicationConfiguration{
			Role:  aws.String(obj.Spec.RoleARN),
			Rules: rules,
		},
	})
	if err != nil {
		return fmt.Errorf("put bucket replication: %w", err)
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionSBR(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "S3BucketReplication reconciled")
}

func (r *S3BucketReplicationReconciler) deleteReplication(ctx context.Context, obj *awsv1alpha1.S3BucketReplication) error {
	_, err := r.S3Client.DeleteBucketReplication(ctx, &awss3.DeleteBucketReplicationInput{
		Bucket: aws.String(obj.Spec.BucketName),
	})
	if err != nil {
		return fmt.Errorf("delete bucket replication: %w", err)
	}
	return nil
}

func (r *S3BucketReplicationReconciler) setConditionSBR(ctx context.Context, obj *awsv1alpha1.S3BucketReplication, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *S3BucketReplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.S3BucketReplication{}).
		Complete(r)
}
