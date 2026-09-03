account_id   = "123456789012" # your prod AWS account
cluster_name = "prod-eks"     # your EKS cluster name

environment = "prod"

# Restrict the IAM path prefix konfig-konector is allowed to manage.
# Roles and policies it creates will live under this path.
allowed_iam_resource_paths = "/konfig/"
