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
	awsbackup "github.com/aws/aws-sdk-go-v2/service/backup"
	awscloudtrail "github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	awsconfigservice "github.com/aws/aws-sdk-go-v2/service/configservice"
	awsguardduty "github.com/aws/aws-sdk-go-v2/service/guardduty"
	awsinspector2 "github.com/aws/aws-sdk-go-v2/service/inspector2"
	inspector2types "github.com/aws/aws-sdk-go-v2/service/inspector2/types"
	awssecurityhub "github.com/aws/aws-sdk-go-v2/service/securityhub"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	awsclient "github.com/konfig-io/konfig-konector/internal/aws"
	securityhubhelper "github.com/konfig-io/konfig-konector/internal/aws/securityhub"
	"github.com/konfig-io/konfig-konector/internal/export"
)

func init() {
	export.Register(export.Exporter{Kind: "Trail", Service: "cloudtrail", Order: 85, Fn: exportTrails})
	export.Register(export.Exporter{Kind: "ConfigRecorder", Service: "config", Order: 86, Fn: exportConfigRecorders})
	export.Register(export.Exporter{Kind: "ConfigDeliveryChannel", Service: "config", Order: 86, Fn: exportConfigDeliveryChannels})
	export.Register(export.Exporter{Kind: "ConfigRule", Service: "config", Order: 87, Fn: exportConfigRules})
	export.Register(export.Exporter{Kind: "BackupVault", Service: "backup", Order: 87, Fn: exportBackupVaults})
	export.Register(export.Exporter{Kind: "BackupPlan", Service: "backup", Order: 88, Fn: exportBackupPlans})
	export.Register(export.Exporter{Kind: "BackupSelection", Service: "backup", Order: 88, Fn: exportBackupSelections})
	export.Register(export.Exporter{Kind: "GuardDutyDetector", Service: "guardduty", Order: 89, Fn: exportGuardDutyDetectors})
	export.Register(export.Exporter{Kind: "SecurityHubAccount", Service: "securityhub", Order: 89, Fn: exportSecurityHubAccount})
	export.Register(export.Exporter{Kind: "SecurityHubStandard", Service: "securityhub", Order: 89, Fn: exportSecurityHubStandards})
	export.Register(export.Exporter{Kind: "InspectorEnabler", Service: "inspector2", Order: 89, Fn: exportInspectorEnabler})
}

func exportTrails(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	region := clients.CloudTrail.Options().Region
	out, err := clients.CloudTrail.DescribeTrails(ctx, &awscloudtrail.DescribeTrailsInput{
		IncludeShadowTrails: aws.Bool(false),
	})
	if err != nil {
		return nil, fmt.Errorf("describe trails: %w", err)
	}
	var objs []client.Object
	for _, t := range out.TrailList {
		// Only export trails whose home region is this region: shadow
		// replications of multi-region trails belong to their home region.
		if aws.ToString(t.HomeRegion) != region {
			continue
		}
		name := aws.ToString(t.Name)
		cr := &awsv1alpha1.Trail{
			ObjectMeta: export.ObjectMeta(name, opts),
			Spec: awsv1alpha1.TrailSpec{
				TrailName:                  name,
				S3BucketName:               aws.ToString(t.S3BucketName),
				S3KeyPrefix:                aws.ToString(t.S3KeyPrefix),
				IncludeGlobalServiceEvents: aws.ToBool(t.IncludeGlobalServiceEvents),
				IsMultiRegionTrail:         aws.ToBool(t.IsMultiRegionTrail),
				EnableLogFileValidation:    aws.ToBool(t.LogFileValidationEnabled),
				CloudWatchLogsLogGroupArn:  aws.ToString(t.CloudWatchLogsLogGroupArn),
				CloudWatchLogsRoleArn:      aws.ToString(t.CloudWatchLogsRoleArn),
				KMSKeyID:                   aws.ToString(t.KmsKeyId),
			},
		}

		if statusOut, err := clients.CloudTrail.GetTrailStatus(ctx, &awscloudtrail.GetTrailStatusInput{
			Name: t.TrailARN,
		}); err == nil {
			cr.Spec.EnableLogging = statusOut.IsLogging
		}

		if tagsOut, err := clients.CloudTrail.ListTags(ctx, &awscloudtrail.ListTagsInput{
			ResourceIdList: []string{aws.ToString(t.TrailARN)},
		}); err == nil && len(tagsOut.ResourceTagList) > 0 {
			tags := map[string]string{}
			for _, tag := range tagsOut.ResourceTagList[0].TagsList {
				tags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
			}
			cr.Spec.Tags = export.TagMap(tags)
		}

		opts.Index.Add(aws.ToString(t.TrailARN), cr.Name)
		objs = append(objs, cr)
	}
	return objs, nil
}

func exportConfigRecorders(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	out, err := clients.ConfigService.DescribeConfigurationRecorders(ctx, &awsconfigservice.DescribeConfigurationRecordersInput{})
	if err != nil {
		return nil, fmt.Errorf("describe configuration recorders: %w", err)
	}

	recording := map[string]bool{}
	if statusOut, err := clients.ConfigService.DescribeConfigurationRecorderStatus(ctx, &awsconfigservice.DescribeConfigurationRecorderStatusInput{}); err == nil {
		for _, s := range statusOut.ConfigurationRecordersStatus {
			recording[aws.ToString(s.Name)] = s.Recording
		}
	}

	var objs []client.Object
	for _, rec := range out.ConfigurationRecorders {
		name := aws.ToString(rec.Name)
		enabled := recording[name]
		cr := &awsv1alpha1.ConfigRecorder{
			ObjectMeta: export.ObjectMeta("config-recorder-"+name, opts),
			Spec: awsv1alpha1.ConfigRecorderSpec{
				RecorderName: name,
				RoleARN:      aws.ToString(rec.RoleARN),
				Enabled:      &enabled,
			},
		}
		if roleName, ok := opts.Index.Lookup(aws.ToString(rec.RoleARN)); ok {
			cr.Spec.RoleARN = ""
			cr.Spec.RoleRef = &awsv1alpha1.RoleRef{Name: roleName}
		}
		if rg := rec.RecordingGroup; rg != nil {
			group := &awsv1alpha1.ConfigRecordingGroup{
				AllSupported:               rg.AllSupported,
				IncludeGlobalResourceTypes: rg.IncludeGlobalResourceTypes,
			}
			for _, rt := range rg.ResourceTypes {
				group.ResourceTypes = append(group.ResourceTypes, string(rt))
			}
			cr.Spec.RecordingGroup = group
		}
		opts.Index.Add(name, cr.Name)
		objs = append(objs, cr)
	}
	return objs, nil
}

func exportConfigDeliveryChannels(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	out, err := clients.ConfigService.DescribeDeliveryChannels(ctx, &awsconfigservice.DescribeDeliveryChannelsInput{})
	if err != nil {
		return nil, fmt.Errorf("describe delivery channels: %w", err)
	}
	var objs []client.Object
	for _, ch := range out.DeliveryChannels {
		name := aws.ToString(ch.Name)
		cr := &awsv1alpha1.ConfigDeliveryChannel{
			ObjectMeta: export.ObjectMeta("config-channel-"+name, opts),
			Spec: awsv1alpha1.ConfigDeliveryChannelSpec{
				ChannelName:  name,
				S3BucketName: aws.ToString(ch.S3BucketName),
				S3KeyPrefix:  aws.ToString(ch.S3KeyPrefix),
				SNSTopicARN:  aws.ToString(ch.SnsTopicARN),
			},
		}
		if props := ch.ConfigSnapshotDeliveryProperties; props != nil {
			cr.Spec.DeliveryFrequency = string(props.DeliveryFrequency)
		}
		objs = append(objs, cr)
	}
	return objs, nil
}

func exportConfigRules(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsconfigservice.NewDescribeConfigRulesPaginator(clients.ConfigService, &awsconfigservice.DescribeConfigRulesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe config rules: %w", err)
		}
		for _, rule := range page.ConfigRules {
			// Skip rules created by AWS services (conformance packs, Security
			// Hub, Control Tower): they are managed by their creator, not us.
			if aws.ToString(rule.CreatedBy) != "" {
				continue
			}
			if rule.Source == nil {
				continue
			}
			name := aws.ToString(rule.ConfigRuleName)
			cr := &awsv1alpha1.ConfigRule{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.ConfigRuleSpec{
					RuleName:                  name,
					Description:               aws.ToString(rule.Description),
					SourceOwner:               string(rule.Source.Owner),
					SourceIdentifier:          aws.ToString(rule.Source.SourceIdentifier),
					InputParameters:           aws.ToString(rule.InputParameters),
					MaximumExecutionFrequency: string(rule.MaximumExecutionFrequency),
				},
			}
			if sc := rule.Scope; sc != nil {
				cr.Spec.Scope = &awsv1alpha1.ConfigRuleScope{
					ComplianceResourceTypes: sc.ComplianceResourceTypes,
					TagKey:                  aws.ToString(sc.TagKey),
					TagValue:                aws.ToString(sc.TagValue),
				}
			}
			opts.Index.Add(aws.ToString(rule.ConfigRuleArn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportBackupVaults(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsbackup.NewListBackupVaultsPaginator(clients.Backup, &awsbackup.ListBackupVaultsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list backup vaults: %w", err)
		}
		for _, v := range page.BackupVaultList {
			name := aws.ToString(v.BackupVaultName)
			// Skip the account's Default vault and service-managed vaults
			// (aws/efs/automatic-backup-vault and similar aws/* names).
			if name == "Default" || strings.HasPrefix(name, "aws/") {
				continue
			}
			cr := &awsv1alpha1.BackupVault{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.BackupVaultSpec{
					VaultName: name,
					KMSKeyARN: aws.ToString(v.EncryptionKeyArn),
				},
			}
			if tagsOut, err := clients.Backup.ListTags(ctx, &awsbackup.ListTagsInput{
				ResourceArn: v.BackupVaultArn,
			}); err == nil {
				cr.Spec.Tags = export.TagMap(tagsOut.Tags)
			}
			opts.Index.Add(aws.ToString(v.BackupVaultArn), cr.Name)
			opts.Index.Add(name, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportBackupPlans(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsbackup.NewListBackupPlansPaginator(clients.Backup, &awsbackup.ListBackupPlansInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list backup plans: %w", err)
		}
		for _, item := range page.BackupPlansList {
			getOut, err := clients.Backup.GetBackupPlan(ctx, &awsbackup.GetBackupPlanInput{
				BackupPlanId: item.BackupPlanId,
			})
			if err != nil || getOut.BackupPlan == nil {
				continue
			}
			plan := getOut.BackupPlan
			name := aws.ToString(plan.BackupPlanName)
			cr := &awsv1alpha1.BackupPlan{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.BackupPlanSpec{
					PlanName: name,
				},
			}
			for _, rule := range plan.Rules {
				specRule := awsv1alpha1.BackupPlanRule{
					RuleName:                aws.ToString(rule.RuleName),
					ScheduleExpression:      aws.ToString(rule.ScheduleExpression),
					StartWindowMinutes:      rule.StartWindowMinutes,
					CompletionWindowMinutes: rule.CompletionWindowMinutes,
				}
				vaultName := aws.ToString(rule.TargetBackupVaultName)
				if crName, ok := opts.Index.Lookup(vaultName); ok {
					specRule.TargetBackupVaultRef = crName
				} else {
					specRule.TargetBackupVaultName = vaultName
				}
				if lc := rule.Lifecycle; lc != nil {
					specRule.Lifecycle = &awsv1alpha1.BackupLifecycle{
						DeleteAfterDays:            lc.DeleteAfterDays,
						MoveToColdStorageAfterDays: lc.MoveToColdStorageAfterDays,
					}
				}
				cr.Spec.Rules = append(cr.Spec.Rules, specRule)
			}
			if tagsOut, err := clients.Backup.ListTags(ctx, &awsbackup.ListTagsInput{
				ResourceArn: getOut.BackupPlanArn,
			}); err == nil {
				cr.Spec.Tags = export.TagMap(tagsOut.Tags)
			}
			opts.Index.Add(aws.ToString(item.BackupPlanId), cr.Name)
			opts.Index.Add(aws.ToString(getOut.BackupPlanArn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportBackupSelections(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	planPager := awsbackup.NewListBackupPlansPaginator(clients.Backup, &awsbackup.ListBackupPlansInput{})
	for planPager.HasMorePages() {
		planPage, err := planPager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list backup plans: %w", err)
		}
		for _, plan := range planPage.BackupPlansList {
			planID := aws.ToString(plan.BackupPlanId)
			planRef, _ := opts.Index.Lookup(planID)
			if planRef == "" {
				planRef = export.CRName(aws.ToString(plan.BackupPlanName))
			}
			selPager := awsbackup.NewListBackupSelectionsPaginator(clients.Backup, &awsbackup.ListBackupSelectionsInput{
				BackupPlanId: plan.BackupPlanId,
			})
			for selPager.HasMorePages() {
				selPage, err := selPager.NextPage(ctx)
				if err != nil {
					return nil, fmt.Errorf("list backup selections for plan %s: %w", planID, err)
				}
				for _, item := range selPage.BackupSelectionsList {
					getOut, err := clients.Backup.GetBackupSelection(ctx, &awsbackup.GetBackupSelectionInput{
						BackupPlanId: plan.BackupPlanId,
						SelectionId:  item.SelectionId,
					})
					if err != nil || getOut.BackupSelection == nil {
						continue
					}
					sel := getOut.BackupSelection
					selName := aws.ToString(sel.SelectionName)
					cr := &awsv1alpha1.BackupSelection{
						ObjectMeta: export.ObjectMeta(planRef+"-"+selName, opts),
						Spec: awsv1alpha1.BackupSelectionSpec{
							PlanRef:       planRef,
							SelectionName: selName,
							IAMRoleARN:    aws.ToString(sel.IamRoleArn),
							Resources:     sel.Resources,
						},
					}
					if roleName, ok := opts.Index.Lookup(aws.ToString(sel.IamRoleArn)); ok {
						cr.Spec.IAMRoleARN = ""
						cr.Spec.IAMRoleRef = &awsv1alpha1.RoleRef{Name: roleName}
					}
					for _, cond := range sel.ListOfTags {
						cr.Spec.ListOfTags = append(cr.Spec.ListOfTags, awsv1alpha1.BackupSelectionTag{
							Key:   aws.ToString(cond.ConditionKey),
							Value: aws.ToString(cond.ConditionValue),
						})
					}
					objs = append(objs, cr)
				}
			}
		}
	}
	return objs, nil
}

func exportGuardDutyDetectors(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsguardduty.NewListDetectorsPaginator(clients.GuardDuty, &awsguardduty.ListDetectorsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list detectors: %w", err)
		}
		for _, id := range page.DetectorIds {
			getOut, err := clients.GuardDuty.GetDetector(ctx, &awsguardduty.GetDetectorInput{
				DetectorId: aws.String(id),
			})
			if err != nil {
				continue
			}
			enabled := getOut.Status == "ENABLED"
			cr := &awsv1alpha1.GuardDutyDetector{
				ObjectMeta: export.ObjectMeta("guardduty-detector", opts),
				Spec: awsv1alpha1.GuardDutyDetectorSpec{
					Enable:                     &enabled,
					FindingPublishingFrequency: string(getOut.FindingPublishingFrequency),
					Tags:                       export.TagMap(getOut.Tags),
				},
			}
			for _, f := range getOut.Features {
				cr.Spec.Features = append(cr.Spec.Features, awsv1alpha1.GuardDutyFeature{
					Name:   string(f.Name),
					Status: string(f.Status),
				})
			}
			opts.Index.Add(id, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportSecurityHubAccount(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	descOut, err := clients.SecurityHub.DescribeHub(ctx, &awssecurityhub.DescribeHubInput{})
	if err != nil {
		if securityhubhelper.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("describe hub: %w", err)
	}
	cr := &awsv1alpha1.SecurityHubAccount{
		ObjectMeta: export.ObjectMeta("securityhub-account", opts),
		Spec: awsv1alpha1.SecurityHubAccountSpec{
			ControlFindingGenerator: string(descOut.ControlFindingGenerator),
		},
	}
	opts.Index.Add(aws.ToString(descOut.HubArn), cr.Name)
	return []client.Object{cr}, nil
}

func exportSecurityHubStandards(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awssecurityhub.NewGetEnabledStandardsPaginator(clients.SecurityHub, &awssecurityhub.GetEnabledStandardsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			if securityhubhelper.IsNotFound(err) {
				return objs, nil
			}
			return nil, fmt.Errorf("get enabled standards: %w", err)
		}
		for _, sub := range page.StandardsSubscriptions {
			standardsARN := aws.ToString(sub.StandardsArn)
			// Derive a stable CR name from the standard's path, e.g.
			// ".../standards/aws-foundational-security-best-practices/v/1.0.0".
			name := standardsARN
			if i := strings.Index(standardsARN, "standards/"); i >= 0 {
				name = strings.TrimPrefix(standardsARN[i:], "standards/")
			}
			cr := &awsv1alpha1.SecurityHubStandard{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.SecurityHubStandardSpec{
					StandardsARN: standardsARN,
				},
			}
			opts.Index.Add(aws.ToString(sub.StandardsSubscriptionArn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportInspectorEnabler(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	out, err := clients.Inspector2.BatchGetAccountStatus(ctx, &awsinspector2.BatchGetAccountStatusInput{})
	if err != nil {
		return nil, fmt.Errorf("get inspector account status: %w", err)
	}
	if len(out.Accounts) == 0 {
		return nil, nil
	}
	account := out.Accounts[0]
	rs := account.ResourceState
	if rs == nil {
		return nil, nil
	}

	var enabled []awsv1alpha1.InspectorResourceType
	appendIfEnabled := func(st *inspector2types.State, name awsv1alpha1.InspectorResourceType) {
		if st != nil && st.Status == inspector2types.StatusEnabled {
			enabled = append(enabled, name)
		}
	}
	appendIfEnabled(rs.Ec2, "EC2")
	appendIfEnabled(rs.Ecr, "ECR")
	appendIfEnabled(rs.Lambda, "LAMBDA")
	appendIfEnabled(rs.LambdaCode, "LAMBDA_CODE")
	if len(enabled) == 0 {
		// Inspector is not enabled for any resource type: nothing to manage.
		return nil, nil
	}

	cr := &awsv1alpha1.InspectorEnabler{
		ObjectMeta: export.ObjectMeta("inspector-enabler", opts),
		Spec: awsv1alpha1.InspectorEnablerSpec{
			ResourceTypes: enabled,
		},
	}
	return []client.Object{cr}, nil
}
