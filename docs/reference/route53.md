# Route53 Reference

## HostedZone

Creates and manages a Route53 hosted zone.

**Status: ✅ Working**

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | `string` | ✅ | DNS zone name. Must end with a dot (e.g. `example.com.`). 1–255 characters. |
| `comment` | `string` | | Comment for the hosted zone. |
| `private` | `bool` | | If `true`, creates a private hosted zone associated with a VPC. Default: `false`. |
| `vpcRef.id` | `string` | ✅ if `private: true` | VPC ID to associate with the private hosted zone. |
| `vpcRef.region` | `string` | | AWS region of the VPC. Defaults to the operator region. |
| `delegationSetId` | `string` | | Reusable delegation set ID. |
| `tags` | `map[string]string` | | AWS resource tags. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `hostedZoneId` | `string` | Route53 hosted zone ID (e.g. `Z1PA6795UKMFR9`). |
| `nameServers` | `[]string` | Name servers delegated by Route53 (public zones only). |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example — public zone

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: HostedZone
metadata:
  name: myapp-zone
  namespace: platform
spec:
  name: myapp.example.com.
  comment: "Managed by konfig-konector"
  tags:
    env: prod
    team: platform
```

### Example — private zone

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: HostedZone
metadata:
  name: internal-zone
  namespace: platform
spec:
  name: internal.myapp.local.
  private: true
  vpcRef:
    id: vpc-0abc1234def
    region: us-east-1
  comment: "Internal DNS for VPC"
```

### Notes

- Route53 uses `CallerReference` for idempotent zone creation. The controller uses the CR's UID as the `CallerReference`.
- Name servers in `status.nameServers` should be added as NS records in the parent zone (e.g. in your registrar) to delegate the subdomain.

### Deletion

**Fails if the zone has non-default records.** AWS rejects `DeleteHostedZone` if the zone contains any record sets other than the default NS and SOA records. Delete all `RecordSet` CRs in the zone first. The controller surfaces the AWS error in the `Ready` condition message and retries automatically once the zone is empty.

---

## RecordSet

Creates and manages a Route53 DNS record within a hosted zone.

**Status: ✅ Working**

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `hostedZoneRef.name` | `string` | ✅ (or `id`) | Name of a `HostedZone` CR in the same namespace. |
| `hostedZoneRef.id` | `string` | ✅ (or `name`) | Direct Route53 zone ID, bypassing CR lookup. |
| `name` | `string` | ✅ | DNS record name. Must end with a dot (e.g. `api.myapp.example.com.`). |
| `type` | `string` | ✅ | Record type. One of: `A`, `AAAA`, `CNAME`, `MX`, `TXT`, `NS`, `SRV`, `CAA`, `PTR`. |
| `ttl` | `int64` | | TTL in seconds. Required unless using `alias`. |
| `records` | `[]string` | | Record values. Required unless using `alias`. |
| `alias.dnsName` | `string` | | Alias target DNS name (e.g. an ALB DNS name). |
| `alias.hostedZoneId` | `string` | | Hosted zone ID of the alias target. |
| `alias.evaluateTargetHealth` | `bool` | | Whether to evaluate health of the alias target. |
| `weight` | `int64` | | Weight for weighted routing (0–255). |
| `setIdentifier` | `string` | | Unique identifier for weighted/failover records. Required if `weight` or `failover` is set. |
| `failover` | `string` | | Failover routing role. One of: `PRIMARY`, `SECONDARY`. |
| `healthCheckRef.name` | `string` | | Name of a `HealthCheck` CR to associate with this record. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `changeId` | `string` | Route53 change ID for the last change batch. |
| `changeStatus` | `string` | Status of the last change: `PENDING` or `INSYNC`. |
| `hostedZoneId` | `string` | Resolved Route53 zone ID. |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example — A record

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: RecordSet
metadata:
  name: api-record
  namespace: platform
spec:
  hostedZoneRef:
    name: myapp-zone
  name: api.myapp.example.com.
  type: A
  ttl: 300
  records:
    - "1.2.3.4"
```

### Example — alias record (ALB)

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: RecordSet
metadata:
  name: api-alias
  namespace: platform
spec:
  hostedZoneRef:
    name: myapp-zone
  name: api.myapp.example.com.
  type: A
  alias:
    dnsName: my-alb-123456.us-east-1.elb.amazonaws.com.
    hostedZoneId: Z35SXDOTRQ7X7K
    evaluateTargetHealth: true
```

### Example — weighted routing

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: RecordSet
metadata:
  name: api-primary
  namespace: platform
spec:
  hostedZoneRef:
    name: myapp-zone
  name: api.myapp.example.com.
  type: A
  ttl: 60
  records:
    - "1.2.3.4"
  weight: 90
  setIdentifier: primary
---
apiVersion: aws.konfig.io/v1alpha1
kind: RecordSet
metadata:
  name: api-secondary
  namespace: platform
spec:
  hostedZoneRef:
    name: myapp-zone
  name: api.myapp.example.com.
  type: A
  ttl: 60
  records:
    - "5.6.7.8"
  weight: 10
  setIdentifier: secondary
```

### Example — TXT record

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: RecordSet
metadata:
  name: spf-record
  namespace: platform
spec:
  hostedZoneRef:
    name: myapp-zone
  name: myapp.example.com.
  type: TXT
  ttl: 300
  records:
    - '"v=spf1 include:_spf.google.com ~all"'
```

### Notes

- Route53 changes propagate asynchronously. The `changeStatus` field reflects the propagation state (`PENDING` → `INSYNC`). The `Ready` condition is set to `True` once the change is submitted, not once it is `INSYNC`.
- Record names must be fully-qualified (trailing dot). The controller does not add the dot automatically.
- Route53 applies throttling on bulk changes. The controller handles throttle errors with exponential backoff.

---

## HealthCheck

Creates and manages a Route53 health check.

**Status: ✅ Working**

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | `string` | ✅ | Health check type. One of: `HTTP`, `HTTPS`, `HTTP_STR_MATCH`, `HTTPS_STR_MATCH`, `TCP`, `CALCULATED`. |
| `ipAddress` | `string` | | IP address to monitor. |
| `fqdn` | `string` | | Fully qualified domain name to monitor. |
| `port` | `int32` | | Port to connect to. Default: 80 for HTTP, 443 for HTTPS, 443 for HTTPS_STR_MATCH. |
| `resourcePath` | `string` | | Path to request for HTTP/HTTPS checks (e.g. `/healthz`). |
| `searchString` | `string` | | String to search for in the response body (for `*_STR_MATCH` types). |
| `requestInterval` | `int32` | | Interval between checks: `10` or `30` seconds. Default: 30. |
| `failureThreshold` | `int32` | | Number of consecutive failures before marking unhealthy. Range: 1–10. |
| `tags` | `map[string]string` | | AWS resource tags. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `healthCheckId` | `string` | Route53 health check ID. |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example — HTTP health check

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: HealthCheck
metadata:
  name: api-healthcheck
  namespace: platform
spec:
  type: HTTPS
  fqdn: api.myapp.example.com
  port: 443
  resourcePath: /healthz
  requestInterval: 30
  failureThreshold: 3
  tags:
    env: prod
```

### Example — associate with a RecordSet

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: RecordSet
metadata:
  name: api-primary
  namespace: platform
spec:
  hostedZoneRef:
    name: myapp-zone
  name: api.myapp.example.com.
  type: A
  ttl: 60
  records:
    - "1.2.3.4"
  failover: PRIMARY
  setIdentifier: primary
  healthCheckRef:
    name: api-healthcheck
```

### Notes

- Either `ipAddress` or `fqdn` is required for non-`CALCULATED` check types.
- `requestInterval: 10` (fast health checks) costs extra in AWS — use `30` for non-critical endpoints.
- Health check ID is stored in `status.healthCheckId` and referenced by `RecordSet.spec.healthCheckRef`.
