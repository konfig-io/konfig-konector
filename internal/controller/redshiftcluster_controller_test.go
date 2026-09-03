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
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsredshift "github.com/aws/aws-sdk-go-v2/service/redshift"
	redshifttypes "github.com/aws/aws-sdk-go-v2/service/redshift/types"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

type fakeRedshiftClusterAPI struct {
	describe func(ctx context.Context, params *awsredshift.DescribeClustersInput) (*awsredshift.DescribeClustersOutput, error)
	create   func(ctx context.Context, params *awsredshift.CreateClusterInput) (*awsredshift.CreateClusterOutput, error)
	modify   func(ctx context.Context, params *awsredshift.ModifyClusterInput) (*awsredshift.ModifyClusterOutput, error)
	delete   func(ctx context.Context, params *awsredshift.DeleteClusterInput) (*awsredshift.DeleteClusterOutput, error)

	createCalled bool
	modifyCalled bool
	deleteCalled bool
	createInput  *awsredshift.CreateClusterInput
	deleteInput  *awsredshift.DeleteClusterInput
}

func (f *fakeRedshiftClusterAPI) DescribeClusters(ctx context.Context, params *awsredshift.DescribeClustersInput, _ ...func(*awsredshift.Options)) (*awsredshift.DescribeClustersOutput, error) {
	if f.describe == nil {
		return nil, fmt.Errorf("unexpected call to DescribeClusters")
	}
	return f.describe(ctx, params)
}

func (f *fakeRedshiftClusterAPI) CreateCluster(ctx context.Context, params *awsredshift.CreateClusterInput, _ ...func(*awsredshift.Options)) (*awsredshift.CreateClusterOutput, error) {
	f.createCalled = true
	f.createInput = params
	if f.create == nil {
		return nil, fmt.Errorf("unexpected call to CreateCluster")
	}
	return f.create(ctx, params)
}

func (f *fakeRedshiftClusterAPI) ModifyCluster(ctx context.Context, params *awsredshift.ModifyClusterInput, _ ...func(*awsredshift.Options)) (*awsredshift.ModifyClusterOutput, error) {
	f.modifyCalled = true
	if f.modify == nil {
		return nil, fmt.Errorf("unexpected call to ModifyCluster")
	}
	return f.modify(ctx, params)
}

func (f *fakeRedshiftClusterAPI) DeleteCluster(ctx context.Context, params *awsredshift.DeleteClusterInput, _ ...func(*awsredshift.Options)) (*awsredshift.DeleteClusterOutput, error) {
	f.deleteCalled = true
	f.deleteInput = params
	if f.delete == nil {
		return nil, fmt.Errorf("unexpected call to DeleteCluster")
	}
	return f.delete(ctx, params)
}

const testRedshiftPassword = "r3dshift-s3cret-pw"

func redshiftScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add aws scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}
	return scheme
}

func redshiftPasswordSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "redshift-creds", Namespace: "default"},
		Data:       map[string][]byte{"password": []byte(testRedshiftPassword)},
	}
}

func redshiftClusterCR(mutate ...func(*awsv1alpha1.RedshiftCluster)) *awsv1alpha1.RedshiftCluster {
	rc := &awsv1alpha1.RedshiftCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-cluster",
			Namespace: "default",
		},
		Spec: awsv1alpha1.RedshiftClusterSpec{
			ClusterIdentifier:        "my-cluster",
			NodeType:                 "ra3.xlplus",
			NumberOfNodes:            2,
			MasterUsername:           "admin",
			MasterUserPasswordRef:    awsv1alpha1.SecretRef{Name: "redshift-creds", Key: "password"},
			DBName:                   "analytics",
			SkipFinalClusterSnapshot: true,
			Tags:                     map[string]string{"env": "test"},
		},
	}
	for _, m := range mutate {
		m(rc)
	}
	return rc
}

func availableRedshiftDescribeOutput() *awsredshift.DescribeClustersOutput {
	return &awsredshift.DescribeClustersOutput{
		Clusters: []redshifttypes.Cluster{{
			ClusterIdentifier: aws.String("my-cluster"),
			ClusterStatus:     aws.String("available"),
			Endpoint: &redshifttypes.Endpoint{
				Address: aws.String("my-cluster.abc.us-east-1.redshift.amazonaws.com"),
				Port:    aws.Int32(5439),
			},
		}},
	}
}

// assertNoRedshiftPasswordLeak fails if the master password appears anywhere
// in the CR's status (including conditions).
func assertNoRedshiftPasswordLeak(t *testing.T, rc *awsv1alpha1.RedshiftCluster) {
	t.Helper()
	raw, err := json.Marshal(rc.Status)
	if err != nil {
		t.Fatalf("marshal status: %v", err)
	}
	if strings.Contains(string(raw), testRedshiftPassword) {
		t.Errorf("master password leaked into status: %s", raw)
	}
}

func TestRedshiftClusterReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-cluster", Namespace: "default"}}

	t.Run("create happy path persists identifier then becomes Ready", func(t *testing.T) {
		ctx := context.Background()
		scheme := redshiftScheme(t)
		f := &fakeRedshiftClusterAPI{
			create: func(_ context.Context, params *awsredshift.CreateClusterInput) (*awsredshift.CreateClusterOutput, error) {
				if aws.ToString(params.MasterUserPassword) != testRedshiftPassword {
					return nil, fmt.Errorf("expected master password from secret, got %q", aws.ToString(params.MasterUserPassword))
				}
				if aws.ToString(params.ClusterType) != "multi-node" || aws.ToInt32(params.NumberOfNodes) != 2 {
					return nil, fmt.Errorf("expected multi-node cluster of 2, got %s/%d", aws.ToString(params.ClusterType), aws.ToInt32(params.NumberOfNodes))
				}
				return &awsredshift.CreateClusterOutput{Cluster: &redshifttypes.Cluster{
					ClusterIdentifier: params.ClusterIdentifier,
					ClusterStatus:     aws.String("creating"),
				}}, nil
			},
			describe: func(_ context.Context, _ *awsredshift.DescribeClustersInput) (*awsredshift.DescribeClustersOutput, error) {
				return availableRedshiftDescribeOutput(), nil
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.RedshiftCluster{}).
			WithObjects(redshiftClusterCR(), redshiftPasswordSecret()).Build()
		r := &RedshiftClusterReconciler{Client: c, Scheme: scheme, RedshiftClient: f}

		// First reconcile: adds finalizer, creates the cluster, persists ID.
		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile 1: %v", err)
		}
		if !f.createCalled {
			t.Fatal("expected CreateCluster to be called")
		}
		if res != requeueRedshiftPolling {
			t.Errorf("result = %+v, want polling requeue %+v", res, requeueRedshiftPolling)
		}
		got := &awsv1alpha1.RedshiftCluster{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ClusterIdentifier != "my-cluster" {
			t.Errorf("status.clusterIdentifier = %q, want my-cluster", got.Status.ClusterIdentifier)
		}
		if got.Status.ClusterStatus != "creating" {
			t.Errorf("status.clusterStatus = %q, want creating", got.Status.ClusterStatus)
		}
		assertNoRedshiftPasswordLeak(t, got)

		// Second reconcile: cluster available; controller reports Ready.
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile 2: %v", err)
		}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
		if cond == nil || cond.Status != metav1.ConditionTrue {
			t.Errorf("Ready condition = %+v, want True", cond)
		}
		if got.Status.Endpoint == "" || got.Status.Port != 5439 {
			t.Errorf("endpoint/port not synced: %q/%d", got.Status.Endpoint, got.Status.Port)
		}
		assertNoRedshiftPasswordLeak(t, got)
	})

	t.Run("transient status polls without modifying", func(t *testing.T) {
		ctx := context.Background()
		scheme := redshiftScheme(t)
		f := &fakeRedshiftClusterAPI{
			describe: func(_ context.Context, _ *awsredshift.DescribeClustersInput) (*awsredshift.DescribeClustersOutput, error) {
				return &awsredshift.DescribeClustersOutput{
					Clusters: []redshifttypes.Cluster{{
						ClusterIdentifier: aws.String("my-cluster"),
						ClusterStatus:     aws.String("modifying"),
					}},
				}, nil
			},
		}
		rc := redshiftClusterCR(func(rc *awsv1alpha1.RedshiftCluster) {
			rc.Finalizers = []string{awsv1alpha1.FinalizerName}
			rc.Generation = 2
			rc.Status.ClusterIdentifier = "my-cluster"
			rc.Status.ObservedGeneration = 1
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.RedshiftCluster{}).
			WithObjects(rc, redshiftPasswordSecret()).Build()
		r := &RedshiftClusterReconciler{Client: c, Scheme: scheme, RedshiftClient: f}

		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if res != requeueRedshiftPolling {
			t.Errorf("result = %+v, want polling requeue", res)
		}
		if f.modifyCalled {
			t.Error("ModifyCluster must not be called while cluster is transient")
		}
		if f.createCalled {
			t.Error("CreateCluster must not be called while cluster is transient")
		}
	})

	t.Run("generation change on available cluster calls ModifyCluster", func(t *testing.T) {
		ctx := context.Background()
		scheme := redshiftScheme(t)
		f := &fakeRedshiftClusterAPI{
			describe: func(_ context.Context, _ *awsredshift.DescribeClustersInput) (*awsredshift.DescribeClustersOutput, error) {
				return availableRedshiftDescribeOutput(), nil
			},
			modify: func(_ context.Context, params *awsredshift.ModifyClusterInput) (*awsredshift.ModifyClusterOutput, error) {
				if aws.ToString(params.ClusterIdentifier) != "my-cluster" {
					return nil, fmt.Errorf("unexpected cluster id")
				}
				if params.MasterUserPassword != nil {
					return nil, fmt.Errorf("password must not be sent on modify")
				}
				return &awsredshift.ModifyClusterOutput{}, nil
			},
		}
		rc := redshiftClusterCR(func(rc *awsv1alpha1.RedshiftCluster) {
			rc.Finalizers = []string{awsv1alpha1.FinalizerName}
			rc.Generation = 2
			rc.Status.ClusterIdentifier = "my-cluster"
			rc.Status.ObservedGeneration = 1
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.RedshiftCluster{}).
			WithObjects(rc, redshiftPasswordSecret()).Build()
		r := &RedshiftClusterReconciler{Client: c, Scheme: scheme, RedshiftClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.modifyCalled {
			t.Error("expected ModifyCluster to be called on generation change")
		}
		got := &awsv1alpha1.RedshiftCluster{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ObservedGeneration != 2 {
			t.Errorf("observedGeneration = %d, want 2", got.Status.ObservedGeneration)
		}
		assertNoRedshiftPasswordLeak(t, got)
	})

	t.Run("steady state does not create or modify", func(t *testing.T) {
		ctx := context.Background()
		scheme := redshiftScheme(t)
		f := &fakeRedshiftClusterAPI{
			describe: func(_ context.Context, _ *awsredshift.DescribeClustersInput) (*awsredshift.DescribeClustersOutput, error) {
				return availableRedshiftDescribeOutput(), nil
			},
		}
		rc := redshiftClusterCR(func(rc *awsv1alpha1.RedshiftCluster) {
			rc.Finalizers = []string{awsv1alpha1.FinalizerName}
			rc.Generation = 1
			rc.Status.ClusterIdentifier = "my-cluster"
			rc.Status.ObservedGeneration = 1
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.RedshiftCluster{}).
			WithObjects(rc, redshiftPasswordSecret()).Build()
		r := &RedshiftClusterReconciler{Client: c, Scheme: scheme, RedshiftClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.createCalled {
			t.Error("expected CreateCluster NOT to be called")
		}
		if f.modifyCalled {
			t.Error("expected ModifyCluster NOT to be called")
		}
	})

	t.Run("missing secret key errors and never leaks password", func(t *testing.T) {
		ctx := context.Background()
		scheme := redshiftScheme(t)
		f := &fakeRedshiftClusterAPI{}
		secret := redshiftPasswordSecret()
		secret.Data = map[string][]byte{"wrong-key": []byte(testRedshiftPassword)}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.RedshiftCluster{}).
			WithObjects(redshiftClusterCR(), secret).Build()
		r := &RedshiftClusterReconciler{Client: c, Scheme: scheme, RedshiftClient: f}

		if _, err := r.Reconcile(ctx, req); err == nil {
			t.Fatal("expected error for missing secret key")
		}
		if f.createCalled {
			t.Error("expected CreateCluster NOT to be called")
		}
		got := &awsv1alpha1.RedshiftCluster{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
		if cond == nil || cond.Status != metav1.ConditionFalse {
			t.Errorf("Ready condition = %+v, want False", cond)
		}
		assertNoRedshiftPasswordLeak(t, got)
	})

	t.Run("subnet group ref not ready requeues", func(t *testing.T) {
		ctx := context.Background()
		scheme := redshiftScheme(t)
		f := &fakeRedshiftClusterAPI{}
		rc := redshiftClusterCR(func(rc *awsv1alpha1.RedshiftCluster) {
			rc.Spec.ClusterSubnetGroupRef = "my-sng"
		})
		sng := &awsv1alpha1.RedshiftSubnetGroup{
			ObjectMeta: metav1.ObjectMeta{Name: "my-sng", Namespace: "default"},
			Spec: awsv1alpha1.RedshiftSubnetGroupSpec{
				Name:        "my-sng",
				Description: "test",
				SubnetRefs:  []awsv1alpha1.SubnetRef{{ID: "subnet-1"}},
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.RedshiftCluster{}, &awsv1alpha1.RedshiftSubnetGroup{}).
			WithObjects(rc, sng, redshiftPasswordSecret()).Build()
		r := &RedshiftClusterReconciler{Client: c, Scheme: scheme, RedshiftClient: f}

		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if res != requeueDependency {
			t.Errorf("result = %+v, want requeueDependency", res)
		}
		if f.createCalled {
			t.Error("CreateCluster must not be called while subnet group is not ready")
		}
	})

	t.Run("delete with skipFinalClusterSnapshot=true skips snapshot", func(t *testing.T) {
		ctx := context.Background()
		scheme := redshiftScheme(t)
		f := &fakeRedshiftClusterAPI{
			delete: func(_ context.Context, _ *awsredshift.DeleteClusterInput) (*awsredshift.DeleteClusterOutput, error) {
				return &awsredshift.DeleteClusterOutput{}, nil
			},
		}
		now := metav1.Now()
		rc := redshiftClusterCR(func(rc *awsv1alpha1.RedshiftCluster) {
			rc.Finalizers = []string{awsv1alpha1.FinalizerName}
			rc.DeletionTimestamp = &now
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.RedshiftCluster{}).
			WithObjects(rc).Build()
		r := &RedshiftClusterReconciler{Client: c, Scheme: scheme, RedshiftClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled {
			t.Fatal("expected DeleteCluster to be called")
		}
		if !aws.ToBool(f.deleteInput.SkipFinalClusterSnapshot) {
			t.Error("expected SkipFinalClusterSnapshot=true")
		}
		if f.deleteInput.FinalClusterSnapshotIdentifier != nil {
			t.Errorf("expected no FinalClusterSnapshotIdentifier, got %q", aws.ToString(f.deleteInput.FinalClusterSnapshotIdentifier))
		}
		got := &awsv1alpha1.RedshiftCluster{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected object gone after finalizer removal, got err=%v", err)
		}
	})

	t.Run("delete with skipFinalClusterSnapshot=false requests final snapshot", func(t *testing.T) {
		ctx := context.Background()
		scheme := redshiftScheme(t)
		f := &fakeRedshiftClusterAPI{
			delete: func(_ context.Context, _ *awsredshift.DeleteClusterInput) (*awsredshift.DeleteClusterOutput, error) {
				return &awsredshift.DeleteClusterOutput{}, nil
			},
		}
		now := metav1.Now()
		rc := redshiftClusterCR(func(rc *awsv1alpha1.RedshiftCluster) {
			rc.Finalizers = []string{awsv1alpha1.FinalizerName}
			rc.DeletionTimestamp = &now
			rc.Spec.SkipFinalClusterSnapshot = false
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.RedshiftCluster{}).
			WithObjects(rc).Build()
		r := &RedshiftClusterReconciler{Client: c, Scheme: scheme, RedshiftClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if aws.ToBool(f.deleteInput.SkipFinalClusterSnapshot) {
			t.Error("expected SkipFinalClusterSnapshot=false")
		}
		if aws.ToString(f.deleteInput.FinalClusterSnapshotIdentifier) != "my-cluster-final" {
			t.Errorf("FinalClusterSnapshotIdentifier = %q, want my-cluster-final", aws.ToString(f.deleteInput.FinalClusterSnapshotIdentifier))
		}
	})

	t.Run("abandon annotation skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := redshiftScheme(t)
		f := &fakeRedshiftClusterAPI{}
		now := metav1.Now()
		rc := redshiftClusterCR(func(rc *awsv1alpha1.RedshiftCluster) {
			rc.Finalizers = []string{awsv1alpha1.FinalizerName}
			rc.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			rc.DeletionTimestamp = &now
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.RedshiftCluster{}).
			WithObjects(rc).Build()
		r := &RedshiftClusterReconciler{Client: c, Scheme: scheme, RedshiftClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("expected DeleteCluster NOT to be called for abandoned resource")
		}
		got := &awsv1alpha1.RedshiftCluster{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected object gone after finalizer removal, got err=%v", err)
		}
	})
}
