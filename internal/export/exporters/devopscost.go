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
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsamp "github.com/aws/aws-sdk-go-v2/service/amp"
	awsbudgets "github.com/aws/aws-sdk-go-v2/service/budgets"
	awscodeartifact "github.com/aws/aws-sdk-go-v2/service/codeartifact"
	awsce "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	awsgrafana "github.com/aws/aws-sdk-go-v2/service/grafana"
	awsxray "github.com/aws/aws-sdk-go-v2/service/xray"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	awsclient "github.com/konfig-io/konfig-konector/internal/aws"
	"github.com/konfig-io/konfig-konector/internal/export"
)

func init() {
	// Domains before repositories; workspaces before namespaces/definitions;
	// monitors before subscriptions.
	export.Register(export.Exporter{Kind: "CodeArtifactDomain", Service: "codeartifact", Order: 110, Fn: exportCodeArtifactDomains})
	export.Register(export.Exporter{Kind: "CodeArtifactRepository", Service: "codeartifact", Order: 111, Fn: exportCodeArtifactRepositories})
	export.Register(export.Exporter{Kind: "XRayGroup", Service: "xray", Order: 110, Fn: exportXRayGroups})
	export.Register(export.Exporter{Kind: "XRaySamplingRule", Service: "xray", Order: 110, Fn: exportXRaySamplingRules})
	export.Register(export.Exporter{Kind: "PrometheusWorkspace", Service: "amp", Order: 110, Fn: exportPrometheusWorkspaces})
	export.Register(export.Exporter{Kind: "PrometheusRuleGroupsNamespace", Service: "amp", Order: 111, Fn: exportPrometheusRuleGroupsNamespaces})
	export.Register(export.Exporter{Kind: "PrometheusAlertManagerDefinition", Service: "amp", Order: 112, Fn: exportPrometheusAlertManagerDefinitions})
	export.Register(export.Exporter{Kind: "GrafanaWorkspace", Service: "grafana", Order: 112, Fn: exportGrafanaWorkspaces})
	export.Register(export.Exporter{Kind: "CostAnomalyMonitor", Service: "costexplorer", Order: 113, Fn: exportCostAnomalyMonitors})
	export.Register(export.Exporter{Kind: "CostAnomalySubscription", Service: "costexplorer", Order: 114, Fn: exportCostAnomalySubscriptions})
	export.Register(export.Exporter{Kind: "Budget", Service: "budgets", Order: 114, Fn: exportBudgets})
}

func exportCodeArtifactDomains(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awscodeartifact.NewListDomainsPaginator(clients.CodeArtifact, &awscodeartifact.ListDomainsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list codeartifact domains: %w", err)
		}
		for _, d := range page.Domains {
			name := aws.ToString(d.Name)
			cr := &awsv1alpha1.CodeArtifactDomain{
				ObjectMeta: export.ObjectMeta("codeartifact-domain-"+name, opts),
				Spec: awsv1alpha1.CodeArtifactDomainSpec{
					DomainName: name,
				},
			}
			if key := aws.ToString(d.EncryptionKey); key != "" {
				cr.Spec.EncryptionKeyRef = &awsv1alpha1.KMSKeyRef{KeyID: key}
			}
			if tags, err := clients.CodeArtifact.ListTagsForResource(ctx, &awscodeartifact.ListTagsForResourceInput{
				ResourceArn: d.Arn,
			}); err == nil {
				pairs := make([][2]*string, 0, len(tags.Tags))
				for _, t := range tags.Tags {
					pairs = append(pairs, [2]*string{t.Key, t.Value})
				}
				cr.Spec.Tags = kvTagMap(pairs)
			}
			opts.Index.Add(aws.ToString(d.Arn), cr.Name)
			opts.Index.Add(name, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportCodeArtifactRepositories(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awscodeartifact.NewListRepositoriesPaginator(clients.CodeArtifact, &awscodeartifact.ListRepositoriesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list codeartifact repositories: %w", err)
		}
		for _, r := range page.Repositories {
			domainName := aws.ToString(r.DomainName)
			repoName := aws.ToString(r.Name)
			got, err := clients.CodeArtifact.DescribeRepository(ctx, &awscodeartifact.DescribeRepositoryInput{
				Domain:     r.DomainName,
				Repository: r.Name,
			})
			if err != nil || got.Repository == nil {
				continue
			}
			desc := got.Repository
			cr := &awsv1alpha1.CodeArtifactRepository{
				ObjectMeta: export.ObjectMeta(domainName+"-"+repoName, opts),
				Spec: awsv1alpha1.CodeArtifactRepositorySpec{
					RepositoryName: repoName,
					Description:    aws.ToString(desc.Description),
				},
			}
			if crName, ok := opts.Index.Lookup(domainName); ok {
				cr.Spec.DomainRef = awsv1alpha1.CodeArtifactDomainRef{Name: crName}
			} else {
				cr.Spec.DomainRef = awsv1alpha1.CodeArtifactDomainRef{DomainName: domainName}
			}
			for _, u := range desc.Upstreams {
				cr.Spec.Upstreams = append(cr.Spec.Upstreams, aws.ToString(u.RepositoryName))
			}
			for _, ec := range desc.ExternalConnections {
				cr.Spec.ExternalConnections = append(cr.Spec.ExternalConnections, aws.ToString(ec.ExternalConnectionName))
			}
			if tags, err := clients.CodeArtifact.ListTagsForResource(ctx, &awscodeartifact.ListTagsForResourceInput{
				ResourceArn: desc.Arn,
			}); err == nil {
				pairs := make([][2]*string, 0, len(tags.Tags))
				for _, t := range tags.Tags {
					pairs = append(pairs, [2]*string{t.Key, t.Value})
				}
				cr.Spec.Tags = kvTagMap(pairs)
			}
			opts.Index.Add(aws.ToString(desc.Arn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportXRayGroups(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	var next *string
	for {
		page, err := clients.XRay.GetGroups(ctx, &awsxray.GetGroupsInput{NextToken: next})
		if err != nil {
			return nil, fmt.Errorf("get xray groups: %w", err)
		}
		for _, g := range page.Groups {
			name := aws.ToString(g.GroupName)
			// Default is AWS-managed and cannot be created or deleted.
			if name == "Default" {
				continue
			}
			cr := &awsv1alpha1.XRayGroup{
				ObjectMeta: export.ObjectMeta("xray-group-"+name, opts),
				Spec: awsv1alpha1.XRayGroupSpec{
					GroupName:        name,
					FilterExpression: aws.ToString(g.FilterExpression),
				},
			}
			if ic := g.InsightsConfiguration; ic != nil {
				cr.Spec.InsightsConfiguration = &awsv1alpha1.XRayInsightsConfiguration{
					Enabled:              aws.ToBool(ic.InsightsEnabled),
					NotificationsEnabled: aws.ToBool(ic.NotificationsEnabled),
				}
			}
			if tags, err := clients.XRay.ListTagsForResource(ctx, &awsxray.ListTagsForResourceInput{
				ResourceARN: g.GroupARN,
			}); err == nil {
				pairs := make([][2]*string, 0, len(tags.Tags))
				for _, t := range tags.Tags {
					pairs = append(pairs, [2]*string{t.Key, t.Value})
				}
				cr.Spec.Tags = kvTagMap(pairs)
			}
			opts.Index.Add(aws.ToString(g.GroupARN), cr.Name)
			objs = append(objs, cr)
		}
		if page.NextToken == nil {
			return objs, nil
		}
		next = page.NextToken
	}
}

func exportXRaySamplingRules(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	var next *string
	for {
		page, err := clients.XRay.GetSamplingRules(ctx, &awsxray.GetSamplingRulesInput{NextToken: next})
		if err != nil {
			return nil, fmt.Errorf("get xray sampling rules: %w", err)
		}
		for _, rec := range page.SamplingRuleRecords {
			rule := rec.SamplingRule
			if rule == nil {
				continue
			}
			name := aws.ToString(rule.RuleName)
			// Default is AWS-managed and cannot be created or deleted.
			if name == "Default" {
				continue
			}
			cr := &awsv1alpha1.XRaySamplingRule{
				ObjectMeta: export.ObjectMeta("xray-rule-"+name, opts),
				Spec: awsv1alpha1.XRaySamplingRuleSpec{
					RuleName:      name,
					Priority:      aws.ToInt32(rule.Priority),
					FixedRate:     rule.FixedRate,
					ReservoirSize: rule.ReservoirSize,
					ServiceName:   aws.ToString(rule.ServiceName),
					ServiceType:   aws.ToString(rule.ServiceType),
					Host:          aws.ToString(rule.Host),
					HTTPMethod:    aws.ToString(rule.HTTPMethod),
					URLPath:       aws.ToString(rule.URLPath),
					ResourceARN:   aws.ToString(rule.ResourceARN),
				},
			}
			if tags, err := clients.XRay.ListTagsForResource(ctx, &awsxray.ListTagsForResourceInput{
				ResourceARN: rule.RuleARN,
			}); err == nil {
				pairs := make([][2]*string, 0, len(tags.Tags))
				for _, t := range tags.Tags {
					pairs = append(pairs, [2]*string{t.Key, t.Value})
				}
				cr.Spec.Tags = kvTagMap(pairs)
			}
			opts.Index.Add(aws.ToString(rule.RuleARN), cr.Name)
			objs = append(objs, cr)
		}
		if page.NextToken == nil {
			return objs, nil
		}
		next = page.NextToken
	}
}

func exportPrometheusWorkspaces(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsamp.NewListWorkspacesPaginator(clients.AMP, &awsamp.ListWorkspacesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list prometheus workspaces: %w", err)
		}
		for _, ws := range page.Workspaces {
			if ws.Status != nil && string(ws.Status.StatusCode) == "DELETING" {
				continue
			}
			id := aws.ToString(ws.WorkspaceId)
			name := aws.ToString(ws.Alias)
			if name == "" {
				name = id
			}
			cr := &awsv1alpha1.PrometheusWorkspace{
				ObjectMeta: export.ObjectMeta("amp-"+name, opts),
				Spec: awsv1alpha1.PrometheusWorkspaceSpec{
					Alias:     aws.ToString(ws.Alias),
					KMSKeyARN: aws.ToString(ws.KmsKeyArn),
					Tags:      export.TagMap(ws.Tags),
				},
			}
			opts.Index.Add(id, cr.Name)
			opts.Index.Add(aws.ToString(ws.Arn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

// prometheusWorkspaceIDs lists the IDs of non-deleting AMP workspaces.
func prometheusWorkspaceIDs(ctx context.Context, clients *awsclient.Clients) ([]string, error) {
	var ids []string
	p := awsamp.NewListWorkspacesPaginator(clients.AMP, &awsamp.ListWorkspacesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, ws := range page.Workspaces {
			if ws.Status != nil && string(ws.Status.StatusCode) == "DELETING" {
				continue
			}
			ids = append(ids, aws.ToString(ws.WorkspaceId))
		}
	}
	return ids, nil
}

func exportPrometheusRuleGroupsNamespaces(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	wsIDs, err := prometheusWorkspaceIDs(ctx, clients)
	if err != nil {
		return nil, fmt.Errorf("list prometheus workspaces: %w", err)
	}
	var objs []client.Object
	for _, wsID := range wsIDs {
		p := awsamp.NewListRuleGroupsNamespacesPaginator(clients.AMP, &awsamp.ListRuleGroupsNamespacesInput{
			WorkspaceId: aws.String(wsID),
		})
		for p.HasMorePages() {
			page, err := p.NextPage(ctx)
			if err != nil {
				break
			}
			for _, ns := range page.RuleGroupsNamespaces {
				nsName := aws.ToString(ns.Name)
				got, err := clients.AMP.DescribeRuleGroupsNamespace(ctx, &awsamp.DescribeRuleGroupsNamespaceInput{
					WorkspaceId: aws.String(wsID),
					Name:        ns.Name,
				})
				if err != nil || got.RuleGroupsNamespace == nil {
					continue
				}
				cr := &awsv1alpha1.PrometheusRuleGroupsNamespace{
					ObjectMeta: export.ObjectMeta(wsID+"-"+nsName, opts),
					Spec: awsv1alpha1.PrometheusRuleGroupsNamespaceSpec{
						Name: nsName,
						Data: string(got.RuleGroupsNamespace.Data),
						Tags: export.TagMap(ns.Tags),
					},
				}
				if crName, ok := opts.Index.Lookup(wsID); ok {
					cr.Spec.WorkspaceRef = awsv1alpha1.PrometheusWorkspaceRef{Name: crName}
				} else {
					cr.Spec.WorkspaceRef = awsv1alpha1.PrometheusWorkspaceRef{WorkspaceID: wsID}
				}
				opts.Index.Add(aws.ToString(ns.Arn), cr.Name)
				objs = append(objs, cr)
			}
		}
	}
	return objs, nil
}

func exportPrometheusAlertManagerDefinitions(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	wsIDs, err := prometheusWorkspaceIDs(ctx, clients)
	if err != nil {
		return nil, fmt.Errorf("list prometheus workspaces: %w", err)
	}
	var objs []client.Object
	for _, wsID := range wsIDs {
		got, err := clients.AMP.DescribeAlertManagerDefinition(ctx, &awsamp.DescribeAlertManagerDefinitionInput{
			WorkspaceId: aws.String(wsID),
		})
		// Most workspaces have no alert manager definition; skip quietly.
		if err != nil || got.AlertManagerDefinition == nil {
			continue
		}
		cr := &awsv1alpha1.PrometheusAlertManagerDefinition{
			ObjectMeta: export.ObjectMeta(wsID+"-alertmanager", opts),
			Spec: awsv1alpha1.PrometheusAlertManagerDefinitionSpec{
				Definition: string(got.AlertManagerDefinition.Data),
			},
		}
		if crName, ok := opts.Index.Lookup(wsID); ok {
			cr.Spec.WorkspaceRef = awsv1alpha1.PrometheusWorkspaceRef{Name: crName}
		} else {
			cr.Spec.WorkspaceRef = awsv1alpha1.PrometheusWorkspaceRef{WorkspaceID: wsID}
		}
		objs = append(objs, cr)
	}
	return objs, nil
}

func exportGrafanaWorkspaces(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsgrafana.NewListWorkspacesPaginator(clients.Grafana, &awsgrafana.ListWorkspacesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list grafana workspaces: %w", err)
		}
		for _, ws := range page.Workspaces {
			if string(ws.Status) == "DELETING" {
				continue
			}
			id := aws.ToString(ws.Id)
			got, err := clients.Grafana.DescribeWorkspace(ctx, &awsgrafana.DescribeWorkspaceInput{
				WorkspaceId: ws.Id,
			})
			if err != nil || got.Workspace == nil {
				continue
			}
			desc := got.Workspace
			name := aws.ToString(desc.Name)
			if name == "" {
				name = id
			}
			cr := &awsv1alpha1.GrafanaWorkspace{
				ObjectMeta: export.ObjectMeta("grafana-"+name, opts),
				Spec: awsv1alpha1.GrafanaWorkspaceSpec{
					WorkspaceName:     name,
					AccountAccessType: string(desc.AccountAccessType),
					PermissionType:    string(desc.PermissionType),
					GrafanaVersion:    aws.ToString(desc.GrafanaVersion),
					Tags:              export.TagMap(desc.Tags),
				},
			}
			if desc.Authentication != nil {
				for _, prov := range desc.Authentication.Providers {
					cr.Spec.AuthenticationProviders = append(cr.Spec.AuthenticationProviders, string(prov))
				}
			}
			for _, ds := range desc.DataSources {
				cr.Spec.DataSources = append(cr.Spec.DataSources, string(ds))
			}
			if roleArn := aws.ToString(desc.WorkspaceRoleArn); roleArn != "" {
				if crName, ok := opts.Index.Lookup(roleArn); ok {
					cr.Spec.WorkspaceRoleRef = &awsv1alpha1.RoleRef{Name: crName}
				} else {
					cr.Spec.WorkspaceRoleRef = &awsv1alpha1.RoleRef{ARN: roleArn}
				}
			}
			opts.Index.Add(id, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportCostAnomalyMonitors(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	var next *string
	for {
		page, err := clients.CostExplorer.GetAnomalyMonitors(ctx, &awsce.GetAnomalyMonitorsInput{NextPageToken: next})
		if err != nil {
			return nil, fmt.Errorf("get anomaly monitors: %w", err)
		}
		for _, m := range page.AnomalyMonitors {
			name := aws.ToString(m.MonitorName)
			cr := &awsv1alpha1.CostAnomalyMonitor{
				ObjectMeta: export.ObjectMeta("ce-monitor-"+name, opts),
				Spec: awsv1alpha1.CostAnomalyMonitorSpec{
					MonitorName:      name,
					MonitorType:      string(m.MonitorType),
					MonitorDimension: string(m.MonitorDimension),
				},
			}
			opts.Index.Add(aws.ToString(m.MonitorArn), cr.Name)
			objs = append(objs, cr)
		}
		if page.NextPageToken == nil {
			return objs, nil
		}
		next = page.NextPageToken
	}
}

func exportCostAnomalySubscriptions(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	var next *string
	for {
		page, err := clients.CostExplorer.GetAnomalySubscriptions(ctx, &awsce.GetAnomalySubscriptionsInput{NextPageToken: next})
		if err != nil {
			return nil, fmt.Errorf("get anomaly subscriptions: %w", err)
		}
		for _, s := range page.AnomalySubscriptions {
			name := aws.ToString(s.SubscriptionName)
			cr := &awsv1alpha1.CostAnomalySubscription{
				ObjectMeta: export.ObjectMeta("ce-subscription-"+name, opts),
				Spec: awsv1alpha1.CostAnomalySubscriptionSpec{
					SubscriptionName: name,
					Frequency:        string(s.Frequency),
				},
			}
			for _, arn := range s.MonitorArnList {
				if crName, ok := opts.Index.Lookup(arn); ok {
					cr.Spec.MonitorRefs = append(cr.Spec.MonitorRefs, awsv1alpha1.CostAnomalyMonitorRef{Name: crName})
				} else {
					cr.Spec.MonitorRefs = append(cr.Spec.MonitorRefs, awsv1alpha1.CostAnomalyMonitorRef{ARN: arn})
				}
			}
			for _, sub := range s.Subscribers {
				cr.Spec.Subscribers = append(cr.Spec.Subscribers, awsv1alpha1.CostAnomalySubscriber{
					Address: aws.ToString(sub.Address),
					Type:    string(sub.Type),
				})
			}
			switch {
			case s.Threshold != nil:
				cr.Spec.Threshold = strconv.FormatFloat(aws.ToFloat64(s.Threshold), 'f', -1, 64)
			case s.ThresholdExpression != nil && s.ThresholdExpression.Dimensions != nil && len(s.ThresholdExpression.Dimensions.Values) > 0:
				cr.Spec.Threshold = s.ThresholdExpression.Dimensions.Values[0]
			default:
				// Nested (And/Or) threshold expressions can't round-trip into
				// the simple absolute-value threshold field.
				cr.Spec.Threshold = "CHANGEME"
			}
			opts.Index.Add(aws.ToString(s.SubscriptionArn), cr.Name)
			objs = append(objs, cr)
		}
		if page.NextPageToken == nil {
			return objs, nil
		}
		next = page.NextPageToken
	}
}

// accountIDFromARN extracts the account ID field from an ARN
// (arn:partition:service:region:account-id:resource); empty when absent.
func accountIDFromARN(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) >= 6 {
		return parts[4]
	}
	return ""
}

// discoverAccountID derives the AWS account ID from ARNs returned by other
// devopscost-family APIs. The Budgets API requires the caller to supply the
// account ID even for listing, and the export CLI has no STS client.
func discoverAccountID(ctx context.Context, clients *awsclient.Clients) string {
	if out, err := clients.AMP.ListWorkspaces(ctx, &awsamp.ListWorkspacesInput{}); err == nil {
		for _, ws := range out.Workspaces {
			if id := accountIDFromARN(aws.ToString(ws.Arn)); id != "" {
				return id
			}
		}
	}
	if out, err := clients.CostExplorer.GetAnomalyMonitors(ctx, &awsce.GetAnomalyMonitorsInput{}); err == nil {
		for _, m := range out.AnomalyMonitors {
			if id := accountIDFromARN(aws.ToString(m.MonitorArn)); id != "" {
				return id
			}
		}
	}
	if out, err := clients.XRay.GetGroups(ctx, &awsxray.GetGroupsInput{}); err == nil {
		for _, g := range out.Groups {
			if id := accountIDFromARN(aws.ToString(g.GroupARN)); id != "" {
				return id
			}
		}
	}
	return ""
}

func exportBudgets(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	// DescribeBudgets requires the account ID in the request; derive it from
	// ARNs of other resources in the account. If none exist, budgets cannot be
	// exported — surface a WARN instead of failing silently.
	accountID := discoverAccountID(ctx, clients)
	if accountID == "" {
		return nil, fmt.Errorf("cannot determine AWS account ID (no ARN-bearing resources found); skipping budgets export")
	}

	var objs []client.Object
	p := awsbudgets.NewDescribeBudgetsPaginator(clients.Budgets, &awsbudgets.DescribeBudgetsInput{
		AccountId: aws.String(accountID),
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe budgets: %w", err)
		}
		for _, b := range page.Budgets {
			name := aws.ToString(b.BudgetName)
			cr := &awsv1alpha1.Budget{
				ObjectMeta: export.ObjectMeta("budget-"+name, opts),
				Spec: awsv1alpha1.BudgetSpec{
					AccountID:   accountID,
					BudgetName:  name,
					BudgetType:  string(b.BudgetType),
					TimeUnit:    string(b.TimeUnit),
					CostFilters: b.CostFilters,
				},
			}
			if b.BudgetLimit != nil {
				cr.Spec.LimitAmount = aws.ToString(b.BudgetLimit.Amount)
				cr.Spec.LimitUnit = aws.ToString(b.BudgetLimit.Unit)
			} else {
				// RI/SP utilization budgets have no explicit limit; the spec
				// requires one, so flag for the operator to fill in.
				cr.Spec.LimitAmount = "100"
				cr.Spec.LimitUnit = "PERCENTAGE"
			}
			notifs, err := clients.Budgets.DescribeNotificationsForBudget(ctx, &awsbudgets.DescribeNotificationsForBudgetInput{
				AccountId:  aws.String(accountID),
				BudgetName: b.BudgetName,
			})
			if err == nil {
				for _, n := range notifs.Notifications {
					bn := awsv1alpha1.BudgetNotification{
						NotificationType:   string(n.NotificationType),
						ComparisonOperator: string(n.ComparisonOperator),
						Threshold:          strconv.FormatFloat(n.Threshold, 'f', -1, 64),
						ThresholdType:      string(n.ThresholdType),
					}
					subs, err := clients.Budgets.DescribeSubscribersForNotification(ctx, &awsbudgets.DescribeSubscribersForNotificationInput{
						AccountId:    aws.String(accountID),
						BudgetName:   b.BudgetName,
						Notification: &n,
					})
					if err != nil {
						continue
					}
					for _, s := range subs.Subscribers {
						bn.Subscribers = append(bn.Subscribers, awsv1alpha1.BudgetSubscriber{
							Address:          aws.ToString(s.Address),
							SubscriptionType: string(s.SubscriptionType),
						})
					}
					if len(bn.Subscribers) > 0 {
						cr.Spec.Notifications = append(cr.Spec.Notifications, bn)
					}
				}
			}
			opts.Index.Add(name, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}
