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

func DescribeFargateProfile(ctx context.Context, c *eks.Client, clusterName, profileName string) (*types.FargateProfile, error) {
	out, err := c.DescribeFargateProfile(ctx, &eks.DescribeFargateProfileInput{
		ClusterName:        aws.String(clusterName),
		FargateProfileName: aws.String(profileName),
	})
	if err != nil {
		return nil, err
	}
	return out.FargateProfile, nil
}

type FargateProfileInput struct {
	ClusterName      string
	ProfileName      string
	PodExecutionRole string
	SubnetIDs        []string
	Selectors        []types.FargateProfileSelector
	Tags             map[string]string
}

func CreateFargateProfile(ctx context.Context, c *eks.Client, in FargateProfileInput) (*types.FargateProfile, error) {
	input := &eks.CreateFargateProfileInput{
		ClusterName:         aws.String(in.ClusterName),
		FargateProfileName:  aws.String(in.ProfileName),
		PodExecutionRoleArn: aws.String(in.PodExecutionRole),
		Subnets:             in.SubnetIDs,
		Selectors:           in.Selectors,
		Tags:                in.Tags,
	}
	out, err := c.CreateFargateProfile(ctx, input)
	if err != nil {
		return nil, err
	}
	return out.FargateProfile, nil
}

func DeleteFargateProfile(ctx context.Context, c *eks.Client, clusterName, profileName string) error {
	_, err := c.DeleteFargateProfile(ctx, &eks.DeleteFargateProfileInput{
		ClusterName:        aws.String(clusterName),
		FargateProfileName: aws.String(profileName),
	})
	return err
}
