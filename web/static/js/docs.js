/* ── Sidebar Data ────────────────────────────────────────────────────────────── */
const SIDEBAR_DATA = [
  {
    id: "acm",
    label: "ACM",
    href: "/docs/acm.html",
    items: [
      { label: "Certificate", anchor: "certificate" }
    ]
  },
  {
    id: "acmpca",
    label: "ACM PCA",
    href: "/docs/acmpca.html",
    items: [
      { label: "PrivateCA", anchor: "privateca" }
    ]
  },
  {
    id: "mq",
    label: "Amazon MQ",
    href: "/docs/mq.html",
    items: [
      { label: "MQBroker", anchor: "mqbroker" },
      { label: "MQConfiguration", anchor: "mqconfiguration" }
    ]
  },
  {
    id: "apigateway",
    label: "API Gateway",
    href: "/docs/apigateway.html",
    items: [
      { label: "RestAPI", anchor: "restapi" },
      { label: "RestAPIDeployment", anchor: "restapideployment" },
      { label: "RestAPIStage", anchor: "restapistage" }
    ]
  },
  {
    id: "apigatewayv2",
    label: "API Gateway v2",
    href: "/docs/apigatewayv2.html",
    items: [
      { label: "APIGatewayV2API", anchor: "apigatewayv2api" },
      { label: "APIGatewayV2ApiMapping", anchor: "apigatewayv2apimapping" },
      { label: "APIGatewayV2Authorizer", anchor: "apigatewayv2authorizer" },
      { label: "APIGatewayV2DomainName", anchor: "apigatewayv2domainname" },
      { label: "APIGatewayV2Integration", anchor: "apigatewayv2integration" },
      { label: "APIGatewayV2Route", anchor: "apigatewayv2route" },
      { label: "APIGatewayV2Stage", anchor: "apigatewayv2stage" },
      { label: "APIGatewayV2VpcLink", anchor: "apigatewayv2vpclink" }
    ]
  },
  {
    id: "apprunner",
    label: "App Runner",
    href: "/docs/apprunner.html",
    items: [
      { label: "AppRunnerAutoScaling", anchor: "apprunnerautoscaling" },
      { label: "AppRunnerService", anchor: "apprunnerservice" }
    ]
  },
  {
    id: "applicationautoscaling",
    label: "Application Auto Scaling",
    href: "/docs/applicationautoscaling.html",
    items: [
      { label: "AppScalingPolicy", anchor: "appscalingpolicy" },
      { label: "ScalableTarget", anchor: "scalabletarget" }
    ]
  },
  {
    id: "athena",
    label: "Athena",
    href: "/docs/athena.html",
    items: [
      { label: "AthenaDataCatalog", anchor: "athenadatacatalog" },
      { label: "AthenaNamedQuery", anchor: "athenanamedquery" },
      { label: "AthenaWorkGroup", anchor: "athenaworkgroup" }
    ]
  },
  {
    id: "autoscaling",
    label: "Auto Scaling",
    href: "/docs/autoscaling.html",
    items: [
      { label: "AutoScalingGroup", anchor: "autoscalinggroup" }
    ]
  },
  {
    id: "configservice",
    label: "AWS Config",
    href: "/docs/configservice.html",
    items: [
      { label: "ConfigDeliveryChannel", anchor: "configdeliverychannel" },
      { label: "ConfigRecorder", anchor: "configrecorder" },
      { label: "ConfigRule", anchor: "configrule" }
    ]
  },
  {
    id: "backup",
    label: "Backup",
    href: "/docs/backup.html",
    items: [
      { label: "BackupPlan", anchor: "backupplan" },
      { label: "BackupSelection", anchor: "backupselection" },
      { label: "BackupVault", anchor: "backupvault" }
    ]
  },
  {
    id: "batch",
    label: "Batch",
    href: "/docs/batch.html",
    items: [
      { label: "BatchComputeEnvironment", anchor: "batchcomputeenvironment" },
      { label: "BatchJobDefinition", anchor: "batchjobdefinition" },
      { label: "BatchJobQueue", anchor: "batchjobqueue" }
    ]
  },
  {
    id: "budgets",
    label: "Budgets",
    href: "/docs/budgets.html",
    items: [
      { label: "Budget", anchor: "budget" }
    ]
  },
  {
    id: "cloudcontrol",
    label: "Cloud Control API",
    href: "/docs/cloudcontrol.html",
    items: [
      { label: "CloudControlResource", anchor: "cloudcontrolresource" }
    ]
  },
  {
    id: "servicediscovery",
    label: "Cloud Map",
    href: "/docs/servicediscovery.html",
    items: [
      { label: "CloudMapNamespace", anchor: "cloudmapnamespace" },
      { label: "CloudMapService", anchor: "cloudmapservice" }
    ]
  },
  {
    id: "cloudformation",
    label: "CloudFormation",
    href: "/docs/cloudformation.html",
    items: [
      { label: "CloudFormationStack", anchor: "cloudformationstack" },
      { label: "CloudFormationStackSet", anchor: "cloudformationstackset" }
    ]
  },
  {
    id: "cloudfront",
    label: "CloudFront",
    href: "/docs/cloudfront.html",
    items: [
      { label: "CloudFrontCachePolicy", anchor: "cloudfrontcachepolicy" },
      { label: "CloudFrontDistribution", anchor: "cloudfrontdistribution" },
      { label: "CloudFrontFunction", anchor: "cloudfrontfunction" },
      { label: "CloudFrontOriginAccessControl", anchor: "cloudfrontoriginaccesscontrol" }
    ]
  },
  {
    id: "cloudtrail",
    label: "CloudTrail",
    href: "/docs/cloudtrail.html",
    items: [
      { label: "Trail", anchor: "trail" }
    ]
  },
  {
    id: "cloudwatch",
    label: "CloudWatch",
    href: "/docs/cloudwatch.html",
    items: [
      { label: "CloudWatchAlarm", anchor: "cloudwatchalarm" },
      { label: "CloudWatchDashboard", anchor: "cloudwatchdashboard" },
      { label: "CompositeAlarm", anchor: "compositealarm" }
    ]
  },
  {
    id: "cloudwatchlogs",
    label: "CloudWatch Logs",
    href: "/docs/cloudwatchlogs.html",
    items: [
      { label: "LogGroup", anchor: "loggroup" },
      { label: "MetricFilter", anchor: "metricfilter" },
      { label: "SubscriptionFilter", anchor: "subscriptionfilter" }
    ]
  },
  {
    id: "codeartifact",
    label: "CodeArtifact",
    href: "/docs/codeartifact.html",
    items: [
      { label: "CodeArtifactDomain", anchor: "codeartifactdomain" },
      { label: "CodeArtifactRepository", anchor: "codeartifactrepository" }
    ]
  },
  {
    id: "codecommit",
    label: "CodeCommit",
    href: "/docs/codecommit.html",
    items: [
      { label: "CodeCommitRepository", anchor: "codecommitrepository" }
    ]
  },
  {
    id: "codedeploy",
    label: "CodeDeploy",
    href: "/docs/codedeploy.html",
    items: [
      { label: "CodeDeployApplication", anchor: "codedeployapplication" },
      { label: "CodeDeployDeploymentGroup", anchor: "codedeploydeploymentgroup" }
    ]
  },
  {
    id: "codepipeline",
    label: "CodePipeline",
    href: "/docs/codepipeline.html",
    items: [
      { label: "CodePipeline", anchor: "codepipeline" }
    ]
  },
  {
    id: "cognito",
    label: "Cognito",
    href: "/docs/cognito.html",
    items: [
      { label: "IdentityProvider", anchor: "identityprovider" },
      { label: "UserPool", anchor: "userpool" },
      { label: "UserPoolClient", anchor: "userpoolclient" }
    ]
  },
  {
    id: "controltower",
    label: "Control Tower",
    href: "/docs/controltower.html",
    items: [
      { label: "CTEnabledControl", anchor: "ctenabledcontrol" }
    ]
  },
  {
    id: "costexplorer",
    label: "Cost Explorer",
    href: "/docs/costexplorer.html",
    items: [
      { label: "CostAnomalyMonitor", anchor: "costanomalymonitor" },
      { label: "CostAnomalySubscription", anchor: "costanomalysubscription" }
    ]
  },
  {
    id: "firehose",
    label: "Data Firehose",
    href: "/docs/firehose.html",
    items: [
      { label: "FirehoseDeliveryStream", anchor: "firehosedeliverystream" }
    ]
  },
  {
    id: "dax",
    label: "DAX",
    href: "/docs/dax.html",
    items: [
      { label: "DAXCluster", anchor: "daxcluster" }
    ]
  },
  {
    id: "dynamodb",
    label: "DynamoDB",
    href: "/docs/dynamodb.html",
    items: [
      { label: "DynamoDBBackup", anchor: "dynamodbbackup" },
      { label: "DynamoDBGlobalTable", anchor: "dynamodbglobaltable" },
      { label: "DynamoDBTable", anchor: "dynamodbtable" },
      { label: "DynamoDBTablePolicy", anchor: "dynamodbtablepolicy" }
    ]
  },
  {
    id: "ec2",
    label: "EC2 & VPC",
    href: "/docs/ec2.html",
    items: [
      { label: "AMI", anchor: "ami" },
      { label: "CapacityReservation", anchor: "capacityreservation" },
      { label: "CustomerGateway", anchor: "customergateway" },
      { label: "EBSVolume", anchor: "ebsvolume" },
      { label: "EC2Instance", anchor: "ec2instance" },
      { label: "EIPAssociation", anchor: "eipassociation" },
      { label: "EgressOnlyIGW", anchor: "egressonlyigw" },
      { label: "ElasticIP", anchor: "elasticip" },
      { label: "FlowLog", anchor: "flowlog" },
      { label: "InternetGateway", anchor: "internetgateway" },
      { label: "KeyPair", anchor: "keypair" },
      { label: "LaunchTemplate", anchor: "launchtemplate" },
      { label: "ManagedPrefixList", anchor: "managedprefixlist" },
      { label: "NatGateway", anchor: "natgateway" },
      { label: "NetworkACL", anchor: "networkacl" },
      { label: "PlacementGroup", anchor: "placementgroup" },
      { label: "RouteTable", anchor: "routetable" },
      { label: "SecurityGroup", anchor: "securitygroup" },
      { label: "SpotFleet", anchor: "spotfleet" },
      { label: "Subnet", anchor: "subnet" },
      { label: "TransitGateway", anchor: "transitgateway" },
      { label: "TransitGatewayVpcAttachment", anchor: "transitgatewayvpcattachment" },
      { label: "VPC", anchor: "vpc" },
      { label: "VPCEndpoint", anchor: "vpcendpoint" },
      { label: "VPCEndpointService", anchor: "vpcendpointservice" },
      { label: "VPCPeeringConnection", anchor: "vpcpeeringconnection" },
      { label: "VPNConnection", anchor: "vpnconnection" },
      { label: "VPNConnectionRoute", anchor: "vpnconnectionroute" },
      { label: "VPNGateway", anchor: "vpngateway" }
    ]
  },
  {
    id: "ecr",
    label: "ECR",
    href: "/docs/ecr.html",
    items: [
      { label: "ECRLifecyclePolicy", anchor: "ecrlifecyclepolicy" },
      { label: "ECRRepository", anchor: "ecrrepository" },
      { label: "ECRRepositoryPolicy", anchor: "ecrrepositorypolicy" }
    ]
  },
  {
    id: "ecs",
    label: "ECS",
    href: "/docs/ecs.html",
    items: [
      { label: "ECSCapacityProvider", anchor: "ecscapacityprovider" },
      { label: "ECSCluster", anchor: "ecscluster" },
      { label: "ECSService", anchor: "ecsservice" },
      { label: "ECSTaskDefinition", anchor: "ecstaskdefinition" }
    ]
  },
  {
    id: "efs",
    label: "EFS",
    href: "/docs/efs.html",
    items: [
      { label: "EFSAccessPoint", anchor: "efsaccesspoint" },
      { label: "EFSFileSystem", anchor: "efsfilesystem" },
      { label: "EFSMountTarget", anchor: "efsmounttarget" }
    ]
  },
  {
    id: "eks",
    label: "EKS",
    href: "/docs/eks.html",
    items: [
      { label: "EKSAccessEntry", anchor: "eksaccessentry" },
      { label: "EKSAddon", anchor: "eksaddon" },
      { label: "EKSCluster", anchor: "ekscluster" },
      { label: "EKSFargateProfile", anchor: "eksfargateprofile" },
      { label: "EKSIdentityProviderConfig", anchor: "eksidentityproviderconfig" },
      { label: "EKSNodeGroup", anchor: "eksnodegroup" },
      { label: "PodIdentityAssociation", anchor: "podidentityassociation" }
    ]
  },
  {
    id: "elasticache",
    label: "ElastiCache",
    href: "/docs/elasticache.html",
    items: [
      { label: "ElastiCacheParameterGroup", anchor: "elasticacheparametergroup" },
      { label: "ElastiCacheReplicationGroup", anchor: "elasticachereplicationgroup" },
      { label: "ElastiCacheServerlessCache", anchor: "elasticacheserverlesscache" },
      { label: "ElastiCacheSubnetGroup", anchor: "elasticachesubnetgroup" }
    ]
  },
  {
    id: "eventbridge",
    label: "EventBridge",
    href: "/docs/eventbridge.html",
    items: [
      { label: "ECSScheduledTask", anchor: "ecsscheduledtask" },
      { label: "EventBus", anchor: "eventbus" },
      { label: "EventRule", anchor: "eventrule" },
      { label: "EventTarget", anchor: "eventtarget" }
    ]
  },
  {
    id: "scheduler",
    label: "EventBridge Scheduler",
    href: "/docs/scheduler.html",
    items: [
      { label: "Schedule", anchor: "schedule" },
      { label: "ScheduleGroup", anchor: "schedulegroup" }
    ]
  },
  {
    id: "glue",
    label: "Glue",
    href: "/docs/glue.html",
    items: [
      { label: "GlueConnection", anchor: "glueconnection" },
      { label: "GlueCrawler", anchor: "gluecrawler" },
      { label: "GlueDatabase", anchor: "gluedatabase" },
      { label: "GlueJob", anchor: "gluejob" },
      { label: "GlueTrigger", anchor: "gluetrigger" }
    ]
  },
  {
    id: "guardduty",
    label: "GuardDuty",
    href: "/docs/guardduty.html",
    items: [
      { label: "GuardDutyDetector", anchor: "guarddutydetector" }
    ]
  },
  {
    id: "iam",
    label: "IAM",
    href: "/docs/iam.html",
    items: [
      { label: "IAMGroup", anchor: "iamgroup" },
      { label: "IAMGroupMembership", anchor: "iamgroupmembership" },
      { label: "IAMGroupPolicyAttachment", anchor: "iamgrouppolicyattachment" },
      { label: "IAMInstanceProfile", anchor: "iaminstanceprofile" },
      { label: "IAMOIDCProvider", anchor: "iamoidcprovider" },
      { label: "IAMPolicy", anchor: "iampolicy" },
      { label: "IAMPolicyAttachment", anchor: "iampolicyattachment" },
      { label: "IAMRole", anchor: "iamrole" },
      { label: "IAMRolePolicy", anchor: "iamrolepolicy" },
      { label: "IAMSAMLProvider", anchor: "iamsamlprovider" },
      { label: "IAMUser", anchor: "iamuser" }
    ]
  },
  {
    id: "ssoadmin",
    label: "IAM Identity Center",
    href: "/docs/ssoadmin.html",
    items: [
      { label: "PermissionSet", anchor: "permissionset" },
      { label: "SSOAssignment", anchor: "ssoassignment" }
    ]
  },
  {
    id: "inspector2",
    label: "Inspector",
    href: "/docs/inspector2.html",
    items: [
      { label: "InspectorEnabler", anchor: "inspectorenabler" }
    ]
  },
  {
    id: "kinesis",
    label: "Kinesis",
    href: "/docs/kinesis.html",
    items: [
      { label: "KinesisStream", anchor: "kinesisstream" },
      { label: "KinesisStreamConsumer", anchor: "kinesisstreamconsumer" }
    ]
  },
  {
    id: "kms",
    label: "KMS",
    href: "/docs/kms.html",
    items: [
      { label: "KMSAlias", anchor: "kmsalias" },
      { label: "KMSGrant", anchor: "kmsgrant" },
      { label: "KMSKey", anchor: "kmskey" }
    ]
  },
  {
    id: "lambda",
    label: "Lambda",
    href: "/docs/lambda.html",
    items: [
      { label: "LambdaAlias", anchor: "lambdaalias" },
      { label: "LambdaCodeSigningConfig", anchor: "lambdacodesigningconfig" },
      { label: "LambdaEventInvokeConfig", anchor: "lambdaeventinvokeconfig" },
      { label: "LambdaEventSourceMapping", anchor: "lambdaeventsourcemapping" },
      { label: "LambdaFunction", anchor: "lambdafunction" },
      { label: "LambdaFunctionURL", anchor: "lambdafunctionurl" },
      { label: "LambdaLayerVersion", anchor: "lambdalayerversion" },
      { label: "LambdaPermission", anchor: "lambdapermission" },
      { label: "LambdaProvisionedConcurrency", anchor: "lambdaprovisionedconcurrency" }
    ]
  },
  {
    id: "elbv2",
    label: "Load Balancing",
    href: "/docs/elbv2.html",
    items: [
      { label: "Listener", anchor: "listener" },
      { label: "ListenerRule", anchor: "listenerrule" },
      { label: "LoadBalancer", anchor: "loadbalancer" },
      { label: "TargetGroup", anchor: "targetgroup" }
    ]
  },
  {
    id: "grafana",
    label: "Managed Grafana",
    href: "/docs/grafana.html",
    items: [
      { label: "GrafanaWorkspace", anchor: "grafanaworkspace" }
    ]
  },
  {
    id: "amp",
    label: "Managed Prometheus",
    href: "/docs/amp.html",
    items: [
      { label: "PrometheusAlertManagerDefinition", anchor: "prometheusalertmanagerdefinition" },
      { label: "PrometheusRuleGroupsNamespace", anchor: "prometheusrulegroupsnamespace" },
      { label: "PrometheusWorkspace", anchor: "prometheusworkspace" }
    ]
  },
  {
    id: "memorydb",
    label: "MemoryDB",
    href: "/docs/memorydb.html",
    items: [
      { label: "MemoryDBCluster", anchor: "memorydbcluster" }
    ]
  },
  {
    id: "kafka",
    label: "MSK",
    href: "/docs/kafka.html",
    items: [
      { label: "MSKCluster", anchor: "mskcluster" },
      { label: "MSKConfiguration", anchor: "mskconfiguration" },
      { label: "MSKServerlessCluster", anchor: "mskserverlesscluster" }
    ]
  },
  {
    id: "multi",
    label: "MULTI",
    href: "/docs/multi.html",
    items: [
      { label: "Activity", anchor: "activity" },
      { label: "CertificateValidation", anchor: "certificatevalidation" },
      { label: "CodeBuildProject", anchor: "codebuildproject" },
      { label: "DBCluster", anchor: "dbcluster" },
      { label: "DBClusterParameterGroup", anchor: "dbclusterparametergroup" },
      { label: "DBOptionGroup", anchor: "dboptiongroup" },
      { label: "DBParameterGroup", anchor: "dbparametergroup" },
      { label: "DBProxy", anchor: "dbproxy" },
      { label: "DBSnapshot", anchor: "dbsnapshot" },
      { label: "DBSubnetGroup", anchor: "dbsubnetgroup" },
      { label: "DelegationSignerRecord", anchor: "delegationsignerrecord" },
      { label: "EventBridgePipe", anchor: "eventbridgepipe" },
      { label: "IPSet", anchor: "ipset" },
      { label: "KMSKeyPolicy", anchor: "kmskeypolicy" },
      { label: "OpenSearchAccessPolicy", anchor: "opensearchaccesspolicy" },
      { label: "OpenSearchDomain", anchor: "opensearchdomain" },
      { label: "OpenSearchServerlessCollection", anchor: "opensearchserverlesscollection" },
      { label: "ResolverEndpoint", anchor: "resolverendpoint" },
      { label: "ResolverRule", anchor: "resolverrule" },
      { label: "S3BucketCORS", anchor: "s3bucketcors" },
      { label: "S3BucketLifecycle", anchor: "s3bucketlifecycle" },
      { label: "S3BucketNotification", anchor: "s3bucketnotification" },
      { label: "S3BucketPolicy", anchor: "s3bucketpolicy" },
      { label: "S3BucketReplication", anchor: "s3bucketreplication" },
      { label: "SESConfigurationSet", anchor: "sesconfigurationset" },
      { label: "SESEmailIdentity", anchor: "sesemailidentity" },
      { label: "SNSSubscription", anchor: "snssubscription" },
      { label: "SSMDocument", anchor: "ssmdocument" },
      { label: "ScalingPolicy", anchor: "scalingpolicy" },
      { label: "SecretRotation", anchor: "secretrotation" },
      { label: "ShieldProtection", anchor: "shieldprotection" },
      { label: "StateMachine", anchor: "statemachine" },
      { label: "WAFRegexPatternSet", anchor: "wafregexpatternset" },
      { label: "WAFRuleGroup", anchor: "wafrulegroup" },
      { label: "WebACL", anchor: "webacl" }
    ]
  },
  {
    id: "provider",
    label: "Multi-Account Provider",
    href: "/docs/provider.html",
    items: [
      { label: "AWSProvider", anchor: "awsprovider" }
    ]
  },
  {
    id: "networkfirewall",
    label: "Network Firewall",
    href: "/docs/networkfirewall.html",
    items: [
      { label: "Firewall", anchor: "firewall" },
      { label: "FirewallPolicy", anchor: "firewallpolicy" },
      { label: "FirewallRuleGroup", anchor: "firewallrulegroup" }
    ]
  },
  {
    id: "organizations",
    label: "Organizations",
    href: "/docs/organizations.html",
    items: [
      { label: "OrganizationsAccount", anchor: "organizationsaccount" },
      { label: "OrganizationsOU", anchor: "organizationsou" },
      { label: "OrganizationsPolicy", anchor: "organizationspolicy" },
      { label: "OrganizationsPolicyAttachment", anchor: "organizationspolicyattachment" }
    ]
  },
  {
    id: "ram",
    label: "RAM",
    href: "/docs/ram.html",
    items: [
      { label: "ResourceShare", anchor: "resourceshare" },
      { label: "ResourceShareInvitation", anchor: "resourceshareinvitation" }
    ]
  },
  {
    id: "rds",
    label: "RDS & Aurora",
    href: "/docs/rds.html",
    items: [
      { label: "DBInstance", anchor: "dbinstance" },
      { label: "RDSEventSubscription", anchor: "rdseventsubscription" },
      { label: "RDSGlobalCluster", anchor: "rdsglobalcluster" }
    ]
  },
  {
    id: "redshift",
    label: "Redshift",
    href: "/docs/redshift.html",
    items: [
      { label: "RedshiftCluster", anchor: "redshiftcluster" },
      { label: "RedshiftParameterGroup", anchor: "redshiftparametergroup" },
      { label: "RedshiftSubnetGroup", anchor: "redshiftsubnetgroup" }
    ]
  },
  {
    id: "route53",
    label: "Route 53",
    href: "/docs/route53.html",
    items: [
      { label: "HealthCheck", anchor: "healthcheck" },
      { label: "HostedZone", anchor: "hostedzone" },
      { label: "HostedZoneVPCAssociation", anchor: "hostedzonevpcassociation" },
      { label: "RecordSet", anchor: "recordset" }
    ]
  },
  {
    id: "s3",
    label: "S3",
    href: "/docs/s3.html",
    items: [
      { label: "S3Bucket", anchor: "s3bucket" }
    ]
  },
  {
    id: "s3control",
    label: "S3 Control",
    href: "/docs/s3control.html",
    items: [
      { label: "S3AccessPoint", anchor: "s3accesspoint" }
    ]
  },
  {
    id: "secretsmanager",
    label: "Secrets Manager",
    href: "/docs/secretsmanager.html",
    items: [
      { label: "Secret", anchor: "secret" }
    ]
  },
  {
    id: "securityhub",
    label: "Security Hub",
    href: "/docs/securityhub.html",
    items: [
      { label: "SecurityHubAccount", anchor: "securityhubaccount" },
      { label: "SecurityHubStandard", anchor: "securityhubstandard" }
    ]
  },
  {
    id: "servicecatalog",
    label: "Service Catalog",
    href: "/docs/servicecatalog.html",
    items: [
      { label: "SCPortfolio", anchor: "scportfolio" },
      { label: "SCPortfolioProductAssociation", anchor: "scportfolioproductassociation" },
      { label: "SCProduct", anchor: "scproduct" }
    ]
  },
  {
    id: "sns",
    label: "SNS",
    href: "/docs/sns.html",
    items: [
      { label: "SNSTopic", anchor: "snstopic" }
    ]
  },
  {
    id: "sqs",
    label: "SQS",
    href: "/docs/sqs.html",
    items: [
      { label: "SQSQueue", anchor: "sqsqueue" }
    ]
  },
  {
    id: "ssm",
    label: "SSM",
    href: "/docs/ssm.html",
    items: [
      { label: "SSMAssociation", anchor: "ssmassociation" },
      { label: "SSMMaintenanceWindow", anchor: "ssmmaintenancewindow" },
      { label: "SSMParameter", anchor: "ssmparameter" },
      { label: "SSMPatchBaseline", anchor: "ssmpatchbaseline" }
    ]
  },
  {
    id: "vpclattice",
    label: "VPC Lattice",
    href: "/docs/vpclattice.html",
    items: [
      { label: "LatticeListener", anchor: "latticelistener" },
      { label: "LatticeService", anchor: "latticeservice" },
      { label: "LatticeServiceNetwork", anchor: "latticeservicenetwork" },
      { label: "LatticeServiceNetworkServiceAssociation", anchor: "latticeservicenetworkserviceassociation" },
      { label: "LatticeServiceNetworkVpcAssociation", anchor: "latticeservicenetworkvpcassociation" },
      { label: "LatticeTargetGroup", anchor: "latticetargetgroup" }
    ]
  },
  {
    id: "xray",
    label: "X-Ray",
    href: "/docs/xray.html",
    items: [
      { label: "XRayGroup", anchor: "xraygroup" },
      { label: "XRaySamplingRule", anchor: "xraysamplingrule" }
    ]
  }
];

/* ── Determine current page ──────────────────────────────────────────────────── */
function getCurrentPageId() {
  const path = window.location.pathname;
  if (path === '/docs/' || path === '/docs/index.html') return 'overview';
  const match = path.match(/\/docs\/([^/]+)\.html$/);
  return match ? match[1] : 'overview';
}

/* ── Build Sidebar HTML ──────────────────────────────────────────────────────── */
function buildSidebar() {
  const sidebar = document.getElementById('docs-sidebar');
  if (!sidebar) return;

  const currentPage = getCurrentPageId();

  let html = `
    <a class="sidebar-overview${currentPage === 'overview' ? ' active' : ''}" href="/docs/">
      Overview
    </a>
    <div class="sidebar-divider"></div>
    <div class="docs-section-divider">Resources</div>
  `;

  SIDEBAR_DATA.forEach(group => {
    const isCurrentPage = group.id === currentPage;
    const isOpen = isCurrentPage;

    html += `<div class="sidebar-group${isOpen ? ' open' : ''}" data-group="${group.id}">`;
    html += `<a class="sidebar-section-title${isCurrentPage ? ' active' : ''}" href="${group.href}">
      ${group.label}
      <span class="sidebar-arrow">▼</span>
    </a>`;
    html += `<ul class="sidebar-items">`;

    group.items.forEach(item => {
      const href = isCurrentPage
        ? `#${item.anchor}`
        : `${group.href}#${item.anchor}`;
      html += `<li><a href="${href}" data-anchor="${item.anchor}">${item.label}</a></li>`;
    });

    html += `</ul></div>`;
  });

  sidebar.innerHTML = html;

  // Attach toggle listeners for section titles
  sidebar.querySelectorAll('.sidebar-section-title').forEach(title => {
    title.addEventListener('click', function(e) {
      const group = this.closest('.sidebar-group');
      if (!group) return;
      // If clicking the link of the current page, just toggle open; otherwise navigate
      const href = this.getAttribute('href');
      const isCurrentPage = href === window.location.pathname ||
                            (href === '/docs/' && window.location.pathname.endsWith('/docs/')) ||
                            window.location.pathname.endsWith(href);
      if (isCurrentPage) {
        e.preventDefault();
        group.classList.toggle('open');
      }
      // else: let the link navigate normally
    });
  });
}

/* ── Scroll Spy ──────────────────────────────────────────────────────────────── */
function initScrollSpy() {
  const sections = document.querySelectorAll('.resource-section[id]');
  if (!sections.length) return;

  const sidebarLinks = document.querySelectorAll('.sidebar-items li a[data-anchor]');

  function onScroll() {
    const scrollY = window.scrollY + 80; // offset for nav height

    let current = null;
    sections.forEach(section => {
      if (section.offsetTop <= scrollY) {
        current = section.id;
      }
    });

    sidebarLinks.forEach(link => {
      const anchor = link.getAttribute('data-anchor');
      link.classList.toggle('active', anchor === current);
    });

    // Scroll the active sidebar link into view (within sidebar)
    const activeLink = document.querySelector('.sidebar-items li a.active');
    if (activeLink) {
      const sidebar = document.getElementById('docs-sidebar');
      if (sidebar) {
        const linkTop = activeLink.offsetTop;
        const sidebarHeight = sidebar.clientHeight;
        const sidebarScroll = sidebar.scrollTop;
        if (linkTop < sidebarScroll + 60 || linkTop > sidebarScroll + sidebarHeight - 60) {
          sidebar.scrollTo({ top: linkTop - sidebarHeight / 2 + 20, behavior: 'smooth' });
        }
      }
    }
  }

  window.addEventListener('scroll', onScroll, { passive: true });
  onScroll(); // run once on load
}

/* ── Copy Buttons ────────────────────────────────────────────────────────────── */
function initCopyButtons() {
  document.querySelectorAll('.code-copy-btn').forEach(btn => {
    btn.addEventListener('click', function() {
      const codeEl = this.closest('.code-wrap')?.querySelector('code');
      if (!codeEl) return;

      const text = codeEl.innerText || codeEl.textContent;
      navigator.clipboard.writeText(text).then(() => {
        this.textContent = 'copied!';
        this.classList.add('copied');
        setTimeout(() => {
          this.textContent = 'copy';
          this.classList.remove('copied');
        }, 2000);
      }).catch(() => {
        // Fallback for older browsers
        const textarea = document.createElement('textarea');
        textarea.value = text;
        textarea.style.position = 'fixed';
        textarea.style.opacity = '0';
        document.body.appendChild(textarea);
        textarea.select();
        document.execCommand('copy');
        document.body.removeChild(textarea);
        this.textContent = 'copied!';
        this.classList.add('copied');
        setTimeout(() => {
          this.textContent = 'copy';
          this.classList.remove('copied');
        }, 2000);
      });
    });
  });
}

/* ── Mobile Sidebar Toggle ───────────────────────────────────────────────────── */
function initMobileToggle() {
  const toggle = document.getElementById('sidebar-toggle');
  const sidebar = document.getElementById('docs-sidebar');
  const overlay = document.getElementById('sidebar-overlay');

  if (!toggle || !sidebar) return;

  function openSidebar() {
    sidebar.classList.add('open');
    if (overlay) overlay.classList.add('open');
    toggle.textContent = '✕';
    document.body.style.overflow = 'hidden';
  }

  function closeSidebar() {
    sidebar.classList.remove('open');
    if (overlay) overlay.classList.remove('open');
    toggle.textContent = '☰';
    document.body.style.overflow = '';
  }

  toggle.addEventListener('click', () => {
    if (sidebar.classList.contains('open')) {
      closeSidebar();
    } else {
      openSidebar();
    }
  });

  if (overlay) {
    overlay.addEventListener('click', closeSidebar);
  }

  // Close sidebar when clicking a link (on mobile)
  sidebar.querySelectorAll('a').forEach(link => {
    link.addEventListener('click', () => {
      if (window.innerWidth <= 900) {
        closeSidebar();
      }
    });
  });
}

/* ── Nav scroll effect ───────────────────────────────────────────────────────── */
function initNavScroll() {
  const nav = document.querySelector('nav');
  if (!nav) return;
  // Docs pages always show scrolled nav since content starts immediately
  nav.classList.add('scrolled');
}

/* ── Init ────────────────────────────────────────────────────────────────────── */
document.addEventListener('DOMContentLoaded', () => {
  buildSidebar();
  initScrollSpy();
  initCopyButtons();
  initMobileToggle();
  initNavScroll();

  // Re-run highlight.js on dynamically-highlighted blocks (it's called in HTML too)
  if (window.hljs) {
    hljs.highlightAll();
    // Re-init copy buttons after hljs potentially rewrites the DOM
    initCopyButtons();
  }
});
