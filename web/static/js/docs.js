/* ── Sidebar Data ────────────────────────────────────────────────────────────── */
const SIDEBAR_DATA = [
  {
    id: "accessanalyzer",
    label: "ACCESSANALYZER",
    href: "/docs/accessanalyzer.html",
    items: [
      { label: "AccessAnalyzerAnalyzer", anchor: "accessanalyzeranalyzer" },
      { label: "AccessAnalyzerArchiveRule", anchor: "accessanalyzerarchiverule" }
    ]
  },
  {
    id: "accountaccess",
    label: "ACCOUNTACCESS",
    href: "/docs/accountaccess.html",
    items: [
      { label: "AccountAccessApplication", anchor: "accountaccessapplication" },
      { label: "AccountAccessEntitlement", anchor: "accountaccessentitlement" }
    ]
  },
  {
    id: "acm",
    label: "ACM",
    href: "/docs/acm.html",
    items: [
      { label: "ACMAccount", anchor: "acmaccount" },
      { label: "ACMAcmeDomainValidation", anchor: "acmacmedomainvalidation" },
      { label: "ACMAcmeEndpoint", anchor: "acmacmeendpoint" },
      { label: "ACMAcmeExternalAccountBinding", anchor: "acmacmeexternalaccountbinding" },
      { label: "Certificate", anchor: "certificate" }
    ]
  },
  {
    id: "acmpca",
    label: "ACM PCA",
    href: "/docs/acmpca.html",
    items: [
      { label: "ACMPCACertificate", anchor: "acmpcacertificate" },
      { label: "ACMPCACertificateAuthorityActivation", anchor: "acmpcacertificateauthorityactivation" },
      { label: "ACMPCAPermission", anchor: "acmpcapermission" },
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
    id: "amplify",
    label: "AMPLIFY",
    href: "/docs/amplify.html",
    items: [
      { label: "AmplifyApp", anchor: "amplifyapp" },
      { label: "AmplifyBranch", anchor: "amplifybranch" },
      { label: "AmplifyDomain", anchor: "amplifydomain" },
      { label: "AmplifyWebhook", anchor: "amplifywebhook" }
    ]
  },
  {
    id: "apigateway",
    label: "API Gateway",
    href: "/docs/apigateway.html",
    items: [
      { label: "ApiGatewayAccount", anchor: "apigatewayaccount" },
      { label: "ApiGatewayApiKey", anchor: "apigatewayapikey" },
      { label: "ApiGatewayAuthorizer", anchor: "apigatewayauthorizer" },
      { label: "ApiGatewayBasePathMapping", anchor: "apigatewaybasepathmapping" },
      { label: "ApiGatewayBasePathMappingV2", anchor: "apigatewaybasepathmappingv2" },
      { label: "ApiGatewayClientCertificate", anchor: "apigatewayclientcertificate" },
      { label: "ApiGatewayDocumentationPart", anchor: "apigatewaydocumentationpart" },
      { label: "ApiGatewayDocumentationVersion", anchor: "apigatewaydocumentationversion" },
      { label: "ApiGatewayDomainName", anchor: "apigatewaydomainname" },
      { label: "ApiGatewayDomainNameAccessAssociation", anchor: "apigatewaydomainnameaccessassociation" },
      { label: "ApiGatewayDomainNameV2", anchor: "apigatewaydomainnamev2" },
      { label: "ApiGatewayGatewayResponse", anchor: "apigatewaygatewayresponse" },
      { label: "ApiGatewayMethod", anchor: "apigatewaymethod" },
      { label: "ApiGatewayModel", anchor: "apigatewaymodel" },
      { label: "ApiGatewayRequestValidator", anchor: "apigatewayrequestvalidator" },
      { label: "ApiGatewayResource", anchor: "apigatewayresource" },
      { label: "ApiGatewayUsagePlan", anchor: "apigatewayusageplan" },
      { label: "ApiGatewayUsagePlanKey", anchor: "apigatewayusageplankey" },
      { label: "ApiGatewayVpcLink", anchor: "apigatewayvpclink" },
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
      { label: "APIGatewayV2VpcLink", anchor: "apigatewayv2vpclink" },
      { label: "ApiGatewayV2ApiGatewayManagedOverrides", anchor: "apigatewayv2apigatewaymanagedoverrides" },
      { label: "ApiGatewayV2Deployment", anchor: "apigatewayv2deployment" },
      { label: "ApiGatewayV2IntegrationResponse", anchor: "apigatewayv2integrationresponse" },
      { label: "ApiGatewayV2Model", anchor: "apigatewayv2model" },
      { label: "ApiGatewayV2PortalProduct", anchor: "apigatewayv2portalproduct" },
      { label: "ApiGatewayV2RouteResponse", anchor: "apigatewayv2routeresponse" },
      { label: "ApiGatewayV2RoutingRule", anchor: "apigatewayv2routingrule" }
    ]
  },
  {
    id: "apprunner",
    label: "App Runner",
    href: "/docs/apprunner.html",
    items: [
      { label: "AppRunnerAutoScaling", anchor: "apprunnerautoscaling" },
      { label: "AppRunnerObservabilityConfiguration", anchor: "apprunnerobservabilityconfiguration" },
      { label: "AppRunnerService", anchor: "apprunnerservice" },
      { label: "AppRunnerVpcConnector", anchor: "apprunnervpcconnector" },
      { label: "AppRunnerVpcIngressConnection", anchor: "apprunnervpcingressconnection" }
    ]
  },
  {
    id: "appconfig",
    label: "APPCONFIG",
    href: "/docs/appconfig.html",
    items: [
      { label: "AppConfigApplication", anchor: "appconfigapplication" },
      { label: "AppConfigConfigurationProfile", anchor: "appconfigconfigurationprofile" },
      { label: "AppConfigDeployment", anchor: "appconfigdeployment" },
      { label: "AppConfigDeploymentStrategy", anchor: "appconfigdeploymentstrategy" },
      { label: "AppConfigEnvironment", anchor: "appconfigenvironment" },
      { label: "AppConfigExperimentDefinition", anchor: "appconfigexperimentdefinition" },
      { label: "AppConfigExperimentRun", anchor: "appconfigexperimentrun" },
      { label: "AppConfigExtension", anchor: "appconfigextension" },
      { label: "AppConfigExtensionAssociation", anchor: "appconfigextensionassociation" },
      { label: "AppConfigHostedConfigurationVersion", anchor: "appconfighostedconfigurationversion" }
    ]
  },
  {
    id: "appflow",
    label: "APPFLOW",
    href: "/docs/appflow.html",
    items: [
      { label: "AppFlowConnector", anchor: "appflowconnector" },
      { label: "AppFlowConnectorProfile", anchor: "appflowconnectorprofile" },
      { label: "AppFlowFlow", anchor: "appflowflow" }
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
    id: "applicationinsights",
    label: "APPLICATIONINSIGHTS",
    href: "/docs/applicationinsights.html",
    items: [
      { label: "ApplicationInsightsApplication", anchor: "applicationinsightsapplication" }
    ]
  },
  {
    id: "applicationsignals",
    label: "APPLICATIONSIGNALS",
    href: "/docs/applicationsignals.html",
    items: [
      { label: "ApplicationSignalsDiscovery", anchor: "applicationsignalsdiscovery" },
      { label: "ApplicationSignalsGroupingConfiguration", anchor: "applicationsignalsgroupingconfiguration" },
      { label: "ApplicationSignalsServiceLevelObjective", anchor: "applicationsignalsservicelevelobjective" }
    ]
  },
  {
    id: "appsync",
    label: "APPSYNC",
    href: "/docs/appsync.html",
    items: [
      { label: "AppSyncApi", anchor: "appsyncapi" },
      { label: "AppSyncChannelNamespace", anchor: "appsyncchannelnamespace" },
      { label: "AppSyncDataSource", anchor: "appsyncdatasource" },
      { label: "AppSyncDomainName", anchor: "appsyncdomainname" },
      { label: "AppSyncDomainNameApiAssociation", anchor: "appsyncdomainnameapiassociation" },
      { label: "AppSyncFunctionConfiguration", anchor: "appsyncfunctionconfiguration" },
      { label: "AppSyncGraphQLApi", anchor: "appsyncgraphqlapi" },
      { label: "AppSyncResolver", anchor: "appsyncresolver" },
      { label: "AppSyncSourceApiAssociation", anchor: "appsyncsourceapiassociation" },
      { label: "AppSyncType", anchor: "appsynctype" }
    ]
  },
  {
    id: "arczonalshift",
    label: "ARCZONALSHIFT",
    href: "/docs/arczonalshift.html",
    items: [
      { label: "ARCZonalShiftAutoshiftObserverNotificationStatus", anchor: "arczonalshiftautoshiftobservernotificationstatus" },
      { label: "ARCZonalShiftZonalAutoshiftConfiguration", anchor: "arczonalshiftzonalautoshiftconfiguration" }
    ]
  },
  {
    id: "athena",
    label: "Athena",
    href: "/docs/athena.html",
    items: [
      { label: "AthenaCapacityReservation", anchor: "athenacapacityreservation" },
      { label: "AthenaDataCatalog", anchor: "athenadatacatalog" },
      { label: "AthenaNamedQuery", anchor: "athenanamedquery" },
      { label: "AthenaPreparedStatement", anchor: "athenapreparedstatement" },
      { label: "AthenaWorkGroup", anchor: "athenaworkgroup" }
    ]
  },
  {
    id: "auditmanager",
    label: "AUDITMANAGER",
    href: "/docs/auditmanager.html",
    items: [
      { label: "AuditManagerAssessment", anchor: "auditmanagerassessment" },
      { label: "AuditManagerAssessmentFramework", anchor: "auditmanagerassessmentframework" }
    ]
  },
  {
    id: "autoscaling",
    label: "Auto Scaling",
    href: "/docs/autoscaling.html",
    items: [
      { label: "AutoScalingGroup", anchor: "autoscalinggroup" },
      { label: "AutoScalingLaunchConfiguration", anchor: "autoscalinglaunchconfiguration" },
      { label: "AutoScalingLifecycleHook", anchor: "autoscalinglifecyclehook" },
      { label: "AutoScalingScheduledAction", anchor: "autoscalingscheduledaction" },
      { label: "AutoScalingWarmPool", anchor: "autoscalingwarmpool" }
    ]
  },
  {
    id: "configservice",
    label: "AWS Config",
    href: "/docs/configservice.html",
    items: [
      { label: "ConfigAggregationAuthorization", anchor: "configaggregationauthorization" },
      { label: "ConfigConfigurationAggregator", anchor: "configconfigurationaggregator" },
      { label: "ConfigConformancePack", anchor: "configconformancepack" },
      { label: "ConfigConnector", anchor: "configconnector" },
      { label: "ConfigDeliveryChannel", anchor: "configdeliverychannel" },
      { label: "ConfigOrganizationConformancePack", anchor: "configorganizationconformancepack" },
      { label: "ConfigRecorder", anchor: "configrecorder" },
      { label: "ConfigRemediationConfiguration", anchor: "configremediationconfiguration" },
      { label: "ConfigRule", anchor: "configrule" },
      { label: "ConfigStoredQuery", anchor: "configstoredquery" }
    ]
  },
  {
    id: "backup",
    label: "Backup",
    href: "/docs/backup.html",
    items: [
      { label: "BackupFramework", anchor: "backupframework" },
      { label: "BackupLegalHold", anchor: "backuplegalhold" },
      { label: "BackupLogicallyAirGappedBackupVault", anchor: "backuplogicallyairgappedbackupvault" },
      { label: "BackupPlan", anchor: "backupplan" },
      { label: "BackupReportPlan", anchor: "backupreportplan" },
      { label: "BackupRestoreTestingPlan", anchor: "backuprestoretestingplan" },
      { label: "BackupRestoreTestingSelection", anchor: "backuprestoretestingselection" },
      { label: "BackupSelection", anchor: "backupselection" },
      { label: "BackupTieringConfiguration", anchor: "backuptieringconfiguration" },
      { label: "BackupVault", anchor: "backupvault" }
    ]
  },
  {
    id: "batch",
    label: "Batch",
    href: "/docs/batch.html",
    items: [
      { label: "BatchComputeEnvironment", anchor: "batchcomputeenvironment" },
      { label: "BatchConsumableResource", anchor: "batchconsumableresource" },
      { label: "BatchJobDefinition", anchor: "batchjobdefinition" },
      { label: "BatchJobQueue", anchor: "batchjobqueue" },
      { label: "BatchQuotaShare", anchor: "batchquotashare" },
      { label: "BatchSchedulingPolicy", anchor: "batchschedulingpolicy" },
      { label: "BatchServiceEnvironment", anchor: "batchserviceenvironment" }
    ]
  },
  {
    id: "bedrock",
    label: "BEDROCK",
    href: "/docs/bedrock.html",
    items: [
      { label: "BedrockAgent", anchor: "bedrockagent" },
      { label: "BedrockAgentAlias", anchor: "bedrockagentalias" },
      { label: "BedrockApplicationInferenceProfile", anchor: "bedrockapplicationinferenceprofile" },
      { label: "BedrockAutomatedReasoningPolicy", anchor: "bedrockautomatedreasoningpolicy" },
      { label: "BedrockAutomatedReasoningPolicyVersion", anchor: "bedrockautomatedreasoningpolicyversion" },
      { label: "BedrockBlueprint", anchor: "bedrockblueprint" },
      { label: "BedrockDataAutomationLibrary", anchor: "bedrockdataautomationlibrary" },
      { label: "BedrockDataAutomationProject", anchor: "bedrockdataautomationproject" },
      { label: "BedrockDataSource", anchor: "bedrockdatasource" },
      { label: "BedrockEnforcedGuardrailConfiguration", anchor: "bedrockenforcedguardrailconfiguration" },
      { label: "BedrockFlow", anchor: "bedrockflow" },
      { label: "BedrockFlowAlias", anchor: "bedrockflowalias" },
      { label: "BedrockFlowVersion", anchor: "bedrockflowversion" },
      { label: "BedrockGuardrail", anchor: "bedrockguardrail" },
      { label: "BedrockGuardrailVersion", anchor: "bedrockguardrailversion" },
      { label: "BedrockIntelligentPromptRouter", anchor: "bedrockintelligentpromptrouter" },
      { label: "BedrockKnowledgeBase", anchor: "bedrockknowledgebase" },
      { label: "BedrockKnowledgeBasePolicy", anchor: "bedrockknowledgebasepolicy" },
      { label: "BedrockPrompt", anchor: "bedrockprompt" },
      { label: "BedrockPromptVersion", anchor: "bedrockpromptversion" },
      { label: "BedrockResourcePolicy", anchor: "bedrockresourcepolicy" },
      { label: "BedrockSession", anchor: "bedrocksession" }
    ]
  },
  {
    id: "bedrockagentcore",
    label: "BEDROCKAGENTCORE",
    href: "/docs/bedrockagentcore.html",
    items: [
      { label: "BedrockAgentCoreApiKeyCredentialProvider", anchor: "bedrockagentcoreapikeycredentialprovider" },
      { label: "BedrockAgentCoreBrowserCustom", anchor: "bedrockagentcorebrowsercustom" },
      { label: "BedrockAgentCoreBrowserProfile", anchor: "bedrockagentcorebrowserprofile" },
      { label: "BedrockAgentCoreCapacityProvider", anchor: "bedrockagentcorecapacityprovider" },
      { label: "BedrockAgentCoreCodeInterpreterCustom", anchor: "bedrockagentcorecodeinterpretercustom" },
      { label: "BedrockAgentCoreConfigurationBundle", anchor: "bedrockagentcoreconfigurationbundle" },
      { label: "BedrockAgentCoreDataset", anchor: "bedrockagentcoredataset" },
      { label: "BedrockAgentCoreEvaluator", anchor: "bedrockagentcoreevaluator" },
      { label: "BedrockAgentCoreGateway", anchor: "bedrockagentcoregateway" },
      { label: "BedrockAgentCoreGatewayRateLimit", anchor: "bedrockagentcoregatewayratelimit" },
      { label: "BedrockAgentCoreGatewayRule", anchor: "bedrockagentcoregatewayrule" },
      { label: "BedrockAgentCoreGatewayTarget", anchor: "bedrockagentcoregatewaytarget" },
      { label: "BedrockAgentCoreHarness", anchor: "bedrockagentcoreharness" },
      { label: "BedrockAgentCoreHarnessEndpoint", anchor: "bedrockagentcoreharnessendpoint" },
      { label: "BedrockAgentCoreMemory", anchor: "bedrockagentcorememory" },
      { label: "BedrockAgentCoreOAuth2CredentialProvider", anchor: "bedrockagentcoreoauth2credentialprovider" },
      { label: "BedrockAgentCoreOnlineEvaluationConfig", anchor: "bedrockagentcoreonlineevaluationconfig" },
      { label: "BedrockAgentCorePolicy", anchor: "bedrockagentcorepolicy" },
      { label: "BedrockAgentCorePolicyEngine", anchor: "bedrockagentcorepolicyengine" },
      { label: "BedrockAgentCoreResourcePolicy", anchor: "bedrockagentcoreresourcepolicy" },
      { label: "BedrockAgentCoreRuntime", anchor: "bedrockagentcoreruntime" },
      { label: "BedrockAgentCoreRuntimeEndpoint", anchor: "bedrockagentcoreruntimeendpoint" },
      { label: "BedrockAgentCoreWorkloadIdentity", anchor: "bedrockagentcoreworkloadidentity" }
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
    id: "cassandra",
    label: "CASSANDRA",
    href: "/docs/cassandra.html",
    items: [
      { label: "CassandraKeyspace", anchor: "cassandrakeyspace" },
      { label: "CassandraTable", anchor: "cassandratable" },
      { label: "CassandraType", anchor: "cassandratype" }
    ]
  },
  {
    id: "chatbot",
    label: "CHATBOT",
    href: "/docs/chatbot.html",
    items: [
      { label: "ChatbotCustomAction", anchor: "chatbotcustomaction" },
      { label: "ChatbotMicrosoftTeamsChannelConfiguration", anchor: "chatbotmicrosoftteamschannelconfiguration" },
      { label: "ChatbotSlackChannelConfiguration", anchor: "chatbotslackchannelconfiguration" }
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
      { label: "CloudMapPrivateDnsNamespace", anchor: "cloudmapprivatednsnamespace" },
      { label: "CloudMapPublicDnsNamespace", anchor: "cloudmappublicdnsnamespace" },
      { label: "CloudMapService", anchor: "cloudmapservice" }
    ]
  },
  {
    id: "cloudformation",
    label: "CloudFormation",
    href: "/docs/cloudformation.html",
    items: [
      { label: "CloudFormationChangeSet", anchor: "cloudformationchangeset" },
      { label: "CloudFormationGeneratedTemplate", anchor: "cloudformationgeneratedtemplate" },
      { label: "CloudFormationGuardHook", anchor: "cloudformationguardhook" },
      { label: "CloudFormationHookDefaultVersion", anchor: "cloudformationhookdefaultversion" },
      { label: "CloudFormationHookTypeConfig", anchor: "cloudformationhooktypeconfig" },
      { label: "CloudFormationHookVersion", anchor: "cloudformationhookversion" },
      { label: "CloudFormationLambdaHook", anchor: "cloudformationlambdahook" },
      { label: "CloudFormationModuleDefaultVersion", anchor: "cloudformationmoduledefaultversion" },
      { label: "CloudFormationModuleVersion", anchor: "cloudformationmoduleversion" },
      { label: "CloudFormationPublicTypeVersion", anchor: "cloudformationpublictypeversion" },
      { label: "CloudFormationPublisher", anchor: "cloudformationpublisher" },
      { label: "CloudFormationResourceDefaultVersion", anchor: "cloudformationresourcedefaultversion" },
      { label: "CloudFormationResourceVersion", anchor: "cloudformationresourceversion" },
      { label: "CloudFormationStack", anchor: "cloudformationstack" },
      { label: "CloudFormationStackSet", anchor: "cloudformationstackset" },
      { label: "CloudFormationTypeActivation", anchor: "cloudformationtypeactivation" }
    ]
  },
  {
    id: "cloudfront",
    label: "CloudFront",
    href: "/docs/cloudfront.html",
    items: [
      { label: "CloudFrontAnycastIpList", anchor: "cloudfrontanycastiplist" },
      { label: "CloudFrontCachePolicy", anchor: "cloudfrontcachepolicy" },
      { label: "CloudFrontCloudFrontOriginAccessIdentity", anchor: "cloudfrontcloudfrontoriginaccessidentity" },
      { label: "CloudFrontConnectionFunction", anchor: "cloudfrontconnectionfunction" },
      { label: "CloudFrontConnectionGroup", anchor: "cloudfrontconnectiongroup" },
      { label: "CloudFrontContinuousDeploymentPolicy", anchor: "cloudfrontcontinuousdeploymentpolicy" },
      { label: "CloudFrontDistribution", anchor: "cloudfrontdistribution" },
      { label: "CloudFrontDistributionTenant", anchor: "cloudfrontdistributiontenant" },
      { label: "CloudFrontFunction", anchor: "cloudfrontfunction" },
      { label: "CloudFrontKeyGroup", anchor: "cloudfrontkeygroup" },
      { label: "CloudFrontKeyValueStore", anchor: "cloudfrontkeyvaluestore" },
      { label: "CloudFrontMonitoringSubscription", anchor: "cloudfrontmonitoringsubscription" },
      { label: "CloudFrontOriginAccessControl", anchor: "cloudfrontoriginaccesscontrol" },
      { label: "CloudFrontOriginRequestPolicy", anchor: "cloudfrontoriginrequestpolicy" },
      { label: "CloudFrontPublicKey", anchor: "cloudfrontpublickey" },
      { label: "CloudFrontRealtimeLogConfig", anchor: "cloudfrontrealtimelogconfig" },
      { label: "CloudFrontResponseHeadersPolicy", anchor: "cloudfrontresponseheaderspolicy" },
      { label: "CloudFrontTrustStore", anchor: "cloudfronttruststore" },
      { label: "CloudFrontVpcOrigin", anchor: "cloudfrontvpcorigin" }
    ]
  },
  {
    id: "cloudhsm",
    label: "CLOUDHSM",
    href: "/docs/cloudhsm.html",
    items: [
      { label: "CloudHSMCluster", anchor: "cloudhsmcluster" }
    ]
  },
  {
    id: "cloudtrail",
    label: "CloudTrail",
    href: "/docs/cloudtrail.html",
    items: [
      { label: "CloudTrailChannel", anchor: "cloudtrailchannel" },
      { label: "CloudTrailDashboard", anchor: "cloudtraildashboard" },
      { label: "CloudTrailEventDataStore", anchor: "cloudtraileventdatastore" },
      { label: "CloudTrailResourcePolicy", anchor: "cloudtrailresourcepolicy" },
      { label: "Trail", anchor: "trail" }
    ]
  },
  {
    id: "cloudwatch",
    label: "CloudWatch",
    href: "/docs/cloudwatch.html",
    items: [
      { label: "CloudWatchAlarm", anchor: "cloudwatchalarm" },
      { label: "CloudWatchAlarmMuteRule", anchor: "cloudwatchalarmmuterule" },
      { label: "CloudWatchDashboard", anchor: "cloudwatchdashboard" },
      { label: "CloudWatchInsightRule", anchor: "cloudwatchinsightrule" },
      { label: "CloudWatchLogAlarm", anchor: "cloudwatchlogalarm" },
      { label: "CloudWatchMetricStream", anchor: "cloudwatchmetricstream" },
      { label: "CloudWatchOTelEnrichment", anchor: "cloudwatchotelenrichment" },
      { label: "CompositeAlarm", anchor: "compositealarm" }
    ]
  },
  {
    id: "cloudwatchlogs",
    label: "CloudWatch Logs",
    href: "/docs/cloudwatchlogs.html",
    items: [
      { label: "LogGroup", anchor: "loggroup" },
      { label: "LogsAccountPolicy", anchor: "logsaccountpolicy" },
      { label: "LogsDelivery", anchor: "logsdelivery" },
      { label: "LogsDeliveryDestination", anchor: "logsdeliverydestination" },
      { label: "LogsDeliverySource", anchor: "logsdeliverysource" },
      { label: "LogsDestination", anchor: "logsdestination" },
      { label: "LogsIntegration", anchor: "logsintegration" },
      { label: "LogsLogAnomalyDetector", anchor: "logsloganomalydetector" },
      { label: "LogsLogStream", anchor: "logslogstream" },
      { label: "LogsQueryDefinition", anchor: "logsquerydefinition" },
      { label: "LogsResourcePolicy", anchor: "logsresourcepolicy" },
      { label: "LogsScheduledQuery", anchor: "logsscheduledquery" },
      { label: "LogsStorageTierPolicy", anchor: "logsstoragetierpolicy" },
      { label: "LogsTransformer", anchor: "logstransformer" },
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
      { label: "CodeArtifactPackageGroup", anchor: "codeartifactpackagegroup" },
      { label: "CodeArtifactRepository", anchor: "codeartifactrepository" }
    ]
  },
  {
    id: "codebuild",
    label: "CodeBuild",
    href: "/docs/codebuild.html",
    items: [
      { label: "CodeBuildFleet", anchor: "codebuildfleet" },
      { label: "CodeBuildReportGroup", anchor: "codebuildreportgroup" },
      { label: "CodeBuildSourceCredential", anchor: "codebuildsourcecredential" }
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
    id: "codeconnections",
    label: "CODECONNECTIONS",
    href: "/docs/codeconnections.html",
    items: [
      { label: "CodeConnectionsConnection", anchor: "codeconnectionsconnection" },
      { label: "CodeConnectionsHost", anchor: "codeconnectionshost" }
    ]
  },
  {
    id: "codedeploy",
    label: "CodeDeploy",
    href: "/docs/codedeploy.html",
    items: [
      { label: "CodeDeployApplication", anchor: "codedeployapplication" },
      { label: "CodeDeployDeploymentConfig", anchor: "codedeploydeploymentconfig" },
      { label: "CodeDeployDeploymentGroup", anchor: "codedeploydeploymentgroup" }
    ]
  },
  {
    id: "codeguruprofiler",
    label: "CODEGURUPROFILER",
    href: "/docs/codeguruprofiler.html",
    items: [
      { label: "CodeGuruProfilerProfilingGroup", anchor: "codeguruprofilerprofilinggroup" }
    ]
  },
  {
    id: "codegurureviewer",
    label: "CODEGURUREVIEWER",
    href: "/docs/codegurureviewer.html",
    items: [
      { label: "CodeGuruReviewerRepositoryAssociation", anchor: "codegurureviewerrepositoryassociation" }
    ]
  },
  {
    id: "codepipeline",
    label: "CodePipeline",
    href: "/docs/codepipeline.html",
    items: [
      { label: "CodePipeline", anchor: "codepipeline" },
      { label: "CodePipelineCustomActionType", anchor: "codepipelinecustomactiontype" },
      { label: "CodePipelineWebhook", anchor: "codepipelinewebhook" }
    ]
  },
  {
    id: "codestarconnections",
    label: "CODESTARCONNECTIONS",
    href: "/docs/codestarconnections.html",
    items: [
      { label: "CodeStarConnectionsConnection", anchor: "codestarconnectionsconnection" },
      { label: "CodeStarConnectionsRepositoryLink", anchor: "codestarconnectionsrepositorylink" },
      { label: "CodeStarConnectionsSyncConfiguration", anchor: "codestarconnectionssyncconfiguration" }
    ]
  },
  {
    id: "codestarnotifications",
    label: "CODESTARNOTIFICATIONS",
    href: "/docs/codestarnotifications.html",
    items: [
      { label: "CodeStarNotificationsNotificationRule", anchor: "codestarnotificationsnotificationrule" }
    ]
  },
  {
    id: "cognito",
    label: "Cognito",
    href: "/docs/cognito.html",
    items: [
      { label: "CognitoIdentityPool", anchor: "cognitoidentitypool" },
      { label: "CognitoIdentityPoolPrincipalTag", anchor: "cognitoidentitypoolprincipaltag" },
      { label: "CognitoIdentityPoolRoleAttachment", anchor: "cognitoidentitypoolroleattachment" },
      { label: "CognitoLogDeliveryConfiguration", anchor: "cognitologdeliveryconfiguration" },
      { label: "CognitoManagedLoginBranding", anchor: "cognitomanagedloginbranding" },
      { label: "CognitoTerms", anchor: "cognitoterms" },
      { label: "CognitoUserPoolDomain", anchor: "cognitouserpooldomain" },
      { label: "CognitoUserPoolGroup", anchor: "cognitouserpoolgroup" },
      { label: "CognitoUserPoolIdentityProvider", anchor: "cognitouserpoolidentityprovider" },
      { label: "CognitoUserPoolRegionalConfigurationAttachment", anchor: "cognitouserpoolregionalconfigurationattachment" },
      { label: "CognitoUserPoolReplica", anchor: "cognitouserpoolreplica" },
      { label: "CognitoUserPoolResourceServer", anchor: "cognitouserpoolresourceserver" },
      { label: "CognitoUserPoolRiskConfigurationAttachment", anchor: "cognitouserpoolriskconfigurationattachment" },
      { label: "CognitoUserPoolUICustomizationAttachment", anchor: "cognitouserpooluicustomizationattachment" },
      { label: "CognitoUserPoolUser", anchor: "cognitouserpooluser" },
      { label: "CognitoUserPoolUserToGroupAttachment", anchor: "cognitouserpoolusertogroupattachment" },
      { label: "IdentityProvider", anchor: "identityprovider" },
      { label: "UserPool", anchor: "userpool" },
      { label: "UserPoolClient", anchor: "userpoolclient" }
    ]
  },
  {
    id: "computeoptimizer",
    label: "COMPUTEOPTIMIZER",
    href: "/docs/computeoptimizer.html",
    items: [
      { label: "ComputeOptimizerAutomationRule", anchor: "computeoptimizerautomationrule" }
    ]
  },
  {
    id: "controltower",
    label: "Control Tower",
    href: "/docs/controltower.html",
    items: [
      { label: "CTEnabledControl", anchor: "ctenabledcontrol" },
      { label: "ControlTowerEnabledBaseline", anchor: "controltowerenabledbaseline" },
      { label: "ControlTowerLandingZone", anchor: "controltowerlandingzone" }
    ]
  },
  {
    id: "costexplorer",
    label: "Cost Explorer",
    href: "/docs/costexplorer.html",
    items: [
      { label: "CECostCategory", anchor: "cecostcategory" },
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
    id: "datasync",
    label: "DATASYNC",
    href: "/docs/datasync.html",
    items: [
      { label: "DataSyncAgent", anchor: "datasyncagent" },
      { label: "DataSyncLocationAzureBlob", anchor: "datasynclocationazureblob" },
      { label: "DataSyncLocationEFS", anchor: "datasynclocationefs" },
      { label: "DataSyncLocationFSxLustre", anchor: "datasynclocationfsxlustre" },
      { label: "DataSyncLocationFSxONTAP", anchor: "datasynclocationfsxontap" },
      { label: "DataSyncLocationFSxOpenZFS", anchor: "datasynclocationfsxopenzfs" },
      { label: "DataSyncLocationFSxWindows", anchor: "datasynclocationfsxwindows" },
      { label: "DataSyncLocationHDFS", anchor: "datasynclocationhdfs" },
      { label: "DataSyncLocationNFS", anchor: "datasynclocationnfs" },
      { label: "DataSyncLocationObjectStorage", anchor: "datasynclocationobjectstorage" },
      { label: "DataSyncLocationS3", anchor: "datasynclocations3" },
      { label: "DataSyncLocationSMB", anchor: "datasynclocationsmb" },
      { label: "DataSyncTask", anchor: "datasynctask" }
    ]
  },
  {
    id: "datazone",
    label: "DATAZONE",
    href: "/docs/datazone.html",
    items: [
      { label: "DataZoneConnection", anchor: "datazoneconnection" },
      { label: "DataZoneDataSource", anchor: "datazonedatasource" },
      { label: "DataZoneDomain", anchor: "datazonedomain" },
      { label: "DataZoneDomainUnit", anchor: "datazonedomainunit" },
      { label: "DataZoneEnvironment", anchor: "datazoneenvironment" },
      { label: "DataZoneEnvironmentActions", anchor: "datazoneenvironmentactions" },
      { label: "DataZoneEnvironmentBlueprintConfiguration", anchor: "datazoneenvironmentblueprintconfiguration" },
      { label: "DataZoneEnvironmentProfile", anchor: "datazoneenvironmentprofile" },
      { label: "DataZoneFormType", anchor: "datazoneformtype" },
      { label: "DataZoneGroupProfile", anchor: "datazonegroupprofile" },
      { label: "DataZoneOwner", anchor: "datazoneowner" },
      { label: "DataZonePolicyGrant", anchor: "datazonepolicygrant" },
      { label: "DataZoneProject", anchor: "datazoneproject" },
      { label: "DataZoneProjectMembership", anchor: "datazoneprojectmembership" },
      { label: "DataZoneProjectProfile", anchor: "datazoneprojectprofile" },
      { label: "DataZoneSubscriptionTarget", anchor: "datazonesubscriptiontarget" },
      { label: "DataZoneUserProfile", anchor: "datazoneuserprofile" }
    ]
  },
  {
    id: "dax",
    label: "DAX",
    href: "/docs/dax.html",
    items: [
      { label: "DAXCluster", anchor: "daxcluster" },
      { label: "DAXParameterGroup", anchor: "daxparametergroup" }
    ]
  },
  {
    id: "detective",
    label: "DETECTIVE",
    href: "/docs/detective.html",
    items: [
      { label: "DetectiveGraph", anchor: "detectivegraph" },
      { label: "DetectiveMemberInvitation", anchor: "detectivememberinvitation" },
      { label: "DetectiveOrganizationAdmin", anchor: "detectiveorganizationadmin" }
    ]
  },
  {
    id: "directconnect",
    label: "DIRECTCONNECT",
    href: "/docs/directconnect.html",
    items: [
      { label: "DirectConnectConnection", anchor: "directconnectconnection" },
      { label: "DirectConnectDirectConnectGateway", anchor: "directconnectdirectconnectgateway" },
      { label: "DirectConnectDirectConnectGatewayAssociation", anchor: "directconnectdirectconnectgatewayassociation" },
      { label: "DirectConnectLag", anchor: "directconnectlag" },
      { label: "DirectConnectPrivateVirtualInterface", anchor: "directconnectprivatevirtualinterface" },
      { label: "DirectConnectPublicVirtualInterface", anchor: "directconnectpublicvirtualinterface" },
      { label: "DirectConnectTransitVirtualInterface", anchor: "directconnecttransitvirtualinterface" }
    ]
  },
  {
    id: "dms",
    label: "DMS",
    href: "/docs/dms.html",
    items: [
      { label: "DMSCertificate", anchor: "dmscertificate" },
      { label: "DMSDataMigration", anchor: "dmsdatamigration" },
      { label: "DMSDataProvider", anchor: "dmsdataprovider" },
      { label: "DMSEndpoint", anchor: "dmsendpoint" },
      { label: "DMSEventSubscription", anchor: "dmseventsubscription" },
      { label: "DMSInstanceProfile", anchor: "dmsinstanceprofile" },
      { label: "DMSMigrationProject", anchor: "dmsmigrationproject" },
      { label: "DMSReplicationConfig", anchor: "dmsreplicationconfig" },
      { label: "DMSReplicationSubnetGroup", anchor: "dmsreplicationsubnetgroup" },
      { label: "DMSReplicationTask", anchor: "dmsreplicationtask" }
    ]
  },
  {
    id: "docdb",
    label: "DOCDB",
    href: "/docs/docdb.html",
    items: [
      { label: "DocDBDBClusterParameterGroup", anchor: "docdbdbclusterparametergroup" },
      { label: "DocDBDBSubnetGroup", anchor: "docdbdbsubnetgroup" },
      { label: "DocDBEventSubscription", anchor: "docdbeventsubscription" },
      { label: "DocDBGlobalCluster", anchor: "docdbglobalcluster" }
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
      { label: "EC2ApplicationStatusCheck", anchor: "ec2applicationstatuscheck" },
      { label: "EC2CapacityManagerDataExport", anchor: "ec2capacitymanagerdataexport" },
      { label: "EC2CapacityReservationFleet", anchor: "ec2capacityreservationfleet" },
      { label: "EC2CarrierGateway", anchor: "ec2carriergateway" },
      { label: "EC2DHCPOptions", anchor: "ec2dhcpoptions" },
      { label: "EC2EC2Fleet", anchor: "ec2ec2fleet" },
      { label: "EC2EnclaveCertificateIamRoleAssociation", anchor: "ec2enclavecertificateiamroleassociation" },
      { label: "EC2FpgaImage", anchor: "ec2fpgaimage" },
      { label: "EC2GatewayRouteTableAssociation", anchor: "ec2gatewayroutetableassociation" },
      { label: "EC2Host", anchor: "ec2host" },
      { label: "EC2IPAM", anchor: "ec2ipam" },
      { label: "EC2IPAMAllocation", anchor: "ec2ipamallocation" },
      { label: "EC2IPAMPool", anchor: "ec2ipampool" },
      { label: "EC2IPAMPoolCidr", anchor: "ec2ipampoolcidr" },
      { label: "EC2IPAMPrefixListResolver", anchor: "ec2ipamprefixlistresolver" },
      { label: "EC2IPAMPrefixListResolverTarget", anchor: "ec2ipamprefixlistresolvertarget" },
      { label: "EC2IPAMResourceDiscovery", anchor: "ec2ipamresourcediscovery" },
      { label: "EC2IPAMResourceDiscoveryAssociation", anchor: "ec2ipamresourcediscoveryassociation" },
      { label: "EC2IPAMScope", anchor: "ec2ipamscope" },
      { label: "EC2Instance", anchor: "ec2instance" },
      { label: "EC2InstanceConnectEndpoint", anchor: "ec2instanceconnectendpoint" },
      { label: "EC2IpPoolRouteTableAssociation", anchor: "ec2ippoolroutetableassociation" },
      { label: "EC2IpamExternalResourceVerificationToken", anchor: "ec2ipamexternalresourceverificationtoken" },
      { label: "EC2LocalGatewayRoute", anchor: "ec2localgatewayroute" },
      { label: "EC2LocalGatewayRouteTable", anchor: "ec2localgatewayroutetable" },
      { label: "EC2LocalGatewayRouteTableVPCAssociation", anchor: "ec2localgatewayroutetablevpcassociation" },
      { label: "EC2LocalGatewayRouteTableVirtualInterfaceGroupAssociation", anchor: "ec2localgatewayroutetablevirtualinterfacegroupassociation" },
      { label: "EC2LocalGatewayVirtualInterface", anchor: "ec2localgatewayvirtualinterface" },
      { label: "EC2LocalGatewayVirtualInterfaceGroup", anchor: "ec2localgatewayvirtualinterfacegroup" },
      { label: "EC2NetworkInsightsAccessScope", anchor: "ec2networkinsightsaccessscope" },
      { label: "EC2NetworkInsightsAccessScopeAnalysis", anchor: "ec2networkinsightsaccessscopeanalysis" },
      { label: "EC2NetworkInsightsAnalysis", anchor: "ec2networkinsightsanalysis" },
      { label: "EC2NetworkInsightsPath", anchor: "ec2networkinsightspath" },
      { label: "EC2NetworkInterface", anchor: "ec2networkinterface" },
      { label: "EC2NetworkInterfaceAttachment", anchor: "ec2networkinterfaceattachment" },
      { label: "EC2NetworkPerformanceMetricSubscription", anchor: "ec2networkperformancemetricsubscription" },
      { label: "EC2Route", anchor: "ec2route" },
      { label: "EC2RouteServer", anchor: "ec2routeserver" },
      { label: "EC2RouteServerAssociation", anchor: "ec2routeserverassociation" },
      { label: "EC2RouteServerEndpoint", anchor: "ec2routeserverendpoint" },
      { label: "EC2RouteServerPeer", anchor: "ec2routeserverpeer" },
      { label: "EC2RouteServerPropagation", anchor: "ec2routeserverpropagation" },
      { label: "EC2SecurityGroupEgress", anchor: "ec2securitygroupegress" },
      { label: "EC2SecurityGroupIngress", anchor: "ec2securitygroupingress" },
      { label: "EC2SecurityGroupVpcAssociation", anchor: "ec2securitygroupvpcassociation" },
      { label: "EC2SnapshotBlockPublicAccess", anchor: "ec2snapshotblockpublicaccess" },
      { label: "EC2SqlHaStandbyDetectedInstance", anchor: "ec2sqlhastandbydetectedinstance" },
      { label: "EC2SubnetCidrBlock", anchor: "ec2subnetcidrblock" },
      { label: "EC2SubnetNetworkAclAssociation", anchor: "ec2subnetnetworkaclassociation" },
      { label: "EC2SubnetRouteTableAssociation", anchor: "ec2subnetroutetableassociation" },
      { label: "EC2TrafficMirrorFilter", anchor: "ec2trafficmirrorfilter" },
      { label: "EC2TrafficMirrorFilterRule", anchor: "ec2trafficmirrorfilterrule" },
      { label: "EC2TrafficMirrorSession", anchor: "ec2trafficmirrorsession" },
      { label: "EC2TrafficMirrorTarget", anchor: "ec2trafficmirrortarget" },
      { label: "EC2TransitGatewayConnect", anchor: "ec2transitgatewayconnect" },
      { label: "EC2TransitGatewayConnectPeer", anchor: "ec2transitgatewayconnectpeer" },
      { label: "EC2TransitGatewayMeteringPolicy", anchor: "ec2transitgatewaymeteringpolicy" },
      { label: "EC2TransitGatewayMeteringPolicyEntry", anchor: "ec2transitgatewaymeteringpolicyentry" },
      { label: "EC2TransitGatewayMulticastDomain", anchor: "ec2transitgatewaymulticastdomain" },
      { label: "EC2TransitGatewayMulticastDomainAssociation", anchor: "ec2transitgatewaymulticastdomainassociation" },
      { label: "EC2TransitGatewayMulticastGroupMember", anchor: "ec2transitgatewaymulticastgroupmember" },
      { label: "EC2TransitGatewayMulticastGroupSource", anchor: "ec2transitgatewaymulticastgroupsource" },
      { label: "EC2TransitGatewayPeeringAttachment", anchor: "ec2transitgatewaypeeringattachment" },
      { label: "EC2TransitGatewayPolicyTable", anchor: "ec2transitgatewaypolicytable" },
      { label: "EC2TransitGatewayPolicyTableAssociation", anchor: "ec2transitgatewaypolicytableassociation" },
      { label: "EC2TransitGatewayPolicyTableEntry", anchor: "ec2transitgatewaypolicytableentry" },
      { label: "EC2TransitGatewayRoute", anchor: "ec2transitgatewayroute" },
      { label: "EC2TransitGatewayRouteTable", anchor: "ec2transitgatewayroutetable" },
      { label: "EC2TransitGatewayRouteTableAssociation", anchor: "ec2transitgatewayroutetableassociation" },
      { label: "EC2TransitGatewayRouteTablePropagation", anchor: "ec2transitgatewayroutetablepropagation" },
      { label: "EC2VPCBlockPublicAccessExclusion", anchor: "ec2vpcblockpublicaccessexclusion" },
      { label: "EC2VPCBlockPublicAccessOptions", anchor: "ec2vpcblockpublicaccessoptions" },
      { label: "EC2VPCCidrBlock", anchor: "ec2vpccidrblock" },
      { label: "EC2VPCDHCPOptionsAssociation", anchor: "ec2vpcdhcpoptionsassociation" },
      { label: "EC2VPCEncryptionControl", anchor: "ec2vpcencryptioncontrol" },
      { label: "EC2VPCEndpointConnectionNotification", anchor: "ec2vpcendpointconnectionnotification" },
      { label: "EC2VPCEndpointServicePermissions", anchor: "ec2vpcendpointservicepermissions" },
      { label: "EC2VPCGatewayAttachment", anchor: "ec2vpcgatewayattachment" },
      { label: "EC2VPNConcentrator", anchor: "ec2vpnconcentrator" },
      { label: "EC2VerifiedAccessEndpoint", anchor: "ec2verifiedaccessendpoint" },
      { label: "EC2VerifiedAccessGroup", anchor: "ec2verifiedaccessgroup" },
      { label: "EC2VerifiedAccessInstance", anchor: "ec2verifiedaccessinstance" },
      { label: "EC2VerifiedAccessTrustProvider", anchor: "ec2verifiedaccesstrustprovider" },
      { label: "EC2VolumeAttachment", anchor: "ec2volumeattachment" },
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
      { label: "ECRPublicRepository", anchor: "ecrpublicrepository" },
      { label: "ECRPullThroughCacheRule", anchor: "ecrpullthroughcacherule" },
      { label: "ECRPullTimeUpdateExclusion", anchor: "ecrpulltimeupdateexclusion" },
      { label: "ECRRegistryPolicy", anchor: "ecrregistrypolicy" },
      { label: "ECRRegistryScanningConfiguration", anchor: "ecrregistryscanningconfiguration" },
      { label: "ECRReplicationConfiguration", anchor: "ecrreplicationconfiguration" },
      { label: "ECRRepository", anchor: "ecrrepository" },
      { label: "ECRRepositoryCreationTemplate", anchor: "ecrrepositorycreationtemplate" },
      { label: "ECRRepositoryPolicy", anchor: "ecrrepositorypolicy" },
      { label: "ECRSigningConfiguration", anchor: "ecrsigningconfiguration" }
    ]
  },
  {
    id: "ecs",
    label: "ECS",
    href: "/docs/ecs.html",
    items: [
      { label: "ECSCapacityProvider", anchor: "ecscapacityprovider" },
      { label: "ECSCluster", anchor: "ecscluster" },
      { label: "ECSClusterCapacityProviderAssociations", anchor: "ecsclustercapacityproviderassociations" },
      { label: "ECSDaemon", anchor: "ecsdaemon" },
      { label: "ECSDaemonTaskDefinition", anchor: "ecsdaemontaskdefinition" },
      { label: "ECSExpressGatewayService", anchor: "ecsexpressgatewayservice" },
      { label: "ECSPrimaryTaskSet", anchor: "ecsprimarytaskset" },
      { label: "ECSService", anchor: "ecsservice" },
      { label: "ECSTaskDefinition", anchor: "ecstaskdefinition" },
      { label: "ECSTaskSet", anchor: "ecstaskset" }
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
      { label: "EKSCapability", anchor: "ekscapability" },
      { label: "EKSCertificateAuthority", anchor: "ekscertificateauthority" },
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
      { label: "ElastiCacheCacheCluster", anchor: "elasticachecachecluster" },
      { label: "ElastiCacheGlobalReplicationGroup", anchor: "elasticacheglobalreplicationgroup" },
      { label: "ElastiCacheParameterGroup", anchor: "elasticacheparametergroup" },
      { label: "ElastiCacheReplicationGroup", anchor: "elasticachereplicationgroup" },
      { label: "ElastiCacheServerlessCache", anchor: "elasticacheserverlesscache" },
      { label: "ElastiCacheServerlessCacheSnapshot", anchor: "elasticacheserverlesscachesnapshot" },
      { label: "ElastiCacheSubnetGroup", anchor: "elasticachesubnetgroup" },
      { label: "ElastiCacheUser", anchor: "elasticacheuser" },
      { label: "ElastiCacheUserGroup", anchor: "elasticacheusergroup" }
    ]
  },
  {
    id: "elasticbeanstalk",
    label: "ELASTICBEANSTALK",
    href: "/docs/elasticbeanstalk.html",
    items: [
      { label: "ElasticBeanstalkApplication", anchor: "elasticbeanstalkapplication" },
      { label: "ElasticBeanstalkApplicationVersion", anchor: "elasticbeanstalkapplicationversion" },
      { label: "ElasticBeanstalkConfigurationTemplate", anchor: "elasticbeanstalkconfigurationtemplate" },
      { label: "ElasticBeanstalkEnvironment", anchor: "elasticbeanstalkenvironment" }
    ]
  },
  {
    id: "emr",
    label: "EMR",
    href: "/docs/emr.html",
    items: [
      { label: "EMRSecurityConfiguration", anchor: "emrsecurityconfiguration" },
      { label: "EMRStep", anchor: "emrstep" },
      { label: "EMRStudio", anchor: "emrstudio" },
      { label: "EMRStudioSessionMapping", anchor: "emrstudiosessionmapping" },
      { label: "EMRWALWorkspace", anchor: "emrwalworkspace" }
    ]
  },
  {
    id: "emrcontainers",
    label: "EMRCONTAINERS",
    href: "/docs/emrcontainers.html",
    items: [
      { label: "EMRContainersEndpoint", anchor: "emrcontainersendpoint" },
      { label: "EMRContainersSecurityConfiguration", anchor: "emrcontainerssecurityconfiguration" },
      { label: "EMRContainersVirtualCluster", anchor: "emrcontainersvirtualcluster" }
    ]
  },
  {
    id: "emrserverless",
    label: "EMRSERVERLESS",
    href: "/docs/emrserverless.html",
    items: [
      { label: "EMRServerlessApplication", anchor: "emrserverlessapplication" }
    ]
  },
  {
    id: "eventbridge",
    label: "EventBridge",
    href: "/docs/eventbridge.html",
    items: [
      { label: "ECSScheduledTask", anchor: "ecsscheduledtask" },
      { label: "EventBridgeApiDestination", anchor: "eventbridgeapidestination" },
      { label: "EventBridgeArchive", anchor: "eventbridgearchive" },
      { label: "EventBridgeConnection", anchor: "eventbridgeconnection" },
      { label: "EventBridgeEndpoint", anchor: "eventbridgeendpoint" },
      { label: "EventBridgeEventBusPolicy", anchor: "eventbridgeeventbuspolicy" },
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
    id: "eventschemas",
    label: "EVENTSCHEMAS",
    href: "/docs/eventschemas.html",
    items: [
      { label: "EventSchemasDiscoverer", anchor: "eventschemasdiscoverer" },
      { label: "EventSchemasRegistry", anchor: "eventschemasregistry" },
      { label: "EventSchemasRegistryPolicy", anchor: "eventschemasregistrypolicy" },
      { label: "EventSchemasSchema", anchor: "eventschemasschema" }
    ]
  },
  {
    id: "evidently",
    label: "EVIDENTLY",
    href: "/docs/evidently.html",
    items: [
      { label: "EvidentlyExperiment", anchor: "evidentlyexperiment" },
      { label: "EvidentlyFeature", anchor: "evidentlyfeature" },
      { label: "EvidentlyLaunch", anchor: "evidentlylaunch" },
      { label: "EvidentlyProject", anchor: "evidentlyproject" },
      { label: "EvidentlySegment", anchor: "evidentlysegment" }
    ]
  },
  {
    id: "fis",
    label: "FIS",
    href: "/docs/fis.html",
    items: [
      { label: "FISExperimentTemplate", anchor: "fisexperimenttemplate" },
      { label: "FISTargetAccountConfiguration", anchor: "fistargetaccountconfiguration" }
    ]
  },
  {
    id: "fms",
    label: "FMS",
    href: "/docs/fms.html",
    items: [
      { label: "FMSNotificationChannel", anchor: "fmsnotificationchannel" },
      { label: "FMSPolicy", anchor: "fmspolicy" },
      { label: "FMSResourceSet", anchor: "fmsresourceset" }
    ]
  },
  {
    id: "fsx",
    label: "FSX",
    href: "/docs/fsx.html",
    items: [
      { label: "FSxDataRepositoryAssociation", anchor: "fsxdatarepositoryassociation" },
      { label: "FSxS3AccessPointAttachment", anchor: "fsxs3accesspointattachment" },
      { label: "FSxVolume", anchor: "fsxvolume" }
    ]
  },
  {
    id: "globalaccelerator",
    label: "GLOBALACCELERATOR",
    href: "/docs/globalaccelerator.html",
    items: [
      { label: "GlobalAcceleratorAccelerator", anchor: "globalacceleratoraccelerator" },
      { label: "GlobalAcceleratorCrossAccountAttachment", anchor: "globalacceleratorcrossaccountattachment" },
      { label: "GlobalAcceleratorEndpointGroup", anchor: "globalacceleratorendpointgroup" },
      { label: "GlobalAcceleratorListener", anchor: "globalacceleratorlistener" }
    ]
  },
  {
    id: "glue",
    label: "Glue",
    href: "/docs/glue.html",
    items: [
      { label: "GlueBlueprint", anchor: "glueblueprint" },
      { label: "GlueCatalog", anchor: "gluecatalog" },
      { label: "GlueClassifier", anchor: "glueclassifier" },
      { label: "GlueConnection", anchor: "glueconnection" },
      { label: "GlueCrawler", anchor: "gluecrawler" },
      { label: "GlueCustomEntityType", anchor: "gluecustomentitytype" },
      { label: "GlueDataCatalogEncryptionSettings", anchor: "gluedatacatalogencryptionsettings" },
      { label: "GlueDataQualityRuleset", anchor: "gluedataqualityruleset" },
      { label: "GlueDatabase", anchor: "gluedatabase" },
      { label: "GlueIdentityCenterConfiguration", anchor: "glueidentitycenterconfiguration" },
      { label: "GlueIntegration", anchor: "glueintegration" },
      { label: "GlueIntegrationResourceProperty", anchor: "glueintegrationresourceproperty" },
      { label: "GlueJob", anchor: "gluejob" },
      { label: "GlueMLTransform", anchor: "gluemltransform" },
      { label: "GlueRegistry", anchor: "glueregistry" },
      { label: "GlueSchema", anchor: "glueschema" },
      { label: "GlueSchemaVersion", anchor: "glueschemaversion" },
      { label: "GlueSchemaVersionMetadata", anchor: "glueschemaversionmetadata" },
      { label: "GlueSecurityConfiguration", anchor: "gluesecurityconfiguration" },
      { label: "GlueSession", anchor: "gluesession" },
      { label: "GlueTableOptimizer", anchor: "gluetableoptimizer" },
      { label: "GlueTrigger", anchor: "gluetrigger" },
      { label: "GlueUsageProfile", anchor: "glueusageprofile" },
      { label: "GlueUserDefinedFunction", anchor: "glueuserdefinedfunction" },
      { label: "GlueWorkflow", anchor: "glueworkflow" }
    ]
  },
  {
    id: "guardduty",
    label: "GuardDuty",
    href: "/docs/guardduty.html",
    items: [
      { label: "GuardDutyCustomDetectionRuleAssociation", anchor: "guarddutycustomdetectionruleassociation" },
      { label: "GuardDutyDetector", anchor: "guarddutydetector" },
      { label: "GuardDutyFilter", anchor: "guarddutyfilter" },
      { label: "GuardDutyIPSet", anchor: "guarddutyipset" },
      { label: "GuardDutyMalwareProtectionPlan", anchor: "guarddutymalwareprotectionplan" },
      { label: "GuardDutyMaster", anchor: "guarddutymaster" },
      { label: "GuardDutyMember", anchor: "guarddutymember" },
      { label: "GuardDutyPublishingDestination", anchor: "guarddutypublishingdestination" },
      { label: "GuardDutyThreatEntitySet", anchor: "guarddutythreatentityset" },
      { label: "GuardDutyThreatIntelSet", anchor: "guarddutythreatintelset" },
      { label: "GuardDutyTrustedEntitySet", anchor: "guarddutytrustedentityset" }
    ]
  },
  {
    id: "iam",
    label: "IAM",
    href: "/docs/iam.html",
    items: [
      { label: "IAMGroup", anchor: "iamgroup" },
      { label: "IAMGroupMembership", anchor: "iamgroupmembership" },
      { label: "IAMGroupPolicy", anchor: "iamgrouppolicy" },
      { label: "IAMGroupPolicyAttachment", anchor: "iamgrouppolicyattachment" },
      { label: "IAMInstanceProfile", anchor: "iaminstanceprofile" },
      { label: "IAMOIDCProvider", anchor: "iamoidcprovider" },
      { label: "IAMPolicy", anchor: "iampolicy" },
      { label: "IAMPolicyAttachment", anchor: "iampolicyattachment" },
      { label: "IAMRole", anchor: "iamrole" },
      { label: "IAMRolePolicy", anchor: "iamrolepolicy" },
      { label: "IAMSAMLProvider", anchor: "iamsamlprovider" },
      { label: "IAMServerCertificate", anchor: "iamservercertificate" },
      { label: "IAMServiceLinkedRole", anchor: "iamservicelinkedrole" },
      { label: "IAMUser", anchor: "iamuser" },
      { label: "IAMUserPolicy", anchor: "iamuserpolicy" },
      { label: "IAMVirtualMFADevice", anchor: "iamvirtualmfadevice" }
    ]
  },
  {
    id: "ssoadmin",
    label: "IAM Identity Center",
    href: "/docs/ssoadmin.html",
    items: [
      { label: "PermissionSet", anchor: "permissionset" },
      { label: "SSOAdminApplication", anchor: "ssoadminapplication" },
      { label: "SSOAdminApplicationAssignment", anchor: "ssoadminapplicationassignment" },
      { label: "SSOAdminInstance", anchor: "ssoadmininstance" },
      { label: "SSOAdminInstanceAccessControlAttributeConfiguration", anchor: "ssoadmininstanceaccesscontrolattributeconfiguration" },
      { label: "SSOAssignment", anchor: "ssoassignment" }
    ]
  },
  {
    id: "identitystore",
    label: "IDENTITYSTORE",
    href: "/docs/identitystore.html",
    items: [
      { label: "IdentityStoreGroup", anchor: "identitystoregroup" },
      { label: "IdentityStoreGroupMembership", anchor: "identitystoregroupmembership" },
      { label: "IdentityStoreUser", anchor: "identitystoreuser" }
    ]
  },
  {
    id: "imagebuilder",
    label: "IMAGEBUILDER",
    href: "/docs/imagebuilder.html",
    items: [
      { label: "ImageBuilderComponent", anchor: "imagebuildercomponent" },
      { label: "ImageBuilderContainerRecipe", anchor: "imagebuildercontainerrecipe" },
      { label: "ImageBuilderDistributionConfiguration", anchor: "imagebuilderdistributionconfiguration" },
      { label: "ImageBuilderImagePipeline", anchor: "imagebuilderimagepipeline" },
      { label: "ImageBuilderImageRecipe", anchor: "imagebuilderimagerecipe" },
      { label: "ImageBuilderInfrastructureConfiguration", anchor: "imagebuilderinfrastructureconfiguration" },
      { label: "ImageBuilderLifecyclePolicy", anchor: "imagebuilderlifecyclepolicy" },
      { label: "ImageBuilderWorkflow", anchor: "imagebuilderworkflow" }
    ]
  },
  {
    id: "inspector2",
    label: "Inspector",
    href: "/docs/inspector2.html",
    items: [
      { label: "InspectorEnabler", anchor: "inspectorenabler" },
      { label: "InspectorV2CisScanConfiguration", anchor: "inspectorv2cisscanconfiguration" },
      { label: "InspectorV2CodeSecurityIntegration", anchor: "inspectorv2codesecurityintegration" },
      { label: "InspectorV2CodeSecurityScanConfiguration", anchor: "inspectorv2codesecurityscanconfiguration" },
      { label: "InspectorV2Connector", anchor: "inspectorv2connector" },
      { label: "InspectorV2Filter", anchor: "inspectorv2filter" }
    ]
  },
  {
    id: "inspector",
    label: "INSPECTOR",
    href: "/docs/inspector.html",
    items: [
      { label: "InspectorAssessmentTarget", anchor: "inspectorassessmenttarget" },
      { label: "InspectorAssessmentTemplate", anchor: "inspectorassessmenttemplate" },
      { label: "InspectorResourceGroup", anchor: "inspectorresourcegroup" }
    ]
  },
  {
    id: "internetmonitor",
    label: "INTERNETMONITOR",
    href: "/docs/internetmonitor.html",
    items: [
      { label: "InternetMonitorMonitor", anchor: "internetmonitormonitor" }
    ]
  },
  {
    id: "kafkaconnect",
    label: "KAFKACONNECT",
    href: "/docs/kafkaconnect.html",
    items: [
      { label: "KafkaConnectConnector", anchor: "kafkaconnectconnector" },
      { label: "KafkaConnectCustomPlugin", anchor: "kafkaconnectcustomplugin" },
      { label: "KafkaConnectWorkerConfiguration", anchor: "kafkaconnectworkerconfiguration" }
    ]
  },
  {
    id: "kinesis",
    label: "Kinesis",
    href: "/docs/kinesis.html",
    items: [
      { label: "KinesisResourcePolicy", anchor: "kinesisresourcepolicy" },
      { label: "KinesisStream", anchor: "kinesisstream" },
      { label: "KinesisStreamConsumer", anchor: "kinesisstreamconsumer" }
    ]
  },
  {
    id: "kinesisanalyticsv2",
    label: "KINESISANALYTICSV2",
    href: "/docs/kinesisanalyticsv2.html",
    items: [
      { label: "KinesisAnalyticsV2Application", anchor: "kinesisanalyticsv2application" }
    ]
  },
  {
    id: "kms",
    label: "KMS",
    href: "/docs/kms.html",
    items: [
      { label: "KMSAlias", anchor: "kmsalias" },
      { label: "KMSGrant", anchor: "kmsgrant" },
      { label: "KMSKey", anchor: "kmskey" },
      { label: "KMSReplicaKey", anchor: "kmsreplicakey" }
    ]
  },
  {
    id: "lakeformation",
    label: "LAKEFORMATION",
    href: "/docs/lakeformation.html",
    items: [
      { label: "LakeFormationDataCellsFilter", anchor: "lakeformationdatacellsfilter" },
      { label: "LakeFormationPrincipalPermissions", anchor: "lakeformationprincipalpermissions" },
      { label: "LakeFormationTag", anchor: "lakeformationtag" },
      { label: "LakeFormationTagAssociation", anchor: "lakeformationtagassociation" }
    ]
  },
  {
    id: "lambda",
    label: "Lambda",
    href: "/docs/lambda.html",
    items: [
      { label: "LambdaAlias", anchor: "lambdaalias" },
      { label: "LambdaCapacityProvider", anchor: "lambdacapacityprovider" },
      { label: "LambdaCodeSigningConfig", anchor: "lambdacodesigningconfig" },
      { label: "LambdaEventInvokeConfig", anchor: "lambdaeventinvokeconfig" },
      { label: "LambdaEventSourceMapping", anchor: "lambdaeventsourcemapping" },
      { label: "LambdaFunction", anchor: "lambdafunction" },
      { label: "LambdaFunctionURL", anchor: "lambdafunctionurl" },
      { label: "LambdaLayerVersion", anchor: "lambdalayerversion" },
      { label: "LambdaLayerVersionPermission", anchor: "lambdalayerversionpermission" },
      { label: "LambdaMicrovmImage", anchor: "lambdamicrovmimage" },
      { label: "LambdaNetworkConnector", anchor: "lambdanetworkconnector" },
      { label: "LambdaPermission", anchor: "lambdapermission" },
      { label: "LambdaProvisionedConcurrency", anchor: "lambdaprovisionedconcurrency" },
      { label: "LambdaResourcePolicy", anchor: "lambdaresourcepolicy" }
    ]
  },
  {
    id: "licensemanager",
    label: "LICENSEMANAGER",
    href: "/docs/licensemanager.html",
    items: [
      { label: "LicenseManagerGrant", anchor: "licensemanagergrant" },
      { label: "LicenseManagerLicense", anchor: "licensemanagerlicense" },
      { label: "LicenseManagerLicenseAssetRuleSet", anchor: "licensemanagerlicenseassetruleset" }
    ]
  },
  {
    id: "elbv2",
    label: "Load Balancing",
    href: "/docs/elbv2.html",
    items: [
      { label: "ELBLoadBalancer", anchor: "elbloadbalancer" },
      { label: "ELBv2TrustStore", anchor: "elbv2truststore" },
      { label: "ELBv2TrustStoreRevocation", anchor: "elbv2truststorerevocation" },
      { label: "Listener", anchor: "listener" },
      { label: "ListenerRule", anchor: "listenerrule" },
      { label: "LoadBalancer", anchor: "loadbalancer" },
      { label: "TargetGroup", anchor: "targetgroup" }
    ]
  },
  {
    id: "macie",
    label: "MACIE",
    href: "/docs/macie.html",
    items: [
      { label: "MacieAllowList", anchor: "macieallowlist" },
      { label: "MacieCustomDataIdentifier", anchor: "maciecustomdataidentifier" },
      { label: "MacieFindingsFilter", anchor: "maciefindingsfilter" },
      { label: "MacieSession", anchor: "maciesession" }
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
      { label: "PrometheusAnomalyDetector", anchor: "prometheusanomalydetector" },
      { label: "PrometheusResourcePolicy", anchor: "prometheusresourcepolicy" },
      { label: "PrometheusRuleGroupsNamespace", anchor: "prometheusrulegroupsnamespace" },
      { label: "PrometheusScraper", anchor: "prometheusscraper" },
      { label: "PrometheusWorkspace", anchor: "prometheusworkspace" }
    ]
  },
  {
    id: "memorydb",
    label: "MemoryDB",
    href: "/docs/memorydb.html",
    items: [
      { label: "MemoryDBACL", anchor: "memorydbacl" },
      { label: "MemoryDBCluster", anchor: "memorydbcluster" },
      { label: "MemoryDBMultiRegionCluster", anchor: "memorydbmultiregioncluster" },
      { label: "MemoryDBParameterGroup", anchor: "memorydbparametergroup" },
      { label: "MemoryDBSubnetGroup", anchor: "memorydbsubnetgroup" },
      { label: "MemoryDBUser", anchor: "memorydbuser" }
    ]
  },
  {
    id: "kafka",
    label: "MSK",
    href: "/docs/kafka.html",
    items: [
      { label: "MSKBatchScramSecret", anchor: "mskbatchscramsecret" },
      { label: "MSKChannel", anchor: "mskchannel" },
      { label: "MSKCluster", anchor: "mskcluster" },
      { label: "MSKClusterPolicy", anchor: "mskclusterpolicy" },
      { label: "MSKConfiguration", anchor: "mskconfiguration" },
      { label: "MSKReplicator", anchor: "mskreplicator" },
      { label: "MSKServerlessCluster", anchor: "mskserverlesscluster" },
      { label: "MSKTopic", anchor: "msktopic" },
      { label: "MSKVpcConnection", anchor: "mskvpcconnection" }
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
    id: "mwaa",
    label: "MWAA",
    href: "/docs/mwaa.html",
    items: [
      { label: "MWAAEnvironment", anchor: "mwaaenvironment" }
    ]
  },
  {
    id: "neptune",
    label: "NEPTUNE",
    href: "/docs/neptune.html",
    items: [
      { label: "NeptuneDBCluster", anchor: "neptunedbcluster" },
      { label: "NeptuneDBClusterParameterGroup", anchor: "neptunedbclusterparametergroup" },
      { label: "NeptuneDBInstance", anchor: "neptunedbinstance" },
      { label: "NeptuneDBParameterGroup", anchor: "neptunedbparametergroup" },
      { label: "NeptuneDBSubnetGroup", anchor: "neptunedbsubnetgroup" },
      { label: "NeptuneEventSubscription", anchor: "neptuneeventsubscription" },
      { label: "NeptuneGlobalCluster", anchor: "neptuneglobalcluster" }
    ]
  },
  {
    id: "networkfirewall",
    label: "Network Firewall",
    href: "/docs/networkfirewall.html",
    items: [
      { label: "Firewall", anchor: "firewall" },
      { label: "FirewallPolicy", anchor: "firewallpolicy" },
      { label: "FirewallRuleGroup", anchor: "firewallrulegroup" },
      { label: "NetworkFirewallLoggingConfiguration", anchor: "networkfirewallloggingconfiguration" },
      { label: "NetworkFirewallTLSInspectionConfiguration", anchor: "networkfirewalltlsinspectionconfiguration" },
      { label: "NetworkFirewallVpcEndpointAssociation", anchor: "networkfirewallvpcendpointassociation" }
    ]
  },
  {
    id: "networkflowmonitor",
    label: "NETWORKFLOWMONITOR",
    href: "/docs/networkflowmonitor.html",
    items: [
      { label: "NetworkFlowMonitorMonitor", anchor: "networkflowmonitormonitor" }
    ]
  },
  {
    id: "networkmanager",
    label: "NETWORKMANAGER",
    href: "/docs/networkmanager.html",
    items: [
      { label: "NetworkManagerConnectAttachment", anchor: "networkmanagerconnectattachment" },
      { label: "NetworkManagerConnectPeer", anchor: "networkmanagerconnectpeer" },
      { label: "NetworkManagerCoreNetwork", anchor: "networkmanagercorenetwork" },
      { label: "NetworkManagerCoreNetworkPrefixListAssociation", anchor: "networkmanagercorenetworkprefixlistassociation" },
      { label: "NetworkManagerCustomerGatewayAssociation", anchor: "networkmanagercustomergatewayassociation" },
      { label: "NetworkManagerDevice", anchor: "networkmanagerdevice" },
      { label: "NetworkManagerDirectConnectGatewayAttachment", anchor: "networkmanagerdirectconnectgatewayattachment" },
      { label: "NetworkManagerGlobalNetwork", anchor: "networkmanagerglobalnetwork" },
      { label: "NetworkManagerLink", anchor: "networkmanagerlink" },
      { label: "NetworkManagerLinkAssociation", anchor: "networkmanagerlinkassociation" },
      { label: "NetworkManagerSite", anchor: "networkmanagersite" },
      { label: "NetworkManagerSiteToSiteVpnAttachment", anchor: "networkmanagersitetositevpnattachment" },
      { label: "NetworkManagerTransitGatewayPeering", anchor: "networkmanagertransitgatewaypeering" },
      { label: "NetworkManagerTransitGatewayRegistration", anchor: "networkmanagertransitgatewayregistration" },
      { label: "NetworkManagerTransitGatewayRouteTableAttachment", anchor: "networkmanagertransitgatewayroutetableattachment" },
      { label: "NetworkManagerVpcAttachment", anchor: "networkmanagervpcattachment" }
    ]
  },
  {
    id: "notifications",
    label: "NOTIFICATIONS",
    href: "/docs/notifications.html",
    items: [
      { label: "NotificationsChannelAssociation", anchor: "notificationschannelassociation" },
      { label: "NotificationsEventRule", anchor: "notificationseventrule" },
      { label: "NotificationsManagedNotificationAccountContactAssociation", anchor: "notificationsmanagednotificationaccountcontactassociation" },
      { label: "NotificationsManagedNotificationAdditionalChannelAssociation", anchor: "notificationsmanagednotificationadditionalchannelassociation" },
      { label: "NotificationsNotificationConfiguration", anchor: "notificationsnotificationconfiguration" },
      { label: "NotificationsNotificationHub", anchor: "notificationsnotificationhub" },
      { label: "NotificationsOrganizationalUnitAssociation", anchor: "notificationsorganizationalunitassociation" }
    ]
  },
  {
    id: "notificationscontacts",
    label: "NOTIFICATIONSCONTACTS",
    href: "/docs/notificationscontacts.html",
    items: [
      { label: "NotificationsContactsEmailContact", anchor: "notificationscontactsemailcontact" }
    ]
  },
  {
    id: "oam",
    label: "OAM",
    href: "/docs/oam.html",
    items: [
      { label: "OAMLink", anchor: "oamlink" },
      { label: "OAMSink", anchor: "oamsink" }
    ]
  },
  {
    id: "observabilityadmin",
    label: "OBSERVABILITYADMIN",
    href: "/docs/observabilityadmin.html",
    items: [
      { label: "ObservabilityAdminOrganizationCentralizationRule", anchor: "observabilityadminorganizationcentralizationrule" },
      { label: "ObservabilityAdminOrganizationTelemetryRule", anchor: "observabilityadminorganizationtelemetryrule" },
      { label: "ObservabilityAdminS3TableIntegration", anchor: "observabilityadmins3tableintegration" },
      { label: "ObservabilityAdminTelemetryEnrichment", anchor: "observabilityadmintelemetryenrichment" },
      { label: "ObservabilityAdminTelemetryPipelines", anchor: "observabilityadmintelemetrypipelines" },
      { label: "ObservabilityAdminTelemetryRule", anchor: "observabilityadmintelemetryrule" }
    ]
  },
  {
    id: "opensearch",
    label: "OpenSearch",
    href: "/docs/opensearch.html",
    items: [
      { label: "OpenSearchApplication", anchor: "opensearchapplication" },
      { label: "OpenSearchDataSource", anchor: "opensearchdatasource" }
    ]
  },
  {
    id: "opensearchserverless",
    label: "OpenSearch Serverless",
    href: "/docs/opensearchserverless.html",
    items: [
      { label: "OpenSearchServerlessAccessPolicy", anchor: "opensearchserverlessaccesspolicy" },
      { label: "OpenSearchServerlessCollectionGroup", anchor: "opensearchserverlesscollectiongroup" },
      { label: "OpenSearchServerlessCollectionIndex", anchor: "opensearchserverlesscollectionindex" },
      { label: "OpenSearchServerlessIndex", anchor: "opensearchserverlessindex" },
      { label: "OpenSearchServerlessLifecyclePolicy", anchor: "opensearchserverlesslifecyclepolicy" },
      { label: "OpenSearchServerlessSecurityConfig", anchor: "opensearchserverlesssecurityconfig" },
      { label: "OpenSearchServerlessSecurityPolicy", anchor: "opensearchserverlesssecuritypolicy" },
      { label: "OpenSearchServerlessVpcEndpoint", anchor: "opensearchserverlessvpcendpoint" }
    ]
  },
  {
    id: "organizations",
    label: "Organizations",
    href: "/docs/organizations.html",
    items: [
      { label: "OrganizationsAccount", anchor: "organizationsaccount" },
      { label: "OrganizationsOU", anchor: "organizationsou" },
      { label: "OrganizationsOrganization", anchor: "organizationsorganization" },
      { label: "OrganizationsPolicy", anchor: "organizationspolicy" },
      { label: "OrganizationsPolicyAttachment", anchor: "organizationspolicyattachment" },
      { label: "OrganizationsResourcePolicy", anchor: "organizationsresourcepolicy" }
    ]
  },
  {
    id: "osis",
    label: "OSIS",
    href: "/docs/osis.html",
    items: [
      { label: "OSISPipeline", anchor: "osispipeline" }
    ]
  },
  {
    id: "ram",
    label: "RAM",
    href: "/docs/ram.html",
    items: [
      { label: "RAMPermission", anchor: "rampermission" },
      { label: "ResourceShare", anchor: "resourceshare" },
      { label: "ResourceShareInvitation", anchor: "resourceshareinvitation" }
    ]
  },
  {
    id: "rbin",
    label: "RBIN",
    href: "/docs/rbin.html",
    items: [
      { label: "RbinRule", anchor: "rbinrule" }
    ]
  },
  {
    id: "rds",
    label: "RDS & Aurora",
    href: "/docs/rds.html",
    items: [
      { label: "DBInstance", anchor: "dbinstance" },
      { label: "RDSClusterSnapshot", anchor: "rdsclustersnapshot" },
      { label: "RDSCustomDBEngineVersion", anchor: "rdscustomdbengineversion" },
      { label: "RDSDBProxyEndpoint", anchor: "rdsdbproxyendpoint" },
      { label: "RDSDBProxyTargetGroup", anchor: "rdsdbproxytargetgroup" },
      { label: "RDSDBShardGroup", anchor: "rdsdbshardgroup" },
      { label: "RDSDBSnapshot", anchor: "rdsdbsnapshot" },
      { label: "RDSEventSubscription", anchor: "rdseventsubscription" },
      { label: "RDSGlobalCluster", anchor: "rdsglobalcluster" },
      { label: "RDSIntegration", anchor: "rdsintegration" }
    ]
  },
  {
    id: "redshift",
    label: "Redshift",
    href: "/docs/redshift.html",
    items: [
      { label: "RedshiftCluster", anchor: "redshiftcluster" },
      { label: "RedshiftEndpointAccess", anchor: "redshiftendpointaccess" },
      { label: "RedshiftEndpointAuthorization", anchor: "redshiftendpointauthorization" },
      { label: "RedshiftEventSubscription", anchor: "redshifteventsubscription" },
      { label: "RedshiftIntegration", anchor: "redshiftintegration" },
      { label: "RedshiftParameterGroup", anchor: "redshiftparametergroup" },
      { label: "RedshiftScheduledAction", anchor: "redshiftscheduledaction" },
      { label: "RedshiftSnapshotSchedule", anchor: "redshiftsnapshotschedule" },
      { label: "RedshiftSubnetGroup", anchor: "redshiftsubnetgroup" }
    ]
  },
  {
    id: "redshiftserverless",
    label: "REDSHIFTSERVERLESS",
    href: "/docs/redshiftserverless.html",
    items: [
      { label: "RedshiftServerlessNamespace", anchor: "redshiftserverlessnamespace" },
      { label: "RedshiftServerlessSnapshot", anchor: "redshiftserverlesssnapshot" },
      { label: "RedshiftServerlessWorkgroup", anchor: "redshiftserverlessworkgroup" }
    ]
  },
  {
    id: "resiliencehub",
    label: "RESILIENCEHUB",
    href: "/docs/resiliencehub.html",
    items: [
      { label: "ResilienceHubApp", anchor: "resiliencehubapp" },
      { label: "ResilienceHubResiliencyPolicy", anchor: "resiliencehubresiliencypolicy" }
    ]
  },
  {
    id: "resiliencehubv2",
    label: "RESILIENCEHUBV2",
    href: "/docs/resiliencehubv2.html",
    items: [
      { label: "ResilienceHubV2Policy", anchor: "resiliencehubv2policy" },
      { label: "ResilienceHubV2Service", anchor: "resiliencehubv2service" },
      { label: "ResilienceHubV2ServiceFunction", anchor: "resiliencehubv2servicefunction" },
      { label: "ResilienceHubV2System", anchor: "resiliencehubv2system" },
      { label: "ResilienceHubV2UserJourney", anchor: "resiliencehubv2userjourney" }
    ]
  },
  {
    id: "resourceexplorer2",
    label: "RESOURCEEXPLORER2",
    href: "/docs/resourceexplorer2.html",
    items: [
      { label: "ResourceExplorer2DefaultViewAssociation", anchor: "resourceexplorer2defaultviewassociation" },
      { label: "ResourceExplorer2Index", anchor: "resourceexplorer2index" },
      { label: "ResourceExplorer2View", anchor: "resourceexplorer2view" }
    ]
  },
  {
    id: "resourcegroups",
    label: "RESOURCEGROUPS",
    href: "/docs/resourcegroups.html",
    items: [
      { label: "ResourceGroupsGroup", anchor: "resourcegroupsgroup" },
      { label: "ResourceGroupsTagSyncTask", anchor: "resourcegroupstagsynctask" }
    ]
  },
  {
    id: "rolesanywhere",
    label: "ROLESANYWHERE",
    href: "/docs/rolesanywhere.html",
    items: [
      { label: "RolesAnywhereCRL", anchor: "rolesanywherecrl" },
      { label: "RolesAnywhereProfile", anchor: "rolesanywhereprofile" },
      { label: "RolesAnywhereTrustAnchor", anchor: "rolesanywheretrustanchor" }
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
      { label: "RecordSet", anchor: "recordset" },
      { label: "Route53CidrCollection", anchor: "route53cidrcollection" },
      { label: "Route53DNSSEC", anchor: "route53dnssec" }
    ]
  },
  {
    id: "route53resolver",
    label: "Route 53 Resolver",
    href: "/docs/route53resolver.html",
    items: [
      { label: "Route53ResolverFirewallDomainList", anchor: "route53resolverfirewalldomainlist" },
      { label: "Route53ResolverFirewallRuleGroup", anchor: "route53resolverfirewallrulegroup" },
      { label: "Route53ResolverFirewallRuleGroupAssociation", anchor: "route53resolverfirewallrulegroupassociation" },
      { label: "Route53ResolverOutpostResolver", anchor: "route53resolveroutpostresolver" },
      { label: "Route53ResolverResolverConfig", anchor: "route53resolverresolverconfig" },
      { label: "Route53ResolverResolverDNSSECConfig", anchor: "route53resolverresolverdnssecconfig" },
      { label: "Route53ResolverResolverQueryLoggingConfig", anchor: "route53resolverresolverqueryloggingconfig" },
      { label: "Route53ResolverResolverQueryLoggingConfigAssociation", anchor: "route53resolverresolverqueryloggingconfigassociation" },
      { label: "Route53ResolverResolverRuleAssociation", anchor: "route53resolverresolverruleassociation" }
    ]
  },
  {
    id: "route53profiles",
    label: "ROUTE53PROFILES",
    href: "/docs/route53profiles.html",
    items: [
      { label: "Route53ProfilesProfile", anchor: "route53profilesprofile" },
      { label: "Route53ProfilesProfileAssociation", anchor: "route53profilesprofileassociation" },
      { label: "Route53ProfilesProfileResourceAssociation", anchor: "route53profilesprofileresourceassociation" }
    ]
  },
  {
    id: "route53recoverycontrol",
    label: "ROUTE53RECOVERYCONTROL",
    href: "/docs/route53recoverycontrol.html",
    items: [
      { label: "Route53RecoveryControlCluster", anchor: "route53recoverycontrolcluster" },
      { label: "Route53RecoveryControlControlPanel", anchor: "route53recoverycontrolcontrolpanel" },
      { label: "Route53RecoveryControlRoutingControl", anchor: "route53recoverycontrolroutingcontrol" },
      { label: "Route53RecoveryControlSafetyRule", anchor: "route53recoverycontrolsafetyrule" }
    ]
  },
  {
    id: "route53recoveryreadiness",
    label: "ROUTE53RECOVERYREADINESS",
    href: "/docs/route53recoveryreadiness.html",
    items: [
      { label: "Route53RecoveryReadinessCell", anchor: "route53recoveryreadinesscell" },
      { label: "Route53RecoveryReadinessReadinessCheck", anchor: "route53recoveryreadinessreadinesscheck" },
      { label: "Route53RecoveryReadinessRecoveryGroup", anchor: "route53recoveryreadinessrecoverygroup" },
      { label: "Route53RecoveryReadinessResourceSet", anchor: "route53recoveryreadinessresourceset" }
    ]
  },
  {
    id: "rum",
    label: "RUM",
    href: "/docs/rum.html",
    items: [
      { label: "RUMAppMonitor", anchor: "rumappmonitor" }
    ]
  },
  {
    id: "s3",
    label: "S3",
    href: "/docs/s3.html",
    items: [
      { label: "S3AccessGrant", anchor: "s3accessgrant" },
      { label: "S3AccessGrantsInstance", anchor: "s3accessgrantsinstance" },
      { label: "S3AccessGrantsLocation", anchor: "s3accessgrantslocation" },
      { label: "S3Bucket", anchor: "s3bucket" },
      { label: "S3MultiRegionAccessPoint", anchor: "s3multiregionaccesspoint" },
      { label: "S3MultiRegionAccessPointPolicy", anchor: "s3multiregionaccesspointpolicy" },
      { label: "S3StorageLens", anchor: "s3storagelens" },
      { label: "S3StorageLensGroup", anchor: "s3storagelensgroup" }
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
    id: "s3express",
    label: "S3EXPRESS",
    href: "/docs/s3express.html",
    items: [
      { label: "S3ExpressAccessPoint", anchor: "s3expressaccesspoint" },
      { label: "S3ExpressBucketPolicy", anchor: "s3expressbucketpolicy" },
      { label: "S3ExpressDirectoryBucket", anchor: "s3expressdirectorybucket" }
    ]
  },
  {
    id: "s3objectlambda",
    label: "S3OBJECTLAMBDA",
    href: "/docs/s3objectlambda.html",
    items: [
      { label: "S3ObjectLambdaAccessPoint", anchor: "s3objectlambdaaccesspoint" },
      { label: "S3ObjectLambdaAccessPointPolicy", anchor: "s3objectlambdaaccesspointpolicy" }
    ]
  },
  {
    id: "s3tables",
    label: "S3TABLES",
    href: "/docs/s3tables.html",
    items: [
      { label: "S3TablesNamespace", anchor: "s3tablesnamespace" },
      { label: "S3TablesTable", anchor: "s3tablestable" },
      { label: "S3TablesTableBucket", anchor: "s3tablestablebucket" },
      { label: "S3TablesTableBucketPolicy", anchor: "s3tablestablebucketpolicy" },
      { label: "S3TablesTablePolicy", anchor: "s3tablestablepolicy" }
    ]
  },
  {
    id: "sagemaker",
    label: "SAGEMAKER",
    href: "/docs/sagemaker.html",
    items: [
      { label: "SageMakerAction", anchor: "sagemakeraction" },
      { label: "SageMakerAlgorithm", anchor: "sagemakeralgorithm" },
      { label: "SageMakerApp", anchor: "sagemakerapp" },
      { label: "SageMakerAppImageConfig", anchor: "sagemakerappimageconfig" },
      { label: "SageMakerArtifact", anchor: "sagemakerartifact" },
      { label: "SageMakerCluster", anchor: "sagemakercluster" },
      { label: "SageMakerContext", anchor: "sagemakercontext" },
      { label: "SageMakerDataQualityJobDefinition", anchor: "sagemakerdataqualityjobdefinition" },
      { label: "SageMakerDevice", anchor: "sagemakerdevice" },
      { label: "SageMakerDeviceFleet", anchor: "sagemakerdevicefleet" },
      { label: "SageMakerDomain", anchor: "sagemakerdomain" },
      { label: "SageMakerEndpoint", anchor: "sagemakerendpoint" },
      { label: "SageMakerEndpointConfig", anchor: "sagemakerendpointconfig" },
      { label: "SageMakerExperiment", anchor: "sagemakerexperiment" },
      { label: "SageMakerFeatureGroup", anchor: "sagemakerfeaturegroup" },
      { label: "SageMakerHub", anchor: "sagemakerhub" },
      { label: "SageMakerHumanTaskUi", anchor: "sagemakerhumantaskui" },
      { label: "SageMakerImage", anchor: "sagemakerimage" },
      { label: "SageMakerImageVersion", anchor: "sagemakerimageversion" },
      { label: "SageMakerInferenceComponent", anchor: "sagemakerinferencecomponent" },
      { label: "SageMakerInferenceExperiment", anchor: "sagemakerinferenceexperiment" },
      { label: "SageMakerMlflowApp", anchor: "sagemakermlflowapp" },
      { label: "SageMakerMlflowTrackingServer", anchor: "sagemakermlflowtrackingserver" },
      { label: "SageMakerModel", anchor: "sagemakermodel" },
      { label: "SageMakerModelBiasJobDefinition", anchor: "sagemakermodelbiasjobdefinition" },
      { label: "SageMakerModelCard", anchor: "sagemakermodelcard" },
      { label: "SageMakerModelExplainabilityJobDefinition", anchor: "sagemakermodelexplainabilityjobdefinition" },
      { label: "SageMakerModelPackage", anchor: "sagemakermodelpackage" },
      { label: "SageMakerModelPackageGroup", anchor: "sagemakermodelpackagegroup" },
      { label: "SageMakerModelQualityJobDefinition", anchor: "sagemakermodelqualityjobdefinition" },
      { label: "SageMakerMonitoringSchedule", anchor: "sagemakermonitoringschedule" },
      { label: "SageMakerPartnerApp", anchor: "sagemakerpartnerapp" },
      { label: "SageMakerPipeline", anchor: "sagemakerpipeline" },
      { label: "SageMakerProcessingJob", anchor: "sagemakerprocessingjob" },
      { label: "SageMakerProject", anchor: "sagemakerproject" },
      { label: "SageMakerSpace", anchor: "sagemakerspace" },
      { label: "SageMakerStudioLifecycleConfig", anchor: "sagemakerstudiolifecycleconfig" },
      { label: "SageMakerTrialComponent", anchor: "sagemakertrialcomponent" },
      { label: "SageMakerUserProfile", anchor: "sagemakeruserprofile" },
      { label: "SageMakerWorkforce", anchor: "sagemakerworkforce" }
    ]
  },
  {
    id: "secretsmanager",
    label: "Secrets Manager",
    href: "/docs/secretsmanager.html",
    items: [
      { label: "Secret", anchor: "secret" },
      { label: "SecretsManagerResourcePolicy", anchor: "secretsmanagerresourcepolicy" },
      { label: "SecretsManagerSecretTargetAttachment", anchor: "secretsmanagersecrettargetattachment" }
    ]
  },
  {
    id: "securityhub",
    label: "Security Hub",
    href: "/docs/securityhub.html",
    items: [
      { label: "SecurityHubAccount", anchor: "securityhubaccount" },
      { label: "SecurityHubAggregatorV2", anchor: "securityhubaggregatorv2" },
      { label: "SecurityHubAutomationRule", anchor: "securityhubautomationrule" },
      { label: "SecurityHubAutomationRuleV2", anchor: "securityhubautomationrulev2" },
      { label: "SecurityHubConfigurationPolicy", anchor: "securityhubconfigurationpolicy" },
      { label: "SecurityHubConnector", anchor: "securityhubconnector" },
      { label: "SecurityHubConnectorV2", anchor: "securityhubconnectorv2" },
      { label: "SecurityHubDelegatedAdmin", anchor: "securityhubdelegatedadmin" },
      { label: "SecurityHubFindingAggregator", anchor: "securityhubfindingaggregator" },
      { label: "SecurityHubHubV2", anchor: "securityhubhubv2" },
      { label: "SecurityHubInsight", anchor: "securityhubinsight" },
      { label: "SecurityHubOrganizationConfiguration", anchor: "securityhuborganizationconfiguration" },
      { label: "SecurityHubPolicyAssociation", anchor: "securityhubpolicyassociation" },
      { label: "SecurityHubProductSubscription", anchor: "securityhubproductsubscription" },
      { label: "SecurityHubSecurityControl", anchor: "securityhubsecuritycontrol" },
      { label: "SecurityHubStandard", anchor: "securityhubstandard" }
    ]
  },
  {
    id: "securitylake",
    label: "SECURITYLAKE",
    href: "/docs/securitylake.html",
    items: [
      { label: "SecurityLakeAwsLogSource", anchor: "securitylakeawslogsource" },
      { label: "SecurityLakeDataLake", anchor: "securitylakedatalake" },
      { label: "SecurityLakeSubscriber", anchor: "securitylakesubscriber" },
      { label: "SecurityLakeSubscriberNotification", anchor: "securitylakesubscribernotification" }
    ]
  },
  {
    id: "servicecatalog",
    label: "Service Catalog",
    href: "/docs/servicecatalog.html",
    items: [
      { label: "SCPortfolio", anchor: "scportfolio" },
      { label: "SCPortfolioProductAssociation", anchor: "scportfolioproductassociation" },
      { label: "SCProduct", anchor: "scproduct" },
      { label: "ServiceCatalogAcceptedPortfolioShare", anchor: "servicecatalogacceptedportfolioshare" },
      { label: "ServiceCatalogCloudFormationProvisionedProduct", anchor: "servicecatalogcloudformationprovisionedproduct" },
      { label: "ServiceCatalogLaunchNotificationConstraint", anchor: "servicecataloglaunchnotificationconstraint" },
      { label: "ServiceCatalogLaunchRoleConstraint", anchor: "servicecataloglaunchroleconstraint" },
      { label: "ServiceCatalogLaunchTemplateConstraint", anchor: "servicecataloglaunchtemplateconstraint" },
      { label: "ServiceCatalogPortfolioPrincipalAssociation", anchor: "servicecatalogportfolioprincipalassociation" },
      { label: "ServiceCatalogPortfolioShare", anchor: "servicecatalogportfolioshare" },
      { label: "ServiceCatalogResourceUpdateConstraint", anchor: "servicecatalogresourceupdateconstraint" },
      { label: "ServiceCatalogServiceAction", anchor: "servicecatalogserviceaction" },
      { label: "ServiceCatalogServiceActionAssociation", anchor: "servicecatalogserviceactionassociation" },
      { label: "ServiceCatalogStackSetConstraint", anchor: "servicecatalogstacksetconstraint" },
      { label: "ServiceCatalogTagOption", anchor: "servicecatalogtagoption" },
      { label: "ServiceCatalogTagOptionAssociation", anchor: "servicecatalogtagoptionassociation" }
    ]
  },
  {
    id: "servicecatalogappregistry",
    label: "SERVICECATALOGAPPREGISTRY",
    href: "/docs/servicecatalogappregistry.html",
    items: [
      { label: "ServiceCatalogAppRegistryApplication", anchor: "servicecatalogappregistryapplication" },
      { label: "ServiceCatalogAppRegistryAttributeGroup", anchor: "servicecatalogappregistryattributegroup" },
      { label: "ServiceCatalogAppRegistryAttributeGroupAssociation", anchor: "servicecatalogappregistryattributegroupassociation" },
      { label: "ServiceCatalogAppRegistryResourceAssociation", anchor: "servicecatalogappregistryresourceassociation" }
    ]
  },
  {
    id: "ses",
    label: "SES",
    href: "/docs/ses.html",
    items: [
      { label: "SESConfigurationSetEventDestination", anchor: "sesconfigurationseteventdestination" },
      { label: "SESContactList", anchor: "sescontactlist" },
      { label: "SESCustomVerificationEmailTemplate", anchor: "sescustomverificationemailtemplate" },
      { label: "SESDedicatedIpPool", anchor: "sesdedicatedippool" },
      { label: "SESMailManagerAddonInstance", anchor: "sesmailmanageraddoninstance" },
      { label: "SESMailManagerAddonSubscription", anchor: "sesmailmanageraddonsubscription" },
      { label: "SESMailManagerAddressList", anchor: "sesmailmanageraddresslist" },
      { label: "SESMailManagerArchive", anchor: "sesmailmanagerarchive" },
      { label: "SESMailManagerIngressPoint", anchor: "sesmailmanageringresspoint" },
      { label: "SESMailManagerRelay", anchor: "sesmailmanagerrelay" },
      { label: "SESMailManagerRuleSet", anchor: "sesmailmanagerruleset" },
      { label: "SESMailManagerTrafficPolicy", anchor: "sesmailmanagertrafficpolicy" },
      { label: "SESMultiRegionEndpoint", anchor: "sesmultiregionendpoint" },
      { label: "SESReceiptFilter", anchor: "sesreceiptfilter" },
      { label: "SESReceiptRule", anchor: "sesreceiptrule" },
      { label: "SESReceiptRuleSet", anchor: "sesreceiptruleset" },
      { label: "SESTemplate", anchor: "sestemplate" },
      { label: "SESTenant", anchor: "sestenant" },
      { label: "SESVdmAttributes", anchor: "sesvdmattributes" }
    ]
  },
  {
    id: "shield",
    label: "Shield",
    href: "/docs/shield.html",
    items: [
      { label: "ShieldDRTAccess", anchor: "shielddrtaccess" },
      { label: "ShieldProactiveEngagement", anchor: "shieldproactiveengagement" },
      { label: "ShieldProtectionGroup", anchor: "shieldprotectiongroup" }
    ]
  },
  {
    id: "signer",
    label: "SIGNER",
    href: "/docs/signer.html",
    items: [
      { label: "SignerProfilePermission", anchor: "signerprofilepermission" },
      { label: "SignerSigningProfile", anchor: "signersigningprofile" }
    ]
  },
  {
    id: "sns",
    label: "SNS",
    href: "/docs/sns.html",
    items: [
      { label: "SNSTopic", anchor: "snstopic" },
      { label: "SNSTopicInlinePolicy", anchor: "snstopicinlinepolicy" }
    ]
  },
  {
    id: "sqs",
    label: "SQS",
    href: "/docs/sqs.html",
    items: [
      { label: "SQSQueue", anchor: "sqsqueue" },
      { label: "SQSQueueInlinePolicy", anchor: "sqsqueueinlinepolicy" }
    ]
  },
  {
    id: "ssm",
    label: "SSM",
    href: "/docs/ssm.html",
    items: [
      { label: "SSMAssociation", anchor: "ssmassociation" },
      { label: "SSMCloudConnector", anchor: "ssmcloudconnector" },
      { label: "SSMMaintenanceWindow", anchor: "ssmmaintenancewindow" },
      { label: "SSMMaintenanceWindowTarget", anchor: "ssmmaintenancewindowtarget" },
      { label: "SSMMaintenanceWindowTask", anchor: "ssmmaintenancewindowtask" },
      { label: "SSMOpsItem", anchor: "ssmopsitem" },
      { label: "SSMParameter", anchor: "ssmparameter" },
      { label: "SSMPatchBaseline", anchor: "ssmpatchbaseline" },
      { label: "SSMResourceDataSync", anchor: "ssmresourcedatasync" },
      { label: "SSMResourcePolicy", anchor: "ssmresourcepolicy" },
      { label: "SSMServiceSetting", anchor: "ssmservicesetting" }
    ]
  },
  {
    id: "ssmcontacts",
    label: "SSMCONTACTS",
    href: "/docs/ssmcontacts.html",
    items: [
      { label: "SSMContactsContact", anchor: "ssmcontactscontact" },
      { label: "SSMContactsContactChannel", anchor: "ssmcontactscontactchannel" },
      { label: "SSMContactsPlan", anchor: "ssmcontactsplan" },
      { label: "SSMContactsRotation", anchor: "ssmcontactsrotation" }
    ]
  },
  {
    id: "ssmincidents",
    label: "SSMINCIDENTS",
    href: "/docs/ssmincidents.html",
    items: [
      { label: "SSMIncidentsReplicationSet", anchor: "ssmincidentsreplicationset" },
      { label: "SSMIncidentsResponsePlan", anchor: "ssmincidentsresponseplan" }
    ]
  },
  {
    id: "ssmquicksetup",
    label: "SSMQUICKSETUP",
    href: "/docs/ssmquicksetup.html",
    items: [
      { label: "SSMQuickSetupConfigurationManager", anchor: "ssmquicksetupconfigurationmanager" },
      { label: "SSMQuickSetupLifecycleAutomation", anchor: "ssmquicksetuplifecycleautomation" }
    ]
  },
  {
    id: "sfn",
    label: "Step Functions",
    href: "/docs/sfn.html",
    items: [
      { label: "SFNStateMachineAlias", anchor: "sfnstatemachinealias" },
      { label: "SFNStateMachineVersion", anchor: "sfnstatemachineversion" }
    ]
  },
  {
    id: "storagegateway",
    label: "STORAGEGATEWAY",
    href: "/docs/storagegateway.html",
    items: [
      { label: "StorageGatewayTapePool", anchor: "storagegatewaytapepool" }
    ]
  },
  {
    id: "synthetics",
    label: "SYNTHETICS",
    href: "/docs/synthetics.html",
    items: [
      { label: "SyntheticsCanary", anchor: "syntheticscanary" },
      { label: "SyntheticsGroup", anchor: "syntheticsgroup" }
    ]
  },
  {
    id: "timestream",
    label: "TIMESTREAM",
    href: "/docs/timestream.html",
    items: [
      { label: "TimestreamDatabase", anchor: "timestreamdatabase" },
      { label: "TimestreamInfluxDBCluster", anchor: "timestreaminfluxdbcluster" },
      { label: "TimestreamInfluxDBInstance", anchor: "timestreaminfluxdbinstance" },
      { label: "TimestreamScheduledQuery", anchor: "timestreamscheduledquery" },
      { label: "TimestreamTable", anchor: "timestreamtable" }
    ]
  },
  {
    id: "transfer",
    label: "TRANSFER",
    href: "/docs/transfer.html",
    items: [
      { label: "TransferAgreement", anchor: "transferagreement" },
      { label: "TransferCertificate", anchor: "transfercertificate" },
      { label: "TransferConnector", anchor: "transferconnector" },
      { label: "TransferHostKey", anchor: "transferhostkey" },
      { label: "TransferProfile", anchor: "transferprofile" },
      { label: "TransferServer", anchor: "transferserver" },
      { label: "TransferUser", anchor: "transferuser" },
      { label: "TransferWebApp", anchor: "transferwebapp" },
      { label: "TransferWorkflow", anchor: "transferworkflow" }
    ]
  },
  {
    id: "verifiedpermissions",
    label: "VERIFIEDPERMISSIONS",
    href: "/docs/verifiedpermissions.html",
    items: [
      { label: "VerifiedPermissionsIdentitySource", anchor: "verifiedpermissionsidentitysource" },
      { label: "VerifiedPermissionsPolicy", anchor: "verifiedpermissionspolicy" },
      { label: "VerifiedPermissionsPolicyStore", anchor: "verifiedpermissionspolicystore" },
      { label: "VerifiedPermissionsPolicyStoreAlias", anchor: "verifiedpermissionspolicystorealias" },
      { label: "VerifiedPermissionsPolicyTemplate", anchor: "verifiedpermissionspolicytemplate" }
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
      { label: "LatticeTargetGroup", anchor: "latticetargetgroup" },
      { label: "VpcLatticeAccessLogSubscription", anchor: "vpclatticeaccesslogsubscription" },
      { label: "VpcLatticeAuthPolicy", anchor: "vpclatticeauthpolicy" },
      { label: "VpcLatticeDomainVerification", anchor: "vpclatticedomainverification" },
      { label: "VpcLatticeResourceConfiguration", anchor: "vpclatticeresourceconfiguration" },
      { label: "VpcLatticeResourceGateway", anchor: "vpclatticeresourcegateway" },
      { label: "VpcLatticeResourcePolicy", anchor: "vpclatticeresourcepolicy" },
      { label: "VpcLatticeRule", anchor: "vpclatticerule" },
      { label: "VpcLatticeServiceNetworkResourceAssociation", anchor: "vpclatticeservicenetworkresourceassociation" }
    ]
  },
  {
    id: "wafv2",
    label: "WAFv2",
    href: "/docs/wafv2.html",
    items: [
      { label: "WAFv2LoggingConfiguration", anchor: "wafv2loggingconfiguration" },
      { label: "WAFv2WebACLAssociation", anchor: "wafv2webaclassociation" }
    ]
  },
  {
    id: "xray",
    label: "X-Ray",
    href: "/docs/xray.html",
    items: [
      { label: "XRayGroup", anchor: "xraygroup" },
      { label: "XRayResourcePolicy", anchor: "xrayresourcepolicy" },
      { label: "XRaySamplingRule", anchor: "xraysamplingrule" },
      { label: "XRayTransactionSearchConfig", anchor: "xraytransactionsearchconfig" }
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
