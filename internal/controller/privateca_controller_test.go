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
	awsacmpca "github.com/aws/aws-sdk-go-v2/service/acmpca"
	acmpcatypes "github.com/aws/aws-sdk-go-v2/service/acmpca/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

const testPrivateCAARN = "arn:aws:acm-pca:us-east-1:123456789012:certificate-authority/12345678-1234-1234-1234-123456789012"

type fakePrivateCAAPI struct {
	create func(ctx context.Context, params *awsacmpca.CreateCertificateAuthorityInput) (*awsacmpca.CreateCertificateAuthorityOutput, error)

	createCalled bool
	updateCalled bool
	deleteCalled bool
	deleteInput  *awsacmpca.DeleteCertificateAuthorityInput
	updateInput  *awsacmpca.UpdateCertificateAuthorityInput
}

func (f *fakePrivateCAAPI) CreateCertificateAuthority(ctx context.Context, params *awsacmpca.CreateCertificateAuthorityInput, _ ...func(*awsacmpca.Options)) (*awsacmpca.CreateCertificateAuthorityOutput, error) {
	f.createCalled = true
	if f.create == nil {
		return nil, fmt.Errorf("unexpected call to CreateCertificateAuthority")
	}
	return f.create(ctx, params)
}

func (f *fakePrivateCAAPI) DescribeCertificateAuthority(_ context.Context, _ *awsacmpca.DescribeCertificateAuthorityInput, _ ...func(*awsacmpca.Options)) (*awsacmpca.DescribeCertificateAuthorityOutput, error) {
	return nil, fmt.Errorf("unexpected call to DescribeCertificateAuthority")
}

func (f *fakePrivateCAAPI) UpdateCertificateAuthority(_ context.Context, params *awsacmpca.UpdateCertificateAuthorityInput, _ ...func(*awsacmpca.Options)) (*awsacmpca.UpdateCertificateAuthorityOutput, error) {
	f.updateCalled = true
	f.updateInput = params
	return &awsacmpca.UpdateCertificateAuthorityOutput{}, nil
}

func (f *fakePrivateCAAPI) DeleteCertificateAuthority(_ context.Context, params *awsacmpca.DeleteCertificateAuthorityInput, _ ...func(*awsacmpca.Options)) (*awsacmpca.DeleteCertificateAuthorityOutput, error) {
	f.deleteCalled = true
	f.deleteInput = params
	return &awsacmpca.DeleteCertificateAuthorityOutput{}, nil
}

func privateCACR(mutate ...func(*awsv1alpha1.PrivateCA)) *awsv1alpha1.PrivateCA {
	ca := &awsv1alpha1.PrivateCA{
		ObjectMeta: metav1.ObjectMeta{Name: "my-ca", Namespace: "default"},
		Spec: awsv1alpha1.PrivateCASpec{
			Type:             "ROOT",
			KeyAlgorithm:     "RSA_2048",
			SigningAlgorithm: "SHA256WITHRSA",
			Subject: awsv1alpha1.PrivateCASubject{
				CommonName:   "ca.example.com",
				Organization: "Example Corp",
				Country:      "US",
			},
			PermanentDeletionTimeInDays: 7,
		},
	}
	for _, m := range mutate {
		m(ca)
	}
	return ca
}

func TestPrivateCAReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-ca", Namespace: "default"}}
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}

	t.Run("create persists ARN and polls", func(t *testing.T) {
		ctx := context.Background()
		f := &fakePrivateCAAPI{
			create: func(_ context.Context, params *awsacmpca.CreateCertificateAuthorityInput) (*awsacmpca.CreateCertificateAuthorityOutput, error) {
				if params.CertificateAuthorityType != acmpcatypes.CertificateAuthorityTypeRoot {
					return nil, fmt.Errorf("unexpected type %q", params.CertificateAuthorityType)
				}
				cfg := params.CertificateAuthorityConfiguration
				if cfg == nil || aws.ToString(cfg.Subject.CommonName) != "ca.example.com" {
					return nil, fmt.Errorf("unexpected subject %+v", cfg)
				}
				return &awsacmpca.CreateCertificateAuthorityOutput{
					CertificateAuthorityArn: aws.String(testPrivateCAARN),
				}, nil
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.PrivateCA{}).
			WithObjects(privateCACR()).Build()
		r := &PrivateCAReconciler{Client: c, Scheme: scheme, ACMPCAClient: f}

		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.createCalled {
			t.Fatal("expected CreateCertificateAuthority to be called")
		}
		if res.RequeueAfter == 0 {
			t.Error("expected polling requeue after create")
		}
		got := &awsv1alpha1.PrivateCA{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ARN != testPrivateCAARN {
			t.Errorf("status.arn = %q, want %q (must persist right after create)", got.Status.ARN, testPrivateCAARN)
		}
	})

	t.Run("delete disables ACTIVE CA then deletes with spec deletion window", func(t *testing.T) {
		ctx := context.Background()
		f := &fakePrivateCAAPI{}
		now := metav1.Now()
		ca := privateCACR(func(ca *awsv1alpha1.PrivateCA) {
			ca.Finalizers = []string{awsv1alpha1.FinalizerName}
			ca.DeletionTimestamp = &now
			ca.Status.ARN = testPrivateCAARN
			ca.Status.Status = "ACTIVE"
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.PrivateCA{}).
			WithObjects(ca).Build()
		r := &PrivateCAReconciler{Client: c, Scheme: scheme, ACMPCAClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.updateCalled || f.updateInput.Status != acmpcatypes.CertificateAuthorityStatusDisabled {
			t.Error("expected ACTIVE CA to be DISABLED before delete")
		}
		if !f.deleteCalled {
			t.Fatal("expected DeleteCertificateAuthority to be called")
		}
		if aws.ToInt32(f.deleteInput.PermanentDeletionTimeInDays) != 7 {
			t.Errorf("permanentDeletionTimeInDays = %d, want 7", aws.ToInt32(f.deleteInput.PermanentDeletionTimeInDays))
		}
		got := &awsv1alpha1.PrivateCA{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected object gone after finalizer removal, got err=%v", err)
		}
	})

	t.Run("abandon annotation skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		f := &fakePrivateCAAPI{}
		now := metav1.Now()
		ca := privateCACR(func(ca *awsv1alpha1.PrivateCA) {
			ca.Finalizers = []string{awsv1alpha1.FinalizerName}
			ca.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			ca.DeletionTimestamp = &now
			ca.Status.ARN = testPrivateCAARN
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.PrivateCA{}).
			WithObjects(ca).Build()
		r := &PrivateCAReconciler{Client: c, Scheme: scheme, ACMPCAClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled || f.updateCalled {
			t.Error("expected no AWS calls for abandoned resource")
		}
	})
}
