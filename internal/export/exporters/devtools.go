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
	awscfn "github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	awscodebuild "github.com/aws/aws-sdk-go-v2/service/codebuild"
	awscodecommit "github.com/aws/aws-sdk-go-v2/service/codecommit"
	awscodedeploy "github.com/aws/aws-sdk-go-v2/service/codedeploy"
	awscodepipeline "github.com/aws/aws-sdk-go-v2/service/codepipeline"
	awsopensearch "github.com/aws/aws-sdk-go-v2/service/opensearch"
	awsoss "github.com/aws/aws-sdk-go-v2/service/opensearchserverless"
	osstypes "github.com/aws/aws-sdk-go-v2/service/opensearchserverless/types"
	awssecrets "github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	awssfn "github.com/aws/aws-sdk-go-v2/service/sfn"
	awsssm "github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	awsclient "github.com/konfig-io/konfig-konector/internal/aws"
	"github.com/konfig-io/konfig-konector/internal/export"
)

func init() {
	export.Register(export.Exporter{Kind: "CloudFormationStack", Service: "cloudformation", Order: 70, Fn: exportCloudFormationStacks})
	export.Register(export.Exporter{Kind: "CloudFormationStackSet", Service: "cloudformation", Order: 70, Fn: exportCloudFormationStackSets})
	export.Register(export.Exporter{Kind: "StateMachine", Service: "sfn", Order: 71, Fn: exportStateMachines})
	export.Register(export.Exporter{Kind: "Activity", Service: "sfn", Order: 71, Fn: exportActivities})
	export.Register(export.Exporter{Kind: "CodeBuildProject", Service: "codebuild", Order: 72, Fn: exportCodeBuildProjects})
	export.Register(export.Exporter{Kind: "CodePipeline", Service: "codepipeline", Order: 73, Fn: exportCodePipelines})
	// Applications before deployment groups: groups are listed per application.
	export.Register(export.Exporter{Kind: "CodeDeployApplication", Service: "codedeploy", Order: 74, Fn: exportCodeDeployApplications})
	export.Register(export.Exporter{Kind: "CodeDeployDeploymentGroup", Service: "codedeploy", Order: 75, Fn: exportCodeDeployDeploymentGroups})
	export.Register(export.Exporter{Kind: "CodeCommitRepository", Service: "codecommit", Order: 74, Fn: exportCodeCommitRepositories})
	export.Register(export.Exporter{Kind: "OpenSearchDomain", Service: "opensearch", Order: 76, Fn: exportOpenSearchDomains})
	export.Register(export.Exporter{Kind: "OpenSearchServerlessCollection", Service: "opensearchserverless", Order: 76, Fn: exportOpenSearchServerlessCollections})
	export.Register(export.Exporter{Kind: "OpenSearchAccessPolicy", Service: "opensearchserverless", Order: 77, Fn: exportOpenSearchAccessPolicies})
	export.Register(export.Exporter{Kind: "SSMDocument", Service: "ssm", Order: 78, Fn: exportSSMDocuments})
	export.Register(export.Exporter{Kind: "SecretRotation", Service: "secretsmanager", Order: 79, Fn: exportSecretRotations})
}

// kvTagMap converts generic Key/Value pointer tag pairs into a spec tag map.
func kvTagMap(pairs [][2]*string) map[string]string {
	if len(pairs) == 0 {
		return nil
	}
	m := make(map[string]string, len(pairs))
	for _, p := range pairs {
		m[aws.ToString(p[0])] = aws.ToString(p[1])
	}
	return export.TagMap(m)
}

func exportCloudFormationStacks(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awscfn.NewDescribeStacksPaginator(clients.CloudFormation, &awscfn.DescribeStacksInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe stacks: %w", err)
		}
		for _, s := range page.Stacks {
			// Only healthy, steady-state stacks; nested stacks belong to their
			// parent's template, not to a standalone CR.
			if s.StackStatus != cfntypes.StackStatusCreateComplete && s.StackStatus != cfntypes.StackStatusUpdateComplete {
				continue
			}
			if s.ParentId != nil {
				continue
			}
			name := aws.ToString(s.StackName)
			cr := &awsv1alpha1.CloudFormationStack{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.CloudFormationStackSpec{
					StackName: name,
				},
			}
			for _, c := range s.Capabilities {
				cr.Spec.Capabilities = append(cr.Spec.Capabilities, string(c))
			}
			for _, prm := range s.Parameters {
				cr.Spec.Parameters = append(cr.Spec.Parameters, awsv1alpha1.CloudFormationParameter{
					ParameterKey:   aws.ToString(prm.ParameterKey),
					ParameterValue: aws.ToString(prm.ParameterValue),
				})
			}
			pairs := make([][2]*string, 0, len(s.Tags))
			for _, t := range s.Tags {
				pairs = append(pairs, [2]*string{t.Key, t.Value})
			}
			cr.Spec.Tags = kvTagMap(pairs)
			tpl, err := clients.CloudFormation.GetTemplate(ctx, &awscfn.GetTemplateInput{
				StackName:     s.StackName,
				TemplateStage: cfntypes.TemplateStageOriginal,
			})
			if err != nil {
				continue
			}
			cr.Spec.TemplateBody = aws.ToString(tpl.TemplateBody)
			opts.Index.Add(aws.ToString(s.StackId), cr.Name)
			opts.Index.Add(name, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportCloudFormationStackSets(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awscfn.NewListStackSetsPaginator(clients.CloudFormation, &awscfn.ListStackSetsInput{
		Status: cfntypes.StackSetStatusActive,
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list stack sets: %w", err)
		}
		for _, s := range page.Summaries {
			name := aws.ToString(s.StackSetName)
			got, err := clients.CloudFormation.DescribeStackSet(ctx, &awscfn.DescribeStackSetInput{
				StackSetName: s.StackSetName,
			})
			if err != nil || got.StackSet == nil {
				continue
			}
			ss := got.StackSet
			cr := &awsv1alpha1.CloudFormationStackSet{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.CloudFormationStackSetSpec{
					StackSetName: name,
					TemplateBody: aws.ToString(ss.TemplateBody),
					Description:  aws.ToString(ss.Description),
				},
			}
			for _, c := range ss.Capabilities {
				cr.Spec.Capabilities = append(cr.Spec.Capabilities, string(c))
			}
			for _, prm := range ss.Parameters {
				cr.Spec.Parameters = append(cr.Spec.Parameters, awsv1alpha1.CloudFormationParameter{
					ParameterKey:   aws.ToString(prm.ParameterKey),
					ParameterValue: aws.ToString(prm.ParameterValue),
				})
			}
			pairs := make([][2]*string, 0, len(ss.Tags))
			for _, t := range ss.Tags {
				pairs = append(pairs, [2]*string{t.Key, t.Value})
			}
			cr.Spec.Tags = kvTagMap(pairs)
			opts.Index.Add(aws.ToString(ss.StackSetId), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

// sfnTags fetches Step Functions resource tags, returning nil on failure.
func sfnTags(ctx context.Context, clients *awsclient.Clients, arn *string) map[string]string {
	out, err := clients.SFN.ListTagsForResource(ctx, &awssfn.ListTagsForResourceInput{ResourceArn: arn})
	if err != nil {
		return nil
	}
	pairs := make([][2]*string, 0, len(out.Tags))
	for _, t := range out.Tags {
		pairs = append(pairs, [2]*string{t.Key, t.Value})
	}
	return kvTagMap(pairs)
}

func exportStateMachines(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awssfn.NewListStateMachinesPaginator(clients.SFN, &awssfn.ListStateMachinesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list state machines: %w", err)
		}
		for _, sm := range page.StateMachines {
			got, err := clients.SFN.DescribeStateMachine(ctx, &awssfn.DescribeStateMachineInput{
				StateMachineArn: sm.StateMachineArn,
			})
			if err != nil {
				continue
			}
			name := aws.ToString(sm.Name)
			cr := &awsv1alpha1.StateMachine{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.StateMachineSpec{
					Name:       name,
					Definition: aws.ToString(got.Definition),
					RoleARN:    aws.ToString(got.RoleArn),
					Type:       string(got.Type),
					Tags:       sfnTags(ctx, clients, sm.StateMachineArn),
				},
			}
			opts.Index.Add(aws.ToString(sm.StateMachineArn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportActivities(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awssfn.NewListActivitiesPaginator(clients.SFN, &awssfn.ListActivitiesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list activities: %w", err)
		}
		for _, a := range page.Activities {
			name := aws.ToString(a.Name)
			cr := &awsv1alpha1.Activity{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.ActivitySpec{
					Name: name,
					Tags: sfnTags(ctx, clients, a.ActivityArn),
				},
			}
			opts.Index.Add(aws.ToString(a.ActivityArn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportCodePipelines(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awscodepipeline.NewListPipelinesPaginator(clients.CodePipeline, &awscodepipeline.ListPipelinesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list pipelines: %w", err)
		}
		for _, s := range page.Pipelines {
			got, err := clients.CodePipeline.GetPipeline(ctx, &awscodepipeline.GetPipelineInput{Name: s.Name})
			if err != nil || got.Pipeline == nil {
				continue
			}
			pl := got.Pipeline
			name := aws.ToString(pl.Name)
			cr := &awsv1alpha1.CodePipeline{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.CodePipelineSpec{
					PipelineName: name,
					RoleARN:      aws.ToString(pl.RoleArn),
				},
			}
			if pl.ArtifactStore != nil {
				cr.Spec.ArtifactStore = awsv1alpha1.CodePipelineArtifactStore{
					Type:     string(pl.ArtifactStore.Type),
					Location: aws.ToString(pl.ArtifactStore.Location),
				}
			}
			for _, st := range pl.Stages {
				stage := awsv1alpha1.CodePipelineStage{Name: aws.ToString(st.Name)}
				for _, act := range st.Actions {
					action := awsv1alpha1.CodePipelineAction{
						Name:          aws.ToString(act.Name),
						Configuration: act.Configuration,
						RunOrder:      act.RunOrder,
					}
					if act.ActionTypeId != nil {
						action.ActionTypeID = awsv1alpha1.CodePipelineActionTypeID{
							Category: string(act.ActionTypeId.Category),
							Owner:    string(act.ActionTypeId.Owner),
							Provider: aws.ToString(act.ActionTypeId.Provider),
							Version:  aws.ToString(act.ActionTypeId.Version),
						}
					}
					for _, ia := range act.InputArtifacts {
						action.InputArtifacts = append(action.InputArtifacts, aws.ToString(ia.Name))
					}
					for _, oa := range act.OutputArtifacts {
						action.OutputArtifacts = append(action.OutputArtifacts, aws.ToString(oa.Name))
					}
					stage.Actions = append(stage.Actions, action)
				}
				cr.Spec.Stages = append(cr.Spec.Stages, stage)
			}
			var pipelineARN *string
			if got.Metadata != nil {
				pipelineARN = got.Metadata.PipelineArn
			}
			if pipelineARN != nil {
				tagsOut, err := clients.CodePipeline.ListTagsForResource(ctx, &awscodepipeline.ListTagsForResourceInput{
					ResourceArn: pipelineARN,
				})
				if err == nil {
					pairs := make([][2]*string, 0, len(tagsOut.Tags))
					for _, t := range tagsOut.Tags {
						pairs = append(pairs, [2]*string{t.Key, t.Value})
					}
					cr.Spec.Tags = kvTagMap(pairs)
				}
				opts.Index.Add(aws.ToString(pipelineARN), cr.Name)
			}
			opts.Index.Add(name, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportCodeBuildProjects(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awscodebuild.NewListProjectsPaginator(clients.CodeBuild, &awscodebuild.ListProjectsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list codebuild projects: %w", err)
		}
		if len(page.Projects) == 0 {
			continue
		}
		got, err := clients.CodeBuild.BatchGetProjects(ctx, &awscodebuild.BatchGetProjectsInput{
			Names: page.Projects,
		})
		if err != nil {
			return nil, fmt.Errorf("batch get codebuild projects: %w", err)
		}
		for _, proj := range got.Projects {
			name := aws.ToString(proj.Name)
			cr := &awsv1alpha1.CodeBuildProject{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.CodeBuildProjectSpec{
					Name:           name,
					Description:    aws.ToString(proj.Description),
					ServiceRoleARN: aws.ToString(proj.ServiceRole),
				},
			}
			if proj.Source != nil {
				cr.Spec.Source = awsv1alpha1.CodeBuildSource{
					Type:      string(proj.Source.Type),
					Location:  aws.ToString(proj.Source.Location),
					Buildspec: aws.ToString(proj.Source.Buildspec),
				}
			}
			if proj.Artifacts != nil {
				cr.Spec.Artifacts = awsv1alpha1.CodeBuildArtifacts{
					Type:     string(proj.Artifacts.Type),
					Location: aws.ToString(proj.Artifacts.Location),
				}
			}
			if proj.Environment != nil {
				cr.Spec.Environment = awsv1alpha1.CodeBuildEnvironment{
					Type:           string(proj.Environment.Type),
					Image:          aws.ToString(proj.Environment.Image),
					ComputeType:    string(proj.Environment.ComputeType),
					PrivilegedMode: aws.ToBool(proj.Environment.PrivilegedMode),
				}
			}
			pairs := make([][2]*string, 0, len(proj.Tags))
			for _, t := range proj.Tags {
				pairs = append(pairs, [2]*string{t.Key, t.Value})
			}
			cr.Spec.Tags = kvTagMap(pairs)
			opts.Index.Add(aws.ToString(proj.Arn), cr.Name)
			opts.Index.Add(name, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportCodeDeployApplications(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awscodedeploy.NewListApplicationsPaginator(clients.CodeDeploy, &awscodedeploy.ListApplicationsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list codedeploy applications: %w", err)
		}
		if len(page.Applications) == 0 {
			continue
		}
		got, err := clients.CodeDeploy.BatchGetApplications(ctx, &awscodedeploy.BatchGetApplicationsInput{
			ApplicationNames: page.Applications,
		})
		if err != nil {
			return nil, fmt.Errorf("batch get codedeploy applications: %w", err)
		}
		for _, app := range got.ApplicationsInfo {
			name := aws.ToString(app.ApplicationName)
			cr := &awsv1alpha1.CodeDeployApplication{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.CodeDeployApplicationSpec{
					ApplicationName: name,
					ComputePlatform: string(app.ComputePlatform),
				},
			}
			opts.Index.Add(aws.ToString(app.ApplicationId), cr.Name)
			opts.Index.Add(name, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportCodeDeployDeploymentGroups(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	appsP := awscodedeploy.NewListApplicationsPaginator(clients.CodeDeploy, &awscodedeploy.ListApplicationsInput{})
	for appsP.HasMorePages() {
		appsPage, err := appsP.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list codedeploy applications: %w", err)
		}
		for _, appName := range appsPage.Applications {
			appName := appName
			dgP := awscodedeploy.NewListDeploymentGroupsPaginator(clients.CodeDeploy, &awscodedeploy.ListDeploymentGroupsInput{
				ApplicationName: aws.String(appName),
			})
			for dgP.HasMorePages() {
				dgPage, err := dgP.NextPage(ctx)
				if err != nil {
					break
				}
				if len(dgPage.DeploymentGroups) == 0 {
					continue
				}
				got, err := clients.CodeDeploy.BatchGetDeploymentGroups(ctx, &awscodedeploy.BatchGetDeploymentGroupsInput{
					ApplicationName:      aws.String(appName),
					DeploymentGroupNames: dgPage.DeploymentGroups,
				})
				if err != nil {
					continue
				}
				for _, dg := range got.DeploymentGroupsInfo {
					dgName := aws.ToString(dg.DeploymentGroupName)
					cr := &awsv1alpha1.CodeDeployDeploymentGroup{
						ObjectMeta: export.ObjectMeta(appName+"-"+dgName, opts),
						Spec: awsv1alpha1.CodeDeployDeploymentGroupSpec{
							ApplicationName:      aws.ToString(dg.ApplicationName),
							DeploymentGroupName:  dgName,
							ServiceRoleARN:       aws.ToString(dg.ServiceRoleArn),
							DeploymentConfigName: aws.ToString(dg.DeploymentConfigName),
						},
					}
					for _, asg := range dg.AutoScalingGroups {
						cr.Spec.AutoScalingGroups = append(cr.Spec.AutoScalingGroups, aws.ToString(asg.Name))
					}
					opts.Index.Add(aws.ToString(dg.DeploymentGroupId), cr.Name)
					objs = append(objs, cr)
				}
			}
		}
	}
	return objs, nil
}

func exportCodeCommitRepositories(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awscodecommit.NewListRepositoriesPaginator(clients.CodeCommit, &awscodecommit.ListRepositoriesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list codecommit repositories: %w", err)
		}
		for _, r := range page.Repositories {
			got, err := clients.CodeCommit.GetRepository(ctx, &awscodecommit.GetRepositoryInput{
				RepositoryName: r.RepositoryName,
			})
			if err != nil || got.RepositoryMetadata == nil {
				continue
			}
			md := got.RepositoryMetadata
			name := aws.ToString(md.RepositoryName)
			cr := &awsv1alpha1.CodeCommitRepository{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.CodeCommitRepositorySpec{
					RepositoryName:        name,
					RepositoryDescription: aws.ToString(md.RepositoryDescription),
				},
			}
			tagsOut, err := clients.CodeCommit.ListTagsForResource(ctx, &awscodecommit.ListTagsForResourceInput{
				ResourceArn: md.Arn,
			})
			if err == nil {
				cr.Spec.Tags = export.TagMap(tagsOut.Tags)
			}
			opts.Index.Add(aws.ToString(md.Arn), cr.Name)
			opts.Index.Add(aws.ToString(md.RepositoryId), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportOpenSearchDomains(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	// ListDomainNames is not paginated: it returns every domain in the region.
	names, err := clients.OpenSearch.ListDomainNames(ctx, &awsopensearch.ListDomainNamesInput{})
	if err != nil {
		return nil, fmt.Errorf("list opensearch domains: %w", err)
	}
	for _, d := range names.DomainNames {
		got, err := clients.OpenSearch.DescribeDomain(ctx, &awsopensearch.DescribeDomainInput{
			DomainName: d.DomainName,
		})
		if err != nil || got.DomainStatus == nil {
			continue
		}
		ds := got.DomainStatus
		// Skip domains mid-create/update: their config isn't steady state.
		if aws.ToBool(ds.Processing) {
			continue
		}
		name := aws.ToString(ds.DomainName)
		cr := &awsv1alpha1.OpenSearchDomain{
			ObjectMeta: export.ObjectMeta(name, opts),
			Spec: awsv1alpha1.OpenSearchDomainSpec{
				DomainName:    name,
				EngineVersion: aws.ToString(ds.EngineVersion),
			},
		}
		if cc := ds.ClusterConfig; cc != nil {
			cr.Spec.ClusterConfig = &awsv1alpha1.OpenSearchClusterConfig{
				InstanceType:           string(cc.InstanceType),
				InstanceCount:          cc.InstanceCount,
				DedicatedMasterEnabled: cc.DedicatedMasterEnabled,
			}
		}
		tagsOut, err := clients.OpenSearch.ListTags(ctx, &awsopensearch.ListTagsInput{ARN: ds.ARN})
		if err == nil {
			pairs := make([][2]*string, 0, len(tagsOut.TagList))
			for _, t := range tagsOut.TagList {
				pairs = append(pairs, [2]*string{t.Key, t.Value})
			}
			cr.Spec.Tags = kvTagMap(pairs)
		}
		opts.Index.Add(aws.ToString(ds.ARN), cr.Name)
		opts.Index.Add(name, cr.Name)
		objs = append(objs, cr)
	}
	return objs, nil
}

func exportOpenSearchServerlessCollections(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsoss.NewListCollectionsPaginator(clients.OpenSearchServerless, &awsoss.ListCollectionsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list opensearch serverless collections: %w", err)
		}
		for _, s := range page.CollectionSummaries {
			got, err := clients.OpenSearchServerless.BatchGetCollection(ctx, &awsoss.BatchGetCollectionInput{
				Ids: []string{aws.ToString(s.Id)},
			})
			if err != nil || len(got.CollectionDetails) == 0 {
				continue
			}
			detail := got.CollectionDetails[0]
			name := aws.ToString(detail.Name)
			cr := &awsv1alpha1.OpenSearchServerlessCollection{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.OpenSearchServerlessCollectionSpec{
					Name:        name,
					Type:        string(detail.Type),
					Description: aws.ToString(detail.Description),
				},
			}
			tagsOut, err := clients.OpenSearchServerless.ListTagsForResource(ctx, &awsoss.ListTagsForResourceInput{
				ResourceArn: detail.Arn,
			})
			if err == nil {
				pairs := make([][2]*string, 0, len(tagsOut.Tags))
				for _, t := range tagsOut.Tags {
					pairs = append(pairs, [2]*string{t.Key, t.Value})
				}
				cr.Spec.Tags = kvTagMap(pairs)
			}
			opts.Index.Add(aws.ToString(detail.Id), cr.Name)
			opts.Index.Add(aws.ToString(detail.Arn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportOpenSearchAccessPolicies(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsoss.NewListAccessPoliciesPaginator(clients.OpenSearchServerless, &awsoss.ListAccessPoliciesInput{
		Type: osstypes.AccessPolicyTypeData,
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list opensearch access policies: %w", err)
		}
		for _, s := range page.AccessPolicySummaries {
			got, err := clients.OpenSearchServerless.GetAccessPolicy(ctx, &awsoss.GetAccessPolicyInput{
				Name: s.Name,
				Type: s.Type,
			})
			if err != nil || got.AccessPolicyDetail == nil || got.AccessPolicyDetail.Policy == nil {
				continue
			}
			doc, err := got.AccessPolicyDetail.Policy.MarshalSmithyDocument()
			if err != nil {
				continue
			}
			name := aws.ToString(s.Name)
			cr := &awsv1alpha1.OpenSearchAccessPolicy{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.OpenSearchAccessPolicySpec{
					Name:           name,
					Type:           string(s.Type),
					PolicyDocument: string(doc),
					Description:    aws.ToString(s.Description),
				},
			}
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportSSMDocuments(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsssm.NewListDocumentsPaginator(clients.SSM, &awsssm.ListDocumentsInput{
		// Owner=Self excludes the thousands of AWS-owned public documents.
		Filters: []ssmtypes.DocumentKeyValuesFilter{
			{Key: aws.String("Owner"), Values: []string{"Self"}},
		},
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list ssm documents: %w", err)
		}
		for _, d := range page.DocumentIdentifiers {
			got, err := clients.SSM.GetDocument(ctx, &awsssm.GetDocumentInput{
				Name:           d.Name,
				DocumentFormat: d.DocumentFormat,
			})
			if err != nil {
				continue
			}
			name := aws.ToString(d.Name)
			cr := &awsv1alpha1.SSMDocument{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.SSMDocumentSpec{
					Name:           name,
					Content:        aws.ToString(got.Content),
					DocumentType:   string(d.DocumentType),
					DocumentFormat: string(d.DocumentFormat),
					VersionName:    aws.ToString(d.VersionName),
				},
			}
			pairs := make([][2]*string, 0, len(d.Tags))
			for _, t := range d.Tags {
				pairs = append(pairs, [2]*string{t.Key, t.Value})
			}
			cr.Spec.Tags = kvTagMap(pairs)
			opts.Index.Add(name, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportSecretRotations(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awssecrets.NewListSecretsPaginator(clients.SecretsManager, &awssecrets.ListSecretsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list secrets: %w", err)
		}
		for _, s := range page.SecretList {
			// Only secrets with rotation actually configured produce a CR.
			if !aws.ToBool(s.RotationEnabled) || aws.ToString(s.RotationLambdaARN) == "" {
				continue
			}
			// The CRD requires a day interval (1-365); schedule-expression-only
			// rotations cannot be expressed in the spec, so skip them.
			if s.RotationRules == nil || s.RotationRules.AutomaticallyAfterDays == nil {
				continue
			}
			secretARN := aws.ToString(s.ARN)
			cr := &awsv1alpha1.SecretRotation{
				ObjectMeta: export.ObjectMeta(aws.ToString(s.Name)+"-rotation", opts),
				Spec: awsv1alpha1.SecretRotationSpec{
					RotationLambdaARN:      aws.ToString(s.RotationLambdaARN),
					AutomaticallyAfterDays: int32(aws.ToInt64(s.RotationRules.AutomaticallyAfterDays)),
				},
			}
			// Reference the Secret CR when it was exported in this run; fall
			// back to the raw ARN for unmanaged secrets.
			if crName, ok := opts.Index.Lookup(secretARN); ok {
				cr.Spec.SecretRef = awsv1alpha1.SecretsManagerSecretRef{Name: crName}
			} else {
				cr.Spec.SecretRef = awsv1alpha1.SecretsManagerSecretRef{SecretARN: secretARN}
			}
			objs = append(objs, cr)
		}
	}
	return objs, nil
}
