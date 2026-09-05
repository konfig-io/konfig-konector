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
	"errors"

	"github.com/konfig-io/konfig-konector/internal/aws/multi"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
)

// ── SAML Provider ────────────────────────────────────────────────────────────

// GetSAMLProvider fetches a SAML provider by ARN. Returns nil, nil if not found.
func GetSAMLProvider(ctx context.Context, client *multi.IAM, providerARN string) (*iam.GetSAMLProviderOutput, error) {
	out, err := client.GetSAMLProvider(ctx, &iam.GetSAMLProviderInput{
		SAMLProviderArn: aws.String(providerARN),
	})
	if err != nil {
		var nse *types.NoSuchEntityException
		if errors.As(err, &nse) {
			return nil, nil
		}
		return nil, err
	}
	return out, nil
}

// CreateSAMLProvider creates a new SAML identity provider.
func CreateSAMLProvider(ctx context.Context, client *multi.IAM, input *iam.CreateSAMLProviderInput) (string, error) {
	out, err := client.CreateSAMLProvider(ctx, input)
	if err != nil {
		return "", err
	}
	return aws.ToString(out.SAMLProviderArn), nil
}

// UpdateSAMLProvider replaces the metadata document of an existing SAML provider.
func UpdateSAMLProvider(ctx context.Context, client *multi.IAM, providerARN, metadataDoc string) error {
	_, err := client.UpdateSAMLProvider(ctx, &iam.UpdateSAMLProviderInput{
		SAMLProviderArn:      aws.String(providerARN),
		SAMLMetadataDocument: aws.String(metadataDoc),
	})
	return err
}

// DeleteSAMLProvider deletes a SAML identity provider. Returns nil if not found.
func DeleteSAMLProvider(ctx context.Context, client *multi.IAM, providerARN string) error {
	_, err := client.DeleteSAMLProvider(ctx, &iam.DeleteSAMLProviderInput{
		SAMLProviderArn: aws.String(providerARN),
	})
	if IsNotFound(err) {
		return nil
	}
	return err
}

// SyncSAMLTags reconciles AWS tags on a SAML provider.
func SyncSAMLTags(ctx context.Context, client *multi.IAM, providerARN string, desired map[string]string) error {
	out, err := client.ListSAMLProviderTags(ctx, &iam.ListSAMLProviderTagsInput{
		SAMLProviderArn: aws.String(providerARN),
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
			toAdd = append(toAdd, types.Tag{Key: aws.String(k), Value: aws.String(v)})
		}
	}
	if len(toAdd) > 0 {
		if _, err := client.TagSAMLProvider(ctx, &iam.TagSAMLProviderInput{
			SAMLProviderArn: aws.String(providerARN),
			Tags:            toAdd,
		}); err != nil {
			return err
		}
	}

	var toRemove []string
	for k := range current {
		if _, ok := desired[k]; !ok {
			toRemove = append(toRemove, k)
		}
	}
	if len(toRemove) > 0 {
		if _, err := client.UntagSAMLProvider(ctx, &iam.UntagSAMLProviderInput{
			SAMLProviderArn: aws.String(providerARN),
			TagKeys:         toRemove,
		}); err != nil {
			return err
		}
	}
	return nil
}

// ── OIDC Provider ────────────────────────────────────────────────────────────

// GetOIDCProvider fetches an OIDC provider by ARN. Returns nil, nil if not found.
func GetOIDCProvider(ctx context.Context, client *multi.IAM, providerARN string) (*iam.GetOpenIDConnectProviderOutput, error) {
	out, err := client.GetOpenIDConnectProvider(ctx, &iam.GetOpenIDConnectProviderInput{
		OpenIDConnectProviderArn: aws.String(providerARN),
	})
	if err != nil {
		var nse *types.NoSuchEntityException
		if errors.As(err, &nse) {
			return nil, nil
		}
		return nil, err
	}
	return out, nil
}

// CreateOIDCProvider creates a new OIDC identity provider. Returns the new provider ARN.
func CreateOIDCProvider(ctx context.Context, client *multi.IAM, input *iam.CreateOpenIDConnectProviderInput) (string, error) {
	out, err := client.CreateOpenIDConnectProvider(ctx, input)
	if err != nil {
		return "", err
	}
	return aws.ToString(out.OpenIDConnectProviderArn), nil
}

// DeleteOIDCProvider deletes an OIDC identity provider. Returns nil if not found.
func DeleteOIDCProvider(ctx context.Context, client *multi.IAM, providerARN string) error {
	_, err := client.DeleteOpenIDConnectProvider(ctx, &iam.DeleteOpenIDConnectProviderInput{
		OpenIDConnectProviderArn: aws.String(providerARN),
	})
	if IsNotFound(err) {
		return nil
	}
	return err
}

// SyncOIDCThumbprints replaces the thumbprint list on an OIDC provider.
func SyncOIDCThumbprints(ctx context.Context, client *multi.IAM, providerARN string, thumbprints []string) error {
	_, err := client.UpdateOpenIDConnectProviderThumbprint(ctx, &iam.UpdateOpenIDConnectProviderThumbprintInput{
		OpenIDConnectProviderArn: aws.String(providerARN),
		ThumbprintList:           thumbprints,
	})
	return err
}

// SyncOIDCClientIDs reconciles the client ID list on an OIDC provider.
func SyncOIDCClientIDs(ctx context.Context, client *multi.IAM, providerARN string, current, desired []string) error {
	currentSet := make(map[string]struct{}, len(current))
	for _, id := range current {
		currentSet[id] = struct{}{}
	}
	desiredSet := make(map[string]struct{}, len(desired))
	for _, id := range desired {
		desiredSet[id] = struct{}{}
	}

	for _, id := range desired {
		if _, ok := currentSet[id]; !ok {
			if _, err := client.AddClientIDToOpenIDConnectProvider(ctx, &iam.AddClientIDToOpenIDConnectProviderInput{
				OpenIDConnectProviderArn: aws.String(providerARN),
				ClientID:                 aws.String(id),
			}); err != nil {
				return err
			}
		}
	}
	for _, id := range current {
		if _, ok := desiredSet[id]; !ok {
			if _, err := client.RemoveClientIDFromOpenIDConnectProvider(ctx, &iam.RemoveClientIDFromOpenIDConnectProviderInput{
				OpenIDConnectProviderArn: aws.String(providerARN),
				ClientID:                 aws.String(id),
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// SyncOIDCTags reconciles AWS tags on an OIDC provider.
func SyncOIDCTags(ctx context.Context, client *multi.IAM, providerARN string, desired map[string]string) error {
	out, err := client.ListOpenIDConnectProviderTags(ctx, &iam.ListOpenIDConnectProviderTagsInput{
		OpenIDConnectProviderArn: aws.String(providerARN),
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
			toAdd = append(toAdd, types.Tag{Key: aws.String(k), Value: aws.String(v)})
		}
	}
	if len(toAdd) > 0 {
		if _, err := client.TagOpenIDConnectProvider(ctx, &iam.TagOpenIDConnectProviderInput{
			OpenIDConnectProviderArn: aws.String(providerARN),
			Tags:                     toAdd,
		}); err != nil {
			return err
		}
	}

	var toRemove []string
	for k := range current {
		if _, ok := desired[k]; !ok {
			toRemove = append(toRemove, k)
		}
	}
	if len(toRemove) > 0 {
		if _, err := client.UntagOpenIDConnectProvider(ctx, &iam.UntagOpenIDConnectProviderInput{
			OpenIDConnectProviderArn: aws.String(providerARN),
			TagKeys:                  toRemove,
		}); err != nil {
			return err
		}
	}
	return nil
}
