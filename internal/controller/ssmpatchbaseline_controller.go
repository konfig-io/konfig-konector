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
	awsssm "github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	ssmhelper "github.com/konfig-io/konfig-konector/internal/aws/ssm"
)

// SSMPatchBaselineAWSAPI is the subset of the SSM API used by this controller.
type SSMPatchBaselineAWSAPI interface {
	CreatePatchBaseline(ctx context.Context, params *awsssm.CreatePatchBaselineInput, optFns ...func(*awsssm.Options)) (*awsssm.CreatePatchBaselineOutput, error)
	UpdatePatchBaseline(ctx context.Context, params *awsssm.UpdatePatchBaselineInput, optFns ...func(*awsssm.Options)) (*awsssm.UpdatePatchBaselineOutput, error)
	DeletePatchBaseline(ctx context.Context, params *awsssm.DeletePatchBaselineInput, optFns ...func(*awsssm.Options)) (*awsssm.DeletePatchBaselineOutput, error)
}

// SSMPatchBaselineReconciler reconciles SSMPatchBaseline objects.
type SSMPatchBaselineReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	SSMClient SSMPatchBaselineAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=ssmpatchbaselines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ssmpatchbaselines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=ssmpatchbaselines/finalizers,verbs=update

func (r *SSMPatchBaselineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	pb := &awsv1alpha1.SSMPatchBaseline{}
	if err := r.Get(ctx, req.NamespacedName, pb); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, pb); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !pb.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(pb, awsv1alpha1.FinalizerName) {
			if shouldAbandon(pb) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(pb, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, pb)
			}
			if err := r.deleteBaseline(ctx, pb); err != nil {
				logger.Error(err, "failed to delete patch baseline")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(pb, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, pb)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(pb, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(pb, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, pb); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcileBaseline(ctx, pb); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, pb, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func buildPatchRuleGroup(rules []awsv1alpha1.SSMPatchRule) *ssmtypes.PatchRuleGroup {
	if len(rules) == 0 {
		return nil
	}
	group := &ssmtypes.PatchRuleGroup{}
	for _, rule := range rules {
		pr := ssmtypes.PatchRule{
			PatchFilterGroup: &ssmtypes.PatchFilterGroup{},
		}
		if rule.ApproveAfterDays != nil {
			pr.ApproveAfterDays = rule.ApproveAfterDays
		}
		if rule.ComplianceLevel != "" {
			pr.ComplianceLevel = ssmtypes.PatchComplianceLevel(rule.ComplianceLevel)
		}
		for _, f := range rule.PatchFilters {
			pr.PatchFilterGroup.PatchFilters = append(pr.PatchFilterGroup.PatchFilters, ssmtypes.PatchFilter{
				Key:    ssmtypes.PatchFilterKey(f.Key),
				Values: f.Values,
			})
		}
		group.PatchRules = append(group.PatchRules, pr)
	}
	return group
}

func (r *SSMPatchBaselineReconciler) reconcileBaseline(ctx context.Context, pb *awsv1alpha1.SSMPatchBaseline) error {
	if pb.Status.BaselineID == "" {
		input := &awsssm.CreatePatchBaselineInput{
			Name:            aws.String(pb.Spec.Name),
			OperatingSystem: ssmtypes.OperatingSystem(pb.Spec.OperatingSystem),
			ApprovalRules:   buildPatchRuleGroup(pb.Spec.ApprovalRules),
			ApprovedPatches: pb.Spec.ApprovedPatches,
			RejectedPatches: pb.Spec.RejectedPatches,
		}
		if pb.Spec.Description != "" {
			input.Description = aws.String(pb.Spec.Description)
		}
		for k, v := range pb.Spec.Tags {
			input.Tags = append(input.Tags, ssmtypes.Tag{Key: aws.String(k), Value: aws.String(v)})
		}
		out, err := r.SSMClient.CreatePatchBaseline(ctx, input)
		if err != nil {
			return fmt.Errorf("create patch baseline: %w", err)
		}
		pb.Status.BaselineID = aws.ToString(out.BaselineId)
		if err := persistStatus(ctx, r.Client, pb); err != nil {
			return fmt.Errorf("persist baseline ID after create: %w", err)
		}
	} else if pb.Status.ObservedGeneration != pb.Generation {
		input := &awsssm.UpdatePatchBaselineInput{
			BaselineId:      aws.String(pb.Status.BaselineID),
			Name:            aws.String(pb.Spec.Name),
			ApprovalRules:   buildPatchRuleGroup(pb.Spec.ApprovalRules),
			ApprovedPatches: pb.Spec.ApprovedPatches,
			RejectedPatches: pb.Spec.RejectedPatches,
			// Replace so removed patches/rules do not linger.
			Replace: aws.Bool(true),
		}
		if pb.Spec.Description != "" {
			input.Description = aws.String(pb.Spec.Description)
		}
		if _, err := r.SSMClient.UpdatePatchBaseline(ctx, input); err != nil {
			return fmt.Errorf("update patch baseline: %w", err)
		}
	}

	pb.Status.ObservedGeneration = pb.Generation
	now := metav1.Now()
	pb.Status.LastSyncTime = &now
	return r.setCondition(ctx, pb, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "patch baseline reconciled")
}

func (r *SSMPatchBaselineReconciler) deleteBaseline(ctx context.Context, pb *awsv1alpha1.SSMPatchBaseline) error {
	if pb.Status.BaselineID == "" {
		// Baseline IDs are AWS-generated and names are not unique, so there
		// is no unambiguous spec-based lookup. Nothing to delete.
		return nil
	}
	_, err := r.SSMClient.DeletePatchBaseline(ctx, &awsssm.DeletePatchBaselineInput{
		BaselineId: aws.String(pb.Status.BaselineID),
	})
	if ssmhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *SSMPatchBaselineReconciler) setCondition(ctx context.Context, pb *awsv1alpha1.SSMPatchBaseline, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&pb.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: pb.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, pb); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *SSMPatchBaselineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.SSMPatchBaseline{}).
		Complete(r)
}
