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
	awsecr "github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

type fakeECR struct {
	describeRepositories func(ctx context.Context, params *awsecr.DescribeRepositoriesInput) (*awsecr.DescribeRepositoriesOutput, error)
	createRepository     func(ctx context.Context, params *awsecr.CreateRepositoryInput) (*awsecr.CreateRepositoryOutput, error)
	deleteRepository     func(ctx context.Context, params *awsecr.DeleteRepositoryInput) (*awsecr.DeleteRepositoryOutput, error)

	createCalled bool
	createInput  *awsecr.CreateRepositoryInput
	deleteCalled bool
	deletedName  string
}

func (f *fakeECR) DescribeRepositories(ctx context.Context, params *awsecr.DescribeRepositoriesInput, _ ...func(*awsecr.Options)) (*awsecr.DescribeRepositoriesOutput, error) {
	if f.describeRepositories == nil {
		return nil, fmt.Errorf("unexpected call to DescribeRepositories")
	}
	return f.describeRepositories(ctx, params)
}

func (f *fakeECR) CreateRepository(ctx context.Context, params *awsecr.CreateRepositoryInput, _ ...func(*awsecr.Options)) (*awsecr.CreateRepositoryOutput, error) {
	f.createCalled = true
	f.createInput = params
	if f.createRepository == nil {
		return nil, fmt.Errorf("unexpected call to CreateRepository")
	}
	return f.createRepository(ctx, params)
}

func (f *fakeECR) DeleteRepository(ctx context.Context, params *awsecr.DeleteRepositoryInput, _ ...func(*awsecr.Options)) (*awsecr.DeleteRepositoryOutput, error) {
	f.deleteCalled = true
	f.deletedName = aws.ToString(params.RepositoryName)
	if f.deleteRepository == nil {
		return nil, fmt.Errorf("unexpected call to DeleteRepository")
	}
	return f.deleteRepository(ctx, params)
}

const (
	testRepoARN = "arn:aws:ecr:us-east-1:123456789012:repository/my-repo"
	testRepoURI = "123456789012.dkr.ecr.us-east-1.amazonaws.com/my-repo"
)

func ecrRepoCR(mutate ...func(*awsv1alpha1.ECRRepository)) *awsv1alpha1.ECRRepository {
	repo := &awsv1alpha1.ECRRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-repo",
			Namespace:  "default",
			Finalizers: []string{awsv1alpha1.FinalizerName},
		},
		Spec: awsv1alpha1.ECRRepositorySpec{
			RepositoryName:     "my-repo",
			ImageTagMutability: "IMMUTABLE",
			ScanOnPush:         true,
		},
	}
	for _, m := range mutate {
		m(repo)
	}
	return repo
}

func newECRScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return scheme
}

func TestECRRepositoryReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-repo", Namespace: "default"}}

	tests := []struct {
		name    string
		objs    []client.Object
		fake    *fakeECR
		setup   func(t *testing.T, ctx context.Context, c client.Client)
		wantErr bool
		assert  func(t *testing.T, ctx context.Context, c client.Client, f *fakeECR, res ctrl.Result)
	}{
		{
			name: "create happy path persists ARN, URI and Ready",
			objs: []client.Object{ecrRepoCR()},
			fake: &fakeECR{
				describeRepositories: func(_ context.Context, _ *awsecr.DescribeRepositoriesInput) (*awsecr.DescribeRepositoriesOutput, error) {
					return &awsecr.DescribeRepositoriesOutput{}, nil
				},
				createRepository: func(_ context.Context, params *awsecr.CreateRepositoryInput) (*awsecr.CreateRepositoryOutput, error) {
					if aws.ToString(params.RepositoryName) != "my-repo" {
						return nil, fmt.Errorf("unexpected repository name")
					}
					if params.ImageTagMutability != ecrtypes.ImageTagMutabilityImmutable {
						return nil, fmt.Errorf("unexpected tag mutability %q", params.ImageTagMutability)
					}
					if params.ImageScanningConfiguration == nil || !params.ImageScanningConfiguration.ScanOnPush {
						return nil, fmt.Errorf("expected scanOnPush")
					}
					return &awsecr.CreateRepositoryOutput{
						Repository: &ecrtypes.Repository{
							RepositoryArn: aws.String(testRepoARN),
							RepositoryUri: aws.String(testRepoURI),
						},
					}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeECR, _ ctrl.Result) {
				got := &awsv1alpha1.ECRRepository{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if !f.createCalled {
					t.Error("expected CreateRepository to be called")
				}
				if got.Status.ARN != testRepoARN {
					t.Errorf("status.arn = %q, want %q", got.Status.ARN, testRepoARN)
				}
				if got.Status.RepositoryURI != testRepoURI {
					t.Errorf("status.repositoryUri = %q, want %q", got.Status.RepositoryURI, testRepoURI)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionTrue {
					t.Errorf("Ready condition = %+v, want True", cond)
				}
			},
		},
		{
			name: "steady state does not create",
			objs: []client.Object{ecrRepoCR(func(repo *awsv1alpha1.ECRRepository) {
				repo.Finalizers = []string{awsv1alpha1.FinalizerName}
				repo.Status.ARN = testRepoARN
				repo.Status.RepositoryURI = testRepoURI
			})},
			fake: &fakeECR{
				describeRepositories: func(_ context.Context, params *awsecr.DescribeRepositoriesInput) (*awsecr.DescribeRepositoriesOutput, error) {
					if len(params.RepositoryNames) != 1 || params.RepositoryNames[0] != "my-repo" {
						return nil, fmt.Errorf("unexpected repository names %v", params.RepositoryNames)
					}
					return &awsecr.DescribeRepositoriesOutput{
						Repositories: []ecrtypes.Repository{{
							RepositoryArn: aws.String(testRepoARN),
							RepositoryUri: aws.String(testRepoURI),
						}},
					}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeECR, _ ctrl.Result) {
				if f.createCalled {
					t.Error("CreateRepository must not be called in steady state")
				}
				got := &awsv1alpha1.ECRRepository{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionTrue {
					t.Errorf("Ready condition = %+v, want True", cond)
				}
			},
		},
		{
			name: "recreates when repo disappeared from AWS",
			objs: []client.Object{ecrRepoCR(func(repo *awsv1alpha1.ECRRepository) {
				repo.Finalizers = []string{awsv1alpha1.FinalizerName}
				repo.Status.ARN = testRepoARN
			})},
			fake: &fakeECR{
				describeRepositories: func(_ context.Context, _ *awsecr.DescribeRepositoriesInput) (*awsecr.DescribeRepositoriesOutput, error) {
					return &awsecr.DescribeRepositoriesOutput{Repositories: nil}, nil
				},
				createRepository: func(_ context.Context, _ *awsecr.CreateRepositoryInput) (*awsecr.CreateRepositoryOutput, error) {
					return &awsecr.CreateRepositoryOutput{
						Repository: &ecrtypes.Repository{
							RepositoryArn: aws.String(testRepoARN),
							RepositoryUri: aws.String(testRepoURI),
						},
					}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeECR, _ ctrl.Result) {
				if !f.createCalled {
					t.Error("expected CreateRepository to be called when AWS resource vanished")
				}
				got := &awsv1alpha1.ECRRepository{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.ARN != testRepoARN {
					t.Errorf("status.arn = %q, want %q", got.Status.ARN, testRepoARN)
				}
			},
		},
		{
			name: "create failure surfaces error and Ready=False",
			objs: []client.Object{ecrRepoCR()},
			fake: &fakeECR{
				createRepository: func(_ context.Context, _ *awsecr.CreateRepositoryInput) (*awsecr.CreateRepositoryOutput, error) {
					return nil, fmt.Errorf("access denied")
				},
			},
			wantErr: true,
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeECR, _ ctrl.Result) {
				got := &awsv1alpha1.ECRRepository{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionFalse {
					t.Errorf("Ready condition = %+v, want False", cond)
				}
				if got.Status.ARN != "" {
					t.Errorf("status.arn = %q, want empty (nothing was created)", got.Status.ARN)
				}
			},
		},
		{
			name: "delete with finalizer calls AWS delete with repository name",
			objs: []client.Object{ecrRepoCR(func(repo *awsv1alpha1.ECRRepository) {
				repo.Finalizers = []string{awsv1alpha1.FinalizerName}
				repo.Status.ARN = testRepoARN
			})},
			fake: &fakeECR{
				deleteRepository: func(_ context.Context, params *awsecr.DeleteRepositoryInput) (*awsecr.DeleteRepositoryOutput, error) {
					if !params.Force {
						return nil, fmt.Errorf("expected force delete")
					}
					return &awsecr.DeleteRepositoryOutput{}, nil
				},
			},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, ecrRepoCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeECR, _ ctrl.Result) {
				if !f.deleteCalled {
					t.Error("expected DeleteRepository to be called")
				}
				if f.deletedName != "my-repo" {
					t.Errorf("DeleteRepository name = %q, want %q", f.deletedName, "my-repo")
				}
				got := &awsv1alpha1.ECRRepository{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "delete uses spec name even when status ARN empty (fallback)",
			objs: []client.Object{ecrRepoCR(func(repo *awsv1alpha1.ECRRepository) {
				repo.Finalizers = []string{awsv1alpha1.FinalizerName}
			})},
			fake: &fakeECR{
				deleteRepository: func(_ context.Context, _ *awsecr.DeleteRepositoryInput) (*awsecr.DeleteRepositoryOutput, error) {
					return &awsecr.DeleteRepositoryOutput{}, nil
				},
			},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, ecrRepoCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeECR, _ ctrl.Result) {
				if !f.deleteCalled {
					t.Error("expected DeleteRepository to be called via spec name")
				}
				if f.deletedName != "my-repo" {
					t.Errorf("DeleteRepository name = %q, want %q", f.deletedName, "my-repo")
				}
			},
		},
		{
			name: "abandon policy skips AWS delete",
			objs: []client.Object{ecrRepoCR(func(repo *awsv1alpha1.ECRRepository) {
				repo.Finalizers = []string{awsv1alpha1.FinalizerName}
				repo.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
				repo.Status.ARN = testRepoARN
			})},
			fake: &fakeECR{},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, ecrRepoCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeECR, _ ctrl.Result) {
				if f.deleteCalled {
					t.Error("DeleteRepository must not be called when abandoning")
				}
				got := &awsv1alpha1.ECRRepository{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			scheme := newECRScheme(t)
			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&awsv1alpha1.ECRRepository{}).
				WithObjects(tc.objs...).
				Build()
			r := &ECRRepositoryReconciler{Client: c, Scheme: scheme, ECRClient: tc.fake}

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
