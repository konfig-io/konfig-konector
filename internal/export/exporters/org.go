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

	"github.com/aws/aws-sdk-go-v2/aws"
	awsct "github.com/aws/aws-sdk-go-v2/service/controltower"
	awsorgs "github.com/aws/aws-sdk-go-v2/service/organizations"
	orgtypes "github.com/aws/aws-sdk-go-v2/service/organizations/types"
	awsram "github.com/aws/aws-sdk-go-v2/service/ram"
	ramtypes "github.com/aws/aws-sdk-go-v2/service/ram/types"
	awssc "github.com/aws/aws-sdk-go-v2/service/servicecatalog"
	awssso "github.com/aws/aws-sdk-go-v2/service/ssoadmin"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	awsclient "github.com/konfig-io/konfig-konector/internal/aws"
	"github.com/konfig-io/konfig-konector/internal/export"
)

// Organization-tier exporters. These only succeed from the organization's
// management (or delegated administrator) account; elsewhere they return
// AccessDenied/AWSOrganizationsNotInUseException errors which the export CLI
// warns about and continues past.
func init() {
	// OUs first so accounts and attachments can reference them by CR name.
	export.Register(export.Exporter{Kind: "OrganizationsOU", Service: "organizations", Order: 115, Fn: exportOrganizationsOUs})
	// Policies before attachments.
	export.Register(export.Exporter{Kind: "OrganizationsPolicy", Service: "organizations", Order: 115, Fn: exportOrganizationsPolicies})
	export.Register(export.Exporter{Kind: "OrganizationsAccount", Service: "organizations", Order: 116, Fn: exportOrganizationsAccounts})
	export.Register(export.Exporter{Kind: "OrganizationsPolicyAttachment", Service: "organizations", Order: 116, Fn: exportOrganizationsPolicyAttachments})
	export.Register(export.Exporter{Kind: "PermissionSet", Service: "ssoadmin", Order: 117, Fn: exportPermissionSets})
	export.Register(export.Exporter{Kind: "SSOAssignment", Service: "ssoadmin", Order: 118, Fn: exportSSOAssignments})
	export.Register(export.Exporter{Kind: "ResourceShare", Service: "ram", Order: 117, Fn: exportResourceShares})
	// Portfolios/products before associations.
	export.Register(export.Exporter{Kind: "SCPortfolio", Service: "servicecatalog", Order: 117, Fn: exportSCPortfolios})
	export.Register(export.Exporter{Kind: "SCProduct", Service: "servicecatalog", Order: 118, Fn: exportSCProducts})
	export.Register(export.Exporter{Kind: "SCPortfolioProductAssociation", Service: "servicecatalog", Order: 119, Fn: exportSCPortfolioProductAssociations})
	export.Register(export.Exporter{Kind: "CTEnabledControl", Service: "controltower", Order: 119, Fn: exportCTEnabledControls})
}

func orgTagMap(tags []orgtypes.Tag) map[string]string {
	if len(tags) == 0 {
		return nil
	}
	m := make(map[string]string, len(tags))
	for _, t := range tags {
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return export.TagMap(m)
}

func listOrgTags(ctx context.Context, c *awsclient.Clients, resourceID string) map[string]string {
	var tags []orgtypes.Tag
	var next *string
	for {
		out, err := c.Organizations.ListTagsForResource(ctx, &awsorgs.ListTagsForResourceInput{
			ResourceId: aws.String(resourceID),
			NextToken:  next,
		})
		if err != nil {
			return nil
		}
		tags = append(tags, out.Tags...)
		if out.NextToken == nil {
			break
		}
		next = out.NextToken
	}
	return orgTagMap(tags)
}

// exportOrganizationsOUs walks the organization tree from ListRoots
// recursively so parent OUs are indexed before their children.
func exportOrganizationsOUs(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object

	var walk func(parentID, parentRefName string) error
	walk = func(parentID, parentRefName string) error {
		p := awsorgs.NewListOrganizationalUnitsForParentPaginator(clients.Organizations, &awsorgs.ListOrganizationalUnitsForParentInput{
			ParentId: aws.String(parentID),
		})
		for p.HasMorePages() {
			page, err := p.NextPage(ctx)
			if err != nil {
				return fmt.Errorf("list OUs for parent %s: %w", parentID, err)
			}
			for _, ou := range page.OrganizationalUnits {
				name := aws.ToString(ou.Name)
				id := aws.ToString(ou.Id)
				cr := &awsv1alpha1.OrganizationsOU{
					ObjectMeta: export.ObjectMeta(name, opts),
					Spec: awsv1alpha1.OrganizationsOUSpec{
						Name: name,
						Tags: listOrgTags(ctx, clients, id),
					},
				}
				if parentRefName != "" {
					cr.Spec.ParentRef = &awsv1alpha1.ResourceRef{Name: parentRefName}
				} else {
					cr.Spec.ParentID = parentID
				}
				opts.Index.Add(id, cr.Name)
				opts.Index.Add(aws.ToString(ou.Arn), cr.Name)
				objs = append(objs, cr)
				if err := walk(id, cr.Name); err != nil {
					return err
				}
			}
		}
		return nil
	}

	rp := awsorgs.NewListRootsPaginator(clients.Organizations, &awsorgs.ListRootsInput{})
	for rp.HasMorePages() {
		page, err := rp.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list roots: %w", err)
		}
		for _, root := range page.Roots {
			rootID := aws.ToString(root.Id)
			opts.Index.Add(rootID, "")
			if err := walk(rootID, ""); err != nil {
				return objs, err
			}
		}
	}
	return objs, nil
}

var orgPolicyTypes = []orgtypes.PolicyType{
	orgtypes.PolicyTypeServiceControlPolicy,
	orgtypes.PolicyTypeTagPolicy,
	orgtypes.PolicyTypeBackupPolicy,
	orgtypes.PolicyTypeAiservicesOptOutPolicy,
}

func exportOrganizationsPolicies(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	for _, pt := range orgPolicyTypes {
		p := awsorgs.NewListPoliciesPaginator(clients.Organizations, &awsorgs.ListPoliciesInput{Filter: pt})
		for p.HasMorePages() {
			page, err := p.NextPage(ctx)
			if err != nil {
				return objs, fmt.Errorf("list %s policies: %w", pt, err)
			}
			for _, sum := range page.Policies {
				// Skip AWS-managed policies (e.g. FullAWSAccess).
				if sum.AwsManaged {
					continue
				}
				id := aws.ToString(sum.Id)
				desc, err := clients.Organizations.DescribePolicy(ctx, &awsorgs.DescribePolicyInput{PolicyId: aws.String(id)})
				if err != nil {
					continue
				}
				name := aws.ToString(sum.Name)
				cr := &awsv1alpha1.OrganizationsPolicy{
					ObjectMeta: export.ObjectMeta(name, opts),
					Spec: awsv1alpha1.OrganizationsPolicySpec{
						Name:        name,
						Type:        string(sum.Type),
						Content:     aws.ToString(desc.Policy.Content),
						Description: aws.ToString(sum.Description),
						Tags:        listOrgTags(ctx, clients, id),
					},
				}
				opts.Index.Add(id, cr.Name)
				opts.Index.Add(aws.ToString(sum.Arn), cr.Name)
				objs = append(objs, cr)
			}
		}
	}
	return objs, nil
}

func exportOrganizationsPolicyAttachments(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	for _, pt := range orgPolicyTypes {
		p := awsorgs.NewListPoliciesPaginator(clients.Organizations, &awsorgs.ListPoliciesInput{Filter: pt})
		for p.HasMorePages() {
			page, err := p.NextPage(ctx)
			if err != nil {
				return objs, fmt.Errorf("list %s policies: %w", pt, err)
			}
			for _, sum := range page.Policies {
				if sum.AwsManaged {
					continue
				}
				policyID := aws.ToString(sum.Id)
				tp := awsorgs.NewListTargetsForPolicyPaginator(clients.Organizations, &awsorgs.ListTargetsForPolicyInput{
					PolicyId: aws.String(policyID),
				})
				for tp.HasMorePages() {
					tpage, err := tp.NextPage(ctx)
					if err != nil {
						break
					}
					for _, tgt := range tpage.Targets {
						targetID := aws.ToString(tgt.TargetId)
						cr := &awsv1alpha1.OrganizationsPolicyAttachment{
							ObjectMeta: export.ObjectMeta(fmt.Sprintf("%s-%s", aws.ToString(sum.Name), targetID), opts),
							Spec: awsv1alpha1.OrganizationsPolicyAttachmentSpec{
								TargetID: targetID,
							},
						}
						if name, ok := opts.Index.Lookup(policyID); ok && name != "" {
							cr.Spec.PolicyRef = awsv1alpha1.OrganizationsPolicyRef{Name: name}
						} else {
							cr.Spec.PolicyRef = awsv1alpha1.OrganizationsPolicyRef{PolicyID: policyID}
						}
						objs = append(objs, cr)
					}
				}
			}
		}
	}
	return objs, nil
}

// exportOrganizationsAccounts exports ACTIVE member accounts only. Exported
// CRs never set closeOnDelete: a freshly imported organization must not be
// able to close accounts by deleting CRs.
func exportOrganizationsAccounts(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsorgs.NewListAccountsPaginator(clients.Organizations, &awsorgs.ListAccountsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list accounts: %w", err)
		}
		for _, acct := range page.Accounts {
			if acct.Status != orgtypes.AccountStatusActive {
				continue
			}
			id := aws.ToString(acct.Id)
			name := aws.ToString(acct.Name)
			cr := &awsv1alpha1.OrganizationsAccount{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.OrganizationsAccountSpec{
					Email:         aws.ToString(acct.Email),
					AccountName:   name,
					CloseOnDelete: false,
					Tags:          listOrgTags(ctx, clients, id),
				},
			}
			if parents, err := clients.Organizations.ListParents(ctx, &awsorgs.ListParentsInput{ChildId: aws.String(id)}); err == nil && len(parents.Parents) > 0 {
				parentID := aws.ToString(parents.Parents[0].Id)
				if refName, ok := opts.Index.Lookup(parentID); ok && refName != "" {
					cr.Spec.ParentRef = &awsv1alpha1.ResourceRef{Name: refName}
				} else {
					cr.Spec.ParentID = parentID
				}
			}
			opts.Index.Add(id, cr.Name)
			opts.Index.Add(aws.ToString(acct.Arn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportPermissionSets(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	ip := awssso.NewListInstancesPaginator(clients.SSOAdmin, &awssso.ListInstancesInput{})
	for ip.HasMorePages() {
		ipage, err := ip.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list identity center instances: %w", err)
		}
		for _, inst := range ipage.Instances {
			instanceArn := aws.ToString(inst.InstanceArn)
			pp := awssso.NewListPermissionSetsPaginator(clients.SSOAdmin, &awssso.ListPermissionSetsInput{
				InstanceArn: aws.String(instanceArn),
			})
			for pp.HasMorePages() {
				ppage, err := pp.NextPage(ctx)
				if err != nil {
					return objs, fmt.Errorf("list permission sets: %w", err)
				}
				for _, psArn := range ppage.PermissionSets {
					desc, err := clients.SSOAdmin.DescribePermissionSet(ctx, &awssso.DescribePermissionSetInput{
						InstanceArn:      aws.String(instanceArn),
						PermissionSetArn: aws.String(psArn),
					})
					if err != nil || desc.PermissionSet == nil {
						continue
					}
					ps := desc.PermissionSet
					cr := &awsv1alpha1.PermissionSet{
						ObjectMeta: export.ObjectMeta(aws.ToString(ps.Name), opts),
						Spec: awsv1alpha1.PermissionSetSpec{
							InstanceArn:     instanceArn,
							Name:            aws.ToString(ps.Name),
							Description:     aws.ToString(ps.Description),
							SessionDuration: aws.ToString(ps.SessionDuration),
							RelayState:      aws.ToString(ps.RelayState),
						},
					}

					mp := awssso.NewListManagedPoliciesInPermissionSetPaginator(clients.SSOAdmin, &awssso.ListManagedPoliciesInPermissionSetInput{
						InstanceArn:      aws.String(instanceArn),
						PermissionSetArn: aws.String(psArn),
					})
					for mp.HasMorePages() {
						mpage, err := mp.NextPage(ctx)
						if err != nil {
							break
						}
						for _, pol := range mpage.AttachedManagedPolicies {
							cr.Spec.ManagedPolicies = append(cr.Spec.ManagedPolicies, aws.ToString(pol.Arn))
						}
					}

					if inline, err := clients.SSOAdmin.GetInlinePolicyForPermissionSet(ctx, &awssso.GetInlinePolicyForPermissionSetInput{
						InstanceArn:      aws.String(instanceArn),
						PermissionSetArn: aws.String(psArn),
					}); err == nil {
						cr.Spec.InlinePolicy = aws.ToString(inline.InlinePolicy)
					}

					if tagsOut, err := clients.SSOAdmin.ListTagsForResource(ctx, &awssso.ListTagsForResourceInput{
						InstanceArn: aws.String(instanceArn),
						ResourceArn: aws.String(psArn),
					}); err == nil {
						m := map[string]string{}
						for _, t := range tagsOut.Tags {
							m[aws.ToString(t.Key)] = aws.ToString(t.Value)
						}
						cr.Spec.Tags = export.TagMap(m)
					}

					opts.Index.Add(psArn, cr.Name)
					objs = append(objs, cr)
				}
			}
		}
	}
	return objs, nil
}

func exportSSOAssignments(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	ip := awssso.NewListInstancesPaginator(clients.SSOAdmin, &awssso.ListInstancesInput{})
	for ip.HasMorePages() {
		ipage, err := ip.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list identity center instances: %w", err)
		}
		for _, inst := range ipage.Instances {
			instanceArn := aws.ToString(inst.InstanceArn)
			pp := awssso.NewListPermissionSetsPaginator(clients.SSOAdmin, &awssso.ListPermissionSetsInput{
				InstanceArn: aws.String(instanceArn),
			})
			for pp.HasMorePages() {
				ppage, err := pp.NextPage(ctx)
				if err != nil {
					return objs, fmt.Errorf("list permission sets: %w", err)
				}
				for _, psArn := range ppage.PermissionSets {
					ap := awssso.NewListAccountsForProvisionedPermissionSetPaginator(clients.SSOAdmin, &awssso.ListAccountsForProvisionedPermissionSetInput{
						InstanceArn:      aws.String(instanceArn),
						PermissionSetArn: aws.String(psArn),
					})
					for ap.HasMorePages() {
						apage, err := ap.NextPage(ctx)
						if err != nil {
							break
						}
						for _, accountID := range apage.AccountIds {
							aap := awssso.NewListAccountAssignmentsPaginator(clients.SSOAdmin, &awssso.ListAccountAssignmentsInput{
								InstanceArn:      aws.String(instanceArn),
								AccountId:        aws.String(accountID),
								PermissionSetArn: aws.String(psArn),
							})
							for aap.HasMorePages() {
								aapage, err := aap.NextPage(ctx)
								if err != nil {
									break
								}
								for _, a := range aapage.AccountAssignments {
									principalID := aws.ToString(a.PrincipalId)
									cr := &awsv1alpha1.SSOAssignment{
										ObjectMeta: export.ObjectMeta(fmt.Sprintf("%s-%s-%s", psArnShortName(psArn), accountID, principalID), opts),
										Spec: awsv1alpha1.SSOAssignmentSpec{
											InstanceArn:   instanceArn,
											PrincipalType: string(a.PrincipalType),
											PrincipalId:   principalID,
											TargetType:    "AWS_ACCOUNT",
											TargetId:      accountID,
										},
									}
									if name, ok := opts.Index.Lookup(psArn); ok && name != "" {
										cr.Spec.PermissionSetRef = awsv1alpha1.PermissionSetRef{Name: name}
									} else {
										cr.Spec.PermissionSetRef = awsv1alpha1.PermissionSetRef{ARN: psArn}
									}
									objs = append(objs, cr)
								}
							}
						}
					}
				}
			}
		}
	}
	return objs, nil
}

// psArnShortName extracts the trailing ID segment of a permission set ARN for
// building readable CR names.
func psArnShortName(arn string) string {
	for i := len(arn) - 1; i >= 0; i-- {
		if arn[i] == '/' {
			return arn[i+1:]
		}
	}
	return arn
}

func exportResourceShares(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsram.NewGetResourceSharesPaginator(clients.RAM, &awsram.GetResourceSharesInput{
		ResourceOwner: ramtypes.ResourceOwnerSelf,
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("get resource shares: %w", err)
		}
		for _, share := range page.ResourceShares {
			if share.Status != ramtypes.ResourceShareStatusActive {
				continue
			}
			// Shares synthesized from resource policies are not managed here.
			if share.FeatureSet == ramtypes.ResourceShareFeatureSetCreatedFromPolicy {
				continue
			}
			shareArn := aws.ToString(share.ResourceShareArn)
			cr := &awsv1alpha1.ResourceShare{
				ObjectMeta: export.ObjectMeta(aws.ToString(share.Name), opts),
				Spec: awsv1alpha1.ResourceShareSpec{
					Name:                    aws.ToString(share.Name),
					AllowExternalPrincipals: aws.ToBool(share.AllowExternalPrincipals),
				},
			}
			if len(share.Tags) > 0 {
				m := map[string]string{}
				for _, t := range share.Tags {
					m[aws.ToString(t.Key)] = aws.ToString(t.Value)
				}
				cr.Spec.Tags = export.TagMap(m)
			}

			rp := awsram.NewListResourcesPaginator(clients.RAM, &awsram.ListResourcesInput{
				ResourceOwner:     ramtypes.ResourceOwnerSelf,
				ResourceShareArns: []string{shareArn},
			})
			for rp.HasMorePages() {
				rpage, err := rp.NextPage(ctx)
				if err != nil {
					break
				}
				for _, res := range rpage.Resources {
					cr.Spec.ResourceArns = append(cr.Spec.ResourceArns, aws.ToString(res.Arn))
				}
			}

			pp := awsram.NewListPrincipalsPaginator(clients.RAM, &awsram.ListPrincipalsInput{
				ResourceOwner:     ramtypes.ResourceOwnerSelf,
				ResourceShareArns: []string{shareArn},
			})
			for pp.HasMorePages() {
				ppage, err := pp.NextPage(ctx)
				if err != nil {
					break
				}
				for _, pr := range ppage.Principals {
					cr.Spec.Principals = append(cr.Spec.Principals, aws.ToString(pr.Id))
				}
			}

			opts.Index.Add(shareArn, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportSCPortfolios(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awssc.NewListPortfoliosPaginator(clients.ServiceCatalog, &awssc.ListPortfoliosInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list portfolios: %w", err)
		}
		for _, pf := range page.PortfolioDetails {
			id := aws.ToString(pf.Id)
			cr := &awsv1alpha1.SCPortfolio{
				ObjectMeta: export.ObjectMeta(aws.ToString(pf.DisplayName), opts),
				Spec: awsv1alpha1.SCPortfolioSpec{
					DisplayName:  aws.ToString(pf.DisplayName),
					ProviderName: aws.ToString(pf.ProviderName),
					Description:  aws.ToString(pf.Description),
				},
			}
			if desc, err := clients.ServiceCatalog.DescribePortfolio(ctx, &awssc.DescribePortfolioInput{Id: aws.String(id)}); err == nil {
				m := map[string]string{}
				for _, t := range desc.Tags {
					m[aws.ToString(t.Key)] = aws.ToString(t.Value)
				}
				cr.Spec.Tags = export.TagMap(m)
			}
			opts.Index.Add(id, cr.Name)
			opts.Index.Add(aws.ToString(pf.ARN), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportSCProducts(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awssc.NewSearchProductsAsAdminPaginator(clients.ServiceCatalog, &awssc.SearchProductsAsAdminInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("search products as admin: %w", err)
		}
		for _, pvd := range page.ProductViewDetails {
			if pvd.ProductViewSummary == nil {
				continue
			}
			sum := pvd.ProductViewSummary
			productID := aws.ToString(sum.ProductId)
			cr := &awsv1alpha1.SCProduct{
				ObjectMeta: export.ObjectMeta(aws.ToString(sum.Name), opts),
				Spec: awsv1alpha1.SCProductSpec{
					Name:         aws.ToString(sum.Name),
					Owner:        aws.ToString(sum.Owner),
					ProductType:  string(sum.Type),
					Description:  aws.ToString(sum.ShortDescription),
					Distributor:  aws.ToString(sum.Distributor),
					SupportEmail: aws.ToString(sum.SupportEmail),
					// The original template location cannot always be
					// recovered; overwritten below when the provisioning
					// artifact info exposes it.
					ProvisioningArtifact: awsv1alpha1.SCProvisioningArtifact{
						Name:        "v1",
						TemplateURL: "CHANGEME", // template URL does not round-trip via the API
					},
				},
			}

			if desc, err := clients.ServiceCatalog.DescribeProductAsAdmin(ctx, &awssc.DescribeProductAsAdminInput{Id: aws.String(productID)}); err == nil {
				m := map[string]string{}
				for _, t := range desc.Tags {
					m[aws.ToString(t.Key)] = aws.ToString(t.Value)
				}
				cr.Spec.Tags = export.TagMap(m)
				if len(desc.ProvisioningArtifactSummaries) > 0 {
					pa := desc.ProvisioningArtifactSummaries[0]
					cr.Spec.ProvisioningArtifact.Name = aws.ToString(pa.Name)
					cr.Spec.ProvisioningArtifact.Description = aws.ToString(pa.Description)
					if paDesc, err := clients.ServiceCatalog.DescribeProvisioningArtifact(ctx, &awssc.DescribeProvisioningArtifactInput{
						ProductId:              aws.String(productID),
						ProvisioningArtifactId: pa.Id,
					}); err == nil {
						if url, ok := paDesc.Info["TemplateUrl"]; ok && url != "" {
							cr.Spec.ProvisioningArtifact.TemplateURL = url
						} else if url, ok := paDesc.Info["LoadTemplateFromURL"]; ok && url != "" {
							cr.Spec.ProvisioningArtifact.TemplateURL = url
						}
					}
				}
			}

			opts.Index.Add(productID, cr.Name)
			opts.Index.Add(aws.ToString(pvd.ProductARN), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportSCPortfolioProductAssociations(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awssc.NewSearchProductsAsAdminPaginator(clients.ServiceCatalog, &awssc.SearchProductsAsAdminInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("search products as admin: %w", err)
		}
		for _, pvd := range page.ProductViewDetails {
			if pvd.ProductViewSummary == nil {
				continue
			}
			productID := aws.ToString(pvd.ProductViewSummary.ProductId)
			pp := awssc.NewListPortfoliosForProductPaginator(clients.ServiceCatalog, &awssc.ListPortfoliosForProductInput{
				ProductId: aws.String(productID),
			})
			for pp.HasMorePages() {
				ppage, err := pp.NextPage(ctx)
				if err != nil {
					break
				}
				for _, pf := range ppage.PortfolioDetails {
					portfolioID := aws.ToString(pf.Id)
					cr := &awsv1alpha1.SCPortfolioProductAssociation{
						ObjectMeta: export.ObjectMeta(fmt.Sprintf("%s-%s", productID, portfolioID), opts),
					}
					if name, ok := opts.Index.Lookup(productID); ok && name != "" {
						cr.Spec.ProductRef = awsv1alpha1.SCProductRef{Name: name}
					} else {
						cr.Spec.ProductRef = awsv1alpha1.SCProductRef{ProductID: productID}
					}
					if name, ok := opts.Index.Lookup(portfolioID); ok && name != "" {
						cr.Spec.PortfolioRef = awsv1alpha1.SCPortfolioRef{Name: name}
					} else {
						cr.Spec.PortfolioRef = awsv1alpha1.SCPortfolioRef{PortfolioID: portfolioID}
					}
					objs = append(objs, cr)
				}
			}
		}
	}
	return objs, nil
}

// exportCTEnabledControls walks the organization tree to collect OU ARNs
// (Control Tower controls target OUs) and lists enabled controls per OU.
func exportCTEnabledControls(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var ouArns []string

	var walk func(parentID string) error
	walk = func(parentID string) error {
		p := awsorgs.NewListOrganizationalUnitsForParentPaginator(clients.Organizations, &awsorgs.ListOrganizationalUnitsForParentInput{
			ParentId: aws.String(parentID),
		})
		for p.HasMorePages() {
			page, err := p.NextPage(ctx)
			if err != nil {
				return fmt.Errorf("list OUs for parent %s: %w", parentID, err)
			}
			for _, ou := range page.OrganizationalUnits {
				ouArns = append(ouArns, aws.ToString(ou.Arn))
				if err := walk(aws.ToString(ou.Id)); err != nil {
					return err
				}
			}
		}
		return nil
	}

	rp := awsorgs.NewListRootsPaginator(clients.Organizations, &awsorgs.ListRootsInput{})
	for rp.HasMorePages() {
		page, err := rp.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list roots: %w", err)
		}
		for _, root := range page.Roots {
			if err := walk(aws.ToString(root.Id)); err != nil {
				return nil, err
			}
		}
	}

	var objs []client.Object
	for _, ouArn := range ouArns {
		p := awsct.NewListEnabledControlsPaginator(clients.ControlTower, &awsct.ListEnabledControlsInput{
			TargetIdentifier: aws.String(ouArn),
		})
		for p.HasMorePages() {
			page, err := p.NextPage(ctx)
			if err != nil {
				// A non-Control-Tower OU (or region) is expected; skip it.
				break
			}
			for _, ec := range page.EnabledControls {
				controlID := aws.ToString(ec.ControlIdentifier)
				cr := &awsv1alpha1.CTEnabledControl{
					ObjectMeta: export.ObjectMeta(fmt.Sprintf("%s-%s", ctControlShortName(controlID), ouArnShortName(ouArn)), opts),
					Spec: awsv1alpha1.CTEnabledControlSpec{
						ControlIdentifier: controlID,
						TargetIdentifier:  ouArn,
					},
				}
				opts.Index.Add(aws.ToString(ec.Arn), cr.Name)
				objs = append(objs, cr)
			}
		}
	}
	return objs, nil
}

func ctControlShortName(arn string) string {
	for i := len(arn) - 1; i >= 0; i-- {
		if arn[i] == '/' {
			return arn[i+1:]
		}
	}
	return arn
}

func ouArnShortName(arn string) string {
	for i := len(arn) - 1; i >= 0; i-- {
		if arn[i] == '/' {
			return arn[i+1:]
		}
	}
	return arn
}
