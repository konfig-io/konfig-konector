#!/usr/bin/env python3
"""Generate typed, Cloud Control-backed kinds from the CloudFormation resource
schema registry.

For every in-scope CloudFormation type that has create/read/delete handlers
and no hand-written native kind, emit:

  api/v1alpha1/zz_cc_<service>.go            typed Spec/Status/kind structs
  internal/controller/zz_cc_registry.go      kind registry for the shared engine
  internal/export/exporters/zz_cc_kinds.go   generic exporters (ListResources)
  hack/gen-cloudcontrol/kinds.json           kind -> service/typeName manifest

The shared engine (internal/controller/cc_engine.go) drives them; property
names round-trip through `cfn:"PropertyName"` struct tags (internal/cfn).

Run: make gen-cloudcontrol   (downloads CloudformationSchema.zip into /tmp)
"""
import glob
import io
import json
import os
import re
import subprocess
import sys
import urllib.request
import zipfile

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
SCHEMA_DIR = os.environ.get("CFN_SCHEMA_DIR", "/tmp/cfnschema")
SCHEMA_URL = "https://schema.cloudformation.us-east-1.amazonaws.com/CloudformationSchema.zip"
HERE = os.path.dirname(os.path.abspath(__file__))

# CloudFormation services an EKS platform team touches. Everything else stays
# reachable through the generic CloudControlResource kind.
SERVICES = set("""
ACMPCA APS ARCZonalShift AccessAnalyzer AccountAccess AmazonMQ Amplify ApiGateway ApiGatewayV2 AppConfig AppFlow
AppRunner AppSync ApplicationAutoScaling ApplicationInsights ApplicationSignals Athena AuditManager AutoScaling
BCMDataExports Backup Batch Bedrock BedrockAgentCore Billing Budgets CE CUR Cassandra CertificateManager Chatbot
CloudFormation CloudFront CloudHSM CloudTrail CloudWatch CodeArtifact CodeBuild CodeCommit CodeConnections
CodeDeploy CodeGuruProfiler CodeGuruReviewer CodePipeline CodeStarConnections CodeStarNotifications Cognito
ComputeOptimizer Config ControlTower DAX DMS DataSync DataZone Detective DirectConnect DocDB DynamoDB EC2 ECR ECS
EFS EKS EMR EMRContainers EMRServerless ElastiCache ElasticBeanstalk ElasticLoadBalancing ElasticLoadBalancingV2
EventSchemas Events Evidently FIS FMS FSx GlobalAccelerator Glue Grafana GuardDuty IAM IdentityStore ImageBuilder
Inspector InspectorV2 InternetMonitor Invoicing KMS KafkaConnect Kinesis KinesisAnalyticsV2 KinesisFirehose
LakeFormation Lambda LicenseManager Logs MSK MWAA Macie MemoryDB Neptune NetworkFirewall NetworkFlowMonitor
NetworkManager Notifications NotificationsContacts OSIS Oam ObservabilityAdmin OpenSearch OpenSearchServerless
OpenSearchService Organizations Pipes RAM RDS RUM Rbin Redshift RedshiftServerless ResilienceHub ResilienceHubV2
ResourceExplorer2 ResourceGroups RolesAnywhere Route53 Route53Profiles Route53RecoveryControl
Route53RecoveryReadiness Route53Resolver S3 S3Express S3ObjectLambda S3Tables SES SNS SQS SSM SSMContacts
SSMIncidents SSMQuickSetup SSO SageMaker Scheduler SecretsManager SecurityHub SecurityLake ServiceCatalog
ServiceCatalogAppRegistry ServiceDiscovery Shield Signer StepFunctions StorageGateway Synthetics Timestream
Transfer VerifiedPermissions VpcLattice WAFv2 XRay
""".split())

# Shorter, conventional kind prefixes for long CloudFormation service names.
SERVICE_PREFIX = {
    "ElasticLoadBalancingV2": "ELBv2", "ElasticLoadBalancing": "ELB", "ApplicationAutoScaling": "AppAutoScaling",
    "CertificateManager": "ACM", "KinesisFirehose": "Firehose", "OpenSearchService": "OpenSearch",
    "StepFunctions": "SFN", "ServiceDiscovery": "CloudMap", "AmazonMQ": "MQ", "Events": "EventBridge",
    "Logs": "Logs", "SSO": "SSOAdmin", "APS": "Prometheus", "Oam": "OAM",
}

# CloudFormation types already covered by hand-written native kinds. Kept
# explicit so a rename never silently double-registers a kind.
NATIVE = set("""
AWS::ACMPCA::CertificateAuthority AWS::APS::RuleGroupsNamespace AWS::APS::Workspace AWS::AmazonMQ::Broker
AWS::AmazonMQ::Configuration AWS::ApiGateway::Deployment AWS::ApiGateway::RestApi AWS::ApiGateway::Stage
AWS::ApiGatewayV2::Api AWS::ApiGatewayV2::ApiMapping AWS::ApiGatewayV2::Authorizer AWS::ApiGatewayV2::DomainName
AWS::ApiGatewayV2::Integration AWS::ApiGatewayV2::Route AWS::ApiGatewayV2::Stage AWS::ApiGatewayV2::VpcLink
AWS::AppRunner::AutoScalingConfiguration AWS::AppRunner::Service AWS::ApplicationAutoScaling::ScalableTarget
AWS::ApplicationAutoScaling::ScalingPolicy AWS::Athena::DataCatalog AWS::Athena::NamedQuery AWS::Athena::WorkGroup
AWS::AutoScaling::AutoScalingGroup AWS::AutoScaling::ScalingPolicy AWS::Backup::BackupPlan AWS::Backup::BackupSelection
AWS::Backup::BackupVault AWS::Batch::ComputeEnvironment AWS::Batch::JobDefinition AWS::Batch::JobQueue
AWS::Budgets::Budget AWS::CE::AnomalyMonitor AWS::CE::AnomalySubscription AWS::CertificateManager::Certificate
AWS::CloudFormation::Stack AWS::CloudFormation::StackSet AWS::CloudFront::CachePolicy AWS::CloudFront::Distribution
AWS::CloudFront::Function AWS::CloudFront::OriginAccessControl AWS::CloudTrail::Trail AWS::CloudWatch::Alarm
AWS::CloudWatch::CompositeAlarm AWS::CloudWatch::Dashboard AWS::CodeArtifact::Domain AWS::CodeArtifact::Repository
AWS::CodeBuild::Project AWS::CodeCommit::Repository AWS::CodeDeploy::Application AWS::CodeDeploy::DeploymentGroup
AWS::CodePipeline::Pipeline AWS::Cognito::IdentityProvider AWS::Cognito::UserPool AWS::Cognito::UserPoolClient
AWS::Config::ConfigRule AWS::Config::ConfigurationRecorder AWS::Config::DeliveryChannel AWS::ControlTower::EnabledControl
AWS::DAX::Cluster AWS::DynamoDB::GlobalTable AWS::DynamoDB::Table AWS::EC2::CapacityReservation AWS::EC2::CustomerGateway
AWS::EC2::EIP AWS::EC2::EIPAssociation AWS::EC2::EgressOnlyInternetGateway AWS::EC2::FlowLog AWS::EC2::Instance
AWS::EC2::InternetGateway AWS::EC2::KeyPair AWS::EC2::LaunchTemplate AWS::EC2::NatGateway AWS::EC2::NetworkAcl
AWS::EC2::PlacementGroup AWS::EC2::PrefixList AWS::EC2::RouteTable AWS::EC2::SecurityGroup AWS::EC2::SpotFleet
AWS::EC2::Subnet AWS::EC2::TransitGateway AWS::EC2::TransitGatewayAttachment AWS::EC2::TransitGatewayVpcAttachment
AWS::EC2::VPC AWS::EC2::VPCEndpoint AWS::EC2::VPCEndpointService AWS::EC2::VPCPeeringConnection AWS::EC2::VPNConnection
AWS::EC2::VPNConnectionRoute AWS::EC2::VPNGateway AWS::EC2::Volume AWS::ECR::LifecyclePolicy AWS::ECR::Repository
AWS::ECS::CapacityProvider AWS::ECS::Cluster AWS::ECS::Service AWS::ECS::TaskDefinition AWS::EFS::AccessPoint
AWS::EFS::FileSystem AWS::EFS::MountTarget AWS::EKS::AccessEntry AWS::EKS::Addon AWS::EKS::Cluster
AWS::EKS::FargateProfile AWS::EKS::IdentityProviderConfig AWS::EKS::Nodegroup AWS::EKS::PodIdentityAssociation
AWS::ElastiCache::ParameterGroup AWS::ElastiCache::ReplicationGroup AWS::ElastiCache::ServerlessCache
AWS::ElastiCache::SubnetGroup AWS::ElasticLoadBalancingV2::Listener AWS::ElasticLoadBalancingV2::ListenerRule
AWS::ElasticLoadBalancingV2::LoadBalancer AWS::ElasticLoadBalancingV2::TargetGroup AWS::Events::EventBus
AWS::Events::Rule AWS::Glue::Connection AWS::Glue::Crawler AWS::Glue::Database AWS::Glue::Job AWS::Glue::Trigger
AWS::Grafana::Workspace AWS::GuardDuty::Detector AWS::IAM::Group AWS::IAM::InstanceProfile AWS::IAM::ManagedPolicy
AWS::IAM::OIDCProvider AWS::IAM::Role AWS::IAM::RolePolicy AWS::IAM::SAMLProvider AWS::IAM::User
AWS::IAM::UserToGroupAddition AWS::ImageBuilder::Image AWS::InspectorV2::Enabler AWS::KMS::Alias AWS::KMS::Key
AWS::Kinesis::Stream AWS::Kinesis::StreamConsumer AWS::KinesisFirehose::DeliveryStream AWS::Lambda::Alias
AWS::Lambda::CodeSigningConfig AWS::Lambda::EventInvokeConfig AWS::Lambda::EventSourceMapping AWS::Lambda::Function
AWS::Lambda::LayerVersion AWS::Lambda::Permission AWS::Lambda::Url AWS::Lambda::Version AWS::Logs::LogGroup
AWS::Logs::MetricFilter AWS::Logs::SubscriptionFilter AWS::MSK::Cluster AWS::MSK::Configuration
AWS::MSK::ServerlessCluster AWS::MemoryDB::Cluster AWS::NetworkFirewall::Firewall AWS::NetworkFirewall::FirewallPolicy
AWS::NetworkFirewall::RuleGroup AWS::OpenSearchServerless::Collection AWS::OpenSearchService::Domain
AWS::Organizations::Account AWS::Organizations::OrganizationalUnit AWS::Organizations::Policy AWS::Pipes::Pipe
AWS::RAM::ResourceShare AWS::RDS::DBCluster AWS::RDS::DBClusterParameterGroup AWS::RDS::DBInstance
AWS::RDS::DBParameterGroup AWS::RDS::DBProxy AWS::RDS::DBSubnetGroup AWS::RDS::EventSubscription
AWS::RDS::GlobalCluster AWS::RDS::OptionGroup AWS::Redshift::Cluster AWS::Redshift::ClusterParameterGroup
AWS::Redshift::ClusterSubnetGroup AWS::Route53::HealthCheck AWS::Route53::HostedZone AWS::Route53::KeySigningKey
AWS::Route53::RecordSet AWS::Route53Resolver::ResolverEndpoint AWS::Route53Resolver::ResolverRule
AWS::S3::AccessPoint AWS::S3::Bucket AWS::S3::BucketPolicy AWS::SES::ConfigurationSet AWS::SES::EmailIdentity
AWS::SNS::Subscription AWS::SNS::Topic AWS::SQS::Queue AWS::SSM::Association AWS::SSM::MaintenanceWindow
AWS::SSM::Parameter AWS::SSM::PatchBaseline AWS::SSO::Assignment AWS::SSO::PermissionSet
AWS::Scheduler::Schedule AWS::Scheduler::ScheduleGroup AWS::SecretsManager::RotationSchedule AWS::SecretsManager::Secret
AWS::SecurityHub::Hub AWS::SecurityHub::Standard AWS::ServiceCatalog::CloudFormationProduct AWS::ServiceCatalog::Portfolio
AWS::ServiceCatalog::PortfolioProductAssociation AWS::ServiceDiscovery::HttpNamespace AWS::ServiceDiscovery::Service
AWS::Shield::Protection AWS::StepFunctions::Activity AWS::StepFunctions::StateMachine AWS::VpcLattice::Listener
AWS::VpcLattice::Service AWS::VpcLattice::ServiceNetwork AWS::VpcLattice::ServiceNetworkServiceAssociation
AWS::VpcLattice::ServiceNetworkVpcAssociation AWS::VpcLattice::TargetGroup AWS::WAFv2::IPSet AWS::WAFv2::RegexPatternSet
AWS::WAFv2::RuleGroup AWS::WAFv2::WebACL AWS::XRay::Group AWS::XRay::SamplingRule
""".split())

GO_KEYWORDS = {"type", "func", "map", "range", "select", "case", "default", "go", "chan", "package", "import", "var", "const"}
LICENSE = open(os.path.join(ROOT, "hack", "boilerplate.go.txt")).read()


def ensure_schemas():
    if glob.glob(os.path.join(SCHEMA_DIR, "aws-*.json")):
        return
    os.makedirs(SCHEMA_DIR, exist_ok=True)
    print("downloading", SCHEMA_URL, file=sys.stderr)
    data = urllib.request.urlopen(SCHEMA_URL, timeout=120).read()
    zipfile.ZipFile(io.BytesIO(data)).extractall(SCHEMA_DIR)


def lower_camel(name):
    if not name:
        return name
    # Keep runs of capitals lowercase as a block: "ARN" -> "arn", "DBName" -> "dbName", "VPCId" -> "vpcId".
    m = re.match(r"^([A-Z]+)(?=[A-Z][a-z]|\d|$)(.*)$", name)
    if m and len(m.group(1)) > 1:
        return m.group(1).lower() + m.group(2)
    return name[0].lower() + name[1:]


def go_ident(name):
    s = re.sub(r"[^A-Za-z0-9]", "", name)
    if not s:
        s = "Field"
    if s[0].isdigit():
        s = "N" + s
    return s[0].upper() + s[1:]


def comment(text, width=110):
    if not text:
        return []
    text = re.sub(r"\s+", " ", str(text)).strip().replace("*/", "* /")
    if len(text) > 400:
        text = text[:397] + "..."
    words, lines, cur = text.split(" "), [], ""
    for w in words:
        if len(cur) + len(w) + 1 > width and cur:
            lines.append(cur)
            cur = w
        else:
            cur = (cur + " " + w).strip()
    if cur:
        lines.append(cur)
    return ["// " + l for l in lines]


# Type names already taken in package v1alpha1 (native types plus every
# generated kind's Spec/Status/List). Nested struct names must not collide.
USED_TYPES = set()


class KindGen:
    """Emits Go for one CloudFormation type."""

    def __init__(self, schema, kind):
        self.s = schema
        self.kind = kind
        self.defs = schema.get("definitions", {}) or {}
        self.structs = {}       # go name -> code
        self.def_names = {}     # definition name -> go type name (or builtin)
        self.in_progress = set()
        self.warnings = []

    # ---- schema -> Go type -------------------------------------------------
    def resolve_ref(self, ref):
        m = re.match(r"^#/definitions/([^/]+)$", ref)
        if not m or m.group(1) not in self.defs:
            return None, None
        return m.group(1), self.defs[m.group(1)]

    def is_tag_def(self, node):
        props = node.get("properties") or {}
        return node.get("type") == "object" and set(props.keys()) == {"Key", "Value"} and \
            props["Key"].get("type") == "string" and props["Value"].get("type") == "string"

    def gotype(self, node, hint, optional=True):
        """Return (go type string, extra markers list). Pointer-ness for scalars
        is applied here for optional fields."""
        node = node or {}
        if "$ref" in node:
            name, target = self.resolve_ref(node["$ref"])
            if name is None:
                return "apiextensionsv1.JSON", ["+kubebuilder:pruning:PreserveUnknownFields"]
            if self.is_tag_def(target):
                return "CFNTag", []
            tgt_type = target.get("type")
            if tgt_type == "object" and target.get("properties"):
                gname = self.struct_for(target, self.kind + go_ident(name), defname=name)
                return ("*" + gname if optional else gname), []
            # alias of a primitive/array/map: inline it
            return self.gotype(target, hint, optional)
        t = node.get("type")
        if isinstance(t, list):
            t = [x for x in t if x != "null"]
            t = t[0] if len(t) == 1 else None
        if "oneOf" in node or "anyOf" in node or t is None and not node.get("properties"):
            if "enum" in node and all(isinstance(v, str) for v in node["enum"]):
                t = "string"
            elif node.get("properties"):
                t = "object"
            else:
                return "apiextensionsv1.JSON", ["+kubebuilder:pruning:PreserveUnknownFields"]
        if t == "string":
            markers = []
            enum = node.get("enum")
            if enum and all(isinstance(v, str) and re.match(r"^[A-Za-z][A-Za-z0-9._/+-]*$", v) for v in enum) and len(enum) <= 60:
                markers.append("+kubebuilder:validation:Enum=" + ";".join(enum))
            if isinstance(node.get("minLength"), int) and node["minLength"] > 0 and not optional:
                markers.append("+kubebuilder:validation:MinLength=%d" % node["minLength"])
            if isinstance(node.get("maxLength"), int) and 0 < node["maxLength"] < 1 << 20:
                markers.append("+kubebuilder:validation:MaxLength=%d" % node["maxLength"])
            return "string", markers
        if t == "boolean":
            return ("*bool" if optional else "bool"), []
        if t == "integer":
            markers = []
            if isinstance(node.get("minimum"), (int, float)):
                markers.append("+kubebuilder:validation:Minimum=%d" % int(node["minimum"]))
            if isinstance(node.get("maximum"), (int, float)) and node["maximum"] < 1 << 62:
                markers.append("+kubebuilder:validation:Maximum=%d" % int(node["maximum"]))
            return ("*int64" if optional else "int64"), markers
        if t == "number":
            return ("*float64" if optional else "float64"), []
        if t == "array":
            items = node.get("items") or {}
            it, _ = self.gotype(items, hint + "Item", optional=False)
            if it.startswith("*"):
                it = it[1:]
            return "[]" + it, []
        if t == "object":
            props = node.get("properties")
            if props:
                gname = self.struct_for(node, hint)
                return ("*" + gname if optional else gname), []
            ap = node.get("additionalProperties")
            pp = node.get("patternProperties")
            val = None
            if isinstance(ap, dict):
                val = ap
            elif isinstance(pp, dict) and len(pp) >= 1:
                val = list(pp.values())[0]
            if isinstance(val, dict):
                vt, _ = self.gotype(val, hint + "Value", optional=False)
                if vt.startswith("*"):
                    vt = vt[1:]
                if vt in ("string", "int64", "bool", "float64", "[]string", "CFNTag") or vt.startswith(self.kind) or vt == "apiextensionsv1.JSON":
                    if vt == "apiextensionsv1.JSON":
                        return "map[string]apiextensionsv1.JSON", ["+kubebuilder:pruning:PreserveUnknownFields"]
                    return "map[string]" + vt, []
            return "apiextensionsv1.JSON", ["+kubebuilder:pruning:PreserveUnknownFields"]
        return "apiextensionsv1.JSON", ["+kubebuilder:pruning:PreserveUnknownFields"]

    def struct_for(self, node, gname, defname=None):
        if defname is not None and defname in self.def_names:
            return self.def_names[defname]
        base = gname
        n = 2
        while (gname in self.structs and self.structs[gname] is not node) or (gname in USED_TYPES and gname not in self.structs):
            gname = "%s%d" % (base, n)
            n += 1
        if gname in self.structs:
            return gname
        if defname is not None:
            self.def_names[defname] = gname
        USED_TYPES.add(gname)
        self.structs[gname] = node  # reserve (handles recursion)
        required = set(node.get("required") or [])
        fields, used = [], set()
        for pname, pnode in (node.get("properties") or {}).items():
            fname = go_ident(pname)
            while fname in used:
                fname += "_"
            used.add(fname)
            jname = lower_camel(pname)
            gt, markers = self.gotype(pnode, gname + fname, optional=pname not in required)
            lines = comment(pnode.get("description") if isinstance(pnode, dict) else None)
            for m in markers:
                lines.append("// " + m)
            if pname not in required:
                lines.append("// +optional")
                tag = '`json:"%s,omitempty" cfn:"%s"`' % (jname, pname)
            else:
                tag = '`json:"%s" cfn:"%s"`' % (jname, pname)
            lines.append("%s %s %s" % (fname, gt, tag))
            fields.append("\n".join("\t" + l for l in lines))
        code = "// %s is a nested property type of %s.\ntype %s struct {\n%s\n}\n" % (gname, self.s["typeName"], gname, "\n\n".join(fields))
        self.structs[gname] = code
        return gname

    # ---- top level ---------------------------------------------------------
    def emit(self):
        s = self.s
        props = s.get("properties") or {}
        required = set(s.get("required") or [])
        read_only = {p.split("/")[-1] for p in s.get("readOnlyProperties", []) if p.startswith("/properties/") and p.count("/") == 2}
        create_only = {p.split("/")[-1] for p in s.get("createOnlyProperties", []) if p.startswith("/properties/") and p.count("/") == 2}
        spec_fields, status_fields, used = [], [], set()
        spec_fields.append("\t// ProviderRef selects the AWSProvider (account/region) this resource is\n\t// reconciled against. Defaults to the namespace annotation, then the\n\t// default provider, then the operator's own credentials.\n\t// +optional\n\tProviderRef *ProviderRef `json:\"providerRef,omitempty\"`")
        used.add("ProviderRef")
        for pname, pnode in props.items():
            fname = go_ident(pname)
            while fname in used:
                fname += "_"
            used.add(fname)
            jname = lower_camel(pname)
            if jname == "providerRef":
                jname = "cfnProviderRef"
            is_ro = pname in read_only
            optional = is_ro or pname not in required
            gt, markers = self.gotype(pnode, self.kind + fname, optional=optional)
            lines = comment(pnode.get("description") if isinstance(pnode, dict) else None)
            if is_ro:
                lines.append("// Read-only: reported by AWS after creation.")
            for m in markers:
                lines.append("// " + m)
            if not is_ro and pname in create_only:
                lines.append("// Create-only in AWS: changing it requires replacing the resource.")
                if pname in required and gt in ("string", "int64", "bool"):
                    lines.append('// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="%s is immutable"' % jname)
            if optional:
                lines.append("// +optional")
                tag = '`json:"%s,omitempty" cfn:"%s"`' % (jname, pname)
            else:
                tag = '`json:"%s" cfn:"%s"`' % (jname, pname)
            lines.append("%s %s %s" % (fname, gt, tag))
            block = "\n".join("\t" + l for l in lines)
            (status_fields if is_ro else spec_fields).append(block)
        k = self.kind
        desc = re.sub(r"\s+", " ", s.get("description", "")).strip()
        head = comment("%s manages %s through the AWS Cloud Control API. %s" % (k, s["typeName"], desc))
        out = []
        out.append("// %sSpec is the desired state of %s.\ntype %sSpec struct {\n%s\n}\n" % (k, s["typeName"], k, "\n\n".join(spec_fields)))
        out.append("// %sStatus is the observed state of %s.\ntype %sStatus struct {\n\tCloudControlStatus `json:\",inline\"`\n%s\n}\n" % (
            k, s["typeName"], k, ("\n" + "\n\n".join(status_fields)) if status_fields else ""))
        out.append("\n".join(head) + "\n" + """// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Identifier",type="string",JSONPath=".status.identifier"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type %(k)s struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   %(k)sSpec   `json:"spec,omitempty"`
	Status %(k)sStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// %(k)sList contains a list of %(k)s.
type %(k)sList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []%(k)s `json:"items"`
}

func (o *%(k)s) GetProviderRef() *ProviderRef              { return o.Spec.ProviderRef }
func (o *%(k)s) CloudControlTypeName() string              { return %(tn)r }
func (o *%(k)s) CloudControlSpec() interface{}             { return &o.Spec }
func (o *%(k)s) CloudControlStatusRef() *CloudControlStatus { return &o.Status.CloudControlStatus }
func (o *%(k)s) CloudControlObserved() interface{}         { return &o.Status }
""" % {"k": k, "tn": s["typeName"]})
        out.extend(v for v in self.structs.values() if isinstance(v, str))
        return "\n".join(out).replace("%r" % s["typeName"], '"%s"' % s["typeName"])


def kind_name(type_name):
    _, service, typ = type_name.split("::")
    prefix = SERVICE_PREFIX.get(service, service)
    return go_ident(prefix) + go_ident(typ)


def main():
    ensure_schemas()
    native_kinds = set()
    for f in glob.glob(os.path.join(ROOT, "api/v1alpha1/*_types.go")):
        native_kinds |= set(re.findall(r"SchemeBuilder\.Register\(&(\w+)\{\}", open(f).read()))
    by_service, manifest, skipped = {}, [], {"native": 0, "nohandlers": 0, "outofscope": 0, "collision": 0}
    for path in sorted(glob.glob(os.path.join(SCHEMA_DIR, "aws-*.json"))):
        try:
            s = json.load(open(path))
        except Exception:
            continue
        tn = s.get("typeName", "")
        if not tn.startswith("AWS::"):
            continue
        service = tn.split("::")[1]
        if service not in SERVICES:
            skipped["outofscope"] += 1
            continue
        h = s.get("handlers", {})
        if not all(k in h for k in ("create", "read", "delete")):
            skipped["nohandlers"] += 1
            continue
        if tn in NATIVE:
            skipped["native"] += 1
            continue
        kind = kind_name(tn)
        if kind in native_kinds:
            skipped["collision"] += 1
            print("collision with native kind, skipping:", tn, "->", kind, file=sys.stderr)
            continue
        by_service.setdefault(service, []).append((kind, s))
        manifest.append({"kind": kind, "typeName": tn, "service": service.lower(), "list": "list" in h})

    # reserve every type name in the package before emitting nested structs
    for f in glob.glob(os.path.join(ROOT, "api/v1alpha1/*.go")):
        if os.path.basename(f).startswith("zz_"):
            continue
        USED_TYPES.update(re.findall(r"^type (\w+) ", open(f).read(), re.M))
    for kinds in by_service.values():
        for kind, _ in kinds:
            USED_TYPES.update({kind, kind + "Spec", kind + "Status", kind + "List"})

    # wipe previous output
    for f in glob.glob(os.path.join(ROOT, "api/v1alpha1/zz_cc_*.go")):
        os.remove(f)
    total = 0
    for service, kinds in sorted(by_service.items()):
        parts = [LICENSE, "// Code generated by hack/gen-cloudcontrol/gen.py from the CloudFormation schema registry. DO NOT EDIT.\n",
                 "package v1alpha1\n", 'import (\n\tapiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"\n\tmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"\n)\n',
                 "var _ = apiextensionsv1.JSON{}\n"]
        regs = []
        for kind, s in sorted(kinds, key=lambda x: x[0]):
            parts.append(KindGen(s, kind).emit())
            regs.append("\tSchemeBuilder.Register(&%s{}, &%sList{})" % (kind, kind))
            total += 1
        parts.append("func init() {\n%s\n}\n" % "\n".join(regs))
        open(os.path.join(ROOT, "api/v1alpha1/zz_cc_%s.go" % service.lower()), "w").write("\n".join(parts))

    reg = [LICENSE, "// Code generated by hack/gen-cloudcontrol/gen.py. DO NOT EDIT.\n", "package controller\n",
           'import (\n\tawsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"\n\t"github.com/konfig-io/konfig-konector/internal/cfn"\n)\n', "func init() {\n\tRegisterCloudControlKinds("]
    for m in sorted(manifest, key=lambda m: m["kind"]):
        reg.append('\t\tCloudControlKind{Kind: "%s", TypeName: "%s", New: func() cfn.CloudControlObject { return &awsv1alpha1.%s{} }},' % (m["kind"], m["typeName"], m["kind"]))
    reg.append("\t)\n}\n")
    open(os.path.join(ROOT, "internal/controller/zz_cc_registry.go"), "w").write("\n".join(reg))

    exp = [LICENSE, "// Code generated by hack/gen-cloudcontrol/gen.py. DO NOT EDIT.\n", "package exporters\n",
           'import (\n\tawsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"\n\t"github.com/konfig-io/konfig-konector/internal/cfn"\n)\n', "func init() {"]
    for m in sorted(manifest, key=lambda m: m["kind"]):
        if m["list"]:
            exp.append('\tregisterCloudControlExporter("%s", "%s", "%s", func() cfn.CloudControlObject { return &awsv1alpha1.%s{} })' % (m["kind"], m["typeName"], m["service"], m["kind"]))
    exp.append("}\n")
    open(os.path.join(ROOT, "internal/export/exporters/zz_cc_kinds.go"), "w").write("\n".join(exp))

    json.dump(sorted(manifest, key=lambda m: m["kind"]), open(os.path.join(HERE, "kinds.json"), "w"), indent=1)
    subprocess.run(["gofmt", "-w", os.path.join(ROOT, "api/v1alpha1"), os.path.join(ROOT, "internal/controller/zz_cc_registry.go"),
                    os.path.join(ROOT, "internal/export/exporters/zz_cc_kinds.go")], check=False)
    print("generated %d kinds across %d services; skipped %s" % (total, len(by_service), skipped))


if __name__ == "__main__":
    main()
