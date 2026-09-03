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
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsddb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

type fakeDynamoDBAPI struct {
	describeTable func(ctx context.Context, params *awsddb.DescribeTableInput) (*awsddb.DescribeTableOutput, error)
	createTable   func(ctx context.Context, params *awsddb.CreateTableInput) (*awsddb.CreateTableOutput, error)
	updateBackups func(ctx context.Context, params *awsddb.UpdateContinuousBackupsInput) (*awsddb.UpdateContinuousBackupsOutput, error)

	createCalled        bool
	updateTableCalled   bool
	updateBackupsCalled bool
	deleteCalled        bool
	updateTableInput    *awsddb.UpdateTableInput
	updateBackupsInput  *awsddb.UpdateContinuousBackupsInput
}

func (f *fakeDynamoDBAPI) DescribeTable(ctx context.Context, params *awsddb.DescribeTableInput, _ ...func(*awsddb.Options)) (*awsddb.DescribeTableOutput, error) {
	if f.describeTable == nil {
		return nil, fmt.Errorf("unexpected call to DescribeTable")
	}
	return f.describeTable(ctx, params)
}

func (f *fakeDynamoDBAPI) CreateTable(ctx context.Context, params *awsddb.CreateTableInput, _ ...func(*awsddb.Options)) (*awsddb.CreateTableOutput, error) {
	f.createCalled = true
	if f.createTable == nil {
		return nil, fmt.Errorf("unexpected call to CreateTable")
	}
	return f.createTable(ctx, params)
}

func (f *fakeDynamoDBAPI) UpdateTable(_ context.Context, params *awsddb.UpdateTableInput, _ ...func(*awsddb.Options)) (*awsddb.UpdateTableOutput, error) {
	f.updateTableCalled = true
	f.updateTableInput = params
	return &awsddb.UpdateTableOutput{}, nil
}

func (f *fakeDynamoDBAPI) UpdateContinuousBackups(ctx context.Context, params *awsddb.UpdateContinuousBackupsInput, _ ...func(*awsddb.Options)) (*awsddb.UpdateContinuousBackupsOutput, error) {
	f.updateBackupsCalled = true
	f.updateBackupsInput = params
	if f.updateBackups != nil {
		return f.updateBackups(ctx, params)
	}
	return &awsddb.UpdateContinuousBackupsOutput{}, nil
}

func (f *fakeDynamoDBAPI) DeleteTable(_ context.Context, _ *awsddb.DeleteTableInput, _ ...func(*awsddb.Options)) (*awsddb.DeleteTableOutput, error) {
	f.deleteCalled = true
	return &awsddb.DeleteTableOutput{}, nil
}

const testDDBTableARN = "arn:aws:dynamodb:us-east-1:123456789012:table/my-table"

func ddbScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return scheme
}

func ddbTableCR(mutate ...func(*awsv1alpha1.DynamoDBTable)) *awsv1alpha1.DynamoDBTable {
	tbl := &awsv1alpha1.DynamoDBTable{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-table",
			Namespace: "default",
		},
		Spec: awsv1alpha1.DynamoDBTableSpec{
			TableName: "my-table",
			AttributeDefinitions: []awsv1alpha1.DynamoDBAttributeDefinition{
				{AttributeName: "pk", AttributeType: "S"},
			},
			KeySchema: []awsv1alpha1.DynamoDBKeySchema{
				{AttributeName: "pk", KeyType: "HASH"},
			},
			BillingMode: "PAY_PER_REQUEST",
		},
	}
	for _, m := range mutate {
		m(tbl)
	}
	return tbl
}

func ddbActiveDescribe(billingMode ddbtypes.BillingMode) *awsddb.DescribeTableOutput {
	return &awsddb.DescribeTableOutput{
		Table: &ddbtypes.TableDescription{
			TableArn:    aws.String(testDDBTableARN),
			TableStatus: ddbtypes.TableStatusActive,
			BillingModeSummary: &ddbtypes.BillingModeSummary{
				BillingMode: billingMode,
			},
		},
	}
}

func TestDynamoDBTableReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-table", Namespace: "default"}}

	t.Run("create happy path persists ARN and Ready=True", func(t *testing.T) {
		ctx := context.Background()
		scheme := ddbScheme(t)
		f := &fakeDynamoDBAPI{
			createTable: func(_ context.Context, params *awsddb.CreateTableInput) (*awsddb.CreateTableOutput, error) {
				if aws.ToString(params.TableName) != "my-table" {
					return nil, fmt.Errorf("unexpected table name %q", aws.ToString(params.TableName))
				}
				return &awsddb.CreateTableOutput{TableDescription: &ddbtypes.TableDescription{
					TableArn:    aws.String(testDDBTableARN),
					TableStatus: ddbtypes.TableStatusCreating,
				}}, nil
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.DynamoDBTable{}).
			WithObjects(ddbTableCR()).Build()
		r := &DynamoDBTableReconciler{Client: c, Scheme: scheme, DynamoDBClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.createCalled {
			t.Fatal("expected CreateTable to be called")
		}
		got := &awsv1alpha1.DynamoDBTable{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ARN != testDDBTableARN {
			t.Errorf("status.arn = %q, want %q", got.Status.ARN, testDDBTableARN)
		}
		cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
		if cond == nil || cond.Status != metav1.ConditionTrue {
			t.Errorf("Ready condition = %+v, want True", cond)
		}
	})

	t.Run("ARN persisted when post-create PITR step fails", func(t *testing.T) {
		ctx := context.Background()
		scheme := ddbScheme(t)
		f := &fakeDynamoDBAPI{
			createTable: func(_ context.Context, _ *awsddb.CreateTableInput) (*awsddb.CreateTableOutput, error) {
				return &awsddb.CreateTableOutput{TableDescription: &ddbtypes.TableDescription{
					TableArn:    aws.String(testDDBTableARN),
					TableStatus: ddbtypes.TableStatusCreating,
				}}, nil
			},
			updateBackups: func(_ context.Context, _ *awsddb.UpdateContinuousBackupsInput) (*awsddb.UpdateContinuousBackupsOutput, error) {
				return nil, fmt.Errorf("pitr boom")
			},
		}
		tbl := ddbTableCR(func(tbl *awsv1alpha1.DynamoDBTable) {
			tbl.Spec.PointInTimeRecovery = true
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.DynamoDBTable{}).
			WithObjects(tbl).Build()
		r := &DynamoDBTableReconciler{Client: c, Scheme: scheme, DynamoDBClient: f}

		if _, err := r.Reconcile(ctx, req); err == nil {
			t.Fatal("expected reconcile error from PITR step")
		}
		got := &awsv1alpha1.DynamoDBTable{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ARN != testDDBTableARN {
			t.Errorf("status.arn = %q, want %q (identifier must survive later-step failure)", got.Status.ARN, testDDBTableARN)
		}
		cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
		if cond == nil || cond.Status != metav1.ConditionFalse {
			t.Errorf("Ready condition = %+v, want False", cond)
		}
	})

	t.Run("steady state calls neither UpdateTable nor UpdateContinuousBackups", func(t *testing.T) {
		ctx := context.Background()
		scheme := ddbScheme(t)
		f := &fakeDynamoDBAPI{
			describeTable: func(_ context.Context, _ *awsddb.DescribeTableInput) (*awsddb.DescribeTableOutput, error) {
				return ddbActiveDescribe(ddbtypes.BillingModePayPerRequest), nil
			},
		}
		tbl := ddbTableCR(func(tbl *awsv1alpha1.DynamoDBTable) {
			tbl.Finalizers = []string{awsv1alpha1.FinalizerName}
			tbl.Generation = 1
			tbl.Status.ARN = testDDBTableARN
			tbl.Status.ObservedGeneration = 1
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.DynamoDBTable{}).
			WithObjects(tbl).Build()
		r := &DynamoDBTableReconciler{Client: c, Scheme: scheme, DynamoDBClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.createCalled {
			t.Error("expected CreateTable NOT to be called")
		}
		if f.updateTableCalled {
			t.Error("expected UpdateTable NOT to be called at steady state")
		}
		if f.updateBackupsCalled {
			t.Error("expected UpdateContinuousBackups NOT to be called at steady state")
		}
	})

	t.Run("billing mode change with generation bump calls UpdateTable", func(t *testing.T) {
		ctx := context.Background()
		scheme := ddbScheme(t)
		f := &fakeDynamoDBAPI{
			describeTable: func(_ context.Context, _ *awsddb.DescribeTableInput) (*awsddb.DescribeTableOutput, error) {
				// AWS reports PROVISIONED while spec wants PAY_PER_REQUEST.
				return ddbActiveDescribe(ddbtypes.BillingModeProvisioned), nil
			},
		}
		tbl := ddbTableCR(func(tbl *awsv1alpha1.DynamoDBTable) {
			tbl.Finalizers = []string{awsv1alpha1.FinalizerName}
			tbl.Generation = 2
			tbl.Spec.BillingMode = "PAY_PER_REQUEST"
			tbl.Status.ARN = testDDBTableARN
			tbl.Status.ObservedGeneration = 1
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.DynamoDBTable{}).
			WithObjects(tbl).Build()
		r := &DynamoDBTableReconciler{Client: c, Scheme: scheme, DynamoDBClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.updateTableCalled {
			t.Fatal("expected UpdateTable to be called for billing mode change")
		}
		if f.updateTableInput.BillingMode != ddbtypes.BillingModePayPerRequest {
			t.Errorf("UpdateTable billing mode = %q, want PAY_PER_REQUEST", f.updateTableInput.BillingMode)
		}
	})

	t.Run("PITR toggle with generation bump calls UpdateContinuousBackups", func(t *testing.T) {
		ctx := context.Background()
		scheme := ddbScheme(t)
		f := &fakeDynamoDBAPI{
			describeTable: func(_ context.Context, _ *awsddb.DescribeTableInput) (*awsddb.DescribeTableOutput, error) {
				return ddbActiveDescribe(ddbtypes.BillingModePayPerRequest), nil
			},
		}
		tbl := ddbTableCR(func(tbl *awsv1alpha1.DynamoDBTable) {
			tbl.Finalizers = []string{awsv1alpha1.FinalizerName}
			tbl.Generation = 2
			tbl.Spec.PointInTimeRecovery = true
			tbl.Status.ARN = testDDBTableARN
			tbl.Status.ObservedGeneration = 1
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.DynamoDBTable{}).
			WithObjects(tbl).Build()
		r := &DynamoDBTableReconciler{Client: c, Scheme: scheme, DynamoDBClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.updateBackupsCalled {
			t.Fatal("expected UpdateContinuousBackups to be called for PITR toggle")
		}
		if !aws.ToBool(f.updateBackupsInput.PointInTimeRecoverySpecification.PointInTimeRecoveryEnabled) {
			t.Error("expected PITR to be enabled in UpdateContinuousBackups input")
		}
		// Billing mode matches; no UpdateTable expected.
		if f.updateTableCalled {
			t.Error("expected UpdateTable NOT to be called when only PITR changed")
		}
	})

	t.Run("delete with finalizer calls DeleteTable", func(t *testing.T) {
		ctx := context.Background()
		scheme := ddbScheme(t)
		f := &fakeDynamoDBAPI{}
		now := metav1.Now()
		tbl := ddbTableCR(func(tbl *awsv1alpha1.DynamoDBTable) {
			tbl.Finalizers = []string{awsv1alpha1.FinalizerName}
			tbl.DeletionTimestamp = &now
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.DynamoDBTable{}).
			WithObjects(tbl).Build()
		r := &DynamoDBTableReconciler{Client: c, Scheme: scheme, DynamoDBClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled {
			t.Error("expected DeleteTable to be called")
		}
		got := &awsv1alpha1.DynamoDBTable{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected object gone after finalizer removal, got err=%v", err)
		}
	})

	t.Run("abandon annotation skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := ddbScheme(t)
		f := &fakeDynamoDBAPI{}
		now := metav1.Now()
		tbl := ddbTableCR(func(tbl *awsv1alpha1.DynamoDBTable) {
			tbl.Finalizers = []string{awsv1alpha1.FinalizerName}
			tbl.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			tbl.DeletionTimestamp = &now
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.DynamoDBTable{}).
			WithObjects(tbl).Build()
		r := &DynamoDBTableReconciler{Client: c, Scheme: scheme, DynamoDBClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("expected DeleteTable NOT to be called for abandoned resource")
		}
		got := &awsv1alpha1.DynamoDBTable{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected object gone after finalizer removal, got err=%v", err)
		}
	})
}
