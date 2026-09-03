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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	awsclient "github.com/konfig-io/konfig-konector/internal/aws"
	"github.com/konfig-io/konfig-konector/internal/export"
)

func init() {
	// Functions must be indexed before their subresources so that
	// aliases/permissions/URLs can emit CR-name refs.
	export.Register(export.Exporter{Kind: "LambdaFunction", Service: "lambda", Order: 50, Fn: exportLambdaFunctions})
	export.Register(export.Exporter{Kind: "LambdaAlias", Service: "lambda", Order: 51, Fn: exportLambdaAliases})
	export.Register(export.Exporter{Kind: "LambdaPermission", Service: "lambda", Order: 52, Fn: exportLambdaPermissions})
	export.Register(export.Exporter{Kind: "LambdaEventSourceMapping", Service: "lambda", Order: 53, Fn: exportLambdaEventSourceMappings})
	export.Register(export.Exporter{Kind: "LambdaFunctionURL", Service: "lambda", Order: 54, Fn: exportLambdaFunctionURLs})
	export.Register(export.Exporter{Kind: "LambdaLayerVersion", Service: "lambda", Order: 55, Fn: exportLambdaLayerVersions})
	export.Register(export.Exporter{Kind: "LambdaCodeSigningConfig", Service: "lambda", Order: 55, Fn: exportLambdaCodeSigningConfigs})
}

// lambdaFuncNameFromArn extracts the function name from a (possibly
// qualified) Lambda function ARN.
func lambdaFuncNameFromArn(arn string) string {
	parts := strings.Split(arn, ":")
	// arn:aws:lambda:region:acct:function:name[:qualifier]
	if len(parts) >= 7 {
		return parts[6]
	}
	return arn
}

// lambdaUnqualifiedArn strips a version/alias qualifier from a function ARN.
func lambdaUnqualifiedArn(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) == 8 {
		return strings.Join(parts[:7], ":")
	}
	return arn
}

func exportLambdaFunctions(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awslambda.NewListFunctionsPaginator(clients.Lambda, &awslambda.ListFunctionsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list functions: %w", err)
		}
		for _, cfg := range page.Functions {
			name := aws.ToString(cfg.FunctionName)
			fn := &awsv1alpha1.LambdaFunction{
				ObjectMeta: export.ObjectMeta(name, opts),
				Spec: awsv1alpha1.LambdaFunctionSpec{
					FunctionName: name,
					Runtime:      string(cfg.Runtime),
					Handler:      aws.ToString(cfg.Handler),
					Description:  aws.ToString(cfg.Description),
					Timeout:      cfg.Timeout,
					MemorySize:   cfg.MemorySize,
				},
			}

			// Execution role: CR-name ref when the role was exported this run.
			roleArn := aws.ToString(cfg.Role)
			if crName, ok := opts.Index.Lookup(roleArn); ok {
				fn.Spec.RoleRef = &awsv1alpha1.RoleRef{Name: crName}
			} else {
				fn.Spec.RoleArn = roleArn
			}

			if cfg.Environment != nil && len(cfg.Environment.Variables) > 0 {
				fn.Spec.Environment = cfg.Environment.Variables
			}
			if len(cfg.Architectures) > 0 {
				fn.Spec.Architecture = string(cfg.Architectures[0])
			}
			if cfg.EphemeralStorage != nil {
				fn.Spec.EphemeralStorageSize = cfg.EphemeralStorage.Size
			}
			for _, l := range cfg.Layers {
				fn.Spec.Layers = append(fn.Spec.Layers, aws.ToString(l.Arn))
			}
			if cfg.DeadLetterConfig != nil && aws.ToString(cfg.DeadLetterConfig.TargetArn) != "" {
				fn.Spec.DeadLetterConfig = &awsv1alpha1.LambdaDeadLetterConfig{
					TargetARN: aws.ToString(cfg.DeadLetterConfig.TargetArn),
				}
			}
			// PassThrough is the AWS default; only Active is worth exporting.
			if cfg.TracingConfig != nil && cfg.TracingConfig.Mode == lambdatypes.TracingModeActive {
				fn.Spec.TracingConfig = &awsv1alpha1.LambdaTracingConfig{Mode: string(cfg.TracingConfig.Mode)}
			}
			if lc := cfg.LoggingConfig; lc != nil {
				fn.Spec.LoggingConfig = &awsv1alpha1.LambdaLoggingConfig{
					LogFormat:           string(lc.LogFormat),
					LogGroup:            aws.ToString(lc.LogGroup),
					SystemLogLevel:      string(lc.SystemLogLevel),
					ApplicationLogLevel: string(lc.ApplicationLogLevel),
				}
			}
			for _, fs := range cfg.FileSystemConfigs {
				fn.Spec.FileSystemConfigs = append(fn.Spec.FileSystemConfigs, awsv1alpha1.LambdaFileSystemConfig{
					ARN:            aws.ToString(fs.Arn),
					LocalMountPath: aws.ToString(fs.LocalMountPath),
				})
			}
			if cfg.SnapStart != nil && cfg.SnapStart.ApplyOn == lambdatypes.SnapStartApplyOnPublishedVersions {
				fn.Spec.SnapStart = &awsv1alpha1.LambdaSnapStart{ApplyOn: string(cfg.SnapStart.ApplyOn)}
			}
			if cfg.ImageConfigResponse != nil && cfg.ImageConfigResponse.ImageConfig != nil {
				ic := cfg.ImageConfigResponse.ImageConfig
				fn.Spec.ImageConfig = &awsv1alpha1.LambdaImageConfig{
					Command:          ic.Command,
					EntryPoint:       ic.EntryPoint,
					WorkingDirectory: aws.ToString(ic.WorkingDirectory),
				}
			}
			if cfg.VpcConfig != nil && len(cfg.VpcConfig.SubnetIds) > 0 {
				vc := &awsv1alpha1.LambdaVpcConfig{}
				for _, sn := range cfg.VpcConfig.SubnetIds {
					if crName, ok := opts.Index.Lookup(sn); ok {
						vc.SubnetRefs = append(vc.SubnetRefs, awsv1alpha1.SubnetRef{Name: crName})
					} else {
						vc.SubnetRefs = append(vc.SubnetRefs, awsv1alpha1.SubnetRef{ID: sn})
					}
				}
				for _, sg := range cfg.VpcConfig.SecurityGroupIds {
					if crName, ok := opts.Index.Lookup(sg); ok {
						vc.SecurityGroupRefs = append(vc.SecurityGroupRefs, awsv1alpha1.SecurityGroupRef{Name: crName})
					} else {
						vc.SecurityGroupRefs = append(vc.SecurityGroupRefs, awsv1alpha1.SecurityGroupRef{ID: sg})
					}
				}
				fn.Spec.VpcConfig = vc
			}

			// Code source: GetFunction returns a presigned download URL, not
			// the original S3 coordinates, so those cannot be recovered. The
			// spec requires a code location: emit the image URI for container
			// functions, and a CHANGEME placeholder for zip functions that the
			// operator of the import must point at a real bucket/key.
			getOut, err := clients.Lambda.GetFunction(ctx, &awslambda.GetFunctionInput{
				FunctionName: cfg.FunctionName,
			})
			if err == nil {
				if getOut.Concurrency != nil {
					fn.Spec.ReservedConcurrency = getOut.Concurrency.ReservedConcurrentExecutions
				}
				fn.Spec.Tags = export.TagMap(getOut.Tags)
				if cfg.PackageType == lambdatypes.PackageTypeImage && getOut.Code != nil {
					fn.Spec.Code = awsv1alpha1.LambdaCodeSource{ImageURI: aws.ToString(getOut.Code.ImageUri)}
				}
			}
			if fn.Spec.Code.ImageURI == "" {
				fn.Spec.Code = awsv1alpha1.LambdaCodeSource{
					S3: &awsv1alpha1.LambdaS3Code{
						S3Bucket: "CHANGEME-code-bucket",
						S3Key:    name + ".zip",
					},
				}
			}

			opts.Index.Add(aws.ToString(cfg.FunctionArn), fn.Name)
			opts.Index.Add(name, fn.Name)
			objs = append(objs, fn)
		}
	}
	return objs, nil
}

func exportLambdaAliases(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	fp := awslambda.NewListFunctionsPaginator(clients.Lambda, &awslambda.ListFunctionsInput{})
	for fp.HasMorePages() {
		page, err := fp.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list functions: %w", err)
		}
		for _, cfg := range page.Functions {
			fnName := aws.ToString(cfg.FunctionName)
			ap := awslambda.NewListAliasesPaginator(clients.Lambda, &awslambda.ListAliasesInput{
				FunctionName: cfg.FunctionName,
			})
			for ap.HasMorePages() {
				apage, err := ap.NextPage(ctx)
				if err != nil {
					// Per-function failure: keep exporting other functions.
					break
				}
				for _, a := range apage.Aliases {
					alias := &awsv1alpha1.LambdaAlias{
						ObjectMeta: export.ObjectMeta(fnName+"-"+aws.ToString(a.Name), opts),
						Spec: awsv1alpha1.LambdaAliasSpec{
							FunctionName:    fnName,
							Name:            aws.ToString(a.Name),
							FunctionVersion: aws.ToString(a.FunctionVersion),
							Description:     aws.ToString(a.Description),
						},
					}
					if a.RoutingConfig != nil && len(a.RoutingConfig.AdditionalVersionWeights) > 0 {
						alias.Spec.RoutingConfig = &awsv1alpha1.LambdaAliasRoutingConfig{
							AdditionalVersionWeights: a.RoutingConfig.AdditionalVersionWeights,
						}
					}
					opts.Index.Add(aws.ToString(a.AliasArn), alias.Name)
					objs = append(objs, alias)
				}
			}
		}
	}
	return objs, nil
}

// lambdaPolicyDoc mirrors the shape of a Lambda resource-based policy.
type lambdaPolicyDoc struct {
	Statement []struct {
		Sid       string                                `json:"Sid"`
		Action    json.RawMessage                       `json:"Action"`
		Principal json.RawMessage                       `json:"Principal"`
		Condition map[string]map[string]json.RawMessage `json:"Condition"`
	} `json:"Statement"`
}

// lambdaFirstString decodes a JSON string or the first element of a JSON
// string array.
func lambdaFirstString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil && len(list) > 0 {
		return list[0]
	}
	return ""
}

// lambdaPrincipalString extracts the principal from a policy statement:
// either a bare string ("*") or {"Service": "..."} / {"AWS": "..."}.
func lambdaPrincipalString(raw json.RawMessage) string {
	if s := lambdaFirstString(raw); s != "" {
		return s
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err == nil {
		for _, key := range []string{"Service", "AWS", "Federated"} {
			if v, ok := m[key]; ok {
				if s := lambdaFirstString(v); s != "" {
					return s
				}
			}
		}
	}
	return ""
}

func exportLambdaPermissions(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	fp := awslambda.NewListFunctionsPaginator(clients.Lambda, &awslambda.ListFunctionsInput{})
	for fp.HasMorePages() {
		page, err := fp.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list functions: %w", err)
		}
		for _, cfg := range page.Functions {
			fnName := aws.ToString(cfg.FunctionName)
			polOut, err := clients.Lambda.GetPolicy(ctx, &awslambda.GetPolicyInput{FunctionName: cfg.FunctionName})
			if err != nil {
				// Most functions have no resource policy (ResourceNotFound);
				// skip and continue either way.
				continue
			}
			var doc lambdaPolicyDoc
			if err := json.Unmarshal([]byte(aws.ToString(polOut.Policy)), &doc); err != nil {
				continue
			}
			for _, st := range doc.Statement {
				perm := &awsv1alpha1.LambdaPermission{
					ObjectMeta: export.ObjectMeta(fnName+"-"+st.Sid, opts),
					Spec: awsv1alpha1.LambdaPermissionSpec{
						StatementId: st.Sid,
						Action:      lambdaFirstString(st.Action),
						Principal:   lambdaPrincipalString(st.Principal),
					},
				}
				if crName, ok := opts.Index.Lookup(aws.ToString(cfg.FunctionArn)); ok {
					perm.Spec.FunctionRef = &awsv1alpha1.LambdaFunctionRef{Name: crName}
				} else {
					perm.Spec.FunctionArn = aws.ToString(cfg.FunctionArn)
				}
				for op, kv := range st.Condition {
					for key, val := range kv {
						switch {
						case strings.EqualFold(key, "aws:SourceArn") && strings.EqualFold(op, "ArnLike"):
							perm.Spec.SourceArn = lambdaFirstString(val)
						case strings.EqualFold(key, "aws:SourceAccount") && strings.EqualFold(op, "StringEquals"):
							perm.Spec.SourceAccount = lambdaFirstString(val)
						}
					}
				}
				objs = append(objs, perm)
			}
		}
	}
	return objs, nil
}

func exportLambdaEventSourceMappings(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awslambda.NewListEventSourceMappingsPaginator(clients.Lambda, &awslambda.ListEventSourceMappingsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list event source mappings: %w", err)
		}
		for _, m := range page.EventSourceMappings {
			uuid := aws.ToString(m.UUID)
			fnArn := lambdaUnqualifiedArn(aws.ToString(m.FunctionArn))
			fnName := lambdaFuncNameFromArn(fnArn)
			short := uuid
			if len(short) > 8 {
				short = short[:8]
			}
			esm := &awsv1alpha1.LambdaEventSourceMapping{
				ObjectMeta: export.ObjectMeta(fnName+"-esm-"+short, opts),
				Spec: awsv1alpha1.LambdaEventSourceMappingSpec{
					EventSourceArn:                 aws.ToString(m.EventSourceArn),
					BatchSize:                      m.BatchSize,
					StartingPosition:               string(m.StartingPosition),
					MaximumBatchingWindowInSeconds: m.MaximumBatchingWindowInSeconds,
				},
			}
			if crName, ok := opts.Index.Lookup(fnArn); ok {
				esm.Spec.FunctionRef = &awsv1alpha1.LambdaFunctionRef{Name: crName}
			} else {
				esm.Spec.FunctionArn = aws.ToString(m.FunctionArn)
			}
			switch aws.ToString(m.State) {
			case "Disabled", "Disabling":
				esm.Spec.Enabled = aws.Bool(false)
			}
			if m.FilterCriteria != nil && len(m.FilterCriteria.Filters) > 0 {
				fc := &awsv1alpha1.LambdaFilterCriteria{}
				for _, f := range m.FilterCriteria.Filters {
					fc.Filters = append(fc.Filters, aws.ToString(f.Pattern))
				}
				esm.Spec.FilterCriteria = fc
			}
			opts.Index.Add(uuid, esm.Name)
			objs = append(objs, esm)
		}
	}
	return objs, nil
}

func exportLambdaFunctionURLs(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	fp := awslambda.NewListFunctionsPaginator(clients.Lambda, &awslambda.ListFunctionsInput{})
	for fp.HasMorePages() {
		page, err := fp.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list functions: %w", err)
		}
		for _, cfg := range page.Functions {
			fnName := aws.ToString(cfg.FunctionName)
			up := awslambda.NewListFunctionUrlConfigsPaginator(clients.Lambda, &awslambda.ListFunctionUrlConfigsInput{
				FunctionName: cfg.FunctionName,
			})
			for up.HasMorePages() {
				upage, err := up.NextPage(ctx)
				if err != nil {
					break
				}
				for _, u := range upage.FunctionUrlConfigs {
					// A qualified ARN means the URL targets an alias.
					qualifier := ""
					if parts := strings.Split(aws.ToString(u.FunctionArn), ":"); len(parts) == 8 {
						qualifier = parts[7]
					}
					crName := fnName + "-url"
					if qualifier != "" {
						crName += "-" + qualifier
					}
					fu := &awsv1alpha1.LambdaFunctionURL{
						ObjectMeta: export.ObjectMeta(crName, opts),
						Spec: awsv1alpha1.LambdaFunctionURLSpec{
							FunctionName: fnName,
							AuthType:     string(u.AuthType),
							Qualifier:    qualifier,
							InvokeMode:   string(u.InvokeMode),
						},
					}
					if c := u.Cors; c != nil {
						fu.Spec.CORS = &awsv1alpha1.LambdaURLCORSConfig{
							AllowCredentials: aws.ToBool(c.AllowCredentials),
							AllowHeaders:     c.AllowHeaders,
							AllowMethods:     c.AllowMethods,
							AllowOrigins:     c.AllowOrigins,
							ExposeHeaders:    c.ExposeHeaders,
							MaxAge:           c.MaxAge,
						}
					}
					opts.Index.Add(aws.ToString(u.FunctionUrl), fu.Name)
					objs = append(objs, fu)
				}
			}
		}
	}
	return objs, nil
}

func exportLambdaLayerVersions(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awslambda.NewListLayersPaginator(clients.Lambda, &awslambda.ListLayersInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list layers: %w", err)
		}
		for _, l := range page.Layers {
			lv := l.LatestMatchingVersion
			if lv == nil {
				continue
			}
			layerName := aws.ToString(l.LayerName)
			cr := &awsv1alpha1.LambdaLayerVersion{
				ObjectMeta: export.ObjectMeta(layerName, opts),
				Spec: awsv1alpha1.LambdaLayerVersionSpec{
					LayerName: layerName,
					// GetLayerVersion returns a presigned download URL, not
					// the original S3 bucket/key: placeholder for the importer.
					Content: awsv1alpha1.LambdaLayerContent{
						S3Bucket: "CHANGEME-code-bucket",
						S3Key:    layerName + ".zip",
					},
					Description: aws.ToString(lv.Description),
					LicenseInfo: aws.ToString(lv.LicenseInfo),
				},
			}
			for _, r := range lv.CompatibleRuntimes {
				cr.Spec.CompatibleRuntimes = append(cr.Spec.CompatibleRuntimes, string(r))
			}
			for _, a := range lv.CompatibleArchitectures {
				cr.Spec.CompatibleArchitectures = append(cr.Spec.CompatibleArchitectures, string(a))
			}
			opts.Index.Add(aws.ToString(l.LayerArn), cr.Name)
			opts.Index.Add(aws.ToString(lv.LayerVersionArn), cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportLambdaCodeSigningConfigs(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awslambda.NewListCodeSigningConfigsPaginator(clients.Lambda, &awslambda.ListCodeSigningConfigsInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list code signing configs: %w", err)
		}
		for _, c := range page.CodeSigningConfigs {
			id := aws.ToString(c.CodeSigningConfigId)
			csc := &awsv1alpha1.LambdaCodeSigningConfig{
				ObjectMeta: export.ObjectMeta(id, opts),
				Spec: awsv1alpha1.LambdaCodeSigningConfigSpec{
					Description: aws.ToString(c.Description),
				},
			}
			if c.AllowedPublishers != nil {
				csc.Spec.AllowedPublisherARNs = c.AllowedPublishers.SigningProfileVersionArns
			}
			if c.CodeSigningPolicies != nil {
				csc.Spec.UntrustedArtifactOnDeployment = string(c.CodeSigningPolicies.UntrustedArtifactOnDeployment)
			}
			opts.Index.Add(aws.ToString(c.CodeSigningConfigArn), csc.Name)
			objs = append(objs, csc)
		}
	}
	return objs, nil
}
