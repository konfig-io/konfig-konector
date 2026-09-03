# Resource Reference

All resources use the API group `aws.konfig.io/v1alpha1`.

## Status

| Symbol | Meaning |
|--------|---------|
| ✅ | Fully working in production |
| ⚠️ | Working with known limitations |
| 🔄 | Async — takes minutes to provision |

## IAM

| Kind | AWS Service | Status | Deletion |
|------|-------------|--------|----------|
| [IAMRole](iam.md#iamrole) | IAM | ✅ | Immediate — detaches all policies first |
| [IAMPolicy](iam.md#iampolicy) | IAM | ✅ | Fails if policy is still attached to any role |
| [IAMPolicyAttachment](iam.md#iampolicyattachment) | IAM | ✅ | Immediate |
| [IAMRolePolicy](iam.md#iamrolepolicy) | IAM inline policy | ✅ | Immediate |
| [PodIdentityAssociation](iam.md#podidentityassociation) | EKS Pod Identity | ✅ | Immediate |

## DNS

| Kind | AWS Service | Status | Deletion |
|------|-------------|--------|----------|
| [HostedZone](route53.md#hostedzone) | Route53 | ✅ | Fails if zone has non-NS/SOA records |
| [RecordSet](route53.md#recordset) | Route53 | ✅ | Immediate |
| [HealthCheck](route53.md#healthcheck) | Route53 | ✅ | Immediate |

## Networking

| Kind | AWS Service | Status | Deletion |
|------|-------------|--------|----------|
| [VPC](networking.md#vpc) | EC2 | ✅ | Fails if VPC has subnets, security groups, or gateways |
| [Subnet](networking.md#subnet) | EC2 | ✅ | Fails if subnet has ENIs attached |
| [InternetGateway](networking.md#internetgateway) | EC2 | ✅ | Immediate — detaches from VPC first |
| [RouteTable](networking.md#routetable) | EC2 | ✅ | Immediate — disassociates subnets first |
| [NatGateway](networking.md#natgateway) | EC2 | ✅ 🔄 | Async — polls until deleted, then releases EIP |
| [SecurityGroup](networking.md#securitygroup) | EC2 | ✅ | Fails if SG is referenced by ENIs or other SGs |
| [VPCEndpoint](networking.md#vpcendpoint) | EC2 | ✅ | Immediate |

## Compute

| Kind | AWS Service | Status | Deletion |
|------|-------------|--------|----------|
| [KeyPair](compute.md#keypair) | EC2 | ✅ | Immediate |
| [LaunchTemplate](compute.md#launchtemplate) | EC2 | ✅ | Immediate |
| [AutoScalingGroup](compute.md#autoscalinggroup) | Auto Scaling | ✅ | Immediate — force-terminates all instances |
| [EC2Instance](compute.md#ec2instance) | EC2 | ✅ 🔄 | Immediate — terminates instance |

## Database

| Kind | AWS Service | Status | Deletion |
|------|-------------|--------|----------|
| [DBSubnetGroup](rds.md#dbsubnetgroup) | RDS | ✅ | Fails if used by a DB instance or cluster |
| [DBParameterGroup](rds.md#dbparametergroup) | RDS | ✅ | Fails if used by a DB instance |
| [DBClusterParameterGroup](rds.md#dbclusterparametergroup) | RDS | ✅ | Fails if used by a DB cluster |
| [DBInstance](rds.md#dbinstance) | RDS | ✅ 🔄 | Takes final snapshot unless `skipFinalSnapshot: true` |
| [DBCluster](rds.md#dbcluster) | RDS Aurora | ✅ 🔄 | Fails if cluster has instances; takes final snapshot unless `skipFinalSnapshot: true` |

## Storage

| Kind | AWS Service | Status | Deletion |
|------|-------------|--------|----------|
| [S3Bucket](storage.md#s3bucket) | S3 | ✅ | Fails if bucket is not empty |
| [S3BucketPolicy](storage.md#s3bucketpolicy) | S3 | ✅ | Immediate |

## Messaging

| Kind | AWS Service | Status | Deletion |
|------|-------------|--------|----------|
| [SQSQueue](messaging.md#sqsqueue) | SQS | ✅ | Immediate — in-flight messages are lost |
| [SNSTopic](messaging.md#snstopic) | SNS | ✅ | Immediate — all subscriptions are deleted |
| [SNSSubscription](messaging.md#snssubscription) | SNS | ✅ | Immediate |

## Cache

| Kind | AWS Service | Status | Deletion |
|------|-------------|--------|----------|
| [ElastiCacheSubnetGroup](elasticache.md#elasticachesubnetgroup) | ElastiCache | ✅ | Fails if used by a replication group |
| [ElastiCacheReplicationGroup](elasticache.md#elasticachereplicationgroup) | ElastiCache | ✅ 🔄 | Immediate (no final snapshot) |

## EKS

| Kind | AWS Service | Status | Deletion |
|------|-------------|--------|----------|
| [EKSCluster](eks.md#ekscluster) | EKS | ✅ 🔄 | Async — deletes control plane (10–15 min) |
| [EKSNodeGroup](eks.md#eksnodegroup) | EKS | ✅ 🔄 | Async — drains and terminates nodes (3–5 min) |
| [EKSAddon](eks.md#eksaddon) | EKS | ✅ | Immediate |
| [EKSFargateProfile](eks.md#eksfargateprofile) | EKS | ✅ 🔄 | Async — takes ~2 min |
| [EKSAccessEntry](eks.md#eksaccessentry) | EKS | ✅ | Immediate — disassociates all policies first |

## Lambda

| Kind | AWS Service | Status | Deletion |
|------|-------------|--------|----------|
| [LambdaFunction](lambda-ecs.md#lambdafunction) | Lambda | ✅ 🔄 | Immediate |
| [LambdaEventSourceMapping](lambda-ecs.md#lambdaeventsourcemapping) | Lambda | ✅ | Immediate |
| [LambdaPermission](lambda-ecs.md#lambdapermission) | Lambda | ✅ | Immediate |

## ECS

| Kind | AWS Service | Status | Deletion |
|------|-------------|--------|----------|
| [ECSCluster](lambda-ecs.md#ecscluster) | ECS | ✅ | Immediate |
| [ECSTaskDefinition](lambda-ecs.md#ecstaskdefinition) | ECS | ✅ | Deregisters active revision |
| [ECSService](lambda-ecs.md#ecsservice) | ECS | ✅ | Force-deleted (drains tasks immediately) |

---

## Deletion Behavior

Every resource carries the finalizer `aws.konfig.io/finalizer`. When a CR is deleted (`kubectl delete`), the controller:

1. Sets the `Ready` condition to `Deleting`
2. Calls the AWS delete API
3. Removes the finalizer only after AWS confirms deletion

The Kubernetes object stays visible in `kubectl get` until the finalizer is removed. Most resources complete in under a second. A few require special handling:

### Immediately deleted

The AWS resource is deleted synchronously and the finalizer is removed in a single reconcile loop.

`IAMPolicyAttachment` · `IAMRolePolicy` · `PodIdentityAssociation` · `RecordSet` · `HealthCheck` · `InternetGateway` · `RouteTable` · `VPCEndpoint` · `KeyPair` · `LaunchTemplate` · `EC2Instance` · `S3BucketPolicy` · `SQSQueue` · `SNSTopic` · `SNSSubscription`

### Deleted with pre-flight cleanup

The controller performs preparatory steps before calling the delete API:

| Resource | Pre-flight steps |
|----------|-----------------|
| `IAMRole` | Detaches all managed policies, deletes all inline policies |
| `InternetGateway` | Detaches from the VPC (`DetachInternetGateway`) |
| `RouteTable` | Disassociates all subnet associations |
| `NatGateway` | Polls until `deleted` state, then releases the Elastic IP |

### Deletion can fail (requires manual action)

The controller calls the AWS delete API but AWS will reject it if dependencies are still present. The CR stays in `Deleting` state and the condition message shows the AWS error. Resolve the dependency, and the controller retries automatically.

| Resource | What blocks deletion |
|----------|---------------------|
| `IAMPolicy` | Still attached to one or more roles or users |
| `HostedZone` | Zone contains record sets other than the default NS and SOA |
| `VPC` | Has subnets, non-default security groups, route tables, or attached gateways |
| `Subnet` | Has ENIs attached (instances, RDS, ELBs, etc.) |
| `SecurityGroup` | Referenced by a network interface, instance, or another security group rule |
| `DBSubnetGroup` | In use by a DB instance or cluster |
| `DBParameterGroup` | In use by a DB instance |
| `DBClusterParameterGroup` | In use by a DB cluster |
| `DBCluster` | Has one or more DB instances still provisioned |
| `S3Bucket` | Bucket is not empty |
| `ElastiCacheSubnetGroup` | In use by a replication group |

### Deletion with final snapshot

`DBInstance` and `DBCluster` take a final snapshot before deletion unless `skipFinalSnapshot: true` is set in the spec. The snapshot is taken synchronously by AWS before the instance/cluster enters `deleting` state — this can add several minutes to the deletion process.

### Async deletion

`NatGateway`, `EKSCluster`, `EKSNodeGroup`, and `EKSFargateProfile` have async deletion. The controller calls the AWS delete API and then waits (polling) until the resource is fully removed before releasing the finalizer. While waiting, the CR shows `Ready: False` with reason `Deleting`.

- `NatGateway`: 60–90 seconds
- `EKSFargateProfile`: ~2 minutes
- `EKSNodeGroup`: 3–5 minutes (drains nodes)
- `EKSCluster`: 10–15 minutes

---

## Common Fields

Every resource shares these status fields:

| Field | Type | Description |
|-------|------|-------------|
| `status.conditions` | `[]metav1.Condition` | Standard Kubernetes conditions. The `Ready` condition is always present. |
| `status.observedGeneration` | `int64` | The `metadata.generation` last processed by the controller. |
| `status.lastSyncTime` | `metav1.Time` | Timestamp of the last successful AWS sync. |

The `Ready` condition reasons:

| Reason | Meaning |
|--------|---------|
| `Synced` | Resource is in sync with AWS. |
| `Created` | Resource was just created in AWS. |
| `Updated` | Resource was just updated in AWS. |
| `Deleting` | Deletion is in progress. |
| `Error` | An error occurred — see `message`. |
| `ReferenceNotReady` | A cross-resource reference has not been provisioned yet. Retries every 5 seconds. |

## Cross-Resource References

Resources reference each other by CR name within the same namespace. The controller waits — requeueing every 5 seconds — until the referenced resource is provisioned and its AWS ID is available.

```yaml
# Reference by CR name
vpcRef:
  name: my-vpc          # waits until VPC CR has vpcId in status

subnetRefs:
  - name: my-subnet     # waits until Subnet CR has subnetId in status

# Reference AWS resources directly (no CR lookup)
vpcRef:
  id: vpc-0abc1234def

subnetRefs:
  - id: subnet-0abc1234def
```

| Type | Fields |
|------|--------|
| `VPCResourceRef` | `name` (CR) or `id` (direct `vpc-xxx`) |
| `SubnetRef` | `name` (CR) or `id` (direct `subnet-xxx`) |
| `SecurityGroupRef` | `name` (CR) or `id` (direct `sg-xxx`) |
| `PolicyRef` | `name` (CR) or `arn` (direct policy ARN) |
| `RoleRef` | `name` (CR) or `arn` (direct role ARN) |
| `SecretRef` | `name` + `key` (Kubernetes Secret) |
| `EKSClusterRef` | `name` (EKSCluster CR) or `clusterName` (direct AWS cluster name) |
| `LambdaFunctionRef` | `name` (LambdaFunction CR) or `functionArn` (direct ARN/name) |
| `ECSClusterRef` | `name` (ECSCluster CR) or `clusterName` (direct AWS cluster name) |
| `ECSTaskDefinitionRef` | `name` (ECSTaskDefinition CR) or `taskDefinitionArn` (direct ARN) |
