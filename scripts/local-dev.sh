#!/usr/bin/env bash
# Local development loop for konfig-konector.
#
# Stands up a k3d (k3s-in-Docker) cluster, builds the operator image from the
# working tree, and installs the deployed chart from ../argo/konfig-konector
# pointed at a real AWS account via static credentials. The operator then
# reconciles CRs against AWS exactly as it would in-cluster — this is the
# fastest way to watch the smoke resources converge without pushing an image.
#
# Usage:
#   ./scripts/local-dev.sh up       # create cluster + build + deploy (helm)
#   ./scripts/local-dev.sh argo     # bootstrap Argo CD + hand deployment to GitOps
#   ./scripts/local-dev.sh deploy   # rebuild image + import + restart operator
#   ./scripts/local-dev.sh smoke    # apply the smoke-test CRs
#   ./scripts/local-dev.sh status   # Ready condition of every konfig CR
#   ./scripts/local-dev.sh unsmoke  # delete smoke CRs (operator deletes AWS resources)
#   ./scripts/local-dev.sh down     # delete the cluster (run unsmoke first!)
#
# Two deployment modes:
#   helm (default, `up`)  — installs the chart from the local ../argo working
#                           tree; fastest for iterating on chart changes.
#   argo (`argo`)         — installs Argo CD in the cluster with the app-of-apps
#                           from clusters/local-dev/, mirroring how real
#                           clusters are managed. Argo pulls from GitHub, so
#                           chart changes must be pushed to be seen; operator
#                           code changes still flow via `deploy` (the image
#                           tag stays :dev, Argo doesn't care about restarts).
#
# AWS credentials come from your current environment/profile:
#   AWS_PROFILE=platformdev ./scripts/local-dev.sh up
# Use a role scoped to the dev account. Everything the smoke test creates is
# tagged managed-by=konfig-konector.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CHART_DIR="${REPO_ROOT}/../argo/konfig-konector"
CLUSTER_NAME="konfig-dev"
IMAGE="konfig-konector:dev"
NAMESPACE="konfig-system"
AWS_REGION="${AWS_REGION:-us-east-1}"

need() { command -v "$1" >/dev/null || { echo "missing required tool: $1" >&2; exit 1; }; }

install_k3d() {
  if command -v k3d >/dev/null; then return; fi
  echo ">> installing k3d to ~/.local/bin"
  mkdir -p "$HOME/.local/bin"
  curl -sfL https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh \
    | K3D_INSTALL_DIR="$HOME/.local/bin" USE_SUDO=false bash
  export PATH="$HOME/.local/bin:$PATH"
}

check_aws() {
  echo ">> verifying AWS credentials"
  local ident
  if ! ident=$(aws sts get-caller-identity --output text --query 'Arn' 2>&1); then
    echo "AWS credentials are not valid in this shell:" >&2
    echo "  $ident" >&2
    echo "Set AWS_PROFILE (or run aws sso login / aws configure) and retry." >&2
    exit 1
  fi
  echo "   authenticated as: $ident"
  case "$ident" in
    *prod*|*Prod*|*422645413018*)
      echo "REFUSING: these credentials look like the prod account. Use dev." >&2
      exit 1;;
  esac
}

cluster_up() {
  if k3d cluster list 2>/dev/null | grep -q "^${CLUSTER_NAME}\b"; then
    echo ">> cluster ${CLUSTER_NAME} already exists"
  else
    echo ">> creating k3d cluster ${CLUSTER_NAME}"
    k3d cluster create "${CLUSTER_NAME}" \
      --servers 1 --agents 0 \
      --k3s-arg "--disable=traefik@server:0" \
      --wait
  fi
  kubectl config use-context "k3d-${CLUSTER_NAME}" >/dev/null
}

build_and_import() {
  echo ">> building ${IMAGE} from working tree"
  docker build -t "${IMAGE}" "${REPO_ROOT}"
  echo ">> importing image into k3d"
  k3d image import "${IMAGE}" -c "${CLUSTER_NAME}"
}

credentials_secret() {
  echo ">> materialising AWS credentials into ${NAMESPACE}/aws-local-credentials"
  # The chart also templates this namespace; pre-create it with helm ownership
  # metadata so the later install can adopt it instead of erroring.
  kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
  kubectl label namespace "${NAMESPACE}" app.kubernetes.io/managed-by=Helm --overwrite >/dev/null
  kubectl annotate namespace "${NAMESPACE}" \
    meta.helm.sh/release-name=konfig-konector \
    meta.helm.sh/release-namespace="${NAMESPACE}" --overwrite >/dev/null
  # Resolve whatever the ambient chain provides (profile, SSO, env) into
  # static values the pod can use. Session credentials expire — rerun
  # `./scripts/local-dev.sh deploy` to refresh them.
  local creds
  creds=$(aws configure export-credentials --format env 2>/dev/null) || {
    echo "aws configure export-credentials failed (needs awscli v2.9+)" >&2; exit 1; }
  # shellcheck disable=SC2046
  eval "$creds"
  kubectl -n "${NAMESPACE}" create secret generic aws-local-credentials \
    --from-literal=AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID}" \
    --from-literal=AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY}" \
    ${AWS_SESSION_TOKEN:+--from-literal=AWS_SESSION_TOKEN="${AWS_SESSION_TOKEN}"} \
    --dry-run=client -o yaml | kubectl apply -f -
}

deploy() {
  echo ">> installing chart from ${CHART_DIR}"
  helm upgrade --install konfig-konector "${CHART_DIR}" \
    --namespace "${NAMESPACE}" \
    --set image.repository="${IMAGE%%:*}" \
    --set image.tag="${IMAGE##*:}" \
    --set image.pullPolicy=IfNotPresent \
    --set aws.region="${AWS_REGION}" \
    --set aws.credentialsSecret=aws-local-credentials \
    --set operatorRoleArn="" \
    --set smoketest.enabled=false \
    --set dagster.enabled=false
  kubectl -n "${NAMESPACE}" rollout restart deployment/konfig-controller >/dev/null 2>&1 || true
  echo ">> waiting for operator to become ready"
  kubectl -n "${NAMESPACE}" rollout status deployment/konfig-controller --timeout=120s
  kubectl -n "${NAMESPACE}" get pods
}

smoke() {
  echo ">> applying smoke-test CRs"
  helm template "${CHART_DIR}" --set smoketest.enabled=true -s templates/test.yaml \
    | kubectl apply -f -
  echo ">> watch convergence with: ./scripts/local-dev.sh status"
}

unsmoke() {
  echo ">> deleting smoke-test CRs (operator will delete the AWS resources;"
  echo "   resources annotated deletion-policy: abandon are left in AWS)"
  helm template "${CHART_DIR}" --set smoketest.enabled=true -s templates/test.yaml \
    | kubectl delete -f - --ignore-not-found --wait=false
  echo ">> deletion is asynchronous; check progress with: ./scripts/local-dev.sh status"
}

status() {
  # Every aws.konfig.io kind with its Ready condition, one line each.
  local kinds
  kinds=$(kubectl api-resources --api-group=aws.konfig.io -o name 2>/dev/null | sort)
  [ -n "$kinds" ] || { echo "no aws.konfig.io CRDs installed"; exit 1; }
  printf '%-28s %-32s %-8s %-22s %s\n' KIND NAME READY REASON MESSAGE
  for k in $kinds; do
    kubectl get "$k" -A -o json 2>/dev/null | python3 -c "
import json, sys
for it in json.load(sys.stdin).get('items', []):
    conds = {c['type']: c for c in it.get('status', {}).get('conditions', [])}
    r = conds.get('Ready', {})
    print('%-28s %-32s %-8s %-22s %s' % (
        it['kind'], it['metadata']['name'],
        r.get('status', '-'), r.get('reason', '-'),
        r.get('message', '')[:60]))
"
  done
}

down() {
  echo ">> deleting k3d cluster ${CLUSTER_NAME}"
  echo "   (if smoke CRs are still applied, their AWS resources are now ORPHANED"
  echo "    — run './scripts/local-dev.sh unsmoke' and wait for cleanup first)"
  k3d cluster delete "${CLUSTER_NAME}"
}

argo_bootstrap() {
  echo ">> installing Argo CD"
  kubectl create namespace argocd --dry-run=client -o yaml | kubectl apply -f -
  # Server-side apply: the ApplicationSet CRD is too large for client-side
  # last-applied annotations.
  kubectl apply -n argocd --server-side --force-conflicts \
    -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml >/dev/null
  echo ">> waiting for Argo CD server"
  kubectl -n argocd rollout status deployment/argocd-server --timeout=300s
  kubectl -n argocd rollout status deployment/argocd-repo-server --timeout=120s

  # Repo access: reuse the local SSH key if GitHub access is configured.
  local keyfile=""
  for f in "$HOME/.ssh/id_ed25519" "$HOME/.ssh/id_rsa"; do
    [ -f "$f" ] && keyfile="$f" && break
  done
  if [ -n "$keyfile" ]; then
    echo ">> registering repo credentials from ${keyfile}"
    kubectl -n argocd create secret generic repo-argo \
      --from-literal=type=git \
      --from-literal=url=git@github.com:your-org/infra-gitops.git \
      --from-file=sshPrivateKey="$keyfile" \
      --dry-run=client -o yaml | kubectl apply -f -
    kubectl -n argocd label secret repo-argo \
      argocd.argoproj.io/secret-type=repository --overwrite
  else
    echo "WARNING: no SSH key found in ~/.ssh; add repo credentials manually:" >&2
    echo "  argocd repo add git@github.com:your-org/infra-gitops.git --ssh-private-key-path <key>" >&2
  fi

  # Register every local-dev Application directly (app-of-apps included) so
  # the UI shows the konfig-konector app tree immediately — the git source
  # only matters when you click Sync. If helm-mode installed the chart
  # earlier, Argo matches the live resources by name and shows them under
  # the app.
  echo ">> applying local-dev Argo applications"
  kubectl apply -f "${CHART_DIR}/../clusters/local-dev/"

  local pw
  pw=$(kubectl -n argocd get secret argocd-initial-admin-secret \
        -o jsonpath='{.data.password}' 2>/dev/null | base64 -d || true)
  echo ""
  echo ">> Argo CD ready. UI access:"
  echo "     kubectl -n argocd port-forward svc/argocd-server 8443:443 &"
  echo "     open https://localhost:8443  (user: admin, password: ${pw:-<secret gone, use argocd admin initial-password>})"
  echo "   NOTE: Argo pulls clusters/local-dev/ from GitHub — push argo-repo"
  echo "   changes before expecting them to sync. Operator image updates still"
  echo "   work via './scripts/local-dev.sh deploy' (tag stays :dev)."
}

need docker; need kubectl; need helm; need aws
install_k3d

case "${1:-}" in
  up)      check_aws; cluster_up; build_and_import; credentials_secret; deploy ;;
  argo)    check_aws; cluster_up; build_and_import; credentials_secret; argo_bootstrap ;;
  deploy)  check_aws; build_and_import; credentials_secret
           kubectl -n "${NAMESPACE}" rollout restart deployment/konfig-controller 2>/dev/null || deploy ;;
  smoke)   smoke ;;
  unsmoke) unsmoke ;;
  status)  status ;;
  down)    down ;;
  *) grep '^#' "$0" | sed 's/^# \{0,1\}//' | head -32; exit 1 ;;
esac
