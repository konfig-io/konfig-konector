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
	awsnfw "github.com/aws/aws-sdk-go-v2/service/networkfirewall"
	nfwtypes "github.com/aws/aws-sdk-go-v2/service/networkfirewall/types"
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

func nfwNotFoundErr() error {
	return &smithy.GenericAPIError{Code: "ResourceNotFoundException", Message: "not found"}
}

func newNetsecScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return scheme
}

func newNetsecFakeClient(scheme *runtime.Scheme, objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&awsv1alpha1.Firewall{},
			&awsv1alpha1.FirewallPolicy{},
			&awsv1alpha1.FirewallRuleGroup{},
			&awsv1alpha1.LatticeService{},
			&awsv1alpha1.LatticeServiceNetwork{},
			&awsv1alpha1.LatticeServiceNetworkVpcAssociation{},
			&awsv1alpha1.LatticeServiceNetworkServiceAssociation{},
			&awsv1alpha1.LatticeTargetGroup{},
			&awsv1alpha1.LatticeListener{},
			&awsv1alpha1.CustomerGateway{},
			&awsv1alpha1.VPNGateway{},
			&awsv1alpha1.VPNConnection{},
			&awsv1alpha1.VPNConnectionRoute{},
			&awsv1alpha1.ManagedPrefixList{},
			&awsv1alpha1.CapacityReservation{},
			&awsv1alpha1.VPC{},
			&awsv1alpha1.Subnet{},
			&awsv1alpha1.SecurityGroup{},
			&awsv1alpha1.TransitGateway{},
		).
		WithObjects(objs...).
		Build()
}

// fakeNFW implements the Network Firewall controller interfaces.
type fakeNFW struct {
	describeFirewall       func(*awsnfw.DescribeFirewallInput) (*awsnfw.DescribeFirewallOutput, error)
	createFirewall         func(*awsnfw.CreateFirewallInput) (*awsnfw.CreateFirewallOutput, error)
	deleteFirewall         func(*awsnfw.DeleteFirewallInput) (*awsnfw.DeleteFirewallOutput, error)
	updateDeleteProtection func(*awsnfw.UpdateFirewallDeleteProtectionInput) (*awsnfw.UpdateFirewallDeleteProtectionOutput, error)
	tagResource            func(*awsnfw.TagResourceInput) (*awsnfw.TagResourceOutput, error)

	describeRuleGroup func(*awsnfw.DescribeRuleGroupInput) (*awsnfw.DescribeRuleGroupOutput, error)
	createRuleGroup   func(*awsnfw.CreateRuleGroupInput) (*awsnfw.CreateRuleGroupOutput, error)
	updateRuleGroup   func(*awsnfw.UpdateRuleGroupInput) (*awsnfw.UpdateRuleGroupOutput, error)
	deleteRuleGroup   func(*awsnfw.DeleteRuleGroupInput) (*awsnfw.DeleteRuleGroupOutput, error)

	describePolicy func(*awsnfw.DescribeFirewallPolicyInput) (*awsnfw.DescribeFirewallPolicyOutput, error)
	createPolicy   func(*awsnfw.CreateFirewallPolicyInput) (*awsnfw.CreateFirewallPolicyOutput, error)
	updatePolicy   func(*awsnfw.UpdateFirewallPolicyInput) (*awsnfw.UpdateFirewallPolicyOutput, error)
	deletePolicy   func(*awsnfw.DeleteFirewallPolicyInput) (*awsnfw.DeleteFirewallPolicyOutput, error)

	createFirewallCalled bool
	deleteFirewallCalled bool
	createFirewallInput  *awsnfw.CreateFirewallInput
	deleteFirewallInput  *awsnfw.DeleteFirewallInput

	createRuleGroupCalled bool
	deleteRuleGroupCalled bool
	createPolicyCalled    bool
	deletePolicyCalled    bool
}

func (f *fakeNFW) DescribeFirewall(_ context.Context, p *awsnfw.DescribeFirewallInput, _ ...func(*awsnfw.Options)) (*awsnfw.DescribeFirewallOutput, error) {
	if f.describeFirewall == nil {
		return nil, fmt.Errorf("unexpected call to DescribeFirewall")
	}
	return f.describeFirewall(p)
}

func (f *fakeNFW) CreateFirewall(_ context.Context, p *awsnfw.CreateFirewallInput, _ ...func(*awsnfw.Options)) (*awsnfw.CreateFirewallOutput, error) {
	f.createFirewallCalled = true
	f.createFirewallInput = p
	if f.createFirewall == nil {
		return nil, fmt.Errorf("unexpected call to CreateFirewall")
	}
	return f.createFirewall(p)
}

func (f *fakeNFW) DeleteFirewall(_ context.Context, p *awsnfw.DeleteFirewallInput, _ ...func(*awsnfw.Options)) (*awsnfw.DeleteFirewallOutput, error) {
	f.deleteFirewallCalled = true
	f.deleteFirewallInput = p
	if f.deleteFirewall == nil {
		return nil, fmt.Errorf("unexpected call to DeleteFirewall")
	}
	return f.deleteFirewall(p)
}

func (f *fakeNFW) UpdateFirewallDeleteProtection(_ context.Context, p *awsnfw.UpdateFirewallDeleteProtectionInput, _ ...func(*awsnfw.Options)) (*awsnfw.UpdateFirewallDeleteProtectionOutput, error) {
	if f.updateDeleteProtection == nil {
		return nil, fmt.Errorf("unexpected call to UpdateFirewallDeleteProtection")
	}
	return f.updateDeleteProtection(p)
}

func (f *fakeNFW) TagResource(_ context.Context, p *awsnfw.TagResourceInput, _ ...func(*awsnfw.Options)) (*awsnfw.TagResourceOutput, error) {
	if f.tagResource == nil {
		return nil, fmt.Errorf("unexpected call to TagResource")
	}
	return f.tagResource(p)
}

func (f *fakeNFW) DescribeRuleGroup(_ context.Context, p *awsnfw.DescribeRuleGroupInput, _ ...func(*awsnfw.Options)) (*awsnfw.DescribeRuleGroupOutput, error) {
	if f.describeRuleGroup == nil {
		return nil, fmt.Errorf("unexpected call to DescribeRuleGroup")
	}
	return f.describeRuleGroup(p)
}

func (f *fakeNFW) CreateRuleGroup(_ context.Context, p *awsnfw.CreateRuleGroupInput, _ ...func(*awsnfw.Options)) (*awsnfw.CreateRuleGroupOutput, error) {
	f.createRuleGroupCalled = true
	if f.createRuleGroup == nil {
		return nil, fmt.Errorf("unexpected call to CreateRuleGroup")
	}
	return f.createRuleGroup(p)
}

func (f *fakeNFW) UpdateRuleGroup(_ context.Context, p *awsnfw.UpdateRuleGroupInput, _ ...func(*awsnfw.Options)) (*awsnfw.UpdateRuleGroupOutput, error) {
	if f.updateRuleGroup == nil {
		return nil, fmt.Errorf("unexpected call to UpdateRuleGroup")
	}
	return f.updateRuleGroup(p)
}

func (f *fakeNFW) DeleteRuleGroup(_ context.Context, p *awsnfw.DeleteRuleGroupInput, _ ...func(*awsnfw.Options)) (*awsnfw.DeleteRuleGroupOutput, error) {
	f.deleteRuleGroupCalled = true
	if f.deleteRuleGroup == nil {
		return nil, fmt.Errorf("unexpected call to DeleteRuleGroup")
	}
	return f.deleteRuleGroup(p)
}

func (f *fakeNFW) DescribeFirewallPolicy(_ context.Context, p *awsnfw.DescribeFirewallPolicyInput, _ ...func(*awsnfw.Options)) (*awsnfw.DescribeFirewallPolicyOutput, error) {
	if f.describePolicy == nil {
		return nil, fmt.Errorf("unexpected call to DescribeFirewallPolicy")
	}
	return f.describePolicy(p)
}

func (f *fakeNFW) CreateFirewallPolicy(_ context.Context, p *awsnfw.CreateFirewallPolicyInput, _ ...func(*awsnfw.Options)) (*awsnfw.CreateFirewallPolicyOutput, error) {
	f.createPolicyCalled = true
	if f.createPolicy == nil {
		return nil, fmt.Errorf("unexpected call to CreateFirewallPolicy")
	}
	return f.createPolicy(p)
}

func (f *fakeNFW) UpdateFirewallPolicy(_ context.Context, p *awsnfw.UpdateFirewallPolicyInput, _ ...func(*awsnfw.Options)) (*awsnfw.UpdateFirewallPolicyOutput, error) {
	if f.updatePolicy == nil {
		return nil, fmt.Errorf("unexpected call to UpdateFirewallPolicy")
	}
	return f.updatePolicy(p)
}

func (f *fakeNFW) DeleteFirewallPolicy(_ context.Context, p *awsnfw.DeleteFirewallPolicyInput, _ ...func(*awsnfw.Options)) (*awsnfw.DeleteFirewallPolicyOutput, error) {
	f.deletePolicyCalled = true
	if f.deletePolicy == nil {
		return nil, fmt.Errorf("unexpected call to DeleteFirewallPolicy")
	}
	return f.deletePolicy(p)
}

const (
	testFirewallARN  = "arn:aws:network-firewall:us-east-1:123456789012:firewall/my-firewall"
	testFWPolicyARN  = "arn:aws:network-firewall:us-east-1:123456789012:firewall-policy/my-policy"
	testRuleGroupARN = "arn:aws:network-firewall:us-east-1:123456789012:stateful-rulegroup/my-rg"
)

func firewallCR(mutate ...func(*awsv1alpha1.Firewall)) *awsv1alpha1.Firewall {
	fw := &awsv1alpha1.Firewall{
		ObjectMeta: metav1.ObjectMeta{Name: "my-firewall", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec: awsv1alpha1.FirewallSpec{
			Name:              "my-firewall",
			FirewallPolicyRef: awsv1alpha1.FirewallPolicyRef{ARN: testFWPolicyARN},
			VPCRef:            awsv1alpha1.VPCResourceRef{ID: "vpc-123"},
			SubnetRefs:        []awsv1alpha1.SubnetRef{{ID: "subnet-123"}},
			Tags:              map[string]string{"env": "test"},
		},
	}
	for _, m := range mutate {
		m(fw)
	}
	return fw
}

func TestFirewallReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-firewall", Namespace: "default"}}

	readyFirewallDescribe := func(_ *awsnfw.DescribeFirewallInput) (*awsnfw.DescribeFirewallOutput, error) {
		return &awsnfw.DescribeFirewallOutput{
			Firewall: &nfwtypes.Firewall{
				FirewallArn:       aws.String(testFirewallARN),
				FirewallId:        aws.String("fw-id-1"),
				FirewallPolicyArn: aws.String(testFWPolicyARN),
				VpcId:             aws.String("vpc-123"),
			},
			FirewallStatus: &nfwtypes.FirewallStatus{Status: nfwtypes.FirewallStatusValueReady},
			UpdateToken:    aws.String("tok-1"),
		}, nil
	}

	tests := []struct {
		name    string
		objs    []client.Object
		fake    *fakeNFW
		setup   func(t *testing.T, ctx context.Context, c client.Client)
		wantErr bool
		assert  func(t *testing.T, ctx context.Context, c client.Client, f *fakeNFW, res ctrl.Result)
	}{
		{
			name: "create persists identifier and polls",
			objs: []client.Object{firewallCR()},
			fake: &fakeNFW{
				describeFirewall: func(_ *awsnfw.DescribeFirewallInput) (*awsnfw.DescribeFirewallOutput, error) {
					return nil, nfwNotFoundErr()
				},
				createFirewall: func(p *awsnfw.CreateFirewallInput) (*awsnfw.CreateFirewallOutput, error) {
					if aws.ToString(p.FirewallName) != "my-firewall" {
						return nil, fmt.Errorf("unexpected firewall name %q", aws.ToString(p.FirewallName))
					}
					if aws.ToString(p.VpcId) != "vpc-123" {
						return nil, fmt.Errorf("unexpected vpc %q", aws.ToString(p.VpcId))
					}
					return &awsnfw.CreateFirewallOutput{
						Firewall: &nfwtypes.Firewall{
							FirewallArn: aws.String(testFirewallARN),
							FirewallId:  aws.String("fw-id-1"),
						},
						FirewallStatus: &nfwtypes.FirewallStatus{Status: nfwtypes.FirewallStatusValueProvisioning},
					}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeNFW, res ctrl.Result) {
				if !f.createFirewallCalled {
					t.Error("expected CreateFirewall to be called")
				}
				if res != requeueNetworkFirewallPolling {
					t.Errorf("result = %+v, want polling requeue", res)
				}
				got := &awsv1alpha1.Firewall{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.ARN != testFirewallARN {
					t.Errorf("status.arn = %q, want %q", got.Status.ARN, testFirewallARN)
				}
				if got.Status.State != "PROVISIONING" {
					t.Errorf("status.state = %q, want PROVISIONING", got.Status.State)
				}
			},
		},
		{
			name: "provisioning firewall keeps polling until READY",
			objs: []client.Object{firewallCR(func(fw *awsv1alpha1.Firewall) {
				fw.Finalizers = []string{awsv1alpha1.FinalizerName}
				fw.Status.ARN = testFirewallARN
			})},
			fake: &fakeNFW{
				describeFirewall: func(_ *awsnfw.DescribeFirewallInput) (*awsnfw.DescribeFirewallOutput, error) {
					return &awsnfw.DescribeFirewallOutput{
						Firewall: &nfwtypes.Firewall{
							FirewallArn: aws.String(testFirewallARN),
							FirewallId:  aws.String("fw-id-1"),
						},
						FirewallStatus: &nfwtypes.FirewallStatus{Status: nfwtypes.FirewallStatusValueProvisioning},
					}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeNFW, res ctrl.Result) {
				if f.createFirewallCalled {
					t.Error("CreateFirewall must not be called while provisioning")
				}
				if res != requeueNetworkFirewallPolling {
					t.Errorf("result = %+v, want polling requeue", res)
				}
				got := &awsv1alpha1.Firewall{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionFalse {
					t.Errorf("Ready condition = %+v, want False while provisioning", cond)
				}
			},
		},
		{
			name: "steady state READY does not create and updates delete protection",
			objs: []client.Object{firewallCR(func(fw *awsv1alpha1.Firewall) {
				fw.Finalizers = []string{awsv1alpha1.FinalizerName}
				fw.Generation = 2
				fw.Spec.DeleteProtection = true
				fw.Status.ARN = testFirewallARN
				fw.Status.ObservedGeneration = 1
			})},
			fake: &fakeNFW{
				describeFirewall: readyFirewallDescribe,
				updateDeleteProtection: func(p *awsnfw.UpdateFirewallDeleteProtectionInput) (*awsnfw.UpdateFirewallDeleteProtectionOutput, error) {
					if !p.DeleteProtection {
						return nil, fmt.Errorf("expected delete protection true")
					}
					return &awsnfw.UpdateFirewallDeleteProtectionOutput{}, nil
				},
				tagResource: func(_ *awsnfw.TagResourceInput) (*awsnfw.TagResourceOutput, error) {
					return &awsnfw.TagResourceOutput{}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeNFW, _ ctrl.Result) {
				if f.createFirewallCalled {
					t.Error("CreateFirewall must not be called in steady state")
				}
				got := &awsv1alpha1.Firewall{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionTrue {
					t.Errorf("Ready condition = %+v, want True", cond)
				}
				if got.Status.ObservedGeneration != 2 {
					t.Errorf("observedGeneration = %d, want 2", got.Status.ObservedGeneration)
				}
			},
		},
		{
			name: "delete with finalizer calls AWS delete with status ARN",
			objs: []client.Object{firewallCR(func(fw *awsv1alpha1.Firewall) {
				fw.Finalizers = []string{awsv1alpha1.FinalizerName}
				fw.Status.ARN = testFirewallARN
			})},
			fake: &fakeNFW{
				deleteFirewall: func(_ *awsnfw.DeleteFirewallInput) (*awsnfw.DeleteFirewallOutput, error) {
					return &awsnfw.DeleteFirewallOutput{}, nil
				},
			},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, firewallCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeNFW, _ ctrl.Result) {
				if !f.deleteFirewallCalled {
					t.Error("expected DeleteFirewall to be called")
				}
				if aws.ToString(f.deleteFirewallInput.FirewallArn) != testFirewallARN {
					t.Errorf("DeleteFirewall arn = %q, want %q", aws.ToString(f.deleteFirewallInput.FirewallArn), testFirewallARN)
				}
				got := &awsv1alpha1.Firewall{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "delete fallback uses spec name when status empty",
			objs: []client.Object{firewallCR(func(fw *awsv1alpha1.Firewall) {
				fw.Finalizers = []string{awsv1alpha1.FinalizerName}
			})},
			fake: &fakeNFW{
				deleteFirewall: func(_ *awsnfw.DeleteFirewallInput) (*awsnfw.DeleteFirewallOutput, error) {
					return &awsnfw.DeleteFirewallOutput{}, nil
				},
			},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, firewallCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeNFW, _ ctrl.Result) {
				if !f.deleteFirewallCalled {
					t.Error("expected DeleteFirewall to be called via name fallback")
				}
				if aws.ToString(f.deleteFirewallInput.FirewallName) != "my-firewall" {
					t.Errorf("DeleteFirewall name = %q, want my-firewall", aws.ToString(f.deleteFirewallInput.FirewallName))
				}
			},
		},
		{
			name: "abandon policy skips AWS delete",
			objs: []client.Object{firewallCR(func(fw *awsv1alpha1.Firewall) {
				fw.Finalizers = []string{awsv1alpha1.FinalizerName}
				fw.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
				fw.Status.ARN = testFirewallARN
			})},
			fake: &fakeNFW{},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, firewallCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeNFW, _ ctrl.Result) {
				if f.deleteFirewallCalled {
					t.Error("DeleteFirewall must not be called when abandoning")
				}
				got := &awsv1alpha1.Firewall{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "waits for FirewallPolicy dependency without ARN",
			objs: []client.Object{
				firewallCR(func(fw *awsv1alpha1.Firewall) {
					fw.Spec.FirewallPolicyRef = awsv1alpha1.FirewallPolicyRef{Name: "my-policy"}
				}),
				&awsv1alpha1.FirewallPolicy{
					ObjectMeta: metav1.ObjectMeta{Name: "my-policy", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
					Spec:       awsv1alpha1.FirewallPolicySpec{Name: "my-policy"},
				},
			},
			fake: &fakeNFW{
				describeFirewall: func(_ *awsnfw.DescribeFirewallInput) (*awsnfw.DescribeFirewallOutput, error) {
					return nil, nfwNotFoundErr()
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeNFW, res ctrl.Result) {
				if res != requeueDependency {
					t.Errorf("result = %+v, want requeueDependency %+v", res, requeueDependency)
				}
				if f.createFirewallCalled {
					t.Error("CreateFirewall must not be called while policy dependency is not ready")
				}
			},
		},
		{
			name: "resolves FirewallPolicy ref from CR status",
			objs: []client.Object{
				firewallCR(func(fw *awsv1alpha1.Firewall) {
					fw.Spec.FirewallPolicyRef = awsv1alpha1.FirewallPolicyRef{Name: "my-policy"}
				}),
				&awsv1alpha1.FirewallPolicy{
					ObjectMeta: metav1.ObjectMeta{Name: "my-policy", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
					Spec:       awsv1alpha1.FirewallPolicySpec{Name: "my-policy"},
					Status:     awsv1alpha1.FirewallPolicyStatus{ARN: testFWPolicyARN},
				},
			},
			fake: &fakeNFW{
				describeFirewall: func(_ *awsnfw.DescribeFirewallInput) (*awsnfw.DescribeFirewallOutput, error) {
					return nil, nfwNotFoundErr()
				},
				createFirewall: func(p *awsnfw.CreateFirewallInput) (*awsnfw.CreateFirewallOutput, error) {
					if aws.ToString(p.FirewallPolicyArn) != testFWPolicyARN {
						return nil, fmt.Errorf("unexpected policy arn %q", aws.ToString(p.FirewallPolicyArn))
					}
					return &awsnfw.CreateFirewallOutput{
						Firewall: &nfwtypes.Firewall{
							FirewallArn: aws.String(testFirewallARN),
							FirewallId:  aws.String("fw-id-1"),
						},
						FirewallStatus: &nfwtypes.FirewallStatus{Status: nfwtypes.FirewallStatusValueProvisioning},
					}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeNFW, _ ctrl.Result) {
				if !f.createFirewallCalled {
					t.Error("expected CreateFirewall to be called")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			scheme := newNetsecScheme(t)
			c := newNetsecFakeClient(scheme, tc.objs...)
			r := &FirewallReconciler{Client: c, Scheme: scheme, NetworkFirewallClient: tc.fake}

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

func TestFirewallRuleGroupReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-rg", Namespace: "default"}}
	rgCR := func(mutate ...func(*awsv1alpha1.FirewallRuleGroup)) *awsv1alpha1.FirewallRuleGroup {
		rg := &awsv1alpha1.FirewallRuleGroup{
			ObjectMeta: metav1.ObjectMeta{Name: "my-rg", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
			Spec: awsv1alpha1.FirewallRuleGroupSpec{
				Name:        "my-rg",
				Type:        "STATEFUL",
				Capacity:    100,
				RulesString: `pass tcp any any -> any any (sid:1;)`,
			},
		}
		for _, m := range mutate {
			m(rg)
		}
		return rg
	}

	t.Run("create persists identifier", func(t *testing.T) {
		ctx := context.Background()
		scheme := newNetsecScheme(t)
		c := newNetsecFakeClient(scheme, rgCR())
		f := &fakeNFW{
			describeRuleGroup: func(_ *awsnfw.DescribeRuleGroupInput) (*awsnfw.DescribeRuleGroupOutput, error) {
				return nil, nfwNotFoundErr()
			},
			createRuleGroup: func(p *awsnfw.CreateRuleGroupInput) (*awsnfw.CreateRuleGroupOutput, error) {
				if aws.ToString(p.Rules) == "" {
					return nil, fmt.Errorf("expected rules string")
				}
				return &awsnfw.CreateRuleGroupOutput{
					RuleGroupResponse: &nfwtypes.RuleGroupResponse{
						RuleGroupArn: aws.String(testRuleGroupARN),
						RuleGroupId:  aws.String("rg-id-1"),
					},
					UpdateToken: aws.String("tok-1"),
				}, nil
			},
		}
		r := &FirewallRuleGroupReconciler{Client: c, Scheme: scheme, NetworkFirewallClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.FirewallRuleGroup{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ARN != testRuleGroupARN {
			t.Errorf("status.arn = %q, want %q", got.Status.ARN, testRuleGroupARN)
		}
		if got.Status.UpdateToken != "tok-1" {
			t.Errorf("status.updateToken = %q, want tok-1", got.Status.UpdateToken)
		}
	})

	t.Run("delete with finalizer calls AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newNetsecScheme(t)
		obj := rgCR(func(rg *awsv1alpha1.FirewallRuleGroup) {
			rg.Finalizers = []string{awsv1alpha1.FinalizerName}
			rg.Status.ARN = testRuleGroupARN
		})
		c := newNetsecFakeClient(scheme, obj)
		f := &fakeNFW{
			deleteRuleGroup: func(p *awsnfw.DeleteRuleGroupInput) (*awsnfw.DeleteRuleGroupOutput, error) {
				if aws.ToString(p.RuleGroupArn) != testRuleGroupARN {
					return nil, fmt.Errorf("unexpected arn %q", aws.ToString(p.RuleGroupArn))
				}
				return &awsnfw.DeleteRuleGroupOutput{}, nil
			},
		}
		r := &FirewallRuleGroupReconciler{Client: c, Scheme: scheme, NetworkFirewallClient: f}
		if err := c.Delete(ctx, rgCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteRuleGroupCalled {
			t.Error("expected DeleteRuleGroup to be called")
		}
		got := &awsv1alpha1.FirewallRuleGroup{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected CR gone, got err=%v", err)
		}
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newNetsecScheme(t)
		obj := rgCR(func(rg *awsv1alpha1.FirewallRuleGroup) {
			rg.Finalizers = []string{awsv1alpha1.FinalizerName}
			rg.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			rg.Status.ARN = testRuleGroupARN
		})
		c := newNetsecFakeClient(scheme, obj)
		f := &fakeNFW{}
		r := &FirewallRuleGroupReconciler{Client: c, Scheme: scheme, NetworkFirewallClient: f}
		if err := c.Delete(ctx, rgCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteRuleGroupCalled {
			t.Error("DeleteRuleGroup must not be called when abandoning")
		}
	})
}

func TestFirewallPolicyReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-policy", Namespace: "default"}}
	polCR := func(mutate ...func(*awsv1alpha1.FirewallPolicy)) *awsv1alpha1.FirewallPolicy {
		p := &awsv1alpha1.FirewallPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "my-policy", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
			Spec: awsv1alpha1.FirewallPolicySpec{
				Name:                            "my-policy",
				StatelessDefaultActions:         []string{"aws:forward_to_sfe"},
				StatelessFragmentDefaultActions: []string{"aws:forward_to_sfe"},
				StatefulRuleGroupRefs:           []awsv1alpha1.StatefulRuleGroupRef{{ARN: testRuleGroupARN}},
			},
		}
		for _, m := range mutate {
			m(p)
		}
		return p
	}

	t.Run("create persists identifier with resolved rule group refs", func(t *testing.T) {
		ctx := context.Background()
		scheme := newNetsecScheme(t)
		c := newNetsecFakeClient(scheme, polCR())
		f := &fakeNFW{
			describePolicy: func(_ *awsnfw.DescribeFirewallPolicyInput) (*awsnfw.DescribeFirewallPolicyOutput, error) {
				return nil, nfwNotFoundErr()
			},
			createPolicy: func(p *awsnfw.CreateFirewallPolicyInput) (*awsnfw.CreateFirewallPolicyOutput, error) {
				if len(p.FirewallPolicy.StatefulRuleGroupReferences) != 1 ||
					aws.ToString(p.FirewallPolicy.StatefulRuleGroupReferences[0].ResourceArn) != testRuleGroupARN {
					return nil, fmt.Errorf("unexpected stateful rule group refs %+v", p.FirewallPolicy.StatefulRuleGroupReferences)
				}
				return &awsnfw.CreateFirewallPolicyOutput{
					FirewallPolicyResponse: &nfwtypes.FirewallPolicyResponse{
						FirewallPolicyArn: aws.String(testFWPolicyARN),
						FirewallPolicyId:  aws.String("pol-id-1"),
					},
					UpdateToken: aws.String("tok-1"),
				}, nil
			},
		}
		r := &FirewallPolicyReconciler{Client: c, Scheme: scheme, NetworkFirewallClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.FirewallPolicy{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ARN != testFWPolicyARN {
			t.Errorf("status.arn = %q, want %q", got.Status.ARN, testFWPolicyARN)
		}
	})

	t.Run("delete with finalizer calls AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newNetsecScheme(t)
		obj := polCR(func(p *awsv1alpha1.FirewallPolicy) {
			p.Finalizers = []string{awsv1alpha1.FinalizerName}
			p.Status.ARN = testFWPolicyARN
		})
		c := newNetsecFakeClient(scheme, obj)
		f := &fakeNFW{
			deletePolicy: func(_ *awsnfw.DeleteFirewallPolicyInput) (*awsnfw.DeleteFirewallPolicyOutput, error) {
				return &awsnfw.DeleteFirewallPolicyOutput{}, nil
			},
		}
		r := &FirewallPolicyReconciler{Client: c, Scheme: scheme, NetworkFirewallClient: f}
		if err := c.Delete(ctx, polCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deletePolicyCalled {
			t.Error("expected DeleteFirewallPolicy to be called")
		}
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newNetsecScheme(t)
		obj := polCR(func(p *awsv1alpha1.FirewallPolicy) {
			p.Finalizers = []string{awsv1alpha1.FinalizerName}
			p.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			p.Status.ARN = testFWPolicyARN
		})
		c := newNetsecFakeClient(scheme, obj)
		f := &fakeNFW{}
		r := &FirewallPolicyReconciler{Client: c, Scheme: scheme, NetworkFirewallClient: f}
		if err := c.Delete(ctx, polCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deletePolicyCalled {
			t.Error("DeleteFirewallPolicy must not be called when abandoning")
		}
	})
}
