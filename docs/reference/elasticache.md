# ElastiCache Reference

## ElastiCacheSubnetGroup

Creates and manages an ElastiCache subnet group.

**Status: ✅ Working**

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `subnetGroupName` | `string` | ✅ | Name of the subnet group. |
| `description` | `string` | ✅ | Description for the subnet group. |
| `subnetRefs` | `[]SubnetRef` | ✅ | Subnets for the cache cluster. At least 1 required; use multiple AZs for HA. |
| `tags` | `map[string]string` | | AWS resource tags. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `arn` | `string` | ARN of the ElastiCache subnet group. |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: ElastiCacheSubnetGroup
metadata:
  name: my-cache-subnet-group
  namespace: platform
spec:
  subnetGroupName: my-cache-subnet-group
  description: "Subnets for ElastiCache clusters"
  subnetRefs:
    - name: private-subnet-1a
    - name: private-subnet-1b
  tags:
    env: prod
```

### Notes

- For multi-AZ ElastiCache replication groups with `automaticFailover: true`, provide subnets in at least two availability zones.
- The controller uses `DescribeCacheSubnetGroups` (not `DescribeSubnetGroups`) for existence checks.
- Subnet list changes are applied via `ModifyCacheSubnetGroup`.

### Deletion

**Fails if in use.** AWS rejects `DeleteCacheSubnetGroup` if any replication group is still using this subnet group. Delete the `ElastiCacheReplicationGroup` CR first.

---

## ElastiCacheReplicationGroup

Creates and manages an ElastiCache replication group (Redis or Valkey) with optional cluster mode, encryption, and automatic failover.

**Status: ✅ Working — async provisioning**

> ElastiCache replication groups take several minutes to provision. The controller polls every 30 seconds until status reaches `available`.

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `replicationGroupId` | `string` | ✅ | Replication group ID. 1–40 alphanumeric characters and hyphens. |
| `description` | `string` | ✅ | Description of the replication group. |
| `engine` | `string` | ✅ | Cache engine: `redis` or `valkey`. |
| `engineVersion` | `string` | ✅ | Engine version (e.g. `7.1`, `7.2` for Valkey). |
| `cacheNodeType` | `string` | ✅ | Node type (e.g. `cache.t3.micro`, `cache.r7g.large`). |
| `numCacheClusters` | `int32` | | Number of cache clusters in the replication group (1–6). Default: 1. |
| `automaticFailover` | `bool` | | Enable automatic failover. Requires `numCacheClusters >= 2`. Default: `false`. |
| `subnetGroupRef` | `string` | ✅ | Name of an `ElastiCacheSubnetGroup` CR or direct subnet group name. |
| `securityGroupRefs` | `[]SecurityGroupRef` | | Security groups for the replication group. |
| `atRestEncryption` | `bool` | | Enable encryption at rest. Default: `false`. |
| `transitEncryption` | `bool` | | Enable in-transit encryption (TLS). Default: `false`. |
| `authToken` | `string` | | AUTH token for Redis `AUTH` command. Required when `transitEncryption: true`. |
| `snapshotRetentionLimit` | `int32` | | Days to retain automatic snapshots (0–35). `0` disables. Default: 0. |
| `tags` | `map[string]string` | | AWS resource tags. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `arn` | `string` | ARN of the replication group. |
| `primaryEndpoint` | `string` | Primary node endpoint for write operations. |
| `readerEndpoint` | `string` | Reader endpoint for load-balanced reads. |
| `status` | `string` | Replication group status: `creating`, `available`, `modifying`, `deleting`. |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example — single-node Redis (dev)

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: ElastiCacheReplicationGroup
metadata:
  name: my-cache-dev
  namespace: platform
spec:
  replicationGroupId: my-cache-dev
  description: "Redis cache for development"
  engine: redis
  engineVersion: "7.1"
  cacheNodeType: cache.t3.micro
  numCacheClusters: 1
  subnetGroupRef: my-cache-subnet-group
  securityGroupRefs:
    - name: cache-sg
  tags:
    env: dev
```

### Example — highly available Redis (production)

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: ElastiCacheReplicationGroup
metadata:
  name: my-cache-prod
  namespace: platform
spec:
  replicationGroupId: my-cache-prod
  description: "Redis cache for production"
  engine: redis
  engineVersion: "7.1"
  cacheNodeType: cache.r7g.large
  numCacheClusters: 3           # 1 primary + 2 replicas
  automaticFailover: true
  subnetGroupRef: my-cache-subnet-group
  securityGroupRefs:
    - name: cache-sg
  atRestEncryption: true
  transitEncryption: true
  authToken: "my-strong-auth-token-32chars-min"
  snapshotRetentionLimit: 7
  tags:
    env: prod
    team: platform
```

### Example — Valkey (Redis-compatible open source)

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: ElastiCacheReplicationGroup
metadata:
  name: my-valkey-cache
  namespace: platform
spec:
  replicationGroupId: my-valkey-cache
  description: "Valkey cache"
  engine: valkey
  engineVersion: "7.2"
  cacheNodeType: cache.t3.medium
  numCacheClusters: 2
  automaticFailover: true
  subnetGroupRef: my-cache-subnet-group
  securityGroupRefs:
    - name: cache-sg
  atRestEncryption: true
  tags:
    env: prod
```

### Notes

- Spec changes (node type, engine version, cluster count) are generation-gated — the controller only calls `ModifyReplicationGroup` when `metadata.generation` changes. This prevents unnecessary modifications that could cause brief downtime.
- `automaticFailover: true` requires `numCacheClusters >= 2`. Enabling failover on a single-node group will fail with an AWS validation error.
- `authToken` is sent to the ElastiCache API and is not stored in status. Rotate it by updating the CR spec.
- Tags are managed via `AddTagsToResource`/`RemoveTagsFromResource` using the replication group ARN.
- The `readerEndpoint` distributes read traffic across replica nodes. Applications should use the `readerEndpoint` for reads and `primaryEndpoint` for writes only.
- `replicationGroupId` is immutable after creation.

### Deletion

**Immediate — no final snapshot.** The controller calls `DeleteReplicationGroup` and removes the finalizer once the call succeeds. AWS begins deleting the cluster nodes asynchronously, but the CR finalizer is removed immediately. All cached data is permanently lost. If you need a backup, take a manual snapshot in the AWS console before deleting the CR.
