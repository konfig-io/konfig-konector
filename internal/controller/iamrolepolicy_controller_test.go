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
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsiam "github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

// fakeIAMRolePolicyAPI implements IAMRolePolicyAWSAPI via function fields.
type fakeIAMRolePolicyAPI struct {
	GetRolePolicyFn    func(ctx context.Context, params *awsiam.GetRolePolicyInput, optFns ...func(*awsiam.Options)) (*awsiam.GetRolePolicyOutput, error)
	PutRolePolicyFn    func(ctx context.Context, params *awsiam.PutRolePolicyInput, optFns ...func(*awsiam.Options)) (*awsiam.PutRolePolicyOutput, error)
	DeleteRolePolicyFn func(ctx context.Context, params *awsiam.DeleteRolePolicyInput, optFns ...func(*awsiam.Options)) (*awsiam.DeleteRolePolicyOutput, error)
}

func (f *fakeIAMRolePolicyAPI) GetRolePolicy(ctx context.Context, params *awsiam.GetRolePolicyInput, optFns ...func(*awsiam.Options)) (*awsiam.GetRolePolicyOutput, error) {
	if f.GetRolePolicyFn != nil {
		return f.GetRolePolicyFn(ctx, params, optFns...)
	}
	return nil, &iamtypes.NoSuchEntityException{}
}

func (f *fakeIAMRolePolicyAPI) PutRolePolicy(ctx context.Context, params *awsiam.PutRolePolicyInput, optFns ...func(*awsiam.Options)) (*awsiam.PutRolePolicyOutput, error) {
	if f.PutRolePolicyFn != nil {
		return f.PutRolePolicyFn(ctx, params, optFns...)
	}
	return &awsiam.PutRolePolicyOutput{}, nil
}

func (f *fakeIAMRolePolicyAPI) DeleteRolePolicy(ctx context.Context, params *awsiam.DeleteRolePolicyInput, optFns ...func(*awsiam.Options)) (*awsiam.DeleteRolePolicyOutput, error) {
	if f.DeleteRolePolicyFn != nil {
		return f.DeleteRolePolicyFn(ctx, params, optFns...)
	}
	return &awsiam.DeleteRolePolicyOutput{}, nil
}

const testRoleARN = "arn:aws:iam::123456789012:role/my-role"

func newTestIAMRolePolicy(mutators ...func(*awsv1alpha1.IAMRolePolicy)) *awsv1alpha1.IAMRolePolicy {
	rp := &awsv1alpha1.IAMRolePolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rolepolicy",
			Namespace: "default",
		},
		Spec: awsv1alpha1.IAMRolePolicySpec{
			RoleRef:        awsv1alpha1.RoleRef{Name: "test-role"},
			PolicyName:     "inline-policy",
			PolicyDocument: testPolicyDoc,
		},
	}
	for _, m := range mutators {
		m(rp)
	}
	return rp
}

func newIAMRolePolicyFakeClient(s *runtime.Scheme, objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(&awsv1alpha1.IAMRolePolicy{}, &awsv1alpha1.IAMRole{}).
		Build()
}

// readyIAMRole returns an IAMRole CR whose status carries an ARN (dependency ready).
func readyIAMRole() *awsv1alpha1.IAMRole {
	return newTestIAMRole(func(r *awsv1alpha1.IAMRole) {
		r.Status.ARN = testRoleARN
	})
}

func TestIAMRolePolicyCreateHappyPath(t *testing.T) {
	s := iamRoleTestScheme(t)
	rp := newTestIAMRolePolicy()
	cl := newIAMRolePolicyFakeClient(s, rp, readyIAMRole())

	var put *awsiam.PutRolePolicyInput
	fakeAWS := &fakeIAMRolePolicyAPI{
		PutRolePolicyFn: func(_ context.Context, params *awsiam.PutRolePolicyInput, _ ...func(*awsiam.Options)) (*awsiam.PutRolePolicyOutput, error) {
			put = params
			return &awsiam.PutRolePolicyOutput{}, nil
		},
	}
	r := &IAMRolePolicyReconciler{Client: cl, Scheme: s, IAMClient: fakeAWS}

	key := k8stypes.NamespacedName{Name: "test-rolepolicy", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	if put == nil {
		t.Fatal("expected PutRolePolicy to be called")
	}
	if aws.ToString(put.RoleName) != "my-role" || aws.ToString(put.PolicyName) != "inline-policy" {
		t.Errorf("PutRolePolicy role/policy = %q/%q", aws.ToString(put.RoleName), aws.ToString(put.PolicyName))
	}

	got := &awsv1alpha1.IAMRolePolicy{}
	if err := cl.Get(context.Background(), key, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.RoleARN != testRoleARN {
		t.Errorf("status RoleARN = %q", got.Status.RoleARN)
	}
	cond := meta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
	if cond == nil || cond.Status != metav1.ConditionTrue {
		t.Errorf("Ready condition = %+v, want True", cond)
	}
}

func TestIAMRolePolicySteadyStateNoPut(t *testing.T) {
	s := iamRoleTestScheme(t)
	rp := newTestIAMRolePolicy(func(rp *awsv1alpha1.IAMRolePolicy) {
		rp.Finalizers = []string{awsv1alpha1.FinalizerName}
	})
	cl := newIAMRolePolicyFakeClient(s, rp, readyIAMRole())

	putCalls := 0
	fakeAWS := &fakeIAMRolePolicyAPI{
		GetRolePolicyFn: func(_ context.Context, _ *awsiam.GetRolePolicyInput, _ ...func(*awsiam.Options)) (*awsiam.GetRolePolicyOutput, error) {
			return &awsiam.GetRolePolicyOutput{PolicyDocument: aws.String(testPolicyDoc)}, nil
		},
		PutRolePolicyFn: func(_ context.Context, _ *awsiam.PutRolePolicyInput, _ ...func(*awsiam.Options)) (*awsiam.PutRolePolicyOutput, error) {
			putCalls++
			return &awsiam.PutRolePolicyOutput{}, nil
		},
	}
	r := &IAMRolePolicyReconciler{Client: cl, Scheme: s, IAMClient: fakeAWS}

	key := k8stypes.NamespacedName{Name: "test-rolepolicy", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if putCalls != 0 {
		t.Errorf("PutRolePolicy called %d times, want 0 (document unchanged)", putCalls)
	}
}

func TestIAMRolePolicyDependencyNotReady(t *testing.T) {
	s := iamRoleTestScheme(t)
	rp := newTestIAMRolePolicy(func(rp *awsv1alpha1.IAMRolePolicy) {
		rp.Finalizers = []string{awsv1alpha1.FinalizerName}
	})
	// Role exists but has no ARN yet.
	cl := newIAMRolePolicyFakeClient(s, rp, newTestIAMRole())

	putCalls := 0
	fakeAWS := &fakeIAMRolePolicyAPI{
		PutRolePolicyFn: func(_ context.Context, _ *awsiam.PutRolePolicyInput, _ ...func(*awsiam.Options)) (*awsiam.PutRolePolicyOutput, error) {
			putCalls++
			return &awsiam.PutRolePolicyOutput{}, nil
		},
	}
	r := &IAMRolePolicyReconciler{Client: cl, Scheme: s, IAMClient: fakeAWS}

	key := k8stypes.NamespacedName{Name: "test-rolepolicy", Namespace: "default"}
	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key})
	if err != nil {
		t.Fatalf("dependency-not-ready must not return an error, got %v", err)
	}
	if res != requeueDependency {
		t.Errorf("result = %+v, want requeueDependency %+v", res, requeueDependency)
	}
	if putCalls != 0 {
		t.Errorf("PutRolePolicy called %d times, want 0", putCalls)
	}
}

func TestIAMRolePolicyDeleteWithFinalizer(t *testing.T) {
	s := iamRoleTestScheme(t)
	rp := newTestIAMRolePolicy(func(rp *awsv1alpha1.IAMRolePolicy) {
		rp.Finalizers = []string{awsv1alpha1.FinalizerName}
		rp.Status.RoleARN = testRoleARN
	})
	cl := newIAMRolePolicyFakeClient(s, rp)

	deleteCalls := 0
	fakeAWS := &fakeIAMRolePolicyAPI{
		DeleteRolePolicyFn: func(_ context.Context, params *awsiam.DeleteRolePolicyInput, _ ...func(*awsiam.Options)) (*awsiam.DeleteRolePolicyOutput, error) {
			deleteCalls++
			if aws.ToString(params.RoleName) != "my-role" {
				t.Errorf("DeleteRolePolicy role = %q, want my-role (from status ARN)", aws.ToString(params.RoleName))
			}
			if aws.ToString(params.PolicyName) != "inline-policy" {
				t.Errorf("DeleteRolePolicy policy = %q", aws.ToString(params.PolicyName))
			}
			return &awsiam.DeleteRolePolicyOutput{}, nil
		},
	}
	r := &IAMRolePolicyReconciler{Client: cl, Scheme: s, IAMClient: fakeAWS}

	ctx := context.Background()
	if err := cl.Delete(ctx, rp); err != nil {
		t.Fatalf("delete: %v", err)
	}

	key := k8stypes.NamespacedName{Name: "test-rolepolicy", Namespace: "default"}
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if deleteCalls != 1 {
		t.Errorf("DeleteRolePolicy called %d times, want 1", deleteCalls)
	}

	got := &awsv1alpha1.IAMRolePolicy{}
	if err := cl.Get(ctx, key, got); !errors.IsNotFound(err) {
		t.Errorf("expected object gone, got err=%v", err)
	}
}

func TestIAMRolePolicyDeleteToleratesNotFound(t *testing.T) {
	s := iamRoleTestScheme(t)
	rp := newTestIAMRolePolicy(func(rp *awsv1alpha1.IAMRolePolicy) {
		rp.Finalizers = []string{awsv1alpha1.FinalizerName}
		rp.Status.RoleARN = testRoleARN
	})
	cl := newIAMRolePolicyFakeClient(s, rp)

	fakeAWS := &fakeIAMRolePolicyAPI{
		DeleteRolePolicyFn: func(_ context.Context, _ *awsiam.DeleteRolePolicyInput, _ ...func(*awsiam.Options)) (*awsiam.DeleteRolePolicyOutput, error) {
			return nil, &iamtypes.NoSuchEntityException{Message: aws.String("gone")}
		},
	}
	r := &IAMRolePolicyReconciler{Client: cl, Scheme: s, IAMClient: fakeAWS}

	ctx := context.Background()
	if err := cl.Delete(ctx, rp); err != nil {
		t.Fatalf("delete: %v", err)
	}

	key := k8stypes.NamespacedName{Name: "test-rolepolicy", Namespace: "default"}
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile should tolerate NoSuchEntity on delete: %v", err)
	}

	got := &awsv1alpha1.IAMRolePolicy{}
	if err := cl.Get(ctx, key, got); !errors.IsNotFound(err) {
		t.Errorf("expected object gone, got err=%v", err)
	}
}

func TestIAMRolePolicyAbandonAnnotation(t *testing.T) {
	s := iamRoleTestScheme(t)
	rp := newTestIAMRolePolicy(func(rp *awsv1alpha1.IAMRolePolicy) {
		rp.Finalizers = []string{awsv1alpha1.FinalizerName}
		rp.Status.RoleARN = testRoleARN
		rp.Annotations = map[string]string{
			awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon,
		}
	})
	cl := newIAMRolePolicyFakeClient(s, rp)

	deleteCalls := 0
	fakeAWS := &fakeIAMRolePolicyAPI{
		DeleteRolePolicyFn: func(_ context.Context, _ *awsiam.DeleteRolePolicyInput, _ ...func(*awsiam.Options)) (*awsiam.DeleteRolePolicyOutput, error) {
			deleteCalls++
			return &awsiam.DeleteRolePolicyOutput{}, nil
		},
	}
	r := &IAMRolePolicyReconciler{Client: cl, Scheme: s, IAMClient: fakeAWS}

	ctx := context.Background()
	if err := cl.Delete(ctx, rp); err != nil {
		t.Fatalf("delete: %v", err)
	}

	key := k8stypes.NamespacedName{Name: "test-rolepolicy", Namespace: "default"}
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if deleteCalls != 0 {
		t.Errorf("DeleteRolePolicy called %d times, want 0 (abandon)", deleteCalls)
	}

	got := &awsv1alpha1.IAMRolePolicy{}
	if err := cl.Get(ctx, key, got); !errors.IsNotFound(err) {
		t.Errorf("expected object gone, got err=%v", err)
	}
}
