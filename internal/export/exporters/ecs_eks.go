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
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	awseks "github.com/aws/aws-sdk-go-v2/service/eks"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	awsclient "github.com/konfig-io/konfig-konector/internal/aws"
	"github.com/konfig-io/konfig-konector/internal/export"
)

func init() {
	// Clusters must be indexed before their children.
	export.Register(export.Exporter{Kind: "ECSCluster", Service: "ecs", Order: 50, Fn: exportECSClusters})
	export.Register(export.Exporter{Kind: "ECSCapacityProvider", Service: "ecs", Order: 51, Fn: exportECSCapacityProviders})
	export.Register(export.Exporter{Kind: "ECSTaskDefinition", Service: "ecs", Order: 52, Fn: exportECSTaskDefinitions})
	export.Register(export.Exporter{Kind: "ECSService", Service: "ecs", Order: 53, Fn: exportECSServices})
	export.Register(export.Exporter{Kind: "EKSCluster", Service: "eks", Order: 50, Fn: exportEKSClusters})
	export.Register(export.Exporter{Kind: "EKSNodeGroup", Service: "eks", Order: 55, Fn: exportEKSNodeGroups})
	export.Register(export.Exporter{Kind: "EKSAddon", Service: "eks", Order: 55, Fn: exportEKSAddons})
	export.Register(export.Exporter{Kind: "EKSFargateProfile", Service: "eks", Order: 55, Fn: exportEKSFargateProfiles})
	export.Register(export.Exporter{Kind: "EKSAccessEntry", Service: "eks", Order: 55, Fn: exportEKSAccessEntries})
	export.Register(export.Exporter{Kind: "EKSIdentityProviderConfig", Service: "eks", Order: 55, Fn: exportEKSIdentityProviderConfigs})
	export.Register(export.Exporter{Kind: "PodIdentityAssociation", Service: "eks", Order: 56, Fn: exportPodIdentityAssociations})
	// ECSScheduledTask is intentionally not exported: scheduled tasks live as
	// EventBridge rules + targets, which the messaging exporters already
	// cover as EventRule/EventTarget CRs. Exporting the same rule twice
	// (once as ECSScheduledTask, once as EventRule) would double-manage it.
}

// ecsTagMap converts ECS tag slices to a spec tag map, dropping aws: tags.
func ecsTagMap(tags []ecstypes.Tag) map[string]string {
	if len(tags) == 0 {
		return nil
	}
	m := make(map[string]string, len(tags))
	for _, t := range tags {
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return export.TagMap(m)
}

// chunkStrings splits a slice into batches of at most n for batched
// Describe* calls.
func chunkStrings(in []string, n int) [][]string {
	var out [][]string
	for len(in) > n {
		out = append(out, in[:n])
		in = in[n:]
	}
	if len(in) > 0 {
		out = append(out, in)
	}
	return out
}

func exportECSClusters(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var arns []string
	p := awsecs.NewListClustersPaginator(clients.ECS, &awsecs.ListClustersInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list ecs clusters: %w", err)
		}
		arns = append(arns, page.ClusterArns...)
	}

	var objs []client.Object
	for _, batch := range chunkStrings(arns, 100) {
		out, err := clients.ECS.DescribeClusters(ctx, &awsecs.DescribeClustersInput{
			Clusters: batch,
			Include:  []ecstypes.ClusterField{ecstypes.ClusterFieldTags, ecstypes.ClusterFieldSettings},
		})
		if err != nil {
			return nil, fmt.Errorf("describe ecs clusters: %w", err)
		}
		for _, c := range out.Clusters {
			if aws.ToString(c.Status) != "ACTIVE" {
				continue
			}
			name := aws.ToString(c.ClusterName)
			cr := &awsv1alpha1.ECSCluster{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.ECSClusterSpec{
					ClusterName:       name,
					CapacityProviders: c.CapacityProviders,
					Tags:              ecsTagMap(c.Tags),
				},
			}
			for _, s := range c.Settings {
				if s.Name == ecstypes.ClusterSettingNameContainerInsights {
					cr.Spec.ContainerInsights = aws.Bool(aws.ToString(s.Value) == "enabled")
				}
			}
			opts.Index.Add(aws.ToString(c.ClusterArn), cr.Name)
			opts.Index.Add(name, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportECSCapacityProviders(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	// DescribeCapacityProviders has no SDK paginator; page manually.
	input := &awsecs.DescribeCapacityProvidersInput{
		Include: []ecstypes.CapacityProviderField{ecstypes.CapacityProviderFieldTags},
	}
	for {
		out, err := clients.ECS.DescribeCapacityProviders(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("describe capacity providers: %w", err)
		}
		for _, cp := range out.CapacityProviders {
			name := aws.ToString(cp.Name)
			// FARGATE and FARGATE_SPOT are AWS-managed built-ins.
			if name == "FARGATE" || name == "FARGATE_SPOT" {
				continue
			}
			if cp.AutoScalingGroupProvider == nil {
				continue
			}
			cr := &awsv1alpha1.ECSCapacityProvider{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.ECSCapacityProviderSpec{
					Name: name,
					AutoScalingGroupProvider: awsv1alpha1.ECSAutoScalingGroupProvider{
						AutoScalingGroupARN:          aws.ToString(cp.AutoScalingGroupProvider.AutoScalingGroupArn),
						ManagedTerminationProtection: string(cp.AutoScalingGroupProvider.ManagedTerminationProtection),
					},
					Tags: ecsTagMap(cp.Tags),
				},
			}
			if ms := cp.AutoScalingGroupProvider.ManagedScaling; ms != nil {
				cr.Spec.AutoScalingGroupProvider.ManagedScaling = &awsv1alpha1.ECSManagedScaling{
					Status:                 string(ms.Status),
					TargetCapacity:         ms.TargetCapacity,
					MinimumScalingStepSize: ms.MinimumScalingStepSize,
					MaximumScalingStepSize: ms.MaximumScalingStepSize,
					InstanceWarmupPeriod:   ms.InstanceWarmupPeriod,
				}
			}
			opts.Index.Add(aws.ToString(cp.CapacityProviderArn), cr.Name)
			opts.Index.Add(name, cr.Name)
			objs = append(objs, cr)
		}
		if aws.ToString(out.NextToken) == "" {
			break
		}
		input.NextToken = out.NextToken
	}
	return objs, nil
}

func exportECSTaskDefinitions(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	// One CR per ACTIVE family; DescribeTaskDefinition on the bare family
	// name resolves to the latest ACTIVE revision.
	fp := awsecs.NewListTaskDefinitionFamiliesPaginator(clients.ECS, &awsecs.ListTaskDefinitionFamiliesInput{
		Status: ecstypes.TaskDefinitionFamilyStatusActive,
	})
	for fp.HasMorePages() {
		page, err := fp.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list task definition families: %w", err)
		}
		for _, family := range page.Families {
			out, err := clients.ECS.DescribeTaskDefinition(ctx, &awsecs.DescribeTaskDefinitionInput{
				TaskDefinition: aws.String(family),
				Include:        []ecstypes.TaskDefinitionField{ecstypes.TaskDefinitionFieldTags},
			})
			if err != nil || out.TaskDefinition == nil {
				continue
			}
			td := out.TaskDefinition
			cr := &awsv1alpha1.ECSTaskDefinition{
				ObjectMeta: export.ObjectMeta(family, opts),
				Spec: awsv1alpha1.ECSTaskDefinitionSpec{
					Family:      family,
					CPU:         aws.ToString(td.Cpu),
					Memory:      aws.ToString(td.Memory),
					NetworkMode: string(td.NetworkMode),
					Tags:        ecsTagMap(out.Tags),
				},
			}
			if execArn := aws.ToString(td.ExecutionRoleArn); execArn != "" {
				if crName, ok := opts.Index.Lookup(execArn); ok {
					cr.Spec.ExecutionRoleRef = &awsv1alpha1.RoleRef{Name: crName}
				} else {
					cr.Spec.ExecutionRoleArn = execArn
				}
			}
			if taskArn := aws.ToString(td.TaskRoleArn); taskArn != "" {
				if crName, ok := opts.Index.Lookup(taskArn); ok {
					cr.Spec.TaskRoleRef = &awsv1alpha1.RoleRef{Name: crName}
				} else {
					cr.Spec.TaskRoleArn = taskArn
				}
			}
			for _, cd := range td.ContainerDefinitions {
				spec := awsv1alpha1.ECSContainerDefinition{
					Name:       aws.ToString(cd.Name),
					Image:      aws.ToString(cd.Image),
					Essential:  cd.Essential,
					Command:    cd.Command,
					EntryPoint: cd.EntryPoint,
				}
				if cd.Cpu != 0 {
					spec.CPU = aws.Int32(cd.Cpu)
				}
				spec.Memory = cd.Memory
				for _, pm := range cd.PortMappings {
					if pm.ContainerPort == nil {
						continue
					}
					spec.PortMappings = append(spec.PortMappings, awsv1alpha1.ECSPortMapping{
						ContainerPort: aws.ToInt32(pm.ContainerPort),
						Protocol:      string(pm.Protocol),
					})
				}
				if len(cd.Environment) > 0 {
					spec.Environment = make(map[string]string, len(cd.Environment))
					for _, kv := range cd.Environment {
						spec.Environment[aws.ToString(kv.Name)] = aws.ToString(kv.Value)
					}
				}
				if lc := cd.LogConfiguration; lc != nil && lc.LogDriver == ecstypes.LogDriverAwslogs {
					spec.LogGroup = lc.Options["awslogs-group"]
				}
				cr.Spec.ContainerDefinitions = append(cr.Spec.ContainerDefinitions, spec)
			}
			opts.Index.Add(aws.ToString(td.TaskDefinitionArn), cr.Name)
			opts.Index.Add(family, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportECSServices(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	cp := awsecs.NewListClustersPaginator(clients.ECS, &awsecs.ListClustersInput{})
	for cp.HasMorePages() {
		cpage, err := cp.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list ecs clusters: %w", err)
		}
		for _, clusterArn := range cpage.ClusterArns {
			var svcArns []string
			sp := awsecs.NewListServicesPaginator(clients.ECS, &awsecs.ListServicesInput{
				Cluster: aws.String(clusterArn),
			})
			for sp.HasMorePages() {
				spage, err := sp.NextPage(ctx)
				if err != nil {
					// Per-cluster failure: keep exporting other clusters.
					svcArns = nil
					break
				}
				svcArns = append(svcArns, spage.ServiceArns...)
			}
			clusterName := clusterArn[strings.LastIndex(clusterArn, "/")+1:]
			for _, batch := range chunkStrings(svcArns, 10) {
				out, err := clients.ECS.DescribeServices(ctx, &awsecs.DescribeServicesInput{
					Cluster:  aws.String(clusterArn),
					Services: batch,
					Include:  []ecstypes.ServiceField{ecstypes.ServiceFieldTags},
				})
				if err != nil {
					continue
				}
				for _, s := range out.Services {
					if aws.ToString(s.Status) != "ACTIVE" {
						continue
					}
					svcName := aws.ToString(s.ServiceName)
					cr := &awsv1alpha1.ECSService{
						ObjectMeta: export.ObjectMeta(clusterName+"-"+svcName, opts),
						Spec: awsv1alpha1.ECSServiceSpec{
							ServiceName:                   svcName,
							DesiredCount:                  s.DesiredCount,
							LaunchType:                    string(s.LaunchType),
							HealthCheckGracePeriodSeconds: s.HealthCheckGracePeriodSeconds,
							Tags:                          ecsTagMap(s.Tags),
						},
					}
					if s.EnableExecuteCommand {
						cr.Spec.EnableExecuteCommand = aws.Bool(true)
					}
					if crName, ok := opts.Index.Lookup(aws.ToString(s.ClusterArn)); ok {
						cr.Spec.ClusterRef = &awsv1alpha1.ECSClusterRef{Name: crName}
					} else {
						cr.Spec.ClusterName = clusterName
					}
					tdArn := aws.ToString(s.TaskDefinition)
					if crName, ok := opts.Index.Lookup(tdArn); ok {
						cr.Spec.TaskDefinitionRef = &awsv1alpha1.ECSTaskDefinitionRef{Name: crName}
					} else {
						cr.Spec.TaskDefinitionArn = tdArn
					}
					if nc := s.NetworkConfiguration; nc != nil && nc.AwsvpcConfiguration != nil {
						vc := nc.AwsvpcConfiguration
						netCfg := &awsv1alpha1.ECSNetworkConfiguration{
							AssignPublicIP: string(vc.AssignPublicIp),
						}
						for _, sn := range vc.Subnets {
							if crName, ok := opts.Index.Lookup(sn); ok {
								netCfg.SubnetRefs = append(netCfg.SubnetRefs, awsv1alpha1.SubnetRef{Name: crName})
							} else {
								netCfg.SubnetRefs = append(netCfg.SubnetRefs, awsv1alpha1.SubnetRef{ID: sn})
							}
						}
						for _, sg := range vc.SecurityGroups {
							if crName, ok := opts.Index.Lookup(sg); ok {
								netCfg.SecurityGroupRefs = append(netCfg.SecurityGroupRefs, awsv1alpha1.SecurityGroupRef{Name: crName})
							} else {
								netCfg.SecurityGroupRefs = append(netCfg.SecurityGroupRefs, awsv1alpha1.SecurityGroupRef{ID: sg})
							}
						}
						cr.Spec.NetworkConfiguration = netCfg
					}
					for _, lb := range s.LoadBalancers {
						if aws.ToString(lb.TargetGroupArn) == "" {
							continue
						}
						cr.Spec.LoadBalancers = append(cr.Spec.LoadBalancers, awsv1alpha1.ECSLoadBalancer{
							TargetGroupArn: aws.ToString(lb.TargetGroupArn),
							ContainerName:  aws.ToString(lb.ContainerName),
							ContainerPort:  aws.ToInt32(lb.ContainerPort),
						})
					}
					opts.Index.Add(aws.ToString(s.ServiceArn), cr.Name)
					objs = append(objs, cr)
				}
			}
		}
	}
	return objs, nil
}

func exportEKSClusters(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awseks.NewListClustersPaginator(clients.EKS, &awseks.ListClustersInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list eks clusters: %w", err)
		}
		for _, name := range page.Clusters {
			out, err := clients.EKS.DescribeCluster(ctx, &awseks.DescribeClusterInput{Name: aws.String(name)})
			if err != nil || out.Cluster == nil {
				continue
			}
			c := out.Cluster
			cr := &awsv1alpha1.EKSCluster{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.EKSClusterSpec{
					ClusterName: name,
					Version:     aws.ToString(c.Version),
					Tags:        export.TagMap(c.Tags),
				},
			}
			roleArn := aws.ToString(c.RoleArn)
			if crName, ok := opts.Index.Lookup(roleArn); ok {
				cr.Spec.RoleRef = &awsv1alpha1.RoleRef{Name: crName}
			} else {
				cr.Spec.RoleArn = roleArn
			}
			if vc := c.ResourcesVpcConfig; vc != nil {
				spec := awsv1alpha1.EKSClusterVpcConfig{
					EndpointPublicAccess:  aws.Bool(vc.EndpointPublicAccess),
					EndpointPrivateAccess: aws.Bool(vc.EndpointPrivateAccess),
					PublicAccessCidrs:     vc.PublicAccessCidrs,
				}
				for _, sn := range vc.SubnetIds {
					if crName, ok := opts.Index.Lookup(sn); ok {
						spec.SubnetRefs = append(spec.SubnetRefs, awsv1alpha1.SubnetRef{Name: crName})
					} else {
						spec.SubnetRefs = append(spec.SubnetRefs, awsv1alpha1.SubnetRef{ID: sn})
					}
				}
				for _, sg := range vc.SecurityGroupIds {
					if crName, ok := opts.Index.Lookup(sg); ok {
						spec.SecurityGroupRefs = append(spec.SecurityGroupRefs, awsv1alpha1.SecurityGroupRef{Name: crName})
					} else {
						spec.SecurityGroupRefs = append(spec.SecurityGroupRefs, awsv1alpha1.SecurityGroupRef{ID: sg})
					}
				}
				cr.Spec.ResourcesVpcConfig = spec
			}
			if c.Logging != nil {
				var enabled []string
				for _, ls := range c.Logging.ClusterLogging {
					if aws.ToBool(ls.Enabled) {
						for _, t := range ls.Types {
							enabled = append(enabled, string(t))
						}
					}
				}
				if len(enabled) > 0 {
					cr.Spec.Logging = &awsv1alpha1.EKSClusterLogging{EnabledTypes: enabled}
				}
			}
			// CRD models a single encryption config; AWS returns a list but
			// only ever one entry in practice.
			if len(c.EncryptionConfig) > 0 {
				ec := c.EncryptionConfig[0]
				spec := &awsv1alpha1.EKSClusterEncryptionConfig{Resources: ec.Resources}
				if ec.Provider != nil {
					spec.ProviderKeyArn = aws.ToString(ec.Provider.KeyArn)
				}
				cr.Spec.EncryptionConfig = spec
			}
			if ac := c.AccessConfig; ac != nil {
				cr.Spec.AccessConfig = &awsv1alpha1.EKSClusterAccessConfig{
					AuthenticationMode:                      string(ac.AuthenticationMode),
					BootstrapClusterCreatorAdminPermissions: ac.BootstrapClusterCreatorAdminPermissions,
				}
			}
			if knc := c.KubernetesNetworkConfig; knc != nil {
				cr.Spec.KubernetesNetworkConfig = &awsv1alpha1.EKSKubernetesNetworkConfig{
					ServiceIPv4CIDR: aws.ToString(knc.ServiceIpv4Cidr),
					IPFamily:        string(knc.IpFamily),
				}
			}
			if up := c.UpgradePolicy; up != nil && up.SupportType != "" {
				cr.Spec.UpgradePolicy = &awsv1alpha1.EKSUpgradePolicy{SupportType: string(up.SupportType)}
			}
			opts.Index.Add(aws.ToString(c.Arn), cr.Name)
			opts.Index.Add(name, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

// eksClusterNames lists all EKS cluster names (shared by per-cluster child
// exporters).
func eksClusterNames(ctx context.Context, clients *awsclient.Clients) ([]string, error) {
	var names []string
	p := awseks.NewListClustersPaginator(clients.EKS, &awseks.ListClustersInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list eks clusters: %w", err)
		}
		names = append(names, page.Clusters...)
	}
	return names, nil
}

// eksClusterRefFor resolves a cluster name to a CR-name ref when exported.
func eksClusterRefFor(clusterName string, opts *export.Options) *awsv1alpha1.EKSClusterRef {
	if crName, ok := opts.Index.Lookup(clusterName); ok {
		return &awsv1alpha1.EKSClusterRef{Name: crName}
	}
	return nil
}

func exportEKSNodeGroups(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	clusters, err := eksClusterNames(ctx, clients)
	if err != nil {
		return nil, err
	}
	var objs []client.Object
	for _, cluster := range clusters {
		p := awseks.NewListNodegroupsPaginator(clients.EKS, &awseks.ListNodegroupsInput{ClusterName: aws.String(cluster)})
		for p.HasMorePages() {
			page, err := p.NextPage(ctx)
			if err != nil {
				break
			}
			for _, ngName := range page.Nodegroups {
				out, err := clients.EKS.DescribeNodegroup(ctx, &awseks.DescribeNodegroupInput{
					ClusterName: aws.String(cluster), NodegroupName: aws.String(ngName),
				})
				if err != nil || out.Nodegroup == nil {
					continue
				}
				ng := out.Nodegroup
				cr := &awsv1alpha1.EKSNodeGroup{
					ObjectMeta: export.ObjectMeta(cluster+"-"+ngName, opts),
					Spec: awsv1alpha1.EKSNodeGroupSpec{
						NodegroupName:  ngName,
						InstanceTypes:  ng.InstanceTypes,
						AmiType:        string(ng.AmiType),
						CapacityType:   string(ng.CapacityType),
						DiskSize:       ng.DiskSize,
						Labels:         ng.Labels,
						ReleaseVersion: aws.ToString(ng.ReleaseVersion),
						Version:        aws.ToString(ng.Version),
						Tags:           export.TagMap(ng.Tags),
					},
				}
				if ref := eksClusterRefFor(cluster, opts); ref != nil {
					cr.Spec.ClusterRef = ref
				} else {
					cr.Spec.ClusterName = cluster
				}
				nodeRole := aws.ToString(ng.NodeRole)
				if crName, ok := opts.Index.Lookup(nodeRole); ok {
					cr.Spec.NodeRoleRef = &awsv1alpha1.RoleRef{Name: crName}
				} else {
					cr.Spec.NodeRoleArn = nodeRole
				}
				for _, sn := range ng.Subnets {
					if crName, ok := opts.Index.Lookup(sn); ok {
						cr.Spec.SubnetRefs = append(cr.Spec.SubnetRefs, awsv1alpha1.SubnetRef{Name: crName})
					} else {
						cr.Spec.SubnetRefs = append(cr.Spec.SubnetRefs, awsv1alpha1.SubnetRef{ID: sn})
					}
				}
				if sc := ng.ScalingConfig; sc != nil {
					cr.Spec.ScalingConfig = awsv1alpha1.EKSNodeGroupScalingConfig{
						MinSize:     aws.ToInt32(sc.MinSize),
						MaxSize:     aws.ToInt32(sc.MaxSize),
						DesiredSize: aws.ToInt32(sc.DesiredSize),
					}
				}
				for _, t := range ng.Taints {
					cr.Spec.Taints = append(cr.Spec.Taints, awsv1alpha1.EKSTaint{
						Key:    aws.ToString(t.Key),
						Value:  aws.ToString(t.Value),
						Effect: string(t.Effect),
					})
				}
				if uc := ng.UpdateConfig; uc != nil {
					cr.Spec.UpdateConfig = &awsv1alpha1.EKSNodeGroupUpdateConfig{
						MaxUnavailable:           uc.MaxUnavailable,
						MaxUnavailablePercentage: uc.MaxUnavailablePercentage,
					}
				}
				if lt := ng.LaunchTemplate; lt != nil {
					spec := &awsv1alpha1.EKSNodeGroupLaunchTemplate{Version: aws.ToString(lt.Version)}
					ltID := aws.ToString(lt.Id)
					if crName, ok := opts.Index.Lookup(ltID); ok {
						spec.Name = crName
					} else {
						spec.ID = ltID
					}
					cr.Spec.LaunchTemplate = spec
				}
				if ra := ng.RemoteAccess; ra != nil {
					spec := &awsv1alpha1.EKSNodeGroupRemoteAccess{EC2SshKey: aws.ToString(ra.Ec2SshKey)}
					for _, sg := range ra.SourceSecurityGroups {
						if crName, ok := opts.Index.Lookup(sg); ok {
							spec.SourceSecurityGroupRefs = append(spec.SourceSecurityGroupRefs, awsv1alpha1.SecurityGroupRef{Name: crName})
						} else {
							spec.SourceSecurityGroupRefs = append(spec.SourceSecurityGroupRefs, awsv1alpha1.SecurityGroupRef{ID: sg})
						}
					}
					cr.Spec.RemoteAccess = spec
				}
				opts.Index.Add(aws.ToString(ng.NodegroupArn), cr.Name)
				objs = append(objs, cr)
			}
		}
	}
	return objs, nil
}

func exportEKSAddons(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	clusters, err := eksClusterNames(ctx, clients)
	if err != nil {
		return nil, err
	}
	var objs []client.Object
	for _, cluster := range clusters {
		p := awseks.NewListAddonsPaginator(clients.EKS, &awseks.ListAddonsInput{ClusterName: aws.String(cluster)})
		for p.HasMorePages() {
			page, err := p.NextPage(ctx)
			if err != nil {
				break
			}
			for _, addonName := range page.Addons {
				out, err := clients.EKS.DescribeAddon(ctx, &awseks.DescribeAddonInput{
					ClusterName: aws.String(cluster), AddonName: aws.String(addonName),
				})
				if err != nil || out.Addon == nil {
					continue
				}
				a := out.Addon
				cr := &awsv1alpha1.EKSAddon{
					ObjectMeta: export.ObjectMeta(cluster+"-"+addonName, opts),
					Spec: awsv1alpha1.EKSAddonSpec{
						AddonName:           addonName,
						AddonVersion:        aws.ToString(a.AddonVersion),
						ConfigurationValues: aws.ToString(a.ConfigurationValues),
						Tags:                export.TagMap(a.Tags),
					},
				}
				if ref := eksClusterRefFor(cluster, opts); ref != nil {
					cr.Spec.ClusterRef = ref
				} else {
					cr.Spec.ClusterName = cluster
				}
				if saRole := aws.ToString(a.ServiceAccountRoleArn); saRole != "" {
					if crName, ok := opts.Index.Lookup(saRole); ok {
						cr.Spec.ServiceAccountRoleRef = &awsv1alpha1.RoleRef{Name: crName}
					} else {
						cr.Spec.ServiceAccountRoleArn = saRole
					}
				}
				opts.Index.Add(aws.ToString(a.AddonArn), cr.Name)
				objs = append(objs, cr)
			}
		}
	}
	return objs, nil
}

func exportEKSFargateProfiles(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	clusters, err := eksClusterNames(ctx, clients)
	if err != nil {
		return nil, err
	}
	var objs []client.Object
	for _, cluster := range clusters {
		p := awseks.NewListFargateProfilesPaginator(clients.EKS, &awseks.ListFargateProfilesInput{ClusterName: aws.String(cluster)})
		for p.HasMorePages() {
			page, err := p.NextPage(ctx)
			if err != nil {
				break
			}
			for _, fpName := range page.FargateProfileNames {
				out, err := clients.EKS.DescribeFargateProfile(ctx, &awseks.DescribeFargateProfileInput{
					ClusterName: aws.String(cluster), FargateProfileName: aws.String(fpName),
				})
				if err != nil || out.FargateProfile == nil {
					continue
				}
				fp := out.FargateProfile
				cr := &awsv1alpha1.EKSFargateProfile{
					ObjectMeta: export.ObjectMeta(cluster+"-"+fpName, opts),
					Spec: awsv1alpha1.EKSFargateProfileSpec{
						FargateProfileName: fpName,
						Tags:               export.TagMap(fp.Tags),
					},
				}
				if ref := eksClusterRefFor(cluster, opts); ref != nil {
					cr.Spec.ClusterRef = ref
				} else {
					cr.Spec.ClusterName = cluster
				}
				podRole := aws.ToString(fp.PodExecutionRoleArn)
				if crName, ok := opts.Index.Lookup(podRole); ok {
					cr.Spec.PodExecutionRoleRef = &awsv1alpha1.RoleRef{Name: crName}
				} else {
					cr.Spec.PodExecutionRoleArn = podRole
				}
				for _, sn := range fp.Subnets {
					if crName, ok := opts.Index.Lookup(sn); ok {
						cr.Spec.SubnetRefs = append(cr.Spec.SubnetRefs, awsv1alpha1.SubnetRef{Name: crName})
					} else {
						cr.Spec.SubnetRefs = append(cr.Spec.SubnetRefs, awsv1alpha1.SubnetRef{ID: sn})
					}
				}
				for _, sel := range fp.Selectors {
					cr.Spec.Selectors = append(cr.Spec.Selectors, awsv1alpha1.EKSFargateSelector{
						Namespace: aws.ToString(sel.Namespace),
						Labels:    sel.Labels,
					})
				}
				opts.Index.Add(aws.ToString(fp.FargateProfileArn), cr.Name)
				objs = append(objs, cr)
			}
		}
	}
	return objs, nil
}

func exportEKSAccessEntries(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	clusters, err := eksClusterNames(ctx, clients)
	if err != nil {
		return nil, err
	}
	var objs []client.Object
	for _, cluster := range clusters {
		p := awseks.NewListAccessEntriesPaginator(clients.EKS, &awseks.ListAccessEntriesInput{ClusterName: aws.String(cluster)})
		for p.HasMorePages() {
			page, err := p.NextPage(ctx)
			if err != nil {
				// Clusters in CONFIG_MAP auth mode reject access entry APIs.
				break
			}
			for _, principalArn := range page.AccessEntries {
				out, err := clients.EKS.DescribeAccessEntry(ctx, &awseks.DescribeAccessEntryInput{
					ClusterName: aws.String(cluster), PrincipalArn: aws.String(principalArn),
				})
				if err != nil || out.AccessEntry == nil {
					continue
				}
				ae := out.AccessEntry
				principalName := principalArn[strings.LastIndex(principalArn, "/")+1:]
				cr := &awsv1alpha1.EKSAccessEntry{
					ObjectMeta: export.ObjectMeta(cluster+"-"+principalName, opts),
					Spec: awsv1alpha1.EKSAccessEntrySpec{
						Type:             aws.ToString(ae.Type),
						KubernetesGroups: ae.KubernetesGroups,
						Username:         aws.ToString(ae.Username),
						Tags:             export.TagMap(ae.Tags),
					},
				}
				if ref := eksClusterRefFor(cluster, opts); ref != nil {
					cr.Spec.ClusterRef = ref
				} else {
					cr.Spec.ClusterName = cluster
				}
				if crName, ok := opts.Index.Lookup(principalArn); ok {
					cr.Spec.PrincipalRef = &awsv1alpha1.RoleRef{Name: crName}
				} else {
					cr.Spec.PrincipalArn = principalArn
				}
				// Associated policies are a separate paginated list call.
				ap := awseks.NewListAssociatedAccessPoliciesPaginator(clients.EKS, &awseks.ListAssociatedAccessPoliciesInput{
					ClusterName: aws.String(cluster), PrincipalArn: aws.String(principalArn),
				})
				for ap.HasMorePages() {
					apage, err := ap.NextPage(ctx)
					if err != nil {
						break
					}
					for _, pol := range apage.AssociatedAccessPolicies {
						assoc := awsv1alpha1.EKSAccessPolicyAssociation{
							PolicyArn: aws.ToString(pol.PolicyArn),
						}
						if pol.AccessScope != nil {
							assoc.AccessScope = awsv1alpha1.EKSAccessScope{
								Type:       string(pol.AccessScope.Type),
								Namespaces: pol.AccessScope.Namespaces,
							}
						}
						cr.Spec.AccessPolicies = append(cr.Spec.AccessPolicies, assoc)
					}
				}
				opts.Index.Add(aws.ToString(ae.AccessEntryArn), cr.Name)
				objs = append(objs, cr)
			}
		}
	}
	return objs, nil
}

func exportEKSIdentityProviderConfigs(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	clusters, err := eksClusterNames(ctx, clients)
	if err != nil {
		return nil, err
	}
	var objs []client.Object
	for _, cluster := range clusters {
		p := awseks.NewListIdentityProviderConfigsPaginator(clients.EKS, &awseks.ListIdentityProviderConfigsInput{ClusterName: aws.String(cluster)})
		for p.HasMorePages() {
			page, err := p.NextPage(ctx)
			if err != nil {
				break
			}
			for _, ipc := range page.IdentityProviderConfigs {
				out, err := clients.EKS.DescribeIdentityProviderConfig(ctx, &awseks.DescribeIdentityProviderConfigInput{
					ClusterName:            aws.String(cluster),
					IdentityProviderConfig: &ipc,
				})
				if err != nil || out.IdentityProviderConfig == nil || out.IdentityProviderConfig.Oidc == nil {
					continue
				}
				oidc := out.IdentityProviderConfig.Oidc
				name := aws.ToString(oidc.IdentityProviderConfigName)
				cr := &awsv1alpha1.EKSIdentityProviderConfig{
					ObjectMeta: export.ObjectMeta(cluster+"-"+name, opts),
					Spec: awsv1alpha1.EKSIdentityProviderConfigSpec{
						ClusterName:                cluster,
						IdentityProviderConfigName: name,
						OIDC: awsv1alpha1.EKSOIDCConfig{
							ClientID:       aws.ToString(oidc.ClientId),
							IssuerURL:      aws.ToString(oidc.IssuerUrl),
							UsernameClaim:  aws.ToString(oidc.UsernameClaim),
							UsernamePrefix: aws.ToString(oidc.UsernamePrefix),
							GroupsClaim:    aws.ToString(oidc.GroupsClaim),
							GroupsPrefix:   aws.ToString(oidc.GroupsPrefix),
							RequiredClaims: oidc.RequiredClaims,
						},
						Tags: export.TagMap(oidc.Tags),
					},
				}
				opts.Index.Add(aws.ToString(oidc.IdentityProviderConfigArn), cr.Name)
				objs = append(objs, cr)
			}
		}
	}
	return objs, nil
}

func exportPodIdentityAssociations(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	clusters, err := eksClusterNames(ctx, clients)
	if err != nil {
		return nil, err
	}
	var objs []client.Object
	for _, cluster := range clusters {
		p := awseks.NewListPodIdentityAssociationsPaginator(clients.EKS, &awseks.ListPodIdentityAssociationsInput{ClusterName: aws.String(cluster)})
		for p.HasMorePages() {
			page, err := p.NextPage(ctx)
			if err != nil {
				break
			}
			for _, summary := range page.Associations {
				out, err := clients.EKS.DescribePodIdentityAssociation(ctx, &awseks.DescribePodIdentityAssociationInput{
					ClusterName: aws.String(cluster), AssociationId: summary.AssociationId,
				})
				if err != nil || out.Association == nil {
					continue
				}
				a := out.Association
				ns := aws.ToString(a.Namespace)
				sa := aws.ToString(a.ServiceAccount)
				cr := &awsv1alpha1.PodIdentityAssociation{
					ObjectMeta: export.ObjectMeta(cluster+"-"+ns+"-"+sa, opts),
					Spec: awsv1alpha1.PodIdentityAssociationSpec{
						ClusterName:        cluster,
						TargetNamespace:    ns,
						ServiceAccountName: sa,
						Tags:               export.TagMap(a.Tags),
					},
				}
				roleArn := aws.ToString(a.RoleArn)
				if crName, ok := opts.Index.Lookup(roleArn); ok {
					cr.Spec.RoleRef = awsv1alpha1.RoleRef{Name: crName}
				} else {
					cr.Spec.RoleRef = awsv1alpha1.RoleRef{ARN: roleArn}
				}
				opts.Index.Add(aws.ToString(a.AssociationArn), cr.Name)
				objs = append(objs, cr)
			}
		}
	}
	return objs, nil
}
