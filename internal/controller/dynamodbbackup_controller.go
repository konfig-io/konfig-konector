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

	"github.com/aws/aws-sdk-go-v2/aws"
	awsdynamodb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	dynamohelper "github.com/konfig-io/konfig-konector/internal/aws/dynamodb"
)

// DynamoDBBackupReconciler reconciles DynamoDBBackup objects.
type DynamoDBBackupReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	DynamoDBClient *awsdynamodb.Client
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=dynamodbbackups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=dynamodbbackups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=dynamodbbackups/finalizers,verbs=update

func (r *DynamoDBBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.DynamoDBBackup{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !obj.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(obj, awsv1alpha1.FinalizerName) {
			if shouldAbandon(obj) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(obj, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, obj)
			}
			if err := r.deleteBackup(ctx, obj); err != nil {
				logger.Error(err, "failed to delete DynamoDBBackup")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(obj, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, obj)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(obj, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(obj, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, obj); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileBackup(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setConditionDB(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *DynamoDBBackupReconciler) reconcileBackup(ctx context.Context, obj *awsv1alpha1.DynamoDBBackup) error {
	// Backups are immutable: once created, just verify existence.
	if obj.Status.BackupARN != "" {
		descOut, err := r.DynamoDBClient.DescribeBackup(ctx, &awsdynamodb.DescribeBackupInput{
			BackupArn: aws.String(obj.Status.BackupARN),
		})
		if err != nil && !dynamohelper.IsNotFound(err) {
			return fmt.Errorf("describe dynamodb backup: %w", err)
		}
		if err == nil && descOut.BackupDescription != nil && descOut.BackupDescription.BackupDetails != nil {
			obj.Status.BackupStatus = string(descOut.BackupDescription.BackupDetails.BackupStatus)
			obj.Status.ObservedGeneration = obj.Generation
			now := metav1.Now()
			obj.Status.LastSyncTime = &now
			return r.setConditionDB(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "DynamoDBBackup reconciled")
		}
		obj.Status.BackupARN = ""
	}

	out, err := r.DynamoDBClient.CreateBackup(ctx, &awsdynamodb.CreateBackupInput{
		TableName:  aws.String(obj.Spec.TableName),
		BackupName: aws.String(obj.Spec.BackupName),
	})
	if err != nil {
		return fmt.Errorf("create dynamodb backup: %w", err)
	}

	if out.BackupDetails != nil {
		obj.Status.BackupARN = aws.ToString(out.BackupDetails.BackupArn)
		obj.Status.BackupStatus = string(out.BackupDetails.BackupStatus)
		// Persist the ARN immediately: the AWS resource now exists, and losing
		// the identifier would orphan it or create a duplicate on retry.
		if err := persistStatus(ctx, r.Client, obj); err != nil {
			return fmt.Errorf("persist backup ARN after create: %w", err)
		}
	}
	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setConditionDB(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "DynamoDBBackup created")
}

func (r *DynamoDBBackupReconciler) deleteBackup(ctx context.Context, obj *awsv1alpha1.DynamoDBBackup) error {
	if obj.Status.BackupARN == "" {
		return nil
	}
	_, err := r.DynamoDBClient.DeleteBackup(ctx, &awsdynamodb.DeleteBackupInput{
		BackupArn: aws.String(obj.Status.BackupARN),
	})
	if dynamohelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *DynamoDBBackupReconciler) setConditionDB(ctx context.Context, obj *awsv1alpha1.DynamoDBBackup, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *DynamoDBBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.DynamoDBBackup{}).
		Complete(r)
}
