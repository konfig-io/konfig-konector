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

const testBatchJDARN = "arn:aws:batch:us-east-1:123456789012:job-definition/my-job:1"

type fakeBatchJDAPI struct {
	registerCalled   bool
	deregisterCalled bool
	registerInput    *awsbatch.RegisterJobDefinitionInput
	deregisterInput  *awsbatch.DeregisterJobDefinitionInput
	revision         int32
}

func (f *fakeBatchJDAPI) RegisterJobDefinition(_ context.Context, params *awsbatch.RegisterJobDefinitionInput, _ ...func(*awsbatch.Options)) (*awsbatch.RegisterJobDefinitionOutput, error) {
	f.registerCalled = true
	f.registerInput = params
	f.revision++
	return &awsbatch.RegisterJobDefinitionOutput{
		JobDefinitionArn:  aws.String(fmt.Sprintf("arn:aws:batch:us-east-1:123456789012:job-definition/my-job:%d", f.revision)),
		JobDefinitionName: params.JobDefinitionName,
		Revision:          aws.Int32(f.revision),
	}, nil
}

func (f *fakeBatchJDAPI) DeregisterJobDefinition(_ context.Context, params *awsbatch.DeregisterJobDefinitionInput, _ ...func(*awsbatch.Options)) (*awsbatch.DeregisterJobDefinitionOutput, error) {
	f.deregisterCalled = true
	f.deregisterInput = params
	return &awsbatch.DeregisterJobDefinitionOutput{}, nil
}

func batchJDCR(mutate ...func(*awsv1alpha1.BatchJobDefinition)) *awsv1alpha1.BatchJobDefinition {
	jd := &awsv1alpha1.BatchJobDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "my-job", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec: awsv1alpha1.BatchJobDefinitionSpec{
			Name: "my-job",
			Type: "container",
			ContainerProperties: awsv1alpha1.BatchContainerProperties{
				Image: "public.ecr.aws/docker/library/busybox:latest",
				ResourceRequirements: []awsv1alpha1.BatchResourceRequirement{
					{Type: "VCPU", Value: "1"},
					{Type: "MEMORY", Value: "2048"},
				},
				Command: []string{"echo", "hello"},
			},
			PlatformCapabilities: []string{"FARGATE"},
		},
	}
	for _, m := range mutate {
		m(jd)
	}
	return jd
}

func TestBatchJobDefinitionReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-job", Namespace: "default"}}
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}

	t.Run("create registers a revision and persists ARN", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeBatchJDAPI{}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.BatchJobDefinition{}).
			WithObjects(batchJDCR()).Build()
		r := &BatchJobDefinitionReconciler{Client: c, Scheme: scheme, BatchClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.registerCalled {
			t.Fatal("expected RegisterJobDefinition to be called")
		}
		if got := len(f.registerInput.ContainerProperties.ResourceRequirements); got != 2 {
			t.Errorf("resource requirements = %d, want 2", got)
		}
		got := &awsv1alpha1.BatchJobDefinition{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.JobDefinitionARN != testBatchJDARN {
			t.Errorf("status.jobDefinitionArn = %q, want %q", got.Status.JobDefinitionARN, testBatchJDARN)
		}
		if got.Status.Revision != 1 {
			t.Errorf("status.revision = %d, want 1", got.Status.Revision)
		}

		// A second reconcile with an unchanged spec must not register a new revision.
		f.registerCalled = false
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile 2: %v", err)
		}
		if f.registerCalled {
			t.Error("expected no new revision for unchanged spec")
		}
	})

	t.Run("delete with finalizer deregisters the revision", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeBatchJDAPI{}
		now := metav1.Now()
		jd := batchJDCR(func(jd *awsv1alpha1.BatchJobDefinition) {
			jd.Finalizers = []string{awsv1alpha1.FinalizerName}
			jd.DeletionTimestamp = &now
			jd.Status.JobDefinitionARN = testBatchJDARN
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.BatchJobDefinition{}).
			WithObjects(jd).Build()
		r := &BatchJobDefinitionReconciler{Client: c, Scheme: scheme, BatchClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deregisterCalled {
			t.Fatal("expected DeregisterJobDefinition to be called")
		}
		if aws.ToString(f.deregisterInput.JobDefinition) != testBatchJDARN {
			t.Errorf("deregister ARN = %q, want %q", aws.ToString(f.deregisterInput.JobDefinition), testBatchJDARN)
		}
		got := &awsv1alpha1.BatchJobDefinition{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected object gone after finalizer removal, got err=%v", err)
		}
	})

	t.Run("abandon annotation skips AWS deregister", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeBatchJDAPI{}
		now := metav1.Now()
		jd := batchJDCR(func(jd *awsv1alpha1.BatchJobDefinition) {
			jd.Finalizers = []string{awsv1alpha1.FinalizerName}
			jd.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			jd.DeletionTimestamp = &now
			jd.Status.JobDefinitionARN = testBatchJDARN
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.BatchJobDefinition{}).
			WithObjects(jd).Build()
		r := &BatchJobDefinitionReconciler{Client: c, Scheme: scheme, BatchClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deregisterCalled {
			t.Error("expected DeregisterJobDefinition NOT to be called for abandoned resource")
		}
	})
}
