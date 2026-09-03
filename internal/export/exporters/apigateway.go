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
	awsapigw "github.com/aws/aws-sdk-go-v2/service/apigateway"
	awsapigwv2 "github.com/aws/aws-sdk-go-v2/service/apigatewayv2"
	apigwv2types "github.com/aws/aws-sdk-go-v2/service/apigatewayv2/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	awsclient "github.com/konfig-io/konfig-konector/internal/aws"
	"github.com/konfig-io/konfig-konector/internal/export"
)

func init() {
	// APIs (and standalone kinds) first so routes/integrations/stages can
	// reference them by CR name via the index.
	export.Register(export.Exporter{Kind: "APIGatewayV2API", Service: "apigatewayv2", Order: 80, Fn: exportAPIGatewayV2APIs})
	export.Register(export.Exporter{Kind: "APIGatewayV2VpcLink", Service: "apigatewayv2", Order: 80, Fn: exportAPIGatewayV2VpcLinks})
	export.Register(export.Exporter{Kind: "APIGatewayV2DomainName", Service: "apigatewayv2", Order: 80, Fn: exportAPIGatewayV2DomainNames})
	export.Register(export.Exporter{Kind: "RestAPI", Service: "apigateway", Order: 80, Fn: exportRestAPIs})
	export.Register(export.Exporter{Kind: "APIGatewayV2Integration", Service: "apigatewayv2", Order: 81, Fn: exportAPIGatewayV2Integrations})
	export.Register(export.Exporter{Kind: "APIGatewayV2Authorizer", Service: "apigatewayv2", Order: 81, Fn: exportAPIGatewayV2Authorizers})
	export.Register(export.Exporter{Kind: "APIGatewayV2Route", Service: "apigatewayv2", Order: 82, Fn: exportAPIGatewayV2Routes})
	export.Register(export.Exporter{Kind: "APIGatewayV2Stage", Service: "apigatewayv2", Order: 82, Fn: exportAPIGatewayV2Stages})
	export.Register(export.Exporter{Kind: "APIGatewayV2ApiMapping", Service: "apigatewayv2", Order: 83, Fn: exportAPIGatewayV2ApiMappings})
	export.Register(export.Exporter{Kind: "RestAPIStage", Service: "apigateway", Order: 84, Fn: exportRestAPIStages})
}

// listV2APIs pages through GetApis (the v2 SDK has no paginator for it).
func listV2APIs(ctx context.Context, clients *awsclient.Clients) ([]apigwv2types.Api, error) {
	var apis []apigwv2types.Api
	var next *string
	for {
		page, err := clients.APIGatewayV2.GetApis(ctx, &awsapigwv2.GetApisInput{NextToken: next})
		if err != nil {
			return nil, fmt.Errorf("get apis: %w", err)
		}
		apis = append(apis, page.Items...)
		if page.NextToken == nil || *page.NextToken == "" {
			return apis, nil
		}
		next = page.NextToken
	}
}

func exportAPIGatewayV2APIs(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	apis, err := listV2APIs(ctx, clients)
	if err != nil {
		return nil, err
	}
	var objs []client.Object
	for _, api := range apis {
		cr := &awsv1alpha1.APIGatewayV2API{
			ObjectMeta: export.ObjectMeta(aws.ToString(api.Name), opts),
			Spec: awsv1alpha1.APIGatewayV2APISpec{
				Name:                      aws.ToString(api.Name),
				ProtocolType:              string(api.ProtocolType),
				Description:               aws.ToString(api.Description),
				RouteSelectionExpression:  aws.ToString(api.RouteSelectionExpression),
				APIKeySelectionExpression: aws.ToString(api.ApiKeySelectionExpression),
				DisableExecuteAPIEndpoint: aws.ToBool(api.DisableExecuteApiEndpoint),
				Tags:                      export.TagMap(api.Tags),
			},
		}
		if api.CorsConfiguration != nil {
			cr.Spec.CORSConfiguration = &awsv1alpha1.APIGatewayV2CorsConfiguration{
				AllowCredentials: api.CorsConfiguration.AllowCredentials,
				AllowHeaders:     api.CorsConfiguration.AllowHeaders,
				AllowMethods:     api.CorsConfiguration.AllowMethods,
				AllowOrigins:     api.CorsConfiguration.AllowOrigins,
				ExposeHeaders:    api.CorsConfiguration.ExposeHeaders,
				MaxAge:           api.CorsConfiguration.MaxAge,
			}
		}
		opts.Index.Add(aws.ToString(api.ApiId), cr.Name)
		objs = append(objs, cr)
	}
	return objs, nil
}

// v2APIRef builds an APIRef preferring the exported CR name over the raw ID.
func v2APIRef(apiID string, opts *export.Options) awsv1alpha1.APIRef {
	if name, ok := opts.Index.Lookup(apiID); ok {
		return awsv1alpha1.APIRef{Name: name}
	}
	return awsv1alpha1.APIRef{APIID: apiID}
}

func exportAPIGatewayV2Integrations(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	apis, err := listV2APIs(ctx, clients)
	if err != nil {
		return nil, err
	}
	var objs []client.Object
	for _, api := range apis {
		apiID := aws.ToString(api.ApiId)
		var next *string
		for {
			page, err := clients.APIGatewayV2.GetIntegrations(ctx, &awsapigwv2.GetIntegrationsInput{
				ApiId: aws.String(apiID), NextToken: next,
			})
			if err != nil {
				// Per-API errors: skip this API's integrations.
				break
			}
			for _, integ := range page.Items {
				if aws.ToBool(integ.ApiGatewayManaged) {
					continue
				}
				integID := aws.ToString(integ.IntegrationId)
				cr := &awsv1alpha1.APIGatewayV2Integration{
					ObjectMeta: export.ObjectMeta(fmt.Sprintf("%s-%s", aws.ToString(api.Name), integID), opts),
					Spec: awsv1alpha1.APIGatewayV2IntegrationSpec{
						APIRef:               v2APIRef(apiID, opts),
						IntegrationType:      string(integ.IntegrationType),
						IntegrationMethod:    aws.ToString(integ.IntegrationMethod),
						PayloadFormatVersion: aws.ToString(integ.PayloadFormatVersion),
						Description:          aws.ToString(integ.Description),
						ConnectionType:       string(integ.ConnectionType),
						CredentialsARN:       aws.ToString(integ.CredentialsArn),
						RequestParameters:    integ.RequestParameters,
						TimeoutInMillis:      integ.TimeoutInMillis,
					},
				}
				uri := aws.ToString(integ.IntegrationUri)
				if name, ok := opts.Index.Lookup(uri); ok {
					cr.Spec.FunctionRef = &awsv1alpha1.LambdaFunctionRef{Name: name}
				} else {
					cr.Spec.IntegrationURI = uri
				}
				if connID := aws.ToString(integ.ConnectionId); connID != "" {
					if name, ok := opts.Index.Lookup(connID); ok {
						cr.Spec.VPCLinkRef = &awsv1alpha1.ResourceRef{Name: name}
					} else {
						cr.Spec.ConnectionID = connID
					}
				}
				opts.Index.Add(integID, cr.Name)
				objs = append(objs, cr)
			}
			if page.NextToken == nil || *page.NextToken == "" {
				break
			}
			next = page.NextToken
		}
	}
	return objs, nil
}

func exportAPIGatewayV2Routes(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	apis, err := listV2APIs(ctx, clients)
	if err != nil {
		return nil, err
	}
	var objs []client.Object
	for _, api := range apis {
		apiID := aws.ToString(api.ApiId)
		var next *string
		for {
			page, err := clients.APIGatewayV2.GetRoutes(ctx, &awsapigwv2.GetRoutesInput{
				ApiId: aws.String(apiID), NextToken: next,
			})
			if err != nil {
				break
			}
			for _, route := range page.Items {
				if aws.ToBool(route.ApiGatewayManaged) {
					continue
				}
				cr := &awsv1alpha1.APIGatewayV2Route{
					ObjectMeta: export.ObjectMeta(fmt.Sprintf("%s-%s", aws.ToString(api.Name), aws.ToString(route.RouteKey)), opts),
					Spec: awsv1alpha1.APIGatewayV2RouteSpec{
						APIRef:              v2APIRef(apiID, opts),
						RouteKey:            aws.ToString(route.RouteKey),
						AuthorizationType:   string(route.AuthorizationType),
						AuthorizationScopes: route.AuthorizationScopes,
						APIKeyRequired:      aws.ToBool(route.ApiKeyRequired),
						OperationName:       aws.ToString(route.OperationName),
					},
				}
				if cr.Spec.AuthorizationType == string(apigwv2types.AuthorizationTypeNone) {
					cr.Spec.AuthorizationType = ""
				}
				if target := aws.ToString(route.Target); target != "" {
					integID := strings.TrimPrefix(target, "integrations/")
					if name, ok := opts.Index.Lookup(integID); ok {
						cr.Spec.IntegrationRef = &awsv1alpha1.IntegrationRef{Name: name}
					} else {
						cr.Spec.Target = target
					}
				}
				if authID := aws.ToString(route.AuthorizerId); authID != "" {
					if name, ok := opts.Index.Lookup(authID); ok {
						cr.Spec.AuthorizerRef = &awsv1alpha1.AuthorizerRef{Name: name}
					} else {
						cr.Spec.AuthorizerRef = &awsv1alpha1.AuthorizerRef{AuthorizerID: authID}
					}
				}
				objs = append(objs, cr)
			}
			if page.NextToken == nil || *page.NextToken == "" {
				break
			}
			next = page.NextToken
		}
	}
	return objs, nil
}

func exportAPIGatewayV2Stages(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	apis, err := listV2APIs(ctx, clients)
	if err != nil {
		return nil, err
	}
	var objs []client.Object
	for _, api := range apis {
		apiID := aws.ToString(api.ApiId)
		var next *string
		for {
			page, err := clients.APIGatewayV2.GetStages(ctx, &awsapigwv2.GetStagesInput{
				ApiId: aws.String(apiID), NextToken: next,
			})
			if err != nil {
				break
			}
			for _, stage := range page.Items {
				if aws.ToBool(stage.ApiGatewayManaged) {
					continue
				}
				cr := &awsv1alpha1.APIGatewayV2Stage{
					ObjectMeta: export.ObjectMeta(fmt.Sprintf("%s-%s", aws.ToString(api.Name), aws.ToString(stage.StageName)), opts),
					Spec: awsv1alpha1.APIGatewayV2StageSpec{
						APIRef:         v2APIRef(apiID, opts),
						StageName:      aws.ToString(stage.StageName),
						AutoDeploy:     aws.ToBool(stage.AutoDeploy),
						Description:    aws.ToString(stage.Description),
						StageVariables: stage.StageVariables,
						Tags:           export.TagMap(stage.Tags),
					},
				}
				if rs := stage.DefaultRouteSettings; rs != nil &&
					(rs.ThrottlingBurstLimit != nil || rs.ThrottlingRateLimit != nil || rs.DetailedMetricsEnabled != nil) {
					cr.Spec.DefaultRouteSettings = &awsv1alpha1.APIGatewayV2RouteSettings{
						ThrottlingBurstLimit:   rs.ThrottlingBurstLimit,
						ThrottlingRateLimit:    rs.ThrottlingRateLimit,
						DetailedMetricsEnabled: rs.DetailedMetricsEnabled,
					}
				}
				objs = append(objs, cr)
			}
			if page.NextToken == nil || *page.NextToken == "" {
				break
			}
			next = page.NextToken
		}
	}
	return objs, nil
}

func exportAPIGatewayV2Authorizers(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	apis, err := listV2APIs(ctx, clients)
	if err != nil {
		return nil, err
	}
	var objs []client.Object
	for _, api := range apis {
		apiID := aws.ToString(api.ApiId)
		var next *string
		for {
			page, err := clients.APIGatewayV2.GetAuthorizers(ctx, &awsapigwv2.GetAuthorizersInput{
				ApiId: aws.String(apiID), NextToken: next,
			})
			if err != nil {
				break
			}
			for _, auth := range page.Items {
				cr := &awsv1alpha1.APIGatewayV2Authorizer{
					ObjectMeta: export.ObjectMeta(fmt.Sprintf("%s-%s", aws.ToString(api.Name), aws.ToString(auth.Name)), opts),
					Spec: awsv1alpha1.APIGatewayV2AuthorizerSpec{
						APIRef:                         v2APIRef(apiID, opts),
						Name:                           aws.ToString(auth.Name),
						AuthorizerType:                 string(auth.AuthorizerType),
						IdentitySource:                 auth.IdentitySource,
						AuthorizerURI:                  aws.ToString(auth.AuthorizerUri),
						AuthorizerPayloadFormatVersion: aws.ToString(auth.AuthorizerPayloadFormatVersion),
						AuthorizerResultTTLInSeconds:   auth.AuthorizerResultTtlInSeconds,
						EnableSimpleResponses:          aws.ToBool(auth.EnableSimpleResponses),
						AuthorizerCredentialsARN:       aws.ToString(auth.AuthorizerCredentialsArn),
					},
				}
				if auth.JwtConfiguration != nil {
					cr.Spec.JWTConfiguration = &awsv1alpha1.JWTConfiguration{
						Issuer:   aws.ToString(auth.JwtConfiguration.Issuer),
						Audience: auth.JwtConfiguration.Audience,
					}
				}
				opts.Index.Add(aws.ToString(auth.AuthorizerId), cr.Name)
				objs = append(objs, cr)
			}
			if page.NextToken == nil || *page.NextToken == "" {
				break
			}
			next = page.NextToken
		}
	}
	return objs, nil
}

func exportAPIGatewayV2DomainNames(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	var next *string
	for {
		page, err := clients.APIGatewayV2.GetDomainNames(ctx, &awsapigwv2.GetDomainNamesInput{NextToken: next})
		if err != nil {
			return nil, fmt.Errorf("get domain names: %w", err)
		}
		for _, dn := range page.Items {
			cr := &awsv1alpha1.APIGatewayV2DomainName{
				ObjectMeta: export.ObjectMeta(aws.ToString(dn.DomainName), opts),
				Spec: awsv1alpha1.APIGatewayV2DomainNameSpec{
					DomainName: aws.ToString(dn.DomainName),
					Tags:       export.TagMap(dn.Tags),
				},
			}
			if len(dn.DomainNameConfigurations) > 0 {
				cfg := dn.DomainNameConfigurations[0]
				cr.Spec.CertificateARN = aws.ToString(cfg.CertificateArn)
				cr.Spec.EndpointType = string(cfg.EndpointType)
				cr.Spec.SecurityPolicy = string(cfg.SecurityPolicy)
			}
			opts.Index.Add(aws.ToString(dn.DomainName), cr.Name)
			objs = append(objs, cr)
		}
		if page.NextToken == nil || *page.NextToken == "" {
			return objs, nil
		}
		next = page.NextToken
	}
}

func exportAPIGatewayV2ApiMappings(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var domains []string
	var next *string
	for {
		page, err := clients.APIGatewayV2.GetDomainNames(ctx, &awsapigwv2.GetDomainNamesInput{NextToken: next})
		if err != nil {
			return nil, fmt.Errorf("get domain names: %w", err)
		}
		for _, dn := range page.Items {
			domains = append(domains, aws.ToString(dn.DomainName))
		}
		if page.NextToken == nil || *page.NextToken == "" {
			break
		}
		next = page.NextToken
	}

	var objs []client.Object
	for _, domain := range domains {
		var mnext *string
		for {
			page, err := clients.APIGatewayV2.GetApiMappings(ctx, &awsapigwv2.GetApiMappingsInput{
				DomainName: aws.String(domain), NextToken: mnext,
			})
			if err != nil {
				break
			}
			for _, m := range page.Items {
				cr := &awsv1alpha1.APIGatewayV2ApiMapping{
					ObjectMeta: export.ObjectMeta(fmt.Sprintf("%s-%s", domain, aws.ToString(m.Stage)), opts),
					Spec: awsv1alpha1.APIGatewayV2ApiMappingSpec{
						APIRef:        v2APIRef(aws.ToString(m.ApiId), opts),
						Stage:         aws.ToString(m.Stage),
						APIMappingKey: aws.ToString(m.ApiMappingKey),
					},
				}
				if name, ok := opts.Index.Lookup(domain); ok {
					cr.Spec.DomainNameRef = awsv1alpha1.DomainNameRef{Name: name}
				} else {
					cr.Spec.DomainNameRef = awsv1alpha1.DomainNameRef{DomainName: domain}
				}
				objs = append(objs, cr)
			}
			if page.NextToken == nil || *page.NextToken == "" {
				break
			}
			mnext = page.NextToken
		}
	}
	return objs, nil
}

func exportAPIGatewayV2VpcLinks(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	var next *string
	for {
		page, err := clients.APIGatewayV2.GetVpcLinks(ctx, &awsapigwv2.GetVpcLinksInput{NextToken: next})
		if err != nil {
			return nil, fmt.Errorf("get vpc links: %w", err)
		}
		for _, vl := range page.Items {
			cr := &awsv1alpha1.APIGatewayV2VpcLink{
				ObjectMeta: export.ObjectMeta(aws.ToString(vl.Name), opts),
				Spec: awsv1alpha1.APIGatewayV2VpcLinkSpec{
					Name: aws.ToString(vl.Name),
					Tags: export.TagMap(vl.Tags),
				},
			}
			for _, id := range vl.SubnetIds {
				if name, ok := opts.Index.Lookup(id); ok {
					cr.Spec.SubnetRefs = append(cr.Spec.SubnetRefs, awsv1alpha1.SubnetRef{Name: name})
				} else {
					cr.Spec.SubnetRefs = append(cr.Spec.SubnetRefs, awsv1alpha1.SubnetRef{ID: id})
				}
			}
			for _, id := range vl.SecurityGroupIds {
				if name, ok := opts.Index.Lookup(id); ok {
					cr.Spec.SecurityGroupRefs = append(cr.Spec.SecurityGroupRefs, awsv1alpha1.SecurityGroupRef{Name: name})
				} else {
					cr.Spec.SecurityGroupRefs = append(cr.Spec.SecurityGroupRefs, awsv1alpha1.SecurityGroupRef{ID: id})
				}
			}
			opts.Index.Add(aws.ToString(vl.VpcLinkId), cr.Name)
			objs = append(objs, cr)
		}
		if page.NextToken == nil || *page.NextToken == "" {
			return objs, nil
		}
		next = page.NextToken
	}
}

func exportRestAPIs(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var objs []client.Object
	p := awsapigw.NewGetRestApisPaginator(clients.APIGateway, &awsapigw.GetRestApisInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("get rest apis: %w", err)
		}
		for _, api := range page.Items {
			apiID := aws.ToString(api.Id)
			cr := &awsv1alpha1.RestAPI{
				ObjectMeta: export.ObjectMeta(aws.ToString(api.Name), opts),
				Spec: awsv1alpha1.RestAPISpec{
					Name:                      aws.ToString(api.Name),
					Description:               aws.ToString(api.Description),
					DisableExecuteAPIEndpoint: api.DisableExecuteApiEndpoint,
					MinimumCompressionSize:    api.MinimumCompressionSize,
					Policy:                    aws.ToString(api.Policy),
					Tags:                      export.TagMap(api.Tags),
				},
			}
			if api.EndpointConfiguration != nil {
				for _, t := range api.EndpointConfiguration.Types {
					cr.Spec.EndpointTypes = append(cr.Spec.EndpointTypes, string(t))
				}
			}
			// Export the OpenAPI body via GetExport where feasible. GetExport
			// requires a stage, so use the API's first stage if one exists;
			// otherwise export without a body.
			if stagesOut, err := clients.APIGateway.GetStages(ctx, &awsapigw.GetStagesInput{
				RestApiId: aws.String(apiID),
			}); err == nil && len(stagesOut.Item) > 0 {
				exp, expErr := clients.APIGateway.GetExport(ctx, &awsapigw.GetExportInput{
					RestApiId:  aws.String(apiID),
					StageName:  stagesOut.Item[0].StageName,
					ExportType: aws.String("oas30"),
					Accepts:    aws.String("application/json"),
					Parameters: map[string]string{"extensions": "integrations"},
				})
				if expErr == nil {
					cr.Spec.Body = string(exp.Body)
				}
			}
			opts.Index.Add(apiID, cr.Name)
			objs = append(objs, cr)
		}
	}
	return objs, nil
}

func exportRestAPIStages(ctx context.Context, clients *awsclient.Clients, opts *export.Options) ([]client.Object, error) {
	var apiIDs []string
	apiNames := map[string]string{}
	p := awsapigw.NewGetRestApisPaginator(clients.APIGateway, &awsapigw.GetRestApisInput{})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("get rest apis: %w", err)
		}
		for _, api := range page.Items {
			apiIDs = append(apiIDs, aws.ToString(api.Id))
			apiNames[aws.ToString(api.Id)] = aws.ToString(api.Name)
		}
	}

	var objs []client.Object
	for _, apiID := range apiIDs {
		stagesOut, err := clients.APIGateway.GetStages(ctx, &awsapigw.GetStagesInput{
			RestApiId: aws.String(apiID),
		})
		if err != nil {
			continue
		}
		for _, stage := range stagesOut.Item {
			ref := awsv1alpha1.APIRef{APIID: apiID}
			if name, ok := opts.Index.Lookup(apiID); ok {
				ref = awsv1alpha1.APIRef{Name: name}
			}
			cr := &awsv1alpha1.RestAPIStage{
				ObjectMeta: export.ObjectMeta(fmt.Sprintf("%s-%s", apiNames[apiID], aws.ToString(stage.StageName)), opts),
				Spec: awsv1alpha1.RestAPIStageSpec{
					RestAPIRef: ref,
					StageName:  aws.ToString(stage.StageName),
					// Deployments are immutable snapshots and are not exported
					// as CRs; reference the live deployment by raw ID.
					DeploymentRef:  awsv1alpha1.RestAPIDeploymentRef{DeploymentID: aws.ToString(stage.DeploymentId)},
					Description:    aws.ToString(stage.Description),
					Variables:      stage.Variables,
					TracingEnabled: stage.TracingEnabled,
					Tags:           export.TagMap(stage.Tags),
				},
			}
			objs = append(objs, cr)
		}
	}
	return objs, nil
}
