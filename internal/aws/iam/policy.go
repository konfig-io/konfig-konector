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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
)

// PolicyAPI is the narrow subset of the IAM SDK client used by the managed
// policy helpers in this file. *iam.Client satisfies it.
type PolicyAPI interface {
	GetPolicy(ctx context.Context, params *iam.GetPolicyInput, optFns ...func(*iam.Options)) (*iam.GetPolicyOutput, error)
	GetPolicyVersion(ctx context.Context, params *iam.GetPolicyVersionInput, optFns ...func(*iam.Options)) (*iam.GetPolicyVersionOutput, error)
	ListPolicyVersions(ctx context.Context, params *iam.ListPolicyVersionsInput, optFns ...func(*iam.Options)) (*iam.ListPolicyVersionsOutput, error)
	CreatePolicyVersion(ctx context.Context, params *iam.CreatePolicyVersionInput, optFns ...func(*iam.Options)) (*iam.CreatePolicyVersionOutput, error)
	DeletePolicyVersion(ctx context.Context, params *iam.DeletePolicyVersionInput, optFns ...func(*iam.Options)) (*iam.DeletePolicyVersionOutput, error)
	DeletePolicy(ctx context.Context, params *iam.DeletePolicyInput, optFns ...func(*iam.Options)) (*iam.DeletePolicyOutput, error)
}

// AttachedPolicyAPI is the subset used by IsPolicyAttached. *iam.Client satisfies it.
type AttachedPolicyAPI interface {
	ListAttachedRolePolicies(ctx context.Context, params *iam.ListAttachedRolePoliciesInput, optFns ...func(*iam.Options)) (*iam.ListAttachedRolePoliciesOutput, error)
}

// InlinePolicyAPI is the subset used by GetInlinePolicy. *iam.Client satisfies it.
type InlinePolicyAPI interface {
	GetRolePolicy(ctx context.Context, params *iam.GetRolePolicyInput, optFns ...func(*iam.Options)) (*iam.GetRolePolicyOutput, error)
}

// GetPolicy fetches an IAM managed policy by ARN. Returns nil, nil if not found.
func GetPolicy(ctx context.Context, client PolicyAPI, arn string) (*types.Policy, error) {
	out, err := client.GetPolicy(ctx, &iam.GetPolicyInput{
		PolicyArn: aws.String(arn),
	})
	if err != nil {
		if IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return out.Policy, nil
}

// GetPolicyDocument returns the current default policy document for a managed policy.
func GetPolicyDocument(ctx context.Context, client PolicyAPI, arn, versionID string) (string, error) {
	out, err := client.GetPolicyVersion(ctx, &iam.GetPolicyVersionInput{
		PolicyArn: aws.String(arn),
		VersionId: aws.String(versionID),
	})
	if err != nil {
		return "", err
	}
	if out.PolicyVersion == nil || out.PolicyVersion.Document == nil {
		return "", nil
	}
	// AWS URL-encodes the document.
	return aws.ToString(out.PolicyVersion.Document), nil
}

// UpdatePolicyDocument creates a new policy version and sets it as default,
// pruning old non-default versions to stay within the 5-version limit.
func UpdatePolicyDocument(ctx context.Context, client PolicyAPI, arn, newDoc string) (string, error) {
	// List existing versions to prune if at limit (5).
	listOut, err := client.ListPolicyVersions(ctx, &iam.ListPolicyVersionsInput{
		PolicyArn: aws.String(arn),
	})
	if err != nil {
		return "", err
	}
	// Delete oldest non-default version if we already have 5.
	if len(listOut.Versions) >= 5 {
		for _, v := range listOut.Versions {
			if !v.IsDefaultVersion {
				if _, err := client.DeletePolicyVersion(ctx, &iam.DeletePolicyVersionInput{
					PolicyArn: aws.String(arn),
					VersionId: v.VersionId,
				}); err != nil {
					return "", err
				}
				break
			}
		}
	}

	out, err := client.CreatePolicyVersion(ctx, &iam.CreatePolicyVersionInput{
		PolicyArn:      aws.String(arn),
		PolicyDocument: aws.String(newDoc),
		SetAsDefault:   true,
	})
	if err != nil {
		return "", err
	}
	return aws.ToString(out.PolicyVersion.VersionId), nil
}

// DeletePolicy deletes an IAM managed policy and all its non-default versions.
func DeletePolicy(ctx context.Context, client PolicyAPI, arn string) error {
	// Delete all non-default versions first.
	listOut, err := client.ListPolicyVersions(ctx, &iam.ListPolicyVersionsInput{
		PolicyArn: aws.String(arn),
	})
	if err != nil {
		if IsNotFound(err) {
			return nil
		}
		return err
	}
	for _, v := range listOut.Versions {
		if !v.IsDefaultVersion {
			if _, err := client.DeletePolicyVersion(ctx, &iam.DeletePolicyVersionInput{
				PolicyArn: aws.String(arn),
				VersionId: v.VersionId,
			}); err != nil && !IsNotFound(err) {
				return err
			}
		}
	}

	_, err = client.DeletePolicy(ctx, &iam.DeletePolicyInput{
		PolicyArn: aws.String(arn),
	})
	if IsNotFound(err) {
		return nil
	}
	return err
}

// IsPolicyAttached checks whether policyARN is currently attached to roleName.
func IsPolicyAttached(ctx context.Context, client AttachedPolicyAPI, roleName, policyARN string) (bool, error) {
	paginator := iam.NewListAttachedRolePoliciesPaginator(client, &iam.ListAttachedRolePoliciesInput{
		RoleName: aws.String(roleName),
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

// GetInlinePolicy returns the policy document for an inline role policy.
// Returns "", nil if it does not exist.
func GetInlinePolicy(ctx context.Context, client InlinePolicyAPI, roleName, policyName string) (string, error) {
	out, err := client.GetRolePolicy(ctx, &iam.GetRolePolicyInput{
		RoleName:   aws.String(roleName),
		PolicyName: aws.String(policyName),
	})
	if err != nil {
		if IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	return aws.ToString(out.PolicyDocument), nil
}
