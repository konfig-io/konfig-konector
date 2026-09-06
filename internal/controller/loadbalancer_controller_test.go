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
	awselbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

// fakeLBAPI implements LoadBalancerAWSAPI with overridable function fields.
type fakeLBAPI struct {
	createCalls           int
	deleteCalls           int
	setSecurityGroupCalls int
	setSubnetsCalls       int
	modifyAttrsCalls      int

	describeLoadBalancers func(*awselbv2.DescribeLoadBalancersInput) (*awselbv2.DescribeLoadBalancersOutput, error)
	createLoadBalancer    func(*awselbv2.CreateLoadBalancerInput) (*awselbv2.CreateLoadBalancerOutput, error)
	deleteLoadBalancer    func(*awselbv2.DeleteLoadBalancerInput) (*awselbv2.DeleteLoadBalancerOutput, error)
	setSecurityGroups     func(*awselbv2.SetSecurityGroupsInput) (*awselbv2.SetSecurityGroupsOutput, error)
}

func (f *fakeLBAPI) DescribeLoadBalancers(_ context.Context, in *awselbv2.DescribeLoadBalancersInput, _ ...func(*awselbv2.Options)) (*awselbv2.DescribeLoadBalancersOutput, error) {
	if f.describeLoadBalancers == nil {
		return &awselbv2.DescribeLoadBalancersOutput{}, nil
	}
	return f.describeLoadBalancers(in)
}

func (f *fakeLBAPI) CreateLoadBalancer(_ context.Context, in *awselbv2.CreateLoadBalancerInput, _ ...func(*awselbv2.Options)) (*awselbv2.CreateLoadBalancerOutput, error) {
	f.createCalls++
	if f.createLoadBalancer == nil {
		return &awselbv2.CreateLoadBalancerOutput{}, nil
	}
	return f.createLoadBalancer(in)
}

func (f *fakeLBAPI) DeleteLoadBalancer(_ context.Context, in *awselbv2.DeleteLoadBalancerInput, _ ...func(*awselbv2.Options)) (*awselbv2.DeleteLoadBalancerOutput, error) {
	f.deleteCalls++
	if f.deleteLoadBalancer == nil {
		return &awselbv2.DeleteLoadBalancerOutput{}, nil
	}
	return f.deleteLoadBalancer(in)
}

func (f *fakeLBAPI) SetSecurityGroups(_ context.Context, in *awselbv2.SetSecurityGroupsInput, _ ...func(*awselbv2.Options)) (*awselbv2.SetSecurityGroupsOutput, error) {
	f.setSecurityGroupCalls++
	if f.setSecurityGroups == nil {
		return &awselbv2.SetSecurityGroupsOutput{}, nil
	}
	return f.setSecurityGroups(in)
}

func (f *fakeLBAPI) SetSubnets(_ context.Context, _ *awselbv2.SetSubnetsInput, _ ...func(*awselbv2.Options)) (*awselbv2.SetSubnetsOutput, error) {
	f.setSubnetsCalls++
	return &awselbv2.SetSubnetsOutput{}, nil
}

func (f *fakeLBAPI) ModifyLoadBalancerAttributes(_ context.Context, _ *awselbv2.ModifyLoadBalancerAttributesInput, _ ...func(*awselbv2.Options)) (*awselbv2.ModifyLoadBalancerAttributesOutput, error) {
	f.modifyAttrsCalls++
	return &awselbv2.ModifyLoadBalancerAttributesOutput{}, nil
}

const lbTestARN = "arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/web-alb/50dc6c495c0c9188"

// lbDescribeExisting serves an active LB with the given SGs and subnets.
func lbDescribeExisting(sgs, subnets []string) func(*awselbv2.DescribeLoadBalancersInput) (*awselbv2.DescribeLoadBalancersOutput, error) {
	return func(*awselbv2.DescribeLoadBalancersInput) (*awselbv2.DescribeLoadBalancersOutput, error) {
		azs := make([]elbv2types.AvailabilityZone, 0, len(subnets))
		for _, sn := range subnets {
			sn := sn
			azs = append(azs, elbv2types.AvailabilityZone{SubnetId: &sn})
		}
		return &awselbv2.DescribeLoadBalancersOutput{
			LoadBalancers: []elbv2types.LoadBalancer{{
				LoadBalancerArn:   aws.String(lbTestARN),
				DNSName:           aws.String("web-alb-123.us-east-1.elb.amazonaws.com"),
				State:             &elbv2types.LoadBalancerState{Code: elbv2types.LoadBalancerStateEnumActive},
				SecurityGroups:    sgs,
				AvailabilityZones: azs,
			}},
		}, nil
	}
}

func lbTestClient(s *runtime.Scheme, objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(&awsv1alpha1.LoadBalancer{}).
		Build()
}

func TestLoadBalancerReconcile(t *testing.T) {
	ctx := context.Background()
	nn := k8stypes.NamespacedName{Name: "test-lb", Namespace: "default"}
	req := ctrl.Request{NamespacedName: nn}

	newLB := func(mutate ...func(*awsv1alpha1.LoadBalancer)) *awsv1alpha1.LoadBalancer {
		lb := &awsv1alpha1.LoadBalancer{
			ObjectMeta: metav1.ObjectMeta{Name: nn.Name, Namespace: nn.Namespace, Finalizers: []string{awsv1alpha1.FinalizerName}, Generation: 1},
			Spec: awsv1alpha1.LoadBalancerSpec{
				Name:   "web-alb",
				Type:   "application",
				Scheme: "internet-facing",
				SubnetRefs: []awsv1alpha1.SubnetRef{
					{ID: "subnet-0aaa"},
					{ID: "subnet-0bbb"},
				},
				SecurityGroupRefs: []awsv1alpha1.SecurityGroupRef{{ID: "sg-0123"}},
			},
		}
		for _, m := range mutate {
			m(lb)
		}
		return lb
	}

	t.Run("create happy path persists ARN and Ready", func(t *testing.T) {
		s := vpcTestScheme(t)
		cl := lbTestClient(s, newLB())
		f := &fakeLBAPI{
			createLoadBalancer: func(in *awselbv2.CreateLoadBalancerInput) (*awselbv2.CreateLoadBalancerOutput, error) {
				if aws.ToString(in.Name) != "web-alb" {
					t.Errorf("CreateLoadBalancer name = %q, want web-alb", aws.ToString(in.Name))
				}
				if len(in.Subnets) != 2 {
					t.Errorf("CreateLoadBalancer subnets = %v, want 2 entries", in.Subnets)
				}
				return &awselbv2.CreateLoadBalancerOutput{
					LoadBalancers: []elbv2types.LoadBalancer{{
						LoadBalancerArn: aws.String(lbTestARN),
						DNSName:         aws.String("web-alb-123.us-east-1.elb.amazonaws.com"),
						State:           &elbv2types.LoadBalancerState{Code: elbv2types.LoadBalancerStateEnumProvisioning},
					}},
				}, nil
			},
		}
		r := &LoadBalancerReconciler{Client: cl, Scheme: s, ELBv2Client: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		got := &awsv1alpha1.LoadBalancer{}
		if err := cl.Get(ctx, nn, got); err != nil {
			t.Fatalf("Get: %v", err)
		}
		if f.createCalls != 1 {
			t.Errorf("CreateLoadBalancer calls = %d, want 1", f.createCalls)
		}
		if got.Status.ARN != lbTestARN {
			t.Errorf("status.arn = %q, want %q", got.Status.ARN, lbTestARN)
		}
		if got.Status.DNSName == "" {
			t.Error("status.dnsName empty, want populated")
		}
		cond := meta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
		if cond == nil || cond.Status != metav1.ConditionTrue {
			t.Errorf("Ready condition = %+v, want True", cond)
		}
	})

	t.Run("steady state does not call create or SetSecurityGroups", func(t *testing.T) {
		s := vpcTestScheme(t)
		existing := newLB(func(lb *awsv1alpha1.LoadBalancer) {
			lb.Finalizers = []string{awsv1alpha1.FinalizerName}
			lb.Generation = 2
			lb.Status.ARN = lbTestARN
			lb.Status.ObservedGeneration = 2 // no spec change since last sync
		})
		cl := lbTestClient(s, existing)
		f := &fakeLBAPI{
			describeLoadBalancers: lbDescribeExisting([]string{"sg-0123"}, []string{"subnet-0aaa", "subnet-0bbb"}),
		}
		r := &LoadBalancerReconciler{Client: cl, Scheme: s, ELBv2Client: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if f.createCalls != 0 {
			t.Errorf("CreateLoadBalancer calls = %d, want 0", f.createCalls)
		}
		if f.setSecurityGroupCalls != 0 {
			t.Errorf("SetSecurityGroups calls = %d, want 0 on steady state", f.setSecurityGroupCalls)
		}
		if f.setSubnetsCalls != 0 {
			t.Errorf("SetSubnets calls = %d, want 0 on steady state", f.setSubnetsCalls)
		}
	})

	t.Run("SG spec change with generation bump calls SetSecurityGroups", func(t *testing.T) {
		s := vpcTestScheme(t)
		existing := newLB(func(lb *awsv1alpha1.LoadBalancer) {
			lb.Finalizers = []string{awsv1alpha1.FinalizerName}
			lb.Generation = 3
			lb.Spec.SecurityGroupRefs = []awsv1alpha1.SecurityGroupRef{{ID: "sg-0999"}} // changed
			lb.Status.ARN = lbTestARN
			lb.Status.ObservedGeneration = 2 // generation bumped
		})
		cl := lbTestClient(s, existing)
		var gotSGs []string
		f := &fakeLBAPI{
			describeLoadBalancers: lbDescribeExisting([]string{"sg-0123"}, []string{"subnet-0aaa", "subnet-0bbb"}),
			setSecurityGroups: func(in *awselbv2.SetSecurityGroupsInput) (*awselbv2.SetSecurityGroupsOutput, error) {
				gotSGs = in.SecurityGroups
				return &awselbv2.SetSecurityGroupsOutput{}, nil
			},
		}
		r := &LoadBalancerReconciler{Client: cl, Scheme: s, ELBv2Client: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if f.setSecurityGroupCalls != 1 {
			t.Fatalf("SetSecurityGroups calls = %d, want 1", f.setSecurityGroupCalls)
		}
		if len(gotSGs) != 1 || gotSGs[0] != "sg-0999" {
			t.Errorf("SetSecurityGroups sgs = %v, want [sg-0999]", gotSGs)
		}
		if f.setSubnetsCalls != 0 {
			t.Errorf("SetSubnets calls = %d, want 0 (subnets unchanged)", f.setSubnetsCalls)
		}
		if f.modifyAttrsCalls != 1 {
			t.Errorf("ModifyLoadBalancerAttributes calls = %d, want 1 on update path", f.modifyAttrsCalls)
		}
	})

	t.Run("delete with finalizer calls DeleteLoadBalancer with status ARN", func(t *testing.T) {
		s := vpcTestScheme(t)
		existing := newLB(func(lb *awsv1alpha1.LoadBalancer) {
			lb.Finalizers = []string{awsv1alpha1.FinalizerName}
			lb.Status.ARN = lbTestARN
		})
		cl := lbTestClient(s, existing)
		var deletedARN string
		f := &fakeLBAPI{
			deleteLoadBalancer: func(in *awselbv2.DeleteLoadBalancerInput) (*awselbv2.DeleteLoadBalancerOutput, error) {
				deletedARN = aws.ToString(in.LoadBalancerArn)
				return &awselbv2.DeleteLoadBalancerOutput{}, nil
			},
		}
		r := &LoadBalancerReconciler{Client: cl, Scheme: s, ELBv2Client: f}

		if err := cl.Delete(ctx, existing); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if deletedARN != lbTestARN {
			t.Errorf("DeleteLoadBalancer arn = %q, want %q", deletedARN, lbTestARN)
		}
		if err := cl.Get(ctx, nn, &awsv1alpha1.LoadBalancer{}); !apierrors.IsNotFound(err) {
			t.Errorf("Get after finalize = %v, want NotFound", err)
		}
	})

	t.Run("abandon annotation skips AWS delete and removes finalizer", func(t *testing.T) {
		s := vpcTestScheme(t)
		existing := newLB(func(lb *awsv1alpha1.LoadBalancer) {
			lb.Finalizers = []string{awsv1alpha1.FinalizerName}
			lb.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			lb.Status.ARN = lbTestARN
		})
		cl := lbTestClient(s, existing)
		f := &fakeLBAPI{}
		r := &LoadBalancerReconciler{Client: cl, Scheme: s, ELBv2Client: f}

		if err := cl.Delete(ctx, existing); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if f.deleteCalls != 0 {
			t.Errorf("DeleteLoadBalancer calls = %d, want 0", f.deleteCalls)
		}
		if err := cl.Get(ctx, nn, &awsv1alpha1.LoadBalancer{}); !apierrors.IsNotFound(err) {
			t.Errorf("Get after finalize = %v, want NotFound", err)
		}
	})

	t.Run("delete fallback looks up ARN by spec name when status ARN lost", func(t *testing.T) {
		s := vpcTestScheme(t)
		existing := newLB(func(lb *awsv1alpha1.LoadBalancer) {
			lb.Finalizers = []string{awsv1alpha1.FinalizerName}
		})
		cl := lbTestClient(s, existing)
		var describedNames []string
		var deletedARN string
		f := &fakeLBAPI{
			describeLoadBalancers: func(in *awselbv2.DescribeLoadBalancersInput) (*awselbv2.DescribeLoadBalancersOutput, error) {
				describedNames = in.Names
				return &awselbv2.DescribeLoadBalancersOutput{
					LoadBalancers: []elbv2types.LoadBalancer{{
						LoadBalancerArn: aws.String(lbTestARN),
					}},
				}, nil
			},
			deleteLoadBalancer: func(in *awselbv2.DeleteLoadBalancerInput) (*awselbv2.DeleteLoadBalancerOutput, error) {
				deletedARN = aws.ToString(in.LoadBalancerArn)
				return &awselbv2.DeleteLoadBalancerOutput{}, nil
			},
		}
		r := &LoadBalancerReconciler{Client: cl, Scheme: s, ELBv2Client: f}

		if err := cl.Delete(ctx, existing); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if len(describedNames) != 1 || describedNames[0] != "web-alb" {
			t.Errorf("DescribeLoadBalancers names = %v, want [web-alb]", describedNames)
		}
		if deletedARN != lbTestARN {
			t.Errorf("DeleteLoadBalancer arn = %q, want %q (from name lookup)", deletedARN, lbTestARN)
		}
	})
}
