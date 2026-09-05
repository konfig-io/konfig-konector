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

	"github.com/konfig-io/konfig-konector/internal/aws/multi"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/eks/types"
)

func DescribeAddon(ctx context.Context, c *multi.EKS, clusterName, addonName string) (*types.Addon, error) {
	out, err := c.DescribeAddon(ctx, &eks.DescribeAddonInput{
		ClusterName: aws.String(clusterName),
		AddonName:   aws.String(addonName),
	})
	if err != nil {
		return nil, err
	}
	return out.Addon, nil
}

type AddonInput struct {
	ClusterName         string
	AddonName           string
	AddonVersion        string
	ServiceAccountRole  string
	ResolveConflicts    types.ResolveConflicts
	ConfigurationValues string
	Tags                map[string]string
}

func CreateAddon(ctx context.Context, c *multi.EKS, in AddonInput) (*types.Addon, error) {
	input := &eks.CreateAddonInput{
		ClusterName: aws.String(in.ClusterName),
		AddonName:   aws.String(in.AddonName),
		Tags:        in.Tags,
	}
	if in.AddonVersion != "" {
		input.AddonVersion = aws.String(in.AddonVersion)
	}
	if in.ServiceAccountRole != "" {
		input.ServiceAccountRoleArn = aws.String(in.ServiceAccountRole)
	}
	if in.ResolveConflicts != "" {
		input.ResolveConflicts = in.ResolveConflicts
	}
	if in.ConfigurationValues != "" {
		input.ConfigurationValues = aws.String(in.ConfigurationValues)
	}
	out, err := c.CreateAddon(ctx, input)
	if err != nil {
		return nil, err
	}
	return out.Addon, nil
}

func UpdateAddon(ctx context.Context, c *multi.EKS, in AddonInput) error {
	input := &eks.UpdateAddonInput{
		ClusterName: aws.String(in.ClusterName),
		AddonName:   aws.String(in.AddonName),
	}
	if in.AddonVersion != "" {
		input.AddonVersion = aws.String(in.AddonVersion)
	}
	if in.ServiceAccountRole != "" {
		input.ServiceAccountRoleArn = aws.String(in.ServiceAccountRole)
	}
	if in.ResolveConflicts != "" {
		input.ResolveConflicts = in.ResolveConflicts
	}
	if in.ConfigurationValues != "" {
		input.ConfigurationValues = aws.String(in.ConfigurationValues)
	}
	_, err := c.UpdateAddon(ctx, input)
	return err
}

func DeleteAddon(ctx context.Context, c *multi.EKS, clusterName, addonName string) error {
	_, err := c.DeleteAddon(ctx, &eks.DeleteAddonInput{
		ClusterName: aws.String(clusterName),
		AddonName:   aws.String(addonName),
	})
	return err
}
