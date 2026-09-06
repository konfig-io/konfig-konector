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
	orgtypes "github.com/aws/aws-sdk-go-v2/service/organizations/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

type fakeOrgAccount struct {
	createCalled bool
	closeCalled  bool
	closedID     string
	statusState  orgtypes.CreateAccountState
}

func (f *fakeOrgAccount) CreateAccount(_ context.Context, _ *awsorgs.CreateAccountInput, _ ...func(*awsorgs.Options)) (*awsorgs.CreateAccountOutput, error) {
	f.createCalled = true
	return &awsorgs.CreateAccountOutput{
		CreateAccountStatus: &orgtypes.CreateAccountStatus{
			Id:    aws.String("car-1234567890abcdef"),
			State: orgtypes.CreateAccountStateInProgress,
		},
	}, nil
}

func (f *fakeOrgAccount) DescribeCreateAccountStatus(_ context.Context, _ *awsorgs.DescribeCreateAccountStatusInput, _ ...func(*awsorgs.Options)) (*awsorgs.DescribeCreateAccountStatusOutput, error) {
	state := f.statusState
	if state == "" {
		state = orgtypes.CreateAccountStateInProgress
	}
	return &awsorgs.DescribeCreateAccountStatusOutput{
		CreateAccountStatus: &orgtypes.CreateAccountStatus{
			Id:        aws.String("car-1234567890abcdef"),
			State:     state,
			AccountId: aws.String("111122223333"),
		},
	}, nil
}

func (f *fakeOrgAccount) DescribeAccount(_ context.Context, params *awsorgs.DescribeAccountInput, _ ...func(*awsorgs.Options)) (*awsorgs.DescribeAccountOutput, error) {
	return &awsorgs.DescribeAccountOutput{
		Account: &orgtypes.Account{
			Id:  params.AccountId,
			Arn: aws.String("arn:aws:organizations::123456789012:account/o-example/111122223333"),
		},
	}, nil
}

func (f *fakeOrgAccount) MoveAccount(_ context.Context, _ *awsorgs.MoveAccountInput, _ ...func(*awsorgs.Options)) (*awsorgs.MoveAccountOutput, error) {
	return &awsorgs.MoveAccountOutput{}, nil
}

func (f *fakeOrgAccount) ListParents(_ context.Context, _ *awsorgs.ListParentsInput, _ ...func(*awsorgs.Options)) (*awsorgs.ListParentsOutput, error) {
	return &awsorgs.ListParentsOutput{Parents: []orgtypes.Parent{{Id: aws.String("r-abcd"), Type: orgtypes.ParentTypeRoot}}}, nil
}

func (f *fakeOrgAccount) CloseAccount(_ context.Context, params *awsorgs.CloseAccountInput, _ ...func(*awsorgs.Options)) (*awsorgs.CloseAccountOutput, error) {
	f.closeCalled = true
	f.closedID = aws.ToString(params.AccountId)
	return &awsorgs.CloseAccountOutput{}, nil
}

func (f *fakeOrgAccount) TagResource(_ context.Context, _ *awsorgs.TagResourceInput, _ ...func(*awsorgs.Options)) (*awsorgs.TagResourceOutput, error) {
	return &awsorgs.TagResourceOutput{}, nil
}

func orgAccountCR(mutate ...func(*awsv1alpha1.OrganizationsAccount)) *awsv1alpha1.OrganizationsAccount {
	acct := &awsv1alpha1.OrganizationsAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "sandbox", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec: awsv1alpha1.OrganizationsAccountSpec{
			Email:       "sandbox@example.com",
			AccountName: "sandbox",
		},
	}
	for _, m := range mutate {
		m(acct)
	}
	return acct
}

func TestOrganizationsAccountReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "sandbox", Namespace: "default"}}

	t.Run("create kicks off async creation and persists request ID", func(t *testing.T) {
		ctx := context.Background()
		scheme := newOrgScheme(t)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.OrganizationsAccount{}).
			WithObjects(orgAccountCR()).Build()
		f := &fakeOrgAccount{}
		r := &OrganizationsAccountReconciler{Client: c, Scheme: scheme, OrganizationsClient: f}

		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if res != requeueOrgPolling {
			t.Errorf("result = %+v, want requeueOrgPolling", res)
		}
		got := &awsv1alpha1.OrganizationsAccount{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if !f.createCalled {
			t.Error("expected CreateAccount to be called")
		}
		if got.Status.CreateAccountRequestID != "car-1234567890abcdef" {
			t.Errorf("status.createAccountRequestId = %q, want car-1234567890abcdef", got.Status.CreateAccountRequestID)
		}
		if got.Status.State != "IN_PROGRESS" {
			t.Errorf("status.state = %q, want IN_PROGRESS", got.Status.State)
		}
	})

	t.Run("poll success stores account ID and reaches Ready", func(t *testing.T) {
		ctx := context.Background()
		scheme := newOrgScheme(t)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.OrganizationsAccount{}).
			WithObjects(orgAccountCR(func(a *awsv1alpha1.OrganizationsAccount) {
				a.Finalizers = []string{awsv1alpha1.FinalizerName}
				a.Status.CreateAccountRequestID = "car-1234567890abcdef"
				a.Status.State = "IN_PROGRESS"
			})).Build()
		f := &fakeOrgAccount{statusState: orgtypes.CreateAccountStateSucceeded}
		r := &OrganizationsAccountReconciler{Client: c, Scheme: scheme, OrganizationsClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.OrganizationsAccount{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.AccountID != "111122223333" {
			t.Errorf("status.accountId = %q, want 111122223333", got.Status.AccountID)
		}
		if f.createCalled {
			t.Error("CreateAccount must not be called while a request is in flight")
		}
	})

	t.Run("delete with closeOnDelete=true calls CloseAccount", func(t *testing.T) {
		ctx := context.Background()
		scheme := newOrgScheme(t)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.OrganizationsAccount{}).
			WithObjects(orgAccountCR(func(a *awsv1alpha1.OrganizationsAccount) {
				a.Finalizers = []string{awsv1alpha1.FinalizerName}
				a.Spec.CloseOnDelete = true
				a.Status.AccountID = "111122223333"
			})).Build()
		f := &fakeOrgAccount{}
		r := &OrganizationsAccountReconciler{Client: c, Scheme: scheme, OrganizationsClient: f}

		if err := c.Delete(ctx, orgAccountCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.closeCalled || f.closedID != "111122223333" {
			t.Errorf("expected CloseAccount(111122223333), got called=%v id=%q", f.closeCalled, f.closedID)
		}
		got := &awsv1alpha1.OrganizationsAccount{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected CR gone, got err=%v", err)
		}
	})

	t.Run("delete with closeOnDelete=false abandons the account", func(t *testing.T) {
		ctx := context.Background()
		scheme := newOrgScheme(t)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.OrganizationsAccount{}).
			WithObjects(orgAccountCR(func(a *awsv1alpha1.OrganizationsAccount) {
				a.Finalizers = []string{awsv1alpha1.FinalizerName}
				a.Status.AccountID = "111122223333"
			})).Build()
		f := &fakeOrgAccount{}
		r := &OrganizationsAccountReconciler{Client: c, Scheme: scheme, OrganizationsClient: f}

		if err := c.Delete(ctx, orgAccountCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.closeCalled {
			t.Error("CloseAccount must not be called when closeOnDelete is false")
		}
		got := &awsv1alpha1.OrganizationsAccount{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected CR gone, got err=%v", err)
		}
	})

	t.Run("abandon annotation skips CloseAccount even with closeOnDelete=true", func(t *testing.T) {
		ctx := context.Background()
		scheme := newOrgScheme(t)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.OrganizationsAccount{}).
			WithObjects(orgAccountCR(func(a *awsv1alpha1.OrganizationsAccount) {
				a.Finalizers = []string{awsv1alpha1.FinalizerName}
				a.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
				a.Spec.CloseOnDelete = true
				a.Status.AccountID = "111122223333"
			})).Build()
		f := &fakeOrgAccount{}
		r := &OrganizationsAccountReconciler{Client: c, Scheme: scheme, OrganizationsClient: f}

		if err := c.Delete(ctx, orgAccountCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.closeCalled {
			t.Error("CloseAccount must not be called when abandoning")
		}
	})
}
