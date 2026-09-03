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
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsroute53 "github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

// fakeHealthCheckAPI implements HealthCheckAWSAPI with function fields.
type fakeHealthCheckAPI struct {
	CreateHealthCheckFn     func(ctx context.Context, params *awsroute53.CreateHealthCheckInput, optFns ...func(*awsroute53.Options)) (*awsroute53.CreateHealthCheckOutput, error)
	GetHealthCheckFn        func(ctx context.Context, params *awsroute53.GetHealthCheckInput, optFns ...func(*awsroute53.Options)) (*awsroute53.GetHealthCheckOutput, error)
	UpdateHealthCheckFn     func(ctx context.Context, params *awsroute53.UpdateHealthCheckInput, optFns ...func(*awsroute53.Options)) (*awsroute53.UpdateHealthCheckOutput, error)
	DeleteHealthCheckFn     func(ctx context.Context, params *awsroute53.DeleteHealthCheckInput, optFns ...func(*awsroute53.Options)) (*awsroute53.DeleteHealthCheckOutput, error)
	ListHealthChecksFn      func(ctx context.Context, params *awsroute53.ListHealthChecksInput, optFns ...func(*awsroute53.Options)) (*awsroute53.ListHealthChecksOutput, error)
	ChangeTagsForResourceFn func(ctx context.Context, params *awsroute53.ChangeTagsForResourceInput, optFns ...func(*awsroute53.Options)) (*awsroute53.ChangeTagsForResourceOutput, error)
}

func (f *fakeHealthCheckAPI) CreateHealthCheck(ctx context.Context, params *awsroute53.CreateHealthCheckInput, optFns ...func(*awsroute53.Options)) (*awsroute53.CreateHealthCheckOutput, error) {
	if f.CreateHealthCheckFn == nil {
		panic("unexpected call to CreateHealthCheck")
	}
	return f.CreateHealthCheckFn(ctx, params, optFns...)
}

func (f *fakeHealthCheckAPI) GetHealthCheck(ctx context.Context, params *awsroute53.GetHealthCheckInput, optFns ...func(*awsroute53.Options)) (*awsroute53.GetHealthCheckOutput, error) {
	if f.GetHealthCheckFn == nil {
		panic("unexpected call to GetHealthCheck")
	}
	return f.GetHealthCheckFn(ctx, params, optFns...)
}

func (f *fakeHealthCheckAPI) UpdateHealthCheck(ctx context.Context, params *awsroute53.UpdateHealthCheckInput, optFns ...func(*awsroute53.Options)) (*awsroute53.UpdateHealthCheckOutput, error) {
	if f.UpdateHealthCheckFn == nil {
		panic("unexpected call to UpdateHealthCheck")
	}
	return f.UpdateHealthCheckFn(ctx, params, optFns...)
}

func (f *fakeHealthCheckAPI) DeleteHealthCheck(ctx context.Context, params *awsroute53.DeleteHealthCheckInput, optFns ...func(*awsroute53.Options)) (*awsroute53.DeleteHealthCheckOutput, error) {
	if f.DeleteHealthCheckFn == nil {
		panic("unexpected call to DeleteHealthCheck")
	}
	return f.DeleteHealthCheckFn(ctx, params, optFns...)
}

func (f *fakeHealthCheckAPI) ListHealthChecks(ctx context.Context, params *awsroute53.ListHealthChecksInput, optFns ...func(*awsroute53.Options)) (*awsroute53.ListHealthChecksOutput, error) {
	if f.ListHealthChecksFn == nil {
		panic("unexpected call to ListHealthChecks")
	}
	return f.ListHealthChecksFn(ctx, params, optFns...)
}

func (f *fakeHealthCheckAPI) ChangeTagsForResource(ctx context.Context, params *awsroute53.ChangeTagsForResourceInput, optFns ...func(*awsroute53.Options)) (*awsroute53.ChangeTagsForResourceOutput, error) {
	if f.ChangeTagsForResourceFn == nil {
		panic("unexpected call to ChangeTagsForResource")
	}
	return f.ChangeTagsForResourceFn(ctx, params, optFns...)
}

var _ HealthCheckAWSAPI = (*fakeHealthCheckAPI)(nil)

func newHCClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&awsv1alpha1.HealthCheck{}).
		Build()
}

func hcScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return scheme
}

func testHealthCheck() *awsv1alpha1.HealthCheck {
	return &awsv1alpha1.HealthCheck{
		ObjectMeta: metav1.ObjectMeta{Name: "hc1", Namespace: "default", UID: "uid-hc", Generation: 1},
		Spec: awsv1alpha1.HealthCheckSpec{
			Type:             "HTTPS",
			FQDN:             "app.example.com",
			Port:             443,
			ResourcePath:     "/healthz",
			FailureThreshold: 3,
		},
	}
}

func TestHealthCheckCreateHappyPath(t *testing.T) {
	hc := testHealthCheck()
	k8s := newHCClient(t, hc)

	createCalls := 0
	api := &fakeHealthCheckAPI{
		CreateHealthCheckFn: func(_ context.Context, params *awsroute53.CreateHealthCheckInput, _ ...func(*awsroute53.Options)) (*awsroute53.CreateHealthCheckOutput, error) {
			createCalls++
			if aws.ToString(params.CallerReference) != healthCheckCallerRef("uid-hc") {
				t.Errorf("CallerReference = %q, want %q", aws.ToString(params.CallerReference), healthCheckCallerRef("uid-hc"))
			}
			if params.HealthCheckConfig.Type != types.HealthCheckTypeHttps {
				t.Errorf("type = %s, want HTTPS", params.HealthCheckConfig.Type)
			}
			return &awsroute53.CreateHealthCheckOutput{
				HealthCheck: &types.HealthCheck{Id: aws.String("hc-abc")},
			}, nil
		},
	}
	r := &HealthCheckReconciler{Client: k8s, Scheme: hcScheme(t), Route53Client: api, CallerReference: "test"}

	key := k8stypes.NamespacedName{Name: "hc1", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if createCalls != 1 {
		t.Errorf("CreateHealthCheck calls = %d, want 1", createCalls)
	}

	got := &awsv1alpha1.HealthCheck{}
	if err := k8s.Get(context.Background(), key, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.HealthCheckID != "hc-abc" {
		t.Errorf("status.healthCheckId = %q, want hc-abc", got.Status.HealthCheckID)
	}
	cond := meta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
	if cond == nil || cond.Status != metav1.ConditionTrue {
		t.Errorf("Ready condition = %+v, want True", cond)
	}
}

func TestHealthCheckIDPersistedWhenTagSyncFails(t *testing.T) {
	hc := testHealthCheck()
	hc.Spec.Tags = map[string]string{"env": "test"}
	k8s := newHCClient(t, hc)

	api := &fakeHealthCheckAPI{
		CreateHealthCheckFn: func(_ context.Context, _ *awsroute53.CreateHealthCheckInput, _ ...func(*awsroute53.Options)) (*awsroute53.CreateHealthCheckOutput, error) {
			return &awsroute53.CreateHealthCheckOutput{
				HealthCheck: &types.HealthCheck{Id: aws.String("hc-def")},
			}, nil
		},
		ChangeTagsForResourceFn: func(_ context.Context, _ *awsroute53.ChangeTagsForResourceInput, _ ...func(*awsroute53.Options)) (*awsroute53.ChangeTagsForResourceOutput, error) {
			return nil, errors.New("tag api unavailable")
		},
	}
	r := &HealthCheckReconciler{Client: k8s, Scheme: hcScheme(t), Route53Client: api, CallerReference: "test"}

	key := k8stypes.NamespacedName{Name: "hc1", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err == nil {
		t.Fatal("expected error from tag sync, got nil")
	}

	got := &awsv1alpha1.HealthCheck{}
	if err := k8s.Get(context.Background(), key, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.HealthCheckID != "hc-def" {
		t.Errorf("status.healthCheckId = %q, want hc-def (must be persisted even though tag sync failed)", got.Status.HealthCheckID)
	}
}

func TestHealthCheckSteadyStateNoCreate(t *testing.T) {
	hc := testHealthCheck()
	hc.Finalizers = []string{awsv1alpha1.FinalizerName}
	hc.Status.HealthCheckID = "hc-abc"
	hc.Status.ObservedGeneration = 1
	k8s := newHCClient(t, hc)

	api := &fakeHealthCheckAPI{
		// CreateHealthCheckFn and UpdateHealthCheckFn intentionally nil:
		// calling either panics the test.
		GetHealthCheckFn: func(_ context.Context, params *awsroute53.GetHealthCheckInput, _ ...func(*awsroute53.Options)) (*awsroute53.GetHealthCheckOutput, error) {
			return &awsroute53.GetHealthCheckOutput{
				HealthCheck: &types.HealthCheck{Id: params.HealthCheckId},
			}, nil
		},
	}
	r := &HealthCheckReconciler{Client: k8s, Scheme: hcScheme(t), Route53Client: api, CallerReference: "test"}

	key := k8stypes.NamespacedName{Name: "hc1", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got := &awsv1alpha1.HealthCheck{}
	if err := k8s.Get(context.Background(), key, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.HealthCheckID != "hc-abc" {
		t.Errorf("status.healthCheckId = %q, want hc-abc", got.Status.HealthCheckID)
	}
}

func TestHealthCheckDeleteWithFinalizer(t *testing.T) {
	hc := testHealthCheck()
	hc.Finalizers = []string{awsv1alpha1.FinalizerName}
	hc.Status.HealthCheckID = "hc-abc"
	k8s := newHCClient(t, hc)
	if err := k8s.Delete(context.Background(), hc); err != nil {
		t.Fatalf("delete: %v", err)
	}

	deleteCalls := 0
	api := &fakeHealthCheckAPI{
		DeleteHealthCheckFn: func(_ context.Context, params *awsroute53.DeleteHealthCheckInput, _ ...func(*awsroute53.Options)) (*awsroute53.DeleteHealthCheckOutput, error) {
			deleteCalls++
			if aws.ToString(params.HealthCheckId) != "hc-abc" {
				t.Errorf("DeleteHealthCheck id = %q, want hc-abc", aws.ToString(params.HealthCheckId))
			}
			return &awsroute53.DeleteHealthCheckOutput{}, nil
		},
	}
	r := &HealthCheckReconciler{Client: k8s, Scheme: hcScheme(t), Route53Client: api, CallerReference: "test"}

	key := k8stypes.NamespacedName{Name: "hc1", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if deleteCalls != 1 {
		t.Errorf("DeleteHealthCheck calls = %d, want 1", deleteCalls)
	}
	got := &awsv1alpha1.HealthCheck{}
	if err := k8s.Get(context.Background(), key, got); err == nil {
		t.Errorf("expected HealthCheck to be gone after finalizer removal, still present with finalizers %v", got.Finalizers)
	}
}

func TestHealthCheckAbandonAnnotation(t *testing.T) {
	hc := testHealthCheck()
	hc.Finalizers = []string{awsv1alpha1.FinalizerName}
	hc.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
	hc.Status.HealthCheckID = "hc-abc"
	k8s := newHCClient(t, hc)
	if err := k8s.Delete(context.Background(), hc); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// All fns nil: any AWS call panics the test.
	api := &fakeHealthCheckAPI{}
	r := &HealthCheckReconciler{Client: k8s, Scheme: hcScheme(t), Route53Client: api, CallerReference: "test"}

	key := k8stypes.NamespacedName{Name: "hc1", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	got := &awsv1alpha1.HealthCheck{}
	if err := k8s.Get(context.Background(), key, got); err == nil {
		t.Errorf("expected HealthCheck to be gone after abandon, still present with finalizers %v", got.Finalizers)
	}
}

func TestHealthCheckDeleteFallbackCallerReferenceMatch(t *testing.T) {
	hc := testHealthCheck()
	hc.Finalizers = []string{awsv1alpha1.FinalizerName}
	// Status.HealthCheckID intentionally empty: forces the fallback lookup.
	k8s := newHCClient(t, hc)
	if err := k8s.Delete(context.Background(), hc); err != nil {
		t.Fatalf("delete: %v", err)
	}

	deleteCalls := 0
	api := &fakeHealthCheckAPI{
		ListHealthChecksFn: func(_ context.Context, _ *awsroute53.ListHealthChecksInput, _ ...func(*awsroute53.Options)) (*awsroute53.ListHealthChecksOutput, error) {
			return &awsroute53.ListHealthChecksOutput{
				HealthChecks: []types.HealthCheck{
					{Id: aws.String("hc-other"), CallerReference: aws.String("someone-else")},
					{Id: aws.String("hc-mine"), CallerReference: aws.String(healthCheckCallerRef("uid-hc"))},
				},
				IsTruncated: false,
			}, nil
		},
		DeleteHealthCheckFn: func(_ context.Context, params *awsroute53.DeleteHealthCheckInput, _ ...func(*awsroute53.Options)) (*awsroute53.DeleteHealthCheckOutput, error) {
			deleteCalls++
			if aws.ToString(params.HealthCheckId) != "hc-mine" {
				t.Errorf("DeleteHealthCheck id = %q, want hc-mine", aws.ToString(params.HealthCheckId))
			}
			return &awsroute53.DeleteHealthCheckOutput{}, nil
		},
	}
	r := &HealthCheckReconciler{Client: k8s, Scheme: hcScheme(t), Route53Client: api, CallerReference: "test"}

	key := k8stypes.NamespacedName{Name: "hc1", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if deleteCalls != 1 {
		t.Errorf("DeleteHealthCheck calls = %d, want 1", deleteCalls)
	}
}

func TestHealthCheckDeleteFallbackNoMatchSkipped(t *testing.T) {
	hc := testHealthCheck()
	hc.Finalizers = []string{awsv1alpha1.FinalizerName}
	// Status.HealthCheckID intentionally empty: forces the fallback lookup.
	k8s := newHCClient(t, hc)
	if err := k8s.Delete(context.Background(), hc); err != nil {
		t.Fatalf("delete: %v", err)
	}

	api := &fakeHealthCheckAPI{
		ListHealthChecksFn: func(_ context.Context, _ *awsroute53.ListHealthChecksInput, _ ...func(*awsroute53.Options)) (*awsroute53.ListHealthChecksOutput, error) {
			return &awsroute53.ListHealthChecksOutput{
				HealthChecks: []types.HealthCheck{
					{Id: aws.String("hc-other"), CallerReference: aws.String("someone-else")},
				},
				IsTruncated: false,
			}, nil
		},
		// DeleteHealthCheckFn intentionally nil: a delete would panic the test.
	}
	r := &HealthCheckReconciler{Client: k8s, Scheme: hcScheme(t), Route53Client: api, CallerReference: "test"}

	key := k8stypes.NamespacedName{Name: "hc1", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	got := &awsv1alpha1.HealthCheck{}
	if err := k8s.Get(context.Background(), key, got); err == nil {
		t.Errorf("expected HealthCheck to be gone (finalizer removed without AWS delete), still present with finalizers %v", got.Finalizers)
	}
}
