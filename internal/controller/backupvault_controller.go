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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	backuphelper "github.com/konfig-io/konfig-konector/internal/aws/backup"
)

// BackupVaultAWSAPI is the subset of the AWS Backup API used by this controller.
type BackupVaultAWSAPI interface {
	DescribeBackupVault(ctx context.Context, params *awsbackup.DescribeBackupVaultInput, optFns ...func(*awsbackup.Options)) (*awsbackup.DescribeBackupVaultOutput, error)
	CreateBackupVault(ctx context.Context, params *awsbackup.CreateBackupVaultInput, optFns ...func(*awsbackup.Options)) (*awsbackup.CreateBackupVaultOutput, error)
	DeleteBackupVault(ctx context.Context, params *awsbackup.DeleteBackupVaultInput, optFns ...func(*awsbackup.Options)) (*awsbackup.DeleteBackupVaultOutput, error)
	TagResource(ctx context.Context, params *awsbackup.TagResourceInput, optFns ...func(*awsbackup.Options)) (*awsbackup.TagResourceOutput, error)
}

// BackupVaultReconciler reconciles BackupVault objects.
type BackupVaultReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	BackupClient BackupVaultAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=backupvaults,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=backupvaults/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=backupvaults/finalizers,verbs=update

func (r *BackupVaultReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	vault := &awsv1alpha1.BackupVault{}
	if err := r.Get(ctx, req.NamespacedName, vault); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !vault.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(vault, awsv1alpha1.FinalizerName) {
			if shouldAbandon(vault) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(vault, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, vault)
			}
			if err := r.deleteVault(ctx, vault); err != nil {
				logger.Error(err, "failed to delete backup vault")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(vault, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, vault)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(vault, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(vault, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, vault); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileVault(ctx, vault); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, vault, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *BackupVaultReconciler) reconcileVault(ctx context.Context, vault *awsv1alpha1.BackupVault) error {
	descOut, err := r.BackupClient.DescribeBackupVault(ctx, &awsbackup.DescribeBackupVaultInput{
		BackupVaultName: aws.String(vault.Spec.VaultName),
	})

	var vaultARN string
	if backuphelper.IsNotFound(err) {
		createOut, err := r.BackupClient.CreateBackupVault(ctx, &awsbackup.CreateBackupVaultInput{
			BackupVaultName:  aws.String(vault.Spec.VaultName),
			EncryptionKeyArn: govOptionalStr(vault.Spec.KMSKeyARN),
			BackupVaultTags:  vault.Spec.Tags,
		})
		if err != nil {
			return fmt.Errorf("create backup vault: %w", err)
		}
		vaultARN = aws.ToString(createOut.BackupVaultArn)
		// Persist the ARN immediately after create.
		vault.Status.VaultARN = vaultARN
		if err := persistStatus(ctx, r.Client, vault); err != nil {
			return fmt.Errorf("persist vault ARN after create: %w", err)
		}
	} else if err != nil {
		return err
	} else {
		vaultARN = aws.ToString(descOut.BackupVaultArn)
		if len(vault.Spec.Tags) > 0 && vault.Status.ObservedGeneration != vault.Generation {
			if _, err := r.BackupClient.TagResource(ctx, &awsbackup.TagResourceInput{
				ResourceArn: aws.String(vaultARN),
				Tags:        vault.Spec.Tags,
			}); err != nil {
				return fmt.Errorf("tag backup vault: %w", err)
			}
		}
	}

	vault.Status.VaultARN = vaultARN
	vault.Status.ObservedGeneration = vault.Generation
	now := metav1.Now()
	vault.Status.LastSyncTime = &now
	return r.setCondition(ctx, vault, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "backup vault reconciled")
}

func (r *BackupVaultReconciler) deleteVault(ctx context.Context, vault *awsv1alpha1.BackupVault) error {
	// The vault name is a deterministic spec-based identifier.
	_, err := r.BackupClient.DeleteBackupVault(ctx, &awsbackup.DeleteBackupVaultInput{
		BackupVaultName: aws.String(vault.Spec.VaultName),
	})
	if backuphelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *BackupVaultReconciler) setCondition(ctx context.Context, vault *awsv1alpha1.BackupVault, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&vault.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: vault.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, vault); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *BackupVaultReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.BackupVault{}).
		Complete(r)
}
