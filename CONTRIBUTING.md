# Contributing to konfig-konector

Thanks for your interest in contributing! This document covers the practical
side of getting a change merged.

## Before you start

- For significant changes (new resource kinds, behavioral changes to
  reconciliation, API changes), **open an issue first** so the approach can be
  discussed before you invest time.
- Bug fixes and documentation improvements can go straight to a pull request.

## Development setup

Prerequisites: Go 1.24+, Docker, Helm 3.x, `make`, Python 3 (generators), and for live testing the AWS CLI v2 and k3d (installed by `scripts/local-dev.sh`).

```bash
git clone https://github.com/konfig-io/konfig-konector.git
cd konfig-konector

make build            # compile everything
make test             # unit tests (downloads envtest binaries on first run)
make lint             # golangci-lint
```

For a full local development loop against a k3d cluster (no AWS account
needed for controller plumbing work), see `scripts/local-dev.sh`.

### Running against a real cluster

```bash
make install          # install CRDs into the active kubecontext
make run              # run the controller locally with your AWS credentials
```

## Adding a new resource kind

Follow the vertical-slice conventions in `docs/adding-a-kind.md` (or the
summary in the README). In short:

1. `api/v1alpha1/<kind>_types.go` — spec/status, kubebuilder markers
2. `make generate manifests` — deepcopy + CRD YAML
3. Copy the CRD to `helm/konfig-konector/crds/`
4. `internal/controller/<kind>_controller.go` — reconciler
5. Register in `cmd/main.go`, add RBAC to the Helm `ClusterRole`
6. Add the kind to the operator IAM policy in `terraform/main.tf`

Use `SQSQueue` as the reference implementation
(`api/v1alpha1/sqsqueue_types.go`, `internal/controller/sqsqueue_controller.go`,
`internal/aws/sqs/helpers.go`).

## Pull request checklist

- [ ] `make test lint` passes
- [ ] `make manifests generate` produces no diff (CI enforces this)
- [ ] New/changed behavior is covered by a test
- [ ] Commit messages explain *why*, not just *what*

CI runs build, vet, unit tests, and a stale-manifest check on every PR.

## Reporting bugs

Use the bug report issue template. Always include the operator version,
Kubernetes/EKS version, the CR (redact account IDs if you prefer), and the
relevant controller log lines.

## Security issues

Do **not** open a public issue — see [SECURITY.md](SECURITY.md).

## License

By contributing, you agree that your contributions are licensed under the
Apache License 2.0, the same license as the project.
