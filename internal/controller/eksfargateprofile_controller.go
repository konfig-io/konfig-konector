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

// EKSFargateProfileReconciler reconciles EKSFargateProfile objects.
type EKSFargateProfileReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EKSClient *multi.EKS
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=eksfargateprofiles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=eksfargateprofiles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=eksfargateprofiles/finalizers,verbs=update

func (r *EKSFargateProfileReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	fp := &awsv1alpha1.EKSFargateProfile{}
	if err := r.Get(ctx, req.NamespacedName, fp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, fp); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !fp.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(fp, awsv1alpha1.FinalizerName) {
			if shouldAbandon(fp) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(fp, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, fp)
			}
			clusterName, err := resolveEKSClusterName(ctx, r.Client, fp.Namespace, fp.Spec.ClusterName, fp.Spec.ClusterRef)
			if err != nil && !errors.As(err, new(*dependencyNotReady)) && !apierrors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			if clusterName != "" {
				if err := ekshelper.DeleteFargateProfile(ctx, r.EKSClient, clusterName, fp.Spec.FargateProfileName); err != nil && !ekshelper.IsNotFound(err) {
					logger.Error(err, "failed to delete EKS Fargate profile")
					return ctrl.Result{}, err
				}
			}
			controllerutil.RemoveFinalizer(fp, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, fp)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(fp, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(fp, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, fp); err != nil {
			return ctrl.Result{}, err
		}
	}

	result, err := r.reconcileFargateProfile(ctx, fp)
	if err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, fp, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *EKSFargateProfileReconciler) reconcileFargateProfile(ctx context.Context, fp *awsv1alpha1.EKSFargateProfile) (ctrl.Result, error) {
	clusterName, err := resolveEKSClusterName(ctx, r.Client, fp.Namespace, fp.Spec.ClusterName, fp.Spec.ClusterRef)
	if err != nil {
		return ctrl.Result{}, err
	}

	if fp.Status.FargateProfileArn != "" {
		existing, err := ekshelper.DescribeFargateProfile(ctx, r.EKSClient, clusterName, fp.Spec.FargateProfileName)
		if err != nil && !ekshelper.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if err == nil && existing != nil {
			fp.Status.Status = string(existing.Status)

			if fp.Status.Status == "CREATING" || fp.Status.Status == "DELETING" {
				_ = r.setCondition(ctx, fp, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning",
					fmt.Sprintf("EKS Fargate profile is %s", fp.Status.Status))
				return requeueFargatePolling, nil
			}
			if fp.Status.Status == "CREATE_FAILED" || fp.Status.Status == "DELETE_FAILED" {
				_ = r.setCondition(ctx, fp, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Failed",
					fmt.Sprintf("EKS Fargate profile is %s", fp.Status.Status))
				return requeueResult(), nil
			}

			fp.Status.ObservedGeneration = fp.Generation
			now := metav1.Now()
			fp.Status.LastSyncTime = &now
			return requeueResult(), r.setCondition(ctx, fp, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "EKS Fargate profile active")
		}
	}

	var podExecRoleArn string
	if fp.Spec.PodExecutionRoleArn != "" {
		podExecRoleArn = fp.Spec.PodExecutionRoleArn
	} else if fp.Spec.PodExecutionRoleRef != nil {
		podExecRoleArn, err = resolveIAMRoleARN(ctx, r.Client, fp.Namespace, *fp.Spec.PodExecutionRoleRef)
		if err != nil {
			return ctrl.Result{}, err
		}
	} else {
		return ctrl.Result{}, fmt.Errorf("either podExecutionRoleArn or podExecutionRoleRef must be set")
	}

	subnetIDs, err := resolveSubnetIDs(ctx, r.Client, fp.Namespace, fp.Spec.SubnetRefs)
	if err != nil {
		return ctrl.Result{}, err
	}

	selectors := make([]types.FargateProfileSelector, 0, len(fp.Spec.Selectors))
	for _, s := range fp.Spec.Selectors {
		selectors = append(selectors, types.FargateProfileSelector{
			Namespace: &s.Namespace,
			Labels:    s.Labels,
		})
	}

	created, err := ekshelper.CreateFargateProfile(ctx, r.EKSClient, ekshelper.FargateProfileInput{
		ClusterName:      clusterName,
		ProfileName:      fp.Spec.FargateProfileName,
		PodExecutionRole: podExecRoleArn,
		SubnetIDs:        subnetIDs,
		Selectors:        selectors,
		Tags:             fp.Spec.Tags,
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("create EKS Fargate profile: %w", err)
	}
	if created.FargateProfileArn != nil {
		fp.Status.FargateProfileArn = *created.FargateProfileArn
	}
	fp.Status.Status = string(created.Status)
	_ = r.setCondition(ctx, fp, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning", "EKS Fargate profile is CREATING")
	return requeueFargatePolling, nil
}

func (r *EKSFargateProfileReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.EKSFargateProfile, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *EKSFargateProfileReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.EKSFargateProfile{}).
		Complete(r)
}
