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
	awsbackup "github.com/aws/aws-sdk-go-v2/service/backup"
	backuptypes "github.com/aws/aws-sdk-go-v2/service/backup/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	backuphelper "github.com/konfig-io/konfig-konector/internal/aws/backup"
)

// BackupSelectionAWSAPI is the subset of the AWS Backup API used by this controller.
type BackupSelectionAWSAPI interface {
	GetBackupSelection(ctx context.Context, params *awsbackup.GetBackupSelectionInput, optFns ...func(*awsbackup.Options)) (*awsbackup.GetBackupSelectionOutput, error)
	CreateBackupSelection(ctx context.Context, params *awsbackup.CreateBackupSelectionInput, optFns ...func(*awsbackup.Options)) (*awsbackup.CreateBackupSelectionOutput, error)
	DeleteBackupSelection(ctx context.Context, params *awsbackup.DeleteBackupSelectionInput, optFns ...func(*awsbackup.Options)) (*awsbackup.DeleteBackupSelectionOutput, error)
}

// BackupSelectionReconciler reconciles BackupSelection objects.
type BackupSelectionReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	BackupClient BackupSelectionAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=backupselections,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=backupselections/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=backupselections/finalizers,verbs=update

func (r *BackupSelectionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	sel := &awsv1alpha1.BackupSelection{}
	if err := r.Get(ctx, req.NamespacedName, sel); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !sel.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(sel, awsv1alpha1.FinalizerName) {
			if shouldAbandon(sel) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(sel, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, sel)
			}
			if err := r.deleteSelection(ctx, sel); err != nil {
				logger.Error(err, "failed to delete backup selection")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(sel, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, sel)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(sel, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(sel, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, sel); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileSelection(ctx, sel); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, sel, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *BackupSelectionReconciler) reconcileSelection(ctx context.Context, sel *awsv1alpha1.BackupSelection) error {
	planID, err := r.resolvePlanID(ctx, sel)
	if err != nil {
		return err
	}

	roleARN := sel.Spec.IAMRoleARN
	if roleARN == "" {
		if sel.Spec.IAMRoleRef == nil {
			return fmt.Errorf("either iamRoleArn or iamRoleRef must be set")
		}
		arn, err := resolveIAMRoleARN(ctx, r.Client, sel.Namespace, *sel.Spec.IAMRoleRef)
		if err != nil {
			return err
		}
		roleARN = arn
	}

	// Selections are immutable in AWS; if we already created one, verify it
	// still exists and report Ready.
	if sel.Status.SelectionID != "" {
		_, err := r.BackupClient.GetBackupSelection(ctx, &awsbackup.GetBackupSelectionInput{
			BackupPlanId: aws.String(planID),
			SelectionId:  aws.String(sel.Status.SelectionID),
		})
		if err == nil {
			if sel.Status.ObservedGeneration != sel.Generation && sel.Generation > 1 {
				// AWS Backup has no UpdateBackupSelection API.
				return r.setCondition(ctx, sel, awsv1alpha1.ConditionReady, metav1.ConditionFalse,
					awsv1alpha1.ReasonUpdateNotSupported, "backup selections are immutable in AWS; recreate the CR to change it")
			}
			return r.markSynced(ctx, sel, planID)
		}
		if !backuphelper.IsNotFound(err) {
			return err
		}
		// Fall through and recreate.
	}

	selection := &backuptypes.BackupSelection{
		SelectionName: aws.String(sel.Spec.SelectionName),
		IamRoleArn:    aws.String(roleARN),
		Resources:     sel.Spec.Resources,
	}
	for _, tag := range sel.Spec.ListOfTags {
		selection.ListOfTags = append(selection.ListOfTags, backuptypes.Condition{
			ConditionType:  backuptypes.ConditionTypeStringequals,
			ConditionKey:   aws.String(tag.Key),
			ConditionValue: aws.String(tag.Value),
		})
	}

	createOut, err := r.BackupClient.CreateBackupSelection(ctx, &awsbackup.CreateBackupSelectionInput{
		BackupPlanId:    aws.String(planID),
		BackupSelection: selection,
	})
	if err != nil {
		return fmt.Errorf("create backup selection: %w", err)
	}
	// Persist the selection ID immediately after create.
	sel.Status.SelectionID = aws.ToString(createOut.SelectionId)
	sel.Status.PlanID = planID
	if err := persistStatus(ctx, r.Client, sel); err != nil {
		return fmt.Errorf("persist selection ID after create: %w", err)
	}

	return r.markSynced(ctx, sel, planID)
}

func (r *BackupSelectionReconciler) markSynced(ctx context.Context, sel *awsv1alpha1.BackupSelection, planID string) error {
	sel.Status.PlanID = planID
	sel.Status.ObservedGeneration = sel.Generation
	now := metav1.Now()
	sel.Status.LastSyncTime = &now
	return r.setCondition(ctx, sel, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "backup selection reconciled")
}

func (r *BackupSelectionReconciler) resolvePlanID(ctx context.Context, sel *awsv1alpha1.BackupSelection) (string, error) {
	plan := &awsv1alpha1.BackupPlan{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: sel.Spec.PlanRef, Namespace: sel.Namespace}, plan); err != nil {
		return "", err
	}
	if plan.Status.PlanID == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("BackupPlan %s/%s has no ID yet", sel.Namespace, sel.Spec.PlanRef)}
	}
	return plan.Status.PlanID, nil
}

func (r *BackupSelectionReconciler) deleteSelection(ctx context.Context, sel *awsv1alpha1.BackupSelection) error {
	if sel.Status.SelectionID == "" {
		// Selection IDs are random; without a stored ID there is no
		// unambiguous spec-based lookup, so abandon.
		return nil
	}
	planID := sel.Status.PlanID
	if planID == "" {
		plan := &awsv1alpha1.BackupPlan{}
		if err := r.Get(ctx, k8stypes.NamespacedName{Name: sel.Spec.PlanRef, Namespace: sel.Namespace}, plan); err != nil || plan.Status.PlanID == "" {
			return nil
		}
		planID = plan.Status.PlanID
	}
	_, err := r.BackupClient.DeleteBackupSelection(ctx, &awsbackup.DeleteBackupSelectionInput{
		BackupPlanId: aws.String(planID),
		SelectionId:  aws.String(sel.Status.SelectionID),
	})
	if backuphelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *BackupSelectionReconciler) setCondition(ctx context.Context, sel *awsv1alpha1.BackupSelection, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&sel.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: sel.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, sel); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *BackupSelectionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.BackupSelection{}).
		Complete(r)
}
