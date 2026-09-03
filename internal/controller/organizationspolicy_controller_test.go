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
	awsorgs "github.com/aws/aws-sdk-go-v2/service/organizations"
	orgtypes "github.com/aws/aws-sdk-go-v2/service/organizations/types"
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

func orgPolicyNotFoundErr() error {
	return &smithy.GenericAPIError{Code: "PolicyNotFoundException", Message: "policy not found"}
}

type fakeOrgPolicy struct {
	createPolicy   func(ctx context.Context, params *awsorgs.CreatePolicyInput) (*awsorgs.CreatePolicyOutput, error)
	updatePolicy   func(ctx context.Context, params *awsorgs.UpdatePolicyInput) (*awsorgs.UpdatePolicyOutput, error)
	deletePolicy   func(ctx context.Context, params *awsorgs.DeletePolicyInput) (*awsorgs.DeletePolicyOutput, error)
	describePolicy func(ctx context.Context, params *awsorgs.DescribePolicyInput) (*awsorgs.DescribePolicyOutput, error)
	tagResource    func(ctx context.Context, params *awsorgs.TagResourceInput) (*awsorgs.TagResourceOutput, error)

	createCalled bool
	createInput  *awsorgs.CreatePolicyInput
	updateCalled bool
	deleteCalled bool
	deletedID    string
	tagCalled    bool
}

func (f *fakeOrgPolicy) CreatePolicy(ctx context.Context, params *awsorgs.CreatePolicyInput, _ ...func(*awsorgs.Options)) (*awsorgs.CreatePolicyOutput, error) {
	f.createCalled = true
	f.createInput = params
	if f.createPolicy == nil {
		return nil, fmt.Errorf("unexpected call to CreatePolicy")
	}
	return f.createPolicy(ctx, params)
}

func (f *fakeOrgPolicy) UpdatePolicy(ctx context.Context, params *awsorgs.UpdatePolicyInput, _ ...func(*awsorgs.Options)) (*awsorgs.UpdatePolicyOutput, error) {
	f.updateCalled = true
	if f.updatePolicy == nil {
		return nil, fmt.Errorf("unexpected call to UpdatePolicy")
	}
	return f.updatePolicy(ctx, params)
}

func (f *fakeOrgPolicy) DeletePolicy(ctx context.Context, params *awsorgs.DeletePolicyInput, _ ...func(*awsorgs.Options)) (*awsorgs.DeletePolicyOutput, error) {
	f.deleteCalled = true
	f.deletedID = aws.ToString(params.PolicyId)
	if f.deletePolicy == nil {
		return nil, fmt.Errorf("unexpected call to DeletePolicy")
	}
	return f.deletePolicy(ctx, params)
}

func (f *fakeOrgPolicy) DescribePolicy(ctx context.Context, params *awsorgs.DescribePolicyInput, _ ...func(*awsorgs.Options)) (*awsorgs.DescribePolicyOutput, error) {
	if f.describePolicy == nil {
		return nil, fmt.Errorf("unexpected call to DescribePolicy")
	}
	return f.describePolicy(ctx, params)
}

func (f *fakeOrgPolicy) TagResource(ctx context.Context, params *awsorgs.TagResourceInput, _ ...func(*awsorgs.Options)) (*awsorgs.TagResourceOutput, error) {
	f.tagCalled = true
	if f.tagResource == nil {
		return nil, fmt.Errorf("unexpected call to TagResource")
	}
	return f.tagResource(ctx, params)
}

const (
	testOrgPolicyID  = "p-examplepolicy1"
	testOrgPolicyARN = "arn:aws:organizations::123456789012:policy/o-example/service_control_policy/p-examplepolicy1"
)

func orgPolicyCR(mutate ...func(*awsv1alpha1.OrganizationsPolicy)) *awsv1alpha1.OrganizationsPolicy {
	pol := &awsv1alpha1.OrganizationsPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "deny-regions",
			Namespace: "default",
		},
		Spec: awsv1alpha1.OrganizationsPolicySpec{
			Name:        "deny-regions",
			Type:        "SERVICE_CONTROL_POLICY",
			Content:     `{"Version":"2012-10-17","Statement":[]}`,
			Description: "deny disallowed regions",
		},
	}
	for _, m := range mutate {
		m(pol)
	}
	return pol
}

func newOrgScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return scheme
}

func TestOrganizationsPolicyReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "deny-regions", Namespace: "default"}}

	tests := []struct {
		name    string
		objs    []client.Object
		fake    *fakeOrgPolicy
		setup   func(t *testing.T, ctx context.Context, c client.Client)
		wantErr bool
		assert  func(t *testing.T, ctx context.Context, c client.Client, f *fakeOrgPolicy, res ctrl.Result)
	}{
		{
			name: "create happy path persists policy ID, ARN and Ready",
			objs: []client.Object{orgPolicyCR()},
			fake: &fakeOrgPolicy{
				createPolicy: func(_ context.Context, params *awsorgs.CreatePolicyInput) (*awsorgs.CreatePolicyOutput, error) {
					if aws.ToString(params.Name) != "deny-regions" {
						return nil, fmt.Errorf("unexpected policy name %q", aws.ToString(params.Name))
					}
					if params.Type != orgtypes.PolicyTypeServiceControlPolicy {
						return nil, fmt.Errorf("unexpected policy type %q", params.Type)
					}
					if aws.ToString(params.Content) == "" {
						return nil, fmt.Errorf("expected content")
					}
					return &awsorgs.CreatePolicyOutput{
						Policy: &orgtypes.Policy{
							PolicySummary: &orgtypes.PolicySummary{
								Id:  aws.String(testOrgPolicyID),
								Arn: aws.String(testOrgPolicyARN),
							},
						},
					}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeOrgPolicy, _ ctrl.Result) {
				got := &awsv1alpha1.OrganizationsPolicy{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if !f.createCalled {
					t.Error("expected CreatePolicy to be called")
				}
				if got.Status.PolicyID != testOrgPolicyID {
					t.Errorf("status.policyId = %q, want %q", got.Status.PolicyID, testOrgPolicyID)
				}
				if got.Status.ARN != testOrgPolicyARN {
					t.Errorf("status.arn = %q, want %q", got.Status.ARN, testOrgPolicyARN)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionTrue {
					t.Errorf("Ready condition = %+v, want True", cond)
				}
			},
		},
		{
			name: "steady state does not create or update",
			objs: []client.Object{orgPolicyCR(func(pol *awsv1alpha1.OrganizationsPolicy) {
				pol.Finalizers = []string{awsv1alpha1.FinalizerName}
				pol.Generation = 1
				pol.Status.PolicyID = testOrgPolicyID
				pol.Status.ARN = testOrgPolicyARN
				pol.Status.ObservedGeneration = 1
			})},
			fake: &fakeOrgPolicy{
				describePolicy: func(_ context.Context, params *awsorgs.DescribePolicyInput) (*awsorgs.DescribePolicyOutput, error) {
					if aws.ToString(params.PolicyId) != testOrgPolicyID {
						return nil, fmt.Errorf("unexpected policy ID %q", aws.ToString(params.PolicyId))
					}
					return &awsorgs.DescribePolicyOutput{Policy: &orgtypes.Policy{
						PolicySummary: &orgtypes.PolicySummary{Id: aws.String(testOrgPolicyID)},
					}}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeOrgPolicy, _ ctrl.Result) {
				if f.createCalled {
					t.Error("CreatePolicy must not be called in steady state")
				}
				if f.updateCalled {
					t.Error("UpdatePolicy must not be called when generation is unchanged")
				}
				got := &awsv1alpha1.OrganizationsPolicy{}
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
			name: "update path calls UpdatePolicy and TagResource when generation changed",
			objs: []client.Object{orgPolicyCR(func(pol *awsv1alpha1.OrganizationsPolicy) {
				pol.Finalizers = []string{awsv1alpha1.FinalizerName}
				pol.Generation = 2
				pol.Spec.Tags = map[string]string{"team": "platform"}
				pol.Status.PolicyID = testOrgPolicyID
				pol.Status.ObservedGeneration = 1
			})},
			fake: &fakeOrgPolicy{
				describePolicy: func(_ context.Context, _ *awsorgs.DescribePolicyInput) (*awsorgs.DescribePolicyOutput, error) {
					return &awsorgs.DescribePolicyOutput{Policy: &orgtypes.Policy{
						PolicySummary: &orgtypes.PolicySummary{Id: aws.String(testOrgPolicyID)},
					}}, nil
				},
				updatePolicy: func(_ context.Context, params *awsorgs.UpdatePolicyInput) (*awsorgs.UpdatePolicyOutput, error) {
					if aws.ToString(params.PolicyId) != testOrgPolicyID {
						return nil, fmt.Errorf("unexpected policy ID %q", aws.ToString(params.PolicyId))
					}
					return &awsorgs.UpdatePolicyOutput{}, nil
				},
				tagResource: func(_ context.Context, _ *awsorgs.TagResourceInput) (*awsorgs.TagResourceOutput, error) {
					return &awsorgs.TagResourceOutput{}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeOrgPolicy, _ ctrl.Result) {
				if !f.updateCalled {
					t.Error("expected UpdatePolicy to be called")
				}
				if !f.tagCalled {
					t.Error("expected TagResource to be called")
				}
				got := &awsv1alpha1.OrganizationsPolicy{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.ObservedGeneration != 2 {
					t.Errorf("observedGeneration = %d, want 2", got.Status.ObservedGeneration)
				}
			},
		},
		{
			name: "recreates when policy disappeared from AWS",
			objs: []client.Object{orgPolicyCR(func(pol *awsv1alpha1.OrganizationsPolicy) {
				pol.Finalizers = []string{awsv1alpha1.FinalizerName}
				pol.Status.PolicyID = testOrgPolicyID
			})},
			fake: &fakeOrgPolicy{
				describePolicy: func(_ context.Context, _ *awsorgs.DescribePolicyInput) (*awsorgs.DescribePolicyOutput, error) {
					return nil, orgPolicyNotFoundErr()
				},
				createPolicy: func(_ context.Context, _ *awsorgs.CreatePolicyInput) (*awsorgs.CreatePolicyOutput, error) {
					return &awsorgs.CreatePolicyOutput{
						Policy: &orgtypes.Policy{
							PolicySummary: &orgtypes.PolicySummary{
								Id:  aws.String(testOrgPolicyID),
								Arn: aws.String(testOrgPolicyARN),
							},
						},
					}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeOrgPolicy, _ ctrl.Result) {
				if !f.createCalled {
					t.Error("expected CreatePolicy to be called when AWS resource vanished")
				}
			},
		},
		{
			name: "create failure surfaces error and Ready=False",
			objs: []client.Object{orgPolicyCR()},
			fake: &fakeOrgPolicy{
				createPolicy: func(_ context.Context, _ *awsorgs.CreatePolicyInput) (*awsorgs.CreatePolicyOutput, error) {
					return nil, fmt.Errorf("access denied")
				},
			},
			wantErr: true,
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeOrgPolicy, _ ctrl.Result) {
				got := &awsv1alpha1.OrganizationsPolicy{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionFalse {
					t.Errorf("Ready condition = %+v, want False", cond)
				}
				if got.Status.PolicyID != "" {
					t.Errorf("status.policyId = %q, want empty (nothing was created)", got.Status.PolicyID)
				}
			},
		},
		{
			name: "delete with finalizer calls DeletePolicy with stored ID",
			objs: []client.Object{orgPolicyCR(func(pol *awsv1alpha1.OrganizationsPolicy) {
				pol.Finalizers = []string{awsv1alpha1.FinalizerName}
				pol.Status.PolicyID = testOrgPolicyID
			})},
			fake: &fakeOrgPolicy{
				deletePolicy: func(_ context.Context, _ *awsorgs.DeletePolicyInput) (*awsorgs.DeletePolicyOutput, error) {
					return &awsorgs.DeletePolicyOutput{}, nil
				},
			},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, orgPolicyCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeOrgPolicy, _ ctrl.Result) {
				if !f.deleteCalled {
					t.Error("expected DeletePolicy to be called")
				}
				if f.deletedID != testOrgPolicyID {
					t.Errorf("DeletePolicy ID = %q, want %q", f.deletedID, testOrgPolicyID)
				}
				got := &awsv1alpha1.OrganizationsPolicy{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "delete with empty status ID skips AWS delete",
			objs: []client.Object{orgPolicyCR(func(pol *awsv1alpha1.OrganizationsPolicy) {
				pol.Finalizers = []string{awsv1alpha1.FinalizerName}
			})},
			fake: &fakeOrgPolicy{},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, orgPolicyCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeOrgPolicy, _ ctrl.Result) {
				if f.deleteCalled {
					t.Error("DeletePolicy must not be called when no ID was stored")
				}
				got := &awsv1alpha1.OrganizationsPolicy{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "abandon policy annotation skips AWS delete",
			objs: []client.Object{orgPolicyCR(func(pol *awsv1alpha1.OrganizationsPolicy) {
				pol.Finalizers = []string{awsv1alpha1.FinalizerName}
				pol.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
				pol.Status.PolicyID = testOrgPolicyID
			})},
			fake: &fakeOrgPolicy{},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, orgPolicyCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeOrgPolicy, _ ctrl.Result) {
				if f.deleteCalled {
					t.Error("DeletePolicy must not be called when abandoning")
				}
				got := &awsv1alpha1.OrganizationsPolicy{}
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
				WithStatusSubresource(&awsv1alpha1.OrganizationsPolicy{}).
				WithObjects(tc.objs...).
				Build()
			r := &OrganizationsPolicyReconciler{Client: c, Scheme: scheme, OrganizationsClient: tc.fake}

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
