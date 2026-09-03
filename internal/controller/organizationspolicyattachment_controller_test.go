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
	awsorgs "github.com/aws/aws-sdk-go-v2/service/organizations"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

type fakeOrgPolicyAttachment struct {
	attachCalled bool
	attachedID   string
	detachCalled bool
	detachedID   string
}

func (f *fakeOrgPolicyAttachment) AttachPolicy(_ context.Context, params *awsorgs.AttachPolicyInput, _ ...func(*awsorgs.Options)) (*awsorgs.AttachPolicyOutput, error) {
	f.attachCalled = true
	f.attachedID = aws.ToString(params.PolicyId)
	return &awsorgs.AttachPolicyOutput{}, nil
}

func (f *fakeOrgPolicyAttachment) DetachPolicy(_ context.Context, params *awsorgs.DetachPolicyInput, _ ...func(*awsorgs.Options)) (*awsorgs.DetachPolicyOutput, error) {
	f.detachCalled = true
	f.detachedID = aws.ToString(params.PolicyId)
	return &awsorgs.DetachPolicyOutput{}, nil
}

func orgPolicyAttachmentCR(mutate ...func(*awsv1alpha1.OrganizationsPolicyAttachment)) *awsv1alpha1.OrganizationsPolicyAttachment {
	att := &awsv1alpha1.OrganizationsPolicyAttachment{
		ObjectMeta: metav1.ObjectMeta{Name: "deny-regions-workloads", Namespace: "default"},
		Spec: awsv1alpha1.OrganizationsPolicyAttachmentSpec{
			PolicyRef: awsv1alpha1.OrganizationsPolicyRef{PolicyID: testOrgPolicyID},
			TargetID:  "ou-abcd-11111111",
		},
	}
	for _, m := range mutate {
		m(att)
	}
	return att
}

func TestOrganizationsPolicyAttachmentReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "deny-regions-workloads", Namespace: "default"}}

	t.Run("create attaches policy and persists identifiers", func(t *testing.T) {
		ctx := context.Background()
		scheme := newOrgScheme(t)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.OrganizationsPolicyAttachment{}).
			WithObjects(orgPolicyAttachmentCR()).Build()
		f := &fakeOrgPolicyAttachment{}
		r := &OrganizationsPolicyAttachmentReconciler{Client: c, Scheme: scheme, OrganizationsClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.OrganizationsPolicyAttachment{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if !f.attachCalled || f.attachedID != testOrgPolicyID {
			t.Errorf("expected AttachPolicy(%s), got called=%v id=%q", testOrgPolicyID, f.attachCalled, f.attachedID)
		}
		if got.Status.PolicyID != testOrgPolicyID || got.Status.TargetID != "ou-abcd-11111111" {
			t.Errorf("status = %q/%q, want %s/ou-abcd-11111111", got.Status.PolicyID, got.Status.TargetID, testOrgPolicyID)
		}
	})

	t.Run("policyRef to a not-ready OrganizationsPolicy requeues as dependency", func(t *testing.T) {
		ctx := context.Background()
		scheme := newOrgScheme(t)
		pol := orgPolicyCR() // no status.policyId yet
		att := orgPolicyAttachmentCR(func(a *awsv1alpha1.OrganizationsPolicyAttachment) {
			a.Spec.PolicyRef = awsv1alpha1.OrganizationsPolicyRef{Name: "deny-regions"}
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.OrganizationsPolicyAttachment{}, &awsv1alpha1.OrganizationsPolicy{}).
			WithObjects(pol, att).Build()
		f := &fakeOrgPolicyAttachment{}
		r := &OrganizationsPolicyAttachmentReconciler{Client: c, Scheme: scheme, OrganizationsClient: f}

		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if res != requeueDependency {
			t.Errorf("result = %+v, want requeueDependency", res)
		}
		if f.attachCalled {
			t.Error("AttachPolicy must not be called while the policy has no ID")
		}
	})

	t.Run("delete detaches policy", func(t *testing.T) {
		ctx := context.Background()
		scheme := newOrgScheme(t)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.OrganizationsPolicyAttachment{}).
			WithObjects(orgPolicyAttachmentCR(func(a *awsv1alpha1.OrganizationsPolicyAttachment) {
				a.Finalizers = []string{awsv1alpha1.FinalizerName}
				a.Status.PolicyID = testOrgPolicyID
				a.Status.TargetID = "ou-abcd-11111111"
			})).Build()
		f := &fakeOrgPolicyAttachment{}
		r := &OrganizationsPolicyAttachmentReconciler{Client: c, Scheme: scheme, OrganizationsClient: f}

		if err := c.Delete(ctx, orgPolicyAttachmentCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.detachCalled || f.detachedID != testOrgPolicyID {
			t.Errorf("expected DetachPolicy(%s), got called=%v id=%q", testOrgPolicyID, f.detachCalled, f.detachedID)
		}
		got := &awsv1alpha1.OrganizationsPolicyAttachment{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected CR gone, got err=%v", err)
		}
	})

	t.Run("abandon annotation skips detach", func(t *testing.T) {
		ctx := context.Background()
		scheme := newOrgScheme(t)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.OrganizationsPolicyAttachment{}).
			WithObjects(orgPolicyAttachmentCR(func(a *awsv1alpha1.OrganizationsPolicyAttachment) {
				a.Finalizers = []string{awsv1alpha1.FinalizerName}
				a.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
				a.Status.PolicyID = testOrgPolicyID
				a.Status.TargetID = "ou-abcd-11111111"
			})).Build()
		f := &fakeOrgPolicyAttachment{}
		r := &OrganizationsPolicyAttachmentReconciler{Client: c, Scheme: scheme, OrganizationsClient: f}

		if err := c.Delete(ctx, orgPolicyAttachmentCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.detachCalled {
			t.Error("DetachPolicy must not be called when abandoning")
		}
	})
}
