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
)

// DynamoDBTableAWSAPI is the subset of the DynamoDB SDK client used by this
// controller. *awsddb.Client satisfies it.
type DynamoDBTableAWSAPI interface {
	DescribeTable(ctx context.Context, params *awsddb.DescribeTableInput, optFns ...func(*awsddb.Options)) (*awsddb.DescribeTableOutput, error)
	CreateTable(ctx context.Context, params *awsddb.CreateTableInput, optFns ...func(*awsddb.Options)) (*awsddb.CreateTableOutput, error)
	UpdateTable(ctx context.Context, params *awsddb.UpdateTableInput, optFns ...func(*awsddb.Options)) (*awsddb.UpdateTableOutput, error)
	UpdateContinuousBackups(ctx context.Context, params *awsddb.UpdateContinuousBackupsInput, optFns ...func(*awsddb.Options)) (*awsddb.UpdateContinuousBackupsOutput, error)
	DeleteTable(ctx context.Context, params *awsddb.DeleteTableInput, optFns ...func(*awsddb.Options)) (*awsddb.DeleteTableOutput, error)
}

// DynamoDBTableReconciler reconciles DynamoDBTable objects.
type DynamoDBTableReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	DynamoDBClient DynamoDBTableAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=dynamodbtables,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=dynamodbtables/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=dynamodbtables/finalizers,verbs=update

func (r *DynamoDBTableReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	t := &awsv1alpha1.DynamoDBTable{}
	if err := r.Get(ctx, req.NamespacedName, t); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !t.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(t, awsv1alpha1.FinalizerName) {
			if shouldAbandon(t) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(t, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, t)
			}
			if err := r.deleteDynamoDBTable(ctx, t); err != nil {
				logger.Error(err, "failed to delete DynamoDBTable")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(t, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, t)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(t, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(t, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, t); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileDynamoDBTable(ctx, t); err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setConditionDDB(ctx, t, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *DynamoDBTableReconciler) reconcileDynamoDBTable(ctx context.Context, t *awsv1alpha1.DynamoDBTable) error {
	if t.Status.ARN != "" {
		out, err := r.DynamoDBClient.DescribeTable(ctx, &awsddb.DescribeTableInput{
			TableName: aws.String(t.Spec.TableName),
		})
		if err != nil && !ddbhelper.IsNotFound(err) {
			return fmt.Errorf("describe table: %w", err)
		}
		if err == nil {
			t.Status.TableStatus = string(out.Table.TableStatus)
			if t.Status.ObservedGeneration != t.Generation {
				if err := r.updateDynamoDBTable(ctx, t, out.Table); err != nil {
					return err
				}
			}
			t.Status.ObservedGeneration = t.Generation
			now := metav1.Now()
			t.Status.LastSyncTime = &now
			return r.setConditionDDB(ctx, t, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "DynamoDBTable reconciled")
		}
		t.Status.ARN = ""
	}

	attrDefs := make([]ddbtypes.AttributeDefinition, 0, len(t.Spec.AttributeDefinitions))
	for _, ad := range t.Spec.AttributeDefinitions {
		ad := ad
		attrDefs = append(attrDefs, ddbtypes.AttributeDefinition{
			AttributeName: aws.String(ad.AttributeName),
			AttributeType: ddbtypes.ScalarAttributeType(ad.AttributeType),
		})
	}

	keySchema := make([]ddbtypes.KeySchemaElement, 0, len(t.Spec.KeySchema))
	for _, ks := range t.Spec.KeySchema {
		ks := ks
		keySchema = append(keySchema, ddbtypes.KeySchemaElement{
			AttributeName: aws.String(ks.AttributeName),
			KeyType:       ddbtypes.KeyType(ks.KeyType),
		})
	}

	input := &awsddb.CreateTableInput{
		TableName:            aws.String(t.Spec.TableName),
		AttributeDefinitions: attrDefs,
		KeySchema:            keySchema,
	}

	if t.Spec.BillingMode != "" {
		input.BillingMode = ddbtypes.BillingMode(t.Spec.BillingMode)
	}
	if t.Spec.ProvisionedThroughput != nil {
		input.ProvisionedThroughput = &ddbtypes.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(t.Spec.ProvisionedThroughput.ReadCapacityUnits),
			WriteCapacityUnits: aws.Int64(t.Spec.ProvisionedThroughput.WriteCapacityUnits),
		}
	}
	if t.Spec.SSEEnabled {
		sse := &ddbtypes.SSESpecification{Enabled: aws.Bool(true)}
		if t.Spec.KMSKeyARN != "" {
			sse.SSEType = ddbtypes.SSETypeKms
			sse.KMSMasterKeyId = aws.String(t.Spec.KMSKeyARN)
		}
		input.SSESpecification = sse
	}
	if len(t.Spec.Tags) > 0 {
		tags := make([]ddbtypes.Tag, 0, len(t.Spec.Tags))
		for k, v := range t.Spec.Tags {
			k, v := k, v
			tags = append(tags, ddbtypes.Tag{Key: &k, Value: &v})
		}
		input.Tags = tags
	}

	out, err := r.DynamoDBClient.CreateTable(ctx, input)
	if err != nil {
		return fmt.Errorf("create table: %w", err)
	}

	t.Status.ARN = aws.ToString(out.TableDescription.TableArn)
	t.Status.TableStatus = string(out.TableDescription.TableStatus)
	// Persist the ARN immediately: the AWS resource now exists, and losing the
	// identifier would leave later steps (PITR) re-creating state on retry.
	if err := persistStatus(ctx, r.Client, t); err != nil {
		return fmt.Errorf("persist table ARN after create: %w", err)
	}

	if t.Spec.PointInTimeRecovery {
		if _, err2 := r.DynamoDBClient.UpdateContinuousBackups(ctx, &awsddb.UpdateContinuousBackupsInput{
			TableName: aws.String(t.Spec.TableName),
			PointInTimeRecoverySpecification: &ddbtypes.PointInTimeRecoverySpecification{
				PointInTimeRecoveryEnabled: aws.Bool(true),
			},
		}); err2 != nil {
			return fmt.Errorf("enable pitr: %w", err2)
		}
	}

	t.Status.ObservedGeneration = t.Generation
	now := metav1.Now()
	t.Status.LastSyncTime = &now
	return r.setConditionDDB(ctx, t, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "DynamoDBTable created")
}

// updateDynamoDBTable applies in-place updates for the mutable fields the spec
// models: billing mode / provisioned throughput, SSE settings, and PITR.
func (r *DynamoDBTableReconciler) updateDynamoDBTable(ctx context.Context, t *awsv1alpha1.DynamoDBTable, actual *ddbtypes.TableDescription) error {
	update := &awsddb.UpdateTableInput{TableName: aws.String(t.Spec.TableName)}
	needsUpdate := false

	// Billing mode. AWS reports no BillingModeSummary for legacy PROVISIONED tables.
	actualBilling := string(ddbtypes.BillingModeProvisioned)
	if actual.BillingModeSummary != nil && actual.BillingModeSummary.BillingMode != "" {
		actualBilling = string(actual.BillingModeSummary.BillingMode)
	}
	if t.Spec.BillingMode != "" && t.Spec.BillingMode != actualBilling {
		update.BillingMode = ddbtypes.BillingMode(t.Spec.BillingMode)
		needsUpdate = true
	}

	// Provisioned throughput (only meaningful when not PAY_PER_REQUEST).
	if t.Spec.ProvisionedThroughput != nil && t.Spec.BillingMode != string(ddbtypes.BillingModePayPerRequest) {
		var curRead, curWrite int64
		if actual.ProvisionedThroughput != nil {
			curRead = aws.ToInt64(actual.ProvisionedThroughput.ReadCapacityUnits)
			curWrite = aws.ToInt64(actual.ProvisionedThroughput.WriteCapacityUnits)
		}
		if curRead != t.Spec.ProvisionedThroughput.ReadCapacityUnits || curWrite != t.Spec.ProvisionedThroughput.WriteCapacityUnits {
			update.ProvisionedThroughput = &ddbtypes.ProvisionedThroughput{
				ReadCapacityUnits:  aws.Int64(t.Spec.ProvisionedThroughput.ReadCapacityUnits),
				WriteCapacityUnits: aws.Int64(t.Spec.ProvisionedThroughput.WriteCapacityUnits),
			}
			needsUpdate = true
		}
	}

	// SSE settings.
	actualSSE := actual.SSEDescription != nil && actual.SSEDescription.Status == ddbtypes.SSEStatusEnabled
	if t.Spec.SSEEnabled != actualSSE {
		sse := &ddbtypes.SSESpecification{Enabled: aws.Bool(t.Spec.SSEEnabled)}
		if t.Spec.SSEEnabled && t.Spec.KMSKeyARN != "" {
			sse.SSEType = ddbtypes.SSETypeKms
			sse.KMSMasterKeyId = aws.String(t.Spec.KMSKeyARN)
		}
		update.SSESpecification = sse
		needsUpdate = true
	}

	if needsUpdate {
		if _, err := r.DynamoDBClient.UpdateTable(ctx, update); err != nil {
			return fmt.Errorf("update table: %w", err)
		}
	}

	// Point-in-time recovery (idempotent).
	if _, err := r.DynamoDBClient.UpdateContinuousBackups(ctx, &awsddb.UpdateContinuousBackupsInput{
		TableName: aws.String(t.Spec.TableName),
		PointInTimeRecoverySpecification: &ddbtypes.PointInTimeRecoverySpecification{
			PointInTimeRecoveryEnabled: aws.Bool(t.Spec.PointInTimeRecovery),
		},
	}); err != nil {
		return fmt.Errorf("update pitr: %w", err)
	}
	return nil
}

func (r *DynamoDBTableReconciler) deleteDynamoDBTable(ctx context.Context, t *awsv1alpha1.DynamoDBTable) error {
	if t.Spec.TableName == "" {
		return nil
	}
	_, err := r.DynamoDBClient.DeleteTable(ctx, &awsddb.DeleteTableInput{
		TableName: aws.String(t.Spec.TableName),
	})
	if ddbhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *DynamoDBTableReconciler) setConditionDDB(ctx context.Context, t *awsv1alpha1.DynamoDBTable, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&t.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: t.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, t); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *DynamoDBTableReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.DynamoDBTable{}).
		Complete(r)
}
