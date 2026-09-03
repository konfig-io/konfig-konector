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
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsdax "github.com/aws/aws-sdk-go-v2/service/dax"
	awsdynamodb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	awselasticache "github.com/aws/aws-sdk-go-v2/service/elasticache"
	ectypes "github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	awsmemorydb "github.com/aws/aws-sdk-go-v2/service/memorydb"
	mdbtypes "github.com/aws/aws-sdk-go-v2/service/memorydb/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	awsclient "github.com/konfig-io/konfig-konector/internal/aws"
	"github.com/konfig-io/konfig-konector/internal/export"
)

func init() {
	// Tables/groups first, then table policies and clusters that reference them.
	export.Register(export.Exporter{Kind: "DynamoDBTable", Service: "dynamodb", Order: 30, Fn: exportDynamoDBTables})
	export.Register(export.Exporter{Kind: "DynamoDBGlobalTable", Service: "dynamodb", Order: 32, Fn: exportDynamoDBGlobalTables})
	export.Register(export.Exporter{Kind: "DynamoDBTablePolicy", Service: "dynamodb", Order: 33, Fn: exportDynamoDBTablePolicies})
	export.Register(export.Exporter{Kind: "ElastiCacheSubnetGroup", Service: "elasticache", Order: 30, Fn: exportElastiCacheSubnetGroups})
	export.Register(export.Exporter{Kind: "ElastiCacheParameterGroup", Service: "elasticache", Order: 30, Fn: exportElastiCacheParameterGroups})
	export.Register(export.Exporter{Kind: "ElastiCacheReplicationGroup", Service: "elasticache", Order: 33, Fn: exportElastiCacheReplicationGroups})
	export.Register(export.Exporter{Kind: "ElastiCacheServerlessCache", Service: "elasticache", Order: 33, Fn: exportElastiCacheServerlessCaches})
	export.Register(export.Exporter{Kind: "MemoryDBCluster", Service: "memorydb", Order: 33, Fn: exportMemoryDBClusters})
	export.Register(export.Exporter{Kind: "DAXCluster", Service: "dax", Order: 33, Fn: exportDAXClusters})
}

// ddbTagMap fetches DynamoDB resource tags by ARN; errors are tolerated.
func ddbTagMap(ctx context.Context, clients *awsclient.Clients, arn string) map[string]string {
	if arn == "" {
		return nil
	}
	m := map[string]string{}
	var next *string
	for {
		out, err := clients.DynamoDB.ListTagsOfResource(ctx, &awsdynamodb.ListTagsOfResourceInput{
			ResourceArn: aws.String(arn),
			NextToken:   next,
		})
		if err != nil {
			return nil
		}
		for _, t := range out.Tags {
			m[aws.ToString(t.Key)] = aws.ToString(t.Value)
		}
		if out.NextToken == nil {
			break
		}
		next = out.NextToken
	}
	return export.TagMap(m)
}

func exportDynamoDBTables(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsdynamodb.NewListTablesPaginator(clients.DynamoDB, &awsdynamodb.ListTablesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list tables: %w", err)
		}
		for _, name := range page.TableNames {
			desc, err := clients.DynamoDB.DescribeTable(ctx, &awsdynamodb.DescribeTableInput{
				TableName: aws.String(name),
			})
			if err != nil || desc.Table == nil {
				continue // per-table errors: skip and keep exporting
			}
			t := desc.Table
			// Only ACTIVE tables round-trip cleanly into a spec.
			if t.TableStatus != ddbtypes.TableStatusActive {
				continue
			}
			tbl := &awsv1alpha1.DynamoDBTable{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.DynamoDBTableSpec{
					TableName: name,
					Tags:      ddbTagMap(ctx, clients, aws.ToString(t.TableArn)),
				},
			}
			for _, ad := range t.AttributeDefinitions {
				tbl.Spec.AttributeDefinitions = append(tbl.Spec.AttributeDefinitions, awsv1alpha1.DynamoDBAttributeDefinition{
					AttributeName: aws.ToString(ad.AttributeName),
					AttributeType: string(ad.AttributeType),
				})
			}
			for _, ks := range t.KeySchema {
				tbl.Spec.KeySchema = append(tbl.Spec.KeySchema, awsv1alpha1.DynamoDBKeySchema{
					AttributeName: aws.ToString(ks.AttributeName),
					KeyType:       string(ks.KeyType),
				})
			}
			// Billing mode: tables created before on-demand existed have no
			// BillingModeSummary — those are PROVISIONED.
			if t.BillingModeSummary != nil {
				tbl.Spec.BillingMode = string(t.BillingModeSummary.BillingMode)
			} else {
				tbl.Spec.BillingMode = string(ddbtypes.BillingModeProvisioned)
			}
			if tbl.Spec.BillingMode == string(ddbtypes.BillingModeProvisioned) && t.ProvisionedThroughput != nil {
				tbl.Spec.ProvisionedThroughput = &awsv1alpha1.DynamoDBProvisionedThroughput{
					ReadCapacityUnits:  aws.ToInt64(t.ProvisionedThroughput.ReadCapacityUnits),
					WriteCapacityUnits: aws.ToInt64(t.ProvisionedThroughput.WriteCapacityUnits),
				}
			}
			if t.SSEDescription != nil && t.SSEDescription.Status == ddbtypes.SSEStatusEnabled {
				tbl.Spec.SSEEnabled = true
				tbl.Spec.KMSKeyARN = aws.ToString(t.SSEDescription.KMSMasterKeyArn)
			}
			// PITR is a separate describe call; tolerate errors (leave false).
			if cb, err := clients.DynamoDB.DescribeContinuousBackups(ctx, &awsdynamodb.DescribeContinuousBackupsInput{
				TableName: aws.String(name),
			}); err == nil && cb.ContinuousBackupsDescription != nil &&
				cb.ContinuousBackupsDescription.PointInTimeRecoveryDescription != nil &&
				cb.ContinuousBackupsDescription.PointInTimeRecoveryDescription.PointInTimeRecoveryStatus == ddbtypes.PointInTimeRecoveryStatusEnabled {
				tbl.Spec.PointInTimeRecovery = true
			}
			opts.Index.Add(aws.ToString(t.TableArn), tbl.Name)
			opts.Index.Add(name, tbl.Name)
			objs = append(objs, tbl)
		}
	}
	return objs, nil
}

func exportDynamoDBGlobalTables(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	// ListGlobalTables (2017.11.29 version) has no SDK paginator; page manually.
	var start *string
	for {
		out, err := clients.DynamoDB.ListGlobalTables(ctx, &awsdynamodb.ListGlobalTablesInput{
			ExclusiveStartGlobalTableName: start,
		})
		if err != nil {
			return nil, fmt.Errorf("list global tables: %w", err)
		}
		for _, gt := range out.GlobalTables {
			name := aws.ToString(gt.GlobalTableName)
			g := &awsv1alpha1.DynamoDBGlobalTable{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.DynamoDBGlobalTableSpec{
					TableName: name,
				},
			}
			for _, r := range gt.ReplicationGroup {
				g.Spec.ReplicationGroup = append(g.Spec.ReplicationGroup, awsv1alpha1.DynamoDBReplicaSpec{
					RegionName: aws.ToString(r.RegionName),
				})
			}
			opts.Index.Add("dynamodb-global-table/"+name, g.Name)
			objs = append(objs, g)
		}
		if out.LastEvaluatedGlobalTableName == nil {
			break
		}
		start = out.LastEvaluatedGlobalTableName
	}
	return objs, nil
}

func exportDynamoDBTablePolicies(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsdynamodb.NewListTablesPaginator(clients.DynamoDB, &awsdynamodb.ListTablesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list tables: %w", err)
		}
		for _, name := range page.TableNames {
			desc, err := clients.DynamoDB.DescribeTable(ctx, &awsdynamodb.DescribeTableInput{
				TableName: aws.String(name),
			})
			if err != nil || desc.Table == nil || desc.Table.TableArn == nil {
				continue
			}
			arn := aws.ToString(desc.Table.TableArn)
			pol, err := clients.DynamoDB.GetResourcePolicy(ctx, &awsdynamodb.GetResourcePolicyInput{
				ResourceArn: aws.String(arn),
			})
			if err != nil {
				// PolicyNotFoundException just means no policy is attached.
				var nf *ddbtypes.PolicyNotFoundException
				if errors.As(err, &nf) {
					continue
				}
				continue // per-table errors: skip
			}
			if pol.Policy == nil || *pol.Policy == "" {
				continue
			}
			tp := &awsv1alpha1.DynamoDBTablePolicy{
				ObjectMeta: export.ObjectMeta(name+"-policy", opts),
				Spec: awsv1alpha1.DynamoDBTablePolicySpec{
					ResourceARN:    arn,
					PolicyDocument: *pol.Policy,
				},
			}
			objs = append(objs, tp)
		}
	}
	return objs, nil
}

// ecTagMap fetches ElastiCache resource tags by ARN; errors are tolerated.
func ecTagMap(ctx context.Context, clients *awsclient.Clients, arn string) map[string]string {
	if arn == "" {
		return nil
	}
	out, err := clients.ElastiCache.ListTagsForResource(ctx, &awselasticache.ListTagsForResourceInput{
		ResourceName: aws.String(arn),
	})
	if err != nil {
		return nil
	}
	m := make(map[string]string, len(out.TagList))
	for _, t := range out.TagList {
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return export.TagMap(m)
}

func exportElastiCacheSubnetGroups(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awselasticache.NewDescribeCacheSubnetGroupsPaginator(clients.ElastiCache, &awselasticache.DescribeCacheSubnetGroupsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe cache subnet groups: %w", err)
		}
		for _, g := range page.CacheSubnetGroups {
			name := aws.ToString(g.CacheSubnetGroupName)
			sg := &awsv1alpha1.ElastiCacheSubnetGroup{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.ElastiCacheSubnetGroupSpec{
					SubnetGroupName: name,
					Description:     aws.ToString(g.CacheSubnetGroupDescription),
					Tags:            ecTagMap(ctx, clients, aws.ToString(g.ARN)),
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
			opts.Index.Add(aws.ToString(g.ARN), sg.Name)
			opts.Index.Add("elasticache-subnet-group/"+name, sg.Name)
			objs = append(objs, sg)
		}
	}
	return objs, nil
}

func exportElastiCacheParameterGroups(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awselasticache.NewDescribeCacheParameterGroupsPaginator(clients.ElastiCache, &awselasticache.DescribeCacheParameterGroupsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe cache parameter groups: %w", err)
		}
		for _, g := range page.CacheParameterGroups {
			name := aws.ToString(g.CacheParameterGroupName)
			// default.* groups are AWS-managed and cannot be created/modified.
			if strings.HasPrefix(name, "default.") {
				continue
			}
			pg := &awsv1alpha1.ElastiCacheParameterGroup{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.ElastiCacheParameterGroupSpec{
					CacheParameterGroupName:   name,
					CacheParameterGroupFamily: aws.ToString(g.CacheParameterGroupFamily),
					Description:               aws.ToString(g.Description),
					Tags:                      ecTagMap(ctx, clients, aws.ToString(g.ARN)),
				},
			}
			// Only user-modified parameters belong in the spec.
			pp := awselasticache.NewDescribeCacheParametersPaginator(clients.ElastiCache, &awselasticache.DescribeCacheParametersInput{
				CacheParameterGroupName: g.CacheParameterGroupName,
				Source:                  aws.String("user"),
			})
			for pp.HasMorePages() {
				ppage, err := pp.NextPage(ctx)
				if err != nil {
					break // parameters are best-effort; keep the group itself
				}
				for _, par := range ppage.Parameters {
					pg.Spec.Parameters = append(pg.Spec.Parameters, awsv1alpha1.ElastiCacheParameter{
						Name:  aws.ToString(par.ParameterName),
						Value: aws.ToString(par.ParameterValue),
					})
				}
			}
			opts.Index.Add(aws.ToString(g.ARN), pg.Name)
			opts.Index.Add("elasticache-parameter-group/"+name, pg.Name)
			objs = append(objs, pg)
		}
	}
	return objs, nil
}

func exportElastiCacheReplicationGroups(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awselasticache.NewDescribeReplicationGroupsPaginator(clients.ElastiCache, &awselasticache.DescribeReplicationGroupsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe replication groups: %w", err)
		}
		for _, g := range page.ReplicationGroups {
			id := aws.ToString(g.ReplicationGroupId)
			rg := &awsv1alpha1.ElastiCacheReplicationGroup{
				ObjectMeta: export.ObjectMeta(id, opts),
				Spec: awsv1alpha1.ElastiCacheReplicationGroupSpec{
					ReplicationGroupID:     id,
					Description:            aws.ToString(g.Description),
					CacheNodeType:          aws.ToString(g.CacheNodeType),
					NumCacheClusters:       int32(len(g.MemberClusters)),
					AutomaticFailover:      g.AutomaticFailover == ectypes.AutomaticFailoverStatusEnabled || g.AutomaticFailover == ectypes.AutomaticFailoverStatusEnabling,
					AtRestEncryption:       aws.ToBool(g.AtRestEncryptionEnabled),
					TransitEncryption:      aws.ToBool(g.TransitEncryptionEnabled),
					SnapshotRetentionLimit: aws.ToInt32(g.SnapshotRetentionLimit),
					Tags:                   ecTagMap(ctx, clients, aws.ToString(g.ARN)),
					// SECURITY: authToken/authTokenRef are deliberately left unset.
					// The auth token is write-only in the ElastiCache API and can
					// never be read back; exporting anything here would be wrong.
				},
			}
			// Engine/version and subnet/security groups live on the member cache
			// clusters, not the replication group; describe the first member.
			if len(g.MemberClusters) > 0 {
				cc, err := clients.ElastiCache.DescribeCacheClusters(ctx, &awselasticache.DescribeCacheClustersInput{
					CacheClusterId: aws.String(g.MemberClusters[0]),
				})
				if err == nil && len(cc.CacheClusters) > 0 {
					m := cc.CacheClusters[0]
					rg.Spec.Engine = aws.ToString(m.Engine)
					rg.Spec.EngineVersion = aws.ToString(m.EngineVersion)
					subnetGroup := aws.ToString(m.CacheSubnetGroupName)
					if crName, ok := opts.Index.Lookup("elasticache-subnet-group/" + subnetGroup); ok {
						rg.Spec.SubnetGroupRef = crName
					} else if subnetGroup != "" {
						rg.Spec.SubnetGroupRef = export.CRName(subnetGroup)
					}
					for _, sgm := range m.SecurityGroups {
						sgID := aws.ToString(sgm.SecurityGroupId)
						if sgID == "" {
							continue
						}
						if crName, ok := opts.Index.Lookup(sgID); ok {
							rg.Spec.SecurityGroupRefs = append(rg.Spec.SecurityGroupRefs, awsv1alpha1.SecurityGroupRef{Name: crName})
						} else {
							rg.Spec.SecurityGroupRefs = append(rg.Spec.SecurityGroupRefs, awsv1alpha1.SecurityGroupRef{ID: sgID})
						}
					}
				}
			}
			if rg.Spec.Engine == "" {
				// Spec requires an engine; redis is the only pre-valkey value.
				rg.Spec.Engine = "redis"
			}
			opts.Index.Add(aws.ToString(g.ARN), rg.Name)
			opts.Index.Add(id, rg.Name)
			objs = append(objs, rg)
		}
	}
	return objs, nil
}

func exportElastiCacheServerlessCaches(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awselasticache.NewDescribeServerlessCachesPaginator(clients.ElastiCache, &awselasticache.DescribeServerlessCachesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe serverless caches: %w", err)
		}
		for _, c := range page.ServerlessCaches {
			name := aws.ToString(c.ServerlessCacheName)
			sc := &awsv1alpha1.ElastiCacheServerlessCache{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.ElastiCacheServerlessCacheSpec{
					ServerlessCacheName:    name,
					Engine:                 aws.ToString(c.Engine),
					Description:            aws.ToString(c.Description),
					KMSKeyID:               aws.ToString(c.KmsKeyId),
					SecurityGroupIDs:       c.SecurityGroupIds,
					SubnetIDs:              c.SubnetIds,
					SnapshotRetentionLimit: c.SnapshotRetentionLimit,
					Tags:                   ecTagMap(ctx, clients, aws.ToString(c.ARN)),
				},
			}
			opts.Index.Add(aws.ToString(c.ARN), sc.Name)
			objs = append(objs, sc)
		}
	}
	return objs, nil
}

// memorydbTagMap fetches MemoryDB resource tags by ARN; errors are tolerated.
func memorydbTagMap(ctx context.Context, clients *awsclient.Clients, arn string, tags []mdbtypes.Tag) map[string]string {
	m := map[string]string{}
	if len(tags) == 0 && arn != "" {
		out, err := clients.MemoryDB.ListTags(ctx, &awsmemorydb.ListTagsInput{ResourceArn: aws.String(arn)})
		if err != nil {
			return nil
		}
		tags = out.TagList
	}
	for _, t := range tags {
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return export.TagMap(m)
}

func exportMemoryDBClusters(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsmemorydb.NewDescribeClustersPaginator(clients.MemoryDB, &awsmemorydb.DescribeClustersInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe memorydb clusters: %w", err)
		}
		for _, c := range page.Clusters {
			name := aws.ToString(c.Name)
			mc := &awsv1alpha1.MemoryDBCluster{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.MemoryDBClusterSpec{
					ClusterName:            name,
					NodeType:               aws.ToString(c.NodeType),
					ACLName:                aws.ToString(c.ACLName),
					NumShards:              c.NumberOfShards,
					SubnetGroupName:        aws.ToString(c.SubnetGroupName),
					EngineVersion:          aws.ToString(c.EngineVersion),
					SnapshotRetentionLimit: c.SnapshotRetentionLimit,
					TLSEnabled:             c.TLSEnabled,
					KMSKeyID:               aws.ToString(c.KmsKeyId),
					Tags:                   memorydbTagMap(ctx, clients, aws.ToString(c.ARN), nil),
				},
			}
			// Replicas per shard: node count minus the primary of the first shard.
			if len(c.Shards) > 0 && c.Shards[0].NumberOfNodes != nil {
				replicas := aws.ToInt32(c.Shards[0].NumberOfNodes) - 1
				if replicas >= 0 {
					mc.Spec.NumReplicasPerShard = &replicas
				}
			}
			for _, sgm := range c.SecurityGroups {
				if id := aws.ToString(sgm.SecurityGroupId); id != "" {
					mc.Spec.SecurityGroupIDs = append(mc.Spec.SecurityGroupIDs, id)
				}
			}
			opts.Index.Add(aws.ToString(c.ARN), mc.Name)
			objs = append(objs, mc)
		}
	}
	return objs, nil
}

// daxTagMap fetches DAX resource tags by resource name; errors are tolerated.
func daxTagMap(ctx context.Context, clients *awsclient.Clients, resourceName string) map[string]string {
	if resourceName == "" {
		return nil
	}
	m := map[string]string{}
	var next *string
	for {
		out, err := clients.DAX.ListTags(ctx, &awsdax.ListTagsInput{
			ResourceName: aws.String(resourceName),
			NextToken:    next,
		})
		if err != nil {
			return nil
		}
		for _, t := range out.Tags {
			m[aws.ToString(t.Key)] = aws.ToString(t.Value)
		}
		if out.NextToken == nil {
			break
		}
		next = out.NextToken
	}
	return export.TagMap(m)
}

func exportDAXClusters(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	// DescribeClusters has no SDK paginator in this dax module version.
	var next *string
	for {
		out, err := clients.DAX.DescribeClusters(ctx, &awsdax.DescribeClustersInput{NextToken: next})
		if err != nil {
			return nil, fmt.Errorf("describe dax clusters: %w", err)
		}
		for _, c := range out.Clusters {
			name := aws.ToString(c.ClusterName)
			dc := &awsv1alpha1.DAXCluster{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.DAXClusterSpec{
					ClusterName:       name,
					NodeType:          aws.ToString(c.NodeType),
					ReplicationFactor: aws.ToInt32(c.TotalNodes),
					IAMRoleARN:        aws.ToString(c.IamRoleArn),
					SubnetGroupName:   aws.ToString(c.SubnetGroup),
					Tags:              daxTagMap(ctx, clients, aws.ToString(c.ClusterArn)),
				},
			}
			if c.ParameterGroup != nil {
				dc.Spec.ParameterGroupName = aws.ToString(c.ParameterGroup.ParameterGroupName)
			}
			for _, sg := range c.SecurityGroups {
				if id := aws.ToString(sg.SecurityGroupIdentifier); id != "" {
					dc.Spec.SecurityGroupIDs = append(dc.Spec.SecurityGroupIDs, id)
				}
			}
			var azs []string
			seen := map[string]bool{}
			for _, n := range c.Nodes {
				az := aws.ToString(n.AvailabilityZone)
				if az != "" && !seen[az] {
					seen[az] = true
					azs = append(azs, az)
				}
			}
			dc.Spec.AvailabilityZones = azs
			opts.Index.Add(aws.ToString(c.ClusterArn), dc.Name)
			objs = append(objs, dc)
		}
		if out.NextToken == nil {
			break
		}
		next = out.NextToken
	}
	return objs, nil
}
