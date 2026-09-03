# EKS Reference

All resources use API group `aws.konfig.io/v1alpha1`.

---

## EKSCluster

Manages an EKS control plane. Creation takes 10–15 minutes; the controller polls every 30 seconds until the cluster is `ACTIVE`.

### Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `clusterName` | string | ✅ | EKS cluster name. Immutable after creation. Max 100 chars. |
| `version` | string | | Kubernetes version (e.g. `"1.30"`). Leave empty for latest. |
| `roleArn` | string | ✅* | IAM role ARN for the EKS control plane. |
| `roleRef.name` | string | ✅* | IAMRole CR name in same namespace. |
| `resourcesVpcConfig.subnetRefs` | []SubnetRef | ✅ | At least 2 subnets in different AZs. |
| `resourcesVpcConfig.securityGroupRefs` | []SecurityGroupRef | | Additional control plane security groups. |
| `resourcesVpcConfig.endpointPublicAccess` | bool | | Enable public API endpoint. Default: `true`. |
| `resourcesVpcConfig.endpointPrivateAccess` | bool | | Enable private API endpoint. Default: `false`. |
| `resourcesVpcConfig.publicAccessCidrs` | []string | | CIDRs allowed to reach the public endpoint. |
| `logging.enabledTypes` | []string | | Log types to enable: `api`, `audit`, `authenticator`, `controllerManager`, `scheduler`. |
| `encryptionConfig.providerKeyArn` | string | | KMS key ARN for encrypting Kubernetes secrets. |
| `encryptionConfig.resources` | []string | | Resources to encrypt (e.g. `["secrets"]`). |
| `accessConfig.authenticationMode` | string | | `API`, `API_AND_CONFIG_MAP`, or `CONFIG_MAP`. |
| `accessConfig.bootstrapClusterCreatorAdminPermissions` | bool | | Grant cluster creator admin access entry. |
| `kubernetesNetworkConfig.serviceIpv4Cidr` | string | | IPv4 CIDR block for Kubernetes service IPs. Immutable after creation. |
| `kubernetesNetworkConfig.ipFamily` | string | | IP family: `ipv4` or `ipv6`. Immutable after creation. |
| `upgradePolicy.supportType` | string | | Cluster support tier: `STANDARD` or `EXTENDED`. |
| `bootstrapSelfManagedAddons` | bool | | Install self-managed add-ons (kube-proxy, CoreDNS, vpc-cni) at cluster creation. Default: `true`. |
| `tags` | map[string]string | | AWS resource tags. |

*Either `roleArn` or `roleRef` must be set.

### Status

| Field | Description |
|-------|-------------|
| `clusterArn` | Full ARN of the cluster. |
| `endpoint` | Kubernetes API server URL. |
| `version` | Current Kubernetes version. |
| `status` | Cluster status: `CREATING`, `ACTIVE`, `UPDATING`, `DELETING`, `FAILED`. |
| `certificateAuthority` | Base64-encoded CA data for kubeconfig. |
| `oidcIssuer` | OIDC issuer URL (used for IRSA). |
| `conditions` | Kubernetes conditions. `Ready: True` when `ACTIVE`. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: EKSCluster
metadata:
  name: my-cluster
  namespace: konfig-system
spec:
  clusterName: my-cluster
  version: "1.30"
  roleRef:
    name: eks-cluster-role
  resourcesVpcConfig:
    subnetRefs:
      - name: private-subnet-a
      - name: private-subnet-b
    endpointPublicAccess: true
    endpointPrivateAccess: true
  accessConfig:
    authenticationMode: API
  tags:
    env: production
```

### Example — full-featured cluster

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: EKSCluster
metadata:
  name: my-cluster
  namespace: konfig-system
spec:
  clusterName: my-cluster
  version: "1.30"
  roleRef:
    name: eks-cluster-role
  resourcesVpcConfig:
    subnetRefs:
      - name: private-subnet-a
      - name: private-subnet-b
    endpointPublicAccess: true
    endpointPrivateAccess: true
  accessConfig:
    authenticationMode: API
  kubernetesNetworkConfig:
    serviceIpv4Cidr: "172.20.0.0/16"
    ipFamily: ipv4
  upgradePolicy:
    supportType: EXTENDED
  logging:
    enabledTypes:
      - api
      - audit
  tags:
    env: production
```

### Deletion

The controller calls `DeleteCluster`. EKS will reject deletion if the cluster has node groups, Fargate profiles, or addons still provisioned — delete those first. Deletion takes 10–15 minutes.

---

## EKSNodeGroup

Manages a managed node group. Creation takes 3–5 minutes; the controller polls every 30 seconds.

### Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `clusterName` | string | ✅* | EKS cluster name. |
| `clusterRef.name` | string | ✅* | EKSCluster CR in same namespace. Waits for cluster to be ACTIVE. |
| `nodegroupName` | string | ✅ | Node group name. Immutable after creation. Max 63 chars. |
| `nodeRoleArn` | string | ✅* | IAM role ARN for node instances. |
| `nodeRoleRef.name` | string | ✅* | IAMRole CR name in same namespace. |
| `subnetRefs` | []SubnetRef | ✅ | Subnets to launch nodes in. |
| `scalingConfig.minSize` | int32 | ✅ | Minimum node count. |
| `scalingConfig.maxSize` | int32 | ✅ | Maximum node count. |
| `scalingConfig.desiredSize` | int32 | ✅ | Desired node count. |
| `instanceTypes` | []string | | EC2 instance types. Default: `["t3.medium"]`. |
| `amiType` | string | | `AL2_x86_64`, `AL2_ARM_64`, `AL2023_x86_64_STANDARD`, `BOTTLEROCKET_x86_64`, etc. |
| `capacityType` | string | | `ON_DEMAND` or `SPOT`. |
| `diskSize` | int32 | | Root EBS volume size in GiB. |
| `labels` | map[string]string | | Kubernetes node labels. |
| `taints[].key` | string | | Taint key. |
| `taints[].value` | string | | Taint value. |
| `taints[].effect` | string | | `NO_SCHEDULE`, `NO_EXECUTE`, or `PREFER_NO_SCHEDULE`. |
| `updateConfig.maxUnavailable` | int32 | | Max nodes unavailable during update. |
| `updateConfig.maxUnavailablePercentage` | int32 | | Max % nodes unavailable during update. |
| `launchTemplate.name` | string | | LaunchTemplate CR name (same namespace). |
| `launchTemplate.id` | string | | Direct AWS launch template ID. |
| `launchTemplate.version` | string | | Launch template version. Default: `$Latest`. |
| `releaseVersion` | string | | AMI release version. Leave empty for latest. |
| `version` | string | | Kubernetes version override. Defaults to cluster version. |
| `remoteAccess.ec2SshKey` | string | | EC2 key pair name for SSH access. |
| `remoteAccess.sourceSecurityGroupRefs` | []SecurityGroupRef | | Restrict SSH to these security groups. |
| `nodeRepairConfig.enabled` | bool | | Enable automatic node repair (replaces unhealthy nodes). |
| `tags` | map[string]string | | AWS resource tags. |

*Either `clusterName`/`clusterRef` and `nodeRoleArn`/`nodeRoleRef` must be set.

### Status

| Field | Description |
|-------|-------------|
| `nodegroupArn` | Full ARN of the node group. |
| `status` | `CREATING`, `ACTIVE`, `UPDATING`, `DELETING`, `DEGRADED`. |
| `conditions` | Kubernetes conditions. `Ready: True` when `ACTIVE`. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: EKSNodeGroup
metadata:
  name: workers
  namespace: konfig-system
spec:
  clusterRef:
    name: my-cluster
  nodegroupName: workers
  nodeRoleRef:
    name: eks-node-role
  subnetRefs:
    - name: private-subnet-a
    - name: private-subnet-b
  scalingConfig:
    minSize: 1
    maxSize: 10
    desiredSize: 3
  instanceTypes: ["m5.large"]
  amiType: AL2023_x86_64_STANDARD
  capacityType: ON_DEMAND
  tags:
    env: production
```

### Deletion

The controller calls `DeleteNodegroup`. EKS drains all pods and terminates the underlying EC2 instances (3–5 minutes). The cluster must be in `ACTIVE` state for deletion to proceed.

---

## EKSAddon

Manages an EKS add-on (e.g. `vpc-cni`, `kube-proxy`, `coredns`, `aws-ebs-csi-driver`).

### Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `clusterName` | string | ✅* | EKS cluster name. |
| `clusterRef.name` | string | ✅* | EKSCluster CR in same namespace. |
| `addonName` | string | ✅ | Add-on name (e.g. `vpc-cni`). |
| `addonVersion` | string | | Specific version. Leave empty for latest recommended. |
| `serviceAccountRoleArn` | string | | IAM role ARN for IRSA. |
| `serviceAccountRoleRef.name` | string | | IAMRole CR name in same namespace. |
| `resolveConflicts` | string | | `OVERWRITE`, `NONE`, or `PRESERVE`. |
| `configurationValues` | string | | JSON or YAML configuration blob for the add-on. |
| `tags` | map[string]string | | AWS resource tags. |

### Status

| Field | Description |
|-------|-------------|
| `addonArn` | Full ARN of the add-on. |
| `addonVersion` | Currently installed version. |
| `status` | `CREATING`, `ACTIVE`, `UPDATING`, `DEGRADED`, `DELETING`. |
| `conditions` | Kubernetes conditions. `Ready: True` when `ACTIVE`. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: EKSAddon
metadata:
  name: vpc-cni
  namespace: konfig-system
spec:
  clusterName: PlatformDev-eks
  addonName: vpc-cni
  resolveConflicts: OVERWRITE
```

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: EKSAddon
metadata:
  name: ebs-csi-driver
  namespace: konfig-system
spec:
  clusterName: PlatformDev-eks
  addonName: aws-ebs-csi-driver
  serviceAccountRoleRef:
    name: ebs-csi-role
  resolveConflicts: OVERWRITE
```

### Deletion

Calls `DeleteAddon`. Completes immediately in most cases. The running add-on pods are removed from the cluster.

---

## EKSFargateProfile

Manages a Fargate profile — defines which pods run on Fargate. Creation takes ~2 minutes; the controller polls every 15 seconds.

> **Note:** Fargate profiles are mostly immutable after creation. Selectors and subnets cannot be changed without deleting and recreating the profile.

### Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `clusterName` | string | ✅* | EKS cluster name. |
| `clusterRef.name` | string | ✅* | EKSCluster CR in same namespace. |
| `fargateProfileName` | string | ✅ | Profile name. Immutable after creation. |
| `podExecutionRoleArn` | string | ✅* | IAM role ARN for Fargate pod execution. |
| `podExecutionRoleRef.name` | string | ✅* | IAMRole CR name in same namespace. |
| `subnetRefs` | []SubnetRef | | Private subnets to run Fargate pods in. |
| `selectors[].namespace` | string | ✅ | Kubernetes namespace to match. |
| `selectors[].labels` | map[string]string | | Pod labels to match. Empty = all pods in namespace. |
| `tags` | map[string]string | | AWS resource tags. |

### Status

| Field | Description |
|-------|-------------|
| `fargateProfileArn` | Full ARN of the profile. |
| `status` | `CREATING`, `ACTIVE`, `DELETING`, `CREATE_FAILED`, `DELETE_FAILED`. |
| `conditions` | Kubernetes conditions. `Ready: True` when `ACTIVE`. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: EKSFargateProfile
metadata:
  name: kube-system-fargate
  namespace: konfig-system
spec:
  clusterRef:
    name: my-cluster
  fargateProfileName: kube-system
  podExecutionRoleRef:
    name: fargate-pod-execution-role
  subnetRefs:
    - name: private-subnet-a
    - name: private-subnet-b
  selectors:
    - namespace: kube-system
    - namespace: default
      labels:
        fargate: "true"
```

### Deletion

Calls `DeleteFargateProfile`. Takes ~2 minutes. If deletion fails (`DELETE_FAILED`), the CR stays in `Ready: False` until the issue is resolved manually in the AWS console.

---

## EKSAccessEntry

Manages an EKS access entry — grants an IAM principal (role or user) access to the cluster using the new API-based authentication mode. Also manages the associated access policy bindings.

> **Prerequisite:** The cluster must have `authenticationMode: API` or `API_AND_CONFIG_MAP` in its access config.

### Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `clusterName` | string | ✅* | EKS cluster name. |
| `clusterRef.name` | string | ✅* | EKSCluster CR in same namespace. |
| `principalArn` | string | ✅* | IAM role or user ARN. |
| `principalRef.name` | string | ✅* | IAMRole CR name in same namespace. |
| `type` | string | | `STANDARD`, `FARGATE_LINUX`, `EC2_LINUX`, `EC2_WINDOWS`. Default: `STANDARD`. |
| `kubernetesGroups` | []string | | Kubernetes RBAC groups to bind to this principal. |
| `username` | string | | Kubernetes username override. |
| `accessPolicies[].policyArn` | string | | EKS access policy ARN. |
| `accessPolicies[].accessScope.type` | string | | `cluster` or `namespace`. |
| `accessPolicies[].accessScope.namespaces` | []string | | Namespaces when scope type is `namespace`. |
| `tags` | map[string]string | | AWS resource tags. |

**Common policy ARNs:**

| Policy | ARN |
|--------|-----|
| Cluster Admin | `arn:aws:eks::aws:cluster-access-policy/AmazonEKSClusterAdminPolicy` |
| Admin | `arn:aws:eks::aws:cluster-access-policy/AmazonEKSAdminPolicy` |
| Edit | `arn:aws:eks::aws:cluster-access-policy/AmazonEKSEditPolicy` |
| View | `arn:aws:eks::aws:cluster-access-policy/AmazonEKSViewPolicy` |

### Status

| Field | Description |
|-------|-------------|
| `accessEntryArn` | Full ARN of the access entry. |
| `conditions` | Kubernetes conditions. `Ready: True` when synced. |

### Example

```yaml
# Grant a role cluster-admin access
apiVersion: aws.konfig.io/v1alpha1
kind: EKSAccessEntry
metadata:
  name: platform-admin
  namespace: konfig-system
spec:
  clusterName: PlatformDev-eks
  principalArn: arn:aws:iam::123456789012:role/PlatformDev-AdminRole
  accessPolicies:
    - policyArn: arn:aws:eks::aws:cluster-access-policy/AmazonEKSClusterAdminPolicy
      accessScope:
        type: cluster
```

```yaml
# Grant a CI role namespace-scoped edit access
apiVersion: aws.konfig.io/v1alpha1
kind: EKSAccessEntry
metadata:
  name: ci-role
  namespace: konfig-system
spec:
  clusterName: PlatformDev-eks
  principalRef:
    name: ci-iam-role
  accessPolicies:
    - policyArn: arn:aws:eks::aws:cluster-access-policy/AmazonEKSEditPolicy
      accessScope:
        type: namespace
        namespaces: ["staging", "preview"]
```

### Deletion

Calls `DeleteAccessEntry`. All associated policy bindings are automatically removed by EKS. Completes immediately.
