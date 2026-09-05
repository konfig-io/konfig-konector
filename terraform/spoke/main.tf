# konfig-konector spoke role.
#
# Apply this module in EVERY account the operator should manage other than the
# one it runs in. It creates a role trusted by the operator (hub) role and
# attaches the same permission set the hub uses. Then declare an AWSProvider
# pointing at the role ARN (helm value `providers`), and select it from
# resources with spec.providerRef or the aws.konfig.io/provider namespace
# annotation.

terraform {
  required_version = ">= 1.0"
  required_providers {
    aws = { source = "hashicorp/aws", version = ">= 5.0" }
  }
}

variable "hub_operator_role_arn" {
  description = "ARN of the operator role output by the root module (operator_role_arn)."
  type        = string
}

variable "name" {
  description = "Name of the spoke role."
  type        = string
  default     = "konfig-konector-spoke"
}

variable "external_id" {
  description = "Optional sts:ExternalId the operator must present (set AWSProvider.spec.externalId to match)."
  type        = string
  default     = ""
}

variable "policy_arns" {
  description = "Managed policy ARNs to attach. Defaults to AdministratorAccess; narrow this for production."
  type        = list(string)
  default     = ["arn:aws:iam::aws:policy/AdministratorAccess"]
}

variable "max_session_duration" {
  type    = number
  default = 3600
}

data "aws_iam_policy_document" "trust" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRole", "sts:TagSession"]
    principals {
      type        = "AWS"
      identifiers = [var.hub_operator_role_arn]
    }
    dynamic "condition" {
      for_each = var.external_id != "" ? [1] : []
      content {
        test     = "StringEquals"
        variable = "sts:ExternalId"
        values   = [var.external_id]
      }
    }
  }
}

resource "aws_iam_role" "spoke" {
  name                 = var.name
  description          = "Assumed by the konfig-konector operator in the hub account"
  assume_role_policy   = data.aws_iam_policy_document.trust.json
  max_session_duration = var.max_session_duration
}

resource "aws_iam_role_policy_attachment" "spoke" {
  for_each   = toset(var.policy_arns)
  role       = aws_iam_role.spoke.name
  policy_arn = each.value
}

output "spoke_role_arn" {
  description = "Use as AWSProvider.spec.roleArn."
  value       = aws_iam_role.spoke.arn
}
