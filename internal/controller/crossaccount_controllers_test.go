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
	awsram "github.com/aws/aws-sdk-go-v2/service/ram"
	ramtypes "github.com/aws/aws-sdk-go-v2/service/ram/types"
	awsroute53 "github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	"github.com/konfig-io/konfig-konector/internal/aws/provider"
)

func crossAccountScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

func crossAccountClient(s *runtime.Scheme, objs ...client.Object) client.Client {
	return fake.NewClientBuilder().WithScheme(s).
		WithStatusSubresource(&awsv1alpha1.ResourceShareInvitation{}, &awsv1alpha1.HostedZoneVPCAssociation{}, &awsv1alpha1.VPCEndpointService{}, &awsv1alpha1.AWSProvider{}).
		WithObjects(objs...).Build()
}

// ---- ResourceShareInvitation -------------------------------------------------

type fakeRAMInv struct {
	invitations []ramtypes.ResourceShareInvitation
	resources   []ramtypes.Resource
	accepted    []string
}

func (f *fakeRAMInv) GetResourceShareInvitations(context.Context, *awsram.GetResourceShareInvitationsInput, ...func(*awsram.Options)) (*awsram.GetResourceShareInvitationsOutput, error) {
	return &awsram.GetResourceShareInvitationsOutput{ResourceShareInvitations: f.invitations}, nil
}
func (f *fakeRAMInv) AcceptResourceShareInvitation(_ context.Context, in *awsram.AcceptResourceShareInvitationInput, _ ...func(*awsram.Options)) (*awsram.AcceptResourceShareInvitationOutput, error) {
	f.accepted = append(f.accepted, aws.ToString(in.ResourceShareInvitationArn))
	return &awsram.AcceptResourceShareInvitationOutput{ResourceShareInvitation: &ramtypes.ResourceShareInvitation{Status: ramtypes.ResourceShareInvitationStatusAccepted}}, nil
}
func (f *fakeRAMInv) ListResources(context.Context, *awsram.ListResourcesInput, ...func(*awsram.Options)) (*awsram.ListResourcesOutput, error) {
	return &awsram.ListResourcesOutput{Resources: f.resources}, nil
}

func TestResourceShareInvitationAcceptsPending(t *testing.T) {
	s := crossAccountScheme(t)
	obj := &awsv1alpha1.ResourceShareInvitation{
		ObjectMeta: metav1.ObjectMeta{Name: "inv", Namespace: "ns", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec:       awsv1alpha1.ResourceShareInvitationSpec{ResourceShareRef: awsv1alpha1.ResourceShareRef{ARN: "arn:aws:ram:us-east-1:111122223333:resource-share/abc"}},
	}
	f := &fakeRAMInv{
		invitations: []ramtypes.ResourceShareInvitation{{
			ResourceShareInvitationArn: aws.String("arn:inv"), ResourceShareArn: aws.String("arn:aws:ram:us-east-1:111122223333:resource-share/abc"),
			SenderAccountId: aws.String("111122223333"), Status: ramtypes.ResourceShareInvitationStatusPending,
		}},
		resources: []ramtypes.Resource{{Arn: aws.String("arn:aws:ec2:us-east-1:111122223333:transit-gateway/tgw-1")}},
	}
	r := &ResourceShareInvitationReconciler{Client: crossAccountClient(s, obj), Scheme: s, RAMClient: f}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "inv", Namespace: "ns"}}); err != nil {
		t.Fatal(err)
	}
	if len(f.accepted) != 1 || f.accepted[0] != "arn:inv" {
		t.Fatalf("expected invitation accepted, got %v", f.accepted)
	}
	got := &awsv1alpha1.ResourceShareInvitation{}
	_ = r.Get(context.Background(), k8stypes.NamespacedName{Name: "inv", Namespace: "ns"}, got)
	if got.Status.Status != "ACCEPTED" || got.Status.SenderAccountID != "111122223333" || len(got.Status.ResourceARNs) != 1 {
		t.Fatalf("bad status %+v", got.Status)
	}
	if c := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady); c == nil || c.Status != metav1.ConditionTrue {
		t.Fatalf("expected Ready=True, got %+v", c)
	}
}

func TestResourceShareInvitationWaitsWhenNoneSent(t *testing.T) {
	s := crossAccountScheme(t)
	obj := &awsv1alpha1.ResourceShareInvitation{
		ObjectMeta: metav1.ObjectMeta{Name: "inv", Namespace: "ns", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec:       awsv1alpha1.ResourceShareInvitationSpec{ResourceShareRef: awsv1alpha1.ResourceShareRef{ARN: "arn:share"}},
	}
	f := &fakeRAMInv{}
	r := &ResourceShareInvitationReconciler{Client: crossAccountClient(s, obj), Scheme: s, RAMClient: f}
	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "inv", Namespace: "ns"}})
	if err != nil {
		t.Fatal(err)
	}
	if res != requeuePending {
		t.Fatalf("expected pending requeue, got %+v", res)
	}
	got := &awsv1alpha1.ResourceShareInvitation{}
	_ = r.Get(context.Background(), k8stypes.NamespacedName{Name: "inv", Namespace: "ns"}, got)
	c := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
	if c == nil || c.Status != metav1.ConditionFalse || c.Reason != awsv1alpha1.ReasonPendingAcceptance {
		t.Fatalf("expected PendingAcceptance, got %+v", c)
	}
}

func TestResourceShareInvitationResolvesShareCR(t *testing.T) {
	s := crossAccountScheme(t)
	share := &awsv1alpha1.ResourceShare{ObjectMeta: metav1.ObjectMeta{Name: "tgw-share", Namespace: "hub"}}
	share.Status.ARN = "arn:share"
	obj := &awsv1alpha1.ResourceShareInvitation{
		ObjectMeta: metav1.ObjectMeta{Name: "inv", Namespace: "spoke", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec:       awsv1alpha1.ResourceShareInvitationSpec{ResourceShareRef: awsv1alpha1.ResourceShareRef{Name: "tgw-share", Namespace: "hub"}},
	}
	f := &fakeRAMInv{resources: []ramtypes.Resource{{Arn: aws.String("arn:res")}}}
	r := &ResourceShareInvitationReconciler{Client: crossAccountClient(s, obj, share), Scheme: s, RAMClient: f}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "inv", Namespace: "spoke"}}); err != nil {
		t.Fatal(err)
	}
	got := &awsv1alpha1.ResourceShareInvitation{}
	_ = r.Get(context.Background(), k8stypes.NamespacedName{Name: "inv", Namespace: "spoke"}, got)
	if got.Status.ResourceShareARN != "arn:share" || got.Status.Status != "ACCEPTED" {
		t.Fatalf("expected org-shared (no invitation) share to be Ready, got %+v", got.Status)
	}
}

// ---- HostedZoneVPCAssociation ------------------------------------------------

type fakeR53Assoc struct {
	vpcs          []route53types.VPC
	authCreated   int
	authDeleted   int
	associated    []string
	disassociated []string
	assocScopes   []*provider.Scope
}

func (f *fakeR53Assoc) GetHostedZone(context.Context, *awsroute53.GetHostedZoneInput, ...func(*awsroute53.Options)) (*awsroute53.GetHostedZoneOutput, error) {
	return &awsroute53.GetHostedZoneOutput{HostedZone: &route53types.HostedZone{}, VPCs: f.vpcs}, nil
}
func (f *fakeR53Assoc) CreateVPCAssociationAuthorization(context.Context, *awsroute53.CreateVPCAssociationAuthorizationInput, ...func(*awsroute53.Options)) (*awsroute53.CreateVPCAssociationAuthorizationOutput, error) {
	f.authCreated++
	return &awsroute53.CreateVPCAssociationAuthorizationOutput{}, nil
}
func (f *fakeR53Assoc) DeleteVPCAssociationAuthorization(context.Context, *awsroute53.DeleteVPCAssociationAuthorizationInput, ...func(*awsroute53.Options)) (*awsroute53.DeleteVPCAssociationAuthorizationOutput, error) {
	f.authDeleted++
	return &awsroute53.DeleteVPCAssociationAuthorizationOutput{}, nil
}
func (f *fakeR53Assoc) AssociateVPCWithHostedZone(ctx context.Context, in *awsroute53.AssociateVPCWithHostedZoneInput, _ ...func(*awsroute53.Options)) (*awsroute53.AssociateVPCWithHostedZoneOutput, error) {
	f.associated = append(f.associated, aws.ToString(in.VPC.VPCId))
	f.assocScopes = append(f.assocScopes, provider.ScopeFrom(ctx))
	f.vpcs = append(f.vpcs, *in.VPC)
	return &awsroute53.AssociateVPCWithHostedZoneOutput{}, nil
}
func (f *fakeR53Assoc) DisassociateVPCFromHostedZone(_ context.Context, in *awsroute53.DisassociateVPCFromHostedZoneInput, _ ...func(*awsroute53.Options)) (*awsroute53.DisassociateVPCFromHostedZoneOutput, error) {
	f.disassociated = append(f.disassociated, aws.ToString(in.VPC.VPCId))
	return &awsroute53.DisassociateVPCFromHostedZoneOutput{}, nil
}

func TestHostedZoneVPCAssociationCrossAccountHandshake(t *testing.T) {
	s := crossAccountScheme(t)
	zone := &awsv1alpha1.HostedZone{ObjectMeta: metav1.ObjectMeta{Name: "internal", Namespace: "ns"}}
	zone.Status.HostedZoneID = "/hostedzone/Z123"
	prov := &awsv1alpha1.AWSProvider{ObjectMeta: metav1.ObjectMeta{Name: "spoke"}, Spec: awsv1alpha1.AWSProviderSpec{Region: "eu-west-1"}}
	obj := &awsv1alpha1.HostedZoneVPCAssociation{
		ObjectMeta: metav1.ObjectMeta{Name: "assoc", Namespace: "ns", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec: awsv1alpha1.HostedZoneVPCAssociationSpec{
			HostedZoneRef:  awsv1alpha1.HostedZoneIDRef{Name: "internal"},
			VPCRef:         awsv1alpha1.VPCResourceRef{ID: "vpc-spoke"},
			VPCRegion:      "eu-west-1",
			VPCProviderRef: &awsv1alpha1.ProviderRef{Name: "spoke"},
		},
	}
	c := crossAccountClient(s, obj, zone, prov)
	SetProviderResolver(&provider.Resolver{Client: c, BaseRegion: "us-east-1"})
	defer SetProviderResolver(nil)
	f := &fakeR53Assoc{}
	r := &HostedZoneVPCAssociationReconciler{Client: c, Scheme: s, Route53Client: f}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "assoc", Namespace: "ns"}}); err != nil {
		t.Fatal(err)
	}
	if f.authCreated != 1 || f.authDeleted != 1 || len(f.associated) != 1 {
		t.Fatalf("expected authorize→associate→cleanup, got auth=%d del=%d assoc=%v", f.authCreated, f.authDeleted, f.associated)
	}
	if f.assocScopes[0] == nil || f.assocScopes[0].Name != "spoke" {
		t.Fatalf("association must run under the VPC account's provider, got %+v", f.assocScopes[0])
	}
	got := &awsv1alpha1.HostedZoneVPCAssociation{}
	_ = c.Get(context.Background(), k8stypes.NamespacedName{Name: "assoc", Namespace: "ns"}, got)
	if !got.Status.Associated || got.Status.HostedZoneID != "Z123" {
		t.Fatalf("bad status %+v", got.Status)
	}

	// Steady state: already associated, no new calls.
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "assoc", Namespace: "ns"}}); err != nil {
		t.Fatal(err)
	}
	if f.authCreated != 1 || len(f.associated) != 1 {
		t.Fatalf("steady state must not re-associate")
	}

	// Delete disassociates.
	_ = c.Get(context.Background(), k8stypes.NamespacedName{Name: "assoc", Namespace: "ns"}, got)
	if err := c.Delete(context.Background(), got); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "assoc", Namespace: "ns"}}); err != nil {
		t.Fatal(err)
	}
	if len(f.disassociated) != 1 || f.disassociated[0] != "vpc-spoke" {
		t.Fatalf("expected disassociate, got %v", f.disassociated)
	}
}

func TestHostedZoneVPCAssociationSameAccountNoAuthorization(t *testing.T) {
	s := crossAccountScheme(t)
	obj := &awsv1alpha1.HostedZoneVPCAssociation{
		ObjectMeta: metav1.ObjectMeta{Name: "assoc", Namespace: "ns", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec: awsv1alpha1.HostedZoneVPCAssociationSpec{
			HostedZoneRef: awsv1alpha1.HostedZoneIDRef{ID: "Z1"}, VPCRef: awsv1alpha1.VPCResourceRef{ID: "vpc-1"}, VPCRegion: "us-east-1",
		},
	}
	f := &fakeR53Assoc{}
	r := &HostedZoneVPCAssociationReconciler{Client: crossAccountClient(s, obj), Scheme: s, Route53Client: f}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "assoc", Namespace: "ns"}}); err != nil {
		t.Fatal(err)
	}
	if f.authCreated != 0 || len(f.associated) != 1 {
		t.Fatalf("same-account must skip authorization: auth=%d assoc=%v", f.authCreated, f.associated)
	}
}

// ---- VPCEndpointService ------------------------------------------------------

type fakeEC2Svc struct {
	cfg         *ec2types.ServiceConfiguration
	principals  []string
	pendingConn []string
	accepted    []string
	permMods    int
}

func (f *fakeEC2Svc) CreateVpcEndpointServiceConfiguration(_ context.Context, in *awsec2.CreateVpcEndpointServiceConfigurationInput, _ ...func(*awsec2.Options)) (*awsec2.CreateVpcEndpointServiceConfigurationOutput, error) {
	f.cfg = &ec2types.ServiceConfiguration{
		ServiceId: aws.String("vpce-svc-1"), ServiceName: aws.String("com.amazonaws.vpce.us-east-1.vpce-svc-1"),
		ServiceState: ec2types.ServiceStateAvailable, NetworkLoadBalancerArns: in.NetworkLoadBalancerArns,
		AcceptanceRequired: in.AcceptanceRequired, AvailabilityZones: []string{"us-east-1a"},
	}
	return &awsec2.CreateVpcEndpointServiceConfigurationOutput{ServiceConfiguration: f.cfg}, nil
}
func (f *fakeEC2Svc) DescribeVpcEndpointServiceConfigurations(context.Context, *awsec2.DescribeVpcEndpointServiceConfigurationsInput, ...func(*awsec2.Options)) (*awsec2.DescribeVpcEndpointServiceConfigurationsOutput, error) {
	if f.cfg == nil {
		return &awsec2.DescribeVpcEndpointServiceConfigurationsOutput{}, nil
	}
	return &awsec2.DescribeVpcEndpointServiceConfigurationsOutput{ServiceConfigurations: []ec2types.ServiceConfiguration{*f.cfg}}, nil
}
func (f *fakeEC2Svc) ModifyVpcEndpointServiceConfiguration(context.Context, *awsec2.ModifyVpcEndpointServiceConfigurationInput, ...func(*awsec2.Options)) (*awsec2.ModifyVpcEndpointServiceConfigurationOutput, error) {
	return &awsec2.ModifyVpcEndpointServiceConfigurationOutput{}, nil
}
func (f *fakeEC2Svc) DeleteVpcEndpointServiceConfigurations(context.Context, *awsec2.DeleteVpcEndpointServiceConfigurationsInput, ...func(*awsec2.Options)) (*awsec2.DeleteVpcEndpointServiceConfigurationsOutput, error) {
	f.cfg = nil
	return &awsec2.DeleteVpcEndpointServiceConfigurationsOutput{}, nil
}
func (f *fakeEC2Svc) DescribeVpcEndpointServicePermissions(context.Context, *awsec2.DescribeVpcEndpointServicePermissionsInput, ...func(*awsec2.Options)) (*awsec2.DescribeVpcEndpointServicePermissionsOutput, error) {
	out := &awsec2.DescribeVpcEndpointServicePermissionsOutput{}
	for _, p := range f.principals {
		out.AllowedPrincipals = append(out.AllowedPrincipals, ec2types.AllowedPrincipal{Principal: aws.String(p)})
	}
	return out, nil
}
func (f *fakeEC2Svc) ModifyVpcEndpointServicePermissions(_ context.Context, in *awsec2.ModifyVpcEndpointServicePermissionsInput, _ ...func(*awsec2.Options)) (*awsec2.ModifyVpcEndpointServicePermissionsOutput, error) {
	f.permMods++
	f.principals = append(f.principals, in.AddAllowedPrincipals...)
	return &awsec2.ModifyVpcEndpointServicePermissionsOutput{}, nil
}
func (f *fakeEC2Svc) DescribeVpcEndpointConnections(context.Context, *awsec2.DescribeVpcEndpointConnectionsInput, ...func(*awsec2.Options)) (*awsec2.DescribeVpcEndpointConnectionsOutput, error) {
	out := &awsec2.DescribeVpcEndpointConnectionsOutput{}
	for _, id := range f.pendingConn {
		out.VpcEndpointConnections = append(out.VpcEndpointConnections, ec2types.VpcEndpointConnection{VpcEndpointId: aws.String(id), VpcEndpointState: ec2types.StatePendingAcceptance})
	}
	return out, nil
}
func (f *fakeEC2Svc) AcceptVpcEndpointConnections(_ context.Context, in *awsec2.AcceptVpcEndpointConnectionsInput, _ ...func(*awsec2.Options)) (*awsec2.AcceptVpcEndpointConnectionsOutput, error) {
	f.accepted = append(f.accepted, in.VpcEndpointIds...)
	f.pendingConn = nil
	return &awsec2.AcceptVpcEndpointConnectionsOutput{}, nil
}
func (f *fakeEC2Svc) CreateTags(context.Context, *awsec2.CreateTagsInput, ...func(*awsec2.Options)) (*awsec2.CreateTagsOutput, error) {
	return &awsec2.CreateTagsOutput{}, nil
}

func TestVPCEndpointServiceCreateAcceptsConnections(t *testing.T) {
	s := crossAccountScheme(t)
	lb := &awsv1alpha1.LoadBalancer{ObjectMeta: metav1.ObjectMeta{Name: "nlb", Namespace: "ns"}}
	lb.Status.ARN = "arn:nlb"
	obj := &awsv1alpha1.VPCEndpointService{
		ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "ns", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec: awsv1alpha1.VPCEndpointServiceSpec{
			NetworkLoadBalancerRefs: []awsv1alpha1.LoadBalancerRef{{Name: "nlb"}},
			AcceptanceRequired:      true, AutoAcceptConnections: true,
			AllowedPrincipals: []string{"arn:aws:iam::444455556666:root"},
		},
	}
	f := &fakeEC2Svc{pendingConn: []string{"vpce-consumer-1"}}
	c := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&awsv1alpha1.VPCEndpointService{}).WithObjects(obj, lb).Build()
	r := &VPCEndpointServiceReconciler{Client: c, Scheme: s, EC2Client: f}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "svc", Namespace: "ns"}}); err != nil {
		t.Fatal(err)
	}
	if f.cfg == nil || f.cfg.NetworkLoadBalancerArns[0] != "arn:nlb" {
		t.Fatalf("service not created from LoadBalancer CR ARN: %+v", f.cfg)
	}
	if f.permMods != 1 || len(f.principals) != 1 {
		t.Fatalf("allowed principals not applied: %v", f.principals)
	}
	if len(f.accepted) != 1 || f.accepted[0] != "vpce-consumer-1" {
		t.Fatalf("pending consumer connection not accepted: %v", f.accepted)
	}
	got := &awsv1alpha1.VPCEndpointService{}
	_ = c.Get(context.Background(), k8stypes.NamespacedName{Name: "svc", Namespace: "ns"}, got)
	if got.Status.ServiceID != "vpce-svc-1" || got.Status.ServiceName == "" || got.Status.PendingConnections != 0 {
		t.Fatalf("bad status %+v", got.Status)
	}
	if cnd := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady); cnd == nil || cnd.Status != metav1.ConditionTrue {
		t.Fatalf("expected Ready, got %+v", cnd)
	}
}

func TestVPCEndpointServiceDependencyNotReady(t *testing.T) {
	s := crossAccountScheme(t)
	lb := &awsv1alpha1.LoadBalancer{ObjectMeta: metav1.ObjectMeta{Name: "nlb", Namespace: "ns"}}
	obj := &awsv1alpha1.VPCEndpointService{
		ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "ns", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec:       awsv1alpha1.VPCEndpointServiceSpec{NetworkLoadBalancerRefs: []awsv1alpha1.LoadBalancerRef{{Name: "nlb"}}},
	}
	f := &fakeEC2Svc{}
	c := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&awsv1alpha1.VPCEndpointService{}).WithObjects(obj, lb).Build()
	r := &VPCEndpointServiceReconciler{Client: c, Scheme: s, EC2Client: f}
	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "svc", Namespace: "ns"}})
	if err != nil || res != requeueDependency || f.cfg != nil {
		t.Fatalf("expected dependency requeue without create, got %+v %v cfg=%v", res, err, f.cfg)
	}
}

func TestVPCEndpointServiceAbandon(t *testing.T) {
	s := crossAccountScheme(t)
	now := metav1.Now()
	obj := &awsv1alpha1.VPCEndpointService{
		ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "ns", Finalizers: []string{awsv1alpha1.FinalizerName}, DeletionTimestamp: &now,
			Annotations: map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}},
		Spec: awsv1alpha1.VPCEndpointServiceSpec{GatewayLoadBalancerArns: []string{"arn:gwlb"}},
	}
	obj.Status.ServiceID = "vpce-svc-1"
	f := &fakeEC2Svc{cfg: &ec2types.ServiceConfiguration{ServiceId: aws.String("vpce-svc-1")}}
	c := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&awsv1alpha1.VPCEndpointService{}).WithObjects(obj).Build()
	r := &VPCEndpointServiceReconciler{Client: c, Scheme: s, EC2Client: f}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "svc", Namespace: "ns"}}); err != nil {
		t.Fatal(err)
	}
	if f.cfg == nil {
		t.Fatal("abandon must not delete the AWS service")
	}
}
