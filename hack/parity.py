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


# Terraform resource prefix -> generated-kind service prefix (kind names are
# <Service><Type>, e.g. aws_cloudwatch_log_stream -> LogsLogStream).
TF_PREFIX_TO_SERVICE = {
    "cloudwatch_log": "logs", "cloudwatch_event": "eventbridge", "cloudwatch": "cloudwatch", "lb": "elbv2",
    "alb": "elbv2", "elb": "elb", "db": "rds", "rds": "rds", "api_gateway": "apigateway", "apigatewayv2": "apigatewayv2",
    "ec2": "ec2", "vpc": "ec2", "subnet": "ec2", "route": "ec2", "internet_gateway": "ec2", "nat_gateway": "ec2",
    "network": "ec2", "security_group": "ec2", "eip": "ec2", "ebs": "ec2", "volume": "ec2", "ami": "ec2", "key_pair": "ec2",
    "launch_template": "ec2", "placement_group": "ec2", "spot": "ec2", "flow_log": "ec2", "customer_gateway": "ec2",
    "vpn": "ec2", "default": "ec2", "egress_only_internet_gateway": "ec2", "instance": "ec2", "snapshot": "ec2",
    "iam": "iam", "s3": "s3", "s3control": "s3", "sqs": "sqs", "sns": "sns", "kms": "kms", "lambda": "lambda",
    "ecs": "ecs", "ecr": "ecr", "eks": "eks", "efs": "efs", "elasticache": "elasticache", "dynamodb": "dynamodb",
    "route53": "route53", "route53_resolver": "route53resolver", "acm": "acm", "acmpca": "acmpca",
    "cloudfront": "cloudfront", "cloudtrail": "cloudtrail", "config": "config", "guardduty": "guardduty",
    "securityhub": "securityhub", "inspector2": "inspectorv2", "ssm": "ssm", "ssoadmin": "ssoadmin",
    "identitystore": "identitystore", "secretsmanager": "secretsmanager", "kinesis": "kinesis",
    "kinesis_firehose": "firehose", "msk": "msk", "mq": "mq", "sfn": "sfn", "scheduler": "scheduler",
    "pipes": "pipes", "schemas": "eventschemas", "glue": "glue", "athena": "athena", "redshift": "redshift",
    "redshiftserverless": "redshiftserverless", "opensearch": "opensearchservice", "opensearchserverless": "opensearchserverless",
    "memorydb": "memorydb", "neptune": "neptune", "docdb": "docdb", "backup": "backup", "batch": "batch",
    "apprunner": "apprunner", "imagebuilder": "imagebuilder", "codebuild": "codebuild", "codepipeline": "codepipeline",
    "codedeploy": "codedeploy", "codeartifact": "codeartifact", "codecommit": "codecommit",
    "codeconnections": "codeconnections", "codestarconnections": "codestarconnections",
    "codestarnotifications": "codestarnotifications", "cloudformation": "cloudformation", "servicecatalog": "servicecatalog",
    "appconfig": "appconfig", "appsync": "appsync", "networkfirewall": "networkfirewall", "shield": "shield",
    "wafv2": "wafv2", "organizations": "organizations", "ram": "ram", "controltower": "controltower",
    "servicequotas": "servicequotas", "budgets": "budgets", "ce": "ce", "oam": "oam", "xray": "xray",
    "prometheus": "prometheus", "grafana": "grafana", "vpclattice": "vpclattice", "service_discovery": "cloudmap",
    "globalaccelerator": "globalaccelerator", "dx": "directconnect", "autoscaling": "autoscaling",
    "appautoscaling": "appautoscaling", "cognito": "cognito", "transfer": "transfer", "datasync": "datasync",
    "fsx": "fsx", "storagegateway": "storagegateway", "emr": "emr", "emrcontainers": "emrcontainers",
    "emrserverless": "emrserverless", "mwaa": "mwaa", "lakeformation": "lakeformation", "sesv2": "ses", "ses": "ses",
    "synthetics": "synthetics", "rum": "rum", "evidently": "evidently", "detective": "detective", "macie2": "macie",
    "securitylake": "securitylake", "fms": "fms", "accessanalyzer": "accessanalyzer", "rolesanywhere": "rolesanywhere",
    "verifiedpermissions": "verifiedpermissions", "licensemanager": "licensemanager", "resourcegroups": "resourcegroups",
    "resourceexplorer2": "resourceexplorer2", "notifications": "notifications", "chatbot": "chatbot", "fis": "fis",
    "dms": "dms", "datazone": "datazone", "bedrock": "bedrock", "bedrockagent": "bedrock", "sagemaker": "sagemaker",
    "timestreamwrite": "timestream", "keyspaces": "cassandra", "kafkaconnect": "kafkaconnect", "mskconnect": "kafkaconnect",
    "amplify": "amplify", "appflow": "appflow", "cloudhsm": "cloudhsm", "signer": "signer", "auditmanager": "auditmanager",
    "internetmonitor": "internetmonitor", "networkmanager": "networkmanager", "route53profiles": "route53profiles",
    "route53recoverycontrolconfig": "route53recoverycontrol", "route53recoveryreadiness": "route53recoveryreadiness",
    "arczonalshift": "arczonalshift", "ssmcontacts": "ssmcontacts", "ssmincidents": "ssmincidents",
    "ssmquicksetup": "ssmquicksetup", "cur": "cur", "bcmdataexports": "bcmdataexports", "computeoptimizer": "computeoptimizer",
    "applicationinsights": "applicationinsights", "observabilityadmin": "observabilityadmin", "rbin": "rbin",
    "dlm": "dlm", "elasticbeanstalk": "elasticbeanstalk", "cloudcontrolapi": "cloudcontrol",
}


# Terraform resources that konfig models as a property of the parent kind
# (native spec field or CloudFormation property) rather than a separate kind.
# They are reported separately, not as gaps.
MODELLED_IN_PARENT = set("""
vpc_endpoint_subnet_association vpc_endpoint_route_table_association vpc_endpoint_security_group_association
vpc_endpoint_policy vpc_endpoint_private_dns vpc_ipv4_cidr_block_association vpc_ipv6_cidr_block_association
vpc_dhcp_options_association vpc_peering_connection_options vpc_peering_connection_accepter
ec2_managed_prefix_list_entry ec2_transit_gateway_vpc_attachment_accepter ec2_transit_gateway_peering_attachment_accepter
ec2_transit_gateway_default_route_table_association ec2_transit_gateway_default_route_table_propagation ec2_tag
ec2_instance_state route_table_association route main_route_table_association network_interface_attachment
network_interface_sg_attachment network_acl_association network_acl_rule security_group_rule
vpc_security_group_ingress_rule vpc_security_group_egress_rule volume_attachment eip_association
internet_gateway_attachment iam_role_policy_attachment iam_policy_attachment iam_user_policy_attachment
iam_group_policy_attachment iam_group_membership iam_user_group_membership iam_role_policies_exclusive
iam_role_policy_attachments_exclusive iam_user_policies_exclusive iam_user_policy_attachments_exclusive
iam_group_policies_exclusive iam_group_policy_attachments_exclusive iam_user_login_profile
s3_bucket_versioning s3_bucket_server_side_encryption_configuration s3_bucket_public_access_block
s3_bucket_ownership_controls s3_bucket_acl s3_bucket_logging s3_bucket_website_configuration
s3_bucket_accelerate_configuration s3_bucket_request_payment_configuration s3_bucket_object_lock_configuration
s3_bucket_intelligent_tiering_configuration s3_bucket_analytics_configuration s3_bucket_inventory
s3_bucket_metric s3_bucket_notification s3_bucket_cors_configuration s3_bucket_lifecycle_configuration
s3_bucket_replication_configuration s3_bucket_policy s3_object s3_object_copy s3_bucket_object
lb_listener_certificate lb_target_group_attachment lb_trust_store_revocation autoscaling_attachment
autoscaling_group_tag autoscaling_traffic_source_attachment autoscaling_schedule autoscaling_lifecycle_hook
autoscaling_notification ecs_cluster_capacity_providers ecs_tag ecs_account_setting_default eks_addon
lambda_function_event_invoke_config lambda_layer_version_permission lambda_provisioned_concurrency_config
lambda_permission lambda_alias lambda_function_url lambda_runtime_management_config lambda_invocation
sqs_queue_policy sqs_queue_redrive_policy sqs_queue_redrive_allow_policy sns_topic_policy sns_topic_data_protection_policy
sns_topic_subscription kms_key_policy kms_grant kms_alias secretsmanager_secret_policy secretsmanager_secret_version
ssm_parameter route53_record route53_zone_association route53_vpc_association_authorization
route53_hosted_zone_dnssec route53_key_signing_key route53_query_log route53_traffic_policy_instance
cloudwatch_log_group_policy cloudwatch_log_resource_policy cloudwatch_log_retention_policy
cloudwatch_log_data_protection_policy cloudwatch_log_index_policy cloudwatch_log_delivery_source
cloudwatch_log_delivery_destination_policy dynamodb_table_item dynamodb_tag dynamodb_kinesis_streaming_destination
dynamodb_contributor_insights dynamodb_table_replica dynamodb_table_export rds_cluster_role_association
db_instance_role_association db_instance_automated_backups_replication rds_cluster_endpoint
db_cluster_snapshot db_snapshot_copy rds_export_task rds_integration elasticache_user elasticache_user_group_association
ecr_repository_policy ecr_registry_policy ecr_lifecycle_policy ecr_repository_creation_template
eks_access_policy_association api_gateway_method api_gateway_method_response api_gateway_method_settings
api_gateway_integration api_gateway_integration_response api_gateway_resource api_gateway_gateway_response
api_gateway_base_path_mapping api_gateway_rest_api_policy api_gateway_rest_api_put
cloudfront_distribution_tenant_web_acl_association organizations_policy_attachment
organizations_delegated_administrator organizations_resource_policy ram_principal_association
ram_resource_association ram_resource_share_accepter ram_sharing_with_organization servicecatalog_tag_option_resource_association
servicecatalog_principal_portfolio_association servicecatalog_portfolio_share servicecatalog_constraint
servicecatalog_provisioning_artifact servicecatalog_budget_resource_association servicecatalog_organizations_access
codebuild_source_credential codebuild_webhook codebuild_resource_policy codeartifact_domain_permissions_policy
codeartifact_repository_permissions_policy cognito_user cognito_user_in_group cognito_user_pool_ui_customization
cognito_identity_pool_roles_attachment cognito_managed_user_pool_client cognito_identity_pool_provider_principal_tag
guardduty_member guardduty_invite_accepter guardduty_organization_admin_account guardduty_detector_feature
guardduty_organization_configuration guardduty_organization_configuration_feature securityhub_member
securityhub_invite_accepter securityhub_account securityhub_organization_admin_account securityhub_organization_configuration
securityhub_standards_control securityhub_standards_control_association securityhub_finding_aggregator
securityhub_action_target securityhub_product_subscription config_configuration_recorder_status
config_retention_configuration config_organization_custom_rule config_organization_managed_rule
config_organization_conformance_pack config_organization_custom_policy_rule cloudtrail_organization_delegated_admin_account
ssoadmin_managed_policy_attachment ssoadmin_customer_managed_policy_attachment ssoadmin_permissions_boundary_attachment
ssoadmin_permission_set_inline_policy ssoadmin_account_assignment ssoadmin_application_assignment
ssoadmin_application_access_scope ssoadmin_application_assignment_configuration ssoadmin_trusted_token_issuer
ssoadmin_instance_access_control_attributes identitystore_group_membership glue_partition glue_partition_index
glue_resource_policy glue_data_catalog_encryption_settings glue_catalog_table_optimizer glue_user_defined_function
glue_security_configuration redshift_snapshot_copy redshift_snapshot_schedule_association redshift_cluster_iam_roles
redshift_endpoint_authorization redshift_partner redshift_resource_policy redshift_data_share_authorization
redshift_data_share_consumer_association redshift_logging redshift_authentication_profile redshift_hsm_client_certificate
redshift_hsm_configuration redshift_usage_limit redshift_cluster_snapshot redshift_parameter_group opensearch_vpc_endpoint
opensearch_inbound_connection_accepter opensearch_outbound_connection opensearch_package opensearch_package_association
opensearch_domain_saml_options opensearch_authorize_vpc_endpoint_access sesv2_account_suppression_attributes
sesv2_account_vdm_attributes sesv2_email_identity_feedback_attributes sesv2_email_identity_mail_from_attributes
sesv2_email_identity_policy sesv2_configuration_set_event_destination sesv2_dedicated_ip_assignment
ses_domain_dkim ses_domain_mail_from ses_identity_notification_topic ses_identity_policy ses_domain_identity_verification
ses_active_receipt_rule_set ses_receipt_rule ses_receipt_filter ses_email_identity ses_domain_identity
ses_configuration_set ses_event_destination ses_template ses_receipt_rule_set backup_vault_policy backup_vault_notifications
backup_vault_lock_configuration backup_global_settings backup_region_settings backup_plan backup_selection
grafana_role_association grafana_workspace_api_key grafana_license_association grafana_workspace_saml_configuration
grafana_workspace_service_account grafana_workspace_service_account_token dx_connection_association dx_connection_confirmation
dx_hosted_connection dx_hosted_private_virtual_interface_accepter dx_hosted_public_virtual_interface_accepter
dx_hosted_transit_virtual_interface_accepter dx_bgp_peer dx_gateway_association_proposal dx_macsec_key_association
ebs_default_kms_key ebs_encryption_by_default ebs_snapshot_block_public_access ebs_snapshot_copy ebs_snapshot_import
ebs_fast_snapshot_restore ebs_volume_attachment default_vpc default_subnet default_route_table default_network_acl
default_security_group default_vpc_dhcp_options ec2_allowed_images_settings ec2_image_block_public_access
ec2_instance_metadata_defaults ec2_serial_console_access ec2_default_credit_specification ec2_availability_zone_group
s3_account_public_access_block s3_directory_bucket s3_bucket_metadata_configuration
""".split())


def candidates(r):
    """Kind-name candidates (lowercase, no separators) for a Terraform resource."""
    flat = r.replace("_", "")
    out = [flat]
    if r in ALIASES:
        out.append(ALIASES[r])
    parts = r.split("_")
    # longest matching prefix first
    for n in range(min(3, len(parts)), 0, -1):
        pre = "_".join(parts[:n])
        if pre in TF_PREFIX_TO_SERVICE:
            rest = "".join(parts[n:])
            out.append(TF_PREFIX_TO_SERVICE[pre] + rest)
            out.append(TF_PREFIX_TO_SERVICE[pre] + rest.replace("configuration", "config"))
            break
    return out


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
    kinds, generated = set(), set()
    for f in glob.glob(os.path.join(ROOT, "api/v1alpha1/*.go")):
        base = os.path.basename(f)
        if base.startswith("zz_generated") or base.startswith("zz_providerref"):
            continue
        for k in re.findall(r"SchemeBuilder\.Register\(&(\w+)\{\}", open(f).read()):
            kinds.add(k.lower())
            if base.startswith("zz_cc_"):
                generated.add(k.lower())
    kinds.discard("awsprovider")
    covered = {}
    for r in tf:
        for cand in candidates(r):
            if cand in kinds:
                covered[r] = cand
                break
    prefix = lambda r: r.split("_")[0]
    inscope = [r for r in tf if prefix(r) in SCOPE]
    out = [r for r in tf if prefix(r) not in SCOPE]
    parented = [r for r in inscope if r not in covered and r in MODELLED_IN_PARENT]
    gap = [r for r in inscope if r not in covered and r not in MODELLED_IN_PARENT]
    g1 = [r for r in gap if prefix(r) in TIER1]
    g2 = [r for r in gap if prefix(r) not in TIER1]
    sha = subprocess.run(["git", "-C", TF, "rev-parse", "--short", "HEAD"], capture_output=True, text=True).stdout.strip()
    today = datetime.date.today().isoformat()

    def table(counter):
        return "".join(f"| {p} | {c} |\n" for p, c in counter.most_common())

    doc = [f"# Terraform AWS provider parity\n\nGenerated by `make parity` on {today} from hashicorp/terraform-provider-aws@{sha} (`website/docs/r/*`) against the kinds in `api/v1alpha1`. Do not hand-edit.\n\n",
           "| Metric | Count |\n|---|---|\n",
           f"| Terraform resources | {len(tf)} |\n| konfig kinds (excluding AWSProvider) | {len(kinds)} |\n| of which generated from the CloudFormation registry | {len(generated)} |\n| Terraform resources with a konfig equivalent | {len(covered)} |\n| Gap (all of Terraform) | {len(tf) - len(covered)} |\n\n",
           "## EKS-platform scope\n\nThe parity target is every service an EKS platform team touches. Media, gaming, end-user computing, telephony, and similar services are out of scope. Any in-scope resource without a native kind can be managed today through `CloudControlResource` when its CloudFormation type is registered.\n\n",
           "| Metric | Count |\n|---|---|\n",
           f"| In-scope Terraform resources | {len(inscope)} |\n| Covered by a kind | {len(inscope) - len(gap) - len(parented)} |\n| Modelled as a property of the parent kind | {len(parented)} |\n| Gap, tier 1 (core platform) | {len(g1)} |\n| Gap, tier 2 (data, ML, governance extras) | {len(g2)} |\n| Out of scope | {len(out)} |\n\n",
           "### Tier 1 gap by prefix\n\n| Prefix | Missing |\n|---|---|\n", table(collections.Counter(prefix(r) for r in g1)),
           "\n### Tier 2 gap by prefix\n\n| Prefix | Missing |\n|---|---|\n", table(collections.Counter(prefix(r) for r in g2)),
           "\n### Out-of-scope prefixes\n\n", ", ".join(sorted(set(prefix(r) for r in out))), "\n\n## Missing resources (in scope)\n"]
    cur = None
    for r in gap:
        if prefix(r) != cur:
            cur = prefix(r)
            doc.append(f"\n### {cur}\n\n")
        doc.append(f"- `aws_{r}` ({'T1' if r in g1 else 'T2'})\n")
    doc.append("\n## Modelled as a parent property\n\nTerraform splits these out as separate resources; konfig (and CloudFormation) express them as fields on the parent kind.\n\n")
    doc += [f"- `aws_{r}`\n" for r in parented]
    doc.append("\n## Covered resources\n\n| Terraform | konfig kind |\n|---|---|\n")
    doc += [f"| `aws_{r}` | `{covered[r]}` |\n" for r in tf if r in covered]
    open(os.path.join(ROOT, "docs/terraform-parity.md"), "w").write("".join(doc))
    open(os.path.join(ROOT, "hack/terraform-parity-gap.txt"), "w").write("".join(("T1 " if r in g1 else "T2 ") + "aws_" + r + "\n" for r in gap))
    print(f"terraform={len(tf)} kinds={len(kinds)} (generated={len(generated)}) covered={len(covered)} parent-property={len(parented)} in-scope gap: T1={len(g1)} T2={len(g2)}")


if __name__ == "__main__":
    main()
