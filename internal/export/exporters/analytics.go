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
	awsathena "github.com/aws/aws-sdk-go-v2/service/athena"
	awsglue "github.com/aws/aws-sdk-go-v2/service/glue"
	awsredshift "github.com/aws/aws-sdk-go-v2/service/redshift"
	redshifttypes "github.com/aws/aws-sdk-go-v2/service/redshift/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	awsclient "github.com/konfig-io/konfig-konector/internal/aws"
	"github.com/konfig-io/konfig-konector/internal/export"
)

func init() {
	// Databases before crawlers/jobs/triggers/connections; subnet and
	// parameter groups before clusters.
	export.Register(export.Exporter{Kind: "GlueDatabase", Service: "glue", Order: 95, Fn: exportGlueDatabases})
	export.Register(export.Exporter{Kind: "GlueConnection", Service: "glue", Order: 96, Fn: exportGlueConnections})
	export.Register(export.Exporter{Kind: "GlueCrawler", Service: "glue", Order: 96, Fn: exportGlueCrawlers})
	export.Register(export.Exporter{Kind: "GlueJob", Service: "glue", Order: 96, Fn: exportGlueJobs})
	export.Register(export.Exporter{Kind: "GlueTrigger", Service: "glue", Order: 97, Fn: exportGlueTriggers})
	export.Register(export.Exporter{Kind: "AthenaWorkGroup", Service: "athena", Order: 97, Fn: exportAthenaWorkGroups})
	export.Register(export.Exporter{Kind: "AthenaDataCatalog", Service: "athena", Order: 97, Fn: exportAthenaDataCatalogs})
	export.Register(export.Exporter{Kind: "AthenaNamedQuery", Service: "athena", Order: 98, Fn: exportAthenaNamedQueries})
	export.Register(export.Exporter{Kind: "RedshiftSubnetGroup", Service: "redshift", Order: 98, Fn: exportRedshiftSubnetGroups})
	export.Register(export.Exporter{Kind: "RedshiftParameterGroup", Service: "redshift", Order: 98, Fn: exportRedshiftParameterGroups})
	export.Register(export.Exporter{Kind: "RedshiftCluster", Service: "redshift", Order: 99, Fn: exportRedshiftClusters})
}

func exportGlueDatabases(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsglue.NewGetDatabasesPaginator(clients.Glue, &awsglue.GetDatabasesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("get glue databases: %w", err)
		}
		for _, d := range page.DatabaseList {
			name := aws.ToString(d.Name)
			cr := &awsv1alpha1.GlueDatabase{
				ObjectMeta: export.ObjectMeta("glue-db-"+name, opts),
				Spec: awsv1alpha1.GlueDatabaseSpec{
					Name:        name,
					Description: aws.ToString(d.Description),
					LocationURI: aws.ToString(d.LocationUri),
					Parameters:  d.Parameters,
				},
			}
			opts.Index.Add("glue-database/"+name, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportGlueCrawlers(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsglue.NewGetCrawlersPaginator(clients.Glue, &awsglue.GetCrawlersInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("get glue crawlers: %w", err)
		}
		for _, c := range page.Crawlers {
			name := aws.ToString(c.Name)
			cr := &awsv1alpha1.GlueCrawler{
				ObjectMeta: export.ObjectMeta("glue-crawler-"+name, opts),
				Spec: awsv1alpha1.GlueCrawlerSpec{
					Name:        name,
					RoleRef:     awsv1alpha1.RoleRef{ARN: aws.ToString(c.Role)},
					TablePrefix: aws.ToString(c.TablePrefix),
					Description: aws.ToString(c.Description),
				},
			}
			if c.Configuration != nil {
				cr.Spec.Configuration = aws.ToString(c.Configuration)
			}
			if dbName := aws.ToString(c.DatabaseName); dbName != "" {
				// Reference the GlueDatabase CR when it was exported in this
				// run; fall back to the raw database name.
				if crName, ok := opts.Index.Lookup("glue-database/" + dbName); ok {
					cr.Spec.DatabaseRef = crName
				} else {
					cr.Spec.DatabaseName = dbName
				}
			}
			if c.Schedule != nil {
				cr.Spec.Schedule = aws.ToString(c.Schedule.ScheduleExpression)
			}
			if c.SchemaChangePolicy != nil {
				cr.Spec.SchemaChangePolicy = &awsv1alpha1.GlueSchemaChangePolicy{
					UpdateBehavior: string(c.SchemaChangePolicy.UpdateBehavior),
					DeleteBehavior: string(c.SchemaChangePolicy.DeleteBehavior),
				}
			}
			if c.Targets != nil {
				for _, t := range c.Targets.S3Targets {
					cr.Spec.Targets.S3Targets = append(cr.Spec.Targets.S3Targets, awsv1alpha1.GlueS3Target{
						Path:       aws.ToString(t.Path),
						Exclusions: t.Exclusions,
					})
				}
				for _, t := range c.Targets.JdbcTargets {
					cr.Spec.Targets.JDBCTargets = append(cr.Spec.Targets.JDBCTargets, awsv1alpha1.GlueJDBCTarget{
						ConnectionName: aws.ToString(t.ConnectionName),
						Path:           aws.ToString(t.Path),
						Exclusions:     t.Exclusions,
					})
				}
			}
			opts.Index.Add("glue-crawler/"+name, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportGlueJobs(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsglue.NewGetJobsPaginator(clients.Glue, &awsglue.GetJobsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("get glue jobs: %w", err)
		}
		for _, j := range page.Jobs {
			// A job without a command cannot round-trip into a valid spec.
			if j.Command == nil {
				continue
			}
			name := aws.ToString(j.Name)
			cr := &awsv1alpha1.GlueJob{
				ObjectMeta: export.ObjectMeta("glue-job-"+name, opts),
				Spec: awsv1alpha1.GlueJobSpec{
					Name:    name,
					RoleRef: awsv1alpha1.RoleRef{ARN: aws.ToString(j.Role)},
					Command: awsv1alpha1.GlueJobCommand{
						Name:           aws.ToString(j.Command.Name),
						ScriptLocation: aws.ToString(j.Command.ScriptLocation),
						PythonVersion:  aws.ToString(j.Command.PythonVersion),
					},
					DefaultArguments: j.DefaultArguments,
					MaxRetries:       j.MaxRetries,
					Timeout:          j.Timeout,
					GlueVersion:      aws.ToString(j.GlueVersion),
					NumberOfWorkers:  j.NumberOfWorkers,
					WorkerType:       string(j.WorkerType),
					Description:      aws.ToString(j.Description),
				},
			}
			opts.Index.Add("glue-job/"+name, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportGlueTriggers(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsglue.NewGetTriggersPaginator(clients.Glue, &awsglue.GetTriggersInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("get glue triggers: %w", err)
		}
		for _, t := range page.Triggers {
			// The CRD only models SCHEDULED/CONDITIONAL/ON_DEMAND triggers.
			switch t.Type {
			case "SCHEDULED", "CONDITIONAL", "ON_DEMAND":
			default:
				continue
			}
			name := aws.ToString(t.Name)
			cr := &awsv1alpha1.GlueTrigger{
				ObjectMeta: export.ObjectMeta("glue-trigger-"+name, opts),
				Spec: awsv1alpha1.GlueTriggerSpec{
					Name:        name,
					Type:        string(t.Type),
					Schedule:    aws.ToString(t.Schedule),
					Description: aws.ToString(t.Description),
				},
			}
			for _, a := range t.Actions {
				// Crawler-only actions cannot round-trip; jobName is required.
				if aws.ToString(a.JobName) == "" {
					continue
				}
				cr.Spec.Actions = append(cr.Spec.Actions, awsv1alpha1.GlueTriggerAction{
					JobName:   aws.ToString(a.JobName),
					Arguments: a.Arguments,
				})
			}
			if len(cr.Spec.Actions) == 0 {
				continue
			}
			if t.Predicate != nil {
				pred := &awsv1alpha1.GlueTriggerPredicate{Logical: string(t.Predicate.Logical)}
				for _, c := range t.Predicate.Conditions {
					pred.Conditions = append(pred.Conditions, awsv1alpha1.GlueTriggerCondition{
						JobName:         aws.ToString(c.JobName),
						CrawlerName:     aws.ToString(c.CrawlerName),
						State:           string(c.State),
						LogicalOperator: string(c.LogicalOperator),
					})
				}
				cr.Spec.Predicate = pred
			}
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportGlueConnections(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsglue.NewGetConnectionsPaginator(clients.Glue, &awsglue.GetConnectionsInput{
		// Never fetch (let alone export) connection passwords.
		HidePassword: true,
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("get glue connections: %w", err)
		}
		for _, c := range page.ConnectionList {
			name := aws.ToString(c.Name)
			// SECURITY: strip PASSWORD/ENCRYPTED_PASSWORD from properties —
			// secret material must never round-trip into a spec. The
			// PasswordSecretRef CHANGEME placeholder marks where the operator
			// must be given a real Secret.
			props := make(map[string]string, len(c.ConnectionProperties))
			hadPassword := false
			for k, v := range c.ConnectionProperties {
				if k == "PASSWORD" || k == "ENCRYPTED_PASSWORD" {
					hadPassword = true
					continue
				}
				props[k] = v
			}
			cr := &awsv1alpha1.GlueConnection{
				ObjectMeta: export.ObjectMeta("glue-conn-"+name, opts),
				Spec: awsv1alpha1.GlueConnectionSpec{
					Name:                 name,
					ConnectionType:       string(c.ConnectionType),
					ConnectionProperties: props,
					Description:          aws.ToString(c.Description),
				},
			}
			if hadPassword {
				cr.Spec.PasswordSecretRef = &awsv1alpha1.SecretRef{
					Name: "CHANGEME-glue-connection-password",
					Key:  "password",
				}
			}
			if pcr := c.PhysicalConnectionRequirements; pcr != nil {
				req := &awsv1alpha1.GluePhysicalConnectionRequirements{
					AvailabilityZone: aws.ToString(pcr.AvailabilityZone),
				}
				if subnetID := aws.ToString(pcr.SubnetId); subnetID != "" {
					if crName, ok := opts.Index.Lookup(subnetID); ok {
						req.SubnetRef = &awsv1alpha1.SubnetRef{Name: crName}
					} else {
						req.SubnetRef = &awsv1alpha1.SubnetRef{ID: subnetID}
					}
				}
				for _, sgID := range pcr.SecurityGroupIdList {
					if crName, ok := opts.Index.Lookup(sgID); ok {
						req.SecurityGroupRefs = append(req.SecurityGroupRefs, awsv1alpha1.SecurityGroupRef{Name: crName})
					} else {
						req.SecurityGroupRefs = append(req.SecurityGroupRefs, awsv1alpha1.SecurityGroupRef{ID: sgID})
					}
				}
				cr.Spec.PhysicalConnectionRequirements = req
			}
			opts.Index.Add("glue-connection/"+name, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportAthenaWorkGroups(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsathena.NewListWorkGroupsPaginator(clients.Athena, &awsathena.ListWorkGroupsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list athena workgroups: %w", err)
		}
		for _, s := range page.WorkGroups {
			name := aws.ToString(s.Name)
			// The primary workgroup is AWS-managed and cannot be recreated.
			if name == "primary" {
				continue
			}
			got, err := clients.Athena.GetWorkGroup(ctx, &awsathena.GetWorkGroupInput{
				WorkGroup: s.Name,
			})
			if err != nil || got.WorkGroup == nil {
				continue
			}
			wg := got.WorkGroup
			cr := &awsv1alpha1.AthenaWorkGroup{
				ObjectMeta: export.ObjectMeta("athena-wg-"+name, opts),
				Spec: awsv1alpha1.AthenaWorkGroupSpec{
					Name:        name,
					Description: aws.ToString(wg.Description),
				},
			}
			if cfg := wg.Configuration; cfg != nil {
				cr.Spec.EnforceWorkGroupConfiguration = cfg.EnforceWorkGroupConfiguration
				cr.Spec.PublishCloudWatchMetricsEnabled = cfg.PublishCloudWatchMetricsEnabled
				cr.Spec.BytesScannedCutoffPerQuery = cfg.BytesScannedCutoffPerQuery
				if rc := cfg.ResultConfiguration; rc != nil {
					src := &awsv1alpha1.AthenaResultConfiguration{
						OutputLocation: aws.ToString(rc.OutputLocation),
					}
					if ec := rc.EncryptionConfiguration; ec != nil {
						src.EncryptionConfiguration = &awsv1alpha1.AthenaEncryptionConfiguration{
							EncryptionOption: string(ec.EncryptionOption),
							KMSKey:           aws.ToString(ec.KmsKey),
						}
					}
					cr.Spec.ResultConfiguration = src
				}
			}
			opts.Index.Add("athena-workgroup/"+name, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportAthenaDataCatalogs(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsathena.NewListDataCatalogsPaginator(clients.Athena, &awsathena.ListDataCatalogsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list athena data catalogs: %w", err)
		}
		for _, s := range page.DataCatalogsSummary {
			name := aws.ToString(s.CatalogName)
			// AwsDataCatalog is the AWS-managed default Glue catalog.
			if name == "AwsDataCatalog" {
				continue
			}
			got, err := clients.Athena.GetDataCatalog(ctx, &awsathena.GetDataCatalogInput{
				Name: s.CatalogName,
			})
			if err != nil || got.DataCatalog == nil {
				continue
			}
			dc := got.DataCatalog
			cr := &awsv1alpha1.AthenaDataCatalog{
				ObjectMeta: export.ObjectMeta("athena-catalog-"+name, opts),
				Spec: awsv1alpha1.AthenaDataCatalogSpec{
					Name:        name,
					Type:        string(dc.Type),
					Parameters:  dc.Parameters,
					Description: aws.ToString(dc.Description),
				},
			}
			opts.Index.Add("athena-datacatalog/"+name, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportAthenaNamedQueries(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsathena.NewListNamedQueriesPaginator(clients.Athena, &awsathena.ListNamedQueriesInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list athena named queries: %w", err)
		}
		for _, id := range page.NamedQueryIds {
			got, err := clients.Athena.GetNamedQuery(ctx, &awsathena.GetNamedQueryInput{
				NamedQueryId: aws.String(id),
			})
			if err != nil || got.NamedQuery == nil {
				continue
			}
			nq := got.NamedQuery
			cr := &awsv1alpha1.AthenaNamedQuery{
				ObjectMeta: export.ObjectMeta("athena-query-"+aws.ToString(nq.Name), opts),
				Spec: awsv1alpha1.AthenaNamedQuerySpec{
					Name:        aws.ToString(nq.Name),
					Database:    aws.ToString(nq.Database),
					QueryString: aws.ToString(nq.QueryString),
					WorkGroup:   aws.ToString(nq.WorkGroup),
					Description: aws.ToString(nq.Description),
				},
			}
			opts.Index.Add("athena-namedquery/"+id, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func redshiftTagMap(tags []redshifttypes.Tag) map[string]string {
	if len(tags) == 0 {
		return nil
	}
	m := make(map[string]string, len(tags))
	for _, t := range tags {
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return export.TagMap(m)
}

func exportRedshiftSubnetGroups(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsredshift.NewDescribeClusterSubnetGroupsPaginator(clients.Redshift, &awsredshift.DescribeClusterSubnetGroupsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe redshift subnet groups: %w", err)
		}
		for _, g := range page.ClusterSubnetGroups {
			name := aws.ToString(g.ClusterSubnetGroupName)
			cr := &awsv1alpha1.RedshiftSubnetGroup{
				ObjectMeta: export.ObjectMeta("redshift-sng-"+name, opts),
				Spec: awsv1alpha1.RedshiftSubnetGroupSpec{
					Name:        name,
					Description: aws.ToString(g.Description),
					Tags:        redshiftTagMap(g.Tags),
				},
			}
			for _, sn := range g.Subnets {
				id := aws.ToString(sn.SubnetIdentifier)
				if crName, ok := opts.Index.Lookup(id); ok {
					cr.Spec.SubnetRefs = append(cr.Spec.SubnetRefs, awsv1alpha1.SubnetRef{Name: crName})
				} else {
					cr.Spec.SubnetRefs = append(cr.Spec.SubnetRefs, awsv1alpha1.SubnetRef{ID: id})
				}
			}
			opts.Index.Add("redshift-subnetgroup/"+name, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportRedshiftParameterGroups(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsredshift.NewDescribeClusterParameterGroupsPaginator(clients.Redshift, &awsredshift.DescribeClusterParameterGroupsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe redshift parameter groups: %w", err)
		}
		for _, g := range page.ParameterGroups {
			name := aws.ToString(g.ParameterGroupName)
			// default.* parameter groups are AWS-managed and immutable.
			if len(name) >= 8 && name[:8] == "default." {
				continue
			}
			cr := &awsv1alpha1.RedshiftParameterGroup{
				ObjectMeta: export.ObjectMeta("redshift-pg-"+name, opts),
				Spec: awsv1alpha1.RedshiftParameterGroupSpec{
					Name:        name,
					Family:      aws.ToString(g.ParameterGroupFamily),
					Description: aws.ToString(g.Description),
					Tags:        redshiftTagMap(g.Tags),
				},
			}
			// Only user-modified parameters round-trip into the spec.
			pp := awsredshift.NewDescribeClusterParametersPaginator(clients.Redshift, &awsredshift.DescribeClusterParametersInput{
				ParameterGroupName: g.ParameterGroupName,
				Source:             aws.String("user"),
			})
			for pp.HasMorePages() {
				paramsPage, err := pp.NextPage(ctx)
				if err != nil {
					break
				}
				for _, prm := range paramsPage.Parameters {
					cr.Spec.Parameters = append(cr.Spec.Parameters, awsv1alpha1.RedshiftParameter{
						Name:  aws.ToString(prm.ParameterName),
						Value: aws.ToString(prm.ParameterValue),
					})
				}
			}
			opts.Index.Add("redshift-parametergroup/"+name, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportRedshiftClusters(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsredshift.NewDescribeClustersPaginator(clients.Redshift, &awsredshift.DescribeClustersInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe redshift clusters: %w", err)
		}
		for _, c := range page.Clusters {
			// Only steady-state clusters export a stable config.
			if aws.ToString(c.ClusterStatus) != "available" {
				continue
			}
			id := aws.ToString(c.ClusterIdentifier)
			cr := &awsv1alpha1.RedshiftCluster{
				ObjectMeta: export.ObjectMeta("redshift-"+id, opts),
				Spec: awsv1alpha1.RedshiftClusterSpec{
					ClusterIdentifier: id,
					NodeType:          aws.ToString(c.NodeType),
					NumberOfNodes:     aws.ToInt32(c.NumberOfNodes),
					MasterUsername:    aws.ToString(c.MasterUsername),
					// SECURITY: the master password can never be exported;
					// the operator requires a real Secret to be created.
					MasterUserPasswordRef: awsv1alpha1.SecretRef{
						Name: "CHANGEME-redshift-master-password",
						Key:  "password",
					},
					DBName:             aws.ToString(c.DBName),
					Encrypted:          aws.ToBool(c.Encrypted),
					KMSKeyID:           aws.ToString(c.KmsKeyId),
					PubliclyAccessible: aws.ToBool(c.PubliclyAccessible),
					Tags:               redshiftTagMap(c.Tags),
				},
			}
			if sng := aws.ToString(c.ClusterSubnetGroupName); sng != "" {
				if crName, ok := opts.Index.Lookup("redshift-subnetgroup/" + sng); ok {
					cr.Spec.ClusterSubnetGroupRef = crName
				} else {
					cr.Spec.ClusterSubnetGroupName = sng
				}
			}
			for _, sg := range c.VpcSecurityGroups {
				sgID := aws.ToString(sg.VpcSecurityGroupId)
				if crName, ok := opts.Index.Lookup(sgID); ok {
					cr.Spec.VPCSecurityGroupRefs = append(cr.Spec.VPCSecurityGroupRefs, awsv1alpha1.SecurityGroupRef{Name: crName})
				} else {
					cr.Spec.VPCSecurityGroupRefs = append(cr.Spec.VPCSecurityGroupRefs, awsv1alpha1.SecurityGroupRef{ID: sgID})
				}
			}
			opts.Index.Add("redshift-cluster/"+id, cr.Name)
			opts.Index.Add(aws.ToString(c.ClusterNamespaceArn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}
