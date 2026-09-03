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
	"github.com/aws/aws-sdk-go-v2/service/vpclattice"
	"github.com/aws/aws-sdk-go-v2/service/wafv2"
	"github.com/aws/aws-sdk-go-v2/service/xray"
)

// Clients holds initialised AWS service clients shared across all controllers.
type Clients struct {
	IAM                  *iam.Client
	Route53              *route53.Client
	EKS                  *eks.Client
	EC2                  *ec2.Client
	RDS                  *rds.Client
	AutoScaling          *autoscaling.Client
	S3                   *s3.Client
	SQS                  *sqs.Client
	SNS                  *sns.Client
	ElastiCache          *elasticache.Client
	MemoryDB             *memorydb.Client
	Lambda               *lambda.Client
	ECS                  *ecs.Client
	ACM                  *acm.Client
	ELBv2                *elasticloadbalancingv2.Client
	ECR                  *ecr.Client
	EFS                  *efs.Client
	CloudFront           *cloudfront.Client
	CloudWatch           *cloudwatch.Client
	CloudWatchLogs       *cloudwatchlogs.Client
	KMS                  *kms.Client
	SecretsManager       *secretsmanager.Client
	EventBridge          *eventbridge.Client
	Pipes                *pipes.Client
	SESv2                *sesv2.Client
	DynamoDB             *dynamodb.Client
	DAX                  *dax.Client
	Cognito              *cognitoidentityprovider.Client
	CloudFormation       *cloudformation.Client
	CodeBuild            *codebuild.Client
	CodeCommit           *codecommit.Client
	CodeDeploy           *codedeploy.Client
	CodePipeline         *codepipeline.Client
	Firehose             *firehose.Client
	Kafka                *kafka.Client
	Kinesis              *kinesis.Client
	SFN                  *sfn.Client
	OpenSearch           *opensearch.Client
	OpenSearchServerless *opensearchserverless.Client
	Route53Resolver      *route53resolver.Client
	Shield               *shield.Client
	WAFv2                *wafv2.Client
	SSM                  *ssm.Client
	APIGateway           *apigateway.Client
	APIGatewayV2         *apigatewayv2.Client
	CloudTrail           *cloudtrail.Client
	ConfigService        *configservice.Client
	Backup               *backup.Client
	AppAutoScaling       *applicationautoscaling.Client
	Scheduler            *scheduler.Client
	GuardDuty            *guardduty.Client
	SecurityHub          *securityhub.Client
	Inspector2           *inspector2.Client
	Glue                 *glue.Client
	Athena               *athena.Client
	Redshift             *redshift.Client
	MQ                   *mq.Client
	Batch                *batch.Client
	AppRunner            *apprunner.Client
	ACMPCA               *acmpca.Client
	ServiceDiscovery     *servicediscovery.Client
	NetworkFirewall      *networkfirewall.Client
	VPCLattice           *vpclattice.Client
	CodeArtifact         *codeartifact.Client
	XRay                 *xray.Client
	AMP                  *amp.Client
	Grafana              *grafana.Client
	Budgets              *budgets.Client
	CostExplorer         *costexplorer.Client
	Organizations        *organizations.Client
	SSOAdmin             *ssoadmin.Client
	RAM                  *ram.Client
	ServiceCatalog       *servicecatalog.Client
	ControlTower         *controltower.Client
	S3Control            *s3control.Client
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
		IAM:                  iam.NewFromConfig(cfg),
		Route53:              route53.NewFromConfig(cfg),
		EKS:                  eks.NewFromConfig(cfg),
		EC2:                  ec2.NewFromConfig(cfg),
		RDS:                  rds.NewFromConfig(cfg),
		AutoScaling:          autoscaling.NewFromConfig(cfg),
		S3:                   s3.NewFromConfig(cfg),
		SQS:                  sqs.NewFromConfig(cfg),
		SNS:                  sns.NewFromConfig(cfg),
		ElastiCache:          elasticache.NewFromConfig(cfg),
		MemoryDB:             memorydb.NewFromConfig(cfg),
		Lambda:               lambda.NewFromConfig(cfg),
		ECS:                  ecs.NewFromConfig(cfg),
		ACM:                  acm.NewFromConfig(cfg),
		ELBv2:                elasticloadbalancingv2.NewFromConfig(cfg),
		ECR:                  ecr.NewFromConfig(cfg),
		EFS:                  efs.NewFromConfig(cfg),
		CloudFront:           cloudfront.NewFromConfig(cfg),
		CloudWatch:           cloudwatch.NewFromConfig(cfg),
		CloudWatchLogs:       cloudwatchlogs.NewFromConfig(cfg),
		KMS:                  kms.NewFromConfig(cfg),
		SecretsManager:       secretsmanager.NewFromConfig(cfg),
		EventBridge:          eventbridge.NewFromConfig(cfg),
		Pipes:                pipes.NewFromConfig(cfg),
		SESv2:                sesv2.NewFromConfig(cfg),
		DynamoDB:             dynamodb.NewFromConfig(cfg),
		DAX:                  dax.NewFromConfig(cfg),
		Cognito:              cognitoidentityprovider.NewFromConfig(cfg),
		CloudFormation:       cloudformation.NewFromConfig(cfg),
		CodeBuild:            codebuild.NewFromConfig(cfg),
		CodeCommit:           codecommit.NewFromConfig(cfg),
		CodeDeploy:           codedeploy.NewFromConfig(cfg),
		CodePipeline:         codepipeline.NewFromConfig(cfg),
		Firehose:             firehose.NewFromConfig(cfg),
		Kafka:                kafka.NewFromConfig(cfg),
		Kinesis:              kinesis.NewFromConfig(cfg),
		SFN:                  sfn.NewFromConfig(cfg),
		OpenSearch:           opensearch.NewFromConfig(cfg),
		OpenSearchServerless: opensearchserverless.NewFromConfig(cfg),
		Route53Resolver:      route53resolver.NewFromConfig(cfg),
		Shield:               shield.NewFromConfig(cfg),
		WAFv2:                wafv2.NewFromConfig(cfg),
		SSM:                  ssm.NewFromConfig(cfg),
		APIGateway:           apigateway.NewFromConfig(cfg),
		APIGatewayV2:         apigatewayv2.NewFromConfig(cfg),
		CloudTrail:           cloudtrail.NewFromConfig(cfg),
		ConfigService:        configservice.NewFromConfig(cfg),
		Backup:               backup.NewFromConfig(cfg),
		AppAutoScaling:       applicationautoscaling.NewFromConfig(cfg),
		Scheduler:            scheduler.NewFromConfig(cfg),
		GuardDuty:            guardduty.NewFromConfig(cfg),
		SecurityHub:          securityhub.NewFromConfig(cfg),
		Inspector2:           inspector2.NewFromConfig(cfg),
		Glue:                 glue.NewFromConfig(cfg),
		Athena:               athena.NewFromConfig(cfg),
		Redshift:             redshift.NewFromConfig(cfg),
		MQ:                   mq.NewFromConfig(cfg),
		Batch:                batch.NewFromConfig(cfg),
		AppRunner:            apprunner.NewFromConfig(cfg),
		ACMPCA:               acmpca.NewFromConfig(cfg),
		ServiceDiscovery:     servicediscovery.NewFromConfig(cfg),
		NetworkFirewall:      networkfirewall.NewFromConfig(cfg),
		VPCLattice:           vpclattice.NewFromConfig(cfg),
		CodeArtifact:         codeartifact.NewFromConfig(cfg),
		XRay:                 xray.NewFromConfig(cfg),
		AMP:                  amp.NewFromConfig(cfg),
		Grafana:              grafana.NewFromConfig(cfg),
		Budgets:              budgets.NewFromConfig(cfg),
		CostExplorer:         costexplorer.NewFromConfig(cfg),
		Organizations:        organizations.NewFromConfig(cfg),
		SSOAdmin:             ssoadmin.NewFromConfig(cfg),
		RAM:                  ram.NewFromConfig(cfg),
		ServiceCatalog:       servicecatalog.NewFromConfig(cfg),
		ControlTower:         controltower.NewFromConfig(cfg),
		S3Control:            s3control.NewFromConfig(cfg),
	}, nil
}
