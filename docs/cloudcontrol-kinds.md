# Typed kinds generated from the CloudFormation registry

Beyond the hand-written native kinds, konfig-konector ships typed kinds for
every in-scope AWS resource type that the AWS Cloud Control API can manage.
They are generated from the public CloudFormation resource schema registry by
`make gen-cloudcontrol` and reconciled by one shared engine, the same way the
Terraform `awscc` provider covers the long tail.

| | Native kinds | Generated kinds | `CloudControlResource` |
|---|---|---|---|
| Source | hand-written controllers per kind | `hack/gen-cloudcontrol/gen.py` from CloudFormation schemas | one generic kind |
| Spec | konfig field names, cross-resource refs, Secret refs | CloudFormation property names in lowerCamel, typed and validated | free-form `desiredState` JSON |
| Status | AWS identifiers, typed fields | shared `identifier` block plus read-only properties from the schema | `identifier` plus raw `properties` |
| Drift | per-controller describe and update | JSON Patch of every desired property that differs | same |
| Installed | always (`helm/konfig-konector/crds`) | opt-in per service bundle (`config/crd/cloudcontrol/<service>.yaml`) | always |

Every generated kind carries `spec.providerRef`, honours the namespace
provider annotation and the default provider, uses the finalizer and the
`aws.konfig.io/deletion-policy: abandon` annotation, and reports the
standard `Ready` condition. See `docs/multi-account.md`.

## Naming

Kinds are named `<Service><Type>` from the CloudFormation type name, with a
few conventional prefixes: `AWS::Logs::LogStream` becomes `LogsLogStream`,
`AWS::ElasticLoadBalancingV2::TrustStore` becomes `ELBv2TrustStore`,
`AWS::EC2::NetworkInterface` becomes `EC2NetworkInterface`. Property names
become lowerCamel fields: `LogGroupName` is `logGroupName`. Read-only
properties (`Arn`, `Id`, ...) live under `status`. Nested definitions become
nested structs; properties whose schema is a free-form object, a `oneOf`, or
an untyped map are exposed as raw JSON.

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: LogsQueryDefinition
metadata:
  name: api-errors
  namespace: platform
spec:
  providerRef: {name: prod}
  name: platform/api-errors
  queryString: "fields @timestamp, @message | filter @message like /ERROR/ | sort @timestamp desc"
  logGroupNames: ["/app/api"]
```

## Installing kinds

Installing all bundles adds over eight hundred CRDs, which is heavy for the
API server and for Helm's release size limit, so bundles are opt-in and live
outside the chart:

```sh
kubectl apply --server-side -f config/crd/cloudcontrol/logs.yaml
kubectl apply --server-side -f config/crd/cloudcontrol/ec2.yaml
```

At startup the operator checks which generated CRDs exist and starts a
controller only for those. The log line looks like:

```
Cloud Control typed kinds  started=13  skipped (CRD not installed)=815
```

Install a bundle later and restart the operator to pick it up. The Helm
ClusterRole grants the operator every resource in the `aws.konfig.io` group,
so no RBAC change is needed per bundle. `config/crd/cloudcontrol/README.md`
lists the bundles and their kind counts.

## Permissions

Cloud Control performs the AWS API calls using the caller's credentials (the
operator role or the assumed `AWSProvider` role). Each CloudFormation schema
lists the exact IAM actions its handlers need under `handlers.<op>.permissions`.
The Terraform spoke module attaches an administrator policy by default; narrow
it with the permissions from the schemas you use.

## Deliberately excluded

Billing, Invoicing, Cost and Usage Reports, BCM Data Exports, Budget actions
and Bedrock payment connectors are not generated (`EXCLUDE` in
`hack/gen-cloudcontrol/gen.py`). Automating account finances from a cluster
is a liability rather than a platform capability. Anyone who still needs one
can use the generic `CloudControlResource`.

## Destructive scope

Some kinds change state shared by every workload in an account, region, or
organization, or must carry credential material in their spec. They are
generated for completeness and marked in three places: the CRD description,
the reference site (`⚠ DESTRUCTIVE SCOPE` badge), and `kinds.json`
(`caution`). Examples: `OrganizationsOrganization`, `ControlTowerLandingZone`,
`SSOAdminInstance`, `LogsAccountPolicy`, `EC2VPCBlockPublicAccessOptions`,
`Route53ResolverResolverConfig`, `SecurityHubOrganizationConfiguration`,
`IAMServerCertificate`, `CodeBuildSourceCredential`.

Do not use them from application namespaces. Put them in a dedicated
platform namespace, point that namespace at a provider whose
`allowedNamespaces` lists only it, and grant RBAC on those kinds to the
platform team alone. Two CRs managing the same singleton will fight over it;
keep exactly one.

## Limits worth knowing

- **Create-only properties.** Changing a property the schema marks
  `createOnlyProperties` is rejected by AWS. The controller reports
  `Ready=False` with reason `UpdateNotSupported`; required create-only scalars
  are additionally immutable in the CRD.
- **Removing a property** from the spec does not remove it from the resource.
  Set it to the value you want instead.
- **Exporting.** `konfig-export` covers generated kinds whose schema has a
  `list` handler that needs no parent identifier. Types that require one
  (for example log streams) are reported and skipped.
- **Documentation.** Generated kinds carry a `CLOUD CONTROL` badge on the
  reference site and have full-options examples under `examples/<service>/`.

## Regenerating

```sh
make gen-cloudcontrol   # downloads the schema zip, regenerates types, CRDs, bundles
make parity             # re-measures coverage against the Terraform AWS provider
make gen-reference      # regenerates the site and examples
```

`hack/gen-cloudcontrol/gen.py` holds the in-scope service list, the kind
prefix table, and the list of CloudFormation types that already have a native
kind (`NATIVE`). Add a type there when a hand-written kind replaces its
generated counterpart.
