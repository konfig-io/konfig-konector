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

// Package ecs provides helper functions for AWS ECS operations.
package ecs

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/smithy-go"
)

// ServiceAPI is the narrow ECS client surface needed by the service helpers.
// *ecs.Client satisfies it, so callers passing a real client are unaffected.
type ServiceAPI interface {
	DescribeServices(ctx context.Context, params *ecs.DescribeServicesInput, optFns ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error)
	CreateService(ctx context.Context, params *ecs.CreateServiceInput, optFns ...func(*ecs.Options)) (*ecs.CreateServiceOutput, error)
	UpdateService(ctx context.Context, params *ecs.UpdateServiceInput, optFns ...func(*ecs.Options)) (*ecs.UpdateServiceOutput, error)
	DeleteService(ctx context.Context, params *ecs.DeleteServiceInput, optFns ...func(*ecs.Options)) (*ecs.DeleteServiceOutput, error)
}

// IsNotFound returns true when the error indicates the resource does not exist.
func IsNotFound(err error) bool {
	var nfe *types.ClusterNotFoundException
	if errors.As(err, &nfe) {
		return true
	}
	var snfe *types.ServiceNotFoundException
	if errors.As(err, &snfe) {
		return true
	}
	var ae smithy.APIError
	if errors.As(err, &ae) {
		code := ae.ErrorCode()
		return code == "ClusterNotFoundException" || code == "ServiceNotFoundException" || code == "ResourceNotFoundException"
	}
	return false
}

// DescribeCluster returns the ECS cluster or nil if not found.
func DescribeCluster(ctx context.Context, c *ecs.Client, clusterName string) (*types.Cluster, error) {
	out, err := c.DescribeClusters(ctx, &ecs.DescribeClustersInput{
		Clusters: []string{clusterName},
	})
	if err != nil {
		return nil, err
	}
	if len(out.Clusters) == 0 || out.Clusters[0].Status == nil || *out.Clusters[0].Status == "INACTIVE" {
		return nil, nil
	}
	return &out.Clusters[0], nil
}

type CreateClusterInput struct {
	ClusterName       string
	CapacityProviders []string
	InsightsEnabled   *bool
	Tags              map[string]string
}

func CreateCluster(ctx context.Context, c *ecs.Client, in CreateClusterInput) (*types.Cluster, error) {
	input := &ecs.CreateClusterInput{
		ClusterName: aws.String(in.ClusterName),
	}
	if len(in.CapacityProviders) > 0 {
		input.CapacityProviders = in.CapacityProviders
	}
	if in.InsightsEnabled != nil {
		val := "disabled"
		if *in.InsightsEnabled {
			val = "enabled"
		}
		input.Settings = []types.ClusterSetting{
			{Name: types.ClusterSettingNameContainerInsights, Value: aws.String(val)},
		}
	}
	if len(in.Tags) > 0 {
		for k, v := range in.Tags {
			k, v := k, v
			input.Tags = append(input.Tags, types.Tag{Key: &k, Value: &v})
		}
	}
	out, err := c.CreateCluster(ctx, input)
	if err != nil {
		return nil, err
	}
	return out.Cluster, nil
}

func DeleteCluster(ctx context.Context, c *ecs.Client, clusterName string) error {
	_, err := c.DeleteCluster(ctx, &ecs.DeleteClusterInput{Cluster: aws.String(clusterName)})
	return err
}

// DescribeService returns the ECS service or nil if not found / inactive.
func DescribeService(ctx context.Context, c ServiceAPI, clusterName, serviceName string) (*types.Service, error) {
	out, err := c.DescribeServices(ctx, &ecs.DescribeServicesInput{
		Cluster:  aws.String(clusterName),
		Services: []string{serviceName},
	})
	if err != nil {
		return nil, err
	}
	for _, svc := range out.Services {
		if aws.ToString(svc.ServiceName) == serviceName && aws.ToString(svc.Status) != "INACTIVE" {
			return &svc, nil
		}
	}
	return nil, nil
}

type CreateServiceInput struct {
	ClusterName                   string
	ServiceName                   string
	TaskDefinitionArn             string
	DesiredCount                  int32
	LaunchType                    types.LaunchType
	SubnetIDs                     []string
	SecurityGroupIDs              []string
	AssignPublicIP                types.AssignPublicIp
	LoadBalancers                 []types.LoadBalancer
	HealthCheckGracePeriodSeconds *int32
	EnableExecuteCommand          bool
	Tags                          map[string]string
}

func CreateService(ctx context.Context, c ServiceAPI, in CreateServiceInput) (*types.Service, error) {
	input := &ecs.CreateServiceInput{
		Cluster:        aws.String(in.ClusterName),
		ServiceName:    aws.String(in.ServiceName),
		TaskDefinition: aws.String(in.TaskDefinitionArn),
		DesiredCount:   aws.Int32(in.DesiredCount),
	}
	if in.LaunchType != "" {
		input.LaunchType = in.LaunchType
	}
	if len(in.SubnetIDs) > 0 {
		assignIP := in.AssignPublicIP
		if assignIP == "" {
			assignIP = types.AssignPublicIpDisabled
		}
		input.NetworkConfiguration = &types.NetworkConfiguration{
			AwsvpcConfiguration: &types.AwsVpcConfiguration{
				Subnets:        in.SubnetIDs,
				SecurityGroups: in.SecurityGroupIDs,
				AssignPublicIp: assignIP,
			},
		}
	}
	if len(in.LoadBalancers) > 0 {
		input.LoadBalancers = in.LoadBalancers
	}
	if in.HealthCheckGracePeriodSeconds != nil {
		input.HealthCheckGracePeriodSeconds = in.HealthCheckGracePeriodSeconds
	}
	if in.EnableExecuteCommand {
		input.EnableExecuteCommand = true
	}
	if len(in.Tags) > 0 {
		for k, v := range in.Tags {
			k, v := k, v
			input.Tags = append(input.Tags, types.Tag{Key: &k, Value: &v})
		}
	}
	out, err := c.CreateService(ctx, input)
	if err != nil {
		return nil, err
	}
	return out.Service, nil
}

func UpdateService(ctx context.Context, c ServiceAPI, in CreateServiceInput) error {
	input := &ecs.UpdateServiceInput{
		Cluster:        aws.String(in.ClusterName),
		Service:        aws.String(in.ServiceName),
		TaskDefinition: aws.String(in.TaskDefinitionArn),
		DesiredCount:   aws.Int32(in.DesiredCount),
	}
	if len(in.SubnetIDs) > 0 {
		assignIP := in.AssignPublicIP
		if assignIP == "" {
			assignIP = types.AssignPublicIpDisabled
		}
		input.NetworkConfiguration = &types.NetworkConfiguration{
			AwsvpcConfiguration: &types.AwsVpcConfiguration{
				Subnets:        in.SubnetIDs,
				SecurityGroups: in.SecurityGroupIDs,
				AssignPublicIp: assignIP,
			},
		}
	}
	if in.HealthCheckGracePeriodSeconds != nil {
		input.HealthCheckGracePeriodSeconds = in.HealthCheckGracePeriodSeconds
	}
	if in.EnableExecuteCommand {
		input.EnableExecuteCommand = aws.Bool(true)
	}
	_, err := c.UpdateService(ctx, input)
	return err
}

func DeleteService(ctx context.Context, c ServiceAPI, clusterName, serviceName string) error {
	_, err := c.DeleteService(ctx, &ecs.DeleteServiceInput{
		Cluster: aws.String(clusterName),
		Service: aws.String(serviceName),
		Force:   aws.Bool(true),
	})
	return err
}

// TaskDefinitionInput holds all inputs for registering a task definition.
type TaskDefinitionInput struct {
	Family           string
	ContainerDefs    []types.ContainerDefinition
	CPU              string
	Memory           string
	NetworkMode      types.NetworkMode
	ExecutionRoleArn string
	TaskRoleArn      string
	Tags             map[string]string
}

func RegisterTaskDefinition(ctx context.Context, c *ecs.Client, in TaskDefinitionInput) (*types.TaskDefinition, error) {
	input := &ecs.RegisterTaskDefinitionInput{
		Family:                  aws.String(in.Family),
		ContainerDefinitions:    in.ContainerDefs,
		RequiresCompatibilities: []types.Compatibility{types.CompatibilityFargate},
	}
	if in.CPU != "" {
		input.Cpu = aws.String(in.CPU)
	}
	if in.Memory != "" {
		input.Memory = aws.String(in.Memory)
	}
	if in.NetworkMode != "" {
		input.NetworkMode = in.NetworkMode
	}
	if in.ExecutionRoleArn != "" {
		input.ExecutionRoleArn = aws.String(in.ExecutionRoleArn)
	}
	if in.TaskRoleArn != "" {
		input.TaskRoleArn = aws.String(in.TaskRoleArn)
	}
	if len(in.Tags) > 0 {
		for k, v := range in.Tags {
			k, v := k, v
			input.Tags = append(input.Tags, types.Tag{Key: &k, Value: &v})
		}
	}
	out, err := c.RegisterTaskDefinition(ctx, input)
	if err != nil {
		return nil, err
	}
	return out.TaskDefinition, nil
}

func DeregisterTaskDefinition(ctx context.Context, c *ecs.Client, taskDefArn string) error {
	_, err := c.DeregisterTaskDefinition(ctx, &ecs.DeregisterTaskDefinitionInput{
		TaskDefinition: aws.String(taskDefArn),
	})
	return err
}
