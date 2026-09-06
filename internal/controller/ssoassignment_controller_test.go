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
	awssso "github.com/aws/aws-sdk-go-v2/service/ssoadmin"
	ssotypes "github.com/aws/aws-sdk-go-v2/service/ssoadmin/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

type fakeSSOAssignment struct {
	createCalled bool
	deleteCalled bool
	deletedArn   string
}

func (f *fakeSSOAssignment) CreateAccountAssignment(_ context.Context, _ *awssso.CreateAccountAssignmentInput, _ ...func(*awssso.Options)) (*awssso.CreateAccountAssignmentOutput, error) {
	f.createCalled = true
	return &awssso.CreateAccountAssignmentOutput{
		AccountAssignmentCreationStatus: &ssotypes.AccountAssignmentOperationStatus{
			RequestId: aws.String("req-123"),
			Status:    ssotypes.StatusValuesInProgress,
		},
	}, nil
}

func (f *fakeSSOAssignment) DeleteAccountAssignment(_ context.Context, params *awssso.DeleteAccountAssignmentInput, _ ...func(*awssso.Options)) (*awssso.DeleteAccountAssignmentOutput, error) {
	f.deleteCalled = true
	f.deletedArn = aws.ToString(params.PermissionSetArn)
	return &awssso.DeleteAccountAssignmentOutput{
		AccountAssignmentDeletionStatus: &ssotypes.AccountAssignmentOperationStatus{
			Status: ssotypes.StatusValuesInProgress,
		},
	}, nil
}

func (f *fakeSSOAssignment) DescribeAccountAssignmentCreationStatus(_ context.Context, _ *awssso.DescribeAccountAssignmentCreationStatusInput, _ ...func(*awssso.Options)) (*awssso.DescribeAccountAssignmentCreationStatusOutput, error) {
	return &awssso.DescribeAccountAssignmentCreationStatusOutput{
		AccountAssignmentCreationStatus: &ssotypes.AccountAssignmentOperationStatus{
			RequestId: aws.String("req-123"),
			Status:    ssotypes.StatusValuesSucceeded,
		},
	}, nil
}

func (f *fakeSSOAssignment) ListAccountAssignments(_ context.Context, _ *awssso.ListAccountAssignmentsInput, _ ...func(*awssso.Options)) (*awssso.ListAccountAssignmentsOutput, error) {
	return &awssso.ListAccountAssignmentsOutput{}, nil
}

func ssoAssignmentCR(mutate ...func(*awsv1alpha1.SSOAssignment)) *awsv1alpha1.SSOAssignment {
	sa := &awsv1alpha1.SSOAssignment{
		ObjectMeta: metav1.ObjectMeta{Name: "admin-sandbox", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec: awsv1alpha1.SSOAssignmentSpec{
			InstanceArn:      testSSOInstanceARN,
			PermissionSetRef: awsv1alpha1.PermissionSetRef{ARN: testPermissionSetARN},
			PrincipalType:    "GROUP",
			PrincipalId:      "f81d4fae-7dec-11d0-a765-00a0c91e6bf6",
			TargetType:       "AWS_ACCOUNT",
			TargetId:         "111122223333",
		},
	}
	for _, m := range mutate {
		m(sa)
	}
	return sa
}

func TestSSOAssignmentReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "admin-sandbox", Namespace: "default"}}

	t.Run("create kicks off async assignment and persists request ID", func(t *testing.T) {
		ctx := context.Background()
		scheme := newOrgScheme(t)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.SSOAssignment{}).
			WithObjects(ssoAssignmentCR()).Build()
		f := &fakeSSOAssignment{}
		r := &SSOAssignmentReconciler{Client: c, Scheme: scheme, SSOAdminClient: f}

		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if res != requeueOrgPolling {
			t.Errorf("result = %+v, want requeueOrgPolling", res)
		}
		got := &awsv1alpha1.SSOAssignment{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if !f.createCalled {
			t.Error("expected CreateAccountAssignment to be called")
		}
		if got.Status.RequestId != "req-123" {
			t.Errorf("status.requestId = %q, want req-123", got.Status.RequestId)
		}
		if got.Status.PermissionSetArn != testPermissionSetARN {
			t.Errorf("status.permissionSetArn = %q, want %q", got.Status.PermissionSetArn, testPermissionSetARN)
		}
	})

	t.Run("permissionSetRef to not-ready PermissionSet requeues as dependency", func(t *testing.T) {
		ctx := context.Background()
		scheme := newOrgScheme(t)
		ps := permissionSetCR() // no status ARN
		sa := ssoAssignmentCR(func(s *awsv1alpha1.SSOAssignment) {
			s.Spec.PermissionSetRef = awsv1alpha1.PermissionSetRef{Name: "admin-access"}
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.SSOAssignment{}, &awsv1alpha1.PermissionSet{}).
			WithObjects(ps, sa).Build()
		f := &fakeSSOAssignment{}
		r := &SSOAssignmentReconciler{Client: c, Scheme: scheme, SSOAdminClient: f}

		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if res != requeueDependency {
			t.Errorf("result = %+v, want requeueDependency", res)
		}
		if f.createCalled {
			t.Error("CreateAccountAssignment must not be called while the permission set has no ARN")
		}
	})

	t.Run("delete calls DeleteAccountAssignment", func(t *testing.T) {
		ctx := context.Background()
		scheme := newOrgScheme(t)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.SSOAssignment{}).
			WithObjects(ssoAssignmentCR(func(s *awsv1alpha1.SSOAssignment) {
				s.Finalizers = []string{awsv1alpha1.FinalizerName}
				s.Status.PermissionSetArn = testPermissionSetARN
			})).Build()
		f := &fakeSSOAssignment{}
		r := &SSOAssignmentReconciler{Client: c, Scheme: scheme, SSOAdminClient: f}

		if err := c.Delete(ctx, ssoAssignmentCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled || f.deletedArn != testPermissionSetARN {
			t.Errorf("expected DeleteAccountAssignment(%s), got called=%v arn=%q", testPermissionSetARN, f.deleteCalled, f.deletedArn)
		}
		got := &awsv1alpha1.SSOAssignment{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected CR gone, got err=%v", err)
		}
	})

	t.Run("abandon annotation skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newOrgScheme(t)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.SSOAssignment{}).
			WithObjects(ssoAssignmentCR(func(s *awsv1alpha1.SSOAssignment) {
				s.Finalizers = []string{awsv1alpha1.FinalizerName}
				s.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
				s.Status.PermissionSetArn = testPermissionSetARN
			})).Build()
		f := &fakeSSOAssignment{}
		r := &SSOAssignmentReconciler{Client: c, Scheme: scheme, SSOAdminClient: f}

		if err := c.Delete(ctx, ssoAssignmentCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("DeleteAccountAssignment must not be called when abandoning")
		}
	})
}
