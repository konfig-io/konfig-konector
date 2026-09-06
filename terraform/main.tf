terraform {
  required_version = ">= 1.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.0.0"
    }
  }

  backend "s3" {
    region = "us-east-1"
  }
}

provider "aws" {
  region = var.region
  default_tags {
    tags = {
      Environment = var.environment
      Service     = "konfig-konector"
      ManagedBy   = "Terraform"
    }
  }
}

# ── Data sources ───────────────────────────────────────────────────────────────

data "aws_caller_identity" "current" {}
data "aws_region" "current" {}

locals {
  prefix       = var.environment
  cluster_name = var.cluster_name
}

# ── Operator IAM role ──────────────────────────────────────────────────────────
#
# The operator pod assumes this role via EKS Pod Identity. It must be able to
# manage IAM roles/policies, Route53 zones/records, and EKS Pod Identity
# associations on behalf of the CRs it reconciles.

resource "aws_iam_role" "operator" {
  name        = "${local.prefix}-konfig-konector-operator"
  description = "IAM role assumed by the konfig-konector operator via EKS Pod Identity"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          Service = "pods.eks.amazonaws.com"
        }
        Action = [
          "sts:AssumeRole",
          "sts:TagSession",
        ]
      }
    ]
  })
}

# ── Multi-account: allow assuming spoke roles ────────────────────────────────
resource "aws_iam_role_policy" "operator_assume_spokes" {
  count = length(var.spoke_role_arns) > 0 ? 1 : 0
  name  = "${local.prefix}-konfig-konector-assume-spokes"
  role  = aws_iam_role.operator.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["sts:AssumeRole", "sts:TagSession"]
      Resource = var.spoke_role_arns
    }]
  })
}

resource "aws_iam_role_policy" "operator" {
  name   = "${local.prefix}-konfig-konector-operator-policy"
  role   = aws_iam_role.operator.id
  policy = data.aws_iam_policy_document.operator.json
}

data "aws_iam_policy_document" "operator" {
  # ── IAM: read-only (must be * so the controller can check existence on any path) ─
  statement {
    sid    = "IAMReadOnly"
    effect = "Allow"
    actions = [
      "iam:GetRole",
      "iam:ListRoleTags",
      "iam:GetPolicy",
      "iam:GetPolicyVersion",
      "iam:ListPolicyVersions",
      "iam:GetRolePolicy",
      "iam:ListRolePolicies",
      "iam:ListAttachedRolePolicies",
    ]
    resources = ["*"]
  }

  # ── IAM: mutate roles (scoped to allowed path) ───────────────────────────────
  statement {
    sid    = "IAMRoles"
    effect = "Allow"
    actions = [
      "iam:CreateRole",
      "iam:DeleteRole",
      "iam:UpdateRole",
      "iam:UpdateAssumeRolePolicy",
      "iam:TagRole",
      "iam:UntagRole",
    ]
    resources = [
      "arn:aws:iam::${var.account_id}:role${var.allowed_iam_resource_paths}*"
    ]
  }

  # ── IAM: mutate managed policies (scoped to allowed path) ────────────────────
  statement {
    sid    = "IAMPolicies"
    effect = "Allow"
    actions = [
      "iam:CreatePolicy",
      "iam:DeletePolicy",
      "iam:CreatePolicyVersion",
      "iam:DeletePolicyVersion",
      "iam:TagPolicy",
      "iam:UntagPolicy",
    ]
    resources = [
      "arn:aws:iam::${var.account_id}:policy${var.allowed_iam_resource_paths}*"
    ]
  }

  # ── IAM: attach / detach policies from roles (scoped to allowed path) ────────
  statement {
    sid    = "IAMPolicyAttachments"
    effect = "Allow"
    actions = [
      "iam:AttachRolePolicy",
      "iam:DetachRolePolicy",
      "iam:PutRolePolicy",
      "iam:DeleteRolePolicy",
    ]
    resources = [
      "arn:aws:iam::${var.account_id}:role${var.allowed_iam_resource_paths}*"
    ]
  }

  # ── IAM: pass role (for EKS Pod Identity, Lambda, ECS) ──────────────────────
  statement {
    sid     = "IAMPassRole"
    effect  = "Allow"
    actions = ["iam:PassRole"]
    resources = [
      "arn:aws:iam::${var.account_id}:role${var.allowed_iam_resource_paths}*"
    ]
    condition {
      test     = "StringEquals"
      variable = "iam:PassedToService"
      values = [
        "pods.eks.amazonaws.com",
        "lambda.amazonaws.com",
        "ecs-tasks.amazonaws.com",
      ]
    }
  }

  # ── IAM: users and groups ────────────────────────────────────────────────────
  statement {
    sid    = "IAMUsersAndGroups"
    effect = "Allow"
    actions = [
      "iam:CreateUser",
      "iam:DeleteUser",
      "iam:GetUser",
      "iam:UpdateUser",
      "iam:TagUser",
      "iam:UntagUser",
      "iam:ListUserTags",
      "iam:CreateGroup",
      "iam:DeleteGroup",
      "iam:GetGroup",
      "iam:AddUserToGroup",
      "iam:RemoveUserFromGroup",
      "iam:AttachGroupPolicy",
      "iam:DetachGroupPolicy",
      "iam:ListAttachedGroupPolicies",
    ]
    resources = ["*"]
  }

  # ── IAM: federation providers (SAML / OIDC) ──────────────────────────────────
  statement {
    sid    = "IAMFederationProviders"
    effect = "Allow"
    actions = [
      "iam:CreateSAMLProvider",
      "iam:GetSAMLProvider",
      "iam:UpdateSAMLProvider",
      "iam:DeleteSAMLProvider",
      "iam:ListSAMLProviders",
      "iam:TagSAMLProvider",
      "iam:UntagSAMLProvider",
      "iam:ListSAMLProviderTags",
      "iam:CreateOpenIDConnectProvider",
      "iam:GetOpenIDConnectProvider",
      "iam:UpdateOpenIDConnectProviderThumbprint",
      "iam:AddClientIDToOpenIDConnectProvider",
      "iam:RemoveClientIDFromOpenIDConnectProvider",
      "iam:DeleteOpenIDConnectProvider",
      "iam:ListOpenIDConnectProviders",
      "iam:TagOpenIDConnectProvider",
      "iam:UntagOpenIDConnectProvider",
      "iam:ListOpenIDConnectProviderTags",
    ]
    resources = ["*"]
  }

  # ── Route53: hosted zones ────────────────────────────────────────────────────
  statement {
    sid    = "Route53Zones"
    effect = "Allow"
    actions = [
      "route53:CreateHostedZone",
      "route53:DeleteHostedZone",
      "route53:GetHostedZone",
      "route53:ListHostedZones",
      "route53:ListHostedZonesByName",
      "route53:UpdateHostedZoneComment",
    ]
    resources = ["*"]
  }

  # ── Route53: record sets ─────────────────────────────────────────────────────
  statement {
    sid    = "Route53Records"
    effect = "Allow"
    actions = [
      "route53:ChangeResourceRecordSets",
      "route53:ListResourceRecordSets",
      "route53:GetChange",
    ]
    resources = [
      "arn:aws:route53:::hostedzone/*",
      "arn:aws:route53:::change/*",
    ]
  }

  # ── Route53: health checks ───────────────────────────────────────────────────
  statement {
    sid    = "Route53HealthChecks"
    effect = "Allow"
    actions = [
      "route53:CreateHealthCheck",
      "route53:DeleteHealthCheck",
      "route53:GetHealthCheck",
      "route53:UpdateHealthCheck",
      "route53:ListHealthChecks",
    ]
    resources = ["*"]
  }

  # ── Route53: tags ─────────────────────────────────────────────────────────────
  statement {
    sid    = "Route53Tags"
    effect = "Allow"
    actions = [
      "route53:ChangeTagsForResource",
      "route53:ListTagsForResource",
      "route53:ListTagsForResources",
    ]
    resources = ["*"]
  }

  # ── EKS Pod Identity associations ────────────────────────────────────────────
  statement {
    sid    = "EKSPodIdentity"
    effect = "Allow"
    actions = [
      "eks:CreatePodIdentityAssociation",
      "eks:DeletePodIdentityAssociation",
      "eks:DescribePodIdentityAssociation",
      "eks:UpdatePodIdentityAssociation",
      "eks:ListPodIdentityAssociations",
    ]
    resources = [
      "arn:aws:eks:${var.region}:${var.account_id}:cluster/${local.cluster_name}",
      "arn:aws:eks:${var.region}:${var.account_id}:podidentityassociation/${local.cluster_name}/*",
    ]
  }

  # ── EKS: Cluster, NodeGroup, Addon, FargateProfile, AccessEntry ─────────────
  statement {
    sid    = "EKSManagement"
    effect = "Allow"
    actions = [
      "eks:CreateCluster",
      "eks:DeleteCluster",
      "eks:DescribeCluster",
      "eks:UpdateClusterConfig",
      "eks:UpdateClusterVersion",
      "eks:CreateNodegroup",
      "eks:DeleteNodegroup",
      "eks:DescribeNodegroup",
      "eks:UpdateNodegroupConfig",
      "eks:UpdateNodegroupVersion",
      "eks:ListNodegroups",
      "eks:CreateAddon",
      "eks:DeleteAddon",
      "eks:DescribeAddon",
      "eks:UpdateAddon",
      "eks:ListAddons",
      "eks:CreateFargateProfile",
      "eks:DeleteFargateProfile",
      "eks:DescribeFargateProfile",
      "eks:ListFargateProfiles",
      "eks:CreateAccessEntry",
      "eks:DeleteAccessEntry",
      "eks:DescribeAccessEntry",
      "eks:UpdateAccessEntry",
      "eks:AssociateAccessPolicy",
      "eks:DisassociateAccessPolicy",
      "eks:ListAssociatedAccessPolicies",
      "eks:ListAccessEntries",
      "eks:TagResource",
      "eks:UntagResource",
    ]
    resources = ["*"]
  }

  # ── Lambda: Function, EventSourceMapping, Permission ─────────────────────────
  statement {
    sid    = "LambdaManagement"
    effect = "Allow"
    actions = [
      "lambda:CreateFunction",
      "lambda:DeleteFunction",
      "lambda:GetFunction",
      "lambda:UpdateFunctionCode",
      "lambda:UpdateFunctionConfiguration",
      "lambda:AddPermission",
      "lambda:RemovePermission",
      "lambda:GetPolicy",
      "lambda:CreateEventSourceMapping",
      "lambda:DeleteEventSourceMapping",
      "lambda:GetEventSourceMapping",
      "lambda:UpdateEventSourceMapping",
      "lambda:ListEventSourceMappings",
      "lambda:TagResource",
      "lambda:UntagResource",
    ]
    resources = ["*"]
  }

  # ── ECS: Cluster, TaskDefinition, Service ────────────────────────────────────
  statement {
    sid    = "ECSManagement"
    effect = "Allow"
    actions = [
      "ecs:CreateCluster",
      "ecs:DeleteCluster",
      "ecs:DescribeClusters",
      "ecs:RegisterTaskDefinition",
      "ecs:DeregisterTaskDefinition",
      "ecs:DescribeTaskDefinition",
      "ecs:CreateService",
      "ecs:DeleteService",
      "ecs:UpdateService",
      "ecs:DescribeServices",
      "ecs:TagResource",
      "ecs:UntagResource",
    ]
    resources = ["*"]
  }

  # ── EC2: VPC & networking ─────────────────────────────────────────────────────
  statement {
    sid    = "EC2VPC"
    effect = "Allow"
    actions = [
      "ec2:CreateVpc",
      "ec2:DeleteVpc",
      "ec2:DescribeVpcs",
      "ec2:ModifyVpcAttribute",
      "ec2:CreateSubnet",
      "ec2:DeleteSubnet",
      "ec2:DescribeSubnets",
      "ec2:ModifySubnetAttribute",
      "ec2:CreateInternetGateway",
      "ec2:DeleteInternetGateway",
      "ec2:AttachInternetGateway",
      "ec2:DetachInternetGateway",
      "ec2:DescribeInternetGateways",
      "ec2:CreateRouteTable",
      "ec2:DeleteRouteTable",
      "ec2:DescribeRouteTables",
      "ec2:CreateRoute",
      "ec2:DeleteRoute",
      "ec2:AssociateRouteTable",
      "ec2:DisassociateRouteTable",
      "ec2:AllocateAddress",
      "ec2:ReleaseAddress",
      "ec2:DescribeAddresses",
      "ec2:CreateNatGateway",
      "ec2:DeleteNatGateway",
      "ec2:DescribeNatGateways",
      "ec2:CreateSecurityGroup",
      "ec2:DeleteSecurityGroup",
      "ec2:DescribeSecurityGroups",
      "ec2:AuthorizeSecurityGroupIngress",
      "ec2:RevokeSecurityGroupIngress",
      "ec2:AuthorizeSecurityGroupEgress",
      "ec2:RevokeSecurityGroupEgress",
      "ec2:CreateVpcEndpoint",
      "ec2:DeleteVpcEndpoints",
      "ec2:DescribeVpcEndpoints",
      "ec2:ModifyVpcEndpoint",
    ]
    resources = ["*"]
  }

  # ── EC2: compute (key pairs, launch templates) ────────────────────────────────
  statement {
    sid    = "EC2Compute"
    effect = "Allow"
    actions = [
      "ec2:CreateKeyPair",
      "ec2:DeleteKeyPair",
      "ec2:DescribeKeyPairs",
      "ec2:ImportKeyPair",
      "ec2:CreateLaunchTemplate",
      "ec2:DeleteLaunchTemplate",
      "ec2:DescribeLaunchTemplates",
      "ec2:DescribeLaunchTemplateVersions",
      "ec2:CreateLaunchTemplateVersion",
      "ec2:RunInstances",
      "ec2:TerminateInstances",
      "ec2:DescribeInstances",
      "ec2:DescribeInstanceStatus",
      "ec2:CreateTags",
      "ec2:DeleteTags",
      "ec2:DescribeTags",
    ]
    resources = ["*"]
  }

  # ── AutoScaling ───────────────────────────────────────────────────────────────
  statement {
    sid    = "AutoScaling"
    effect = "Allow"
    actions = [
      "autoscaling:CreateAutoScalingGroup",
      "autoscaling:DeleteAutoScalingGroup",
      "autoscaling:UpdateAutoScalingGroup",
      "autoscaling:DescribeAutoScalingGroups",
      "autoscaling:StartInstanceRefresh",
      "autoscaling:DescribeInstanceRefreshes",
      "autoscaling:CreateOrUpdateTags",
      "autoscaling:DeleteTags",
      "autoscaling:DescribeTags",
    ]
    resources = ["*"]
  }

  # ── RDS ───────────────────────────────────────────────────────────────────────
  statement {
    sid    = "RDS"
    effect = "Allow"
    actions = [
      "rds:CreateDBInstance",
      "rds:DeleteDBInstance",
      "rds:DescribeDBInstances",
      "rds:ModifyDBInstance",
      "rds:CreateDBCluster",
      "rds:DeleteDBCluster",
      "rds:DescribeDBClusters",
      "rds:ModifyDBCluster",
      "rds:CreateDBSubnetGroup",
      "rds:DeleteDBSubnetGroup",
      "rds:DescribeDBSubnetGroups",
      "rds:ModifyDBSubnetGroup",
      "rds:CreateDBParameterGroup",
      "rds:DeleteDBParameterGroup",
      "rds:DescribeDBParameterGroups",
      "rds:ModifyDBParameterGroup",
      "rds:ResetDBParameterGroup",
      "rds:CreateDBClusterParameterGroup",
      "rds:DeleteDBClusterParameterGroup",
      "rds:DescribeDBClusterParameterGroups",
      "rds:ModifyDBClusterParameterGroup",
      "rds:AddTagsToResource",
      "rds:RemoveTagsFromResource",
      "rds:ListTagsForResource",
    ]
    resources = ["*"]
  }

  # ── S3 ────────────────────────────────────────────────────────────────────────
  statement {
    sid    = "S3"
    effect = "Allow"
    actions = [
      "s3:CreateBucket",
      "s3:DeleteBucket",
      "s3:ListBucket",
      "s3:GetBucketLocation",
      "s3:GetBucketTagging",
      "s3:PutBucketTagging",
      "s3:GetBucketVersioning",
      "s3:PutBucketVersioning",
      "s3:GetEncryptionConfiguration",
      "s3:PutEncryptionConfiguration",
      "s3:GetBucketPolicy",
      "s3:PutBucketPolicy",
      "s3:DeleteBucketPolicy",
      "s3:GetAccelerateConfiguration",
      "s3:PutAccelerateConfiguration",
      "s3:GetLifecycleConfiguration",
      "s3:PutLifecycleConfiguration",
      "s3:GetBucketCORS",
      "s3:PutBucketCORS",
      "s3:GetBucketNotification",
      "s3:PutBucketNotification",
      "s3:GetBucketPublicAccessBlock",
      "s3:PutBucketPublicAccessBlock",
      "s3:GetBucketWebsite",
      "s3:PutBucketWebsite",
      "s3:DeleteBucketWebsite",
      "s3:GetBucketObjectLockConfiguration",
      "s3:GetBucketLogging",
      "s3:PutBucketLogging",
      "s3:GetReplicationConfiguration",
      "s3:PutReplicationConfiguration",
    ]
    resources = ["*"]
  }

  # ── SQS ───────────────────────────────────────────────────────────────────────
  statement {
    sid    = "SQS"
    effect = "Allow"
    actions = [
      "sqs:CreateQueue",
      "sqs:DeleteQueue",
      "sqs:GetQueueUrl",
      "sqs:GetQueueAttributes",
      "sqs:SetQueueAttributes",
      "sqs:TagQueue",
      "sqs:UntagQueue",
      "sqs:ListQueueTags",
    ]
    resources = ["*"]
  }

  # ── SNS ───────────────────────────────────────────────────────────────────────
  statement {
    sid    = "SNS"
    effect = "Allow"
    actions = [
      "sns:CreateTopic",
      "sns:DeleteTopic",
      "sns:GetTopicAttributes",
      "sns:SetTopicAttributes",
      "sns:Subscribe",
      "sns:Unsubscribe",
      "sns:GetSubscriptionAttributes",
      "sns:SetSubscriptionAttributes",
      "sns:TagResource",
      "sns:UntagResource",
      "sns:ListTagsForResource",
    ]
    resources = ["*"]
  }

  # ── ElastiCache ───────────────────────────────────────────────────────────────
  statement {
    sid    = "ElastiCache"
    effect = "Allow"
    actions = [
      "elasticache:CreateCacheSubnetGroup",
      "elasticache:DeleteCacheSubnetGroup",
      "elasticache:DescribeCacheSubnetGroups",
      "elasticache:ModifyCacheSubnetGroup",
      "elasticache:CreateReplicationGroup",
      "elasticache:DeleteReplicationGroup",
      "elasticache:DescribeReplicationGroups",
      "elasticache:ModifyReplicationGroup",
      "elasticache:AddTagsToResource",
      "elasticache:RemoveTagsFromResource",
      "elasticache:ListTagsForResource",
    ]
    resources = ["*"]
  }

  # ── DynamoDB ──────────────────────────────────────────────────────────────────
  statement {
    sid    = "DynamoDB"
    effect = "Allow"
    actions = [
      "dynamodb:CreateTable",
      "dynamodb:DeleteTable",
      "dynamodb:DescribeTable",
      "dynamodb:UpdateTable",
      "dynamodb:DescribeContinuousBackups",
      "dynamodb:UpdateContinuousBackups",
      "dynamodb:TagResource",
      "dynamodb:UntagResource",
      "dynamodb:ListTagsOfResource",
    ]
    resources = ["*"]
  }

  # ── Secrets Manager (secret metadata + value writes; reads only what it manages) ─
  statement {
    sid    = "SecretsManager"
    effect = "Allow"
    actions = [
      "secretsmanager:CreateSecret",
      "secretsmanager:DeleteSecret",
      "secretsmanager:DescribeSecret",
      "secretsmanager:GetSecretValue",
      "secretsmanager:PutSecretValue",
      "secretsmanager:TagResource",
      "secretsmanager:UntagResource",
    ]
    resources = ["*"]
  }

  # ── SSM Parameter Store & documents ──────────────────────────────────────────
  statement {
    sid    = "SSM"
    effect = "Allow"
    actions = [
      "ssm:PutParameter",
      "ssm:GetParameter",
      "ssm:DeleteParameter",
      "ssm:AddTagsToResource",
      "ssm:RemoveTagsFromResource",
      "ssm:ListTagsForResource",
      "ssm:CreateDocument",
      "ssm:UpdateDocument",
      "ssm:DeleteDocument",
      "ssm:DescribeDocument",
    ]
    resources = ["*"]
  }

  # ── KMS ───────────────────────────────────────────────────────────────────────
  statement {
    sid    = "KMS"
    effect = "Allow"
    actions = [
      "kms:CreateKey",
      "kms:DescribeKey",
      "kms:ScheduleKeyDeletion",
      "kms:EnableKeyRotation",
      "kms:DisableKeyRotation",
      "kms:GetKeyRotationStatus",
      "kms:UpdateKeyDescription",
      "kms:GetKeyPolicy",
      "kms:PutKeyPolicy",
      "kms:CreateAlias",
      "kms:DeleteAlias",
      "kms:ListAliases",
      "kms:CreateGrant",
      "kms:RetireGrant",
      "kms:RevokeGrant",
      "kms:TagResource",
      "kms:UntagResource",
      "kms:ListResourceTags",
    ]
    resources = ["*"]
  }

  # ── ECR ───────────────────────────────────────────────────────────────────────
  statement {
    sid    = "ECR"
    effect = "Allow"
    actions = [
      "ecr:CreateRepository",
      "ecr:DeleteRepository",
      "ecr:DescribeRepositories",
      "ecr:PutLifecyclePolicy",
      "ecr:DeleteLifecyclePolicy",
      "ecr:GetLifecyclePolicy",
      "ecr:SetRepositoryPolicy",
      "ecr:DeleteRepositoryPolicy",
      "ecr:GetRepositoryPolicy",
      "ecr:TagResource",
      "ecr:UntagResource",
      "ecr:ListTagsForResource",
    ]
    resources = ["*"]
  }

  # ── CloudWatch Logs ───────────────────────────────────────────────────────────
  statement {
    sid    = "CloudWatchLogs"
    effect = "Allow"
    actions = [
      "logs:CreateLogGroup",
      "logs:DeleteLogGroup",
      "logs:DescribeLogGroups",
      "logs:PutRetentionPolicy",
      "logs:DeleteRetentionPolicy",
      "logs:AssociateKmsKey",
      "logs:DisassociateKmsKey",
      "logs:TagResource",
      "logs:UntagResource",
      "logs:ListTagsForResource",
      "logs:PutMetricFilter",
      "logs:DeleteMetricFilter",
      "logs:DescribeMetricFilters",
      "logs:PutSubscriptionFilter",
      "logs:DeleteSubscriptionFilter",
      "logs:DescribeSubscriptionFilters",
    ]
    resources = ["*"]
  }
}

# ── EKS Pod Identity association for the operator itself ──────────────────────

resource "aws_eks_pod_identity_association" "operator" {
  cluster_name    = local.cluster_name
  namespace       = var.operator_namespace
  service_account = var.operator_service_account
  role_arn        = aws_iam_role.operator.arn
}

# ── EKS Pod Identity addon ─────────────────────────────────────────────────────
#
# Installs the Pod Identity Agent DaemonSet. Skip if already present on the cluster.

resource "aws_eks_addon" "pod_identity_agent" {
  cluster_name                = local.cluster_name
  addon_name                  = "eks-pod-identity-agent"
  resolve_conflicts_on_create = "OVERWRITE"
  resolve_conflicts_on_update = "OVERWRITE"
}
