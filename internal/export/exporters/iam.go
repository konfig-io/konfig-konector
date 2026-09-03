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
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsiam "github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	awsclient "github.com/konfig-io/konfig-konector/internal/aws"
	"github.com/konfig-io/konfig-konector/internal/export"
)

func init() {
	// Roles, policies, users, groups and identity providers must be indexed
	// before the attachment kinds that reference them.
	export.Register(export.Exporter{Kind: "IAMRole", Service: "iam", Order: 1, Fn: exportIAMRoles})
	export.Register(export.Exporter{Kind: "IAMPolicy", Service: "iam", Order: 2, Fn: exportIAMPolicies})
	export.Register(export.Exporter{Kind: "IAMUser", Service: "iam", Order: 3, Fn: exportIAMUsers})
	export.Register(export.Exporter{Kind: "IAMGroup", Service: "iam", Order: 4, Fn: exportIAMGroups})
	export.Register(export.Exporter{Kind: "IAMOIDCProvider", Service: "iam", Order: 5, Fn: exportIAMOIDCProviders})
	export.Register(export.Exporter{Kind: "IAMSAMLProvider", Service: "iam", Order: 5, Fn: exportIAMSAMLProviders})
	export.Register(export.Exporter{Kind: "IAMRolePolicy", Service: "iam", Order: 7, Fn: exportIAMRolePolicies})
	export.Register(export.Exporter{Kind: "IAMPolicyAttachment", Service: "iam", Order: 8, Fn: exportIAMPolicyAttachments})
	export.Register(export.Exporter{Kind: "IAMGroupPolicyAttachment", Service: "iam", Order: 8, Fn: exportIAMGroupPolicyAttachments})
	export.Register(export.Exporter{Kind: "IAMGroupMembership", Service: "iam", Order: 9, Fn: exportIAMGroupMemberships})
}

// iamTagMap converts IAM tag slices to a spec tag map, dropping aws: tags.
func iamTagMap(tags []iamtypes.Tag) map[string]string {
	if len(tags) == 0 {
		return nil
	}
	m := make(map[string]string, len(tags))
	for _, t := range tags {
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return export.TagMap(m)
}

// decodeIAMPolicyDocument URL-decodes an IAM policy document. The IAM API
// returns all policy documents (trust policies, inline policies, managed
// policy versions) URL-encoded.
func decodeIAMPolicyDocument(doc string) string {
	if d, err := url.QueryUnescape(doc); err == nil {
		return d
	}
	return doc
}

// skipIAMRolePath reports whether a role path belongs to an AWS
// service-linked role. Roles under /aws-service-role/ are created and owned
// by AWS services and cannot be user-managed; /service-role/ roles are
// user-created (console defaults) and are kept.
func skipIAMRolePath(path string) bool {
	return strings.HasPrefix(path, "/aws-service-role/")
}

func exportIAMRoles(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsiam.NewListRolesPaginator(clients.IAM, &awsiam.ListRolesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list roles: %w", err)
		}
		for _, r := range page.Roles {
			if skipIAMRolePath(aws.ToString(r.Path)) {
				continue
			}
			roleName := aws.ToString(r.RoleName)
			// ListRoles omits tags, description drift, and permissions
			// boundary details; GetRole returns the full role. Fall back to
			// the list entry on error.
			role := r
			if out, err := clients.IAM.GetRole(ctx, &awsiam.GetRoleInput{RoleName: r.RoleName}); err == nil && out.Role != nil {
				role = *out.Role
			}
			cr := &awsv1alpha1.IAMRole{
				ObjectMeta: export.ObjectMeta(roleName, opts),
				Spec: awsv1alpha1.IAMRoleSpec{
					RoleName:                 roleName,
					Description:              aws.ToString(role.Description),
					MaxSessionDuration:       aws.ToInt32(role.MaxSessionDuration),
					Path:                     aws.ToString(role.Path),
					AssumeRolePolicyDocument: decodeIAMPolicyDocument(aws.ToString(role.AssumeRolePolicyDocument)),
					Tags:                     iamTagMap(role.Tags),
				},
			}
			if role.PermissionsBoundary != nil {
				cr.Spec.PermissionsBoundary = aws.ToString(role.PermissionsBoundary.PermissionsBoundaryArn)
			}
			opts.Index.Add(aws.ToString(role.Arn), cr.Name)
			opts.Index.Add("iam-role/"+roleName, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportIAMPolicies(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	// Scope=Local restricts the listing to customer-managed policies —
	// AWS-managed policies must never be exported as CRs.
	p := awsiam.NewListPoliciesPaginator(clients.IAM, &awsiam.ListPoliciesInput{
		Scope: iamtypes.PolicyScopeTypeLocal,
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list policies: %w", err)
		}
		for _, pol := range page.Policies {
			arn := aws.ToString(pol.Arn)
			verOut, err := clients.IAM.GetPolicyVersion(ctx, &awsiam.GetPolicyVersionInput{
				PolicyArn: pol.Arn,
				VersionId: pol.DefaultVersionId,
			})
			if err != nil || verOut.PolicyVersion == nil {
				// Per-resource failure: skip the policy rather than abort.
				continue
			}
			cr := &awsv1alpha1.IAMPolicy{
				ObjectMeta: export.ObjectMeta(aws.ToString(pol.PolicyName), opts),
				Spec: awsv1alpha1.IAMPolicySpec{
					PolicyName:     aws.ToString(pol.PolicyName),
					Description:    aws.ToString(pol.Description),
					Path:           aws.ToString(pol.Path),
					PolicyDocument: decodeIAMPolicyDocument(aws.ToString(verOut.PolicyVersion.Document)),
				},
			}
			// ListPolicies does not populate tags; fetch them best-effort.
			tp := awsiam.NewListPolicyTagsPaginator(clients.IAM, &awsiam.ListPolicyTagsInput{PolicyArn: pol.Arn})
			var tags []iamtypes.Tag
			for tp.HasMorePages() {
				tpage, err := tp.NextPage(ctx)
				if err != nil {
					break
				}
				tags = append(tags, tpage.Tags...)
			}
			cr.Spec.Tags = iamTagMap(tags)
			opts.Index.Add(arn, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportIAMUsers(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsiam.NewListUsersPaginator(clients.IAM, &awsiam.ListUsersInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list users: %w", err)
		}
		for _, u := range page.Users {
			userName := aws.ToString(u.UserName)
			// ListUsers omits tags and permissions boundary; GetUser is
			// authoritative. Fall back to the list entry on error.
			user := u
			if out, err := clients.IAM.GetUser(ctx, &awsiam.GetUserInput{UserName: u.UserName}); err == nil && out.User != nil {
				user = *out.User
			}
			cr := &awsv1alpha1.IAMUser{
				ObjectMeta: export.ObjectMeta(userName, opts),
				Spec: awsv1alpha1.IAMUserSpec{
					UserName: userName,
					Path:     aws.ToString(user.Path),
					Tags:     iamTagMap(user.Tags),
				},
			}
			if user.PermissionsBoundary != nil {
				cr.Spec.PermissionsBoundary = aws.ToString(user.PermissionsBoundary.PermissionsBoundaryArn)
			}
			opts.Index.Add(aws.ToString(user.Arn), cr.Name)
			opts.Index.Add("iam-user/"+userName, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportIAMGroups(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsiam.NewListGroupsPaginator(clients.IAM, &awsiam.ListGroupsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list groups: %w", err)
		}
		for _, g := range page.Groups {
			groupName := aws.ToString(g.GroupName)
			cr := &awsv1alpha1.IAMGroup{
				ObjectMeta: export.ObjectMeta(groupName, opts),
				Spec: awsv1alpha1.IAMGroupSpec{
					GroupName: groupName,
					Path:      aws.ToString(g.Path),
				},
			}
			opts.Index.Add(aws.ToString(g.Arn), cr.Name)
			opts.Index.Add("iam-group/"+groupName, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportIAMGroupMemberships(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsiam.NewListGroupsPaginator(clients.IAM, &awsiam.ListGroupsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list groups: %w", err)
		}
		for _, g := range page.Groups {
			groupName := aws.ToString(g.GroupName)
			gp := awsiam.NewGetGroupPaginator(clients.IAM, &awsiam.GetGroupInput{GroupName: g.GroupName})
			for gp.HasMorePages() {
				gpage, err := gp.NextPage(ctx)
				if err != nil {
					// Per-group failure: skip the group's memberships.
					break
				}
				for _, u := range gpage.Users {
					userName := aws.ToString(u.UserName)
					cr := &awsv1alpha1.IAMGroupMembership{
						ObjectMeta: export.ObjectMeta(groupName+"-"+userName, opts),
						Spec:       awsv1alpha1.IAMGroupMembershipSpec{},
					}
					if crName, ok := opts.Index.Lookup("iam-group/" + groupName); ok {
						cr.Spec.GroupRef = awsv1alpha1.GroupRef{Name: crName}
					} else {
						cr.Spec.GroupRef = awsv1alpha1.GroupRef{GroupName: groupName}
					}
					if crName, ok := opts.Index.Lookup("iam-user/" + userName); ok {
						cr.Spec.UserRef = awsv1alpha1.UserRef{Name: crName}
					} else {
						cr.Spec.UserRef = awsv1alpha1.UserRef{UserName: userName}
					}
					objs = append(objs, cr)
				}
			}
		}
	}
	return objs, nil
}

func exportIAMRolePolicies(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsiam.NewListRolesPaginator(clients.IAM, &awsiam.ListRolesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list roles: %w", err)
		}
		for _, r := range page.Roles {
			if skipIAMRolePath(aws.ToString(r.Path)) {
				continue
			}
			roleName := aws.ToString(r.RoleName)
			// Only export inline policies of roles exported in this run.
			roleCR, exported := opts.Index.Lookup("iam-role/" + roleName)
			if !exported {
				continue
			}
			lp := awsiam.NewListRolePoliciesPaginator(clients.IAM, &awsiam.ListRolePoliciesInput{RoleName: r.RoleName})
			for lp.HasMorePages() {
				lpage, err := lp.NextPage(ctx)
				if err != nil {
					break
				}
				for _, polName := range lpage.PolicyNames {
					out, err := clients.IAM.GetRolePolicy(ctx, &awsiam.GetRolePolicyInput{
						RoleName:   r.RoleName,
						PolicyName: aws.String(polName),
					})
					if err != nil {
						// Per-policy failure: continue with the next one.
						continue
					}
					cr := &awsv1alpha1.IAMRolePolicy{
						ObjectMeta: export.ObjectMeta(roleName+"-"+polName, opts),
						Spec: awsv1alpha1.IAMRolePolicySpec{
							RoleRef:    awsv1alpha1.RoleRef{Name: roleCR},
							PolicyName: polName,
							// GetRolePolicy returns the document URL-encoded.
							PolicyDocument: decodeIAMPolicyDocument(aws.ToString(out.PolicyDocument)),
						},
					}
					objs = append(objs, cr)
				}
			}
		}
	}
	return objs, nil
}

func exportIAMPolicyAttachments(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsiam.NewListRolesPaginator(clients.IAM, &awsiam.ListRolesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list roles: %w", err)
		}
		for _, r := range page.Roles {
			if skipIAMRolePath(aws.ToString(r.Path)) {
				continue
			}
			roleName := aws.ToString(r.RoleName)
			// Only export attachments for roles exported in this run.
			roleCR, exported := opts.Index.Lookup("iam-role/" + roleName)
			if !exported {
				continue
			}
			ap := awsiam.NewListAttachedRolePoliciesPaginator(clients.IAM, &awsiam.ListAttachedRolePoliciesInput{RoleName: r.RoleName})
			for ap.HasMorePages() {
				apage, err := ap.NextPage(ctx)
				if err != nil {
					break
				}
				for _, att := range apage.AttachedPolicies {
					polARN := aws.ToString(att.PolicyArn)
					cr := &awsv1alpha1.IAMPolicyAttachment{
						ObjectMeta: export.ObjectMeta(roleName+"-"+aws.ToString(att.PolicyName), opts),
						Spec: awsv1alpha1.IAMPolicyAttachmentSpec{
							RoleRef: awsv1alpha1.RoleRef{Name: roleCR},
						},
					}
					// Customer-managed policies exported in this run are
					// referenced by CR name; AWS-managed ones by direct ARN.
					if crName, ok := opts.Index.Lookup(polARN); ok {
						cr.Spec.PolicyRef = awsv1alpha1.PolicyRef{Name: crName}
					} else {
						cr.Spec.PolicyRef = awsv1alpha1.PolicyRef{ARN: polARN}
					}
					objs = append(objs, cr)
				}
			}
		}
	}
	return objs, nil
}

func exportIAMGroupPolicyAttachments(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsiam.NewListGroupsPaginator(clients.IAM, &awsiam.ListGroupsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list groups: %w", err)
		}
		for _, g := range page.Groups {
			groupName := aws.ToString(g.GroupName)
			ap := awsiam.NewListAttachedGroupPoliciesPaginator(clients.IAM, &awsiam.ListAttachedGroupPoliciesInput{GroupName: g.GroupName})
			for ap.HasMorePages() {
				apage, err := ap.NextPage(ctx)
				if err != nil {
					break
				}
				for _, att := range apage.AttachedPolicies {
					polARN := aws.ToString(att.PolicyArn)
					cr := &awsv1alpha1.IAMGroupPolicyAttachment{
						ObjectMeta: export.ObjectMeta(groupName+"-"+aws.ToString(att.PolicyName), opts),
						Spec:       awsv1alpha1.IAMGroupPolicyAttachmentSpec{},
					}
					if crName, ok := opts.Index.Lookup("iam-group/" + groupName); ok {
						cr.Spec.GroupRef = awsv1alpha1.GroupRef{Name: crName}
					} else {
						cr.Spec.GroupRef = awsv1alpha1.GroupRef{GroupName: groupName}
					}
					if crName, ok := opts.Index.Lookup(polARN); ok {
						cr.Spec.PolicyRef = awsv1alpha1.PolicyRef{Name: crName}
					} else {
						cr.Spec.PolicyRef = awsv1alpha1.PolicyRef{ARN: polARN}
					}
					objs = append(objs, cr)
				}
			}
		}
	}
	return objs, nil
}

func exportIAMOIDCProviders(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	// ListOpenIDConnectProviders is not paginated in the IAM API.
	out, err := clients.IAM.ListOpenIDConnectProviders(ctx, &awsiam.ListOpenIDConnectProvidersInput{})
	if err != nil {
		return nil, fmt.Errorf("list OIDC providers: %w", err)
	}
	for _, entry := range out.OpenIDConnectProviderList {
		arn := aws.ToString(entry.Arn)
		det, err := clients.IAM.GetOpenIDConnectProvider(ctx, &awsiam.GetOpenIDConnectProviderInput{
			OpenIDConnectProviderArn: entry.Arn,
		})
		if err != nil {
			// Per-resource failure: continue with the next provider.
			continue
		}
		// GetOpenIDConnectProvider returns the URL without its scheme;
		// CreateOpenIDConnectProvider requires the https:// prefix, so
		// restore it to round-trip.
		u := aws.ToString(det.Url)
		if u != "" && !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://") {
			u = "https://" + u
		}
		host := arn[strings.LastIndex(arn, "/")+1:]
		cr := &awsv1alpha1.IAMOIDCProvider{
			ObjectMeta: export.ObjectMeta("oidc-"+host, opts),
			Spec: awsv1alpha1.IAMOIDCProviderSpec{
				URL:            u,
				ClientIDList:   det.ClientIDList,
				ThumbprintList: det.ThumbprintList,
				Tags:           iamTagMap(det.Tags),
			},
		}
		opts.Index.Add(arn, cr.Name)
		objs = append(objs, cr)
	}
	return objs, nil
}

func exportIAMSAMLProviders(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	// ListSAMLProviders is not paginated in the IAM API.
	out, err := clients.IAM.ListSAMLProviders(ctx, &awsiam.ListSAMLProvidersInput{})
	if err != nil {
		return nil, fmt.Errorf("list SAML providers: %w", err)
	}
	for _, entry := range out.SAMLProviderList {
		arn := aws.ToString(entry.Arn)
		// The provider name is the ARN suffix: arn:aws:iam::123:saml-provider/NAME
		name := arn[strings.LastIndex(arn, "/")+1:]
		det, err := clients.IAM.GetSAMLProvider(ctx, &awsiam.GetSAMLProviderInput{SAMLProviderArn: entry.Arn})
		if err != nil {
			// Per-resource failure: continue with the next provider.
			continue
		}
		cr := &awsv1alpha1.IAMSAMLProvider{
			ObjectMeta: export.ObjectMeta("saml-"+name, opts),
			Spec: awsv1alpha1.IAMSAMLProviderSpec{
				Name: name,
				// The SAML metadata document is the IdP's public metadata
				// XML (certificates are public signing certs, not secrets).
				SAMLMetadataDocument: aws.ToString(det.SAMLMetadataDocument),
				Tags:                 iamTagMap(det.Tags),
			},
		}
		opts.Index.Add(arn, cr.Name)
		objs = append(objs, cr)
	}
	return objs, nil
}
