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
	awsbatch "github.com/aws/aws-sdk-go-v2/service/batch"
	batchtypes "github.com/aws/aws-sdk-go-v2/service/batch/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

const testBatchCEARN = "arn:aws:batch:us-east-1:123456789012:compute-environment/my-ce"

type fakeBatchCEAPI struct {
	describe func(ctx context.Context, params *awsbatch.DescribeComputeEnvironmentsInput) (*awsbatch.DescribeComputeEnvironmentsOutput, error)
	create   func(ctx context.Context, params *awsbatch.CreateComputeEnvironmentInput) (*awsbatch.CreateComputeEnvironmentOutput, error)

	createCalled bool
	updateCalled bool
	deleteCalled bool
	updateInput  *awsbatch.UpdateComputeEnvironmentInput
}

func (f *fakeBatchCEAPI) CreateComputeEnvironment(ctx context.Context, params *awsbatch.CreateComputeEnvironmentInput, _ ...func(*awsbatch.Options)) (*awsbatch.CreateComputeEnvironmentOutput, error) {
	f.createCalled = true
	if f.create == nil {
		return nil, fmt.Errorf("unexpected call to CreateComputeEnvironment")
	}
	return f.create(ctx, params)
}

func (f *fakeBatchCEAPI) DescribeComputeEnvironments(ctx context.Context, params *awsbatch.DescribeComputeEnvironmentsInput, _ ...func(*awsbatch.Options)) (*awsbatch.DescribeComputeEnvironmentsOutput, error) {
	if f.describe == nil {
		return &awsbatch.DescribeComputeEnvironmentsOutput{}, nil
	}
	return f.describe(ctx, params)
}

func (f *fakeBatchCEAPI) UpdateComputeEnvironment(_ context.Context, params *awsbatch.UpdateComputeEnvironmentInput, _ ...func(*awsbatch.Options)) (*awsbatch.UpdateComputeEnvironmentOutput, error) {
	f.updateCalled = true
	f.updateInput = params
	return &awsbatch.UpdateComputeEnvironmentOutput{}, nil
}

func (f *fakeBatchCEAPI) DeleteComputeEnvironment(_ context.Context, _ *awsbatch.DeleteComputeEnvironmentInput, _ ...func(*awsbatch.Options)) (*awsbatch.DeleteComputeEnvironmentOutput, error) {
	f.deleteCalled = true
	return &awsbatch.DeleteComputeEnvironmentOutput{}, nil
}

func batchCEScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return scheme
}

func batchCECR(mutate ...func(*awsv1alpha1.BatchComputeEnvironment)) *awsv1alpha1.BatchComputeEnvironment {
	ce := &awsv1alpha1.BatchComputeEnvironment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-ce",
			Namespace:  "default",
			Finalizers: []string{awsv1alpha1.FinalizerName},
		},
		Spec: awsv1alpha1.BatchComputeEnvironmentSpec{
			Name:  "my-ce",
			Type:  "MANAGED",
			State: "ENABLED",
			ComputeResources: &awsv1alpha1.BatchComputeResources{
				Type:     "FARGATE",
				MaxvCpus: 16,
				SubnetRefs: []awsv1alpha1.SubnetRef{
					{ID: "subnet-0aaa"},
				},
				SecurityGroupRefs: []awsv1alpha1.SecurityGroupRef{
					{ID: "sg-0bbb"},
				},
			},
		},
	}
	for _, m := range mutate {
		m(ce)
	}
	return ce
}

func batchCEDescribe(status batchtypes.CEStatus) *awsbatch.DescribeComputeEnvironmentsOutput {
	return &awsbatch.DescribeComputeEnvironmentsOutput{
		ComputeEnvironments: []batchtypes.ComputeEnvironmentDetail{{
			ComputeEnvironmentArn:  aws.String(testBatchCEARN),
			ComputeEnvironmentName: aws.String("my-ce"),
			Status:                 status,
			State:                  batchtypes.CEStateEnabled,
		}},
	}
}

func TestBatchComputeEnvironmentReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-ce", Namespace: "default"}}

	t.Run("create happy path persists ARN then becomes Ready", func(t *testing.T) {
		ctx := context.Background()
		scheme := batchCEScheme(t)
		created := false
		f := &fakeBatchCEAPI{
			describe: func(_ context.Context, _ *awsbatch.DescribeComputeEnvironmentsInput) (*awsbatch.DescribeComputeEnvironmentsOutput, error) {
				if !created {
					return &awsbatch.DescribeComputeEnvironmentsOutput{}, nil
				}
				return batchCEDescribe(batchtypes.CEStatusValid), nil
			},
			create: func(_ context.Context, params *awsbatch.CreateComputeEnvironmentInput) (*awsbatch.CreateComputeEnvironmentOutput, error) {
				if aws.ToString(params.ComputeEnvironmentName) != "my-ce" {
					return nil, fmt.Errorf("unexpected name %q", aws.ToString(params.ComputeEnvironmentName))
				}
				if params.Type != batchtypes.CETypeManaged {
					return nil, fmt.Errorf("unexpected type %q", params.Type)
				}
				if params.ComputeResources == nil || params.ComputeResources.Type != batchtypes.CRTypeFargate {
					return nil, fmt.Errorf("expected FARGATE compute resources")
				}
				if got := params.ComputeResources.Subnets; len(got) != 1 || got[0] != "subnet-0aaa" {
					return nil, fmt.Errorf("unexpected subnets %v", got)
				}
				created = true
				return &awsbatch.CreateComputeEnvironmentOutput{
					ComputeEnvironmentArn:  aws.String(testBatchCEARN),
					ComputeEnvironmentName: params.ComputeEnvironmentName,
				}, nil
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.BatchComputeEnvironment{}).
			WithObjects(batchCECR()).Build()
		r := &BatchComputeEnvironmentReconciler{Client: c, Scheme: scheme, BatchClient: f}

		// First reconcile: creates the compute environment, persists ARN.
		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile 1: %v", err)
		}
		if !f.createCalled {
			t.Fatal("expected CreateComputeEnvironment to be called")
		}
		if res.RequeueAfter == 0 {
			t.Error("expected polling requeue after create")
		}
		got := &awsv1alpha1.BatchComputeEnvironment{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ComputeEnvironmentARN != testBatchCEARN {
			t.Errorf("status.computeEnvironmentArn = %q, want %q (must persist right after create)", got.Status.ComputeEnvironmentARN, testBatchCEARN)
		}
		cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
		if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != "Provisioning" {
			t.Errorf("Ready condition = %+v, want False/Provisioning", cond)
		}

		// Second reconcile: environment is VALID → Ready=True.
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
		if got.Status.Status != "VALID" {
			t.Errorf("status.status = %q, want VALID", got.Status.Status)
		}
	})

	t.Run("identifier persisted when post-create persist fails is retried", func(t *testing.T) {
		// The create path persists the ARN via persistStatus immediately after
		// CreateComputeEnvironment; a subsequent Get must observe it even when
		// the reconcile ends in a polling requeue.
		ctx := context.Background()
		scheme := batchCEScheme(t)
		f := &fakeBatchCEAPI{
			describe: func(_ context.Context, _ *awsbatch.DescribeComputeEnvironmentsInput) (*awsbatch.DescribeComputeEnvironmentsOutput, error) {
				return &awsbatch.DescribeComputeEnvironmentsOutput{}, nil
			},
			create: func(_ context.Context, _ *awsbatch.CreateComputeEnvironmentInput) (*awsbatch.CreateComputeEnvironmentOutput, error) {
				return &awsbatch.CreateComputeEnvironmentOutput{
					ComputeEnvironmentArn: aws.String(testBatchCEARN),
				}, nil
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.BatchComputeEnvironment{}).
			WithObjects(batchCECR(func(ce *awsv1alpha1.BatchComputeEnvironment) {
				ce.Finalizers = []string{awsv1alpha1.FinalizerName}
			})).Build()
		r := &BatchComputeEnvironmentReconciler{Client: c, Scheme: scheme, BatchClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.BatchComputeEnvironment{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ComputeEnvironmentARN != testBatchCEARN {
			t.Errorf("ARN not persisted immediately after create: %q", got.Status.ComputeEnvironmentARN)
		}
	})

	t.Run("steady state does not create or update", func(t *testing.T) {
		ctx := context.Background()
		scheme := batchCEScheme(t)
		f := &fakeBatchCEAPI{
			describe: func(_ context.Context, _ *awsbatch.DescribeComputeEnvironmentsInput) (*awsbatch.DescribeComputeEnvironmentsOutput, error) {
				return batchCEDescribe(batchtypes.CEStatusValid), nil
			},
		}
		ce := batchCECR(func(ce *awsv1alpha1.BatchComputeEnvironment) {
			ce.Finalizers = []string{awsv1alpha1.FinalizerName}
			ce.Generation = 1
			ce.Status.ComputeEnvironmentARN = testBatchCEARN
			ce.Status.ObservedGeneration = 1
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.BatchComputeEnvironment{}).
			WithObjects(ce).Build()
		r := &BatchComputeEnvironmentReconciler{Client: c, Scheme: scheme, BatchClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.createCalled {
			t.Error("expected CreateComputeEnvironment NOT to be called")
		}
		if f.updateCalled {
			t.Error("expected UpdateComputeEnvironment NOT to be called at steady state")
		}
	})

	t.Run("generation change while VALID applies update", func(t *testing.T) {
		ctx := context.Background()
		scheme := batchCEScheme(t)
		f := &fakeBatchCEAPI{
			describe: func(_ context.Context, _ *awsbatch.DescribeComputeEnvironmentsInput) (*awsbatch.DescribeComputeEnvironmentsOutput, error) {
				return batchCEDescribe(batchtypes.CEStatusValid), nil
			},
		}
		ce := batchCECR(func(ce *awsv1alpha1.BatchComputeEnvironment) {
			ce.Finalizers = []string{awsv1alpha1.FinalizerName}
			ce.Generation = 2
			ce.Spec.ComputeResources.MaxvCpus = 32
			ce.Status.ComputeEnvironmentARN = testBatchCEARN
			ce.Status.ObservedGeneration = 1
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.BatchComputeEnvironment{}).
			WithObjects(ce).Build()
		r := &BatchComputeEnvironmentReconciler{Client: c, Scheme: scheme, BatchClient: f}

		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.updateCalled {
			t.Fatal("expected UpdateComputeEnvironment to be called on generation change")
		}
		if aws.ToInt32(f.updateInput.ComputeResources.MaxvCpus) != 32 {
			t.Errorf("update maxvCpus = %d, want 32", aws.ToInt32(f.updateInput.ComputeResources.MaxvCpus))
		}
		if res.RequeueAfter == 0 {
			t.Error("expected polling requeue while UPDATING")
		}
		got := &awsv1alpha1.BatchComputeEnvironment{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ObservedGeneration != 2 {
			t.Errorf("observedGeneration = %d, want 2", got.Status.ObservedGeneration)
		}
	})

	t.Run("dependency not ready requeues without error", func(t *testing.T) {
		ctx := context.Background()
		scheme := batchCEScheme(t)
		f := &fakeBatchCEAPI{
			describe: func(_ context.Context, _ *awsbatch.DescribeComputeEnvironmentsInput) (*awsbatch.DescribeComputeEnvironmentsOutput, error) {
				return &awsbatch.DescribeComputeEnvironmentsOutput{}, nil
			},
		}
		// Subnet CR exists but has no ID yet.
		subnet := &awsv1alpha1.Subnet{
			ObjectMeta: metav1.ObjectMeta{Name: "my-subnet", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
		}
		ce := batchCECR(func(ce *awsv1alpha1.BatchComputeEnvironment) {
			ce.Finalizers = []string{awsv1alpha1.FinalizerName}
			ce.Spec.ComputeResources.SubnetRefs = []awsv1alpha1.SubnetRef{{Name: "my-subnet"}}
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.BatchComputeEnvironment{}, &awsv1alpha1.Subnet{}).
			WithObjects(ce, subnet).Build()
		r := &BatchComputeEnvironmentReconciler{Client: c, Scheme: scheme, BatchClient: f}

		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v (dependencyNotReady must not surface as error)", err)
		}
		if res != requeueDependency {
			t.Errorf("result = %+v, want requeueDependency", res)
		}
		if f.createCalled {
			t.Error("expected CreateComputeEnvironment NOT to be called while dependency is not ready")
		}
	})

	t.Run("delete with finalizer calls DeleteComputeEnvironment", func(t *testing.T) {
		ctx := context.Background()
		scheme := batchCEScheme(t)
		f := &fakeBatchCEAPI{}
		now := metav1.Now()
		ce := batchCECR(func(ce *awsv1alpha1.BatchComputeEnvironment) {
			ce.Finalizers = []string{awsv1alpha1.FinalizerName}
			ce.DeletionTimestamp = &now
			ce.Status.ComputeEnvironmentARN = testBatchCEARN
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.BatchComputeEnvironment{}).
			WithObjects(ce).Build()
		r := &BatchComputeEnvironmentReconciler{Client: c, Scheme: scheme, BatchClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled {
			t.Error("expected DeleteComputeEnvironment to be called")
		}
		got := &awsv1alpha1.BatchComputeEnvironment{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected object gone after finalizer removal, got err=%v", err)
		}
	})

	t.Run("abandon annotation skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := batchCEScheme(t)
		f := &fakeBatchCEAPI{}
		now := metav1.Now()
		ce := batchCECR(func(ce *awsv1alpha1.BatchComputeEnvironment) {
			ce.Finalizers = []string{awsv1alpha1.FinalizerName}
			ce.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			ce.DeletionTimestamp = &now
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.BatchComputeEnvironment{}).
			WithObjects(ce).Build()
		r := &BatchComputeEnvironmentReconciler{Client: c, Scheme: scheme, BatchClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("expected DeleteComputeEnvironment NOT to be called for abandoned resource")
		}
		got := &awsv1alpha1.BatchComputeEnvironment{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected object gone after finalizer removal, got err=%v", err)
		}
	})
}
