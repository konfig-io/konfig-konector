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

// ClusterAPI is the narrow subset of the EKS SDK client used by the cluster
// helpers. *eks.Client satisfies it, so existing callers keep compiling.
type ClusterAPI interface {
	DescribeCluster(ctx context.Context, params *eks.DescribeClusterInput, optFns ...func(*eks.Options)) (*eks.DescribeClusterOutput, error)
	CreateCluster(ctx context.Context, params *eks.CreateClusterInput, optFns ...func(*eks.Options)) (*eks.CreateClusterOutput, error)
	UpdateClusterConfig(ctx context.Context, params *eks.UpdateClusterConfigInput, optFns ...func(*eks.Options)) (*eks.UpdateClusterConfigOutput, error)
	UpdateClusterVersion(ctx context.Context, params *eks.UpdateClusterVersionInput, optFns ...func(*eks.Options)) (*eks.UpdateClusterVersionOutput, error)
	DeleteCluster(ctx context.Context, params *eks.DeleteClusterInput, optFns ...func(*eks.Options)) (*eks.DeleteClusterOutput, error)
}

func DescribeCluster(ctx context.Context, c ClusterAPI, name string) (*types.Cluster, error) {
	out, err := c.DescribeCluster(ctx, &eks.DescribeClusterInput{Name: aws.String(name)})
	if err != nil {
		return nil, err
	}
	return out.Cluster, nil
}

type CreateClusterInput struct {
	Name                  string
	Version               string
	RoleArn               string
	SubnetIDs             []string
	SecurityGroupIDs      []string
	EndpointPublicAccess  *bool
	EndpointPrivateAccess *bool
	PublicAccessCidrs     []string
	LogTypes              []string
	EncryptionProviderArn string
	EncryptionResources   []string
	AuthenticationMode    types.AuthenticationMode
	BootstrapAdminPerms   *bool
	Tags                  map[string]string

	KubernetesServiceIPv4CIDR  string
	KubernetesIPFamily         string
	UpgradePolicySupportType   string
	BootstrapSelfManagedAddons *bool
}

func CreateCluster(ctx context.Context, c ClusterAPI, in CreateClusterInput) (*types.Cluster, error) {
	input := &eks.CreateClusterInput{
		Name:    aws.String(in.Name),
		RoleArn: aws.String(in.RoleArn),
		ResourcesVpcConfig: &types.VpcConfigRequest{
			SubnetIds:             in.SubnetIDs,
			SecurityGroupIds:      in.SecurityGroupIDs,
			EndpointPublicAccess:  in.EndpointPublicAccess,
			EndpointPrivateAccess: in.EndpointPrivateAccess,
			PublicAccessCidrs:     in.PublicAccessCidrs,
		},
		Tags: in.Tags,
	}
	if in.Version != "" {
		input.Version = aws.String(in.Version)
	}
	if len(in.LogTypes) > 0 {
		enabled := make([]types.LogType, 0, len(in.LogTypes))
		for _, lt := range in.LogTypes {
			enabled = append(enabled, types.LogType(lt))
		}
		input.Logging = &types.Logging{
			ClusterLogging: []types.LogSetup{
				{Types: enabled, Enabled: aws.Bool(true)},
			},
		}
	}
	if in.EncryptionProviderArn != "" {
		input.EncryptionConfig = []types.EncryptionConfig{
			{
				Provider:  &types.Provider{KeyArn: aws.String(in.EncryptionProviderArn)},
				Resources: in.EncryptionResources,
			},
		}
	}
	if in.AuthenticationMode != "" || in.BootstrapAdminPerms != nil {
		input.AccessConfig = &types.CreateAccessConfigRequest{
			AuthenticationMode:                      in.AuthenticationMode,
			BootstrapClusterCreatorAdminPermissions: in.BootstrapAdminPerms,
		}
	}
	if in.KubernetesServiceIPv4CIDR != "" || in.KubernetesIPFamily != "" {
		nc := &types.KubernetesNetworkConfigRequest{}
		if in.KubernetesServiceIPv4CIDR != "" {
			nc.ServiceIpv4Cidr = aws.String(in.KubernetesServiceIPv4CIDR)
		}
		if in.KubernetesIPFamily != "" {
			nc.IpFamily = types.IpFamily(in.KubernetesIPFamily)
		}
		input.KubernetesNetworkConfig = nc
	}
	if in.UpgradePolicySupportType != "" {
		input.UpgradePolicy = &types.UpgradePolicyRequest{
			SupportType: types.SupportType(in.UpgradePolicySupportType),
		}
	}
	if in.BootstrapSelfManagedAddons != nil {
		input.BootstrapSelfManagedAddons = in.BootstrapSelfManagedAddons
	}
	out, err := c.CreateCluster(ctx, input)
	if err != nil {
		return nil, err
	}
	return out.Cluster, nil
}

func UpdateClusterConfig(ctx context.Context, c ClusterAPI, name string, in CreateClusterInput) error {
	input := &eks.UpdateClusterConfigInput{
		Name: aws.String(name),
		ResourcesVpcConfig: &types.VpcConfigRequest{
			SubnetIds:             in.SubnetIDs,
			SecurityGroupIds:      in.SecurityGroupIDs,
			EndpointPublicAccess:  in.EndpointPublicAccess,
			EndpointPrivateAccess: in.EndpointPrivateAccess,
			PublicAccessCidrs:     in.PublicAccessCidrs,
		},
	}
	if len(in.LogTypes) > 0 {
		enabled := make([]types.LogType, 0, len(in.LogTypes))
		for _, lt := range in.LogTypes {
			enabled = append(enabled, types.LogType(lt))
		}
		input.Logging = &types.Logging{
			ClusterLogging: []types.LogSetup{
				{Types: enabled, Enabled: aws.Bool(true)},
			},
		}
	}
	if in.AuthenticationMode != "" {
		input.AccessConfig = &types.UpdateAccessConfigRequest{
			AuthenticationMode: in.AuthenticationMode,
		}
	}
	if in.UpgradePolicySupportType != "" {
		input.UpgradePolicy = &types.UpgradePolicyRequest{
			SupportType: types.SupportType(in.UpgradePolicySupportType),
		}
	}
	_, err := c.UpdateClusterConfig(ctx, input)
	return err
}

func UpdateClusterVersion(ctx context.Context, c ClusterAPI, name, version string) error {
	_, err := c.UpdateClusterVersion(ctx, &eks.UpdateClusterVersionInput{
		Name:    aws.String(name),
		Version: aws.String(version),
	})
	return err
}

func DeleteCluster(ctx context.Context, c ClusterAPI, name string) error {
	_, err := c.DeleteCluster(ctx, &eks.DeleteClusterInput{Name: aws.String(name)})
	return err
}

func TagCluster(ctx context.Context, c *eks.Client, arn string, tags map[string]string) error {
	if len(tags) == 0 {
		return nil
	}
	_, err := c.TagResource(ctx, &eks.TagResourceInput{ResourceArn: aws.String(arn), Tags: tags})
	return err
}
