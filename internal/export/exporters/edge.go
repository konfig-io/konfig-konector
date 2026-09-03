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
	awscf "github.com/aws/aws-sdk-go-v2/service/cloudfront"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	awsr53r "github.com/aws/aws-sdk-go-v2/service/route53resolver"
	r53rtypes "github.com/aws/aws-sdk-go-v2/service/route53resolver/types"
	awsshield "github.com/aws/aws-sdk-go-v2/service/shield"
	awswaf "github.com/aws/aws-sdk-go-v2/service/wafv2"
	waftypes "github.com/aws/aws-sdk-go-v2/service/wafv2/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	awsclient "github.com/konfig-io/konfig-konector/internal/aws"
	"github.com/konfig-io/konfig-konector/internal/export"
)

func init() {
	// Cache policies and OACs before distributions so behaviors could resolve
	// against the index; WAF sets before WebACLs for the same reason.
	export.Register(export.Exporter{Kind: "CloudFrontCachePolicy", Service: "cloudfront", Order: 60, Fn: exportCloudFrontCachePolicies})
	export.Register(export.Exporter{Kind: "CloudFrontOriginAccessControl", Service: "cloudfront", Order: 60, Fn: exportCloudFrontOACs})
	export.Register(export.Exporter{Kind: "CloudFrontFunction", Service: "cloudfront", Order: 61, Fn: exportCloudFrontFunctions})
	export.Register(export.Exporter{Kind: "CloudFrontDistribution", Service: "cloudfront", Order: 62, Fn: exportCloudFrontDistributions})
	export.Register(export.Exporter{Kind: "IPSet", Service: "wafv2", Order: 63, Fn: exportWAFIPSets})
	export.Register(export.Exporter{Kind: "WAFRegexPatternSet", Service: "wafv2", Order: 63, Fn: exportWAFRegexPatternSets})
	export.Register(export.Exporter{Kind: "WAFRuleGroup", Service: "wafv2", Order: 64, Fn: exportWAFRuleGroups})
	export.Register(export.Exporter{Kind: "WebACL", Service: "wafv2", Order: 65, Fn: exportWebACLs})
	export.Register(export.Exporter{Kind: "ShieldProtection", Service: "shield", Order: 66, Fn: exportShieldProtections})
	export.Register(export.Exporter{Kind: "ResolverEndpoint", Service: "route53resolver", Order: 67, Fn: exportResolverEndpoints})
	export.Register(export.Exporter{Kind: "ResolverRule", Service: "route53resolver", Order: 68, Fn: exportResolverRules})
}

// wafScopes are attempted in order; the CLOUDFRONT scope only works in
// us-east-1, so failures there are skipped rather than fatal.
var wafScopes = []waftypes.Scope{waftypes.ScopeRegional, waftypes.ScopeCloudfront}

// wafTags fetches WAFv2 resource tags, returning nil on failure.
func wafTags(ctx context.Context, clients *awsclient.Clients, arn *string) map[string]string {
	out, err := clients.WAFv2.ListTagsForResource(ctx, &awswaf.ListTagsForResourceInput{ResourceARN: arn})
	if err != nil || out.TagInfoForResource == nil {
		return nil
	}
	m := make(map[string]string, len(out.TagInfoForResource.TagList))
	for _, t := range out.TagInfoForResource.TagList {
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return export.TagMap(m)
}

// cfTags fetches CloudFront resource tags, returning nil on failure.
func cfTags(ctx context.Context, clients *awsclient.Clients, arn *string) map[string]string {
	out, err := clients.CloudFront.ListTagsForResource(ctx, &awscf.ListTagsForResourceInput{Resource: arn})
	if err != nil || out.Tags == nil {
		return nil
	}
	m := make(map[string]string, len(out.Tags.Items))
	for _, t := range out.Tags.Items {
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return export.TagMap(m)
}

func exportCloudFrontDistributions(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awscf.NewListDistributionsPaginator(clients.CloudFront, &awscf.ListDistributionsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list distributions: %w", err)
		}
		if page.DistributionList == nil {
			continue
		}
		for _, d := range page.DistributionList.Items {
			id := aws.ToString(d.Id)
			cr := &awsv1alpha1.CloudFrontDistribution{
				ObjectMeta: export.ObjectMeta("distribution-"+id, opts),
				Spec: awsv1alpha1.CloudFrontDistributionSpec{
					Comment:    aws.ToString(d.Comment),
					Enabled:    d.Enabled,
					PriceClass: string(d.PriceClass),
				},
			}
			if d.Origins != nil {
				for _, o := range d.Origins.Items {
					cr.Spec.Origins = append(cr.Spec.Origins, awsv1alpha1.CFOrigin{
						ID:         aws.ToString(o.Id),
						DomainName: aws.ToString(o.DomainName),
						OriginPath: aws.ToString(o.OriginPath),
					})
				}
			}
			if d.DefaultCacheBehavior != nil {
				dcb := awsv1alpha1.CFDefaultCacheBehavior{
					TargetOriginID:       aws.ToString(d.DefaultCacheBehavior.TargetOriginId),
					ViewerProtocolPolicy: string(d.DefaultCacheBehavior.ViewerProtocolPolicy),
					CachePolicyID:        aws.ToString(d.DefaultCacheBehavior.CachePolicyId),
				}
				if am := d.DefaultCacheBehavior.AllowedMethods; am != nil {
					for _, m := range am.Items {
						dcb.AllowedMethods = append(dcb.AllowedMethods, string(m))
					}
				}
				cr.Spec.DefaultCacheBehavior = dcb
			}
			if d.Aliases != nil {
				cr.Spec.Aliases = d.Aliases.Items
			}
			cr.Spec.Tags = cfTags(ctx, clients, d.ARN)
			opts.Index.Add(id, cr.Name)
			opts.Index.Add(aws.ToString(d.ARN), cr.Name)
			opts.Index.Add(aws.ToString(d.DomainName), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportCloudFrontCachePolicies(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	// No SDK paginator for ListCachePolicies; walk NextMarker manually.
	// Type=custom excludes the AWS Managed-* policies.
	var marker *string
	for {
		page, err := clients.CloudFront.ListCachePolicies(ctx, &awscf.ListCachePoliciesInput{
			Type:   cftypes.CachePolicyTypeCustom,
			Marker: marker,
		})
		if err != nil {
			return nil, fmt.Errorf("list cache policies: %w", err)
		}
		if page.CachePolicyList == nil {
			break
		}
		for _, s := range page.CachePolicyList.Items {
			if s.CachePolicy == nil || s.CachePolicy.CachePolicyConfig == nil {
				continue
			}
			cfg := s.CachePolicy.CachePolicyConfig
			name := aws.ToString(cfg.Name)
			cr := &awsv1alpha1.CloudFrontCachePolicy{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.CloudFrontCachePolicySpec{
					Name:       name,
					MinTTL:     aws.ToInt64(cfg.MinTTL),
					DefaultTTL: cfg.DefaultTTL,
					MaxTTL:     cfg.MaxTTL,
					Comment:    aws.ToString(cfg.Comment),
				},
			}
			opts.Index.Add(aws.ToString(s.CachePolicy.Id), cr.Name)
			objs = append(objs, cr)
		}
		if aws.ToString(page.CachePolicyList.NextMarker) == "" {
			break
		}
		marker = page.CachePolicyList.NextMarker
	}
	return objs, nil
}

func exportCloudFrontOACs(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	var marker *string
	for {
		page, err := clients.CloudFront.ListOriginAccessControls(ctx, &awscf.ListOriginAccessControlsInput{Marker: marker})
		if err != nil {
			return nil, fmt.Errorf("list origin access controls: %w", err)
		}
		if page.OriginAccessControlList == nil {
			break
		}
		for _, o := range page.OriginAccessControlList.Items {
			name := aws.ToString(o.Name)
			cr := &awsv1alpha1.CloudFrontOriginAccessControl{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.CloudFrontOriginAccessControlSpec{
					Name:                          name,
					OriginAccessControlOriginType: string(o.OriginAccessControlOriginType),
					SigningBehavior:               string(o.SigningBehavior),
					SigningProtocol:               string(o.SigningProtocol),
					Description:                   aws.ToString(o.Description),
				},
			}
			opts.Index.Add(aws.ToString(o.Id), cr.Name)
			objs = append(objs, cr)
		}
		if aws.ToBool(page.OriginAccessControlList.IsTruncated) && aws.ToString(page.OriginAccessControlList.NextMarker) != "" {
			marker = page.OriginAccessControlList.NextMarker
			continue
		}
		break
	}
	return objs, nil
}

func exportCloudFrontFunctions(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	var marker *string
	for {
		page, err := clients.CloudFront.ListFunctions(ctx, &awscf.ListFunctionsInput{Marker: marker})
		if err != nil {
			return nil, fmt.Errorf("list functions: %w", err)
		}
		if page.FunctionList == nil {
			break
		}
		for _, f := range page.FunctionList.Items {
			name := aws.ToString(f.Name)
			cr := &awsv1alpha1.CloudFrontFunction{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.CloudFrontFunctionSpec{
					Name: name,
				},
			}
			if f.FunctionConfig != nil {
				cr.Spec.Runtime = string(f.FunctionConfig.Runtime)
				cr.Spec.Comment = aws.ToString(f.FunctionConfig.Comment)
			}
			stage := cftypes.FunctionStageDevelopment
			if f.FunctionMetadata != nil && f.FunctionMetadata.Stage != "" {
				stage = f.FunctionMetadata.Stage
			}
			code, err := clients.CloudFront.GetFunction(ctx, &awscf.GetFunctionInput{
				Name:  f.Name,
				Stage: stage,
			})
			if err != nil {
				continue
			}
			cr.Spec.FunctionCode = string(code.FunctionCode)
			if f.FunctionMetadata != nil {
				opts.Index.Add(aws.ToString(f.FunctionMetadata.FunctionARN), cr.Name)
			}
			objs = append(objs, cr)
		}
		if aws.ToString(page.FunctionList.NextMarker) == "" {
			break
		}
		marker = page.FunctionList.NextMarker
	}
	return objs, nil
}

func exportWebACLs(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	for _, scope := range wafScopes {
		var marker *string
		for {
			page, err := clients.WAFv2.ListWebACLs(ctx, &awswaf.ListWebACLsInput{Scope: scope, NextMarker: marker})
			if err != nil {
				// CLOUDFRONT scope is only valid in us-east-1; don't sink the
				// REGIONAL results already collected.
				if scope == waftypes.ScopeCloudfront {
					break
				}
				return nil, fmt.Errorf("list web acls (%s): %w", scope, err)
			}
			for _, s := range page.WebACLs {
				got, err := clients.WAFv2.GetWebACL(ctx, &awswaf.GetWebACLInput{
					Id: s.Id, Name: s.Name, Scope: scope,
				})
				if err != nil || got.WebACL == nil {
					continue
				}
				name := aws.ToString(s.Name)
				cr := &awsv1alpha1.WebACL{
					ObjectMeta: export.ObjectMeta(strings.ToLower(string(scope))+"-"+name, opts),
					Spec: awsv1alpha1.WebACLSpec{
						Name:        name,
						Scope:       string(scope),
						Description: aws.ToString(s.Description),
						Tags:        wafTags(ctx, clients, s.ARN),
					},
				}
				if da := got.WebACL.DefaultAction; da != nil {
					cr.Spec.DefaultAction = awsv1alpha1.WebACLDefaultAction{
						Allow: da.Allow != nil,
						Block: da.Block != nil,
					}
				}
				opts.Index.Add(aws.ToString(s.ARN), cr.Name)
				opts.Index.Add(aws.ToString(s.Id), cr.Name)
				objs = append(objs, cr)
			}
			if aws.ToString(page.NextMarker) == "" {
				break
			}
			marker = page.NextMarker
		}
	}
	return objs, nil
}

func exportWAFIPSets(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	for _, scope := range wafScopes {
		var marker *string
		for {
			page, err := clients.WAFv2.ListIPSets(ctx, &awswaf.ListIPSetsInput{Scope: scope, NextMarker: marker})
			if err != nil {
				if scope == waftypes.ScopeCloudfront {
					break
				}
				return nil, fmt.Errorf("list ip sets (%s): %w", scope, err)
			}
			for _, s := range page.IPSets {
				got, err := clients.WAFv2.GetIPSet(ctx, &awswaf.GetIPSetInput{
					Id: s.Id, Name: s.Name, Scope: scope,
				})
				if err != nil || got.IPSet == nil {
					continue
				}
				name := aws.ToString(s.Name)
				cr := &awsv1alpha1.IPSet{
					ObjectMeta: export.ObjectMeta(strings.ToLower(string(scope))+"-"+name, opts),
					Spec: awsv1alpha1.IPSetSpec{
						Name:             name,
						Scope:            string(scope),
						IPAddressVersion: string(got.IPSet.IPAddressVersion),
						Addresses:        got.IPSet.Addresses,
						Description:      aws.ToString(s.Description),
						Tags:             wafTags(ctx, clients, s.ARN),
					},
				}
				opts.Index.Add(aws.ToString(s.ARN), cr.Name)
				opts.Index.Add(aws.ToString(s.Id), cr.Name)
				objs = append(objs, cr)
			}
			if aws.ToString(page.NextMarker) == "" {
				break
			}
			marker = page.NextMarker
		}
	}
	return objs, nil
}

func exportWAFRegexPatternSets(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	for _, scope := range wafScopes {
		var marker *string
		for {
			page, err := clients.WAFv2.ListRegexPatternSets(ctx, &awswaf.ListRegexPatternSetsInput{Scope: scope, NextMarker: marker})
			if err != nil {
				if scope == waftypes.ScopeCloudfront {
					break
				}
				return nil, fmt.Errorf("list regex pattern sets (%s): %w", scope, err)
			}
			for _, s := range page.RegexPatternSets {
				got, err := clients.WAFv2.GetRegexPatternSet(ctx, &awswaf.GetRegexPatternSetInput{
					Id: s.Id, Name: s.Name, Scope: scope,
				})
				if err != nil || got.RegexPatternSet == nil {
					continue
				}
				name := aws.ToString(s.Name)
				cr := &awsv1alpha1.WAFRegexPatternSet{
					ObjectMeta: export.ObjectMeta(strings.ToLower(string(scope))+"-"+name, opts),
					Spec: awsv1alpha1.WAFRegexPatternSetSpec{
						Name:        name,
						Scope:       string(scope),
						Description: aws.ToString(s.Description),
						Tags:        wafTags(ctx, clients, s.ARN),
					},
				}
				for _, r := range got.RegexPatternSet.RegularExpressionList {
					cr.Spec.RegularExpressionList = append(cr.Spec.RegularExpressionList, aws.ToString(r.RegexString))
				}
				opts.Index.Add(aws.ToString(s.ARN), cr.Name)
				opts.Index.Add(aws.ToString(s.Id), cr.Name)
				objs = append(objs, cr)
			}
			if aws.ToString(page.NextMarker) == "" {
				break
			}
			marker = page.NextMarker
		}
	}
	return objs, nil
}

func exportWAFRuleGroups(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	for _, scope := range wafScopes {
		var marker *string
		for {
			page, err := clients.WAFv2.ListRuleGroups(ctx, &awswaf.ListRuleGroupsInput{Scope: scope, NextMarker: marker})
			if err != nil {
				if scope == waftypes.ScopeCloudfront {
					break
				}
				return nil, fmt.Errorf("list rule groups (%s): %w", scope, err)
			}
			for _, s := range page.RuleGroups {
				got, err := clients.WAFv2.GetRuleGroup(ctx, &awswaf.GetRuleGroupInput{
					Id: s.Id, Name: s.Name, Scope: scope,
				})
				if err != nil || got.RuleGroup == nil {
					continue
				}
				name := aws.ToString(s.Name)
				cr := &awsv1alpha1.WAFRuleGroup{
					ObjectMeta: export.ObjectMeta(strings.ToLower(string(scope))+"-"+name, opts),
					Spec: awsv1alpha1.WAFRuleGroupSpec{
						Name:        name,
						Scope:       string(scope),
						Capacity:    got.RuleGroup.Capacity,
						Description: aws.ToString(s.Description),
						Tags:        wafTags(ctx, clients, s.ARN),
					},
				}
				opts.Index.Add(aws.ToString(s.ARN), cr.Name)
				opts.Index.Add(aws.ToString(s.Id), cr.Name)
				objs = append(objs, cr)
			}
			if aws.ToString(page.NextMarker) == "" {
				break
			}
			marker = page.NextMarker
		}
	}
	return objs, nil
}

func exportShieldProtections(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsshield.NewListProtectionsPaginator(clients.Shield, &awsshield.ListProtectionsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			// Accounts without a Shield Advanced subscription fail here; the
			// caller reports it as a warning.
			return nil, fmt.Errorf("list protections: %w", err)
		}
		for _, prot := range page.Protections {
			name := aws.ToString(prot.Name)
			cr := &awsv1alpha1.ShieldProtection{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.ShieldProtectionSpec{
					Name:        name,
					ResourceARN: aws.ToString(prot.ResourceArn),
				},
			}
			tagsOut, err := clients.Shield.ListTagsForResource(ctx, &awsshield.ListTagsForResourceInput{
				ResourceARN: prot.ProtectionArn,
			})
			if err == nil {
				m := make(map[string]string, len(tagsOut.Tags))
				for _, t := range tagsOut.Tags {
					m[aws.ToString(t.Key)] = aws.ToString(t.Value)
				}
				cr.Spec.Tags = export.TagMap(m)
			}
			opts.Index.Add(aws.ToString(prot.Id), cr.Name)
			opts.Index.Add(aws.ToString(prot.ProtectionArn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

// resolverTags fetches Route53 Resolver resource tags, returning nil on failure.
func resolverTags(ctx context.Context, clients *awsclient.Clients, arn *string) map[string]string {
	out, err := clients.Route53Resolver.ListTagsForResource(ctx, &awsr53r.ListTagsForResourceInput{ResourceArn: arn})
	if err != nil {
		return nil
	}
	m := make(map[string]string, len(out.Tags))
	for _, t := range out.Tags {
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return export.TagMap(m)
}

func exportResolverEndpoints(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsr53r.NewListResolverEndpointsPaginator(clients.Route53Resolver, &awsr53r.ListResolverEndpointsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list resolver endpoints: %w", err)
		}
		for _, ep := range page.ResolverEndpoints {
			id := aws.ToString(ep.Id)
			name := aws.ToString(ep.Name)
			if name == "" {
				name = id
			}
			cr := &awsv1alpha1.ResolverEndpoint{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.ResolverEndpointSpec{
					Name:             name,
					Direction:        string(ep.Direction),
					SecurityGroupIDs: ep.SecurityGroupIds,
					Tags:             resolverTags(ctx, clients, ep.Arn),
				},
			}
			ipp := awsr53r.NewListResolverEndpointIpAddressesPaginator(clients.Route53Resolver, &awsr53r.ListResolverEndpointIpAddressesInput{
				ResolverEndpointId: ep.Id,
			})
			for ipp.HasMorePages() {
				ipPage, err := ipp.NextPage(ctx)
				if err != nil {
					break
				}
				for _, ip := range ipPage.IpAddresses {
					cr.Spec.IPAddresses = append(cr.Spec.IPAddresses, awsv1alpha1.ResolverIPAddress{
						SubnetID: aws.ToString(ip.SubnetId),
						IP:       aws.ToString(ip.Ip),
					})
				}
			}
			opts.Index.Add(id, cr.Name)
			opts.Index.Add(aws.ToString(ep.Arn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportResolverRules(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsr53r.NewListResolverRulesPaginator(clients.Route53Resolver, &awsr53r.ListResolverRulesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list resolver rules: %w", err)
		}
		for _, r := range page.ResolverRules {
			// SYSTEM/RECURSIVE rules are AWS-managed autodefined rules
			// (including the default internet resolver); only user-created
			// FORWARD rules are exportable.
			if r.RuleType != r53rtypes.RuleTypeOptionForward {
				continue
			}
			id := aws.ToString(r.Id)
			name := aws.ToString(r.Name)
			if name == "" {
				name = id
			}
			cr := &awsv1alpha1.ResolverRule{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.ResolverRuleSpec{
					Name:               name,
					DomainName:         aws.ToString(r.DomainName),
					RuleType:           string(r.RuleType),
					ResolverEndpointID: aws.ToString(r.ResolverEndpointId),
					Tags:               resolverTags(ctx, clients, r.Arn),
				},
			}
			for _, t := range r.TargetIps {
				cr.Spec.TargetIPs = append(cr.Spec.TargetIPs, awsv1alpha1.ResolverTargetIP{
					IP:   aws.ToString(t.Ip),
					Port: t.Port,
				})
			}
			opts.Index.Add(id, cr.Name)
			opts.Index.Add(aws.ToString(r.Arn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}
