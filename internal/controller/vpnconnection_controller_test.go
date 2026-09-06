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
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

// fakeVPNEC2 implements the EC2 interfaces used by the netsec EC2 controllers.
type fakeVPNEC2 struct {
	describeCustomerGateways func(*awsec2.DescribeCustomerGatewaysInput) (*awsec2.DescribeCustomerGatewaysOutput, error)
	createCustomerGateway    func(*awsec2.CreateCustomerGatewayInput) (*awsec2.CreateCustomerGatewayOutput, error)
	deleteCustomerGateway    func(*awsec2.DeleteCustomerGatewayInput) (*awsec2.DeleteCustomerGatewayOutput, error)

	describeVpnGateways func(*awsec2.DescribeVpnGatewaysInput) (*awsec2.DescribeVpnGatewaysOutput, error)
	createVpnGateway    func(*awsec2.CreateVpnGatewayInput) (*awsec2.CreateVpnGatewayOutput, error)
	deleteVpnGateway    func(*awsec2.DeleteVpnGatewayInput) (*awsec2.DeleteVpnGatewayOutput, error)
	attachVpnGateway    func(*awsec2.AttachVpnGatewayInput) (*awsec2.AttachVpnGatewayOutput, error)
	detachVpnGateway    func(*awsec2.DetachVpnGatewayInput) (*awsec2.DetachVpnGatewayOutput, error)

	describeVpnConnections func(*awsec2.DescribeVpnConnectionsInput) (*awsec2.DescribeVpnConnectionsOutput, error)
	createVpnConnection    func(*awsec2.CreateVpnConnectionInput) (*awsec2.CreateVpnConnectionOutput, error)
	deleteVpnConnection    func(*awsec2.DeleteVpnConnectionInput) (*awsec2.DeleteVpnConnectionOutput, error)

	createVpnConnectionRoute func(*awsec2.CreateVpnConnectionRouteInput) (*awsec2.CreateVpnConnectionRouteOutput, error)
	deleteVpnConnectionRoute func(*awsec2.DeleteVpnConnectionRouteInput) (*awsec2.DeleteVpnConnectionRouteOutput, error)

	describeManagedPrefixLists  func(*awsec2.DescribeManagedPrefixListsInput) (*awsec2.DescribeManagedPrefixListsOutput, error)
	getManagedPrefixListEntries func(*awsec2.GetManagedPrefixListEntriesInput) (*awsec2.GetManagedPrefixListEntriesOutput, error)
	createManagedPrefixList     func(*awsec2.CreateManagedPrefixListInput) (*awsec2.CreateManagedPrefixListOutput, error)
	modifyManagedPrefixList     func(*awsec2.ModifyManagedPrefixListInput) (*awsec2.ModifyManagedPrefixListOutput, error)
	deleteManagedPrefixList     func(*awsec2.DeleteManagedPrefixListInput) (*awsec2.DeleteManagedPrefixListOutput, error)

	describeCapacityReservations func(*awsec2.DescribeCapacityReservationsInput) (*awsec2.DescribeCapacityReservationsOutput, error)
	createCapacityReservation    func(*awsec2.CreateCapacityReservationInput) (*awsec2.CreateCapacityReservationOutput, error)
	modifyCapacityReservation    func(*awsec2.ModifyCapacityReservationInput) (*awsec2.ModifyCapacityReservationOutput, error)
	cancelCapacityReservation    func(*awsec2.CancelCapacityReservationInput) (*awsec2.CancelCapacityReservationOutput, error)

	createTags func(*awsec2.CreateTagsInput) (*awsec2.CreateTagsOutput, error)

	createCGWCalled   bool
	deleteCGWCalled   bool
	createVGWCalled   bool
	deleteVGWCalled   bool
	createVPNCalled   bool
	deleteVPNCalled   bool
	createRouteCalled bool
	deleteRouteCalled bool
	createMPLCalled   bool
	deleteMPLCalled   bool
	createCapCalled   bool
	cancelCapCalled   bool
	createVPNInput    *awsec2.CreateVpnConnectionInput
	deleteVPNInput    *awsec2.DeleteVpnConnectionInput
	createRouteInput  *awsec2.CreateVpnConnectionRouteInput
	deleteRouteInput  *awsec2.DeleteVpnConnectionRouteInput
	modifyMPLCalled   bool
	modifyMPLInput    *awsec2.ModifyManagedPrefixListInput
}

func (f *fakeVPNEC2) DescribeCustomerGateways(_ context.Context, p *awsec2.DescribeCustomerGatewaysInput, _ ...func(*awsec2.Options)) (*awsec2.DescribeCustomerGatewaysOutput, error) {
	if f.describeCustomerGateways == nil {
		return nil, fmt.Errorf("unexpected call to DescribeCustomerGateways")
	}
	return f.describeCustomerGateways(p)
}

func (f *fakeVPNEC2) CreateCustomerGateway(_ context.Context, p *awsec2.CreateCustomerGatewayInput, _ ...func(*awsec2.Options)) (*awsec2.CreateCustomerGatewayOutput, error) {
	f.createCGWCalled = true
	if f.createCustomerGateway == nil {
		return nil, fmt.Errorf("unexpected call to CreateCustomerGateway")
	}
	return f.createCustomerGateway(p)
}

func (f *fakeVPNEC2) DeleteCustomerGateway(_ context.Context, p *awsec2.DeleteCustomerGatewayInput, _ ...func(*awsec2.Options)) (*awsec2.DeleteCustomerGatewayOutput, error) {
	f.deleteCGWCalled = true
	if f.deleteCustomerGateway == nil {
		return nil, fmt.Errorf("unexpected call to DeleteCustomerGateway")
	}
	return f.deleteCustomerGateway(p)
}

func (f *fakeVPNEC2) DescribeVpnGateways(_ context.Context, p *awsec2.DescribeVpnGatewaysInput, _ ...func(*awsec2.Options)) (*awsec2.DescribeVpnGatewaysOutput, error) {
	if f.describeVpnGateways == nil {
		return nil, fmt.Errorf("unexpected call to DescribeVpnGateways")
	}
	return f.describeVpnGateways(p)
}

func (f *fakeVPNEC2) CreateVpnGateway(_ context.Context, p *awsec2.CreateVpnGatewayInput, _ ...func(*awsec2.Options)) (*awsec2.CreateVpnGatewayOutput, error) {
	f.createVGWCalled = true
	if f.createVpnGateway == nil {
		return nil, fmt.Errorf("unexpected call to CreateVpnGateway")
	}
	return f.createVpnGateway(p)
}

func (f *fakeVPNEC2) DeleteVpnGateway(_ context.Context, p *awsec2.DeleteVpnGatewayInput, _ ...func(*awsec2.Options)) (*awsec2.DeleteVpnGatewayOutput, error) {
	f.deleteVGWCalled = true
	if f.deleteVpnGateway == nil {
		return nil, fmt.Errorf("unexpected call to DeleteVpnGateway")
	}
	return f.deleteVpnGateway(p)
}

func (f *fakeVPNEC2) AttachVpnGateway(_ context.Context, p *awsec2.AttachVpnGatewayInput, _ ...func(*awsec2.Options)) (*awsec2.AttachVpnGatewayOutput, error) {
	if f.attachVpnGateway == nil {
		return nil, fmt.Errorf("unexpected call to AttachVpnGateway")
	}
	return f.attachVpnGateway(p)
}

func (f *fakeVPNEC2) DetachVpnGateway(_ context.Context, p *awsec2.DetachVpnGatewayInput, _ ...func(*awsec2.Options)) (*awsec2.DetachVpnGatewayOutput, error) {
	if f.detachVpnGateway == nil {
		return nil, fmt.Errorf("unexpected call to DetachVpnGateway")
	}
	return f.detachVpnGateway(p)
}

func (f *fakeVPNEC2) DescribeVpnConnections(_ context.Context, p *awsec2.DescribeVpnConnectionsInput, _ ...func(*awsec2.Options)) (*awsec2.DescribeVpnConnectionsOutput, error) {
	if f.describeVpnConnections == nil {
		return nil, fmt.Errorf("unexpected call to DescribeVpnConnections")
	}
	return f.describeVpnConnections(p)
}

func (f *fakeVPNEC2) CreateVpnConnection(_ context.Context, p *awsec2.CreateVpnConnectionInput, _ ...func(*awsec2.Options)) (*awsec2.CreateVpnConnectionOutput, error) {
	f.createVPNCalled = true
	f.createVPNInput = p
	if f.createVpnConnection == nil {
		return nil, fmt.Errorf("unexpected call to CreateVpnConnection")
	}
	return f.createVpnConnection(p)
}

func (f *fakeVPNEC2) DeleteVpnConnection(_ context.Context, p *awsec2.DeleteVpnConnectionInput, _ ...func(*awsec2.Options)) (*awsec2.DeleteVpnConnectionOutput, error) {
	f.deleteVPNCalled = true
	f.deleteVPNInput = p
	if f.deleteVpnConnection == nil {
		return nil, fmt.Errorf("unexpected call to DeleteVpnConnection")
	}
	return f.deleteVpnConnection(p)
}

func (f *fakeVPNEC2) CreateVpnConnectionRoute(_ context.Context, p *awsec2.CreateVpnConnectionRouteInput, _ ...func(*awsec2.Options)) (*awsec2.CreateVpnConnectionRouteOutput, error) {
	f.createRouteCalled = true
	f.createRouteInput = p
	if f.createVpnConnectionRoute == nil {
		return nil, fmt.Errorf("unexpected call to CreateVpnConnectionRoute")
	}
	return f.createVpnConnectionRoute(p)
}

func (f *fakeVPNEC2) DeleteVpnConnectionRoute(_ context.Context, p *awsec2.DeleteVpnConnectionRouteInput, _ ...func(*awsec2.Options)) (*awsec2.DeleteVpnConnectionRouteOutput, error) {
	f.deleteRouteCalled = true
	f.deleteRouteInput = p
	if f.deleteVpnConnectionRoute == nil {
		return nil, fmt.Errorf("unexpected call to DeleteVpnConnectionRoute")
	}
	return f.deleteVpnConnectionRoute(p)
}

func (f *fakeVPNEC2) DescribeManagedPrefixLists(_ context.Context, p *awsec2.DescribeManagedPrefixListsInput, _ ...func(*awsec2.Options)) (*awsec2.DescribeManagedPrefixListsOutput, error) {
	if f.describeManagedPrefixLists == nil {
		return nil, fmt.Errorf("unexpected call to DescribeManagedPrefixLists")
	}
	return f.describeManagedPrefixLists(p)
}

func (f *fakeVPNEC2) GetManagedPrefixListEntries(_ context.Context, p *awsec2.GetManagedPrefixListEntriesInput, _ ...func(*awsec2.Options)) (*awsec2.GetManagedPrefixListEntriesOutput, error) {
	if f.getManagedPrefixListEntries == nil {
		return nil, fmt.Errorf("unexpected call to GetManagedPrefixListEntries")
	}
	return f.getManagedPrefixListEntries(p)
}

func (f *fakeVPNEC2) CreateManagedPrefixList(_ context.Context, p *awsec2.CreateManagedPrefixListInput, _ ...func(*awsec2.Options)) (*awsec2.CreateManagedPrefixListOutput, error) {
	f.createMPLCalled = true
	if f.createManagedPrefixList == nil {
		return nil, fmt.Errorf("unexpected call to CreateManagedPrefixList")
	}
	return f.createManagedPrefixList(p)
}

func (f *fakeVPNEC2) ModifyManagedPrefixList(_ context.Context, p *awsec2.ModifyManagedPrefixListInput, _ ...func(*awsec2.Options)) (*awsec2.ModifyManagedPrefixListOutput, error) {
	f.modifyMPLCalled = true
	f.modifyMPLInput = p
	if f.modifyManagedPrefixList == nil {
		return nil, fmt.Errorf("unexpected call to ModifyManagedPrefixList")
	}
	return f.modifyManagedPrefixList(p)
}

func (f *fakeVPNEC2) DeleteManagedPrefixList(_ context.Context, p *awsec2.DeleteManagedPrefixListInput, _ ...func(*awsec2.Options)) (*awsec2.DeleteManagedPrefixListOutput, error) {
	f.deleteMPLCalled = true
	if f.deleteManagedPrefixList == nil {
		return nil, fmt.Errorf("unexpected call to DeleteManagedPrefixList")
	}
	return f.deleteManagedPrefixList(p)
}

func (f *fakeVPNEC2) DescribeCapacityReservations(_ context.Context, p *awsec2.DescribeCapacityReservationsInput, _ ...func(*awsec2.Options)) (*awsec2.DescribeCapacityReservationsOutput, error) {
	if f.describeCapacityReservations == nil {
		return nil, fmt.Errorf("unexpected call to DescribeCapacityReservations")
	}
	return f.describeCapacityReservations(p)
}

func (f *fakeVPNEC2) CreateCapacityReservation(_ context.Context, p *awsec2.CreateCapacityReservationInput, _ ...func(*awsec2.Options)) (*awsec2.CreateCapacityReservationOutput, error) {
	f.createCapCalled = true
	if f.createCapacityReservation == nil {
		return nil, fmt.Errorf("unexpected call to CreateCapacityReservation")
	}
	return f.createCapacityReservation(p)
}

func (f *fakeVPNEC2) ModifyCapacityReservation(_ context.Context, p *awsec2.ModifyCapacityReservationInput, _ ...func(*awsec2.Options)) (*awsec2.ModifyCapacityReservationOutput, error) {
	if f.modifyCapacityReservation == nil {
		return nil, fmt.Errorf("unexpected call to ModifyCapacityReservation")
	}
	return f.modifyCapacityReservation(p)
}

func (f *fakeVPNEC2) CancelCapacityReservation(_ context.Context, p *awsec2.CancelCapacityReservationInput, _ ...func(*awsec2.Options)) (*awsec2.CancelCapacityReservationOutput, error) {
	f.cancelCapCalled = true
	if f.cancelCapacityReservation == nil {
		return nil, fmt.Errorf("unexpected call to CancelCapacityReservation")
	}
	return f.cancelCapacityReservation(p)
}

func (f *fakeVPNEC2) CreateTags(_ context.Context, p *awsec2.CreateTagsInput, _ ...func(*awsec2.Options)) (*awsec2.CreateTagsOutput, error) {
	if f.createTags == nil {
		return &awsec2.CreateTagsOutput{}, nil
	}
	return f.createTags(p)
}

func newVPNScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}
	return scheme
}

func newVPNFakeClient(scheme *runtime.Scheme, objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&awsv1alpha1.CustomerGateway{},
			&awsv1alpha1.VPNGateway{},
			&awsv1alpha1.VPNConnection{},
			&awsv1alpha1.VPNConnectionRoute{},
			&awsv1alpha1.ManagedPrefixList{},
			&awsv1alpha1.CapacityReservation{},
		).
		WithObjects(objs...).
		Build()
}

func TestCustomerGatewayReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-cgw", Namespace: "default"}}
	cgwCR := func(mutate ...func(*awsv1alpha1.CustomerGateway)) *awsv1alpha1.CustomerGateway {
		c := &awsv1alpha1.CustomerGateway{
			ObjectMeta: metav1.ObjectMeta{Name: "my-cgw", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
			Spec: awsv1alpha1.CustomerGatewaySpec{
				BGPASN:    65000,
				IPAddress: "203.0.113.10",
				Type:      "ipsec.1",
			},
		}
		for _, m := range mutate {
			m(c)
		}
		return c
	}

	t.Run("create persists identifier", func(t *testing.T) {
		ctx := context.Background()
		scheme := newVPNScheme(t)
		c := newVPNFakeClient(scheme, cgwCR())
		f := &fakeVPNEC2{
			createCustomerGateway: func(p *awsec2.CreateCustomerGatewayInput) (*awsec2.CreateCustomerGatewayOutput, error) {
				if aws.ToString(p.IpAddress) != "203.0.113.10" {
					return nil, fmt.Errorf("unexpected ip %q", aws.ToString(p.IpAddress))
				}
				return &awsec2.CreateCustomerGatewayOutput{
					CustomerGateway: &ec2types.CustomerGateway{
						CustomerGatewayId: aws.String("cgw-123"),
						State:             aws.String("available"),
					},
				}, nil
			},
		}
		r := &CustomerGatewayReconciler{Client: c, Scheme: scheme, EC2Client: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.CustomerGateway{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.CustomerGatewayID != "cgw-123" {
			t.Errorf("status.customerGatewayId = %q, want cgw-123", got.Status.CustomerGatewayID)
		}
	})

	t.Run("delete with finalizer calls AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newVPNScheme(t)
		obj := cgwCR(func(cg *awsv1alpha1.CustomerGateway) {
			cg.Finalizers = []string{awsv1alpha1.FinalizerName}
			cg.Status.CustomerGatewayID = "cgw-123"
		})
		c := newVPNFakeClient(scheme, obj)
		f := &fakeVPNEC2{
			deleteCustomerGateway: func(p *awsec2.DeleteCustomerGatewayInput) (*awsec2.DeleteCustomerGatewayOutput, error) {
				if aws.ToString(p.CustomerGatewayId) != "cgw-123" {
					return nil, fmt.Errorf("unexpected id %q", aws.ToString(p.CustomerGatewayId))
				}
				return &awsec2.DeleteCustomerGatewayOutput{}, nil
			},
		}
		r := &CustomerGatewayReconciler{Client: c, Scheme: scheme, EC2Client: f}
		if err := c.Delete(ctx, cgwCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCGWCalled {
			t.Error("expected DeleteCustomerGateway to be called")
		}
		got := &awsv1alpha1.CustomerGateway{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected CR gone, got err=%v", err)
		}
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newVPNScheme(t)
		obj := cgwCR(func(cg *awsv1alpha1.CustomerGateway) {
			cg.Finalizers = []string{awsv1alpha1.FinalizerName}
			cg.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			cg.Status.CustomerGatewayID = "cgw-123"
		})
		c := newVPNFakeClient(scheme, obj)
		f := &fakeVPNEC2{}
		r := &CustomerGatewayReconciler{Client: c, Scheme: scheme, EC2Client: f}
		if err := c.Delete(ctx, cgwCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCGWCalled {
			t.Error("DeleteCustomerGateway must not be called when abandoning")
		}
	})
}

func TestVPNGatewayReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-vgw", Namespace: "default"}}
	vgwCR := func(mutate ...func(*awsv1alpha1.VPNGateway)) *awsv1alpha1.VPNGateway {
		v := &awsv1alpha1.VPNGateway{
			ObjectMeta: metav1.ObjectMeta{Name: "my-vgw", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
			Spec: awsv1alpha1.VPNGatewaySpec{
				Type:   "ipsec.1",
				VPCRef: &awsv1alpha1.VPCResourceRef{ID: "vpc-123"},
			},
		}
		for _, m := range mutate {
			m(v)
		}
		return v
	}

	t.Run("create persists identifier and attaches VPC", func(t *testing.T) {
		ctx := context.Background()
		scheme := newVPNScheme(t)
		c := newVPNFakeClient(scheme, vgwCR())
		attached := false
		f := &fakeVPNEC2{
			createVpnGateway: func(_ *awsec2.CreateVpnGatewayInput) (*awsec2.CreateVpnGatewayOutput, error) {
				return &awsec2.CreateVpnGatewayOutput{
					VpnGateway: &ec2types.VpnGateway{
						VpnGatewayId: aws.String("vgw-123"),
						State:        ec2types.VpnStateAvailable,
					},
				}, nil
			},
			attachVpnGateway: func(p *awsec2.AttachVpnGatewayInput) (*awsec2.AttachVpnGatewayOutput, error) {
				if aws.ToString(p.VpcId) != "vpc-123" {
					return nil, fmt.Errorf("unexpected vpc %q", aws.ToString(p.VpcId))
				}
				attached = true
				return &awsec2.AttachVpnGatewayOutput{}, nil
			},
		}
		r := &VPNGatewayReconciler{Client: c, Scheme: scheme, EC2Client: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !attached {
			t.Error("expected AttachVpnGateway to be called")
		}
		got := &awsv1alpha1.VPNGateway{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.VPNGatewayID != "vgw-123" {
			t.Errorf("status.vpnGatewayId = %q, want vgw-123", got.Status.VPNGatewayID)
		}
		if got.Status.AttachedVPCID != "vpc-123" {
			t.Errorf("status.attachedVpcId = %q, want vpc-123", got.Status.AttachedVPCID)
		}
	})

	t.Run("delete detaches then deletes", func(t *testing.T) {
		ctx := context.Background()
		scheme := newVPNScheme(t)
		obj := vgwCR(func(v *awsv1alpha1.VPNGateway) {
			v.Finalizers = []string{awsv1alpha1.FinalizerName}
			v.Status.VPNGatewayID = "vgw-123"
		})
		c := newVPNFakeClient(scheme, obj)
		detached := false
		f := &fakeVPNEC2{
			describeVpnGateways: func(_ *awsec2.DescribeVpnGatewaysInput) (*awsec2.DescribeVpnGatewaysOutput, error) {
				return &awsec2.DescribeVpnGatewaysOutput{
					VpnGateways: []ec2types.VpnGateway{{
						VpnGatewayId: aws.String("vgw-123"),
						State:        ec2types.VpnStateAvailable,
						VpcAttachments: []ec2types.VpcAttachment{
							{VpcId: aws.String("vpc-123"), State: ec2types.AttachmentStatusAttached},
						},
					}},
				}, nil
			},
			detachVpnGateway: func(_ *awsec2.DetachVpnGatewayInput) (*awsec2.DetachVpnGatewayOutput, error) {
				detached = true
				return &awsec2.DetachVpnGatewayOutput{}, nil
			},
			deleteVpnGateway: func(_ *awsec2.DeleteVpnGatewayInput) (*awsec2.DeleteVpnGatewayOutput, error) {
				return &awsec2.DeleteVpnGatewayOutput{}, nil
			},
		}
		r := &VPNGatewayReconciler{Client: c, Scheme: scheme, EC2Client: f}
		if err := c.Delete(ctx, vgwCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !detached {
			t.Error("expected DetachVpnGateway before delete")
		}
		if !f.deleteVGWCalled {
			t.Error("expected DeleteVpnGateway to be called")
		}
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newVPNScheme(t)
		obj := vgwCR(func(v *awsv1alpha1.VPNGateway) {
			v.Finalizers = []string{awsv1alpha1.FinalizerName}
			v.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			v.Status.VPNGatewayID = "vgw-123"
		})
		c := newVPNFakeClient(scheme, obj)
		f := &fakeVPNEC2{}
		r := &VPNGatewayReconciler{Client: c, Scheme: scheme, EC2Client: f}
		if err := c.Delete(ctx, vgwCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteVGWCalled {
			t.Error("DeleteVpnGateway must not be called when abandoning")
		}
	})
}

func TestVPNConnectionReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-vpn", Namespace: "default"}}
	vpnCR := func(mutate ...func(*awsv1alpha1.VPNConnection)) *awsv1alpha1.VPNConnection {
		v := &awsv1alpha1.VPNConnection{
			ObjectMeta: metav1.ObjectMeta{Name: "my-vpn", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
			Spec: awsv1alpha1.VPNConnectionSpec{
				CustomerGatewayRef: awsv1alpha1.CustomerGatewayRef{ID: "cgw-123"},
				VPNGatewayRef:      &awsv1alpha1.VPNGatewayRef{ID: "vgw-123"},
				Type:               "ipsec.1",
				StaticRoutesOnly:   true,
			},
		}
		for _, m := range mutate {
			m(v)
		}
		return v
	}

	t.Run("create persists identifier, resolves PSK from Secret, polls", func(t *testing.T) {
		ctx := context.Background()
		scheme := newVPNScheme(t)
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "vpn-psk", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
			Data:       map[string][]byte{"psk": []byte("super-secret-key")},
		}
		obj := vpnCR(func(v *awsv1alpha1.VPNConnection) {
			v.Spec.TunnelOptions = []awsv1alpha1.VPNTunnelOptions{{
				InsideCIDR:      "169.254.10.0/30",
				PreSharedKeyRef: &awsv1alpha1.SecretRef{Name: "vpn-psk", Key: "psk"},
			}}
		})
		c := newVPNFakeClient(scheme, obj, secret)
		f := &fakeVPNEC2{
			createVpnConnection: func(p *awsec2.CreateVpnConnectionInput) (*awsec2.CreateVpnConnectionOutput, error) {
				if aws.ToString(p.CustomerGatewayId) != "cgw-123" {
					return nil, fmt.Errorf("unexpected cgw %q", aws.ToString(p.CustomerGatewayId))
				}
				if aws.ToString(p.VpnGatewayId) != "vgw-123" {
					return nil, fmt.Errorf("unexpected vgw %q", aws.ToString(p.VpnGatewayId))
				}
				if p.Options == nil || !aws.ToBool(p.Options.StaticRoutesOnly) {
					return nil, fmt.Errorf("expected staticRoutesOnly")
				}
				if len(p.Options.TunnelOptions) != 1 || aws.ToString(p.Options.TunnelOptions[0].PreSharedKey) != "super-secret-key" {
					return nil, fmt.Errorf("expected PSK from secret")
				}
				return &awsec2.CreateVpnConnectionOutput{
					VpnConnection: &ec2types.VpnConnection{
						VpnConnectionId: aws.String("vpn-123"),
						State:           ec2types.VpnStatePending,
					},
				}, nil
			},
		}
		r := &VPNConnectionReconciler{Client: c, Scheme: scheme, EC2Client: f}
		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if res != requeueVPNPolling {
			t.Errorf("result = %+v, want VPN polling requeue", res)
		}
		got := &awsv1alpha1.VPNConnection{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.VPNConnectionID != "vpn-123" {
			t.Errorf("status.vpnConnectionId = %q, want vpn-123", got.Status.VPNConnectionID)
		}
		// SECURITY: PSK must never leak into status or conditions.
		for _, cond := range got.Status.Conditions {
			if cond.Message != "" && cond.Message != "VPN connection is being created" {
				if containsPSK := (cond.Message == "super-secret-key"); containsPSK {
					t.Error("PSK leaked into condition message")
				}
			}
		}
	})

	t.Run("pending connection polls until available", func(t *testing.T) {
		ctx := context.Background()
		scheme := newVPNScheme(t)
		obj := vpnCR(func(v *awsv1alpha1.VPNConnection) {
			v.Finalizers = []string{awsv1alpha1.FinalizerName}
			v.Status.VPNConnectionID = "vpn-123"
		})
		c := newVPNFakeClient(scheme, obj)
		f := &fakeVPNEC2{
			describeVpnConnections: func(_ *awsec2.DescribeVpnConnectionsInput) (*awsec2.DescribeVpnConnectionsOutput, error) {
				return &awsec2.DescribeVpnConnectionsOutput{
					VpnConnections: []ec2types.VpnConnection{{
						VpnConnectionId: aws.String("vpn-123"),
						State:           ec2types.VpnStatePending,
					}},
				}, nil
			},
		}
		r := &VPNConnectionReconciler{Client: c, Scheme: scheme, EC2Client: f}
		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if res != requeueVPNPolling {
			t.Errorf("result = %+v, want VPN polling requeue", res)
		}
		if f.createVPNCalled {
			t.Error("CreateVpnConnection must not be called while pending")
		}
	})

	t.Run("available connection is Ready", func(t *testing.T) {
		ctx := context.Background()
		scheme := newVPNScheme(t)
		obj := vpnCR(func(v *awsv1alpha1.VPNConnection) {
			v.Finalizers = []string{awsv1alpha1.FinalizerName}
			v.Status.VPNConnectionID = "vpn-123"
		})
		c := newVPNFakeClient(scheme, obj)
		f := &fakeVPNEC2{
			describeVpnConnections: func(_ *awsec2.DescribeVpnConnectionsInput) (*awsec2.DescribeVpnConnectionsOutput, error) {
				return &awsec2.DescribeVpnConnectionsOutput{
					VpnConnections: []ec2types.VpnConnection{{
						VpnConnectionId: aws.String("vpn-123"),
						State:           ec2types.VpnStateAvailable,
					}},
				}, nil
			},
		}
		r := &VPNConnectionReconciler{Client: c, Scheme: scheme, EC2Client: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.VPNConnection{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.State != "available" {
			t.Errorf("status.state = %q, want available", got.Status.State)
		}
	})

	t.Run("delete with finalizer calls AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newVPNScheme(t)
		obj := vpnCR(func(v *awsv1alpha1.VPNConnection) {
			v.Finalizers = []string{awsv1alpha1.FinalizerName}
			v.Status.VPNConnectionID = "vpn-123"
		})
		c := newVPNFakeClient(scheme, obj)
		f := &fakeVPNEC2{
			deleteVpnConnection: func(_ *awsec2.DeleteVpnConnectionInput) (*awsec2.DeleteVpnConnectionOutput, error) {
				return &awsec2.DeleteVpnConnectionOutput{}, nil
			},
		}
		r := &VPNConnectionReconciler{Client: c, Scheme: scheme, EC2Client: f}
		if err := c.Delete(ctx, vpnCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteVPNCalled {
			t.Error("expected DeleteVpnConnection to be called")
		}
		if aws.ToString(f.deleteVPNInput.VpnConnectionId) != "vpn-123" {
			t.Errorf("DeleteVpnConnection id = %q, want vpn-123", aws.ToString(f.deleteVPNInput.VpnConnectionId))
		}
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newVPNScheme(t)
		obj := vpnCR(func(v *awsv1alpha1.VPNConnection) {
			v.Finalizers = []string{awsv1alpha1.FinalizerName}
			v.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			v.Status.VPNConnectionID = "vpn-123"
		})
		c := newVPNFakeClient(scheme, obj)
		f := &fakeVPNEC2{}
		r := &VPNConnectionReconciler{Client: c, Scheme: scheme, EC2Client: f}
		if err := c.Delete(ctx, vpnCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteVPNCalled {
			t.Error("DeleteVpnConnection must not be called when abandoning")
		}
	})

	t.Run("waits for CustomerGateway dependency", func(t *testing.T) {
		ctx := context.Background()
		scheme := newVPNScheme(t)
		obj := vpnCR(func(v *awsv1alpha1.VPNConnection) {
			v.Spec.CustomerGatewayRef = awsv1alpha1.CustomerGatewayRef{Name: "my-cgw"}
		})
		cgw := &awsv1alpha1.CustomerGateway{
			ObjectMeta: metav1.ObjectMeta{Name: "my-cgw", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
			Spec:       awsv1alpha1.CustomerGatewaySpec{BGPASN: 65000, IPAddress: "203.0.113.10"},
		}
		c := newVPNFakeClient(scheme, obj, cgw)
		f := &fakeVPNEC2{}
		r := &VPNConnectionReconciler{Client: c, Scheme: scheme, EC2Client: f}
		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if res != requeueDependency {
			t.Errorf("result = %+v, want requeueDependency", res)
		}
		if f.createVPNCalled {
			t.Error("CreateVpnConnection must not be called while dependency is not ready")
		}
	})
}

func TestVPNConnectionRouteReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-route", Namespace: "default"}}
	routeCR := func(mutate ...func(*awsv1alpha1.VPNConnectionRoute)) *awsv1alpha1.VPNConnectionRoute {
		rt := &awsv1alpha1.VPNConnectionRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "my-route", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
			Spec: awsv1alpha1.VPNConnectionRouteSpec{
				VPNConnectionRef:     awsv1alpha1.VPNConnectionRef{ID: "vpn-123"},
				DestinationCIDRBlock: "10.100.0.0/16",
			},
		}
		for _, m := range mutate {
			m(rt)
		}
		return rt
	}

	t.Run("create calls AWS and persists connection ID", func(t *testing.T) {
		ctx := context.Background()
		scheme := newVPNScheme(t)
		c := newVPNFakeClient(scheme, routeCR())
		f := &fakeVPNEC2{
			createVpnConnectionRoute: func(p *awsec2.CreateVpnConnectionRouteInput) (*awsec2.CreateVpnConnectionRouteOutput, error) {
				if aws.ToString(p.DestinationCidrBlock) != "10.100.0.0/16" {
					return nil, fmt.Errorf("unexpected cidr %q", aws.ToString(p.DestinationCidrBlock))
				}
				return &awsec2.CreateVpnConnectionRouteOutput{}, nil
			},
		}
		r := &VPNConnectionRouteReconciler{Client: c, Scheme: scheme, EC2Client: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.createRouteCalled {
			t.Error("expected CreateVpnConnectionRoute to be called")
		}
		got := &awsv1alpha1.VPNConnectionRoute{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.VPNConnectionID != "vpn-123" {
			t.Errorf("status.vpnConnectionId = %q, want vpn-123", got.Status.VPNConnectionID)
		}
	})

	t.Run("delete with finalizer calls AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newVPNScheme(t)
		obj := routeCR(func(rt *awsv1alpha1.VPNConnectionRoute) {
			rt.Finalizers = []string{awsv1alpha1.FinalizerName}
			rt.Status.VPNConnectionID = "vpn-123"
		})
		c := newVPNFakeClient(scheme, obj)
		f := &fakeVPNEC2{
			deleteVpnConnectionRoute: func(_ *awsec2.DeleteVpnConnectionRouteInput) (*awsec2.DeleteVpnConnectionRouteOutput, error) {
				return &awsec2.DeleteVpnConnectionRouteOutput{}, nil
			},
		}
		r := &VPNConnectionRouteReconciler{Client: c, Scheme: scheme, EC2Client: f}
		if err := c.Delete(ctx, routeCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteRouteCalled {
			t.Error("expected DeleteVpnConnectionRoute to be called")
		}
		if aws.ToString(f.deleteRouteInput.VpnConnectionId) != "vpn-123" {
			t.Errorf("delete route vpn id = %q, want vpn-123", aws.ToString(f.deleteRouteInput.VpnConnectionId))
		}
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newVPNScheme(t)
		obj := routeCR(func(rt *awsv1alpha1.VPNConnectionRoute) {
			rt.Finalizers = []string{awsv1alpha1.FinalizerName}
			rt.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			rt.Status.VPNConnectionID = "vpn-123"
		})
		c := newVPNFakeClient(scheme, obj)
		f := &fakeVPNEC2{}
		r := &VPNConnectionRouteReconciler{Client: c, Scheme: scheme, EC2Client: f}
		if err := c.Delete(ctx, routeCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteRouteCalled {
			t.Error("DeleteVpnConnectionRoute must not be called when abandoning")
		}
	})
}

func TestManagedPrefixListReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-pl", Namespace: "default"}}
	plCR := func(mutate ...func(*awsv1alpha1.ManagedPrefixList)) *awsv1alpha1.ManagedPrefixList {
		pl := &awsv1alpha1.ManagedPrefixList{
			ObjectMeta: metav1.ObjectMeta{Name: "my-pl", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
			Spec: awsv1alpha1.ManagedPrefixListSpec{
				Name:          "my-pl",
				AddressFamily: "IPv4",
				MaxEntries:    10,
				Entries: []awsv1alpha1.PrefixListEntry{
					{CIDR: "10.0.0.0/16", Description: "vpc"},
				},
			},
		}
		for _, m := range mutate {
			m(pl)
		}
		return pl
	}

	t.Run("create persists identifier and version", func(t *testing.T) {
		ctx := context.Background()
		scheme := newVPNScheme(t)
		c := newVPNFakeClient(scheme, plCR())
		f := &fakeVPNEC2{
			createManagedPrefixList: func(p *awsec2.CreateManagedPrefixListInput) (*awsec2.CreateManagedPrefixListOutput, error) {
				if len(p.Entries) != 1 || aws.ToString(p.Entries[0].Cidr) != "10.0.0.0/16" {
					return nil, fmt.Errorf("unexpected entries %+v", p.Entries)
				}
				return &awsec2.CreateManagedPrefixListOutput{
					PrefixList: &ec2types.ManagedPrefixList{
						PrefixListId:  aws.String("pl-123"),
						PrefixListArn: aws.String("arn:pl"),
						Version:       aws.Int64(1),
						State:         ec2types.PrefixListStateCreateComplete,
					},
				}, nil
			},
		}
		r := &ManagedPrefixListReconciler{Client: c, Scheme: scheme, EC2Client: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.ManagedPrefixList{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.PrefixListID != "pl-123" {
			t.Errorf("status.prefixListId = %q, want pl-123", got.Status.PrefixListID)
		}
		if got.Status.Version != 1 {
			t.Errorf("status.version = %d, want 1", got.Status.Version)
		}
	})

	t.Run("modify diffs entries with version token", func(t *testing.T) {
		ctx := context.Background()
		scheme := newVPNScheme(t)
		obj := plCR(func(pl *awsv1alpha1.ManagedPrefixList) {
			pl.Finalizers = []string{awsv1alpha1.FinalizerName}
			pl.Generation = 2
			pl.Status.PrefixListID = "pl-123"
			pl.Status.ObservedGeneration = 1
			pl.Spec.Entries = []awsv1alpha1.PrefixListEntry{
				{CIDR: "10.0.0.0/16", Description: "vpc"},
				{CIDR: "192.168.0.0/24", Description: "office"},
			}
		})
		c := newVPNFakeClient(scheme, obj)
		f := &fakeVPNEC2{
			describeManagedPrefixLists: func(_ *awsec2.DescribeManagedPrefixListsInput) (*awsec2.DescribeManagedPrefixListsOutput, error) {
				return &awsec2.DescribeManagedPrefixListsOutput{
					PrefixLists: []ec2types.ManagedPrefixList{{
						PrefixListId:   aws.String("pl-123"),
						PrefixListName: aws.String("my-pl"),
						MaxEntries:     aws.Int32(10),
						Version:        aws.Int64(3),
						State:          ec2types.PrefixListStateCreateComplete,
					}},
				}, nil
			},
			getManagedPrefixListEntries: func(_ *awsec2.GetManagedPrefixListEntriesInput) (*awsec2.GetManagedPrefixListEntriesOutput, error) {
				return &awsec2.GetManagedPrefixListEntriesOutput{
					Entries: []ec2types.PrefixListEntry{
						{Cidr: aws.String("10.0.0.0/16"), Description: aws.String("vpc")},
						{Cidr: aws.String("172.16.0.0/12"), Description: aws.String("stale")},
					},
				}, nil
			},
			modifyManagedPrefixList: func(p *awsec2.ModifyManagedPrefixListInput) (*awsec2.ModifyManagedPrefixListOutput, error) {
				if aws.ToInt64(p.CurrentVersion) != 3 {
					return nil, fmt.Errorf("expected version token 3, got %d", aws.ToInt64(p.CurrentVersion))
				}
				return &awsec2.ModifyManagedPrefixListOutput{
					PrefixList: &ec2types.ManagedPrefixList{
						PrefixListId: aws.String("pl-123"),
						Version:      aws.Int64(4),
						State:        ec2types.PrefixListStateModifyComplete,
					},
				}, nil
			},
		}
		r := &ManagedPrefixListReconciler{Client: c, Scheme: scheme, EC2Client: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.modifyMPLCalled {
			t.Fatal("expected ModifyManagedPrefixList to be called")
		}
		if len(f.modifyMPLInput.AddEntries) != 1 || aws.ToString(f.modifyMPLInput.AddEntries[0].Cidr) != "192.168.0.0/24" {
			t.Errorf("addEntries = %+v, want 192.168.0.0/24", f.modifyMPLInput.AddEntries)
		}
		if len(f.modifyMPLInput.RemoveEntries) != 1 || aws.ToString(f.modifyMPLInput.RemoveEntries[0].Cidr) != "172.16.0.0/12" {
			t.Errorf("removeEntries = %+v, want 172.16.0.0/12", f.modifyMPLInput.RemoveEntries)
		}
	})

	t.Run("delete with finalizer calls AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newVPNScheme(t)
		obj := plCR(func(pl *awsv1alpha1.ManagedPrefixList) {
			pl.Finalizers = []string{awsv1alpha1.FinalizerName}
			pl.Status.PrefixListID = "pl-123"
		})
		c := newVPNFakeClient(scheme, obj)
		f := &fakeVPNEC2{
			deleteManagedPrefixList: func(p *awsec2.DeleteManagedPrefixListInput) (*awsec2.DeleteManagedPrefixListOutput, error) {
				if aws.ToString(p.PrefixListId) != "pl-123" {
					return nil, fmt.Errorf("unexpected id %q", aws.ToString(p.PrefixListId))
				}
				return &awsec2.DeleteManagedPrefixListOutput{}, nil
			},
		}
		r := &ManagedPrefixListReconciler{Client: c, Scheme: scheme, EC2Client: f}
		if err := c.Delete(ctx, plCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteMPLCalled {
			t.Error("expected DeleteManagedPrefixList to be called")
		}
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newVPNScheme(t)
		obj := plCR(func(pl *awsv1alpha1.ManagedPrefixList) {
			pl.Finalizers = []string{awsv1alpha1.FinalizerName}
			pl.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			pl.Status.PrefixListID = "pl-123"
		})
		c := newVPNFakeClient(scheme, obj)
		f := &fakeVPNEC2{}
		r := &ManagedPrefixListReconciler{Client: c, Scheme: scheme, EC2Client: f}
		if err := c.Delete(ctx, plCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteMPLCalled {
			t.Error("DeleteManagedPrefixList must not be called when abandoning")
		}
	})
}

func TestCapacityReservationReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-cr", Namespace: "default"}}
	crCR := func(mutate ...func(*awsv1alpha1.CapacityReservation)) *awsv1alpha1.CapacityReservation {
		cr := &awsv1alpha1.CapacityReservation{
			ObjectMeta: metav1.ObjectMeta{Name: "my-cr", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
			Spec: awsv1alpha1.CapacityReservationSpec{
				InstanceType:     "m5.large",
				InstancePlatform: "Linux/UNIX",
				AvailabilityZone: "us-east-1a",
				InstanceCount:    2,
				Tenancy:          "default",
			},
		}
		for _, m := range mutate {
			m(cr)
		}
		return cr
	}

	t.Run("create persists identifier", func(t *testing.T) {
		ctx := context.Background()
		scheme := newVPNScheme(t)
		c := newVPNFakeClient(scheme, crCR())
		f := &fakeVPNEC2{
			createCapacityReservation: func(p *awsec2.CreateCapacityReservationInput) (*awsec2.CreateCapacityReservationOutput, error) {
				if aws.ToString(p.InstanceType) != "m5.large" || aws.ToInt32(p.InstanceCount) != 2 {
					return nil, fmt.Errorf("unexpected input %+v", p)
				}
				return &awsec2.CreateCapacityReservationOutput{
					CapacityReservation: &ec2types.CapacityReservation{
						CapacityReservationId:  aws.String("cr-123"),
						CapacityReservationArn: aws.String("arn:cr"),
						State:                  ec2types.CapacityReservationStateActive,
					},
				}, nil
			},
		}
		r := &CapacityReservationReconciler{Client: c, Scheme: scheme, EC2Client: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.CapacityReservation{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.CapacityReservationID != "cr-123" {
			t.Errorf("status.capacityReservationId = %q, want cr-123", got.Status.CapacityReservationID)
		}
	})

	t.Run("delete cancels the reservation", func(t *testing.T) {
		ctx := context.Background()
		scheme := newVPNScheme(t)
		obj := crCR(func(cr *awsv1alpha1.CapacityReservation) {
			cr.Finalizers = []string{awsv1alpha1.FinalizerName}
			cr.Status.CapacityReservationID = "cr-123"
		})
		c := newVPNFakeClient(scheme, obj)
		f := &fakeVPNEC2{
			cancelCapacityReservation: func(p *awsec2.CancelCapacityReservationInput) (*awsec2.CancelCapacityReservationOutput, error) {
				if aws.ToString(p.CapacityReservationId) != "cr-123" {
					return nil, fmt.Errorf("unexpected id %q", aws.ToString(p.CapacityReservationId))
				}
				return &awsec2.CancelCapacityReservationOutput{}, nil
			},
		}
		r := &CapacityReservationReconciler{Client: c, Scheme: scheme, EC2Client: f}
		if err := c.Delete(ctx, crCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.cancelCapCalled {
			t.Error("expected CancelCapacityReservation to be called")
		}
	})

	t.Run("abandon skips cancel", func(t *testing.T) {
		ctx := context.Background()
		scheme := newVPNScheme(t)
		obj := crCR(func(cr *awsv1alpha1.CapacityReservation) {
			cr.Finalizers = []string{awsv1alpha1.FinalizerName}
			cr.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			cr.Status.CapacityReservationID = "cr-123"
		})
		c := newVPNFakeClient(scheme, obj)
		f := &fakeVPNEC2{}
		r := &CapacityReservationReconciler{Client: c, Scheme: scheme, EC2Client: f}
		if err := c.Delete(ctx, crCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.cancelCapCalled {
			t.Error("CancelCapacityReservation must not be called when abandoning")
		}
	})
}
