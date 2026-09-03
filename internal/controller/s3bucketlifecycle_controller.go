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
)

// S3BucketLifecycleReconciler reconciles S3BucketLifecycle objects.
type S3BucketLifecycleReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	S3Client *awss3.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=s3bucketlifecycles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=s3bucketlifecycles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=s3bucketlifecycles/finalizers,verbs=update

func (r *S3BucketLifecycleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.S3BucketLifecycle{}
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
			if err := r.deleteLifecycle(ctx, obj); err != nil {
				logger.Error(err, "failed to delete S3BucketLifecycle")
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

	if err := r.reconcileLifecycle(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionSBL(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *S3BucketLifecycleReconciler) reconcileLifecycle(ctx context.Context, obj *awsv1alpha1.S3BucketLifecycle) error {
	rules := make([]s3types.LifecycleRule, 0, len(obj.Spec.Rules))
	for _, rule := range obj.Spec.Rules {
		rule := rule
		lr := s3types.LifecycleRule{
			Status: s3types.ExpirationStatus(rule.Status),
		}
		if rule.ID != "" {
			lr.ID = aws.String(rule.ID)
		}
		if rule.Prefix != "" {
			lr.Filter = &s3types.LifecycleRuleFilterMemberPrefix{Value: rule.Prefix}
		}
		if rule.ExpirationDays != nil || rule.ExpirationDate != "" {
			exp := &s3types.LifecycleExpiration{}
			if rule.ExpirationDays != nil {
				exp.Days = rule.ExpirationDays
			}
			lr.Expiration = exp
		}
		if rule.NoncurrentVersionExpirationDays != nil {
			lr.NoncurrentVersionExpiration = &s3types.NoncurrentVersionExpiration{
				NoncurrentDays: rule.NoncurrentVersionExpirationDays,
			}
		}
		if rule.AbortIncompleteMultipartUploadDays != nil {
			lr.AbortIncompleteMultipartUpload = &s3types.AbortIncompleteMultipartUpload{
				DaysAfterInitiation: rule.AbortIncompleteMultipartUploadDays,
			}
		}
		for _, t := range rule.Transitions {
			t := t
			tr := s3types.Transition{
				StorageClass: s3types.TransitionStorageClass(t.StorageClass),
			}
			if t.Days != nil {
				tr.Days = t.Days
			}
			lr.Transitions = append(lr.Transitions, tr)
		}
		rules = append(rules, lr)
	}

	_, err := r.S3Client.PutBucketLifecycleConfiguration(ctx, &awss3.PutBucketLifecycleConfigurationInput{
		Bucket: aws.String(obj.Spec.BucketName),
		LifecycleConfiguration: &s3types.BucketLifecycleConfiguration{
			Rules: rules,
		},
	})
	if err != nil {
		return fmt.Errorf("put bucket lifecycle configuration: %w", err)
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionSBL(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "S3BucketLifecycle reconciled")
}

func (r *S3BucketLifecycleReconciler) deleteLifecycle(ctx context.Context, obj *awsv1alpha1.S3BucketLifecycle) error {
	_, err := r.S3Client.DeleteBucketLifecycle(ctx, &awss3.DeleteBucketLifecycleInput{
		Bucket: aws.String(obj.Spec.BucketName),
	})
	if err != nil {
		return fmt.Errorf("delete bucket lifecycle: %w", err)
	}
	return nil
}

func (r *S3BucketLifecycleReconciler) setConditionSBL(ctx context.Context, obj *awsv1alpha1.S3BucketLifecycle, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *S3BucketLifecycleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.S3BucketLifecycle{}).
		Complete(r)
}
