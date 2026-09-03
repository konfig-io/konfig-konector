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
	awssd "github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	sdtypes "github.com/aws/aws-sdk-go-v2/service/servicediscovery/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

const (
	testCloudMapServiceID  = "srv-abc123"
	testCloudMapServiceARN = "arn:aws:servicediscovery:us-east-1:123456789012:service/srv-abc123"
	testCloudMapNSID       = "ns-def456"
)

type fakeCloudMapServiceAPI struct {
	createService func(ctx context.Context, params *awssd.CreateServiceInput) (*awssd.CreateServiceOutput, error)
	getService    func(ctx context.Context, params *awssd.GetServiceInput) (*awssd.GetServiceOutput, error)

	createCalled bool
	updateCalled bool
	deleteCalled bool
	deleteInput  *awssd.DeleteServiceInput
	updateInput  *awssd.UpdateServiceInput
}

func (f *fakeCloudMapServiceAPI) CreateService(ctx context.Context, params *awssd.CreateServiceInput, _ ...func(*awssd.Options)) (*awssd.CreateServiceOutput, error) {
	f.createCalled = true
	if f.createService == nil {
		return nil, fmt.Errorf("unexpected call to CreateService")
	}
	return f.createService(ctx, params)
}

func (f *fakeCloudMapServiceAPI) GetService(ctx context.Context, params *awssd.GetServiceInput, _ ...func(*awssd.Options)) (*awssd.GetServiceOutput, error) {
	if f.getService == nil {
		return nil, fmt.Errorf("unexpected call to GetService")
	}
	return f.getService(ctx, params)
}

func (f *fakeCloudMapServiceAPI) UpdateService(_ context.Context, params *awssd.UpdateServiceInput, _ ...func(*awssd.Options)) (*awssd.UpdateServiceOutput, error) {
	f.updateCalled = true
	f.updateInput = params
	return &awssd.UpdateServiceOutput{}, nil
}

func (f *fakeCloudMapServiceAPI) DeleteService(_ context.Context, params *awssd.DeleteServiceInput, _ ...func(*awssd.Options)) (*awssd.DeleteServiceOutput, error) {
	f.deleteCalled = true
	f.deleteInput = params
	return &awssd.DeleteServiceOutput{}, nil
}

func cloudMapServiceScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return scheme
}

func cloudMapServiceCR(mutate ...func(*awsv1alpha1.CloudMapService)) *awsv1alpha1.CloudMapService {
	svc := &awsv1alpha1.CloudMapService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-service",
			Namespace: "default",
		},
		Spec: awsv1alpha1.CloudMapServiceSpec{
			Name:         "my-service",
			NamespaceRef: awsv1alpha1.CloudMapNamespaceRef{ID: testCloudMapNSID},
			DnsConfig: &awsv1alpha1.CloudMapDnsConfig{
				RecordType:    "A",
				TTL:           60,
				RoutingPolicy: "MULTIVALUE",
			},
		},
	}
	for _, m := range mutate {
		m(svc)
	}
	return svc
}

func TestCloudMapServiceReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-service", Namespace: "default"}}

	t.Run("create happy path persists service ID and becomes Ready", func(t *testing.T) {
		ctx := context.Background()
		scheme := cloudMapServiceScheme(t)
		f := &fakeCloudMapServiceAPI{
			createService: func(_ context.Context, params *awssd.CreateServiceInput) (*awssd.CreateServiceOutput, error) {
				if aws.ToString(params.Name) != "my-service" {
					return nil, fmt.Errorf("unexpected name %q", aws.ToString(params.Name))
				}
				if aws.ToString(params.NamespaceId) != testCloudMapNSID {
					return nil, fmt.Errorf("unexpected namespace ID %q", aws.ToString(params.NamespaceId))
				}
				if params.DnsConfig == nil || len(params.DnsConfig.DnsRecords) != 1 {
					return nil, fmt.Errorf("expected one DNS record")
				}
				if params.DnsConfig.DnsRecords[0].Type != sdtypes.RecordTypeA {
					return nil, fmt.Errorf("unexpected record type %q", params.DnsConfig.DnsRecords[0].Type)
				}
				if params.DnsConfig.RoutingPolicy != sdtypes.RoutingPolicyMultivalue {
					return nil, fmt.Errorf("unexpected routing policy %q", params.DnsConfig.RoutingPolicy)
				}
				return &awssd.CreateServiceOutput{Service: &sdtypes.Service{
					Id:  aws.String(testCloudMapServiceID),
					Arn: aws.String(testCloudMapServiceARN),
				}}, nil
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.CloudMapService{}).
			WithObjects(cloudMapServiceCR()).Build()
		r := &CloudMapServiceReconciler{Client: c, Scheme: scheme, ServiceDiscoveryClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.createCalled {
			t.Fatal("expected CreateService to be called")
		}
		got := &awsv1alpha1.CloudMapService{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ServiceID != testCloudMapServiceID {
			t.Errorf("status.serviceId = %q, want %q (must persist right after CreateService)", got.Status.ServiceID, testCloudMapServiceID)
		}
		if got.Status.ARN != testCloudMapServiceARN {
			t.Errorf("status.arn = %q, want %q", got.Status.ARN, testCloudMapServiceARN)
		}
		cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
		if cond == nil || cond.Status != metav1.ConditionTrue {
			t.Errorf("Ready condition = %+v, want True", cond)
		}
	})

	t.Run("steady state does not create", func(t *testing.T) {
		ctx := context.Background()
		scheme := cloudMapServiceScheme(t)
		f := &fakeCloudMapServiceAPI{
			getService: func(_ context.Context, _ *awssd.GetServiceInput) (*awssd.GetServiceOutput, error) {
				return &awssd.GetServiceOutput{Service: &sdtypes.Service{
					Id:  aws.String(testCloudMapServiceID),
					Arn: aws.String(testCloudMapServiceARN),
				}}, nil
			},
		}
		svc := cloudMapServiceCR(func(s *awsv1alpha1.CloudMapService) {
			s.Finalizers = []string{awsv1alpha1.FinalizerName}
			s.Generation = 1
			s.Status.ServiceID = testCloudMapServiceID
			s.Status.ObservedGeneration = 1
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.CloudMapService{}).
			WithObjects(svc).Build()
		r := &CloudMapServiceReconciler{Client: c, Scheme: scheme, ServiceDiscoveryClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.createCalled {
			t.Error("expected CreateService NOT to be called")
		}
		if f.updateCalled {
			t.Error("expected UpdateService NOT to be called at steady state")
		}
	})

	t.Run("generation change applies update", func(t *testing.T) {
		ctx := context.Background()
		scheme := cloudMapServiceScheme(t)
		f := &fakeCloudMapServiceAPI{
			getService: func(_ context.Context, _ *awssd.GetServiceInput) (*awssd.GetServiceOutput, error) {
				return &awssd.GetServiceOutput{Service: &sdtypes.Service{
					Id: aws.String(testCloudMapServiceID),
				}}, nil
			},
		}
		svc := cloudMapServiceCR(func(s *awsv1alpha1.CloudMapService) {
			s.Finalizers = []string{awsv1alpha1.FinalizerName}
			s.Generation = 2
			s.Spec.DnsConfig.TTL = 120
			s.Status.ServiceID = testCloudMapServiceID
			s.Status.ObservedGeneration = 1
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.CloudMapService{}).
			WithObjects(svc).Build()
		r := &CloudMapServiceReconciler{Client: c, Scheme: scheme, ServiceDiscoveryClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.updateCalled {
			t.Fatal("expected UpdateService to be called on generation change")
		}
		if aws.ToString(f.updateInput.Id) != testCloudMapServiceID {
			t.Errorf("update service ID = %q, want %q", aws.ToString(f.updateInput.Id), testCloudMapServiceID)
		}
		if ttl := aws.ToInt64(f.updateInput.Service.DnsConfig.DnsRecords[0].TTL); ttl != 120 {
			t.Errorf("update TTL = %d, want 120", ttl)
		}
		got := &awsv1alpha1.CloudMapService{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ObservedGeneration != 2 {
			t.Errorf("observedGeneration = %d, want 2", got.Status.ObservedGeneration)
		}
	})

	t.Run("namespace ref not ready requeues without error", func(t *testing.T) {
		ctx := context.Background()
		scheme := cloudMapServiceScheme(t)
		f := &fakeCloudMapServiceAPI{}
		// Namespace CR exists but has no ID yet.
		nsCR := &awsv1alpha1.CloudMapNamespace{
			ObjectMeta: metav1.ObjectMeta{Name: "my-namespace", Namespace: "default"},
		}
		svc := cloudMapServiceCR(func(s *awsv1alpha1.CloudMapService) {
			s.Finalizers = []string{awsv1alpha1.FinalizerName}
			s.Spec.NamespaceRef = awsv1alpha1.CloudMapNamespaceRef{Name: "my-namespace"}
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.CloudMapService{}, &awsv1alpha1.CloudMapNamespace{}).
			WithObjects(svc, nsCR).Build()
		r := &CloudMapServiceReconciler{Client: c, Scheme: scheme, ServiceDiscoveryClient: f}

		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v (dependencyNotReady must not surface as error)", err)
		}
		if res != requeueDependency {
			t.Errorf("result = %+v, want requeueDependency", res)
		}
		if f.createCalled {
			t.Error("expected CreateService NOT to be called while namespace is not ready")
		}
	})

	t.Run("namespace ref ready resolves ID from CR status", func(t *testing.T) {
		ctx := context.Background()
		scheme := cloudMapServiceScheme(t)
		f := &fakeCloudMapServiceAPI{
			createService: func(_ context.Context, params *awssd.CreateServiceInput) (*awssd.CreateServiceOutput, error) {
				if aws.ToString(params.NamespaceId) != testCloudMapNSID {
					return nil, fmt.Errorf("unexpected namespace ID %q", aws.ToString(params.NamespaceId))
				}
				return &awssd.CreateServiceOutput{Service: &sdtypes.Service{
					Id: aws.String(testCloudMapServiceID),
				}}, nil
			},
		}
		nsCR := &awsv1alpha1.CloudMapNamespace{
			ObjectMeta: metav1.ObjectMeta{Name: "my-namespace", Namespace: "default"},
			Status:     awsv1alpha1.CloudMapNamespaceStatus{NamespaceID: testCloudMapNSID},
		}
		svc := cloudMapServiceCR(func(s *awsv1alpha1.CloudMapService) {
			s.Finalizers = []string{awsv1alpha1.FinalizerName}
			s.Spec.NamespaceRef = awsv1alpha1.CloudMapNamespaceRef{Name: "my-namespace"}
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.CloudMapService{}, &awsv1alpha1.CloudMapNamespace{}).
			WithObjects(svc, nsCR).Build()
		r := &CloudMapServiceReconciler{Client: c, Scheme: scheme, ServiceDiscoveryClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.createCalled {
			t.Fatal("expected CreateService to be called with the resolved namespace ID")
		}
	})

	t.Run("delete with finalizer calls DeleteService", func(t *testing.T) {
		ctx := context.Background()
		scheme := cloudMapServiceScheme(t)
		f := &fakeCloudMapServiceAPI{}
		now := metav1.Now()
		svc := cloudMapServiceCR(func(s *awsv1alpha1.CloudMapService) {
			s.Finalizers = []string{awsv1alpha1.FinalizerName}
			s.DeletionTimestamp = &now
			s.Status.ServiceID = testCloudMapServiceID
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.CloudMapService{}).
			WithObjects(svc).Build()
		r := &CloudMapServiceReconciler{Client: c, Scheme: scheme, ServiceDiscoveryClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled {
			t.Fatal("expected DeleteService to be called")
		}
		if aws.ToString(f.deleteInput.Id) != testCloudMapServiceID {
			t.Errorf("delete service ID = %q, want %q", aws.ToString(f.deleteInput.Id), testCloudMapServiceID)
		}
		got := &awsv1alpha1.CloudMapService{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected object gone after finalizer removal, got err=%v", err)
		}
	})

	t.Run("delete without stored ID skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := cloudMapServiceScheme(t)
		f := &fakeCloudMapServiceAPI{}
		now := metav1.Now()
		svc := cloudMapServiceCR(func(s *awsv1alpha1.CloudMapService) {
			s.Finalizers = []string{awsv1alpha1.FinalizerName}
			s.DeletionTimestamp = &now
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.CloudMapService{}).
			WithObjects(svc).Build()
		r := &CloudMapServiceReconciler{Client: c, Scheme: scheme, ServiceDiscoveryClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("expected DeleteService NOT to be called without a stored ID")
		}
	})

	t.Run("abandon annotation skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := cloudMapServiceScheme(t)
		f := &fakeCloudMapServiceAPI{}
		now := metav1.Now()
		svc := cloudMapServiceCR(func(s *awsv1alpha1.CloudMapService) {
			s.Finalizers = []string{awsv1alpha1.FinalizerName}
			s.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			s.DeletionTimestamp = &now
			s.Status.ServiceID = testCloudMapServiceID
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.CloudMapService{}).
			WithObjects(svc).Build()
		r := &CloudMapServiceReconciler{Client: c, Scheme: scheme, ServiceDiscoveryClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("expected DeleteService NOT to be called for abandoned resource")
		}
		got := &awsv1alpha1.CloudMapService{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected object gone after finalizer removal, got err=%v", err)
		}
	})
}
