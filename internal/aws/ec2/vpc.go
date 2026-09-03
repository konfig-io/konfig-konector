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

package ec2

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// GetVPC fetches a VPC by ID. Returns nil, nil if not found.
func GetVPC(ctx context.Context, client DescribeVpcsAPI, vpcID string) (*types.Vpc, error) {
	out, err := client.DescribeVpcs(ctx, &awsec2.DescribeVpcsInput{
		VpcIds: []string{vpcID},
	})
	if err != nil {
		if IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(out.Vpcs) == 0 {
		return nil, nil
	}
	return &out.Vpcs[0], nil
}

// SyncVPCTags updates the tags on a VPC to match the desired map.
func SyncVPCTags(ctx context.Context, client *awsec2.Client, vpcID string, desired map[string]string) error {
	out, err := client.DescribeTags(ctx, &awsec2.DescribeTagsInput{
		Filters: []types.Filter{
			{Name: aws.String("resource-id"), Values: []string{vpcID}},
			{Name: aws.String("resource-type"), Values: []string{"vpc"}},
		},
	})
	if err != nil {
		return err
	}

	current := make(map[string]string, len(out.Tags))
	for _, t := range out.Tags {
		current[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}

	var toAdd []types.Tag
	for k, v := range desired {
		if cur, ok := current[k]; !ok || cur != v {
			k, v := k, v
			toAdd = append(toAdd, types.Tag{Key: &k, Value: &v})
		}
	}

	var toRemove []types.Tag
	for k := range current {
		if _, ok := desired[k]; !ok {
			k := k
			toRemove = append(toRemove, types.Tag{Key: &k})
		}
	}

	if len(toAdd) > 0 {
		if _, err := client.CreateTags(ctx, &awsec2.CreateTagsInput{
			Resources: []string{vpcID},
			Tags:      toAdd,
		}); err != nil {
			return err
		}
	}

	if len(toRemove) > 0 {
		if _, err := client.DeleteTags(ctx, &awsec2.DeleteTagsInput{
			Resources: []string{vpcID},
			Tags:      toRemove,
		}); err != nil {
			return err
		}
	}

	return nil
}

// SyncResourceTags is a generic tag sync for any EC2 resource.
func SyncResourceTags(ctx context.Context, client TagsAPI, resourceID string, resourceType string, desired map[string]string) error {
	out, err := client.DescribeTags(ctx, &awsec2.DescribeTagsInput{
		Filters: []types.Filter{
			{Name: aws.String("resource-id"), Values: []string{resourceID}},
			{Name: aws.String("resource-type"), Values: []string{resourceType}},
		},
	})
	if err != nil {
		return err
	}

	current := make(map[string]string, len(out.Tags))
	for _, t := range out.Tags {
		current[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}

	var toAdd []types.Tag
	for k, v := range desired {
		if cur, ok := current[k]; !ok || cur != v {
			k, v := k, v
			toAdd = append(toAdd, types.Tag{Key: &k, Value: &v})
		}
	}

	var toRemove []types.Tag
	for k := range current {
		if _, ok := desired[k]; !ok {
			k := k
			toRemove = append(toRemove, types.Tag{Key: &k})
		}
	}

	if len(toAdd) > 0 {
		if _, err := client.CreateTags(ctx, &awsec2.CreateTagsInput{
			Resources: []string{resourceID},
			Tags:      toAdd,
		}); err != nil {
			return err
		}
	}

	if len(toRemove) > 0 {
		if _, err := client.DeleteTags(ctx, &awsec2.DeleteTagsInput{
			Resources: []string{resourceID},
			Tags:      toRemove,
		}); err != nil {
			return err
		}
	}

	return nil
}
