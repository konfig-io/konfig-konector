#!/usr/bin/env python3
"""Generate docs/terraform-parity.md and hack/terraform-parity-gap.txt by
diffing api/v1alpha1 kinds against the Terraform AWS provider resource docs.

Run: make parity   (clones hashicorp/terraform-provider-aws docs shallowly into /tmp)
"""
import collections, datetime, glob, os, re, subprocess, sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
TF = os.environ.get("TF_AWS_CHECKOUT", "/tmp/terraform-provider-aws-docs")

# Terraform resource name (without aws_) -> konfig kind (lowercase) where the
# flattened names differ.
ALIASES = {l.split()[0]: l.split()[1] for l in """
route53_zone hostedzone
route53_record recordset
route53_health_check healthcheck
route53_key_signing_key delegationsignerrecord
route53_resolver_endpoint resolverendpoint
route53_resolver_rule resolverrule
route53_zone_association hostedzonevpcassociation
instance ec2instance
eip elasticip
egress_only_internet_gateway egressonlyigw
ec2_transit_gateway transitgateway
ec2_transit_gateway_vpc_attachment transitgatewayvpcattachment
ec2_transit_gateway_vpc_attachment_accepter transitgatewayvpcattachment
vpc_peering_connection_accepter vpcpeeringconnection
ec2_managed_prefix_list managedprefixlist
ec2_capacity_reservation capacityreservation
spot_fleet_request spotfleet
vpc_endpoint_service vpcendpointservice
vpc_endpoint_service_allowed_principal vpcendpointservice
vpc_endpoint_connection_accepter vpcendpointservice
cloudwatch_log_group loggroup
cloudwatch_metric_alarm cloudwatchalarm
cloudwatch_composite_alarm compositealarm
cloudwatch_log_metric_filter metricfilter
cloudwatch_log_subscription_filter subscriptionfilter
cloudwatch_event_bus eventbus
cloudwatch_event_rule eventrule
cloudwatch_event_target eventtarget
rds_cluster dbcluster
rds_cluster_parameter_group dbclusterparametergroup
db_event_subscription rdseventsubscription
lb loadbalancer
lb_listener listener
lb_listener_rule listenerrule
lb_target_group targetgroup
acm_certificate certificate
acm_certificate_validation certificatevalidation
acmpca_certificate_authority privateca
kinesis_firehose_delivery_stream firehosedeliverystream
sfn_state_machine statemachine
sfn_activity activity
secretsmanager_secret secret
secretsmanager_secret_rotation secretrotation
cognito_user_pool userpool
cognito_user_pool_client userpoolclient
cognito_identity_provider identityprovider
wafv2_web_acl webacl
wafv2_ip_set ipset
networkfirewall_firewall firewall
networkfirewall_firewall_policy firewallpolicy
networkfirewall_rule_group firewallrulegroup
vpclattice_listener latticelistener
vpclattice_service latticeservice
vpclattice_service_network latticeservicenetwork
vpclattice_service_network_service_association latticeservicenetworkserviceassociation
vpclattice_service_network_vpc_association latticeservicenetworkvpcassociation
vpclattice_target_group latticetargetgroup
service_discovery_http_namespace cloudmapnamespace
service_discovery_service cloudmapservice
ssoadmin_permission_set permissionset
ssoadmin_account_assignment ssoassignment
ram_resource_share resourceshare
ram_resource_share_accepter resourceshareinvitation
api_gateway_rest_api restapi
api_gateway_deployment restapideployment
api_gateway_stage restapistage
s3_bucket_cors_configuration s3bucketcors
s3_bucket_lifecycle_configuration s3bucketlifecycle
s3_bucket_replication_configuration s3bucketreplication
appautoscaling_target scalabletarget
appautoscaling_policy appscalingpolicy
autoscaling_policy scalingpolicy
scheduler_schedule schedule
scheduler_schedule_group schedulegroup
servicecatalog_portfolio scportfolio
servicecatalog_product scproduct
servicecatalog_product_portfolio_association scportfolioproductassociation
config_configuration_recorder configrecorder
config_config_rule configrule
cloudtrail trail
budgets_budget budget
ce_anomaly_monitor costanomalymonitor
ce_anomaly_subscription costanomalysubscription
controltower_control ctenabledcontrol
eks_pod_identity_association podidentityassociation
iam_openid_connect_provider iamoidcprovider
glue_catalog_database gluedatabase
organizations_organizational_unit organizationsou
prometheus_rule_group_namespace prometheusrulegroupsnamespace
opensearch_domain_policy opensearchaccesspolicy
sns_topic_subscription snssubscription
securityhub_standards_subscription securityhubstandard
lambda_function_event_invoke_config lambdaeventinvokeconfig
lambda_provisioned_concurrency_config lambdaprovisionedconcurrency
dynamodb_resource_policy dynamodbtablepolicy
codedeploy_app codedeployapplication
apprunner_auto_scaling_configuration_version apprunnerautoscaling
pipes_pipe eventbridgepipe
inspector2_enabler inspectorenabler
cloudcontrolapi_resource cloudcontrolresource
""".strip().splitlines()}

# Services an EKS platform team touches. Everything else is out of scope.
SCOPE = set("""iam ec2 vpc subnet route internet nat eip egress flow default network networkfirewall networkmanager dx vpn customer volume ebs snapshot ami key launch placement spot instance autoscaling autoscalingplans appautoscaling
eks ecs ecr ecrpublic lambda batch apprunner imagebuilder
lb elb globalaccelerator vpclattice service appmesh api apigatewayv2 cloudfront cloudfrontkeyvaluestore route53 route53profiles route53recoverycontrolconfig route53recoveryreadiness arczonalshift acm acmpca
s3 s3control s3tables efs fsx backup dlm rbin glacier datasync transfer storagegateway
rds db docdb neptune dynamodb dax elasticache memorydb redshift redshiftserverless keyspaces timestreamwrite timestreaminfluxdb opensearch opensearchserverless elasticsearch osis
sqs sns cloudwatch pipes scheduler schemas mq msk mskconnect kinesis kinesisanalyticsv2 sfn appsync appconfig appflow
xray oam prometheus grafana synthetics rum applicationinsights evidently internetmonitor networkmonitor networkflowmonitor observabilityadmin notifications notificationscontacts chatbot
kms secretsmanager ssm ssmcontacts ssmincidents ssmquicksetup cognito identitystore ssoadmin rolesanywhere accessanalyzer guardduty securityhub inspector2 inspector macie2 detective securitylake fms shield wafv2 waf verifiedaccess verifiedpermissions cloudhsm signer cloudtrail config auditmanager
codebuild codecommit codedeploy codepipeline codeartifact codeconnections codestarconnections codestarnotifications codeguruprofiler codegurureviewer codecatalyst amplify cloud9 cloudformation cloudcontrolapi servicecatalog servicecatalogappregistry
organizations account accountaccess controltower ram resourcegroups resourceexplorer2 servicequotas licensemanager budgets ce cur bcmdataexports costoptimizationhub computeoptimizer savingsplans billing invoicing fis resiliencehub resiliencehubv2 drs
glue athena lakeformation emr emrcontainers emrserverless mwaa datazone dms sesv2 ses
bedrock bedrockagent bedrockagentcore sagemaker""".split())

TIER1 = set("""iam ec2 vpc subnet route internet nat eip egress flow default network eks ecs ecr lambda lb elb autoscaling appautoscaling rds db dynamodb elasticache s3 s3control efs sqs sns cloudwatch kms secretsmanager ssm cognito ssoadmin identitystore route53 acm cloudfront wafv2 guardduty securityhub inspector2 config cloudtrail organizations account controltower ram servicequotas budgets ce oam xray prometheus grafana kinesis msk mq sfn scheduler pipes schemas api apigatewayv2 vpclattice service globalaccelerator dx vpn customer ebs volume snapshot ami key launch placement spot instance imagebuilder batch apprunner backup dlm rbin codebuild codepipeline codedeploy codeartifact codeconnections codestarconnections codestarnotifications cloudformation cloudcontrolapi servicecatalog appconfig appsync networkfirewall shield accessanalyzer verifiedaccess rolesanywhere cloudfrontkeyvaluestore route53profiles arczonalshift opensearch opensearchserverless memorydb redshift redshiftserverless glue athena emr emrcontainers mwaa sesv2 ses ecrpublic resourcegroups resourceexplorer2 licensemanager notifications notificationscontacts chatbot synthetics rum applicationinsights internetmonitor networkmonitor networkflowmonitor observabilityadmin fis""".split())


def ensure_checkout():
    if os.path.isdir(os.path.join(TF, "website/docs/r")):
        subprocess.run(["git", "-C", TF, "pull", "-q", "--depth", "1"], check=False)
        return
    subprocess.run(["git", "clone", "-q", "--depth", "1", "--filter=blob:none", "--sparse",
                    "https://github.com/hashicorp/terraform-provider-aws", TF], check=True)
    subprocess.run(["git", "-C", TF, "sparse-checkout", "set", "website/docs/r"], check=True)


def main():
    ensure_checkout()
    tf = sorted(os.path.basename(p)[:-len(".html.markdown")] for p in glob.glob(os.path.join(TF, "website/docs/r/*.html.markdown")))
    kinds = set()
    for f in glob.glob(os.path.join(ROOT, "api/v1alpha1/*_types.go")):
        for k in re.findall(r"SchemeBuilder\.Register\(&(\w+)\{\}", open(f).read()):
            kinds.add(k.lower())
    kinds.discard("awsprovider")
    covered = {}
    for r in tf:
        flat = r.replace("_", "")
        if flat in kinds:
            covered[r] = flat
        elif ALIASES.get(r) in kinds:
            covered[r] = ALIASES[r]
    prefix = lambda r: r.split("_")[0]
    inscope = [r for r in tf if prefix(r) in SCOPE]
    out = [r for r in tf if prefix(r) not in SCOPE]
    gap = [r for r in inscope if r not in covered]
    g1 = [r for r in gap if prefix(r) in TIER1]
    g2 = [r for r in gap if prefix(r) not in TIER1]
    sha = subprocess.run(["git", "-C", TF, "rev-parse", "--short", "HEAD"], capture_output=True, text=True).stdout.strip()
    today = datetime.date.today().isoformat()

    def table(counter):
        return "".join(f"| {p} | {c} |\n" for p, c in counter.most_common())

    doc = [f"# Terraform AWS provider parity\n\nGenerated by `make parity` on {today} from hashicorp/terraform-provider-aws@{sha} (`website/docs/r/*`) against the kinds in `api/v1alpha1`. Do not hand-edit.\n\n",
           "| Metric | Count |\n|---|---|\n",
           f"| Terraform resources | {len(tf)} |\n| konfig kinds (excluding AWSProvider) | {len(kinds)} |\n| Terraform resources with a konfig equivalent | {len(covered)} |\n| Gap (all of Terraform) | {len(tf) - len(covered)} |\n\n",
           "## EKS-platform scope\n\nThe parity target is every service an EKS platform team touches. Media, gaming, end-user computing, telephony, and similar services are out of scope. Any in-scope resource without a native kind can be managed today through `CloudControlResource` when its CloudFormation type is registered.\n\n",
           "| Metric | Count |\n|---|---|\n",
           f"| In-scope Terraform resources | {len(inscope)} |\n| Covered | {len(inscope) - len(gap)} |\n| Gap, tier 1 (core platform) | {len(g1)} |\n| Gap, tier 2 (data, ML, governance extras) | {len(g2)} |\n| Out of scope | {len(out)} |\n\n",
           "### Tier 1 gap by prefix\n\n| Prefix | Missing |\n|---|---|\n", table(collections.Counter(prefix(r) for r in g1)),
           "\n### Tier 2 gap by prefix\n\n| Prefix | Missing |\n|---|---|\n", table(collections.Counter(prefix(r) for r in g2)),
           "\n### Out-of-scope prefixes\n\n", ", ".join(sorted(set(prefix(r) for r in out))), "\n\n## Missing resources (in scope)\n"]
    cur = None
    for r in gap:
        if prefix(r) != cur:
            cur = prefix(r)
            doc.append(f"\n### {cur}\n\n")
        doc.append(f"- `aws_{r}` ({'T1' if r in g1 else 'T2'})\n")
    doc.append("\n## Covered resources\n\n| Terraform | konfig kind |\n|---|---|\n")
    doc += [f"| `aws_{r}` | `{covered[r]}` |\n" for r in tf if r in covered]
    open(os.path.join(ROOT, "docs/terraform-parity.md"), "w").write("".join(doc))
    open(os.path.join(ROOT, "hack/terraform-parity-gap.txt"), "w").write("".join(("T1 " if r in g1 else "T2 ") + "aws_" + r + "\n" for r in gap))
    print(f"terraform={len(tf)} kinds={len(kinds)} covered={len(covered)} in-scope gap: T1={len(g1)} T2={len(g2)}")


if __name__ == "__main__":
    main()
