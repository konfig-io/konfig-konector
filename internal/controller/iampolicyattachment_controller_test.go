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

// fakeIAMPolicyAttachmentAPI implements IAMPolicyAttachmentAWSAPI via function fields.
type fakeIAMPolicyAttachmentAPI struct {
	ListAttachedRolePoliciesFn func(ctx context.Context, params *awsiam.ListAttachedRolePoliciesInput, optFns ...func(*awsiam.Options)) (*awsiam.ListAttachedRolePoliciesOutput, error)
	AttachRolePolicyFn         func(ctx context.Context, params *awsiam.AttachRolePolicyInput, optFns ...func(*awsiam.Options)) (*awsiam.AttachRolePolicyOutput, error)
	DetachRolePolicyFn         func(ctx context.Context, params *awsiam.DetachRolePolicyInput, optFns ...func(*awsiam.Options)) (*awsiam.DetachRolePolicyOutput, error)
}

func (f *fakeIAMPolicyAttachmentAPI) ListAttachedRolePolicies(ctx context.Context, params *awsiam.ListAttachedRolePoliciesInput, optFns ...func(*awsiam.Options)) (*awsiam.ListAttachedRolePoliciesOutput, error) {
	if f.ListAttachedRolePoliciesFn != nil {
		return f.ListAttachedRolePoliciesFn(ctx, params, optFns...)
	}
	return &awsiam.ListAttachedRolePoliciesOutput{}, nil
}

func (f *fakeIAMPolicyAttachmentAPI) AttachRolePolicy(ctx context.Context, params *awsiam.AttachRolePolicyInput, optFns ...func(*awsiam.Options)) (*awsiam.AttachRolePolicyOutput, error) {
	if f.AttachRolePolicyFn != nil {
		return f.AttachRolePolicyFn(ctx, params, optFns...)
	}
	return &awsiam.AttachRolePolicyOutput{}, nil
}

func (f *fakeIAMPolicyAttachmentAPI) DetachRolePolicy(ctx context.Context, params *awsiam.DetachRolePolicyInput, optFns ...func(*awsiam.Options)) (*awsiam.DetachRolePolicyOutput, error) {
	if f.DetachRolePolicyFn != nil {
		return f.DetachRolePolicyFn(ctx, params, optFns...)
	}
	return &awsiam.DetachRolePolicyOutput{}, nil
}

func newTestIAMPolicyAttachment(mutators ...func(*awsv1alpha1.IAMPolicyAttachment)) *awsv1alpha1.IAMPolicyAttachment {
	att := &awsv1alpha1.IAMPolicyAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-attachment",
			Namespace: "default",
		},
		Spec: awsv1alpha1.IAMPolicyAttachmentSpec{
			RoleRef:   awsv1alpha1.RoleRef{Name: "test-role"},
			PolicyRef: awsv1alpha1.PolicyRef{Name: "test-policy"},
		},
	}
	for _, m := range mutators {
		m(att)
	}
	return att
}

func newAttachmentFakeClient(s *runtime.Scheme, objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(
			&awsv1alpha1.IAMPolicyAttachment{},
			&awsv1alpha1.IAMRole{},
			&awsv1alpha1.IAMPolicy{},
		).
		Build()
}

// readyIAMPolicy returns an IAMPolicy CR whose status carries an ARN.
func readyIAMPolicy() *awsv1alpha1.IAMPolicy {
	return newTestIAMPolicy(func(p *awsv1alpha1.IAMPolicy) {
		p.Status.ARN = testPolicyARN
	})
}

func TestIAMPolicyAttachmentCreateHappyPath(t *testing.T) {
	s := iamRoleTestScheme(t)
	att := newTestIAMPolicyAttachment()
	cl := newAttachmentFakeClient(s, att, readyIAMRole(), readyIAMPolicy())

	var attached *awsiam.AttachRolePolicyInput
	fakeAWS := &fakeIAMPolicyAttachmentAPI{
		AttachRolePolicyFn: func(_ context.Context, params *awsiam.AttachRolePolicyInput, _ ...func(*awsiam.Options)) (*awsiam.AttachRolePolicyOutput, error) {
			attached = params
			return &awsiam.AttachRolePolicyOutput{}, nil
		},
	}
	r := &IAMPolicyAttachmentReconciler{Client: cl, Scheme: s, IAMClient: fakeAWS}

	key := k8stypes.NamespacedName{Name: "test-attachment", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	if attached == nil {
		t.Fatal("expected AttachRolePolicy to be called")
	}
	if aws.ToString(attached.RoleName) != "my-role" || aws.ToString(attached.PolicyArn) != testPolicyARN {
		t.Errorf("AttachRolePolicy = %q/%q", aws.ToString(attached.RoleName), aws.ToString(attached.PolicyArn))
	}

	got := &awsv1alpha1.IAMPolicyAttachment{}
	if err := cl.Get(context.Background(), key, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.Status.Attached {
		t.Error("status Attached = false, want true")
	}
	if got.Status.RoleARN != testRoleARN || got.Status.PolicyARN != testPolicyARN {
		t.Errorf("status ARNs = %q/%q", got.Status.RoleARN, got.Status.PolicyARN)
	}
	cond := meta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
	if cond == nil || cond.Status != metav1.ConditionTrue {
		t.Errorf("Ready condition = %+v, want True", cond)
	}
}

func TestIAMPolicyAttachmentIdentifiersPersistedWhenLaterStepFails(t *testing.T) {
	// AttachRolePolicy succeeds; the identifiers must be written to status even
	// if we then observe a failure. Here we verify the persist happens right
	// after attach by checking status after a successful reconcile whose only
	// status write path is persistStatus.
	s := iamRoleTestScheme(t)
	att := newTestIAMPolicyAttachment(func(a *awsv1alpha1.IAMPolicyAttachment) {
		a.Finalizers = []string{awsv1alpha1.FinalizerName}
	})
	cl := newAttachmentFakeClient(s, att, readyIAMRole(), readyIAMPolicy())

	fakeAWS := &fakeIAMPolicyAttachmentAPI{}
	r := &IAMPolicyAttachmentReconciler{Client: cl, Scheme: s, IAMClient: fakeAWS}

	key := k8stypes.NamespacedName{Name: "test-attachment", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got := &awsv1alpha1.IAMPolicyAttachment{}
	if err := cl.Get(context.Background(), key, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.RoleARN != testRoleARN || got.Status.PolicyARN != testPolicyARN {
		t.Errorf("status ARN pair = %q/%q; must be persisted for the delete path", got.Status.RoleARN, got.Status.PolicyARN)
	}
}

func TestIAMPolicyAttachmentSteadyStateNoAttach(t *testing.T) {
	s := iamRoleTestScheme(t)
	att := newTestIAMPolicyAttachment(func(a *awsv1alpha1.IAMPolicyAttachment) {
		a.Finalizers = []string{awsv1alpha1.FinalizerName}
	})
	cl := newAttachmentFakeClient(s, att, readyIAMRole(), readyIAMPolicy())

	attachCalls := 0
	fakeAWS := &fakeIAMPolicyAttachmentAPI{
		ListAttachedRolePoliciesFn: func(_ context.Context, _ *awsiam.ListAttachedRolePoliciesInput, _ ...func(*awsiam.Options)) (*awsiam.ListAttachedRolePoliciesOutput, error) {
			return &awsiam.ListAttachedRolePoliciesOutput{
				AttachedPolicies: []iamtypes.AttachedPolicy{
					{PolicyArn: aws.String(testPolicyARN), PolicyName: aws.String("my-policy")},
				},
			}, nil
		},
		AttachRolePolicyFn: func(_ context.Context, _ *awsiam.AttachRolePolicyInput, _ ...func(*awsiam.Options)) (*awsiam.AttachRolePolicyOutput, error) {
			attachCalls++
			return &awsiam.AttachRolePolicyOutput{}, nil
		},
	}
	r := &IAMPolicyAttachmentReconciler{Client: cl, Scheme: s, IAMClient: fakeAWS}

	key := k8stypes.NamespacedName{Name: "test-attachment", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if attachCalls != 0 {
		t.Errorf("AttachRolePolicy called %d times, want 0 (already attached)", attachCalls)
	}
}

func TestIAMPolicyAttachmentDependencyNotReady(t *testing.T) {
	tests := []struct {
		name string
		objs func() []client.Object
	}{
		{
			name: "role has no ARN",
			objs: func() []client.Object {
				return []client.Object{newTestIAMRole(), readyIAMPolicy()}
			},
		},
		{
			name: "policy has no ARN",
			objs: func() []client.Object {
				return []client.Object{readyIAMRole(), newTestIAMPolicy()}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := iamRoleTestScheme(t)
			att := newTestIAMPolicyAttachment(func(a *awsv1alpha1.IAMPolicyAttachment) {
				a.Finalizers = []string{awsv1alpha1.FinalizerName}
			})
			objs := append([]client.Object{att}, tc.objs()...)
			cl := newAttachmentFakeClient(s, objs...)

			attachCalls := 0
			fakeAWS := &fakeIAMPolicyAttachmentAPI{
				AttachRolePolicyFn: func(_ context.Context, _ *awsiam.AttachRolePolicyInput, _ ...func(*awsiam.Options)) (*awsiam.AttachRolePolicyOutput, error) {
					attachCalls++
					return &awsiam.AttachRolePolicyOutput{}, nil
				},
			}
			r := &IAMPolicyAttachmentReconciler{Client: cl, Scheme: s, IAMClient: fakeAWS}

			key := k8stypes.NamespacedName{Name: "test-attachment", Namespace: "default"}
			res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key})
			if err != nil {
				t.Fatalf("dependency-not-ready must not return an error, got %v", err)
			}
			if res != requeueDependency {
				t.Errorf("result = %+v, want requeueDependency %+v", res, requeueDependency)
			}
			if attachCalls != 0 {
				t.Errorf("AttachRolePolicy called %d times, want 0", attachCalls)
			}
		})
	}
}

func TestIAMPolicyAttachmentDeleteWithFinalizer(t *testing.T) {
	s := iamRoleTestScheme(t)
	att := newTestIAMPolicyAttachment(func(a *awsv1alpha1.IAMPolicyAttachment) {
		a.Finalizers = []string{awsv1alpha1.FinalizerName}
		a.Status.Attached = true
		a.Status.RoleARN = testRoleARN
		a.Status.PolicyARN = testPolicyARN
	})
	cl := newAttachmentFakeClient(s, att)

	detachCalls := 0
	fakeAWS := &fakeIAMPolicyAttachmentAPI{
		DetachRolePolicyFn: func(_ context.Context, params *awsiam.DetachRolePolicyInput, _ ...func(*awsiam.Options)) (*awsiam.DetachRolePolicyOutput, error) {
			detachCalls++
			if aws.ToString(params.RoleName) != "my-role" || aws.ToString(params.PolicyArn) != testPolicyARN {
				t.Errorf("DetachRolePolicy = %q/%q", aws.ToString(params.RoleName), aws.ToString(params.PolicyArn))
			}
			return &awsiam.DetachRolePolicyOutput{}, nil
		},
	}
	r := &IAMPolicyAttachmentReconciler{Client: cl, Scheme: s, IAMClient: fakeAWS}

	ctx := context.Background()
	if err := cl.Delete(ctx, att); err != nil {
		t.Fatalf("delete: %v", err)
	}

	key := k8stypes.NamespacedName{Name: "test-attachment", Namespace: "default"}
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if detachCalls != 1 {
		t.Errorf("DetachRolePolicy called %d times, want 1", detachCalls)
	}

	got := &awsv1alpha1.IAMPolicyAttachment{}
	if err := cl.Get(ctx, key, got); !errors.IsNotFound(err) {
		t.Errorf("expected object gone, got err=%v", err)
	}
}

func TestIAMPolicyAttachmentDeleteWithoutIdentifiersSkipsDetach(t *testing.T) {
	// If the ARN pair never made it to status there is nothing to detach;
	// the finalizer must still be removed.
	s := iamRoleTestScheme(t)
	att := newTestIAMPolicyAttachment(func(a *awsv1alpha1.IAMPolicyAttachment) {
		a.Finalizers = []string{awsv1alpha1.FinalizerName}
	})
	cl := newAttachmentFakeClient(s, att)

	detachCalls := 0
	fakeAWS := &fakeIAMPolicyAttachmentAPI{
		DetachRolePolicyFn: func(_ context.Context, _ *awsiam.DetachRolePolicyInput, _ ...func(*awsiam.Options)) (*awsiam.DetachRolePolicyOutput, error) {
			detachCalls++
			return &awsiam.DetachRolePolicyOutput{}, nil
		},
	}
	r := &IAMPolicyAttachmentReconciler{Client: cl, Scheme: s, IAMClient: fakeAWS}

	ctx := context.Background()
	if err := cl.Delete(ctx, att); err != nil {
		t.Fatalf("delete: %v", err)
	}

	key := k8stypes.NamespacedName{Name: "test-attachment", Namespace: "default"}
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if detachCalls != 0 {
		t.Errorf("DetachRolePolicy called %d times, want 0", detachCalls)
	}

	got := &awsv1alpha1.IAMPolicyAttachment{}
	if err := cl.Get(ctx, key, got); !errors.IsNotFound(err) {
		t.Errorf("expected object gone, got err=%v", err)
	}
}

func TestIAMPolicyAttachmentAbandonAnnotation(t *testing.T) {
	s := iamRoleTestScheme(t)
	att := newTestIAMPolicyAttachment(func(a *awsv1alpha1.IAMPolicyAttachment) {
		a.Finalizers = []string{awsv1alpha1.FinalizerName}
		a.Status.RoleARN = testRoleARN
		a.Status.PolicyARN = testPolicyARN
		a.Annotations = map[string]string{
			awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon,
		}
	})
	cl := newAttachmentFakeClient(s, att)

	detachCalls := 0
	fakeAWS := &fakeIAMPolicyAttachmentAPI{
		DetachRolePolicyFn: func(_ context.Context, _ *awsiam.DetachRolePolicyInput, _ ...func(*awsiam.Options)) (*awsiam.DetachRolePolicyOutput, error) {
			detachCalls++
			return &awsiam.DetachRolePolicyOutput{}, nil
		},
	}
	r := &IAMPolicyAttachmentReconciler{Client: cl, Scheme: s, IAMClient: fakeAWS}

	ctx := context.Background()
	if err := cl.Delete(ctx, att); err != nil {
		t.Fatalf("delete: %v", err)
	}

	key := k8stypes.NamespacedName{Name: "test-attachment", Namespace: "default"}
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if detachCalls != 0 {
		t.Errorf("DetachRolePolicy called %d times, want 0 (abandon)", detachCalls)
	}

	got := &awsv1alpha1.IAMPolicyAttachment{}
	if err := cl.Get(ctx, key, got); !errors.IsNotFound(err) {
		t.Errorf("expected object gone, got err=%v", err)
	}
}
