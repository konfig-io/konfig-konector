// gen-multiclient emits internal/aws/multi/zz_<service>.go: one wrapper type per
// AWS SDK service client that forwards every operation to the base client with
// a per-call option override for Region and Credentials taken from the
// provider.Scope in the context. This is what lets a single operator instance
// reconcile resources across many AWS accounts and regions without every
// controller knowing about it.
//
// Run: go run ./hack/gen-multiclient
package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

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
)

type svc struct {
	Pkg      string // import path suffix
	TypeName string // exported wrapper type name
	Client   interface{}
}

var services = []svc{
	{"acm", "ACM", &acm.Client{}},
	{"acmpca", "ACMPCA", &acmpca.Client{}},
	{"amp", "AMP", &amp.Client{}},
	{"apigateway", "APIGateway", &apigateway.Client{}},
	{"apigatewayv2", "APIGatewayV2", &apigatewayv2.Client{}},
	{"applicationautoscaling", "AppAutoScaling", &applicationautoscaling.Client{}},
	{"apprunner", "AppRunner", &apprunner.Client{}},
	{"athena", "Athena", &athena.Client{}},
	{"autoscaling", "AutoScaling", &autoscaling.Client{}},
	{"backup", "Backup", &backup.Client{}},
	{"batch", "Batch", &batch.Client{}},
	{"budgets", "Budgets", &budgets.Client{}},
	{"cloudcontrol", "CloudControl", &cloudcontrol.Client{}},
	{"cloudformation", "CloudFormation", &cloudformation.Client{}},
	{"cloudfront", "CloudFront", &cloudfront.Client{}},
	{"cloudtrail", "CloudTrail", &cloudtrail.Client{}},
	{"cloudwatch", "CloudWatch", &cloudwatch.Client{}},
	{"cloudwatchlogs", "CloudWatchLogs", &cloudwatchlogs.Client{}},
	{"codeartifact", "CodeArtifact", &codeartifact.Client{}},
	{"codebuild", "CodeBuild", &codebuild.Client{}},
	{"codecommit", "CodeCommit", &codecommit.Client{}},
	{"codedeploy", "CodeDeploy", &codedeploy.Client{}},
	{"codepipeline", "CodePipeline", &codepipeline.Client{}},
	{"cognitoidentityprovider", "Cognito", &cognitoidentityprovider.Client{}},
	{"configservice", "ConfigService", &configservice.Client{}},
	{"controltower", "ControlTower", &controltower.Client{}},
	{"costexplorer", "CostExplorer", &costexplorer.Client{}},
	{"dax", "DAX", &dax.Client{}},
	{"dynamodb", "DynamoDB", &dynamodb.Client{}},
	{"ec2", "EC2", &ec2.Client{}},
	{"ecr", "ECR", &ecr.Client{}},
	{"ecs", "ECS", &ecs.Client{}},
	{"efs", "EFS", &efs.Client{}},
	{"eks", "EKS", &eks.Client{}},
	{"elasticache", "ElastiCache", &elasticache.Client{}},
	{"elasticloadbalancingv2", "ELBv2", &elasticloadbalancingv2.Client{}},
	{"eventbridge", "EventBridge", &eventbridge.Client{}},
	{"firehose", "Firehose", &firehose.Client{}},
	{"glue", "Glue", &glue.Client{}},
	{"grafana", "Grafana", &grafana.Client{}},
	{"guardduty", "GuardDuty", &guardduty.Client{}},
	{"iam", "IAM", &iam.Client{}},
	{"inspector2", "Inspector2", &inspector2.Client{}},
	{"kafka", "Kafka", &kafka.Client{}},
	{"kinesis", "Kinesis", &kinesis.Client{}},
	{"kms", "KMS", &kms.Client{}},
	{"lambda", "Lambda", &lambda.Client{}},
	{"memorydb", "MemoryDB", &memorydb.Client{}},
	{"mq", "MQ", &mq.Client{}},
	{"networkfirewall", "NetworkFirewall", &networkfirewall.Client{}},
	{"opensearch", "OpenSearch", &opensearch.Client{}},
	{"opensearchserverless", "OpenSearchServerless", &opensearchserverless.Client{}},
	{"organizations", "Organizations", &organizations.Client{}},
	{"pipes", "Pipes", &pipes.Client{}},
	{"ram", "RAM", &ram.Client{}},
	{"rds", "RDS", &rds.Client{}},
	{"redshift", "Redshift", &redshift.Client{}},
	{"route53", "Route53", &route53.Client{}},
	{"route53resolver", "Route53Resolver", &route53resolver.Client{}},
	{"s3", "S3", &s3.Client{}},
	{"s3control", "S3Control", &s3control.Client{}},
	{"scheduler", "Scheduler", &scheduler.Client{}},
	{"secretsmanager", "SecretsManager", &secretsmanager.Client{}},
	{"securityhub", "SecurityHub", &securityhub.Client{}},
	{"servicecatalog", "ServiceCatalog", &servicecatalog.Client{}},
	{"servicediscovery", "ServiceDiscovery", &servicediscovery.Client{}},
	{"sesv2", "SESv2", &sesv2.Client{}},
	{"sfn", "SFN", &sfn.Client{}},
	{"shield", "Shield", &shield.Client{}},
	{"sns", "SNS", &sns.Client{}},
	{"sqs", "SQS", &sqs.Client{}},
	{"ssm", "SSM", &ssm.Client{}},
	{"ssoadmin", "SSOAdmin", &ssoadmin.Client{}},
	{"sts", "STS", &sts.Client{}},
	{"vpclattice", "VPCLattice", &vpclattice.Client{}},
	{"wafv2", "WAFv2", &wafv2.Client{}},
	{"xray", "XRay", &xray.Client{}},
}

const header = `// Code generated by hack/gen-multiclient. DO NOT EDIT.

package multi

import (
	"context"

	svc "github.com/aws/aws-sdk-go-v2/service/%s"

	"github.com/konfig-io/konfig-konector/internal/aws/provider"
)

// %[2]s wraps *svc.Client and applies the provider.Scope from the context
// (Region and Credentials) as per-operation options on every call.
type %[2]s struct{ base *svc.Client }

// New%[2]s wraps base.
func New%[2]s(base *svc.Client) *%[2]s { return &%[2]s{base: base} }

// Base returns the underlying client.
func (m *%[2]s) Base() *svc.Client { return m.base }

func (m *%[2]s) opts(ctx context.Context, optFns []func(*svc.Options)) []func(*svc.Options) {
	s := provider.ScopeFrom(ctx)
	if s == nil || (s.Region == "" && s.Credentials == nil) {
		return optFns
	}
	out := make([]func(*svc.Options), 0, len(optFns)+1)
	out = append(out, func(o *svc.Options) {
		if s.Region != "" {
			o.Region = s.Region
		}
		if s.Credentials != nil {
			o.Credentials = s.Credentials
		}
	})
	return append(out, optFns...)
}
`

func main() {
	outDir := filepath.Join("internal", "aws", "multi")
	ctxType := reflect.TypeOf((*interface{ Done() <-chan struct{} })(nil)).Elem()
	_ = ctxType
	total := 0
	for _, s := range services {
		t := reflect.TypeOf(s.Client)
		var buf bytes.Buffer
		fmt.Fprintf(&buf, header, s.Pkg, s.TypeName)
		var names []string
		for i := 0; i < t.NumMethod(); i++ {
			m := t.Method(i)
			mt := m.Type
			// (recv, ctx, *Input, ...optFns) (*Output, error)
			if mt.NumIn() != 4 || !mt.IsVariadic() || mt.NumOut() != 2 {
				continue
			}
			if mt.In(1).String() != "context.Context" {
				continue
			}
			in := mt.In(2)
			if in.Kind() != reflect.Ptr || in.Elem().PkgPath() == "" {
				continue
			}
			names = append(names, m.Name)
		}
		sort.Strings(names)
		for _, n := range names {
			fmt.Fprintf(&buf, `
func (m *%[1]s) %[2]s(ctx context.Context, params *svc.%[2]sInput, optFns ...func(*svc.Options)) (*svc.%[2]sOutput, error) {
	return m.base.%[2]s(ctx, params, m.opts(ctx, optFns)...)
}
`, s.TypeName, n)
		}
		src, err := format.Source(buf.Bytes())
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", s.Pkg, err)
			os.Exit(1)
		}
		if err := os.WriteFile(filepath.Join(outDir, "zz_"+strings.ToLower(s.Pkg)+".go"), src, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		total += len(names)
	}
	fmt.Printf("generated %d services, %d operations\n", len(services), total)
}
