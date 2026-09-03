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
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsiam "github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	iamhelper "github.com/konfig-io/konfig-konector/internal/aws/iam"
)

const requeueAfter = 5 * time.Minute

// IAMRoleAWSAPI is the subset of the IAM SDK client used by this controller
// (directly and via the iamhelper role functions). *iam.Client satisfies it.
type IAMRoleAWSAPI interface {
	iamhelper.RoleAPI
}

// IAMRoleReconciler reconciles IAMRole objects.
type IAMRoleReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	IAMClient IAMRoleAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=iamroles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=iamroles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=iamroles/finalizers,verbs=update

func (r *IAMRoleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	role := &awsv1alpha1.IAMRole{}
	if err := r.Get(ctx, req.NamespacedName, role); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion.
	if !role.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(role, awsv1alpha1.FinalizerName) {
			if shouldAbandon(role) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(role, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, role)
			}
			if err := r.deleteIAMRole(ctx, role); err != nil {
				logger.Error(err, "failed to delete IAM role", "roleName", role.Spec.RoleName)
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(role, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, role)
		}
		return ctrl.Result{}, nil
	}

	// Ensure finalizer.
	if !controllerutil.ContainsFinalizer(role, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(role, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, role); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileIAMRole(ctx, role); err != nil {
		logger.Error(err, "reconcile error", "roleName", role.Spec.RoleName)
		_ = r.setCondition(ctx, role, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}

	return requeueResult(), nil
}

func (r *IAMRoleReconciler) reconcileIAMRole(ctx context.Context, role *awsv1alpha1.IAMRole) error {
	existing, err := iamhelper.GetRole(ctx, r.IAMClient, role.Spec.RoleName)
	if err != nil {
		return err
	}

	if existing == nil {
		// Create.
		path := role.Spec.Path
		if path == "" {
			path = "/"
		}
		input := &awsiam.CreateRoleInput{
			RoleName:                 aws.String(role.Spec.RoleName),
			AssumeRolePolicyDocument: aws.String(role.Spec.AssumeRolePolicyDocument),
			Description:              aws.String(role.Spec.Description),
			Path:                     aws.String(path),
		}
		if role.Spec.MaxSessionDuration > 0 {
			input.MaxSessionDuration = aws.Int32(role.Spec.MaxSessionDuration)
		}
		if role.Spec.PermissionsBoundary != "" {
			input.PermissionsBoundary = aws.String(role.Spec.PermissionsBoundary)
		}
		for k, v := range role.Spec.Tags {
			kCopy, vCopy := k, v
			input.Tags = append(input.Tags, iamtypes.Tag{Key: &kCopy, Value: &vCopy})
		}
		created, err := iamhelper.CreateRole(ctx, r.IAMClient, input)
		if err != nil {
			return fmt.Errorf("create role: %w", err)
		}
		role.Status.ARN = aws.ToString(created.Arn)
		role.Status.RoleID = aws.ToString(created.RoleId)
	} else {
		// Update trust policy if changed.
		if aws.ToString(existing.AssumeRolePolicyDocument) != role.Spec.AssumeRolePolicyDocument {
			if err := iamhelper.UpdateAssumeRolePolicy(ctx, r.IAMClient, role.Spec.RoleName, role.Spec.AssumeRolePolicyDocument); err != nil {
				return fmt.Errorf("update assume role policy: %w", err)
			}
		}
		// Update description / max session.
		if aws.ToString(existing.Description) != role.Spec.Description ||
			(role.Spec.MaxSessionDuration > 0 && aws.ToInt32(existing.MaxSessionDuration) != role.Spec.MaxSessionDuration) {
			if err := iamhelper.UpdateRoleDescription(ctx, r.IAMClient, role.Spec.RoleName, role.Spec.Description, role.Spec.MaxSessionDuration); err != nil {
				return fmt.Errorf("update role: %w", err)
			}
		}
		role.Status.ARN = aws.ToString(existing.Arn)
		role.Status.RoleID = aws.ToString(existing.RoleId)
	}

	// Sync tags.
	if err := iamhelper.SyncTags(ctx, r.IAMClient, role.Spec.RoleName, role.Spec.Tags); err != nil {
		return fmt.Errorf("sync tags: %w", err)
	}

	now := metav1.Now()
	role.Status.LastSyncTime = &now
	role.Status.ObservedGeneration = role.Generation
	return r.setCondition(ctx, role, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "IAM role reconciled")
}

func (r *IAMRoleReconciler) deleteIAMRole(ctx context.Context, role *awsv1alpha1.IAMRole) error {
	if err := iamhelper.DetachAllPolicies(ctx, r.IAMClient, role.Spec.RoleName); err != nil {
		return fmt.Errorf("detach policies: %w", err)
	}
	if err := iamhelper.DeleteAllInlinePolicies(ctx, r.IAMClient, role.Spec.RoleName); err != nil {
		return fmt.Errorf("delete inline policies: %w", err)
	}
	return iamhelper.DeleteRole(ctx, r.IAMClient, role.Spec.RoleName)
}

func (r *IAMRoleReconciler) setCondition(ctx context.Context, role *awsv1alpha1.IAMRole, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&role.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: role.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, role); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IAMRoleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.IAMRole{}).
		Complete(r)
}
