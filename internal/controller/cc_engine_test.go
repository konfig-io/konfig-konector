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

	cctypes "github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	"github.com/konfig-io/konfig-konector/internal/cfn"
)

// The generated LogsLogStream kind exercises the shared typed-kind engine:
// typed spec → DesiredState, async create, steady state, drift patch, delete.
func TestCloudControlKindEngineLifecycle(t *testing.T) {
	s := crossAccountScheme(t)
	kind := CloudControlKind{Kind: "LogsLogStream", TypeName: "AWS::Logs::LogStream", New: func() cfn.CloudControlObject { return &awsv1alpha1.LogsLogStream{} }}
	obj := &awsv1alpha1.LogsLogStream{
		ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "ns", UID: "u1", Generation: 1, Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec:       awsv1alpha1.LogsLogStreamSpec{LogGroupName: "/app", LogStreamName: "web"},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&awsv1alpha1.LogsLogStream{}).WithObjects(obj).Build()
	f := &fakeCC{live: map[string]string{}, status: cctypes.OperationStatusInProgress}
	r := &CloudControlKindReconciler{Client: c, Scheme: s, CCClient: f, Kind: kind}
	key := k8stypes.NamespacedName{Name: "ls", Namespace: "ns"}

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key})
	if err != nil || res != requeuePending || f.created != 1 {
		t.Fatalf("expected async create, got %+v %v created=%d", res, err, f.created)
	}
	got := &awsv1alpha1.LogsLogStream{}
	_ = c.Get(context.Background(), key, got)
	if got.Status.RequestToken != "req-1" || got.Status.Operation != "CREATE" {
		t.Fatalf("token not persisted: %+v", got.Status)
	}

	// Desired state must use CloudFormation property names.
	f.status = cctypes.OperationStatusSuccess
	f.live["id-1"] = `{"LogGroupName":"/app","LogStreamName":"web"}`
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatal(err)
	}
	_ = c.Get(context.Background(), key, got)
	if cnd := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady); cnd == nil || cnd.Status != metav1.ConditionTrue || f.updated != 0 {
		t.Fatalf("expected Ready with no update, got %+v upd=%d", cnd, f.updated)
	}

	// Drift in a property → JSON patch with the CFN name.
	f.live["id-1"] = `{"LogGroupName":"/app","LogStreamName":"renamed"}`
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatal(err)
	}
	if f.updated != 1 || f.lastPatch != `[{"op":"replace","path":"/LogStreamName","value":"web"}]` {
		t.Fatalf("unexpected patch %q (updated=%d)", f.lastPatch, f.updated)
	}

	// Delete.
	_ = c.Get(context.Background(), key, got)
	got.Status.RequestToken, got.Status.Operation = "", ""
	_ = c.Status().Update(context.Background(), got)
	if err := c.Delete(context.Background(), got); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatal(err)
	}
	if f.deleted != 1 {
		t.Fatalf("expected delete, got %d", f.deleted)
	}
}

func TestCloudControlKindsRegistryIsPopulated(t *testing.T) {
	kinds := CloudControlKinds()
	if len(kinds) < 500 {
		t.Fatalf("expected generated registry, got %d kinds", len(kinds))
	}
	seen := map[string]bool{}
	for _, k := range kinds {
		if seen[k.Kind] {
			t.Fatalf("duplicate kind %s", k.Kind)
		}
		seen[k.Kind] = true
		if k.New().CloudControlTypeName() != k.TypeName {
			t.Fatalf("%s: type name mismatch", k.Kind)
		}
	}
}

// A DELETE whose handler reports NotFound (resource or parent already gone)
// completes instead of failing forever.
func TestCloudControlKindDeleteNotFoundCompletes(t *testing.T) {
	s := crossAccountScheme(t)
	kind := CloudControlKind{Kind: "LogsLogStream", TypeName: "AWS::Logs::LogStream", New: func() cfn.CloudControlObject { return &awsv1alpha1.LogsLogStream{} }}
	now := metav1.Now()
	obj := &awsv1alpha1.LogsLogStream{
		ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "ns", UID: "u1", Finalizers: []string{awsv1alpha1.FinalizerName}, DeletionTimestamp: &now},
		Spec:       awsv1alpha1.LogsLogStreamSpec{LogGroupName: "/gone", LogStreamName: "web"},
	}
	obj.Status.Identifier, obj.Status.Operation, obj.Status.RequestToken = "/gone|web", "DELETE", "req-del"
	c := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&awsv1alpha1.LogsLogStream{}).WithObjects(obj).Build()
	f := &fakeCC{live: map[string]string{}, status: cctypes.OperationStatusFailed, errorCode: cctypes.HandlerErrorCodeNotFound}
	r := &CloudControlKindReconciler{Client: c, Scheme: s, CCClient: f, Kind: kind}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "ls", Namespace: "ns"}}); err != nil {
		t.Fatalf("delete should complete on NotFound, got %v", err)
	}
	got := &awsv1alpha1.LogsLogStream{}
	if err := c.Get(context.Background(), k8stypes.NamespacedName{Name: "ls", Namespace: "ns"}, got); err == nil {
		t.Fatal("finalizer should have been removed and object deleted")
	}
}

// A failed create must not be retried with the same idempotency token, or
// Cloud Control replays the cached failure forever.
func TestCloudControlKindFailedCreateRotatesToken(t *testing.T) {
	s := crossAccountScheme(t)
	kind := CloudControlKind{Kind: "LogsLogStream", TypeName: "AWS::Logs::LogStream", New: func() cfn.CloudControlObject { return &awsv1alpha1.LogsLogStream{} }}
	obj := &awsv1alpha1.LogsLogStream{
		ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "ns", UID: "u1", Generation: 1, Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec:       awsv1alpha1.LogsLogStreamSpec{LogGroupName: "/missing", LogStreamName: "web"},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&awsv1alpha1.LogsLogStream{}).WithObjects(obj).Build()
	f := &fakeCC{live: map[string]string{}, status: cctypes.OperationStatusFailed}
	r := &CloudControlKindReconciler{Client: c, Scheme: s, CCClient: f, Kind: kind}
	key := k8stypes.NamespacedName{Name: "ls", Namespace: "ns"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err == nil {
		t.Fatal("expected the failed create to surface as an error")
	}
	first := f.createToken
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err == nil {
		t.Fatal("expected the second failed create to surface as an error")
	}
	if f.created != 2 || f.createToken == first {
		t.Fatalf("retry must use a fresh token: created=%d first=%q second=%q", f.created, first, f.createToken)
	}
}
