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

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

// Deleting an AWSProvider is blocked while a resource still references it,
// and completes once the reference is gone.
func TestAWSProviderDeletionBlockedWhileReferenced(t *testing.T) {
	s := crossAccountScheme(t)
	now := metav1.Now()
	p := &awsv1alpha1.AWSProvider{ObjectMeta: metav1.ObjectMeta{Name: "prod", Finalizers: []string{awsv1alpha1.FinalizerName}, DeletionTimestamp: &now}}
	q := &awsv1alpha1.SQSQueue{ObjectMeta: metav1.ObjectMeta{Name: "q", Namespace: "ns"},
		Spec: awsv1alpha1.SQSQueueSpec{QueueName: "q", ProviderRef: &awsv1alpha1.ProviderRef{Name: "prod"}}}
	c := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&awsv1alpha1.AWSProvider{}).WithObjects(p, q).Build()
	r := &AWSProviderReconciler{Client: c, Scheme: s}
	key := k8stypes.NamespacedName{Name: "prod"}

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key})
	if err != nil || res.RequeueAfter == 0 {
		t.Fatalf("expected deletion to be blocked and requeued, got %+v %v", res, err)
	}
	got := &awsv1alpha1.AWSProvider{}
	if err := c.Get(context.Background(), key, got); err != nil {
		t.Fatalf("provider must still exist: %v", err)
	}
	if cnd := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady); cnd == nil || cnd.Reason != "InUse" {
		t.Fatalf("expected InUse condition, got %+v", cnd)
	}

	if err := c.Delete(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatal(err)
	}
	if err := c.Get(context.Background(), key, got); err == nil {
		t.Fatal("provider should be gone once nothing references it")
	}
}
