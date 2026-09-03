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
	awssso "github.com/aws/aws-sdk-go-v2/service/ssoadmin"
	ssotypes "github.com/aws/aws-sdk-go-v2/service/ssoadmin/types"
	"github.com/aws/smithy-go"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

func ssoNotFoundErr() error {
	return &smithy.GenericAPIError{Code: "ResourceNotFoundException", Message: "not found"}
}

type fakePermissionSetAPI struct {
	createPermissionSet   func(ctx context.Context, params *awssso.CreatePermissionSetInput) (*awssso.CreatePermissionSetOutput, error)
	updatePermissionSet   func(ctx context.Context, params *awssso.UpdatePermissionSetInput) (*awssso.UpdatePermissionSetOutput, error)
	deletePermissionSet   func(ctx context.Context, params *awssso.DeletePermissionSetInput) (*awssso.DeletePermissionSetOutput, error)
	describePermissionSet func(ctx context.Context, params *awssso.DescribePermissionSetInput) (*awssso.DescribePermissionSetOutput, error)
	listManagedPolicies   func(ctx context.Context, params *awssso.ListManagedPoliciesInPermissionSetInput) (*awssso.ListManagedPoliciesInPermissionSetOutput, error)
	attachManagedPolicy   func(ctx context.Context, params *awssso.AttachManagedPolicyToPermissionSetInput) (*awssso.AttachManagedPolicyToPermissionSetOutput, error)
	detachManagedPolicy   func(ctx context.Context, params *awssso.DetachManagedPolicyFromPermissionSetInput) (*awssso.DetachManagedPolicyFromPermissionSetOutput, error)
	putInlinePolicy       func(ctx context.Context, params *awssso.PutInlinePolicyToPermissionSetInput) (*awssso.PutInlinePolicyToPermissionSetOutput, error)
	deleteInlinePolicy    func(ctx context.Context, params *awssso.DeleteInlinePolicyFromPermissionSetInput) (*awssso.DeleteInlinePolicyFromPermissionSetOutput, error)
	tagResource           func(ctx context.Context, params *awssso.TagResourceInput) (*awssso.TagResourceOutput, error)

	createCalled       bool
	updateCalled       bool
	deleteCalled       bool
	deletedArn         string
	attachedPolicies   []string
	detachedPolicies   []string
	putInlineCalled    bool
	deleteInlineCalled bool
}

func (f *fakePermissionSetAPI) CreatePermissionSet(ctx context.Context, params *awssso.CreatePermissionSetInput, _ ...func(*awssso.Options)) (*awssso.CreatePermissionSetOutput, error) {
	f.createCalled = true
	if f.createPermissionSet == nil {
		return nil, fmt.Errorf("unexpected call to CreatePermissionSet")
	}
	return f.createPermissionSet(ctx, params)
}

func (f *fakePermissionSetAPI) UpdatePermissionSet(ctx context.Context, params *awssso.UpdatePermissionSetInput, _ ...func(*awssso.Options)) (*awssso.UpdatePermissionSetOutput, error) {
	f.updateCalled = true
	if f.updatePermissionSet == nil {
		return nil, fmt.Errorf("unexpected call to UpdatePermissionSet")
	}
	return f.updatePermissionSet(ctx, params)
}

func (f *fakePermissionSetAPI) DeletePermissionSet(ctx context.Context, params *awssso.DeletePermissionSetInput, _ ...func(*awssso.Options)) (*awssso.DeletePermissionSetOutput, error) {
	f.deleteCalled = true
	f.deletedArn = aws.ToString(params.PermissionSetArn)
	if f.deletePermissionSet == nil {
		return nil, fmt.Errorf("unexpected call to DeletePermissionSet")
	}
	return f.deletePermissionSet(ctx, params)
}

func (f *fakePermissionSetAPI) DescribePermissionSet(ctx context.Context, params *awssso.DescribePermissionSetInput, _ ...func(*awssso.Options)) (*awssso.DescribePermissionSetOutput, error) {
	if f.describePermissionSet == nil {
		return nil, fmt.Errorf("unexpected call to DescribePermissionSet")
	}
	return f.describePermissionSet(ctx, params)
}

func (f *fakePermissionSetAPI) ListManagedPoliciesInPermissionSet(ctx context.Context, params *awssso.ListManagedPoliciesInPermissionSetInput, _ ...func(*awssso.Options)) (*awssso.ListManagedPoliciesInPermissionSetOutput, error) {
	if f.listManagedPolicies == nil {
		return &awssso.ListManagedPoliciesInPermissionSetOutput{}, nil
	}
	return f.listManagedPolicies(ctx, params)
}

func (f *fakePermissionSetAPI) AttachManagedPolicyToPermissionSet(ctx context.Context, params *awssso.AttachManagedPolicyToPermissionSetInput, _ ...func(*awssso.Options)) (*awssso.AttachManagedPolicyToPermissionSetOutput, error) {
	f.attachedPolicies = append(f.attachedPolicies, aws.ToString(params.ManagedPolicyArn))
	if f.attachManagedPolicy == nil {
		return &awssso.AttachManagedPolicyToPermissionSetOutput{}, nil
	}
	return f.attachManagedPolicy(ctx, params)
}

func (f *fakePermissionSetAPI) DetachManagedPolicyFromPermissionSet(ctx context.Context, params *awssso.DetachManagedPolicyFromPermissionSetInput, _ ...func(*awssso.Options)) (*awssso.DetachManagedPolicyFromPermissionSetOutput, error) {
	f.detachedPolicies = append(f.detachedPolicies, aws.ToString(params.ManagedPolicyArn))
	if f.detachManagedPolicy == nil {
		return &awssso.DetachManagedPolicyFromPermissionSetOutput{}, nil
	}
	return f.detachManagedPolicy(ctx, params)
}

func (f *fakePermissionSetAPI) PutInlinePolicyToPermissionSet(ctx context.Context, params *awssso.PutInlinePolicyToPermissionSetInput, _ ...func(*awssso.Options)) (*awssso.PutInlinePolicyToPermissionSetOutput, error) {
	f.putInlineCalled = true
	if f.putInlinePolicy == nil {
		return &awssso.PutInlinePolicyToPermissionSetOutput{}, nil
	}
	return f.putInlinePolicy(ctx, params)
}

func (f *fakePermissionSetAPI) DeleteInlinePolicyFromPermissionSet(ctx context.Context, params *awssso.DeleteInlinePolicyFromPermissionSetInput, _ ...func(*awssso.Options)) (*awssso.DeleteInlinePolicyFromPermissionSetOutput, error) {
	f.deleteInlineCalled = true
	if f.deleteInlinePolicy == nil {
		return &awssso.DeleteInlinePolicyFromPermissionSetOutput{}, nil
	}
	return f.deleteInlinePolicy(ctx, params)
}

func (f *fakePermissionSetAPI) TagResource(ctx context.Context, params *awssso.TagResourceInput, _ ...func(*awssso.Options)) (*awssso.TagResourceOutput, error) {
	if f.tagResource == nil {
		return &awssso.TagResourceOutput{}, nil
	}
	return f.tagResource(ctx, params)
}

const (
	testSSOInstanceARN    = "arn:aws:sso:::instance/ssoins-1234567890abcdef"
	testPermissionSetARN  = "arn:aws:sso:::permissionSet/ssoins-1234567890abcdef/ps-1234567890abcdef"
	testAdminPolicyARN    = "arn:aws:iam::aws:policy/AdministratorAccess"
	testReadOnlyPolicyARN = "arn:aws:iam::aws:policy/ReadOnlyAccess"
)

func permissionSetCR(mutate ...func(*awsv1alpha1.PermissionSet)) *awsv1alpha1.PermissionSet {
	ps := &awsv1alpha1.PermissionSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "admin-access",
			Namespace: "default",
		},
		Spec: awsv1alpha1.PermissionSetSpec{
			InstanceArn:     testSSOInstanceARN,
			Name:            "AdminAccess",
			Description:     "administrator access",
			SessionDuration: "PT8H",
			ManagedPolicies: []string{testAdminPolicyARN},
		},
	}
	for _, m := range mutate {
		m(ps)
	}
	return ps
}

func TestPermissionSetReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "admin-access", Namespace: "default"}}

	tests := []struct {
		name    string
		objs    []client.Object
		fake    *fakePermissionSetAPI
		setup   func(t *testing.T, ctx context.Context, c client.Client)
		wantErr bool
		assert  func(t *testing.T, ctx context.Context, c client.Client, f *fakePermissionSetAPI, res ctrl.Result)
	}{
		{
			name: "create happy path persists ARN, attaches managed policies, Ready",
			objs: []client.Object{permissionSetCR()},
			fake: &fakePermissionSetAPI{
				createPermissionSet: func(_ context.Context, params *awssso.CreatePermissionSetInput) (*awssso.CreatePermissionSetOutput, error) {
					if aws.ToString(params.InstanceArn) != testSSOInstanceARN {
						return nil, fmt.Errorf("unexpected instance ARN %q", aws.ToString(params.InstanceArn))
					}
					if aws.ToString(params.Name) != "AdminAccess" {
						return nil, fmt.Errorf("unexpected name %q", aws.ToString(params.Name))
					}
					if aws.ToString(params.SessionDuration) != "PT8H" {
						return nil, fmt.Errorf("unexpected session duration %q", aws.ToString(params.SessionDuration))
					}
					return &awssso.CreatePermissionSetOutput{
						PermissionSet: &ssotypes.PermissionSet{
							PermissionSetArn: aws.String(testPermissionSetARN),
						},
					}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakePermissionSetAPI, _ ctrl.Result) {
				got := &awsv1alpha1.PermissionSet{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if !f.createCalled {
					t.Error("expected CreatePermissionSet to be called")
				}
				if got.Status.PermissionSetArn != testPermissionSetARN {
					t.Errorf("status.permissionSetArn = %q, want %q", got.Status.PermissionSetArn, testPermissionSetARN)
				}
				if len(f.attachedPolicies) != 1 || f.attachedPolicies[0] != testAdminPolicyARN {
					t.Errorf("attached policies = %v, want [%s]", f.attachedPolicies, testAdminPolicyARN)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionTrue {
					t.Errorf("Ready condition = %+v, want True", cond)
				}
			},
		},
		{
			name: "identifier persisted even when a post-create step fails",
			objs: []client.Object{permissionSetCR()},
			fake: &fakePermissionSetAPI{
				createPermissionSet: func(_ context.Context, _ *awssso.CreatePermissionSetInput) (*awssso.CreatePermissionSetOutput, error) {
					return &awssso.CreatePermissionSetOutput{
						PermissionSet: &ssotypes.PermissionSet{
							PermissionSetArn: aws.String(testPermissionSetARN),
						},
					}, nil
				},
				attachManagedPolicy: func(_ context.Context, _ *awssso.AttachManagedPolicyToPermissionSetInput) (*awssso.AttachManagedPolicyToPermissionSetOutput, error) {
					return nil, fmt.Errorf("throttled")
				},
			},
			wantErr: true,
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakePermissionSetAPI, _ ctrl.Result) {
				got := &awsv1alpha1.PermissionSet{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.PermissionSetArn != testPermissionSetARN {
					t.Errorf("status.permissionSetArn = %q, want %q (must be persisted before post-create steps)", got.Status.PermissionSetArn, testPermissionSetARN)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionFalse {
					t.Errorf("Ready condition = %+v, want False", cond)
				}
			},
		},
		{
			name: "steady state does not create; syncs policies without churn",
			objs: []client.Object{permissionSetCR(func(ps *awsv1alpha1.PermissionSet) {
				ps.Finalizers = []string{awsv1alpha1.FinalizerName}
				ps.Generation = 1
				ps.Status.PermissionSetArn = testPermissionSetARN
				ps.Status.ObservedGeneration = 1
			})},
			fake: &fakePermissionSetAPI{
				describePermissionSet: func(_ context.Context, params *awssso.DescribePermissionSetInput) (*awssso.DescribePermissionSetOutput, error) {
					if aws.ToString(params.PermissionSetArn) != testPermissionSetARN {
						return nil, fmt.Errorf("unexpected ARN %q", aws.ToString(params.PermissionSetArn))
					}
					return &awssso.DescribePermissionSetOutput{
						PermissionSet: &ssotypes.PermissionSet{PermissionSetArn: aws.String(testPermissionSetARN)},
					}, nil
				},
				listManagedPolicies: func(_ context.Context, _ *awssso.ListManagedPoliciesInPermissionSetInput) (*awssso.ListManagedPoliciesInPermissionSetOutput, error) {
					return &awssso.ListManagedPoliciesInPermissionSetOutput{
						AttachedManagedPolicies: []ssotypes.AttachedManagedPolicy{{Arn: aws.String(testAdminPolicyARN)}},
					}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakePermissionSetAPI, _ ctrl.Result) {
				if f.createCalled {
					t.Error("CreatePermissionSet must not be called in steady state")
				}
				if f.updateCalled {
					t.Error("UpdatePermissionSet must not be called when generation unchanged")
				}
				if len(f.attachedPolicies) != 0 || len(f.detachedPolicies) != 0 {
					t.Errorf("policy churn in steady state: attached=%v detached=%v", f.attachedPolicies, f.detachedPolicies)
				}
			},
		},
		{
			name: "update path detaches extra and attaches missing managed policies, puts inline policy",
			objs: []client.Object{permissionSetCR(func(ps *awsv1alpha1.PermissionSet) {
				ps.Finalizers = []string{awsv1alpha1.FinalizerName}
				ps.Generation = 2
				ps.Spec.ManagedPolicies = []string{testReadOnlyPolicyARN}
				ps.Spec.InlinePolicy = `{"Version":"2012-10-17","Statement":[]}`
				ps.Status.PermissionSetArn = testPermissionSetARN
				ps.Status.ObservedGeneration = 1
			})},
			fake: &fakePermissionSetAPI{
				describePermissionSet: func(_ context.Context, _ *awssso.DescribePermissionSetInput) (*awssso.DescribePermissionSetOutput, error) {
					return &awssso.DescribePermissionSetOutput{
						PermissionSet: &ssotypes.PermissionSet{PermissionSetArn: aws.String(testPermissionSetARN)},
					}, nil
				},
				updatePermissionSet: func(_ context.Context, _ *awssso.UpdatePermissionSetInput) (*awssso.UpdatePermissionSetOutput, error) {
					return &awssso.UpdatePermissionSetOutput{}, nil
				},
				listManagedPolicies: func(_ context.Context, _ *awssso.ListManagedPoliciesInPermissionSetInput) (*awssso.ListManagedPoliciesInPermissionSetOutput, error) {
					return &awssso.ListManagedPoliciesInPermissionSetOutput{
						AttachedManagedPolicies: []ssotypes.AttachedManagedPolicy{{Arn: aws.String(testAdminPolicyARN)}},
					}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakePermissionSetAPI, _ ctrl.Result) {
				if !f.updateCalled {
					t.Error("expected UpdatePermissionSet to be called")
				}
				if len(f.attachedPolicies) != 1 || f.attachedPolicies[0] != testReadOnlyPolicyARN {
					t.Errorf("attached = %v, want [%s]", f.attachedPolicies, testReadOnlyPolicyARN)
				}
				if len(f.detachedPolicies) != 1 || f.detachedPolicies[0] != testAdminPolicyARN {
					t.Errorf("detached = %v, want [%s]", f.detachedPolicies, testAdminPolicyARN)
				}
				if !f.putInlineCalled {
					t.Error("expected PutInlinePolicyToPermissionSet to be called")
				}
				got := &awsv1alpha1.PermissionSet{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.ObservedGeneration != 2 {
					t.Errorf("observedGeneration = %d, want 2", got.Status.ObservedGeneration)
				}
			},
		},
		{
			name: "no inline policy in spec deletes lingering inline policy, tolerating NotFound",
			objs: []client.Object{permissionSetCR(func(ps *awsv1alpha1.PermissionSet) {
				ps.Finalizers = []string{awsv1alpha1.FinalizerName}
				ps.Generation = 1
				ps.Status.PermissionSetArn = testPermissionSetARN
				ps.Status.ObservedGeneration = 1
			})},
			fake: &fakePermissionSetAPI{
				describePermissionSet: func(_ context.Context, _ *awssso.DescribePermissionSetInput) (*awssso.DescribePermissionSetOutput, error) {
					return &awssso.DescribePermissionSetOutput{
						PermissionSet: &ssotypes.PermissionSet{PermissionSetArn: aws.String(testPermissionSetARN)},
					}, nil
				},
				listManagedPolicies: func(_ context.Context, _ *awssso.ListManagedPoliciesInPermissionSetInput) (*awssso.ListManagedPoliciesInPermissionSetOutput, error) {
					return &awssso.ListManagedPoliciesInPermissionSetOutput{
						AttachedManagedPolicies: []ssotypes.AttachedManagedPolicy{{Arn: aws.String(testAdminPolicyARN)}},
					}, nil
				},
				deleteInlinePolicy: func(_ context.Context, _ *awssso.DeleteInlinePolicyFromPermissionSetInput) (*awssso.DeleteInlinePolicyFromPermissionSetOutput, error) {
					return nil, ssoNotFoundErr()
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakePermissionSetAPI, _ ctrl.Result) {
				if !f.deleteInlineCalled {
					t.Error("expected DeleteInlinePolicyFromPermissionSet to be called")
				}
				got := &awsv1alpha1.PermissionSet{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionTrue {
					t.Errorf("Ready condition = %+v, want True (NotFound on inline delete is benign)", cond)
				}
			},
		},
		{
			name: "create failure surfaces error and Ready=False",
			objs: []client.Object{permissionSetCR()},
			fake: &fakePermissionSetAPI{
				createPermissionSet: func(_ context.Context, _ *awssso.CreatePermissionSetInput) (*awssso.CreatePermissionSetOutput, error) {
					return nil, fmt.Errorf("access denied")
				},
			},
			wantErr: true,
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakePermissionSetAPI, _ ctrl.Result) {
				got := &awsv1alpha1.PermissionSet{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionFalse {
					t.Errorf("Ready condition = %+v, want False", cond)
				}
				if got.Status.PermissionSetArn != "" {
					t.Errorf("status.permissionSetArn = %q, want empty", got.Status.PermissionSetArn)
				}
			},
		},
		{
			name: "delete with finalizer calls DeletePermissionSet with stored ARN",
			objs: []client.Object{permissionSetCR(func(ps *awsv1alpha1.PermissionSet) {
				ps.Finalizers = []string{awsv1alpha1.FinalizerName}
				ps.Status.PermissionSetArn = testPermissionSetARN
			})},
			fake: &fakePermissionSetAPI{
				deletePermissionSet: func(_ context.Context, params *awssso.DeletePermissionSetInput) (*awssso.DeletePermissionSetOutput, error) {
					if aws.ToString(params.InstanceArn) != testSSOInstanceARN {
						return nil, fmt.Errorf("unexpected instance ARN")
					}
					return &awssso.DeletePermissionSetOutput{}, nil
				},
			},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, permissionSetCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakePermissionSetAPI, _ ctrl.Result) {
				if !f.deleteCalled {
					t.Error("expected DeletePermissionSet to be called")
				}
				if f.deletedArn != testPermissionSetARN {
					t.Errorf("DeletePermissionSet ARN = %q, want %q", f.deletedArn, testPermissionSetARN)
				}
				got := &awsv1alpha1.PermissionSet{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "delete with empty status ARN skips AWS delete",
			objs: []client.Object{permissionSetCR(func(ps *awsv1alpha1.PermissionSet) {
				ps.Finalizers = []string{awsv1alpha1.FinalizerName}
			})},
			fake: &fakePermissionSetAPI{},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, permissionSetCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakePermissionSetAPI, _ ctrl.Result) {
				if f.deleteCalled {
					t.Error("DeletePermissionSet must not be called when no ARN was stored")
				}
			},
		},
		{
			name: "abandon annotation skips AWS delete",
			objs: []client.Object{permissionSetCR(func(ps *awsv1alpha1.PermissionSet) {
				ps.Finalizers = []string{awsv1alpha1.FinalizerName}
				ps.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
				ps.Status.PermissionSetArn = testPermissionSetARN
			})},
			fake: &fakePermissionSetAPI{},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, permissionSetCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakePermissionSetAPI, _ ctrl.Result) {
				if f.deleteCalled {
					t.Error("DeletePermissionSet must not be called when abandoning")
				}
				got := &awsv1alpha1.PermissionSet{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			scheme := newOrgScheme(t)
			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&awsv1alpha1.PermissionSet{}).
				WithObjects(tc.objs...).
				Build()
			r := &PermissionSetReconciler{Client: c, Scheme: scheme, SSOAdminClient: tc.fake}

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
