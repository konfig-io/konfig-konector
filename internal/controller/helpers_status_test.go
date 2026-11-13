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
	"time"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

// A reconcile that started from a stale read (no ARN yet) must not erase the
// ARN a previous reconcile already persisted when its status write conflicts.
func TestPersistStatusConflictKeepsRecordedIdentifiers(t *testing.T) {
	ctx := context.Background()
	scheme := dbInstanceScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&awsv1alpha1.DBInstance{}).
		WithObjects(dbInstanceCR()).Build()
	key := client.ObjectKey{Name: "my-db", Namespace: "default"}

	stale := &awsv1alpha1.DBInstance{}
	if err := c.Get(ctx, key, stale); err != nil {
		t.Fatalf("get stale: %v", err)
	}

	fresh := stale.DeepCopy()
	fresh.Status.DBInstanceARN = testDBInstanceARN
	fresh.Status.DBInstanceStatus = "creating"
	if err := c.Status().Update(ctx, fresh); err != nil {
		t.Fatalf("persist ARN: %v", err)
	}

	stale.Status.DBInstanceStatus = "failed"
	apimeta.SetStatusCondition(&stale.Status.Conditions, metav1.Condition{
		Type: awsv1alpha1.ConditionReady, Status: metav1.ConditionFalse, Reason: awsv1alpha1.ReasonError, Message: "boom",
	})
	if err := persistStatus(ctx, c, stale); err != nil {
		t.Fatalf("persistStatus: %v", err)
	}

	got := &awsv1alpha1.DBInstance{}
	if err := c.Get(ctx, key, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.DBInstanceARN != testDBInstanceARN {
		t.Errorf("status.dbInstanceArn = %q, want %q preserved across the conflict", got.Status.DBInstanceARN, testDBInstanceARN)
	}
	if got.Status.DBInstanceStatus != "failed" {
		t.Errorf("status.dbInstanceStatus = %q, want the writer's own value %q", got.Status.DBInstanceStatus, "failed")
	}
	if cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady); cond == nil || cond.Message != "boom" {
		t.Errorf("Ready condition = %+v, want the writer's condition", cond)
	}
	if stale.Status.DBInstanceARN != testDBInstanceARN {
		t.Errorf("in-memory object not updated with preserved ARN: %q", stale.Status.DBInstanceARN)
	}
}

func TestIsUserChange(t *testing.T) {
	base := func() *awsv1alpha1.DBInstance {
		db := dbInstanceCR()
		db.Generation = 1
		return db
	}
	now := metav1.NewTime(time.Now())
	cases := []struct {
		name   string
		mutate func(*awsv1alpha1.DBInstance)
		want   bool
	}{
		{"status only", func(db *awsv1alpha1.DBInstance) { db.Status.DBInstanceARN = testDBInstanceARN }, false},
		{"finalizer only", func(db *awsv1alpha1.DBInstance) { db.Finalizers = nil }, false},
		{"spec (generation)", func(db *awsv1alpha1.DBInstance) { db.Generation = 2 }, true},
		{"annotation", func(db *awsv1alpha1.DBInstance) {
			db.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
		}, true},
		{"label", func(db *awsv1alpha1.DBInstance) { db.Labels = map[string]string{"a": "b"} }, true},
		{"deletion", func(db *awsv1alpha1.DBInstance) { db.DeletionTimestamp = &now }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := base()
			tc.mutate(n)
			if got := isUserChange(event.UpdateEvent{ObjectOld: base(), ObjectNew: n}); got != tc.want {
				t.Errorf("isUserChange = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSameJSON(t *testing.T) {
	if !sameJSON(`{"rules":[{"a":1,"b":2}]}`, "{\n \"rules\": [ {\"b\":2, \"a\":1} ]\n}") {
		t.Error("expected documents differing only in whitespace and key order to be equal")
	}
	if sameJSON(`{"rules":[{"a":1}]}`, `{"rules":[{"a":2}]}`) {
		t.Error("expected different documents to be unequal")
	}
	if sameJSON(``, ``) {
		t.Error("expected unparseable input to compare unequal")
	}
}
