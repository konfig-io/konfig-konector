# RDS Reference

## DBSubnetGroup

Creates and manages an RDS DB Subnet Group, which defines the subnets where RDS instances and clusters can be deployed.

**Status: ✅ Working**

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `dbSubnetGroupName` | `string` | ✅ | Name of the DB subnet group. |
| `description` | `string` | ✅ | Description for the subnet group. |
| `subnetRefs` | `[]SubnetRef` | ✅ | At least 2 subnets in different AZs. Use `name` for CR references or `id` for direct IDs. |
| `tags` | `map[string]string` | | AWS resource tags. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `arn` | `string` | ARN of the DB subnet group. |
| `status` | `string` | DB subnet group status (e.g. `Complete`). |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: DBSubnetGroup
metadata:
  name: my-db-subnet-group
  namespace: platform
spec:
  dbSubnetGroupName: my-db-subnet-group
  description: "Subnets for production RDS instances"
  subnetRefs:
    - name: private-subnet-1a
    - name: private-subnet-1b
    - name: private-subnet-1c
  tags:
    env: prod
```

### Notes

- RDS requires a subnet group with subnets in at least two availability zones for Multi-AZ deployments.
- The controller updates the subnet list on spec change via `ModifyDBSubnetGroup`.

### Deletion

**Fails if in use.** AWS rejects `DeleteDBSubnetGroup` if any DB instance or cluster is still using this subnet group. Delete all `DBInstance` and `DBCluster` CRs in the group first.

---

## DBParameterGroup

Creates and manages an RDS DB Parameter Group for configuring database engine settings.

**Status: ✅ Working**

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `dbParameterGroupName` | `string` | ✅ | Name of the parameter group. |
| `dbParameterGroupFamily` | `string` | ✅ | Parameter group family (e.g. `mysql8.0`, `postgres15`). |
| `description` | `string` | ✅ | Description for the parameter group. |
| `parameters` | `[]DBParameter` | | List of parameter overrides. |
| `tags` | `map[string]string` | | AWS resource tags. |

#### DBParameter Fields

| Field | Type | Description |
|-------|------|-------------|
| `parameterName` | `string` | Parameter name (e.g. `max_connections`). |
| `parameterValue` | `string` | Parameter value. |
| `applyMethod` | `string` | When to apply: `immediate` or `pending-reboot`. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `arn` | `string` | ARN of the parameter group. |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: DBParameterGroup
metadata:
  name: my-mysql-params
  namespace: platform
spec:
  dbParameterGroupName: my-mysql-params
  dbParameterGroupFamily: mysql8.0
  description: "MySQL 8.0 parameter group"
  parameters:
    - parameterName: max_connections
      parameterValue: "500"
      applyMethod: immediate
    - parameterName: slow_query_log
      parameterValue: "1"
      applyMethod: immediate
    - parameterName: long_query_time
      parameterValue: "2"
      applyMethod: immediate
  tags:
    env: prod
```

### Notes

- `dbParameterGroupFamily` must match the engine version (e.g. `mysql8.0` for MySQL 8.0, `postgres15` for PostgreSQL 15).
- Some parameters require a reboot to take effect — use `applyMethod: pending-reboot` for those.
- AWS default parameter groups cannot be modified. This resource creates a custom parameter group.

### Deletion

**Fails if in use.** AWS rejects deletion if any DB instance is currently using this parameter group. Delete or reassign the DB instances first.

---

## DBClusterParameterGroup

Creates and manages a DB Cluster Parameter Group for Aurora clusters.

**Status: ✅ Working**

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `dbClusterParameterGroupName` | `string` | ✅ | Name of the cluster parameter group. |
| `dbParameterGroupFamily` | `string` | ✅ | Parameter group family (e.g. `aurora-mysql8.0`, `aurora-postgresql15`). |
| `description` | `string` | ✅ | Description for the parameter group. |
| `parameters` | `[]DBParameter` | | List of parameter overrides. |
| `tags` | `map[string]string` | | AWS resource tags. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `arn` | `string` | ARN of the cluster parameter group. |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: DBClusterParameterGroup
metadata:
  name: my-aurora-cluster-params
  namespace: platform
spec:
  dbClusterParameterGroupName: my-aurora-cluster-params
  dbParameterGroupFamily: aurora-mysql8.0
  description: "Aurora MySQL 8.0 cluster parameter group"
  parameters:
    - parameterName: character_set_server
      parameterValue: utf8mb4
      applyMethod: pending-reboot
    - parameterName: time_zone
      parameterValue: UTC
      applyMethod: immediate
  tags:
    env: prod
```

### Notes

- Use `DBClusterParameterGroup` for parameters that apply at the cluster level. Use `DBParameterGroup` for instance-level parameters.
- Both a cluster parameter group and an instance parameter group can be associated with Aurora instances simultaneously.

### Deletion

**Fails if in use.** AWS rejects deletion if any DB cluster is using this parameter group. Delete the `DBCluster` CR first.

---

## DBInstance

Creates and manages an RDS database instance.

**Status: ✅ Working — async provisioning**

> RDS instances take several minutes to provision. The controller polls every 30 seconds until the status reaches `available`.

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `dbInstanceIdentifier` | `string` | ✅ | DB instance identifier. |
| `dbInstanceClass` | `string` | ✅ | Instance class (e.g. `db.t3.micro`, `db.r6g.large`). |
| `engine` | `string` | ✅ | Database engine: `mysql`, `postgres`, `mariadb`, `oracle-ee`, `sqlserver-se`. |
| `engineVersion` | `string` | ✅ | Engine version (e.g. `8.0`, `15.4`). |
| `masterUsername` | `string` | ✅ | Master username. Immutable after creation. |
| `masterUserPasswordRef` | `SecretRef` | ✅ | Reference to a Kubernetes Secret containing the password. |
| `dbName` | `string` | | Initial database name. |
| `allocatedStorage` | `int32` | ✅ | Storage size in GiB. |
| `storageType` | `string` | | `gp2`, `gp3`, `io1`, or `io2`. Default: `gp2`. |
| `storageEncrypted` | `bool` | | Enable storage encryption. Default: `false`. |
| `kmsKeyId` | `string` | | KMS key ID for encryption. Uses AWS-managed key if not set. |
| `multiAZ` | `bool` | | Enable Multi-AZ. Default: `false`. |
| `publiclyAccessible` | `bool` | | Allow public internet access. Default: `false`. |
| `dbSubnetGroupRef` | `string` | ✅ | Name of a `DBSubnetGroup` CR or direct subnet group name. |
| `dbParameterGroupRef` | `string` | | Name of a `DBParameterGroup` CR or direct parameter group name. |
| `vpcSecurityGroupRefs` | `[]SecurityGroupRef` | | Security groups for the DB instance. |
| `port` | `int32` | | Database port. Defaults to engine default. |
| `backupRetentionPeriod` | `int32` | | Backup retention in days (0–35). `0` disables automated backups. |
| `preferredBackupWindow` | `string` | | Daily time range for automated backups (e.g. `02:00-03:00`). |
| `preferredMaintenanceWindow` | `string` | | Weekly maintenance window (e.g. `sun:05:00-sun:06:00`). |
| `iops` | `int32` | | Provisioned IOPS for `io1`/`io2` storage types. |
| `storageThroughput` | `int32` | | Storage throughput in MiB/s for `gp3` storage. |
| `maxAllocatedStorage` | `int32` | | Upper limit for autoscaling storage (GiB). Enables storage autoscaling when set. |
| `autoMinorVersionUpgrade` | `bool` | | Automatically apply minor engine version upgrades. Default: `true`. |
| `copyTagsToSnapshot` | `bool` | | Copy instance tags to automated snapshots. Default: `false`. |
| `enableIAMDatabaseAuthentication` | `bool` | | Enable IAM database authentication. |
| `enablePerformanceInsights` | `bool` | | Enable Performance Insights. |
| `performanceInsightsKmsKeyId` | `string` | | KMS key for Performance Insights data encryption. |
| `performanceInsightsRetentionPeriod` | `int32` | | Retention period in days: `7` (default, free tier) or 731 days. |
| `monitoringInterval` | `int32` | | Enhanced Monitoring interval in seconds: `0`, `1`, `5`, `10`, `15`, `30`, or `60`. `0` disables it. |
| `monitoringRoleArn` | `string` | | IAM role ARN for Enhanced Monitoring to publish to CloudWatch. |
| `enabledCloudwatchLogsExports` | `[]string` | | Log types to export to CloudWatch (e.g. `["error", "slowquery", "general"]` for MySQL; `["postgresql", "upgrade"]` for PostgreSQL). |
| `deletionProtection` | `bool` | | Prevent accidental deletion. Default: `false`. |
| `skipFinalSnapshot` | `bool` | | Skip final snapshot on deletion. Default: `false`. |
| `tags` | `map[string]string` | | AWS resource tags. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `dbInstanceArn` | `string` | ARN of the DB instance. |
| `endpoint` | `string` | DB instance endpoint hostname. |
| `port` | `int32` | DB port. |
| `dbInstanceStatus` | `string` | RDS instance status: `creating`, `available`, `modifying`, `deleting`. |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example — MySQL

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-db-password
  namespace: platform
type: Opaque
stringData:
  password: "my-secure-password"
---
apiVersion: aws.konfig.io/v1alpha1
kind: DBInstance
metadata:
  name: my-mysql-db
  namespace: platform
spec:
  dbInstanceIdentifier: my-mysql-db
  dbInstanceClass: db.t3.medium
  engine: mysql
  engineVersion: "8.0"
  masterUsername: admin
  masterUserPasswordRef:
    name: my-db-password
    key: password
  dbName: myapp
  allocatedStorage: 100
  storageType: gp3
  storageEncrypted: true
  multiAZ: true
  publiclyAccessible: false
  dbSubnetGroupRef: my-db-subnet-group
  vpcSecurityGroupRefs:
    - name: db-sg
  backupRetentionPeriod: 7
  deletionProtection: true
  skipFinalSnapshot: false
  tags:
    env: prod
    team: platform
```

### Example — full-featured PostgreSQL

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: DBInstance
metadata:
  name: my-postgres-full
  namespace: platform
spec:
  dbInstanceIdentifier: my-postgres-full
  dbInstanceClass: db.r6g.large
  engine: postgres
  engineVersion: "15.4"
  masterUsername: pgadmin
  masterUserPasswordRef:
    name: postgres-db-password
    key: password
  allocatedStorage: 200
  maxAllocatedStorage: 1000
  storageType: gp3
  storageThroughput: 500
  storageEncrypted: true
  kmsKeyId: arn:aws:kms:us-east-1:123456789012:key/mrk-abc123
  multiAZ: true
  publiclyAccessible: false
  dbSubnetGroupRef: my-db-subnet-group
  dbParameterGroupRef: my-postgres-params
  vpcSecurityGroupRefs:
    - name: db-sg
  backupRetentionPeriod: 14
  preferredBackupWindow: "02:00-03:00"
  preferredMaintenanceWindow: "sun:05:00-sun:06:00"
  autoMinorVersionUpgrade: false
  copyTagsToSnapshot: true
  enableIAMDatabaseAuthentication: true
  enablePerformanceInsights: true
  performanceInsightsRetentionPeriod: 7
  monitoringInterval: 60
  monitoringRoleArn: arn:aws:iam::123456789012:role/rds-enhanced-monitoring
  enabledCloudwatchLogsExports:
    - postgresql
    - upgrade
  deletionProtection: true
  skipFinalSnapshot: false
  tags:
    env: prod
    team: platform
```

### Example — PostgreSQL

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: DBInstance
metadata:
  name: my-postgres-db
  namespace: platform
spec:
  dbInstanceIdentifier: my-postgres-db
  dbInstanceClass: db.r6g.large
  engine: postgres
  engineVersion: "15.4"
  masterUsername: pgadmin
  masterUserPasswordRef:
    name: postgres-db-password
    key: password
  allocatedStorage: 200
  storageType: gp3
  storageEncrypted: true
  multiAZ: true
  publiclyAccessible: false
  dbSubnetGroupRef: my-db-subnet-group
  dbParameterGroupRef: my-postgres-params
  vpcSecurityGroupRefs:
    - name: db-sg
  backupRetentionPeriod: 14
  deletionProtection: true
  skipFinalSnapshot: false
  tags:
    env: prod
```

### Notes

- The master password is read from the referenced Kubernetes Secret at reconcile time. It is **never** written to CR status or operator logs.
- **Password rotation is not supported.** The `masterUserPasswordRef` secret is only read at instance creation. Changing the referenced secret value or swapping the secret reference after creation has no effect — the controller does not call `ModifyDBInstance` to rotate passwords. To rotate the master password, use [AWS Secrets Manager automatic rotation](https://docs.aws.amazon.com/secretsmanager/latest/userguide/rotating-secrets-rds.html) or run `aws rds modify-db-instance --master-user-password <new>` manually.
- Spec updates (instance class, storage, Multi-AZ) are gated on `metadata.generation` — the controller only calls `ModifyDBInstance` when the spec actually changes, not on every reconcile. This prevents unnecessary instance modifications.
- While `dbInstanceStatus` is `creating` or `modifying`, the controller skips updates and requeues.
- `skipFinalSnapshot: false` (the default) means AWS takes a final snapshot before deletion. Set to `true` for ephemeral dev instances to skip this.
- `deletionProtection: true` prevents accidental deletion from both the CR and the AWS console.

### Deletion

**May take several minutes — final snapshot is taken by default.** When `skipFinalSnapshot: false` (the default), AWS creates a final snapshot before the instance enters `deleting` state. This adds minutes to the process. The controller does not poll for deletion completion — it calls `DeleteDBInstance` and removes the finalizer once the call succeeds. If `deletionProtection: true` is set on the instance in AWS (not the CR spec), the delete call will fail; disable it first via the CR spec before deleting.

---

## DBCluster

Creates and manages an Aurora DB cluster (MySQL-compatible or PostgreSQL-compatible).

**Status: ✅ Working — async provisioning**

> Aurora clusters take several minutes to provision. The controller polls every 30 seconds.

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `dbClusterIdentifier` | `string` | ✅ | DB cluster identifier. |
| `engine` | `string` | ✅ | `aurora-mysql` or `aurora-postgresql`. |
| `engineVersion` | `string` | ✅ | Engine version (e.g. `8.0.mysql_aurora.3.05.1`). |
| `masterUsername` | `string` | ✅ | Master username. |
| `masterUserPasswordRef` | `SecretRef` | ✅ | Reference to a Kubernetes Secret containing the password. |
| `dbSubnetGroupRef` | `string` | ✅ | Name of a `DBSubnetGroup` CR or direct name. |
| `dbClusterParameterGroupRef` | `string` | | Name of a `DBClusterParameterGroup` CR or direct name. |
| `vpcSecurityGroupRefs` | `[]SecurityGroupRef` | | Security groups for the cluster. |
| `backupRetentionPeriod` | `int32` | | Backup retention in days (1–35). |
| `preferredBackupWindow` | `string` | | Daily time range for automated backups (e.g. `02:00-03:00`). |
| `preferredMaintenanceWindow` | `string` | | Weekly maintenance window (e.g. `sun:05:00-sun:06:00`). |
| `storageEncrypted` | `bool` | | Enable storage encryption. |
| `kmsKeyId` | `string` | | KMS key ID for encryption. |
| `storageType` | `string` | | Aurora storage type (e.g. `aurora`, `aurora-iopt1`). |
| `allocatedStorage` | `int32` | | Storage size in GiB (Aurora I/O-Optimized only). |
| `port` | `int32` | | Database port override. |
| `engineMode` | `string` | | `provisioned` (default) or `serverless`. Immutable after creation. |
| `serverlessV2ScalingConfig.minCapacity` | `float64` | | Minimum Aurora Serverless v2 ACUs (e.g. `0.5`). |
| `serverlessV2ScalingConfig.maxCapacity` | `float64` | | Maximum Aurora Serverless v2 ACUs (e.g. `128`). |
| `backtrackWindow` | `int64` | | Backtrack window in seconds (Aurora MySQL only, max 259200). |
| `networkType` | `string` | | `IPV4` or `DUAL` (dual-stack IPv4+IPv6). |
| `enableHttpEndpoint` | `bool` | | Enable the RDS Data API HTTP endpoint (Aurora Serverless). |
| `enableIAMDatabaseAuthentication` | `bool` | | Enable IAM database authentication. |
| `copyTagsToSnapshot` | `bool` | | Copy cluster tags to automated snapshots. |
| `autoMinorVersionUpgrade` | `bool` | | Automatically apply minor engine version upgrades. |
| `enabledCloudwatchLogsExports` | `[]string` | | Log types to export to CloudWatch (e.g. `["audit", "error", "slowquery"]`). |
| `performanceInsightsEnabled` | `bool` | | Enable Performance Insights for cluster instances. |
| `performanceInsightsKmsKeyId` | `string` | | KMS key for Performance Insights data encryption. |
| `performanceInsightsRetentionPeriod` | `int32` | | Performance Insights retention in days. |
| `deletionProtection` | `bool` | | Prevent accidental deletion. |
| `skipFinalSnapshot` | `bool` | | Skip final snapshot on deletion. |
| `tags` | `map[string]string` | | AWS resource tags. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `dbClusterArn` | `string` | ARN of the DB cluster. |
| `endpoint` | `string` | Cluster writer endpoint. |
| `readerEndpoint` | `string` | Cluster reader endpoint. |
| `status` | `string` | Cluster status: `creating`, `available`, `modifying`, `deleting`. |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: DBCluster
metadata:
  name: my-aurora-cluster
  namespace: platform
spec:
  dbClusterIdentifier: my-aurora-cluster
  engine: aurora-mysql
  engineVersion: "8.0.mysql_aurora.3.05.1"
  masterUsername: admin
  masterUserPasswordRef:
    name: aurora-db-password
    key: password
  dbSubnetGroupRef: my-db-subnet-group
  dbClusterParameterGroupRef: my-aurora-cluster-params
  vpcSecurityGroupRefs:
    - name: db-sg
  backupRetentionPeriod: 7
  storageEncrypted: true
  deletionProtection: true
  skipFinalSnapshot: false
  tags:
    env: prod
```

### Example — Aurora Serverless v2

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: DBCluster
metadata:
  name: my-serverless-cluster
  namespace: platform
spec:
  dbClusterIdentifier: my-serverless-cluster
  engine: aurora-postgresql
  engineVersion: "15.4"
  masterUsername: pgadmin
  masterUserPasswordRef:
    name: aurora-db-password
    key: password
  dbSubnetGroupRef: my-db-subnet-group
  vpcSecurityGroupRefs:
    - name: db-sg
  serverlessV2ScalingConfig:
    minCapacity: 0.5
    maxCapacity: 32
  enableHttpEndpoint: true
  enableIAMDatabaseAuthentication: true
  backupRetentionPeriod: 7
  preferredBackupWindow: "02:00-03:00"
  storageEncrypted: true
  enabledCloudwatchLogsExports:
    - postgresql
  deletionProtection: true
  skipFinalSnapshot: true
  tags:
    env: dev
```

### Notes

- `DBCluster` provisions the Aurora cluster only. Aurora reader/writer instances must be provisioned separately using `DBInstance` with the cluster's `dbClusterIdentifier`.
- The `readerEndpoint` automatically load-balances read traffic across Aurora reader instances.
- Spec changes to `DBCluster` are generation-gated (same as `DBInstance`).
- `engineMode` is immutable after creation — you cannot switch between `provisioned` and `serverless` on an existing cluster.
- `enabledCloudwatchLogsExports` changes are applied as a delta (Enable/Disable sets) — the controller does not replace all exports on every reconcile.

### Deletion

**Fails if the cluster has instances.** AWS rejects `DeleteDBCluster` if any DB instances are still part of the cluster. Delete all member `DBInstance` CRs first. Once they are gone, deleting the `DBCluster` CR takes a final snapshot (unless `skipFinalSnapshot: true`) and then removes the cluster.
