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

// fakeVPCAPI implements VPCAWSAPI with overridable function fields.
// Methods with a nil function field return an empty output.
type fakeVPCAPI struct {
	createVpcCalls int
	deleteVpcCalls int

	createVpc                func(*awsec2.CreateVpcInput) (*awsec2.CreateVpcOutput, error)
	deleteVpc                func(*awsec2.DeleteVpcInput) (*awsec2.DeleteVpcOutput, error)
	describeVpcs             func(*awsec2.DescribeVpcsInput) (*awsec2.DescribeVpcsOutput, error)
	modifyVpcAttribute       func(*awsec2.ModifyVpcAttributeInput) (*awsec2.ModifyVpcAttributeOutput, error)
	associateVpcCidrBlock    func(*awsec2.AssociateVpcCidrBlockInput) (*awsec2.AssociateVpcCidrBlockOutput, error)
	disassociateVpcCidrBlock func(*awsec2.DisassociateVpcCidrBlockInput) (*awsec2.DisassociateVpcCidrBlockOutput, error)
}

func (f *fakeVPCAPI) CreateVpc(_ context.Context, in *awsec2.CreateVpcInput, _ ...func(*awsec2.Options)) (*awsec2.CreateVpcOutput, error) {
	f.createVpcCalls++
	if f.createVpc == nil {
		return &awsec2.CreateVpcOutput{}, nil
	}
	return f.createVpc(in)
}

func (f *fakeVPCAPI) DeleteVpc(_ context.Context, in *awsec2.DeleteVpcInput, _ ...func(*awsec2.Options)) (*awsec2.DeleteVpcOutput, error) {
	f.deleteVpcCalls++
	if f.deleteVpc == nil {
		return &awsec2.DeleteVpcOutput{}, nil
	}
	return f.deleteVpc(in)
}

func (f *fakeVPCAPI) DescribeVpcs(_ context.Context, in *awsec2.DescribeVpcsInput, _ ...func(*awsec2.Options)) (*awsec2.DescribeVpcsOutput, error) {
	if f.describeVpcs == nil {
		return &awsec2.DescribeVpcsOutput{}, nil
	}
	return f.describeVpcs(in)
}

func (f *fakeVPCAPI) ModifyVpcAttribute(_ context.Context, in *awsec2.ModifyVpcAttributeInput, _ ...func(*awsec2.Options)) (*awsec2.ModifyVpcAttributeOutput, error) {
	if f.modifyVpcAttribute == nil {
		return &awsec2.ModifyVpcAttributeOutput{}, nil
	}
	return f.modifyVpcAttribute(in)
}

func (f *fakeVPCAPI) AssociateVpcCidrBlock(_ context.Context, in *awsec2.AssociateVpcCidrBlockInput, _ ...func(*awsec2.Options)) (*awsec2.AssociateVpcCidrBlockOutput, error) {
	if f.associateVpcCidrBlock == nil {
		return &awsec2.AssociateVpcCidrBlockOutput{}, nil
	}
	return f.associateVpcCidrBlock(in)
}

func (f *fakeVPCAPI) DisassociateVpcCidrBlock(_ context.Context, in *awsec2.DisassociateVpcCidrBlockInput, _ ...func(*awsec2.Options)) (*awsec2.DisassociateVpcCidrBlockOutput, error) {
	if f.disassociateVpcCidrBlock == nil {
		return &awsec2.DisassociateVpcCidrBlockOutput{}, nil
	}
	return f.disassociateVpcCidrBlock(in)
}

func (f *fakeVPCAPI) CreateTags(_ context.Context, _ *awsec2.CreateTagsInput, _ ...func(*awsec2.Options)) (*awsec2.CreateTagsOutput, error) {
	return &awsec2.CreateTagsOutput{}, nil
}

func (f *fakeVPCAPI) DeleteTags(_ context.Context, _ *awsec2.DeleteTagsInput, _ ...func(*awsec2.Options)) (*awsec2.DeleteTagsOutput, error) {
	return &awsec2.DeleteTagsOutput{}, nil
}

func (f *fakeVPCAPI) DescribeTags(_ context.Context, _ *awsec2.DescribeTagsInput, _ ...func(*awsec2.Options)) (*awsec2.DescribeTagsOutput, error) {
	return &awsec2.DescribeTagsOutput{}, nil
}

func vpcTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return s
}

func vpcTestClient(s *runtime.Scheme, objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(&awsv1alpha1.VPC{}).
		Build()
}

// vpcDescribeByID returns a DescribeVpcs implementation that serves lookups
// by VpcIds for the given VPC ID/CIDR.
func vpcDescribeByID(id, cidr string) func(*awsec2.DescribeVpcsInput) (*awsec2.DescribeVpcsOutput, error) {
	return func(in *awsec2.DescribeVpcsInput) (*awsec2.DescribeVpcsOutput, error) {
		return &awsec2.DescribeVpcsOutput{
			Vpcs: []ec2types.Vpc{{
				VpcId: aws.String(id),
				State: ec2types.VpcStateAvailable,
				CidrBlockAssociationSet: []ec2types.VpcCidrBlockAssociation{{
					AssociationId: aws.String("vpc-cidr-assoc-1"),
					CidrBlock:     aws.String(cidr),
					CidrBlockState: &ec2types.VpcCidrBlockState{
						State: ec2types.VpcCidrBlockStateCodeAssociated,
					},
				}},
			}},
		}, nil
	}
}

func TestVPCReconcile(t *testing.T) {
	ctx := context.Background()
	nn := k8stypes.NamespacedName{Name: "test-vpc", Namespace: "default"}
	req := ctrl.Request{NamespacedName: nn}

	newVPC := func(mutate ...func(*awsv1alpha1.VPC)) *awsv1alpha1.VPC {
		v := &awsv1alpha1.VPC{
			ObjectMeta: metav1.ObjectMeta{Name: nn.Name, Namespace: nn.Namespace, Finalizers: []string{awsv1alpha1.FinalizerName}},
			Spec:       awsv1alpha1.VPCSpec{CIDRBlock: "10.0.0.0/16"},
		}
		for _, m := range mutate {
			m(v)
		}
		return v
	}

	t.Run("create happy path persists ID and Ready", func(t *testing.T) {
		s := vpcTestScheme(t)
		cl := vpcTestClient(s, newVPC())
		f := &fakeVPCAPI{
			createVpc: func(in *awsec2.CreateVpcInput) (*awsec2.CreateVpcOutput, error) {
				if aws.ToString(in.CidrBlock) != "10.0.0.0/16" {
					t.Errorf("CreateVpc cidr = %q, want 10.0.0.0/16", aws.ToString(in.CidrBlock))
				}
				return &awsec2.CreateVpcOutput{Vpc: &ec2types.Vpc{
					VpcId: aws.String("vpc-0123"),
					State: ec2types.VpcStatePending,
				}}, nil
			},
			describeVpcs: vpcDescribeByID("vpc-0123", "10.0.0.0/16"),
		}
		r := &VPCReconciler{Client: cl, Scheme: s, EC2Client: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		got := &awsv1alpha1.VPC{}
		if err := cl.Get(ctx, nn, got); err != nil {
			t.Fatalf("Get: %v", err)
		}
		if f.createVpcCalls != 1 {
			t.Errorf("CreateVpc calls = %d, want 1", f.createVpcCalls)
		}
		if got.Status.VPCID != "vpc-0123" {
			t.Errorf("status.vpcId = %q, want vpc-0123", got.Status.VPCID)
		}
		cond := meta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
		if cond == nil || cond.Status != metav1.ConditionTrue {
			t.Errorf("Ready condition = %+v, want True", cond)
		}
	})

	t.Run("VpcID persisted when ModifyVpcAttribute fails after create", func(t *testing.T) {
		s := vpcTestScheme(t)
		cl := vpcTestClient(s, newVPC())
		f := &fakeVPCAPI{
			createVpc: func(*awsec2.CreateVpcInput) (*awsec2.CreateVpcOutput, error) {
				return &awsec2.CreateVpcOutput{Vpc: &ec2types.Vpc{
					VpcId: aws.String("vpc-0123"),
					State: ec2types.VpcStatePending,
				}}, nil
			},
			modifyVpcAttribute: func(*awsec2.ModifyVpcAttributeInput) (*awsec2.ModifyVpcAttributeOutput, error) {
				return nil, errors.New("modify boom")
			},
		}
		r := &VPCReconciler{Client: cl, Scheme: s, EC2Client: f}

		if _, err := r.Reconcile(ctx, req); err == nil {
			t.Fatal("Reconcile: want error, got nil")
		}
		got := &awsv1alpha1.VPC{}
		if err := cl.Get(ctx, nn, got); err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Status.VPCID != "vpc-0123" {
			t.Errorf("status.vpcId = %q, want vpc-0123 persisted despite later failure", got.Status.VPCID)
		}
	})

	t.Run("steady state does not call create", func(t *testing.T) {
		s := vpcTestScheme(t)
		existing := newVPC(func(v *awsv1alpha1.VPC) {
			v.Finalizers = []string{awsv1alpha1.FinalizerName}
			v.Status.VPCID = "vpc-0123"
		})
		cl := vpcTestClient(s, existing)
		f := &fakeVPCAPI{describeVpcs: vpcDescribeByID("vpc-0123", "10.0.0.0/16")}
		r := &VPCReconciler{Client: cl, Scheme: s, EC2Client: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if f.createVpcCalls != 0 {
			t.Errorf("CreateVpc calls = %d, want 0", f.createVpcCalls)
		}
	})

	t.Run("delete with finalizer calls DeleteVpc with status ID", func(t *testing.T) {
		s := vpcTestScheme(t)
		existing := newVPC(func(v *awsv1alpha1.VPC) {
			v.Finalizers = []string{awsv1alpha1.FinalizerName}
			v.Status.VPCID = "vpc-0123"
		})
		cl := vpcTestClient(s, existing)
		var deletedID string
		f := &fakeVPCAPI{
			deleteVpc: func(in *awsec2.DeleteVpcInput) (*awsec2.DeleteVpcOutput, error) {
				deletedID = aws.ToString(in.VpcId)
				return &awsec2.DeleteVpcOutput{}, nil
			},
		}
		r := &VPCReconciler{Client: cl, Scheme: s, EC2Client: f}

		if err := cl.Delete(ctx, existing); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if deletedID != "vpc-0123" {
			t.Errorf("DeleteVpc id = %q, want vpc-0123", deletedID)
		}
		if err := cl.Get(ctx, nn, &awsv1alpha1.VPC{}); !apierrors.IsNotFound(err) {
			t.Errorf("Get after finalize = %v, want NotFound", err)
		}
	})

	t.Run("abandon annotation skips AWS delete and removes finalizer", func(t *testing.T) {
		s := vpcTestScheme(t)
		existing := newVPC(func(v *awsv1alpha1.VPC) {
			v.Finalizers = []string{awsv1alpha1.FinalizerName}
			v.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			v.Status.VPCID = "vpc-0123"
		})
		cl := vpcTestClient(s, existing)
		f := &fakeVPCAPI{}
		r := &VPCReconciler{Client: cl, Scheme: s, EC2Client: f}

		if err := cl.Delete(ctx, existing); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if f.deleteVpcCalls != 0 {
			t.Errorf("DeleteVpc calls = %d, want 0", f.deleteVpcCalls)
		}
		if err := cl.Get(ctx, nn, &awsv1alpha1.VPC{}); !apierrors.IsNotFound(err) {
			t.Errorf("Get after finalize = %v, want NotFound", err)
		}
	})

	t.Run("delete fallback uses CIDR/tag lookup when status ID lost", func(t *testing.T) {
		s := vpcTestScheme(t)
		existing := newVPC(func(v *awsv1alpha1.VPC) {
			v.Finalizers = []string{awsv1alpha1.FinalizerName}
			v.Spec.Tags = map[string]string{"team": "platform"}
		})
		cl := vpcTestClient(s, existing)
		var deletedID string
		var gotFilters []ec2types.Filter
		f := &fakeVPCAPI{
			describeVpcs: func(in *awsec2.DescribeVpcsInput) (*awsec2.DescribeVpcsOutput, error) {
				gotFilters = in.Filters
				return &awsec2.DescribeVpcsOutput{
					Vpcs: []ec2types.Vpc{{VpcId: aws.String("vpc-found")}},
				}, nil
			},
			deleteVpc: func(in *awsec2.DeleteVpcInput) (*awsec2.DeleteVpcOutput, error) {
				deletedID = aws.ToString(in.VpcId)
				return &awsec2.DeleteVpcOutput{}, nil
			},
		}
		r := &VPCReconciler{Client: cl, Scheme: s, EC2Client: f}

		if err := cl.Delete(ctx, existing); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if deletedID != "vpc-found" {
			t.Errorf("DeleteVpc id = %q, want vpc-found (from fallback lookup)", deletedID)
		}
		foundCIDRFilter := false
		for _, flt := range gotFilters {
			if aws.ToString(flt.Name) == "cidr-block-association.cidr-block" {
				foundCIDRFilter = true
			}
		}
		if !foundCIDRFilter {
			t.Errorf("fallback lookup filters = %+v, want cidr-block-association.cidr-block filter", gotFilters)
		}
	})
}
