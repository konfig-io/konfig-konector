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

// Create + delete + abandon coverage for AthenaWorkGroup, AthenaDataCatalog,
// and AthenaNamedQuery (plus the NamedQuery UpdateNotSupported contract).

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsathena "github.com/aws/aws-sdk-go-v2/service/athena"
	athenatypes "github.com/aws/aws-sdk-go-v2/service/athena/types"
	"github.com/aws/smithy-go"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

func athenaScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return scheme
}

// athenaNotFoundErr mimics Athena's workgroup not-found shape: an
// InvalidRequestException whose message says the resource was not found.
func athenaNotFoundErr() error {
	return &smithy.GenericAPIError{Code: "InvalidRequestException", Message: "WorkGroup was not found"}
}

// ---- AthenaWorkGroup ----

type fakeAthenaWorkGroupAPI struct {
	createCalled bool
	deleteCalled bool
	deletedName  string
	createInput  *awsathena.CreateWorkGroupInput
}

func (f *fakeAthenaWorkGroupAPI) GetWorkGroup(context.Context, *awsathena.GetWorkGroupInput, ...func(*awsathena.Options)) (*awsathena.GetWorkGroupOutput, error) {
	return nil, athenaNotFoundErr()
}
func (f *fakeAthenaWorkGroupAPI) CreateWorkGroup(_ context.Context, params *awsathena.CreateWorkGroupInput, _ ...func(*awsathena.Options)) (*awsathena.CreateWorkGroupOutput, error) {
	f.createCalled = true
	f.createInput = params
	return &awsathena.CreateWorkGroupOutput{}, nil
}
func (f *fakeAthenaWorkGroupAPI) UpdateWorkGroup(context.Context, *awsathena.UpdateWorkGroupInput, ...func(*awsathena.Options)) (*awsathena.UpdateWorkGroupOutput, error) {
	return &awsathena.UpdateWorkGroupOutput{}, nil
}
func (f *fakeAthenaWorkGroupAPI) DeleteWorkGroup(_ context.Context, params *awsathena.DeleteWorkGroupInput, _ ...func(*awsathena.Options)) (*awsathena.DeleteWorkGroupOutput, error) {
	f.deleteCalled = true
	f.deletedName = aws.ToString(params.WorkGroup)
	return &awsathena.DeleteWorkGroupOutput{}, nil
}

func TestAthenaWorkGroupReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-wg", Namespace: "default"}}
	newCR := func(mutate ...func(*awsv1alpha1.AthenaWorkGroup)) *awsv1alpha1.AthenaWorkGroup {
		wg := &awsv1alpha1.AthenaWorkGroup{
			ObjectMeta: metav1.ObjectMeta{Name: "my-wg", Namespace: "default"},
			Spec: awsv1alpha1.AthenaWorkGroupSpec{
				Name:        "my-wg",
				Description: "test workgroup",
				ResultConfiguration: &awsv1alpha1.AthenaResultConfiguration{
					OutputLocation: "s3://results/",
					EncryptionConfiguration: &awsv1alpha1.AthenaEncryptionConfiguration{
						EncryptionOption: "SSE_S3",
					},
				},
				EnforceWorkGroupConfiguration:   aws.Bool(true),
				PublishCloudWatchMetricsEnabled: aws.Bool(true),
				BytesScannedCutoffPerQuery:      aws.Int64(10000000),
			},
		}
		for _, m := range mutate {
			m(wg)
		}
		return wg
	}

	t.Run("create", func(t *testing.T) {
		ctx := context.Background()
		scheme := athenaScheme(t)
		f := &fakeAthenaWorkGroupAPI{}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.AthenaWorkGroup{}).
			WithObjects(newCR()).Build()
		r := &AthenaWorkGroupReconciler{Client: c, Scheme: scheme, AthenaClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.createCalled {
			t.Fatal("expected CreateWorkGroup to be called")
		}
		cfg := f.createInput.Configuration
		if cfg == nil || cfg.ResultConfiguration == nil || aws.ToString(cfg.ResultConfiguration.OutputLocation) != "s3://results/" {
			t.Errorf("configuration missing result configuration: %+v", cfg)
		}
		got := &awsv1alpha1.AthenaWorkGroup{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.WorkGroupName != "my-wg" {
			t.Errorf("status.workGroupName = %q", got.Status.WorkGroupName)
		}
		cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
		if cond == nil || cond.Status != metav1.ConditionTrue {
			t.Errorf("Ready condition = %+v, want True", cond)
		}
	})

	t.Run("delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := athenaScheme(t)
		f := &fakeAthenaWorkGroupAPI{}
		now := metav1.Now()
		wg := newCR(func(wg *awsv1alpha1.AthenaWorkGroup) {
			wg.Finalizers = []string{awsv1alpha1.FinalizerName}
			wg.DeletionTimestamp = &now
			wg.Status.WorkGroupName = "my-wg"
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.AthenaWorkGroup{}).
			WithObjects(wg).Build()
		r := &AthenaWorkGroupReconciler{Client: c, Scheme: scheme, AthenaClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled || f.deletedName != "my-wg" {
			t.Errorf("DeleteWorkGroup called=%v name=%q", f.deleteCalled, f.deletedName)
		}
		got := &awsv1alpha1.AthenaWorkGroup{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected CR gone, got err=%v", err)
		}
	})

	t.Run("abandon", func(t *testing.T) {
		ctx := context.Background()
		scheme := athenaScheme(t)
		f := &fakeAthenaWorkGroupAPI{}
		now := metav1.Now()
		wg := newCR(func(wg *awsv1alpha1.AthenaWorkGroup) {
			wg.Finalizers = []string{awsv1alpha1.FinalizerName}
			wg.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			wg.DeletionTimestamp = &now
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.AthenaWorkGroup{}).
			WithObjects(wg).Build()
		r := &AthenaWorkGroupReconciler{Client: c, Scheme: scheme, AthenaClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("DeleteWorkGroup must not be called when abandoning")
		}
	})
}

// ---- AthenaDataCatalog ----

type fakeAthenaDataCatalogAPI struct {
	createCalled bool
	deleteCalled bool
	deletedName  string
}

func (f *fakeAthenaDataCatalogAPI) GetDataCatalog(context.Context, *awsathena.GetDataCatalogInput, ...func(*awsathena.Options)) (*awsathena.GetDataCatalogOutput, error) {
	return nil, &athenatypes.ResourceNotFoundException{Message: aws.String("not found")}
}
func (f *fakeAthenaDataCatalogAPI) CreateDataCatalog(_ context.Context, params *awsathena.CreateDataCatalogInput, _ ...func(*awsathena.Options)) (*awsathena.CreateDataCatalogOutput, error) {
	f.createCalled = true
	if params.Type != athenatypes.DataCatalogTypeLambda {
		return nil, &smithy.GenericAPIError{Code: "InvalidRequestException", Message: "bad type"}
	}
	return &awsathena.CreateDataCatalogOutput{}, nil
}
func (f *fakeAthenaDataCatalogAPI) UpdateDataCatalog(context.Context, *awsathena.UpdateDataCatalogInput, ...func(*awsathena.Options)) (*awsathena.UpdateDataCatalogOutput, error) {
	return &awsathena.UpdateDataCatalogOutput{}, nil
}
func (f *fakeAthenaDataCatalogAPI) DeleteDataCatalog(_ context.Context, params *awsathena.DeleteDataCatalogInput, _ ...func(*awsathena.Options)) (*awsathena.DeleteDataCatalogOutput, error) {
	f.deleteCalled = true
	f.deletedName = aws.ToString(params.Name)
	return &awsathena.DeleteDataCatalogOutput{}, nil
}

func TestAthenaDataCatalogReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-catalog", Namespace: "default"}}
	newCR := func(mutate ...func(*awsv1alpha1.AthenaDataCatalog)) *awsv1alpha1.AthenaDataCatalog {
		dc := &awsv1alpha1.AthenaDataCatalog{
			ObjectMeta: metav1.ObjectMeta{Name: "my-catalog", Namespace: "default"},
			Spec: awsv1alpha1.AthenaDataCatalogSpec{
				Name:       "my-catalog",
				Type:       "LAMBDA",
				Parameters: map[string]string{"function": "arn:aws:lambda:us-east-1:123456789012:function:athena-fn"},
			},
		}
		for _, m := range mutate {
			m(dc)
		}
		return dc
	}

	t.Run("create", func(t *testing.T) {
		ctx := context.Background()
		scheme := athenaScheme(t)
		f := &fakeAthenaDataCatalogAPI{}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.AthenaDataCatalog{}).
			WithObjects(newCR()).Build()
		r := &AthenaDataCatalogReconciler{Client: c, Scheme: scheme, AthenaClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.createCalled {
			t.Fatal("expected CreateDataCatalog to be called")
		}
		got := &awsv1alpha1.AthenaDataCatalog{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.CatalogName != "my-catalog" {
			t.Errorf("status.catalogName = %q", got.Status.CatalogName)
		}
	})

	t.Run("delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := athenaScheme(t)
		f := &fakeAthenaDataCatalogAPI{}
		now := metav1.Now()
		dc := newCR(func(dc *awsv1alpha1.AthenaDataCatalog) {
			dc.Finalizers = []string{awsv1alpha1.FinalizerName}
			dc.DeletionTimestamp = &now
			dc.Status.CatalogName = "my-catalog"
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.AthenaDataCatalog{}).
			WithObjects(dc).Build()
		r := &AthenaDataCatalogReconciler{Client: c, Scheme: scheme, AthenaClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled || f.deletedName != "my-catalog" {
			t.Errorf("DeleteDataCatalog called=%v name=%q", f.deleteCalled, f.deletedName)
		}
	})

	t.Run("abandon", func(t *testing.T) {
		ctx := context.Background()
		scheme := athenaScheme(t)
		f := &fakeAthenaDataCatalogAPI{}
		now := metav1.Now()
		dc := newCR(func(dc *awsv1alpha1.AthenaDataCatalog) {
			dc.Finalizers = []string{awsv1alpha1.FinalizerName}
			dc.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			dc.DeletionTimestamp = &now
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.AthenaDataCatalog{}).
			WithObjects(dc).Build()
		r := &AthenaDataCatalogReconciler{Client: c, Scheme: scheme, AthenaClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("DeleteDataCatalog must not be called when abandoning")
		}
	})
}

// ---- AthenaNamedQuery ----

type fakeAthenaNamedQueryAPI struct {
	getNamedQuery func(ctx context.Context, params *awsathena.GetNamedQueryInput) (*awsathena.GetNamedQueryOutput, error)

	createCalled bool
	deleteCalled bool
	deletedID    string
}

func (f *fakeAthenaNamedQueryAPI) GetNamedQuery(ctx context.Context, params *awsathena.GetNamedQueryInput, _ ...func(*awsathena.Options)) (*awsathena.GetNamedQueryOutput, error) {
	if f.getNamedQuery == nil {
		return nil, &athenatypes.ResourceNotFoundException{Message: aws.String("not found")}
	}
	return f.getNamedQuery(ctx, params)
}
func (f *fakeAthenaNamedQueryAPI) CreateNamedQuery(_ context.Context, _ *awsathena.CreateNamedQueryInput, _ ...func(*awsathena.Options)) (*awsathena.CreateNamedQueryOutput, error) {
	f.createCalled = true
	return &awsathena.CreateNamedQueryOutput{NamedQueryId: aws.String("nq-123")}, nil
}
func (f *fakeAthenaNamedQueryAPI) DeleteNamedQuery(_ context.Context, params *awsathena.DeleteNamedQueryInput, _ ...func(*awsathena.Options)) (*awsathena.DeleteNamedQueryOutput, error) {
	f.deleteCalled = true
	f.deletedID = aws.ToString(params.NamedQueryId)
	return &awsathena.DeleteNamedQueryOutput{}, nil
}

func TestAthenaNamedQueryReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-query", Namespace: "default"}}
	newCR := func(mutate ...func(*awsv1alpha1.AthenaNamedQuery)) *awsv1alpha1.AthenaNamedQuery {
		nq := &awsv1alpha1.AthenaNamedQuery{
			ObjectMeta: metav1.ObjectMeta{Name: "my-query", Namespace: "default"},
			Spec: awsv1alpha1.AthenaNamedQuerySpec{
				Name:        "my-query",
				Database:    "analytics",
				QueryString: "SELECT 1",
			},
		}
		for _, m := range mutate {
			m(nq)
		}
		return nq
	}

	t.Run("create persists generated ID", func(t *testing.T) {
		ctx := context.Background()
		scheme := athenaScheme(t)
		f := &fakeAthenaNamedQueryAPI{}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.AthenaNamedQuery{}).
			WithObjects(newCR()).Build()
		r := &AthenaNamedQueryReconciler{Client: c, Scheme: scheme, AthenaClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.createCalled {
			t.Fatal("expected CreateNamedQuery to be called")
		}
		got := &awsv1alpha1.AthenaNamedQuery{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.NamedQueryID != "nq-123" {
			t.Errorf("status.namedQueryId = %q, want nq-123", got.Status.NamedQueryID)
		}
	})

	t.Run("spec change reports UpdateNotSupported without recreate", func(t *testing.T) {
		ctx := context.Background()
		scheme := athenaScheme(t)
		f := &fakeAthenaNamedQueryAPI{
			getNamedQuery: func(_ context.Context, _ *awsathena.GetNamedQueryInput) (*awsathena.GetNamedQueryOutput, error) {
				return &awsathena.GetNamedQueryOutput{NamedQuery: &athenatypes.NamedQuery{
					NamedQueryId: aws.String("nq-123"),
					Name:         aws.String("my-query"),
					Database:     aws.String("analytics"),
					QueryString:  aws.String("SELECT 1"),
				}}, nil
			},
		}
		nq := newCR(func(nq *awsv1alpha1.AthenaNamedQuery) {
			nq.Finalizers = []string{awsv1alpha1.FinalizerName}
			nq.Generation = 2
			nq.Spec.QueryString = "SELECT 2" // changed after creation
			nq.Status.NamedQueryID = "nq-123"
			nq.Status.ObservedGeneration = 1
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.AthenaNamedQuery{}).
			WithObjects(nq).Build()
		r := &AthenaNamedQueryReconciler{Client: c, Scheme: scheme, AthenaClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.createCalled {
			t.Error("CreateNamedQuery must not be called for an unsupported update")
		}
		if f.deleteCalled {
			t.Error("DeleteNamedQuery must not be called for an unsupported update (no delete+recreate)")
		}
		got := &awsv1alpha1.AthenaNamedQuery{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ObservedGeneration != 1 {
			t.Errorf("observedGeneration = %d, want 1 (must not advance)", got.Status.ObservedGeneration)
		}
		cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
		if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != awsv1alpha1.ReasonUpdateNotSupported {
			t.Errorf("Ready condition = %+v, want False/UpdateNotSupported", cond)
		}
	})

	t.Run("delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := athenaScheme(t)
		f := &fakeAthenaNamedQueryAPI{}
		now := metav1.Now()
		nq := newCR(func(nq *awsv1alpha1.AthenaNamedQuery) {
			nq.Finalizers = []string{awsv1alpha1.FinalizerName}
			nq.DeletionTimestamp = &now
			nq.Status.NamedQueryID = "nq-123"
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.AthenaNamedQuery{}).
			WithObjects(nq).Build()
		r := &AthenaNamedQueryReconciler{Client: c, Scheme: scheme, AthenaClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled || f.deletedID != "nq-123" {
			t.Errorf("DeleteNamedQuery called=%v id=%q", f.deleteCalled, f.deletedID)
		}
	})

	t.Run("abandon", func(t *testing.T) {
		ctx := context.Background()
		scheme := athenaScheme(t)
		f := &fakeAthenaNamedQueryAPI{}
		now := metav1.Now()
		nq := newCR(func(nq *awsv1alpha1.AthenaNamedQuery) {
			nq.Finalizers = []string{awsv1alpha1.FinalizerName}
			nq.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			nq.DeletionTimestamp = &now
			nq.Status.NamedQueryID = "nq-123"
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.AthenaNamedQuery{}).
			WithObjects(nq).Build()
		r := &AthenaNamedQueryReconciler{Client: c, Scheme: scheme, AthenaClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("DeleteNamedQuery must not be called when abandoning")
		}
	})
}
