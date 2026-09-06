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
	awselasticache "github.com/aws/aws-sdk-go-v2/service/elasticache"
	elasticachetypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	elasticachehelper "github.com/konfig-io/konfig-konector/internal/aws/elasticache"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// ElastiCacheServerlessCacheReconciler reconciles ElastiCacheServerlessCache objects.
type ElastiCacheServerlessCacheReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	ElastiCacheClient *multi.ElastiCache
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=elasticacheserverlesscaches,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=elasticacheserverlesscaches/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=elasticacheserverlesscaches/finalizers,verbs=update

func (r *ElastiCacheServerlessCacheReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.ElastiCacheServerlessCache{}
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
			if err := r.deleteServerlessCache(ctx, obj); err != nil {
				logger.Error(err, "failed to delete ElastiCacheServerlessCache")
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

	if err := r.reconcileServerlessCache(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionESC(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *ElastiCacheServerlessCacheReconciler) reconcileServerlessCache(ctx context.Context, obj *awsv1alpha1.ElastiCacheServerlessCache) error {
	if obj.Status.ARN != "" {
		out, err := r.ElastiCacheClient.DescribeServerlessCaches(ctx, &awselasticache.DescribeServerlessCachesInput{
			ServerlessCacheName: aws.String(obj.Spec.ServerlessCacheName),
		})
		if err != nil && !elasticachehelper.IsNotFound(err) {
			return fmt.Errorf("describe serverless cache: %w", err)
		}
		if err == nil && len(out.ServerlessCaches) > 0 {
			sc := out.ServerlessCaches[0]
			obj.Status.Status = aws.ToString(sc.Status)
			obj.Status.ARN = aws.ToString(sc.ARN)
			if sc.Endpoint != nil {
				obj.Status.Endpoint = fmt.Sprintf("%s:%d", aws.ToString(sc.Endpoint.Address), sc.Endpoint.Port)
			}
			if elasticachehelper.IsTransient(obj.Status.Status) {
				return nil
			}
			obj.Status.ObservedGeneration = obj.Generation
			now := metav1.Now()
			obj.Status.LastSyncTime = &now
			return r.setConditionESC(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "ElastiCacheServerlessCache reconciled")
		}
		obj.Status.ARN = ""
	}

	tags := make([]elasticachetypes.Tag, 0, len(obj.Spec.Tags))
	for k, v := range obj.Spec.Tags {
		k, v := k, v
		tags = append(tags, elasticachetypes.Tag{Key: &k, Value: &v})
	}

	input := &awselasticache.CreateServerlessCacheInput{
		ServerlessCacheName: aws.String(obj.Spec.ServerlessCacheName),
		Engine:              aws.String(obj.Spec.Engine),
		Tags:                tags,
	}
	if obj.Spec.Description != "" {
		input.Description = aws.String(obj.Spec.Description)
	}
	if obj.Spec.KMSKeyID != "" {
		input.KmsKeyId = aws.String(obj.Spec.KMSKeyID)
	}
	if len(obj.Spec.SecurityGroupIDs) > 0 {
		input.SecurityGroupIds = obj.Spec.SecurityGroupIDs
	}
	if len(obj.Spec.SubnetIDs) > 0 {
		input.SubnetIds = obj.Spec.SubnetIDs
	}
	if obj.Spec.SnapshotRetentionLimit != nil {
		input.SnapshotRetentionLimit = obj.Spec.SnapshotRetentionLimit
	}

	out, err := r.ElastiCacheClient.CreateServerlessCache(ctx, input)
	if err != nil {
		return fmt.Errorf("create serverless cache: %w", err)
	}

	obj.Status.ARN = aws.ToString(out.ServerlessCache.ARN)
	obj.Status.Status = aws.ToString(out.ServerlessCache.Status)
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionESC(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "ElastiCacheServerlessCache created")
}

func (r *ElastiCacheServerlessCacheReconciler) deleteServerlessCache(ctx context.Context, obj *awsv1alpha1.ElastiCacheServerlessCache) error {
	_, err := r.ElastiCacheClient.DeleteServerlessCache(ctx, &awselasticache.DeleteServerlessCacheInput{
		ServerlessCacheName: aws.String(obj.Spec.ServerlessCacheName),
	})
	if elasticachehelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *ElastiCacheServerlessCacheReconciler) setConditionESC(ctx context.Context, obj *awsv1alpha1.ElastiCacheServerlessCache, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *ElastiCacheServerlessCacheReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.ElastiCacheServerlessCache{}).
		Complete(r)
}
