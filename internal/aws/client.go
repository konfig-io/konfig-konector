/*
Copyright 2024.

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

// Package aws provides shared AWS SDK configuration and client factories for
// konfig-konector controllers. Authentication uses EKS Pod Identity by default,
// falling back to IRSA and then the standard credential chain.
package aws

import (
	"context"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	"github.com/aws/aws-sdk-go-v2/service/acmpca"
	"github.com/aws/aws-sdk-go-v2/service/amp"
	"github.com/aws/aws-sdk-go-v2/service/apigateway"
	"github.com/aws/aws-sdk-go-v2/service/apigatewayv2"
	"github.com/aws/aws-sdk-go-v2/service/applicationautoscaling"
	"github.com/aws/aws-sdk-go-v2/service/apprunner"
	"github.com/aws/aws-sdk-go-v2/service/athena"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/aws/aws-sdk-go-v2/service/backup"
	"github.com/aws/aws-sdk-go-v2/service/batch"
	"github.com/aws/aws-sdk-go-v2/service/budgets"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/codeartifact"
	"github.com/aws/aws-sdk-go-v2/service/codebuild"
	"github.com/aws/aws-sdk-go-v2/service/codecommit"
	"github.com/aws/aws-sdk-go-v2/service/codedeploy"
	"github.com/aws/aws-sdk-go-v2/service/codepipeline"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/configservice"
	"github.com/aws/aws-sdk-go-v2/service/controltower"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/dax"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/efs"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/aws/aws-sdk-go-v2/service/firehose"
	"github.com/aws/aws-sdk-go-v2/service/glue"
	"github.com/aws/aws-sdk-go-v2/service/grafana"
	"github.com/aws/aws-sdk-go-v2/service/guardduty"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/inspector2"
	"github.com/aws/aws-sdk-go-v2/service/kafka"
	"github.com/aws/aws-sdk-go-v2/service/kinesis"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/memorydb"
	"github.com/aws/aws-sdk-go-v2/service/mq"
	"github.com/aws/aws-sdk-go-v2/service/networkfirewall"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/opensearchserverless"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
	"github.com/aws/aws-sdk-go-v2/service/pipes"
	"github.com/aws/aws-sdk-go-v2/service/ram"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53resolver"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3control"
	"github.com/aws/aws-sdk-go-v2/service/scheduler"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/securityhub"
	"github.com/aws/aws-sdk-go-v2/service/servicecatalog"
	"github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sfn"
	"github.com/aws/aws-sdk-go-v2/service/shield"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssoadmin"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice"
	"github.com/aws/aws-sdk-go-v2/service/wafv2"
	"github.com/aws/aws-sdk-go-v2/service/xray"

	ec2helper "github.com/konfig-io/konfig-konector/internal/aws/ec2"
	"github.com/konfig-io/konfig-konector/internal/aws/multi"
)

// Clients holds initialised AWS service clients shared across all controllers.
type Clients struct {
	IAM                  *multi.IAM
	Route53              *multi.Route53
	EKS                  *multi.EKS
	EC2                  *multi.EC2
	RDS                  *multi.RDS
	AutoScaling          *multi.AutoScaling
	S3                   *multi.S3
	SQS                  *multi.SQS
	SNS                  *multi.SNS
	ElastiCache          *multi.ElastiCache
	MemoryDB             *multi.MemoryDB
	Lambda               *multi.Lambda
	ECS                  *multi.ECS
	ACM                  *multi.ACM
	ELBv2                *multi.ELBv2
	ECR                  *multi.ECR
	EFS                  *multi.EFS
	CloudFront           *multi.CloudFront
	CloudWatch           *multi.CloudWatch
	CloudWatchLogs       *multi.CloudWatchLogs
	KMS                  *multi.KMS
	SecretsManager       *multi.SecretsManager
	EventBridge          *multi.EventBridge
	Pipes                *multi.Pipes
	SESv2                *multi.SESv2
	DynamoDB             *multi.DynamoDB
	DAX                  *multi.DAX
	Cognito              *multi.Cognito
	CloudFormation       *multi.CloudFormation
	CodeBuild            *multi.CodeBuild
	CodeCommit           *multi.CodeCommit
	CodeDeploy           *multi.CodeDeploy
	CodePipeline         *multi.CodePipeline
	Firehose             *multi.Firehose
	Kafka                *multi.Kafka
	Kinesis              *multi.Kinesis
	SFN                  *multi.SFN
	OpenSearch           *multi.OpenSearch
	OpenSearchServerless *multi.OpenSearchServerless
	Route53Resolver      *multi.Route53Resolver
	Shield               *multi.Shield
	WAFv2                *multi.WAFv2
	SSM                  *multi.SSM
	APIGateway           *multi.APIGateway
	APIGatewayV2         *multi.APIGatewayV2
	CloudTrail           *multi.CloudTrail
	ConfigService        *multi.ConfigService
	Backup               *multi.Backup
	AppAutoScaling       *multi.AppAutoScaling
	Scheduler            *multi.Scheduler
	GuardDuty            *multi.GuardDuty
	SecurityHub          *multi.SecurityHub
	Inspector2           *multi.Inspector2
	Glue                 *multi.Glue
	Athena               *multi.Athena
	Redshift             *multi.Redshift
	MQ                   *multi.MQ
	Batch                *multi.Batch
	AppRunner            *multi.AppRunner
	ACMPCA               *multi.ACMPCA
	ServiceDiscovery     *multi.ServiceDiscovery
	NetworkFirewall      *multi.NetworkFirewall
	VPCLattice           *multi.VPCLattice
	CodeArtifact         *multi.CodeArtifact
	XRay                 *multi.XRay
	AMP                  *multi.AMP
	Grafana              *multi.Grafana
	Budgets              *multi.Budgets
	CostExplorer         *multi.CostExplorer
	Organizations        *multi.Organizations
	SSOAdmin             *multi.SSOAdmin
	RAM                  *multi.RAM
	ServiceCatalog       *multi.ServiceCatalog
	ControlTower         *multi.ControlTower
	S3Control            *multi.S3Control
	STS                  *multi.STS
	CloudControl         *multi.CloudControl

	// Config is the resolved base SDK config (operator credentials + region).
	Config aws.Config
}

// NewClients loads the default AWS configuration and returns service clients.
// The region is read from the AWS_REGION environment variable; if unset it falls
// back to AWS_DEFAULT_REGION and finally the SDK's own region resolution.
func NewClients(ctx context.Context) (*Clients, error) {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
	}

	opts := []func(*config.LoadOptions) error{
		config.WithCredentialsCacheOptions(func(o *aws.CredentialsCacheOptions) {
			// Refresh credentials 2 minutes before they expire so the operator
			// never makes API calls with stale EKS Pod Identity tokens.
			o.ExpiryWindow = 2 * time.Minute
		}),
		// Adaptive retry client-side rate limits on throttle responses. With
		// 148 controllers sharing one credential set, the standard retryer
		// alone lets synchronized resyncs hammer low-rate-limit services
		// (IAM, CloudFront, Route53).
		config.WithRetryMode(aws.RetryModeAdaptive),
		config.WithRetryMaxAttempts(5),
	}
	if region != "" {
		opts = append(opts, config.WithRegion(region))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, err
	}

	return &Clients{
		IAM:                  multi.NewIAM(iam.NewFromConfig(cfg)),
		Route53:              multi.NewRoute53(route53.NewFromConfig(cfg)),
		EKS:                  multi.NewEKS(eks.NewFromConfig(cfg)),
		EC2:                  multi.NewEC2(ec2.NewFromConfig(cfg, func(o *ec2.Options) { o.APIOptions = append(o.APIOptions, ec2helper.StripEmptyTagSpecifications) })),
		RDS:                  multi.NewRDS(rds.NewFromConfig(cfg)),
		AutoScaling:          multi.NewAutoScaling(autoscaling.NewFromConfig(cfg)),
		S3:                   multi.NewS3(s3.NewFromConfig(cfg)),
		SQS:                  multi.NewSQS(sqs.NewFromConfig(cfg)),
		SNS:                  multi.NewSNS(sns.NewFromConfig(cfg)),
		ElastiCache:          multi.NewElastiCache(elasticache.NewFromConfig(cfg)),
		MemoryDB:             multi.NewMemoryDB(memorydb.NewFromConfig(cfg)),
		Lambda:               multi.NewLambda(lambda.NewFromConfig(cfg)),
		ECS:                  multi.NewECS(ecs.NewFromConfig(cfg)),
		ACM:                  multi.NewACM(acm.NewFromConfig(cfg)),
		ELBv2:                multi.NewELBv2(elasticloadbalancingv2.NewFromConfig(cfg)),
		ECR:                  multi.NewECR(ecr.NewFromConfig(cfg)),
		EFS:                  multi.NewEFS(efs.NewFromConfig(cfg)),
		CloudFront:           multi.NewCloudFront(cloudfront.NewFromConfig(cfg)),
		CloudWatch:           multi.NewCloudWatch(cloudwatch.NewFromConfig(cfg)),
		CloudWatchLogs:       multi.NewCloudWatchLogs(cloudwatchlogs.NewFromConfig(cfg)),
		KMS:                  multi.NewKMS(kms.NewFromConfig(cfg)),
		SecretsManager:       multi.NewSecretsManager(secretsmanager.NewFromConfig(cfg)),
		EventBridge:          multi.NewEventBridge(eventbridge.NewFromConfig(cfg)),
		Pipes:                multi.NewPipes(pipes.NewFromConfig(cfg)),
		SESv2:                multi.NewSESv2(sesv2.NewFromConfig(cfg)),
		DynamoDB:             multi.NewDynamoDB(dynamodb.NewFromConfig(cfg)),
		DAX:                  multi.NewDAX(dax.NewFromConfig(cfg)),
		Cognito:              multi.NewCognito(cognitoidentityprovider.NewFromConfig(cfg)),
		CloudFormation:       multi.NewCloudFormation(cloudformation.NewFromConfig(cfg)),
		CodeBuild:            multi.NewCodeBuild(codebuild.NewFromConfig(cfg)),
		CodeCommit:           multi.NewCodeCommit(codecommit.NewFromConfig(cfg)),
		CodeDeploy:           multi.NewCodeDeploy(codedeploy.NewFromConfig(cfg)),
		CodePipeline:         multi.NewCodePipeline(codepipeline.NewFromConfig(cfg)),
		Firehose:             multi.NewFirehose(firehose.NewFromConfig(cfg)),
		Kafka:                multi.NewKafka(kafka.NewFromConfig(cfg)),
		Kinesis:              multi.NewKinesis(kinesis.NewFromConfig(cfg)),
		SFN:                  multi.NewSFN(sfn.NewFromConfig(cfg)),
		OpenSearch:           multi.NewOpenSearch(opensearch.NewFromConfig(cfg)),
		OpenSearchServerless: multi.NewOpenSearchServerless(opensearchserverless.NewFromConfig(cfg)),
		Route53Resolver:      multi.NewRoute53Resolver(route53resolver.NewFromConfig(cfg)),
		Shield:               multi.NewShield(shield.NewFromConfig(cfg)),
		WAFv2:                multi.NewWAFv2(wafv2.NewFromConfig(cfg)),
		SSM:                  multi.NewSSM(ssm.NewFromConfig(cfg)),
		APIGateway:           multi.NewAPIGateway(apigateway.NewFromConfig(cfg)),
		APIGatewayV2:         multi.NewAPIGatewayV2(apigatewayv2.NewFromConfig(cfg)),
		CloudTrail:           multi.NewCloudTrail(cloudtrail.NewFromConfig(cfg)),
		ConfigService:        multi.NewConfigService(configservice.NewFromConfig(cfg)),
		Backup:               multi.NewBackup(backup.NewFromConfig(cfg)),
		AppAutoScaling:       multi.NewAppAutoScaling(applicationautoscaling.NewFromConfig(cfg)),
		Scheduler:            multi.NewScheduler(scheduler.NewFromConfig(cfg)),
		GuardDuty:            multi.NewGuardDuty(guardduty.NewFromConfig(cfg)),
		SecurityHub:          multi.NewSecurityHub(securityhub.NewFromConfig(cfg)),
		Inspector2:           multi.NewInspector2(inspector2.NewFromConfig(cfg)),
		Glue:                 multi.NewGlue(glue.NewFromConfig(cfg)),
		Athena:               multi.NewAthena(athena.NewFromConfig(cfg)),
		Redshift:             multi.NewRedshift(redshift.NewFromConfig(cfg)),
		MQ:                   multi.NewMQ(mq.NewFromConfig(cfg)),
		Batch:                multi.NewBatch(batch.NewFromConfig(cfg)),
		AppRunner:            multi.NewAppRunner(apprunner.NewFromConfig(cfg)),
		ACMPCA:               multi.NewACMPCA(acmpca.NewFromConfig(cfg)),
		ServiceDiscovery:     multi.NewServiceDiscovery(servicediscovery.NewFromConfig(cfg)),
		NetworkFirewall:      multi.NewNetworkFirewall(networkfirewall.NewFromConfig(cfg)),
		VPCLattice:           multi.NewVPCLattice(vpclattice.NewFromConfig(cfg)),
		CodeArtifact:         multi.NewCodeArtifact(codeartifact.NewFromConfig(cfg)),
		XRay:                 multi.NewXRay(xray.NewFromConfig(cfg)),
		AMP:                  multi.NewAMP(amp.NewFromConfig(cfg)),
		Grafana:              multi.NewGrafana(grafana.NewFromConfig(cfg)),
		Budgets:              multi.NewBudgets(budgets.NewFromConfig(cfg)),
		CostExplorer:         multi.NewCostExplorer(costexplorer.NewFromConfig(cfg)),
		Organizations:        multi.NewOrganizations(organizations.NewFromConfig(cfg)),
		SSOAdmin:             multi.NewSSOAdmin(ssoadmin.NewFromConfig(cfg)),
		RAM:                  multi.NewRAM(ram.NewFromConfig(cfg)),
		ServiceCatalog:       multi.NewServiceCatalog(servicecatalog.NewFromConfig(cfg)),
		ControlTower:         multi.NewControlTower(controltower.NewFromConfig(cfg)),
		S3Control:            multi.NewS3Control(s3control.NewFromConfig(cfg)),
		STS:                  multi.NewSTS(sts.NewFromConfig(cfg)),
		CloudControl:         multi.NewCloudControl(cloudcontrol.NewFromConfig(cfg)),
		Config:               cfg,
	}, nil
}
