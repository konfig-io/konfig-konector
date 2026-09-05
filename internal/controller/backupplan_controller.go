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

// BackupPlanAWSAPI is the subset of the AWS Backup API used by this controller.
type BackupPlanAWSAPI interface {
	GetBackupPlan(ctx context.Context, params *awsbackup.GetBackupPlanInput, optFns ...func(*awsbackup.Options)) (*awsbackup.GetBackupPlanOutput, error)
	CreateBackupPlan(ctx context.Context, params *awsbackup.CreateBackupPlanInput, optFns ...func(*awsbackup.Options)) (*awsbackup.CreateBackupPlanOutput, error)
	UpdateBackupPlan(ctx context.Context, params *awsbackup.UpdateBackupPlanInput, optFns ...func(*awsbackup.Options)) (*awsbackup.UpdateBackupPlanOutput, error)
	DeleteBackupPlan(ctx context.Context, params *awsbackup.DeleteBackupPlanInput, optFns ...func(*awsbackup.Options)) (*awsbackup.DeleteBackupPlanOutput, error)
}

// BackupPlanReconciler reconciles BackupPlan objects.
type BackupPlanReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	BackupClient BackupPlanAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=backupplans,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=backupplans/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=backupplans/finalizers,verbs=update

func (r *BackupPlanReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	plan := &awsv1alpha1.BackupPlan{}
	if err := r.Get(ctx, req.NamespacedName, plan); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, plan); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !plan.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(plan, awsv1alpha1.FinalizerName) {
			if shouldAbandon(plan) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(plan, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, plan)
			}
			if err := r.deletePlan(ctx, plan); err != nil {
				logger.Error(err, "failed to delete backup plan")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(plan, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, plan)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(plan, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(plan, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, plan); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcilePlan(ctx, plan); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, plan, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *BackupPlanReconciler) reconcilePlan(ctx context.Context, plan *awsv1alpha1.BackupPlan) error {
	// Resolve vault refs up front so a missing/not-ready vault is reported
	// before any AWS mutation.
	planInput, err := r.buildPlanInput(ctx, plan)
	if err != nil {
		return err
	}

	exists := false
	if plan.Status.PlanID != "" {
		_, err := r.BackupClient.GetBackupPlan(ctx, &awsbackup.GetBackupPlanInput{
			BackupPlanId: aws.String(plan.Status.PlanID),
		})
		if err == nil {
			exists = true
		} else if !backuphelper.IsNotFound(err) {
			return err
		}
	}

	if !exists {
		createOut, err := r.BackupClient.CreateBackupPlan(ctx, &awsbackup.CreateBackupPlanInput{
			BackupPlan:     planInput,
			BackupPlanTags: plan.Spec.Tags,
		})
		if err != nil {
			return fmt.Errorf("create backup plan: %w", err)
		}
		// Persist the plan ID immediately: the AWS resource now exists, and
		// backup plan IDs are random so a lost ID orphans the plan.
		plan.Status.PlanID = aws.ToString(createOut.BackupPlanId)
		plan.Status.PlanARN = aws.ToString(createOut.BackupPlanArn)
		plan.Status.VersionID = aws.ToString(createOut.VersionId)
		if err := persistStatus(ctx, r.Client, plan); err != nil {
			return fmt.Errorf("persist plan ID after create: %w", err)
		}
	} else if plan.Status.ObservedGeneration != plan.Generation {
		updateOut, err := r.BackupClient.UpdateBackupPlan(ctx, &awsbackup.UpdateBackupPlanInput{
			BackupPlanId: aws.String(plan.Status.PlanID),
			BackupPlan:   planInput,
		})
		if err != nil {
			return fmt.Errorf("update backup plan: %w", err)
		}
		plan.Status.PlanARN = aws.ToString(updateOut.BackupPlanArn)
		plan.Status.VersionID = aws.ToString(updateOut.VersionId)
	}

	plan.Status.ObservedGeneration = plan.Generation
	now := metav1.Now()
	plan.Status.LastSyncTime = &now
	return r.setCondition(ctx, plan, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "backup plan reconciled")
}

func (r *BackupPlanReconciler) buildPlanInput(ctx context.Context, plan *awsv1alpha1.BackupPlan) (*backuptypes.BackupPlanInput, error) {
	rules := make([]backuptypes.BackupRuleInput, 0, len(plan.Spec.Rules))
	for _, rule := range plan.Spec.Rules {
		vaultName, err := r.resolveVaultName(ctx, plan.Namespace, rule)
		if err != nil {
			return nil, err
		}
		in := backuptypes.BackupRuleInput{
			RuleName:                aws.String(rule.RuleName),
			TargetBackupVaultName:   aws.String(vaultName),
			ScheduleExpression:      govOptionalStr(rule.ScheduleExpression),
			StartWindowMinutes:      rule.StartWindowMinutes,
			CompletionWindowMinutes: rule.CompletionWindowMinutes,
		}
		if lc := rule.Lifecycle; lc != nil {
			in.Lifecycle = &backuptypes.Lifecycle{
				DeleteAfterDays:            lc.DeleteAfterDays,
				MoveToColdStorageAfterDays: lc.MoveToColdStorageAfterDays,
			}
		}
		rules = append(rules, in)
	}
	return &backuptypes.BackupPlanInput{
		BackupPlanName: aws.String(plan.Spec.PlanName),
		Rules:          rules,
	}, nil
}

// resolveVaultName resolves a rule's target vault: direct name wins,
// otherwise a BackupVault CR in the same namespace is looked up.
func (r *BackupPlanReconciler) resolveVaultName(ctx context.Context, namespace string, rule awsv1alpha1.BackupPlanRule) (string, error) {
	if rule.TargetBackupVaultName != "" {
		return rule.TargetBackupVaultName, nil
	}
	if rule.TargetBackupVaultRef == "" {
		return "", fmt.Errorf("rule %q: either targetBackupVaultName or targetBackupVaultRef must be set", rule.RuleName)
	}
	vault := &awsv1alpha1.BackupVault{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: rule.TargetBackupVaultRef, Namespace: namespace}, vault); err != nil {
		return "", err
	}
	if vault.Status.VaultARN == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("BackupVault %s/%s has no ARN yet", namespace, rule.TargetBackupVaultRef)}
	}
	return vault.Spec.VaultName, nil
}

func (r *BackupPlanReconciler) deletePlan(ctx context.Context, plan *awsv1alpha1.BackupPlan) error {
	if plan.Status.PlanID == "" {
		// Backup plan IDs are random; without a stored ID there is no
		// unambiguous spec-based lookup (names are not unique), so abandon.
		return nil
	}
	_, err := r.BackupClient.DeleteBackupPlan(ctx, &awsbackup.DeleteBackupPlanInput{
		BackupPlanId: aws.String(plan.Status.PlanID),
	})
	if backuphelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *BackupPlanReconciler) setCondition(ctx context.Context, plan *awsv1alpha1.BackupPlan, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&plan.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: plan.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, plan); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *BackupPlanReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.BackupPlan{}).
		Complete(r)
}
