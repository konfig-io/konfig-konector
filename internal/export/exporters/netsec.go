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

package exporters

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	awsnfw "github.com/aws/aws-sdk-go-v2/service/networkfirewall"
	nfwtypes "github.com/aws/aws-sdk-go-v2/service/networkfirewall/types"
	awslattice "github.com/aws/aws-sdk-go-v2/service/vpclattice"
	latticetypes "github.com/aws/aws-sdk-go-v2/service/vpclattice/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	awsclient "github.com/konfig-io/konfig-konector/internal/aws"
	"github.com/konfig-io/konfig-konector/internal/export"
)

func init() {
	// Rule groups before policies before firewalls; service networks before
	// services before associations; gateways before connections before routes.
	export.Register(export.Exporter{Kind: "FirewallRuleGroup", Service: "networkfirewall", Order: 105, Fn: exportFirewallRuleGroups})
	export.Register(export.Exporter{Kind: "FirewallPolicy", Service: "networkfirewall", Order: 106, Fn: exportFirewallPolicies})
	export.Register(export.Exporter{Kind: "Firewall", Service: "networkfirewall", Order: 107, Fn: exportFirewalls})

	export.Register(export.Exporter{Kind: "LatticeServiceNetwork", Service: "vpclattice", Order: 105, Fn: exportLatticeServiceNetworks})
	export.Register(export.Exporter{Kind: "LatticeService", Service: "vpclattice", Order: 106, Fn: exportLatticeServices})
	export.Register(export.Exporter{Kind: "LatticeTargetGroup", Service: "vpclattice", Order: 106, Fn: exportLatticeTargetGroups})
	export.Register(export.Exporter{Kind: "LatticeListener", Service: "vpclattice", Order: 107, Fn: exportLatticeListeners})
	export.Register(export.Exporter{Kind: "LatticeServiceNetworkVpcAssociation", Service: "vpclattice", Order: 108, Fn: exportLatticeSNVpcAssociations})
	export.Register(export.Exporter{Kind: "LatticeServiceNetworkServiceAssociation", Service: "vpclattice", Order: 108, Fn: exportLatticeSNServiceAssociations})

	export.Register(export.Exporter{Kind: "CustomerGateway", Service: "ec2", Order: 105, Fn: exportCustomerGateways})
	export.Register(export.Exporter{Kind: "VPNGateway", Service: "ec2", Order: 105, Fn: exportVPNGateways})
	export.Register(export.Exporter{Kind: "ManagedPrefixList", Service: "ec2", Order: 105, Fn: exportManagedPrefixLists})
	export.Register(export.Exporter{Kind: "CapacityReservation", Service: "ec2", Order: 105, Fn: exportCapacityReservations})
	export.Register(export.Exporter{Kind: "VPNConnection", Service: "ec2", Order: 108, Fn: exportVPNConnections})
	export.Register(export.Exporter{Kind: "VPNConnectionRoute", Service: "ec2", Order: 109, Fn: exportVPNConnectionRoutes})
}

// nfwTagMapExport converts Network Firewall tags to a spec tag map.
func nfwTagMapExport(tags []nfwtypes.Tag) map[string]string {
	if len(tags) == 0 {
		return nil
	}
	m := make(map[string]string, len(tags))
	for _, t := range tags {
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return export.TagMap(m)
}

func exportFirewallRuleGroups(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsnfw.NewListRuleGroupsPaginator(clients.NetworkFirewall, &awsnfw.ListRuleGroupsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list rule groups: %w", err)
		}
		for _, md := range page.RuleGroups {
			desc, err := clients.NetworkFirewall.DescribeRuleGroup(ctx, &awsnfw.DescribeRuleGroupInput{
				RuleGroupArn: md.Arn,
			})
			if err != nil {
				continue
			}
			resp := desc.RuleGroupResponse
			cr := &awsv1alpha1.FirewallRuleGroup{
				ObjectMeta: export.ObjectMeta(aws.ToString(resp.RuleGroupName), opts),
				Spec: awsv1alpha1.FirewallRuleGroupSpec{
					Name:        aws.ToString(resp.RuleGroupName),
					Type:        string(resp.Type),
					Capacity:    aws.ToInt32(resp.Capacity),
					Description: aws.ToString(resp.Description),
					Tags:        nfwTagMapExport(resp.Tags),
				},
			}
			if desc.RuleGroup != nil && desc.RuleGroup.RulesSource != nil && desc.RuleGroup.RulesSource.RulesString != nil {
				cr.Spec.RulesString = aws.ToString(desc.RuleGroup.RulesSource.RulesString)
			}
			opts.Index.Add(aws.ToString(resp.RuleGroupArn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportFirewallPolicies(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsnfw.NewListFirewallPoliciesPaginator(clients.NetworkFirewall, &awsnfw.ListFirewallPoliciesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list firewall policies: %w", err)
		}
		for _, md := range page.FirewallPolicies {
			desc, err := clients.NetworkFirewall.DescribeFirewallPolicy(ctx, &awsnfw.DescribeFirewallPolicyInput{
				FirewallPolicyArn: md.Arn,
			})
			if err != nil {
				continue
			}
			resp := desc.FirewallPolicyResponse
			cr := &awsv1alpha1.FirewallPolicy{
				ObjectMeta: export.ObjectMeta(aws.ToString(resp.FirewallPolicyName), opts),
				Spec: awsv1alpha1.FirewallPolicySpec{
					Name:        aws.ToString(resp.FirewallPolicyName),
					Description: aws.ToString(resp.Description),
					Tags:        nfwTagMapExport(resp.Tags),
				},
			}
			if pol := desc.FirewallPolicy; pol != nil {
				cr.Spec.StatelessDefaultActions = pol.StatelessDefaultActions
				cr.Spec.StatelessFragmentDefaultActions = pol.StatelessFragmentDefaultActions
				for _, ref := range pol.StatelessRuleGroupReferences {
					arn := aws.ToString(ref.ResourceArn)
					r := awsv1alpha1.StatelessRuleGroupRef{Priority: aws.ToInt32(ref.Priority)}
					if crName, ok := opts.Index.Lookup(arn); ok {
						r.Name = crName
					} else {
						r.ARN = arn
					}
					cr.Spec.StatelessRuleGroupRefs = append(cr.Spec.StatelessRuleGroupRefs, r)
				}
				for _, ref := range pol.StatefulRuleGroupReferences {
					arn := aws.ToString(ref.ResourceArn)
					r := awsv1alpha1.StatefulRuleGroupRef{}
					if crName, ok := opts.Index.Lookup(arn); ok {
						r.Name = crName
					} else {
						r.ARN = arn
					}
					cr.Spec.StatefulRuleGroupRefs = append(cr.Spec.StatefulRuleGroupRefs, r)
				}
			}
			opts.Index.Add(aws.ToString(resp.FirewallPolicyArn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportFirewalls(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsnfw.NewListFirewallsPaginator(clients.NetworkFirewall, &awsnfw.ListFirewallsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list firewalls: %w", err)
		}
		for _, md := range page.Firewalls {
			desc, err := clients.NetworkFirewall.DescribeFirewall(ctx, &awsnfw.DescribeFirewallInput{
				FirewallArn: md.FirewallArn,
			})
			if err != nil || desc.Firewall == nil {
				continue
			}
			fw := desc.Firewall
			cr := &awsv1alpha1.Firewall{
				ObjectMeta: export.ObjectMeta(aws.ToString(fw.FirewallName), opts),
				Spec: awsv1alpha1.FirewallSpec{
					Name:             aws.ToString(fw.FirewallName),
					DeleteProtection: fw.DeleteProtection,
					Description:      aws.ToString(fw.Description),
					Tags:             nfwTagMapExport(fw.Tags),
				},
			}
			policyARN := aws.ToString(fw.FirewallPolicyArn)
			if crName, ok := opts.Index.Lookup(policyARN); ok {
				cr.Spec.FirewallPolicyRef = awsv1alpha1.FirewallPolicyRef{Name: crName}
			} else {
				cr.Spec.FirewallPolicyRef = awsv1alpha1.FirewallPolicyRef{ARN: policyARN}
			}
			cr.Spec.VPCRef = vpcRefFor(aws.ToString(fw.VpcId), opts)
			for _, m := range fw.SubnetMappings {
				id := aws.ToString(m.SubnetId)
				if crName, ok := opts.Index.Lookup(id); ok {
					cr.Spec.SubnetRefs = append(cr.Spec.SubnetRefs, awsv1alpha1.SubnetRef{Name: crName})
				} else {
					cr.Spec.SubnetRefs = append(cr.Spec.SubnetRefs, awsv1alpha1.SubnetRef{ID: id})
				}
			}
			opts.Index.Add(aws.ToString(fw.FirewallArn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

// latticeTags fetches tags for a VPC Lattice resource ARN, best-effort.
func latticeTags(ctx context.Context, clients *awsclient.Clients, arn string) map[string]string {
	out, err := clients.VPCLattice.ListTagsForResource(ctx, &awslattice.ListTagsForResourceInput{
		ResourceArn: aws.String(arn),
	})
	if err != nil {
		return nil
	}
	return export.TagMap(out.Tags)
}

func exportLatticeServiceNetworks(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awslattice.NewListServiceNetworksPaginator(clients.VPCLattice, &awslattice.ListServiceNetworksInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list service networks: %w", err)
		}
		for _, sn := range page.Items {
			get, err := clients.VPCLattice.GetServiceNetwork(ctx, &awslattice.GetServiceNetworkInput{
				ServiceNetworkIdentifier: sn.Id,
			})
			if err != nil {
				continue
			}
			cr := &awsv1alpha1.LatticeServiceNetwork{
				ObjectMeta: export.ObjectMeta(aws.ToString(sn.Name), opts),
				Spec: awsv1alpha1.LatticeServiceNetworkSpec{
					Name:     aws.ToString(sn.Name),
					AuthType: string(get.AuthType),
					Tags:     latticeTags(ctx, clients, aws.ToString(sn.Arn)),
				},
			}
			opts.Index.Add(aws.ToString(sn.Id), cr.Name)
			opts.Index.Add(aws.ToString(sn.Arn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportLatticeServices(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awslattice.NewListServicesPaginator(clients.VPCLattice, &awslattice.ListServicesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list services: %w", err)
		}
		for _, svc := range page.Items {
			get, err := clients.VPCLattice.GetService(ctx, &awslattice.GetServiceInput{
				ServiceIdentifier: svc.Id,
			})
			if err != nil {
				continue
			}
			cr := &awsv1alpha1.LatticeService{
				ObjectMeta: export.ObjectMeta(aws.ToString(svc.Name), opts),
				Spec: awsv1alpha1.LatticeServiceSpec{
					Name:             aws.ToString(svc.Name),
					AuthType:         string(get.AuthType),
					CustomDomainName: aws.ToString(get.CustomDomainName),
					CertificateARN:   aws.ToString(get.CertificateArn),
					Tags:             latticeTags(ctx, clients, aws.ToString(svc.Arn)),
				},
			}
			opts.Index.Add(aws.ToString(svc.Id), cr.Name)
			opts.Index.Add(aws.ToString(svc.Arn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportLatticeTargetGroups(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awslattice.NewListTargetGroupsPaginator(clients.VPCLattice, &awslattice.ListTargetGroupsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list target groups: %w", err)
		}
		for _, tg := range page.Items {
			get, err := clients.VPCLattice.GetTargetGroup(ctx, &awslattice.GetTargetGroupInput{
				TargetGroupIdentifier: tg.Id,
			})
			if err != nil {
				continue
			}
			cr := &awsv1alpha1.LatticeTargetGroup{
				ObjectMeta: export.ObjectMeta(aws.ToString(tg.Name), opts),
				Spec: awsv1alpha1.LatticeTargetGroupSpec{
					Name: aws.ToString(tg.Name),
					Type: string(get.Type),
					Tags: latticeTags(ctx, clients, aws.ToString(tg.Arn)),
				},
			}
			if cfg := get.Config; cfg != nil {
				specCfg := &awsv1alpha1.LatticeTargetGroupConfig{
					Port:     aws.ToInt32(cfg.Port),
					Protocol: string(cfg.Protocol),
				}
				if cfg.VpcIdentifier != nil {
					ref := vpcRefFor(aws.ToString(cfg.VpcIdentifier), opts)
					specCfg.VPCRef = &ref
				}
				if hc := cfg.HealthCheck; hc != nil && aws.ToBool(hc.Enabled) {
					specCfg.HealthCheck = &awsv1alpha1.LatticeHealthCheck{
						Path:     aws.ToString(hc.Path),
						Protocol: string(hc.Protocol),
					}
				}
				cr.Spec.Config = specCfg
			}
			opts.Index.Add(aws.ToString(tg.Id), cr.Name)
			opts.Index.Add(aws.ToString(tg.Arn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportLatticeListeners(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	sp := awslattice.NewListServicesPaginator(clients.VPCLattice, &awslattice.ListServicesInput{})
	for sp.HasMorePages() {
		spage, err := sp.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list services: %w", err)
		}
		for _, svc := range spage.Items {
			lp := awslattice.NewListListenersPaginator(clients.VPCLattice, &awslattice.ListListenersInput{
				ServiceIdentifier: svc.Id,
			})
			for lp.HasMorePages() {
				lpage, err := lp.NextPage(ctx)
				if err != nil {
					return nil, fmt.Errorf("list listeners for %s: %w", aws.ToString(svc.Id), err)
				}
				for _, l := range lpage.Items {
					get, err := clients.VPCLattice.GetListener(ctx, &awslattice.GetListenerInput{
						ServiceIdentifier:  svc.Id,
						ListenerIdentifier: l.Id,
					})
					if err != nil {
						continue
					}
					crName := fmt.Sprintf("%s-%s", aws.ToString(svc.Name), aws.ToString(l.Name))
					cr := &awsv1alpha1.LatticeListener{
						ObjectMeta: export.ObjectMeta(crName, opts),
						Spec: awsv1alpha1.LatticeListenerSpec{
							Name:     aws.ToString(l.Name),
							Protocol: string(get.Protocol),
							Port:     aws.ToInt32(get.Port),
							Tags:     latticeTags(ctx, clients, aws.ToString(l.Arn)),
						},
					}
					if svcCR, ok := opts.Index.Lookup(aws.ToString(svc.Id)); ok {
						cr.Spec.ServiceRef = awsv1alpha1.LatticeServiceRef{Name: svcCR}
					} else {
						cr.Spec.ServiceRef = awsv1alpha1.LatticeServiceRef{ID: aws.ToString(svc.Id)}
					}
					cr.Spec.DefaultAction = latticeDefaultActionFor(get, opts)
					opts.Index.Add(aws.ToString(l.Id), cr.Name)
					opts.Index.Add(aws.ToString(l.Arn), cr.Name)
					objs = append(objs, cr)
				}
			}
		}
	}
	return objs, nil
}

// latticeDefaultActionFor converts an SDK listener default action to spec form.
func latticeDefaultActionFor(get *awslattice.GetListenerOutput, opts *export.Options) awsv1alpha1.LatticeDefaultAction {
	var da awsv1alpha1.LatticeDefaultAction
	switch v := get.DefaultAction.(type) {
	case *latticetypes.RuleActionMemberForward:
		for _, wtg := range v.Value.TargetGroups {
			id := aws.ToString(wtg.TargetGroupIdentifier)
			t := awsv1alpha1.LatticeForwardTarget{Weight: aws.ToInt32(wtg.Weight)}
			if crName, ok := opts.Index.Lookup(id); ok {
				t.TargetGroupRef = awsv1alpha1.LatticeTargetGroupRef{Name: crName}
			} else {
				t.TargetGroupRef = awsv1alpha1.LatticeTargetGroupRef{ID: id}
			}
			da.Forward = append(da.Forward, t)
		}
	case *latticetypes.RuleActionMemberFixedResponse:
		da.FixedResponse = &awsv1alpha1.LatticeFixedResponse{
			StatusCode: aws.ToInt32(v.Value.StatusCode),
		}
	}
	return da
}

func exportLatticeSNVpcAssociations(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awslattice.NewListServiceNetworkVpcAssociationsPaginator(clients.VPCLattice, &awslattice.ListServiceNetworkVpcAssociationsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list service network VPC associations: %w", err)
		}
		for _, assoc := range page.Items {
			get, err := clients.VPCLattice.GetServiceNetworkVpcAssociation(ctx, &awslattice.GetServiceNetworkVpcAssociationInput{
				ServiceNetworkVpcAssociationIdentifier: assoc.Id,
			})
			if err != nil {
				continue
			}
			crName := fmt.Sprintf("%s-%s", aws.ToString(assoc.ServiceNetworkName), aws.ToString(get.VpcId))
			cr := &awsv1alpha1.LatticeServiceNetworkVpcAssociation{
				ObjectMeta: export.ObjectMeta(crName, opts),
				Spec: awsv1alpha1.LatticeServiceNetworkVpcAssociationSpec{
					Tags: latticeTags(ctx, clients, aws.ToString(assoc.Arn)),
				},
			}
			snID := aws.ToString(assoc.ServiceNetworkId)
			if name, ok := opts.Index.Lookup(snID); ok {
				cr.Spec.ServiceNetworkRef = awsv1alpha1.LatticeServiceNetworkRef{Name: name}
			} else {
				cr.Spec.ServiceNetworkRef = awsv1alpha1.LatticeServiceNetworkRef{ID: snID}
			}
			cr.Spec.VPCRef = vpcRefFor(aws.ToString(get.VpcId), opts)
			for _, sg := range get.SecurityGroupIds {
				if name, ok := opts.Index.Lookup(sg); ok {
					cr.Spec.SecurityGroupRefs = append(cr.Spec.SecurityGroupRefs, awsv1alpha1.SecurityGroupRef{Name: name})
				} else {
					cr.Spec.SecurityGroupRefs = append(cr.Spec.SecurityGroupRefs, awsv1alpha1.SecurityGroupRef{ID: sg})
				}
			}
			opts.Index.Add(aws.ToString(assoc.Id), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportLatticeSNServiceAssociations(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awslattice.NewListServiceNetworkServiceAssociationsPaginator(clients.VPCLattice, &awslattice.ListServiceNetworkServiceAssociationsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list service network service associations: %w", err)
		}
		for _, assoc := range page.Items {
			crName := fmt.Sprintf("%s-%s", aws.ToString(assoc.ServiceNetworkName), aws.ToString(assoc.ServiceName))
			cr := &awsv1alpha1.LatticeServiceNetworkServiceAssociation{
				ObjectMeta: export.ObjectMeta(crName, opts),
				Spec: awsv1alpha1.LatticeServiceNetworkServiceAssociationSpec{
					Tags: latticeTags(ctx, clients, aws.ToString(assoc.Arn)),
				},
			}
			snID := aws.ToString(assoc.ServiceNetworkId)
			if name, ok := opts.Index.Lookup(snID); ok {
				cr.Spec.ServiceNetworkRef = awsv1alpha1.LatticeServiceNetworkRef{Name: name}
			} else {
				cr.Spec.ServiceNetworkRef = awsv1alpha1.LatticeServiceNetworkRef{ID: snID}
			}
			svcID := aws.ToString(assoc.ServiceId)
			if name, ok := opts.Index.Lookup(svcID); ok {
				cr.Spec.ServiceRef = awsv1alpha1.LatticeServiceRef{Name: name}
			} else {
				cr.Spec.ServiceRef = awsv1alpha1.LatticeServiceRef{ID: svcID}
			}
			opts.Index.Add(aws.ToString(assoc.Id), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportCustomerGateways(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	out, err := clients.EC2.DescribeCustomerGateways(ctx, &awsec2.DescribeCustomerGatewaysInput{})
	if err != nil {
		return nil, fmt.Errorf("describe customer gateways: %w", err)
	}
	for _, cgw := range out.CustomerGateways {
		if aws.ToString(cgw.State) == "deleted" || aws.ToString(cgw.State) == "deleting" {
			continue
		}
		id := aws.ToString(cgw.CustomerGatewayId)
		cr := &awsv1alpha1.CustomerGateway{
			ObjectMeta: export.ObjectMeta(ec2NameTag(cgw.Tags, id), opts),
			Spec: awsv1alpha1.CustomerGatewaySpec{
				BGPASN:     atoi32(aws.ToString(cgw.BgpAsn)),
				IPAddress:  aws.ToString(cgw.IpAddress),
				Type:       aws.ToString(cgw.Type),
				DeviceName: aws.ToString(cgw.DeviceName),
				Tags:       ec2TagMap(cgw.Tags),
			},
		}
		opts.Index.Add(id, cr.Name)
		objs = append(objs, cr)
	}
	return objs, nil
}

func exportVPNGateways(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	out, err := clients.EC2.DescribeVpnGateways(ctx, &awsec2.DescribeVpnGatewaysInput{})
	if err != nil {
		return nil, fmt.Errorf("describe VPN gateways: %w", err)
	}
	for _, vgw := range out.VpnGateways {
		if vgw.State == ec2types.VpnStateDeleted || vgw.State == ec2types.VpnStateDeleting {
			continue
		}
		id := aws.ToString(vgw.VpnGatewayId)
		cr := &awsv1alpha1.VPNGateway{
			ObjectMeta: export.ObjectMeta(ec2NameTag(vgw.Tags, id), opts),
			Spec: awsv1alpha1.VPNGatewaySpec{
				Type:          string(vgw.Type),
				AmazonSideASN: aws.ToInt64(vgw.AmazonSideAsn),
				Tags:          ec2TagMap(vgw.Tags),
			},
		}
		for _, att := range vgw.VpcAttachments {
			if att.State == ec2types.AttachmentStatusAttached {
				ref := vpcRefFor(aws.ToString(att.VpcId), opts)
				cr.Spec.VPCRef = &ref
				break
			}
		}
		opts.Index.Add(id, cr.Name)
		objs = append(objs, cr)
	}
	return objs, nil
}

func exportVPNConnections(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	out, err := clients.EC2.DescribeVpnConnections(ctx, &awsec2.DescribeVpnConnectionsInput{})
	if err != nil {
		return nil, fmt.Errorf("describe VPN connections: %w", err)
	}
	for _, conn := range out.VpnConnections {
		if conn.State == ec2types.VpnStateDeleted || conn.State == ec2types.VpnStateDeleting {
			continue
		}
		id := aws.ToString(conn.VpnConnectionId)
		cr := &awsv1alpha1.VPNConnection{
			ObjectMeta: export.ObjectMeta(ec2NameTag(conn.Tags, id), opts),
			Spec: awsv1alpha1.VPNConnectionSpec{
				Type: string(conn.Type),
				Tags: ec2TagMap(conn.Tags),
			},
		}
		cgwID := aws.ToString(conn.CustomerGatewayId)
		if name, ok := opts.Index.Lookup(cgwID); ok {
			cr.Spec.CustomerGatewayRef = awsv1alpha1.CustomerGatewayRef{Name: name}
		} else {
			cr.Spec.CustomerGatewayRef = awsv1alpha1.CustomerGatewayRef{ID: cgwID}
		}
		if vgwID := aws.ToString(conn.VpnGatewayId); vgwID != "" {
			if name, ok := opts.Index.Lookup(vgwID); ok {
				cr.Spec.VPNGatewayRef = &awsv1alpha1.VPNGatewayRef{Name: name}
			} else {
				cr.Spec.VPNGatewayRef = &awsv1alpha1.VPNGatewayRef{ID: vgwID}
			}
		} else if tgwID := aws.ToString(conn.TransitGatewayId); tgwID != "" {
			if name, ok := opts.Index.Lookup(tgwID); ok {
				cr.Spec.TransitGatewayRef = &awsv1alpha1.TransitGatewayRef{Name: name}
			} else {
				cr.Spec.TransitGatewayRef = &awsv1alpha1.TransitGatewayRef{TransitGatewayID: tgwID}
			}
		}
		if conn.Options != nil {
			cr.Spec.StaticRoutesOnly = aws.ToBool(conn.Options.StaticRoutesOnly)
			// Tunnel inside CIDRs round-trip; pre-shared keys are secret
			// material and are NEVER exported. Users must populate
			// presharedKeyRef Secrets manually if they want the operator to
			// manage tunnel PSKs.
			for _, t := range conn.Options.TunnelOptions {
				if aws.ToString(t.TunnelInsideCidr) != "" {
					cr.Spec.TunnelOptions = append(cr.Spec.TunnelOptions, awsv1alpha1.VPNTunnelOptions{
						InsideCIDR: aws.ToString(t.TunnelInsideCidr),
					})
				}
			}
		}
		opts.Index.Add(id, cr.Name)
		objs = append(objs, cr)
	}
	return objs, nil
}

func exportVPNConnectionRoutes(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	out, err := clients.EC2.DescribeVpnConnections(ctx, &awsec2.DescribeVpnConnectionsInput{})
	if err != nil {
		return nil, fmt.Errorf("describe VPN connections: %w", err)
	}
	for _, conn := range out.VpnConnections {
		if conn.State == ec2types.VpnStateDeleted || conn.State == ec2types.VpnStateDeleting {
			continue
		}
		vpnID := aws.ToString(conn.VpnConnectionId)
		for _, route := range conn.Routes {
			cidr := aws.ToString(route.DestinationCidrBlock)
			crName := fmt.Sprintf("%s-%s", vpnID, cidr)
			cr := &awsv1alpha1.VPNConnectionRoute{
				ObjectMeta: export.ObjectMeta(crName, opts),
				Spec: awsv1alpha1.VPNConnectionRouteSpec{
					DestinationCIDRBlock: cidr,
				},
			}
			if name, ok := opts.Index.Lookup(vpnID); ok {
				cr.Spec.VPNConnectionRef = awsv1alpha1.VPNConnectionRef{Name: name}
			} else {
				cr.Spec.VPNConnectionRef = awsv1alpha1.VPNConnectionRef{ID: vpnID}
			}
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportManagedPrefixLists(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsec2.NewDescribeManagedPrefixListsPaginator(clients.EC2, &awsec2.DescribeManagedPrefixListsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe managed prefix lists: %w", err)
		}
		for _, pl := range page.PrefixLists {
			// Skip AWS-managed prefix lists: owned by AWS, not the account.
			if aws.ToString(pl.OwnerId) == "AWS" || strings.HasPrefix(aws.ToString(pl.PrefixListName), "com.amazonaws.") {
				continue
			}
			id := aws.ToString(pl.PrefixListId)
			cr := &awsv1alpha1.ManagedPrefixList{
				ObjectMeta: export.ObjectMeta(aws.ToString(pl.PrefixListName), opts),
				Spec: awsv1alpha1.ManagedPrefixListSpec{
					Name:          aws.ToString(pl.PrefixListName),
					AddressFamily: aws.ToString(pl.AddressFamily),
					MaxEntries:    aws.ToInt32(pl.MaxEntries),
					Tags:          ec2TagMap(pl.Tags),
				},
			}
			ep := awsec2.NewGetManagedPrefixListEntriesPaginator(clients.EC2, &awsec2.GetManagedPrefixListEntriesInput{
				PrefixListId: pl.PrefixListId,
			})
			entriesOK := true
			for ep.HasMorePages() {
				epage, err := ep.NextPage(ctx)
				if err != nil {
					entriesOK = false
					break
				}
				for _, e := range epage.Entries {
					cr.Spec.Entries = append(cr.Spec.Entries, awsv1alpha1.PrefixListEntry{
						CIDR:        aws.ToString(e.Cidr),
						Description: aws.ToString(e.Description),
					})
				}
			}
			if !entriesOK {
				continue
			}
			opts.Index.Add(id, cr.Name)
			opts.Index.Add(aws.ToString(pl.PrefixListArn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportCapacityReservations(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsec2.NewDescribeCapacityReservationsPaginator(clients.EC2, &awsec2.DescribeCapacityReservationsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe capacity reservations: %w", err)
		}
		for _, res := range page.CapacityReservations {
			// Only active reservations can be meaningfully managed.
			if res.State != ec2types.CapacityReservationStateActive {
				continue
			}
			id := aws.ToString(res.CapacityReservationId)
			cr := &awsv1alpha1.CapacityReservation{
				ObjectMeta: export.ObjectMeta(ec2NameTag(res.Tags, id), opts),
				Spec: awsv1alpha1.CapacityReservationSpec{
					InstanceType:     aws.ToString(res.InstanceType),
					InstancePlatform: string(res.InstancePlatform),
					AvailabilityZone: aws.ToString(res.AvailabilityZone),
					InstanceCount:    aws.ToInt32(res.TotalInstanceCount),
					Tenancy:          string(res.Tenancy),
					EndDateType:      string(res.EndDateType),
					Tags:             ec2TagMap(res.Tags),
				},
			}
			if res.EndDate != nil {
				t := metav1.NewTime(*res.EndDate)
				cr.Spec.EndDate = &t
			}
			opts.Index.Add(id, cr.Name)
			opts.Index.Add(aws.ToString(res.CapacityReservationArn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}
