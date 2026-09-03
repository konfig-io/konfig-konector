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
	awsrds "github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

type fakeRDSScaling struct {
	describeGC func(ctx context.Context, params *awsrds.DescribeGlobalClustersInput) (*awsrds.DescribeGlobalClustersOutput, error)
	createGC   func(ctx context.Context, params *awsrds.CreateGlobalClusterInput) (*awsrds.CreateGlobalClusterOutput, error)
	deleteGC   func(ctx context.Context, params *awsrds.DeleteGlobalClusterInput) (*awsrds.DeleteGlobalClusterOutput, error)

	describeES func(ctx context.Context, params *awsrds.DescribeEventSubscriptionsInput) (*awsrds.DescribeEventSubscriptionsOutput, error)
	createES   func(ctx context.Context, params *awsrds.CreateEventSubscriptionInput) (*awsrds.CreateEventSubscriptionOutput, error)
	modifyES   func(ctx context.Context, params *awsrds.ModifyEventSubscriptionInput) (*awsrds.ModifyEventSubscriptionOutput, error)
	deleteES   func(ctx context.Context, params *awsrds.DeleteEventSubscriptionInput) (*awsrds.DeleteEventSubscriptionOutput, error)

	createGCCalled bool
	deleteGCCalled bool
	createESCalled bool
	deleteESCalled bool
}

func (f *fakeRDSScaling) DescribeGlobalClusters(ctx context.Context, params *awsrds.DescribeGlobalClustersInput, _ ...func(*awsrds.Options)) (*awsrds.DescribeGlobalClustersOutput, error) {
	if f.describeGC == nil {
		return nil, fmt.Errorf("unexpected call to DescribeGlobalClusters")
	}
	return f.describeGC(ctx, params)
}

func (f *fakeRDSScaling) CreateGlobalCluster(ctx context.Context, params *awsrds.CreateGlobalClusterInput, _ ...func(*awsrds.Options)) (*awsrds.CreateGlobalClusterOutput, error) {
	f.createGCCalled = true
	if f.createGC == nil {
		return nil, fmt.Errorf("unexpected call to CreateGlobalCluster")
	}
	return f.createGC(ctx, params)
}

func (f *fakeRDSScaling) DeleteGlobalCluster(ctx context.Context, params *awsrds.DeleteGlobalClusterInput, _ ...func(*awsrds.Options)) (*awsrds.DeleteGlobalClusterOutput, error) {
	f.deleteGCCalled = true
	if f.deleteGC == nil {
		return nil, fmt.Errorf("unexpected call to DeleteGlobalCluster")
	}
	return f.deleteGC(ctx, params)
}

func (f *fakeRDSScaling) DescribeEventSubscriptions(ctx context.Context, params *awsrds.DescribeEventSubscriptionsInput, _ ...func(*awsrds.Options)) (*awsrds.DescribeEventSubscriptionsOutput, error) {
	if f.describeES == nil {
		return nil, fmt.Errorf("unexpected call to DescribeEventSubscriptions")
	}
	return f.describeES(ctx, params)
}

func (f *fakeRDSScaling) CreateEventSubscription(ctx context.Context, params *awsrds.CreateEventSubscriptionInput, _ ...func(*awsrds.Options)) (*awsrds.CreateEventSubscriptionOutput, error) {
	f.createESCalled = true
	if f.createES == nil {
		return nil, fmt.Errorf("unexpected call to CreateEventSubscription")
	}
	return f.createES(ctx, params)
}

func (f *fakeRDSScaling) ModifyEventSubscription(ctx context.Context, params *awsrds.ModifyEventSubscriptionInput, _ ...func(*awsrds.Options)) (*awsrds.ModifyEventSubscriptionOutput, error) {
	if f.modifyES == nil {
		return nil, fmt.Errorf("unexpected call to ModifyEventSubscription")
	}
	return f.modifyES(ctx, params)
}

func (f *fakeRDSScaling) DeleteEventSubscription(ctx context.Context, params *awsrds.DeleteEventSubscriptionInput, _ ...func(*awsrds.Options)) (*awsrds.DeleteEventSubscriptionOutput, error) {
	f.deleteESCalled = true
	if f.deleteES == nil {
		return nil, fmt.Errorf("unexpected call to DeleteEventSubscription")
	}
	return f.deleteES(ctx, params)
}

func TestRDSGlobalClusterReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-global", Namespace: "default"}}
	gcARN := "arn:aws:rds::123456789012:global-cluster:my-global"
	gcCR := func(mutate ...func(*awsv1alpha1.RDSGlobalCluster)) *awsv1alpha1.RDSGlobalCluster {
		gc := &awsv1alpha1.RDSGlobalCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "my-global", Namespace: "default"},
			Spec: awsv1alpha1.RDSGlobalClusterSpec{
				GlobalClusterIdentifier: "my-global",
				Engine:                  "aurora-postgresql",
				EngineVersion:           "15.4",
				StorageEncrypted:        true,
			},
		}
		for _, m := range mutate {
			m(gc)
		}
		return gc
	}

	t.Run("create persists ARN", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, gcCR())
		f := &fakeRDSScaling{
			describeGC: func(_ context.Context, _ *awsrds.DescribeGlobalClustersInput) (*awsrds.DescribeGlobalClustersOutput, error) {
				return nil, &rdstypes.GlobalClusterNotFoundFault{Message: aws.String("not found")}
			},
			createGC: func(_ context.Context, params *awsrds.CreateGlobalClusterInput) (*awsrds.CreateGlobalClusterOutput, error) {
				if aws.ToString(params.Engine) != "aurora-postgresql" {
					t.Errorf("engine = %q", aws.ToString(params.Engine))
				}
				if !aws.ToBool(params.StorageEncrypted) {
					t.Error("storageEncrypted not passed")
				}
				return &awsrds.CreateGlobalClusterOutput{GlobalCluster: &rdstypes.GlobalCluster{
					GlobalClusterArn: aws.String(gcARN),
					Status:           aws.String("creating"),
				}}, nil
			},
		}
		r := &RDSGlobalClusterReconciler{Client: c, Scheme: scheme, RDSClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.RDSGlobalCluster{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ARN != gcARN {
			t.Errorf("status.arn = %q", got.Status.ARN)
		}
	})

	t.Run("delete calls DeleteGlobalCluster", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, gcCR(func(gc *awsv1alpha1.RDSGlobalCluster) {
			gc.Finalizers = []string{awsv1alpha1.FinalizerName}
			gc.Status.ARN = gcARN
		}))
		f := &fakeRDSScaling{
			deleteGC: func(_ context.Context, params *awsrds.DeleteGlobalClusterInput) (*awsrds.DeleteGlobalClusterOutput, error) {
				if aws.ToString(params.GlobalClusterIdentifier) != "my-global" {
					t.Errorf("delete identifier = %q", aws.ToString(params.GlobalClusterIdentifier))
				}
				return &awsrds.DeleteGlobalClusterOutput{}, nil
			},
		}
		r := &RDSGlobalClusterReconciler{Client: c, Scheme: scheme, RDSClient: f}
		if err := c.Delete(ctx, gcCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteGCCalled {
			t.Error("expected DeleteGlobalCluster to be called")
		}
		got := &awsv1alpha1.RDSGlobalCluster{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected CR gone, got err=%v", err)
		}
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, gcCR(func(gc *awsv1alpha1.RDSGlobalCluster) {
			gc.Finalizers = []string{awsv1alpha1.FinalizerName}
			gc.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
		}))
		f := &fakeRDSScaling{}
		r := &RDSGlobalClusterReconciler{Client: c, Scheme: scheme, RDSClient: f}
		if err := c.Delete(ctx, gcCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteGCCalled {
			t.Error("DeleteGlobalCluster must not be called when abandoning")
		}
	})
}

func TestRDSEventSubscriptionReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-sub", Namespace: "default"}}
	esARN := "arn:aws:rds:us-east-1:123456789012:es:my-sub"
	topicARN := "arn:aws:sns:us-east-1:123456789012:rds-events"
	esCR := func(mutate ...func(*awsv1alpha1.RDSEventSubscription)) *awsv1alpha1.RDSEventSubscription {
		es := &awsv1alpha1.RDSEventSubscription{
			ObjectMeta: metav1.ObjectMeta{Name: "my-sub", Namespace: "default"},
			Spec: awsv1alpha1.RDSEventSubscriptionSpec{
				SubscriptionName: "my-sub",
				SnsTopicRef:      awsv1alpha1.SNSTopicRef{ARN: topicARN},
				SourceType:       "db-instance",
				EventCategories:  []string{"failover"},
			},
		}
		for _, m := range mutate {
			m(es)
		}
		return es
	}

	t.Run("create persists ARN", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, esCR())
		f := &fakeRDSScaling{
			describeES: func(_ context.Context, _ *awsrds.DescribeEventSubscriptionsInput) (*awsrds.DescribeEventSubscriptionsOutput, error) {
				return nil, &rdstypes.SubscriptionNotFoundFault{Message: aws.String("not found")}
			},
			createES: func(_ context.Context, params *awsrds.CreateEventSubscriptionInput) (*awsrds.CreateEventSubscriptionOutput, error) {
				if aws.ToString(params.SnsTopicArn) != topicARN {
					t.Errorf("topic ARN = %q", aws.ToString(params.SnsTopicArn))
				}
				if !aws.ToBool(params.Enabled) {
					t.Error("expected enabled=true by default")
				}
				return &awsrds.CreateEventSubscriptionOutput{EventSubscription: &rdstypes.EventSubscription{
					EventSubscriptionArn: aws.String(esARN),
				}}, nil
			},
		}
		r := &RDSEventSubscriptionReconciler{Client: c, Scheme: scheme, RDSClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.RDSEventSubscription{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ARN != esARN {
			t.Errorf("status.arn = %q", got.Status.ARN)
		}
	})

	t.Run("delete calls DeleteEventSubscription", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, esCR(func(es *awsv1alpha1.RDSEventSubscription) {
			es.Finalizers = []string{awsv1alpha1.FinalizerName}
			es.Status.ARN = esARN
		}))
		f := &fakeRDSScaling{
			deleteES: func(_ context.Context, params *awsrds.DeleteEventSubscriptionInput) (*awsrds.DeleteEventSubscriptionOutput, error) {
				if aws.ToString(params.SubscriptionName) != "my-sub" {
					t.Errorf("delete name = %q", aws.ToString(params.SubscriptionName))
				}
				return &awsrds.DeleteEventSubscriptionOutput{}, nil
			},
		}
		r := &RDSEventSubscriptionReconciler{Client: c, Scheme: scheme, RDSClient: f}
		if err := c.Delete(ctx, esCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteESCalled {
			t.Error("expected DeleteEventSubscription to be called")
		}
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, esCR(func(es *awsv1alpha1.RDSEventSubscription) {
			es.Finalizers = []string{awsv1alpha1.FinalizerName}
			es.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
		}))
		f := &fakeRDSScaling{}
		r := &RDSEventSubscriptionReconciler{Client: c, Scheme: scheme, RDSClient: f}
		if err := c.Delete(ctx, esCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteESCalled {
			t.Error("DeleteEventSubscription must not be called when abandoning")
		}
	})
}
