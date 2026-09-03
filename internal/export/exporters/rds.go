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
	awsrds "github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	awsclient "github.com/konfig-io/konfig-konector/internal/aws"
	"github.com/konfig-io/konfig-konector/internal/export"
)

func init() {
	// Groups first (30-31) so clusters/instances (33+) can reference them by CR name.
	export.Register(export.Exporter{Kind: "DBSubnetGroup", Service: "rds", Order: 30, Fn: exportDBSubnetGroups})
	export.Register(export.Exporter{Kind: "DBParameterGroup", Service: "rds", Order: 30, Fn: exportDBParameterGroups})
	export.Register(export.Exporter{Kind: "DBClusterParameterGroup", Service: "rds", Order: 30, Fn: exportDBClusterParameterGroups})
	export.Register(export.Exporter{Kind: "DBOptionGroup", Service: "rds", Order: 31, Fn: exportDBOptionGroups})
	export.Register(export.Exporter{Kind: "DBCluster", Service: "rds", Order: 33, Fn: exportDBClusters})
	export.Register(export.Exporter{Kind: "DBInstance", Service: "rds", Order: 34, Fn: exportDBInstances})
	export.Register(export.Exporter{Kind: "DBProxy", Service: "rds", Order: 35, Fn: exportDBProxies})
	export.Register(export.Exporter{Kind: "DBSnapshot", Service: "rds", Order: 36, Fn: exportDBSnapshots})
}

// rdsTagMap converts RDS tag slices to a spec tag map, dropping aws: tags.
func rdsTagMap(tags []rdstypes.Tag) map[string]string {
	if len(tags) == 0 {
		return nil
	}
	m := make(map[string]string, len(tags))
	for _, t := range tags {
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return export.TagMap(m)
}

// rdsResourceTags fetches tags by ARN; errors are tolerated (tags are optional).
func rdsResourceTags(ctx context.Context, clients *awsclient.Clients, arn string) map[string]string {
	if arn == "" {
		return nil
	}
	out, err := clients.RDS.ListTagsForResource(ctx, &awsrds.ListTagsForResourceInput{
		ResourceName: aws.String(arn),
	})
	if err != nil {
		return nil
	}
	return rdsTagMap(out.TagList)
}

// rdsSecurityGroupRefs converts VPC SG memberships into name-based refs when
// the SecurityGroup was exported in this run, raw-ID refs otherwise.
func rdsSecurityGroupRefs(memberships []rdstypes.VpcSecurityGroupMembership, opts *export.Options) []awsv1alpha1.SecurityGroupRef {
	var refs []awsv1alpha1.SecurityGroupRef
	for _, m := range memberships {
		id := aws.ToString(m.VpcSecurityGroupId)
		if id == "" {
			continue
		}
		if crName, ok := opts.Index.Lookup(id); ok {
			refs = append(refs, awsv1alpha1.SecurityGroupRef{Name: crName})
		} else {
			refs = append(refs, awsv1alpha1.SecurityGroupRef{ID: id})
		}
	}
	return refs
}

// rdsGroupCRRef resolves an RDS group name (subnet/parameter group) to the CR
// name exported earlier in this run; falls back to the derived CR name so the
// reference stays meaningful even if the group exporter was filtered out.
func rdsGroupCRRef(name string, opts *export.Options) string {
	if name == "" {
		return ""
	}
	if crName, ok := opts.Index.Lookup("rds-group/" + name); ok {
		return crName
	}
	// default.* / default:* groups are AWS-managed and never exported; leaving
	// the ref empty makes the operator fall back to the engine default.
	if strings.HasPrefix(name, "default.") || strings.HasPrefix(name, "default:") {
		return ""
	}
	return export.CRName(name)
}

func exportDBSubnetGroups(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsrds.NewDescribeDBSubnetGroupsPaginator(clients.RDS, &awsrds.DescribeDBSubnetGroupsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe db subnet groups: %w", err)
		}
		for _, g := range page.DBSubnetGroups {
			name := aws.ToString(g.DBSubnetGroupName)
			sg := &awsv1alpha1.DBSubnetGroup{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.DBSubnetGroupSpec{
					DBSubnetGroupName: name,
					Description:       aws.ToString(g.DBSubnetGroupDescription),
					Tags:              rdsResourceTags(ctx, clients, aws.ToString(g.DBSubnetGroupArn)),
				},
			}
			for _, sn := range g.Subnets {
				id := aws.ToString(sn.SubnetIdentifier)
				if crName, ok := opts.Index.Lookup(id); ok {
					sg.Spec.SubnetRefs = append(sg.Spec.SubnetRefs, awsv1alpha1.SubnetRef{Name: crName})
				} else {
					sg.Spec.SubnetRefs = append(sg.Spec.SubnetRefs, awsv1alpha1.SubnetRef{ID: id})
				}
			}
			opts.Index.Add(aws.ToString(g.DBSubnetGroupArn), sg.Name)
			opts.Index.Add("rds-group/"+name, sg.Name)
			objs = append(objs, sg)
		}
	}
	return objs, nil
}

// rdsUserParameters returns only user-modified parameters (Source=user) as
// spec overrides — engine defaults must not round-trip into the CR.
func rdsUserParameters(params []rdstypes.Parameter) []awsv1alpha1.DBParameter {
	var out []awsv1alpha1.DBParameter
	for _, p := range params {
		out = append(out, awsv1alpha1.DBParameter{
			ParameterName:  aws.ToString(p.ParameterName),
			ParameterValue: aws.ToString(p.ParameterValue),
			ApplyMethod:    string(p.ApplyMethod),
		})
	}
	return out
}

func exportDBParameterGroups(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsrds.NewDescribeDBParameterGroupsPaginator(clients.RDS, &awsrds.DescribeDBParameterGroupsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe db parameter groups: %w", err)
		}
		for _, g := range page.DBParameterGroups {
			name := aws.ToString(g.DBParameterGroupName)
			// default.* groups are AWS-managed and cannot be created/modified.
			if strings.HasPrefix(name, "default.") {
				continue
			}
			pg := &awsv1alpha1.DBParameterGroup{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.DBParameterGroupSpec{
					DBParameterGroupName:   name,
					DBParameterGroupFamily: aws.ToString(g.DBParameterGroupFamily),
					Description:            aws.ToString(g.Description),
					Tags:                   rdsResourceTags(ctx, clients, aws.ToString(g.DBParameterGroupArn)),
				},
			}
			// Only user-modified parameters (Source=user) belong in the spec.
			pp := awsrds.NewDescribeDBParametersPaginator(clients.RDS, &awsrds.DescribeDBParametersInput{
				DBParameterGroupName: g.DBParameterGroupName,
				Source:               aws.String("user"),
			})
			for pp.HasMorePages() {
				ppage, err := pp.NextPage(ctx)
				if err != nil {
					break // parameters are best-effort; keep the group itself
				}
				pg.Spec.Parameters = append(pg.Spec.Parameters, rdsUserParameters(ppage.Parameters)...)
			}
			opts.Index.Add(aws.ToString(g.DBParameterGroupArn), pg.Name)
			opts.Index.Add("rds-group/"+name, pg.Name)
			objs = append(objs, pg)
		}
	}
	return objs, nil
}

func exportDBClusterParameterGroups(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsrds.NewDescribeDBClusterParameterGroupsPaginator(clients.RDS, &awsrds.DescribeDBClusterParameterGroupsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe db cluster parameter groups: %w", err)
		}
		for _, g := range page.DBClusterParameterGroups {
			name := aws.ToString(g.DBClusterParameterGroupName)
			// default.* groups are AWS-managed and cannot be created/modified.
			if strings.HasPrefix(name, "default.") {
				continue
			}
			pg := &awsv1alpha1.DBClusterParameterGroup{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.DBClusterParameterGroupSpec{
					DBClusterParameterGroupName: name,
					DBParameterGroupFamily:      aws.ToString(g.DBParameterGroupFamily),
					Description:                 aws.ToString(g.Description),
					Tags:                        rdsResourceTags(ctx, clients, aws.ToString(g.DBClusterParameterGroupArn)),
				},
			}
			pp := awsrds.NewDescribeDBClusterParametersPaginator(clients.RDS, &awsrds.DescribeDBClusterParametersInput{
				DBClusterParameterGroupName: g.DBClusterParameterGroupName,
				Source:                      aws.String("user"),
			})
			for pp.HasMorePages() {
				ppage, err := pp.NextPage(ctx)
				if err != nil {
					break
				}
				pg.Spec.Parameters = append(pg.Spec.Parameters, rdsUserParameters(ppage.Parameters)...)
			}
			opts.Index.Add(aws.ToString(g.DBClusterParameterGroupArn), pg.Name)
			opts.Index.Add("rds-group/"+name, pg.Name)
			objs = append(objs, pg)
		}
	}
	return objs, nil
}

func exportDBOptionGroups(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsrds.NewDescribeOptionGroupsPaginator(clients.RDS, &awsrds.DescribeOptionGroupsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe option groups: %w", err)
		}
		for _, g := range page.OptionGroupsList {
			name := aws.ToString(g.OptionGroupName)
			// default:* option groups are AWS-managed.
			if strings.HasPrefix(name, "default:") {
				continue
			}
			og := &awsv1alpha1.DBOptionGroup{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.DBOptionGroupSpec{
					OptionGroupName:        name,
					OptionGroupDescription: aws.ToString(g.OptionGroupDescription),
					EngineName:             aws.ToString(g.EngineName),
					MajorEngineVersion:     aws.ToString(g.MajorEngineVersion),
					Tags:                   rdsResourceTags(ctx, clients, aws.ToString(g.OptionGroupArn)),
				},
			}
			for _, o := range g.Options {
				opt := awsv1alpha1.DBOptionGroupOption{
					OptionName: aws.ToString(o.OptionName),
					Port:       aws.ToInt32(o.Port),
				}
				for _, s := range o.OptionSettings {
					opt.OptionSettings = append(opt.OptionSettings, awsv1alpha1.DBOptionSetting{
						Name:  aws.ToString(s.Name),
						Value: aws.ToString(s.Value),
					})
				}
				og.Spec.Options = append(og.Spec.Options, opt)
			}
			opts.Index.Add(aws.ToString(g.OptionGroupArn), og.Name)
			opts.Index.Add("rds-group/"+name, og.Name)
			objs = append(objs, og)
		}
	}
	return objs, nil
}

// rdsPlaceholderPasswordRef is used wherever a spec requires a master password
// reference: RDS never exposes passwords through any API, so the export cannot
// populate a real Secret. The operator will need a real Secret named here
// (or the ref edited) before this CR is applied.
func rdsPlaceholderPasswordRef() awsv1alpha1.SecretRef {
	return awsv1alpha1.SecretRef{Name: "CHANGEME-db-password", Key: "password"}
}

func exportDBClusters(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsrds.NewDescribeDBClustersPaginator(clients.RDS, &awsrds.DescribeDBClustersInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe db clusters: %w", err)
		}
		for _, c := range page.DBClusters {
			engine := aws.ToString(c.Engine)
			// The DBCluster CRD models Aurora only; DescribeDBClusters also
			// returns Multi-AZ RDS clusters (engine mysql/postgres) and
			// DocumentDB/Neptune clusters, which don't fit the spec enum.
			if engine != "aurora-mysql" && engine != "aurora-postgresql" {
				continue
			}
			id := aws.ToString(c.DBClusterIdentifier)
			cl := &awsv1alpha1.DBCluster{
				ObjectMeta: export.ObjectMeta(id, opts),
				Spec: awsv1alpha1.DBClusterSpec{
					DBClusterIdentifier: id,
					Engine:              engine,
					EngineVersion:       aws.ToString(c.EngineVersion),
					MasterUsername:      aws.ToString(c.MasterUsername),
					// SECURITY: the master password is write-only in AWS and can
					// never be exported. Placeholder ref; see rdsPlaceholderPasswordRef.
					MasterUserPasswordRef:        rdsPlaceholderPasswordRef(),
					DBSubnetGroupRef:             rdsGroupCRRef(aws.ToString(c.DBSubnetGroup), opts),
					DBClusterParameterGroupRef:   rdsGroupCRRef(aws.ToString(c.DBClusterParameterGroup), opts),
					VPCSecurityGroupRefs:         rdsSecurityGroupRefs(c.VpcSecurityGroups, opts),
					BackupRetentionPeriod:        aws.ToInt32(c.BackupRetentionPeriod),
					StorageEncrypted:             aws.ToBool(c.StorageEncrypted),
					KMSKeyID:                     aws.ToString(c.KmsKeyId),
					DeletionProtection:           aws.ToBool(c.DeletionProtection),
					Tags:                         rdsTagMap(c.TagList),
					PreferredBackupWindow:        aws.ToString(c.PreferredBackupWindow),
					PreferredMaintenanceWindow:   aws.ToString(c.PreferredMaintenanceWindow),
					EnabledCloudwatchLogsExports: c.EnabledCloudwatchLogsExports,
					EngineMode:                   aws.ToString(c.EngineMode),
					NetworkType:                  aws.ToString(c.NetworkType),
					StorageType:                  aws.ToString(c.StorageType),
					Port:                         c.Port,
					BacktrackWindow:              c.BacktrackWindow,
				},
			}
			// SkipFinalSnapshot is a delete-time option that AWS does not
			// store or return; it cannot be exported and defaults to false.
			if c.IAMDatabaseAuthenticationEnabled != nil {
				cl.Spec.EnableIAMDatabaseAuthentication = c.IAMDatabaseAuthenticationEnabled
			}
			if c.CopyTagsToSnapshot != nil {
				cl.Spec.CopyTagsToSnapshot = c.CopyTagsToSnapshot
			}
			if c.AutoMinorVersionUpgrade != nil {
				cl.Spec.AutoMinorVersionUpgrade = c.AutoMinorVersionUpgrade
			}
			if c.HttpEndpointEnabled != nil {
				cl.Spec.EnableHttpEndpoint = c.HttpEndpointEnabled
			}
			if c.ServerlessV2ScalingConfiguration != nil {
				cl.Spec.ServerlessV2ScalingConfig = &awsv1alpha1.RDSServerlessV2ScalingConfig{
					MinCapacity: aws.ToFloat64(c.ServerlessV2ScalingConfiguration.MinCapacity),
					MaxCapacity: aws.ToFloat64(c.ServerlessV2ScalingConfiguration.MaxCapacity),
				}
			}
			if c.PerformanceInsightsEnabled != nil {
				cl.Spec.PerformanceInsightsEnabled = c.PerformanceInsightsEnabled
				cl.Spec.PerformanceInsightsKMSKeyID = aws.ToString(c.PerformanceInsightsKMSKeyId)
				cl.Spec.PerformanceInsightsRetentionPeriod = c.PerformanceInsightsRetentionPeriod
			}
			// Aurora standard storage reports AllocatedStorage=1 as a sentinel;
			// only meaningful (and only required) for I/O-Optimized clusters.
			if aws.ToInt32(c.AllocatedStorage) > 1 {
				cl.Spec.AllocatedStorage = c.AllocatedStorage
			}
			opts.Index.Add(aws.ToString(c.DBClusterArn), cl.Name)
			opts.Index.Add(id, cl.Name)
			objs = append(objs, cl)
		}
	}
	return objs, nil
}

func exportDBInstances(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsrds.NewDescribeDBInstancesPaginator(clients.RDS, &awsrds.DescribeDBInstancesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe db instances: %w", err)
		}
		for _, i := range page.DBInstances {
			id := aws.ToString(i.DBInstanceIdentifier)
			inst := &awsv1alpha1.DBInstance{
				ObjectMeta: export.ObjectMeta(id, opts),
				Spec: awsv1alpha1.DBInstanceSpec{
					DBInstanceIdentifier: id,
					DBInstanceClass:      aws.ToString(i.DBInstanceClass),
					Engine:               aws.ToString(i.Engine),
					EngineVersion:        aws.ToString(i.EngineVersion),
					MasterUsername:       aws.ToString(i.MasterUsername),
					// SECURITY: the master password is write-only in AWS and can
					// never be exported. The operator will need a real Secret named
					// here (or the ref edited) before this CR is applied.
					MasterUserPasswordRef:        rdsPlaceholderPasswordRef(),
					DBName:                       aws.ToString(i.DBName),
					AllocatedStorage:             aws.ToInt32(i.AllocatedStorage),
					StorageType:                  aws.ToString(i.StorageType),
					StorageEncrypted:             aws.ToBool(i.StorageEncrypted),
					KMSKeyID:                     aws.ToString(i.KmsKeyId),
					MultiAZ:                      aws.ToBool(i.MultiAZ),
					PubliclyAccessible:           aws.ToBool(i.PubliclyAccessible),
					VPCSecurityGroupRefs:         rdsSecurityGroupRefs(i.VpcSecurityGroups, opts),
					BackupRetentionPeriod:        aws.ToInt32(i.BackupRetentionPeriod),
					DeletionProtection:           aws.ToBool(i.DeletionProtection),
					Tags:                         rdsTagMap(i.TagList),
					MaxAllocatedStorage:          i.MaxAllocatedStorage,
					PreferredBackupWindow:        aws.ToString(i.PreferredBackupWindow),
					PreferredMaintenanceWindow:   aws.ToString(i.PreferredMaintenanceWindow),
					MonitoringRoleARN:            aws.ToString(i.MonitoringRoleArn),
					EnabledCloudwatchLogsExports: i.EnabledCloudwatchLogsExports,
					Iops:                         i.Iops,
					StorageThroughput:            i.StorageThroughput,
				},
			}
			// SkipFinalSnapshot is a delete-time option that AWS does not
			// store or return; it cannot be exported and defaults to false.
			if i.DBSubnetGroup != nil {
				inst.Spec.DBSubnetGroupRef = rdsGroupCRRef(aws.ToString(i.DBSubnetGroup.DBSubnetGroupName), opts)
			}
			for _, pg := range i.DBParameterGroups {
				inst.Spec.DBParameterGroupRef = rdsGroupCRRef(aws.ToString(pg.DBParameterGroupName), opts)
				break
			}
			if i.Endpoint != nil {
				inst.Spec.Port = aws.ToInt32(i.Endpoint.Port)
			}
			if i.PerformanceInsightsEnabled != nil {
				inst.Spec.EnablePerformanceInsights = i.PerformanceInsightsEnabled
				inst.Spec.PerformanceInsightsKMSKeyID = aws.ToString(i.PerformanceInsightsKMSKeyId)
				inst.Spec.PerformanceInsightsRetentionPeriod = i.PerformanceInsightsRetentionPeriod
			}
			if i.MonitoringInterval != nil {
				inst.Spec.MonitoringInterval = i.MonitoringInterval
			}
			if i.IAMDatabaseAuthenticationEnabled != nil {
				inst.Spec.EnableIAMDatabaseAuthentication = i.IAMDatabaseAuthenticationEnabled
			}
			if i.CopyTagsToSnapshot != nil {
				inst.Spec.CopyTagsToSnapshot = i.CopyTagsToSnapshot
			}
			if i.AutoMinorVersionUpgrade != nil {
				inst.Spec.AutoMinorVersionUpgrade = i.AutoMinorVersionUpgrade
			}
			opts.Index.Add(aws.ToString(i.DBInstanceArn), inst.Name)
			opts.Index.Add(id, inst.Name)
			objs = append(objs, inst)
		}
	}
	return objs, nil
}

func exportDBProxies(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsrds.NewDescribeDBProxiesPaginator(clients.RDS, &awsrds.DescribeDBProxiesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe db proxies: %w", err)
		}
		for _, pr := range page.DBProxies {
			name := aws.ToString(pr.DBProxyName)
			proxy := &awsv1alpha1.DBProxy{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.DBProxySpec{
					DBProxyName:         name,
					EngineFamily:        aws.ToString(pr.EngineFamily),
					RoleARN:             aws.ToString(pr.RoleArn),
					VPCSubnetIDs:        pr.VpcSubnetIds,
					VPCSecurityGroupIDs: pr.VpcSecurityGroupIds,
					RequireTLS:          aws.ToBool(pr.RequireTLS),
					IdleClientTimeout:   aws.ToInt32(pr.IdleClientTimeout),
					DebugLogging:        aws.ToBool(pr.DebugLogging),
					Tags:                rdsResourceTags(ctx, clients, aws.ToString(pr.DBProxyArn)),
				},
			}
			for _, a := range pr.Auth {
				// SECURITY: proxy auth points at Secrets Manager ARNs, never at
				// credential material; the secret contents are not exported.
				proxy.Spec.Auth = append(proxy.Spec.Auth, awsv1alpha1.DBProxyAuthConfig{
					Description: aws.ToString(a.Description),
					IAMAuth:     string(a.IAMAuth),
					SecretARN:   aws.ToString(a.SecretArn),
					AuthScheme:  string(a.AuthScheme),
				})
			}
			opts.Index.Add(aws.ToString(pr.DBProxyArn), proxy.Name)
			objs = append(objs, proxy)
		}
	}
	return objs, nil
}

func exportDBSnapshots(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	// Manual snapshots only: automated/system snapshots are lifecycle-managed
	// by RDS itself and cannot be created via CreateDBSnapshot.
	p := awsrds.NewDescribeDBSnapshotsPaginator(clients.RDS, &awsrds.DescribeDBSnapshotsInput{
		SnapshotType: aws.String("manual"),
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe db snapshots: %w", err)
		}
		for _, s := range page.DBSnapshots {
			id := aws.ToString(s.DBSnapshotIdentifier)
			snap := &awsv1alpha1.DBSnapshot{
				ObjectMeta: export.ObjectMeta(id, opts),
				Spec: awsv1alpha1.DBSnapshotSpec{
					DBInstanceIdentifier: aws.ToString(s.DBInstanceIdentifier),
					DBSnapshotIdentifier: id,
					Tags:                 rdsTagMap(s.TagList),
				},
			}
			opts.Index.Add(aws.ToString(s.DBSnapshotArn), snap.Name)
			objs = append(objs, snap)
		}
	}
	return objs, nil
}
