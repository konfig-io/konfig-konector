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

package iam

import (
	"context"

	"github.com/konfig-io/konfig-konector/internal/aws/multi"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
)

// GetGroup fetches the IAM group by name. Returns nil, nil if not found.
func GetGroup(ctx context.Context, client *multi.IAM, groupName string) (*types.Group, error) {
	out, err := client.GetGroup(ctx, &iam.GetGroupInput{
		GroupName: aws.String(groupName),
	})
	if err != nil {
		if IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return out.Group, nil
}

// CreateGroup creates a new IAM group.
func CreateGroup(ctx context.Context, client *multi.IAM, input *iam.CreateGroupInput) (*types.Group, error) {
	out, err := client.CreateGroup(ctx, input)
	if err != nil {
		return nil, err
	}
	return out.Group, nil
}

// DeleteGroup deletes an IAM group. Returns nil if the group does not exist.
func DeleteGroup(ctx context.Context, client *multi.IAM, groupName string) error {
	_, err := client.DeleteGroup(ctx, &iam.DeleteGroupInput{
		GroupName: aws.String(groupName),
	})
	if IsNotFound(err) {
		return nil
	}
	return err
}

// IsGroupPolicyAttached returns true if the given policy ARN is attached to the group.
func IsGroupPolicyAttached(ctx context.Context, client *multi.IAM, groupName, policyARN string) (bool, error) {
	paginator := iam.NewListAttachedGroupPoliciesPaginator(client, &iam.ListAttachedGroupPoliciesInput{
		GroupName: aws.String(groupName),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return false, err
		}
		for _, p := range page.AttachedPolicies {
			if aws.ToString(p.PolicyArn) == policyARN {
				return true, nil
			}
		}
	}
	return false, nil
}

// AttachGroupPolicy attaches a managed policy to an IAM group.
func AttachGroupPolicy(ctx context.Context, client *multi.IAM, groupName, policyARN string) error {
	_, err := client.AttachGroupPolicy(ctx, &iam.AttachGroupPolicyInput{
		GroupName: aws.String(groupName),
		PolicyArn: aws.String(policyARN),
	})
	return err
}

// DetachGroupPolicy detaches a managed policy from an IAM group. Returns nil if already detached.
func DetachGroupPolicy(ctx context.Context, client *multi.IAM, groupName, policyARN string) error {
	_, err := client.DetachGroupPolicy(ctx, &iam.DetachGroupPolicyInput{
		GroupName: aws.String(groupName),
		PolicyArn: aws.String(policyARN),
	})
	if IsNotFound(err) {
		return nil
	}
	return err
}

// IsUserInGroup returns true if the given user is a member of the group.
func IsUserInGroup(ctx context.Context, client *multi.IAM, groupName, userName string) (bool, error) {
	paginator := iam.NewGetGroupPaginator(client, &iam.GetGroupInput{
		GroupName: aws.String(groupName),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return false, err
		}
		for _, u := range page.Users {
			if aws.ToString(u.UserName) == userName {
				return true, nil
			}
		}
	}
	return false, nil
}

// AddUserToGroup adds a user to an IAM group.
func AddUserToGroup(ctx context.Context, client *multi.IAM, groupName, userName string) error {
	_, err := client.AddUserToGroup(ctx, &iam.AddUserToGroupInput{
		GroupName: aws.String(groupName),
		UserName:  aws.String(userName),
	})
	return err
}

// RemoveUserFromGroup removes a user from an IAM group. Returns nil if already removed.
func RemoveUserFromGroup(ctx context.Context, client *multi.IAM, groupName, userName string) error {
	_, err := client.RemoveUserFromGroup(ctx, &iam.RemoveUserFromGroupInput{
		GroupName: aws.String(groupName),
		UserName:  aws.String(userName),
	})
	if IsNotFound(err) {
		return nil
	}
	return err
}
