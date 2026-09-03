# Networking Reference

## VPC

Creates and manages an AWS Virtual Private Cloud with optional IPv6 addressing and secondary CIDR blocks.

**Status: ✅ Working**

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `cidrBlock` | `string` | ✅ | IPv4 CIDR block (e.g. `10.0.0.0/16`). Immutable after creation. |
| `enableDnsSupport` | `bool` | | Enable DNS resolution within the VPC. Default: `true`. |
| `enableDnsHostnames` | `bool` | | Assign DNS hostnames to instances with public IPs. Default: `false`. |
| `instanceTenancy` | `string` | | Tenancy for instances launched in the VPC: `default`, `dedicated`, or `host`. Immutable after creation. Default: `default`. |
| `assignIpv6CidrBlock` | `bool` | | Request an Amazon-provided IPv6 CIDR block for the VPC. |
| `secondaryIPv4CIDRs` | `[]string` | | Additional IPv4 CIDR blocks to associate with the VPC (e.g. `["100.64.0.0/16"]`). Reconciled on every sync — blocks in the list are added, blocks no longer in the list are removed. |
| `tags` | `map[string]string` | | AWS resource tags. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `vpcId` | `string` | AWS VPC ID (e.g. `vpc-0abc1234def`). |
| `state` | `string` | VPC state: `pending` or `available`. |
| `ipv6CidrBlock` | `string` | Amazon-provided IPv6 CIDR block (set when `assignIpv6CidrBlock: true`). |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: VPC
metadata:
  name: my-vpc
  namespace: platform
spec:
  cidrBlock: "10.0.0.0/16"
  enableDnsSupport: true
  enableDnsHostnames: true
  tags:
    env: prod
    team: platform
```

### Example — IPv6 and secondary CIDR

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: VPC
metadata:
  name: my-vpc-dual-stack
  namespace: platform
spec:
  cidrBlock: "10.0.0.0/16"
  enableDnsSupport: true
  enableDnsHostnames: true
  assignIpv6CidrBlock: true
  secondaryIPv4CIDRs:
    - "100.64.0.0/16"
  tags:
    env: prod
```

### Notes

- The primary `cidrBlock` is immutable after creation. To change it, delete and recreate the CR.
- `instanceTenancy` is immutable after creation.
- `secondaryIPv4CIDRs` is reconciled on every sync: CIDR blocks in the list that don't exist on the VPC are associated, and CIDR blocks no longer in the list are disassociated.
- Drift detection: if `enableDnsSupport` or `enableDnsHostnames` are changed in the AWS console, the controller reverts them on the next reconcile.

### Deletion

**Fails if the VPC is not empty.** AWS will reject `DeleteVpc` if the VPC still has subnets, non-default security groups, non-main route tables, or internet gateways attached. Delete dependent CRs first — the recommended order for a full teardown:

1. `EC2Instance`, `DBInstance`, `DBCluster` (instances using subnets/SGs)
2. `AutoScalingGroup`
3. `ElastiCacheReplicationGroup`
4. `NatGateway`
5. `VPCEndpoint`
6. `RouteTable`
7. `InternetGateway`
8. `SecurityGroup`
9. `Subnet`
10. `VPC`

---

## Subnet

Creates and manages a subnet within a VPC.

**Status: ✅ Working**

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `vpcRef.name` | `string` | ✅ (or `id`) | Name of a `VPC` CR in the same namespace. |
| `vpcRef.id` | `string` | ✅ (or `name`) | Direct VPC ID, bypassing CR lookup. |
| `cidrBlock` | `string` | ✅ | IPv4 CIDR block within the VPC (e.g. `10.0.1.0/24`). |
| `availabilityZone` | `string` | ✅ | Availability zone (e.g. `us-east-1a`). |
| `mapPublicIpOnLaunch` | `bool` | | Auto-assign public IPs to instances launched in this subnet. Default: `false`. |
| `tags` | `map[string]string` | | AWS resource tags. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `subnetId` | `string` | AWS Subnet ID (e.g. `subnet-0abc1234def`). |
| `availableIpAddressCount` | `int32` | Number of available IP addresses in the subnet. |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: Subnet
metadata:
  name: public-subnet-1a
  namespace: platform
spec:
  vpcRef:
    name: my-vpc
  cidrBlock: "10.0.1.0/24"
  availabilityZone: us-east-1a
  mapPublicIpOnLaunch: true
  tags:
    tier: public
    env: prod
---
apiVersion: aws.konfig.io/v1alpha1
kind: Subnet
metadata:
  name: private-subnet-1b
  namespace: platform
spec:
  vpcRef:
    name: my-vpc
  cidrBlock: "10.0.2.0/24"
  availabilityZone: us-east-1b
  tags:
    tier: private
    env: prod
```

### Notes

- The controller waits for the `VPC` CR to be `Ready` before creating the subnet.
- CIDR block is immutable after creation.
- For RDS multi-AZ or EKS node groups, create at least two subnets in different availability zones.

### Deletion

**Fails if the subnet has ENIs attached.** Any resource using the subnet — EC2 instances, RDS instances, load balancer nodes, NAT gateways, Lambda functions — must be deleted first. The controller retries automatically once the subnet is free.

---

## InternetGateway

Creates and manages an internet gateway, and attaches it to a VPC.

**Status: ✅ Working**

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `vpcRef.name` | `string` | ✅ (or `id`) | Name of a `VPC` CR to attach this gateway to. |
| `vpcRef.id` | `string` | ✅ (or `name`) | Direct VPC ID. |
| `tags` | `map[string]string` | | AWS resource tags. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `internetGatewayId` | `string` | AWS Internet Gateway ID (e.g. `igw-0abc1234def`). |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: InternetGateway
metadata:
  name: my-igw
  namespace: platform
spec:
  vpcRef:
    name: my-vpc
  tags:
    env: prod
```

### Notes

- A VPC can have at most one internet gateway attached.
- On deletion, the controller detaches the gateway from the VPC before deleting it.
- Add a route to the internet gateway in a `RouteTable` with destination `0.0.0.0/0` to make a subnet public.

### Deletion

**Immediate — detaches from VPC first.** The controller calls `DetachInternetGateway` before `DeleteInternetGateway`. No manual detach is required.

---

## RouteTable

Creates and manages a route table with routes, and associates it with subnets.

**Status: ✅ Working**

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `vpcRef.name` | `string` | ✅ (or `id`) | Name of a `VPC` CR. |
| `vpcRef.id` | `string` | ✅ (or `name`) | Direct VPC ID. |
| `routes` | `[]RouteEntry` | | List of routes to add to the table. |
| `routes[].destinationCidr` | `string` | ✅ | Destination CIDR (e.g. `0.0.0.0/0`). |
| `routes[].gatewayId` | `string` | | Internet gateway ID or `local`. |
| `routes[].natGatewayRef` | `string` | | Name of a `NatGateway` CR in the same namespace. |
| `subnetAssociations` | `[]string` | | Names of `Subnet` CRs to associate with this route table. |
| `tags` | `map[string]string` | | AWS resource tags. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `routeTableId` | `string` | AWS Route Table ID (e.g. `rtb-0abc1234def`). |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example — public route table

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: RouteTable
metadata:
  name: public-rt
  namespace: platform
spec:
  vpcRef:
    name: my-vpc
  routes:
    - destinationCidr: "0.0.0.0/0"
      gatewayId: igw-0abc1234def    # or reference an InternetGateway CR via status
  subnetAssociations:
    - public-subnet-1a
    - public-subnet-1b
  tags:
    tier: public
```

### Example — private route table with NAT

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: RouteTable
metadata:
  name: private-rt
  namespace: platform
spec:
  vpcRef:
    name: my-vpc
  routes:
    - destinationCidr: "0.0.0.0/0"
      natGatewayRef: my-nat-gateway   # NatGateway CR name
  subnetAssociations:
    - private-subnet-1b
    - private-subnet-1c
  tags:
    tier: private
```

### Notes

- The controller diffs routes: it adds missing routes and removes routes no longer in spec. The local route (`10.0.0.0/16 → local`) is never removed.
- Subnet associations are reconciled: subnets in `subnetAssociations` that are not yet associated are associated; subnets that are no longer in the list are disassociated.
- When referencing a `NatGateway` CR, the controller waits for the `NatGateway` to be `Ready` before adding the route.

### Deletion

**Immediate — disassociates subnets first.** The controller calls `DisassociateRouteTable` for each associated subnet before calling `DeleteRouteTable`. The main route table of a VPC cannot be deleted (AWS restriction) — if the route table ID matches the VPC's main route table, the delete call is a no-op and the controller removes the finalizer.

---

## NatGateway

Creates and manages a NAT gateway for private subnets to reach the internet.

**Status: ✅ Working — async provisioning**

> NAT gateways take 1–3 minutes to provision. The controller polls every 15 seconds until the state reaches `available`.

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `subnetRef.name` | `string` | ✅ (or `id`) | Name of a public `Subnet` CR. |
| `subnetRef.id` | `string` | ✅ (or `name`) | Direct Subnet ID. |
| `connectivityType` | `string` | | `public` or `private`. Default: `public`. |
| `tags` | `map[string]string` | | AWS resource tags. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `natGatewayId` | `string` | AWS NAT Gateway ID (e.g. `nat-0abc1234def`). |
| `elasticIpAllocationId` | `string` | Elastic IP allocation ID (public NAT gateways). |
| `state` | `string` | NAT Gateway state: `pending`, `available`, `deleting`, or `deleted`. |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: NatGateway
metadata:
  name: my-nat-gateway
  namespace: platform
spec:
  subnetRef:
    name: public-subnet-1a    # must be a public subnet
  connectivityType: public
  tags:
    env: prod
```

### Notes

- Public NAT gateways automatically allocate an Elastic IP. On deletion, the controller releases the EIP. The `elasticIpAllocationId` is stored in status for cleanup.
- Private NAT gateways do not allocate an EIP.
- Only one NAT gateway per subnet is recommended (cost optimization).
- NAT gateway hourly billing begins as soon as the gateway reaches `available` state.

### Deletion

**Async — the CR stays in `Deleting` state for 60–90 seconds.** The controller calls `DeleteNatGateway`, then polls `DescribeNatGateways` every 15 seconds until the state is `deleted`. Once confirmed, it calls `ReleaseAddress` to release the Elastic IP. The finalizer is removed only after the EIP is released. Do not manually release the EIP while deletion is in progress.

---

## SecurityGroup

Creates and manages a VPC security group with ingress and egress rules.

**Status: ✅ Working**

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `vpcRef.name` | `string` | ✅ (or `id`) | Name of a `VPC` CR. |
| `vpcRef.id` | `string` | ✅ (or `name`) | Direct VPC ID. |
| `groupName` | `string` | ✅ | Security group name. Immutable after creation. |
| `description` | `string` | ✅ | Description of the security group. Immutable after creation. |
| `ingressRules` | `[]SGRule` | | Ingress (inbound) rules. |
| `egressRules` | `[]SGRule` | | Egress (outbound) rules. If empty, AWS defaults to allow-all egress. |
| `tags` | `map[string]string` | | AWS resource tags. |

#### SGRule Fields

| Field | Type | Description |
|-------|------|-------------|
| `protocol` | `string` | IP protocol: `tcp`, `udp`, `icmp`, or `-1` (all). |
| `fromPort` | `int32` | Start of port range (inclusive). |
| `toPort` | `int32` | End of port range (inclusive). |
| `cidrIpv4` | `string` | Source/destination IPv4 CIDR (e.g. `0.0.0.0/0`). |
| `cidrIpv6` | `string` | Source/destination IPv6 CIDR (e.g. `::/0`). |
| `prefixListId` | `string` | AWS-managed prefix list ID (e.g. `pl-63a5400a` for S3). |
| `sourceGroupRef` | `string` | Name of a `SecurityGroup` CR as source (ingress) or destination (egress). |
| `description` | `string` | Human-readable description for this rule. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `groupId` | `string` | AWS Security Group ID (e.g. `sg-0abc1234def`). |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: SecurityGroup
metadata:
  name: web-sg
  namespace: platform
spec:
  vpcRef:
    name: my-vpc
  groupName: web-sg
  description: "Security group for web tier"
  ingressRules:
    - protocol: tcp
      fromPort: 443
      toPort: 443
      cidrIpv4: "0.0.0.0/0"
      cidrIpv6: "::/0"
      description: "HTTPS from anywhere"
    - protocol: tcp
      fromPort: 80
      toPort: 80
      cidrIpv4: "0.0.0.0/0"
      description: "HTTP from anywhere"
  egressRules:
    - protocol: "-1"
      fromPort: 0
      toPort: 0
      cidrIpv4: "0.0.0.0/0"
    - protocol: "-1"
      fromPort: 0
      toPort: 0
      prefixListId: pl-63a5400a   # S3 managed prefix list
      description: "Allow traffic to S3 via prefix list"
  tags:
    tier: web
    env: prod
---
apiVersion: aws.konfig.io/v1alpha1
kind: SecurityGroup
metadata:
  name: db-sg
  namespace: platform
spec:
  vpcRef:
    name: my-vpc
  groupName: db-sg
  description: "Security group for database tier"
  ingressRules:
    - protocol: tcp
      fromPort: 5432
      toPort: 5432
      sourceGroupRef: web-sg    # allow from web tier only
  tags:
    tier: db
    env: prod
```

### Notes

- Drift detection: the controller diffs ingress and egress rules on every reconcile and calls `AuthorizeSecurityGroupIngress`/`RevokeSecurityGroupIngress` as needed.
- `groupName` and `description` are immutable after creation. To change them, delete and recreate the CR.
- `sourceGroupRef` references a security group in the same namespace. The controller waits for that security group to be `Ready` before authorizing the rule.

### Deletion

**Fails if the security group is in use.** AWS rejects `DeleteSecurityGroup` if any network interface (ENI) references the SG, or if another security group has an ingress/egress rule that uses this SG as source or destination. Terminate all instances using it and remove any cross-SG rules first. The controller retries automatically.

---

## VPCEndpoint

Creates and manages a VPC endpoint for private connectivity to AWS services.

**Status: ✅ Working**

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `vpcRef.name` | `string` | ✅ (or `id`) | Name of a `VPC` CR. |
| `vpcRef.id` | `string` | ✅ (or `name`) | Direct VPC ID. |
| `serviceName` | `string` | ✅ | AWS service endpoint name (e.g. `com.amazonaws.us-east-1.s3`). |
| `endpointType` | `string` | ✅ | `Interface` or `Gateway`. |
| `routeTableRefs` | `[]string` | | Names of `RouteTable` CRs (Gateway type only). |
| `subnetRefs` | `[]SubnetRef` | | Subnet refs for interface endpoints. |
| `securityGroupRefs` | `[]SecurityGroupRef` | | Security groups for interface endpoints. |
| `tags` | `map[string]string` | | AWS resource tags. |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `endpointId` | `string` | AWS VPC Endpoint ID (e.g. `vpce-0abc1234def`). |
| `state` | `string` | Endpoint state. |
| `conditions` | `[]metav1.Condition` | Standard conditions including `Ready`. |
| `observedGeneration` | `int64` | Last processed spec generation. |
| `lastSyncTime` | `metav1.Time` | Timestamp of last successful sync. |

### Example — S3 Gateway endpoint

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: VPCEndpoint
metadata:
  name: s3-endpoint
  namespace: platform
spec:
  vpcRef:
    name: my-vpc
  serviceName: com.amazonaws.us-east-1.s3
  endpointType: Gateway
  routeTableRefs:
    - private-rt
  tags:
    env: prod
```

### Example — SSM Interface endpoint

```yaml
apiVersion: aws.konfig.io/v1alpha1
kind: VPCEndpoint
metadata:
  name: ssm-endpoint
  namespace: platform
spec:
  vpcRef:
    name: my-vpc
  serviceName: com.amazonaws.us-east-1.ssm
  endpointType: Interface
  subnetRefs:
    - name: private-subnet-1b
    - name: private-subnet-1c
  securityGroupRefs:
    - name: web-sg
  tags:
    env: prod
```

### Notes

- Gateway endpoints (S3, DynamoDB) are free. Interface endpoints are billed per hour and per GB processed.
- Interface endpoints create ENIs in the specified subnets — ensure your subnets have available IP addresses.
- The service name format is `com.amazonaws.<region>.<service>`.
