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

// fakeSGAPI implements SecurityGroupAWSAPI with overridable function fields.
type fakeSGAPI struct {
	createCalls   int
	deleteCalls   int
	revokeIngress int
	authIngress   int
	revokeEgress  int
	authEgress    int

	createSecurityGroup    func(*awsec2.CreateSecurityGroupInput) (*awsec2.CreateSecurityGroupOutput, error)
	deleteSecurityGroup    func(*awsec2.DeleteSecurityGroupInput) (*awsec2.DeleteSecurityGroupOutput, error)
	describeSecurityGroups func(*awsec2.DescribeSecurityGroupsInput) (*awsec2.DescribeSecurityGroupsOutput, error)
}

func (f *fakeSGAPI) CreateSecurityGroup(_ context.Context, in *awsec2.CreateSecurityGroupInput, _ ...func(*awsec2.Options)) (*awsec2.CreateSecurityGroupOutput, error) {
	f.createCalls++
	if f.createSecurityGroup == nil {
		return &awsec2.CreateSecurityGroupOutput{GroupId: aws.String("sg-0123")}, nil
	}
	return f.createSecurityGroup(in)
}

func (f *fakeSGAPI) DeleteSecurityGroup(_ context.Context, in *awsec2.DeleteSecurityGroupInput, _ ...func(*awsec2.Options)) (*awsec2.DeleteSecurityGroupOutput, error) {
	f.deleteCalls++
	if f.deleteSecurityGroup == nil {
		return &awsec2.DeleteSecurityGroupOutput{}, nil
	}
	return f.deleteSecurityGroup(in)
}

func (f *fakeSGAPI) DescribeSecurityGroups(_ context.Context, in *awsec2.DescribeSecurityGroupsInput, _ ...func(*awsec2.Options)) (*awsec2.DescribeSecurityGroupsOutput, error) {
	if f.describeSecurityGroups == nil {
		return &awsec2.DescribeSecurityGroupsOutput{}, nil
	}
	return f.describeSecurityGroups(in)
}

func (f *fakeSGAPI) AuthorizeSecurityGroupIngress(_ context.Context, _ *awsec2.AuthorizeSecurityGroupIngressInput, _ ...func(*awsec2.Options)) (*awsec2.AuthorizeSecurityGroupIngressOutput, error) {
	f.authIngress++
	return &awsec2.AuthorizeSecurityGroupIngressOutput{}, nil
}

func (f *fakeSGAPI) RevokeSecurityGroupIngress(_ context.Context, _ *awsec2.RevokeSecurityGroupIngressInput, _ ...func(*awsec2.Options)) (*awsec2.RevokeSecurityGroupIngressOutput, error) {
	f.revokeIngress++
	return &awsec2.RevokeSecurityGroupIngressOutput{}, nil
}

func (f *fakeSGAPI) AuthorizeSecurityGroupEgress(_ context.Context, _ *awsec2.AuthorizeSecurityGroupEgressInput, _ ...func(*awsec2.Options)) (*awsec2.AuthorizeSecurityGroupEgressOutput, error) {
	f.authEgress++
	return &awsec2.AuthorizeSecurityGroupEgressOutput{}, nil
}

func (f *fakeSGAPI) RevokeSecurityGroupEgress(_ context.Context, _ *awsec2.RevokeSecurityGroupEgressInput, _ ...func(*awsec2.Options)) (*awsec2.RevokeSecurityGroupEgressOutput, error) {
	f.revokeEgress++
	return &awsec2.RevokeSecurityGroupEgressOutput{}, nil
}

func (f *fakeSGAPI) CreateTags(_ context.Context, _ *awsec2.CreateTagsInput, _ ...func(*awsec2.Options)) (*awsec2.CreateTagsOutput, error) {
	return &awsec2.CreateTagsOutput{}, nil
}

func (f *fakeSGAPI) DeleteTags(_ context.Context, _ *awsec2.DeleteTagsInput, _ ...func(*awsec2.Options)) (*awsec2.DeleteTagsOutput, error) {
	return &awsec2.DeleteTagsOutput{}, nil
}

func (f *fakeSGAPI) DescribeTags(_ context.Context, _ *awsec2.DescribeTagsInput, _ ...func(*awsec2.Options)) (*awsec2.DescribeTagsOutput, error) {
	return &awsec2.DescribeTagsOutput{}, nil
}

// sgDescribeExisting serves DescribeSecurityGroups with a single existing
// group carrying one ingress rule (so revoke paths have something to revoke).
func sgDescribeExisting(id string) func(*awsec2.DescribeSecurityGroupsInput) (*awsec2.DescribeSecurityGroupsOutput, error) {
	return func(*awsec2.DescribeSecurityGroupsInput) (*awsec2.DescribeSecurityGroupsOutput, error) {
		return &awsec2.DescribeSecurityGroupsOutput{
			SecurityGroups: []ec2types.SecurityGroup{{
				GroupId: aws.String(id),
				IpPermissions: []ec2types.IpPermission{{
					IpProtocol: aws.String("tcp"),
					FromPort:   aws.Int32(22),
					ToPort:     aws.Int32(22),
					IpRanges:   []ec2types.IpRange{{CidrIp: aws.String("192.0.2.0/24")}},
				}},
			}},
		}, nil
	}
}

func sgTestClient(s *runtime.Scheme, objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(&awsv1alpha1.SecurityGroup{}, &awsv1alpha1.VPC{}).
		Build()
}

func TestSecurityGroupReconcile(t *testing.T) {
	ctx := context.Background()
	nn := k8stypes.NamespacedName{Name: "test-sg", Namespace: "default"}
	req := ctrl.Request{NamespacedName: nn}

	newSG := func(mutate ...func(*awsv1alpha1.SecurityGroup)) *awsv1alpha1.SecurityGroup {
		sg := &awsv1alpha1.SecurityGroup{
			ObjectMeta: metav1.ObjectMeta{Name: nn.Name, Namespace: nn.Namespace, Finalizers: []string{awsv1alpha1.FinalizerName}, Generation: 1},
			Spec: awsv1alpha1.SecurityGroupSpec{
				VPCRef:      awsv1alpha1.VPCResourceRef{ID: "vpc-0123"},
				GroupName:   "web-sg",
				Description: "web tier",
				IngressRules: []awsv1alpha1.SGRule{{
					Protocol: "tcp",
					FromPort: 443,
					ToPort:   443,
					CIDRIPv4: "10.0.0.0/16",
				}},
			},
		}
		for _, m := range mutate {
			m(sg)
		}
		return sg
	}

	t.Run("create happy path persists ID and Ready", func(t *testing.T) {
		s := vpcTestScheme(t)
		cl := sgTestClient(s, newSG())
		f := &fakeSGAPI{
			createSecurityGroup: func(in *awsec2.CreateSecurityGroupInput) (*awsec2.CreateSecurityGroupOutput, error) {
				if aws.ToString(in.GroupName) != "web-sg" {
					t.Errorf("CreateSecurityGroup name = %q, want web-sg", aws.ToString(in.GroupName))
				}
				if aws.ToString(in.VpcId) != "vpc-0123" {
					t.Errorf("CreateSecurityGroup vpcId = %q, want vpc-0123", aws.ToString(in.VpcId))
				}
				return &awsec2.CreateSecurityGroupOutput{GroupId: aws.String("sg-0123")}, nil
			},
			describeSecurityGroups: sgDescribeExisting("sg-0123"),
		}
		r := &SecurityGroupReconciler{Client: cl, Scheme: s, EC2Client: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		got := &awsv1alpha1.SecurityGroup{}
		if err := cl.Get(ctx, nn, got); err != nil {
			t.Fatalf("Get: %v", err)
		}
		if f.createCalls != 1 {
			t.Errorf("CreateSecurityGroup calls = %d, want 1", f.createCalls)
		}
		if got.Status.GroupID != "sg-0123" {
			t.Errorf("status.groupId = %q, want sg-0123", got.Status.GroupID)
		}
		cond := meta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
		if cond == nil || cond.Status != metav1.ConditionTrue {
			t.Errorf("Ready condition = %+v, want True", cond)
		}
		if got.Status.ObservedGeneration != got.Generation {
			t.Errorf("observedGeneration = %d, want %d", got.Status.ObservedGeneration, got.Generation)
		}
	})

	t.Run("rules synced when generation changed", func(t *testing.T) {
		s := vpcTestScheme(t)
		existing := newSG(func(sg *awsv1alpha1.SecurityGroup) {
			sg.Finalizers = []string{awsv1alpha1.FinalizerName}
			sg.Generation = 3
			sg.Status.GroupID = "sg-0123"
			sg.Status.ObservedGeneration = 2 // spec changed since last sync
		})
		cl := sgTestClient(s, existing)
		f := &fakeSGAPI{describeSecurityGroups: sgDescribeExisting("sg-0123")}
		r := &SecurityGroupReconciler{Client: cl, Scheme: s, EC2Client: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if f.revokeIngress != 1 {
			t.Errorf("RevokeSecurityGroupIngress calls = %d, want 1", f.revokeIngress)
		}
		if f.authIngress != 1 {
			t.Errorf("AuthorizeSecurityGroupIngress calls = %d, want 1", f.authIngress)
		}
	})

	t.Run("steady state resync skips rule churn and create", func(t *testing.T) {
		s := vpcTestScheme(t)
		existing := newSG(func(sg *awsv1alpha1.SecurityGroup) {
			sg.Finalizers = []string{awsv1alpha1.FinalizerName}
			sg.Generation = 3
			sg.Status.GroupID = "sg-0123"
			sg.Status.ObservedGeneration = 3 // ObservedGeneration == Generation
		})
		cl := sgTestClient(s, existing)
		f := &fakeSGAPI{describeSecurityGroups: sgDescribeExisting("sg-0123")}
		r := &SecurityGroupReconciler{Client: cl, Scheme: s, EC2Client: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if f.createCalls != 0 {
			t.Errorf("CreateSecurityGroup calls = %d, want 0", f.createCalls)
		}
		if f.revokeIngress != 0 || f.authIngress != 0 || f.revokeEgress != 0 || f.authEgress != 0 {
			t.Errorf("rule churn on steady state: revokeIn=%d authIn=%d revokeEg=%d authEg=%d, want all 0",
				f.revokeIngress, f.authIngress, f.revokeEgress, f.authEgress)
		}
	})

	t.Run("delete with finalizer calls DeleteSecurityGroup with status ID", func(t *testing.T) {
		s := vpcTestScheme(t)
		existing := newSG(func(sg *awsv1alpha1.SecurityGroup) {
			sg.Finalizers = []string{awsv1alpha1.FinalizerName}
			sg.Status.GroupID = "sg-0123"
		})
		cl := sgTestClient(s, existing)
		var deletedID string
		f := &fakeSGAPI{
			deleteSecurityGroup: func(in *awsec2.DeleteSecurityGroupInput) (*awsec2.DeleteSecurityGroupOutput, error) {
				deletedID = aws.ToString(in.GroupId)
				return &awsec2.DeleteSecurityGroupOutput{}, nil
			},
		}
		r := &SecurityGroupReconciler{Client: cl, Scheme: s, EC2Client: f}

		if err := cl.Delete(ctx, existing); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if deletedID != "sg-0123" {
			t.Errorf("DeleteSecurityGroup id = %q, want sg-0123", deletedID)
		}
		if err := cl.Get(ctx, nn, &awsv1alpha1.SecurityGroup{}); !apierrors.IsNotFound(err) {
			t.Errorf("Get after finalize = %v, want NotFound", err)
		}
	})

	t.Run("abandon annotation skips AWS delete and removes finalizer", func(t *testing.T) {
		s := vpcTestScheme(t)
		existing := newSG(func(sg *awsv1alpha1.SecurityGroup) {
			sg.Finalizers = []string{awsv1alpha1.FinalizerName}
			sg.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			sg.Status.GroupID = "sg-0123"
		})
		cl := sgTestClient(s, existing)
		f := &fakeSGAPI{}
		r := &SecurityGroupReconciler{Client: cl, Scheme: s, EC2Client: f}

		if err := cl.Delete(ctx, existing); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if f.deleteCalls != 0 {
			t.Errorf("DeleteSecurityGroup calls = %d, want 0", f.deleteCalls)
		}
		if err := cl.Get(ctx, nn, &awsv1alpha1.SecurityGroup{}); !apierrors.IsNotFound(err) {
			t.Errorf("Get after finalize = %v, want NotFound", err)
		}
	})

	t.Run("delete fallback uses group-name lookup when status ID lost", func(t *testing.T) {
		s := vpcTestScheme(t)
		existing := newSG(func(sg *awsv1alpha1.SecurityGroup) {
			sg.Finalizers = []string{awsv1alpha1.FinalizerName}
		})
		cl := sgTestClient(s, existing)
		var deletedID string
		var gotFilters []ec2types.Filter
		f := &fakeSGAPI{
			describeSecurityGroups: func(in *awsec2.DescribeSecurityGroupsInput) (*awsec2.DescribeSecurityGroupsOutput, error) {
				gotFilters = in.Filters
				return &awsec2.DescribeSecurityGroupsOutput{
					SecurityGroups: []ec2types.SecurityGroup{{GroupId: aws.String("sg-found")}},
				}, nil
			},
			deleteSecurityGroup: func(in *awsec2.DeleteSecurityGroupInput) (*awsec2.DeleteSecurityGroupOutput, error) {
				deletedID = aws.ToString(in.GroupId)
				return &awsec2.DeleteSecurityGroupOutput{}, nil
			},
		}
		r := &SecurityGroupReconciler{Client: cl, Scheme: s, EC2Client: f}

		if err := cl.Delete(ctx, existing); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
		if deletedID != "sg-found" {
			t.Errorf("DeleteSecurityGroup id = %q, want sg-found (from fallback lookup)", deletedID)
		}
		var haveName, haveVPC bool
		for _, flt := range gotFilters {
			switch aws.ToString(flt.Name) {
			case "group-name":
				haveName = true
				if len(flt.Values) != 1 || flt.Values[0] != "web-sg" {
					t.Errorf("group-name filter values = %v, want [web-sg]", flt.Values)
				}
			case "vpc-id":
				haveVPC = true
			}
		}
		if !haveName || !haveVPC {
			t.Errorf("fallback lookup filters = %+v, want group-name and vpc-id filters", gotFilters)
		}
	})
}
