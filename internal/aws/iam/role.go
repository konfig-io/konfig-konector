/*
Copyright 2024.

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
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
)

// IsNotFound returns true when the AWS error indicates the resource does not exist.
func IsNotFound(err error) bool {
	var nse *types.NoSuchEntityException
	return errors.As(err, &nse)
}

// RoleAPI is the narrow subset of the IAM SDK client used by the role helpers
// in this file. *multi.IAM satisfies it; tests may supply a fake.
type RoleAPI interface {
	GetRole(ctx context.Context, params *iam.GetRoleInput, optFns ...func(*iam.Options)) (*iam.GetRoleOutput, error)
	CreateRole(ctx context.Context, params *iam.CreateRoleInput, optFns ...func(*iam.Options)) (*iam.CreateRoleOutput, error)
	UpdateAssumeRolePolicy(ctx context.Context, params *iam.UpdateAssumeRolePolicyInput, optFns ...func(*iam.Options)) (*iam.UpdateAssumeRolePolicyOutput, error)
	UpdateRole(ctx context.Context, params *iam.UpdateRoleInput, optFns ...func(*iam.Options)) (*iam.UpdateRoleOutput, error)
	DeleteRole(ctx context.Context, params *iam.DeleteRoleInput, optFns ...func(*iam.Options)) (*iam.DeleteRoleOutput, error)
	ListRoleTags(ctx context.Context, params *iam.ListRoleTagsInput, optFns ...func(*iam.Options)) (*iam.ListRoleTagsOutput, error)
	TagRole(ctx context.Context, params *iam.TagRoleInput, optFns ...func(*iam.Options)) (*iam.TagRoleOutput, error)
	UntagRole(ctx context.Context, params *iam.UntagRoleInput, optFns ...func(*iam.Options)) (*iam.UntagRoleOutput, error)
	ListAttachedRolePolicies(ctx context.Context, params *iam.ListAttachedRolePoliciesInput, optFns ...func(*iam.Options)) (*iam.ListAttachedRolePoliciesOutput, error)
	DetachRolePolicy(ctx context.Context, params *iam.DetachRolePolicyInput, optFns ...func(*iam.Options)) (*iam.DetachRolePolicyOutput, error)
	ListRolePolicies(ctx context.Context, params *iam.ListRolePoliciesInput, optFns ...func(*iam.Options)) (*iam.ListRolePoliciesOutput, error)
	DeleteRolePolicy(ctx context.Context, params *iam.DeleteRolePolicyInput, optFns ...func(*iam.Options)) (*iam.DeleteRolePolicyOutput, error)
}

// GetRole fetches the IAM role by name. Returns nil, nil if not found.
func GetRole(ctx context.Context, client RoleAPI, roleName string) (*types.Role, error) {
	out, err := client.GetRole(ctx, &iam.GetRoleInput{
		RoleName: aws.String(roleName),
	})
	if err != nil {
		if IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return out.Role, nil
}

// CreateRole creates a new IAM role.
func CreateRole(ctx context.Context, client RoleAPI, input *iam.CreateRoleInput) (*types.Role, error) {
	out, err := client.CreateRole(ctx, input)
	if err != nil {
		return nil, err
	}
	return out.Role, nil
}

// UpdateAssumeRolePolicy updates the trust policy of an existing role.
func UpdateAssumeRolePolicy(ctx context.Context, client RoleAPI, roleName, policyDoc string) error {
	_, err := client.UpdateAssumeRolePolicy(ctx, &iam.UpdateAssumeRolePolicyInput{
		RoleName:       aws.String(roleName),
		PolicyDocument: aws.String(policyDoc),
	})
	return err
}

// UpdateRoleDescription updates the description and max session duration of a role.
func UpdateRoleDescription(ctx context.Context, client RoleAPI, roleName, description string, maxSession int32) error {
	input := &iam.UpdateRoleInput{
		RoleName:    aws.String(roleName),
		Description: aws.String(description),
	}
	if maxSession > 0 {
		input.MaxSessionDuration = aws.Int32(maxSession)
	}
	_, err := client.UpdateRole(ctx, input)
	return err
}

// DeleteRole deletes an IAM role. The caller must detach all policies first.
func DeleteRole(ctx context.Context, client RoleAPI, roleName string) error {
	_, err := client.DeleteRole(ctx, &iam.DeleteRoleInput{
		RoleName: aws.String(roleName),
	})
	if IsNotFound(err) {
		return nil
	}
	return err
}

// SyncTags reconciles AWS tags on a role to match the desired set.
func SyncTags(ctx context.Context, client RoleAPI, roleName string, desired map[string]string) error {
	// Fetch current tags.
	out, err := client.ListRoleTags(ctx, &iam.ListRoleTagsInput{RoleName: aws.String(roleName)})
	if err != nil {
		return err
	}

	// Build current tag map.
	current := make(map[string]string, len(out.Tags))
	for _, t := range out.Tags {
		current[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}

	// Add / update tags.
	var toAdd []types.Tag
	for k, v := range desired {
		if cur, ok := current[k]; !ok || cur != v {
			toAdd = append(toAdd, types.Tag{Key: aws.String(k), Value: aws.String(v)})
		}
	}
	if len(toAdd) > 0 {
		if _, err := client.TagRole(ctx, &iam.TagRoleInput{
			RoleName: aws.String(roleName),
			Tags:     toAdd,
		}); err != nil {
			return err
		}
	}

	// Remove stale tags.
	var toRemove []string
	for k := range current {
		if _, ok := desired[k]; !ok {
			toRemove = append(toRemove, k)
		}
	}
	if len(toRemove) > 0 {
		if _, err := client.UntagRole(ctx, &iam.UntagRoleInput{
			RoleName: aws.String(roleName),
			TagKeys:  toRemove,
		}); err != nil {
			return err
		}
	}
	return nil
}

// DetachAllPolicies detaches all managed policies from a role (needed before deletion).
func DetachAllPolicies(ctx context.Context, client RoleAPI, roleName string) error {
	paginator := iam.NewListAttachedRolePoliciesPaginator(client, &iam.ListAttachedRolePoliciesInput{
		RoleName: aws.String(roleName),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			if IsNotFound(err) {
				return nil
			}
			return err
		}
		for _, p := range page.AttachedPolicies {
			if _, err := client.DetachRolePolicy(ctx, &iam.DetachRolePolicyInput{
				RoleName:  aws.String(roleName),
				PolicyArn: p.PolicyArn,
			}); err != nil && !IsNotFound(err) {
				return err
			}
		}
	}
	return nil
}

// DeleteAllInlinePolicies deletes all inline policies from a role.
func DeleteAllInlinePolicies(ctx context.Context, client RoleAPI, roleName string) error {
	paginator := iam.NewListRolePoliciesPaginator(client, &iam.ListRolePoliciesInput{
		RoleName: aws.String(roleName),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			if IsNotFound(err) {
				return nil
			}
			return err
		}
		for _, name := range page.PolicyNames {
			if _, err := client.DeleteRolePolicy(ctx, &iam.DeleteRolePolicyInput{
				RoleName:   aws.String(roleName),
				PolicyName: aws.String(name),
			}); err != nil && !IsNotFound(err) {
				return err
			}
		}
	}
	return nil
}
