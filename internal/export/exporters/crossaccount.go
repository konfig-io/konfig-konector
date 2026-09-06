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
	awsram "github.com/aws/aws-sdk-go-v2/service/ram"
	ramtypes "github.com/aws/aws-sdk-go-v2/service/ram/types"
	awsroute53 "github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	awsclient "github.com/konfig-io/konfig-konector/internal/aws"
	"github.com/konfig-io/konfig-konector/internal/export"
)

func init() {
	// After LoadBalancers (elb) so NLB refs resolve to CR names.
	export.Register(export.Exporter{Kind: "VPCEndpointService", Service: "ec2", Order: 45, Fn: exportVPCEndpointServices})
	// After HostedZones so zone refs resolve.
	export.Register(export.Exporter{Kind: "HostedZoneVPCAssociation", Service: "route53", Order: 46, Fn: exportHostedZoneVPCAssociations})
	export.Register(export.Exporter{Kind: "ResourceShareInvitation", Service: "ram", Order: 46, Fn: exportResourceShareInvitations})
}

func exportVPCEndpointServices(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsec2.NewDescribeVpcEndpointServiceConfigurationsPaginator(clients.EC2, &awsec2.DescribeVpcEndpointServiceConfigurationsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe endpoint services: %w", err)
		}
		for _, cfg := range page.ServiceConfigurations {
			if cfg.ServiceState == ec2types.ServiceStateDeleted || cfg.ServiceState == ec2types.ServiceStateDeleting {
				continue
			}
			id := aws.ToString(cfg.ServiceId)
			cr := &awsv1alpha1.VPCEndpointService{
				ObjectMeta: export.ObjectMeta(id, opts),
				Spec: awsv1alpha1.VPCEndpointServiceSpec{
					AcceptanceRequired:      aws.ToBool(cfg.AcceptanceRequired),
					GatewayLoadBalancerArns: cfg.GatewayLoadBalancerArns,
					PrivateDNSName:          aws.ToString(cfg.PrivateDnsName),
					SupportedIPAddressTypes: ipTypesToStrings(cfg.SupportedIpAddressTypes),
					Tags:                    tagMapEC2(cfg.Tags),
				},
			}
			cr.TypeMeta.Kind, cr.TypeMeta.APIVersion = "VPCEndpointService", awsv1alpha1.GroupVersion.String()
			for _, arn := range cfg.NetworkLoadBalancerArns {
				if name, ok := opts.Index.Lookup(arn); ok {
					cr.Spec.NetworkLoadBalancerRefs = append(cr.Spec.NetworkLoadBalancerRefs, awsv1alpha1.LoadBalancerRef{Name: name})
				} else {
					cr.Spec.NetworkLoadBalancerRefs = append(cr.Spec.NetworkLoadBalancerRefs, awsv1alpha1.LoadBalancerRef{ARN: arn})
				}
			}
			perm, err := clients.EC2.DescribeVpcEndpointServicePermissions(ctx, &awsec2.DescribeVpcEndpointServicePermissionsInput{ServiceId: cfg.ServiceId})
			if err == nil {
				for _, ap := range perm.AllowedPrincipals {
					cr.Spec.AllowedPrincipals = append(cr.Spec.AllowedPrincipals, aws.ToString(ap.Principal))
				}
			}
			opts.Index.Add(id, cr.Name)
			opts.Index.Add(aws.ToString(cfg.ServiceName), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func ipTypesToStrings(in []ec2types.ServiceConnectivityType) []string {
	out := make([]string, 0, len(in))
	for _, t := range in {
		out = append(out, string(t))
	}
	return out
}

func tagMapEC2(tags []ec2types.Tag) map[string]string {
	if len(tags) == 0 {
		return nil
	}
	m := map[string]string{}
	for _, t := range tags {
		k := aws.ToString(t.Key)
		if strings.HasPrefix(k, "aws:") {
			continue
		}
		m[k] = aws.ToString(t.Value)
	}
	return m
}

// exportHostedZoneVPCAssociations emits one association per VPC attached to a
// private zone beyond the first (the first is modelled by HostedZone.spec.vpcRef).
func exportHostedZoneVPCAssociations(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsroute53.NewListHostedZonesPaginator(clients.Route53, &awsroute53.ListHostedZonesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list hosted zones: %w", err)
		}
		for _, z := range page.HostedZones {
			if z.Config == nil || !z.Config.PrivateZone {
				continue
			}
			zoneID := strings.TrimPrefix(aws.ToString(z.Id), "/hostedzone/")
			zone, err := clients.Route53.GetHostedZone(ctx, &awsroute53.GetHostedZoneInput{Id: aws.String(zoneID)})
			if err != nil {
				continue
			}
			for i, v := range zone.VPCs {
				if i == 0 {
					continue
				}
				vpcID := aws.ToString(v.VPCId)
				cr := &awsv1alpha1.HostedZoneVPCAssociation{
					ObjectMeta: export.ObjectMeta(zoneID+"-"+vpcID, opts),
					Spec: awsv1alpha1.HostedZoneVPCAssociationSpec{
						VPCRef:    vpcRefFor(vpcID, opts),
						VPCRegion: string(v.VPCRegion),
					},
				}
				cr.TypeMeta.Kind, cr.TypeMeta.APIVersion = "HostedZoneVPCAssociation", awsv1alpha1.GroupVersion.String()
				if name, ok := opts.Index.Lookup(zoneID); ok {
					cr.Spec.HostedZoneRef = awsv1alpha1.HostedZoneIDRef{Name: name}
				} else {
					cr.Spec.HostedZoneRef = awsv1alpha1.HostedZoneIDRef{ID: zoneID}
				}
				objs = append(objs, cr)
			}
		}
	}
	return objs, nil
}

var _ route53types.VPC

func exportResourceShareInvitations(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	var token *string
	for {
		out, err := clients.RAM.GetResourceShareInvitations(ctx, &awsram.GetResourceShareInvitationsInput{NextToken: token})
		if err != nil {
			return nil, fmt.Errorf("get resource share invitations: %w", err)
		}
		for _, inv := range out.ResourceShareInvitations {
			if inv.Status != ramtypes.ResourceShareInvitationStatusAccepted {
				continue
			}
			arn := aws.ToString(inv.ResourceShareArn)
			name := arn[strings.LastIndex(arn, "/")+1:]
			cr := &awsv1alpha1.ResourceShareInvitation{
				ObjectMeta: export.ObjectMeta(aws.ToString(inv.ResourceShareName)+"-"+name[:min(8, len(name))], opts),
				Spec:       awsv1alpha1.ResourceShareInvitationSpec{ResourceShareRef: awsv1alpha1.ResourceShareRef{ARN: arn}},
			}
			cr.TypeMeta.Kind, cr.TypeMeta.APIVersion = "ResourceShareInvitation", awsv1alpha1.GroupVersion.String()
			objs = append(objs, cr)
		}
		if out.NextToken == nil {
			return objs, nil
		}
		token = out.NextToken
	}
}
