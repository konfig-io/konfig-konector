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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	iamhelper "github.com/konfig-io/konfig-konector/internal/aws/iam"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// IAMGroupMembershipReconciler reconciles IAMGroupMembership objects.
type IAMGroupMembershipReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	IAMClient *multi.IAM
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=iamgroupmemberships,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=iamgroupmemberships/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=iamgroupmemberships/finalizers,verbs=update

func (r *IAMGroupMembershipReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	mem := &awsv1alpha1.IAMGroupMembership{}
	if err := r.Get(ctx, req.NamespacedName, mem); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, mem); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !mem.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(mem, awsv1alpha1.FinalizerName) {
			if shouldAbandon(mem) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(mem, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, mem)
			}
			groupName, err := resolveGroupName(ctx, r.Client, mem.Namespace, mem.Spec.GroupRef)
			if err != nil && !apierrors.IsNotFound(err) && !errors.As(err, new(*dependencyNotReady)) {
				return ctrl.Result{}, err
			}
			userName, err2 := resolveUserName(ctx, r.Client, mem.Namespace, mem.Spec.UserRef)
			if err2 != nil && !apierrors.IsNotFound(err2) && !errors.As(err2, new(*dependencyNotReady)) {
				return ctrl.Result{}, err2
			}
			if groupName != "" && userName != "" {
				if err := iamhelper.RemoveUserFromGroup(ctx, r.IAMClient, groupName, userName); err != nil {
					logger.Error(err, "failed to remove user from group")
					return ctrl.Result{}, err
				}
			}
			controllerutil.RemoveFinalizer(mem, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, mem)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(mem, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(mem, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, mem); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileMembership(ctx, mem); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, mem, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *IAMGroupMembershipReconciler) reconcileMembership(ctx context.Context, mem *awsv1alpha1.IAMGroupMembership) error {
	groupName, err := resolveGroupName(ctx, r.Client, mem.Namespace, mem.Spec.GroupRef)
	if err != nil {
		return fmt.Errorf("resolve group: %w", err)
	}
	userName, err := resolveUserName(ctx, r.Client, mem.Namespace, mem.Spec.UserRef)
	if err != nil {
		return fmt.Errorf("resolve user: %w", err)
	}

	member, err := iamhelper.IsUserInGroup(ctx, r.IAMClient, groupName, userName)
	if err != nil {
		return err
	}
	if !member {
		if err := iamhelper.AddUserToGroup(ctx, r.IAMClient, groupName, userName); err != nil {
			return fmt.Errorf("add user to group: %w", err)
		}
	}

	mem.Status.Member = true
	mem.Status.ObservedGeneration = mem.Generation
	now := metav1.Now()
	mem.Status.LastSyncTime = &now
	return r.setCondition(ctx, mem, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "user is member of group")
}

func (r *IAMGroupMembershipReconciler) setCondition(ctx context.Context, mem *awsv1alpha1.IAMGroupMembership, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&mem.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: mem.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, mem); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *IAMGroupMembershipReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.IAMGroupMembership{}).
		Complete(r)
}
