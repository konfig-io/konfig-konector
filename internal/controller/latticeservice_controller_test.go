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
	awslattice "github.com/aws/aws-sdk-go-v2/service/vpclattice"
	latticetypes "github.com/aws/aws-sdk-go-v2/service/vpclattice/types"
	"github.com/aws/smithy-go"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

func latticeNotFoundErr() error {
	return &smithy.GenericAPIError{Code: "ResourceNotFoundException", Message: "not found"}
}

// fakeLattice implements the VPC Lattice controller interfaces.
type fakeLattice struct {
	getService    func(*awslattice.GetServiceInput) (*awslattice.GetServiceOutput, error)
	createService func(*awslattice.CreateServiceInput) (*awslattice.CreateServiceOutput, error)
	updateService func(*awslattice.UpdateServiceInput) (*awslattice.UpdateServiceOutput, error)
	deleteService func(*awslattice.DeleteServiceInput) (*awslattice.DeleteServiceOutput, error)

	getServiceNetwork    func(*awslattice.GetServiceNetworkInput) (*awslattice.GetServiceNetworkOutput, error)
	createServiceNetwork func(*awslattice.CreateServiceNetworkInput) (*awslattice.CreateServiceNetworkOutput, error)
	updateServiceNetwork func(*awslattice.UpdateServiceNetworkInput) (*awslattice.UpdateServiceNetworkOutput, error)
	deleteServiceNetwork func(*awslattice.DeleteServiceNetworkInput) (*awslattice.DeleteServiceNetworkOutput, error)

	getSNVpcAssoc    func(*awslattice.GetServiceNetworkVpcAssociationInput) (*awslattice.GetServiceNetworkVpcAssociationOutput, error)
	createSNVpcAssoc func(*awslattice.CreateServiceNetworkVpcAssociationInput) (*awslattice.CreateServiceNetworkVpcAssociationOutput, error)
	deleteSNVpcAssoc func(*awslattice.DeleteServiceNetworkVpcAssociationInput) (*awslattice.DeleteServiceNetworkVpcAssociationOutput, error)

	getSNSvcAssoc    func(*awslattice.GetServiceNetworkServiceAssociationInput) (*awslattice.GetServiceNetworkServiceAssociationOutput, error)
	createSNSvcAssoc func(*awslattice.CreateServiceNetworkServiceAssociationInput) (*awslattice.CreateServiceNetworkServiceAssociationOutput, error)
	deleteSNSvcAssoc func(*awslattice.DeleteServiceNetworkServiceAssociationInput) (*awslattice.DeleteServiceNetworkServiceAssociationOutput, error)

	getTargetGroup    func(*awslattice.GetTargetGroupInput) (*awslattice.GetTargetGroupOutput, error)
	createTargetGroup func(*awslattice.CreateTargetGroupInput) (*awslattice.CreateTargetGroupOutput, error)
	deleteTargetGroup func(*awslattice.DeleteTargetGroupInput) (*awslattice.DeleteTargetGroupOutput, error)

	getListener    func(*awslattice.GetListenerInput) (*awslattice.GetListenerOutput, error)
	createListener func(*awslattice.CreateListenerInput) (*awslattice.CreateListenerOutput, error)
	updateListener func(*awslattice.UpdateListenerInput) (*awslattice.UpdateListenerOutput, error)
	deleteListener func(*awslattice.DeleteListenerInput) (*awslattice.DeleteListenerOutput, error)

	tagResource func(*awslattice.TagResourceInput) (*awslattice.TagResourceOutput, error)

	createServiceCalled bool
	deleteServiceCalled bool
	createSNCalled      bool
	deleteSNCalled      bool
	createAssocCalled   bool
	deleteAssocCalled   bool
	createTGCalled      bool
	deleteTGCalled      bool
	createLisCalled     bool
	deleteLisCalled     bool
	createListenerInput *awslattice.CreateListenerInput
}

func (f *fakeLattice) GetService(_ context.Context, p *awslattice.GetServiceInput, _ ...func(*awslattice.Options)) (*awslattice.GetServiceOutput, error) {
	if f.getService == nil {
		return nil, fmt.Errorf("unexpected call to GetService")
	}
	return f.getService(p)
}

func (f *fakeLattice) CreateService(_ context.Context, p *awslattice.CreateServiceInput, _ ...func(*awslattice.Options)) (*awslattice.CreateServiceOutput, error) {
	f.createServiceCalled = true
	if f.createService == nil {
		return nil, fmt.Errorf("unexpected call to CreateService")
	}
	return f.createService(p)
}

func (f *fakeLattice) UpdateService(_ context.Context, p *awslattice.UpdateServiceInput, _ ...func(*awslattice.Options)) (*awslattice.UpdateServiceOutput, error) {
	if f.updateService == nil {
		return nil, fmt.Errorf("unexpected call to UpdateService")
	}
	return f.updateService(p)
}

func (f *fakeLattice) DeleteService(_ context.Context, p *awslattice.DeleteServiceInput, _ ...func(*awslattice.Options)) (*awslattice.DeleteServiceOutput, error) {
	f.deleteServiceCalled = true
	if f.deleteService == nil {
		return nil, fmt.Errorf("unexpected call to DeleteService")
	}
	return f.deleteService(p)
}

func (f *fakeLattice) GetServiceNetwork(_ context.Context, p *awslattice.GetServiceNetworkInput, _ ...func(*awslattice.Options)) (*awslattice.GetServiceNetworkOutput, error) {
	if f.getServiceNetwork == nil {
		return nil, fmt.Errorf("unexpected call to GetServiceNetwork")
	}
	return f.getServiceNetwork(p)
}

func (f *fakeLattice) CreateServiceNetwork(_ context.Context, p *awslattice.CreateServiceNetworkInput, _ ...func(*awslattice.Options)) (*awslattice.CreateServiceNetworkOutput, error) {
	f.createSNCalled = true
	if f.createServiceNetwork == nil {
		return nil, fmt.Errorf("unexpected call to CreateServiceNetwork")
	}
	return f.createServiceNetwork(p)
}

func (f *fakeLattice) UpdateServiceNetwork(_ context.Context, p *awslattice.UpdateServiceNetworkInput, _ ...func(*awslattice.Options)) (*awslattice.UpdateServiceNetworkOutput, error) {
	if f.updateServiceNetwork == nil {
		return nil, fmt.Errorf("unexpected call to UpdateServiceNetwork")
	}
	return f.updateServiceNetwork(p)
}

func (f *fakeLattice) DeleteServiceNetwork(_ context.Context, p *awslattice.DeleteServiceNetworkInput, _ ...func(*awslattice.Options)) (*awslattice.DeleteServiceNetworkOutput, error) {
	f.deleteSNCalled = true
	if f.deleteServiceNetwork == nil {
		return nil, fmt.Errorf("unexpected call to DeleteServiceNetwork")
	}
	return f.deleteServiceNetwork(p)
}

func (f *fakeLattice) GetServiceNetworkVpcAssociation(_ context.Context, p *awslattice.GetServiceNetworkVpcAssociationInput, _ ...func(*awslattice.Options)) (*awslattice.GetServiceNetworkVpcAssociationOutput, error) {
	if f.getSNVpcAssoc == nil {
		return nil, fmt.Errorf("unexpected call to GetServiceNetworkVpcAssociation")
	}
	return f.getSNVpcAssoc(p)
}

func (f *fakeLattice) CreateServiceNetworkVpcAssociation(_ context.Context, p *awslattice.CreateServiceNetworkVpcAssociationInput, _ ...func(*awslattice.Options)) (*awslattice.CreateServiceNetworkVpcAssociationOutput, error) {
	f.createAssocCalled = true
	if f.createSNVpcAssoc == nil {
		return nil, fmt.Errorf("unexpected call to CreateServiceNetworkVpcAssociation")
	}
	return f.createSNVpcAssoc(p)
}

func (f *fakeLattice) DeleteServiceNetworkVpcAssociation(_ context.Context, p *awslattice.DeleteServiceNetworkVpcAssociationInput, _ ...func(*awslattice.Options)) (*awslattice.DeleteServiceNetworkVpcAssociationOutput, error) {
	f.deleteAssocCalled = true
	if f.deleteSNVpcAssoc == nil {
		return nil, fmt.Errorf("unexpected call to DeleteServiceNetworkVpcAssociation")
	}
	return f.deleteSNVpcAssoc(p)
}

func (f *fakeLattice) GetServiceNetworkServiceAssociation(_ context.Context, p *awslattice.GetServiceNetworkServiceAssociationInput, _ ...func(*awslattice.Options)) (*awslattice.GetServiceNetworkServiceAssociationOutput, error) {
	if f.getSNSvcAssoc == nil {
		return nil, fmt.Errorf("unexpected call to GetServiceNetworkServiceAssociation")
	}
	return f.getSNSvcAssoc(p)
}

func (f *fakeLattice) CreateServiceNetworkServiceAssociation(_ context.Context, p *awslattice.CreateServiceNetworkServiceAssociationInput, _ ...func(*awslattice.Options)) (*awslattice.CreateServiceNetworkServiceAssociationOutput, error) {
	f.createAssocCalled = true
	if f.createSNSvcAssoc == nil {
		return nil, fmt.Errorf("unexpected call to CreateServiceNetworkServiceAssociation")
	}
	return f.createSNSvcAssoc(p)
}

func (f *fakeLattice) DeleteServiceNetworkServiceAssociation(_ context.Context, p *awslattice.DeleteServiceNetworkServiceAssociationInput, _ ...func(*awslattice.Options)) (*awslattice.DeleteServiceNetworkServiceAssociationOutput, error) {
	f.deleteAssocCalled = true
	if f.deleteSNSvcAssoc == nil {
		return nil, fmt.Errorf("unexpected call to DeleteServiceNetworkServiceAssociation")
	}
	return f.deleteSNSvcAssoc(p)
}

func (f *fakeLattice) GetTargetGroup(_ context.Context, p *awslattice.GetTargetGroupInput, _ ...func(*awslattice.Options)) (*awslattice.GetTargetGroupOutput, error) {
	if f.getTargetGroup == nil {
		return nil, fmt.Errorf("unexpected call to GetTargetGroup")
	}
	return f.getTargetGroup(p)
}

func (f *fakeLattice) CreateTargetGroup(_ context.Context, p *awslattice.CreateTargetGroupInput, _ ...func(*awslattice.Options)) (*awslattice.CreateTargetGroupOutput, error) {
	f.createTGCalled = true
	if f.createTargetGroup == nil {
		return nil, fmt.Errorf("unexpected call to CreateTargetGroup")
	}
	return f.createTargetGroup(p)
}

func (f *fakeLattice) DeleteTargetGroup(_ context.Context, p *awslattice.DeleteTargetGroupInput, _ ...func(*awslattice.Options)) (*awslattice.DeleteTargetGroupOutput, error) {
	f.deleteTGCalled = true
	if f.deleteTargetGroup == nil {
		return nil, fmt.Errorf("unexpected call to DeleteTargetGroup")
	}
	return f.deleteTargetGroup(p)
}

func (f *fakeLattice) GetListener(_ context.Context, p *awslattice.GetListenerInput, _ ...func(*awslattice.Options)) (*awslattice.GetListenerOutput, error) {
	if f.getListener == nil {
		return nil, fmt.Errorf("unexpected call to GetListener")
	}
	return f.getListener(p)
}

func (f *fakeLattice) CreateListener(_ context.Context, p *awslattice.CreateListenerInput, _ ...func(*awslattice.Options)) (*awslattice.CreateListenerOutput, error) {
	f.createLisCalled = true
	f.createListenerInput = p
	if f.createListener == nil {
		return nil, fmt.Errorf("unexpected call to CreateListener")
	}
	return f.createListener(p)
}

func (f *fakeLattice) UpdateListener(_ context.Context, p *awslattice.UpdateListenerInput, _ ...func(*awslattice.Options)) (*awslattice.UpdateListenerOutput, error) {
	if f.updateListener == nil {
		return nil, fmt.Errorf("unexpected call to UpdateListener")
	}
	return f.updateListener(p)
}

func (f *fakeLattice) DeleteListener(_ context.Context, p *awslattice.DeleteListenerInput, _ ...func(*awslattice.Options)) (*awslattice.DeleteListenerOutput, error) {
	f.deleteLisCalled = true
	if f.deleteListener == nil {
		return nil, fmt.Errorf("unexpected call to DeleteListener")
	}
	return f.deleteListener(p)
}

func (f *fakeLattice) TagResource(_ context.Context, p *awslattice.TagResourceInput, _ ...func(*awslattice.Options)) (*awslattice.TagResourceOutput, error) {
	if f.tagResource == nil {
		return nil, fmt.Errorf("unexpected call to TagResource")
	}
	return f.tagResource(p)
}

const (
	testLatticeSvcID  = "svc-0123456789abcdef0"
	testLatticeSvcARN = "arn:aws:vpc-lattice:us-east-1:123456789012:service/svc-0123456789abcdef0"
	testLatticeSNID   = "sn-0123456789abcdef0"
)

func latticeServiceCR(mutate ...func(*awsv1alpha1.LatticeService)) *awsv1alpha1.LatticeService {
	svc := &awsv1alpha1.LatticeService{
		ObjectMeta: metav1.ObjectMeta{Name: "my-service", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec: awsv1alpha1.LatticeServiceSpec{
			Name:     "my-service",
			AuthType: "AWS_IAM",
			Tags:     map[string]string{"env": "test"},
		},
	}
	for _, m := range mutate {
		m(svc)
	}
	return svc
}

func TestLatticeServiceReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-service", Namespace: "default"}}

	tests := []struct {
		name    string
		objs    []client.Object
		fake    *fakeLattice
		setup   func(t *testing.T, ctx context.Context, c client.Client)
		wantErr bool
		assert  func(t *testing.T, ctx context.Context, c client.Client, f *fakeLattice, res ctrl.Result)
	}{
		{
			name: "create persists identifier and polls",
			objs: []client.Object{latticeServiceCR()},
			fake: &fakeLattice{
				createService: func(p *awslattice.CreateServiceInput) (*awslattice.CreateServiceOutput, error) {
					if aws.ToString(p.Name) != "my-service" {
						return nil, fmt.Errorf("unexpected name %q", aws.ToString(p.Name))
					}
					if p.AuthType != latticetypes.AuthTypeAwsIam {
						return nil, fmt.Errorf("unexpected authType %q", p.AuthType)
					}
					return &awslattice.CreateServiceOutput{
						Id:     aws.String(testLatticeSvcID),
						Arn:    aws.String(testLatticeSvcARN),
						Status: latticetypes.ServiceStatusCreateInProgress,
					}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeLattice, res ctrl.Result) {
				if !f.createServiceCalled {
					t.Error("expected CreateService to be called")
				}
				if res != requeueLatticePolling {
					t.Errorf("result = %+v, want polling requeue", res)
				}
				got := &awsv1alpha1.LatticeService{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.ID != testLatticeSvcID {
					t.Errorf("status.id = %q, want %q", got.Status.ID, testLatticeSvcID)
				}
				if got.Status.ARN != testLatticeSvcARN {
					t.Errorf("status.arn = %q, want %q", got.Status.ARN, testLatticeSvcARN)
				}
			},
		},
		{
			name: "in-progress service keeps polling",
			objs: []client.Object{latticeServiceCR(func(s *awsv1alpha1.LatticeService) {
				s.Finalizers = []string{awsv1alpha1.FinalizerName}
				s.Status.ID = testLatticeSvcID
			})},
			fake: &fakeLattice{
				getService: func(_ *awslattice.GetServiceInput) (*awslattice.GetServiceOutput, error) {
					return &awslattice.GetServiceOutput{
						Id:     aws.String(testLatticeSvcID),
						Arn:    aws.String(testLatticeSvcARN),
						Status: latticetypes.ServiceStatusCreateInProgress,
					}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeLattice, res ctrl.Result) {
				if f.createServiceCalled {
					t.Error("CreateService must not be called while creating")
				}
				if res != requeueLatticePolling {
					t.Errorf("result = %+v, want polling requeue", res)
				}
			},
		},
		{
			name: "steady state ACTIVE updates and sets Ready",
			objs: []client.Object{latticeServiceCR(func(s *awsv1alpha1.LatticeService) {
				s.Finalizers = []string{awsv1alpha1.FinalizerName}
				s.Generation = 2
				s.Status.ID = testLatticeSvcID
				s.Status.ObservedGeneration = 1
			})},
			fake: &fakeLattice{
				getService: func(_ *awslattice.GetServiceInput) (*awslattice.GetServiceOutput, error) {
					return &awslattice.GetServiceOutput{
						Id:       aws.String(testLatticeSvcID),
						Arn:      aws.String(testLatticeSvcARN),
						Status:   latticetypes.ServiceStatusActive,
						AuthType: latticetypes.AuthTypeNone,
						DnsEntry: &latticetypes.DnsEntry{DomainName: aws.String("my-service.example")},
					}, nil
				},
				updateService: func(p *awslattice.UpdateServiceInput) (*awslattice.UpdateServiceOutput, error) {
					if p.AuthType != latticetypes.AuthTypeAwsIam {
						return nil, fmt.Errorf("unexpected authType %q", p.AuthType)
					}
					return &awslattice.UpdateServiceOutput{}, nil
				},
				tagResource: func(_ *awslattice.TagResourceInput) (*awslattice.TagResourceOutput, error) {
					return &awslattice.TagResourceOutput{}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeLattice, _ ctrl.Result) {
				if f.createServiceCalled {
					t.Error("CreateService must not be called in steady state")
				}
				got := &awsv1alpha1.LatticeService{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionTrue {
					t.Errorf("Ready condition = %+v, want True", cond)
				}
				if got.Status.DNSName != "my-service.example" {
					t.Errorf("status.dnsName = %q, want my-service.example", got.Status.DNSName)
				}
				if got.Status.ObservedGeneration != 2 {
					t.Errorf("observedGeneration = %d, want 2", got.Status.ObservedGeneration)
				}
			},
		},
		{
			name: "delete with finalizer calls AWS delete with status ID",
			objs: []client.Object{latticeServiceCR(func(s *awsv1alpha1.LatticeService) {
				s.Finalizers = []string{awsv1alpha1.FinalizerName}
				s.Status.ID = testLatticeSvcID
			})},
			fake: &fakeLattice{
				deleteService: func(p *awslattice.DeleteServiceInput) (*awslattice.DeleteServiceOutput, error) {
					if aws.ToString(p.ServiceIdentifier) != testLatticeSvcID {
						return nil, fmt.Errorf("unexpected id %q", aws.ToString(p.ServiceIdentifier))
					}
					return &awslattice.DeleteServiceOutput{}, nil
				},
			},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, latticeServiceCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeLattice, _ ctrl.Result) {
				if !f.deleteServiceCalled {
					t.Error("expected DeleteService to be called")
				}
				got := &awsv1alpha1.LatticeService{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "delete with empty status ID skips AWS delete",
			objs: []client.Object{latticeServiceCR(func(s *awsv1alpha1.LatticeService) {
				s.Finalizers = []string{awsv1alpha1.FinalizerName}
			})},
			fake: &fakeLattice{},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, latticeServiceCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeLattice, _ ctrl.Result) {
				if f.deleteServiceCalled {
					t.Error("DeleteService must not be called without a persisted ID")
				}
				got := &awsv1alpha1.LatticeService{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "abandon policy skips AWS delete",
			objs: []client.Object{latticeServiceCR(func(s *awsv1alpha1.LatticeService) {
				s.Finalizers = []string{awsv1alpha1.FinalizerName}
				s.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
				s.Status.ID = testLatticeSvcID
			})},
			fake: &fakeLattice{},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, latticeServiceCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeLattice, _ ctrl.Result) {
				if f.deleteServiceCalled {
					t.Error("DeleteService must not be called when abandoning")
				}
				got := &awsv1alpha1.LatticeService{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "service deleted out of band is recreated",
			objs: []client.Object{latticeServiceCR(func(s *awsv1alpha1.LatticeService) {
				s.Finalizers = []string{awsv1alpha1.FinalizerName}
				s.Status.ID = testLatticeSvcID
			})},
			fake: &fakeLattice{
				getService: func(_ *awslattice.GetServiceInput) (*awslattice.GetServiceOutput, error) {
					return nil, latticeNotFoundErr()
				},
				createService: func(_ *awslattice.CreateServiceInput) (*awslattice.CreateServiceOutput, error) {
					return &awslattice.CreateServiceOutput{
						Id:     aws.String("svc-new"),
						Arn:    aws.String(testLatticeSvcARN),
						Status: latticetypes.ServiceStatusCreateInProgress,
					}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeLattice, _ ctrl.Result) {
				if !f.createServiceCalled {
					t.Error("expected CreateService to be called after out-of-band delete")
				}
				got := &awsv1alpha1.LatticeService{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.ID != "svc-new" {
					t.Errorf("status.id = %q, want svc-new", got.Status.ID)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			scheme := newNetsecScheme(t)
			c := newNetsecFakeClient(scheme, tc.objs...)
			r := &LatticeServiceReconciler{Client: c, Scheme: scheme, LatticeClient: tc.fake}

			if tc.setup != nil {
				tc.setup(t, ctx, c)
			}

			res, err := r.Reconcile(ctx, req)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tc.assert(t, ctx, c, tc.fake, res)
		})
	}
}

func TestLatticeServiceNetworkReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-sn", Namespace: "default"}}
	snCR := func(mutate ...func(*awsv1alpha1.LatticeServiceNetwork)) *awsv1alpha1.LatticeServiceNetwork {
		sn := &awsv1alpha1.LatticeServiceNetwork{
			ObjectMeta: metav1.ObjectMeta{Name: "my-sn", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
			Spec:       awsv1alpha1.LatticeServiceNetworkSpec{Name: "my-sn", AuthType: "NONE"},
		}
		for _, m := range mutate {
			m(sn)
		}
		return sn
	}

	t.Run("create persists identifier", func(t *testing.T) {
		ctx := context.Background()
		scheme := newNetsecScheme(t)
		c := newNetsecFakeClient(scheme, snCR())
		f := &fakeLattice{
			createServiceNetwork: func(_ *awslattice.CreateServiceNetworkInput) (*awslattice.CreateServiceNetworkOutput, error) {
				return &awslattice.CreateServiceNetworkOutput{
					Id:  aws.String(testLatticeSNID),
					Arn: aws.String("arn:sn"),
				}, nil
			},
		}
		r := &LatticeServiceNetworkReconciler{Client: c, Scheme: scheme, LatticeClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.LatticeServiceNetwork{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ID != testLatticeSNID {
			t.Errorf("status.id = %q, want %q", got.Status.ID, testLatticeSNID)
		}
	})

	t.Run("delete with finalizer calls AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newNetsecScheme(t)
		obj := snCR(func(sn *awsv1alpha1.LatticeServiceNetwork) {
			sn.Finalizers = []string{awsv1alpha1.FinalizerName}
			sn.Status.ID = testLatticeSNID
		})
		c := newNetsecFakeClient(scheme, obj)
		f := &fakeLattice{
			deleteServiceNetwork: func(_ *awslattice.DeleteServiceNetworkInput) (*awslattice.DeleteServiceNetworkOutput, error) {
				return &awslattice.DeleteServiceNetworkOutput{}, nil
			},
		}
		r := &LatticeServiceNetworkReconciler{Client: c, Scheme: scheme, LatticeClient: f}
		if err := c.Delete(ctx, snCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteSNCalled {
			t.Error("expected DeleteServiceNetwork to be called")
		}
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newNetsecScheme(t)
		obj := snCR(func(sn *awsv1alpha1.LatticeServiceNetwork) {
			sn.Finalizers = []string{awsv1alpha1.FinalizerName}
			sn.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			sn.Status.ID = testLatticeSNID
		})
		c := newNetsecFakeClient(scheme, obj)
		f := &fakeLattice{}
		r := &LatticeServiceNetworkReconciler{Client: c, Scheme: scheme, LatticeClient: f}
		if err := c.Delete(ctx, snCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteSNCalled {
			t.Error("DeleteServiceNetwork must not be called when abandoning")
		}
	})
}

func TestLatticeServiceNetworkVpcAssociationReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-assoc", Namespace: "default"}}
	assocCR := func(mutate ...func(*awsv1alpha1.LatticeServiceNetworkVpcAssociation)) *awsv1alpha1.LatticeServiceNetworkVpcAssociation {
		a := &awsv1alpha1.LatticeServiceNetworkVpcAssociation{
			ObjectMeta: metav1.ObjectMeta{Name: "my-assoc", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
			Spec: awsv1alpha1.LatticeServiceNetworkVpcAssociationSpec{
				ServiceNetworkRef: awsv1alpha1.LatticeServiceNetworkRef{ID: testLatticeSNID},
				VPCRef:            awsv1alpha1.VPCResourceRef{ID: "vpc-123"},
			},
		}
		for _, m := range mutate {
			m(a)
		}
		return a
	}

	t.Run("create persists identifier and polls", func(t *testing.T) {
		ctx := context.Background()
		scheme := newNetsecScheme(t)
		c := newNetsecFakeClient(scheme, assocCR())
		f := &fakeLattice{
			createSNVpcAssoc: func(p *awslattice.CreateServiceNetworkVpcAssociationInput) (*awslattice.CreateServiceNetworkVpcAssociationOutput, error) {
				if aws.ToString(p.VpcIdentifier) != "vpc-123" {
					return nil, fmt.Errorf("unexpected vpc %q", aws.ToString(p.VpcIdentifier))
				}
				return &awslattice.CreateServiceNetworkVpcAssociationOutput{
					Id:     aws.String("snva-1"),
					Arn:    aws.String("arn:snva"),
					Status: latticetypes.ServiceNetworkVpcAssociationStatusCreateInProgress,
				}, nil
			},
		}
		r := &LatticeServiceNetworkVpcAssociationReconciler{Client: c, Scheme: scheme, LatticeClient: f}
		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if res != requeueLatticePolling {
			t.Errorf("result = %+v, want polling requeue", res)
		}
		got := &awsv1alpha1.LatticeServiceNetworkVpcAssociation{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ID != "snva-1" {
			t.Errorf("status.id = %q, want snva-1", got.Status.ID)
		}
	})

	t.Run("dependency not ready requeues", func(t *testing.T) {
		ctx := context.Background()
		scheme := newNetsecScheme(t)
		obj := assocCR(func(a *awsv1alpha1.LatticeServiceNetworkVpcAssociation) {
			a.Spec.ServiceNetworkRef = awsv1alpha1.LatticeServiceNetworkRef{Name: "my-sn"}
		})
		sn := &awsv1alpha1.LatticeServiceNetwork{
			ObjectMeta: metav1.ObjectMeta{Name: "my-sn", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
			Spec:       awsv1alpha1.LatticeServiceNetworkSpec{Name: "my-sn"},
		}
		c := newNetsecFakeClient(scheme, obj, sn)
		f := &fakeLattice{}
		r := &LatticeServiceNetworkVpcAssociationReconciler{Client: c, Scheme: scheme, LatticeClient: f}
		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if res != requeueDependency {
			t.Errorf("result = %+v, want requeueDependency", res)
		}
		if f.createAssocCalled {
			t.Error("create must not be called while dependency is not ready")
		}
	})

	t.Run("delete with finalizer calls AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newNetsecScheme(t)
		obj := assocCR(func(a *awsv1alpha1.LatticeServiceNetworkVpcAssociation) {
			a.Finalizers = []string{awsv1alpha1.FinalizerName}
			a.Status.ID = "snva-1"
		})
		c := newNetsecFakeClient(scheme, obj)
		f := &fakeLattice{
			deleteSNVpcAssoc: func(_ *awslattice.DeleteServiceNetworkVpcAssociationInput) (*awslattice.DeleteServiceNetworkVpcAssociationOutput, error) {
				return &awslattice.DeleteServiceNetworkVpcAssociationOutput{}, nil
			},
		}
		r := &LatticeServiceNetworkVpcAssociationReconciler{Client: c, Scheme: scheme, LatticeClient: f}
		if err := c.Delete(ctx, assocCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteAssocCalled {
			t.Error("expected DeleteServiceNetworkVpcAssociation to be called")
		}
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newNetsecScheme(t)
		obj := assocCR(func(a *awsv1alpha1.LatticeServiceNetworkVpcAssociation) {
			a.Finalizers = []string{awsv1alpha1.FinalizerName}
			a.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			a.Status.ID = "snva-1"
		})
		c := newNetsecFakeClient(scheme, obj)
		f := &fakeLattice{}
		r := &LatticeServiceNetworkVpcAssociationReconciler{Client: c, Scheme: scheme, LatticeClient: f}
		if err := c.Delete(ctx, assocCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteAssocCalled {
			t.Error("delete must not be called when abandoning")
		}
	})
}

func TestLatticeServiceNetworkServiceAssociationReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-assoc", Namespace: "default"}}
	assocCR := func(mutate ...func(*awsv1alpha1.LatticeServiceNetworkServiceAssociation)) *awsv1alpha1.LatticeServiceNetworkServiceAssociation {
		a := &awsv1alpha1.LatticeServiceNetworkServiceAssociation{
			ObjectMeta: metav1.ObjectMeta{Name: "my-assoc", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
			Spec: awsv1alpha1.LatticeServiceNetworkServiceAssociationSpec{
				ServiceNetworkRef: awsv1alpha1.LatticeServiceNetworkRef{ID: testLatticeSNID},
				ServiceRef:        awsv1alpha1.LatticeServiceRef{ID: testLatticeSvcID},
			},
		}
		for _, m := range mutate {
			m(a)
		}
		return a
	}

	t.Run("create persists identifier", func(t *testing.T) {
		ctx := context.Background()
		scheme := newNetsecScheme(t)
		c := newNetsecFakeClient(scheme, assocCR())
		f := &fakeLattice{
			createSNSvcAssoc: func(_ *awslattice.CreateServiceNetworkServiceAssociationInput) (*awslattice.CreateServiceNetworkServiceAssociationOutput, error) {
				return &awslattice.CreateServiceNetworkServiceAssociationOutput{
					Id:     aws.String("snsa-1"),
					Arn:    aws.String("arn:snsa"),
					Status: latticetypes.ServiceNetworkServiceAssociationStatusCreateInProgress,
				}, nil
			},
		}
		r := &LatticeServiceNetworkServiceAssociationReconciler{Client: c, Scheme: scheme, LatticeClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.LatticeServiceNetworkServiceAssociation{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ID != "snsa-1" {
			t.Errorf("status.id = %q, want snsa-1", got.Status.ID)
		}
	})

	t.Run("delete with finalizer calls AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newNetsecScheme(t)
		obj := assocCR(func(a *awsv1alpha1.LatticeServiceNetworkServiceAssociation) {
			a.Finalizers = []string{awsv1alpha1.FinalizerName}
			a.Status.ID = "snsa-1"
		})
		c := newNetsecFakeClient(scheme, obj)
		f := &fakeLattice{
			deleteSNSvcAssoc: func(_ *awslattice.DeleteServiceNetworkServiceAssociationInput) (*awslattice.DeleteServiceNetworkServiceAssociationOutput, error) {
				return &awslattice.DeleteServiceNetworkServiceAssociationOutput{}, nil
			},
		}
		r := &LatticeServiceNetworkServiceAssociationReconciler{Client: c, Scheme: scheme, LatticeClient: f}
		if err := c.Delete(ctx, assocCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteAssocCalled {
			t.Error("expected DeleteServiceNetworkServiceAssociation to be called")
		}
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newNetsecScheme(t)
		obj := assocCR(func(a *awsv1alpha1.LatticeServiceNetworkServiceAssociation) {
			a.Finalizers = []string{awsv1alpha1.FinalizerName}
			a.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			a.Status.ID = "snsa-1"
		})
		c := newNetsecFakeClient(scheme, obj)
		f := &fakeLattice{}
		r := &LatticeServiceNetworkServiceAssociationReconciler{Client: c, Scheme: scheme, LatticeClient: f}
		if err := c.Delete(ctx, assocCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteAssocCalled {
			t.Error("delete must not be called when abandoning")
		}
	})
}

func TestLatticeTargetGroupReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-tg", Namespace: "default"}}
	tgCR := func(mutate ...func(*awsv1alpha1.LatticeTargetGroup)) *awsv1alpha1.LatticeTargetGroup {
		tg := &awsv1alpha1.LatticeTargetGroup{
			ObjectMeta: metav1.ObjectMeta{Name: "my-tg", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
			Spec: awsv1alpha1.LatticeTargetGroupSpec{
				Name: "my-tg",
				Type: "IP",
				Config: &awsv1alpha1.LatticeTargetGroupConfig{
					Port:     8080,
					Protocol: "HTTP",
					VPCRef:   &awsv1alpha1.VPCResourceRef{ID: "vpc-123"},
					HealthCheck: &awsv1alpha1.LatticeHealthCheck{
						Path:     "/healthz",
						Protocol: "HTTP",
					},
				},
			},
		}
		for _, m := range mutate {
			m(tg)
		}
		return tg
	}

	t.Run("create persists identifier", func(t *testing.T) {
		ctx := context.Background()
		scheme := newNetsecScheme(t)
		c := newNetsecFakeClient(scheme, tgCR())
		f := &fakeLattice{
			createTargetGroup: func(p *awslattice.CreateTargetGroupInput) (*awslattice.CreateTargetGroupOutput, error) {
				if p.Config == nil || aws.ToString(p.Config.VpcIdentifier) != "vpc-123" {
					return nil, fmt.Errorf("expected config with vpc-123")
				}
				if p.Config.HealthCheck == nil || aws.ToString(p.Config.HealthCheck.Path) != "/healthz" {
					return nil, fmt.Errorf("expected health check path")
				}
				return &awslattice.CreateTargetGroupOutput{
					Id:     aws.String("tg-1"),
					Arn:    aws.String("arn:tg"),
					Status: latticetypes.TargetGroupStatusCreateInProgress,
				}, nil
			},
		}
		r := &LatticeTargetGroupReconciler{Client: c, Scheme: scheme, LatticeClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.LatticeTargetGroup{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ID != "tg-1" {
			t.Errorf("status.id = %q, want tg-1", got.Status.ID)
		}
	})

	t.Run("delete with finalizer calls AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newNetsecScheme(t)
		obj := tgCR(func(tg *awsv1alpha1.LatticeTargetGroup) {
			tg.Finalizers = []string{awsv1alpha1.FinalizerName}
			tg.Status.ID = "tg-1"
		})
		c := newNetsecFakeClient(scheme, obj)
		f := &fakeLattice{
			deleteTargetGroup: func(_ *awslattice.DeleteTargetGroupInput) (*awslattice.DeleteTargetGroupOutput, error) {
				return &awslattice.DeleteTargetGroupOutput{}, nil
			},
		}
		r := &LatticeTargetGroupReconciler{Client: c, Scheme: scheme, LatticeClient: f}
		if err := c.Delete(ctx, tgCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteTGCalled {
			t.Error("expected DeleteTargetGroup to be called")
		}
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newNetsecScheme(t)
		obj := tgCR(func(tg *awsv1alpha1.LatticeTargetGroup) {
			tg.Finalizers = []string{awsv1alpha1.FinalizerName}
			tg.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			tg.Status.ID = "tg-1"
		})
		c := newNetsecFakeClient(scheme, obj)
		f := &fakeLattice{}
		r := &LatticeTargetGroupReconciler{Client: c, Scheme: scheme, LatticeClient: f}
		if err := c.Delete(ctx, tgCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteTGCalled {
			t.Error("DeleteTargetGroup must not be called when abandoning")
		}
	})
}

func TestLatticeListenerReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-listener", Namespace: "default"}}
	lisCR := func(mutate ...func(*awsv1alpha1.LatticeListener)) *awsv1alpha1.LatticeListener {
		l := &awsv1alpha1.LatticeListener{
			ObjectMeta: metav1.ObjectMeta{Name: "my-listener", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
			Spec: awsv1alpha1.LatticeListenerSpec{
				ServiceRef: awsv1alpha1.LatticeServiceRef{ID: testLatticeSvcID},
				Name:       "my-listener",
				Protocol:   "HTTP",
				Port:       80,
				DefaultAction: awsv1alpha1.LatticeDefaultAction{
					Forward: []awsv1alpha1.LatticeForwardTarget{
						{TargetGroupRef: awsv1alpha1.LatticeTargetGroupRef{ID: "tg-1"}, Weight: 100},
					},
				},
			},
		}
		for _, m := range mutate {
			m(l)
		}
		return l
	}

	t.Run("create persists identifier and builds forward action", func(t *testing.T) {
		ctx := context.Background()
		scheme := newNetsecScheme(t)
		c := newNetsecFakeClient(scheme, lisCR())
		f := &fakeLattice{
			createListener: func(p *awslattice.CreateListenerInput) (*awslattice.CreateListenerOutput, error) {
				fwd, ok := p.DefaultAction.(*latticetypes.RuleActionMemberForward)
				if !ok {
					return nil, fmt.Errorf("expected forward action, got %T", p.DefaultAction)
				}
				if len(fwd.Value.TargetGroups) != 1 || aws.ToString(fwd.Value.TargetGroups[0].TargetGroupIdentifier) != "tg-1" {
					return nil, fmt.Errorf("unexpected target groups %+v", fwd.Value.TargetGroups)
				}
				return &awslattice.CreateListenerOutput{
					Id:        aws.String("lis-1"),
					Arn:       aws.String("arn:lis"),
					ServiceId: aws.String(testLatticeSvcID),
				}, nil
			},
		}
		r := &LatticeListenerReconciler{Client: c, Scheme: scheme, LatticeClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.LatticeListener{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ID != "lis-1" {
			t.Errorf("status.id = %q, want lis-1", got.Status.ID)
		}
		if got.Status.ServiceID != testLatticeSvcID {
			t.Errorf("status.serviceId = %q, want %q", got.Status.ServiceID, testLatticeSvcID)
		}
	})

	t.Run("delete with finalizer calls AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newNetsecScheme(t)
		obj := lisCR(func(l *awsv1alpha1.LatticeListener) {
			l.Finalizers = []string{awsv1alpha1.FinalizerName}
			l.Status.ID = "lis-1"
			l.Status.ServiceID = testLatticeSvcID
		})
		c := newNetsecFakeClient(scheme, obj)
		f := &fakeLattice{
			deleteListener: func(p *awslattice.DeleteListenerInput) (*awslattice.DeleteListenerOutput, error) {
				if aws.ToString(p.ServiceIdentifier) != testLatticeSvcID {
					return nil, fmt.Errorf("unexpected service id %q", aws.ToString(p.ServiceIdentifier))
				}
				return &awslattice.DeleteListenerOutput{}, nil
			},
		}
		r := &LatticeListenerReconciler{Client: c, Scheme: scheme, LatticeClient: f}
		if err := c.Delete(ctx, lisCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteLisCalled {
			t.Error("expected DeleteListener to be called")
		}
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newNetsecScheme(t)
		obj := lisCR(func(l *awsv1alpha1.LatticeListener) {
			l.Finalizers = []string{awsv1alpha1.FinalizerName}
			l.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			l.Status.ID = "lis-1"
		})
		c := newNetsecFakeClient(scheme, obj)
		f := &fakeLattice{}
		r := &LatticeListenerReconciler{Client: c, Scheme: scheme, LatticeClient: f}
		if err := c.Delete(ctx, lisCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteLisCalled {
			t.Error("DeleteListener must not be called when abandoning")
		}
	})
}
