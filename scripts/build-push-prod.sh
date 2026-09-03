#!/usr/bin/env bash
# build-push-prod.sh — build and push the konfig-konector image to the PROD ECR registry.
#
# Usage:
#   ./scripts/build-push-prod.sh                  # uses version from helm/konfig-konector/Chart.yaml
#   ./scripts/build-push-prod.sh 0.8.1            # override version
#   ./scripts/build-push-prod.sh 0.8.1 --no-push  # build only, skip push
#
# Account: 422645413018 (prod)
# Prerequisites: docker, aws CLI with prod credentials, go 1.22+

set -euo pipefail

ACCOUNT="422645413018"
REPO="$ACCOUNT.dkr.ecr.us-east-1.amazonaws.com/platform/konfig-konector"
AWS_REGION="${AWS_REGION:-us-east-1}"
CHART_FILE="helm/konfig-konector/Chart.yaml"
ARGO_CHART_FILE="../argo/konfig-konector/Chart.yaml"

# ── Verify AWS account ────────────────────────────────────────────────────────
CURRENT_ACCOUNT=$(aws sts get-caller-identity --query Account --output text 2>&1)
if [[ "$CURRENT_ACCOUNT" != "$ACCOUNT" ]]; then
  echo "ERROR: Expected AWS account $ACCOUNT (prod), got $CURRENT_ACCOUNT. Switch credentials and retry."
  exit 1
fi

# ── Resolve version ───────────────────────────────────────────────────────────
if [[ $# -ge 1 && "$1" != --* ]]; then
  VERSION="$1"
  shift
else
  VERSION=$(grep '^version:' "$CHART_FILE" | awk '{print $2}')
fi

NO_PUSH=false
for arg in "$@"; do
  [[ "$arg" == "--no-push" ]] && NO_PUSH=true
done

echo "==> Environment: prod ($ACCOUNT)"
echo "==> Version:     $VERSION"
echo "==> Image:       $REPO:$VERSION"

# ── Regenerate deepcopy + CRD manifests ──────────────────────────────────────
echo "==> Generating deepcopy methods..."
GOTOOLCHAIN=local go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.14.0 \
  object:headerFile="hack/boilerplate.go.txt" paths="./api/..."

echo "==> Generating CRD manifests..."
GOTOOLCHAIN=local go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.14.0 \
  rbac:roleName=manager-role crd:allowDangerousTypes=true webhook paths="./..." \
  output:crd:artifacts:config=config/crd/bases

echo "==> Copying CRDs to Helm chart..."
cp config/crd/bases/*.yaml helm/konfig-konector/crds/

# ── Bump chart versions ───────────────────────────────────────────────────────
echo "==> Bumping chart versions to $VERSION..."
sed -i "s/^version:.*/version: $VERSION/" "$CHART_FILE"
sed -i "s/^appVersion:.*/appVersion: \"$VERSION\"/" "$CHART_FILE"

if [[ -f "$ARGO_CHART_FILE" ]]; then
  sed -i "s/^version:.*/version: $VERSION/" "$ARGO_CHART_FILE"
  sed -i "s/^appVersion:.*/appVersion: \"$VERSION\"/" "$ARGO_CHART_FILE"
fi

# ── Build image ───────────────────────────────────────────────────────────────
echo "==> Building image..."
GOTOOLCHAIN=local docker build \
  -t "$REPO:$VERSION" \
  -t "$REPO:latest" \
  .

if [[ "$NO_PUSH" == "true" ]]; then
  echo "==> Skipping push (--no-push)"
  exit 0
fi

# ── ECR login + push ──────────────────────────────────────────────────────────
echo "==> Logging into ECR..."
aws ecr get-login-password --region "$AWS_REGION" \
  | docker login --username AWS --password-stdin "$ACCOUNT.dkr.ecr.$AWS_REGION.amazonaws.com"

echo "==> Pushing $REPO:$VERSION ..."
docker push "$REPO:$VERSION"

echo "==> Pushing $REPO:latest ..."
docker push "$REPO:latest"

echo "==> Done. Image: $REPO:$VERSION"
