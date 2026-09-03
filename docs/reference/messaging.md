# Messaging Reference

## SQSQueue

Creates and manages an SQS queue (standard or FIFO).

**Status: ✅ Working**

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `queueName` | `string` | ✅ | Queue name. For FIFO queues, must end in `.fifo`. |
| `fifo` | `bool` | | Create a FIFO queue. Default: `false`. |
| `visibilityTimeout` | `int32` | | Message visibility timeout in seconds (0–43200). Default: 30. |
| `messageRetentionPeriod` | `int32` | | Message retention in seconds (60–1209600). Default: 345600 (4 days). |
| `delaySeconds` | `int32` | | Delivery delay in seconds (0–900). Default: 0. |
| `receiveMessageWaitTime` | `int32` | | Long polling wait time in seconds (0–20). Default: 0. |
| `kmsKeyId` | `string` | | KMS key ID for server-side encryption. |
| `policy` | `string` | | JSON resource-based access policy. |
| `redrivePolicy.deadLetterQueueRef` | `string` | | Name of an `SQSQueue` CR to use as DLQ. |
| `redrivePolicy.maxReceiveCount` | `int32` | | Number of times a message is received before moving to DLQ. |
| `tags` | `map[string]string` | | AWS resource tags. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `queueUrl` | `string` | SQS queue URL. |
| `queueArn` | `string` | ARN of the SQS queue. |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example — standard queue with DLQ

```yaml
# Dead letter queue
apiVersion: aws.konfig.io/v1alpha1
kind: SQSQueue
metadata:
  name: my-queue-dlq
  namespace: platform
spec:
  queueName: my-queue-dlq
  messageRetentionPeriod: 1209600    # 14 days
  tags:
    env: prod
    role: dead-letter
---
# Main queue with redrive policy
apiVersion: aws.konfig.io/v1alpha1
kind: SQSQueue
metadata:
  name: my-queue
  namespace: platform
spec:
  queueName: my-queue
  visibilityTimeout: 300
  messageRetentionPeriod: 86400      # 1 day
  receiveMessageWaitTime: 20         # long polling
  redrivePolicy:
    deadLetterQueueRef: my-queue-dlq
    maxReceiveCount: 5
  tags:
    env: prod
```

### Example — FIFO queue

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: SQSQueue
metadata:
  name: my-ordered-queue
  namespace: platform
spec:
  queueName: my-ordered-queue.fifo   # must end in .fifo
  fifo: true
  visibilityTimeout: 60
  tags:
    env: prod
```

### Example — queue with resource policy

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: SQSQueue
metadata:
  name: sns-subscriber-queue
  namespace: platform
spec:
  queueName: sns-subscriber-queue
  policy: |
    {
      "Version": "2012-10-17",
      "Statement": [{
        "Sid": "AllowSNSPublish",
        "Effect": "Allow",
        "Principal": { "Service": "sns.amazonaws.com" },
        "Action": "sqs:SendMessage",
        "Resource": "arn:aws:sqs:us-east-1:123456789012:sns-subscriber-queue",
        "Condition": {
          "ArnEquals": {
            "aws:SourceArn": "arn:aws:sns:us-east-1:123456789012:my-topic"
          }
        }
      }]
    }
  tags:
    env: prod
```

### Notes

- Queue names are immutable after creation. To rename, delete and recreate the CR.
- FIFO queues have lower throughput than standard queues (3,000 msg/sec with batching vs. unlimited).
- All queue attributes are managed via `SetQueueAttributes` — the controller reconciles drift on every cycle.
- The `queueArn` in status is useful for SNS subscription endpoints and IAM policy resources.

### Deletion

**Immediate — in-flight messages are permanently lost.** SQS does not drain the queue before deletion. Any messages that have not been consumed are gone. If the queue has SNS subscriptions pointing to it, those subscriptions will start failing after the queue is deleted — delete the `SNSSubscription` CRs first to cleanly unsubscribe.

---

## SNSTopic

Creates and manages an SNS topic (standard or FIFO).

**Status: ✅ Working**

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `topicName` | `string` | ✅ | Topic name. For FIFO topics, must end in `.fifo`. |
| `fifo` | `bool` | | Create a FIFO topic. Default: `false`. |
| `contentBasedDeduplication` | `bool` | | Enable content-based deduplication (FIFO only). Default: `false`. |
| `kmsKeyId` | `string` | | KMS key ID for message encryption at rest. |
| `policy` | `string` | | JSON resource-based access policy. |
| `tags` | `map[string]string` | | AWS resource tags. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `topicArn` | `string` | ARN of the SNS topic. |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example — standard topic

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: SNSTopic
metadata:
  name: my-notifications
  namespace: platform
spec:
  topicName: my-notifications
  kmsKeyId: alias/aws/sns
  tags:
    env: prod
    team: platform
```

### Example — FIFO topic

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: SNSTopic
metadata:
  name: my-ordered-events
  namespace: platform
spec:
  topicName: my-ordered-events.fifo    # must end in .fifo
  fifo: true
  contentBasedDeduplication: true
  tags:
    env: prod
```

### Notes

- Topic names are immutable. To rename, delete and recreate.
- For fan-out patterns, create one `SNSTopic` and multiple `SNSSubscription` resources pointing to different SQS queues or Lambda functions.
- The `topicArn` in status is used as the `endpoint` in `SNSSubscription` when subscribing a non-CR endpoint.

### Deletion

**Immediate — all subscriptions are deleted with the topic.** AWS deletes all active subscriptions when the topic is deleted. `SNSSubscription` CRs that referenced this topic will enter an error state on their next reconcile (the subscription ARN no longer exists). Delete the `SNSSubscription` CRs first to clean up properly, or accept that they'll self-resolve when you delete the CRs afterward.

---

## SNSSubscription

Creates and manages an SNS subscription, connecting a topic to an endpoint.

**Status: ✅ Working**

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `topicRef.name` | `string` | ✅ (or `arn`) | Name of an `SNSTopic` CR in the same namespace. |
| `topicRef.arn` | `string` | ✅ (or `name`) | Direct SNS topic ARN. |
| `protocol` | `string` | ✅ | Subscription protocol: `sqs`, `lambda`, `https`, `http`, `email`, `email-json`, `sms`, `application`, `firehose`. |
| `endpoint` | `string` | ✅ | Endpoint to receive messages (queue ARN, Lambda ARN, URL, email address, etc.). |
| `filterPolicy` | `string` | | JSON message filter policy. |
| `rawMessageDelivery` | `bool` | | Send raw message without SNS metadata wrapper. Default: `false`. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `subscriptionArn` | `string` | ARN of the SNS subscription. |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example — SNS → SQS fan-out

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: SNSSubscription
metadata:
  name: notifications-to-queue
  namespace: platform
spec:
  topicRef:
    name: my-notifications
  protocol: sqs
  endpoint: "arn:aws:sqs:us-east-1:123456789012:my-queue"
  rawMessageDelivery: true
```

### Example — with filter policy

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: SNSSubscription
metadata:
  name: error-alerts-to-queue
  namespace: platform
spec:
  topicRef:
    name: my-notifications
  protocol: sqs
  endpoint: "arn:aws:sqs:us-east-1:123456789012:error-queue"
  filterPolicy: |
    {
      "severity": ["ERROR", "CRITICAL"]
    }
```

### Example — HTTPS endpoint

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: SNSSubscription
metadata:
  name: webhook-subscription
  namespace: platform
spec:
  topicRef:
    name: my-notifications
  protocol: https
  endpoint: "https://my-service.example.com/sns/callback"
```

### Notes

- The SQS queue referenced in `endpoint` must have a queue policy allowing `sqs:SendMessage` from the SNS topic. See the `SQSQueue` policy example above.
- Email subscriptions require manual confirmation by the recipient — the `subscriptionArn` will remain `PendingConfirmation` until confirmed.
- `rawMessageDelivery: true` is recommended for SQS subscriptions to avoid double-encoding JSON payloads.
- The controller calls `SetSubscriptionAttributes` on every reconcile to ensure `filterPolicy` and `rawMessageDelivery` are in sync.
