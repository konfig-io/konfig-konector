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
	awsorgs "github.com/aws/aws-sdk-go-v2/service/organizations"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	orghelper "github.com/konfig-io/konfig-konector/internal/aws/organizations"
)

// OrganizationsPolicyAttachmentAWSAPI is the subset of the Organizations API
// used by this controller.
type OrganizationsPolicyAttachmentAWSAPI interface {
	AttachPolicy(ctx context.Context, params *awsorgs.AttachPolicyInput, optFns ...func(*awsorgs.Options)) (*awsorgs.AttachPolicyOutput, error)
	DetachPolicy(ctx context.Context, params *awsorgs.DetachPolicyInput, optFns ...func(*awsorgs.Options)) (*awsorgs.DetachPolicyOutput, error)
}

// OrganizationsPolicyAttachmentReconciler reconciles OrganizationsPolicyAttachment objects.
type OrganizationsPolicyAttachmentReconciler struct {
	client.Client
	Scheme              *runtime.Scheme
	OrganizationsClient OrganizationsPolicyAttachmentAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=organizationspolicyattachments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=organizationspolicyattachments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=organizationspolicyattachments/finalizers,verbs=update

func (r *OrganizationsPolicyAttachmentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	att := &awsv1alpha1.OrganizationsPolicyAttachment{}
	if err := r.Get(ctx, req.NamespacedName, att); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !att.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(att, awsv1alpha1.FinalizerName) {
			if shouldAbandon(att) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(att, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, att)
			}
			if att.Status.PolicyID != "" && att.Status.TargetID != "" {
				if _, err := r.OrganizationsClient.DetachPolicy(ctx, &awsorgs.DetachPolicyInput{
					PolicyId: aws.String(att.Status.PolicyID),
					TargetId: aws.String(att.Status.TargetID),
				}); err != nil && !orghelper.IsNotFound(err) {
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
	}

	if err := r.reconcileAttachment(ctx, att); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, att, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *OrganizationsPolicyAttachmentReconciler) resolvePolicyID(ctx context.Context, att *awsv1alpha1.OrganizationsPolicyAttachment) (string, error) {
	if att.Spec.PolicyRef.PolicyID != "" {
		return att.Spec.PolicyRef.PolicyID, nil
	}
	if att.Spec.PolicyRef.Name == "" {
		return "", fmt.Errorf("policyRef must set either name or policyId")
	}
	pol := &awsv1alpha1.OrganizationsPolicy{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: att.Spec.PolicyRef.Name, Namespace: att.Namespace}, pol); err != nil {
		return "", err
	}
	if pol.Status.PolicyID == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("OrganizationsPolicy %s/%s has no policy ID yet", att.Namespace, att.Spec.PolicyRef.Name)}
	}
	return pol.Status.PolicyID, nil
}

func (r *OrganizationsPolicyAttachmentReconciler) reconcileAttachment(ctx context.Context, att *awsv1alpha1.OrganizationsPolicyAttachment) error {
	policyID, err := r.resolvePolicyID(ctx, att)
	if err != nil {
		return err
	}

	_, err = r.OrganizationsClient.AttachPolicy(ctx, &awsorgs.AttachPolicyInput{
		PolicyId: aws.String(policyID),
		TargetId: aws.String(att.Spec.TargetID),
	})
	if err != nil && !orghelper.IsDuplicateAttachment(err) {
		return fmt.Errorf("attach policy: %w", err)
	}

	att.Status.PolicyID = policyID
	att.Status.TargetID = att.Spec.TargetID
	if err := persistStatus(ctx, r.Client, att); err != nil {
		return fmt.Errorf("persist attachment identifiers: %w", err)
	}

	att.Status.ObservedGeneration = att.Generation
	now := metav1.Now()
	att.Status.LastSyncTime = &now
	return r.setCondition(ctx, att, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "policy attachment reconciled")
}

func (r *OrganizationsPolicyAttachmentReconciler) setCondition(ctx context.Context, att *awsv1alpha1.OrganizationsPolicyAttachment, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *OrganizationsPolicyAttachmentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.OrganizationsPolicyAttachment{}).
		Complete(r)
}
