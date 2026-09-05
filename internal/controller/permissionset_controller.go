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
	awssso "github.com/aws/aws-sdk-go-v2/service/ssoadmin"
	ssotypes "github.com/aws/aws-sdk-go-v2/service/ssoadmin/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	ssohelper "github.com/konfig-io/konfig-konector/internal/aws/ssoadmin"
)

// ssoTags converts a CR tag map to SSO Admin SDK tags.
func ssoTags(tags map[string]string) []ssotypes.Tag {
	if len(tags) == 0 {
		return nil
	}
	out := make([]ssotypes.Tag, 0, len(tags))
	for k, v := range tags {
		out = append(out, ssotypes.Tag{Key: aws.String(k), Value: aws.String(v)})
	}
	return out
}

// PermissionSetAWSAPI is the subset of the SSO Admin API used by this controller.
type PermissionSetAWSAPI interface {
	CreatePermissionSet(ctx context.Context, params *awssso.CreatePermissionSetInput, optFns ...func(*awssso.Options)) (*awssso.CreatePermissionSetOutput, error)
	UpdatePermissionSet(ctx context.Context, params *awssso.UpdatePermissionSetInput, optFns ...func(*awssso.Options)) (*awssso.UpdatePermissionSetOutput, error)
	DeletePermissionSet(ctx context.Context, params *awssso.DeletePermissionSetInput, optFns ...func(*awssso.Options)) (*awssso.DeletePermissionSetOutput, error)
	DescribePermissionSet(ctx context.Context, params *awssso.DescribePermissionSetInput, optFns ...func(*awssso.Options)) (*awssso.DescribePermissionSetOutput, error)
	ListManagedPoliciesInPermissionSet(ctx context.Context, params *awssso.ListManagedPoliciesInPermissionSetInput, optFns ...func(*awssso.Options)) (*awssso.ListManagedPoliciesInPermissionSetOutput, error)
	AttachManagedPolicyToPermissionSet(ctx context.Context, params *awssso.AttachManagedPolicyToPermissionSetInput, optFns ...func(*awssso.Options)) (*awssso.AttachManagedPolicyToPermissionSetOutput, error)
	DetachManagedPolicyFromPermissionSet(ctx context.Context, params *awssso.DetachManagedPolicyFromPermissionSetInput, optFns ...func(*awssso.Options)) (*awssso.DetachManagedPolicyFromPermissionSetOutput, error)
	PutInlinePolicyToPermissionSet(ctx context.Context, params *awssso.PutInlinePolicyToPermissionSetInput, optFns ...func(*awssso.Options)) (*awssso.PutInlinePolicyToPermissionSetOutput, error)
	DeleteInlinePolicyFromPermissionSet(ctx context.Context, params *awssso.DeleteInlinePolicyFromPermissionSetInput, optFns ...func(*awssso.Options)) (*awssso.DeleteInlinePolicyFromPermissionSetOutput, error)
	TagResource(ctx context.Context, params *awssso.TagResourceInput, optFns ...func(*awssso.Options)) (*awssso.TagResourceOutput, error)
}

// PermissionSetReconciler reconciles PermissionSet objects.
type PermissionSetReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	SSOAdminClient PermissionSetAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=permissionsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=permissionsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=permissionsets/finalizers,verbs=update

func (r *PermissionSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	ps := &awsv1alpha1.PermissionSet{}
	if err := r.Get(ctx, req.NamespacedName, ps); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, ps); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !ps.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(ps, awsv1alpha1.FinalizerName) {
			if shouldAbandon(ps) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(ps, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, ps)
			}
			if err := r.deletePermissionSet(ctx, ps); err != nil {
				logger.Error(err, "failed to delete permission set")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(ps, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, ps)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(ps, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(ps, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, ps); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcilePermissionSet(ctx, ps); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, ps, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *PermissionSetReconciler) reconcilePermissionSet(ctx context.Context, ps *awsv1alpha1.PermissionSet) error {
	instanceArn := aws.String(ps.Spec.InstanceArn)

	exists := false
	if ps.Status.PermissionSetArn != "" {
		_, err := r.SSOAdminClient.DescribePermissionSet(ctx, &awssso.DescribePermissionSetInput{
			InstanceArn:      instanceArn,
			PermissionSetArn: aws.String(ps.Status.PermissionSetArn),
		})
		if err == nil {
			exists = true
		} else if !ssohelper.IsNotFound(err) {
			return fmt.Errorf("describe permission set: %w", err)
		}
	}

	if !exists {
		input := &awssso.CreatePermissionSetInput{
			InstanceArn: instanceArn,
			Name:        aws.String(ps.Spec.Name),
			Tags:        ssoTags(ps.Spec.Tags),
		}
		if ps.Spec.Description != "" {
			input.Description = aws.String(ps.Spec.Description)
		}
		if ps.Spec.SessionDuration != "" {
			input.SessionDuration = aws.String(ps.Spec.SessionDuration)
		}
		if ps.Spec.RelayState != "" {
			input.RelayState = aws.String(ps.Spec.RelayState)
		}
		out, err := r.SSOAdminClient.CreatePermissionSet(ctx, input)
		if err != nil {
			return fmt.Errorf("create permission set: %w", err)
		}
		// Persist the ARN immediately: the AWS resource now exists, and
		// losing the identifier would orphan it on delete.
		ps.Status.PermissionSetArn = aws.ToString(out.PermissionSet.PermissionSetArn)
		if err := persistStatus(ctx, r.Client, ps); err != nil {
			return fmt.Errorf("persist permission set ARN after create: %w", err)
		}
	} else if ps.Status.ObservedGeneration != ps.Generation {
		input := &awssso.UpdatePermissionSetInput{
			InstanceArn:      instanceArn,
			PermissionSetArn: aws.String(ps.Status.PermissionSetArn),
			Description:      aws.String(ps.Spec.Description),
		}
		if ps.Spec.SessionDuration != "" {
			input.SessionDuration = aws.String(ps.Spec.SessionDuration)
		}
		if ps.Spec.RelayState != "" {
			input.RelayState = aws.String(ps.Spec.RelayState)
		}
		if _, err := r.SSOAdminClient.UpdatePermissionSet(ctx, input); err != nil {
			return fmt.Errorf("update permission set: %w", err)
		}
		if len(ps.Spec.Tags) > 0 {
			if _, err := r.SSOAdminClient.TagResource(ctx, &awssso.TagResourceInput{
				InstanceArn: instanceArn,
				ResourceArn: aws.String(ps.Status.PermissionSetArn),
				Tags:        ssoTags(ps.Spec.Tags),
			}); err != nil {
				return fmt.Errorf("tag permission set: %w", err)
			}
		}
	}

	if err := r.syncManagedPolicies(ctx, ps); err != nil {
		return err
	}
	if err := r.syncInlinePolicy(ctx, ps); err != nil {
		return err
	}

	ps.Status.ObservedGeneration = ps.Generation
	now := metav1.Now()
	ps.Status.LastSyncTime = &now
	return r.setCondition(ctx, ps, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "permission set reconciled")
}

// syncManagedPolicies attaches missing and detaches extra AWS managed
// policies so the live set matches spec.managedPolicies.
func (r *PermissionSetReconciler) syncManagedPolicies(ctx context.Context, ps *awsv1alpha1.PermissionSet) error {
	instanceArn := aws.String(ps.Spec.InstanceArn)
	psArn := aws.String(ps.Status.PermissionSetArn)

	current := map[string]bool{}
	var next *string
	for {
		out, err := r.SSOAdminClient.ListManagedPoliciesInPermissionSet(ctx, &awssso.ListManagedPoliciesInPermissionSetInput{
			InstanceArn:      instanceArn,
			PermissionSetArn: psArn,
			NextToken:        next,
		})
		if err != nil {
			return fmt.Errorf("list managed policies: %w", err)
		}
		for _, p := range out.AttachedManagedPolicies {
			current[aws.ToString(p.Arn)] = true
		}
		if out.NextToken == nil {
			break
		}
		next = out.NextToken
	}

	desired := map[string]bool{}
	for _, arn := range ps.Spec.ManagedPolicies {
		desired[arn] = true
	}

	for arn := range desired {
		if !current[arn] {
			if _, err := r.SSOAdminClient.AttachManagedPolicyToPermissionSet(ctx, &awssso.AttachManagedPolicyToPermissionSetInput{
				InstanceArn:      instanceArn,
				PermissionSetArn: psArn,
				ManagedPolicyArn: aws.String(arn),
			}); err != nil {
				return fmt.Errorf("attach managed policy %s: %w", arn, err)
			}
		}
	}
	for arn := range current {
		if !desired[arn] {
			if _, err := r.SSOAdminClient.DetachManagedPolicyFromPermissionSet(ctx, &awssso.DetachManagedPolicyFromPermissionSetInput{
				InstanceArn:      instanceArn,
				PermissionSetArn: psArn,
				ManagedPolicyArn: aws.String(arn),
			}); err != nil {
				return fmt.Errorf("detach managed policy %s: %w", arn, err)
			}
		}
	}
	return nil
}

func (r *PermissionSetReconciler) syncInlinePolicy(ctx context.Context, ps *awsv1alpha1.PermissionSet) error {
	instanceArn := aws.String(ps.Spec.InstanceArn)
	psArn := aws.String(ps.Status.PermissionSetArn)

	if ps.Spec.InlinePolicy != "" {
		if _, err := r.SSOAdminClient.PutInlinePolicyToPermissionSet(ctx, &awssso.PutInlinePolicyToPermissionSetInput{
			InstanceArn:      instanceArn,
			PermissionSetArn: psArn,
			InlinePolicy:     aws.String(ps.Spec.InlinePolicy),
		}); err != nil {
			return fmt.Errorf("put inline policy: %w", err)
		}
		return nil
	}
	// Spec has no inline policy: delete any lingering one. Deleting a
	// non-existent inline policy returns ResourceNotFoundException — benign.
	if _, err := r.SSOAdminClient.DeleteInlinePolicyFromPermissionSet(ctx, &awssso.DeleteInlinePolicyFromPermissionSetInput{
		InstanceArn:      instanceArn,
		PermissionSetArn: psArn,
	}); err != nil && !ssohelper.IsNotFound(err) {
		return fmt.Errorf("delete inline policy: %w", err)
	}
	return nil
}

func (r *PermissionSetReconciler) deletePermissionSet(ctx context.Context, ps *awsv1alpha1.PermissionSet) error {
	if ps.Status.PermissionSetArn == "" {
		// Never created (or the identifier was lost). Permission set ARNs
		// cannot be derived from the spec, so do not guess.
		return nil
	}
	_, err := r.SSOAdminClient.DeletePermissionSet(ctx, &awssso.DeletePermissionSetInput{
		InstanceArn:      aws.String(ps.Spec.InstanceArn),
		PermissionSetArn: aws.String(ps.Status.PermissionSetArn),
	})
	if ssohelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *PermissionSetReconciler) setCondition(ctx context.Context, ps *awsv1alpha1.PermissionSet, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&ps.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: ps.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, ps); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *PermissionSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.PermissionSet{}).
		Complete(r)
}
