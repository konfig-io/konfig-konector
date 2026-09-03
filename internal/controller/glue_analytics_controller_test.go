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

// Create + delete + abandon coverage for GlueDatabase, GlueCrawler,
// GlueTrigger, and GlueConnection. GlueJob carries the family's full test
// suite in gluejob_controller_test.go.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsglue "github.com/aws/aws-sdk-go-v2/service/glue"
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

func glueAnalyticsScheme(t *testing.T) *runtime.Scheme {
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

// ---- GlueDatabase ----

type fakeGlueDatabaseAPI struct {
	createCalled bool
	deleteCalled bool
	deletedName  string
}

func (f *fakeGlueDatabaseAPI) GetDatabase(context.Context, *awsglue.GetDatabaseInput, ...func(*awsglue.Options)) (*awsglue.GetDatabaseOutput, error) {
	return nil, glueNotFoundErr()
}
func (f *fakeGlueDatabaseAPI) CreateDatabase(_ context.Context, params *awsglue.CreateDatabaseInput, _ ...func(*awsglue.Options)) (*awsglue.CreateDatabaseOutput, error) {
	f.createCalled = true
	if params.DatabaseInput == nil || aws.ToString(params.DatabaseInput.Name) == "" {
		return nil, fmt.Errorf("missing database input name")
	}
	return &awsglue.CreateDatabaseOutput{}, nil
}
func (f *fakeGlueDatabaseAPI) UpdateDatabase(context.Context, *awsglue.UpdateDatabaseInput, ...func(*awsglue.Options)) (*awsglue.UpdateDatabaseOutput, error) {
	return &awsglue.UpdateDatabaseOutput{}, nil
}
func (f *fakeGlueDatabaseAPI) DeleteDatabase(_ context.Context, params *awsglue.DeleteDatabaseInput, _ ...func(*awsglue.Options)) (*awsglue.DeleteDatabaseOutput, error) {
	f.deleteCalled = true
	f.deletedName = aws.ToString(params.Name)
	return &awsglue.DeleteDatabaseOutput{}, nil
}

func TestGlueDatabaseReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-db", Namespace: "default"}}
	newCR := func(mutate ...func(*awsv1alpha1.GlueDatabase)) *awsv1alpha1.GlueDatabase {
		db := &awsv1alpha1.GlueDatabase{
			ObjectMeta: metav1.ObjectMeta{Name: "my-db", Namespace: "default"},
			Spec: awsv1alpha1.GlueDatabaseSpec{
				Name:        "my_db",
				Description: "test db",
				LocationURI: "s3://bucket/db/",
				Parameters:  map[string]string{"classification": "parquet"},
			},
		}
		for _, m := range mutate {
			m(db)
		}
		return db
	}

	t.Run("create", func(t *testing.T) {
		ctx := context.Background()
		scheme := glueAnalyticsScheme(t)
		f := &fakeGlueDatabaseAPI{}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.GlueDatabase{}).
			WithObjects(newCR()).Build()
		r := &GlueDatabaseReconciler{Client: c, Scheme: scheme, GlueClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.createCalled {
			t.Error("expected CreateDatabase to be called")
		}
		got := &awsv1alpha1.GlueDatabase{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.DatabaseName != "my_db" {
			t.Errorf("status.databaseName = %q, want my_db", got.Status.DatabaseName)
		}
		cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
		if cond == nil || cond.Status != metav1.ConditionTrue {
			t.Errorf("Ready condition = %+v, want True", cond)
		}
	})

	t.Run("delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := glueAnalyticsScheme(t)
		f := &fakeGlueDatabaseAPI{}
		now := metav1.Now()
		db := newCR(func(db *awsv1alpha1.GlueDatabase) {
			db.Finalizers = []string{awsv1alpha1.FinalizerName}
			db.DeletionTimestamp = &now
			db.Status.DatabaseName = "my_db"
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.GlueDatabase{}).
			WithObjects(db).Build()
		r := &GlueDatabaseReconciler{Client: c, Scheme: scheme, GlueClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled || f.deletedName != "my_db" {
			t.Errorf("DeleteDatabase called=%v name=%q, want true/my_db", f.deleteCalled, f.deletedName)
		}
		got := &awsv1alpha1.GlueDatabase{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected CR gone, got err=%v", err)
		}
	})

	t.Run("abandon", func(t *testing.T) {
		ctx := context.Background()
		scheme := glueAnalyticsScheme(t)
		f := &fakeGlueDatabaseAPI{}
		now := metav1.Now()
		db := newCR(func(db *awsv1alpha1.GlueDatabase) {
			db.Finalizers = []string{awsv1alpha1.FinalizerName}
			db.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			db.DeletionTimestamp = &now
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.GlueDatabase{}).
			WithObjects(db).Build()
		r := &GlueDatabaseReconciler{Client: c, Scheme: scheme, GlueClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("DeleteDatabase must not be called when abandoning")
		}
	})
}

// ---- GlueCrawler ----

type fakeGlueCrawlerAPI struct {
	createCalled bool
	deleteCalled bool
	deletedName  string
	createInput  *awsglue.CreateCrawlerInput
}

func (f *fakeGlueCrawlerAPI) GetCrawler(context.Context, *awsglue.GetCrawlerInput, ...func(*awsglue.Options)) (*awsglue.GetCrawlerOutput, error) {
	return nil, glueNotFoundErr()
}
func (f *fakeGlueCrawlerAPI) CreateCrawler(_ context.Context, params *awsglue.CreateCrawlerInput, _ ...func(*awsglue.Options)) (*awsglue.CreateCrawlerOutput, error) {
	f.createCalled = true
	f.createInput = params
	return &awsglue.CreateCrawlerOutput{}, nil
}
func (f *fakeGlueCrawlerAPI) UpdateCrawler(context.Context, *awsglue.UpdateCrawlerInput, ...func(*awsglue.Options)) (*awsglue.UpdateCrawlerOutput, error) {
	return &awsglue.UpdateCrawlerOutput{}, nil
}
func (f *fakeGlueCrawlerAPI) DeleteCrawler(_ context.Context, params *awsglue.DeleteCrawlerInput, _ ...func(*awsglue.Options)) (*awsglue.DeleteCrawlerOutput, error) {
	f.deleteCalled = true
	f.deletedName = aws.ToString(params.Name)
	return &awsglue.DeleteCrawlerOutput{}, nil
}

func TestGlueCrawlerReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-crawler", Namespace: "default"}}
	newCR := func(mutate ...func(*awsv1alpha1.GlueCrawler)) *awsv1alpha1.GlueCrawler {
		cr := &awsv1alpha1.GlueCrawler{
			ObjectMeta: metav1.ObjectMeta{Name: "my-crawler", Namespace: "default"},
			Spec: awsv1alpha1.GlueCrawlerSpec{
				Name:         "my-crawler",
				RoleRef:      awsv1alpha1.RoleRef{ARN: testGlueRoleARN},
				DatabaseName: "my_db",
				Targets: awsv1alpha1.GlueCrawlerTargets{
					S3Targets: []awsv1alpha1.GlueS3Target{{Path: "s3://bucket/data/", Exclusions: []string{"**.tmp"}}},
				},
				Schedule: "cron(15 12 * * ? *)",
			},
		}
		for _, m := range mutate {
			m(cr)
		}
		return cr
	}

	t.Run("create", func(t *testing.T) {
		ctx := context.Background()
		scheme := glueAnalyticsScheme(t)
		f := &fakeGlueCrawlerAPI{}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.GlueCrawler{}).
			WithObjects(newCR()).Build()
		r := &GlueCrawlerReconciler{Client: c, Scheme: scheme, GlueClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.createCalled {
			t.Fatal("expected CreateCrawler to be called")
		}
		if f.createInput.Targets == nil || len(f.createInput.Targets.S3Targets) != 1 {
			t.Errorf("expected one S3 target, got %+v", f.createInput.Targets)
		}
		if aws.ToString(f.createInput.DatabaseName) != "my_db" {
			t.Errorf("database = %q, want my_db", aws.ToString(f.createInput.DatabaseName))
		}
		got := &awsv1alpha1.GlueCrawler{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.CrawlerName != "my-crawler" {
			t.Errorf("status.crawlerName = %q", got.Status.CrawlerName)
		}
	})

	t.Run("delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := glueAnalyticsScheme(t)
		f := &fakeGlueCrawlerAPI{}
		now := metav1.Now()
		cr := newCR(func(cr *awsv1alpha1.GlueCrawler) {
			cr.Finalizers = []string{awsv1alpha1.FinalizerName}
			cr.DeletionTimestamp = &now
			cr.Status.CrawlerName = "my-crawler"
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.GlueCrawler{}).
			WithObjects(cr).Build()
		r := &GlueCrawlerReconciler{Client: c, Scheme: scheme, GlueClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled || f.deletedName != "my-crawler" {
			t.Errorf("DeleteCrawler called=%v name=%q", f.deleteCalled, f.deletedName)
		}
	})

	t.Run("abandon", func(t *testing.T) {
		ctx := context.Background()
		scheme := glueAnalyticsScheme(t)
		f := &fakeGlueCrawlerAPI{}
		now := metav1.Now()
		cr := newCR(func(cr *awsv1alpha1.GlueCrawler) {
			cr.Finalizers = []string{awsv1alpha1.FinalizerName}
			cr.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			cr.DeletionTimestamp = &now
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.GlueCrawler{}).
			WithObjects(cr).Build()
		r := &GlueCrawlerReconciler{Client: c, Scheme: scheme, GlueClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("DeleteCrawler must not be called when abandoning")
		}
	})
}

// ---- GlueTrigger ----

type fakeGlueTriggerAPI struct {
	createCalled bool
	deleteCalled bool
	deletedName  string
	createInput  *awsglue.CreateTriggerInput
}

func (f *fakeGlueTriggerAPI) GetTrigger(context.Context, *awsglue.GetTriggerInput, ...func(*awsglue.Options)) (*awsglue.GetTriggerOutput, error) {
	return nil, glueNotFoundErr()
}
func (f *fakeGlueTriggerAPI) CreateTrigger(_ context.Context, params *awsglue.CreateTriggerInput, _ ...func(*awsglue.Options)) (*awsglue.CreateTriggerOutput, error) {
	f.createCalled = true
	f.createInput = params
	return &awsglue.CreateTriggerOutput{Name: params.Name}, nil
}
func (f *fakeGlueTriggerAPI) UpdateTrigger(context.Context, *awsglue.UpdateTriggerInput, ...func(*awsglue.Options)) (*awsglue.UpdateTriggerOutput, error) {
	return &awsglue.UpdateTriggerOutput{}, nil
}
func (f *fakeGlueTriggerAPI) DeleteTrigger(_ context.Context, params *awsglue.DeleteTriggerInput, _ ...func(*awsglue.Options)) (*awsglue.DeleteTriggerOutput, error) {
	f.deleteCalled = true
	f.deletedName = aws.ToString(params.Name)
	return &awsglue.DeleteTriggerOutput{}, nil
}

func TestGlueTriggerReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-trigger", Namespace: "default"}}
	newCR := func(mutate ...func(*awsv1alpha1.GlueTrigger)) *awsv1alpha1.GlueTrigger {
		tr := &awsv1alpha1.GlueTrigger{
			ObjectMeta: metav1.ObjectMeta{Name: "my-trigger", Namespace: "default"},
			Spec: awsv1alpha1.GlueTriggerSpec{
				Name:     "my-trigger",
				Type:     "SCHEDULED",
				Schedule: "cron(0 6 * * ? *)",
				Actions:  []awsv1alpha1.GlueTriggerAction{{JobName: "my-job"}},
			},
		}
		for _, m := range mutate {
			m(tr)
		}
		return tr
	}

	t.Run("create", func(t *testing.T) {
		ctx := context.Background()
		scheme := glueAnalyticsScheme(t)
		f := &fakeGlueTriggerAPI{}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.GlueTrigger{}).
			WithObjects(newCR()).Build()
		r := &GlueTriggerReconciler{Client: c, Scheme: scheme, GlueClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.createCalled {
			t.Fatal("expected CreateTrigger to be called")
		}
		if len(f.createInput.Actions) != 1 || aws.ToString(f.createInput.Actions[0].JobName) != "my-job" {
			t.Errorf("actions = %+v, want one action for my-job", f.createInput.Actions)
		}
		got := &awsv1alpha1.GlueTrigger{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.TriggerName != "my-trigger" {
			t.Errorf("status.triggerName = %q", got.Status.TriggerName)
		}
	})

	t.Run("delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := glueAnalyticsScheme(t)
		f := &fakeGlueTriggerAPI{}
		now := metav1.Now()
		tr := newCR(func(tr *awsv1alpha1.GlueTrigger) {
			tr.Finalizers = []string{awsv1alpha1.FinalizerName}
			tr.DeletionTimestamp = &now
			tr.Status.TriggerName = "my-trigger"
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.GlueTrigger{}).
			WithObjects(tr).Build()
		r := &GlueTriggerReconciler{Client: c, Scheme: scheme, GlueClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled || f.deletedName != "my-trigger" {
			t.Errorf("DeleteTrigger called=%v name=%q", f.deleteCalled, f.deletedName)
		}
	})

	t.Run("abandon", func(t *testing.T) {
		ctx := context.Background()
		scheme := glueAnalyticsScheme(t)
		f := &fakeGlueTriggerAPI{}
		now := metav1.Now()
		tr := newCR(func(tr *awsv1alpha1.GlueTrigger) {
			tr.Finalizers = []string{awsv1alpha1.FinalizerName}
			tr.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			tr.DeletionTimestamp = &now
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.GlueTrigger{}).
			WithObjects(tr).Build()
		r := &GlueTriggerReconciler{Client: c, Scheme: scheme, GlueClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("DeleteTrigger must not be called when abandoning")
		}
	})
}

// ---- GlueConnection ----

type fakeGlueConnectionAPI struct {
	createCalled bool
	deleteCalled bool
	deletedName  string
	createInput  *awsglue.CreateConnectionInput
}

func (f *fakeGlueConnectionAPI) GetConnection(context.Context, *awsglue.GetConnectionInput, ...func(*awsglue.Options)) (*awsglue.GetConnectionOutput, error) {
	return nil, glueNotFoundErr()
}
func (f *fakeGlueConnectionAPI) CreateConnection(_ context.Context, params *awsglue.CreateConnectionInput, _ ...func(*awsglue.Options)) (*awsglue.CreateConnectionOutput, error) {
	f.createCalled = true
	f.createInput = params
	return &awsglue.CreateConnectionOutput{}, nil
}
func (f *fakeGlueConnectionAPI) UpdateConnection(context.Context, *awsglue.UpdateConnectionInput, ...func(*awsglue.Options)) (*awsglue.UpdateConnectionOutput, error) {
	return &awsglue.UpdateConnectionOutput{}, nil
}
func (f *fakeGlueConnectionAPI) DeleteConnection(_ context.Context, params *awsglue.DeleteConnectionInput, _ ...func(*awsglue.Options)) (*awsglue.DeleteConnectionOutput, error) {
	f.deleteCalled = true
	f.deletedName = aws.ToString(params.ConnectionName)
	return &awsglue.DeleteConnectionOutput{}, nil
}

const testGlueConnPassword = "glue-c0nn-secret"

func TestGlueConnectionReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-conn", Namespace: "default"}}
	passwordSecret := func() *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "conn-creds", Namespace: "default"},
			Data:       map[string][]byte{"password": []byte(testGlueConnPassword)},
		}
	}
	newCR := func(mutate ...func(*awsv1alpha1.GlueConnection)) *awsv1alpha1.GlueConnection {
		conn := &awsv1alpha1.GlueConnection{
			ObjectMeta: metav1.ObjectMeta{Name: "my-conn", Namespace: "default"},
			Spec: awsv1alpha1.GlueConnectionSpec{
				Name:           "my-conn",
				ConnectionType: "JDBC",
				ConnectionProperties: map[string]string{
					"JDBC_CONNECTION_URL": "jdbc:postgresql://db:5432/app",
					"USERNAME":            "app",
				},
				PasswordSecretRef: &awsv1alpha1.SecretRef{Name: "conn-creds", Key: "password"},
			},
		}
		for _, m := range mutate {
			m(conn)
		}
		return conn
	}

	t.Run("create resolves password from secret without leaking to status", func(t *testing.T) {
		ctx := context.Background()
		scheme := glueAnalyticsScheme(t)
		f := &fakeGlueConnectionAPI{}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.GlueConnection{}).
			WithObjects(newCR(), passwordSecret()).Build()
		r := &GlueConnectionReconciler{Client: c, Scheme: scheme, GlueClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.createCalled {
			t.Fatal("expected CreateConnection to be called")
		}
		if f.createInput.ConnectionInput.ConnectionProperties["PASSWORD"] != testGlueConnPassword {
			t.Error("PASSWORD property not resolved from secret")
		}
		got := &awsv1alpha1.GlueConnection{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ConnectionName != "my-conn" {
			t.Errorf("status.connectionName = %q", got.Status.ConnectionName)
		}
		raw, _ := json.Marshal(got.Status)
		if strings.Contains(string(raw), testGlueConnPassword) {
			t.Errorf("connection password leaked into status: %s", raw)
		}
	})

	t.Run("inline PASSWORD property is rejected", func(t *testing.T) {
		ctx := context.Background()
		scheme := glueAnalyticsScheme(t)
		f := &fakeGlueConnectionAPI{}
		conn := newCR(func(conn *awsv1alpha1.GlueConnection) {
			conn.Spec.ConnectionProperties["PASSWORD"] = "inline-pw"
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.GlueConnection{}).
			WithObjects(conn, passwordSecret()).Build()
		r := &GlueConnectionReconciler{Client: c, Scheme: scheme, GlueClient: f}
		if _, err := r.Reconcile(ctx, req); err == nil {
			t.Fatal("expected error for inline PASSWORD property")
		}
		if f.createCalled {
			t.Error("CreateConnection must not be called with inline PASSWORD")
		}
	})

	t.Run("delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := glueAnalyticsScheme(t)
		f := &fakeGlueConnectionAPI{}
		now := metav1.Now()
		conn := newCR(func(conn *awsv1alpha1.GlueConnection) {
			conn.Finalizers = []string{awsv1alpha1.FinalizerName}
			conn.DeletionTimestamp = &now
			conn.Status.ConnectionName = "my-conn"
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.GlueConnection{}).
			WithObjects(conn).Build()
		r := &GlueConnectionReconciler{Client: c, Scheme: scheme, GlueClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled || f.deletedName != "my-conn" {
			t.Errorf("DeleteConnection called=%v name=%q", f.deleteCalled, f.deletedName)
		}
	})

	t.Run("abandon", func(t *testing.T) {
		ctx := context.Background()
		scheme := glueAnalyticsScheme(t)
		f := &fakeGlueConnectionAPI{}
		now := metav1.Now()
		conn := newCR(func(conn *awsv1alpha1.GlueConnection) {
			conn.Finalizers = []string{awsv1alpha1.FinalizerName}
			conn.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			conn.DeletionTimestamp = &now
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.GlueConnection{}).
			WithObjects(conn).Build()
		r := &GlueConnectionReconciler{Client: c, Scheme: scheme, GlueClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("DeleteConnection must not be called when abandoning")
		}
	})
}
