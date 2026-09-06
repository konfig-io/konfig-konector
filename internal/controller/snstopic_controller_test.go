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
	awssns "github.com/aws/aws-sdk-go-v2/service/sns"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
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

type fakeSNS struct {
	createTopic        func(ctx context.Context, params *awssns.CreateTopicInput) (*awssns.CreateTopicOutput, error)
	setTopicAttributes func(ctx context.Context, params *awssns.SetTopicAttributesInput) (*awssns.SetTopicAttributesOutput, error)
	tagResource        func(ctx context.Context, params *awssns.TagResourceInput) (*awssns.TagResourceOutput, error)
	deleteTopic        func(ctx context.Context, params *awssns.DeleteTopicInput) (*awssns.DeleteTopicOutput, error)
	listTopics         func(ctx context.Context, params *awssns.ListTopicsInput) (*awssns.ListTopicsOutput, error)

	createCalled bool
	deleteCalled bool
	deletedARN   string
}

func (f *fakeSNS) CreateTopic(ctx context.Context, params *awssns.CreateTopicInput, _ ...func(*awssns.Options)) (*awssns.CreateTopicOutput, error) {
	f.createCalled = true
	if f.createTopic == nil {
		return nil, fmt.Errorf("unexpected call to CreateTopic")
	}
	return f.createTopic(ctx, params)
}

func (f *fakeSNS) SetTopicAttributes(ctx context.Context, params *awssns.SetTopicAttributesInput, _ ...func(*awssns.Options)) (*awssns.SetTopicAttributesOutput, error) {
	if f.setTopicAttributes == nil {
		return nil, fmt.Errorf("unexpected call to SetTopicAttributes")
	}
	return f.setTopicAttributes(ctx, params)
}

func (f *fakeSNS) TagResource(ctx context.Context, params *awssns.TagResourceInput, _ ...func(*awssns.Options)) (*awssns.TagResourceOutput, error) {
	if f.tagResource == nil {
		return nil, fmt.Errorf("unexpected call to TagResource")
	}
	return f.tagResource(ctx, params)
}

func (f *fakeSNS) DeleteTopic(ctx context.Context, params *awssns.DeleteTopicInput, _ ...func(*awssns.Options)) (*awssns.DeleteTopicOutput, error) {
	f.deleteCalled = true
	f.deletedARN = aws.ToString(params.TopicArn)
	if f.deleteTopic == nil {
		return nil, fmt.Errorf("unexpected call to DeleteTopic")
	}
	return f.deleteTopic(ctx, params)
}

func (f *fakeSNS) ListTopics(ctx context.Context, params *awssns.ListTopicsInput, _ ...func(*awssns.Options)) (*awssns.ListTopicsOutput, error) {
	if f.listTopics == nil {
		return nil, fmt.Errorf("unexpected call to ListTopics")
	}
	return f.listTopics(ctx, params)
}

const testTopicARN = "arn:aws:sns:us-east-1:123456789012:my-topic"

func snsTopicCR(mutate ...func(*awsv1alpha1.SNSTopic)) *awsv1alpha1.SNSTopic {
	tp := &awsv1alpha1.SNSTopic{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-topic",
			Namespace:  "default",
			Finalizers: []string{awsv1alpha1.FinalizerName},
		},
		Spec: awsv1alpha1.SNSTopicSpec{
			TopicName: "my-topic",
			Tags:      map[string]string{"env": "test"},
		},
	}
	for _, m := range mutate {
		m(tp)
	}
	return tp
}

func newSNSScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return scheme
}

func TestSNSTopicReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-topic", Namespace: "default"}}

	tests := []struct {
		name    string
		objs    []client.Object
		fake    *fakeSNS
		setup   func(t *testing.T, ctx context.Context, c client.Client)
		wantErr bool
		assert  func(t *testing.T, ctx context.Context, c client.Client, f *fakeSNS, res ctrl.Result)
	}{
		{
			name: "create happy path persists ARN and Ready",
			objs: []client.Object{snsTopicCR()},
			fake: &fakeSNS{
				createTopic: func(_ context.Context, params *awssns.CreateTopicInput) (*awssns.CreateTopicOutput, error) {
					if aws.ToString(params.Name) != "my-topic" {
						return nil, fmt.Errorf("unexpected topic name %q", aws.ToString(params.Name))
					}
					return &awssns.CreateTopicOutput{TopicArn: aws.String(testTopicARN)}, nil
				},
				tagResource: func(_ context.Context, _ *awssns.TagResourceInput) (*awssns.TagResourceOutput, error) {
					return &awssns.TagResourceOutput{}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSNS, _ ctrl.Result) {
				got := &awsv1alpha1.SNSTopic{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if !f.createCalled {
					t.Error("expected CreateTopic to be called")
				}
				if got.Status.TopicARN != testTopicARN {
					t.Errorf("status.topicArn = %q, want %q", got.Status.TopicARN, testTopicARN)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionTrue {
					t.Errorf("Ready condition = %+v, want True", cond)
				}
			},
		},
		{
			name: "ARN persisted even when tag call fails",
			objs: []client.Object{snsTopicCR()},
			fake: &fakeSNS{
				createTopic: func(_ context.Context, _ *awssns.CreateTopicInput) (*awssns.CreateTopicOutput, error) {
					return &awssns.CreateTopicOutput{TopicArn: aws.String(testTopicARN)}, nil
				},
				tagResource: func(_ context.Context, _ *awssns.TagResourceInput) (*awssns.TagResourceOutput, error) {
					return nil, fmt.Errorf("throttled")
				},
			},
			wantErr: true,
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSNS, _ ctrl.Result) {
				got := &awsv1alpha1.SNSTopic{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.TopicARN != testTopicARN {
					t.Errorf("status.topicArn = %q, want %q (identifier must be persisted before failing step)", got.Status.TopicARN, testTopicARN)
				}
			},
		},
		{
			name: "steady state re-creates idempotently and stays Ready",
			objs: []client.Object{snsTopicCR(func(tp *awsv1alpha1.SNSTopic) {
				tp.Finalizers = []string{awsv1alpha1.FinalizerName}
				tp.Status.TopicARN = testTopicARN
				tp.Spec.KMSKeyID = "alias/my-key"
			})},
			fake: &fakeSNS{
				createTopic: func(_ context.Context, _ *awssns.CreateTopicInput) (*awssns.CreateTopicOutput, error) {
					return &awssns.CreateTopicOutput{TopicArn: aws.String(testTopicARN)}, nil
				},
				setTopicAttributes: func(_ context.Context, _ *awssns.SetTopicAttributesInput) (*awssns.SetTopicAttributesOutput, error) {
					return &awssns.SetTopicAttributesOutput{}, nil
				},
				tagResource: func(_ context.Context, _ *awssns.TagResourceInput) (*awssns.TagResourceOutput, error) {
					return &awssns.TagResourceOutput{}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSNS, _ ctrl.Result) {
				got := &awsv1alpha1.SNSTopic{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.TopicARN != testTopicARN {
					t.Errorf("status.topicArn = %q, want %q", got.Status.TopicARN, testTopicARN)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionTrue {
					t.Errorf("Ready condition = %+v, want True", cond)
				}
			},
		},
		{
			name: "delete with finalizer calls AWS delete with status ARN",
			objs: []client.Object{snsTopicCR(func(tp *awsv1alpha1.SNSTopic) {
				tp.Finalizers = []string{awsv1alpha1.FinalizerName}
				tp.Status.TopicARN = testTopicARN
			})},
			fake: &fakeSNS{
				deleteTopic: func(_ context.Context, _ *awssns.DeleteTopicInput) (*awssns.DeleteTopicOutput, error) {
					return &awssns.DeleteTopicOutput{}, nil
				},
			},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, snsTopicCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSNS, _ ctrl.Result) {
				if !f.deleteCalled {
					t.Error("expected DeleteTopic to be called")
				}
				if f.deletedARN != testTopicARN {
					t.Errorf("DeleteTopic arn = %q, want %q", f.deletedARN, testTopicARN)
				}
				got := &awsv1alpha1.SNSTopic{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "abandon policy skips AWS delete",
			objs: []client.Object{snsTopicCR(func(tp *awsv1alpha1.SNSTopic) {
				tp.Finalizers = []string{awsv1alpha1.FinalizerName}
				tp.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
				tp.Status.TopicARN = testTopicARN
			})},
			fake: &fakeSNS{},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, snsTopicCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSNS, _ ctrl.Result) {
				if f.deleteCalled {
					t.Error("DeleteTopic must not be called when abandoning")
				}
				got := &awsv1alpha1.SNSTopic{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "delete fallback finds topic ARN by name when status empty",
			objs: []client.Object{snsTopicCR(func(tp *awsv1alpha1.SNSTopic) {
				tp.Finalizers = []string{awsv1alpha1.FinalizerName}
			})},
			fake: &fakeSNS{
				listTopics: func(_ context.Context, params *awssns.ListTopicsInput) (*awssns.ListTopicsOutput, error) {
					if params.NextToken == nil {
						return &awssns.ListTopicsOutput{
							Topics:    []snstypes.Topic{{TopicArn: aws.String("arn:aws:sns:us-east-1:123456789012:other-topic")}},
							NextToken: aws.String("page2"),
						}, nil
					}
					return &awssns.ListTopicsOutput{
						Topics: []snstypes.Topic{{TopicArn: aws.String(testTopicARN)}},
					}, nil
				},
				deleteTopic: func(_ context.Context, _ *awssns.DeleteTopicInput) (*awssns.DeleteTopicOutput, error) {
					return &awssns.DeleteTopicOutput{}, nil
				},
			},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, snsTopicCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSNS, _ ctrl.Result) {
				if !f.deleteCalled {
					t.Error("expected DeleteTopic to be called via name lookup fallback")
				}
				if f.deletedARN != testTopicARN {
					t.Errorf("DeleteTopic arn = %q, want %q", f.deletedARN, testTopicARN)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			scheme := newSNSScheme(t)
			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&awsv1alpha1.SNSTopic{}).
				WithObjects(tc.objs...).
				Build()
			r := &SNSTopicReconciler{Client: c, Scheme: scheme, SNSClient: tc.fake}

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
