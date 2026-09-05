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
	awsddb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	ddbhelper "github.com/konfig-io/konfig-konector/internal/aws/dynamodb"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// DynamoDBGlobalTableReconciler reconciles DynamoDBGlobalTable objects.
type DynamoDBGlobalTableReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	DynamoDBClient *multi.DynamoDB
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=dynamodbglobaltables,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=dynamodbglobaltables/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=dynamodbglobaltables/finalizers,verbs=update

func (r *DynamoDBGlobalTableReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	gt := &awsv1alpha1.DynamoDBGlobalTable{}
	if err := r.Get(ctx, req.NamespacedName, gt); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, gt); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !gt.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(gt, awsv1alpha1.FinalizerName) {
			if shouldAbandon(gt) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(gt, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, gt)
			}
			controllerutil.RemoveFinalizer(gt, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, gt)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(gt, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(gt, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, gt); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileDynamoDBGlobalTable(ctx, gt); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionGT(ctx, gt, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *DynamoDBGlobalTableReconciler) reconcileDynamoDBGlobalTable(ctx context.Context, gt *awsv1alpha1.DynamoDBGlobalTable) error {
	if gt.Status.ARN != "" {
		out, err := r.DynamoDBClient.DescribeGlobalTable(ctx, &awsddb.DescribeGlobalTableInput{
			GlobalTableName: aws.String(gt.Spec.TableName),
		})
		if err != nil && !ddbhelper.IsNotFound(err) {
			return fmt.Errorf("describe global table: %w", err)
		}
		if err == nil {
			gt.Status.GlobalTableStatus = string(out.GlobalTableDescription.GlobalTableStatus)
			gt.Status.ObservedGeneration = gt.Generation
			now := metav1.Now()
			gt.Status.LastSyncTime = &now
			return r.setConditionGT(ctx, gt, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "DynamoDBGlobalTable reconciled")
		}
		gt.Status.ARN = ""
	}

	replicas := make([]ddbtypes.Replica, 0, len(gt.Spec.ReplicationGroup))
	for _, r := range gt.Spec.ReplicationGroup {
		r := r
		replicas = append(replicas, ddbtypes.Replica{RegionName: aws.String(r.RegionName)})
	}

	out, err := r.DynamoDBClient.CreateGlobalTable(ctx, &awsddb.CreateGlobalTableInput{
		GlobalTableName:  aws.String(gt.Spec.TableName),
		ReplicationGroup: replicas,
	})
	if err != nil {
		return fmt.Errorf("create global table: %w", err)
	}

	gt.Status.ARN = aws.ToString(out.GlobalTableDescription.GlobalTableArn)
	gt.Status.GlobalTableStatus = string(out.GlobalTableDescription.GlobalTableStatus)
	gt.Status.ObservedGeneration = gt.Generation
	now := metav1.Now()
	gt.Status.LastSyncTime = &now
	return r.setConditionGT(ctx, gt, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "DynamoDBGlobalTable created")
}

func (r *DynamoDBGlobalTableReconciler) setConditionGT(ctx context.Context, gt *awsv1alpha1.DynamoDBGlobalTable, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&gt.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: gt.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, gt); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *DynamoDBGlobalTableReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.DynamoDBGlobalTable{}).
		Complete(r)
}
