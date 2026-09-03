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

type fakeCloudMapNamespaceAPI struct {
	getOperation func(ctx context.Context, params *awssd.GetOperationInput) (*awssd.GetOperationOutput, error)

	createPrivateCalled bool
	createPublicCalled  bool
	createHTTPCalled    bool
	deleteCalled        bool
	deleteInput         *awssd.DeleteNamespaceInput
}

func (f *fakeCloudMapNamespaceAPI) CreatePrivateDnsNamespace(_ context.Context, params *awssd.CreatePrivateDnsNamespaceInput, _ ...func(*awssd.Options)) (*awssd.CreatePrivateDnsNamespaceOutput, error) {
	f.createPrivateCalled = true
	if aws.ToString(params.Vpc) == "" {
		return nil, fmt.Errorf("missing VPC")
	}
	return &awssd.CreatePrivateDnsNamespaceOutput{OperationId: aws.String("op-1")}, nil
}

func (f *fakeCloudMapNamespaceAPI) CreatePublicDnsNamespace(_ context.Context, _ *awssd.CreatePublicDnsNamespaceInput, _ ...func(*awssd.Options)) (*awssd.CreatePublicDnsNamespaceOutput, error) {
	f.createPublicCalled = true
	return &awssd.CreatePublicDnsNamespaceOutput{OperationId: aws.String("op-1")}, nil
}

func (f *fakeCloudMapNamespaceAPI) CreateHttpNamespace(_ context.Context, _ *awssd.CreateHttpNamespaceInput, _ ...func(*awssd.Options)) (*awssd.CreateHttpNamespaceOutput, error) {
	f.createHTTPCalled = true
	return &awssd.CreateHttpNamespaceOutput{OperationId: aws.String("op-1")}, nil
}

func (f *fakeCloudMapNamespaceAPI) GetOperation(ctx context.Context, params *awssd.GetOperationInput, _ ...func(*awssd.Options)) (*awssd.GetOperationOutput, error) {
	if f.getOperation == nil {
		return nil, fmt.Errorf("unexpected call to GetOperation")
	}
	return f.getOperation(ctx, params)
}

func (f *fakeCloudMapNamespaceAPI) GetNamespace(_ context.Context, params *awssd.GetNamespaceInput, _ ...func(*awssd.Options)) (*awssd.GetNamespaceOutput, error) {
	return &awssd.GetNamespaceOutput{Namespace: &sdtypes.Namespace{
		Id:  params.Id,
		Arn: aws.String("arn:aws:servicediscovery:us-east-1:123456789012:namespace/" + aws.ToString(params.Id)),
	}}, nil
}

func (f *fakeCloudMapNamespaceAPI) DeleteNamespace(_ context.Context, params *awssd.DeleteNamespaceInput, _ ...func(*awssd.Options)) (*awssd.DeleteNamespaceOutput, error) {
	f.deleteCalled = true
	f.deleteInput = params
	return &awssd.DeleteNamespaceOutput{OperationId: aws.String("op-del")}, nil
}

func (f *fakeCloudMapNamespaceAPI) ListNamespaces(_ context.Context, _ *awssd.ListNamespacesInput, _ ...func(*awssd.Options)) (*awssd.ListNamespacesOutput, error) {
	return &awssd.ListNamespacesOutput{}, nil
}

func cloudMapNamespaceCR(mutate ...func(*awsv1alpha1.CloudMapNamespace)) *awsv1alpha1.CloudMapNamespace {
	ns := &awsv1alpha1.CloudMapNamespace{
		ObjectMeta: metav1.ObjectMeta{Name: "my-namespace", Namespace: "default"},
		Spec: awsv1alpha1.CloudMapNamespaceSpec{
			Name:   "example.local",
			Type:   "PRIVATE_DNS",
			VPCRef: &awsv1alpha1.VPCResourceRef{ID: "vpc-0abc1234"},
		},
	}
	for _, m := range mutate {
		m(ns)
	}
	return ns
}

func TestCloudMapNamespaceReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-namespace", Namespace: "default"}}
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}

	t.Run("create submits operation then resolves namespace ID via GetOperation", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeCloudMapNamespaceAPI{
			getOperation: func(_ context.Context, params *awssd.GetOperationInput) (*awssd.GetOperationOutput, error) {
				if aws.ToString(params.OperationId) != "op-1" {
					return nil, fmt.Errorf("unexpected operation ID %q", aws.ToString(params.OperationId))
				}
				return &awssd.GetOperationOutput{Operation: &sdtypes.Operation{
					Id:      params.OperationId,
					Status:  sdtypes.OperationStatusSuccess,
					Targets: map[string]string{string(sdtypes.OperationTargetTypeNamespace): testCloudMapNSID},
				}}, nil
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.CloudMapNamespace{}).
			WithObjects(cloudMapNamespaceCR()).Build()
		r := &CloudMapNamespaceReconciler{Client: c, Scheme: scheme, ServiceDiscoveryClient: f}

		// First reconcile: submits the create and persists the operation ID.
		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile 1: %v", err)
		}
		if !f.createPrivateCalled {
			t.Fatal("expected CreatePrivateDnsNamespace to be called")
		}
		if res.RequeueAfter == 0 {
			t.Error("expected polling requeue after create")
		}
		got := &awsv1alpha1.CloudMapNamespace{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.OperationID != "op-1" {
			t.Errorf("status.operationId = %q, want op-1 (must persist right after create)", got.Status.OperationID)
		}

		// Second reconcile: operation SUCCESS → namespace ID stored, Ready=True.
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile 2: %v", err)
		}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.NamespaceID != testCloudMapNSID {
			t.Errorf("status.namespaceId = %q, want %q", got.Status.NamespaceID, testCloudMapNSID)
		}
		cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
		if cond == nil || cond.Status != metav1.ConditionTrue {
			t.Errorf("Ready condition = %+v, want True", cond)
		}
	})

	t.Run("delete with finalizer calls DeleteNamespace", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeCloudMapNamespaceAPI{}
		now := metav1.Now()
		ns := cloudMapNamespaceCR(func(ns *awsv1alpha1.CloudMapNamespace) {
			ns.Finalizers = []string{awsv1alpha1.FinalizerName}
			ns.DeletionTimestamp = &now
			ns.Status.NamespaceID = testCloudMapNSID
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.CloudMapNamespace{}).
			WithObjects(ns).Build()
		r := &CloudMapNamespaceReconciler{Client: c, Scheme: scheme, ServiceDiscoveryClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled {
			t.Fatal("expected DeleteNamespace to be called")
		}
		if aws.ToString(f.deleteInput.Id) != testCloudMapNSID {
			t.Errorf("delete namespace ID = %q, want %q", aws.ToString(f.deleteInput.Id), testCloudMapNSID)
		}
		got := &awsv1alpha1.CloudMapNamespace{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected object gone after finalizer removal, got err=%v", err)
		}
	})

	t.Run("abandon annotation skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeCloudMapNamespaceAPI{}
		now := metav1.Now()
		ns := cloudMapNamespaceCR(func(ns *awsv1alpha1.CloudMapNamespace) {
			ns.Finalizers = []string{awsv1alpha1.FinalizerName}
			ns.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			ns.DeletionTimestamp = &now
			ns.Status.NamespaceID = testCloudMapNSID
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.CloudMapNamespace{}).
			WithObjects(ns).Build()
		r := &CloudMapNamespaceReconciler{Client: c, Scheme: scheme, ServiceDiscoveryClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("expected DeleteNamespace NOT to be called for abandoned resource")
		}
	})
}
