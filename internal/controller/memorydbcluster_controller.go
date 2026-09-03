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
	awsmemorydb "github.com/aws/aws-sdk-go-v2/service/memorydb"
	memorydbtypes "github.com/aws/aws-sdk-go-v2/service/memorydb/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	memorydbhelper "github.com/konfig-io/konfig-konector/internal/aws/memorydb"
)

// MemoryDBClusterReconciler reconciles MemoryDBCluster objects.
type MemoryDBClusterReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	MemoryDBClient *awsmemorydb.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=memorydbclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=memorydbclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=memorydbclusters/finalizers,verbs=update

func (r *MemoryDBClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.MemoryDBCluster{}
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
			if err := r.deleteCluster(ctx, obj); err != nil {
				logger.Error(err, "failed to delete MemoryDBCluster")
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

	if err := r.reconcileCluster(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionMDB(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *MemoryDBClusterReconciler) reconcileCluster(ctx context.Context, obj *awsv1alpha1.MemoryDBCluster) error {
	if obj.Status.ARN != "" {
		out, err := r.MemoryDBClient.DescribeClusters(ctx, &awsmemorydb.DescribeClustersInput{
			ClusterName: aws.String(obj.Spec.ClusterName),
		})
		if err != nil && !memorydbhelper.IsNotFound(err) {
			return fmt.Errorf("describe memorydb cluster: %w", err)
		}
		if err == nil && len(out.Clusters) > 0 {
			cl := out.Clusters[0]
			obj.Status.Status = aws.ToString(cl.Status)
			obj.Status.ARN = aws.ToString(cl.ARN)
			if cl.ClusterEndpoint != nil {
				obj.Status.Endpoint = fmt.Sprintf("%s:%d", aws.ToString(cl.ClusterEndpoint.Address), cl.ClusterEndpoint.Port)
			}
			if aws.ToString(cl.Status) == "creating" || aws.ToString(cl.Status) == "updating" {
				return nil
			}
			obj.Status.ObservedGeneration = obj.Generation
			now := metav1.Now()
			obj.Status.LastSyncTime = &now
			return r.setConditionMDB(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "MemoryDBCluster reconciled")
		}
		obj.Status.ARN = ""
	}

	tags := make([]memorydbtypes.Tag, 0, len(obj.Spec.Tags))
	for k, v := range obj.Spec.Tags {
		k, v := k, v
		tags = append(tags, memorydbtypes.Tag{Key: &k, Value: &v})
	}

	input := &awsmemorydb.CreateClusterInput{
		ClusterName: aws.String(obj.Spec.ClusterName),
		NodeType:    aws.String(obj.Spec.NodeType),
		ACLName:     aws.String(obj.Spec.ACLName),
		Tags:        tags,
	}
	if obj.Spec.NumShards != nil {
		input.NumShards = obj.Spec.NumShards
	}
	if obj.Spec.NumReplicasPerShard != nil {
		input.NumReplicasPerShard = obj.Spec.NumReplicasPerShard
	}
	if obj.Spec.SubnetGroupName != "" {
		input.SubnetGroupName = aws.String(obj.Spec.SubnetGroupName)
	}
	if len(obj.Spec.SecurityGroupIDs) > 0 {
		input.SecurityGroupIds = obj.Spec.SecurityGroupIDs
	}
	if obj.Spec.EngineVersion != "" {
		input.EngineVersion = aws.String(obj.Spec.EngineVersion)
	}
	if obj.Spec.SnapshotRetentionLimit != nil {
		input.SnapshotRetentionLimit = obj.Spec.SnapshotRetentionLimit
	}
	if obj.Spec.TLSEnabled != nil {
		input.TLSEnabled = obj.Spec.TLSEnabled
	}
	if obj.Spec.KMSKeyID != "" {
		input.KmsKeyId = aws.String(obj.Spec.KMSKeyID)
	}

	out, err := r.MemoryDBClient.CreateCluster(ctx, input)
	if err != nil {
		return fmt.Errorf("create memorydb cluster: %w", err)
	}

	obj.Status.ARN = aws.ToString(out.Cluster.ARN)
	obj.Status.Status = aws.ToString(out.Cluster.Status)
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionMDB(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "MemoryDBCluster created")
}

func (r *MemoryDBClusterReconciler) deleteCluster(ctx context.Context, obj *awsv1alpha1.MemoryDBCluster) error {
	if obj.Status.ARN == "" {
		return nil
	}
	_, err := r.MemoryDBClient.DeleteCluster(ctx, &awsmemorydb.DeleteClusterInput{
		ClusterName: aws.String(obj.Spec.ClusterName),
	})
	if memorydbhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *MemoryDBClusterReconciler) setConditionMDB(ctx context.Context, obj *awsv1alpha1.MemoryDBCluster, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *MemoryDBClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.MemoryDBCluster{}).
		Complete(r)
}
