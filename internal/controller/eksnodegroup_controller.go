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

	"github.com/aws/aws-sdk-go-v2/service/eks/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	ekshelper "github.com/konfig-io/konfig-konector/internal/aws/eks"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// EKSNodeGroupReconciler reconciles EKSNodeGroup objects.
type EKSNodeGroupReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EKSClient *multi.EKS
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=eksnodegroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=eksnodegroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=eksnodegroups/finalizers,verbs=update

func (r *EKSNodeGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	ng := &awsv1alpha1.EKSNodeGroup{}
	if err := r.Get(ctx, req.NamespacedName, ng); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, ng); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !ng.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(ng, awsv1alpha1.FinalizerName) {
			if shouldAbandon(ng) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(ng, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, ng)
			}
			clusterName, err := resolveEKSClusterName(ctx, r.Client, ng.Namespace, ng.Spec.ClusterName, ng.Spec.ClusterRef)
			if err != nil && !errors.As(err, new(*dependencyNotReady)) && !apierrors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			if clusterName != "" {
				if err := ekshelper.DeleteNodegroup(ctx, r.EKSClient, clusterName, ng.Spec.NodegroupName); err != nil && !ekshelper.IsNotFound(err) {
					logger.Error(err, "failed to delete EKS nodegroup")
					return ctrl.Result{}, err
				}
			}
			controllerutil.RemoveFinalizer(ng, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, ng)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(ng, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(ng, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, ng); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	result, err := r.reconcileNodegroup(ctx, ng)
	if err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, ng, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *EKSNodeGroupReconciler) reconcileNodegroup(ctx context.Context, ng *awsv1alpha1.EKSNodeGroup) (ctrl.Result, error) {
	clusterName, err := resolveEKSClusterName(ctx, r.Client, ng.Namespace, ng.Spec.ClusterName, ng.Spec.ClusterRef)
	if err != nil {
		return ctrl.Result{}, err
	}

	if ng.Status.NodegroupArn != "" {
		existing, err := ekshelper.DescribeNodegroup(ctx, r.EKSClient, clusterName, ng.Spec.NodegroupName)
		if err != nil && !ekshelper.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if err == nil && existing != nil {
			ng.Status.Status = string(existing.Status)

			if ng.Status.Status != "ACTIVE" {
				_ = r.setCondition(ctx, ng, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning",
					fmt.Sprintf("EKS nodegroup is %s", ng.Status.Status))
				return requeueEKSPolling, nil
			}

			if ng.Status.ObservedGeneration != ng.Generation {
				updateIn := r.buildNodegroupInput(ng, clusterName, nil, nil)
				if err := ekshelper.UpdateNodegroupConfig(ctx, r.EKSClient, clusterName, ng.Spec.NodegroupName, updateIn); err != nil {
					return ctrl.Result{}, fmt.Errorf("update EKS nodegroup config: %w", err)
				}
			}

			ng.Status.ObservedGeneration = ng.Generation
			now := metav1.Now()
			ng.Status.LastSyncTime = &now
			return requeueResult(), r.setCondition(ctx, ng, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "EKS nodegroup active")
		}
	}

	var roleArn string
	if ng.Spec.NodeRoleArn != "" {
		roleArn = ng.Spec.NodeRoleArn
	} else if ng.Spec.NodeRoleRef != nil {
		roleArn, err = resolveIAMRoleARN(ctx, r.Client, ng.Namespace, *ng.Spec.NodeRoleRef)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else {
		return ctrl.Result{}, fmt.Errorf("either nodeRoleArn or nodeRoleRef must be set")
	}

	subnetIDs, err := resolveSubnetIDs(ctx, r.Client, ng.Namespace, ng.Spec.SubnetRefs)
	if err != nil {
		return ctrl.Result{}, err
	}

	var remoteAccessSGIDs []string
	if ng.Spec.RemoteAccess != nil {
		remoteAccessSGIDs, err = resolveSGIDs(ctx, r.Client, ng.Namespace, ng.Spec.RemoteAccess.SourceSecurityGroupRefs)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	createIn := r.buildNodegroupInput(ng, clusterName, &roleArn, subnetIDs)
	createIn.RemoteAccessSGIDs = remoteAccessSGIDs

	created, err := ekshelper.CreateNodegroup(ctx, r.EKSClient, createIn)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("create EKS nodegroup: %w", err)
	}
	if created.NodegroupArn != nil {
		ng.Status.NodegroupArn = *created.NodegroupArn
	}
	ng.Status.Status = string(created.Status)
	_ = r.setCondition(ctx, ng, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning", "EKS nodegroup is CREATING")
	return requeueEKSPolling, nil
}

func (r *EKSNodeGroupReconciler) buildNodegroupInput(ng *awsv1alpha1.EKSNodeGroup, clusterName string, roleArn *string, subnetIDs []string) ekshelper.CreateNodegroupInput {
	in := ekshelper.CreateNodegroupInput{
		ClusterName:    clusterName,
		NodegroupName:  ng.Spec.NodegroupName,
		SubnetIDs:      subnetIDs,
		ScalingMin:     ng.Spec.ScalingConfig.MinSize,
		ScalingMax:     ng.Spec.ScalingConfig.MaxSize,
		ScalingDesired: ng.Spec.ScalingConfig.DesiredSize,
		InstanceTypes:  ng.Spec.InstanceTypes,
		Labels:         ng.Spec.Labels,
		Tags:           ng.Spec.Tags,
	}
	if roleArn != nil {
		in.NodeRoleArn = *roleArn
	}
	if ng.Spec.AmiType != "" {
		in.AmiType = types.AMITypes(ng.Spec.AmiType)
	}
	if ng.Spec.CapacityType != "" {
		in.CapacityType = types.CapacityTypes(ng.Spec.CapacityType)
	}
	in.DiskSize = ng.Spec.DiskSize
	if len(ng.Spec.Taints) > 0 {
		for _, t := range ng.Spec.Taints {
			in.Taints = append(in.Taints, types.Taint{
				Key:    &t.Key,
				Value:  &t.Value,
				Effect: types.TaintEffect(t.Effect),
			})
		}
	}
	if ng.Spec.UpdateConfig != nil {
		in.MaxUnavailable = ng.Spec.UpdateConfig.MaxUnavailable
		in.MaxUnavailablePct = ng.Spec.UpdateConfig.MaxUnavailablePercentage
	}
	if ng.Spec.LaunchTemplate != nil {
		in.LaunchTemplateID = ng.Spec.LaunchTemplate.ID
		in.LaunchTemplateName = ng.Spec.LaunchTemplate.Name
		in.LaunchTemplateVer = ng.Spec.LaunchTemplate.Version
	}
	in.ReleaseVersion = ng.Spec.ReleaseVersion
	in.Version = ng.Spec.Version
	if ng.Spec.RemoteAccess != nil {
		in.RemoteAccessKey = ng.Spec.RemoteAccess.EC2SshKey
	}
	if ng.Spec.NodeRepairConfig != nil {
		enabled := ng.Spec.NodeRepairConfig.Enabled
		in.NodeRepairEnabled = &enabled
	}
	return in
}

func (r *EKSNodeGroupReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.EKSNodeGroup, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *EKSNodeGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.EKSNodeGroup{}).
		Complete(r)
}
