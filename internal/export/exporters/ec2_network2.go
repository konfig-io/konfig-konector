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

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	awsclient "github.com/konfig-io/konfig-konector/internal/aws"
	"github.com/konfig-io/konfig-konector/internal/export"
)

func init() {
	export.Register(export.Exporter{Kind: "InternetGateway", Service: "ec2", Order: 12, Fn: exportInternetGateways})
	export.Register(export.Exporter{Kind: "EgressOnlyIGW", Service: "ec2", Order: 12, Fn: exportEgressOnlyIGWs})
	export.Register(export.Exporter{Kind: "TransitGateway", Service: "ec2", Order: 12, Fn: exportTransitGateways})
	export.Register(export.Exporter{Kind: "NatGateway", Service: "ec2", Order: 13, Fn: exportNatGateways})
	export.Register(export.Exporter{Kind: "SecurityGroup", Service: "ec2", Order: 14, Fn: exportSecurityGroups})
	export.Register(export.Exporter{Kind: "NetworkACL", Service: "ec2", Order: 15, Fn: exportNetworkACLs})
	export.Register(export.Exporter{Kind: "RouteTable", Service: "ec2", Order: 16, Fn: exportRouteTables})
	export.Register(export.Exporter{Kind: "VPCEndpoint", Service: "ec2", Order: 17, Fn: exportVPCEndpoints})
	export.Register(export.Exporter{Kind: "VPCPeeringConnection", Service: "ec2", Order: 17, Fn: exportVPCPeeringConnections})
	export.Register(export.Exporter{Kind: "TransitGatewayVpcAttachment", Service: "ec2", Order: 18, Fn: exportTGWVpcAttachments})
	export.Register(export.Exporter{Kind: "FlowLog", Service: "ec2", Order: 19, Fn: exportFlowLogs})
	// EIPAssociation runs after EC2 instances/EIPs are indexed.
	export.Register(export.Exporter{Kind: "EIPAssociation", Service: "ec2", Order: 24, Fn: exportEIPAssociations})
}

// vpcRefFor resolves a VPC ID to a CR-name ref or a raw-ID ref.
func vpcRefFor(vpcID string, opts *export.Options) awsv1alpha1.VPCResourceRef {
	if crName, ok := opts.Index.Lookup(vpcID); ok {
		return awsv1alpha1.VPCResourceRef{Name: crName}
	}
	return awsv1alpha1.VPCResourceRef{ID: vpcID}
}

func exportInternetGateways(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsec2.NewDescribeInternetGatewaysPaginator(clients.EC2, &awsec2.DescribeInternetGatewaysInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe internet gateways: %w", err)
		}
		for _, igw := range page.InternetGateways {
			// The spec requires a VPC attachment; detached IGWs cannot be
			// represented.
			if len(igw.Attachments) == 0 {
				continue
			}
			id := aws.ToString(igw.InternetGatewayId)
			cr := &awsv1alpha1.InternetGateway{
				ObjectMeta: export.ObjectMeta(ec2NameTag(igw.Tags, id), opts),
				Spec: awsv1alpha1.InternetGatewaySpec{
					VPCRef: vpcRefFor(aws.ToString(igw.Attachments[0].VpcId), opts),
					Tags:   ec2TagMap(igw.Tags),
				},
			}
			opts.Index.Add(id, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportEgressOnlyIGWs(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsec2.NewDescribeEgressOnlyInternetGatewaysPaginator(clients.EC2, &awsec2.DescribeEgressOnlyInternetGatewaysInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe egress-only internet gateways: %w", err)
		}
		for _, eigw := range page.EgressOnlyInternetGateways {
			if len(eigw.Attachments) == 0 {
				continue
			}
			id := aws.ToString(eigw.EgressOnlyInternetGatewayId)
			cr := &awsv1alpha1.EgressOnlyIGW{
				ObjectMeta: export.ObjectMeta(ec2NameTag(eigw.Tags, id), opts),
				Spec: awsv1alpha1.EgressOnlyIGWSpec{
					VPCRef: vpcRefFor(aws.ToString(eigw.Attachments[0].VpcId), opts),
					Tags:   ec2TagMap(eigw.Tags),
				},
			}
			opts.Index.Add(id, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportNatGateways(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsec2.NewDescribeNatGatewaysPaginator(clients.EC2, &awsec2.DescribeNatGatewaysInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe nat gateways: %w", err)
		}
		for _, ngw := range page.NatGateways {
			if ngw.State == ec2types.NatGatewayStateDeleted || ngw.State == ec2types.NatGatewayStateDeleting {
				continue
			}
			id := aws.ToString(ngw.NatGatewayId)
			cr := &awsv1alpha1.NatGateway{
				ObjectMeta: export.ObjectMeta(ec2NameTag(ngw.Tags, id), opts),
				Spec: awsv1alpha1.NatGatewaySpec{
					ConnectivityType: string(ngw.ConnectivityType),
					Tags:             ec2TagMap(ngw.Tags),
				},
			}
			subnetID := aws.ToString(ngw.SubnetId)
			if crName, ok := opts.Index.Lookup(subnetID); ok {
				cr.Spec.SubnetRef = awsv1alpha1.SubnetRef{Name: crName}
			} else {
				cr.Spec.SubnetRef = awsv1alpha1.SubnetRef{ID: subnetID}
			}
			opts.Index.Add(id, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

// sgRules converts EC2 IpPermissions into spec rules, expanding each
// CIDR/group/prefix-list source into its own SGRule.
func sgRules(perms []ec2types.IpPermission, opts *export.Options) []awsv1alpha1.SGRule {
	var rules []awsv1alpha1.SGRule
	for _, perm := range perms {
		base := awsv1alpha1.SGRule{
			Protocol: aws.ToString(perm.IpProtocol),
			FromPort: aws.ToInt32(perm.FromPort),
			ToPort:   aws.ToInt32(perm.ToPort),
		}
		for _, r := range perm.IpRanges {
			rule := base
			rule.CIDRIPv4 = aws.ToString(r.CidrIp)
			rule.Description = aws.ToString(r.Description)
			rules = append(rules, rule)
		}
		for _, r := range perm.Ipv6Ranges {
			rule := base
			rule.CIDRIPv6 = aws.ToString(r.CidrIpv6)
			rule.Description = aws.ToString(r.Description)
			rules = append(rules, rule)
		}
		for _, r := range perm.PrefixListIds {
			rule := base
			rule.PrefixListID = aws.ToString(r.PrefixListId)
			rule.Description = aws.ToString(r.Description)
			rules = append(rules, rule)
		}
		for _, r := range perm.UserIdGroupPairs {
			rule := base
			rule.Description = aws.ToString(r.Description)
			// SourceGroupRef only supports a CR name; when the group was not
			// exported the rule keeps port/protocol but drops the group
			// source (nothing in the spec can carry a raw sg-id source).
			if crName, ok := opts.Index.Lookup(aws.ToString(r.GroupId)); ok {
				rule.SourceGroupRef = crName
			} else {
				continue
			}
			rules = append(rules, rule)
		}
	}
	return rules
}

// isDefaultSGEgress reports whether the egress rule set is exactly the
// implicit allow-all rule AWS creates on every group.
func isDefaultSGEgress(perms []ec2types.IpPermission) bool {
	if len(perms) != 1 {
		return false
	}
	p := perms[0]
	return aws.ToString(p.IpProtocol) == "-1" &&
		len(p.IpRanges) == 1 && aws.ToString(p.IpRanges[0].CidrIp) == "0.0.0.0/0" &&
		len(p.Ipv6Ranges) == 0 && len(p.PrefixListIds) == 0 && len(p.UserIdGroupPairs) == 0
}

func exportSecurityGroups(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	// Two passes: index every non-default group's CR name first so that
	// intra-group SourceGroupRefs resolve regardless of ordering.
	var groups []ec2types.SecurityGroup
	p := awsec2.NewDescribeSecurityGroupsPaginator(clients.EC2, &awsec2.DescribeSecurityGroupsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe security groups: %w", err)
		}
		for _, sg := range page.SecurityGroups {
			// The per-VPC "default" group is AWS-managed and cannot be
			// created or deleted by the operator.
			if aws.ToString(sg.GroupName) == "default" {
				continue
			}
			groups = append(groups, sg)
			name := export.CRName(ec2NameTag(sg.Tags, aws.ToString(sg.GroupName)))
			opts.Index.Add(aws.ToString(sg.GroupId), name)
		}
	}
	var objs []client.Object
	for _, sg := range groups {
		id := aws.ToString(sg.GroupId)
		cr := &awsv1alpha1.SecurityGroup{
			ObjectMeta: export.ObjectMeta(ec2NameTag(sg.Tags, aws.ToString(sg.GroupName)), opts),
			Spec: awsv1alpha1.SecurityGroupSpec{
				VPCRef:       vpcRefFor(aws.ToString(sg.VpcId), opts),
				GroupName:    aws.ToString(sg.GroupName),
				Description:  aws.ToString(sg.Description),
				IngressRules: sgRules(sg.IpPermissions, opts),
				Tags:         ec2TagMap(sg.Tags),
			},
		}
		// Skip the implicit allow-all egress rule AWS adds to every group,
		// but keep any explicitly configured egress rules.
		if !isDefaultSGEgress(sg.IpPermissionsEgress) {
			cr.Spec.EgressRules = sgRules(sg.IpPermissionsEgress, opts)
		}
		opts.Index.Add(id, cr.Name)
		objs = append(objs, cr)
	}
	return objs, nil
}

func exportNetworkACLs(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsec2.NewDescribeNetworkAclsPaginator(clients.EC2, &awsec2.DescribeNetworkAclsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe network acls: %w", err)
		}
		for _, nacl := range page.NetworkAcls {
			// Default NACLs are created with the VPC and cannot be managed.
			if aws.ToBool(nacl.IsDefault) {
				continue
			}
			id := aws.ToString(nacl.NetworkAclId)
			cr := &awsv1alpha1.NetworkACL{
				ObjectMeta: export.ObjectMeta(ec2NameTag(nacl.Tags, id), opts),
				Spec: awsv1alpha1.NetworkACLSpec{
					VPCRef: vpcRefFor(aws.ToString(nacl.VpcId), opts),
					Tags:   ec2TagMap(nacl.Tags),
				},
			}
			for _, e := range nacl.Entries {
				// Rule 32767 is the immutable default deny-all entry.
				if aws.ToInt32(e.RuleNumber) > 32766 {
					continue
				}
				entry := awsv1alpha1.NetworkACLEntry{
					RuleNumber:    aws.ToInt32(e.RuleNumber),
					Protocol:      aws.ToString(e.Protocol),
					RuleAction:    string(e.RuleAction),
					Egress:        aws.ToBool(e.Egress),
					CIDRBlock:     aws.ToString(e.CidrBlock),
					IPv6CIDRBlock: aws.ToString(e.Ipv6CidrBlock),
				}
				if e.PortRange != nil {
					entry.PortRange = &awsv1alpha1.NACLPortRange{
						From: aws.ToInt32(e.PortRange.From),
						To:   aws.ToInt32(e.PortRange.To),
					}
				}
				if e.IcmpTypeCode != nil {
					entry.ICMPTypeCode = &awsv1alpha1.NACLICMPTypeCode{
						Type: aws.ToInt32(e.IcmpTypeCode.Type),
						Code: aws.ToInt32(e.IcmpTypeCode.Code),
					}
				}
				cr.Spec.Entries = append(cr.Spec.Entries, entry)
			}
			opts.Index.Add(id, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportRouteTables(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsec2.NewDescribeRouteTablesPaginator(clients.EC2, &awsec2.DescribeRouteTablesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe route tables: %w", err)
		}
		for _, rt := range page.RouteTables {
			// The main route table is created with the VPC; the operator
			// could never have created it.
			isMain := false
			for _, assoc := range rt.Associations {
				if aws.ToBool(assoc.Main) {
					isMain = true
					break
				}
			}
			if isMain {
				continue
			}
			id := aws.ToString(rt.RouteTableId)
			cr := &awsv1alpha1.RouteTable{
				ObjectMeta: export.ObjectMeta(ec2NameTag(rt.Tags, id), opts),
				Spec: awsv1alpha1.RouteTableSpec{
					VPCRef: vpcRefFor(aws.ToString(rt.VpcId), opts),
					Tags:   ec2TagMap(rt.Tags),
				},
			}
			for _, route := range rt.Routes {
				// Skip the implicit local route.
				if aws.ToString(route.GatewayId) == "local" {
					continue
				}
				// The spec only models IPv4 destinations.
				dest := aws.ToString(route.DestinationCidrBlock)
				if dest == "" {
					continue
				}
				entry := awsv1alpha1.RouteEntry{DestinationCIDR: dest}
				switch {
				case route.NatGatewayId != nil:
					// NatGatewayRef only supports a CR name; keep the raw ID
					// in GatewayID when the NAT GW wasn't exported.
					if crName, ok := opts.Index.Lookup(aws.ToString(route.NatGatewayId)); ok {
						entry.NatGatewayRef = crName
					} else {
						entry.GatewayID = aws.ToString(route.NatGatewayId)
					}
				case route.GatewayId != nil:
					entry.GatewayID = aws.ToString(route.GatewayId)
				case route.EgressOnlyInternetGatewayId != nil:
					entry.GatewayID = aws.ToString(route.EgressOnlyInternetGatewayId)
				case route.TransitGatewayId != nil:
					entry.GatewayID = aws.ToString(route.TransitGatewayId)
				case route.VpcPeeringConnectionId != nil:
					entry.GatewayID = aws.ToString(route.VpcPeeringConnectionId)
				case route.NetworkInterfaceId != nil:
					entry.GatewayID = aws.ToString(route.NetworkInterfaceId)
				default:
					continue
				}
				cr.Spec.Routes = append(cr.Spec.Routes, entry)
			}
			// subnetAssociations are plain Subnet CR names: only include
			// subnets exported in this run.
			for _, assoc := range rt.Associations {
				subnetID := aws.ToString(assoc.SubnetId)
				if subnetID == "" {
					continue
				}
				if crName, ok := opts.Index.Lookup(subnetID); ok {
					cr.Spec.SubnetAssociations = append(cr.Spec.SubnetAssociations, crName)
				}
			}
			opts.Index.Add(id, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportVPCEndpoints(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsec2.NewDescribeVpcEndpointsPaginator(clients.EC2, &awsec2.DescribeVpcEndpointsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe vpc endpoints: %w", err)
		}
		for _, ep := range page.VpcEndpoints {
			if ep.State == ec2types.StateDeleted || ep.State == ec2types.StateDeleting {
				continue
			}
			id := aws.ToString(ep.VpcEndpointId)
			cr := &awsv1alpha1.VPCEndpoint{
				ObjectMeta: export.ObjectMeta(ec2NameTag(ep.Tags, id), opts),
				Spec: awsv1alpha1.VPCEndpointSpec{
					VPCRef:       vpcRefFor(aws.ToString(ep.VpcId), opts),
					ServiceName:  aws.ToString(ep.ServiceName),
					EndpointType: string(ep.VpcEndpointType),
					Tags:         ec2TagMap(ep.Tags),
				},
			}
			// RouteTableRefs are plain CR names; only exported tables resolve.
			for _, rtbID := range ep.RouteTableIds {
				if crName, ok := opts.Index.Lookup(rtbID); ok {
					cr.Spec.RouteTableRefs = append(cr.Spec.RouteTableRefs, crName)
				}
			}
			for _, subnetID := range ep.SubnetIds {
				if crName, ok := opts.Index.Lookup(subnetID); ok {
					cr.Spec.SubnetRefs = append(cr.Spec.SubnetRefs, awsv1alpha1.SubnetRef{Name: crName})
				} else {
					cr.Spec.SubnetRefs = append(cr.Spec.SubnetRefs, awsv1alpha1.SubnetRef{ID: subnetID})
				}
			}
			for _, g := range ep.Groups {
				sgID := aws.ToString(g.GroupId)
				if crName, ok := opts.Index.Lookup(sgID); ok {
					cr.Spec.SecurityGroupRefs = append(cr.Spec.SecurityGroupRefs, awsv1alpha1.SecurityGroupRef{Name: crName})
				} else {
					cr.Spec.SecurityGroupRefs = append(cr.Spec.SecurityGroupRefs, awsv1alpha1.SecurityGroupRef{ID: sgID})
				}
			}
			opts.Index.Add(id, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportVPCPeeringConnections(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsec2.NewDescribeVpcPeeringConnectionsPaginator(clients.EC2, &awsec2.DescribeVpcPeeringConnectionsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe vpc peering connections: %w", err)
		}
		for _, pc := range page.VpcPeeringConnections {
			if pc.Status == nil || pc.Status.Code != ec2types.VpcPeeringConnectionStateReasonCodeActive {
				continue
			}
			if pc.RequesterVpcInfo == nil || pc.AccepterVpcInfo == nil {
				continue
			}
			id := aws.ToString(pc.VpcPeeringConnectionId)
			cr := &awsv1alpha1.VPCPeeringConnection{
				ObjectMeta: export.ObjectMeta(ec2NameTag(pc.Tags, id), opts),
				Spec: awsv1alpha1.VPCPeeringConnectionSpec{
					VPCRef:      vpcRefFor(aws.ToString(pc.RequesterVpcInfo.VpcId), opts),
					PeerVPCID:   aws.ToString(pc.AccepterVpcInfo.VpcId),
					PeerOwnerID: aws.ToString(pc.AccepterVpcInfo.OwnerId),
					PeerRegion:  aws.ToString(pc.AccepterVpcInfo.Region),
					Tags:        ec2TagMap(pc.Tags),
				},
			}
			opts.Index.Add(id, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportEIPAssociations(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	// DescribeAddresses is not a paginated API.
	out, err := clients.EC2.DescribeAddresses(ctx, &awsec2.DescribeAddressesInput{})
	if err != nil {
		return nil, fmt.Errorf("describe addresses: %w", err)
	}
	var objs []client.Object
	for _, a := range out.Addresses {
		if aws.ToString(a.AssociationId) == "" {
			continue
		}
		allocID := aws.ToString(a.AllocationId)
		cr := &awsv1alpha1.EIPAssociation{
			ObjectMeta: export.ObjectMeta(aws.ToString(a.AssociationId), opts),
			Spec: awsv1alpha1.EIPAssociationSpec{
				PrivateIPAddress: aws.ToString(a.PrivateIpAddress),
			},
		}
		if crName, ok := opts.Index.Lookup(allocID); ok {
			cr.Spec.ElasticIPRef = awsv1alpha1.ElasticIPRef{Name: crName}
		} else {
			cr.Spec.ElasticIPRef = awsv1alpha1.ElasticIPRef{AllocationID: allocID}
		}
		if instID := aws.ToString(a.InstanceId); instID != "" {
			if crName, ok := opts.Index.Lookup(instID); ok {
				cr.Spec.InstanceRef = &awsv1alpha1.ResourceRef{Name: crName}
			} else {
				cr.Spec.InstanceID = instID
			}
		} else if eniID := aws.ToString(a.NetworkInterfaceId); eniID != "" {
			cr.Spec.NetworkInterfaceID = eniID
		}
		opts.Index.Add(aws.ToString(a.AssociationId), cr.Name)
		objs = append(objs, cr)
	}
	return objs, nil
}

func exportFlowLogs(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsec2.NewDescribeFlowLogsPaginator(clients.EC2, &awsec2.DescribeFlowLogsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe flow logs: %w", err)
		}
		for _, fl := range page.FlowLogs {
			id := aws.ToString(fl.FlowLogId)
			resourceID := aws.ToString(fl.ResourceId)
			cr := &awsv1alpha1.FlowLog{
				ObjectMeta: export.ObjectMeta(ec2NameTag(fl.Tags, id), opts),
				Spec: awsv1alpha1.FlowLogSpec{
					ResourceID:               resourceID,
					ResourceType:             flowLogResourceType(resourceID),
					TrafficType:              string(fl.TrafficType),
					LogDestinationType:       string(fl.LogDestinationType),
					LogDestination:           aws.ToString(fl.LogDestination),
					LogGroupName:             aws.ToString(fl.LogGroupName),
					DeliverLogsPermissionARN: aws.ToString(fl.DeliverLogsPermissionArn),
					LogFormat:                aws.ToString(fl.LogFormat),
					MaxAggregationInterval:   aws.ToInt32(fl.MaxAggregationInterval),
					Tags:                     ec2TagMap(fl.Tags),
				},
			}
			opts.Index.Add(id, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

// flowLogResourceType derives the spec enum from the target resource ID.
func flowLogResourceType(resourceID string) string {
	switch {
	case len(resourceID) > 4 && resourceID[:4] == "vpc-":
		return "VPC"
	case len(resourceID) > 7 && resourceID[:7] == "subnet-":
		return "Subnet"
	default:
		return "NetworkInterface"
	}
}

func exportTransitGateways(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsec2.NewDescribeTransitGatewaysPaginator(clients.EC2, &awsec2.DescribeTransitGatewaysInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe transit gateways: %w", err)
		}
		for _, tgw := range page.TransitGateways {
			if tgw.State == ec2types.TransitGatewayStateDeleted || tgw.State == ec2types.TransitGatewayStateDeleting {
				continue
			}
			id := aws.ToString(tgw.TransitGatewayId)
			cr := &awsv1alpha1.TransitGateway{
				ObjectMeta: export.ObjectMeta(ec2NameTag(tgw.Tags, id), opts),
				Spec: awsv1alpha1.TransitGatewaySpec{
					Description: aws.ToString(tgw.Description),
					Tags:        ec2TagMap(tgw.Tags),
				},
			}
			if o := tgw.Options; o != nil {
				cr.Spec.AmazonSideASN = aws.ToInt64(o.AmazonSideAsn)
				cr.Spec.AutoAcceptSharedAttachments = o.AutoAcceptSharedAttachments == ec2types.AutoAcceptSharedAttachmentsValueEnable
				cr.Spec.DefaultRouteTableAssociation = o.DefaultRouteTableAssociation == ec2types.DefaultRouteTableAssociationValueEnable
				cr.Spec.DefaultRouteTablePropagation = o.DefaultRouteTablePropagation == ec2types.DefaultRouteTablePropagationValueEnable
				cr.Spec.DNSSupport = o.DnsSupport == ec2types.DnsSupportValueEnable
				cr.Spec.VPNECMPSupport = o.VpnEcmpSupport == ec2types.VpnEcmpSupportValueEnable
			}
			opts.Index.Add(id, cr.Name)
			opts.Index.Add(aws.ToString(tgw.TransitGatewayArn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportTGWVpcAttachments(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsec2.NewDescribeTransitGatewayVpcAttachmentsPaginator(clients.EC2, &awsec2.DescribeTransitGatewayVpcAttachmentsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe transit gateway vpc attachments: %w", err)
		}
		for _, att := range page.TransitGatewayVpcAttachments {
			if att.State == ec2types.TransitGatewayAttachmentStateDeleted || att.State == ec2types.TransitGatewayAttachmentStateDeleting {
				continue
			}
			id := aws.ToString(att.TransitGatewayAttachmentId)
			cr := &awsv1alpha1.TransitGatewayVpcAttachment{
				ObjectMeta: export.ObjectMeta(ec2NameTag(att.Tags, id), opts),
				Spec: awsv1alpha1.TransitGatewayVpcAttachmentSpec{
					VPCRef: vpcRefFor(aws.ToString(att.VpcId), opts),
					Tags:   ec2TagMap(att.Tags),
				},
			}
			tgwID := aws.ToString(att.TransitGatewayId)
			if crName, ok := opts.Index.Lookup(tgwID); ok {
				cr.Spec.TransitGatewayRef = awsv1alpha1.TransitGatewayRef{Name: crName}
			} else {
				cr.Spec.TransitGatewayRef = awsv1alpha1.TransitGatewayRef{TransitGatewayID: tgwID}
			}
			for _, subnetID := range att.SubnetIds {
				if crName, ok := opts.Index.Lookup(subnetID); ok {
					cr.Spec.SubnetRefs = append(cr.Spec.SubnetRefs, awsv1alpha1.SubnetRef{Name: crName})
				} else {
					cr.Spec.SubnetRefs = append(cr.Spec.SubnetRefs, awsv1alpha1.SubnetRef{ID: subnetID})
				}
			}
			if o := att.Options; o != nil {
				cr.Spec.DNSSupport = o.DnsSupport == ec2types.DnsSupportValueEnable
				cr.Spec.IPv6Support = o.Ipv6Support == ec2types.Ipv6SupportValueEnable
				cr.Spec.ApplianceModeSupport = o.ApplianceModeSupport == ec2types.ApplianceModeSupportValueEnable
			}
			opts.Index.Add(id, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}
