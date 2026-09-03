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
	awscf "github.com/aws/aws-sdk-go-v2/service/cloudfront"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	cfhelper "github.com/konfig-io/konfig-konector/internal/aws/cloudfront"
)

// CloudFrontCachePolicyReconciler reconciles CloudFrontCachePolicy objects.
type CloudFrontCachePolicyReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	CloudFrontClient *awscf.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudfrontcachepolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudfrontcachepolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudfrontcachepolicies/finalizers,verbs=update

func (r *CloudFrontCachePolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.CloudFrontCachePolicy{}
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
			if err := r.deleteCachePolicy(ctx, obj); err != nil {
				logger.Error(err, "failed to delete CloudFrontCachePolicy")
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

	if err := r.reconcileCachePolicy(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionCFCP(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func buildCachePolicyConfig(obj *awsv1alpha1.CloudFrontCachePolicy) *cftypes.CachePolicyConfig {
	cfg := &cftypes.CachePolicyConfig{
		Name:   aws.String(obj.Spec.Name),
		MinTTL: aws.Int64(obj.Spec.MinTTL),
	}
	if obj.Spec.DefaultTTL != nil {
		cfg.DefaultTTL = obj.Spec.DefaultTTL
	}
	if obj.Spec.MaxTTL != nil {
		cfg.MaxTTL = obj.Spec.MaxTTL
	}
	if obj.Spec.Comment != "" {
		cfg.Comment = aws.String(obj.Spec.Comment)
	}
	return cfg
}

func (r *CloudFrontCachePolicyReconciler) reconcileCachePolicy(ctx context.Context, obj *awsv1alpha1.CloudFrontCachePolicy) error {
	if obj.Status.PolicyID != "" {
		getOut, err := r.CloudFrontClient.GetCachePolicy(ctx, &awscf.GetCachePolicyInput{
			Id: aws.String(obj.Status.PolicyID),
		})
		if err != nil && !cfhelper.IsNotFound(err) {
			return fmt.Errorf("get cloudfront cache policy: %w", err)
		}
		if err == nil && getOut.CachePolicy != nil {
			obj.Status.ETag = aws.ToString(getOut.ETag)
			_, err := r.CloudFrontClient.UpdateCachePolicy(ctx, &awscf.UpdateCachePolicyInput{
				Id:                aws.String(obj.Status.PolicyID),
				IfMatch:           aws.String(obj.Status.ETag),
				CachePolicyConfig: buildCachePolicyConfig(obj),
			})
			if err != nil {
				return fmt.Errorf("update cloudfront cache policy: %w", err)
			}
			obj.Status.ObservedGeneration = obj.Generation
			now := metav1.Now()
			obj.Status.LastSyncTime = &now
			return r.setConditionCFCP(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "CloudFrontCachePolicy reconciled")
		}
		obj.Status.PolicyID = ""
	}

	out, err := r.CloudFrontClient.CreateCachePolicy(ctx, &awscf.CreateCachePolicyInput{
		CachePolicyConfig: buildCachePolicyConfig(obj),
	})
	if err != nil {
		return fmt.Errorf("create cloudfront cache policy: %w", err)
	}
	if out.CachePolicy != nil {
		obj.Status.PolicyID = aws.ToString(out.CachePolicy.Id)
	}
	obj.Status.ETag = aws.ToString(out.ETag)
	// The AWS resource now exists; losing the ID would orphan it.
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("persist status after create: %w", err)
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionCFCP(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "CloudFrontCachePolicy created")
}

// findCachePolicyIDByName looks up a custom cache policy by its unique name.
// CloudFront enforces unique cache policy names per account, so an exact
// name match on a custom policy is the one this CR created.
// Returns "" when no match is found.
func (r *CloudFrontCachePolicyReconciler) findCachePolicyIDByName(ctx context.Context, name string) (string, error) {
	var marker *string
	for {
		listOut, err := r.CloudFrontClient.ListCachePolicies(ctx, &awscf.ListCachePoliciesInput{
			Type:   cftypes.CachePolicyTypeCustom,
			Marker: marker,
		})
		if err != nil {
			return "", fmt.Errorf("list cloudfront cache policies: %w", err)
		}
		if listOut.CachePolicyList == nil {
			return "", nil
		}
		for _, item := range listOut.CachePolicyList.Items {
			if item.CachePolicy == nil || item.CachePolicy.CachePolicyConfig == nil {
				continue
			}
			if aws.ToString(item.CachePolicy.CachePolicyConfig.Name) == name {
				return aws.ToString(item.CachePolicy.Id), nil
			}
		}
		if listOut.CachePolicyList.NextMarker == nil {
			return "", nil
		}
		marker = listOut.CachePolicyList.NextMarker
	}
}

func (r *CloudFrontCachePolicyReconciler) deleteCachePolicy(ctx context.Context, obj *awsv1alpha1.CloudFrontCachePolicy) error {
	if obj.Status.PolicyID == "" {
		// Status may have been lost after a successful create; fall back to
		// looking the policy up by its unique name before giving up.
		id, err := r.findCachePolicyIDByName(ctx, obj.Spec.Name)
		if err != nil {
			return err
		}
		if id == "" {
			return nil
		}
		obj.Status.PolicyID = id
	}
	etag := obj.Status.ETag
	if etag == "" {
		getOut, err := r.CloudFrontClient.GetCachePolicy(ctx, &awscf.GetCachePolicyInput{
			Id: aws.String(obj.Status.PolicyID),
		})
		if cfhelper.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		etag = aws.ToString(getOut.ETag)
	}
	_, err := r.CloudFrontClient.DeleteCachePolicy(ctx, &awscf.DeleteCachePolicyInput{
		Id:      aws.String(obj.Status.PolicyID),
		IfMatch: aws.String(etag),
	})
	if cfhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *CloudFrontCachePolicyReconciler) setConditionCFCP(ctx context.Context, obj *awsv1alpha1.CloudFrontCachePolicy, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *CloudFrontCachePolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.CloudFrontCachePolicy{}).
		Complete(r)
}
