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

package exporters

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsacm "github.com/aws/aws-sdk-go-v2/service/acm"
	acmtypes "github.com/aws/aws-sdk-go-v2/service/acm/types"
	awscognito "github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	awskms "github.com/aws/aws-sdk-go-v2/service/kms"
	kmstypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	awssecretsmanager "github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smtypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	awsssm "github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	awsclient "github.com/konfig-io/konfig-konector/internal/aws"
	"github.com/konfig-io/konfig-konector/internal/export"
)

func init() {
	// KMS keys must be indexed before the aliases that target them.
	export.Register(export.Exporter{Kind: "KMSKey", Service: "kms", Order: 5, Fn: exportKMSKeys})
	export.Register(export.Exporter{Kind: "KMSAlias", Service: "kms", Order: 6, Fn: exportKMSAliases})
	export.Register(export.Exporter{Kind: "Secret", Service: "secretsmanager", Order: 40, Fn: exportSecrets})
	export.Register(export.Exporter{Kind: "SSMParameter", Service: "ssm", Order: 40, Fn: exportSSMParameters})
	export.Register(export.Exporter{Kind: "Certificate", Service: "acm", Order: 40, Fn: exportCertificates})
	export.Register(export.Exporter{Kind: "UserPool", Service: "cognito", Order: 41, Fn: exportUserPools})
	export.Register(export.Exporter{Kind: "UserPoolClient", Service: "cognito", Order: 42, Fn: exportUserPoolClients})
	export.Register(export.Exporter{Kind: "IdentityProvider", Service: "cognito", Order: 42, Fn: exportIdentityProviders})
}

// kmsTagMap converts KMS tag slices to a spec tag map, dropping aws: tags.
func kmsTagMap(tags []kmstypes.Tag) map[string]string {
	if len(tags) == 0 {
		return nil
	}
	m := make(map[string]string, len(tags))
	for _, t := range tags {
		m[aws.ToString(t.TagKey)] = aws.ToString(t.TagValue)
	}
	return export.TagMap(m)
}

func exportKMSKeys(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	// Aliases give customer keys a human-readable CR name; build the map
	// once. AWS-managed keys are recognisable by their alias/aws/* alias and
	// KeyManager=AWS.
	aliasByKey := map[string]string{}
	ap := awskms.NewListAliasesPaginator(clients.KMS, &awskms.ListAliasesInput{})
	for ap.HasMorePages() {
		page, err := ap.NextPage(ctx)
		if err != nil {
			break
		}
		for _, a := range page.Aliases {
			keyID := aws.ToString(a.TargetKeyId)
			if keyID == "" {
				continue
			}
			if _, ok := aliasByKey[keyID]; !ok {
				aliasByKey[keyID] = aws.ToString(a.AliasName)
			}
		}
	}

	var objs []client.Object
	p := awskms.NewListKeysPaginator(clients.KMS, &awskms.ListKeysInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list keys: %w", err)
		}
		for _, k := range page.Keys {
			keyID := aws.ToString(k.KeyId)
			// Skip keys aliased alias/aws/* — those are AWS-managed.
			if strings.HasPrefix(aliasByKey[keyID], "alias/aws/") {
				continue
			}
			det, err := clients.KMS.DescribeKey(ctx, &awskms.DescribeKeyInput{KeyId: k.KeyId})
			if err != nil || det.KeyMetadata == nil {
				// Per-key failure (e.g. cross-account grants): skip it.
				continue
			}
			md := det.KeyMetadata
			// Customer-managed keys only — never export AWS-managed keys.
			if md.KeyManager == kmstypes.KeyManagerTypeAws {
				continue
			}
			// Keys already scheduled for deletion should not be re-adopted.
			if md.KeyState == kmstypes.KeyStatePendingDeletion || md.KeyState == kmstypes.KeyStatePendingReplicaDeletion {
				continue
			}
			crName := keyID
			if alias := aliasByKey[keyID]; alias != "" {
				crName = strings.TrimPrefix(alias, "alias/")
			}
			cr := &awsv1alpha1.KMSKey{
				ObjectMeta: export.ObjectMeta(crName, opts),
				Spec: awsv1alpha1.KMSKeySpec{
					Description: aws.ToString(md.Description),
					KeyUsage:    string(md.KeyUsage),
					KeySpec:     string(md.KeySpec),
				},
			}
			// The key policy is access-control configuration, not secret
			// material — safe to export.
			if polOut, err := clients.KMS.GetKeyPolicy(ctx, &awskms.GetKeyPolicyInput{
				KeyId:      k.KeyId,
				PolicyName: aws.String("default"),
			}); err == nil {
				cr.Spec.Policy = aws.ToString(polOut.Policy)
			}
			if rotOut, err := clients.KMS.GetKeyRotationStatus(ctx, &awskms.GetKeyRotationStatusInput{KeyId: k.KeyId}); err == nil {
				cr.Spec.EnableKeyRotation = rotOut.KeyRotationEnabled
			}
			tp := awskms.NewListResourceTagsPaginator(clients.KMS, &awskms.ListResourceTagsInput{KeyId: k.KeyId})
			var tags []kmstypes.Tag
			for tp.HasMorePages() {
				tpage, err := tp.NextPage(ctx)
				if err != nil {
					break
				}
				tags = append(tags, tpage.Tags...)
			}
			cr.Spec.Tags = kmsTagMap(tags)
			opts.Index.Add(keyID, cr.Name)
			opts.Index.Add(aws.ToString(md.Arn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportKMSAliases(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awskms.NewListAliasesPaginator(clients.KMS, &awskms.ListAliasesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list aliases: %w", err)
		}
		for _, a := range page.Aliases {
			aliasName := aws.ToString(a.AliasName)
			// alias/aws/* aliases belong to AWS-managed keys.
			if strings.HasPrefix(aliasName, "alias/aws/") {
				continue
			}
			keyID := aws.ToString(a.TargetKeyId)
			if keyID == "" {
				// Dangling alias with no target key cannot round-trip.
				continue
			}
			cr := &awsv1alpha1.KMSAlias{
				ObjectMeta: export.ObjectMeta(strings.TrimPrefix(aliasName, "alias/"), opts),
				Spec: awsv1alpha1.KMSAliasSpec{
					AliasName: aliasName,
				},
			}
			if crName, ok := opts.Index.Lookup(keyID); ok {
				cr.Spec.TargetKeyRef = awsv1alpha1.KMSKeyRef{Name: crName}
			} else {
				cr.Spec.TargetKeyRef = awsv1alpha1.KMSKeyRef{KeyID: keyID}
			}
			opts.Index.Add(aws.ToString(a.AliasArn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

// smTagMap converts Secrets Manager tag slices to a spec tag map.
func smTagMap(tags []smtypes.Tag) map[string]string {
	if len(tags) == 0 {
		return nil
	}
	m := make(map[string]string, len(tags))
	for _, t := range tags {
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return export.TagMap(m)
}

func exportSecrets(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awssecretsmanager.NewListSecretsPaginator(clients.SecretsManager, &awssecretsmanager.ListSecretsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list secrets: %w", err)
		}
		for _, s := range page.SecretList {
			// Secrets owned by AWS services (e.g. RDS-managed rotation)
			// are lifecycle-managed by that service.
			if aws.ToString(s.OwningService) != "" {
				continue
			}
			name := aws.ToString(s.Name)
			cr := &awsv1alpha1.Secret{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.SecretSpec{
					SecretName:  name,
					Description: aws.ToString(s.Description),
					KMSKeyARN:   aws.ToString(s.KmsKeyId),
					Tags:        smTagMap(s.Tags),
					// SECURITY: metadata only — GetSecretValue is never
					// called and SecretStringRef is deliberately left unset.
					// The secret value must not appear in exported CRs.
				},
			}
			opts.Index.Add(aws.ToString(s.ARN), cr.Name)
			opts.Index.Add("secret/"+name, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportSSMParameters(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsssm.NewDescribeParametersPaginator(clients.SSM, &awsssm.DescribeParametersInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe parameters: %w", err)
		}
		for _, pm := range page.Parameters {
			name := aws.ToString(pm.Name)
			cr := &awsv1alpha1.SSMParameter{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.SSMParameterSpec{
					ParameterName: name,
					Type:          string(pm.Type),
					Description:   aws.ToString(pm.Description),
					Tier:          string(pm.Tier),
				},
			}
			if pm.Type == ssmtypes.ParameterTypeSecureString {
				// SECURITY: SecureString values are secret material — export
				// the metadata and type only. Value and ValueFrom stay unset;
				// only the KMS key used for encryption is recorded.
				cr.Spec.KMSKeyID = aws.ToString(pm.KeyId)
			} else {
				// String/StringList values are plain configuration; fetch
				// without decryption. Best-effort: leave Value empty on error.
				if out, err := clients.SSM.GetParameter(ctx, &awsssm.GetParameterInput{
					Name:           pm.Name,
					WithDecryption: aws.Bool(false),
				}); err == nil && out.Parameter != nil {
					cr.Spec.Value = aws.ToString(out.Parameter.Value)
				}
			}
			if tagsOut, err := clients.SSM.ListTagsForResource(ctx, &awsssm.ListTagsForResourceInput{
				ResourceType: ssmtypes.ResourceTypeForTaggingParameter,
				ResourceId:   pm.Name,
			}); err == nil {
				m := make(map[string]string, len(tagsOut.TagList))
				for _, t := range tagsOut.TagList {
					m[aws.ToString(t.Key)] = aws.ToString(t.Value)
				}
				cr.Spec.Tags = export.TagMap(m)
			}
			opts.Index.Add("ssm-parameter/"+name, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportCertificates(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	// Only ISSUED certificates round-trip: the CR models a certificate
	// request, and pending/failed/expired requests should not be adopted.
	p := awsacm.NewListCertificatesPaginator(clients.ACM, &awsacm.ListCertificatesInput{
		CertificateStatuses: []acmtypes.CertificateStatus{acmtypes.CertificateStatusIssued},
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list certificates: %w", err)
		}
		for _, cs := range page.CertificateSummaryList {
			arn := aws.ToString(cs.CertificateArn)
			det, err := clients.ACM.DescribeCertificate(ctx, &awsacm.DescribeCertificateInput{CertificateArn: cs.CertificateArn})
			if err != nil || det.Certificate == nil {
				// Per-certificate failure: continue with the next one.
				continue
			}
			c := det.Certificate
			// Imported certificates were never requested via ACM; the CR
			// spec (RequestCertificate) cannot reproduce them.
			// SECURITY: private key material is never retrievable via the
			// ACM Describe APIs and is never exported.
			if c.Type != acmtypes.CertificateTypeAmazonIssued {
				continue
			}
			domain := aws.ToString(c.DomainName)
			cr := &awsv1alpha1.Certificate{
				ObjectMeta: export.ObjectMeta(domain, opts),
				Spec: awsv1alpha1.CertificateSpec{
					DomainName: domain,
				},
			}
			// SubjectAlternativeNames always includes the primary domain;
			// the CR models SANs as the additional names only.
			for _, san := range c.SubjectAlternativeNames {
				if san != domain {
					cr.Spec.SubjectAlternativeNames = append(cr.Spec.SubjectAlternativeNames, san)
				}
			}
			if len(c.DomainValidationOptions) > 0 {
				cr.Spec.ValidationMethod = string(c.DomainValidationOptions[0].ValidationMethod)
			}
			if tagsOut, err := clients.ACM.ListTagsForCertificate(ctx, &awsacm.ListTagsForCertificateInput{CertificateArn: cs.CertificateArn}); err == nil {
				m := make(map[string]string, len(tagsOut.Tags))
				for _, t := range tagsOut.Tags {
					m[aws.ToString(t.Key)] = aws.ToString(t.Value)
				}
				cr.Spec.Tags = export.TagMap(m)
			}
			opts.Index.Add(arn, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportUserPools(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awscognito.NewListUserPoolsPaginator(clients.Cognito, &awscognito.ListUserPoolsInput{
		MaxResults: aws.Int32(60),
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list user pools: %w", err)
		}
		for _, up := range page.UserPools {
			id := aws.ToString(up.Id)
			det, err := clients.Cognito.DescribeUserPool(ctx, &awscognito.DescribeUserPoolInput{UserPoolId: up.Id})
			if err != nil || det.UserPool == nil {
				// Per-pool failure: continue with the next pool.
				continue
			}
			pool := det.UserPool
			cr := &awsv1alpha1.UserPool{
				ObjectMeta: export.ObjectMeta(aws.ToString(pool.Name), opts),
				Spec: awsv1alpha1.UserPoolSpec{
					PoolName:         aws.ToString(pool.Name),
					MfaConfiguration: string(pool.MfaConfiguration),
					Tags:             export.TagMap(pool.UserPoolTags),
				},
			}
			for _, attr := range pool.AutoVerifiedAttributes {
				cr.Spec.AutoVerifiedAttributes = append(cr.Spec.AutoVerifiedAttributes, string(attr))
			}
			if pool.Policies != nil && pool.Policies.PasswordPolicy != nil {
				pp := pool.Policies.PasswordPolicy
				cr.Spec.PasswordPolicy = &awsv1alpha1.UserPoolPasswordPolicy{
					MinimumLength:    aws.ToInt32(pp.MinimumLength),
					RequireUppercase: pp.RequireUppercase,
					RequireLowercase: pp.RequireLowercase,
					RequireNumbers:   pp.RequireNumbers,
					RequireSymbols:   pp.RequireSymbols,
				}
			}
			opts.Index.Add(id, cr.Name)
			opts.Index.Add(aws.ToString(pool.Arn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportUserPoolClients(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	pp := awscognito.NewListUserPoolsPaginator(clients.Cognito, &awscognito.ListUserPoolsInput{
		MaxResults: aws.Int32(60),
	})
	for pp.HasMorePages() {
		page, err := pp.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list user pools: %w", err)
		}
		for _, up := range page.UserPools {
			poolID := aws.ToString(up.Id)
			poolCR, poolExported := opts.Index.Lookup(poolID)
			cp := awscognito.NewListUserPoolClientsPaginator(clients.Cognito, &awscognito.ListUserPoolClientsInput{
				UserPoolId: up.Id,
				MaxResults: aws.Int32(60),
			})
			for cp.HasMorePages() {
				cpage, err := cp.NextPage(ctx)
				if err != nil {
					// Per-pool failure: continue with the next pool.
					break
				}
				for _, cl := range cpage.UserPoolClients {
					det, err := clients.Cognito.DescribeUserPoolClient(ctx, &awscognito.DescribeUserPoolClientInput{
						UserPoolId: up.Id,
						ClientId:   cl.ClientId,
					})
					if err != nil || det.UserPoolClient == nil {
						continue
					}
					c := det.UserPoolClient
					cr := &awsv1alpha1.UserPoolClient{
						ObjectMeta: export.ObjectMeta(aws.ToString(up.Name)+"-"+aws.ToString(c.ClientName), opts),
						Spec: awsv1alpha1.UserPoolClientSpec{
							ClientName: aws.ToString(c.ClientName),
							// SECURITY: the existing client secret is never
							// exported. GenerateSecret only records whether
							// the client was created with one.
							GenerateSecret:             aws.ToString(c.ClientSecret) != "",
							AllowedOAuthScopes:         c.AllowedOAuthScopes,
							CallbackURLs:               c.CallbackURLs,
							LogoutURLs:                 c.LogoutURLs,
							SupportedIdentityProviders: c.SupportedIdentityProviders,
							AccessTokenValidity:        aws.ToInt32(c.AccessTokenValidity),
							IdTokenValidity:            aws.ToInt32(c.IdTokenValidity),
							RefreshTokenValidity:       c.RefreshTokenValidity,
						},
					}
					if poolExported {
						cr.Spec.UserPoolRef = awsv1alpha1.UserPoolRef{Name: poolCR}
					} else {
						cr.Spec.UserPoolRef = awsv1alpha1.UserPoolRef{UserPoolID: poolID}
					}
					for _, f := range c.ExplicitAuthFlows {
						cr.Spec.ExplicitAuthFlows = append(cr.Spec.ExplicitAuthFlows, string(f))
					}
					for _, f := range c.AllowedOAuthFlows {
						cr.Spec.AllowedOAuthFlows = append(cr.Spec.AllowedOAuthFlows, string(f))
					}
					opts.Index.Add(aws.ToString(c.ClientId), cr.Name)
					objs = append(objs, cr)
				}
			}
		}
	}
	return objs, nil
}

func exportIdentityProviders(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	pp := awscognito.NewListUserPoolsPaginator(clients.Cognito, &awscognito.ListUserPoolsInput{
		MaxResults: aws.Int32(60),
	})
	for pp.HasMorePages() {
		page, err := pp.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list user pools: %w", err)
		}
		for _, up := range page.UserPools {
			poolID := aws.ToString(up.Id)
			poolCR, poolExported := opts.Index.Lookup(poolID)
			ip := awscognito.NewListIdentityProvidersPaginator(clients.Cognito, &awscognito.ListIdentityProvidersInput{
				UserPoolId: up.Id,
				MaxResults: aws.Int32(60),
			})
			for ip.HasMorePages() {
				ipage, err := ip.NextPage(ctx)
				if err != nil {
					// Per-pool failure: continue with the next pool.
					break
				}
				for _, prov := range ipage.Providers {
					det, err := clients.Cognito.DescribeIdentityProvider(ctx, &awscognito.DescribeIdentityProviderInput{
						UserPoolId:   up.Id,
						ProviderName: prov.ProviderName,
					})
					if err != nil || det.IdentityProvider == nil {
						continue
					}
					idp := det.IdentityProvider
					// SECURITY: ProviderDetails contains the IdP client
					// secret (client_secret) for OIDC/social providers —
					// strip secret keys so no secret material lands in
					// exported CRs. The operator must be given the secret
					// out of band before the CR is applied.
					details := make(map[string]string, len(idp.ProviderDetails))
					for k, v := range idp.ProviderDetails {
						if k == "client_secret" || k == "api_key" || k == "private_key" {
							continue
						}
						details[k] = v
					}
					cr := &awsv1alpha1.IdentityProvider{
						ObjectMeta: export.ObjectMeta(aws.ToString(up.Name)+"-"+aws.ToString(idp.ProviderName), opts),
						Spec: awsv1alpha1.IdentityProviderSpec{
							ProviderName:     aws.ToString(idp.ProviderName),
							ProviderType:     string(idp.ProviderType),
							ProviderDetails:  details,
							AttributeMapping: idp.AttributeMapping,
							IdpIdentifiers:   idp.IdpIdentifiers,
						},
					}
					if poolExported {
						cr.Spec.UserPoolRef = awsv1alpha1.UserPoolRef{Name: poolCR}
					} else {
						cr.Spec.UserPoolRef = awsv1alpha1.UserPoolRef{UserPoolID: poolID}
					}
					objs = append(objs, cr)
				}
			}
		}
	}
	return objs, nil
}
