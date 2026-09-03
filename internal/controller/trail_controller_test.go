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
	awscloudtrail "github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	cloudtrailtypes "github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
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

type fakeCloudTrail struct {
	getTrail       func(ctx context.Context, params *awscloudtrail.GetTrailInput) (*awscloudtrail.GetTrailOutput, error)
	createTrail    func(ctx context.Context, params *awscloudtrail.CreateTrailInput) (*awscloudtrail.CreateTrailOutput, error)
	updateTrail    func(ctx context.Context, params *awscloudtrail.UpdateTrailInput) (*awscloudtrail.UpdateTrailOutput, error)
	deleteTrail    func(ctx context.Context, params *awscloudtrail.DeleteTrailInput) (*awscloudtrail.DeleteTrailOutput, error)
	startLogging   func(ctx context.Context, params *awscloudtrail.StartLoggingInput) (*awscloudtrail.StartLoggingOutput, error)
	stopLogging    func(ctx context.Context, params *awscloudtrail.StopLoggingInput) (*awscloudtrail.StopLoggingOutput, error)
	getTrailStatus func(ctx context.Context, params *awscloudtrail.GetTrailStatusInput) (*awscloudtrail.GetTrailStatusOutput, error)
	addTags        func(ctx context.Context, params *awscloudtrail.AddTagsInput) (*awscloudtrail.AddTagsOutput, error)

	createCalled       bool
	updateCalled       bool
	deleteCalled       bool
	startLoggingCalled bool
	stopLoggingCalled  bool
	deletedName        string
	createInput        *awscloudtrail.CreateTrailInput
}

func (f *fakeCloudTrail) GetTrail(ctx context.Context, params *awscloudtrail.GetTrailInput, _ ...func(*awscloudtrail.Options)) (*awscloudtrail.GetTrailOutput, error) {
	if f.getTrail == nil {
		return nil, fmt.Errorf("unexpected call to GetTrail")
	}
	return f.getTrail(ctx, params)
}

func (f *fakeCloudTrail) CreateTrail(ctx context.Context, params *awscloudtrail.CreateTrailInput, _ ...func(*awscloudtrail.Options)) (*awscloudtrail.CreateTrailOutput, error) {
	f.createCalled = true
	f.createInput = params
	if f.createTrail == nil {
		return nil, fmt.Errorf("unexpected call to CreateTrail")
	}
	return f.createTrail(ctx, params)
}

func (f *fakeCloudTrail) UpdateTrail(ctx context.Context, params *awscloudtrail.UpdateTrailInput, _ ...func(*awscloudtrail.Options)) (*awscloudtrail.UpdateTrailOutput, error) {
	f.updateCalled = true
	if f.updateTrail == nil {
		return nil, fmt.Errorf("unexpected call to UpdateTrail")
	}
	return f.updateTrail(ctx, params)
}

func (f *fakeCloudTrail) DeleteTrail(ctx context.Context, params *awscloudtrail.DeleteTrailInput, _ ...func(*awscloudtrail.Options)) (*awscloudtrail.DeleteTrailOutput, error) {
	f.deleteCalled = true
	f.deletedName = aws.ToString(params.Name)
	if f.deleteTrail == nil {
		return nil, fmt.Errorf("unexpected call to DeleteTrail")
	}
	return f.deleteTrail(ctx, params)
}

func (f *fakeCloudTrail) StartLogging(ctx context.Context, params *awscloudtrail.StartLoggingInput, _ ...func(*awscloudtrail.Options)) (*awscloudtrail.StartLoggingOutput, error) {
	f.startLoggingCalled = true
	if f.startLogging == nil {
		return nil, fmt.Errorf("unexpected call to StartLogging")
	}
	return f.startLogging(ctx, params)
}

func (f *fakeCloudTrail) StopLogging(ctx context.Context, params *awscloudtrail.StopLoggingInput, _ ...func(*awscloudtrail.Options)) (*awscloudtrail.StopLoggingOutput, error) {
	f.stopLoggingCalled = true
	if f.stopLogging == nil {
		return nil, fmt.Errorf("unexpected call to StopLogging")
	}
	return f.stopLogging(ctx, params)
}

func (f *fakeCloudTrail) GetTrailStatus(ctx context.Context, params *awscloudtrail.GetTrailStatusInput, _ ...func(*awscloudtrail.Options)) (*awscloudtrail.GetTrailStatusOutput, error) {
	if f.getTrailStatus == nil {
		return nil, fmt.Errorf("unexpected call to GetTrailStatus")
	}
	return f.getTrailStatus(ctx, params)
}

func (f *fakeCloudTrail) AddTags(ctx context.Context, params *awscloudtrail.AddTagsInput, _ ...func(*awscloudtrail.Options)) (*awscloudtrail.AddTagsOutput, error) {
	if f.addTags == nil {
		return nil, fmt.Errorf("unexpected call to AddTags")
	}
	return f.addTags(ctx, params)
}

func trailNotFoundErr() error {
	return &smithy.GenericAPIError{Code: "TrailNotFoundException", Message: "trail not found"}
}

func newGovScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return scheme
}

const (
	testTrailARN = "arn:aws:cloudtrail:us-east-1:123456789012:trail/my-trail"
)

func trailCR(mutate ...func(*awsv1alpha1.Trail)) *awsv1alpha1.Trail {
	tr := &awsv1alpha1.Trail{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-trail",
			Namespace: "default",
		},
		Spec: awsv1alpha1.TrailSpec{
			TrailName:    "my-trail",
			S3BucketName: "my-log-bucket",
			Tags:         map[string]string{"env": "test"},
		},
	}
	for _, m := range mutate {
		m(tr)
	}
	return tr
}

func newTrailFakeClient(scheme *runtime.Scheme, objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&awsv1alpha1.Trail{}).
		WithObjects(objs...).
		Build()
}

func TestTrailReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-trail", Namespace: "default"}}

	tests := []struct {
		name    string
		objs    []client.Object
		fake    *fakeCloudTrail
		setup   func(t *testing.T, ctx context.Context, c client.Client)
		wantErr bool
		assert  func(t *testing.T, ctx context.Context, c client.Client, f *fakeCloudTrail, res ctrl.Result)
	}{
		{
			name: "create happy path persists ARN, starts logging, Ready",
			objs: []client.Object{trailCR()},
			fake: &fakeCloudTrail{
				getTrail: func(_ context.Context, _ *awscloudtrail.GetTrailInput) (*awscloudtrail.GetTrailOutput, error) {
					return nil, trailNotFoundErr()
				},
				createTrail: func(_ context.Context, params *awscloudtrail.CreateTrailInput) (*awscloudtrail.CreateTrailOutput, error) {
					if aws.ToString(params.Name) != "my-trail" {
						return nil, fmt.Errorf("unexpected trail name %q", aws.ToString(params.Name))
					}
					if aws.ToString(params.S3BucketName) != "my-log-bucket" {
						return nil, fmt.Errorf("unexpected bucket %q", aws.ToString(params.S3BucketName))
					}
					return &awscloudtrail.CreateTrailOutput{TrailARN: aws.String(testTrailARN)}, nil
				},
				getTrailStatus: func(_ context.Context, _ *awscloudtrail.GetTrailStatusInput) (*awscloudtrail.GetTrailStatusOutput, error) {
					return &awscloudtrail.GetTrailStatusOutput{IsLogging: aws.Bool(false)}, nil
				},
				startLogging: func(_ context.Context, _ *awscloudtrail.StartLoggingInput) (*awscloudtrail.StartLoggingOutput, error) {
					return &awscloudtrail.StartLoggingOutput{}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeCloudTrail, _ ctrl.Result) {
				got := &awsv1alpha1.Trail{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if !f.createCalled {
					t.Error("expected CreateTrail to be called")
				}
				if !f.startLoggingCalled {
					t.Error("expected StartLogging to be called (enableLogging defaults true)")
				}
				if got.Status.TrailARN != testTrailARN {
					t.Errorf("status.trailArn = %q, want %q", got.Status.TrailARN, testTrailARN)
				}
				if !got.Status.IsLogging {
					t.Error("status.isLogging = false, want true")
				}
				if f.createInput == nil || len(f.createInput.TagsList) != 1 {
					t.Errorf("expected tags on create, got %+v", f.createInput)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionTrue {
					t.Errorf("Ready condition = %+v, want True", cond)
				}
			},
		},
		{
			name: "identifier persisted even when post-create step fails",
			objs: []client.Object{trailCR()},
			fake: &fakeCloudTrail{
				getTrail: func(_ context.Context, _ *awscloudtrail.GetTrailInput) (*awscloudtrail.GetTrailOutput, error) {
					return nil, trailNotFoundErr()
				},
				createTrail: func(_ context.Context, _ *awscloudtrail.CreateTrailInput) (*awscloudtrail.CreateTrailOutput, error) {
					return &awscloudtrail.CreateTrailOutput{TrailARN: aws.String(testTrailARN)}, nil
				},
				getTrailStatus: func(_ context.Context, _ *awscloudtrail.GetTrailStatusInput) (*awscloudtrail.GetTrailStatusOutput, error) {
					return nil, fmt.Errorf("throttled")
				},
			},
			wantErr: true,
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeCloudTrail, _ ctrl.Result) {
				got := &awsv1alpha1.Trail{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.TrailARN != testTrailARN {
					t.Errorf("status.trailArn = %q, want %q (identifier must be persisted before failing step)", got.Status.TrailARN, testTrailARN)
				}
			},
		},
		{
			name: "steady state does not create or update",
			objs: []client.Object{trailCR(func(tr *awsv1alpha1.Trail) {
				tr.Finalizers = []string{awsv1alpha1.FinalizerName}
				tr.Status.TrailARN = testTrailARN
				tr.Status.ObservedGeneration = 1
				tr.Generation = 1
			})},
			fake: &fakeCloudTrail{
				getTrail: func(_ context.Context, _ *awscloudtrail.GetTrailInput) (*awscloudtrail.GetTrailOutput, error) {
					return &awscloudtrail.GetTrailOutput{Trail: &cloudtrailtypes.Trail{
						Name:     aws.String("my-trail"),
						TrailARN: aws.String(testTrailARN),
					}}, nil
				},
				getTrailStatus: func(_ context.Context, _ *awscloudtrail.GetTrailStatusInput) (*awscloudtrail.GetTrailStatusOutput, error) {
					return &awscloudtrail.GetTrailStatusOutput{IsLogging: aws.Bool(true)}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeCloudTrail, _ ctrl.Result) {
				if f.createCalled {
					t.Error("CreateTrail must not be called in steady state")
				}
				if f.updateCalled {
					t.Error("UpdateTrail must not be called when generation is observed")
				}
				if f.startLoggingCalled {
					t.Error("StartLogging must not be called when already logging")
				}
				got := &awsv1alpha1.Trail{}
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
			name: "generation change triggers update",
			objs: []client.Object{trailCR(func(tr *awsv1alpha1.Trail) {
				tr.Finalizers = []string{awsv1alpha1.FinalizerName}
				tr.Status.TrailARN = testTrailARN
				tr.Status.ObservedGeneration = 1
				tr.Generation = 2
			})},
			fake: &fakeCloudTrail{
				getTrail: func(_ context.Context, _ *awscloudtrail.GetTrailInput) (*awscloudtrail.GetTrailOutput, error) {
					return &awscloudtrail.GetTrailOutput{Trail: &cloudtrailtypes.Trail{
						Name:     aws.String("my-trail"),
						TrailARN: aws.String(testTrailARN),
					}}, nil
				},
				updateTrail: func(_ context.Context, params *awscloudtrail.UpdateTrailInput) (*awscloudtrail.UpdateTrailOutput, error) {
					if aws.ToString(params.Name) != testTrailARN {
						return nil, fmt.Errorf("expected update by ARN, got %q", aws.ToString(params.Name))
					}
					return &awscloudtrail.UpdateTrailOutput{TrailARN: aws.String(testTrailARN)}, nil
				},
				addTags: func(_ context.Context, _ *awscloudtrail.AddTagsInput) (*awscloudtrail.AddTagsOutput, error) {
					return &awscloudtrail.AddTagsOutput{}, nil
				},
				getTrailStatus: func(_ context.Context, _ *awscloudtrail.GetTrailStatusInput) (*awscloudtrail.GetTrailStatusOutput, error) {
					return &awscloudtrail.GetTrailStatusOutput{IsLogging: aws.Bool(true)}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeCloudTrail, _ ctrl.Result) {
				if !f.updateCalled {
					t.Error("expected UpdateTrail to be called for new generation")
				}
				got := &awsv1alpha1.Trail{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.ObservedGeneration != 2 {
					t.Errorf("observedGeneration = %d, want 2", got.Status.ObservedGeneration)
				}
			},
		},
		{
			name: "enableLogging false stops logging",
			objs: []client.Object{trailCR(func(tr *awsv1alpha1.Trail) {
				tr.Finalizers = []string{awsv1alpha1.FinalizerName}
				tr.Spec.EnableLogging = aws.Bool(false)
				tr.Status.TrailARN = testTrailARN
				tr.Status.ObservedGeneration = 1
				tr.Generation = 1
			})},
			fake: &fakeCloudTrail{
				getTrail: func(_ context.Context, _ *awscloudtrail.GetTrailInput) (*awscloudtrail.GetTrailOutput, error) {
					return &awscloudtrail.GetTrailOutput{Trail: &cloudtrailtypes.Trail{
						TrailARN: aws.String(testTrailARN),
					}}, nil
				},
				getTrailStatus: func(_ context.Context, _ *awscloudtrail.GetTrailStatusInput) (*awscloudtrail.GetTrailStatusOutput, error) {
					return &awscloudtrail.GetTrailStatusOutput{IsLogging: aws.Bool(true)}, nil
				},
				stopLogging: func(_ context.Context, _ *awscloudtrail.StopLoggingInput) (*awscloudtrail.StopLoggingOutput, error) {
					return &awscloudtrail.StopLoggingOutput{}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeCloudTrail, _ ctrl.Result) {
				if !f.stopLoggingCalled {
					t.Error("expected StopLogging to be called")
				}
				got := &awsv1alpha1.Trail{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.IsLogging {
					t.Error("status.isLogging = true, want false")
				}
			},
		},
		{
			name: "delete with finalizer calls AWS delete with status identifier",
			objs: []client.Object{trailCR(func(tr *awsv1alpha1.Trail) {
				tr.Finalizers = []string{awsv1alpha1.FinalizerName}
				tr.Status.TrailARN = testTrailARN
			})},
			fake: &fakeCloudTrail{
				deleteTrail: func(_ context.Context, _ *awscloudtrail.DeleteTrailInput) (*awscloudtrail.DeleteTrailOutput, error) {
					return &awscloudtrail.DeleteTrailOutput{}, nil
				},
			},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, trailCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeCloudTrail, _ ctrl.Result) {
				if !f.deleteCalled {
					t.Error("expected DeleteTrail to be called")
				}
				if f.deletedName != testTrailARN {
					t.Errorf("DeleteTrail name = %q, want %q", f.deletedName, testTrailARN)
				}
				got := &awsv1alpha1.Trail{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "delete fallback uses spec trail name when status empty",
			objs: []client.Object{trailCR(func(tr *awsv1alpha1.Trail) {
				tr.Finalizers = []string{awsv1alpha1.FinalizerName}
			})},
			fake: &fakeCloudTrail{
				deleteTrail: func(_ context.Context, params *awscloudtrail.DeleteTrailInput) (*awscloudtrail.DeleteTrailOutput, error) {
					if aws.ToString(params.Name) != "my-trail" {
						return nil, fmt.Errorf("unexpected trail name %q", aws.ToString(params.Name))
					}
					return &awscloudtrail.DeleteTrailOutput{}, nil
				},
			},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, trailCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeCloudTrail, _ ctrl.Result) {
				if !f.deleteCalled {
					t.Error("expected DeleteTrail to be called via spec-based fallback")
				}
				if f.deletedName != "my-trail" {
					t.Errorf("DeleteTrail name = %q, want %q", f.deletedName, "my-trail")
				}
			},
		},
		{
			name: "delete tolerates trail already gone",
			objs: []client.Object{trailCR(func(tr *awsv1alpha1.Trail) {
				tr.Finalizers = []string{awsv1alpha1.FinalizerName}
				tr.Status.TrailARN = testTrailARN
			})},
			fake: &fakeCloudTrail{
				deleteTrail: func(_ context.Context, _ *awscloudtrail.DeleteTrailInput) (*awscloudtrail.DeleteTrailOutput, error) {
					return nil, trailNotFoundErr()
				},
			},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, trailCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeCloudTrail, _ ctrl.Result) {
				got := &awsv1alpha1.Trail{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "abandon policy skips AWS delete",
			objs: []client.Object{trailCR(func(tr *awsv1alpha1.Trail) {
				tr.Finalizers = []string{awsv1alpha1.FinalizerName}
				tr.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
				tr.Status.TrailARN = testTrailARN
			})},
			fake: &fakeCloudTrail{},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, trailCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeCloudTrail, _ ctrl.Result) {
				if f.deleteCalled {
					t.Error("DeleteTrail must not be called when abandoning")
				}
				got := &awsv1alpha1.Trail{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			scheme := newGovScheme(t)
			c := newTrailFakeClient(scheme, tc.objs...)
			r := &TrailReconciler{Client: c, Scheme: scheme, CloudTrailClient: tc.fake}

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
