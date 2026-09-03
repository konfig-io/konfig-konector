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
	awsdax "github.com/aws/aws-sdk-go-v2/service/dax"
	daxtypes "github.com/aws/aws-sdk-go-v2/service/dax/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	daxhelper "github.com/konfig-io/konfig-konector/internal/aws/dax"
)

// DAXClusterReconciler reconciles DAXCluster objects.
type DAXClusterReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	DAXClient *awsdax.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=daxclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=daxclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=daxclusters/finalizers,verbs=update

func (r *DAXClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.DAXCluster{}
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
				logger.Error(err, "failed to delete DAXCluster")
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
		_ = r.setConditionDAX(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *DAXClusterReconciler) reconcileCluster(ctx context.Context, obj *awsv1alpha1.DAXCluster) error {
	if obj.Status.ClusterARN != "" {
		out, err := r.DAXClient.DescribeClusters(ctx, &awsdax.DescribeClustersInput{
			ClusterNames: []string{obj.Spec.ClusterName},
		})
		if err != nil && !daxhelper.IsNotFound(err) {
			return fmt.Errorf("describe dax cluster: %w", err)
		}
		if err == nil && len(out.Clusters) > 0 {
			cl := out.Clusters[0]
			obj.Status.Status = aws.ToString(cl.Status)
			obj.Status.ClusterARN = aws.ToString(cl.ClusterArn)
			if cl.ClusterDiscoveryEndpoint != nil {
				obj.Status.ClusterDiscoveryEndpoint = fmt.Sprintf("%s:%d", aws.ToString(cl.ClusterDiscoveryEndpoint.Address), cl.ClusterDiscoveryEndpoint.Port)
			}
			if aws.ToString(cl.Status) == "creating" || aws.ToString(cl.Status) == "modifying" {
				return nil
			}
			obj.Status.ObservedGeneration = obj.Generation
			now := metav1.Now()
			obj.Status.LastSyncTime = &now
			return r.setConditionDAX(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "DAXCluster reconciled")
		}
		obj.Status.ClusterARN = ""
	}

	tags := make([]daxtypes.Tag, 0, len(obj.Spec.Tags))
	for k, v := range obj.Spec.Tags {
		k, v := k, v
		tags = append(tags, daxtypes.Tag{Key: &k, Value: &v})
	}

	input := &awsdax.CreateClusterInput{
		ClusterName:       aws.String(obj.Spec.ClusterName),
		NodeType:          aws.String(obj.Spec.NodeType),
		ReplicationFactor: obj.Spec.ReplicationFactor,
		IamRoleArn:        aws.String(obj.Spec.IAMRoleARN),
		Tags:              tags,
	}
	if obj.Spec.SubnetGroupName != "" {
		input.SubnetGroupName = aws.String(obj.Spec.SubnetGroupName)
	}
	if len(obj.Spec.SecurityGroupIDs) > 0 {
		input.SecurityGroupIds = obj.Spec.SecurityGroupIDs
	}
	if obj.Spec.ParameterGroupName != "" {
		input.ParameterGroupName = aws.String(obj.Spec.ParameterGroupName)
	}
	if len(obj.Spec.AvailabilityZones) > 0 {
		input.AvailabilityZones = obj.Spec.AvailabilityZones
	}

	out, err := r.DAXClient.CreateCluster(ctx, input)
	if err != nil {
		return fmt.Errorf("create dax cluster: %w", err)
	}

	obj.Status.ClusterARN = aws.ToString(out.Cluster.ClusterArn)
	obj.Status.Status = aws.ToString(out.Cluster.Status)
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionDAX(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "DAXCluster created")
}

func (r *DAXClusterReconciler) deleteCluster(ctx context.Context, obj *awsv1alpha1.DAXCluster) error {
	if obj.Status.ClusterARN == "" {
		return nil
	}
	_, err := r.DAXClient.DeleteCluster(ctx, &awsdax.DeleteClusterInput{
		ClusterName: aws.String(obj.Spec.ClusterName),
	})
	if daxhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *DAXClusterReconciler) setConditionDAX(ctx context.Context, obj *awsv1alpha1.DAXCluster, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *DAXClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.DAXCluster{}).
		Complete(r)
}
