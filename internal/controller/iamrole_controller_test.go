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

// fakeIAMRoleAPI implements IAMRoleAWSAPI via function fields. Unset fields
// return empty outputs so tests only wire the calls they care about.
type fakeIAMRoleAPI struct {
	GetRoleFn                  func(ctx context.Context, params *awsiam.GetRoleInput, optFns ...func(*awsiam.Options)) (*awsiam.GetRoleOutput, error)
	CreateRoleFn               func(ctx context.Context, params *awsiam.CreateRoleInput, optFns ...func(*awsiam.Options)) (*awsiam.CreateRoleOutput, error)
	UpdateAssumeRolePolicyFn   func(ctx context.Context, params *awsiam.UpdateAssumeRolePolicyInput, optFns ...func(*awsiam.Options)) (*awsiam.UpdateAssumeRolePolicyOutput, error)
	UpdateRoleFn               func(ctx context.Context, params *awsiam.UpdateRoleInput, optFns ...func(*awsiam.Options)) (*awsiam.UpdateRoleOutput, error)
	DeleteRoleFn               func(ctx context.Context, params *awsiam.DeleteRoleInput, optFns ...func(*awsiam.Options)) (*awsiam.DeleteRoleOutput, error)
	ListRoleTagsFn             func(ctx context.Context, params *awsiam.ListRoleTagsInput, optFns ...func(*awsiam.Options)) (*awsiam.ListRoleTagsOutput, error)
	TagRoleFn                  func(ctx context.Context, params *awsiam.TagRoleInput, optFns ...func(*awsiam.Options)) (*awsiam.TagRoleOutput, error)
	UntagRoleFn                func(ctx context.Context, params *awsiam.UntagRoleInput, optFns ...func(*awsiam.Options)) (*awsiam.UntagRoleOutput, error)
	ListAttachedRolePoliciesFn func(ctx context.Context, params *awsiam.ListAttachedRolePoliciesInput, optFns ...func(*awsiam.Options)) (*awsiam.ListAttachedRolePoliciesOutput, error)
	DetachRolePolicyFn         func(ctx context.Context, params *awsiam.DetachRolePolicyInput, optFns ...func(*awsiam.Options)) (*awsiam.DetachRolePolicyOutput, error)
	ListRolePoliciesFn         func(ctx context.Context, params *awsiam.ListRolePoliciesInput, optFns ...func(*awsiam.Options)) (*awsiam.ListRolePoliciesOutput, error)
	DeleteRolePolicyFn         func(ctx context.Context, params *awsiam.DeleteRolePolicyInput, optFns ...func(*awsiam.Options)) (*awsiam.DeleteRolePolicyOutput, error)
}

func (f *fakeIAMRoleAPI) GetRole(ctx context.Context, params *awsiam.GetRoleInput, optFns ...func(*awsiam.Options)) (*awsiam.GetRoleOutput, error) {
	if f.GetRoleFn != nil {
		return f.GetRoleFn(ctx, params, optFns...)
	}
	return nil, &iamtypes.NoSuchEntityException{}
}

func (f *fakeIAMRoleAPI) CreateRole(ctx context.Context, params *awsiam.CreateRoleInput, optFns ...func(*awsiam.Options)) (*awsiam.CreateRoleOutput, error) {
	if f.CreateRoleFn != nil {
		return f.CreateRoleFn(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("unexpected CreateRole call")
}

func (f *fakeIAMRoleAPI) UpdateAssumeRolePolicy(ctx context.Context, params *awsiam.UpdateAssumeRolePolicyInput, optFns ...func(*awsiam.Options)) (*awsiam.UpdateAssumeRolePolicyOutput, error) {
	if f.UpdateAssumeRolePolicyFn != nil {
		return f.UpdateAssumeRolePolicyFn(ctx, params, optFns...)
	}
	return &awsiam.UpdateAssumeRolePolicyOutput{}, nil
}

func (f *fakeIAMRoleAPI) UpdateRole(ctx context.Context, params *awsiam.UpdateRoleInput, optFns ...func(*awsiam.Options)) (*awsiam.UpdateRoleOutput, error) {
	if f.UpdateRoleFn != nil {
		return f.UpdateRoleFn(ctx, params, optFns...)
	}
	return &awsiam.UpdateRoleOutput{}, nil
}

func (f *fakeIAMRoleAPI) DeleteRole(ctx context.Context, params *awsiam.DeleteRoleInput, optFns ...func(*awsiam.Options)) (*awsiam.DeleteRoleOutput, error) {
	if f.DeleteRoleFn != nil {
		return f.DeleteRoleFn(ctx, params, optFns...)
	}
	return &awsiam.DeleteRoleOutput{}, nil
}

func (f *fakeIAMRoleAPI) ListRoleTags(ctx context.Context, params *awsiam.ListRoleTagsInput, optFns ...func(*awsiam.Options)) (*awsiam.ListRoleTagsOutput, error) {
	if f.ListRoleTagsFn != nil {
		return f.ListRoleTagsFn(ctx, params, optFns...)
	}
	return &awsiam.ListRoleTagsOutput{}, nil
}

func (f *fakeIAMRoleAPI) TagRole(ctx context.Context, params *awsiam.TagRoleInput, optFns ...func(*awsiam.Options)) (*awsiam.TagRoleOutput, error) {
	if f.TagRoleFn != nil {
		return f.TagRoleFn(ctx, params, optFns...)
	}
	return &awsiam.TagRoleOutput{}, nil
}

func (f *fakeIAMRoleAPI) UntagRole(ctx context.Context, params *awsiam.UntagRoleInput, optFns ...func(*awsiam.Options)) (*awsiam.UntagRoleOutput, error) {
	if f.UntagRoleFn != nil {
		return f.UntagRoleFn(ctx, params, optFns...)
	}
	return &awsiam.UntagRoleOutput{}, nil
}

func (f *fakeIAMRoleAPI) ListAttachedRolePolicies(ctx context.Context, params *awsiam.ListAttachedRolePoliciesInput, optFns ...func(*awsiam.Options)) (*awsiam.ListAttachedRolePoliciesOutput, error) {
	if f.ListAttachedRolePoliciesFn != nil {
		return f.ListAttachedRolePoliciesFn(ctx, params, optFns...)
	}
	return &awsiam.ListAttachedRolePoliciesOutput{}, nil
}

func (f *fakeIAMRoleAPI) DetachRolePolicy(ctx context.Context, params *awsiam.DetachRolePolicyInput, optFns ...func(*awsiam.Options)) (*awsiam.DetachRolePolicyOutput, error) {
	if f.DetachRolePolicyFn != nil {
		return f.DetachRolePolicyFn(ctx, params, optFns...)
	}
	return &awsiam.DetachRolePolicyOutput{}, nil
}

func (f *fakeIAMRoleAPI) ListRolePolicies(ctx context.Context, params *awsiam.ListRolePoliciesInput, optFns ...func(*awsiam.Options)) (*awsiam.ListRolePoliciesOutput, error) {
	if f.ListRolePoliciesFn != nil {
		return f.ListRolePoliciesFn(ctx, params, optFns...)
	}
	return &awsiam.ListRolePoliciesOutput{}, nil
}

func (f *fakeIAMRoleAPI) DeleteRolePolicy(ctx context.Context, params *awsiam.DeleteRolePolicyInput, optFns ...func(*awsiam.Options)) (*awsiam.DeleteRolePolicyOutput, error) {
	if f.DeleteRolePolicyFn != nil {
		return f.DeleteRolePolicyFn(ctx, params, optFns...)
	}
	return &awsiam.DeleteRolePolicyOutput{}, nil
}

func iamRoleTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return s
}

func newIAMRoleFakeClient(s *runtime.Scheme, objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(&awsv1alpha1.IAMRole{}).
		Build()
}

const testTrustPolicy = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"eks.amazonaws.com"},"Action":"sts:AssumeRole"}]}`

func newTestIAMRole(mutators ...func(*awsv1alpha1.IAMRole)) *awsv1alpha1.IAMRole {
	r := &awsv1alpha1.IAMRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-role",
			Namespace: "default",
		},
		Spec: awsv1alpha1.IAMRoleSpec{
			RoleName:                 "my-role",
			AssumeRolePolicyDocument: testTrustPolicy,
		},
	}
	for _, m := range mutators {
		m(r)
	}
	return r
}

func reconcileIAMRoleOnce(t *testing.T, r *IAMRoleReconciler, name k8stypes.NamespacedName) (ctrl.Result, error) {
	t.Helper()
	return r.Reconcile(context.Background(), ctrl.Request{NamespacedName: name})
}

func TestIAMRoleCreateHappyPath(t *testing.T) {
	s := iamRoleTestScheme(t)
	role := newTestIAMRole()
	cl := newIAMRoleFakeClient(s, role)

	var createdInput *awsiam.CreateRoleInput
	fakeAWS := &fakeIAMRoleAPI{
		CreateRoleFn: func(_ context.Context, params *awsiam.CreateRoleInput, _ ...func(*awsiam.Options)) (*awsiam.CreateRoleOutput, error) {
			createdInput = params
			return &awsiam.CreateRoleOutput{Role: &iamtypes.Role{
				Arn:    aws.String("arn:aws:iam::123456789012:role/my-role"),
				RoleId: aws.String("AROAEXAMPLE"),
			}}, nil
		},
	}
	r := &IAMRoleReconciler{Client: cl, Scheme: s, IAMClient: fakeAWS}

	key := k8stypes.NamespacedName{Name: "test-role", Namespace: "default"}
	if _, err := reconcileIAMRoleOnce(t, r, key); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	if createdInput == nil {
		t.Fatal("expected CreateRole to be called")
	}
	if aws.ToString(createdInput.RoleName) != "my-role" {
		t.Errorf("CreateRole role name = %q, want my-role", aws.ToString(createdInput.RoleName))
	}

	got := &awsv1alpha1.IAMRole{}
	if err := cl.Get(context.Background(), key, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.ARN != "arn:aws:iam::123456789012:role/my-role" {
		t.Errorf("status ARN = %q", got.Status.ARN)
	}
	if got.Status.RoleID != "AROAEXAMPLE" {
		t.Errorf("status RoleID = %q", got.Status.RoleID)
	}
	cond := meta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
	if cond == nil || cond.Status != metav1.ConditionTrue {
		t.Errorf("Ready condition = %+v, want True", cond)
	}
}

func TestIAMRoleARNPersistedWhenTagSyncFails(t *testing.T) {
	s := iamRoleTestScheme(t)
	role := newTestIAMRole()
	cl := newIAMRoleFakeClient(s, role)

	fakeAWS := &fakeIAMRoleAPI{
		CreateRoleFn: func(_ context.Context, _ *awsiam.CreateRoleInput, _ ...func(*awsiam.Options)) (*awsiam.CreateRoleOutput, error) {
			return &awsiam.CreateRoleOutput{Role: &iamtypes.Role{
				Arn:    aws.String("arn:aws:iam::123456789012:role/my-role"),
				RoleId: aws.String("AROAEXAMPLE"),
			}}, nil
		},
		ListRoleTagsFn: func(_ context.Context, _ *awsiam.ListRoleTagsInput, _ ...func(*awsiam.Options)) (*awsiam.ListRoleTagsOutput, error) {
			return nil, fmt.Errorf("throttled")
		},
	}
	r := &IAMRoleReconciler{Client: cl, Scheme: s, IAMClient: fakeAWS}

	key := k8stypes.NamespacedName{Name: "test-role", Namespace: "default"}
	if _, err := reconcileIAMRoleOnce(t, r, key); err == nil {
		t.Fatal("expected reconcile error when tag sync fails")
	}

	got := &awsv1alpha1.IAMRole{}
	if err := cl.Get(context.Background(), key, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.ARN != "arn:aws:iam::123456789012:role/my-role" {
		t.Errorf("status ARN = %q; the role ARN must be persisted even when a later step fails", got.Status.ARN)
	}
}

func TestIAMRoleSteadyStateNoCreate(t *testing.T) {
	s := iamRoleTestScheme(t)
	role := newTestIAMRole(func(r *awsv1alpha1.IAMRole) {
		r.Finalizers = []string{awsv1alpha1.FinalizerName}
	})
	cl := newIAMRoleFakeClient(s, role)

	createCalls, updateTrustCalls := 0, 0
	fakeAWS := &fakeIAMRoleAPI{
		GetRoleFn: func(_ context.Context, _ *awsiam.GetRoleInput, _ ...func(*awsiam.Options)) (*awsiam.GetRoleOutput, error) {
			return &awsiam.GetRoleOutput{Role: &iamtypes.Role{
				Arn:                      aws.String("arn:aws:iam::123456789012:role/my-role"),
				RoleId:                   aws.String("AROAEXAMPLE"),
				AssumeRolePolicyDocument: aws.String(testTrustPolicy),
			}}, nil
		},
		CreateRoleFn: func(_ context.Context, _ *awsiam.CreateRoleInput, _ ...func(*awsiam.Options)) (*awsiam.CreateRoleOutput, error) {
			createCalls++
			return nil, fmt.Errorf("should not be called")
		},
		UpdateAssumeRolePolicyFn: func(_ context.Context, _ *awsiam.UpdateAssumeRolePolicyInput, _ ...func(*awsiam.Options)) (*awsiam.UpdateAssumeRolePolicyOutput, error) {
			updateTrustCalls++
			return &awsiam.UpdateAssumeRolePolicyOutput{}, nil
		},
	}
	r := &IAMRoleReconciler{Client: cl, Scheme: s, IAMClient: fakeAWS}

	key := k8stypes.NamespacedName{Name: "test-role", Namespace: "default"}
	if _, err := reconcileIAMRoleOnce(t, r, key); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if createCalls != 0 {
		t.Errorf("CreateRole called %d times, want 0", createCalls)
	}
	if updateTrustCalls != 0 {
		t.Errorf("UpdateAssumeRolePolicy called %d times, want 0", updateTrustCalls)
	}

	got := &awsv1alpha1.IAMRole{}
	if err := cl.Get(context.Background(), key, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.ARN != "arn:aws:iam::123456789012:role/my-role" {
		t.Errorf("status ARN = %q", got.Status.ARN)
	}
}

func TestIAMRoleTrustPolicyUpdate(t *testing.T) {
	s := iamRoleTestScheme(t)
	role := newTestIAMRole(func(r *awsv1alpha1.IAMRole) {
		r.Finalizers = []string{awsv1alpha1.FinalizerName}
	})
	cl := newIAMRoleFakeClient(s, role)

	var updatedDoc string
	fakeAWS := &fakeIAMRoleAPI{
		GetRoleFn: func(_ context.Context, _ *awsiam.GetRoleInput, _ ...func(*awsiam.Options)) (*awsiam.GetRoleOutput, error) {
			return &awsiam.GetRoleOutput{Role: &iamtypes.Role{
				Arn:                      aws.String("arn:aws:iam::123456789012:role/my-role"),
				RoleId:                   aws.String("AROAEXAMPLE"),
				AssumeRolePolicyDocument: aws.String(`{"old":"trust"}`),
			}}, nil
		},
		UpdateAssumeRolePolicyFn: func(_ context.Context, params *awsiam.UpdateAssumeRolePolicyInput, _ ...func(*awsiam.Options)) (*awsiam.UpdateAssumeRolePolicyOutput, error) {
			updatedDoc = aws.ToString(params.PolicyDocument)
			return &awsiam.UpdateAssumeRolePolicyOutput{}, nil
		},
	}
	r := &IAMRoleReconciler{Client: cl, Scheme: s, IAMClient: fakeAWS}

	key := k8stypes.NamespacedName{Name: "test-role", Namespace: "default"}
	if _, err := reconcileIAMRoleOnce(t, r, key); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if updatedDoc != testTrustPolicy {
		t.Errorf("UpdateAssumeRolePolicy doc = %q, want spec trust policy", updatedDoc)
	}
}

func TestIAMRoleDeleteWithFinalizer(t *testing.T) {
	s := iamRoleTestScheme(t)
	role := newTestIAMRole(func(r *awsv1alpha1.IAMRole) {
		r.Finalizers = []string{awsv1alpha1.FinalizerName}
	})
	cl := newIAMRoleFakeClient(s, role)

	deleteCalls := 0
	fakeAWS := &fakeIAMRoleAPI{
		DeleteRoleFn: func(_ context.Context, params *awsiam.DeleteRoleInput, _ ...func(*awsiam.Options)) (*awsiam.DeleteRoleOutput, error) {
			deleteCalls++
			if aws.ToString(params.RoleName) != "my-role" {
				t.Errorf("DeleteRole role name = %q", aws.ToString(params.RoleName))
			}
			return &awsiam.DeleteRoleOutput{}, nil
		},
	}
	r := &IAMRoleReconciler{Client: cl, Scheme: s, IAMClient: fakeAWS}

	ctx := context.Background()
	if err := cl.Delete(ctx, role); err != nil {
		t.Fatalf("delete: %v", err)
	}

	key := k8stypes.NamespacedName{Name: "test-role", Namespace: "default"}
	if _, err := reconcileIAMRoleOnce(t, r, key); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if deleteCalls != 1 {
		t.Errorf("DeleteRole called %d times, want 1", deleteCalls)
	}

	got := &awsv1alpha1.IAMRole{}
	if err := cl.Get(ctx, key, got); !errors.IsNotFound(err) {
		t.Errorf("expected object gone after finalizer removal, got err=%v", err)
	}
}

func TestIAMRoleAbandonAnnotation(t *testing.T) {
	s := iamRoleTestScheme(t)
	role := newTestIAMRole(func(r *awsv1alpha1.IAMRole) {
		r.Finalizers = []string{awsv1alpha1.FinalizerName}
		r.Annotations = map[string]string{
			awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon,
		}
	})
	cl := newIAMRoleFakeClient(s, role)

	deleteCalls := 0
	fakeAWS := &fakeIAMRoleAPI{
		DeleteRoleFn: func(_ context.Context, _ *awsiam.DeleteRoleInput, _ ...func(*awsiam.Options)) (*awsiam.DeleteRoleOutput, error) {
			deleteCalls++
			return &awsiam.DeleteRoleOutput{}, nil
		},
	}
	r := &IAMRoleReconciler{Client: cl, Scheme: s, IAMClient: fakeAWS}

	ctx := context.Background()
	if err := cl.Delete(ctx, role); err != nil {
		t.Fatalf("delete: %v", err)
	}

	key := k8stypes.NamespacedName{Name: "test-role", Namespace: "default"}
	if _, err := reconcileIAMRoleOnce(t, r, key); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if deleteCalls != 0 {
		t.Errorf("DeleteRole called %d times, want 0 (abandon)", deleteCalls)
	}

	got := &awsv1alpha1.IAMRole{}
	if err := cl.Get(ctx, key, got); !errors.IsNotFound(err) {
		t.Errorf("expected object gone after finalizer removal, got err=%v", err)
	}
}
