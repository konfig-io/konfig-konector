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
	awskafka "github.com/aws/aws-sdk-go-v2/service/kafka"
	kafkatypes "github.com/aws/aws-sdk-go-v2/service/kafka/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	kafkahelper "github.com/konfig-io/konfig-konector/internal/aws/kafka"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// MSKServerlessClusterReconciler reconciles MSKServerlessCluster objects.
type MSKServerlessClusterReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	KafkaClient *multi.Kafka
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=mskserverlessclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=mskserverlessclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=mskserverlessclusters/finalizers,verbs=update

func (r *MSKServerlessClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.MSKServerlessCluster{}
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
			if err := r.deleteCluster(ctx, obj); err != nil {
				logger.Error(err, "failed to delete MSKServerlessCluster")
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

	if err := r.reconcileCluster(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionMSKSL(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *MSKServerlessClusterReconciler) reconcileCluster(ctx context.Context, obj *awsv1alpha1.MSKServerlessCluster) error {
	if obj.Status.ClusterARN != "" {
		descOut, err := r.KafkaClient.DescribeClusterV2(ctx, &awskafka.DescribeClusterV2Input{
			ClusterArn: aws.String(obj.Status.ClusterARN),
		})
		if err != nil && !kafkahelper.IsNotFound(err) {
			return fmt.Errorf("describe msk serverless cluster: %w", err)
		}
		if err == nil && descOut.ClusterInfo != nil {
			obj.Status.ClusterState = string(descOut.ClusterInfo.State)
			if descOut.ClusterInfo.State == kafkatypes.ClusterStateActive {
				obj.Status.ObservedGeneration = obj.Generation
				now := metav1.Now()
				obj.Status.LastSyncTime = &now
				return r.setConditionMSKSL(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "MSKServerlessCluster active")
			}
			return r.setConditionMSKSL(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonCreated, fmt.Sprintf("cluster state: %s", descOut.ClusterInfo.State))
		}
		obj.Status.ClusterARN = ""
	}

	vpcConfigs := make([]kafkatypes.VpcConfig, 0, len(obj.Spec.VpcConfigs))
	for _, vc := range obj.Spec.VpcConfigs {
		vpcConfigs = append(vpcConfigs, kafkatypes.VpcConfig{
			SubnetIds:        vc.SubnetIDs,
			SecurityGroupIds: vc.SecurityGroupIDs,
		})
	}

	input := &awskafka.CreateClusterV2Input{
		ClusterName: aws.String(obj.Spec.ClusterName),
		Serverless: &kafkatypes.ServerlessRequest{
			VpcConfigs: vpcConfigs,
		},
	}
	if len(obj.Spec.Tags) > 0 {
		input.Tags = obj.Spec.Tags
	}

	out, err := r.KafkaClient.CreateClusterV2(ctx, input)
	if err != nil {
		return fmt.Errorf("create msk serverless cluster: %w", err)
	}

	obj.Status.ClusterARN = aws.ToString(out.ClusterArn)
	obj.Status.ClusterState = string(out.State)
	// The AWS resource now exists; losing the ARN would orphan it and a
	// retried create-by-name would conflict.
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		return fmt.Errorf("persist status after create: %w", err)
	}
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionMSKSL(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonCreated, "MSKServerlessCluster creating")
}

func (r *MSKServerlessClusterReconciler) deleteCluster(ctx context.Context, obj *awsv1alpha1.MSKServerlessCluster) error {
	if obj.Status.ClusterARN == "" {
		// Status may have been lost after a successful create; the create
		// path always uses spec.clusterName, so look it up before giving up.
		paginator := awskafka.NewListClustersV2Paginator(r.KafkaClient, &awskafka.ListClustersV2Input{
			ClusterNameFilter: aws.String(obj.Spec.ClusterName),
			ClusterTypeFilter: aws.String("SERVERLESS"),
		})
		for paginator.HasMorePages() && obj.Status.ClusterARN == "" {
			page, err := paginator.NextPage(ctx)
			if err != nil {
				return fmt.Errorf("list msk clusters: %w", err)
			}
			for _, c := range page.ClusterInfoList {
				if aws.ToString(c.ClusterName) == obj.Spec.ClusterName {
					obj.Status.ClusterARN = aws.ToString(c.ClusterArn)
					break
				}
			}
		}
		if obj.Status.ClusterARN == "" {
			return nil
		}
	}
	_, err := r.KafkaClient.DeleteCluster(ctx, &awskafka.DeleteClusterInput{
		ClusterArn: aws.String(obj.Status.ClusterARN),
	})
	if kafkahelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *MSKServerlessClusterReconciler) setConditionMSKSL(ctx context.Context, obj *awsv1alpha1.MSKServerlessCluster, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *MSKServerlessClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.MSKServerlessCluster{}).
		Complete(r)
}
