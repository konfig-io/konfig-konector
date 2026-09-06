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
	awsaas "github.com/aws/aws-sdk-go-v2/service/applicationautoscaling"
	aastypes "github.com/aws/aws-sdk-go-v2/service/applicationautoscaling/types"
	awsiam "github.com/aws/aws-sdk-go-v2/service/iam"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	awsrds "github.com/aws/aws-sdk-go-v2/service/rds"
	awss3control "github.com/aws/aws-sdk-go-v2/service/s3control"
	awsscheduler "github.com/aws/aws-sdk-go-v2/service/scheduler"
	awsssm "github.com/aws/aws-sdk-go-v2/service/ssm"
	scalingssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	awsclient "github.com/konfig-io/konfig-konector/internal/aws"
	"github.com/konfig-io/konfig-konector/internal/export"
)

func init() {
	export.Register(export.Exporter{Kind: "ScalableTarget", Service: "applicationautoscaling", Order: 90, Fn: exportScalableTargets})
	export.Register(export.Exporter{Kind: "AppScalingPolicy", Service: "applicationautoscaling", Order: 91, Fn: exportAppScalingPolicies})
	export.Register(export.Exporter{Kind: "ScheduleGroup", Service: "scheduler", Order: 90, Fn: exportScheduleGroups})
	export.Register(export.Exporter{Kind: "Schedule", Service: "scheduler", Order: 91, Fn: exportSchedules})
	export.Register(export.Exporter{Kind: "SSMMaintenanceWindow", Service: "ssm", Order: 92, Fn: exportSSMMaintenanceWindows})
	export.Register(export.Exporter{Kind: "SSMPatchBaseline", Service: "ssm", Order: 92, Fn: exportSSMPatchBaselines})
	export.Register(export.Exporter{Kind: "SSMAssociation", Service: "ssm", Order: 93, Fn: exportSSMAssociations})
	export.Register(export.Exporter{Kind: "RDSGlobalCluster", Service: "rds", Order: 92, Fn: exportRDSGlobalClusters})
	export.Register(export.Exporter{Kind: "RDSEventSubscription", Service: "rds", Order: 93, Fn: exportRDSEventSubscriptions})
	export.Register(export.Exporter{Kind: "LambdaProvisionedConcurrency", Service: "lambda", Order: 93, Fn: exportLambdaProvisionedConcurrency})
	export.Register(export.Exporter{Kind: "LambdaEventInvokeConfig", Service: "lambda", Order: 93, Fn: exportLambdaEventInvokeConfigs})
	export.Register(export.Exporter{Kind: "S3AccessPoint", Service: "s3control", Order: 94, Fn: exportS3AccessPoints})
	export.Register(export.Exporter{Kind: "IAMInstanceProfile", Service: "iam", Order: 94, Fn: exportIAMInstanceProfiles})
}

// serviceNamespaces are the Application Auto Scaling namespaces enumerated for
// export; the API requires a namespace per Describe call.
var serviceNamespaces = []aastypes.ServiceNamespace{
	aastypes.ServiceNamespaceEcs,
	aastypes.ServiceNamespaceDynamodb,
	aastypes.ServiceNamespaceRds,
	aastypes.ServiceNamespaceLambda,
	aastypes.ServiceNamespaceEc2,
	aastypes.ServiceNamespaceElasticache,
	aastypes.ServiceNamespaceKafka,
	aastypes.ServiceNamespaceSagemaker,
	aastypes.ServiceNamespaceCassandra,
	aastypes.ServiceNamespaceCustomResource,
	aastypes.ServiceNamespaceAppstream,
	aastypes.ServiceNamespaceComprehend,
	aastypes.ServiceNamespaceNeptune,
	aastypes.ServiceNamespaceWorkspaces,
	aastypes.ServiceNamespaceEmr,
}

// scalableTargetCRName builds a deterministic CR name from the target triple.
func scalableTargetCRName(ns string, resourceID string) string {
	return export.CRName(ns + "-" + resourceID)
}

func exportScalableTargets(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	for _, ns := range serviceNamespaces {
		p := awsaas.NewDescribeScalableTargetsPaginator(clients.AppAutoScaling, &awsaas.DescribeScalableTargetsInput{
			ServiceNamespace: ns,
		})
		for p.HasMorePages() {
			page, err := p.NextPage(ctx)
			if err != nil {
				return nil, fmt.Errorf("describe scalable targets (%s): %w", ns, err)
			}
			for _, t := range page.ScalableTargets {
				resourceID := aws.ToString(t.ResourceId)
				cr := &awsv1alpha1.ScalableTarget{
					ObjectMeta: export.ObjectMeta(scalableTargetCRName(string(ns), resourceID), opts),
					Spec: awsv1alpha1.ScalableTargetSpec{
						ServiceNamespace:  string(t.ServiceNamespace),
						ResourceID:        resourceID,
						ScalableDimension: string(t.ScalableDimension),
						MinCapacity:       aws.ToInt32(t.MinCapacity),
						MaxCapacity:       aws.ToInt32(t.MaxCapacity),
					},
				}
				// Only export explicit (non service-linked) roles.
				if arn := aws.ToString(t.RoleARN); arn != "" && !strings.Contains(arn, "aws-service-role") {
					cr.Spec.RoleARN = arn
				}
				opts.Index.Add(aws.ToString(t.ScalableTargetARN), cr.Name)
				// Index the triple so scaling policies can reference the CR.
				opts.Index.Add(string(t.ServiceNamespace)+"/"+resourceID+"/"+string(t.ScalableDimension), cr.Name)
				objs = append(objs, cr)
			}
		}
	}
	return objs, nil
}

func exportAppScalingPolicies(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	for _, ns := range serviceNamespaces {
		p := awsaas.NewDescribeScalingPoliciesPaginator(clients.AppAutoScaling, &awsaas.DescribeScalingPoliciesInput{
			ServiceNamespace: ns,
		})
		for p.HasMorePages() {
			page, err := p.NextPage(ctx)
			if err != nil {
				return nil, fmt.Errorf("describe scaling policies (%s): %w", ns, err)
			}
			for _, pol := range page.ScalingPolicies {
				// Predictive scaling policies do not round-trip into this CRD.
				if pol.PolicyType != aastypes.PolicyTypeTargetTrackingScaling && pol.PolicyType != aastypes.PolicyTypeStepScaling {
					continue
				}
				name := aws.ToString(pol.PolicyName)
				resourceID := aws.ToString(pol.ResourceId)
				cr := &awsv1alpha1.AppScalingPolicy{
					ObjectMeta: export.ObjectMeta(string(pol.ServiceNamespace)+"-"+name, opts),
					Spec: awsv1alpha1.AppScalingPolicySpec{
						PolicyName: name,
						PolicyType: string(pol.PolicyType),
					},
				}
				triple := string(pol.ServiceNamespace) + "/" + resourceID + "/" + string(pol.ScalableDimension)
				if crName, ok := opts.Index.Lookup(triple); ok {
					cr.Spec.TargetRef = &awsv1alpha1.ScalableTargetRef{Name: crName}
				} else {
					cr.Spec.ServiceNamespace = string(pol.ServiceNamespace)
					cr.Spec.ResourceID = resourceID
					cr.Spec.ScalableDimension = string(pol.ScalableDimension)
				}
				if tt := pol.TargetTrackingScalingPolicyConfiguration; tt != nil {
					cfg := &awsv1alpha1.AppScalingTargetTracking{
						TargetValue: aws.ToFloat64(tt.TargetValue),
					}
					if pm := tt.PredefinedMetricSpecification; pm != nil {
						cfg.PredefinedMetricType = string(pm.PredefinedMetricType)
						cfg.ResourceLabel = aws.ToString(pm.ResourceLabel)
					} else {
						// Customized metrics do not round-trip into this CRD.
						continue
					}
					cfg.ScaleInCooldown = tt.ScaleInCooldown
					cfg.ScaleOutCooldown = tt.ScaleOutCooldown
					cfg.DisableScaleIn = aws.ToBool(tt.DisableScaleIn)
					cr.Spec.TargetTrackingConfiguration = cfg
				}
				if ss := pol.StepScalingPolicyConfiguration; ss != nil {
					cfg := &awsv1alpha1.AppScalingStepScaling{
						AdjustmentType:         string(ss.AdjustmentType),
						Cooldown:               ss.Cooldown,
						MetricAggregationType:  string(ss.MetricAggregationType),
						MinAdjustmentMagnitude: ss.MinAdjustmentMagnitude,
					}
					for _, sa := range ss.StepAdjustments {
						cfg.StepAdjustments = append(cfg.StepAdjustments, awsv1alpha1.AppScalingStepAdjustment{
							MetricIntervalLowerBound: sa.MetricIntervalLowerBound,
							MetricIntervalUpperBound: sa.MetricIntervalUpperBound,
							ScalingAdjustment:        aws.ToInt32(sa.ScalingAdjustment),
						})
					}
					cr.Spec.StepScalingConfiguration = cfg
				}
				opts.Index.Add(aws.ToString(pol.PolicyARN), cr.Name)
				objs = append(objs, cr)
			}
		}
	}
	return objs, nil
}

func exportScheduleGroups(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsscheduler.NewListScheduleGroupsPaginator(clients.Scheduler, &awsscheduler.ListScheduleGroupsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list schedule groups: %w", err)
		}
		for _, g := range page.ScheduleGroups {
			name := aws.ToString(g.Name)
			// The default group is AWS-managed and cannot be created/deleted.
			if name == "default" {
				continue
			}
			cr := &awsv1alpha1.ScheduleGroup{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec:       awsv1alpha1.ScheduleGroupSpec{Name: name},
			}
			tagsOut, err := clients.Scheduler.ListTagsForResource(ctx, &awsscheduler.ListTagsForResourceInput{
				ResourceArn: g.Arn,
			})
			if err == nil {
				tags := map[string]string{}
				for _, t := range tagsOut.Tags {
					tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
				}
				cr.Spec.Tags = export.TagMap(tags)
			}
			opts.Index.Add(aws.ToString(g.Arn), cr.Name)
			opts.Index.Add("schedule-group/"+name, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportSchedules(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsscheduler.NewListSchedulesPaginator(clients.Scheduler, &awsscheduler.ListSchedulesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list schedules: %w", err)
		}
		for _, s := range page.Schedules {
			name := aws.ToString(s.Name)
			groupName := aws.ToString(s.GroupName)
			getIn := &awsscheduler.GetScheduleInput{Name: s.Name}
			if groupName != "" {
				getIn.GroupName = s.GroupName
			}
			detail, err := clients.Scheduler.GetSchedule(ctx, getIn)
			if err != nil {
				continue
			}
			crName := name
			if groupName != "" && groupName != "default" {
				crName = groupName + "-" + name
			}
			cr := &awsv1alpha1.Schedule{
				ObjectMeta: export.ObjectMeta(crName, opts),
				Spec: awsv1alpha1.ScheduleSpec{
					Name:               name,
					ScheduleExpression: aws.ToString(detail.ScheduleExpression),
					Timezone:           aws.ToString(detail.ScheduleExpressionTimezone),
					State:              string(detail.State),
					Description:        aws.ToString(detail.Description),
				},
			}
			if groupName != "" && groupName != "default" {
				if refName, ok := opts.Index.Lookup("schedule-group/" + groupName); ok {
					cr.Spec.GroupRef = &awsv1alpha1.ScheduleGroupRef{Name: refName}
				} else {
					cr.Spec.GroupRef = &awsv1alpha1.ScheduleGroupRef{GroupName: groupName}
				}
			}
			if ftw := detail.FlexibleTimeWindow; ftw != nil {
				cr.Spec.FlexibleTimeWindow = awsv1alpha1.ScheduleFlexibleTimeWindow{
					Mode:                   string(ftw.Mode),
					MaximumWindowInMinutes: ftw.MaximumWindowInMinutes,
				}
			}
			if tgt := detail.Target; tgt != nil {
				st := awsv1alpha1.ScheduleTarget{
					ARN:   aws.ToString(tgt.Arn),
					Input: aws.ToString(tgt.Input),
				}
				roleARN := aws.ToString(tgt.RoleArn)
				if refName, ok := opts.Index.Lookup(roleARN); ok {
					st.RoleRef = awsv1alpha1.RoleRef{Name: refName}
				} else {
					st.RoleRef = awsv1alpha1.RoleRef{ARN: roleARN}
				}
				if rp := tgt.RetryPolicy; rp != nil {
					st.RetryPolicy = &awsv1alpha1.ScheduleRetryPolicy{
						MaximumRetryAttempts:     rp.MaximumRetryAttempts,
						MaximumEventAgeInSeconds: rp.MaximumEventAgeInSeconds,
					}
				}
				if dlc := tgt.DeadLetterConfig; dlc != nil {
					st.DeadLetterARN = aws.ToString(dlc.Arn)
				}
				cr.Spec.Target = st
			}
			opts.Index.Add(aws.ToString(detail.Arn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportSSMMaintenanceWindows(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsssm.NewDescribeMaintenanceWindowsPaginator(clients.SSM, &awsssm.DescribeMaintenanceWindowsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe maintenance windows: %w", err)
		}
		for _, w := range page.WindowIdentities {
			windowID := aws.ToString(w.WindowId)
			detail, err := clients.SSM.GetMaintenanceWindow(ctx, &awsssm.GetMaintenanceWindowInput{
				WindowId: w.WindowId,
			})
			if err != nil {
				continue
			}
			name := aws.ToString(detail.Name)
			cr := &awsv1alpha1.SSMMaintenanceWindow{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.SSMMaintenanceWindowSpec{
					Name:                     name,
					Schedule:                 aws.ToString(detail.Schedule),
					Duration:                 aws.ToInt32(detail.Duration),
					Cutoff:                   detail.Cutoff,
					AllowUnassociatedTargets: detail.AllowUnassociatedTargets,
					Timezone:                 aws.ToString(detail.ScheduleTimezone),
					Description:              aws.ToString(detail.Description),
				},
			}
			opts.Index.Add(windowID, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportSSMPatchBaselines(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsssm.NewDescribePatchBaselinesPaginator(clients.SSM, &awsssm.DescribePatchBaselinesInput{
		Filters: []scalingssmtypes.PatchOrchestratorFilter{{Key: aws.String("OWNER"), Values: []string{"Self"}}},
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe patch baselines: %w", err)
		}
		for _, b := range page.BaselineIdentities {
			baselineID := aws.ToString(b.BaselineId)
			detail, err := clients.SSM.GetPatchBaseline(ctx, &awsssm.GetPatchBaselineInput{
				BaselineId: b.BaselineId,
			})
			if err != nil {
				continue
			}
			name := aws.ToString(detail.Name)
			cr := &awsv1alpha1.SSMPatchBaseline{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.SSMPatchBaselineSpec{
					Name:            name,
					OperatingSystem: string(detail.OperatingSystem),
					ApprovedPatches: detail.ApprovedPatches,
					RejectedPatches: detail.RejectedPatches,
					Description:     aws.ToString(detail.Description),
				},
			}
			if detail.ApprovalRules != nil {
				for _, rule := range detail.ApprovalRules.PatchRules {
					pr := awsv1alpha1.SSMPatchRule{
						ApproveAfterDays: rule.ApproveAfterDays,
						ComplianceLevel:  string(rule.ComplianceLevel),
					}
					if rule.PatchFilterGroup != nil {
						for _, f := range rule.PatchFilterGroup.PatchFilters {
							pr.PatchFilters = append(pr.PatchFilters, awsv1alpha1.SSMPatchFilter{
								Key:    string(f.Key),
								Values: f.Values,
							})
						}
					}
					cr.Spec.ApprovalRules = append(cr.Spec.ApprovalRules, pr)
				}
			}
			opts.Index.Add(baselineID, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportSSMAssociations(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsssm.NewListAssociationsPaginator(clients.SSM, &awsssm.ListAssociationsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list associations: %w", err)
		}
		for _, a := range page.Associations {
			assocID := aws.ToString(a.AssociationId)
			detail, err := clients.SSM.DescribeAssociation(ctx, &awsssm.DescribeAssociationInput{
				AssociationId: a.AssociationId,
			})
			if err != nil || detail.AssociationDescription == nil {
				continue
			}
			d := detail.AssociationDescription
			docName := aws.ToString(d.Name)
			crName := aws.ToString(d.AssociationName)
			if crName == "" {
				crName = docName + "-" + assocID
			}
			cr := &awsv1alpha1.SSMAssociation{
				ObjectMeta: export.ObjectMeta(crName, opts),
				Spec: awsv1alpha1.SSMAssociationSpec{
					AssociationName:    aws.ToString(d.AssociationName),
					ScheduleExpression: aws.ToString(d.ScheduleExpression),
					Parameters:         d.Parameters,
				},
			}
			if refName, ok := opts.Index.Lookup(docName); ok {
				cr.Spec.DocumentRef = &awsv1alpha1.SSMDocumentRef{Name: refName}
			} else {
				cr.Spec.Name = docName
			}
			for _, t := range d.Targets {
				cr.Spec.Targets = append(cr.Spec.Targets, awsv1alpha1.SSMAssociationTarget{
					Key:    aws.ToString(t.Key),
					Values: t.Values,
				})
			}
			opts.Index.Add(assocID, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportRDSGlobalClusters(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsrds.NewDescribeGlobalClustersPaginator(clients.RDS, &awsrds.DescribeGlobalClustersInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe global clusters: %w", err)
		}
		for _, gc := range page.GlobalClusters {
			id := aws.ToString(gc.GlobalClusterIdentifier)
			cr := &awsv1alpha1.RDSGlobalCluster{
				ObjectMeta: export.ObjectMeta(id, opts),
				Spec: awsv1alpha1.RDSGlobalClusterSpec{
					GlobalClusterIdentifier: id,
					Engine:                  aws.ToString(gc.Engine),
					EngineVersion:           aws.ToString(gc.EngineVersion),
					DeletionProtection:      aws.ToBool(gc.DeletionProtection),
					StorageEncrypted:        aws.ToBool(gc.StorageEncrypted),
				},
			}
			opts.Index.Add(aws.ToString(gc.GlobalClusterArn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportRDSEventSubscriptions(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsrds.NewDescribeEventSubscriptionsPaginator(clients.RDS, &awsrds.DescribeEventSubscriptionsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe event subscriptions: %w", err)
		}
		for _, es := range page.EventSubscriptionsList {
			name := aws.ToString(es.CustSubscriptionId)
			topicARN := aws.ToString(es.SnsTopicArn)
			cr := &awsv1alpha1.RDSEventSubscription{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.RDSEventSubscriptionSpec{
					SubscriptionName: name,
					SourceType:       aws.ToString(es.SourceType),
					EventCategories:  es.EventCategoriesList,
					SourceIds:        es.SourceIdsList,
					Enabled:          es.Enabled,
				},
			}
			if refName, ok := opts.Index.Lookup(topicARN); ok {
				cr.Spec.SnsTopicRef = awsv1alpha1.SNSTopicRef{Name: refName}
			} else {
				cr.Spec.SnsTopicRef = awsv1alpha1.SNSTopicRef{ARN: topicARN}
			}
			opts.Index.Add(aws.ToString(es.EventSubscriptionArn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

// listExportedFunctionNames pages through Lambda functions once so the
// per-function config exporters share the same listing logic.
func listExportedFunctionNames(ctx context.Context, clients *awsclient.Clients) ([]string, error) {
	var names []string
	p := awslambda.NewListFunctionsPaginator(clients.Lambda, &awslambda.ListFunctionsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list functions: %w", err)
		}
		for _, f := range page.Functions {
			names = append(names, aws.ToString(f.FunctionName))
		}
	}
	return names, nil
}

func exportLambdaProvisionedConcurrency(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	fnNames, err := listExportedFunctionNames(ctx, clients)
	if err != nil {
		return nil, err
	}
	var objs []client.Object
	for _, fnName := range fnNames {
		p := awslambda.NewListProvisionedConcurrencyConfigsPaginator(clients.Lambda, &awslambda.ListProvisionedConcurrencyConfigsInput{
			FunctionName: aws.String(fnName),
		})
		for p.HasMorePages() {
			page, err := p.NextPage(ctx)
			if err != nil {
				break
			}
			for _, cfg := range page.ProvisionedConcurrencyConfigs {
				fnArn := aws.ToString(cfg.FunctionArn)
				// The qualifier is the last ARN segment (version or alias).
				qualifier := fnArn[strings.LastIndex(fnArn, ":")+1:]
				cr := &awsv1alpha1.LambdaProvisionedConcurrency{
					ObjectMeta: export.ObjectMeta(fnName+"-"+qualifier+"-pc", opts),
					Spec: awsv1alpha1.LambdaProvisionedConcurrencySpec{
						Qualifier:                       qualifier,
						ProvisionedConcurrentExecutions: aws.ToInt32(cfg.RequestedProvisionedConcurrentExecutions),
					},
				}
				if refName, ok := opts.Index.Lookup(fnName); ok {
					cr.Spec.FunctionRef = &awsv1alpha1.LambdaFunctionRef{Name: refName}
				} else {
					cr.Spec.FunctionName = fnName
				}
				objs = append(objs, cr)
			}
		}
	}
	return objs, nil
}

func exportLambdaEventInvokeConfigs(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	fnNames, err := listExportedFunctionNames(ctx, clients)
	if err != nil {
		return nil, err
	}
	var objs []client.Object
	for _, fnName := range fnNames {
		p := awslambda.NewListFunctionEventInvokeConfigsPaginator(clients.Lambda, &awslambda.ListFunctionEventInvokeConfigsInput{
			FunctionName: aws.String(fnName),
		})
		for p.HasMorePages() {
			page, err := p.NextPage(ctx)
			if err != nil {
				break
			}
			for _, cfg := range page.FunctionEventInvokeConfigs {
				fnArn := aws.ToString(cfg.FunctionArn)
				qualifier := fnArn[strings.LastIndex(fnArn, ":")+1:]
				cr := &awsv1alpha1.LambdaEventInvokeConfig{
					ObjectMeta: export.ObjectMeta(fnName+"-"+qualifier+"-eic", opts),
					Spec: awsv1alpha1.LambdaEventInvokeConfigSpec{
						MaximumRetryAttempts:     cfg.MaximumRetryAttempts,
						MaximumEventAgeInSeconds: cfg.MaximumEventAgeInSeconds,
					},
				}
				if qualifier != "" && qualifier != "$LATEST" {
					cr.Spec.Qualifier = qualifier
				}
				if refName, ok := opts.Index.Lookup(fnName); ok {
					cr.Spec.FunctionRef = &awsv1alpha1.LambdaFunctionRef{Name: refName}
				} else {
					cr.Spec.FunctionName = fnName
				}
				if dc := cfg.DestinationConfig; dc != nil {
					if dc.OnSuccess != nil {
						cr.Spec.OnSuccessDestinationARN = aws.ToString(dc.OnSuccess.Destination)
					}
					if dc.OnFailure != nil {
						cr.Spec.OnFailureDestinationARN = aws.ToString(dc.OnFailure.Destination)
					}
				}
				objs = append(objs, cr)
			}
		}
	}
	return objs, nil
}

// exportAccountID derives the account ID from an IAM role ARN; the export
// credential set always sees at least one role (service roles exist in every
// account).
func exportAccountID(ctx context.Context, clients *awsclient.Clients) (string, error) {
	out, err := clients.IAM.ListRoles(ctx, &awsiam.ListRolesInput{MaxItems: aws.Int32(1)})
	if err != nil {
		return "", fmt.Errorf("list roles for account ID: %w", err)
	}
	if len(out.Roles) == 0 {
		return "", fmt.Errorf("no IAM roles visible; cannot derive account ID")
	}
	arn := aws.ToString(out.Roles[0].Arn)
	// arn:aws:iam::123456789012:role/...
	parts := strings.Split(arn, ":")
	if len(parts) < 5 {
		return "", fmt.Errorf("unexpected role ARN %q", arn)
	}
	return parts[4], nil
}

func exportS3AccessPoints(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	accountID, err := exportAccountID(ctx, clients)
	if err != nil {
		return nil, err
	}
	var objs []client.Object
	p := awss3control.NewListAccessPointsPaginator(clients.S3Control, &awss3control.ListAccessPointsInput{
		AccountId: aws.String(accountID),
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list access points: %w", err)
		}
		for _, ap := range page.AccessPointList {
			name := aws.ToString(ap.Name)
			detail, err := clients.S3Control.GetAccessPoint(ctx, &awss3control.GetAccessPointInput{
				AccountId: aws.String(accountID),
				Name:      ap.Name,
			})
			if err != nil {
				continue
			}
			bucketName := aws.ToString(detail.Bucket)
			cr := &awsv1alpha1.S3AccessPoint{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.S3AccessPointSpec{
					Name:      name,
					AccountID: accountID,
				},
			}
			if refName, ok := opts.Index.Lookup("arn:aws:s3:::" + bucketName); ok {
				cr.Spec.BucketRef = awsv1alpha1.S3BucketRef{Name: refName}
			} else {
				cr.Spec.BucketRef = awsv1alpha1.S3BucketRef{BucketName: bucketName}
			}
			if vc := detail.VpcConfiguration; vc != nil {
				vpcID := aws.ToString(vc.VpcId)
				ref := awsv1alpha1.VPCResourceRef{ID: vpcID}
				if refName, ok := opts.Index.Lookup(vpcID); ok {
					ref = awsv1alpha1.VPCResourceRef{Name: refName}
				}
				cr.Spec.VPCConfiguration = &awsv1alpha1.S3AccessPointVPCConfiguration{VPCRef: ref}
			}
			if pab := detail.PublicAccessBlockConfiguration; pab != nil {
				cr.Spec.PublicAccessBlock = &awsv1alpha1.S3AccessPointPublicAccessBlock{
					BlockPublicAcls:       aws.ToBool(pab.BlockPublicAcls),
					BlockPublicPolicy:     aws.ToBool(pab.BlockPublicPolicy),
					IgnorePublicAcls:      aws.ToBool(pab.IgnorePublicAcls),
					RestrictPublicBuckets: aws.ToBool(pab.RestrictPublicBuckets),
				}
			}
			opts.Index.Add(aws.ToString(detail.AccessPointArn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportIAMInstanceProfiles(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsiam.NewListInstanceProfilesPaginator(clients.IAM, &awsiam.ListInstanceProfilesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list instance profiles: %w", err)
		}
		for _, prof := range page.InstanceProfiles {
			name := aws.ToString(prof.InstanceProfileName)
			path := aws.ToString(prof.Path)
			// Skip AWS-managed profiles embedding service-linked roles.
			if strings.HasPrefix(path, "/aws-service-role/") {
				continue
			}
			skip := false
			for _, role := range prof.Roles {
				if strings.HasPrefix(aws.ToString(role.Path), "/aws-service-role/") {
					skip = true
					break
				}
			}
			if skip {
				continue
			}
			cr := &awsv1alpha1.IAMInstanceProfile{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.IAMInstanceProfileSpec{
					InstanceProfileName: name,
				},
			}
			if path != "" && path != "/" {
				cr.Spec.Path = path
			}
			if len(prof.Roles) > 0 {
				roleARN := aws.ToString(prof.Roles[0].Arn)
				if refName, ok := opts.Index.Lookup(roleARN); ok {
					cr.Spec.RoleRef = &awsv1alpha1.RoleRef{Name: refName}
				} else {
					cr.Spec.RoleRef = &awsv1alpha1.RoleRef{ARN: roleARN}
				}
			}
			tags := map[string]string{}
			for _, t := range prof.Tags {
				tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
			}
			cr.Spec.Tags = export.TagMap(tags)
			opts.Index.Add(aws.ToString(prof.Arn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}
