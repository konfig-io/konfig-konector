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

	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
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

// fakeS3BucketAPI implements S3BucketAWSAPI with overridable behavior.
type fakeS3BucketAPI struct {
	headBucket          func(ctx context.Context, params *awss3.HeadBucketInput) (*awss3.HeadBucketOutput, error)
	createBucket        func(ctx context.Context, params *awss3.CreateBucketInput) (*awss3.CreateBucketOutput, error)
	putBucketVersioning func(ctx context.Context, params *awss3.PutBucketVersioningInput) (*awss3.PutBucketVersioningOutput, error)

	createCalled bool
	deleteCalled bool
	putCalls     map[string]int
}

func (f *fakeS3BucketAPI) record(name string) {
	if f.putCalls == nil {
		f.putCalls = map[string]int{}
	}
	f.putCalls[name]++
}

func (f *fakeS3BucketAPI) HeadBucket(ctx context.Context, params *awss3.HeadBucketInput, _ ...func(*awss3.Options)) (*awss3.HeadBucketOutput, error) {
	f.record("HeadBucket")
	if f.headBucket != nil {
		return f.headBucket(ctx, params)
	}
	return &awss3.HeadBucketOutput{}, nil
}

func (f *fakeS3BucketAPI) CreateBucket(ctx context.Context, params *awss3.CreateBucketInput, _ ...func(*awss3.Options)) (*awss3.CreateBucketOutput, error) {
	f.createCalled = true
	f.record("CreateBucket")
	if f.createBucket != nil {
		return f.createBucket(ctx, params)
	}
	return &awss3.CreateBucketOutput{}, nil
}

func (f *fakeS3BucketAPI) PutBucketVersioning(ctx context.Context, params *awss3.PutBucketVersioningInput, _ ...func(*awss3.Options)) (*awss3.PutBucketVersioningOutput, error) {
	f.record("PutBucketVersioning")
	if f.putBucketVersioning != nil {
		return f.putBucketVersioning(ctx, params)
	}
	return &awss3.PutBucketVersioningOutput{}, nil
}

func (f *fakeS3BucketAPI) PutBucketEncryption(context.Context, *awss3.PutBucketEncryptionInput, ...func(*awss3.Options)) (*awss3.PutBucketEncryptionOutput, error) {
	f.record("PutBucketEncryption")
	return &awss3.PutBucketEncryptionOutput{}, nil
}

func (f *fakeS3BucketAPI) PutBucketTagging(context.Context, *awss3.PutBucketTaggingInput, ...func(*awss3.Options)) (*awss3.PutBucketTaggingOutput, error) {
	f.record("PutBucketTagging")
	return &awss3.PutBucketTaggingOutput{}, nil
}

func (f *fakeS3BucketAPI) PutPublicAccessBlock(context.Context, *awss3.PutPublicAccessBlockInput, ...func(*awss3.Options)) (*awss3.PutPublicAccessBlockOutput, error) {
	f.record("PutPublicAccessBlock")
	return &awss3.PutPublicAccessBlockOutput{}, nil
}

func (f *fakeS3BucketAPI) PutBucketLifecycleConfiguration(context.Context, *awss3.PutBucketLifecycleConfigurationInput, ...func(*awss3.Options)) (*awss3.PutBucketLifecycleConfigurationOutput, error) {
	f.record("PutBucketLifecycleConfiguration")
	return &awss3.PutBucketLifecycleConfigurationOutput{}, nil
}

func (f *fakeS3BucketAPI) DeleteBucketLifecycle(context.Context, *awss3.DeleteBucketLifecycleInput, ...func(*awss3.Options)) (*awss3.DeleteBucketLifecycleOutput, error) {
	f.record("DeleteBucketLifecycle")
	return &awss3.DeleteBucketLifecycleOutput{}, nil
}

func (f *fakeS3BucketAPI) PutBucketCors(context.Context, *awss3.PutBucketCorsInput, ...func(*awss3.Options)) (*awss3.PutBucketCorsOutput, error) {
	f.record("PutBucketCors")
	return &awss3.PutBucketCorsOutput{}, nil
}

func (f *fakeS3BucketAPI) DeleteBucketCors(context.Context, *awss3.DeleteBucketCorsInput, ...func(*awss3.Options)) (*awss3.DeleteBucketCorsOutput, error) {
	f.record("DeleteBucketCors")
	return &awss3.DeleteBucketCorsOutput{}, nil
}

func (f *fakeS3BucketAPI) PutBucketNotificationConfiguration(context.Context, *awss3.PutBucketNotificationConfigurationInput, ...func(*awss3.Options)) (*awss3.PutBucketNotificationConfigurationOutput, error) {
	f.record("PutBucketNotificationConfiguration")
	return &awss3.PutBucketNotificationConfigurationOutput{}, nil
}

func (f *fakeS3BucketAPI) PutBucketWebsite(context.Context, *awss3.PutBucketWebsiteInput, ...func(*awss3.Options)) (*awss3.PutBucketWebsiteOutput, error) {
	f.record("PutBucketWebsite")
	return &awss3.PutBucketWebsiteOutput{}, nil
}

func (f *fakeS3BucketAPI) DeleteBucketWebsite(context.Context, *awss3.DeleteBucketWebsiteInput, ...func(*awss3.Options)) (*awss3.DeleteBucketWebsiteOutput, error) {
	f.record("DeleteBucketWebsite")
	return &awss3.DeleteBucketWebsiteOutput{}, nil
}

func (f *fakeS3BucketAPI) PutBucketAccelerateConfiguration(context.Context, *awss3.PutBucketAccelerateConfigurationInput, ...func(*awss3.Options)) (*awss3.PutBucketAccelerateConfigurationOutput, error) {
	f.record("PutBucketAccelerateConfiguration")
	return &awss3.PutBucketAccelerateConfigurationOutput{}, nil
}

func (f *fakeS3BucketAPI) PutObjectLockConfiguration(context.Context, *awss3.PutObjectLockConfigurationInput, ...func(*awss3.Options)) (*awss3.PutObjectLockConfigurationOutput, error) {
	f.record("PutObjectLockConfiguration")
	return &awss3.PutObjectLockConfigurationOutput{}, nil
}

func (f *fakeS3BucketAPI) PutBucketLogging(context.Context, *awss3.PutBucketLoggingInput, ...func(*awss3.Options)) (*awss3.PutBucketLoggingOutput, error) {
	f.record("PutBucketLogging")
	return &awss3.PutBucketLoggingOutput{}, nil
}

func (f *fakeS3BucketAPI) DeleteBucket(context.Context, *awss3.DeleteBucketInput, ...func(*awss3.Options)) (*awss3.DeleteBucketOutput, error) {
	f.deleteCalled = true
	f.record("DeleteBucket")
	return &awss3.DeleteBucketOutput{}, nil
}

func s3BucketNotFoundErr() error {
	return &smithy.GenericAPIError{Code: "NotFound", Message: "bucket not found"}
}

func s3BucketScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return scheme
}

func s3BucketCR(mutate ...func(*awsv1alpha1.S3Bucket)) *awsv1alpha1.S3Bucket {
	b := &awsv1alpha1.S3Bucket{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-bucket",
			Namespace:  "default",
			Finalizers: []string{awsv1alpha1.FinalizerName},
		},
		Spec: awsv1alpha1.S3BucketSpec{
			BucketName: "my-bucket",
			Region:     "us-east-1",
		},
	}
	for _, m := range mutate {
		m(b)
	}
	return b
}

const testS3BucketARN = "arn:aws:s3:::my-bucket"

func TestS3BucketReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-bucket", Namespace: "default"}}

	tests := []struct {
		name       string
		obj        *awsv1alpha1.S3Bucket
		fake       *fakeS3BucketAPI
		reconciles int
		wantErr    bool
		assert     func(t *testing.T, ctx context.Context, c client.Client, f *fakeS3BucketAPI)
	}{
		{
			name: "create happy path persists ARN and Ready=True",
			obj:  s3BucketCR(),
			fake: &fakeS3BucketAPI{
				headBucket: func(_ context.Context, _ *awss3.HeadBucketInput) (*awss3.HeadBucketOutput, error) {
					return nil, s3BucketNotFoundErr()
				},
			},
			reconciles: 1,
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeS3BucketAPI) {
				if !f.createCalled {
					t.Error("expected CreateBucket to be called")
				}
				got := &awsv1alpha1.S3Bucket{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.ARN != testS3BucketARN {
					t.Errorf("status.arn = %q, want %q", got.Status.ARN, testS3BucketARN)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionTrue {
					t.Errorf("Ready condition = %+v, want True", cond)
				}
			},
		},
		{
			name: "ARN persisted even when a post-create Put step fails",
			obj: s3BucketCR(func(b *awsv1alpha1.S3Bucket) {
				b.Spec.Versioning = true
			}),
			fake: &fakeS3BucketAPI{
				headBucket: func(_ context.Context, _ *awss3.HeadBucketInput) (*awss3.HeadBucketOutput, error) {
					return nil, s3BucketNotFoundErr()
				},
				putBucketVersioning: func(_ context.Context, _ *awss3.PutBucketVersioningInput) (*awss3.PutBucketVersioningOutput, error) {
					return nil, fmt.Errorf("versioning boom")
				},
			},
			reconciles: 1,
			wantErr:    true,
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeS3BucketAPI) {
				if !f.createCalled {
					t.Error("expected CreateBucket to be called")
				}
				got := &awsv1alpha1.S3Bucket{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.ARN != testS3BucketARN {
					t.Errorf("status.arn = %q, want %q (identifier must survive later-step failure)", got.Status.ARN, testS3BucketARN)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionFalse {
					t.Errorf("Ready condition = %+v, want False", cond)
				}
			},
		},
		{
			name: "steady state does not call create",
			obj: s3BucketCR(func(b *awsv1alpha1.S3Bucket) {
				b.Finalizers = []string{awsv1alpha1.FinalizerName}
				b.Status.ARN = testS3BucketARN
			}),
			fake:       &fakeS3BucketAPI{}, // HeadBucket default: found
			reconciles: 1,
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeS3BucketAPI) {
				if f.createCalled {
					t.Error("expected CreateBucket NOT to be called")
				}
				got := &awsv1alpha1.S3Bucket{}
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
			name: "delete with finalizer calls AWS delete and removes finalizer",
			obj: s3BucketCR(func(b *awsv1alpha1.S3Bucket) {
				b.Finalizers = []string{awsv1alpha1.FinalizerName}
				now := metav1.Now()
				b.DeletionTimestamp = &now
			}),
			fake:       &fakeS3BucketAPI{},
			reconciles: 1,
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeS3BucketAPI) {
				if !f.deleteCalled {
					t.Error("expected DeleteBucket to be called")
				}
				got := &awsv1alpha1.S3Bucket{}
				err := c.Get(ctx, req.NamespacedName, got)
				if !apierrors.IsNotFound(err) {
					t.Errorf("expected object gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "abandon annotation skips AWS delete but removes finalizer",
			obj: s3BucketCR(func(b *awsv1alpha1.S3Bucket) {
				b.Finalizers = []string{awsv1alpha1.FinalizerName}
				b.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
				now := metav1.Now()
				b.DeletionTimestamp = &now
			}),
			fake:       &fakeS3BucketAPI{},
			reconciles: 1,
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeS3BucketAPI) {
				if f.deleteCalled {
					t.Error("expected DeleteBucket NOT to be called for abandoned resource")
				}
				got := &awsv1alpha1.S3Bucket{}
				err := c.Get(ctx, req.NamespacedName, got)
				if !apierrors.IsNotFound(err) {
					t.Errorf("expected object gone after finalizer removal, got err=%v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			scheme := s3BucketScheme(t)
			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&awsv1alpha1.S3Bucket{}).
				WithObjects(tt.obj).
				Build()

			r := &S3BucketReconciler{Client: c, Scheme: scheme, S3Client: tt.fake}

			var lastErr error
			for i := 0; i < tt.reconciles; i++ {
				_, lastErr = r.Reconcile(ctx, req)
			}
			if tt.wantErr && lastErr == nil {
				t.Fatal("expected reconcile error, got nil")
			}
			if !tt.wantErr && lastErr != nil {
				t.Fatalf("unexpected reconcile error: %v", lastErr)
			}
			tt.assert(t, ctx, c, tt.fake)
		})
	}
}
