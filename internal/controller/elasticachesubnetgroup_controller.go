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
	awselasticache "github.com/aws/aws-sdk-go-v2/service/elasticache"
	ectypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
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

// ElastiCacheSubnetGroupReconciler reconciles ElastiCacheSubnetGroup objects.
type ElastiCacheSubnetGroupReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	ElastiCacheClient *awselasticache.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=elasticachesubnetgroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=elasticachesubnetgroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=elasticachesubnetgroups/finalizers,verbs=update

func (r *ElastiCacheSubnetGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	sg := &awsv1alpha1.ElastiCacheSubnetGroup{}
	if err := r.Get(ctx, req.NamespacedName, sg); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !sg.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(sg, awsv1alpha1.FinalizerName) {
			if shouldAbandon(sg) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(sg, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, sg)
			}
			if err := r.deleteSubnetGroup(ctx, sg); err != nil {
				logger.Error(err, "failed to delete ElastiCache subnet group")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(sg, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, sg)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(sg, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(sg, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, sg); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileSubnetGroup(ctx, sg); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, sg, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *ElastiCacheSubnetGroupReconciler) reconcileSubnetGroup(ctx context.Context, sg *awsv1alpha1.ElastiCacheSubnetGroup) error {
	subnetIDs, err := r.resolveSubnetIDs(ctx, sg.Namespace, sg.Spec.SubnetRefs)
	if err != nil {
		return err
	}

	out, err := r.ElastiCacheClient.DescribeCacheSubnetGroups(ctx, &awselasticache.DescribeCacheSubnetGroupsInput{
		CacheSubnetGroupName: aws.String(sg.Spec.SubnetGroupName),
	})
	if err != nil && !echelper.IsNotFound(err) {
		return err
	}

	if echelper.IsNotFound(err) || len(out.CacheSubnetGroups) == 0 {
		createOut, err := r.ElastiCacheClient.CreateCacheSubnetGroup(ctx, &awselasticache.CreateCacheSubnetGroupInput{
			CacheSubnetGroupName:        aws.String(sg.Spec.SubnetGroupName),
			CacheSubnetGroupDescription: aws.String(sg.Spec.Description),
			SubnetIds:                   subnetIDs,
			Tags:                        ecTagsFromMap(sg.Spec.Tags),
		})
		if err != nil {
			return fmt.Errorf("create ElastiCache subnet group: %w", err)
		}
		sg.Status.ARN = aws.ToString(createOut.CacheSubnetGroup.ARN)
	} else {
		existing := out.CacheSubnetGroups[0]
		sg.Status.ARN = aws.ToString(existing.ARN)

		if sg.Status.ObservedGeneration != sg.Generation {
			if _, err := r.ElastiCacheClient.ModifyCacheSubnetGroup(ctx, &awselasticache.ModifyCacheSubnetGroupInput{
				CacheSubnetGroupName:        aws.String(sg.Spec.SubnetGroupName),
				CacheSubnetGroupDescription: aws.String(sg.Spec.Description),
				SubnetIds:                   subnetIDs,
			}); err != nil {
				return fmt.Errorf("modify ElastiCache subnet group: %w", err)
			}

			// Sync tags using ARN.
			if len(sg.Spec.Tags) > 0 && sg.Status.ARN != "" {
				if _, err := r.ElastiCacheClient.AddTagsToResource(ctx, &awselasticache.AddTagsToResourceInput{
					ResourceName: aws.String(sg.Status.ARN),
					Tags:         ecTagsFromMap(sg.Spec.Tags),
				}); err != nil {
					return fmt.Errorf("add tags to ElastiCache subnet group: %w", err)
				}
			}
		}
	}

	sg.Status.ObservedGeneration = sg.Generation
	now := metav1.Now()
	sg.Status.LastSyncTime = &now
	return r.setCondition(ctx, sg, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "ElastiCache subnet group reconciled")
}

func (r *ElastiCacheSubnetGroupReconciler) resolveSubnetIDs(ctx context.Context, namespace string, refs []awsv1alpha1.SubnetRef) ([]string, error) {
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.ID != "" {
			ids = append(ids, ref.ID)
			continue
		}
		sn := &awsv1alpha1.Subnet{}
		if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, sn); err != nil {
			return nil, err
		}
		if sn.Status.SubnetID == "" {
			return nil, &dependencyNotReady{msg: fmt.Sprintf("Subnet %s/%s has no ID yet", namespace, ref.Name)}
		}
		ids = append(ids, sn.Status.SubnetID)
	}
	return ids, nil
}

func (r *ElastiCacheSubnetGroupReconciler) deleteSubnetGroup(ctx context.Context, sg *awsv1alpha1.ElastiCacheSubnetGroup) error {
	_, err := r.ElastiCacheClient.DeleteCacheSubnetGroup(ctx, &awselasticache.DeleteCacheSubnetGroupInput{
		CacheSubnetGroupName: aws.String(sg.Spec.SubnetGroupName),
	})
	if echelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *ElastiCacheSubnetGroupReconciler) setCondition(ctx context.Context, sg *awsv1alpha1.ElastiCacheSubnetGroup, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&sg.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: sg.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, sg); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

// ecTagsFromMap converts a map to ElastiCache Tag slice.
func ecTagsFromMap(m map[string]string) []ectypes.Tag {
	tags := make([]ectypes.Tag, 0, len(m))
	for k, v := range m {
		k, v := k, v
		tags = append(tags, ectypes.Tag{Key: &k, Value: &v})
	}
	return tags
}

func (r *ElastiCacheSubnetGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.ElastiCacheSubnetGroup{}).
		Complete(r)
}
