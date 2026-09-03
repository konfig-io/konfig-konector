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

	awseks "github.com/aws/aws-sdk-go-v2/service/eks"
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
)

// EKSAddonReconciler reconciles EKSAddon objects.
type EKSAddonReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EKSClient *awseks.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=eksaddons,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=eksaddons/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=eksaddons/finalizers,verbs=update

func (r *EKSAddonReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	addon := &awsv1alpha1.EKSAddon{}
	if err := r.Get(ctx, req.NamespacedName, addon); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !addon.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(addon, awsv1alpha1.FinalizerName) {
			if shouldAbandon(addon) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(addon, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, addon)
			}
			clusterName, err := resolveEKSClusterName(ctx, r.Client, addon.Namespace, addon.Spec.ClusterName, addon.Spec.ClusterRef)
			if err != nil && !errors.As(err, new(*dependencyNotReady)) && !apierrors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			if clusterName != "" {
				if err := ekshelper.DeleteAddon(ctx, r.EKSClient, clusterName, addon.Spec.AddonName); err != nil && !ekshelper.IsNotFound(err) {
					logger.Error(err, "failed to delete EKS addon")
					return ctrl.Result{}, err
				}
			}
			controllerutil.RemoveFinalizer(addon, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, addon)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(addon, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(addon, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, addon); err != nil {
			return ctrl.Result{}, err
		}
	}

	result, err := r.reconcileAddon(ctx, addon)
	if err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, addon, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *EKSAddonReconciler) reconcileAddon(ctx context.Context, addon *awsv1alpha1.EKSAddon) (ctrl.Result, error) {
	clusterName, err := resolveEKSClusterName(ctx, r.Client, addon.Namespace, addon.Spec.ClusterName, addon.Spec.ClusterRef)
	if err != nil {
		return ctrl.Result{}, err
	}

	var saRoleArn string
	if addon.Spec.ServiceAccountRoleArn != "" {
		saRoleArn = addon.Spec.ServiceAccountRoleArn
	} else if addon.Spec.ServiceAccountRoleRef != nil {
		saRoleArn, err = resolveIAMRoleARN(ctx, r.Client, addon.Namespace, *addon.Spec.ServiceAccountRoleRef)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	addonIn := ekshelper.AddonInput{
		ClusterName:         clusterName,
		AddonName:           addon.Spec.AddonName,
		AddonVersion:        addon.Spec.AddonVersion,
		ServiceAccountRole:  saRoleArn,
		ResolveConflicts:    types.ResolveConflicts(addon.Spec.ResolveConflicts),
		ConfigurationValues: addon.Spec.ConfigurationValues,
		Tags:                addon.Spec.Tags,
	}

	// Always check AWS first — adopt if already exists (handles manual pre-creation or prior operator run).
	existing, err := ekshelper.DescribeAddon(ctx, r.EKSClient, clusterName, addon.Spec.AddonName)
	if err != nil && !ekshelper.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	if existing != nil {
		if existing.AddonArn != nil {
			addon.Status.AddonArn = *existing.AddonArn
		}
		addon.Status.Status = string(existing.Status)
		if existing.AddonVersion != nil {
			addon.Status.AddonVersion = *existing.AddonVersion
		}

		if addon.Status.Status == "CREATING" || addon.Status.Status == "UPDATING" || addon.Status.Status == "DELETING" {
			_ = r.setCondition(ctx, addon, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning",
				fmt.Sprintf("EKS addon is %s", addon.Status.Status))
			return requeueEKSPolling, nil
		}

		if addon.Status.ObservedGeneration != addon.Generation {
			if err := ekshelper.UpdateAddon(ctx, r.EKSClient, addonIn); err != nil {
				return ctrl.Result{}, fmt.Errorf("update EKS addon: %w", err)
			}
		}

		addon.Status.ObservedGeneration = addon.Generation
		now := metav1.Now()
		addon.Status.LastSyncTime = &now
		return requeueResult(), r.setCondition(ctx, addon, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "EKS addon active")
	}

	created, err := ekshelper.CreateAddon(ctx, r.EKSClient, addonIn)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("create EKS addon: %w", err)
	}
	if created.AddonArn != nil {
		addon.Status.AddonArn = *created.AddonArn
	}
	addon.Status.Status = string(created.Status)
	_ = r.setCondition(ctx, addon, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning", "EKS addon is CREATING")
	return requeueEKSPolling, nil
}

func (r *EKSAddonReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.EKSAddon, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *EKSAddonReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.EKSAddon{}).
		Complete(r)
}
