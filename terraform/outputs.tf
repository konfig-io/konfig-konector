output "operator_role_arn" {
  description = "ARN of the IAM role for the konfig-konector operator."
  value       = aws_iam_role.operator.arn
}

output "operator_role_name" {
  description = "Name of the IAM role for the konfig-konector operator."
  value       = aws_iam_role.operator.name
}

output "cluster_name" {
  description = "EKS cluster name derived from the environment."
  value       = local.cluster_name
}

output "pod_identity_association_id" {
  description = "EKS Pod Identity association ID for the operator."
  value       = aws_eks_pod_identity_association.operator.association_id
}

output "helm_install_command" {
  description = "Helm install command pre-filled with the operator role ARN. Replace <your-registry> with the registry you pushed the image to."
  value       = <<-EOT
    helm install konfig-konector ./helm/konfig-konector \
      --namespace konfig-system \
      --create-namespace \
      --set image.repository=<your-registry>/konfig-konector \
      --set image.tag=v0.1.0 \
      --set aws.region=${var.region} \
      --set operatorRoleArn=${aws_iam_role.operator.arn}
  EOT
}
