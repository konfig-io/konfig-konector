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
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
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

// fakeSubnetAPI implements SubnetAWSAPI with overridable function fields.
type fakeSubnetAPI struct {
	createSubnetCalls int
	deleteSubnetCalls int

	createSubnet          func(*awsec2.CreateSubnetInput) (*awsec2.CreateSubnetOutput, error)
	deleteSubnet          func(*awsec2.DeleteSubnetInput) (*awsec2.DeleteSubnetOutput, error)
	describeSubnets       func(*awsec2.DescribeSubnetsInput) (*awsec2.DescribeSubnetsOutput, error)
	modifySubnetAttribute func(*awsec2.ModifySubnetAttributeInput) (*awsec2.ModifySubnetAttributeOutput, error)
}

func (f *fakeSubnetAPI) CreateSubnet(_ context.Context, in *awsec2.CreateSubnetInput, _ ...func(*awsec2.Options)) (*awsec2.CreateSubnetOutput, error) {
	f.createSubnetCalls++
	if f.createSubnet == nil {
		return &awsec2.CreateSubnetOutput{}, nil
	}
	return f.createSubnet(in)
}

func (f *fakeSubnetAPI) DeleteSubnet(_ context.Context, in *awsec2.DeleteSubnetInput, _ ...func(*awsec2.Options)) (*awsec2.DeleteSubnetOutput, error) {
	f.deleteSubnetCalls++
	if f.deleteSubnet == nil {
		return &awsec2.DeleteSubnetOutput{}, nil
	}
	return f.deleteSubnet(in)
}

func (f *fakeSubnetAPI) DescribeSubnets(_ context.Context, in *awsec2.DescribeSubnetsInput, _ ...func(*awsec2.Options)) (*awsec2.DescribeSubnetsOutput, error) {
	if f.describeSubnets == nil {
		return &awsec2.DescribeSubnetsOutput{}, nil
	}
	return f.describeSubnets(in)
}

func (f *fakeSubnetAPI) ModifySubnetAttribute(_ context.Context, in *awsec2.ModifySubnetAttributeInput, _ ...func(*awsec2.Options)) (*awsec2.ModifySubnetAttributeOutput, error) {
	if f.modifySubnetAttribute == nil {
		return &awsec2.ModifySubnetAttributeOutput{}, nil
	}
	return f.modifySubnetAttribute(in)
}

func (f *fakeSubnetAPI) CreateTags(_ context.Context, _ *awsec2.CreateTagsInput, _ ...func(*awsec2.Options)) (*awsec2.CreateTagsOutput, error) {
	return &awsec2.CreateTagsOutput{}, nil
}

func (f *fakeSubnetAPI) DeleteTags(_ context.Context, _ *awsec2.DeleteTagsInput, _ ...func(*awsec2.Options)) (*awsec2.DeleteTagsOutput, error) {
	return &awsec2.DeleteTagsOutput{}, nil
}

func (f *fakeSubnetAPI) DescribeTags(_ context.Context, _ *awsec2.DescribeTagsInput, _ ...func(*awsec2.Options)) (*awsec2.DescribeTagsOutput, error) {
	return &awsec2.DescribeTagsOutput{}, nil
}

func subnetTestClient(s *runtime.Scheme, objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(&awsv1alpha1.Subnet{}, &awsv1alpha1.VPC{}).
		Build()
}

func TestSubnetReconcile(t *testing.T) {
	ctx := context.Background()
	nn := k8stypes.NamespacedName{Name: "test-subnet", Namespace: "default"}
	req := ctrl.Request{NamespacedName: nn}

	newSubnet := func(mutate ...func(*awsv1alpha1.Subnet)) *awsv1alpha1.Subnet {
		sn := &awsv1alpha1.Subnet{
			ObjectMeta: metav1.ObjectMeta{Name: nn.Name, Namespace: nn.Namespace, Finalizers: []string{awsv1alpha1.FinalizerName}},
			Spec: awsv1alpha1.SubnetSpec{
				VPCRef:           awsv1alpha1.VPCResourceRef{ID: "vpc-0123"},
				CIDRBlock:        "10.0.1.0/24",
				AvailabilityZone: "us-east-1a",
			},
		}
		for _, m := range mutate {
			m(sn)
		}
		return sn
	}

	t.Run("create happy path persists ID and Ready", func(t *testing.T) {
		s := vpcTestScheme(t)
		cl := subnetTestClient(s, newSubnet())
		f := &fakeSubnetAPI{
			createSubnet: func(in *awsec2.CreateSubnetInput) (*awsec2.CreateSubnetOutput, error) {
				if aws.ToString(in.VpcId) != "vpc-0123" {
					t.Errorf("CreateSubnet vpcId = %q, want vpc-0123", aws.ToString(in.VpcId))
				}
				if aws.ToString(in.CidrBlock) != "10.0.1.0/24" {
					t.Errorf("CreateSubnet cidr = %q, want 10.0.1.0/24", aws.ToString(in.CidrBlock))
				}
				return &awsec2.CreateSubnetOutput{Subnet: &ec2types.Subnet{
					SubnetId:                aws.String("subnet-0123"),
					AvailableIpAddressCount: aws.Int32(251),
				}}, nil
			},
		}
		r := &SubnetReconciler{Client: cl, Scheme: s, EC2Client: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		got := &awsv1alpha1.Subnet{}
		if err := cl.Get(ctx, nn, got); err != nil {
			t.Fatalf("Get: %v", err)
		}
		if f.createSubnetCalls != 1 {
			t.Errorf("CreateSubnet calls = %d, want 1", f.createSubnetCalls)
		}
		if got.Status.SubnetID != "subnet-0123" {
			t.Errorf("status.subnetId = %q, want subnet-0123", got.Status.SubnetID)
		}
		cond := meta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
		if cond == nil || cond.Status != metav1.ConditionTrue {
			t.Errorf("Ready condition = %+v, want True", cond)
		}
	})

	t.Run("SubnetID persisted when ModifySubnetAttribute fails after create", func(t *testing.T) {
		s := vpcTestScheme(t)
		cl := subnetTestClient(s, newSubnet())
		f := &fakeSubnetAPI{
			createSubnet: func(*awsec2.CreateSubnetInput) (*awsec2.CreateSubnetOutput, error) {
				return &awsec2.CreateSubnetOutput{Subnet: &ec2types.Subnet{
					SubnetId: aws.String("subnet-0123"),
				}}, nil
			},
			modifySubnetAttribute: func(*awsec2.ModifySubnetAttributeInput) (*awsec2.ModifySubnetAttributeOutput, error) {
				return nil, errors.New("modify boom")
			},
		}
		r := &SubnetReconciler{Client: cl, Scheme: s, EC2Client: f}

		if _, err := r.Reconcile(ctx, req); err == nil {
			t.Fatal("Reconcile: want error, got nil")
		}
		got := &awsv1alpha1.Subnet{}
		if err := cl.Get(ctx, nn, got); err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Status.SubnetID != "subnet-0123" {
			t.Errorf("status.subnetId = %q, want subnet-0123 persisted despite later failure", got.Status.SubnetID)
		}
	})

	t.Run("steady state does not call create", func(t *testing.T) {
		s := vpcTestScheme(t)
		existing := newSubnet(func(sn *awsv1alpha1.Subnet) {
			sn.Finalizers = []string{awsv1alpha1.FinalizerName}
			sn.Status.SubnetID = "subnet-0123"
		})
		cl := subnetTestClient(s, existing)
		f := &fakeSubnetAPI{
			describeSubnets: func(*awsec2.DescribeSubnetsInput) (*awsec2.DescribeSubnetsOutput, error) {
				return &awsec2.DescribeSubnetsOutput{Subnets: []ec2types.Subnet{{
					SubnetId:                aws.String("subnet-0123"),
					AvailableIpAddressCount: aws.Int32(200),
				}}}, nil
			},
		}
		r := &SubnetReconciler{Client: cl, Scheme: s, EC2Client: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if f.createSubnetCalls != 0 {
			t.Errorf("CreateSubnet calls = %d, want 0", f.createSubnetCalls)
		}
	})

	t.Run("delete with finalizer calls DeleteSubnet with status ID", func(t *testing.T) {
		s := vpcTestScheme(t)
		existing := newSubnet(func(sn *awsv1alpha1.Subnet) {
			sn.Finalizers = []string{awsv1alpha1.FinalizerName}
			sn.Status.SubnetID = "subnet-0123"
		})
		cl := subnetTestClient(s, existing)
		var deletedID string
		f := &fakeSubnetAPI{
			deleteSubnet: func(in *awsec2.DeleteSubnetInput) (*awsec2.DeleteSubnetOutput, error) {
				deletedID = aws.ToString(in.SubnetId)
				return &awsec2.DeleteSubnetOutput{}, nil
			},
		}
		r := &SubnetReconciler{Client: cl, Scheme: s, EC2Client: f}

		if err := cl.Delete(ctx, existing); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if deletedID != "subnet-0123" {
			t.Errorf("DeleteSubnet id = %q, want subnet-0123", deletedID)
		}
		if err := cl.Get(ctx, nn, &awsv1alpha1.Subnet{}); !apierrors.IsNotFound(err) {
			t.Errorf("Get after finalize = %v, want NotFound", err)
		}
	})

	t.Run("abandon annotation skips AWS delete and removes finalizer", func(t *testing.T) {
		s := vpcTestScheme(t)
		existing := newSubnet(func(sn *awsv1alpha1.Subnet) {
			sn.Finalizers = []string{awsv1alpha1.FinalizerName}
			sn.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			sn.Status.SubnetID = "subnet-0123"
		})
		cl := subnetTestClient(s, existing)
		f := &fakeSubnetAPI{}
		r := &SubnetReconciler{Client: cl, Scheme: s, EC2Client: f}

		if err := cl.Delete(ctx, existing); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if f.deleteSubnetCalls != 0 {
			t.Errorf("DeleteSubnet calls = %d, want 0", f.deleteSubnetCalls)
		}
		if err := cl.Get(ctx, nn, &awsv1alpha1.Subnet{}); !apierrors.IsNotFound(err) {
			t.Errorf("Get after finalize = %v, want NotFound", err)
		}
	})

	t.Run("delete fallback uses CIDR/VPC lookup when status ID lost", func(t *testing.T) {
		s := vpcTestScheme(t)
		existing := newSubnet(func(sn *awsv1alpha1.Subnet) {
			sn.Finalizers = []string{awsv1alpha1.FinalizerName}
		})
		cl := subnetTestClient(s, existing)
		var deletedID string
		var gotFilters []ec2types.Filter
		f := &fakeSubnetAPI{
			describeSubnets: func(in *awsec2.DescribeSubnetsInput) (*awsec2.DescribeSubnetsOutput, error) {
				gotFilters = in.Filters
				return &awsec2.DescribeSubnetsOutput{
					Subnets: []ec2types.Subnet{{SubnetId: aws.String("subnet-found")}},
				}, nil
			},
			deleteSubnet: func(in *awsec2.DeleteSubnetInput) (*awsec2.DeleteSubnetOutput, error) {
				deletedID = aws.ToString(in.SubnetId)
				return &awsec2.DeleteSubnetOutput{}, nil
			},
		}
		r := &SubnetReconciler{Client: cl, Scheme: s, EC2Client: f}

		if err := cl.Delete(ctx, existing); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if deletedID != "subnet-found" {
			t.Errorf("DeleteSubnet id = %q, want subnet-found (from fallback lookup)", deletedID)
		}
		var haveCIDR, haveVPC bool
		for _, flt := range gotFilters {
			switch aws.ToString(flt.Name) {
			case "cidr-block":
				haveCIDR = true
			case "vpc-id":
				haveVPC = true
			}
		}
		if !haveCIDR || !haveVPC {
			t.Errorf("fallback lookup filters = %+v, want cidr-block and vpc-id filters", gotFilters)
		}
	})
}
