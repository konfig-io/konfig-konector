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
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsrds "github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

type fakeDBInstanceAPI struct {
	describe func(ctx context.Context, params *awsrds.DescribeDBInstancesInput) (*awsrds.DescribeDBInstancesOutput, error)
	create   func(ctx context.Context, params *awsrds.CreateDBInstanceInput) (*awsrds.CreateDBInstanceOutput, error)
	modify   func(ctx context.Context, params *awsrds.ModifyDBInstanceInput) (*awsrds.ModifyDBInstanceOutput, error)
	delete   func(ctx context.Context, params *awsrds.DeleteDBInstanceInput) (*awsrds.DeleteDBInstanceOutput, error)

	createCalled bool
	modifyCalled bool
	deleteCalled bool
	createInput  *awsrds.CreateDBInstanceInput
	deleteInput  *awsrds.DeleteDBInstanceInput
}

func (f *fakeDBInstanceAPI) DescribeDBInstances(ctx context.Context, params *awsrds.DescribeDBInstancesInput, _ ...func(*awsrds.Options)) (*awsrds.DescribeDBInstancesOutput, error) {
	if f.describe == nil {
		return nil, fmt.Errorf("unexpected call to DescribeDBInstances")
	}
	return f.describe(ctx, params)
}

func (f *fakeDBInstanceAPI) CreateDBInstance(ctx context.Context, params *awsrds.CreateDBInstanceInput, _ ...func(*awsrds.Options)) (*awsrds.CreateDBInstanceOutput, error) {
	f.createCalled = true
	f.createInput = params
	if f.create == nil {
		return nil, fmt.Errorf("unexpected call to CreateDBInstance")
	}
	return f.create(ctx, params)
}

func (f *fakeDBInstanceAPI) ModifyDBInstance(ctx context.Context, params *awsrds.ModifyDBInstanceInput, _ ...func(*awsrds.Options)) (*awsrds.ModifyDBInstanceOutput, error) {
	f.modifyCalled = true
	if f.modify == nil {
		return nil, fmt.Errorf("unexpected call to ModifyDBInstance")
	}
	return f.modify(ctx, params)
}

func (f *fakeDBInstanceAPI) DeleteDBInstance(ctx context.Context, params *awsrds.DeleteDBInstanceInput, _ ...func(*awsrds.Options)) (*awsrds.DeleteDBInstanceOutput, error) {
	f.deleteCalled = true
	f.deleteInput = params
	if f.delete == nil {
		return nil, fmt.Errorf("unexpected call to DeleteDBInstance")
	}
	return f.delete(ctx, params)
}

const (
	testDBInstanceARN = "arn:aws:rds:us-east-1:123456789012:db:my-db"
	testDBPassword    = "sup3r-secret-pw"
)

func dbInstanceScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add aws scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}
	return scheme
}

func dbPasswordSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "db-creds", Namespace: "default"},
		Data:       map[string][]byte{"password": []byte(testDBPassword)},
	}
}

func dbInstanceCR(mutate ...func(*awsv1alpha1.DBInstance)) *awsv1alpha1.DBInstance {
	db := &awsv1alpha1.DBInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-db",
			Namespace: "default",
		},
		Spec: awsv1alpha1.DBInstanceSpec{
			DBInstanceIdentifier:  "my-db",
			DBInstanceClass:       "db.t3.micro",
			Engine:                "postgres",
			MasterUsername:        "admin",
			MasterUserPasswordRef: awsv1alpha1.SecretRef{Name: "db-creds", Key: "password"},
			AllocatedStorage:      20,
			SkipFinalSnapshot:     true,
		},
	}
	for _, m := range mutate {
		m(db)
	}
	return db
}

func availableDescribeOutput() *awsrds.DescribeDBInstancesOutput {
	return &awsrds.DescribeDBInstancesOutput{
		DBInstances: []rdstypes.DBInstance{{
			DBInstanceArn:    aws.String(testDBInstanceARN),
			DBInstanceStatus: aws.String("available"),
			Endpoint: &rdstypes.Endpoint{
				Address: aws.String("my-db.abc.us-east-1.rds.amazonaws.com"),
				Port:    aws.Int32(5432),
			},
		}},
	}
}

// assertNoPasswordLeak fails if the master password appears anywhere in the
// CR's status (including conditions).
func assertNoPasswordLeak(t *testing.T, db *awsv1alpha1.DBInstance) {
	t.Helper()
	raw, err := json.Marshal(db.Status)
	if err != nil {
		t.Fatalf("marshal status: %v", err)
	}
	if strings.Contains(string(raw), testDBPassword) {
		t.Errorf("master password leaked into status: %s", raw)
	}
}

func TestDBInstanceReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-db", Namespace: "default"}}

	t.Run("create happy path persists ARN then becomes Ready", func(t *testing.T) {
		ctx := context.Background()
		scheme := dbInstanceScheme(t)
		f := &fakeDBInstanceAPI{
			create: func(_ context.Context, params *awsrds.CreateDBInstanceInput) (*awsrds.CreateDBInstanceOutput, error) {
				if aws.ToString(params.MasterUserPassword) != testDBPassword {
					return nil, fmt.Errorf("expected master password from secret, got %q", aws.ToString(params.MasterUserPassword))
				}
				return &awsrds.CreateDBInstanceOutput{DBInstance: &rdstypes.DBInstance{
					DBInstanceArn:    aws.String(testDBInstanceARN),
					DBInstanceStatus: aws.String("creating"),
				}}, nil
			},
			describe: func(_ context.Context, _ *awsrds.DescribeDBInstancesInput) (*awsrds.DescribeDBInstancesOutput, error) {
				return availableDescribeOutput(), nil
			},
			modify: func(_ context.Context, _ *awsrds.ModifyDBInstanceInput) (*awsrds.ModifyDBInstanceOutput, error) {
				return &awsrds.ModifyDBInstanceOutput{}, nil
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.DBInstance{}).
			WithObjects(dbInstanceCR(), dbPasswordSecret()).Build()
		r := &DBInstanceReconciler{Client: c, Scheme: scheme, RDSClient: f}

		// First reconcile: adds finalizer, creates the instance, persists ARN.
		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile 1: %v", err)
		}
		if !f.createCalled {
			t.Fatal("expected CreateDBInstance to be called")
		}
		if res.RequeueAfter == 0 {
			t.Error("expected polling requeue after create")
		}
		got := &awsv1alpha1.DBInstance{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.DBInstanceARN != testDBInstanceARN {
			t.Errorf("status.dbInstanceArn = %q, want %q", got.Status.DBInstanceARN, testDBInstanceARN)
		}
		assertNoPasswordLeak(t, got)

		// Second reconcile: instance is available; controller syncs and reports Ready.
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile 2: %v", err)
		}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
		if cond == nil || cond.Status != metav1.ConditionTrue {
			t.Errorf("Ready condition = %+v, want True", cond)
		}
		if got.Status.Endpoint == "" || got.Status.Port != 5432 {
			t.Errorf("endpoint/port not synced: %q/%d", got.Status.Endpoint, got.Status.Port)
		}
		assertNoPasswordLeak(t, got)
	})

	t.Run("missing secret key errors and sets Ready=False without leaking password", func(t *testing.T) {
		ctx := context.Background()
		scheme := dbInstanceScheme(t)
		f := &fakeDBInstanceAPI{}
		secret := dbPasswordSecret()
		secret.Data = map[string][]byte{"wrong-key": []byte(testDBPassword)}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.DBInstance{}).
			WithObjects(dbInstanceCR(), secret).Build()
		r := &DBInstanceReconciler{Client: c, Scheme: scheme, RDSClient: f}

		_, err := r.Reconcile(ctx, req)
		if err == nil {
			t.Fatal("expected error for missing secret key")
		}
		if f.createCalled {
			t.Error("expected CreateDBInstance NOT to be called")
		}
		got := &awsv1alpha1.DBInstance{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
		if cond == nil || cond.Status != metav1.ConditionFalse {
			t.Errorf("Ready condition = %+v, want False", cond)
		}
		assertNoPasswordLeak(t, got)
	})

	t.Run("steady state does not create or modify", func(t *testing.T) {
		ctx := context.Background()
		scheme := dbInstanceScheme(t)
		f := &fakeDBInstanceAPI{
			describe: func(_ context.Context, _ *awsrds.DescribeDBInstancesInput) (*awsrds.DescribeDBInstancesOutput, error) {
				return availableDescribeOutput(), nil
			},
		}
		db := dbInstanceCR(func(db *awsv1alpha1.DBInstance) {
			db.Finalizers = []string{awsv1alpha1.FinalizerName}
			db.Generation = 1
			db.Status.DBInstanceARN = testDBInstanceARN
			db.Status.ObservedGeneration = 1
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.DBInstance{}).
			WithObjects(db, dbPasswordSecret()).Build()
		r := &DBInstanceReconciler{Client: c, Scheme: scheme, RDSClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.createCalled {
			t.Error("expected CreateDBInstance NOT to be called")
		}
		if f.modifyCalled {
			t.Error("expected ModifyDBInstance NOT to be called")
		}
		got := &awsv1alpha1.DBInstance{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
		if cond == nil || cond.Status != metav1.ConditionTrue {
			t.Errorf("Ready condition = %+v, want True", cond)
		}
	})

	t.Run("delete with skipFinalSnapshot=true skips final snapshot", func(t *testing.T) {
		ctx := context.Background()
		scheme := dbInstanceScheme(t)
		f := &fakeDBInstanceAPI{
			delete: func(_ context.Context, _ *awsrds.DeleteDBInstanceInput) (*awsrds.DeleteDBInstanceOutput, error) {
				return &awsrds.DeleteDBInstanceOutput{}, nil
			},
		}
		now := metav1.Now()
		db := dbInstanceCR(func(db *awsv1alpha1.DBInstance) {
			db.Finalizers = []string{awsv1alpha1.FinalizerName}
			db.DeletionTimestamp = &now
			db.Spec.SkipFinalSnapshot = true
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.DBInstance{}).
			WithObjects(db).Build()
		r := &DBInstanceReconciler{Client: c, Scheme: scheme, RDSClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled {
			t.Fatal("expected DeleteDBInstance to be called")
		}
		if !aws.ToBool(f.deleteInput.SkipFinalSnapshot) {
			t.Error("expected SkipFinalSnapshot=true")
		}
		if f.deleteInput.FinalDBSnapshotIdentifier != nil {
			t.Errorf("expected no FinalDBSnapshotIdentifier, got %q", aws.ToString(f.deleteInput.FinalDBSnapshotIdentifier))
		}
		got := &awsv1alpha1.DBInstance{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected object gone after finalizer removal, got err=%v", err)
		}
	})

	t.Run("delete with skipFinalSnapshot=false requests final snapshot", func(t *testing.T) {
		ctx := context.Background()
		scheme := dbInstanceScheme(t)
		f := &fakeDBInstanceAPI{
			delete: func(_ context.Context, _ *awsrds.DeleteDBInstanceInput) (*awsrds.DeleteDBInstanceOutput, error) {
				return &awsrds.DeleteDBInstanceOutput{}, nil
			},
		}
		now := metav1.Now()
		db := dbInstanceCR(func(db *awsv1alpha1.DBInstance) {
			db.Finalizers = []string{awsv1alpha1.FinalizerName}
			db.DeletionTimestamp = &now
			db.Spec.SkipFinalSnapshot = false
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.DBInstance{}).
			WithObjects(db).Build()
		r := &DBInstanceReconciler{Client: c, Scheme: scheme, RDSClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled {
			t.Fatal("expected DeleteDBInstance to be called")
		}
		if aws.ToBool(f.deleteInput.SkipFinalSnapshot) {
			t.Error("expected SkipFinalSnapshot=false")
		}
		if aws.ToString(f.deleteInput.FinalDBSnapshotIdentifier) != "my-db-final" {
			t.Errorf("FinalDBSnapshotIdentifier = %q, want %q", aws.ToString(f.deleteInput.FinalDBSnapshotIdentifier), "my-db-final")
		}
	})

	t.Run("abandon annotation skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := dbInstanceScheme(t)
		f := &fakeDBInstanceAPI{}
		now := metav1.Now()
		db := dbInstanceCR(func(db *awsv1alpha1.DBInstance) {
			db.Finalizers = []string{awsv1alpha1.FinalizerName}
			db.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			db.DeletionTimestamp = &now
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.DBInstance{}).
			WithObjects(db).Build()
		r := &DBInstanceReconciler{Client: c, Scheme: scheme, RDSClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("expected DeleteDBInstance NOT to be called for abandoned resource")
		}
		got := &awsv1alpha1.DBInstance{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected object gone after finalizer removal, got err=%v", err)
		}
	})
}
