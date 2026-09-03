# Security Policy

## Reporting a vulnerability

Please **do not** report security vulnerabilities through public issues.

- **GitHub:** use [private vulnerability reporting](https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-security-vulnerability)
  (Security tab → "Report a vulnerability") on the repository.
- **GitLab:** open a confidential issue on the project.

You should receive an acknowledgement within a few days. Please include a
description of the issue, steps to reproduce, and the affected version.

## Scope

konfig-konector reconciles AWS resources with the permissions granted to its
operator IAM role. Reports we especially care about:

- Privilege escalation beyond the operator's IAM policy or the
  `allowed_iam_resource_paths` boundary
- Secret material (RDS passwords, ElastiCache auth tokens, Secrets Manager
  values) leaking into CR status, events, or logs
- Cross-namespace access via cross-resource references
- Finalizer/deletion-policy bypasses that destroy AWS resources unexpectedly

## Supported versions

Only the latest release receives security fixes.
