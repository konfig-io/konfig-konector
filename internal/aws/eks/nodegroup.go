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

package eks

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/eks/types"
)

func DescribeNodegroup(ctx context.Context, c *eks.Client, clusterName, nodegroupName string) (*types.Nodegroup, error) {
	out, err := c.DescribeNodegroup(ctx, &eks.DescribeNodegroupInput{
		ClusterName:   aws.String(clusterName),
		NodegroupName: aws.String(nodegroupName),
	})
	if err != nil {
		return nil, err
	}
	return out.Nodegroup, nil
}

type CreateNodegroupInput struct {
	ClusterName        string
	NodegroupName      string
	NodeRoleArn        string
	SubnetIDs          []string
	ScalingMin         int32
	ScalingMax         int32
	ScalingDesired     int32
	InstanceTypes      []string
	AmiType            types.AMITypes
	CapacityType       types.CapacityTypes
	DiskSize           *int32
	Labels             map[string]string
	Taints             []types.Taint
	MaxUnavailable     *int32
	MaxUnavailablePct  *int32
	LaunchTemplateID   string
	LaunchTemplateName string
	LaunchTemplateVer  string
	ReleaseVersion     string
	Version            string
	RemoteAccessKey    string
	RemoteAccessSGIDs  []string
	Tags               map[string]string
	NodeRepairEnabled  *bool
}

func CreateNodegroup(ctx context.Context, c *eks.Client, in CreateNodegroupInput) (*types.Nodegroup, error) {
	input := &eks.CreateNodegroupInput{
		ClusterName:   aws.String(in.ClusterName),
		NodegroupName: aws.String(in.NodegroupName),
		NodeRole:      aws.String(in.NodeRoleArn),
		Subnets:       in.SubnetIDs,
		ScalingConfig: &types.NodegroupScalingConfig{
			MinSize:     aws.Int32(in.ScalingMin),
			MaxSize:     aws.Int32(in.ScalingMax),
			DesiredSize: aws.Int32(in.ScalingDesired),
		},
		Tags: in.Tags,
	}
	if len(in.InstanceTypes) > 0 {
		input.InstanceTypes = in.InstanceTypes
	}
	if in.AmiType != "" {
		input.AmiType = in.AmiType
	}
	if in.CapacityType != "" {
		input.CapacityType = in.CapacityType
	}
	if in.DiskSize != nil {
		input.DiskSize = in.DiskSize
	}
	if len(in.Labels) > 0 {
		input.Labels = in.Labels
	}
	if len(in.Taints) > 0 {
		input.Taints = in.Taints
	}
	if in.MaxUnavailable != nil || in.MaxUnavailablePct != nil {
		input.UpdateConfig = &types.NodegroupUpdateConfig{
			MaxUnavailable:           in.MaxUnavailable,
			MaxUnavailablePercentage: in.MaxUnavailablePct,
		}
	}
	if in.LaunchTemplateID != "" || in.LaunchTemplateName != "" {
		lt := &types.LaunchTemplateSpecification{}
		if in.LaunchTemplateID != "" {
			lt.Id = aws.String(in.LaunchTemplateID)
		}
		if in.LaunchTemplateName != "" {
			lt.Name = aws.String(in.LaunchTemplateName)
		}
		if in.LaunchTemplateVer != "" {
			lt.Version = aws.String(in.LaunchTemplateVer)
		}
		input.LaunchTemplate = lt
	}
	if in.ReleaseVersion != "" {
		input.ReleaseVersion = aws.String(in.ReleaseVersion)
	}
	if in.Version != "" {
		input.Version = aws.String(in.Version)
	}
	if in.RemoteAccessKey != "" {
		input.RemoteAccess = &types.RemoteAccessConfig{
			Ec2SshKey:            aws.String(in.RemoteAccessKey),
			SourceSecurityGroups: in.RemoteAccessSGIDs,
		}
	}
	out, err := c.CreateNodegroup(ctx, input)
	if err != nil {
		return nil, err
	}
	return out.Nodegroup, nil
}

func UpdateNodegroupConfig(ctx context.Context, c *eks.Client, clusterName, nodegroupName string, in CreateNodegroupInput) error {
	input := &eks.UpdateNodegroupConfigInput{
		ClusterName:   aws.String(clusterName),
		NodegroupName: aws.String(nodegroupName),
		ScalingConfig: &types.NodegroupScalingConfig{
			MinSize:     aws.Int32(in.ScalingMin),
			MaxSize:     aws.Int32(in.ScalingMax),
			DesiredSize: aws.Int32(in.ScalingDesired),
		},
	}
	if len(in.Labels) > 0 {
		input.Labels = &types.UpdateLabelsPayload{AddOrUpdateLabels: in.Labels}
	}
	if len(in.Taints) > 0 {
		input.Taints = &types.UpdateTaintsPayload{AddOrUpdateTaints: in.Taints}
	}
	if in.MaxUnavailable != nil || in.MaxUnavailablePct != nil {
		input.UpdateConfig = &types.NodegroupUpdateConfig{
			MaxUnavailable:           in.MaxUnavailable,
			MaxUnavailablePercentage: in.MaxUnavailablePct,
		}
	}
	_, err := c.UpdateNodegroupConfig(ctx, input)
	return err
}

func DeleteNodegroup(ctx context.Context, c *eks.Client, clusterName, nodegroupName string) error {
	_, err := c.DeleteNodegroup(ctx, &eks.DeleteNodegroupInput{
		ClusterName:   aws.String(clusterName),
		NodegroupName: aws.String(nodegroupName),
	})
	return err
}
