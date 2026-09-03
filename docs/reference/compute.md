# Compute Reference

## KeyPair

Creates and manages an EC2 key pair for SSH access to instances.

**Status: ✅ Working**

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `keyName` | `string` | ✅ | Name of the key pair. |
| `keyType` | `string` | | Key type: `rsa` or `ed25519`. Default: `rsa`. |
| `tags` | `map[string]string` | | AWS resource tags. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `keyPairId` | `string` | AWS Key Pair ID (e.g. `key-0abc1234def`). |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: KeyPair
metadata:
  name: my-key-pair
  namespace: platform
spec:
  keyName: my-ec2-key
  keyType: ed25519
  tags:
    env: dev
```

### Notes

- The private key is **not** stored in CR status or operator logs. AWS only returns the private key material at creation time. If you need to retrieve the private key, use AWS Systems Manager Parameter Store or store it in a Kubernetes Secret at creation time using external tooling.
- To replace a key pair, delete the CR and recreate it with a new name.
- `keyType` is immutable after creation.

### Deletion

**Immediate.** Deleting the key pair does not affect running instances that were launched with it. SSH sessions already established remain active; new SSH connections using this key will fail after deletion.

---

## LaunchTemplate

Creates and manages an EC2 Launch Template. On spec change, the controller creates a new template version rather than modifying the existing one.

**Status: ✅ Working**

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `launchTemplateName` | `string` | ✅ | Name of the launch template. |
| `imageId` | `string` | ✅ | EC2 AMI ID (e.g. `ami-0abcdef1234567890`). |
| `instanceType` | `string` | ✅ | EC2 instance type (e.g. `t3.medium`). |
| `keyName` | `string` | | EC2 key pair name. |
| `securityGroupRefs` | `[]SecurityGroupRef` | | Security groups to attach. Use `name` for CR references or `id` for direct IDs. |
| `userData` | `string` | | Base64-encoded user data script. |
| `iamInstanceProfile` | `string` | | IAM instance profile name or ARN. |
| `blockDeviceMappings` | `[]BlockDeviceMapping` | | EBS volume configuration. |
| `tags` | `map[string]string` | | AWS resource tags. |

#### BlockDeviceMapping Fields

| Field | Type | Description |
|-------|------|-------------|
| `deviceName` | `string` | Device name (e.g. `/dev/xvda`). |
| `volumeSize` | `int32` | Volume size in GiB. |
| `volumeType` | `string` | EBS volume type: `gp2`, `gp3`, `io1`, `io2`, `st1`, `sc1`. |
| `encrypted` | `bool` | Whether to encrypt the volume. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `launchTemplateId` | `string` | AWS Launch Template ID (e.g. `lt-0abc1234def`). |
| `latestVersionNumber` | `int64` | The latest template version number. |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: LaunchTemplate
metadata:
  name: web-lt
  namespace: platform
spec:
  launchTemplateName: web-launch-template
  imageId: ami-0abcdef1234567890
  instanceType: t3.medium
  keyName: my-ec2-key
  securityGroupRefs:
    - name: web-sg
  iamInstanceProfile: my-ec2-instance-profile
  userData: IyEvYmluL2Jhc2gKZWNobyAiSGVsbG8gV29ybGQi  # base64: #!/bin/bash\necho "Hello World"
  blockDeviceMappings:
    - deviceName: /dev/xvda
      volumeSize: 30
      volumeType: gp3
      encrypted: true
  tags:
    env: prod
    team: platform
```

### Notes

- On spec change, the controller calls `CreateLaunchTemplateVersion` to create a new version. The `latestVersionNumber` in status is updated. Old versions are not automatically deleted.
- Launch template names are immutable. To rename, delete and recreate.
- Reference the `launchTemplateId` from status in `AutoScalingGroup.spec.launchTemplateRef.id` for stable references that survive version updates.

---

## AutoScalingGroup

Creates and manages an EC2 Auto Scaling Group.

**Status: ✅ Working**

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `autoScalingGroupName` | `string` | ✅ | Name of the Auto Scaling Group. |
| `launchTemplateRef.name` | `string` | | Name of a `LaunchTemplate` CR in the same namespace. |
| `launchTemplateRef.id` | `string` | | Direct Launch Template ID. |
| `launchTemplateRef.version` | `string` | | Template version: `$Latest`, `$Default`, or a specific number. Default: `$Latest`. |
| `minSize` | `int32` | ✅ | Minimum number of instances. |
| `maxSize` | `int32` | ✅ | Maximum number of instances. |
| `desiredCapacity` | `int32` | ✅ | Desired number of instances. |
| `vpcZoneIdentifier` | `[]SubnetRef` | ✅ | Subnets to launch instances in. |
| `targetGroupArns` | `[]string` | | ARNs of ALB/NLB target groups to register instances with. |
| `tags` | `map[string]string` | | AWS resource tags applied to the ASG and optionally to instances. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: AutoScalingGroup
metadata:
  name: web-asg
  namespace: platform
spec:
  autoScalingGroupName: web-asg
  launchTemplateRef:
    name: web-lt
    version: "$Latest"
  minSize: 2
  maxSize: 10
  desiredCapacity: 3
  vpcZoneIdentifier:
    - name: private-subnet-1a
    - name: private-subnet-1b
  targetGroupArns:
    - arn:aws:elasticloadbalancing:us-east-1:123456789012:targetgroup/web-tg/abc123
  tags:
    env: prod
    team: platform
```

### Notes

- The controller calls `UpdateAutoScalingGroup` to reconcile `minSize`, `maxSize`, `desiredCapacity`, and subnet configuration.
- When `launchTemplateRef.version` changes, the controller triggers a rolling instance refresh via `StartInstanceRefresh`.
- `desiredCapacity` will be overridden by scaling policies if any are configured directly in AWS. The controller does not manage scaling policies.

### Deletion

**Immediate — all instances are force-terminated.** The controller calls `DeleteAutoScalingGroup` with `ForceDelete: true`, which terminates all running instances immediately without waiting for scale-in hooks or connection draining. If you need graceful draining (e.g. for instances behind an ALB), scale the group down to zero (`desiredCapacity: 0`) before deleting the CR.

---

## EC2Instance

Creates and manages an individual EC2 instance.

**Status: ✅ Working — async provisioning**

> EC2 instances take 30–120 seconds to reach `running` state. The controller polls every 15 seconds.

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `imageId` | `string` | ✅ | AMI ID (e.g. `ami-0abcdef1234567890`). |
| `instanceType` | `string` | ✅ | EC2 instance type (e.g. `t3.micro`). |
| `keyName` | `string` | | EC2 key pair name for SSH access. |
| `subnetRef.name` | `string` | | Name of a `Subnet` CR. |
| `subnetRef.id` | `string` | | Direct Subnet ID. |
| `securityGroupRefs` | `[]SecurityGroupRef` | | Security groups to attach. |
| `iamInstanceProfile` | `string` | | IAM instance profile name. |
| `userData` | `string` | | Base64-encoded user data script. |
| `associatePublicIpAddress` | `bool` | | Assign a public IP. Default: `false`. |
| `tags` | `map[string]string` | | AWS resource tags applied to the instance. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `instanceId` | `string` | AWS Instance ID (e.g. `i-0abc1234def`). |
| `privateIp` | `string` | Private IPv4 address. |
| `publicIp` | `string` | Public IPv4 address (if assigned). |
| `state` | `string` | Instance state: `pending`, `running`, `stopping`, `stopped`, `terminated`. |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: EC2Instance
metadata:
  name: bastion
  namespace: platform
spec:
  imageId: ami-0abcdef1234567890
  instanceType: t3.micro
  keyName: my-ec2-key
  subnetRef:
    name: public-subnet-1a
  securityGroupRefs:
    - name: bastion-sg
  associatePublicIpAddress: true
  tags:
    role: bastion
    env: prod
---
apiVersion: aws.konfig.io/v1alpha1
kind: EC2Instance
metadata:
  name: worker
  namespace: platform
spec:
  imageId: ami-0abcdef1234567890
  instanceType: t3.large
  subnetRef:
    name: private-subnet-1b
  securityGroupRefs:
    - name: web-sg
  iamInstanceProfile: my-worker-instance-profile
  userData: IyEvYmluL2Jhc2gKYXB0LWdldCB1cGRhdGUgLXkK    # base64-encoded bootstrap script
  tags:
    role: worker
    env: prod
```

### Notes

- `EC2Instance` is intended for long-lived standalone instances (bastion hosts, single-node services). For fleets of instances, use `AutoScalingGroup`.
- The controller tags the instance with the provided `tags` on creation and syncs tag drift on each reconcile.
- If the instance enters `terminated` or `shutting-down` state (e.g. terminated manually in AWS), the controller clears the `instanceId` from status and re-creates the instance on the next reconcile. To prevent recreation, delete the CR first.
- `imageId` changes are not applied to running instances. To change the AMI, delete and recreate the CR.
- `userData` is base64-encoded by the caller. The controller passes it directly to the EC2 `RunInstances` API.

### Deletion

**Immediate — terminates the instance.** The controller calls `TerminateInstances` and removes the finalizer once accepted. The instance transitions through `shutting-down` → `terminated` asynchronously in AWS, but the CR is gone immediately. Any EBS volumes attached with `DeleteOnTermination: true` (the default for the root volume) are deleted with the instance. Additional volumes with `DeleteOnTermination: false` persist in AWS and must be cleaned up manually.
