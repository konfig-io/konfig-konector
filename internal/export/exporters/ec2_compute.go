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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	awsclient "github.com/konfig-io/konfig-konector/internal/aws"
	"github.com/konfig-io/konfig-konector/internal/export"
)

func init() {
	// Standalone compute resources first so referrers (instances, spot
	// fleets) can resolve them via the index.
	export.Register(export.Exporter{Kind: "KeyPair", Service: "ec2", Order: 20, Fn: exportKeyPairs})
	export.Register(export.Exporter{Kind: "PlacementGroup", Service: "ec2", Order: 20, Fn: exportPlacementGroups})
	export.Register(export.Exporter{Kind: "AMI", Service: "ec2", Order: 20, Fn: exportAMIs})
	export.Register(export.Exporter{Kind: "EBSVolume", Service: "ec2", Order: 20, Fn: exportEBSVolumes})
	export.Register(export.Exporter{Kind: "ElasticIP", Service: "ec2", Order: 20, Fn: exportElasticIPs})
	export.Register(export.Exporter{Kind: "LaunchTemplate", Service: "ec2", Order: 21, Fn: exportLaunchTemplates})
	export.Register(export.Exporter{Kind: "EC2Instance", Service: "ec2", Order: 22, Fn: exportEC2Instances})
	export.Register(export.Exporter{Kind: "SpotFleet", Service: "ec2", Order: 23, Fn: exportSpotFleets})
}

func exportKeyPairs(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	// DescribeKeyPairs is not a paginated API.
	out, err := clients.EC2.DescribeKeyPairs(ctx, &awsec2.DescribeKeyPairsInput{})
	if err != nil {
		return nil, fmt.Errorf("describe key pairs: %w", err)
	}
	var objs []client.Object
	for _, kp := range out.KeyPairs {
		name := aws.ToString(kp.KeyName)
		cr := &awsv1alpha1.KeyPair{
			ObjectMeta: export.ObjectMeta(name, opts),
			Spec: awsv1alpha1.KeyPairSpec{
				KeyName: name,
				KeyType: string(kp.KeyType),
				Tags:    ec2TagMap(kp.Tags),
			},
		}
		opts.Index.Add(aws.ToString(kp.KeyPairId), cr.Name)
		opts.Index.Add(name, cr.Name)
		objs = append(objs, cr)
	}
	return objs, nil
}

func exportPlacementGroups(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	// DescribePlacementGroups is not a paginated API.
	out, err := clients.EC2.DescribePlacementGroups(ctx, &awsec2.DescribePlacementGroupsInput{})
	if err != nil {
		return nil, fmt.Errorf("describe placement groups: %w", err)
	}
	var objs []client.Object
	for _, pg := range out.PlacementGroups {
		name := aws.ToString(pg.GroupName)
		cr := &awsv1alpha1.PlacementGroup{
			ObjectMeta: export.ObjectMeta(name, opts),
			Spec: awsv1alpha1.PlacementGroupSpec{
				GroupName:      name,
				Strategy:       string(pg.Strategy),
				PartitionCount: aws.ToInt32(pg.PartitionCount),
				SpreadLevel:    string(pg.SpreadLevel),
				Tags:           ec2TagMap(pg.Tags),
			},
		}
		opts.Index.Add(aws.ToString(pg.GroupId), cr.Name)
		opts.Index.Add(name, cr.Name)
		objs = append(objs, cr)
	}
	return objs, nil
}

func exportAMIs(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsec2.NewDescribeImagesPaginator(clients.EC2, &awsec2.DescribeImagesInput{
		Owners: []string{"self"},
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe images: %w", err)
		}
		for _, img := range page.Images {
			id := aws.ToString(img.ImageId)
			name := aws.ToString(img.Name)
			if name == "" {
				name = id
			}
			cr := &awsv1alpha1.AMI{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.AMISpec{
					Name:               aws.ToString(img.Name),
					Description:        aws.ToString(img.Description),
					Architecture:       string(img.Architecture),
					RootDeviceName:     aws.ToString(img.RootDeviceName),
					VirtualizationType: string(img.VirtualizationType),
					KernelID:           aws.ToString(img.KernelId),
					RamdiskID:          aws.ToString(img.RamdiskId),
					SRIOVNetSupport:    aws.ToString(img.SriovNetSupport),
					ENASupport:         aws.ToBool(img.EnaSupport),
					Tags:               ec2TagMap(img.Tags),
				},
			}
			for _, bdm := range img.BlockDeviceMappings {
				m := awsv1alpha1.AMIBlockDeviceMapping{
					DeviceName: aws.ToString(bdm.DeviceName),
				}
				if bdm.Ebs != nil {
					m.SnapshotID = aws.ToString(bdm.Ebs.SnapshotId)
					m.VolumeSize = aws.ToInt32(bdm.Ebs.VolumeSize)
					m.VolumeType = string(bdm.Ebs.VolumeType)
					m.DeleteOnTermination = aws.ToBool(bdm.Ebs.DeleteOnTermination)
					m.Encrypted = aws.ToBool(bdm.Ebs.Encrypted)
				}
				cr.Spec.BlockDeviceMappings = append(cr.Spec.BlockDeviceMappings, m)
			}
			opts.Index.Add(id, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportEBSVolumes(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsec2.NewDescribeVolumesPaginator(clients.EC2, &awsec2.DescribeVolumesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe volumes: %w", err)
		}
		for _, v := range page.Volumes {
			id := aws.ToString(v.VolumeId)
			cr := &awsv1alpha1.EBSVolume{
				ObjectMeta: export.ObjectMeta(ec2NameTag(v.Tags, id), opts),
				Spec: awsv1alpha1.EBSVolumeSpec{
					AvailabilityZone:   aws.ToString(v.AvailabilityZone),
					VolumeType:         string(v.VolumeType),
					Size:               aws.ToInt32(v.Size),
					IOPS:               aws.ToInt32(v.Iops),
					Throughput:         aws.ToInt32(v.Throughput),
					Encrypted:          aws.ToBool(v.Encrypted),
					KMSKeyID:           aws.ToString(v.KmsKeyId),
					SnapshotID:         aws.ToString(v.SnapshotId),
					MultiAttachEnabled: aws.ToBool(v.MultiAttachEnabled),
					Tags:               ec2TagMap(v.Tags),
				},
			}
			opts.Index.Add(id, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportElasticIPs(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	// DescribeAddresses is not a paginated API.
	out, err := clients.EC2.DescribeAddresses(ctx, &awsec2.DescribeAddressesInput{})
	if err != nil {
		return nil, fmt.Errorf("describe addresses: %w", err)
	}
	var objs []client.Object
	for _, a := range out.Addresses {
		id := aws.ToString(a.AllocationId)
		cr := &awsv1alpha1.ElasticIP{
			ObjectMeta: export.ObjectMeta(ec2NameTag(a.Tags, id), opts),
			Spec: awsv1alpha1.ElasticIPSpec{
				Domain: string(a.Domain),
				Tags:   ec2TagMap(a.Tags),
			},
		}
		opts.Index.Add(id, cr.Name)
		opts.Index.Add(aws.ToString(a.PublicIp), cr.Name)
		objs = append(objs, cr)
	}
	return objs, nil
}

func exportLaunchTemplates(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsec2.NewDescribeLaunchTemplatesPaginator(clients.EC2, &awsec2.DescribeLaunchTemplatesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe launch templates: %w", err)
		}
		for _, lt := range page.LaunchTemplates {
			id := aws.ToString(lt.LaunchTemplateId)
			name := aws.ToString(lt.LaunchTemplateName)
			// The spec models the latest version's data.
			vOut, err := clients.EC2.DescribeLaunchTemplateVersions(ctx, &awsec2.DescribeLaunchTemplateVersionsInput{
				LaunchTemplateId: lt.LaunchTemplateId,
				Versions:         []string{"$Latest"},
			})
			if err != nil || len(vOut.LaunchTemplateVersions) == 0 {
				continue
			}
			data := vOut.LaunchTemplateVersions[0].LaunchTemplateData
			if data == nil {
				continue
			}
			cr := &awsv1alpha1.LaunchTemplate{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.LaunchTemplateSpec{
					LaunchTemplateName: name,
					ImageID:            aws.ToString(data.ImageId),
					InstanceType:       string(data.InstanceType),
					KeyName:            aws.ToString(data.KeyName),
					UserData:           aws.ToString(data.UserData),
					Tags:               ec2TagMap(lt.Tags),
				},
			}
			if data.IamInstanceProfile != nil {
				if n := aws.ToString(data.IamInstanceProfile.Name); n != "" {
					cr.Spec.IAMInstanceProfile = n
				} else {
					cr.Spec.IAMInstanceProfile = aws.ToString(data.IamInstanceProfile.Arn)
				}
			}
			for _, sgID := range data.SecurityGroupIds {
				if crName, ok := opts.Index.Lookup(sgID); ok {
					cr.Spec.SecurityGroupRefs = append(cr.Spec.SecurityGroupRefs, awsv1alpha1.SecurityGroupRef{Name: crName})
				} else {
					cr.Spec.SecurityGroupRefs = append(cr.Spec.SecurityGroupRefs, awsv1alpha1.SecurityGroupRef{ID: sgID})
				}
			}
			for _, bdm := range data.BlockDeviceMappings {
				m := awsv1alpha1.BlockDeviceMapping{
					DeviceName: aws.ToString(bdm.DeviceName),
				}
				if bdm.Ebs != nil {
					m.VolumeSize = aws.ToInt32(bdm.Ebs.VolumeSize)
					m.VolumeType = string(bdm.Ebs.VolumeType)
					m.Encrypted = aws.ToBool(bdm.Ebs.Encrypted)
				}
				cr.Spec.BlockDeviceMappings = append(cr.Spec.BlockDeviceMappings, m)
			}
			opts.Index.Add(id, cr.Name)
			opts.Index.Add(name, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportEC2Instances(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsec2.NewDescribeInstancesPaginator(clients.EC2, &awsec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{{
			Name:   aws.String("instance-state-name"),
			Values: []string{"running", "stopped"},
		}},
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe instances: %w", err)
		}
		for _, res := range page.Reservations {
			for _, inst := range res.Instances {
				id := aws.ToString(inst.InstanceId)
				cr := &awsv1alpha1.EC2Instance{
					ObjectMeta: export.ObjectMeta(ec2NameTag(inst.Tags, id), opts),
					Spec: awsv1alpha1.EC2InstanceSpec{
						ImageID:                  aws.ToString(inst.ImageId),
						InstanceType:             string(inst.InstanceType),
						KeyName:                  aws.ToString(inst.KeyName),
						AssociatePublicIPAddress: inst.PublicIpAddress != nil,
						Tags:                     ec2TagMap(inst.Tags),
					},
				}
				// SubnetRef is a plain CR name: only emit it when the subnet
				// was exported in this run.
				if crName, ok := opts.Index.Lookup(aws.ToString(inst.SubnetId)); ok {
					cr.Spec.SubnetRef = crName
				}
				for _, sg := range inst.SecurityGroups {
					sgID := aws.ToString(sg.GroupId)
					if crName, ok := opts.Index.Lookup(sgID); ok {
						cr.Spec.SecurityGroupRefs = append(cr.Spec.SecurityGroupRefs, awsv1alpha1.SecurityGroupRef{Name: crName})
					} else {
						cr.Spec.SecurityGroupRefs = append(cr.Spec.SecurityGroupRefs, awsv1alpha1.SecurityGroupRef{ID: sgID})
					}
				}
				if inst.IamInstanceProfile != nil {
					cr.Spec.IAMInstanceProfile = aws.ToString(inst.IamInstanceProfile.Arn)
				}
				// UserData is a per-instance attribute call; skip on error.
				attr, err := clients.EC2.DescribeInstanceAttribute(ctx, &awsec2.DescribeInstanceAttributeInput{
					InstanceId: inst.InstanceId,
					Attribute:  ec2types.InstanceAttributeNameUserData,
				})
				if err == nil && attr.UserData != nil {
					cr.Spec.UserData = aws.ToString(attr.UserData.Value)
				}
				opts.Index.Add(id, cr.Name)
				objs = append(objs, cr)
			}
		}
	}
	return objs, nil
}

func exportSpotFleets(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsec2.NewDescribeSpotFleetRequestsPaginator(clients.EC2, &awsec2.DescribeSpotFleetRequestsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe spot fleet requests: %w", err)
		}
		for _, sfr := range page.SpotFleetRequestConfigs {
			if sfr.SpotFleetRequestState != ec2types.BatchStateActive {
				continue
			}
			cfg := sfr.SpotFleetRequestConfig
			if cfg == nil {
				continue
			}
			// The spec models classic launch specifications only; fleets
			// defined via launch template configs cannot be represented.
			if len(cfg.LaunchSpecifications) == 0 {
				continue
			}
			id := aws.ToString(sfr.SpotFleetRequestId)
			cr := &awsv1alpha1.SpotFleet{
				ObjectMeta: export.ObjectMeta(ec2NameTag(sfr.Tags, id), opts),
				Spec: awsv1alpha1.SpotFleetSpec{
					IAMFleetRole:                     aws.ToString(cfg.IamFleetRole),
					TargetCapacity:                   aws.ToInt32(cfg.TargetCapacity),
					AllocationStrategy:               string(cfg.AllocationStrategy),
					SpotPrice:                        aws.ToString(cfg.SpotPrice),
					TerminateInstancesWithExpiration: aws.ToBool(cfg.TerminateInstancesWithExpiration),
					Tags:                             ec2TagMap(sfr.Tags),
				},
			}
			if cfg.ValidUntil != nil {
				t := metav1.NewTime(*cfg.ValidUntil)
				cr.Spec.ValidUntil = &t
			}
			for _, ls := range cfg.LaunchSpecifications {
				spec := awsv1alpha1.SpotFleetLaunchSpec{
					ImageID:          aws.ToString(ls.ImageId),
					InstanceType:     string(ls.InstanceType),
					SubnetID:         aws.ToString(ls.SubnetId),
					KeyName:          aws.ToString(ls.KeyName),
					SpotPrice:        aws.ToString(ls.SpotPrice),
					WeightedCapacity: aws.ToFloat64(ls.WeightedCapacity),
				}
				for _, g := range ls.SecurityGroups {
					if gid := aws.ToString(g.GroupId); gid != "" {
						spec.SecurityGroupIDs = append(spec.SecurityGroupIDs, gid)
					}
				}
				cr.Spec.LaunchSpecs = append(cr.Spec.LaunchSpecs, spec)
			}
			opts.Index.Add(id, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}
