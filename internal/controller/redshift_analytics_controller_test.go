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

// Create + delete + abandon coverage for RedshiftSubnetGroup and
// RedshiftParameterGroup. RedshiftCluster carries the family's full test
// suite in redshiftcluster_controller_test.go.

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsredshift "github.com/aws/aws-sdk-go-v2/service/redshift"
	redshifttypes "github.com/aws/aws-sdk-go-v2/service/redshift/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

func redshiftAnalyticsScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return scheme
}

// ---- RedshiftSubnetGroup ----

type fakeRedshiftSubnetGroupAPI struct {
	createCalled bool
	deleteCalled bool
	deletedName  string
	createInput  *awsredshift.CreateClusterSubnetGroupInput
}

func (f *fakeRedshiftSubnetGroupAPI) DescribeClusterSubnetGroups(context.Context, *awsredshift.DescribeClusterSubnetGroupsInput, ...func(*awsredshift.Options)) (*awsredshift.DescribeClusterSubnetGroupsOutput, error) {
	return nil, &redshifttypes.ClusterSubnetGroupNotFoundFault{Message: aws.String("not found")}
}
func (f *fakeRedshiftSubnetGroupAPI) CreateClusterSubnetGroup(_ context.Context, params *awsredshift.CreateClusterSubnetGroupInput, _ ...func(*awsredshift.Options)) (*awsredshift.CreateClusterSubnetGroupOutput, error) {
	f.createCalled = true
	f.createInput = params
	return &awsredshift.CreateClusterSubnetGroupOutput{}, nil
}
func (f *fakeRedshiftSubnetGroupAPI) ModifyClusterSubnetGroup(context.Context, *awsredshift.ModifyClusterSubnetGroupInput, ...func(*awsredshift.Options)) (*awsredshift.ModifyClusterSubnetGroupOutput, error) {
	return &awsredshift.ModifyClusterSubnetGroupOutput{}, nil
}
func (f *fakeRedshiftSubnetGroupAPI) DeleteClusterSubnetGroup(_ context.Context, params *awsredshift.DeleteClusterSubnetGroupInput, _ ...func(*awsredshift.Options)) (*awsredshift.DeleteClusterSubnetGroupOutput, error) {
	f.deleteCalled = true
	f.deletedName = aws.ToString(params.ClusterSubnetGroupName)
	return &awsredshift.DeleteClusterSubnetGroupOutput{}, nil
}

func TestRedshiftSubnetGroupReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-sng", Namespace: "default"}}
	newCR := func(mutate ...func(*awsv1alpha1.RedshiftSubnetGroup)) *awsv1alpha1.RedshiftSubnetGroup {
		sg := &awsv1alpha1.RedshiftSubnetGroup{
			ObjectMeta: metav1.ObjectMeta{Name: "my-sng", Namespace: "default"},
			Spec: awsv1alpha1.RedshiftSubnetGroupSpec{
				Name:        "my-sng",
				Description: "test subnet group",
				SubnetRefs:  []awsv1alpha1.SubnetRef{{ID: "subnet-1"}, {ID: "subnet-2"}},
			},
		}
		for _, m := range mutate {
			m(sg)
		}
		return sg
	}

	t.Run("create", func(t *testing.T) {
		ctx := context.Background()
		scheme := redshiftAnalyticsScheme(t)
		f := &fakeRedshiftSubnetGroupAPI{}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.RedshiftSubnetGroup{}).
			WithObjects(newCR()).Build()
		r := &RedshiftSubnetGroupReconciler{Client: c, Scheme: scheme, RedshiftClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.createCalled {
			t.Fatal("expected CreateClusterSubnetGroup to be called")
		}
		if len(f.createInput.SubnetIds) != 2 {
			t.Errorf("subnet ids = %v, want 2 entries", f.createInput.SubnetIds)
		}
		got := &awsv1alpha1.RedshiftSubnetGroup{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.SubnetGroupName != "my-sng" {
			t.Errorf("status.subnetGroupName = %q", got.Status.SubnetGroupName)
		}
		cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
		if cond == nil || cond.Status != metav1.ConditionTrue {
			t.Errorf("Ready condition = %+v, want True", cond)
		}
	})

	t.Run("delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := redshiftAnalyticsScheme(t)
		f := &fakeRedshiftSubnetGroupAPI{}
		now := metav1.Now()
		sg := newCR(func(sg *awsv1alpha1.RedshiftSubnetGroup) {
			sg.Finalizers = []string{awsv1alpha1.FinalizerName}
			sg.DeletionTimestamp = &now
			sg.Status.SubnetGroupName = "my-sng"
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.RedshiftSubnetGroup{}).
			WithObjects(sg).Build()
		r := &RedshiftSubnetGroupReconciler{Client: c, Scheme: scheme, RedshiftClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled || f.deletedName != "my-sng" {
			t.Errorf("DeleteClusterSubnetGroup called=%v name=%q", f.deleteCalled, f.deletedName)
		}
		got := &awsv1alpha1.RedshiftSubnetGroup{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected CR gone, got err=%v", err)
		}
	})

	t.Run("abandon", func(t *testing.T) {
		ctx := context.Background()
		scheme := redshiftAnalyticsScheme(t)
		f := &fakeRedshiftSubnetGroupAPI{}
		now := metav1.Now()
		sg := newCR(func(sg *awsv1alpha1.RedshiftSubnetGroup) {
			sg.Finalizers = []string{awsv1alpha1.FinalizerName}
			sg.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			sg.DeletionTimestamp = &now
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.RedshiftSubnetGroup{}).
			WithObjects(sg).Build()
		r := &RedshiftSubnetGroupReconciler{Client: c, Scheme: scheme, RedshiftClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("DeleteClusterSubnetGroup must not be called when abandoning")
		}
	})
}

// ---- RedshiftParameterGroup ----

type fakeRedshiftParameterGroupAPI struct {
	createCalled bool
	modifyCalled bool
	deleteCalled bool
	deletedName  string
	modifyInput  *awsredshift.ModifyClusterParameterGroupInput
}

func (f *fakeRedshiftParameterGroupAPI) DescribeClusterParameterGroups(context.Context, *awsredshift.DescribeClusterParameterGroupsInput, ...func(*awsredshift.Options)) (*awsredshift.DescribeClusterParameterGroupsOutput, error) {
	return nil, &redshifttypes.ClusterParameterGroupNotFoundFault{Message: aws.String("not found")}
}
func (f *fakeRedshiftParameterGroupAPI) CreateClusterParameterGroup(_ context.Context, _ *awsredshift.CreateClusterParameterGroupInput, _ ...func(*awsredshift.Options)) (*awsredshift.CreateClusterParameterGroupOutput, error) {
	f.createCalled = true
	return &awsredshift.CreateClusterParameterGroupOutput{}, nil
}
func (f *fakeRedshiftParameterGroupAPI) ModifyClusterParameterGroup(_ context.Context, params *awsredshift.ModifyClusterParameterGroupInput, _ ...func(*awsredshift.Options)) (*awsredshift.ModifyClusterParameterGroupOutput, error) {
	f.modifyCalled = true
	f.modifyInput = params
	return &awsredshift.ModifyClusterParameterGroupOutput{}, nil
}
func (f *fakeRedshiftParameterGroupAPI) DeleteClusterParameterGroup(_ context.Context, params *awsredshift.DeleteClusterParameterGroupInput, _ ...func(*awsredshift.Options)) (*awsredshift.DeleteClusterParameterGroupOutput, error) {
	f.deleteCalled = true
	f.deletedName = aws.ToString(params.ParameterGroupName)
	return &awsredshift.DeleteClusterParameterGroupOutput{}, nil
}

func TestRedshiftParameterGroupReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-pg", Namespace: "default"}}
	newCR := func(mutate ...func(*awsv1alpha1.RedshiftParameterGroup)) *awsv1alpha1.RedshiftParameterGroup {
		pg := &awsv1alpha1.RedshiftParameterGroup{
			ObjectMeta: metav1.ObjectMeta{Name: "my-pg", Namespace: "default"},
			Spec: awsv1alpha1.RedshiftParameterGroupSpec{
				Name:        "my-pg",
				Family:      "redshift-1.0",
				Description: "test parameter group",
				Parameters: []awsv1alpha1.RedshiftParameter{
					{Name: "enable_user_activity_logging", Value: "true"},
				},
			},
		}
		for _, m := range mutate {
			m(pg)
		}
		return pg
	}

	t.Run("create applies parameters", func(t *testing.T) {
		ctx := context.Background()
		scheme := redshiftAnalyticsScheme(t)
		f := &fakeRedshiftParameterGroupAPI{}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.RedshiftParameterGroup{}).
			WithObjects(newCR()).Build()
		r := &RedshiftParameterGroupReconciler{Client: c, Scheme: scheme, RedshiftClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.createCalled {
			t.Fatal("expected CreateClusterParameterGroup to be called")
		}
		if !f.modifyCalled {
			t.Fatal("expected ModifyClusterParameterGroup to apply parameters after create")
		}
		if len(f.modifyInput.Parameters) != 1 || aws.ToString(f.modifyInput.Parameters[0].ParameterName) != "enable_user_activity_logging" {
			t.Errorf("parameters = %+v", f.modifyInput.Parameters)
		}
		got := &awsv1alpha1.RedshiftParameterGroup{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ParameterGroupName != "my-pg" {
			t.Errorf("status.parameterGroupName = %q", got.Status.ParameterGroupName)
		}
	})

	t.Run("delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := redshiftAnalyticsScheme(t)
		f := &fakeRedshiftParameterGroupAPI{}
		now := metav1.Now()
		pg := newCR(func(pg *awsv1alpha1.RedshiftParameterGroup) {
			pg.Finalizers = []string{awsv1alpha1.FinalizerName}
			pg.DeletionTimestamp = &now
			pg.Status.ParameterGroupName = "my-pg"
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.RedshiftParameterGroup{}).
			WithObjects(pg).Build()
		r := &RedshiftParameterGroupReconciler{Client: c, Scheme: scheme, RedshiftClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled || f.deletedName != "my-pg" {
			t.Errorf("DeleteClusterParameterGroup called=%v name=%q", f.deleteCalled, f.deletedName)
		}
	})

	t.Run("abandon", func(t *testing.T) {
		ctx := context.Background()
		scheme := redshiftAnalyticsScheme(t)
		f := &fakeRedshiftParameterGroupAPI{}
		now := metav1.Now()
		pg := newCR(func(pg *awsv1alpha1.RedshiftParameterGroup) {
			pg.Finalizers = []string{awsv1alpha1.FinalizerName}
			pg.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			pg.DeletionTimestamp = &now
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.RedshiftParameterGroup{}).
			WithObjects(pg).Build()
		r := &RedshiftParameterGroupReconciler{Client: c, Scheme: scheme, RedshiftClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("DeleteClusterParameterGroup must not be called when abandoning")
		}
	})
}
