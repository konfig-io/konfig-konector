# Lambda & ECS Reference

API group: `aws.konfig.io/v1alpha1`

---

## LambdaFunction

Manages an AWS Lambda function. Asynchronous — polls every 5s until `State=Active`.

### Spec

| Field | Type | Description |
|---|---|---|
| `functionName` | string | Lambda function name. Immutable after creation. |
| `roleArn` | string | IAM execution role ARN. |
| `roleRef` | RoleRef | Reference to an IAMRole CR (alternative to `roleArn`). |
| `code.imageUri` | string | Container image URI (for Image package type). |
| `code.s3.s3Bucket` | string | S3 bucket with deployment package. |
| `code.s3.s3Key` | string | S3 key for deployment package. |
| `code.s3.s3ObjectVersion` | string | S3 object version. |
| `runtime` | string | Runtime identifier, e.g. `nodejs20.x`, `python3.12`. |
| `handler` | string | Handler entrypoint, e.g. `index.handler`. |
| `architecture` | string | `x86_64` or `arm64`. Default: `x86_64`. |
| `description` | string | Function description. |
| `timeout` | int32 | Timeout in seconds (1–900). |
| `memorySize` | int32 | Memory in MB (128–10240). |
| `ephemeralStorageSize` | int32 | /tmp size in MB (512–10240). |
| `environment` | map[string]string | Environment variables. |
| `vpcConfig.subnetRefs` | []SubnetRef | VPC subnets for the function. |
| `vpcConfig.securityGroupRefs` | []SecurityGroupRef | VPC security groups. |
| `tags` | map[string]string | AWS resource tags. |
| `layers` | []string | List of Lambda layer ARNs to attach (max 5). |
| `deadLetterConfig.targetARN` | string | SQS queue or SNS topic ARN for failed async invocations. |
| `tracingConfig.mode` | string | X-Ray tracing mode: `PassThrough` or `Active`. |
| `loggingConfig.logFormat` | string | CloudWatch log format: `Text` or `JSON`. |
| `loggingConfig.logGroup` | string | CloudWatch log group name. Defaults to `/aws/lambda/{name}`. |
| `loggingConfig.systemLogLevel` | string | System log level: `DEBUG`, `INFO`, or `WARN`. |
| `loggingConfig.applicationLogLevel` | string | App log level: `TRACE`, `DEBUG`, `INFO`, `WARN`, `ERROR`, or `FATAL`. |
| `reservedConcurrency` | int32 | Reserved concurrency limit. `0` = throttle all invocations. `-1` = remove reservation. Omit to leave unmanaged. |
| `fileSystemConfigs[].arn` | string | ARN of an EFS access point to mount. |
| `fileSystemConfigs[].localMountPath` | string | Mount path inside the function (must start with `/mnt/`). |
| `snapStart.applyOn` | string | SnapStart setting (Java runtimes only): `PublishedVersions` or `None`. |
| `imageConfig.command` | []string | Override the CMD in the container image. |
| `imageConfig.entryPoint` | []string | Override the ENTRYPOINT in the container image. |
| `imageConfig.workingDirectory` | string | Override the working directory in the container image. |

### Status

| Field | Description |
|---|---|
| `functionArn` | ARN of the Lambda function. |
| `state` | Function state: `Pending`, `Active`, `Inactive`, `Failed`. |
| `codeSha256` | SHA256 of the deployment package. |
| `conditions` | Standard Ready condition. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: LambdaFunction
metadata:
  name: my-processor
  namespace: konfig-system
spec:
  functionName: my-processor
  roleRef:
    name: my-lambda-role
  code:
    s3:
      s3Bucket: my-artifacts
      s3Key: functions/processor.zip
  runtime: python3.12
  handler: handler.main
  memorySize: 256
  timeout: 30
  environment:
    LOG_LEVEL: INFO
```

### Example — full-featured zip function

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: LambdaFunction
metadata:
  name: my-processor-full
  namespace: konfig-system
spec:
  functionName: my-processor-full
  roleRef:
    name: my-lambda-role
  code:
    s3:
      s3Bucket: my-artifacts
      s3Key: functions/processor.zip
  runtime: python3.12
  handler: handler.main
  architecture: arm64
  memorySize: 512
  timeout: 60
  ephemeralStorageSize: 1024
  environment:
    LOG_LEVEL: INFO
    ENV: production
  vpcConfig:
    subnetRefs:
      - name: private-subnet-a
      - name: private-subnet-b
    securityGroupRefs:
      - name: lambda-sg
  layers:
    - arn:aws:lambda:us-east-1:017000801446:layer:AWSLambdaPowertoolsPythonV2:51
  deadLetterConfig:
    targetARN: arn:aws:sqs:us-east-1:123456789012:my-dlq
  tracingConfig:
    mode: Active
  loggingConfig:
    logFormat: JSON
    logGroup: /platform/my-processor
    systemLogLevel: WARN
    applicationLogLevel: INFO
  reservedConcurrency: 100
  fileSystemConfigs:
    - arn: arn:aws:elasticfilesystem:us-east-1:123456789012:access-point/fsap-abc123
      localMountPath: /mnt/data
  tags:
    env: prod
    team: platform
```

### Example — container image function

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: LambdaFunction
metadata:
  name: my-image-function
  namespace: konfig-system
spec:
  functionName: my-image-function
  roleRef:
    name: my-lambda-role
  code:
    imageUri: 123456789012.dkr.ecr.us-east-1.amazonaws.com/my-function:latest
  memorySize: 1024
  timeout: 300
  imageConfig:
    command: ["handler.main"]
    workingDirectory: /app
  tracingConfig:
    mode: Active
  tags:
    env: prod
```

### Example — Java function with SnapStart

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: LambdaFunction
metadata:
  name: my-java-function
  namespace: konfig-system
spec:
  functionName: my-java-function
  roleRef:
    name: my-lambda-role
  code:
    s3:
      s3Bucket: my-artifacts
      s3Key: functions/app.jar
  runtime: java21
  handler: com.example.Handler::handleRequest
  memorySize: 512
  snapStart:
    applyOn: PublishedVersions
  tags:
    env: prod
```

---

## LambdaEventSourceMapping

Connects a Lambda function to an event source (SQS, Kinesis, DynamoDB Streams, MSK, etc.).

### Spec

| Field | Type | Description |
|---|---|---|
| `functionArn` | string | Lambda function name or ARN. |
| `functionRef` | LambdaFunctionRef | Reference to a LambdaFunction CR. |
| `eventSourceArn` | string | ARN of the event source (SQS queue, Kinesis stream, etc.). |
| `batchSize` | int32 | Max records per invocation. |
| `enabled` | bool | Whether the mapping is enabled. |
| `startingPosition` | string | `TRIM_HORIZON`, `LATEST`, `AT_TIMESTAMP`. |
| `maximumBatchingWindowInSeconds` | int32 | Batching window in seconds. |
| `filterCriteria.filters` | []string | JSON filter patterns. |

### Status

| Field | Description |
|---|---|
| `uuid` | UUID of the event source mapping. |
| `state` | Mapping state: `Creating`, `Enabled`, `Disabling`, `Disabled`, etc. |
| `conditions` | Standard Ready condition. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: LambdaEventSourceMapping
metadata:
  name: my-sqs-trigger
  namespace: konfig-system
spec:
  functionRef:
    name: my-processor
  eventSourceArn: arn:aws:sqs:us-east-1:123456789012:my-queue
  batchSize: 10
  enabled: true
```

---

## LambdaPermission

Grants an AWS service or account permission to invoke a Lambda function. Idempotent — `ResourceConflictException` is treated as success.

### Spec

| Field | Type | Description |
|---|---|---|
| `functionArn` | string | Lambda function name or ARN. |
| `functionRef` | LambdaFunctionRef | Reference to a LambdaFunction CR. |
| `statementId` | string | Unique statement identifier. |
| `action` | string | Lambda action, e.g. `lambda:InvokeFunction`. |
| `principal` | string | AWS service principal, e.g. `apigateway.amazonaws.com`. |
| `sourceArn` | string | ARN of the invoking resource (for scoping permissions). |
| `sourceAccount` | string | Source AWS account ID. |

### Status

| Field | Description |
|---|---|
| `statementExists` | Whether the statement is in the function policy. |
| `conditions` | Standard Ready condition. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: LambdaPermission
metadata:
  name: allow-sqs
  namespace: konfig-system
spec:
  functionRef:
    name: my-processor
  statementId: allow-sqs-invoke
  action: lambda:InvokeFunction
  principal: sqs.amazonaws.com
  sourceArn: arn:aws:sqs:us-east-1:123456789012:my-queue
```

---

## ECSCluster

Manages an ECS cluster.

### Spec

| Field | Type | Description |
|---|---|---|
| `clusterName` | string | ECS cluster name. |
| `capacityProviders` | []string | Capacity providers, e.g. `["FARGATE", "FARGATE_SPOT"]`. |
| `containerInsights` | bool | Enable CloudWatch Container Insights. |
| `tags` | map[string]string | AWS resource tags. |

### Status

| Field | Description |
|---|---|
| `clusterArn` | ARN of the ECS cluster. |
| `status` | Cluster status: `PROVISIONING`, `ACTIVE`, `INACTIVE`. |
| `conditions` | Standard Ready condition. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: ECSCluster
metadata:
  name: my-cluster
  namespace: konfig-system
spec:
  clusterName: my-cluster
  capacityProviders:
    - FARGATE
    - FARGATE_SPOT
  containerInsights: true
```

---

## ECSTaskDefinition

Manages an ECS task definition. Registers a new revision only when the spec changes (detected via SHA256 hash).

### Spec

| Field | Type | Description |
|---|---|---|
| `family` | string | Task definition family name. |
| `cpu` | string | CPU units (e.g. `"256"`, `"1024"`). |
| `memory` | string | Memory in MiB (e.g. `"512"`, `"2048"`). |
| `networkMode` | string | `awsvpc` (default), `bridge`, `host`, `none`. |
| `executionRoleArn` | string | ECS execution role ARN. |
| `executionRoleRef` | RoleRef | Reference to an IAMRole CR. |
| `taskRoleArn` | string | Task IAM role ARN. |
| `taskRoleRef` | RoleRef | Reference to an IAMRole CR. |
| `containerDefinitions` | []ECSContainerDefinition | Container definitions. |
| `tags` | map[string]string | AWS resource tags. |

#### ECSContainerDefinition

| Field | Type | Description |
|---|---|---|
| `name` | string | Container name. |
| `image` | string | Container image URI. |
| `cpu` | int32 | CPU units allocated to this container. |
| `memory` | int32 | Memory hard limit in MiB. |
| `essential` | bool | Whether the container is essential. |
| `command` | []string | Override command. |
| `environment` | map[string]string | Environment variables. |
| `logGroup` | string | CloudWatch log group name (enables awslogs driver). |
| `portMappings` | []ECSPortMapping | Port mappings. |

### Status

| Field | Description |
|---|---|
| `taskDefinitionArn` | ARN of the registered task definition. |
| `revision` | Task definition revision number. |
| `specHash` | SHA256 hash of the spec (used for drift detection). |
| `conditions` | Standard Ready condition. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: ECSTaskDefinition
metadata:
  name: my-app-task
  namespace: konfig-system
spec:
  family: my-app
  cpu: "256"
  memory: "512"
  executionRoleRef:
    name: my-ecs-execution-role
  containerDefinitions:
    - name: app
      image: 123456789012.dkr.ecr.us-east-1.amazonaws.com/my-app:latest
      essential: true
      logGroup: /ecs/my-app
      portMappings:
        - containerPort: 8080
          protocol: tcp
      environment:
        ENV: production
```

---

## ECSService

Manages an ECS service. Supports FARGATE and EC2 launch types.

### Spec

| Field | Type | Description |
|---|---|---|
| `clusterName` | string | ECS cluster name or ARN. |
| `clusterRef` | ECSClusterRef | Reference to an ECSCluster CR. |
| `serviceName` | string | ECS service name. Immutable after creation. |
| `taskDefinitionArn` | string | Task definition ARN (with revision). |
| `taskDefinitionRef` | ECSTaskDefinitionRef | Reference to an ECSTaskDefinition CR. |
| `desiredCount` | int32 | Number of task instances to run. |
| `launchType` | string | `FARGATE`, `EC2`, or `EXTERNAL`. |
| `networkConfiguration.subnetRefs` | []SubnetRef | Subnets for awsvpc tasks. |
| `networkConfiguration.securityGroupRefs` | []SecurityGroupRef | Security groups. |
| `networkConfiguration.assignPublicIp` | string | `ENABLED` or `DISABLED`. |
| `loadBalancers` | []ECSLoadBalancer | ALB/NLB target group attachments. |
| `healthCheckGracePeriodSeconds` | int32 | Grace period before health check. |
| `enableExecuteCommand` | bool | Enable ECS Exec on tasks. |
| `tags` | map[string]string | AWS resource tags. |

### Status

| Field | Description |
|---|---|
| `serviceArn` | ARN of the ECS service. |
| `status` | Service status: `ACTIVE`, `DRAINING`, `INACTIVE`. |
| `runningCount` | Number of currently running tasks. |
| `pendingCount` | Number of tasks in pending state. |
| `conditions` | Standard Ready condition. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: ECSService
metadata:
  name: my-app-service
  namespace: konfig-system
spec:
  clusterRef:
    name: my-cluster
  serviceName: my-app
  taskDefinitionRef:
    name: my-app-task
  desiredCount: 2
  launchType: FARGATE
  networkConfiguration:
    subnetRefs:
      - name: private-subnet-a
      - name: private-subnet-b
    securityGroupRefs:
      - name: app-sg
    assignPublicIp: DISABLED
  loadBalancers:
    - targetGroupArn: arn:aws:elasticloadbalancing:us-east-1:123456789012:targetgroup/my-app/abc123
      containerName: app
      containerPort: 8080
```
