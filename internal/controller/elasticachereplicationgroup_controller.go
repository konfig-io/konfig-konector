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
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awselasticache "github.com/aws/aws-sdk-go-v2/service/elasticache"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	echelper "github.com/konfig-io/konfig-konector/internal/aws/elasticache"
)

var requeueElastiCachePolling = ctrl.Result{RequeueAfter: 30 * time.Second}

// ElastiCacheReplicationGroupReconciler reconciles ElastiCacheReplicationGroup objects.
type ElastiCacheReplicationGroupReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	ElastiCacheClient *awselasticache.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=elasticachereplicationgroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=elasticachereplicationgroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=elasticachereplicationgroups/finalizers,verbs=update

func (r *ElastiCacheReplicationGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	rg := &awsv1alpha1.ElastiCacheReplicationGroup{}
	if err := r.Get(ctx, req.NamespacedName, rg); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !rg.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(rg, awsv1alpha1.FinalizerName) {
			if shouldAbandon(rg) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(rg, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, rg)
			}
			if err := r.deleteReplicationGroup(ctx, rg); err != nil {
				logger.Error(err, "failed to delete ElastiCache replication group")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(rg, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, rg)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(rg, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(rg, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, rg); err != nil {
			return ctrl.Result{}, err
		}
	}

	result, err := r.reconcileReplicationGroup(ctx, rg)
	if err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, rg, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *ElastiCacheReplicationGroupReconciler) reconcileReplicationGroup(ctx context.Context, rg *awsv1alpha1.ElastiCacheReplicationGroup) (ctrl.Result, error) {
	// Check if already exists.
	if rg.Status.ARN != "" {
		out, err := r.ElastiCacheClient.DescribeReplicationGroups(ctx, &awselasticache.DescribeReplicationGroupsInput{
			ReplicationGroupId: aws.String(rg.Spec.ReplicationGroupID),
		})
		if err != nil && !echelper.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if err == nil && len(out.ReplicationGroups) > 0 {
			existing := out.ReplicationGroups[0]
			rg.Status.Status = aws.ToString(existing.Status)
			rg.Status.ARN = aws.ToString(existing.ARN)

			if existing.NodeGroups != nil && len(existing.NodeGroups) > 0 {
				ng := existing.NodeGroups[0]
				if ng.PrimaryEndpoint != nil {
					rg.Status.PrimaryEndpoint = aws.ToString(ng.PrimaryEndpoint.Address)
				}
				if ng.ReaderEndpoint != nil {
					rg.Status.ReaderEndpoint = aws.ToString(ng.ReaderEndpoint.Address)
				}
			}

			if echelper.IsTransient(rg.Status.Status) {
				_ = r.setCondition(ctx, rg, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning",
					fmt.Sprintf("ElastiCache replication group is %s", rg.Status.Status))
				return requeueElastiCachePolling, nil
			}

			if rg.Status.Status == "available" {
				if rg.Status.ObservedGeneration != rg.Generation {
					if err := r.modifyReplicationGroup(ctx, rg); err != nil {
						return ctrl.Result{}, err
					}
				}
				// Sync tags.
				if len(rg.Spec.Tags) > 0 && rg.Status.ARN != "" {
					if _, err := r.ElastiCacheClient.AddTagsToResource(ctx, &awselasticache.AddTagsToResourceInput{
						ResourceName: aws.String(rg.Status.ARN),
						Tags:         ecTagsFromMap(rg.Spec.Tags),
					}); err != nil {
						return ctrl.Result{}, fmt.Errorf("add tags: %w", err)
					}
				}
				rg.Status.ObservedGeneration = rg.Generation
				now := metav1.Now()
				rg.Status.LastSyncTime = &now
				return requeueResult(), r.setCondition(ctx, rg, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "ElastiCache replication group available")
			}
		}
	}

	// Resolve dependencies.
	sgIDs, err := r.resolveSGIDs(ctx, rg.Namespace, rg.Spec.SecurityGroupRefs)
	if err != nil {
		return ctrl.Result{}, err
	}

	var subnetGroupName string
	if rg.Spec.SubnetGroupRef != "" {
		sgCR := &awsv1alpha1.ElastiCacheSubnetGroup{}
		if err := r.Get(ctx, k8stypes.NamespacedName{Name: rg.Spec.SubnetGroupRef, Namespace: rg.Namespace}, sgCR); err != nil {
			return ctrl.Result{}, err
		}
		if sgCR.Spec.SubnetGroupName == "" {
			return ctrl.Result{}, &dependencyNotReady{msg: fmt.Sprintf("ElastiCacheSubnetGroup %s/%s not ready", rg.Namespace, rg.Spec.SubnetGroupRef)}
		}
		subnetGroupName = sgCR.Spec.SubnetGroupName
	}

	// Create replication group.
	input := &awselasticache.CreateReplicationGroupInput{
		ReplicationGroupId:          aws.String(rg.Spec.ReplicationGroupID),
		ReplicationGroupDescription: aws.String(rg.Spec.Description),
		CacheNodeType:               aws.String(rg.Spec.CacheNodeType),
		Engine:                      aws.String(rg.Spec.Engine),
		AtRestEncryptionEnabled:     aws.Bool(rg.Spec.AtRestEncryption),
		TransitEncryptionEnabled:    aws.Bool(rg.Spec.TransitEncryption),
		Tags:                        ecTagsFromMap(rg.Spec.Tags),
	}
	if rg.Spec.EngineVersion != "" {
		input.EngineVersion = aws.String(rg.Spec.EngineVersion)
	}
	if rg.Spec.NumCacheClusters > 0 {
		input.NumCacheClusters = aws.Int32(rg.Spec.NumCacheClusters)
	}
	if rg.Spec.AutomaticFailover {
		input.AutomaticFailoverEnabled = aws.Bool(true)
	}
	if subnetGroupName != "" {
		input.CacheSubnetGroupName = aws.String(subnetGroupName)
	}
	if len(sgIDs) > 0 {
		input.SecurityGroupIds = sgIDs
	}
	if rg.Spec.AuthTokenRef != nil {
		token, err := resolveSecretValue(ctx, r.Client, rg.Namespace, *rg.Spec.AuthTokenRef)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("resolve authTokenRef: %w", err)
		}
		input.AuthToken = aws.String(token)
	} else if rg.Spec.AuthToken != "" {
		input.AuthToken = aws.String(rg.Spec.AuthToken)
	}
	if rg.Spec.SnapshotRetentionLimit > 0 {
		input.SnapshotRetentionLimit = aws.Int32(rg.Spec.SnapshotRetentionLimit)
	}

	createOut, err := r.ElastiCacheClient.CreateReplicationGroup(ctx, input)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("create ElastiCache replication group: %w", err)
	}

	rg.Status.ARN = aws.ToString(createOut.ReplicationGroup.ARN)
	rg.Status.Status = aws.ToString(createOut.ReplicationGroup.Status)
	_ = r.setCondition(ctx, rg, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning", "ElastiCache replication group is being created")
	return requeueElastiCachePolling, nil
}

func (r *ElastiCacheReplicationGroupReconciler) modifyReplicationGroup(ctx context.Context, rg *awsv1alpha1.ElastiCacheReplicationGroup) error {
	input := &awselasticache.ModifyReplicationGroupInput{
		ReplicationGroupId:          aws.String(rg.Spec.ReplicationGroupID),
		ReplicationGroupDescription: aws.String(rg.Spec.Description),
		CacheNodeType:               aws.String(rg.Spec.CacheNodeType),
		ApplyImmediately:            aws.Bool(true),
	}
	if rg.Spec.AutomaticFailover {
		input.AutomaticFailoverEnabled = aws.Bool(true)
	}
	if rg.Spec.SnapshotRetentionLimit > 0 {
		input.SnapshotRetentionLimit = aws.Int32(rg.Spec.SnapshotRetentionLimit)
	}

	sgIDs, err := r.resolveSGIDs(ctx, rg.Namespace, rg.Spec.SecurityGroupRefs)
	if err != nil {
		return err
	}
	if len(sgIDs) > 0 {
		input.SecurityGroupIds = sgIDs
	}

	_, err = r.ElastiCacheClient.ModifyReplicationGroup(ctx, input)
	return err
}

func (r *ElastiCacheReplicationGroupReconciler) resolveSGIDs(ctx context.Context, namespace string, refs []awsv1alpha1.SecurityGroupRef) ([]string, error) {
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.ID != "" {
			ids = append(ids, ref.ID)
			continue
		}
		sgCR := &awsv1alpha1.SecurityGroup{}
		if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, sgCR); err != nil {
			return nil, err
		}
		if sgCR.Status.GroupID == "" {
			return nil, &dependencyNotReady{msg: fmt.Sprintf("SecurityGroup %s/%s has no ID yet", namespace, ref.Name)}
		}
		ids = append(ids, sgCR.Status.GroupID)
	}
	return ids, nil
}

func (r *ElastiCacheReplicationGroupReconciler) deleteReplicationGroup(ctx context.Context, rg *awsv1alpha1.ElastiCacheReplicationGroup) error {
	if rg.Status.ARN == "" {
		return nil
	}
	_, err := r.ElastiCacheClient.DeleteReplicationGroup(ctx, &awselasticache.DeleteReplicationGroupInput{
		ReplicationGroupId: aws.String(rg.Spec.ReplicationGroupID),
	})
	if echelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *ElastiCacheReplicationGroupReconciler) setCondition(ctx context.Context, rg *awsv1alpha1.ElastiCacheReplicationGroup, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&rg.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: rg.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, rg); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *ElastiCacheReplicationGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.ElastiCacheReplicationGroup{}).
		Complete(r)
}
