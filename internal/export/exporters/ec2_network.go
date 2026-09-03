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
	// VPCs must be indexed before anything that references them.
	export.Register(export.Exporter{Kind: "VPC", Service: "ec2", Order: 10, Fn: exportVPCs})
	export.Register(export.Exporter{Kind: "Subnet", Service: "ec2", Order: 11, Fn: exportSubnets})
}

// ec2TagMap converts EC2 tag slices to a spec tag map, dropping aws: tags.
func ec2TagMap(tags []ec2types.Tag) map[string]string {
	if len(tags) == 0 {
		return nil
	}
	m := make(map[string]string, len(tags))
	for _, t := range tags {
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return export.TagMap(m)
}

// ec2NameTag returns the Name tag value, or the fallback if unset.
func ec2NameTag(tags []ec2types.Tag, fallback string) string {
	for _, t := range tags {
		if aws.ToString(t.Key) == "Name" && aws.ToString(t.Value) != "" {
			return aws.ToString(t.Value)
		}
	}
	return fallback
}

func exportVPCs(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsec2.NewDescribeVpcsPaginator(clients.EC2, &awsec2.DescribeVpcsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe vpcs: %w", err)
		}
		for _, v := range page.Vpcs {
			id := aws.ToString(v.VpcId)
			vpc := &awsv1alpha1.VPC{
				ObjectMeta: export.ObjectMeta(ec2NameTag(v.Tags, id), opts),
				Spec: awsv1alpha1.VPCSpec{
					CIDRBlock:       aws.ToString(v.CidrBlock),
					InstanceTenancy: string(v.InstanceTenancy),
					Tags:            ec2TagMap(v.Tags),
				},
			}
			// DNS attributes are per-attribute Describe calls.
			for _, attr := range []ec2types.VpcAttributeName{
				ec2types.VpcAttributeNameEnableDnsSupport,
				ec2types.VpcAttributeNameEnableDnsHostnames,
			} {
				out, err := clients.EC2.DescribeVpcAttribute(ctx, &awsec2.DescribeVpcAttributeInput{
					VpcId: v.VpcId, Attribute: attr,
				})
				if err != nil {
					continue
				}
				switch attr {
				case ec2types.VpcAttributeNameEnableDnsSupport:
					if out.EnableDnsSupport != nil {
						vpc.Spec.EnableDNSSupport = out.EnableDnsSupport.Value
					}
				case ec2types.VpcAttributeNameEnableDnsHostnames:
					if out.EnableDnsHostnames != nil {
						vpc.Spec.EnableDNSHostnames = out.EnableDnsHostnames.Value
					}
				}
			}
			// Secondary CIDRs: the first association is the primary block.
			for _, assoc := range v.CidrBlockAssociationSet {
				cidr := aws.ToString(assoc.CidrBlock)
				if cidr != vpc.Spec.CIDRBlock {
					vpc.Spec.SecondaryIPv4CIDRs = append(vpc.Spec.SecondaryIPv4CIDRs, cidr)
				}
			}
			opts.Index.Add(id, vpc.Name)
			objs = append(objs, vpc)
		}
	}
	return objs, nil
}

func exportSubnets(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsec2.NewDescribeSubnetsPaginator(clients.EC2, &awsec2.DescribeSubnetsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe subnets: %w", err)
		}
		for _, s := range page.Subnets {
			id := aws.ToString(s.SubnetId)
			sn := &awsv1alpha1.Subnet{
				ObjectMeta: export.ObjectMeta(ec2NameTag(s.Tags, id), opts),
				Spec: awsv1alpha1.SubnetSpec{
					CIDRBlock:           aws.ToString(s.CidrBlock),
					AvailabilityZone:    aws.ToString(s.AvailabilityZone),
					MapPublicIPOnLaunch: aws.ToBool(s.MapPublicIpOnLaunch),
					Tags:                ec2TagMap(s.Tags),
				},
			}
			// Prefer a CR-name ref when the VPC was exported in this run;
			// fall back to the raw AWS ID for unmanaged/foreign VPCs.
			vpcID := aws.ToString(s.VpcId)
			if crName, ok := opts.Index.Lookup(vpcID); ok {
				sn.Spec.VPCRef = awsv1alpha1.VPCResourceRef{Name: crName}
			} else {
				sn.Spec.VPCRef = awsv1alpha1.VPCResourceRef{ID: vpcID}
			}
			opts.Index.Add(id, sn.Name)
			objs = append(objs, sn)
		}
	}
	return objs, nil
}
