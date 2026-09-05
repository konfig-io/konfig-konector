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
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
	s3helper "github.com/konfig-io/konfig-konector/internal/aws/s3"
)

// S3BucketPolicyReconciler reconciles S3BucketPolicy objects.
type S3BucketPolicyReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	S3Client *multi.S3
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=s3bucketpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=s3bucketpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=s3bucketpolicies/finalizers,verbs=update

func (r *S3BucketPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	bp := &awsv1alpha1.S3BucketPolicy{}
	if err := r.Get(ctx, req.NamespacedName, bp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, bp); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !bp.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(bp, awsv1alpha1.FinalizerName) {
			if shouldAbandon(bp) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(bp, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, bp)
			}
			if err := r.deletePolicy(ctx, bp); err != nil {
				logger.Error(err, "failed to delete S3 bucket policy")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(bp, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, bp)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(bp, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(bp, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, bp); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcilePolicy(ctx, bp); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, bp, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *S3BucketPolicyReconciler) reconcilePolicy(ctx context.Context, bp *awsv1alpha1.S3BucketPolicy) error {
	bucketName, err := r.resolveBucketName(ctx, bp.Namespace, bp.Spec.BucketRef)
	if err != nil {
		return err
	}

	if _, err := r.S3Client.PutBucketPolicy(ctx, &awss3.PutBucketPolicyInput{
		Bucket: aws.String(bucketName),
		Policy: aws.String(bp.Spec.PolicyDocument),
	}); err != nil {
		return fmt.Errorf("put bucket policy: %w", err)
	}

	bp.Status.ObservedGeneration = bp.Generation
	now := metav1.Now()
	bp.Status.LastSyncTime = &now
	return r.setCondition(ctx, bp, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "S3 bucket policy applied")
}

func (r *S3BucketPolicyReconciler) resolveBucketName(ctx context.Context, namespace string, ref awsv1alpha1.S3BucketRef) (string, error) {
	if ref.BucketName != "" {
		return ref.BucketName, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("bucketRef must specify either name or bucketName")
	}
	bucketCR := &awsv1alpha1.S3Bucket{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, bucketCR); err != nil {
		return "", err
	}
	if bucketCR.Spec.BucketName == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("S3Bucket %s/%s not ready", namespace, ref.Name)}
	}
	return bucketCR.Spec.BucketName, nil
}

func (r *S3BucketPolicyReconciler) deletePolicy(ctx context.Context, bp *awsv1alpha1.S3BucketPolicy) error {
	bucketName, err := r.resolveBucketName(ctx, bp.Namespace, bp.Spec.BucketRef)
	if err != nil {
		// If dependency is gone, nothing to delete.
		return nil
	}
	_, err = r.S3Client.DeleteBucketPolicy(ctx, &awss3.DeleteBucketPolicyInput{
		Bucket: aws.String(bucketName),
	})
	if s3helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *S3BucketPolicyReconciler) setCondition(ctx context.Context, bp *awsv1alpha1.S3BucketPolicy, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&bp.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: bp.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, bp); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *S3BucketPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.S3BucketPolicy{}).
		Complete(r)
}
