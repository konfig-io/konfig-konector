variable "account_id" {
  description = "AWS account ID for this environment."
  type        = string
}

variable "environment" {
  description = "Deployment environment: prod or dev."
  type        = string
  validation {
    condition     = contains(["prod", "dev"], var.environment)
    error_message = "environment must be prod or dev."
  }
}

variable "region" {
  description = "AWS region."
  type        = string
  default     = "us-east-1"
}

variable "operator_namespace" {
  description = "Kubernetes namespace for the konfig-konector operator."
  type        = string
  default     = "konfig-system"
}

variable "operator_service_account" {
  description = "Kubernetes ServiceAccount name for the konfig-konector operator."
  type        = string
  default     = "konfig-controller"
}

# ── Optional: restrict which IAM paths konfig-konector is allowed to manage. ──
# Use "/" to allow all paths in the account.
variable "allowed_iam_resource_paths" {
  description = "IAM path prefix for roles and policies konfig-konector may manage (e.g. \"/konfig/\")."
  type        = string
  default     = "/konfig/"
}

variable "cluster_name" {
  description = "Name of the EKS cluster the operator runs on (used for the Pod Identity association and addon)."
  type        = string
}
