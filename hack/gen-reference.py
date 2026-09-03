#!/usr/bin/env python3
"""Generate the complete resource reference from CRD schemas.

Outputs (all regenerable — do not hand-edit):
  examples/<service>/<kind>.yaml        one full-options example per kind
  web/static/docs/<service>.html        per-service API reference pages
  web/static/docs/index.html            reference overview
  web/static/js/docs.js                 SIDEBAR_DATA block (rewritten in place)
  web/static/index.html                 landing "Everything documented" grid (rewritten in place)

Run: python3 hack/gen-reference.py   (or: make gen-reference)
"""
import glob
import html
import json
import os
import re
import sys

import yaml

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
CRDS = os.path.join(ROOT, "helm/konfig-konector/crds")
CONTROLLERS = os.path.join(ROOT, "internal/controller")
EXAMPLES = os.path.join(ROOT, "examples")
DOCS = os.path.join(ROOT, "web/static/docs")

SERVICE_LABELS = {
    "acm": "ACM", "acmpca": "ACM PCA", "amp": "Managed Prometheus",
    "apigateway": "API Gateway", "apigatewayv2": "API Gateway v2",
    "applicationautoscaling": "Application Auto Scaling", "apprunner": "App Runner",
    "athena": "Athena", "autoscaling": "Auto Scaling", "backup": "Backup",
    "batch": "Batch", "budgets": "Budgets", "cloudformation": "CloudFormation",
    "cloudfront": "CloudFront", "cloudtrail": "CloudTrail",
    "cloudwatch": "CloudWatch", "cloudwatchlogs": "CloudWatch Logs",
    "codeartifact": "CodeArtifact", "codebuild": "CodeBuild",
    "codecommit": "CodeCommit", "codedeploy": "CodeDeploy",
    "codepipeline": "CodePipeline", "cognito": "Cognito",
    "configservice": "AWS Config", "controltower": "Control Tower",
    "costexplorer": "Cost Explorer", "dax": "DAX", "dynamodb": "DynamoDB",
    "ec2": "EC2 & VPC", "ecr": "ECR", "ecs": "ECS", "efs": "EFS", "eks": "EKS",
    "elasticache": "ElastiCache", "elbv2": "Load Balancing",
    "eventbridge": "EventBridge", "firehose": "Data Firehose", "glue": "Glue",
    "grafana": "Managed Grafana", "guardduty": "GuardDuty",
    "iam": "IAM", "inspector2": "Inspector", "kafka": "MSK",
    "kinesis": "Kinesis", "kms": "KMS", "lambda": "Lambda",
    "memorydb": "MemoryDB", "mq": "Amazon MQ",
    "networkfirewall": "Network Firewall", "opensearch": "OpenSearch",
    "opensearchserverless": "OpenSearch Serverless",
    "organizations": "Organizations", "pipes": "EventBridge Pipes",
    "ram": "RAM", "rds": "RDS & Aurora", "redshift": "Redshift",
    "route53": "Route 53", "route53resolver": "Route 53 Resolver",
    "s3": "S3", "s3control": "S3 Control", "scheduler": "EventBridge Scheduler",
    "secretsmanager": "Secrets Manager", "securityhub": "Security Hub",
    "servicecatalog": "Service Catalog", "servicediscovery": "Cloud Map",
    "sesv2": "SES", "sfn": "Step Functions", "shield": "Shield",
    "sns": "SNS", "sqs": "SQS", "ssm": "SSM", "ssoadmin": "IAM Identity Center",
    "vpclattice": "VPC Lattice", "wafv2": "WAFv2", "xray": "X-Ray",
}

MAX_DEPTH = 5


def load_crds():
    crds = []
    for f in sorted(glob.glob(os.path.join(CRDS, "*.yaml"))):
        doc = yaml.safe_load(open(f))
        if not doc or doc.get("kind") != "CustomResourceDefinition":
            continue
        version = doc["spec"]["versions"][0]
        crds.append({
            "kind": doc["spec"]["names"]["kind"],
            "plural": doc["spec"]["names"]["plural"],
            "group": doc["spec"]["group"],
            "version": version["name"],
            "schema": version["schema"]["openAPIV3Schema"],
        })
    return crds


def kind_to_service():
    """Map each kind to its AWS service via the controller's internal/aws import."""
    mapping = {}
    for f in glob.glob(os.path.join(CONTROLLERS, "*_controller.go")):
        base = os.path.basename(f)[: -len("_controller.go")]
        src = open(f).read()
        m = re.findall(r'"github\.com/[^"]+/internal/aws/([a-z0-9]+)"', src)
        if not m:
            # some controllers use the AWS SDK client directly
            m = re.findall(r'"github\.com/aws/aws-sdk-go-v2/service/([a-z0-9]+)"', src)
        if m:
            mapping[base] = m[0]
    return mapping


def first_sentence(text):
    clean = re.sub(r"\s+", " ", (text or "")).strip()
    m = re.match(r"(.+?\.)\s", clean + " ")
    return (m.group(1) if m else clean)[:300]


# ── Example value synthesis ──────────────────────────────────────────────────

ID_PREFIXES = [
    ("imageid", "ami-0abc1234def567890"), ("amiid", "ami-0abc1234def567890"),
    ("vpcid", "vpc-0abc12def3456789"), ("subnetid", "subnet-0abc12def3456789"),
    ("securitygroupid", "sg-0abc12def3456789"), ("instanceid", "i-0abc1234def567890"),
    ("filesystemid", "fs-0abc1234def567890"), ("snapshotid", "snap-0abc1234def567890"),
    ("hostedzoneid", "Z0ABC123EXAMPLE"), ("certificateid", "12345678-1234-1234-1234-123456789012"),
]

PATTERN_CANDIDATES = [
    "example", "100", "alias/example", "10.0.0.0/16", "us-east-1", "1.0",
    "123456789012", "arn:aws:iam::123456789012:role/example-role",
    "ou-ab12-cdef3456", "example.com", "EXAMPLE", "t3.micro", "P7D",
]


def fit_constraints(v, prop):
    """Adjust a candidate string to satisfy pattern/minLength/maxLength."""
    pattern = prop.get("pattern")
    if pattern:
        try:
            if not re.fullmatch(pattern, v):
                for cand in PATTERN_CANDIDATES:
                    if re.fullmatch(pattern, cand):
                        v = cand
                        break
        except re.error:
            pass
    if not prop.get("pattern"):
        lo = prop.get("minLength")
        if lo and len(v) < lo:
            v = (v + "-" + "x" * lo)[:lo]
    hi = prop.get("maxLength")
    if hi and len(v) > hi:
        v = v[:hi]
    return v


def sample_string(name, prop, kind, required=False):
    lname = name.lower()
    if "enum" in prop:
        return prop["enum"][0]
    if lname.endswith("arn") or "rolearn" in lname:
        return "arn:aws:iam::123456789012:role/example-role"
    if "cidrblockipv6" in lname or lname.endswith("ipv6cidrblock"):
        return "2001:db8::/56"
    if "cidr" in lname:
        return "10.0.0.0/16"
    if lname == "region":
        return "us-east-1"
    if "availabilityzone" in lname:
        return "us-east-1a"
    if lname.endswith("accountid") or lname == "accountid":
        return "123456789012"
    if "email" in lname:
        return "ops@example.com"
    if lname.endswith("kmskeyid"):
        return "arn:aws:kms:us-east-1:123456789012:key/1234abcd-12ab-34cd-56ef-1234567890ab"
    if "policydocument" in lname or lname == "policy" or lname.endswith("policyjson"):
        return json.dumps({"Version": "2012-10-17", "Statement": [{"Effect": "Allow", "Action": ["s3:GetObject"], "Resource": "*"}]})
    if "assumerolepolicy" in lname:
        return json.dumps({"Version": "2012-10-17", "Statement": [{"Effect": "Allow", "Principal": {"Service": "pods.eks.amazonaws.com"}, "Action": ["sts:AssumeRole", "sts:TagSession"]}]})
    if lname.endswith("version") and "engine" not in lname:
        return "1.0"
    if "description" in lname or lname == "comment":
        return "Managed by konfig-konector"
    if lname.endswith("name") or lname == "name":
        return f"example-{kind.lower()}"
    if lname.endswith("id"):
        if not required:
            return ""
        for suffix, value in ID_PREFIXES:
            if lname.endswith(suffix):
                return fit_constraints(value, prop)
        return fit_constraints("example-id", prop)
    if "time" in lname and "zone" not in lname:
        return "2026-01-01T00:00:00Z"
    if prop.get("format") == "date-time":
        return "2026-01-01T00:00:00Z"
    return "example"


def sample_int(name, prop):
    if "minimum" in prop:
        return max(int(prop["minimum"]), 1)
    lname = name.lower()
    if "port" in lname:
        return 443
    if "retention" in lname or "days" in lname:
        return 7
    if "timeout" in lname or "seconds" in lname:
        return 300
    if "capacity" in lname or "count" in lname or "size" in lname:
        return 2
    return 1


def sample_bool(name):
    lname = name.lower()
    return any(t in lname for t in ("enable", "enabled", "encrypt", "versioning", "protect"))


def build_example(name, prop, kind, depth, required):
    """Return a sample value for a schema node, or None to omit."""
    t = prop.get("type")
    if t == "string":
        v = sample_string(name, prop, kind, required)
        if v == "" and not required:
            return None
        return fit_constraints(v, prop) if isinstance(v, str) and "enum" not in prop else v
    if t == "integer" or t == "number":
        return sample_int(name, prop)
    if t == "boolean":
        return sample_bool(name)
    if t == "array":
        items = prop.get("items", {})
        v = build_example(name, items, kind, depth + 1, True)
        if v is None:
            return None
        n = max(int(prop.get("minItems", 1)), 1)
        if n > 1 and isinstance(v, dict):
            import copy
            return [copy.deepcopy(v) for _ in range(n)]
        return [v] * n if not isinstance(v, dict) else [v]
    if t == "object":
        if "properties" not in prop:
            # map type (tags etc.)
            ap = prop.get("additionalProperties")
            if isinstance(ap, dict) and ap.get("type") == "string":
                if "tag" in name.lower():
                    return {"environment": "dev", "team": "platform"}
                return {"key": "value"}
            return {}
        if depth >= MAX_DEPTH and not required:
            return None
        if depth >= MAX_DEPTH + 4:
            return None
        out = {}
        req = set(prop.get("required", []))
        # ref-style objects: prefer the CR-name form and stop
        keys = set(prop["properties"].keys())
        if keys and keys <= {"name", "arn", "id", "namespace"}:
            return {"name": f"example-{name.lower().removesuffix('ref')}"}
        for child, cprop in prop["properties"].items():
            child_required = child in req
            # deep optional objects: skip to keep examples readable
            if not child_required and depth >= 3 and cprop.get("type") == "object":
                continue
            v = build_example(child, cprop, kind, depth + 1, child_required)
            if v is not None:
                out[child] = v
        return out if out else None
    return None


def example_yaml(crd, service):
    spec_schema = crd["schema"].get("properties", {}).get("spec", {})
    spec = build_example("spec", spec_schema, crd["kind"], 0, True) or {}
    doc = {
        "apiVersion": f"{crd['group']}/{crd['version']}",
        "kind": crd["kind"],
        "metadata": {
            "name": f"example-{crd['kind'].lower()}",
            "namespace": "default",
        },
        "spec": spec,
    }
    body = yaml.safe_dump(doc, sort_keys=False, default_flow_style=False, width=100)
    header = (
        f"# {crd['kind']} — {SERVICE_LABELS.get(service, service)}\n"
        f"# Full-options example generated from the CRD schema. Optional fields are\n"
        f"# included with sample values; delete what you don't need. Field reference:\n"
        f"# https://konfig-konector.io/docs/{service}.html#{crd['kind'].lower()}\n"
    )
    return header + body


# ── Field tables ─────────────────────────────────────────────────────────────

def flatten(prop, prefix, required, rows, depth=0):
    t = prop.get("type", "object")
    desc = first_sentence(prop.get("description"))
    tname = t
    if t == "array":
        items = prop.get("items", {})
        tname = f"array&lt;{items.get('type', 'object')}&gt;"
    if t == "object" and "additionalProperties" in prop and "properties" not in prop:
        tname = "map&lt;string&gt;"
    if "enum" in prop:
        tname = "enum"
        vals = ", ".join(str(v) for v in prop["enum"][:8])
        more = "…" if len(prop["enum"]) > 8 else ""
        desc = (desc + f" Allowed: <code>{html.escape(vals)}{more}</code>").strip()
    if prefix:
        rows.append((prefix, tname, required, desc))
    if depth >= 6:
        return
    if t == "object" and "properties" in prop:
        req = set(prop.get("required", []))
        for child, cprop in sorted(prop["properties"].items()):
            flatten(cprop, f"{prefix}.{child}" if prefix else child, child in req, rows, depth + 1)
    if t == "array" and isinstance(prop.get("items"), dict) and prop["items"].get("type") == "object":
        req = set(prop["items"].get("required", []))
        for child, cprop in sorted(prop["items"].get("properties", {}).items()):
            flatten(cprop, f"{prefix}[].{child}", child in req, rows, depth + 1)


def field_table(schema_part):
    rows = []
    flatten(schema_part, "", True, rows)
    if not rows:
        return "<p class=\"resource-desc\">No fields.</p>"
    out = ['<div class="docs-table-wrap"><table class="docs-table">',
           "<thead><tr><th>Field</th><th>Type</th><th>Required</th><th>Description</th></tr></thead><tbody>"]
    for path, tname, req, desc in rows:
        req_html = '<span class="req">yes</span>' if req else "—"
        out.append(
            f"<tr><td><code>{html.escape(path)}</code></td><td>{tname}</td>"
            f"<td>{req_html}</td><td>{desc}</td></tr>"
        )
    out.append("</tbody></table></div>")
    return "\n".join(out)


# ── HTML shells ──────────────────────────────────────────────────────────────

PAGE_HEAD = """<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>{title} — konfig-konector Docs</title>
  <meta name="description" content="{meta}" />
  <link rel="preconnect" href="https://fonts.googleapis.com" />
  <link href="https://fonts.googleapis.com/css2?family=IBM+Plex+Sans:wght@400;500;600;700&family=IBM+Plex+Mono:wght@400;500;600&display=swap" rel="stylesheet" />
  <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/styles/atom-one-dark.min.css" />
  <link rel="stylesheet" href="/css/style.css" />
  <link rel="stylesheet" href="/css/docs.css" />
</head>
<body class="docs-body">

<nav>
  <div class="nav-inner">
    <a class="nav-logo" href="/">
      <div class="nav-logo-icon"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M12 2.5l8.2 4.75v9.5L12 21.5l-8.2-4.75v-9.5L12 2.5z"/><circle cx="12" cy="12" r="3" fill="currentColor" stroke="none"/></svg></div>
      konfig-konector
    </a>
    <ul class="nav-links">
      <li><a href="/#features">Features</a></li>
      <li><a href="/#resources">Resources</a></li>
      <li><a href="/#quickstart">Quick Start</a></li>
      <li><a href="/docs/">Docs</a></li>
      <li><a class="nav-cta" href="https://github.com/konfig-io/konfig-konector">GitHub</a></li>
    </ul>
  </div>
</nav>

<div class="docs-wrap">
  <aside class="docs-sidebar" id="docs-sidebar"></aside>
  <div class="sidebar-overlay" id="sidebar-overlay"></div>

  <main class="docs-content">
"""

PAGE_FOOT = """  </main>
</div>

<button class="sidebar-toggle" id="sidebar-toggle" aria-label="Toggle sidebar">☰</button>
<script src="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/highlight.min.js"></script>
<script src="/js/docs.js"></script>
</body>
</html>
"""


def render_service_page(service, label, kinds_data):
    kind_names = ", ".join(k["crd"]["kind"] for k in kinds_data)
    parts = [PAGE_HEAD.format(title=label, meta=html.escape(f"API reference for {kind_names}."))]
    parts.append(f"""    <div class="breadcrumb">
      <a href="/docs/">docs</a>
      <span class="breadcrumb-sep">/</span>
      <span class="breadcrumb-current">{service}</span>
    </div>

    <h1 class="docs-page-title">{html.escape(label)}</h1>
    <p class="docs-page-subtitle">{len(kinds_data)} resource kind{'s' if len(kinds_data) != 1 else ''}. Generated from the operator's CRD schemas.</p>

    <div class="api-group-notice">
      <span>API Group:</span>
      <code>aws.konfig.io/v1alpha1</code>
    </div>
""")
    for k in kinds_data:
        crd, example = k["crd"], k["example"]
        kind = crd["kind"]
        anchor = kind.lower()
        props = crd["schema"].get("properties", {})
        desc = first_sentence(crd["schema"].get("description")) or f"{kind} resource."
        parts.append(f"""
    <section id="{anchor}" class="resource-section">
      <h2>{kind} <a href="#{anchor}" class="anchor">#</a> <span class="resource-badge">GA</span></h2>
      <p class="resource-desc">{html.escape(desc)}</p>
      <p class="resource-desc"><code>kubectl get {crd['plural']}</code> · <a href="https://github.com/konfig-io/konfig-konector/blob/main/examples/{service}/{anchor}.yaml">example on GitHub</a></p>

      <h3>Example</h3>
      <div class="code-wrap"><pre><code class="language-yaml">{html.escape(example)}</code></pre></div>

      <h3>Spec</h3>
      {field_table(props.get('spec', {}))}

      <h3>Status</h3>
      {field_table(props.get('status', {}))}
    </section>
""")
    parts.append(PAGE_FOOT)
    return "".join(parts)


def render_index(services):
    total = sum(len(k) for k in services.values())
    cards = []
    for svc in sorted(services, key=lambda s: SERVICE_LABELS.get(s, s).lower()):
        label = SERVICE_LABELS.get(svc, svc.upper())
        n = len(services[svc])
        cards.append(
            f'      <a class="doc-card" href="/docs/{svc}.html">'
            f'<div><div class="doc-name">{html.escape(label)}</div>'
            f'<div class="doc-count">{n} kind{"s" if n != 1 else ""}</div></div></a>'
        )
    parts = [PAGE_HEAD.format(title="API Reference", meta=f"API reference for all {total} konfig-konector resource kinds.")]
    parts.append(f"""    <div class="breadcrumb">
      <span class="breadcrumb-current">docs</span>
    </div>

    <h1 class="docs-page-title">API Reference</h1>
    <p class="docs-page-subtitle">{total} resource kinds across {len(services)} AWS services, all under the
    <code>aws.konfig.io/v1alpha1</code> API group. Every page is generated from the operator's CRD
    schemas; every kind has a full-options example in the
    <a href="https://github.com/konfig-io/konfig-konector/tree/main/examples">examples/</a> directory.</p>

    <div class="docs-grid" style="margin-top:32px;">
{chr(10).join(cards)}
    </div>
""")
    parts.append(PAGE_FOOT)
    return "".join(parts)


def sidebar_js(services):
    groups = []
    for svc in sorted(services, key=lambda s: SERVICE_LABELS.get(s, s).lower()):
        label = SERVICE_LABELS.get(svc, svc.upper())
        items = ",\n".join(
            f"      {{ label: {json.dumps(k['crd']['kind'])}, anchor: {json.dumps(k['crd']['kind'].lower())} }}"
            for k in services[svc]
        )
        groups.append(
            "  {\n"
            f"    id: {json.dumps(svc)},\n"
            f"    label: {json.dumps(label)},\n"
            f"    href: {json.dumps('/docs/' + svc + '.html')},\n"
            f"    items: [\n{items}\n    ]\n"
            "  }"
        )
    return "const SIDEBAR_DATA = [\n" + ",\n".join(groups) + "\n];"


def landing_grid(services):
    ranked = sorted(services.items(), key=lambda kv: -len(kv[1]))[:19]
    ranked.sort(key=lambda kv: SERVICE_LABELS.get(kv[0], kv[0]).lower())
    cards = []
    for svc, kinds in ranked:
        label = SERVICE_LABELS.get(svc, svc.upper())
        cards.append(
            f'      <a class="doc-card" href="/docs/{svc}.html">\n'
            f'        <div><div class="doc-name">{html.escape(label)}</div>'
            f'<div class="doc-count">{len(kinds)} kinds</div></div>\n      </a>'
        )
    cards.append(
        '      <a class="doc-card" href="/docs/">\n'
        '        <div><div class="doc-name">Full API Reference</div>'
        f'<div class="doc-count">all {sum(len(v) for v in services.values())} kinds</div></div>\n      </a>'
    )
    return "\n".join(cards)


def main():
    crds = load_crds()
    ctrl_service = kind_to_service()

    services = {}
    unmapped = []
    for crd in crds:
        svc = ctrl_service.get(crd["kind"].lower())
        if not svc:
            unmapped.append(crd["kind"])
            svc = "other"
        example = example_yaml(crd, svc)
        services.setdefault(svc, []).append({"crd": crd, "example": example})
    for svc in services:
        services[svc].sort(key=lambda k: k["crd"]["kind"])

    # examples/
    for svc, kinds in services.items():
        d = os.path.join(EXAMPLES, svc)
        os.makedirs(d, exist_ok=True)
        for k in kinds:
            with open(os.path.join(d, f"{k['crd']['kind'].lower()}.yaml"), "w") as f:
                f.write(k["example"])
    with open(os.path.join(EXAMPLES, "README.md"), "w") as f:
        f.write("# Examples\n\nOne full-options example per resource kind, generated from the CRD\n"
                "schemas by `hack/gen-reference.py` (`make gen-reference`). Optional fields\n"
                "carry sample values — delete what you don't need. Hand-written end-to-end\n"
                "scenarios live at the top level of this directory.\n\n")
        for svc in sorted(services, key=lambda s: SERVICE_LABELS.get(s, s).lower()):
            label = SERVICE_LABELS.get(svc, svc.upper())
            f.write(f"- [{label}]({svc}/) — {len(services[svc])} kinds\n")

    # docs pages
    os.makedirs(DOCS, exist_ok=True)
    for old in glob.glob(os.path.join(DOCS, "*.html")):
        os.remove(old)
    for svc, kinds in services.items():
        label = SERVICE_LABELS.get(svc, svc.upper())
        with open(os.path.join(DOCS, f"{svc}.html"), "w") as f:
            f.write(render_service_page(svc, label, kinds))
    with open(os.path.join(DOCS, "index.html"), "w") as f:
        f.write(render_index(services))

    # sidebar data in docs.js
    docs_js_path = os.path.join(ROOT, "web/static/js/docs.js")
    src = open(docs_js_path).read()
    new_block = sidebar_js(services)
    src, n = re.subn(r"const SIDEBAR_DATA = \[.*?\n\];", new_block, src, count=1, flags=re.S)
    assert n == 1, "SIDEBAR_DATA block not found in docs.js"
    open(docs_js_path, "w").write(src)

    # landing page docs grid
    index_path = os.path.join(ROOT, "web/static/index.html")
    src = open(index_path).read()
    src, n = re.subn(
        r'(<div class="docs-grid fade-up">\n).*?(\n    </div>)',
        r"\1" + landing_grid(services) + r"\2",
        src, count=1, flags=re.S,
    )
    assert n == 1, "docs-grid block not found in index.html"
    open(index_path, "w").write(src)

    total = sum(len(v) for v in services.values())
    print(f"generated: {total} kinds, {len(services)} services")
    if unmapped:
        print(f"UNMAPPED kinds (filed under 'other'): {unmapped}")


if __name__ == "__main__":
    main()
