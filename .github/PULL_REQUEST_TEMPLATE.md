## What & why

<!-- Summary of the change and the motivation. Link the issue if one exists. -->

## Checklist

- [ ] `make test lint` passes locally
- [ ] `make manifests generate` produces no diff
- [ ] New/changed behavior is covered by a test
- [ ] For new kinds: CRD copied to `helm/konfig-konector/crds/`, RBAC updated, operator IAM policy updated in `terraform/main.tf`
