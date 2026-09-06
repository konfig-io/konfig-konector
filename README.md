# konfig-konector

**Manage AWS infrastructure as Kubernetes custom resources.**

konfig-konector is a Kubernetes operator that lets you declare AWS resources — IAM roles, VPCs, RDS instances, S3 buckets, and more — as native Kubernetes objects. The controller continuously reconciles your cluster's desired state against AWS, detecting and correcting configuration drift automatically.

Think of it as [Google Config Connector](https://cloud.google.com/config-connector/docs/overview), but for AWS and purpose-built for EKS teams who want to manage cloud infrastructure the same way they manage applications.

**Contents:** [Supported Resources](#supported-resources) · [Installation](#installation) · [Usage](#usage) · [Multi-account](#multi-account-and-cross-account) · [Cross-resource References](#cross-resource-references) · [Exporting an Existing Account](#exporting-an-existing-account) · [Drift Detection](#drift-detection) · [Architecture](#architecture) · [Building from Source](#building-from-source) · [Contributing](#contributing)

## Why konfig-konector?

| | konfig-konector | ACK | Crossplane |
|---|---|---|---|
| Unified IAM + EKS Pod Identity + DNS | ✅ | Fragmented across repos | ✅ |
| Multi-account from one instance (AWSProvider, per-resource or per-namespace) | ✅ | ❌ | ProviderConfig |
| Two-sided cross-account handshakes (peering, TGW, RAM, Route53, PrivateLink) | ✅ | ❌ | ❌ |
| Escape hatch for any Cloud Control type | ✅ `CloudControlResource` | ❌ | ❌ |
| Deep EKS Pod Identity integration | ✅ | ❌ | ❌ |
| Single operator binary | ✅ | One per service | ❌ |
| Drift detection & auto-correction | ✅ | Partial | ✅ |
| Helm + Terraform bootstrap | ✅ | ❌ | Partial |

## Supported Resources

The operator ships **246 hand-written resource kinds across ~75 AWS services** — IAM, EC2/VPC,
RDS/Aurora, S3, DynamoDB, Lambda, ECS, EKS, ElastiCache, SQS/SNS/EventBridge,
Route53, CloudFront, API Gateway, KMS, Secrets Manager, CloudWatch, and more.
List every kind with `konfig-export --list`, browse the generated API reference
at [konfig-konector.io/docs](https://konfig-konector.io/docs/), or start from the
full-options examples in [`examples/`](examples/) — one per kind, generated from
the CRD schemas (`make gen-reference`). On top of those, **819 typed kinds are
generated from the CloudFormation schema registry** and reconciled through the
AWS Cloud Control API, installed per service bundle from
[`config/crd/cloudcontrol/`](config/crd/cloudcontrol/) (see
[docs/cloudcontrol-kinds.md](docs/cloudcontrol-kinds.md)). Billing, invoicing and
cost-report kinds are deliberately excluded, and account- or organization-wide
settings are generated but flagged as destructive scope. Anything else can be
managed through the generic `CloudControlResource`. Coverage is measured
against the Terraform AWS provider in
[`docs/terraform-parity.md`](docs/terraform-parity.md) (`make parity`). A few
highlights:

| Kind | AWS Service | Notes |
|---|---|---|
| `AWSProvider` | STS | Cluster-scoped account/region target; see [Multi-account](#multi-account-and-cross-account) |
| `CloudControlResource` | Cloud Control API | Any `AWS::Service::Type` |
| `IAMRole` | IAM | |
| `IAMPolicy` | IAM | |
| `IAMPolicyAttachment` | IAM | |
| `IAMRolePolicy` | IAM | Inline policies |
| `PodIdentityAssociation` | EKS Pod Identity | |
| `HostedZone` | Route53 | |
| `RecordSet` | Route53 | A, AAAA, CNAME, MX, TXT, NS, SRV, CAA, PTR, alias |
| `HealthCheck` | Route53 | |
| `VPC` | EC2 | |
| `Subnet` | EC2 | |
| `InternetGateway` | EC2 | |
| `RouteTable` | EC2 | |
| `NatGateway` | EC2 | Async — polls until `available` |
| `SecurityGroup` | EC2 | |
| `VPCEndpoint` | EC2 | Interface and Gateway types |
| `VPCEndpointService` | EC2 | PrivateLink provider side; auto-accepts consumer connections |
| `VPCPeeringConnection` | EC2 | Cross-account/region; accepts under `accepterProviderRef` |
| `TransitGatewayVpcAttachment` | EC2 | Accepts shared-TGW attachments under the owner provider |
| `ResourceShareInvitation` | RAM | Accepts shares in the receiving account |
| `HostedZoneVPCAssociation` | Route53 | Cross-account private zone ↔ VPC handshake |
| `KeyPair` | EC2 | |
| `EC2Instance` | EC2 | Async — polls until `running` |
| `LaunchTemplate` | EC2 | New version created on spec change |
| `AutoScalingGroup` | Auto Scaling | |
| `DBSubnetGroup` | RDS | |
| `DBParameterGroup` | RDS | |
| `DBClusterParameterGroup` | RDS | |
| `DBInstance` | RDS | Async — polls until `available` |
| `DBCluster` | RDS | Aurora; async |
| `S3Bucket` | S3 | |
| `S3BucketPolicy` | S3 | |
| `SQSQueue` | SQS | Standard and FIFO |
| `SNSTopic` | SNS | |
| `SNSSubscription` | SNS | |
| `ElastiCacheSubnetGroup` | ElastiCache | |
| `ElastiCacheReplicationGroup` | ElastiCache | Redis/Valkey; async |

All resources use the API group `aws.konfig.io/v1alpha1`.

## Prerequisites

- **Kubernetes** 1.27+ running on EKS (any cluster works with `aws.credentialsSecret`; see [docs/multi-account.md](docs/multi-account.md))
- **EKS Pod Identity Agent** addon installed on your cluster
- **Helm** 3.x
- **Terraform** 1.0+ (for bootstrapping the operator IAM role)
- **Go** 1.24+ (for building from source)
- **Docker** (for building the image)

## Installation

### Step 1 — Bootstrap the operator IAM role

The operator authenticates to AWS via **EKS Pod Identity**. The Terraform module in `terraform/` creates:

- An IAM role the operator assumes, with permissions to manage all supported AWS resources
- The EKS Pod Identity association binding that role to the operator's `ServiceAccount`
- The `eks-pod-identity-agent` EKS addon (if not already installed)

Per-environment settings live in `terraform/accounts/<env>/`. Edit the two
files for your environment:

```bash
cd terraform

# accounts/dev/terraform.tfvars — your account, cluster, and IAM path scope:
#   account_id   = "123456789012"
#   cluster_name = "dev-eks"        # your EKS cluster
#   environment  = "dev"
# accounts/dev/backend.conf — your Terraform state bucket & lock table

terraform init -backend-config=accounts/dev/backend.conf
terraform apply -var-file=accounts/dev/terraform.tfvars
```

Note the `operator_role_arn` output — you need it in the next step.

#### IAM path scoping (optional)

By default the operator can only manage IAM roles and policies under the `/konfig/` path prefix. Adjust `allowed_iam_resource_paths` to match your organization's IAM structure:

```hcl
# Allow all paths
allowed_iam_resource_paths = "/"

# Restrict to a team-specific path
allowed_iam_resource_paths = "/platform/"
```

### Step 2 — Build and push the operator image

No public image is published yet, so build one into a registry your cluster
can pull from (ECR, GHCR, …):

```bash
make docker-build docker-push IMG=<your-registry>/konfig-konector:v0.1.0
```

### Step 3 — Install with Helm

```bash
helm install konfig-konector ./helm/konfig-konector \
  --namespace konfig-system \
  --create-namespace \
  --set image.repository=<your-registry>/konfig-konector \
  --set image.tag=v0.1.0 \
  --set aws.region=us-east-1 \
  --set operatorRoleArn=arn:aws:iam::123456789012:role/dev-konfig-konector-operator
```

The Terraform module also outputs a pre-filled command (set your image
repository in it before running):

```bash
terraform -chdir=terraform output -raw helm_install_command
```

CRDs are installed automatically by Helm before the controller starts.

### Step 4 — Verify

```bash
kubectl get pods -n konfig-system
# NAME                                   READY   STATUS    RESTARTS   AGE
# konfig-controller-7d9f8b6c4-xk2pq     1/1     Running   0          30s

kubectl get crds | grep konfig.io | head
# autoscalinggroups.aws.konfig.io
# dbinstances.aws.konfig.io
# ... (246 native CRDs; generated bundles under config/crd/cloudcontrol/)
```

## Usage

### IAM Role with EKS Pod Identity

Create an IAM role, attach a policy, and associate it with a Kubernetes `ServiceAccount` — all in one namespace:

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: IAMRole
metadata:
  name: my-app-role
  namespace: my-namespace
spec:
  roleName: my-app-role
  description: "Role for my application"
  maxSessionDuration: 3600
  assumeRolePolicyDocument: |
    {
      "Version": "2012-10-17",
      "Statement": [{
        "Effect": "Allow",
        "Principal": { "Service": "pods.eks.amazonaws.com" },
        "Action": ["sts:AssumeRole", "sts:TagSession"]
      }]
    }
  tags:
    team: platform
---
apiVersion: aws.konfig.io/v1alpha1
kind: IAMPolicy
metadata:
  name: my-app-policy
  namespace: my-namespace
spec:
  policyName: my-app-policy
  policyDocument: |
    {
      "Version": "2012-10-17",
      "Statement": [{
        "Effect": "Allow",
        "Action": ["s3:GetObject"],
        "Resource": "arn:aws:s3:::my-bucket/*"
      }]
    }
---
apiVersion: aws.konfig.io/v1alpha1
kind: IAMPolicyAttachment
metadata:
  name: my-app-attachment
  namespace: my-namespace
spec:
  roleRef:
    name: my-app-role
  policyRef:
    name: my-app-policy
---
apiVersion: aws.konfig.io/v1alpha1
kind: PodIdentityAssociation
metadata:
  name: my-app-pod-identity
  namespace: my-namespace
spec:
  clusterName: my-eks-cluster
  targetNamespace: my-namespace
  serviceAccountName: my-app-sa
  roleRef:
    name: my-app-role
  annotateServiceAccount: true
```

### VPC with subnets and internet gateway

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: VPC
metadata:
  name: my-vpc
  namespace: my-namespace
spec:
  cidrBlock: "10.0.0.0/16"
  enableDnsSupport: true
  enableDnsHostnames: true
  tags:
    env: prod
---
apiVersion: aws.konfig.io/v1alpha1
kind: Subnet
metadata:
  name: my-public-subnet
  namespace: my-namespace
spec:
  vpcRef:
    name: my-vpc
  cidrBlock: "10.0.1.0/24"
  availabilityZone: us-east-1a
  mapPublicIpOnLaunch: true
  tags:
    tier: public
---
apiVersion: aws.konfig.io/v1alpha1
kind: Subnet
metadata:
  name: my-private-subnet
  namespace: my-namespace
spec:
  vpcRef:
    name: my-vpc
  cidrBlock: "10.0.2.0/24"
  availabilityZone: us-east-1b
  tags:
    tier: private
---
apiVersion: aws.konfig.io/v1alpha1
kind: InternetGateway
metadata:
  name: my-igw
  namespace: my-namespace
spec:
  vpcRef:
    name: my-vpc
  tags:
    env: prod
---
apiVersion: aws.konfig.io/v1alpha1
kind: SecurityGroup
metadata:
  name: my-sg
  namespace: my-namespace
spec:
  vpcRef:
    name: my-vpc
  groupName: my-sg
  description: "Application security group"
  ingressRules:
    - protocol: tcp
      fromPort: 443
      toPort: 443
      cidrIpv4: "0.0.0.0/0"
  tags:
    env: prod
```

### RDS Instance

Passwords are read from Kubernetes Secrets and never stored in CR status.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-db-password
  namespace: my-namespace
type: Opaque
stringData:
  password: "my-secure-password"
---
apiVersion: aws.konfig.io/v1alpha1
kind: DBInstance
metadata:
  name: my-db
  namespace: my-namespace
spec:
  dbInstanceIdentifier: my-db
  dbInstanceClass: db.t3.micro
  engine: mysql
  engineVersion: "8.0"
  masterUsername: admin
  masterUserPasswordRef:
    name: my-db-password
    key: password
  dbName: myapp
  allocatedStorage: 20
  storageType: gp3
  storageEncrypted: true
  multiAZ: false
  publiclyAccessible: false
  dbSubnetGroupRef: my-db-subnet-group
  vpcSecurityGroupRefs:
    - name: my-sg
  backupRetentionPeriod: 7
  deletionProtection: true
  skipFinalSnapshot: false
  tags:
    env: prod
```

RDS instances take several minutes to provision. The controller polls every 30 seconds and sets `Ready: True` once the instance status reaches `available`.

### S3 Bucket with encryption and policy

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: S3Bucket
metadata:
  name: my-bucket
  namespace: my-namespace
spec:
  bucketName: my-globally-unique-bucket-name
  region: us-east-1
  versioning: true
  serverSideEncryption:
    sseAlgorithm: AES256
  tags:
    team: platform
---
apiVersion: aws.konfig.io/v1alpha1
kind: S3BucketPolicy
metadata:
  name: my-bucket-policy
  namespace: my-namespace
spec:
  bucketRef:
    name: my-bucket
  policyDocument: |
    {
      "Version": "2012-10-17",
      "Statement": [{
        "Effect": "Deny",
        "Principal": "*",
        "Action": "s3:*",
        "Resource": [
          "arn:aws:s3:::my-globally-unique-bucket-name",
          "arn:aws:s3:::my-globally-unique-bucket-name/*"
        ],
        "Condition": {
          "Bool": { "aws:SecureTransport": "false" }
        }
      }]
    }
```

### Route53 DNS record

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: HostedZone
metadata:
  name: my-zone
  namespace: my-namespace
spec:
  name: myapp.example.com.
  comment: "Managed by konfig-konector"
  tags:
    env: prod
---
apiVersion: aws.konfig.io/v1alpha1
kind: RecordSet
metadata:
  name: api-record
  namespace: my-namespace
spec:
  hostedZoneRef:
    name: my-zone
  name: api.myapp.example.com.
  type: A
  ttl: 300
  records:
    - "1.2.3.4"
```

## Multi-account and Cross-account

A single instance manages any number of AWS accounts. Declare each account as
a cluster-scoped `AWSProvider` (a role to assume plus a default region), then
select it per resource with `spec.providerRef`, per namespace with the
`aws.konfig.io/provider` annotation, or mark one provider `default: true` as
the primary account for everything else.

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: AWSProvider
metadata: {name: prod}
spec:
  default: true
  roleArn: arn:aws:iam::111122223333:role/konfig-konector-spoke
  region: us-east-1
  allowedNamespaces: ["prod-*"]
---
apiVersion: aws.konfig.io/v1alpha1
kind: SQSQueue
metadata: {name: orders, namespace: prod-api}
spec:
  providerRef: {name: prod, region: us-west-2}
  queueName: orders
```

Relationships that need actions in two accounts are handled end to end and
only report `Ready` once AWS confirms both sides: `VPCPeeringConnection`
(`accepterProviderRef`), `TransitGatewayVpcAttachment` (`accepterProviderRef`),
`ResourceShareInvitation`, `HostedZoneVPCAssociation` (`vpcProviderRef`) and
`VPCEndpointService`. The `terraform/spoke` module creates the per-account role.
Full details in [docs/multi-account.md](docs/multi-account.md).

## Cross-resource References

Resources reference each other by CR name within the same namespace. The controller waits — requeueing every 5 seconds — until the referenced resource has been provisioned and its AWS ID is available.

```yaml
# Reference by CR name — the operator resolves the real AWS ID automatically
vpcRef:
  name: my-vpc           # waits until VPC CR has a vpcId in status

securityGroupRefs:
  - name: my-sg          # waits until SecurityGroup CR has a groupId in status

# Or reference AWS resources directly by ID — no CR lookup performed
securityGroupRefs:
  - id: sg-0abc1234def
```

| Type | Fields |
|---|---|
| `VPCResourceRef` | `name` (CR) or `id` (direct `vpc-xxx`) |
| `SubnetRef` | `name` (CR) or `id` (direct `subnet-xxx`) |
| `SecurityGroupRef` | `name` (CR) or `id` (direct `sg-xxx`) |
| `PolicyRef` | `name` (CR) or `arn` (direct policy ARN) |
| `RoleRef` | `name` (CR) or `arn` (direct role ARN) |
| `SecretRef` | `name` + `key` (Kubernetes Secret) |

## Checking Resource Status

```bash
# List all managed resources in a namespace
kubectl get iamroles,iampolicies,vpcs,subnets,securitygroups -n my-namespace

# Get full status including conditions
kubectl get dbinstance my-db -n my-namespace -o yaml

# Watch a resource being provisioned
kubectl get dbinstance my-db -n my-namespace -w
```

All resources expose a standard `Ready` condition:

```yaml
status:
  conditions:
    - type: Ready
      status: "True"
      reason: Synced
      message: DB instance available
      lastTransitionTime: "2026-01-01T00:00:00Z"
  dbInstanceStatus: available
  endpoint: my-db.abc123.us-east-1.rds.amazonaws.com
  port: 3306
  observedGeneration: 1
  lastSyncTime: "2026-01-01T00:00:00Z"
```

## Exporting an Existing Account

`konfig-export` walks an AWS account (one region) and renders every supported
resource as CR YAML — the equivalent of GCP Config Connector's
`config-connector export`. It is strictly read-only (List/Describe/Get calls
only) and covers 235 of the 241 CRD kinds.

```bash
make build-export
AWS_PROFILE=platformdev ./bin/konfig-export --namespace konfig-system > account.yaml
./bin/konfig-export --services ec2,iam,s3 --output ./export/   # one file per service
./bin/konfig-export --list                                     # supported kinds
```

Safety properties:

- Every exported CR carries `aws.konfig.io/deletion-policy: abandon`, so
  applying the export and later deleting CRs can never delete AWS resources.
  Remove the annotation per-resource once you trust the operator to own it
  (`--no-abandon` disables this at export time; not recommended).
- Secret material is never exported: Secrets Manager values, SecureString SSM
  parameters, RDS master passwords, ElastiCache auth tokens, and Cognito/IdP
  client secrets are omitted. Where a spec requires a secret ref (RDS), a
  `CHANGEME` placeholder is emitted — create the referenced Kubernetes Secret
  before applying.
- AWS-managed and default resources (default VPC security groups, AWS-managed
  IAM policies, `Managed-*` cache policies, autodefined resolver rules,
  service-linked roles) are skipped.
- Cross-resource references are emitted by CR name when the referenced
  resource is part of the same export, and by raw AWS ID/ARN otherwise.

Apply the export, then run `kubectl get <kind> -A` and check Ready conditions;
the operator adopts each resource on first reconcile.

## Deletion Policy

By default, deleting a CR deletes the underlying AWS resource. To decommission the operator or hand a resource off to another management tool without destroying it, annotate the CR first:

```yaml
metadata:
  annotations:
    aws.konfig.io/deletion-policy: abandon
```

With this annotation, deleting the CR removes the finalizer and leaves the AWS resource untouched. This is also the escape hatch when AWS refuses a delete (for example an RDS instance with `deletionProtection: true`) and the CR would otherwise be stuck terminating.

Related safeguards:

- `Secret` (Secrets Manager) deletion uses the AWS default 30-day recovery window unless `spec.recoveryWindowInDays` is set. Immediate, unrecoverable deletion requires an explicit `spec.forceDelete: true`.
- `DBInstance`/`DBCluster` take a final snapshot on delete unless `spec.skipFinalSnapshot: true`.

## Drift Detection

The controller reconciles every 5 minutes (with ±10% jitter to avoid synchronized reconcile waves). If a resource is modified directly in AWS, the operator reverts it to match the CR spec on the next cycle.

**Always reconciles on requeue** (full drift correction):
VPC, Subnet, SecurityGroup, RouteTable, S3Bucket, SQSQueue, SNSTopic, and all IAM and Route53 resources.

**Only updates on spec change** (generation-gated):
DBInstance, DBCluster, LaunchTemplate, ElastiCacheReplicationGroup. These resources are expensive to modify and may cause downtime, so the controller only calls the AWS Modify API when the CR's `spec` changes.

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                     Kubernetes (EKS)                     │
│                                                          │
│  ┌───────────────┐   reconciles   ┌──────────────────┐  │
│  │  Your CRs     │ ◄───────────── │ konfig-controller │  │
│  │ (IAMRole,     │                │  (konfig-system)  │  │
│  │  VPC, etc.)   │                └────────┬─────────┘  │
│  └───────────────┘                         │             │
│                                            │ EKS Pod     │
│                                            │ Identity    │
└────────────────────────────────────────────┼─────────────┘
                                             │
                                    ┌────────▼────────┐
                                    │  AWS IAM Role   │
                                    │ (operator role) │
                                    └────────┬────────┘
                                             │
                              ┌──────────────▼──────────────┐
                              │          AWS APIs            │
                              │  IAM · EC2 · RDS · S3 · ... │
                              └─────────────────────────────┘
```

### Reconcile loop

Every controller follows the same pattern:

1. Fetch the CR; if not found, return (already deleted)
2. If `DeletionTimestamp` is set → delete the AWS resource, remove finalizer, return
3. Add finalizer if not present (prevents orphaned AWS resources on CR deletion)
4. Fetch current AWS state (live — no caching)
5. Create or update the AWS resource to match spec
6. Write observed state (IDs, ARNs, endpoints) back to `status`
7. Requeue after 5 minutes for drift detection

### Authentication

The operator uses the standard AWS SDK v2 credential chain. On EKS the recommended approach is **EKS Pod Identity**, which injects temporary credentials via a projected token — no long-lived keys, automatic rotation, no IRSA annotation required.

```
konfig-system/konfig-controller ServiceAccount
  └── EKS Pod Identity Association
        └── IAM Role: <operator-role-arn>
              └── inline policy with all required AWS permissions
```

The Terraform module wires this up automatically.

## Helm Chart Reference

| Value | Default | Description |
|---|---|---|
| `image.repository` | — | Container image repository |
| `image.tag` | `latest` | Image tag |
| `image.pullPolicy` | `Always` | Image pull policy |
| `aws.region` | `us-east-1` | AWS region for the operator |
| `operatorRoleArn` | **required** | IAM role ARN for EKS Pod Identity |
| `replicaCount` | `1` | Number of controller replicas |
| `leaderElection` | `true` | Enable leader election (required for `replicaCount > 1`) |
| `serviceAccount.create` | `true` | Create the `ServiceAccount` |
| `serviceAccount.name` | `konfig-controller` | `ServiceAccount` name |
| `rbac.create` | `true` | Create `ClusterRole` and `ClusterRoleBinding` |
| `resources.limits.cpu` | `500m` | CPU limit |
| `resources.limits.memory` | `128Mi` | Memory limit |
| `resources.requests.cpu` | `10m` | CPU request |
| `resources.requests.memory` | `64Mi` | Memory request |

## Terraform Module Reference

```hcl
module "konfig_konector" {
  source = "./terraform"

  account_id   = "123456789012"
  cluster_name = "prod-eks"   # your EKS cluster
  environment  = "prod"       # or "dev"
  region       = "us-east-1"

  # Optional
  operator_namespace         = "konfig-system"
  operator_service_account   = "konfig-controller"
  allowed_iam_resource_paths = "/konfig/"
}
```

### Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `account_id` | yes | — | AWS account ID |
| `cluster_name` | yes | — | EKS cluster the operator runs on |
| `environment` | yes | — | `prod` or `dev` |
| `region` | no | `us-east-1` | AWS region |
| `operator_namespace` | no | `konfig-system` | Kubernetes namespace |
| `operator_service_account` | no | `konfig-controller` | ServiceAccount name |
| `allowed_iam_resource_paths` | no | `/konfig/` | IAM path prefix the operator may manage |

### Outputs

| Output | Description |
|---|---|
| `operator_role_arn` | ARN to pass to `--set operatorRoleArn` |
| `operator_role_name` | IAM role name |
| `cluster_name` | EKS cluster name used |
| `pod_identity_association_id` | EKS Pod Identity association ID |
| `helm_install_command` | Pre-filled `helm install` command |

## Building from Source

```bash
git clone https://github.com/konfig-io/konfig-konector.git
cd konfig-konector

# Install code generation tools and regenerate
make generate manifests

# Build the binary
make build

# Run tests
make test

# Build and push the Docker image
make docker-build docker-push IMG=your-registry/konfig-konector:latest

# Install CRDs into the active cluster context
make install

# Run the controller locally (uses your kubeconfig + AWS environment credentials)
make run
```

### Adding a new resource type

1. Create `api/v1alpha1/<kind>_types.go` with `+kubebuilder:object:root=true` marker
2. Run `make generate manifests` to regenerate deepcopy methods and CRD YAML
3. Copy the new CRD YAML to `helm/konfig-konector/crds/`
4. Create `internal/controller/<kind>_controller.go`
5. Register the controller in `cmd/main.go`
6. Add the resource to the `ClusterRole` in `helm/konfig-konector/templates/clusterrole.yaml`

## Security Considerations

**Principle of least privilege:** The operator's IAM role is scoped to the `allowed_iam_resource_paths` prefix. Set this to the narrowest path that covers your use case.

**Secret handling:** Database passwords and other sensitive values are read from Kubernetes Secrets at reconcile time and never written to CR status or operator logs.

**Non-root container:** The operator runs as UID `65532` on a read-only root filesystem with no privilege escalation.

**Finalizers:** Every managed resource carries the finalizer `aws.konfig.io/finalizer`. Deleting a CR triggers AWS resource cleanup before the finalizer is removed. Deleting a CR does not orphan the underlying AWS resource.

**Namespace scoping:** CRs are namespaced. Restrict which teams can create them using standard Kubernetes RBAC on the `aws.konfig.io` API group.

## Contributing

Contributions are welcome — see [CONTRIBUTING.md](CONTRIBUTING.md) for the
development setup, the checklist for adding a new resource kind, and the PR
checklist. This project follows a [Code of Conduct](CODE_OF_CONDUCT.md);
security issues go through [SECURITY.md](SECURITY.md), not public issues.

```bash
make test      # unit tests
make lint      # golangci-lint
make test-e2e  # end-to-end tests against a local Kind cluster
```

Run `make help` for a full list of available targets.

## License

Copyright 2026 Andy Baxter.

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for details.
