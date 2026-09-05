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

func DescribeAccessEntry(ctx context.Context, c *multi.EKS, clusterName, principalArn string) (*types.AccessEntry, error) {
	out, err := c.DescribeAccessEntry(ctx, &eks.DescribeAccessEntryInput{
		ClusterName:  aws.String(clusterName),
		PrincipalArn: aws.String(principalArn),
	})
	if err != nil {
		return nil, err
	}
	return out.AccessEntry, nil
}

type AccessEntryInput struct {
	ClusterName      string
	PrincipalArn     string
	EntryType        string
	KubernetesGroups []string
	Username         string
	Tags             map[string]string
}

func CreateAccessEntry(ctx context.Context, c *multi.EKS, in AccessEntryInput) (*types.AccessEntry, error) {
	input := &eks.CreateAccessEntryInput{
		ClusterName:      aws.String(in.ClusterName),
		PrincipalArn:     aws.String(in.PrincipalArn),
		KubernetesGroups: in.KubernetesGroups,
		Tags:             in.Tags,
	}
	if in.EntryType != "" {
		input.Type = aws.String(in.EntryType)
	}
	if in.Username != "" {
		input.Username = aws.String(in.Username)
	}
	out, err := c.CreateAccessEntry(ctx, input)
	if err != nil {
		return nil, err
	}
	return out.AccessEntry, nil
}

func UpdateAccessEntry(ctx context.Context, c *multi.EKS, in AccessEntryInput) error {
	input := &eks.UpdateAccessEntryInput{
		ClusterName:      aws.String(in.ClusterName),
		PrincipalArn:     aws.String(in.PrincipalArn),
		KubernetesGroups: in.KubernetesGroups,
	}
	if in.Username != "" {
		input.Username = aws.String(in.Username)
	}
	_, err := c.UpdateAccessEntry(ctx, input)
	return err
}

func DeleteAccessEntry(ctx context.Context, c *multi.EKS, clusterName, principalArn string) error {
	_, err := c.DeleteAccessEntry(ctx, &eks.DeleteAccessEntryInput{
		ClusterName:  aws.String(clusterName),
		PrincipalArn: aws.String(principalArn),
	})
	return err
}

type PolicyAssociationInput struct {
	ClusterName  string
	PrincipalArn string
	PolicyArn    string
	ScopeType    types.AccessScopeType
	Namespaces   []string
}

func AssociateAccessPolicy(ctx context.Context, c *multi.EKS, in PolicyAssociationInput) error {
	_, err := c.AssociateAccessPolicy(ctx, &eks.AssociateAccessPolicyInput{
		ClusterName:  aws.String(in.ClusterName),
		PrincipalArn: aws.String(in.PrincipalArn),
		PolicyArn:    aws.String(in.PolicyArn),
		AccessScope: &types.AccessScope{
			Type:       in.ScopeType,
			Namespaces: in.Namespaces,
		},
	})
	return err
}

func DisassociateAccessPolicy(ctx context.Context, c *multi.EKS, clusterName, principalArn, policyArn string) error {
	_, err := c.DisassociateAccessPolicy(ctx, &eks.DisassociateAccessPolicyInput{
		ClusterName:  aws.String(clusterName),
		PrincipalArn: aws.String(principalArn),
		PolicyArn:    aws.String(policyArn),
	})
	return err
}

func ListAssociatedAccessPolicies(ctx context.Context, c *multi.EKS, clusterName, principalArn string) ([]types.AssociatedAccessPolicy, error) {
	var policies []types.AssociatedAccessPolicy
	paginator := eks.NewListAssociatedAccessPoliciesPaginator(c, &eks.ListAssociatedAccessPoliciesInput{
		ClusterName:  aws.String(clusterName),
		PrincipalArn: aws.String(principalArn),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		policies = append(policies, page.AssociatedAccessPolicies...)
	}
	return policies, nil
}
