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
	awsram "github.com/aws/aws-sdk-go-v2/service/ram"
	ramtypes "github.com/aws/aws-sdk-go-v2/service/ram/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

const testShareARN = "arn:aws:ram:us-east-1:123456789012:resource-share/12345678-1234-1234-1234-123456789012"

type fakeRAM struct {
	createCalled bool
	deleteCalled bool
	deletedArn   string
}

func (f *fakeRAM) CreateResourceShare(_ context.Context, params *awsram.CreateResourceShareInput, _ ...func(*awsram.Options)) (*awsram.CreateResourceShareOutput, error) {
	f.createCalled = true
	return &awsram.CreateResourceShareOutput{
		ResourceShare: &ramtypes.ResourceShare{
			ResourceShareArn: aws.String(testShareARN),
			Name:             params.Name,
		},
	}, nil
}

func (f *fakeRAM) UpdateResourceShare(_ context.Context, _ *awsram.UpdateResourceShareInput, _ ...func(*awsram.Options)) (*awsram.UpdateResourceShareOutput, error) {
	return &awsram.UpdateResourceShareOutput{}, nil
}

func (f *fakeRAM) DeleteResourceShare(_ context.Context, params *awsram.DeleteResourceShareInput, _ ...func(*awsram.Options)) (*awsram.DeleteResourceShareOutput, error) {
	f.deleteCalled = true
	f.deletedArn = aws.ToString(params.ResourceShareArn)
	return &awsram.DeleteResourceShareOutput{}, nil
}

func (f *fakeRAM) GetResourceShares(_ context.Context, _ *awsram.GetResourceSharesInput, _ ...func(*awsram.Options)) (*awsram.GetResourceSharesOutput, error) {
	return &awsram.GetResourceSharesOutput{}, nil
}

func (f *fakeRAM) GetResourceShareAssociations(_ context.Context, _ *awsram.GetResourceShareAssociationsInput, _ ...func(*awsram.Options)) (*awsram.GetResourceShareAssociationsOutput, error) {
	return &awsram.GetResourceShareAssociationsOutput{}, nil
}

func (f *fakeRAM) AssociateResourceShare(_ context.Context, _ *awsram.AssociateResourceShareInput, _ ...func(*awsram.Options)) (*awsram.AssociateResourceShareOutput, error) {
	return &awsram.AssociateResourceShareOutput{}, nil
}

func (f *fakeRAM) DisassociateResourceShare(_ context.Context, _ *awsram.DisassociateResourceShareInput, _ ...func(*awsram.Options)) (*awsram.DisassociateResourceShareOutput, error) {
	return &awsram.DisassociateResourceShareOutput{}, nil
}

func (f *fakeRAM) TagResource(_ context.Context, _ *awsram.TagResourceInput, _ ...func(*awsram.Options)) (*awsram.TagResourceOutput, error) {
	return &awsram.TagResourceOutput{}, nil
}

func resourceShareCR(mutate ...func(*awsv1alpha1.ResourceShare)) *awsv1alpha1.ResourceShare {
	rs := &awsv1alpha1.ResourceShare{
		ObjectMeta: metav1.ObjectMeta{Name: "tgw-share", Namespace: "default"},
		Spec: awsv1alpha1.ResourceShareSpec{
			Name:         "tgw-share",
			ResourceArns: []string{"arn:aws:ec2:us-east-1:123456789012:transit-gateway/tgw-123"},
			Principals:   []string{"111122223333"},
		},
	}
	for _, m := range mutate {
		m(rs)
	}
	return rs
}

func TestResourceShareReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "tgw-share", Namespace: "default"}}

	t.Run("create happy path persists ARN and Ready", func(t *testing.T) {
		ctx := context.Background()
		scheme := newOrgScheme(t)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.ResourceShare{}).
			WithObjects(resourceShareCR()).Build()
		f := &fakeRAM{}
		r := &ResourceShareReconciler{Client: c, Scheme: scheme, RAMClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.ResourceShare{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if !f.createCalled {
			t.Error("expected CreateResourceShare to be called")
		}
		if got.Status.ARN != testShareARN {
			t.Errorf("status.arn = %q, want %q", got.Status.ARN, testShareARN)
		}
		cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
		if cond == nil || cond.Status != metav1.ConditionTrue {
			t.Errorf("Ready condition = %+v, want True", cond)
		}
	})

	t.Run("delete with finalizer calls AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newOrgScheme(t)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.ResourceShare{}).
			WithObjects(resourceShareCR(func(rs *awsv1alpha1.ResourceShare) {
				rs.Finalizers = []string{awsv1alpha1.FinalizerName}
				rs.Status.ARN = testShareARN
			})).Build()
		f := &fakeRAM{}
		r := &ResourceShareReconciler{Client: c, Scheme: scheme, RAMClient: f}

		if err := c.Delete(ctx, resourceShareCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled || f.deletedArn != testShareARN {
			t.Errorf("expected DeleteResourceShare(%s), got called=%v arn=%q", testShareARN, f.deleteCalled, f.deletedArn)
		}
		got := &awsv1alpha1.ResourceShare{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected CR gone, got err=%v", err)
		}
	})

	t.Run("abandon annotation skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newOrgScheme(t)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.ResourceShare{}).
			WithObjects(resourceShareCR(func(rs *awsv1alpha1.ResourceShare) {
				rs.Finalizers = []string{awsv1alpha1.FinalizerName}
				rs.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
				rs.Status.ARN = testShareARN
			})).Build()
		f := &fakeRAM{}
		r := &ResourceShareReconciler{Client: c, Scheme: scheme, RAMClient: f}

		if err := c.Delete(ctx, resourceShareCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("DeleteResourceShare must not be called when abandoning")
		}
	})
}
