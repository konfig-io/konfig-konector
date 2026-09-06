# Multi-account and cross-account operation

One konfig-konector instance can manage any number of AWS accounts and
regions. Accounts are declared as cluster-scoped `AWSProvider` objects;
resources pick one with `spec.providerRef`, inherit one from their namespace,
or fall back to the default provider.

## How credentials flow

```
                       ┌──────────────────────────────┐
  EKS Pod Identity ───▶│ operator role (hub account)  │
                       └──────────────┬───────────────┘
                                      │ sts:AssumeRole
              ┌───────────────────────┼───────────────────────┐
              ▼                       ▼                       ▼
   AWSProvider "prod"       AWSProvider "network-hub"   AWSProvider "dev-eu"
   role in account A        role in account B          role in A, eu-west-1
```

Every reconcile resolves its `AWSProvider` into a scope (credentials, region,
account ID) and attaches it to the reconcile context. The SDK wrappers in
`internal/aws/multi` apply that scope as per-call options on every AWS API
call, so individual controllers are unaware of accounts. Assumed-role
credentials are cached per provider and refreshed two minutes before expiry.

## Declaring providers

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: AWSProvider
metadata:
  name: prod
spec:
  default: true                                   # primary account for unscoped resources
  roleArn: arn:aws:iam::111122223333:role/konfig-konector-spoke
  region: us-east-1
  externalId: optional-shared-secret
  durationSeconds: 3600
  allowedNamespaces: ["prod-*", "platform"]       # who may target this account
---
apiVersion: aws.konfig.io/v1alpha1
kind: AWSProvider
metadata:
  name: hub-eu
spec:
  region: eu-west-1                               # no roleArn: same account, other region
```

Fields:

| Field | Meaning |
|---|---|
| `roleArn` | Role to assume. Empty means "use the operator's own credentials", which makes the provider a pure region override. |
| `region` | Default region for resources using the provider. Falls back to the operator's region. |
| `default` | Marks the primary account. Resources that choose nothing land here. Without any default, unscoped resources use the operator's own account. |
| `externalId`, `sessionName`, `durationSeconds` | Passed to `sts:AssumeRole`. |
| `sourceProviderRef` | Chain assumption: assume `roleArn` using another provider's credentials (up to four hops). |
| `allowedNamespaces` | Exact names or trailing-`*` globs. A resource in any other namespace that references the provider fails with `AWSProvider "x" does not allow namespace "y"`. |

The `AWSProvider` controller verifies credentials with `sts:GetCallerIdentity`
and records the account ID and assumed role ARN in status:

```
$ kubectl get awsproviders
NAME     DEFAULT   ACCOUNT        REGION      READY   AGE
prod     true      111122223333   us-east-1   True    3m
hub-eu             999988887777   eu-west-1   True    3m
```

Helm can render providers from values:

```yaml
providers:
  - name: prod
    default: true
    roleArn: arn:aws:iam::111122223333:role/konfig-konector-spoke
    region: us-east-1
    allowedNamespaces: ["prod-*"]
```

## Selecting a provider from a resource

Resolution order, first match wins:

1. `spec.providerRef` on the resource.
2. The `aws.konfig.io/provider` annotation on the resource's namespace.
3. The `AWSProvider` with `spec.default: true`.
4. The operator's own credentials and region.

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: SQSQueue
metadata:
  name: orders
  namespace: prod-api
spec:
  providerRef:
    name: prod
    region: us-west-2      # optional per-resource region override
  queueName: orders
```

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: team-payments
  annotations:
    aws.konfig.io/provider: payments-account   # everything in here defaults to this account
```

Cross-resource references (`vpcRef`, `roleRef`, ...) resolve through the
Kubernetes API and copy AWS identifiers from the referenced CR's status, so a
resource in account A can reference a VPC CR that was created in account B.
AWS itself decides whether that combination is valid.

## IAM setup: hub and spoke

The operator role in the hub account needs `sts:AssumeRole` on every spoke
role, and each spoke role must trust the operator role.

```hcl
# hub (root module)
module "konfig" {
  source          = "./terraform"
  spoke_role_arns = ["arn:aws:iam::111122223333:role/konfig-konector-spoke"]
  # ...
}

# each spoke account
module "konfig_spoke" {
  source                = "./terraform/spoke"
  providers             = { aws = aws.prod }
  hub_operator_role_arn = module.konfig.operator_role_arn
  external_id           = "optional-shared-secret"
  policy_arns           = ["arn:aws:iam::aws:policy/PowerUserAccess"]
}
```

## Clusters without EKS Pod Identity

On k3s, kind, or any non-EKS cluster, mount static or SSO-derived keys via a
Secret and set `aws.credentialsSecret` in the Helm values. The operator's
credential chain picks up `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` /
`AWS_SESSION_TOKEN` from the environment. Providers with `roleArn` work the
same way on top of those base credentials.

## Two-sided resources: sharing, peering, acceptance

Some AWS relationships need an action in **both** accounts. These kinds do the
whole handshake from one cluster and only report `Ready=True` once AWS says the
relationship is active on both sides. While waiting they carry
`Ready=False` with reason `PendingAcceptance` and poll every 30 seconds.

### VPC peering

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: VPCPeeringConnection
metadata: {name: app-to-shared, namespace: prod-api}
spec:
  vpcRef: {name: app-vpc}                 # requester, reconciled under this resource's provider
  peerVpcId: vpc-0abc123                  # accepter
  peerOwnerId: "999988887777"
  peerRegion: eu-west-1
  accepterProviderRef: {name: shared}     # accept under the accepter account's provider
```

Same-account cross-region peers only need `autoAccept: true` and `peerRegion`.
Status shows `requesterVpcId`, `accepterVpcId`, `accepterAccountId`, and the
AWS state.

### Transit Gateway shared via RAM

```yaml
kind: ResourceShare                       # in the network-hub account
spec:
  providerRef: {name: network-hub}
  name: tgw
  resourceArns: ["arn:aws:ec2:us-east-1:999988887777:transit-gateway/tgw-0abc"]
  principals: ["111122223333"]
---
kind: ResourceShareInvitation             # in the spoke account
spec:
  providerRef: {name: prod}
  resourceShareRef: {name: tgw}           # or arn:
---
kind: TransitGatewayVpcAttachment         # in the spoke account
spec:
  providerRef: {name: prod}
  transitGatewayRef: {transitGatewayId: tgw-0abc}
  vpcRef: {name: app-vpc}
  subnetRefs: [{name: private-a}, {name: private-b}]
  accepterProviderRef: {name: network-hub}   # TGW owner accepts the attachment
```

`ResourceShareInvitation` accepts a pending invitation, or simply confirms
visibility when the share came through AWS Organizations without an
invitation, and lists the shared resource ARNs in status.

### Private hosted zone with VPCs in other accounts

```yaml
kind: HostedZoneVPCAssociation
spec:
  providerRef: {name: dns}                # zone owner
  hostedZoneRef: {name: internal-zone}
  vpcRef: {id: vpc-0def456}
  vpcRegion: us-east-1
  vpcProviderRef: {name: prod}            # VPC owner
```

The controller runs `CreateVPCAssociationAuthorization` in the zone account,
`AssociateVPCWithHostedZone` in the VPC account, then deletes the
authorization. Deleting the CR disassociates the VPC.

### PrivateLink provider side

```yaml
kind: VPCEndpointService
spec:
  networkLoadBalancerRefs: [{name: api-nlb}]
  acceptanceRequired: true
  autoAcceptConnections: true
  allowedPrincipals: ["arn:aws:iam::111122223333:root"]
```

Consumers create ordinary `VPCEndpoint` objects against
`status.serviceName`. With `autoAcceptConnections` the provider side accepts
every pending connection from an allowed principal on each reconcile; without
it, `status.pendingConnections` shows how many are waiting.

## Confirmation and audit

- Every `AWSProvider` reports the verified account ID and caller ARN.
- Two-sided kinds expose both sides' identifiers and the AWS state in status.
- Reconcile errors from the wrong account surface as `AccessDenied` in the
  `Ready` condition message, naming the operation that failed.
- `konfig-export` runs with a single credential set. Run it once per account
  (`AWS_PROFILE=prod konfig-export --namespace prod`) and add
  `providerRef` to the output, or annotate the target namespace.
