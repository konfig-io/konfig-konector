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
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsorgs "github.com/aws/aws-sdk-go-v2/service/organizations"
	orgtypes "github.com/aws/aws-sdk-go-v2/service/organizations/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

type fakeOrgOU struct {
	createCalled bool
	deleteCalled bool
	deletedID    string
	failCreate   bool
}

func (f *fakeOrgOU) CreateOrganizationalUnit(_ context.Context, params *awsorgs.CreateOrganizationalUnitInput, _ ...func(*awsorgs.Options)) (*awsorgs.CreateOrganizationalUnitOutput, error) {
	f.createCalled = true
	if f.failCreate {
		return nil, fmt.Errorf("access denied")
	}
	return &awsorgs.CreateOrganizationalUnitOutput{
		OrganizationalUnit: &orgtypes.OrganizationalUnit{
			Id:   aws.String("ou-abcd-11111111"),
			Arn:  aws.String("arn:aws:organizations::123456789012:ou/o-example/ou-abcd-11111111"),
			Name: params.Name,
		},
	}, nil
}

func (f *fakeOrgOU) UpdateOrganizationalUnit(_ context.Context, _ *awsorgs.UpdateOrganizationalUnitInput, _ ...func(*awsorgs.Options)) (*awsorgs.UpdateOrganizationalUnitOutput, error) {
	return &awsorgs.UpdateOrganizationalUnitOutput{}, nil
}

func (f *fakeOrgOU) DeleteOrganizationalUnit(_ context.Context, params *awsorgs.DeleteOrganizationalUnitInput, _ ...func(*awsorgs.Options)) (*awsorgs.DeleteOrganizationalUnitOutput, error) {
	f.deleteCalled = true
	f.deletedID = aws.ToString(params.OrganizationalUnitId)
	return &awsorgs.DeleteOrganizationalUnitOutput{}, nil
}

func (f *fakeOrgOU) DescribeOrganizationalUnit(_ context.Context, params *awsorgs.DescribeOrganizationalUnitInput, _ ...func(*awsorgs.Options)) (*awsorgs.DescribeOrganizationalUnitOutput, error) {
	return &awsorgs.DescribeOrganizationalUnitOutput{
		OrganizationalUnit: &orgtypes.OrganizationalUnit{Id: params.OrganizationalUnitId},
	}, nil
}

func (f *fakeOrgOU) TagResource(_ context.Context, _ *awsorgs.TagResourceInput, _ ...func(*awsorgs.Options)) (*awsorgs.TagResourceOutput, error) {
	return &awsorgs.TagResourceOutput{}, nil
}

func orgOUCR(mutate ...func(*awsv1alpha1.OrganizationsOU)) *awsv1alpha1.OrganizationsOU {
	ou := &awsv1alpha1.OrganizationsOU{
		ObjectMeta: metav1.ObjectMeta{Name: "workloads", Namespace: "default"},
		Spec: awsv1alpha1.OrganizationsOUSpec{
			Name:     "workloads",
			ParentID: "r-abcd",
		},
	}
	for _, m := range mutate {
		m(ou)
	}
	return ou
}

func TestOrganizationsOUReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "workloads", Namespace: "default"}}

	t.Run("create happy path persists OU ID and Ready", func(t *testing.T) {
		ctx := context.Background()
		scheme := newOrgScheme(t)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.OrganizationsOU{}).
			WithObjects(orgOUCR()).Build()
		f := &fakeOrgOU{}
		r := &OrganizationsOUReconciler{Client: c, Scheme: scheme, OrganizationsClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.OrganizationsOU{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if !f.createCalled {
			t.Error("expected CreateOrganizationalUnit to be called")
		}
		if got.Status.OUID != "ou-abcd-11111111" {
			t.Errorf("status.ouId = %q, want ou-abcd-11111111", got.Status.OUID)
		}
		cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
		if cond == nil || cond.Status != metav1.ConditionTrue {
			t.Errorf("Ready condition = %+v, want True", cond)
		}
	})

	t.Run("parentRef to a not-ready OU requeues as dependency", func(t *testing.T) {
		ctx := context.Background()
		scheme := newOrgScheme(t)
		parent := &awsv1alpha1.OrganizationsOU{
			ObjectMeta: metav1.ObjectMeta{Name: "parent-ou", Namespace: "default"},
			Spec:       awsv1alpha1.OrganizationsOUSpec{Name: "parent", ParentID: "r-abcd"},
		}
		child := orgOUCR(func(ou *awsv1alpha1.OrganizationsOU) {
			ou.Spec.ParentID = ""
			ou.Spec.ParentRef = &awsv1alpha1.ResourceRef{Name: "parent-ou"}
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.OrganizationsOU{}).
			WithObjects(parent, child).Build()
		f := &fakeOrgOU{}
		r := &OrganizationsOUReconciler{Client: c, Scheme: scheme, OrganizationsClient: f}

		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if res != requeueDependency {
			t.Errorf("result = %+v, want requeueDependency", res)
		}
		if f.createCalled {
			t.Error("CreateOrganizationalUnit must not be called while the parent has no OU ID")
		}
	})

	t.Run("delete with finalizer calls AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newOrgScheme(t)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.OrganizationsOU{}).
			WithObjects(orgOUCR(func(ou *awsv1alpha1.OrganizationsOU) {
				ou.Finalizers = []string{awsv1alpha1.FinalizerName}
				ou.Status.OUID = "ou-abcd-11111111"
			})).Build()
		f := &fakeOrgOU{}
		r := &OrganizationsOUReconciler{Client: c, Scheme: scheme, OrganizationsClient: f}

		if err := c.Delete(ctx, orgOUCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled || f.deletedID != "ou-abcd-11111111" {
			t.Errorf("expected DeleteOrganizationalUnit(ou-abcd-11111111), got called=%v id=%q", f.deleteCalled, f.deletedID)
		}
		got := &awsv1alpha1.OrganizationsOU{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected CR gone, got err=%v", err)
		}
	})

	t.Run("abandon annotation skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newOrgScheme(t)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.OrganizationsOU{}).
			WithObjects(orgOUCR(func(ou *awsv1alpha1.OrganizationsOU) {
				ou.Finalizers = []string{awsv1alpha1.FinalizerName}
				ou.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
				ou.Status.OUID = "ou-abcd-11111111"
			})).Build()
		f := &fakeOrgOU{}
		r := &OrganizationsOUReconciler{Client: c, Scheme: scheme, OrganizationsClient: f}

		if err := c.Delete(ctx, orgOUCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("DeleteOrganizationalUnit must not be called when abandoning")
		}
	})
}
