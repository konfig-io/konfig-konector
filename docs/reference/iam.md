# IAM Reference

## IAMRole

Creates and manages an AWS IAM Role.

**Status: ✅ Working**

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `roleName` | `string` | ✅ | Name of the IAM role. Immutable after creation. 1–64 characters. |
| `assumeRolePolicyDocument` | `string` | ✅ | Trust policy JSON document. |
| `description` | `string` | | Human-readable description of the role. |
| `maxSessionDuration` | `int32` | | Maximum session duration in seconds. Range: 3600–43200. Default: 3600. |
| `path` | `string` | | IAM path for the role. Default: `/`. |
| `permissionsBoundary` | `string` | | ARN of a managed policy to use as a permissions boundary. |
| `tags` | `map[string]string` | | AWS resource tags. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `arn` | `string` | ARN of the created IAM role. |
| `roleId` | `string` | Unique ID of the IAM role (e.g. `AROAXXXXXXXXX`). |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example

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
    env: prod
```

### Notes

- `roleName` is immutable after the role is created in AWS. Changing it will not rename the role — it will attempt to create a new role with the new name (and fail if the old one still exists). Delete and recreate the CR to rename a role.
- Drift detection: if the trust policy or description is changed in AWS directly, the controller reverts it on the next 5-minute reconcile cycle.
- By default the operator can only manage roles under the `/konfig/` IAM path. Set `path: /konfig/` or adjust `allowed_iam_resource_paths` in the Terraform module.
- **IAM eventual consistency:** IAM role and policy changes propagate globally across AWS regions within a few seconds but are not instantaneous. If you create an `IAMRole` CR and immediately create a resource that references it (e.g., an `EKSCluster` with `roleRef`), the EKS API may briefly return `InvalidParameterException: The role ... is not retrievable`. This is handled automatically — the controller retries with exponential backoff via the `dependencyNotReady` mechanism when using CR cross-references. Direct ARN references (`roleArn:`) do not get this protection; add a brief delay or use `roleRef` instead.

### Deletion

**Immediate — with pre-flight cleanup.** Before deleting the role, the controller automatically:
1. Detaches all managed policies (equivalent to `aws iam detach-role-policy` for each)
2. Deletes all inline policies (equivalent to `aws iam delete-role-policy` for each)
3. Deletes the role

This means you do **not** need to delete `IAMPolicyAttachment` or `IAMRolePolicy` CRs before deleting an `IAMRole` CR — the controller handles cleanup. However, if you delete them out of order, those CRs will enter an error state when their cleanup calls find the role already gone (which is harmless).

---

## IAMPolicy

Creates and manages a customer-managed AWS IAM Policy.

**Status: ✅ Working**

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `policyName` | `string` | ✅ | Name of the IAM policy. 1–128 characters. |
| `policyDocument` | `string` | ✅ | JSON policy document. |
| `description` | `string` | | Human-readable description. |
| `path` | `string` | | IAM path. Default: `/`. |
| `tags` | `map[string]string` | | AWS resource tags. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `arn` | `string` | ARN of the created policy. |
| `policyId` | `string` | Unique ID of the policy. |
| `defaultVersionId` | `string` | Active policy version ID (e.g. `v3`). |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: IAMPolicy
metadata:
  name: my-app-policy
  namespace: my-namespace
spec:
  policyName: my-app-policy
  description: "Grants S3 read access to my application"
  policyDocument: |
    {
      "Version": "2012-10-17",
      "Statement": [{
        "Effect": "Allow",
        "Action": [
          "s3:GetObject",
          "s3:ListBucket"
        ],
        "Resource": [
          "arn:aws:s3:::my-bucket",
          "arn:aws:s3:::my-bucket/*"
        ]
      }]
    }
  tags:
    team: platform
```

### Notes

- When `policyDocument` changes, the controller creates a new policy version and sets it as default.
- AWS allows a maximum of 5 versions per policy. Older non-default versions are automatically deleted when this limit is reached.
- To attach an AWS managed policy (e.g. `AmazonS3ReadOnlyAccess`), use `IAMPolicyAttachment` with a direct `policyRef.arn` rather than creating an `IAMPolicy`.

### Deletion

**Fails if the policy is still attached.** AWS will reject `DeletePolicy` with `DeleteConflict` if the policy is attached to any role, user, or group. The CR stays in `Deleting` state. Delete the `IAMPolicyAttachment` CRs that reference this policy first, or detach the policy manually in AWS. Once all attachments are removed the controller retries automatically.

---

## IAMPolicyAttachment

Attaches a managed IAM policy to an IAM role.

**Status: ✅ Working**

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `roleRef.name` | `string` | ✅ (or `arn`) | Name of an `IAMRole` CR in the same namespace. |
| `roleRef.arn` | `string` | ✅ (or `name`) | Direct IAM role ARN, bypassing CR lookup. |
| `policyRef.name` | `string` | ✅ (or `arn`) | Name of an `IAMPolicy` CR in the same namespace. |
| `policyRef.arn` | `string` | ✅ (or `name`) | Direct policy ARN (use for AWS managed policies). |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `attached` | `bool` | Whether the policy is currently attached. |
| `roleArn` | `string` | ARN of the role the policy is attached to. |
| `policyArn` | `string` | ARN of the attached policy. |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example — attach a CR-managed policy

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: IAMPolicyAttachment
metadata:
  name: my-app-attachment
  namespace: my-namespace
spec:
  roleRef:
    name: my-app-role        # IAMRole CR in this namespace
  policyRef:
    name: my-app-policy      # IAMPolicy CR in this namespace
```

### Example — attach an AWS managed policy

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: IAMPolicyAttachment
metadata:
  name: readonly-attachment
  namespace: my-namespace
spec:
  roleRef:
    name: my-app-role
  policyRef:
    arn: arn:aws:iam::aws:policy/AmazonS3ReadOnlyAccess
```

### Notes

- The controller waits for both the `IAMRole` and `IAMPolicy` CRs to be `Ready` before attaching.
- Detaching is handled automatically on CR deletion.
- A role can have multiple `IAMPolicyAttachment` CRs.

---

## IAMRolePolicy

Creates an inline policy on an IAM role. Inline policies are embedded directly in the role and deleted when the role is deleted.

**Status: ✅ Working**

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `roleRef.name` | `string` | ✅ (or `arn`) | Name of an `IAMRole` CR in the same namespace. |
| `roleRef.arn` | `string` | ✅ (or `name`) | Direct IAM role ARN, bypassing CR lookup. |
| `policyName` | `string` | ✅ | Name of the inline policy within the role. 1–128 characters. |
| `policyDocument` | `string` | ✅ | JSON policy document. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `roleArn` | `string` | ARN of the role that holds this inline policy. |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: IAMRolePolicy
metadata:
  name: my-app-inline-policy
  namespace: my-namespace
spec:
  roleRef:
    name: my-app-role
  policyName: AllowSecretsRead
  policyDocument: |
    {
      "Version": "2012-10-17",
      "Statement": [{
        "Effect": "Allow",
        "Action": [
          "secretsmanager:GetSecretValue",
          "secretsmanager:DescribeSecret"
        ],
        "Resource": "arn:aws:secretsmanager:us-east-1:123456789012:secret:my-app/*"
      }]
    }
```

### Notes

- Prefer `IAMPolicy` + `IAMPolicyAttachment` for policies shared across multiple roles.
- Use `IAMRolePolicy` when the policy is specific to one role and you want it to be automatically cleaned up when the role is deleted.
- Inline policy content is overwritten on every reconcile (drift correction is always active).

---

## PodIdentityAssociation

Creates an EKS Pod Identity association that binds an IAM role to a Kubernetes `ServiceAccount`, enabling pods using that `ServiceAccount` to assume the role without long-lived credentials.

**Status: ✅ Working**

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `clusterName` | `string` | ✅ | EKS cluster name. |
| `targetNamespace` | `string` | ✅ | Kubernetes namespace of the `ServiceAccount`. |
| `serviceAccountName` | `string` | ✅ | Name of the Kubernetes `ServiceAccount`. |
| `roleRef.name` | `string` | ✅ (or `arn`) | Name of an `IAMRole` CR in the same namespace. |
| `roleRef.arn` | `string` | ✅ (or `name`) | Direct IAM role ARN. |
| `annotateServiceAccount` | `bool` | | If `true`, the controller also writes the `eks.amazonaws.com/role-arn` annotation on the `ServiceAccount`. Default: `false`. |
| `tags` | `map[string]string` | | AWS resource tags. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `associationId` | `string` | EKS Pod Identity association ID. |
| `associationArn` | `string` | ARN of the association. |
| `roleArn` | `string` | ARN of the bound IAM role. |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: IAMRole
metadata:
  name: my-app-role
  namespace: platform
spec:
  roleName: my-app-role
  assumeRolePolicyDocument: |
    {
      "Version": "2012-10-17",
      "Statement": [{
        "Effect": "Allow",
        "Principal": { "Service": "pods.eks.amazonaws.com" },
        "Action": ["sts:AssumeRole", "sts:TagSession"]
      }]
    }
---
apiVersion: aws.konfig.io/v1alpha1
kind: PodIdentityAssociation
metadata:
  name: my-app-pod-identity
  namespace: platform
spec:
  clusterName: my-eks-cluster
  targetNamespace: my-app
  serviceAccountName: my-app-sa
  roleRef:
    name: my-app-role
  annotateServiceAccount: true
  tags:
    team: platform
```

### Notes

- Requires the `eks-pod-identity-agent` addon to be installed on your cluster. The Terraform bootstrap module installs this automatically.
- `annotateServiceAccount: true` grants the controller permission to write the `eks.amazonaws.com/role-arn` annotation. This requires the operator's ClusterRole to have `patch` access on `serviceaccounts` in the target namespace. For cross-namespace service accounts, ensure RBAC allows this.
- Only one Pod Identity association can exist per (cluster, namespace, serviceAccountName) tuple. Creating a second association for the same service account will fail.
- The trust policy on the IAM role **must** allow `sts:AssumeRole` and `sts:TagSession` from principal `pods.eks.amazonaws.com`.

---

## IAMUser

Creates and manages an AWS IAM User.

**Status: ✅ Working**

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `userName` | `string` | ✅ | Name of the IAM user. Immutable after creation. 1–64 characters. |
| `path` | `string` | | IAM path for the user. Default: `/`. |
| `permissionsBoundary` | `string` | | ARN of a managed policy to use as a permissions boundary. |
| `tags` | `map[string]string` | | AWS resource tags. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `arn` | `string` | ARN of the created IAM user. |
| `userId` | `string` | Unique ID of the IAM user. |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: IAMUser
metadata:
  name: my-service-user
  namespace: platform
spec:
  userName: my-service-user
  path: /
  tags:
    team: platform
    env: prod
```

### Notes

- `userName` is immutable after creation. Delete and recreate the CR to rename.
- The operator does **not** manage access keys. Use the AWS CLI or console for access key lifecycle.
- Deletion fails if the user has attached policies, group memberships, or access keys — the controller surfaces the AWS error; clean up those resources first.

---

## IAMGroup

Creates and manages an AWS IAM Group.

**Status: ✅ Working**

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `groupName` | `string` | ✅ | Name of the IAM group. Immutable after creation. 1–128 characters. |
| `path` | `string` | | IAM path for the group. Default: `/`. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `arn` | `string` | ARN of the created IAM group. |
| `groupId` | `string` | Unique ID of the IAM group. |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: IAMGroup
metadata:
  name: platform-devs
  namespace: platform
spec:
  groupName: platform-devs
```

### Notes

- `groupName` is immutable after creation.
- Deletion fails if the group has users or attached policies — detach/remove those first (delete the `IAMGroupPolicyAttachment` and `IAMGroupMembership` CRs).

---

## IAMGroupPolicyAttachment

Attaches a managed IAM policy to an IAM group.

**Status: ✅ Working**

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `groupRef.name` | `string` | ✅ (or `groupName`) | Name of an `IAMGroup` CR in the same namespace. |
| `groupRef.groupName` | `string` | ✅ (or `name`) | Direct AWS group name, bypassing CR lookup. |
| `policyRef.name` | `string` | ✅ (or `arn`) | Name of an `IAMPolicy` CR in the same namespace. |
| `policyRef.arn` | `string` | ✅ (or `name`) | Direct policy ARN (use for AWS managed policies). |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `attached` | `bool` | Whether the policy is currently attached. |
| `groupName` | `string` | Resolved AWS group name. |
| `policyArn` | `string` | ARN of the attached policy. |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: IAMGroupPolicyAttachment
metadata:
  name: platform-devs-s3-read
  namespace: platform
spec:
  groupRef:
    name: platform-devs
  policyRef:
    arn: arn:aws:iam::aws:policy/AmazonS3ReadOnlyAccess
```

---

## IAMGroupMembership

Adds an IAM user to an IAM group.

**Status: ✅ Working**

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `groupRef.name` | `string` | ✅ (or `groupName`) | Name of an `IAMGroup` CR in the same namespace. |
| `groupRef.groupName` | `string` | ✅ (or `name`) | Direct AWS group name, bypassing CR lookup. |
| `userRef.name` | `string` | ✅ (or `userName`) | Name of an `IAMUser` CR in the same namespace. |
| `userRef.userName` | `string` | ✅ (or `name`) | Direct AWS user name, bypassing CR lookup. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `member` | `bool` | Whether the user is currently a member of the group. |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: IAMGroupMembership
metadata:
  name: alice-in-platform-devs
  namespace: platform
spec:
  groupRef:
    name: platform-devs
  userRef:
    name: alice
```

---

## IAMSAMLProvider

Creates and manages an AWS IAM SAML identity provider for federated authentication.

**Status: ✅ Working**

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | `string` | ✅ | Name of the SAML provider. Immutable — forms part of the ARN. 1–128 characters. |
| `samlMetadataDocument` | `string` | ✅ | XML metadata document from the SAML IdP. |
| `tags` | `map[string]string` | | AWS resource tags. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `arn` | `string` | ARN of the SAML provider (`arn:aws:iam::{account}:saml-provider/{name}`). |
| `validUntil` | `string` | Expiry date/time of the metadata document. |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: IAMSAMLProvider
metadata:
  name: my-saml-provider
  namespace: platform
spec:
  name: my-saml-provider
  samlMetadataDocument: |
    <?xml version="1.0"?>
    <EntityDescriptor ...>
      ...
    </EntityDescriptor>
  tags:
    team: platform
```

### Notes

- `spec.name` is immutable — it forms part of the ARN (`arn:aws:iam::{account}:saml-provider/{name}`). Delete and recreate to rename.
- On generation change, the controller updates the metadata document via `UpdateSAMLProvider`.

---

## IAMOIDCProvider

Creates and manages an AWS IAM OpenID Connect (OIDC) identity provider for workload federation.

**Status: ✅ Working**

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `url` | `string` | ✅ | OIDC issuer URL. Immutable — AWS uses this as the provider identifier. |
| `clientIDList` | `[]string` | ✅ | List of client IDs (audiences) for the provider. |
| `thumbprintList` | `[]string` | ✅ | SHA-1 thumbprints of the IdP certificate chain. |
| `tags` | `map[string]string` | | AWS resource tags. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `arn` | `string` | ARN of the OIDC provider. |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: IAMOIDCProvider
metadata:
  name: my-oidc-provider
  namespace: platform
spec:
  url: https://token.actions.githubusercontent.com
  clientIDList:
    - sts.amazonaws.com
  thumbprintList:
    - 6938fd4d98bab03faadb97b34396831e3780aea1
  tags:
    team: platform
```

### Notes

- `url` is immutable — AWS does not support updating the URL. Delete and recreate to change it.
- `thumbprintList` and `clientIDList` are mutable. On generation change, the controller applies delta updates using dedicated AWS APIs (`UpdateOpenIDConnectProviderThumbprint`, `AddClientIDToOpenIDConnectProvider`, `RemoveClientIDFromOpenIDConnectProvider`).
- The ARN is stored in `status.arn` on creation and used for all subsequent API calls.
