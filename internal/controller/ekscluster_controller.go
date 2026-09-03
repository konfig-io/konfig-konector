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

	awseks "github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/eks/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	ekshelper "github.com/konfig-io/konfig-konector/internal/aws/eks"
)

var requeueEKSPolling = ctrl.Result{RequeueAfter: 30 * time.Second}
var requeueFargatePolling = ctrl.Result{RequeueAfter: 15 * time.Second}

// EKSClusterAWSAPI is the subset of the EKS SDK client used by this
// controller (via the ekshelper package). *awseks.Client satisfies it.
type EKSClusterAWSAPI interface {
	DescribeCluster(ctx context.Context, params *awseks.DescribeClusterInput, optFns ...func(*awseks.Options)) (*awseks.DescribeClusterOutput, error)
	CreateCluster(ctx context.Context, params *awseks.CreateClusterInput, optFns ...func(*awseks.Options)) (*awseks.CreateClusterOutput, error)
	UpdateClusterConfig(ctx context.Context, params *awseks.UpdateClusterConfigInput, optFns ...func(*awseks.Options)) (*awseks.UpdateClusterConfigOutput, error)
	UpdateClusterVersion(ctx context.Context, params *awseks.UpdateClusterVersionInput, optFns ...func(*awseks.Options)) (*awseks.UpdateClusterVersionOutput, error)
	DeleteCluster(ctx context.Context, params *awseks.DeleteClusterInput, optFns ...func(*awseks.Options)) (*awseks.DeleteClusterOutput, error)
}

// EKSClusterReconciler reconciles EKSCluster objects.
type EKSClusterReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EKSClient EKSClusterAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=eksclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=eksclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=eksclusters/finalizers,verbs=update

func (r *EKSClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	cluster := &awsv1alpha1.EKSCluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !cluster.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(cluster, awsv1alpha1.FinalizerName) {
			if shouldAbandon(cluster) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(cluster, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, cluster)
			}
			if err := ekshelper.DeleteCluster(ctx, r.EKSClient, cluster.Spec.ClusterName); err != nil && !ekshelper.IsNotFound(err) {
				logger.Error(err, "failed to delete EKS cluster")
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

func (r *EKSClusterReconciler) reconcileCluster(ctx context.Context, cluster *awsv1alpha1.EKSCluster) (ctrl.Result, error) {
	if cluster.Status.ClusterArn != "" {
		existing, err := ekshelper.DescribeCluster(ctx, r.EKSClient, cluster.Spec.ClusterName)
		if err != nil && !ekshelper.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if err == nil && existing != nil {
			cluster.Status.Status = string(existing.Status)
			if existing.Endpoint != nil {
				cluster.Status.Endpoint = *existing.Endpoint
			}
			if existing.Version != nil {
				cluster.Status.Version = *existing.Version
			}
			if existing.CertificateAuthority != nil && existing.CertificateAuthority.Data != nil {
				cluster.Status.CertificateAuthority = *existing.CertificateAuthority.Data
			}
			if existing.Identity != nil && existing.Identity.Oidc != nil && existing.Identity.Oidc.Issuer != nil {
				cluster.Status.OIDCIssuer = *existing.Identity.Oidc.Issuer
			}

			if cluster.Status.Status != "ACTIVE" {
				_ = r.setCondition(ctx, cluster, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning",
					fmt.Sprintf("EKS cluster is %s", cluster.Status.Status))
				return requeueEKSPolling, nil
			}

			// Kubernetes version upgrade is a separate async update from config
			// changes; kick it off and poll until the cluster is ACTIVE at the
			// new version before applying config updates.
			if cluster.Spec.Version != "" && existing.Version != nil && cluster.Spec.Version != *existing.Version {
				if err := ekshelper.UpdateClusterVersion(ctx, r.EKSClient, cluster.Spec.ClusterName, cluster.Spec.Version); err != nil {
					// The version update is async; while one is in progress the
					// cluster reports ACTIVE at the old version and a repeated
					// call fails with ResourceInUse. Keep polling in that case.
					var inUse *types.ResourceInUseException
					if !errors.As(err, &inUse) {
						return ctrl.Result{}, fmt.Errorf("update EKS cluster version: %w", err)
					}
				}
				_ = r.setCondition(ctx, cluster, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Upgrading",
					fmt.Sprintf("EKS cluster upgrading from %s to %s", *existing.Version, cluster.Spec.Version))
				return requeueEKSPolling, nil
			}

			if cluster.Status.ObservedGeneration != cluster.Generation {
				subnetIDs, err := resolveSubnetIDs(ctx, r.Client, cluster.Namespace, cluster.Spec.ResourcesVpcConfig.SubnetRefs)
				if err != nil {
					return ctrl.Result{}, err
				}
				sgIDs, err := resolveSGIDs(ctx, r.Client, cluster.Namespace, cluster.Spec.ResourcesVpcConfig.SecurityGroupRefs)
				if err != nil {
					return ctrl.Result{}, err
				}
				updateIn := ekshelper.CreateClusterInput{
					SubnetIDs:             subnetIDs,
					SecurityGroupIDs:      sgIDs,
					EndpointPublicAccess:  cluster.Spec.ResourcesVpcConfig.EndpointPublicAccess,
					EndpointPrivateAccess: cluster.Spec.ResourcesVpcConfig.EndpointPrivateAccess,
					PublicAccessCidrs:     cluster.Spec.ResourcesVpcConfig.PublicAccessCidrs,
				}
				if cluster.Spec.Logging != nil {
					updateIn.LogTypes = cluster.Spec.Logging.EnabledTypes
				}
				if cluster.Spec.AccessConfig != nil {
					updateIn.AuthenticationMode = types.AuthenticationMode(cluster.Spec.AccessConfig.AuthenticationMode)
				}
				if cluster.Spec.UpgradePolicy != nil {
					updateIn.UpgradePolicySupportType = cluster.Spec.UpgradePolicy.SupportType
				}
				if err := ekshelper.UpdateClusterConfig(ctx, r.EKSClient, cluster.Spec.ClusterName, updateIn); err != nil {
					return ctrl.Result{}, fmt.Errorf("update EKS cluster config: %w", err)
				}
			}

			cluster.Status.ObservedGeneration = cluster.Generation
			now := metav1.Now()
			cluster.Status.LastSyncTime = &now
			return requeueResult(), r.setCondition(ctx, cluster, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "EKS cluster active")
		}
	}

	// Resolve dependencies before creating
	var roleArn string
	if cluster.Spec.RoleArn != "" {
		roleArn = cluster.Spec.RoleArn
	} else if cluster.Spec.RoleRef != nil {
		var err error
		roleArn, err = resolveIAMRoleARN(ctx, r.Client, cluster.Namespace, *cluster.Spec.RoleRef)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else {
		return ctrl.Result{}, fmt.Errorf("either roleArn or roleRef must be set")
	}

	subnetIDs, err := resolveSubnetIDs(ctx, r.Client, cluster.Namespace, cluster.Spec.ResourcesVpcConfig.SubnetRefs)
	if err != nil {
		return ctrl.Result{}, err
	}
	sgIDs, err := resolveSGIDs(ctx, r.Client, cluster.Namespace, cluster.Spec.ResourcesVpcConfig.SecurityGroupRefs)
	if err != nil {
		return ctrl.Result{}, err
	}

	createIn := ekshelper.CreateClusterInput{
		Name:                  cluster.Spec.ClusterName,
		Version:               cluster.Spec.Version,
		RoleArn:               roleArn,
		SubnetIDs:             subnetIDs,
		SecurityGroupIDs:      sgIDs,
		EndpointPublicAccess:  cluster.Spec.ResourcesVpcConfig.EndpointPublicAccess,
		EndpointPrivateAccess: cluster.Spec.ResourcesVpcConfig.EndpointPrivateAccess,
		PublicAccessCidrs:     cluster.Spec.ResourcesVpcConfig.PublicAccessCidrs,
		Tags:                  cluster.Spec.Tags,
	}
	if cluster.Spec.Logging != nil {
		createIn.LogTypes = cluster.Spec.Logging.EnabledTypes
	}
	if cluster.Spec.EncryptionConfig != nil {
		createIn.EncryptionProviderArn = cluster.Spec.EncryptionConfig.ProviderKeyArn
		createIn.EncryptionResources = cluster.Spec.EncryptionConfig.Resources
	}
	if cluster.Spec.AccessConfig != nil {
		createIn.AuthenticationMode = types.AuthenticationMode(cluster.Spec.AccessConfig.AuthenticationMode)
		createIn.BootstrapAdminPerms = cluster.Spec.AccessConfig.BootstrapClusterCreatorAdminPermissions
	}
	if cluster.Spec.KubernetesNetworkConfig != nil {
		createIn.KubernetesServiceIPv4CIDR = cluster.Spec.KubernetesNetworkConfig.ServiceIPv4CIDR
		createIn.KubernetesIPFamily = cluster.Spec.KubernetesNetworkConfig.IPFamily
	}
	if cluster.Spec.UpgradePolicy != nil {
		createIn.UpgradePolicySupportType = cluster.Spec.UpgradePolicy.SupportType
	}
	if cluster.Spec.BootstrapSelfManagedAddons != nil {
		createIn.BootstrapSelfManagedAddons = cluster.Spec.BootstrapSelfManagedAddons
	}

	created, err := ekshelper.CreateCluster(ctx, r.EKSClient, createIn)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("create EKS cluster: %w", err)
	}
	if created.Arn != nil {
		cluster.Status.ClusterArn = *created.Arn
	}
	cluster.Status.Status = string(created.Status)
	// Persist the ARN immediately: the AWS resource now exists, and losing the
	// identifier would orphan it or create a duplicate on retry.
	if err := persistStatus(ctx, r.Client, cluster); err != nil {
		return ctrl.Result{}, fmt.Errorf("persist cluster ARN after create: %w", err)
	}
	_ = r.setCondition(ctx, cluster, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning", "EKS cluster is CREATING")
	return requeueEKSPolling, nil
}

func (r *EKSClusterReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.EKSCluster, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *EKSClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.EKSCluster{}).
		Complete(r)
}
