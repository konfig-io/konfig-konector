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

	awsiam "github.com/aws/aws-sdk-go-v2/service/iam"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	iamhelper "github.com/konfig-io/konfig-konector/internal/aws/iam"

	"github.com/aws/aws-sdk-go-v2/aws"
)

// IAMPolicyAttachmentAWSAPI is the subset of the IAM SDK client used by this
// controller (directly and via iamhelper.IsPolicyAttached). *iam.Client satisfies it.
type IAMPolicyAttachmentAWSAPI interface {
	iamhelper.AttachedPolicyAPI
	AttachRolePolicy(ctx context.Context, params *awsiam.AttachRolePolicyInput, optFns ...func(*awsiam.Options)) (*awsiam.AttachRolePolicyOutput, error)
	DetachRolePolicy(ctx context.Context, params *awsiam.DetachRolePolicyInput, optFns ...func(*awsiam.Options)) (*awsiam.DetachRolePolicyOutput, error)
}

// IAMPolicyAttachmentReconciler reconciles IAMPolicyAttachment objects.
type IAMPolicyAttachmentReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	IAMClient IAMPolicyAttachmentAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=iampolicyattachments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=iampolicyattachments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=iampolicyattachments/finalizers,verbs=update

func (r *IAMPolicyAttachmentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	att := &awsv1alpha1.IAMPolicyAttachment{}
	if err := r.Get(ctx, req.NamespacedName, att); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, att); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !att.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(att, awsv1alpha1.FinalizerName) {
			if shouldAbandon(att) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(att, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, att)
			}
			if att.Status.RoleARN != "" && att.Status.PolicyARN != "" {
				if _, err := r.IAMClient.DetachRolePolicy(ctx, &awsiam.DetachRolePolicyInput{
					RoleName:  aws.String(roleNameFromARN(att.Status.RoleARN)),
					PolicyArn: aws.String(att.Status.PolicyARN),
				}); err != nil && !iamhelper.IsNotFound(err) {
					logger.Error(err, "failed to detach policy")
					return ctrl.Result{}, err
				}
			}
			controllerutil.RemoveFinalizer(att, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, att)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(att, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(att, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, att); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileAttachment(ctx, att); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setAttCondition(ctx, att, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *IAMPolicyAttachmentReconciler) reconcileAttachment(ctx context.Context, att *awsv1alpha1.IAMPolicyAttachment) error {
	roleARN, err := r.resolveRoleARN(ctx, att)
	if err != nil {
		return fmt.Errorf("resolve role: %w", err)
	}
	policyARN, err := r.resolvePolicyARN(ctx, att)
	if err != nil {
		return fmt.Errorf("resolve policy: %w", err)
	}

	roleName := roleNameFromARN(roleARN)

	attached, err := iamhelper.IsPolicyAttached(ctx, r.IAMClient, roleName, policyARN)
	if err != nil {
		return err
	}
	if !attached {
		if _, err := r.IAMClient.AttachRolePolicy(ctx, &awsiam.AttachRolePolicyInput{
			RoleName:  aws.String(roleName),
			PolicyArn: aws.String(policyARN),
		}); err != nil {
			return fmt.Errorf("attach policy: %w", err)
		}
	}

	att.Status.Attached = true
	att.Status.RoleARN = roleARN
	att.Status.PolicyARN = policyARN
	// Persist the ARN pair immediately: the attachment now exists in AWS, and
	// the delete path depends on both identifiers being in status.
	if err := persistStatus(ctx, r.Client, att); err != nil {
		return fmt.Errorf("persist attachment identifiers: %w", err)
	}
	att.Status.ObservedGeneration = att.Generation
	now := metav1.Now()
	att.Status.LastSyncTime = &now
	return r.setAttCondition(ctx, att, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "policy attached")
}

func (r *IAMPolicyAttachmentReconciler) resolveRoleARN(ctx context.Context, att *awsv1alpha1.IAMPolicyAttachment) (string, error) {
	if att.Spec.RoleRef.ARN != "" {
		return att.Spec.RoleRef.ARN, nil
	}
	ns := att.Spec.RoleRef.Namespace
	if ns == "" {
		ns = att.Namespace
	}
	role := &awsv1alpha1.IAMRole{}
	if err := r.Get(ctx, types.NamespacedName{Name: att.Spec.RoleRef.Name, Namespace: ns}, role); err != nil {
		return "", err
	}
	if role.Status.ARN == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("IAMRole %s/%s has no ARN yet", ns, att.Spec.RoleRef.Name)}
	}
	return role.Status.ARN, nil
}

func (r *IAMPolicyAttachmentReconciler) resolvePolicyARN(ctx context.Context, att *awsv1alpha1.IAMPolicyAttachment) (string, error) {
	if att.Spec.PolicyRef.ARN != "" {
		return att.Spec.PolicyRef.ARN, nil
	}
	ns := att.Spec.PolicyRef.Namespace
	if ns == "" {
		ns = att.Namespace
	}
	pol := &awsv1alpha1.IAMPolicy{}
	if err := r.Get(ctx, types.NamespacedName{Name: att.Spec.PolicyRef.Name, Namespace: ns}, pol); err != nil {
		return "", err
	}
	if pol.Status.ARN == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("IAMPolicy %s/%s has no ARN yet", ns, att.Spec.PolicyRef.Name)}
	}
	return pol.Status.ARN, nil
}

func (r *IAMPolicyAttachmentReconciler) setAttCondition(ctx context.Context, att *awsv1alpha1.IAMPolicyAttachment, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&att.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: att.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, att); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *IAMPolicyAttachmentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.IAMPolicyAttachment{}).
		Complete(r)
}
