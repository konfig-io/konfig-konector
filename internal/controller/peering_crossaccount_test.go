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
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	"github.com/konfig-io/konfig-konector/internal/aws/provider"
)

type fakeEC2Peering struct {
	pcx         *ec2types.VpcPeeringConnection
	acceptScope *provider.Scope
	accepted    int
}

func (f *fakeEC2Peering) CreateVpcPeeringConnection(_ context.Context, in *awsec2.CreateVpcPeeringConnectionInput, _ ...func(*awsec2.Options)) (*awsec2.CreateVpcPeeringConnectionOutput, error) {
	f.pcx = &ec2types.VpcPeeringConnection{
		VpcPeeringConnectionId: aws.String("pcx-1"),
		Status:                 &ec2types.VpcPeeringConnectionStateReason{Code: ec2types.VpcPeeringConnectionStateReasonCodePendingAcceptance},
		RequesterVpcInfo:       &ec2types.VpcPeeringConnectionVpcInfo{VpcId: in.VpcId},
		AccepterVpcInfo:        &ec2types.VpcPeeringConnectionVpcInfo{VpcId: in.PeerVpcId, OwnerId: in.PeerOwnerId},
	}
	return &awsec2.CreateVpcPeeringConnectionOutput{VpcPeeringConnection: f.pcx}, nil
}
func (f *fakeEC2Peering) DescribeVpcPeeringConnections(context.Context, *awsec2.DescribeVpcPeeringConnectionsInput, ...func(*awsec2.Options)) (*awsec2.DescribeVpcPeeringConnectionsOutput, error) {
	if f.pcx == nil {
		return &awsec2.DescribeVpcPeeringConnectionsOutput{}, nil
	}
	return &awsec2.DescribeVpcPeeringConnectionsOutput{VpcPeeringConnections: []ec2types.VpcPeeringConnection{*f.pcx}}, nil
}
func (f *fakeEC2Peering) AcceptVpcPeeringConnection(ctx context.Context, _ *awsec2.AcceptVpcPeeringConnectionInput, _ ...func(*awsec2.Options)) (*awsec2.AcceptVpcPeeringConnectionOutput, error) {
	f.accepted++
	f.acceptScope = provider.ScopeFrom(ctx)
	f.pcx.Status.Code = ec2types.VpcPeeringConnectionStateReasonCodeActive
	return &awsec2.AcceptVpcPeeringConnectionOutput{}, nil
}
func (f *fakeEC2Peering) DeleteVpcPeeringConnection(context.Context, *awsec2.DeleteVpcPeeringConnectionInput, ...func(*awsec2.Options)) (*awsec2.DeleteVpcPeeringConnectionOutput, error) {
	f.pcx = nil
	return &awsec2.DeleteVpcPeeringConnectionOutput{}, nil
}
func (f *fakeEC2Peering) CreateTags(context.Context, *awsec2.CreateTagsInput, ...func(*awsec2.Options)) (*awsec2.CreateTagsOutput, error) {
	return &awsec2.CreateTagsOutput{}, nil
}

func TestVPCPeeringCrossAccountAcceptUnderAccepterProvider(t *testing.T) {
	s := crossAccountScheme(t)
	prov := &awsv1alpha1.AWSProvider{ObjectMeta: metav1.ObjectMeta{Name: "acct-b"}, Spec: awsv1alpha1.AWSProviderSpec{Region: "us-east-1"}}
	obj := &awsv1alpha1.VPCPeeringConnection{
		ObjectMeta: metav1.ObjectMeta{Name: "peer", Namespace: "ns", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec: awsv1alpha1.VPCPeeringConnectionSpec{
			VPCRef: awsv1alpha1.VPCResourceRef{ID: "vpc-a"}, PeerVPCID: "vpc-b", PeerOwnerID: "222233334444", PeerRegion: "eu-west-1",
			AccepterProviderRef: &awsv1alpha1.ProviderRef{Name: "acct-b"},
		},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&awsv1alpha1.VPCPeeringConnection{}).WithObjects(obj, prov).Build()
	SetProviderResolver(&provider.Resolver{Client: c, BaseRegion: "us-east-1"})
	defer SetProviderResolver(nil)
	f := &fakeEC2Peering{}
	r := &VPCPeeringConnectionReconciler{Client: c, Scheme: s, EC2Client: f}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "peer", Namespace: "ns"}}); err != nil {
		t.Fatal(err)
	}
	if f.accepted != 1 || f.acceptScope == nil || f.acceptScope.Name != "acct-b" || f.acceptScope.Region != "eu-west-1" {
		t.Fatalf("accept must run under accepter provider in the peer region, got n=%d scope=%+v", f.accepted, f.acceptScope)
	}
	got := &awsv1alpha1.VPCPeeringConnection{}
	_ = c.Get(context.Background(), k8stypes.NamespacedName{Name: "peer", Namespace: "ns"}, got)
	if got.Status.Status != "active" || got.Status.AccepterAccountID != "222233334444" || got.Status.PeeringID != "pcx-1" {
		t.Fatalf("bad status %+v", got.Status)
	}
	if cnd := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady); cnd == nil || cnd.Status != metav1.ConditionTrue {
		t.Fatalf("expected Ready=True once active, got %+v", cnd)
	}
}

func TestVPCPeeringPendingWithoutAccepter(t *testing.T) {
	s := crossAccountScheme(t)
	obj := &awsv1alpha1.VPCPeeringConnection{
		ObjectMeta: metav1.ObjectMeta{Name: "peer", Namespace: "ns", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec:       awsv1alpha1.VPCPeeringConnectionSpec{VPCRef: awsv1alpha1.VPCResourceRef{ID: "vpc-a"}, PeerVPCID: "vpc-b", PeerOwnerID: "222233334444"},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&awsv1alpha1.VPCPeeringConnection{}).WithObjects(obj).Build()
	f := &fakeEC2Peering{}
	r := &VPCPeeringConnectionReconciler{Client: c, Scheme: s, EC2Client: f}
	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "peer", Namespace: "ns"}})
	if err != nil || res != requeuePending || f.accepted != 0 {
		t.Fatalf("expected pending requeue with no accept, got %+v %v accepted=%d", res, err, f.accepted)
	}
	got := &awsv1alpha1.VPCPeeringConnection{}
	_ = c.Get(context.Background(), k8stypes.NamespacedName{Name: "peer", Namespace: "ns"}, got)
	if got.Status.PeeringID != "pcx-1" {
		t.Fatal("peering ID must be persisted even while pending")
	}
	if cnd := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady); cnd == nil || cnd.Reason != awsv1alpha1.ReasonPendingAcceptance {
		t.Fatalf("expected PendingAcceptance, got %+v", cnd)
	}
}

type fakeEC2TGWAtt struct {
	att         *ec2types.TransitGatewayVpcAttachment
	acceptScope *provider.Scope
	accepted    int
}

func (f *fakeEC2TGWAtt) CreateTransitGatewayVpcAttachment(_ context.Context, in *awsec2.CreateTransitGatewayVpcAttachmentInput, _ ...func(*awsec2.Options)) (*awsec2.CreateTransitGatewayVpcAttachmentOutput, error) {
	f.att = &ec2types.TransitGatewayVpcAttachment{TransitGatewayAttachmentId: aws.String("tgw-attach-1"), State: ec2types.TransitGatewayAttachmentStatePendingAcceptance}
	return &awsec2.CreateTransitGatewayVpcAttachmentOutput{TransitGatewayVpcAttachment: f.att}, nil
}
func (f *fakeEC2TGWAtt) DescribeTransitGatewayVpcAttachments(context.Context, *awsec2.DescribeTransitGatewayVpcAttachmentsInput, ...func(*awsec2.Options)) (*awsec2.DescribeTransitGatewayVpcAttachmentsOutput, error) {
	if f.att == nil {
		return &awsec2.DescribeTransitGatewayVpcAttachmentsOutput{}, nil
	}
	return &awsec2.DescribeTransitGatewayVpcAttachmentsOutput{TransitGatewayVpcAttachments: []ec2types.TransitGatewayVpcAttachment{*f.att}}, nil
}
func (f *fakeEC2TGWAtt) AcceptTransitGatewayVpcAttachment(ctx context.Context, _ *awsec2.AcceptTransitGatewayVpcAttachmentInput, _ ...func(*awsec2.Options)) (*awsec2.AcceptTransitGatewayVpcAttachmentOutput, error) {
	f.accepted++
	f.acceptScope = provider.ScopeFrom(ctx)
	f.att.State = ec2types.TransitGatewayAttachmentStateAvailable
	return &awsec2.AcceptTransitGatewayVpcAttachmentOutput{}, nil
}
func (f *fakeEC2TGWAtt) DeleteTransitGatewayVpcAttachment(context.Context, *awsec2.DeleteTransitGatewayVpcAttachmentInput, ...func(*awsec2.Options)) (*awsec2.DeleteTransitGatewayVpcAttachmentOutput, error) {
	f.att = nil
	return &awsec2.DeleteTransitGatewayVpcAttachmentOutput{}, nil
}
func (f *fakeEC2TGWAtt) CreateTags(context.Context, *awsec2.CreateTagsInput, ...func(*awsec2.Options)) (*awsec2.CreateTagsOutput, error) {
	return &awsec2.CreateTagsOutput{}, nil
}

func TestTGWAttachmentAcceptedByOwnerProvider(t *testing.T) {
	s := crossAccountScheme(t)
	prov := &awsv1alpha1.AWSProvider{ObjectMeta: metav1.ObjectMeta{Name: "network-hub"}}
	obj := &awsv1alpha1.TransitGatewayVpcAttachment{
		ObjectMeta: metav1.ObjectMeta{Name: "att", Namespace: "ns", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec: awsv1alpha1.TransitGatewayVpcAttachmentSpec{
			TransitGatewayRef:   awsv1alpha1.TransitGatewayRef{TransitGatewayID: "tgw-shared"},
			VPCRef:              awsv1alpha1.VPCResourceRef{ID: "vpc-spoke"},
			SubnetRefs:          []awsv1alpha1.SubnetRef{{ID: "subnet-1"}},
			AccepterProviderRef: &awsv1alpha1.ProviderRef{Name: "network-hub"},
		},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&awsv1alpha1.TransitGatewayVpcAttachment{}).WithObjects(obj, prov).Build()
	SetProviderResolver(&provider.Resolver{Client: c, BaseRegion: "us-east-1"})
	defer SetProviderResolver(nil)
	f := &fakeEC2TGWAtt{}
	r := &TransitGatewayVpcAttachmentReconciler{Client: c, Scheme: s, EC2Client: f}
	key := k8stypes.NamespacedName{Name: "att", Namespace: "ns"}
	// 1st reconcile creates (async) and reports not ready.
	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key})
	if err != nil || res != requeuePending {
		t.Fatalf("expected pending after create, got %+v %v", res, err)
	}
	// 2nd reconcile sees pendingAcceptance and accepts under the hub provider.
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatal(err)
	}
	if f.accepted != 1 || f.acceptScope == nil || f.acceptScope.Name != "network-hub" {
		t.Fatalf("expected accept under network-hub provider, got n=%d scope=%+v", f.accepted, f.acceptScope)
	}
	// 3rd reconcile: available → Ready.
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatal(err)
	}
	got := &awsv1alpha1.TransitGatewayVpcAttachment{}
	_ = c.Get(context.Background(), key, got)
	if cnd := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady); cnd == nil || cnd.Status != metav1.ConditionTrue || got.Status.State != "available" {
		t.Fatalf("expected Ready/available, got %+v state=%s", cnd, got.Status.State)
	}
}
