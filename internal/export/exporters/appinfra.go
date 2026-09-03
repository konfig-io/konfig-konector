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
	awsacmpca "github.com/aws/aws-sdk-go-v2/service/acmpca"
	acmpcatypes "github.com/aws/aws-sdk-go-v2/service/acmpca/types"
	awsapprunner "github.com/aws/aws-sdk-go-v2/service/apprunner"
	apprunnertypes "github.com/aws/aws-sdk-go-v2/service/apprunner/types"
	awsbatch "github.com/aws/aws-sdk-go-v2/service/batch"
	awsmq "github.com/aws/aws-sdk-go-v2/service/mq"
	awssd "github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	sdtypes "github.com/aws/aws-sdk-go-v2/service/servicediscovery/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	awsclient "github.com/konfig-io/konfig-konector/internal/aws"
	"github.com/konfig-io/konfig-konector/internal/export"
)

func init() {
	// Namespaces before services and compute environments before job queues:
	// referrers look their targets up in the index.
	export.Register(export.Exporter{Kind: "CloudMapNamespace", Service: "servicediscovery", Order: 100, Fn: exportCloudMapNamespaces})
	export.Register(export.Exporter{Kind: "CloudMapService", Service: "servicediscovery", Order: 101, Fn: exportCloudMapServices})
	export.Register(export.Exporter{Kind: "BatchComputeEnvironment", Service: "batch", Order: 100, Fn: exportBatchComputeEnvironments})
	export.Register(export.Exporter{Kind: "BatchJobQueue", Service: "batch", Order: 101, Fn: exportBatchJobQueues})
	export.Register(export.Exporter{Kind: "BatchJobDefinition", Service: "batch", Order: 102, Fn: exportBatchJobDefinitions})
	export.Register(export.Exporter{Kind: "MQBroker", Service: "mq", Order: 102, Fn: exportMQBrokers})
	export.Register(export.Exporter{Kind: "MQConfiguration", Service: "mq", Order: 102, Fn: exportMQConfigurations})
	export.Register(export.Exporter{Kind: "AppRunnerAutoScaling", Service: "apprunner", Order: 103, Fn: exportAppRunnerAutoScalings})
	export.Register(export.Exporter{Kind: "AppRunnerService", Service: "apprunner", Order: 103, Fn: exportAppRunnerServices})
	export.Register(export.Exporter{Kind: "PrivateCA", Service: "acmpca", Order: 104, Fn: exportPrivateCAs})
}

func exportMQBrokers(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsmq.NewListBrokersPaginator(clients.MQ, &awsmq.ListBrokersInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list MQ brokers: %w", err)
		}
		for _, s := range page.BrokerSummaries {
			got, err := clients.MQ.DescribeBroker(ctx, &awsmq.DescribeBrokerInput{
				BrokerId: s.BrokerId,
			})
			if err != nil {
				continue
			}
			name := aws.ToString(got.BrokerName)
			cr := &awsv1alpha1.MQBroker{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.MQBrokerSpec{
					BrokerName:              name,
					EngineType:              string(got.EngineType),
					EngineVersion:           aws.ToString(got.EngineVersion),
					HostInstanceType:        aws.ToString(got.HostInstanceType),
					DeploymentMode:          string(got.DeploymentMode),
					PubliclyAccessible:      aws.ToBool(got.PubliclyAccessible),
					AutoMinorVersionUpgrade: aws.ToBool(got.AutoMinorVersionUpgrade),
					Tags:                    export.TagMap(got.Tags),
				},
			}
			for _, subnetID := range got.SubnetIds {
				subnetRef := awsv1alpha1.SubnetRef{ID: subnetID}
				if crName, ok := opts.Index.Lookup(subnetID); ok {
					subnetRef = awsv1alpha1.SubnetRef{Name: crName}
				}
				cr.Spec.SubnetRefs = append(cr.Spec.SubnetRefs, subnetRef)
			}
			for _, sgID := range got.SecurityGroups {
				sgRef := awsv1alpha1.SecurityGroupRef{ID: sgID}
				if crName, ok := opts.Index.Lookup(sgID); ok {
					sgRef = awsv1alpha1.SecurityGroupRef{Name: crName}
				}
				cr.Spec.SecurityGroupRefs = append(cr.Spec.SecurityGroupRefs, sgRef)
			}
			// Passwords never round-trip: emit a CHANGEME placeholder Secret
			// reference the operator must create before managing the broker.
			for _, u := range got.Users {
				cr.Spec.Users = append(cr.Spec.Users, awsv1alpha1.MQUser{
					Username: aws.ToString(u.Username),
					PasswordRef: awsv1alpha1.SecretRef{
						Name: "CHANGEME",
						Key:  "CHANGEME",
					},
				})
			}
			opts.Index.Add(aws.ToString(got.BrokerArn), cr.Name)
			opts.Index.Add(aws.ToString(got.BrokerId), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportMQConfigurations(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	// ListConfigurations has no SDK paginator: page manually via NextToken.
	var nextToken *string
	for {
		page, err := clients.MQ.ListConfigurations(ctx, &awsmq.ListConfigurationsInput{
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("list MQ configurations: %w", err)
		}
		for _, c := range page.Configurations {
			name := aws.ToString(c.Name)
			cr := &awsv1alpha1.MQConfiguration{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.MQConfigurationSpec{
					Name:          name,
					EngineType:    string(c.EngineType),
					EngineVersion: aws.ToString(c.EngineVersion),
					Tags:          export.TagMap(c.Tags),
				},
			}
			// Fetch the latest revision data (base64 XML/Cuttlefish).
			if c.LatestRevision != nil {
				rev, err := clients.MQ.DescribeConfigurationRevision(ctx, &awsmq.DescribeConfigurationRevisionInput{
					ConfigurationId:       c.Id,
					ConfigurationRevision: aws.String(fmt.Sprintf("%d", aws.ToInt32(c.LatestRevision.Revision))),
				})
				if err == nil {
					cr.Spec.Data = aws.ToString(rev.Data)
				}
			}
			opts.Index.Add(aws.ToString(c.Id), cr.Name)
			opts.Index.Add(aws.ToString(c.Arn), cr.Name)
			objs = append(objs, cr)
		}
		if page.NextToken == nil || aws.ToString(page.NextToken) == "" {
			break
		}
		nextToken = page.NextToken
	}
	return objs, nil
}

func exportBatchComputeEnvironments(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsbatch.NewDescribeComputeEnvironmentsPaginator(clients.Batch, &awsbatch.DescribeComputeEnvironmentsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe Batch compute environments: %w", err)
		}
		for _, ce := range page.ComputeEnvironments {
			name := aws.ToString(ce.ComputeEnvironmentName)
			cr := &awsv1alpha1.BatchComputeEnvironment{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.BatchComputeEnvironmentSpec{
					Name:  name,
					Type:  string(ce.Type),
					State: string(ce.State),
					Tags:  export.TagMap(ce.Tags),
				},
			}
			if res := ce.ComputeResources; res != nil {
				crSpec := &awsv1alpha1.BatchComputeResources{
					Type:            string(res.Type),
					MaxvCpus:        aws.ToInt32(res.MaxvCpus),
					MinvCpus:        res.MinvCpus,
					DesiredvCpus:    res.DesiredvCpus,
					InstanceTypes:   res.InstanceTypes,
					InstanceRoleArn: aws.ToString(res.InstanceRole),
				}
				for _, subnetID := range res.Subnets {
					ref := awsv1alpha1.SubnetRef{ID: subnetID}
					if crName, ok := opts.Index.Lookup(subnetID); ok {
						ref = awsv1alpha1.SubnetRef{Name: crName}
					}
					crSpec.SubnetRefs = append(crSpec.SubnetRefs, ref)
				}
				for _, sgID := range res.SecurityGroupIds {
					ref := awsv1alpha1.SecurityGroupRef{ID: sgID}
					if crName, ok := opts.Index.Lookup(sgID); ok {
						ref = awsv1alpha1.SecurityGroupRef{Name: crName}
					}
					crSpec.SecurityGroupRefs = append(crSpec.SecurityGroupRefs, ref)
				}
				cr.Spec.ComputeResources = crSpec
			}
			opts.Index.Add(aws.ToString(ce.ComputeEnvironmentArn), cr.Name)
			opts.Index.Add(name, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportBatchJobQueues(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsbatch.NewDescribeJobQueuesPaginator(clients.Batch, &awsbatch.DescribeJobQueuesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe Batch job queues: %w", err)
		}
		for _, jq := range page.JobQueues {
			name := aws.ToString(jq.JobQueueName)
			cr := &awsv1alpha1.BatchJobQueue{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.BatchJobQueueSpec{
					Name:     name,
					State:    string(jq.State),
					Priority: aws.ToInt32(jq.Priority),
					Tags:     export.TagMap(jq.Tags),
				},
			}
			for _, ceo := range jq.ComputeEnvironmentOrder {
				arn := aws.ToString(ceo.ComputeEnvironment)
				ref := awsv1alpha1.BatchComputeEnvironmentRef{ARN: arn}
				if crName, ok := opts.Index.Lookup(arn); ok {
					ref = awsv1alpha1.BatchComputeEnvironmentRef{Name: crName}
				}
				cr.Spec.ComputeEnvironmentOrder = append(cr.Spec.ComputeEnvironmentOrder, awsv1alpha1.BatchComputeEnvironmentOrder{
					ComputeEnvironmentRef: ref,
					Order:                 aws.ToInt32(ceo.Order),
				})
			}
			opts.Index.Add(aws.ToString(jq.JobQueueArn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportBatchJobDefinitions(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsbatch.NewDescribeJobDefinitionsPaginator(clients.Batch, &awsbatch.DescribeJobDefinitionsInput{
		Status: aws.String("ACTIVE"),
	})
	seen := map[string]int32{}
	var crs = map[string]*awsv1alpha1.BatchJobDefinition{}
	var order []string
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe Batch job definitions: %w", err)
		}
		for _, jd := range page.JobDefinitions {
			// Only container job definitions map to the CRD.
			if aws.ToString(jd.Type) != "container" || jd.ContainerProperties == nil {
				continue
			}
			name := aws.ToString(jd.JobDefinitionName)
			// Keep only the latest active revision per name.
			if rev, ok := seen[name]; ok && rev >= aws.ToInt32(jd.Revision) {
				continue
			}
			seen[name] = aws.ToInt32(jd.Revision)

			cp := jd.ContainerProperties
			spec := awsv1alpha1.BatchContainerProperties{
				Image:            aws.ToString(cp.Image),
				Command:          cp.Command,
				JobRoleArn:       aws.ToString(cp.JobRoleArn),
				ExecutionRoleArn: aws.ToString(cp.ExecutionRoleArn),
			}
			for _, rr := range cp.ResourceRequirements {
				spec.ResourceRequirements = append(spec.ResourceRequirements, awsv1alpha1.BatchResourceRequirement{
					Type:  string(rr.Type),
					Value: aws.ToString(rr.Value),
				})
			}
			if len(cp.Environment) > 0 {
				spec.Environment = make(map[string]string, len(cp.Environment))
				for _, kv := range cp.Environment {
					spec.Environment[aws.ToString(kv.Name)] = aws.ToString(kv.Value)
				}
			}
			cr := &awsv1alpha1.BatchJobDefinition{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.BatchJobDefinitionSpec{
					Name:                name,
					Type:                aws.ToString(jd.Type),
					ContainerProperties: spec,
					Tags:                export.TagMap(jd.Tags),
				},
			}
			for _, pc := range jd.PlatformCapabilities {
				cr.Spec.PlatformCapabilities = append(cr.Spec.PlatformCapabilities, string(pc))
			}
			if jd.RetryStrategy != nil && jd.RetryStrategy.Attempts != nil {
				cr.Spec.RetryStrategy = &awsv1alpha1.BatchRetryStrategy{Attempts: aws.ToInt32(jd.RetryStrategy.Attempts)}
			}
			if jd.Timeout != nil && jd.Timeout.AttemptDurationSeconds != nil {
				cr.Spec.Timeout = &awsv1alpha1.BatchJobTimeout{AttemptDurationSeconds: aws.ToInt32(jd.Timeout.AttemptDurationSeconds)}
			}
			if _, ok := crs[name]; !ok {
				order = append(order, name)
			}
			crs[name] = cr
			opts.Index.Add(aws.ToString(jd.JobDefinitionArn), cr.Name)
		}
	}
	for _, name := range order {
		objs = append(objs, crs[name])
	}
	return objs, nil
}

func exportAppRunnerServices(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsapprunner.NewListServicesPaginator(clients.AppRunner, &awsapprunner.ListServicesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list App Runner services: %w", err)
		}
		for _, s := range page.ServiceSummaryList {
			// Only steady-state RUNNING services produce a CR.
			if s.Status != apprunnertypes.ServiceStatusRunning {
				continue
			}
			got, err := clients.AppRunner.DescribeService(ctx, &awsapprunner.DescribeServiceInput{
				ServiceArn: s.ServiceArn,
			})
			if err != nil || got.Service == nil {
				continue
			}
			svc := got.Service
			// Only image-based services map to the CRD.
			if svc.SourceConfiguration == nil || svc.SourceConfiguration.ImageRepository == nil {
				continue
			}
			name := aws.ToString(svc.ServiceName)
			imgRepo := svc.SourceConfiguration.ImageRepository
			cr := &awsv1alpha1.AppRunnerService{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.AppRunnerServiceSpec{
					ServiceName: name,
					SourceConfiguration: awsv1alpha1.AppRunnerSourceConfiguration{
						ImageRepository: awsv1alpha1.AppRunnerImageRepository{
							ImageIdentifier:     aws.ToString(imgRepo.ImageIdentifier),
							ImageRepositoryType: string(imgRepo.ImageRepositoryType),
						},
						AutoDeploymentsEnabled: svc.SourceConfiguration.AutoDeploymentsEnabled,
					},
				},
			}
			if ic := imgRepo.ImageConfiguration; ic != nil {
				cr.Spec.SourceConfiguration.ImageRepository.ImageConfiguration = &awsv1alpha1.AppRunnerImageConfiguration{
					Port:                        aws.ToString(ic.Port),
					RuntimeEnvironmentVariables: ic.RuntimeEnvironmentVariables,
					StartCommand:                aws.ToString(ic.StartCommand),
				}
			}
			if ac := svc.SourceConfiguration.AuthenticationConfiguration; ac != nil && aws.ToString(ac.AccessRoleArn) != "" {
				accessRoleArn := aws.ToString(ac.AccessRoleArn)
				authCfg := &awsv1alpha1.AppRunnerAuthenticationConfiguration{AccessRoleArn: accessRoleArn}
				if crName, ok := opts.Index.Lookup(accessRoleArn); ok {
					authCfg = &awsv1alpha1.AppRunnerAuthenticationConfiguration{AccessRoleRef: &awsv1alpha1.RoleRef{Name: crName}}
				}
				cr.Spec.SourceConfiguration.AuthenticationConfiguration = authCfg
			}
			if ic := svc.InstanceConfiguration; ic != nil {
				instCfg := &awsv1alpha1.AppRunnerInstanceConfiguration{
					CPU:    aws.ToString(ic.Cpu),
					Memory: aws.ToString(ic.Memory),
				}
				if roleArn := aws.ToString(ic.InstanceRoleArn); roleArn != "" {
					if crName, ok := opts.Index.Lookup(roleArn); ok {
						instCfg.InstanceRoleRef = &awsv1alpha1.RoleRef{Name: crName}
					} else {
						instCfg.InstanceRoleArn = roleArn
					}
				}
				cr.Spec.InstanceConfiguration = instCfg
			}
			if hc := svc.HealthCheckConfiguration; hc != nil {
				cr.Spec.HealthCheckConfiguration = &awsv1alpha1.AppRunnerHealthCheckConfiguration{
					Protocol:           string(hc.Protocol),
					Path:               aws.ToString(hc.Path),
					Interval:           aws.ToInt32(hc.Interval),
					Timeout:            aws.ToInt32(hc.Timeout),
					HealthyThreshold:   aws.ToInt32(hc.HealthyThreshold),
					UnhealthyThreshold: aws.ToInt32(hc.UnhealthyThreshold),
				}
			}
			tagsOut, err := clients.AppRunner.ListTagsForResource(ctx, &awsapprunner.ListTagsForResourceInput{
				ResourceArn: svc.ServiceArn,
			})
			if err == nil {
				pairs := make([][2]*string, 0, len(tagsOut.Tags))
				for _, t := range tagsOut.Tags {
					pairs = append(pairs, [2]*string{t.Key, t.Value})
				}
				cr.Spec.Tags = kvTagMap(pairs)
			}
			opts.Index.Add(aws.ToString(svc.ServiceArn), cr.Name)
			opts.Index.Add(aws.ToString(svc.ServiceId), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportAppRunnerAutoScalings(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsapprunner.NewListAutoScalingConfigurationsPaginator(clients.AppRunner, &awsapprunner.ListAutoScalingConfigurationsInput{
		LatestOnly: true,
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list App Runner auto scaling configurations: %w", err)
		}
		for _, s := range page.AutoScalingConfigurationSummaryList {
			if s.Status != apprunnertypes.AutoScalingConfigurationStatusActive {
				continue
			}
			got, err := clients.AppRunner.DescribeAutoScalingConfiguration(ctx, &awsapprunner.DescribeAutoScalingConfigurationInput{
				AutoScalingConfigurationArn: s.AutoScalingConfigurationArn,
			})
			if err != nil || got.AutoScalingConfiguration == nil {
				continue
			}
			asc := got.AutoScalingConfiguration
			name := aws.ToString(asc.AutoScalingConfigurationName)
			cr := &awsv1alpha1.AppRunnerAutoScaling{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.AppRunnerAutoScalingSpec{
					Name:           name,
					MaxConcurrency: aws.ToInt32(asc.MaxConcurrency),
					MaxSize:        aws.ToInt32(asc.MaxSize),
					MinSize:        aws.ToInt32(asc.MinSize),
				},
			}
			opts.Index.Add(aws.ToString(asc.AutoScalingConfigurationArn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportPrivateCAs(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsacmpca.NewListCertificateAuthoritiesPaginator(clients.ACMPCA, &awsacmpca.ListCertificateAuthoritiesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list private CAs: %w", err)
		}
		for _, ca := range page.CertificateAuthorities {
			// Only ACTIVE CAs produce a CR.
			if ca.Status != acmpcatypes.CertificateAuthorityStatusActive {
				continue
			}
			if ca.CertificateAuthorityConfiguration == nil {
				continue
			}
			cfg := ca.CertificateAuthorityConfiguration
			name := "private-ca"
			if cfg.Subject != nil && aws.ToString(cfg.Subject.CommonName) != "" {
				name = aws.ToString(cfg.Subject.CommonName)
			}
			cr := &awsv1alpha1.PrivateCA{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.PrivateCASpec{
					Type:                        string(ca.Type),
					KeyAlgorithm:                string(cfg.KeyAlgorithm),
					SigningAlgorithm:            string(cfg.SigningAlgorithm),
					UsageMode:                   string(ca.UsageMode),
					PermanentDeletionTimeInDays: 30,
				},
			}
			if subj := cfg.Subject; subj != nil {
				cr.Spec.Subject = awsv1alpha1.PrivateCASubject{
					CommonName:         aws.ToString(subj.CommonName),
					Organization:       aws.ToString(subj.Organization),
					OrganizationalUnit: aws.ToString(subj.OrganizationalUnit),
					Country:            aws.ToString(subj.Country),
					State:              aws.ToString(subj.State),
					Locality:           aws.ToString(subj.Locality),
				}
			}
			if rc := ca.RevocationConfiguration; rc != nil && rc.CrlConfiguration != nil {
				crl := rc.CrlConfiguration
				cr.Spec.RevocationConfiguration = &awsv1alpha1.PrivateCARevocationConfiguration{
					Crl: &awsv1alpha1.PrivateCACrlConfiguration{
						Enabled:          aws.ToBool(crl.Enabled),
						S3BucketName:     aws.ToString(crl.S3BucketName),
						ExpirationInDays: aws.ToInt32(crl.ExpirationInDays),
					},
				}
			}
			tagsOut, err := clients.ACMPCA.ListTags(ctx, &awsacmpca.ListTagsInput{
				CertificateAuthorityArn: ca.Arn,
			})
			if err == nil {
				pairs := make([][2]*string, 0, len(tagsOut.Tags))
				for _, t := range tagsOut.Tags {
					pairs = append(pairs, [2]*string{t.Key, t.Value})
				}
				cr.Spec.Tags = kvTagMap(pairs)
			}
			opts.Index.Add(aws.ToString(ca.Arn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportCloudMapNamespaces(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awssd.NewListNamespacesPaginator(clients.ServiceDiscovery, &awssd.ListNamespacesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list Cloud Map namespaces: %w", err)
		}
		for _, ns := range page.Namespaces {
			var nsType string
			switch ns.Type {
			case sdtypes.NamespaceTypeDnsPrivate:
				nsType = "PRIVATE_DNS"
			case sdtypes.NamespaceTypeDnsPublic:
				nsType = "PUBLIC_DNS"
			case sdtypes.NamespaceTypeHttp:
				nsType = "HTTP"
			default:
				continue
			}
			name := aws.ToString(ns.Name)
			cr := &awsv1alpha1.CloudMapNamespace{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.CloudMapNamespaceSpec{
					Name:        name,
					Type:        nsType,
					Description: aws.ToString(ns.Description),
				},
			}
			if nsType == "PRIVATE_DNS" {
				// The list/get APIs do not return the associated VPC, and the
				// controller requires vpcRef for private DNS namespaces:
				// emit a CHANGEME placeholder for the operator to fill in.
				cr.Spec.VPCRef = &awsv1alpha1.VPCResourceRef{ID: "CHANGEME"}
			}
			tagsOut, err := clients.ServiceDiscovery.ListTagsForResource(ctx, &awssd.ListTagsForResourceInput{
				ResourceARN: ns.Arn,
			})
			if err == nil {
				pairs := make([][2]*string, 0, len(tagsOut.Tags))
				for _, t := range tagsOut.Tags {
					pairs = append(pairs, [2]*string{t.Key, t.Value})
				}
				cr.Spec.Tags = kvTagMap(pairs)
			}
			opts.Index.Add(aws.ToString(ns.Id), cr.Name)
			opts.Index.Add(aws.ToString(ns.Arn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportCloudMapServices(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awssd.NewListServicesPaginator(clients.ServiceDiscovery, &awssd.ListServicesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list Cloud Map services: %w", err)
		}
		for _, s := range page.Services {
			got, err := clients.ServiceDiscovery.GetService(ctx, &awssd.GetServiceInput{Id: s.Id})
			if err != nil || got.Service == nil {
				continue
			}
			svc := got.Service
			name := aws.ToString(svc.Name)
			cr := &awsv1alpha1.CloudMapService{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.CloudMapServiceSpec{
					Name:        name,
					Description: aws.ToString(svc.Description),
				},
			}
			nsID := aws.ToString(svc.NamespaceId)
			if crName, ok := opts.Index.Lookup(nsID); ok {
				cr.Spec.NamespaceRef = awsv1alpha1.CloudMapNamespaceRef{Name: crName}
			} else {
				cr.Spec.NamespaceRef = awsv1alpha1.CloudMapNamespaceRef{ID: nsID}
			}
			if dc := svc.DnsConfig; dc != nil && len(dc.DnsRecords) > 0 {
				cr.Spec.DnsConfig = &awsv1alpha1.CloudMapDnsConfig{
					RecordType:    string(dc.DnsRecords[0].Type),
					TTL:           aws.ToInt64(dc.DnsRecords[0].TTL),
					RoutingPolicy: string(dc.RoutingPolicy),
				}
			}
			if hc := svc.HealthCheckCustomConfig; hc != nil {
				cr.Spec.HealthCheckCustomConfig = &awsv1alpha1.CloudMapHealthCheckCustomConfig{
					FailureThreshold: aws.ToInt32(hc.FailureThreshold),
				}
			}
			tagsOut, err := clients.ServiceDiscovery.ListTagsForResource(ctx, &awssd.ListTagsForResourceInput{
				ResourceARN: svc.Arn,
			})
			if err == nil {
				pairs := make([][2]*string, 0, len(tagsOut.Tags))
				for _, t := range tagsOut.Tags {
					pairs = append(pairs, [2]*string{t.Key, t.Value})
				}
				cr.Spec.Tags = kvTagMap(pairs)
			}
			opts.Index.Add(aws.ToString(svc.Id), cr.Name)
			opts.Index.Add(aws.ToString(svc.Arn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}
