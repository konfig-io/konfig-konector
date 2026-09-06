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

// fakeRecordSetAPI implements RecordSetAWSAPI with function fields.
type fakeRecordSetAPI struct {
	ChangeResourceRecordSetsFn func(ctx context.Context, params *awsroute53.ChangeResourceRecordSetsInput, optFns ...func(*awsroute53.Options)) (*awsroute53.ChangeResourceRecordSetsOutput, error)
	ListResourceRecordSetsFn   func(ctx context.Context, params *awsroute53.ListResourceRecordSetsInput, optFns ...func(*awsroute53.Options)) (*awsroute53.ListResourceRecordSetsOutput, error)
}

func (f *fakeRecordSetAPI) ChangeResourceRecordSets(ctx context.Context, params *awsroute53.ChangeResourceRecordSetsInput, optFns ...func(*awsroute53.Options)) (*awsroute53.ChangeResourceRecordSetsOutput, error) {
	if f.ChangeResourceRecordSetsFn == nil {
		panic("unexpected call to ChangeResourceRecordSets")
	}
	return f.ChangeResourceRecordSetsFn(ctx, params, optFns...)
}

func (f *fakeRecordSetAPI) ListResourceRecordSets(ctx context.Context, params *awsroute53.ListResourceRecordSetsInput, optFns ...func(*awsroute53.Options)) (*awsroute53.ListResourceRecordSetsOutput, error) {
	if f.ListResourceRecordSetsFn == nil {
		panic("unexpected call to ListResourceRecordSets")
	}
	return f.ListResourceRecordSetsFn(ctx, params, optFns...)
}

var _ RecordSetAWSAPI = (*fakeRecordSetAPI)(nil)

func newRSClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&awsv1alpha1.RecordSet{}, &awsv1alpha1.HostedZone{}, &awsv1alpha1.HealthCheck{}).
		Build()
}

func rsScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return scheme
}

func testRecordSet(zoneRef awsv1alpha1.HostedZoneRef) *awsv1alpha1.RecordSet {
	return &awsv1alpha1.RecordSet{
		ObjectMeta: metav1.ObjectMeta{Name: "rec1", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}, UID: "uid-rs", Generation: 1},
		Spec: awsv1alpha1.RecordSetSpec{
			HostedZoneRef: zoneRef,
			Name:          "www.example.com.",
			Type:          "A",
			TTL:           aws.Int64(300),
			Records:       []string{"192.0.2.10"},
		},
	}
}

func TestRecordSetCreateHappyPath(t *testing.T) {
	rs := testRecordSet(awsv1alpha1.HostedZoneRef{ID: "Z123"})
	k8s := newRSClient(t, rs)

	changeCalls := 0
	api := &fakeRecordSetAPI{
		ListResourceRecordSetsFn: func(_ context.Context, _ *awsroute53.ListResourceRecordSetsInput, _ ...func(*awsroute53.Options)) (*awsroute53.ListResourceRecordSetsOutput, error) {
			return &awsroute53.ListResourceRecordSetsOutput{IsTruncated: false}, nil
		},
		ChangeResourceRecordSetsFn: func(_ context.Context, params *awsroute53.ChangeResourceRecordSetsInput, _ ...func(*awsroute53.Options)) (*awsroute53.ChangeResourceRecordSetsOutput, error) {
			changeCalls++
			if aws.ToString(params.HostedZoneId) != "Z123" {
				t.Errorf("zone id = %q, want Z123", aws.ToString(params.HostedZoneId))
			}
			if len(params.ChangeBatch.Changes) != 1 || params.ChangeBatch.Changes[0].Action != types.ChangeActionUpsert {
				t.Errorf("expected single UPSERT change, got %+v", params.ChangeBatch.Changes)
			}
			return &awsroute53.ChangeResourceRecordSetsOutput{
				ChangeInfo: &types.ChangeInfo{Id: aws.String("/change/C111"), Status: types.ChangeStatusPending},
			}, nil
		},
	}
	r := &RecordSetReconciler{Client: k8s, Scheme: rsScheme(t), Route53Client: api}

	key := k8stypes.NamespacedName{Name: "rec1", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if changeCalls != 1 {
		t.Errorf("ChangeResourceRecordSets calls = %d, want 1", changeCalls)
	}

	got := &awsv1alpha1.RecordSet{}
	if err := k8s.Get(context.Background(), key, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.HostedZoneID != "Z123" {
		t.Errorf("status.hostedZoneId = %q, want Z123", got.Status.HostedZoneID)
	}
	if got.Status.ChangeID != "/change/C111" || got.Status.ChangeStatus != "PENDING" {
		t.Errorf("status change = %q/%q, want /change/C111 PENDING", got.Status.ChangeID, got.Status.ChangeStatus)
	}
	cond := meta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
	if cond == nil || cond.Status != metav1.ConditionTrue {
		t.Errorf("Ready condition = %+v, want True", cond)
	}
}

func TestRecordSetSteadyStateNoChange(t *testing.T) {
	rs := testRecordSet(awsv1alpha1.HostedZoneRef{ID: "Z123"})
	rs.Finalizers = []string{awsv1alpha1.FinalizerName}
	k8s := newRSClient(t, rs)

	api := &fakeRecordSetAPI{
		ListResourceRecordSetsFn: func(_ context.Context, _ *awsroute53.ListResourceRecordSetsInput, _ ...func(*awsroute53.Options)) (*awsroute53.ListResourceRecordSetsOutput, error) {
			return &awsroute53.ListResourceRecordSetsOutput{
				ResourceRecordSets: []types.ResourceRecordSet{
					{
						Name:            aws.String("www.example.com."),
						Type:            types.RRTypeA,
						TTL:             aws.Int64(300),
						ResourceRecords: []types.ResourceRecord{{Value: aws.String("192.0.2.10")}},
					},
				},
				IsTruncated: false,
			}, nil
		},
		// ChangeResourceRecordSetsFn intentionally nil: an upsert would panic the test.
	}
	r := &RecordSetReconciler{Client: k8s, Scheme: rsScheme(t), Route53Client: api}

	key := k8stypes.NamespacedName{Name: "rec1", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got := &awsv1alpha1.RecordSet{}
	if err := k8s.Get(context.Background(), key, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	cond := meta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
	if cond == nil || cond.Status != metav1.ConditionTrue {
		t.Errorf("Ready condition = %+v, want True", cond)
	}
}

func TestRecordSetDeleteWithFinalizer(t *testing.T) {
	rs := testRecordSet(awsv1alpha1.HostedZoneRef{ID: "Z123"})
	rs.Finalizers = []string{awsv1alpha1.FinalizerName}
	rs.Status.HostedZoneID = "Z123"
	k8s := newRSClient(t, rs)
	if err := k8s.Delete(context.Background(), rs); err != nil {
		t.Fatalf("delete: %v", err)
	}

	deleteCalls := 0
	api := &fakeRecordSetAPI{
		ChangeResourceRecordSetsFn: func(_ context.Context, params *awsroute53.ChangeResourceRecordSetsInput, _ ...func(*awsroute53.Options)) (*awsroute53.ChangeResourceRecordSetsOutput, error) {
			deleteCalls++
			if aws.ToString(params.HostedZoneId) != "Z123" {
				t.Errorf("zone id = %q, want Z123", aws.ToString(params.HostedZoneId))
			}
			if len(params.ChangeBatch.Changes) != 1 || params.ChangeBatch.Changes[0].Action != types.ChangeActionDelete {
				t.Errorf("expected single DELETE change, got %+v", params.ChangeBatch.Changes)
			}
			rrs := params.ChangeBatch.Changes[0].ResourceRecordSet
			if aws.ToString(rrs.Name) != "www.example.com." || rrs.Type != types.RRTypeA {
				t.Errorf("record = %q/%s, want www.example.com./A", aws.ToString(rrs.Name), rrs.Type)
			}
			return &awsroute53.ChangeResourceRecordSetsOutput{
				ChangeInfo: &types.ChangeInfo{Id: aws.String("/change/C222")},
			}, nil
		},
	}
	r := &RecordSetReconciler{Client: k8s, Scheme: rsScheme(t), Route53Client: api}

	key := k8stypes.NamespacedName{Name: "rec1", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if deleteCalls != 1 {
		t.Errorf("delete ChangeResourceRecordSets calls = %d, want 1", deleteCalls)
	}
	got := &awsv1alpha1.RecordSet{}
	if err := k8s.Get(context.Background(), key, got); err == nil {
		t.Errorf("expected RecordSet to be gone after finalizer removal, still present with finalizers %v", got.Finalizers)
	}
}

func TestRecordSetAbandonAnnotation(t *testing.T) {
	rs := testRecordSet(awsv1alpha1.HostedZoneRef{ID: "Z123"})
	rs.Finalizers = []string{awsv1alpha1.FinalizerName}
	rs.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
	rs.Status.HostedZoneID = "Z123"
	k8s := newRSClient(t, rs)
	if err := k8s.Delete(context.Background(), rs); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// All fns nil: any AWS call panics the test.
	api := &fakeRecordSetAPI{}
	r := &RecordSetReconciler{Client: k8s, Scheme: rsScheme(t), Route53Client: api}

	key := k8stypes.NamespacedName{Name: "rec1", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	got := &awsv1alpha1.RecordSet{}
	if err := k8s.Get(context.Background(), key, got); err == nil {
		t.Errorf("expected RecordSet to be gone after abandon, still present with finalizers %v", got.Finalizers)
	}
}

// TestRecordSetDeleteReResolvesZoneFromSpec exercises the recent change where
// deletion rebuilds the record from spec and re-resolves the zone ID from the
// referenced HostedZone CR when status.hostedZoneId was never persisted.
func TestRecordSetDeleteReResolvesZoneFromSpec(t *testing.T) {
	hz := &awsv1alpha1.HostedZone{
		ObjectMeta: metav1.ObjectMeta{Name: "zone1", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}, UID: "uid-hz"},
		Spec:       awsv1alpha1.HostedZoneSpec{Name: "example.com."},
		Status:     awsv1alpha1.HostedZoneStatus{HostedZoneID: "ZFROMCR"},
	}
	rs := testRecordSet(awsv1alpha1.HostedZoneRef{Name: "zone1"})
	rs.Finalizers = []string{awsv1alpha1.FinalizerName}
	// Status.HostedZoneID intentionally empty.
	k8s := newRSClient(t, rs, hz)
	if err := k8s.Delete(context.Background(), rs); err != nil {
		t.Fatalf("delete: %v", err)
	}

	deleteCalls := 0
	api := &fakeRecordSetAPI{
		ChangeResourceRecordSetsFn: func(_ context.Context, params *awsroute53.ChangeResourceRecordSetsInput, _ ...func(*awsroute53.Options)) (*awsroute53.ChangeResourceRecordSetsOutput, error) {
			deleteCalls++
			if aws.ToString(params.HostedZoneId) != "ZFROMCR" {
				t.Errorf("zone id = %q, want ZFROMCR (re-resolved from HostedZone CR)", aws.ToString(params.HostedZoneId))
			}
			rrs := params.ChangeBatch.Changes[0].ResourceRecordSet
			if aws.ToString(rrs.Name) != "www.example.com." || rrs.Type != types.RRTypeA {
				t.Errorf("record rebuilt from spec = %q/%s, want www.example.com./A", aws.ToString(rrs.Name), rrs.Type)
			}
			if aws.ToInt64(rrs.TTL) != 300 || len(rrs.ResourceRecords) != 1 {
				t.Errorf("record values not rebuilt from spec: TTL=%d records=%v", aws.ToInt64(rrs.TTL), rrs.ResourceRecords)
			}
			return &awsroute53.ChangeResourceRecordSetsOutput{
				ChangeInfo: &types.ChangeInfo{Id: aws.String("/change/C333")},
			}, nil
		},
	}
	r := &RecordSetReconciler{Client: k8s, Scheme: rsScheme(t), Route53Client: api}

	key := k8stypes.NamespacedName{Name: "rec1", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if deleteCalls != 1 {
		t.Errorf("delete ChangeResourceRecordSets calls = %d, want 1", deleteCalls)
	}
	got := &awsv1alpha1.RecordSet{}
	if err := k8s.Get(context.Background(), key, got); err == nil {
		t.Errorf("expected RecordSet to be gone after finalizer removal, still present with finalizers %v", got.Finalizers)
	}
}

// TestRecordSetDependencyNotReady verifies that a HostedZone CR without an ID
// yet causes a requeue (requeueDependency) without an error and no AWS calls.
func TestRecordSetDependencyNotReady(t *testing.T) {
	hz := &awsv1alpha1.HostedZone{
		ObjectMeta: metav1.ObjectMeta{Name: "zone1", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}, UID: "uid-hz"},
		Spec:       awsv1alpha1.HostedZoneSpec{Name: "example.com."},
		// Status.HostedZoneID intentionally empty: dependency not ready.
	}
	rs := testRecordSet(awsv1alpha1.HostedZoneRef{Name: "zone1"})
	k8s := newRSClient(t, rs, hz)

	// All fns nil: any AWS call panics the test.
	api := &fakeRecordSetAPI{}
	r := &RecordSetReconciler{Client: k8s, Scheme: rsScheme(t), Route53Client: api}

	key := k8stypes.NamespacedName{Name: "rec1", Namespace: "default"}
	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key})
	if err != nil {
		t.Fatalf("expected no error for dependency-not-ready, got %v", err)
	}
	if res != requeueDependency {
		t.Errorf("result = %+v, want requeueDependency %+v", res, requeueDependency)
	}
}
