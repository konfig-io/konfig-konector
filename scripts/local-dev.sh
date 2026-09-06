#!/usr/bin/env bash
# Local bootstrap for konfig-konector: k3s (k3d) + Argo CD + the operator +
# the free-tier smoke test, all from this repository, against a real AWS
# account. Nothing is pushed to GitHub or any public registry: images and
# charts go to a registry that runs inside the k3d cluster.
#
# Usage:
#   ./scripts/local-dev.sh up          # cluster + registry + Argo CD + operator (via Argo)
#   ./scripts/local-dev.sh deploy      # rebuild the operator image and restart it
#   ./scripts/local-dev.sh chart       # repackage + push the operator chart, bump Argo
#   ./scripts/local-dev.sh bundles S.. # install generated CRD bundles (e.g. logs ec2 iam)
#   ./scripts/local-dev.sh smoke [PROVIDER]  # install the konfig-smoke chart as an Argo app,
#                                            # optionally scoped to an AWSProvider (multi-account proof)
#   ./scripts/local-dev.sh status      # Ready condition of every konfig CR
#   ./scripts/local-dev.sh unsmoke     # delete the smoke app + namespace (deletes AWS resources)
#   ./scripts/local-dev.sh argo-ui     # port-forward Argo CD and print the login
#   ./scripts/local-dev.sh down        # delete the cluster (run unsmoke first!)
#
# The cluster uses its own kubeconfig (${KUBECONFIG_FILE}) so your default
# context is never touched. Every command re-exports it.
#
# AWS credentials come from the ambient chain (AWS_PROFILE, SSO, env) and are
# materialised into a Secret the operator reads because k3s has no EKS Pod
# Identity. Session credentials expire: rerun `deploy` to refresh them.
#   AWS_PROFILE=scratch ./scripts/local-dev.sh up
#
# Argo CD extras this script installs:
#   - an OCI Helm repo entry for the in-cluster registry (plain HTTP)
#   - a health check for every aws.konfig.io kind that maps the Ready
#     condition to Healthy/Progressing, so CRs show real status in the UI
#     (see argocd/health-customizations.yaml)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CLUSTER_NAME="${CLUSTER_NAME:-konfig}"
REGISTRY_NAME="${CLUSTER_NAME}-registry"
REGISTRY_PORT="${REGISTRY_PORT:-5111}"
# host-side and in-cluster addresses of the same registry
REGISTRY_HOST="localhost:${REGISTRY_PORT}"
REGISTRY_CLUSTER="${REGISTRY_NAME}:${REGISTRY_PORT}"   # containerd mirror (image pulls)
REGISTRY_ARGO="${REGISTRY_NAME}:5000"                   # Argo repo-server reaches the container port
NAMESPACE="konfig-system"
SMOKE_NAMESPACE="smoke"
AWS_REGION="${AWS_REGION:-us-east-1}"
KUBECONFIG_FILE="${KUBECONFIG_FILE:-/tmp/${CLUSTER_NAME}-kubeconfig}"
export KUBECONFIG="${KUBECONFIG_FILE}"
IMAGE_TAG="${IMAGE_TAG:-dev}"

need() { command -v "$1" >/dev/null || { echo "missing required tool: $1" >&2; exit 1; }; }

install_k3d() {
  command -v k3d >/dev/null && return
  echo ">> installing k3d to ~/.local/bin"
  mkdir -p "$HOME/.local/bin"
  curl -sSL -o "$HOME/.local/bin/k3d" https://github.com/k3d-io/k3d/releases/latest/download/k3d-linux-amd64
  chmod +x "$HOME/.local/bin/k3d"
  export PATH="$HOME/.local/bin:$PATH"
}

check_aws() {
  echo ">> verifying AWS credentials"
  local ident
  if ! ident=$(aws sts get-caller-identity --output text --query 'Arn' 2>&1); then
    echo "AWS credentials are not valid in this shell: $ident" >&2
    echo "Set AWS_PROFILE (or run aws sso login / aws configure) and retry." >&2
    exit 1
  fi
  echo "   $ident"
}

chart_version() { grep '^version:' "${REPO_ROOT}/helm/$1/Chart.yaml" | awk '{print $2}'; }

cluster_up() {
  if k3d cluster list 2>/dev/null | grep -q "^${CLUSTER_NAME}\b"; then
    echo ">> cluster ${CLUSTER_NAME} already exists"
  else
    echo ">> creating k3d cluster ${CLUSTER_NAME} with registry ${REGISTRY_HOST}"
    k3d cluster create "${CLUSTER_NAME}" --servers 1 --agents 0 \
      --registry-create "${REGISTRY_NAME}:0.0.0.0:${REGISTRY_PORT}" --wait --timeout 180s
  fi
  k3d kubeconfig get "${CLUSTER_NAME}" > "${KUBECONFIG_FILE}"
  echo ">> kubeconfig written to ${KUBECONFIG_FILE}"
}

build_and_push() {
  echo ">> building ${REGISTRY_HOST}/konfig-konector:${IMAGE_TAG}"
  docker build -q -t "${REGISTRY_HOST}/konfig-konector:${IMAGE_TAG}" "${REPO_ROOT}"
  docker push -q "${REGISTRY_HOST}/konfig-konector:${IMAGE_TAG}"
}

push_chart() { # push_chart <chart-dir-name>
  local ver; ver=$(chart_version "$1")
  echo ">> packaging + pushing chart $1 ${ver}"
  local tmp; tmp=$(mktemp -d)
  helm package "${REPO_ROOT}/helm/$1" -d "$tmp" >/dev/null
  helm push "$tmp/$1-${ver}.tgz" "oci://${REGISTRY_HOST}/charts" --plain-http >/dev/null
  rm -rf "$tmp"
}

credentials_secret() {
  echo ">> materialising AWS credentials into ${NAMESPACE}/aws-local-credentials"
  kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f - >/dev/null
  local creds
  creds=$(aws configure export-credentials --format env 2>/dev/null) || {
    echo "aws configure export-credentials failed (needs awscli v2.9+)" >&2; exit 1; }
  # shellcheck disable=SC2046
  eval "$creds"
  kubectl -n "${NAMESPACE}" create secret generic aws-local-credentials \
    --from-literal=AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID}" \
    --from-literal=AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY}" \
    ${AWS_SESSION_TOKEN:+--from-literal=AWS_SESSION_TOKEN="${AWS_SESSION_TOKEN}"} \
    --dry-run=client -o yaml | kubectl apply -f - >/dev/null
}

argo_install() {
  echo ">> installing Argo CD"
  kubectl create namespace argocd --dry-run=client -o yaml | kubectl apply -f - >/dev/null
  # Server-side apply: the ApplicationSet CRD is too large for client-side annotations.
  kubectl apply -n argocd --server-side --force-conflicts \
    -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml >/dev/null
  kubectl -n argocd rollout status deployment/argocd-server --timeout=300s >/dev/null
  kubectl -n argocd rollout status deployment/argocd-repo-server --timeout=180s >/dev/null

  echo ">> registering the in-cluster OCI chart registry with Argo CD"
  # insecureOCIForceHttp makes repo-server pull over plain HTTP. Do NOT also
  # set insecure: "true": that adds --insecure-skip-tls-verify and helm then
  # insists on https.
  kubectl apply -f - >/dev/null <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: konfig-registry-helm
  namespace: argocd
  labels: {argocd.argoproj.io/secret-type: repository}
stringData:
  name: konfig-registry
  type: helm
  url: ${REGISTRY_ARGO}/charts
  enableOCI: "true"
  insecureOCIForceHttp: "true"
EOF

  echo ">> installing Ready-condition health checks for aws.konfig.io kinds"
  kubectl -n argocd patch configmap argocd-cm --type merge \
    --patch-file "${REPO_ROOT}/argocd/health-customizations.yaml" >/dev/null
  kubectl -n argocd rollout restart statefulset/argocd-application-controller >/dev/null
}

operator_app() {
  local ver; ver=$(chart_version konfig-konector)
  echo ">> creating/updating Argo Application konfig-konector (chart ${ver})"
  kubectl apply -f - >/dev/null <<EOF
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: konfig-konector
  namespace: argocd
spec:
  project: default
  destination: {server: https://kubernetes.default.svc, namespace: ${NAMESPACE}}
  syncPolicy:
    automated: {prune: true, selfHeal: true}
    syncOptions: [ServerSideApply=true, CreateNamespace=true]
  source:
    repoURL: ${REGISTRY_ARGO}/charts
    chart: konfig-konector
    targetRevision: "${ver}"
    helm:
      valuesObject:
        image: {repository: ${REGISTRY_CLUSTER}/konfig-konector, tag: ${IMAGE_TAG}}
        aws: {region: ${AWS_REGION}, credentialsSecret: aws-local-credentials}
        leaderElection: false
        resources:
          limits: {cpu: "1", memory: 1Gi}
          requests: {cpu: 100m, memory: 256Mi}
EOF
  kubectl -n argocd annotate application konfig-konector argocd.argoproj.io/refresh=hard --overwrite >/dev/null
  echo ">> waiting for the operator"
  for _ in $(seq 1 60); do
    kubectl -n "${NAMESPACE}" rollout status deployment/konfig-controller --timeout=5s >/dev/null 2>&1 && break
    sleep 5
  done
  kubectl -n "${NAMESPACE}" get pods
}

bundles() {
  [ $# -gt 0 ] || { echo "usage: $0 bundles <service> [service...]  (see config/crd/cloudcontrol/README.md)" >&2; exit 1; }
  for s in "$@"; do
    echo ">> installing generated CRD bundle: $s"
    kubectl apply --server-side --force-conflicts -f "${REPO_ROOT}/config/crd/cloudcontrol/${s}.yaml" >/dev/null
  done
  echo ">> restarting the operator so it starts controllers for the new kinds"
  kubectl -n "${NAMESPACE}" rollout restart deployment/konfig-controller >/dev/null
  kubectl -n "${NAMESPACE}" rollout status deployment/konfig-controller --timeout=300s >/dev/null
}

smoke() {
  local provider="${1:-}"
  local acct; acct=$(aws sts get-caller-identity --query Account --output text)
  local ami; ami=$(aws ssm get-parameter --name /aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64 \
    --query Parameter.Value --output text --region "${AWS_REGION}")
  # The smoke chart uses generated kinds from these services.
  bundles iam sqs sns ec2 eventschemas appconfig resourcegroups glue codedeploy xray logs
  push_chart konfig-smoke
  local ver; ver=$(chart_version konfig-smoke)
  echo ">> creating Argo Application konfig-smoke (account ${acct}, ${AWS_REGION})"
  kubectl apply -f - >/dev/null <<EOF
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: konfig-smoke
  namespace: argocd
spec:
  project: default
  destination: {server: https://kubernetes.default.svc, namespace: ${SMOKE_NAMESPACE}}
  syncPolicy:
    automated: {prune: false, selfHeal: false}
    syncOptions: [ServerSideApply=true, CreateNamespace=true]
  source:
    repoURL: ${REGISTRY_ARGO}/charts
    chart: konfig-smoke
    targetRevision: "${ver}"
    helm:
      valuesObject: {accountId: "${acct}", region: ${AWS_REGION}, amiId: ${ami}, providerRef: "${provider}"}
EOF
  [ -n "$provider" ] && echo ">> every smoke resource is scoped to AWSProvider ${provider}; check status.awsProvider on the CRs"
  echo ">> watch convergence in the Argo UI (argo-ui) or with: $0 status"
}

unsmoke() {
  echo ">> deleting the smoke app and namespace (the operator deletes the AWS resources)"
  kubectl -n argocd delete application konfig-smoke --ignore-not-found >/dev/null
  kubectl delete namespace "${SMOKE_NAMESPACE}" --ignore-not-found --timeout=600s
  echo ">> done; verify nothing is left with: aws resourcegroupstaggingapi get-resources --tag-filters Key=Name,Values=kk-smoke"
}

status() {
  local kinds
  kinds=$(kubectl api-resources --api-group=aws.konfig.io -o name 2>/dev/null | sort)
  [ -n "$kinds" ] || { echo "no aws.konfig.io CRDs installed"; exit 1; }
  kubectl get "$(echo "$kinds" | tr '\n' ',' | sed 's/,$//')" -A \
    -o custom-columns='KIND:.kind,NAMESPACE:.metadata.namespace,NAME:.metadata.name,READY:.status.conditions[?(@.type=="Ready")].status,REASON:.status.conditions[?(@.type=="Ready")].reason,MESSAGE:.status.conditions[?(@.type=="Ready")].message' \
    2>/dev/null | cut -c1-200
}

argo_ui() {
  local pw
  pw=$(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' 2>/dev/null | base64 -d || true)
  echo ">> Argo CD: https://localhost:8443  user: admin  password: ${pw:-<use: argocd admin initial-password>}"
  echo ">> press Ctrl-C to stop the port-forward"
  kubectl -n argocd port-forward svc/argocd-server 8443:443 --address 127.0.0.1
}

down() {
  echo ">> deleting k3d cluster ${CLUSTER_NAME}"
  echo "   (if the smoke app is still installed its AWS resources are now ORPHANED;"
  echo "    run '$0 unsmoke' and wait first)"
  k3d cluster delete "${CLUSTER_NAME}"
  rm -f "${KUBECONFIG_FILE}"
}

need docker; need kubectl; need helm; need aws
install_k3d

case "${1:-}" in
  up)       check_aws; cluster_up; build_and_push; push_chart konfig-konector; credentials_secret; argo_install; operator_app ;;
  deploy)   check_aws; build_and_push; credentials_secret
            kubectl -n "${NAMESPACE}" rollout restart deployment/konfig-controller
            kubectl -n "${NAMESPACE}" rollout status deployment/konfig-controller --timeout=300s ;;
  chart)    push_chart konfig-konector; operator_app ;;
  bundles)  shift; bundles "$@" ;;
  smoke)    check_aws; smoke "${2:-}" ;;
  unsmoke)  unsmoke ;;
  status)   status ;;
  argo-ui)  argo_ui ;;
  down)     down ;;
  *) grep '^#' "$0" | sed 's/^# \{0,1\}//' | head -30; exit 1 ;;
esac
