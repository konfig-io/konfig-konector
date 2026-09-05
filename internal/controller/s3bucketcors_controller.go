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

// S3BucketCORSReconciler reconciles S3BucketCORS objects.
type S3BucketCORSReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	S3Client *multi.S3
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=s3bucketcorses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=s3bucketcorses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=s3bucketcorses/finalizers,verbs=update

func (r *S3BucketCORSReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.S3BucketCORS{}
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
			if err := r.deleteCORS(ctx, obj); err != nil {
				logger.Error(err, "failed to delete S3BucketCORS")
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

	if err := r.reconcileCORS(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionSBC(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *S3BucketCORSReconciler) reconcileCORS(ctx context.Context, obj *awsv1alpha1.S3BucketCORS) error {
	rules := make([]s3types.CORSRule, 0, len(obj.Spec.CORSRules))
	for _, rule := range obj.Spec.CORSRules {
		rule := rule
		cr := s3types.CORSRule{
			AllowedOrigins: rule.AllowedOrigins,
			AllowedMethods: rule.AllowedMethods,
		}
		if rule.ID != "" {
			cr.ID = aws.String(rule.ID)
		}
		if len(rule.AllowedHeaders) > 0 {
			cr.AllowedHeaders = rule.AllowedHeaders
		}
		if len(rule.ExposeHeaders) > 0 {
			cr.ExposeHeaders = rule.ExposeHeaders
		}
		if rule.MaxAgeSeconds != nil {
			cr.MaxAgeSeconds = rule.MaxAgeSeconds
		}
		rules = append(rules, cr)
	}

	_, err := r.S3Client.PutBucketCors(ctx, &awss3.PutBucketCorsInput{
		Bucket: aws.String(obj.Spec.BucketName),
		CORSConfiguration: &s3types.CORSConfiguration{
			CORSRules: rules,
		},
	})
	if err != nil {
		return fmt.Errorf("put bucket cors: %w", err)
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionSBC(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "S3BucketCORS reconciled")
}

func (r *S3BucketCORSReconciler) deleteCORS(ctx context.Context, obj *awsv1alpha1.S3BucketCORS) error {
	_, err := r.S3Client.DeleteBucketCors(ctx, &awss3.DeleteBucketCorsInput{
		Bucket: aws.String(obj.Spec.BucketName),
	})
	if err != nil {
		return fmt.Errorf("delete bucket cors: %w", err)
	}
	return nil
}

func (r *S3BucketCORSReconciler) setConditionSBC(ctx context.Context, obj *awsv1alpha1.S3BucketCORS, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *S3BucketCORSReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.S3BucketCORS{}).
		Complete(r)
}
