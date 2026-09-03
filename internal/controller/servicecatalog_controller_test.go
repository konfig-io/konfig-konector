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
	awssc "github.com/aws/aws-sdk-go-v2/service/servicecatalog"
	sctypes "github.com/aws/aws-sdk-go-v2/service/servicecatalog/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

const (
	testPortfolioID = "port-abcdefgh12345678"
	testProductID   = "prod-abcdefgh12345678"
)

// --- SCPortfolio ---

type fakeSCPortfolio struct {
	createCalled bool
	deleteCalled bool
	deletedID    string
}

func (f *fakeSCPortfolio) CreatePortfolio(_ context.Context, params *awssc.CreatePortfolioInput, _ ...func(*awssc.Options)) (*awssc.CreatePortfolioOutput, error) {
	f.createCalled = true
	return &awssc.CreatePortfolioOutput{
		PortfolioDetail: &sctypes.PortfolioDetail{
			Id:          aws.String(testPortfolioID),
			ARN:         aws.String("arn:aws:catalog:us-east-1:123456789012:portfolio/" + testPortfolioID),
			DisplayName: params.DisplayName,
		},
	}, nil
}

func (f *fakeSCPortfolio) UpdatePortfolio(_ context.Context, _ *awssc.UpdatePortfolioInput, _ ...func(*awssc.Options)) (*awssc.UpdatePortfolioOutput, error) {
	return &awssc.UpdatePortfolioOutput{}, nil
}

func (f *fakeSCPortfolio) DeletePortfolio(_ context.Context, params *awssc.DeletePortfolioInput, _ ...func(*awssc.Options)) (*awssc.DeletePortfolioOutput, error) {
	f.deleteCalled = true
	f.deletedID = aws.ToString(params.Id)
	return &awssc.DeletePortfolioOutput{}, nil
}

func (f *fakeSCPortfolio) DescribePortfolio(_ context.Context, params *awssc.DescribePortfolioInput, _ ...func(*awssc.Options)) (*awssc.DescribePortfolioOutput, error) {
	return &awssc.DescribePortfolioOutput{
		PortfolioDetail: &sctypes.PortfolioDetail{Id: params.Id},
	}, nil
}

func scPortfolioCR(mutate ...func(*awsv1alpha1.SCPortfolio)) *awsv1alpha1.SCPortfolio {
	pf := &awsv1alpha1.SCPortfolio{
		ObjectMeta: metav1.ObjectMeta{Name: "platform", Namespace: "default"},
		Spec: awsv1alpha1.SCPortfolioSpec{
			DisplayName:  "Platform",
			ProviderName: "platform-team",
		},
	}
	for _, m := range mutate {
		m(pf)
	}
	return pf
}

func TestSCPortfolioReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "platform", Namespace: "default"}}

	t.Run("create happy path persists portfolio ID and Ready", func(t *testing.T) {
		ctx := context.Background()
		scheme := newOrgScheme(t)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.SCPortfolio{}).
			WithObjects(scPortfolioCR()).Build()
		f := &fakeSCPortfolio{}
		r := &SCPortfolioReconciler{Client: c, Scheme: scheme, ServiceCatalogClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.SCPortfolio{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if !f.createCalled {
			t.Error("expected CreatePortfolio to be called")
		}
		if got.Status.PortfolioID != testPortfolioID {
			t.Errorf("status.portfolioId = %q, want %q", got.Status.PortfolioID, testPortfolioID)
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
			WithStatusSubresource(&awsv1alpha1.SCPortfolio{}).
			WithObjects(scPortfolioCR(func(pf *awsv1alpha1.SCPortfolio) {
				pf.Finalizers = []string{awsv1alpha1.FinalizerName}
				pf.Status.PortfolioID = testPortfolioID
			})).Build()
		f := &fakeSCPortfolio{}
		r := &SCPortfolioReconciler{Client: c, Scheme: scheme, ServiceCatalogClient: f}

		if err := c.Delete(ctx, scPortfolioCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled || f.deletedID != testPortfolioID {
			t.Errorf("expected DeletePortfolio(%s), got called=%v id=%q", testPortfolioID, f.deleteCalled, f.deletedID)
		}
		got := &awsv1alpha1.SCPortfolio{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected CR gone, got err=%v", err)
		}
	})

	t.Run("abandon annotation skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newOrgScheme(t)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.SCPortfolio{}).
			WithObjects(scPortfolioCR(func(pf *awsv1alpha1.SCPortfolio) {
				pf.Finalizers = []string{awsv1alpha1.FinalizerName}
				pf.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
				pf.Status.PortfolioID = testPortfolioID
			})).Build()
		f := &fakeSCPortfolio{}
		r := &SCPortfolioReconciler{Client: c, Scheme: scheme, ServiceCatalogClient: f}

		if err := c.Delete(ctx, scPortfolioCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("DeletePortfolio must not be called when abandoning")
		}
	})
}

// --- SCProduct ---

type fakeSCProduct struct {
	createCalled bool
	createInput  *awssc.CreateProductInput
	deleteCalled bool
	deletedID    string
}

func (f *fakeSCProduct) CreateProduct(_ context.Context, params *awssc.CreateProductInput, _ ...func(*awssc.Options)) (*awssc.CreateProductOutput, error) {
	f.createCalled = true
	f.createInput = params
	return &awssc.CreateProductOutput{
		ProductViewDetail: &sctypes.ProductViewDetail{
			ProductARN: aws.String("arn:aws:catalog:us-east-1:123456789012:product/" + testProductID),
			ProductViewSummary: &sctypes.ProductViewSummary{
				ProductId: aws.String(testProductID),
			},
		},
		ProvisioningArtifactDetail: &sctypes.ProvisioningArtifactDetail{
			Id: aws.String("pa-abcdefgh12345678"),
		},
	}, nil
}

func (f *fakeSCProduct) UpdateProduct(_ context.Context, _ *awssc.UpdateProductInput, _ ...func(*awssc.Options)) (*awssc.UpdateProductOutput, error) {
	return &awssc.UpdateProductOutput{}, nil
}

func (f *fakeSCProduct) DeleteProduct(_ context.Context, params *awssc.DeleteProductInput, _ ...func(*awssc.Options)) (*awssc.DeleteProductOutput, error) {
	f.deleteCalled = true
	f.deletedID = aws.ToString(params.Id)
	return &awssc.DeleteProductOutput{}, nil
}

func (f *fakeSCProduct) DescribeProductAsAdmin(_ context.Context, params *awssc.DescribeProductAsAdminInput, _ ...func(*awssc.Options)) (*awssc.DescribeProductAsAdminOutput, error) {
	return &awssc.DescribeProductAsAdminOutput{
		ProductViewDetail: &sctypes.ProductViewDetail{
			ProductViewSummary: &sctypes.ProductViewSummary{ProductId: params.Id},
		},
	}, nil
}

func scProductCR(mutate ...func(*awsv1alpha1.SCProduct)) *awsv1alpha1.SCProduct {
	prod := &awsv1alpha1.SCProduct{
		ObjectMeta: metav1.ObjectMeta{Name: "vpc-product", Namespace: "default"},
		Spec: awsv1alpha1.SCProductSpec{
			Name:  "vpc-product",
			Owner: "platform-team",
			ProvisioningArtifact: awsv1alpha1.SCProvisioningArtifact{
				Name:        "v1",
				TemplateURL: "https://s3.amazonaws.com/templates/vpc.yaml",
			},
		},
	}
	for _, m := range mutate {
		m(prod)
	}
	return prod
}

func TestSCProductReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "vpc-product", Namespace: "default"}}

	t.Run("create happy path persists product ID, template URL passed via LoadTemplateFromURL", func(t *testing.T) {
		ctx := context.Background()
		scheme := newOrgScheme(t)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.SCProduct{}).
			WithObjects(scProductCR()).Build()
		f := &fakeSCProduct{}
		r := &SCProductReconciler{Client: c, Scheme: scheme, ServiceCatalogClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.SCProduct{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if !f.createCalled {
			t.Error("expected CreateProduct to be called")
		}
		if got.Status.ProductID != testProductID {
			t.Errorf("status.productId = %q, want %q", got.Status.ProductID, testProductID)
		}
		if f.createInput.ProvisioningArtifactParameters == nil ||
			f.createInput.ProvisioningArtifactParameters.Info["LoadTemplateFromURL"] != "https://s3.amazonaws.com/templates/vpc.yaml" {
			t.Errorf("expected LoadTemplateFromURL in provisioning artifact info, got %+v", f.createInput.ProvisioningArtifactParameters)
		}
		if f.createInput.ProductType != sctypes.ProductTypeCloudFormationTemplate {
			t.Errorf("productType = %q, want CLOUD_FORMATION_TEMPLATE", f.createInput.ProductType)
		}
	})

	t.Run("delete with finalizer calls AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newOrgScheme(t)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.SCProduct{}).
			WithObjects(scProductCR(func(p *awsv1alpha1.SCProduct) {
				p.Finalizers = []string{awsv1alpha1.FinalizerName}
				p.Status.ProductID = testProductID
			})).Build()
		f := &fakeSCProduct{}
		r := &SCProductReconciler{Client: c, Scheme: scheme, ServiceCatalogClient: f}

		if err := c.Delete(ctx, scProductCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled || f.deletedID != testProductID {
			t.Errorf("expected DeleteProduct(%s), got called=%v id=%q", testProductID, f.deleteCalled, f.deletedID)
		}
	})

	t.Run("abandon annotation skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newOrgScheme(t)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.SCProduct{}).
			WithObjects(scProductCR(func(p *awsv1alpha1.SCProduct) {
				p.Finalizers = []string{awsv1alpha1.FinalizerName}
				p.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
				p.Status.ProductID = testProductID
			})).Build()
		f := &fakeSCProduct{}
		r := &SCProductReconciler{Client: c, Scheme: scheme, ServiceCatalogClient: f}

		if err := c.Delete(ctx, scProductCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("DeleteProduct must not be called when abandoning")
		}
	})
}

// --- SCPortfolioProductAssociation ---

type fakeSCAssociation struct {
	associateCalled    bool
	disassociateCalled bool
}

func (f *fakeSCAssociation) AssociateProductWithPortfolio(_ context.Context, _ *awssc.AssociateProductWithPortfolioInput, _ ...func(*awssc.Options)) (*awssc.AssociateProductWithPortfolioOutput, error) {
	f.associateCalled = true
	return &awssc.AssociateProductWithPortfolioOutput{}, nil
}

func (f *fakeSCAssociation) DisassociateProductFromPortfolio(_ context.Context, _ *awssc.DisassociateProductFromPortfolioInput, _ ...func(*awssc.Options)) (*awssc.DisassociateProductFromPortfolioOutput, error) {
	f.disassociateCalled = true
	return &awssc.DisassociateProductFromPortfolioOutput{}, nil
}

func scAssociationCR(mutate ...func(*awsv1alpha1.SCPortfolioProductAssociation)) *awsv1alpha1.SCPortfolioProductAssociation {
	assoc := &awsv1alpha1.SCPortfolioProductAssociation{
		ObjectMeta: metav1.ObjectMeta{Name: "vpc-in-platform", Namespace: "default"},
		Spec: awsv1alpha1.SCPortfolioProductAssociationSpec{
			ProductRef:   awsv1alpha1.SCProductRef{ProductID: testProductID},
			PortfolioRef: awsv1alpha1.SCPortfolioRef{PortfolioID: testPortfolioID},
		},
	}
	for _, m := range mutate {
		m(assoc)
	}
	return assoc
}

func TestSCPortfolioProductAssociationReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "vpc-in-platform", Namespace: "default"}}

	t.Run("create associates and persists identifiers", func(t *testing.T) {
		ctx := context.Background()
		scheme := newOrgScheme(t)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.SCPortfolioProductAssociation{}).
			WithObjects(scAssociationCR()).Build()
		f := &fakeSCAssociation{}
		r := &SCPortfolioProductAssociationReconciler{Client: c, Scheme: scheme, ServiceCatalogClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.SCPortfolioProductAssociation{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if !f.associateCalled {
			t.Error("expected AssociateProductWithPortfolio to be called")
		}
		if got.Status.ProductID != testProductID || got.Status.PortfolioID != testPortfolioID {
			t.Errorf("status = %q/%q, want %s/%s", got.Status.ProductID, got.Status.PortfolioID, testProductID, testPortfolioID)
		}
	})

	t.Run("delete disassociates", func(t *testing.T) {
		ctx := context.Background()
		scheme := newOrgScheme(t)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.SCPortfolioProductAssociation{}).
			WithObjects(scAssociationCR(func(a *awsv1alpha1.SCPortfolioProductAssociation) {
				a.Finalizers = []string{awsv1alpha1.FinalizerName}
				a.Status.ProductID = testProductID
				a.Status.PortfolioID = testPortfolioID
			})).Build()
		f := &fakeSCAssociation{}
		r := &SCPortfolioProductAssociationReconciler{Client: c, Scheme: scheme, ServiceCatalogClient: f}

		if err := c.Delete(ctx, scAssociationCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.disassociateCalled {
			t.Error("expected DisassociateProductFromPortfolio to be called")
		}
	})

	t.Run("abandon annotation skips disassociate", func(t *testing.T) {
		ctx := context.Background()
		scheme := newOrgScheme(t)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.SCPortfolioProductAssociation{}).
			WithObjects(scAssociationCR(func(a *awsv1alpha1.SCPortfolioProductAssociation) {
				a.Finalizers = []string{awsv1alpha1.FinalizerName}
				a.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
				a.Status.ProductID = testProductID
				a.Status.PortfolioID = testPortfolioID
			})).Build()
		f := &fakeSCAssociation{}
		r := &SCPortfolioProductAssociationReconciler{Client: c, Scheme: scheme, ServiceCatalogClient: f}

		if err := c.Delete(ctx, scAssociationCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.disassociateCalled {
			t.Error("DisassociateProductFromPortfolio must not be called when abandoning")
		}
	})
}
