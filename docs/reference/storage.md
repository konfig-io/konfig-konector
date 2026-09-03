# Storage Reference

## S3Bucket

Creates and manages an S3 bucket with versioning, encryption, lifecycle policies, CORS, notifications, website hosting, object lock, and access controls.

**Status: ✅ Working**

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `bucketName` | `string` | ✅ | Globally unique bucket name. |
| `region` | `string` | | AWS region for the bucket. Defaults to the operator region. |
| `versioning` | `bool` | | Enable bucket versioning. Default: `false`. |
| `serverSideEncryption.sseAlgorithm` | `string` | | SSE algorithm: `AES256` or `aws:kms`. |
| `serverSideEncryption.kmsKeyId` | `string` | | KMS key ID or ARN (only for `aws:kms`). |
| `blockPublicAccess.blockPublicAcls` | `bool` | | Block new public ACLs and uploads. |
| `blockPublicAccess.blockPublicPolicy` | `bool` | | Block new public bucket policies. |
| `blockPublicAccess.ignorePublicAcls` | `bool` | | Ignore all public ACLs on the bucket. |
| `blockPublicAccess.restrictPublicBuckets` | `bool` | | Restrict public and cross-account access. |
| `lifecycleRules` | `[]S3LifecycleRule` | | Object lifecycle rules. |
| `lifecycleRules[].id` | `string` | | Unique rule ID. |
| `lifecycleRules[].status` | `string` | | `Enabled` or `Disabled`. |
| `lifecycleRules[].prefix` | `string` | | Object key prefix filter. |
| `lifecycleRules[].expirationDays` | `int32` | | Days after which current objects expire. |
| `lifecycleRules[].expirationDate` | `string` | | ISO 8601 date on which current objects expire. |
| `lifecycleRules[].noncurrentVersionExpirationDays` | `int32` | | Days after which noncurrent versions expire. |
| `lifecycleRules[].transitions` | `[]S3LifecycleTransition` | | Storage class transitions for current versions. |
| `lifecycleRules[].transitions[].days` | `int32` | | Days until transition. |
| `lifecycleRules[].transitions[].date` | `string` | | ISO 8601 date for transition. |
| `lifecycleRules[].transitions[].storageClass` | `string` | | Target class: `STANDARD_IA`, `GLACIER`, `DEEP_ARCHIVE`, etc. |
| `lifecycleRules[].noncurrentVersionTransitions` | `[]S3NoncurrentVersionTransition` | | Transitions for noncurrent versions. |
| `lifecycleRules[].abortIncompleteMultipartUploadDays` | `int32` | | Days to abort incomplete multipart uploads. |
| `corsRules` | `[]S3CORSRule` | | CORS rules for the bucket. |
| `corsRules[].id` | `string` | | Rule ID. |
| `corsRules[].allowedHeaders` | `[]string` | | Allowed request headers. |
| `corsRules[].allowedMethods` | `[]string` | | Allowed HTTP methods (e.g. `GET`, `PUT`). |
| `corsRules[].allowedOrigins` | `[]string` | | Allowed request origins. |
| `corsRules[].exposeHeaders` | `[]string` | | Headers exposed in responses. |
| `corsRules[].maxAgeSeconds` | `int32` | | Preflight response cache time in seconds. |
| `notificationConfig.lambdaFunctionConfigurations` | `[]S3LambdaNotification` | | Lambda function event notifications. |
| `notificationConfig.queueConfigurations` | `[]S3QueueNotification` | | SQS queue event notifications. |
| `notificationConfig.topicConfigurations` | `[]S3TopicNotification` | | SNS topic event notifications. |
| `notificationConfig.eventBridgeEnabled` | `bool` | | Send all events to EventBridge. |
| `websiteConfig.indexDocument` | `string` | | Index document (e.g. `index.html`). |
| `websiteConfig.errorDocument` | `string` | | Error document (e.g. `error.html`). |
| `websiteConfig.redirectAllTo.hostName` | `string` | | Redirect all requests to this hostname. |
| `websiteConfig.redirectAllTo.protocol` | `string` | | Redirect protocol: `http` or `https`. |
| `websiteConfig.routingRules` | `[]S3RoutingRule` | | Conditional routing rules. |
| `accelerateStatus` | `string` | | Transfer acceleration: `Enabled` or `Suspended`. |
| `objectLockConfig.objectLockEnabled` | `bool` | | Enable object lock (immutable, set at creation). |
| `objectLockConfig.rule.defaultRetention.mode` | `string` | | Retention mode: `GOVERNANCE` or `COMPLIANCE`. |
| `objectLockConfig.rule.defaultRetention.days` | `int32` | | Default retention period in days. |
| `objectLockConfig.rule.defaultRetention.years` | `int32` | | Default retention period in years. |
| `loggingConfig.targetBucket` | `string` | | Destination bucket for access logs. |
| `loggingConfig.targetPrefix` | `string` | | Key prefix for log objects. |
| `tags` | `map[string]string` | | AWS resource tags. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `arn` | `string` | ARN of the S3 bucket (e.g. `arn:aws:s3:::my-bucket`). |
| `domainName` | `string` | Bucket domain name (e.g. `my-bucket.s3.amazonaws.com`). |
| `websiteEndpoint` | `string` | Static website endpoint URL (set when `websiteConfig` is configured). |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example — basic bucket with encryption

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: S3Bucket
metadata:
  name: my-app-assets
  namespace: platform
spec:
  bucketName: my-app-assets-prod-123456789012
  region: us-east-1
  versioning: true
  serverSideEncryption:
    sseAlgorithm: AES256
  tags:
    env: prod
    team: platform
```

### Example — KMS-encrypted bucket

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: S3Bucket
metadata:
  name: my-secure-bucket
  namespace: platform
spec:
  bucketName: my-secure-data-prod
  region: us-east-1
  versioning: true
  serverSideEncryption:
    sseAlgorithm: aws:kms
    kmsKeyId: arn:aws:kms:us-east-1:123456789012:key/mrk-abc123
  tags:
    env: prod
    classification: sensitive
```

### Example — full-featured bucket

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: S3Bucket
metadata:
  name: my-data-bucket
  namespace: platform
spec:
  bucketName: my-data-prod-123456789012
  region: us-east-1
  versioning: true
  serverSideEncryption:
    sseAlgorithm: aws:kms
    kmsKeyId: arn:aws:kms:us-east-1:123456789012:key/mrk-abc123
  blockPublicAccess:
    blockPublicAcls: true
    blockPublicPolicy: true
    ignorePublicAcls: true
    restrictPublicBuckets: true
  lifecycleRules:
    - id: expire-old-versions
      status: Enabled
      noncurrentVersionExpirationDays: 30
    - id: transition-to-glacier
      status: Enabled
      prefix: archive/
      transitions:
        - days: 90
          storageClass: GLACIER
      expirationDays: 365
    - id: abort-multipart
      status: Enabled
      abortIncompleteMultipartUploadDays: 7
  notificationConfig:
    lambdaFunctionConfigurations:
      - id: process-uploads
        lambdaFunctionARN: arn:aws:lambda:us-east-1:123456789012:function:my-processor
        events:
          - s3:ObjectCreated:*
        filterPrefix: uploads/
    eventBridgeEnabled: true
  loggingConfig:
    targetBucket: my-access-logs-bucket
    targetPrefix: my-data-prod/
  tags:
    env: prod
    team: platform
```

### Example — static website

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: S3Bucket
metadata:
  name: my-website
  namespace: platform
spec:
  bucketName: my-website-prod
  region: us-east-1
  websiteConfig:
    indexDocument: index.html
    errorDocument: 404.html
  corsRules:
    - allowedMethods:
        - GET
      allowedOrigins:
        - "*"
      maxAgeSeconds: 3600
  tags:
    env: prod
```

### Notes

- Bucket names must be globally unique across all AWS accounts.
- For buckets in regions other than `us-east-1`, the `region` field must be set explicitly. The controller passes the appropriate `CreateBucketConfiguration` to the S3 API.
- All sub-resource configurations (lifecycle, CORS, notifications, website, etc.) are applied idempotently on every reconcile — changes take effect immediately.
- Object lock (`objectLockConfig.objectLockEnabled`) must be enabled at bucket creation time and cannot be disabled afterwards.
- Drift detection covers: versioning status, encryption configuration, and tags.

### Deletion

**Fails if the bucket is not empty.** AWS rejects `DeleteBucket` with `BucketNotEmpty` if the bucket contains any objects or versioned object markers. The CR stays in `Deleting` state. To unblock:
- Empty the bucket manually: `aws s3 rm s3://my-bucket --recursive`
- For versioned buckets, also delete all versions: `aws s3api delete-objects ...`
- Or configure an S3 lifecycle rule to expire objects automatically before deleting the CR

The controller does **not** automatically empty the bucket on deletion to prevent accidental data loss.

---

## S3BucketPolicy

Attaches a resource-based policy to an S3 bucket.

**Status: ✅ Working**

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `bucketRef.name` | `string` | ✅ (or `bucketName`) | Name of an `S3Bucket` CR in the same namespace. |
| `bucketRef.bucketName` | `string` | ✅ (or `name`) | Direct S3 bucket name, bypassing CR lookup. |
| `policyDocument` | `string` | ✅ | JSON IAM resource policy document. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example — enforce HTTPS-only access

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: S3BucketPolicy
metadata:
  name: my-app-assets-policy
  namespace: platform
spec:
  bucketRef:
    name: my-app-assets
  policyDocument: |
    {
      "Version": "2012-10-17",
      "Statement": [
        {
          "Sid": "DenyNonHTTPS",
          "Effect": "Deny",
          "Principal": "*",
          "Action": "s3:*",
          "Resource": [
            "arn:aws:s3:::my-app-assets-prod-123456789012",
            "arn:aws:s3:::my-app-assets-prod-123456789012/*"
          ],
          "Condition": {
            "Bool": { "aws:SecureTransport": "false" }
          }
        }
      ]
    }
```

### Example — allow cross-account access

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: S3BucketPolicy
metadata:
  name: cross-account-policy
  namespace: platform
spec:
  bucketRef:
    bucketName: my-shared-data-bucket   # direct bucket name reference
  policyDocument: |
    {
      "Version": "2012-10-17",
      "Statement": [
        {
          "Sid": "AllowPartnerRead",
          "Effect": "Allow",
          "Principal": {
            "AWS": "arn:aws:iam::987654321098:root"
          },
          "Action": ["s3:GetObject"],
          "Resource": "arn:aws:s3:::my-shared-data-bucket/*"
        }
      ]
    }
```

### Notes

- The controller waits for the referenced `S3Bucket` CR to be `Ready` before applying the policy.
- The bucket ARNs in the `policyDocument` must be specified manually — they are not automatically substituted from status.
- Policy changes are applied immediately via `PutBucketPolicy`.
- To remove a policy, delete the `S3BucketPolicy` CR. The controller calls `DeleteBucketPolicy` on deletion.
