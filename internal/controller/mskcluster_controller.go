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
)

// MSKClusterReconciler reconciles MSKCluster objects.
type MSKClusterReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	KafkaClient *awskafka.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=mskclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=mskclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=mskclusters/finalizers,verbs=update

func (r *MSKClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.MSKCluster{}
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
				logger.Error(err, "failed to delete MSKCluster")
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
		_ = r.setConditionMSK(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *MSKClusterReconciler) reconcileCluster(ctx context.Context, obj *awsv1alpha1.MSKCluster) error {
	if obj.Status.ClusterARN != "" {
		descOut, err := r.KafkaClient.DescribeCluster(ctx, &awskafka.DescribeClusterInput{
			ClusterArn: aws.String(obj.Status.ClusterARN),
		})
		if err != nil && !kafkahelper.IsNotFound(err) {
			return fmt.Errorf("describe msk cluster: %w", err)
		}
		if err == nil && descOut.ClusterInfo != nil {
			obj.Status.ClusterState = string(descOut.ClusterInfo.State)
			if descOut.ClusterInfo.State == kafkatypes.ClusterStateActive {
				if obj.Status.ObservedGeneration != obj.Generation {
					return r.updateCluster(ctx, obj, descOut.ClusterInfo)
				}
				obj.Status.ObservedGeneration = obj.Generation
				now := metav1.Now()
				obj.Status.LastSyncTime = &now
				return r.setConditionMSK(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "MSKCluster active")
			}
			return r.setConditionMSK(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonCreated, fmt.Sprintf("cluster state: %s", descOut.ClusterInfo.State))
		}
		obj.Status.ClusterARN = ""
	}

	bng := obj.Spec.BrokerNodeGroupInfo
	brokerInput := &kafkatypes.BrokerNodeGroupInfo{
		InstanceType:  aws.String(bng.InstanceType),
		ClientSubnets: bng.ClientSubnets,
	}
	if len(bng.SecurityGroups) > 0 {
		brokerInput.SecurityGroups = bng.SecurityGroups
	}
	if bng.StorageVolumeSizeGiB != nil {
		brokerInput.StorageInfo = &kafkatypes.StorageInfo{
			EbsStorageInfo: &kafkatypes.EBSStorageInfo{
				VolumeSize: bng.StorageVolumeSizeGiB,
			},
		}
	}

	input := &awskafka.CreateClusterInput{
		ClusterName:         aws.String(obj.Spec.ClusterName),
		KafkaVersion:        aws.String(obj.Spec.KafkaVersion),
		NumberOfBrokerNodes: aws.Int32(obj.Spec.NumberOfBrokerNodes),
		BrokerNodeGroupInfo: brokerInput,
	}
	if len(obj.Spec.Tags) > 0 {
		input.Tags = obj.Spec.Tags
	}

	out, err := r.KafkaClient.CreateCluster(ctx, input)
	if err != nil {
		return fmt.Errorf("create msk cluster: %w", err)
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
	return r.setConditionMSK(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonCreated, "MSKCluster creating")
}

// updateCluster reconciles the limited set of mutable MSK settings the spec
// models: broker count and broker instance type. Any other spec drift is
// surfaced honestly as UpdateNotSupported without bumping ObservedGeneration.
func (r *MSKClusterReconciler) updateCluster(ctx context.Context, obj *awsv1alpha1.MSKCluster, info *kafkatypes.ClusterInfo) error {
	// Detect drift in fields MSK cannot update via this controller.
	var unsupported []string
	if info.CurrentBrokerSoftwareInfo != nil && aws.ToString(info.CurrentBrokerSoftwareInfo.KafkaVersion) != obj.Spec.KafkaVersion {
		unsupported = append(unsupported, "kafkaVersion")
	}
	if bng := info.BrokerNodeGroupInfo; bng != nil {
		spec := obj.Spec.BrokerNodeGroupInfo
		if len(spec.ClientSubnets) != len(bng.ClientSubnets) {
			unsupported = append(unsupported, "brokerNodeGroupInfo.clientSubnets")
		}
		if len(spec.SecurityGroups) > 0 && len(spec.SecurityGroups) != len(bng.SecurityGroups) {
			unsupported = append(unsupported, "brokerNodeGroupInfo.securityGroups")
		}
		if spec.StorageVolumeSizeGiB != nil && bng.StorageInfo != nil && bng.StorageInfo.EbsStorageInfo != nil &&
			aws.ToInt32(bng.StorageInfo.EbsStorageInfo.VolumeSize) != aws.ToInt32(spec.StorageVolumeSizeGiB) {
			unsupported = append(unsupported, "brokerNodeGroupInfo.storageVolumeSizeGiB")
		}
	}
	if len(unsupported) > 0 {
		// Do NOT bump ObservedGeneration: the spec has not been applied.
		return r.setConditionMSK(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonUpdateNotSupported,
			fmt.Sprintf("MSK cluster update not supported by this controller for fields: %v", unsupported))
	}

	// Broker count (supported via UpdateBrokerCount).
	if info.NumberOfBrokerNodes != nil && aws.ToInt32(info.NumberOfBrokerNodes) != obj.Spec.NumberOfBrokerNodes {
		if _, err := r.KafkaClient.UpdateBrokerCount(ctx, &awskafka.UpdateBrokerCountInput{
			ClusterArn:                aws.String(obj.Status.ClusterARN),
			CurrentVersion:            info.CurrentVersion,
			TargetNumberOfBrokerNodes: aws.Int32(obj.Spec.NumberOfBrokerNodes),
		}); err != nil {
			return fmt.Errorf("update broker count: %w", err)
		}
		// The cluster enters UPDATING; ObservedGeneration is bumped once the
		// cluster is ACTIVE again and no further drift remains.
		return r.setConditionMSK(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonUpdated, "updating broker count")
	}

	// Broker instance type (supported via UpdateBrokerType).
	if info.BrokerNodeGroupInfo != nil && aws.ToString(info.BrokerNodeGroupInfo.InstanceType) != obj.Spec.BrokerNodeGroupInfo.InstanceType {
		if _, err := r.KafkaClient.UpdateBrokerType(ctx, &awskafka.UpdateBrokerTypeInput{
			ClusterArn:         aws.String(obj.Status.ClusterARN),
			CurrentVersion:     info.CurrentVersion,
			TargetInstanceType: aws.String(obj.Spec.BrokerNodeGroupInfo.InstanceType),
		}); err != nil {
			return fmt.Errorf("update broker type: %w", err)
		}
		return r.setConditionMSK(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonUpdated, "updating broker instance type")
	}

	// No drift remaining; the generation is fully applied.
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionMSK(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "MSKCluster active")
}

// findClusterARNByName looks up a provisioned MSK cluster by exact name.
// MSK enforces unique cluster names per account/region, so an exact match
// is the cluster this CR created. Returns "" when no match is found.
func (r *MSKClusterReconciler) findClusterARNByName(ctx context.Context, name string) (string, error) {
	paginator := awskafka.NewListClustersV2Paginator(r.KafkaClient, &awskafka.ListClustersV2Input{
		ClusterNameFilter: aws.String(name),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return "", fmt.Errorf("list msk clusters: %w", err)
		}
		for _, c := range page.ClusterInfoList {
			if aws.ToString(c.ClusterName) == name {
				return aws.ToString(c.ClusterArn), nil
			}
		}
	}
	return "", nil
}

func (r *MSKClusterReconciler) deleteCluster(ctx context.Context, obj *awsv1alpha1.MSKCluster) error {
	if obj.Status.ClusterARN == "" {
		// Status may have been lost after a successful create; the create
		// path always uses spec.clusterName, so look it up before giving up.
		arn, err := r.findClusterARNByName(ctx, obj.Spec.ClusterName)
		if err != nil {
			return err
		}
		if arn == "" {
			return nil
		}
		obj.Status.ClusterARN = arn
	}
	_, err := r.KafkaClient.DeleteCluster(ctx, &awskafka.DeleteClusterInput{
		ClusterArn: aws.String(obj.Status.ClusterARN),
	})
	if kafkahelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *MSKClusterReconciler) setConditionMSK(ctx context.Context, obj *awsv1alpha1.MSKCluster, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *MSKClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.MSKCluster{}).
		Complete(r)
}
