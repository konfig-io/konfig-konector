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

// fakeHostedZoneAPI implements HostedZoneAWSAPI with function fields.
type fakeHostedZoneAPI struct {
	CreateHostedZoneFn         func(ctx context.Context, params *awsroute53.CreateHostedZoneInput, optFns ...func(*awsroute53.Options)) (*awsroute53.CreateHostedZoneOutput, error)
	GetHostedZoneFn            func(ctx context.Context, params *awsroute53.GetHostedZoneInput, optFns ...func(*awsroute53.Options)) (*awsroute53.GetHostedZoneOutput, error)
	ListHostedZonesByNameFn    func(ctx context.Context, params *awsroute53.ListHostedZonesByNameInput, optFns ...func(*awsroute53.Options)) (*awsroute53.ListHostedZonesByNameOutput, error)
	DeleteHostedZoneFn         func(ctx context.Context, params *awsroute53.DeleteHostedZoneInput, optFns ...func(*awsroute53.Options)) (*awsroute53.DeleteHostedZoneOutput, error)
	ListResourceRecordSetsFn   func(ctx context.Context, params *awsroute53.ListResourceRecordSetsInput, optFns ...func(*awsroute53.Options)) (*awsroute53.ListResourceRecordSetsOutput, error)
	ChangeResourceRecordSetsFn func(ctx context.Context, params *awsroute53.ChangeResourceRecordSetsInput, optFns ...func(*awsroute53.Options)) (*awsroute53.ChangeResourceRecordSetsOutput, error)
	ListTagsForResourceFn      func(ctx context.Context, params *awsroute53.ListTagsForResourceInput, optFns ...func(*awsroute53.Options)) (*awsroute53.ListTagsForResourceOutput, error)
	ChangeTagsForResourceFn    func(ctx context.Context, params *awsroute53.ChangeTagsForResourceInput, optFns ...func(*awsroute53.Options)) (*awsroute53.ChangeTagsForResourceOutput, error)
}

func (f *fakeHostedZoneAPI) CreateHostedZone(ctx context.Context, params *awsroute53.CreateHostedZoneInput, optFns ...func(*awsroute53.Options)) (*awsroute53.CreateHostedZoneOutput, error) {
	if f.CreateHostedZoneFn == nil {
		panic("unexpected call to CreateHostedZone")
	}
	return f.CreateHostedZoneFn(ctx, params, optFns...)
}

func (f *fakeHostedZoneAPI) GetHostedZone(ctx context.Context, params *awsroute53.GetHostedZoneInput, optFns ...func(*awsroute53.Options)) (*awsroute53.GetHostedZoneOutput, error) {
	if f.GetHostedZoneFn == nil {
		panic("unexpected call to GetHostedZone")
	}
	return f.GetHostedZoneFn(ctx, params, optFns...)
}

func (f *fakeHostedZoneAPI) ListHostedZonesByName(ctx context.Context, params *awsroute53.ListHostedZonesByNameInput, optFns ...func(*awsroute53.Options)) (*awsroute53.ListHostedZonesByNameOutput, error) {
	if f.ListHostedZonesByNameFn == nil {
		panic("unexpected call to ListHostedZonesByName")
	}
	return f.ListHostedZonesByNameFn(ctx, params, optFns...)
}

func (f *fakeHostedZoneAPI) DeleteHostedZone(ctx context.Context, params *awsroute53.DeleteHostedZoneInput, optFns ...func(*awsroute53.Options)) (*awsroute53.DeleteHostedZoneOutput, error) {
	if f.DeleteHostedZoneFn == nil {
		panic("unexpected call to DeleteHostedZone")
	}
	return f.DeleteHostedZoneFn(ctx, params, optFns...)
}

func (f *fakeHostedZoneAPI) ListResourceRecordSets(ctx context.Context, params *awsroute53.ListResourceRecordSetsInput, optFns ...func(*awsroute53.Options)) (*awsroute53.ListResourceRecordSetsOutput, error) {
	if f.ListResourceRecordSetsFn == nil {
		panic("unexpected call to ListResourceRecordSets")
	}
	return f.ListResourceRecordSetsFn(ctx, params, optFns...)
}

func (f *fakeHostedZoneAPI) ChangeResourceRecordSets(ctx context.Context, params *awsroute53.ChangeResourceRecordSetsInput, optFns ...func(*awsroute53.Options)) (*awsroute53.ChangeResourceRecordSetsOutput, error) {
	if f.ChangeResourceRecordSetsFn == nil {
		panic("unexpected call to ChangeResourceRecordSets")
	}
	return f.ChangeResourceRecordSetsFn(ctx, params, optFns...)
}

func (f *fakeHostedZoneAPI) ListTagsForResource(ctx context.Context, params *awsroute53.ListTagsForResourceInput, optFns ...func(*awsroute53.Options)) (*awsroute53.ListTagsForResourceOutput, error) {
	if f.ListTagsForResourceFn == nil {
		panic("unexpected call to ListTagsForResource")
	}
	return f.ListTagsForResourceFn(ctx, params, optFns...)
}

func (f *fakeHostedZoneAPI) ChangeTagsForResource(ctx context.Context, params *awsroute53.ChangeTagsForResourceInput, optFns ...func(*awsroute53.Options)) (*awsroute53.ChangeTagsForResourceOutput, error) {
	if f.ChangeTagsForResourceFn == nil {
		panic("unexpected call to ChangeTagsForResource")
	}
	return f.ChangeTagsForResourceFn(ctx, params, optFns...)
}

var _ HostedZoneAWSAPI = (*fakeHostedZoneAPI)(nil)

func newHZScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return scheme
}

func newHZClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	return fake.NewClientBuilder().
		WithScheme(newHZScheme(t)).
		WithObjects(objs...).
		WithStatusSubresource(&awsv1alpha1.HostedZone{}).
		Build()
}

// emptyHZTagsFn is a ListTagsForResource stub returning no existing tags.
func emptyHZTagsFn(_ context.Context, params *awsroute53.ListTagsForResourceInput, _ ...func(*awsroute53.Options)) (*awsroute53.ListTagsForResourceOutput, error) {
	return &awsroute53.ListTagsForResourceOutput{
		ResourceTagSet: &types.ResourceTagSet{
			ResourceId:   params.ResourceId,
			ResourceType: params.ResourceType,
		},
	}, nil
}

func TestHostedZoneCreateHappyPath(t *testing.T) {
	hz := &awsv1alpha1.HostedZone{
		ObjectMeta: metav1.ObjectMeta{Name: "zone1", Namespace: "default", UID: "uid-1", Generation: 1},
		Spec:       awsv1alpha1.HostedZoneSpec{Name: "example.com."},
	}
	k8s := newHZClient(t, hz)

	createCalls := 0
	api := &fakeHostedZoneAPI{
		CreateHostedZoneFn: func(_ context.Context, params *awsroute53.CreateHostedZoneInput, _ ...func(*awsroute53.Options)) (*awsroute53.CreateHostedZoneOutput, error) {
			createCalls++
			if aws.ToString(params.Name) != "example.com." {
				t.Errorf("CreateHostedZone name = %q, want %q", aws.ToString(params.Name), "example.com.")
			}
			if aws.ToString(params.CallerReference) != hostedZoneCallerRef("uid-1") {
				t.Errorf("CallerReference = %q, want %q", aws.ToString(params.CallerReference), hostedZoneCallerRef("uid-1"))
			}
			return &awsroute53.CreateHostedZoneOutput{
				HostedZone:    &types.HostedZone{Id: aws.String("/hostedzone/Z123")},
				DelegationSet: &types.DelegationSet{NameServers: []string{"ns-1.awsdns.com"}},
			}, nil
		},
		ListTagsForResourceFn: emptyHZTagsFn,
	}
	r := &HostedZoneReconciler{Client: k8s, Scheme: newHZScheme(t), Route53Client: api, CallerReference: "test"}

	key := k8stypes.NamespacedName{Name: "zone1", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if createCalls != 1 {
		t.Errorf("CreateHostedZone calls = %d, want 1", createCalls)
	}

	got := &awsv1alpha1.HostedZone{}
	if err := k8s.Get(context.Background(), key, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.HostedZoneID != "Z123" {
		t.Errorf("status.hostedZoneId = %q, want Z123", got.Status.HostedZoneID)
	}
	if len(got.Status.NameServers) != 1 || got.Status.NameServers[0] != "ns-1.awsdns.com" {
		t.Errorf("status.nameServers = %v", got.Status.NameServers)
	}
	cond := meta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
	if cond == nil || cond.Status != metav1.ConditionTrue {
		t.Errorf("Ready condition = %+v, want True", cond)
	}
}

func TestHostedZoneIDPersistedWhenTagSyncFails(t *testing.T) {
	hz := &awsv1alpha1.HostedZone{
		ObjectMeta: metav1.ObjectMeta{Name: "zone1", Namespace: "default", UID: "uid-1", Generation: 1},
		Spec:       awsv1alpha1.HostedZoneSpec{Name: "example.com.", Tags: map[string]string{"env": "test"}},
	}
	k8s := newHZClient(t, hz)

	api := &fakeHostedZoneAPI{
		CreateHostedZoneFn: func(_ context.Context, _ *awsroute53.CreateHostedZoneInput, _ ...func(*awsroute53.Options)) (*awsroute53.CreateHostedZoneOutput, error) {
			return &awsroute53.CreateHostedZoneOutput{
				HostedZone:    &types.HostedZone{Id: aws.String("/hostedzone/Z456")},
				DelegationSet: &types.DelegationSet{NameServers: []string{"ns-1.awsdns.com"}},
			}, nil
		},
		ListTagsForResourceFn: func(_ context.Context, _ *awsroute53.ListTagsForResourceInput, _ ...func(*awsroute53.Options)) (*awsroute53.ListTagsForResourceOutput, error) {
			return nil, errors.New("tag api unavailable")
		},
	}
	r := &HostedZoneReconciler{Client: k8s, Scheme: newHZScheme(t), Route53Client: api, CallerReference: "test"}

	key := k8stypes.NamespacedName{Name: "zone1", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err == nil {
		t.Fatal("expected error from tag sync, got nil")
	}

	got := &awsv1alpha1.HostedZone{}
	if err := k8s.Get(context.Background(), key, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.HostedZoneID != "Z456" {
		t.Errorf("status.hostedZoneId = %q, want Z456 (must be persisted even though tag sync failed)", got.Status.HostedZoneID)
	}
}

func TestHostedZoneSteadyStateNoCreate(t *testing.T) {
	hz := &awsv1alpha1.HostedZone{
		ObjectMeta: metav1.ObjectMeta{
			Name: "zone1", Namespace: "default", UID: "uid-1", Generation: 1,
			Finalizers: []string{awsv1alpha1.FinalizerName},
		},
		Spec: awsv1alpha1.HostedZoneSpec{Name: "example.com."},
		Status: awsv1alpha1.HostedZoneStatus{
			HostedZoneID: "Z123",
			NameServers:  []string{"ns-1.awsdns.com"},
		},
	}
	k8s := newHZClient(t, hz)

	api := &fakeHostedZoneAPI{
		// CreateHostedZoneFn intentionally nil: a create would panic the test.
		GetHostedZoneFn: func(_ context.Context, params *awsroute53.GetHostedZoneInput, _ ...func(*awsroute53.Options)) (*awsroute53.GetHostedZoneOutput, error) {
			return &awsroute53.GetHostedZoneOutput{
				HostedZone: &types.HostedZone{Id: params.Id, Name: aws.String("example.com.")},
			}, nil
		},
		ListTagsForResourceFn: emptyHZTagsFn,
	}
	r := &HostedZoneReconciler{Client: k8s, Scheme: newHZScheme(t), Route53Client: api, CallerReference: "test"}

	key := k8stypes.NamespacedName{Name: "zone1", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got := &awsv1alpha1.HostedZone{}
	if err := k8s.Get(context.Background(), key, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.HostedZoneID != "Z123" {
		t.Errorf("status.hostedZoneId = %q, want Z123", got.Status.HostedZoneID)
	}
}

func TestHostedZoneDeleteWithFinalizer(t *testing.T) {
	hz := &awsv1alpha1.HostedZone{
		ObjectMeta: metav1.ObjectMeta{
			Name: "zone1", Namespace: "default", UID: "uid-1",
			Finalizers: []string{awsv1alpha1.FinalizerName},
		},
		Spec:   awsv1alpha1.HostedZoneSpec{Name: "example.com."},
		Status: awsv1alpha1.HostedZoneStatus{HostedZoneID: "Z123"},
	}
	k8s := newHZClient(t, hz)
	if err := k8s.Delete(context.Background(), hz); err != nil {
		t.Fatalf("delete: %v", err)
	}

	deleteCalls := 0
	api := &fakeHostedZoneAPI{
		ListResourceRecordSetsFn: func(_ context.Context, _ *awsroute53.ListResourceRecordSetsInput, _ ...func(*awsroute53.Options)) (*awsroute53.ListResourceRecordSetsOutput, error) {
			return &awsroute53.ListResourceRecordSetsOutput{IsTruncated: false}, nil
		},
		DeleteHostedZoneFn: func(_ context.Context, params *awsroute53.DeleteHostedZoneInput, _ ...func(*awsroute53.Options)) (*awsroute53.DeleteHostedZoneOutput, error) {
			deleteCalls++
			if aws.ToString(params.Id) != "Z123" {
				t.Errorf("DeleteHostedZone id = %q, want Z123", aws.ToString(params.Id))
			}
			return &awsroute53.DeleteHostedZoneOutput{}, nil
		},
	}
	r := &HostedZoneReconciler{Client: k8s, Scheme: newHZScheme(t), Route53Client: api, CallerReference: "test"}

	key := k8stypes.NamespacedName{Name: "zone1", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if deleteCalls != 1 {
		t.Errorf("DeleteHostedZone calls = %d, want 1", deleteCalls)
	}
	got := &awsv1alpha1.HostedZone{}
	if err := k8s.Get(context.Background(), key, got); err == nil {
		t.Errorf("expected HostedZone to be gone after finalizer removal, still present with finalizers %v", got.Finalizers)
	}
}

func TestHostedZoneAbandonAnnotation(t *testing.T) {
	hz := &awsv1alpha1.HostedZone{
		ObjectMeta: metav1.ObjectMeta{
			Name: "zone1", Namespace: "default", UID: "uid-1",
			Finalizers:  []string{awsv1alpha1.FinalizerName},
			Annotations: map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon},
		},
		Spec:   awsv1alpha1.HostedZoneSpec{Name: "example.com."},
		Status: awsv1alpha1.HostedZoneStatus{HostedZoneID: "Z123"},
	}
	k8s := newHZClient(t, hz)
	if err := k8s.Delete(context.Background(), hz); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// All fns nil: any AWS call panics the test.
	api := &fakeHostedZoneAPI{}
	r := &HostedZoneReconciler{Client: k8s, Scheme: newHZScheme(t), Route53Client: api, CallerReference: "test"}

	key := k8stypes.NamespacedName{Name: "zone1", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	got := &awsv1alpha1.HostedZone{}
	if err := k8s.Get(context.Background(), key, got); err == nil {
		t.Errorf("expected HostedZone to be gone after abandon, still present with finalizers %v", got.Finalizers)
	}
}

func TestHostedZoneDeleteFallbackCallerReferenceMatch(t *testing.T) {
	hz := &awsv1alpha1.HostedZone{
		ObjectMeta: metav1.ObjectMeta{
			Name: "zone1", Namespace: "default", UID: "uid-1",
			Finalizers: []string{awsv1alpha1.FinalizerName},
		},
		Spec: awsv1alpha1.HostedZoneSpec{Name: "example.com"},
		// Status.HostedZoneID intentionally empty: forces the fallback lookup.
	}
	k8s := newHZClient(t, hz)
	if err := k8s.Delete(context.Background(), hz); err != nil {
		t.Fatalf("delete: %v", err)
	}

	deleteCalls := 0
	api := &fakeHostedZoneAPI{
		ListHostedZonesByNameFn: func(_ context.Context, params *awsroute53.ListHostedZonesByNameInput, _ ...func(*awsroute53.Options)) (*awsroute53.ListHostedZonesByNameOutput, error) {
			if aws.ToString(params.DNSName) != "example.com" {
				t.Errorf("ListHostedZonesByName DNSName = %q, want example.com", aws.ToString(params.DNSName))
			}
			return &awsroute53.ListHostedZonesByNameOutput{
				HostedZones: []types.HostedZone{
					{
						// Same name, different creator: must be skipped.
						Id:              aws.String("/hostedzone/ZOTHER"),
						Name:            aws.String("example.com."),
						CallerReference: aws.String("someone-else"),
					},
					{
						Id:              aws.String("/hostedzone/ZMINE"),
						Name:            aws.String("example.com."),
						CallerReference: aws.String(hostedZoneCallerRef("uid-1")),
					},
				},
			}, nil
		},
		ListResourceRecordSetsFn: func(_ context.Context, _ *awsroute53.ListResourceRecordSetsInput, _ ...func(*awsroute53.Options)) (*awsroute53.ListResourceRecordSetsOutput, error) {
			return &awsroute53.ListResourceRecordSetsOutput{IsTruncated: false}, nil
		},
		DeleteHostedZoneFn: func(_ context.Context, params *awsroute53.DeleteHostedZoneInput, _ ...func(*awsroute53.Options)) (*awsroute53.DeleteHostedZoneOutput, error) {
			deleteCalls++
			if aws.ToString(params.Id) != "ZMINE" {
				t.Errorf("DeleteHostedZone id = %q, want ZMINE", aws.ToString(params.Id))
			}
			return &awsroute53.DeleteHostedZoneOutput{}, nil
		},
	}
	r := &HostedZoneReconciler{Client: k8s, Scheme: newHZScheme(t), Route53Client: api, CallerReference: "test"}

	key := k8stypes.NamespacedName{Name: "zone1", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if deleteCalls != 1 {
		t.Errorf("DeleteHostedZone calls = %d, want 1", deleteCalls)
	}
}

func TestHostedZoneDeleteFallbackAmbiguousSkipped(t *testing.T) {
	hz := &awsv1alpha1.HostedZone{
		ObjectMeta: metav1.ObjectMeta{
			Name: "zone1", Namespace: "default", UID: "uid-1",
			Finalizers: []string{awsv1alpha1.FinalizerName},
		},
		Spec: awsv1alpha1.HostedZoneSpec{Name: "example.com."},
		// Status.HostedZoneID intentionally empty: forces the fallback lookup.
	}
	k8s := newHZClient(t, hz)
	if err := k8s.Delete(context.Background(), hz); err != nil {
		t.Fatalf("delete: %v", err)
	}

	api := &fakeHostedZoneAPI{
		ListHostedZonesByNameFn: func(_ context.Context, _ *awsroute53.ListHostedZonesByNameInput, _ ...func(*awsroute53.Options)) (*awsroute53.ListHostedZonesByNameOutput, error) {
			return &awsroute53.ListHostedZonesByNameOutput{
				HostedZones: []types.HostedZone{
					{
						// Same name but a CallerReference this CR never issued.
						Id:              aws.String("/hostedzone/ZOTHER"),
						Name:            aws.String("example.com."),
						CallerReference: aws.String("someone-else"),
					},
				},
			}, nil
		},
		// DeleteHostedZoneFn intentionally nil: a delete would panic the test.
	}
	r := &HostedZoneReconciler{Client: k8s, Scheme: newHZScheme(t), Route53Client: api, CallerReference: "test"}

	key := k8stypes.NamespacedName{Name: "zone1", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	got := &awsv1alpha1.HostedZone{}
	if err := k8s.Get(context.Background(), key, got); err == nil {
		t.Errorf("expected HostedZone to be gone (finalizer removed without AWS delete), still present with finalizers %v", got.Finalizers)
	}
}
