#!/usr/bin/env bash
# build-push.sh — build, tag, and push the konfig-konector operator image to ECR.
#
# Usage:
#   ./scripts/build-push.sh                  # uses version from helm/konfig-konector/Chart.yaml
#   ./scripts/build-push.sh 0.3.0            # override version
#   ./scripts/build-push.sh 0.3.0 --no-push  # build only, skip push
#
# Prerequisites:
#   - docker
#   - aws CLI with valid credentials (set AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY / AWS_SESSION_TOKEN)
#   - GOTOOLCHAIN=local or go 1.22+ on PATH

set -euo pipefail

REPO="647029483662.dkr.ecr.us-east-1.amazonaws.com/platform/konfig-konector"
AWS_REGION="${AWS_REGION:-us-east-1}"
CHART_FILE="helm/konfig-konector/Chart.yaml"
ARGO_CHART_FILE="../argo/konfig-konector/Chart.yaml"

# ── Resolve version ──────────────────────────────────────────────────────────
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

echo "==> Version: $VERSION"
echo "==> Image:   $REPO:$VERSION"

# ── Regenerate deepcopy + CRD manifests ─────────────────────────────────────
echo "==> Generating deepcopy methods..."
GOTOOLCHAIN=local go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.14.0 \
  object:headerFile="hack/boilerplate.go.txt" paths="./api/..."

echo "==> Generating CRD manifests..."
GOTOOLCHAIN=local go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.14.0 \
  rbac:roleName=manager-role crd:allowDangerousTypes=true webhook paths="./..." \
  output:crd:artifacts:config=config/crd/bases

echo "==> Copying CRDs to Helm chart..."
cp config/crd/bases/*.yaml helm/konfig-konector/crds/

# ── Bump chart versions ──────────────────────────────────────────────────────
echo "==> Bumping chart versions to $VERSION..."
sed -i "s/^version:.*/version: $VERSION/" "$CHART_FILE"
sed -i "s/^appVersion:.*/appVersion: \"$VERSION\"/" "$CHART_FILE"

if [[ -f "$ARGO_CHART_FILE" ]]; then
  sed -i "s/^version:.*/version: $VERSION/" "$ARGO_CHART_FILE"
  sed -i "s/^appVersion:.*/appVersion: \"$VERSION\"/" "$ARGO_CHART_FILE"
fi

# ── Build image ──────────────────────────────────────────────────────────────
echo "==> Building image..."
GOTOOLCHAIN=local docker build \
  -t "$REPO:$VERSION" \
  -t "$REPO:latest" \
  .

if [[ "$NO_PUSH" == "true" ]]; then
  echo "==> Skipping push (--no-push)"
  exit 0
fi

# ── ECR login + push ─────────────────────────────────────────────────────────
echo "==> Logging into ECR..."
aws ecr get-login-password --region "$AWS_REGION" \
  | docker login --username AWS --password-stdin \
      "647029483662.dkr.ecr.$AWS_REGION.amazonaws.com"

echo "==> Pushing $REPO:$VERSION ..."
docker push "$REPO:$VERSION"

echo "==> Pushing $REPO:latest ..."
docker push "$REPO:latest"

echo "==> Done. Image: $REPO:$VERSION"
