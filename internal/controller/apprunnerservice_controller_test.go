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
	awsapprunner "github.com/aws/aws-sdk-go-v2/service/apprunner"
	apprunnertypes "github.com/aws/aws-sdk-go-v2/service/apprunner/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

const testAppRunnerServiceARN = "arn:aws:apprunner:us-east-1:123456789012:service/my-service/abc123"

type fakeAppRunnerServiceAPI struct {
	create func(ctx context.Context, params *awsapprunner.CreateServiceInput) (*awsapprunner.CreateServiceOutput, error)

	createCalled bool
	deleteCalled bool
	deleteInput  *awsapprunner.DeleteServiceInput
}

func (f *fakeAppRunnerServiceAPI) CreateService(ctx context.Context, params *awsapprunner.CreateServiceInput, _ ...func(*awsapprunner.Options)) (*awsapprunner.CreateServiceOutput, error) {
	f.createCalled = true
	if f.create == nil {
		return nil, fmt.Errorf("unexpected call to CreateService")
	}
	return f.create(ctx, params)
}

func (f *fakeAppRunnerServiceAPI) DescribeService(_ context.Context, _ *awsapprunner.DescribeServiceInput, _ ...func(*awsapprunner.Options)) (*awsapprunner.DescribeServiceOutput, error) {
	return nil, fmt.Errorf("unexpected call to DescribeService")
}

func (f *fakeAppRunnerServiceAPI) UpdateService(_ context.Context, _ *awsapprunner.UpdateServiceInput, _ ...func(*awsapprunner.Options)) (*awsapprunner.UpdateServiceOutput, error) {
	return nil, fmt.Errorf("unexpected call to UpdateService")
}

func (f *fakeAppRunnerServiceAPI) DeleteService(_ context.Context, params *awsapprunner.DeleteServiceInput, _ ...func(*awsapprunner.Options)) (*awsapprunner.DeleteServiceOutput, error) {
	f.deleteCalled = true
	f.deleteInput = params
	return &awsapprunner.DeleteServiceOutput{}, nil
}

func (f *fakeAppRunnerServiceAPI) ListServices(_ context.Context, _ *awsapprunner.ListServicesInput, _ ...func(*awsapprunner.Options)) (*awsapprunner.ListServicesOutput, error) {
	return &awsapprunner.ListServicesOutput{}, nil
}

func appRunnerServiceCR(mutate ...func(*awsv1alpha1.AppRunnerService)) *awsv1alpha1.AppRunnerService {
	svc := &awsv1alpha1.AppRunnerService{
		ObjectMeta: metav1.ObjectMeta{Name: "my-service", Namespace: "default"},
		Spec: awsv1alpha1.AppRunnerServiceSpec{
			ServiceName: "my-service",
			SourceConfiguration: awsv1alpha1.AppRunnerSourceConfiguration{
				ImageRepository: awsv1alpha1.AppRunnerImageRepository{
					ImageIdentifier:     "public.ecr.aws/aws-containers/hello-app-runner:latest",
					ImageRepositoryType: "ECR_PUBLIC",
					ImageConfiguration: &awsv1alpha1.AppRunnerImageConfiguration{
						Port: "8080",
					},
				},
			},
		},
	}
	for _, m := range mutate {
		m(svc)
	}
	return svc
}

func TestAppRunnerServiceReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-service", Namespace: "default"}}
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}

	t.Run("create persists ARN and polls", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeAppRunnerServiceAPI{
			create: func(_ context.Context, params *awsapprunner.CreateServiceInput) (*awsapprunner.CreateServiceOutput, error) {
				if aws.ToString(params.ServiceName) != "my-service" {
					return nil, fmt.Errorf("unexpected service name %q", aws.ToString(params.ServiceName))
				}
				repo := params.SourceConfiguration.ImageRepository
				if repo == nil || repo.ImageRepositoryType != apprunnertypes.ImageRepositoryTypeEcrPublic {
					return nil, fmt.Errorf("unexpected image repository %+v", repo)
				}
				return &awsapprunner.CreateServiceOutput{
					OperationId: aws.String("op-1"),
					Service: &apprunnertypes.Service{
						ServiceArn:  aws.String(testAppRunnerServiceARN),
						ServiceId:   aws.String("abc123"),
						ServiceName: params.ServiceName,
						Status:      apprunnertypes.ServiceStatusOperationInProgress,
					},
				}, nil
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.AppRunnerService{}).
			WithObjects(appRunnerServiceCR()).Build()
		r := &AppRunnerServiceReconciler{Client: c, Scheme: scheme, AppRunnerClient: f}

		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.createCalled {
			t.Fatal("expected CreateService to be called")
		}
		if res.RequeueAfter == 0 {
			t.Error("expected polling requeue after create")
		}
		got := &awsv1alpha1.AppRunnerService{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ServiceARN != testAppRunnerServiceARN {
			t.Errorf("status.serviceArn = %q, want %q (must persist right after CreateService)", got.Status.ServiceARN, testAppRunnerServiceARN)
		}
	})

	t.Run("delete with finalizer calls DeleteService", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeAppRunnerServiceAPI{}
		now := metav1.Now()
		svc := appRunnerServiceCR(func(s *awsv1alpha1.AppRunnerService) {
			s.Finalizers = []string{awsv1alpha1.FinalizerName}
			s.DeletionTimestamp = &now
			s.Status.ServiceARN = testAppRunnerServiceARN
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.AppRunnerService{}).
			WithObjects(svc).Build()
		r := &AppRunnerServiceReconciler{Client: c, Scheme: scheme, AppRunnerClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled {
			t.Fatal("expected DeleteService to be called")
		}
		if aws.ToString(f.deleteInput.ServiceArn) != testAppRunnerServiceARN {
			t.Errorf("delete ARN = %q, want %q", aws.ToString(f.deleteInput.ServiceArn), testAppRunnerServiceARN)
		}
		got := &awsv1alpha1.AppRunnerService{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected object gone after finalizer removal, got err=%v", err)
		}
	})

	t.Run("abandon annotation skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeAppRunnerServiceAPI{}
		now := metav1.Now()
		svc := appRunnerServiceCR(func(s *awsv1alpha1.AppRunnerService) {
			s.Finalizers = []string{awsv1alpha1.FinalizerName}
			s.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			s.DeletionTimestamp = &now
			s.Status.ServiceARN = testAppRunnerServiceARN
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.AppRunnerService{}).
			WithObjects(svc).Build()
		r := &AppRunnerServiceReconciler{Client: c, Scheme: scheme, AppRunnerClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("expected DeleteService NOT to be called for abandoned resource")
		}
	})
}
