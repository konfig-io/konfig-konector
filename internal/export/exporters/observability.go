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
	awscw "github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	awslogs "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	awsclient "github.com/konfig-io/konfig-konector/internal/aws"
	"github.com/konfig-io/konfig-konector/internal/export"
)

func init() {
	// Log groups first: metric/subscription filters reference them by name.
	export.Register(export.Exporter{Kind: "LogGroup", Service: "logs", Order: 60, Fn: exportLogGroups})
	export.Register(export.Exporter{Kind: "MetricFilter", Service: "logs", Order: 61, Fn: exportMetricFilters})
	export.Register(export.Exporter{Kind: "SubscriptionFilter", Service: "logs", Order: 62, Fn: exportSubscriptionFilters})
	export.Register(export.Exporter{Kind: "CloudWatchAlarm", Service: "cloudwatch", Order: 63, Fn: exportCloudWatchAlarms})
	export.Register(export.Exporter{Kind: "CompositeAlarm", Service: "cloudwatch", Order: 63, Fn: exportCompositeAlarms})
	export.Register(export.Exporter{Kind: "CloudWatchDashboard", Service: "cloudwatch", Order: 64, Fn: exportCloudWatchDashboards})
}

// cwTagMap converts CloudWatch tag slices to a spec tag map.
func cwTagMap(tags []cwtypes.Tag) map[string]string {
	if len(tags) == 0 {
		return nil
	}
	m := make(map[string]string, len(tags))
	for _, t := range tags {
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return export.TagMap(m)
}

// logGroupRefFor resolves a log group name into a CR ref when it was exported
// in this run, otherwise falls back to the raw AWS name.
func logGroupRefFor(logGroupName string, opts *export.Options) awsv1alpha1.LogGroupRef {
	if crName, ok := opts.Index.Lookup(logGroupName); ok {
		return awsv1alpha1.LogGroupRef{Name: crName}
	}
	return awsv1alpha1.LogGroupRef{LogGroupName: logGroupName}
}

func exportLogGroups(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awslogs.NewDescribeLogGroupsPaginator(clients.CloudWatchLogs, &awslogs.DescribeLogGroupsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe log groups: %w", err)
		}
		for _, lg := range page.LogGroups {
			name := aws.ToString(lg.LogGroupName)
			cr := &awsv1alpha1.LogGroup{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.LogGroupSpec{
					LogGroupName:    name,
					RetentionInDays: aws.ToInt32(lg.RetentionInDays),
					KMSKeyARN:       aws.ToString(lg.KmsKeyId),
				},
			}
			// The DescribeLogGroups Arn carries a trailing ":*" that the tag
			// API rejects; LogGroupArn is the clean form when present.
			arn := aws.ToString(lg.LogGroupArn)
			if arn == "" {
				arn = strings.TrimSuffix(aws.ToString(lg.Arn), ":*")
			}
			if arn != "" {
				tagsOut, err := clients.CloudWatchLogs.ListTagsForResource(ctx, &awslogs.ListTagsForResourceInput{
					ResourceArn: aws.String(arn),
				})
				if err == nil {
					cr.Spec.Tags = export.TagMap(tagsOut.Tags)
				}
			}
			opts.Index.Add(name, cr.Name)
			opts.Index.Add(arn, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportMetricFilters(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awslogs.NewDescribeMetricFiltersPaginator(clients.CloudWatchLogs, &awslogs.DescribeMetricFiltersInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe metric filters: %w", err)
		}
		for _, mf := range page.MetricFilters {
			lgName := aws.ToString(mf.LogGroupName)
			filterName := aws.ToString(mf.FilterName)
			cr := &awsv1alpha1.MetricFilter{
				ObjectMeta: export.ObjectMeta(lgName+"-"+filterName, opts),
				Spec: awsv1alpha1.MetricFilterSpec{
					LogGroupRef:   logGroupRefFor(lgName, opts),
					FilterName:    filterName,
					FilterPattern: aws.ToString(mf.FilterPattern),
				},
			}
			for _, mt := range mf.MetricTransformations {
				cr.Spec.MetricTransformations = append(cr.Spec.MetricTransformations, awsv1alpha1.MetricTransformation{
					MetricName:      aws.ToString(mt.MetricName),
					MetricNamespace: aws.ToString(mt.MetricNamespace),
					MetricValue:     aws.ToString(mt.MetricValue),
					DefaultValue:    mt.DefaultValue,
				})
			}
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportSubscriptionFilters(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	// DescribeSubscriptionFilters requires a log group name, so walk them all.
	lgp := awslogs.NewDescribeLogGroupsPaginator(clients.CloudWatchLogs, &awslogs.DescribeLogGroupsInput{})
	for lgp.HasMorePages() {
		page, err := lgp.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe log groups: %w", err)
		}
		for _, lg := range page.LogGroups {
			lgName := aws.ToString(lg.LogGroupName)
			sfp := awslogs.NewDescribeSubscriptionFiltersPaginator(clients.CloudWatchLogs, &awslogs.DescribeSubscriptionFiltersInput{
				LogGroupName: lg.LogGroupName,
			})
			for sfp.HasMorePages() {
				sfPage, err := sfp.NextPage(ctx)
				if err != nil {
					// Per-log-group failure must not sink the whole export.
					break
				}
				for _, sf := range sfPage.SubscriptionFilters {
					filterName := aws.ToString(sf.FilterName)
					cr := &awsv1alpha1.SubscriptionFilter{
						ObjectMeta: export.ObjectMeta(lgName+"-"+filterName, opts),
						Spec: awsv1alpha1.SubscriptionFilterSpec{
							LogGroupRef:    logGroupRefFor(lgName, opts),
							FilterName:     filterName,
							FilterPattern:  aws.ToString(sf.FilterPattern),
							DestinationARN: aws.ToString(sf.DestinationArn),
							RoleARN:        aws.ToString(sf.RoleArn),
						},
					}
					objs = append(objs, cr)
				}
			}
		}
	}
	return objs, nil
}

func exportCloudWatchAlarms(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awscw.NewDescribeAlarmsPaginator(clients.CloudWatch, &awscw.DescribeAlarmsInput{
		AlarmTypes: []cwtypes.AlarmType{cwtypes.AlarmTypeMetricAlarm},
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe alarms: %w", err)
		}
		for _, a := range page.MetricAlarms {
			name := aws.ToString(a.AlarmName)
			cr := &awsv1alpha1.CloudWatchAlarm{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.CloudWatchAlarmSpec{
					AlarmName:               name,
					ComparisonOperator:      string(a.ComparisonOperator),
					EvaluationPeriods:       aws.ToInt32(a.EvaluationPeriods),
					MetricName:              aws.ToString(a.MetricName),
					Namespace:               aws.ToString(a.Namespace),
					Statistic:               string(a.Statistic),
					Period:                  a.Period,
					Threshold:               a.Threshold,
					AlarmDescription:        aws.ToString(a.AlarmDescription),
					ActionsEnabled:          a.ActionsEnabled,
					AlarmActions:            a.AlarmActions,
					OKActions:               a.OKActions,
					InsufficientDataActions: a.InsufficientDataActions,
					DatapointsToAlarm:       a.DatapointsToAlarm,
					TreatMissingData:        aws.ToString(a.TreatMissingData),
					Unit:                    string(a.Unit),
				},
			}
			for _, d := range a.Dimensions {
				cr.Spec.Dimensions = append(cr.Spec.Dimensions, awsv1alpha1.CloudWatchDimension{
					Name:  aws.ToString(d.Name),
					Value: aws.ToString(d.Value),
				})
			}
			tagsOut, err := clients.CloudWatch.ListTagsForResource(ctx, &awscw.ListTagsForResourceInput{
				ResourceARN: a.AlarmArn,
			})
			if err == nil {
				cr.Spec.Tags = cwTagMap(tagsOut.Tags)
			}
			opts.Index.Add(aws.ToString(a.AlarmArn), cr.Name)
			opts.Index.Add(name, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportCompositeAlarms(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awscw.NewDescribeAlarmsPaginator(clients.CloudWatch, &awscw.DescribeAlarmsInput{
		AlarmTypes: []cwtypes.AlarmType{cwtypes.AlarmTypeCompositeAlarm},
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe composite alarms: %w", err)
		}
		for _, a := range page.CompositeAlarms {
			name := aws.ToString(a.AlarmName)
			cr := &awsv1alpha1.CompositeAlarm{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.CompositeAlarmSpec{
					AlarmName:               name,
					AlarmRule:               aws.ToString(a.AlarmRule),
					AlarmDescription:        aws.ToString(a.AlarmDescription),
					ActionsEnabled:          a.ActionsEnabled,
					AlarmActions:            a.AlarmActions,
					OKActions:               a.OKActions,
					InsufficientDataActions: a.InsufficientDataActions,
				},
			}
			tagsOut, err := clients.CloudWatch.ListTagsForResource(ctx, &awscw.ListTagsForResourceInput{
				ResourceARN: a.AlarmArn,
			})
			if err == nil {
				cr.Spec.Tags = cwTagMap(tagsOut.Tags)
			}
			opts.Index.Add(aws.ToString(a.AlarmArn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportCloudWatchDashboards(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awscw.NewListDashboardsPaginator(clients.CloudWatch, &awscw.ListDashboardsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list dashboards: %w", err)
		}
		for _, d := range page.DashboardEntries {
			name := aws.ToString(d.DashboardName)
			body, err := clients.CloudWatch.GetDashboard(ctx, &awscw.GetDashboardInput{
				DashboardName: d.DashboardName,
			})
			if err != nil {
				continue
			}
			cr := &awsv1alpha1.CloudWatchDashboard{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.CloudWatchDashboardSpec{
					DashboardName: name,
					DashboardBody: aws.ToString(body.DashboardBody),
				},
			}
			opts.Index.Add(aws.ToString(d.DashboardArn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}
