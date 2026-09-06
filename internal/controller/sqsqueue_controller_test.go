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
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
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

type fakeSQS struct {
	getQueueUrl        func(ctx context.Context, params *awssqs.GetQueueUrlInput) (*awssqs.GetQueueUrlOutput, error)
	createQueue        func(ctx context.Context, params *awssqs.CreateQueueInput) (*awssqs.CreateQueueOutput, error)
	setQueueAttributes func(ctx context.Context, params *awssqs.SetQueueAttributesInput) (*awssqs.SetQueueAttributesOutput, error)
	tagQueue           func(ctx context.Context, params *awssqs.TagQueueInput) (*awssqs.TagQueueOutput, error)
	getQueueAttributes func(ctx context.Context, params *awssqs.GetQueueAttributesInput) (*awssqs.GetQueueAttributesOutput, error)
	deleteQueue        func(ctx context.Context, params *awssqs.DeleteQueueInput) (*awssqs.DeleteQueueOutput, error)

	createCalled bool
	deleteCalled bool
	deletedURL   string
	createInput  *awssqs.CreateQueueInput
}

func (f *fakeSQS) GetQueueUrl(ctx context.Context, params *awssqs.GetQueueUrlInput, _ ...func(*awssqs.Options)) (*awssqs.GetQueueUrlOutput, error) {
	if f.getQueueUrl == nil {
		return nil, fmt.Errorf("unexpected call to GetQueueUrl")
	}
	return f.getQueueUrl(ctx, params)
}

func (f *fakeSQS) CreateQueue(ctx context.Context, params *awssqs.CreateQueueInput, _ ...func(*awssqs.Options)) (*awssqs.CreateQueueOutput, error) {
	f.createCalled = true
	f.createInput = params
	if f.createQueue == nil {
		return nil, fmt.Errorf("unexpected call to CreateQueue")
	}
	return f.createQueue(ctx, params)
}

func (f *fakeSQS) SetQueueAttributes(ctx context.Context, params *awssqs.SetQueueAttributesInput, _ ...func(*awssqs.Options)) (*awssqs.SetQueueAttributesOutput, error) {
	if f.setQueueAttributes == nil {
		return nil, fmt.Errorf("unexpected call to SetQueueAttributes")
	}
	return f.setQueueAttributes(ctx, params)
}

func (f *fakeSQS) TagQueue(ctx context.Context, params *awssqs.TagQueueInput, _ ...func(*awssqs.Options)) (*awssqs.TagQueueOutput, error) {
	if f.tagQueue == nil {
		return nil, fmt.Errorf("unexpected call to TagQueue")
	}
	return f.tagQueue(ctx, params)
}

func (f *fakeSQS) GetQueueAttributes(ctx context.Context, params *awssqs.GetQueueAttributesInput, _ ...func(*awssqs.Options)) (*awssqs.GetQueueAttributesOutput, error) {
	if f.getQueueAttributes == nil {
		return nil, fmt.Errorf("unexpected call to GetQueueAttributes")
	}
	return f.getQueueAttributes(ctx, params)
}

func (f *fakeSQS) DeleteQueue(ctx context.Context, params *awssqs.DeleteQueueInput, _ ...func(*awssqs.Options)) (*awssqs.DeleteQueueOutput, error) {
	f.deleteCalled = true
	f.deletedURL = aws.ToString(params.QueueUrl)
	if f.deleteQueue == nil {
		return nil, fmt.Errorf("unexpected call to DeleteQueue")
	}
	return f.deleteQueue(ctx, params)
}

func sqsNotFoundErr() error {
	return &smithy.GenericAPIError{Code: "QueueDoesNotExist", Message: "queue does not exist"}
}

func newSQSScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return scheme
}

func newSQSFakeClient(scheme *runtime.Scheme, objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&awsv1alpha1.SQSQueue{}).
		WithObjects(objs...).
		Build()
}

const (
	testQueueURL = "https://sqs.us-east-1.amazonaws.com/123456789012/my-queue"
	testQueueARN = "arn:aws:sqs:us-east-1:123456789012:my-queue"
)

func sqsQueueCR(mutate ...func(*awsv1alpha1.SQSQueue)) *awsv1alpha1.SQSQueue {
	q := &awsv1alpha1.SQSQueue{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-queue",
			Namespace:  "default",
			Finalizers: []string{awsv1alpha1.FinalizerName},
		},
		Spec: awsv1alpha1.SQSQueueSpec{
			QueueName:         "my-queue",
			VisibilityTimeout: 30,
			Tags:              map[string]string{"env": "test"},
		},
	}
	for _, m := range mutate {
		m(q)
	}
	return q
}

func TestSQSQueueReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-queue", Namespace: "default"}}

	tests := []struct {
		name    string
		objs    []client.Object
		fake    *fakeSQS
		setup   func(t *testing.T, ctx context.Context, c client.Client)
		wantErr bool
		assert  func(t *testing.T, ctx context.Context, c client.Client, f *fakeSQS, res ctrl.Result)
	}{
		{
			name: "create happy path persists identifier and Ready",
			objs: []client.Object{sqsQueueCR()},
			fake: &fakeSQS{
				getQueueUrl: func(_ context.Context, _ *awssqs.GetQueueUrlInput) (*awssqs.GetQueueUrlOutput, error) {
					return nil, sqsNotFoundErr()
				},
				createQueue: func(_ context.Context, params *awssqs.CreateQueueInput) (*awssqs.CreateQueueOutput, error) {
					if aws.ToString(params.QueueName) != "my-queue" {
						return nil, fmt.Errorf("unexpected queue name %q", aws.ToString(params.QueueName))
					}
					return &awssqs.CreateQueueOutput{QueueUrl: aws.String(testQueueURL)}, nil
				},
				getQueueAttributes: func(_ context.Context, _ *awssqs.GetQueueAttributesInput) (*awssqs.GetQueueAttributesOutput, error) {
					return &awssqs.GetQueueAttributesOutput{
						Attributes: map[string]string{string(sqstypes.QueueAttributeNameQueueArn): testQueueARN},
					}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSQS, _ ctrl.Result) {
				got := &awsv1alpha1.SQSQueue{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if !f.createCalled {
					t.Error("expected CreateQueue to be called")
				}
				if got.Status.QueueURL != testQueueURL {
					t.Errorf("status.queueUrl = %q, want %q", got.Status.QueueURL, testQueueURL)
				}
				if got.Status.QueueARN != testQueueARN {
					t.Errorf("status.queueArn = %q, want %q", got.Status.QueueARN, testQueueARN)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionTrue {
					t.Errorf("Ready condition = %+v, want True", cond)
				}
			},
		},
		{
			name: "identifier persisted even when post-create step fails",
			objs: []client.Object{sqsQueueCR()},
			fake: &fakeSQS{
				getQueueUrl: func(_ context.Context, _ *awssqs.GetQueueUrlInput) (*awssqs.GetQueueUrlOutput, error) {
					return nil, sqsNotFoundErr()
				},
				createQueue: func(_ context.Context, _ *awssqs.CreateQueueInput) (*awssqs.CreateQueueOutput, error) {
					return &awssqs.CreateQueueOutput{QueueUrl: aws.String(testQueueURL)}, nil
				},
				getQueueAttributes: func(_ context.Context, _ *awssqs.GetQueueAttributesInput) (*awssqs.GetQueueAttributesOutput, error) {
					return nil, fmt.Errorf("throttled")
				},
			},
			wantErr: true,
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSQS, _ ctrl.Result) {
				got := &awsv1alpha1.SQSQueue{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.QueueURL != testQueueURL {
					t.Errorf("status.queueUrl = %q, want %q (identifier must be persisted before failing step)", got.Status.QueueURL, testQueueURL)
				}
			},
		},
		{
			name: "steady state does not create",
			objs: []client.Object{sqsQueueCR(func(q *awsv1alpha1.SQSQueue) {
				q.Finalizers = []string{awsv1alpha1.FinalizerName}
				q.Status.QueueURL = testQueueURL
				q.Status.QueueARN = testQueueARN
			})},
			fake: &fakeSQS{
				getQueueUrl: func(_ context.Context, _ *awssqs.GetQueueUrlInput) (*awssqs.GetQueueUrlOutput, error) {
					return &awssqs.GetQueueUrlOutput{QueueUrl: aws.String(testQueueURL)}, nil
				},
				setQueueAttributes: func(_ context.Context, _ *awssqs.SetQueueAttributesInput) (*awssqs.SetQueueAttributesOutput, error) {
					return &awssqs.SetQueueAttributesOutput{}, nil
				},
				tagQueue: func(_ context.Context, _ *awssqs.TagQueueInput) (*awssqs.TagQueueOutput, error) {
					return &awssqs.TagQueueOutput{}, nil
				},
				getQueueAttributes: func(_ context.Context, _ *awssqs.GetQueueAttributesInput) (*awssqs.GetQueueAttributesOutput, error) {
					return &awssqs.GetQueueAttributesOutput{
						Attributes: map[string]string{string(sqstypes.QueueAttributeNameQueueArn): testQueueARN},
					}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSQS, _ ctrl.Result) {
				if f.createCalled {
					t.Error("CreateQueue must not be called in steady state")
				}
				got := &awsv1alpha1.SQSQueue{}
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
			name: "delete with finalizer calls AWS delete with status identifier",
			objs: []client.Object{sqsQueueCR(func(q *awsv1alpha1.SQSQueue) {
				q.Finalizers = []string{awsv1alpha1.FinalizerName}
				q.Status.QueueURL = testQueueURL
			})},
			fake: &fakeSQS{
				deleteQueue: func(_ context.Context, _ *awssqs.DeleteQueueInput) (*awssqs.DeleteQueueOutput, error) {
					return &awssqs.DeleteQueueOutput{}, nil
				},
			},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, sqsQueueCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSQS, _ ctrl.Result) {
				if !f.deleteCalled {
					t.Error("expected DeleteQueue to be called")
				}
				if f.deletedURL != testQueueURL {
					t.Errorf("DeleteQueue url = %q, want %q", f.deletedURL, testQueueURL)
				}
				got := &awsv1alpha1.SQSQueue{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "abandon policy skips AWS delete",
			objs: []client.Object{sqsQueueCR(func(q *awsv1alpha1.SQSQueue) {
				q.Finalizers = []string{awsv1alpha1.FinalizerName}
				q.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
				q.Status.QueueURL = testQueueURL
			})},
			fake: &fakeSQS{},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, sqsQueueCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSQS, _ ctrl.Result) {
				if f.deleteCalled {
					t.Error("DeleteQueue must not be called when abandoning")
				}
				got := &awsv1alpha1.SQSQueue{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "delete fallback looks up queue URL from spec when status empty",
			objs: []client.Object{sqsQueueCR(func(q *awsv1alpha1.SQSQueue) {
				q.Finalizers = []string{awsv1alpha1.FinalizerName}
			})},
			fake: &fakeSQS{
				getQueueUrl: func(_ context.Context, params *awssqs.GetQueueUrlInput) (*awssqs.GetQueueUrlOutput, error) {
					if aws.ToString(params.QueueName) != "my-queue" {
						return nil, fmt.Errorf("unexpected queue name")
					}
					return &awssqs.GetQueueUrlOutput{QueueUrl: aws.String(testQueueURL)}, nil
				},
				deleteQueue: func(_ context.Context, _ *awssqs.DeleteQueueInput) (*awssqs.DeleteQueueOutput, error) {
					return &awssqs.DeleteQueueOutput{}, nil
				},
			},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, sqsQueueCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSQS, _ ctrl.Result) {
				if !f.deleteCalled {
					t.Error("expected DeleteQueue to be called via spec-based fallback")
				}
				if f.deletedURL != testQueueURL {
					t.Errorf("DeleteQueue url = %q, want %q", f.deletedURL, testQueueURL)
				}
			},
		},
		{
			name: "redrive policy waits for DLQ without ARN",
			objs: []client.Object{
				sqsQueueCR(func(q *awsv1alpha1.SQSQueue) {
					q.Spec.RedrivePolicy = &awsv1alpha1.SQSRedrivePolicy{
						DeadLetterQueueRef: "my-dlq",
						MaxReceiveCount:    5,
					}
				}),
				&awsv1alpha1.SQSQueue{
					ObjectMeta: metav1.ObjectMeta{Name: "my-dlq", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
					Spec:       awsv1alpha1.SQSQueueSpec{QueueName: "my-dlq"},
				},
			},
			fake: &fakeSQS{
				getQueueUrl: func(_ context.Context, _ *awssqs.GetQueueUrlInput) (*awssqs.GetQueueUrlOutput, error) {
					return nil, sqsNotFoundErr()
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSQS, res ctrl.Result) {
				if res != requeueDependency {
					t.Errorf("result = %+v, want requeueDependency %+v", res, requeueDependency)
				}
				if f.createCalled {
					t.Error("CreateQueue must not be called while DLQ dependency is not ready")
				}
			},
		},
		{
			name: "redrive policy resolves DLQ ARN into create attributes",
			objs: []client.Object{
				sqsQueueCR(func(q *awsv1alpha1.SQSQueue) {
					q.Spec.RedrivePolicy = &awsv1alpha1.SQSRedrivePolicy{
						DeadLetterQueueRef: "my-dlq",
						MaxReceiveCount:    5,
					}
				}),
				&awsv1alpha1.SQSQueue{
					ObjectMeta: metav1.ObjectMeta{Name: "my-dlq", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
					Spec:       awsv1alpha1.SQSQueueSpec{QueueName: "my-dlq"},
					Status: awsv1alpha1.SQSQueueStatus{
						QueueARN: "arn:aws:sqs:us-east-1:123456789012:my-dlq",
					},
				},
			},
			fake: &fakeSQS{
				getQueueUrl: func(_ context.Context, _ *awssqs.GetQueueUrlInput) (*awssqs.GetQueueUrlOutput, error) {
					return nil, sqsNotFoundErr()
				},
				createQueue: func(_ context.Context, _ *awssqs.CreateQueueInput) (*awssqs.CreateQueueOutput, error) {
					return &awssqs.CreateQueueOutput{QueueUrl: aws.String(testQueueURL)}, nil
				},
				getQueueAttributes: func(_ context.Context, _ *awssqs.GetQueueAttributesInput) (*awssqs.GetQueueAttributesOutput, error) {
					return &awssqs.GetQueueAttributesOutput{
						Attributes: map[string]string{string(sqstypes.QueueAttributeNameQueueArn): testQueueARN},
					}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSQS, _ ctrl.Result) {
				if f.createInput == nil {
					t.Fatal("expected CreateQueue to be called")
				}
				rp := f.createInput.Attributes[string(sqstypes.QueueAttributeNameRedrivePolicy)]
				if !strings.Contains(rp, "arn:aws:sqs:us-east-1:123456789012:my-dlq") {
					t.Errorf("redrive policy %q does not contain DLQ ARN", rp)
				}
				if !strings.Contains(rp, `"maxReceiveCount":5`) {
					t.Errorf("redrive policy %q does not contain maxReceiveCount", rp)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			scheme := newSQSScheme(t)
			c := newSQSFakeClient(scheme, tc.objs...)
			r := &SQSQueueReconciler{Client: c, Scheme: scheme, SQSClient: tc.fake}

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

// A queue with no attribute fields set must not call SetQueueAttributes:
// SQS returns MissingParameter for an empty attribute map.
func TestSQSQueueSteadyStateNoAttributesSkipsSetQueueAttributes(t *testing.T) {
	scheme := newSQSScheme(t)
	q := &awsv1alpha1.SQSQueue{
		ObjectMeta: metav1.ObjectMeta{Name: "plain", Namespace: "ns", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec:       awsv1alpha1.SQSQueueSpec{QueueName: "plain"},
	}
	f := &fakeSQS{
		getQueueUrl: func(context.Context, *awssqs.GetQueueUrlInput) (*awssqs.GetQueueUrlOutput, error) {
			return &awssqs.GetQueueUrlOutput{QueueUrl: aws.String("https://sqs/plain")}, nil
		},
		getQueueAttributes: func(context.Context, *awssqs.GetQueueAttributesInput) (*awssqs.GetQueueAttributesOutput, error) {
			return &awssqs.GetQueueAttributesOutput{Attributes: map[string]string{"QueueArn": "arn:plain"}}, nil
		},
	}
	r := &SQSQueueReconciler{Client: newSQSFakeClient(scheme, q), Scheme: scheme, SQSClient: f}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "plain", Namespace: "ns"}}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
}
