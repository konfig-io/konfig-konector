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

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	ecshelper "github.com/konfig-io/konfig-konector/internal/aws/ecs"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// ECSClusterReconciler reconciles ECSCluster objects.
type ECSClusterReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	ECSClient *multi.ECS
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=ecsclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ecsclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ecsclusters/finalizers,verbs=update

func (r *ECSClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	cluster := &awsv1alpha1.ECSCluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, cluster); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !cluster.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(cluster, awsv1alpha1.FinalizerName) {
			if shouldAbandon(cluster) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(cluster, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, cluster)
			}
			if err := ecshelper.DeleteCluster(ctx, r.ECSClient, cluster.Spec.ClusterName); err != nil && !ecshelper.IsNotFound(err) {
				logger.Error(err, "failed to delete ECS cluster")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(cluster, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, cluster)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(cluster, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(cluster, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, cluster); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	result, err := r.reconcileCluster(ctx, cluster)
	if err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, cluster, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *ECSClusterReconciler) reconcileCluster(ctx context.Context, cluster *awsv1alpha1.ECSCluster) (ctrl.Result, error) {
	if cluster.Status.ClusterARN != "" {
		existing, err := ecshelper.DescribeCluster(ctx, r.ECSClient, cluster.Spec.ClusterName)
		if err != nil {
			return ctrl.Result{}, err
		}
		if existing != nil {
			if existing.Status != nil {
				cluster.Status.Status = *existing.Status
			}
			if cluster.Status.Status == "PROVISIONING" {
				_ = r.setCondition(ctx, cluster, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning", "ECS cluster is PROVISIONING")
				return requeueDependency, nil
			}
			cluster.Status.ObservedGeneration = cluster.Generation
			now := metav1.Now()
			cluster.Status.LastSyncTime = &now
			return requeueResult(), r.setCondition(ctx, cluster, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "ECS cluster active")
		}
	}

	created, err := ecshelper.CreateCluster(ctx, r.ECSClient, ecshelper.CreateClusterInput{
		ClusterName:       cluster.Spec.ClusterName,
		CapacityProviders: cluster.Spec.CapacityProviders,
		InsightsEnabled:   cluster.Spec.ContainerInsights,
		Tags:              cluster.Spec.Tags,
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("create ECS cluster: %w", err)
	}
	if created.ClusterArn != nil {
		cluster.Status.ClusterARN = *created.ClusterArn
	}
	if created.Status != nil {
		cluster.Status.Status = *created.Status
	}
	cluster.Status.ObservedGeneration = cluster.Generation
	now := metav1.Now()
	cluster.Status.LastSyncTime = &now
	return requeueResult(), r.setCondition(ctx, cluster, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "ECS cluster created")
}

func (r *ECSClusterReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.ECSCluster, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *ECSClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.ECSCluster{}).
		Complete(r)
}
