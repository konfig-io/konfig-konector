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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

const testBatchJQARN = "arn:aws:batch:us-east-1:123456789012:job-queue/my-queue"

type fakeBatchJQAPI struct {
	create func(ctx context.Context, params *awsbatch.CreateJobQueueInput) (*awsbatch.CreateJobQueueOutput, error)

	createCalled bool
	deleteCalled bool
	deleteInput  *awsbatch.DeleteJobQueueInput
}

func (f *fakeBatchJQAPI) CreateJobQueue(ctx context.Context, params *awsbatch.CreateJobQueueInput, _ ...func(*awsbatch.Options)) (*awsbatch.CreateJobQueueOutput, error) {
	f.createCalled = true
	if f.create == nil {
		return nil, fmt.Errorf("unexpected call to CreateJobQueue")
	}
	return f.create(ctx, params)
}

func (f *fakeBatchJQAPI) DescribeJobQueues(_ context.Context, _ *awsbatch.DescribeJobQueuesInput, _ ...func(*awsbatch.Options)) (*awsbatch.DescribeJobQueuesOutput, error) {
	return &awsbatch.DescribeJobQueuesOutput{}, nil
}

func (f *fakeBatchJQAPI) UpdateJobQueue(_ context.Context, _ *awsbatch.UpdateJobQueueInput, _ ...func(*awsbatch.Options)) (*awsbatch.UpdateJobQueueOutput, error) {
	return nil, fmt.Errorf("unexpected call to UpdateJobQueue")
}

func (f *fakeBatchJQAPI) DeleteJobQueue(_ context.Context, params *awsbatch.DeleteJobQueueInput, _ ...func(*awsbatch.Options)) (*awsbatch.DeleteJobQueueOutput, error) {
	f.deleteCalled = true
	f.deleteInput = params
	return &awsbatch.DeleteJobQueueOutput{}, nil
}

func batchJQCR(mutate ...func(*awsv1alpha1.BatchJobQueue)) *awsv1alpha1.BatchJobQueue {
	jq := &awsv1alpha1.BatchJobQueue{
		ObjectMeta: metav1.ObjectMeta{Name: "my-queue", Namespace: "default"},
		Spec: awsv1alpha1.BatchJobQueueSpec{
			Name:     "my-queue",
			Priority: 10,
			ComputeEnvironmentOrder: []awsv1alpha1.BatchComputeEnvironmentOrder{{
				ComputeEnvironmentRef: awsv1alpha1.BatchComputeEnvironmentRef{ARN: testBatchCEARN},
				Order:                 1,
			}},
		},
	}
	for _, m := range mutate {
		m(jq)
	}
	return jq
}

func TestBatchJobQueueReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-queue", Namespace: "default"}}
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}

	t.Run("create persists ARN and maps compute environment order", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeBatchJQAPI{
			create: func(_ context.Context, params *awsbatch.CreateJobQueueInput) (*awsbatch.CreateJobQueueOutput, error) {
				if aws.ToString(params.JobQueueName) != "my-queue" {
					return nil, fmt.Errorf("unexpected name %q", aws.ToString(params.JobQueueName))
				}
				if len(params.ComputeEnvironmentOrder) != 1 || aws.ToString(params.ComputeEnvironmentOrder[0].ComputeEnvironment) != testBatchCEARN {
					return nil, fmt.Errorf("unexpected compute environment order %+v", params.ComputeEnvironmentOrder)
				}
				return &awsbatch.CreateJobQueueOutput{
					JobQueueArn:  aws.String(testBatchJQARN),
					JobQueueName: params.JobQueueName,
				}, nil
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.BatchJobQueue{}).
			WithObjects(batchJQCR()).Build()
		r := &BatchJobQueueReconciler{Client: c, Scheme: scheme, BatchClient: f}

		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.createCalled {
			t.Fatal("expected CreateJobQueue to be called")
		}
		if res.RequeueAfter == 0 {
			t.Error("expected polling requeue after create")
		}
		got := &awsv1alpha1.BatchJobQueue{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.JobQueueARN != testBatchJQARN {
			t.Errorf("status.jobQueueArn = %q, want %q", got.Status.JobQueueARN, testBatchJQARN)
		}
	})

	t.Run("compute environment ref not ready requeues without error", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeBatchJQAPI{}
		ceCR := &awsv1alpha1.BatchComputeEnvironment{
			ObjectMeta: metav1.ObjectMeta{Name: "my-ce", Namespace: "default"},
		}
		jq := batchJQCR(func(jq *awsv1alpha1.BatchJobQueue) {
			jq.Finalizers = []string{awsv1alpha1.FinalizerName}
			jq.Spec.ComputeEnvironmentOrder = []awsv1alpha1.BatchComputeEnvironmentOrder{{
				ComputeEnvironmentRef: awsv1alpha1.BatchComputeEnvironmentRef{Name: "my-ce"},
				Order:                 1,
			}}
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.BatchJobQueue{}, &awsv1alpha1.BatchComputeEnvironment{}).
			WithObjects(jq, ceCR).Build()
		r := &BatchJobQueueReconciler{Client: c, Scheme: scheme, BatchClient: f}

		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v (dependencyNotReady must not surface as error)", err)
		}
		if res != requeueDependency {
			t.Errorf("result = %+v, want requeueDependency", res)
		}
		if f.createCalled {
			t.Error("expected CreateJobQueue NOT to be called while dependency is not ready")
		}
	})

	t.Run("delete with finalizer calls DeleteJobQueue", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeBatchJQAPI{}
		now := metav1.Now()
		jq := batchJQCR(func(jq *awsv1alpha1.BatchJobQueue) {
			jq.Finalizers = []string{awsv1alpha1.FinalizerName}
			jq.DeletionTimestamp = &now
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.BatchJobQueue{}).
			WithObjects(jq).Build()
		r := &BatchJobQueueReconciler{Client: c, Scheme: scheme, BatchClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled {
			t.Fatal("expected DeleteJobQueue to be called")
		}
		if aws.ToString(f.deleteInput.JobQueue) != "my-queue" {
			t.Errorf("delete job queue = %q, want my-queue", aws.ToString(f.deleteInput.JobQueue))
		}
		got := &awsv1alpha1.BatchJobQueue{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected object gone after finalizer removal, got err=%v", err)
		}
	})

	t.Run("abandon annotation skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeBatchJQAPI{}
		now := metav1.Now()
		jq := batchJQCR(func(jq *awsv1alpha1.BatchJobQueue) {
			jq.Finalizers = []string{awsv1alpha1.FinalizerName}
			jq.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			jq.DeletionTimestamp = &now
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.BatchJobQueue{}).
			WithObjects(jq).Build()
		r := &BatchJobQueueReconciler{Client: c, Scheme: scheme, BatchClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("expected DeleteJobQueue NOT to be called for abandoned resource")
		}
	})
}
