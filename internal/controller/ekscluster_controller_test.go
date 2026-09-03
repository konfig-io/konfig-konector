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
	awseks "github.com/aws/aws-sdk-go-v2/service/eks"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

type fakeEKSClusterAPI struct {
	describeCluster func(ctx context.Context, params *awseks.DescribeClusterInput) (*awseks.DescribeClusterOutput, error)
	createCluster   func(ctx context.Context, params *awseks.CreateClusterInput) (*awseks.CreateClusterOutput, error)

	createCalled        bool
	updateConfigCalled  bool
	updateVersionCalled bool
	deleteCalled        bool
	updateVersionInput  *awseks.UpdateClusterVersionInput
}

func (f *fakeEKSClusterAPI) DescribeCluster(ctx context.Context, params *awseks.DescribeClusterInput, _ ...func(*awseks.Options)) (*awseks.DescribeClusterOutput, error) {
	if f.describeCluster == nil {
		return nil, fmt.Errorf("unexpected call to DescribeCluster")
	}
	return f.describeCluster(ctx, params)
}

func (f *fakeEKSClusterAPI) CreateCluster(ctx context.Context, params *awseks.CreateClusterInput, _ ...func(*awseks.Options)) (*awseks.CreateClusterOutput, error) {
	f.createCalled = true
	if f.createCluster == nil {
		return nil, fmt.Errorf("unexpected call to CreateCluster")
	}
	return f.createCluster(ctx, params)
}

func (f *fakeEKSClusterAPI) UpdateClusterConfig(_ context.Context, _ *awseks.UpdateClusterConfigInput, _ ...func(*awseks.Options)) (*awseks.UpdateClusterConfigOutput, error) {
	f.updateConfigCalled = true
	return &awseks.UpdateClusterConfigOutput{}, nil
}

func (f *fakeEKSClusterAPI) UpdateClusterVersion(_ context.Context, params *awseks.UpdateClusterVersionInput, _ ...func(*awseks.Options)) (*awseks.UpdateClusterVersionOutput, error) {
	f.updateVersionCalled = true
	f.updateVersionInput = params
	return &awseks.UpdateClusterVersionOutput{}, nil
}

func (f *fakeEKSClusterAPI) DeleteCluster(_ context.Context, _ *awseks.DeleteClusterInput, _ ...func(*awseks.Options)) (*awseks.DeleteClusterOutput, error) {
	f.deleteCalled = true
	return &awseks.DeleteClusterOutput{}, nil
}

const (
	testEKSClusterARN = "arn:aws:eks:us-east-1:123456789012:cluster/my-cluster"
	testEKSRoleARN    = "arn:aws:iam::123456789012:role/eks-cluster-role"
)

func eksClusterScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return scheme
}

func eksClusterCR(mutate ...func(*awsv1alpha1.EKSCluster)) *awsv1alpha1.EKSCluster {
	cluster := &awsv1alpha1.EKSCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-cluster",
			Namespace: "default",
		},
		Spec: awsv1alpha1.EKSClusterSpec{
			ClusterName: "my-cluster",
			Version:     "1.29",
			RoleArn:     testEKSRoleARN,
			ResourcesVpcConfig: awsv1alpha1.EKSClusterVpcConfig{
				SubnetRefs: []awsv1alpha1.SubnetRef{
					{ID: "subnet-0aaa"},
					{ID: "subnet-0bbb"},
				},
			},
		},
	}
	for _, m := range mutate {
		m(cluster)
	}
	return cluster
}

func eksActiveDescribe(version string) *awseks.DescribeClusterOutput {
	return &awseks.DescribeClusterOutput{
		Cluster: &ekstypes.Cluster{
			Arn:      aws.String(testEKSClusterARN),
			Name:     aws.String("my-cluster"),
			Status:   ekstypes.ClusterStatusActive,
			Version:  aws.String(version),
			Endpoint: aws.String("https://ABCDEF.gr7.us-east-1.eks.amazonaws.com"),
			CertificateAuthority: &ekstypes.Certificate{
				Data: aws.String("Y2EtZGF0YQ=="),
			},
		},
	}
}

func TestEKSClusterReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-cluster", Namespace: "default"}}

	t.Run("create happy path persists ARN then becomes Ready", func(t *testing.T) {
		ctx := context.Background()
		scheme := eksClusterScheme(t)
		f := &fakeEKSClusterAPI{
			createCluster: func(_ context.Context, params *awseks.CreateClusterInput) (*awseks.CreateClusterOutput, error) {
				if aws.ToString(params.Name) != "my-cluster" {
					return nil, fmt.Errorf("unexpected cluster name %q", aws.ToString(params.Name))
				}
				if aws.ToString(params.RoleArn) != testEKSRoleARN {
					return nil, fmt.Errorf("unexpected role %q", aws.ToString(params.RoleArn))
				}
				return &awseks.CreateClusterOutput{Cluster: &ekstypes.Cluster{
					Arn:    aws.String(testEKSClusterARN),
					Status: ekstypes.ClusterStatusCreating,
				}}, nil
			},
			describeCluster: func(_ context.Context, _ *awseks.DescribeClusterInput) (*awseks.DescribeClusterOutput, error) {
				return eksActiveDescribe("1.29"), nil
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.EKSCluster{}).
			WithObjects(eksClusterCR()).Build()
		r := &EKSClusterReconciler{Client: c, Scheme: scheme, EKSClient: f}

		// First reconcile: adds finalizer + creates the cluster, persists ARN.
		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile 1: %v", err)
		}
		if !f.createCalled {
			t.Fatal("expected CreateCluster to be called")
		}
		if res.RequeueAfter == 0 {
			t.Error("expected polling requeue after create")
		}
		got := &awsv1alpha1.EKSCluster{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ClusterArn != testEKSClusterARN {
			t.Errorf("status.clusterArn = %q, want %q (must persist right after CreateCluster)", got.Status.ClusterArn, testEKSClusterARN)
		}
		cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
		if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != "Provisioning" {
			t.Errorf("Ready condition = %+v, want False/Provisioning", cond)
		}

		// Second reconcile: cluster is ACTIVE at the spec version → Ready=True.
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile 2: %v", err)
		}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		cond = apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
		if cond == nil || cond.Status != metav1.ConditionTrue {
			t.Errorf("Ready condition = %+v, want True", cond)
		}
		if got.Status.Endpoint == "" || got.Status.Version != "1.29" {
			t.Errorf("endpoint/version not synced: %q/%q", got.Status.Endpoint, got.Status.Version)
		}
	})

	t.Run("steady state does not create", func(t *testing.T) {
		ctx := context.Background()
		scheme := eksClusterScheme(t)
		f := &fakeEKSClusterAPI{
			describeCluster: func(_ context.Context, _ *awseks.DescribeClusterInput) (*awseks.DescribeClusterOutput, error) {
				return eksActiveDescribe("1.29"), nil
			},
		}
		cluster := eksClusterCR(func(cl *awsv1alpha1.EKSCluster) {
			cl.Finalizers = []string{awsv1alpha1.FinalizerName}
			cl.Generation = 1
			cl.Status.ClusterArn = testEKSClusterARN
			cl.Status.ObservedGeneration = 1
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.EKSCluster{}).
			WithObjects(cluster).Build()
		r := &EKSClusterReconciler{Client: c, Scheme: scheme, EKSClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.createCalled {
			t.Error("expected CreateCluster NOT to be called")
		}
		if f.updateConfigCalled {
			t.Error("expected UpdateClusterConfig NOT to be called at steady state")
		}
		if f.updateVersionCalled {
			t.Error("expected UpdateClusterVersion NOT to be called at steady state")
		}
	})

	t.Run("version upgrade calls UpdateClusterVersion and sets Upgrading", func(t *testing.T) {
		ctx := context.Background()
		scheme := eksClusterScheme(t)
		f := &fakeEKSClusterAPI{
			describeCluster: func(_ context.Context, _ *awseks.DescribeClusterInput) (*awseks.DescribeClusterOutput, error) {
				// Cluster is ACTIVE at 1.29 while the spec wants 1.30.
				return eksActiveDescribe("1.29"), nil
			},
		}
		cluster := eksClusterCR(func(cl *awsv1alpha1.EKSCluster) {
			cl.Finalizers = []string{awsv1alpha1.FinalizerName}
			cl.Generation = 2
			cl.Spec.Version = "1.30"
			cl.Status.ClusterArn = testEKSClusterARN
			cl.Status.ObservedGeneration = 1
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.EKSCluster{}).
			WithObjects(cluster).Build()
		r := &EKSClusterReconciler{Client: c, Scheme: scheme, EKSClient: f}

		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.updateVersionCalled {
			t.Fatal("expected UpdateClusterVersion to be called")
		}
		if aws.ToString(f.updateVersionInput.Version) != "1.30" {
			t.Errorf("UpdateClusterVersion version = %q, want 1.30", aws.ToString(f.updateVersionInput.Version))
		}
		if f.updateConfigCalled {
			t.Error("expected config update NOT to run while an upgrade is in flight")
		}
		if res.RequeueAfter == 0 {
			t.Error("expected polling requeue during upgrade")
		}
		got := &awsv1alpha1.EKSCluster{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
		if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != "Upgrading" {
			t.Errorf("Ready condition = %+v, want False/Upgrading", cond)
		}
	})

	t.Run("generation change while ACTIVE applies config update", func(t *testing.T) {
		ctx := context.Background()
		scheme := eksClusterScheme(t)
		f := &fakeEKSClusterAPI{
			describeCluster: func(_ context.Context, _ *awseks.DescribeClusterInput) (*awseks.DescribeClusterOutput, error) {
				return eksActiveDescribe("1.29"), nil
			},
		}
		cluster := eksClusterCR(func(cl *awsv1alpha1.EKSCluster) {
			cl.Finalizers = []string{awsv1alpha1.FinalizerName}
			cl.Generation = 2
			cl.Status.ClusterArn = testEKSClusterARN
			cl.Status.ObservedGeneration = 1
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.EKSCluster{}).
			WithObjects(cluster).Build()
		r := &EKSClusterReconciler{Client: c, Scheme: scheme, EKSClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.updateConfigCalled {
			t.Error("expected UpdateClusterConfig to be called on generation change")
		}
		got := &awsv1alpha1.EKSCluster{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ObservedGeneration != 2 {
			t.Errorf("observedGeneration = %d, want 2", got.Status.ObservedGeneration)
		}
	})

	t.Run("delete with finalizer calls DeleteCluster", func(t *testing.T) {
		ctx := context.Background()
		scheme := eksClusterScheme(t)
		f := &fakeEKSClusterAPI{}
		now := metav1.Now()
		cluster := eksClusterCR(func(cl *awsv1alpha1.EKSCluster) {
			cl.Finalizers = []string{awsv1alpha1.FinalizerName}
			cl.DeletionTimestamp = &now
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.EKSCluster{}).
			WithObjects(cluster).Build()
		r := &EKSClusterReconciler{Client: c, Scheme: scheme, EKSClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled {
			t.Error("expected DeleteCluster to be called")
		}
		got := &awsv1alpha1.EKSCluster{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected object gone after finalizer removal, got err=%v", err)
		}
	})

	t.Run("abandon annotation skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := eksClusterScheme(t)
		f := &fakeEKSClusterAPI{}
		now := metav1.Now()
		cluster := eksClusterCR(func(cl *awsv1alpha1.EKSCluster) {
			cl.Finalizers = []string{awsv1alpha1.FinalizerName}
			cl.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			cl.DeletionTimestamp = &now
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.EKSCluster{}).
			WithObjects(cluster).Build()
		r := &EKSClusterReconciler{Client: c, Scheme: scheme, EKSClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("expected DeleteCluster NOT to be called for abandoned resource")
		}
		got := &awsv1alpha1.EKSCluster{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected object gone after finalizer removal, got err=%v", err)
		}
	})
}
