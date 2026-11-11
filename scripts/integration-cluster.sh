#!/usr/bin/env bash
# Creates or removes the disposable kind cluster for the Kubernetes integration tests.
# Usage: scripts/integration-cluster.sh up|down
# "up" installs the OpenBao Operator in single-tenant mode, creates the console-bao sample in
# console-sample, waits until the operator reports it Available, applies the reader RBAC, and
# issues a short-lived reader kubeconfig. It touches only its own kind cluster, console-dev,
# and keeps both kubeconfigs in .local.
set -euo pipefail
cd "$(dirname "$0")/.."

# shellcheck source=scripts/edge-tools.env
. scripts/edge-tools.env

CLUSTER=console-dev
OPERATOR_VERSION=${OPENBAO_OPERATOR_VERSION:-0.6.1}
ADMIN_KUBECONFIG="$PWD/.local/kubeconfig"
READER_KUBECONFIG="$PWD/.local/reader-kubeconfig"
export KUBECONFIG="$ADMIN_KUBECONFIG"

log() { printf '\n==> %s\n' "$*"; }

require() {
  for tool in "$@"; do
    command -v "$tool" >/dev/null || { echo "$tool is required" >&2; exit 1; }
  done
}

up() {
  require kind kubectl helm python3
  mkdir -p .local
  if kind get clusters | grep -qx "$CLUSTER"; then
    log "Reusing kind cluster $CLUSTER"
    kind export kubeconfig --name "$CLUSTER" --kubeconfig "$ADMIN_KUBECONFIG" >/dev/null
  else
    log "Creating kind cluster $CLUSTER"
    kind create cluster --name "$CLUSTER" --image "$KIND_IMAGE" --kubeconfig "$ADMIN_KUBECONFIG"
  fi

  log "Installing the OpenBao Operator $OPERATOR_VERSION for namespace console-sample"
  kubectl create namespace console-sample --dry-run=client -o yaml | kubectl apply -f - >/dev/null
  # The release name and namespace match examples/reader-rbac.yaml and the integration tests.
  helm upgrade --install console-operator oci://ghcr.io/kubebao/charts/openbao-operator \
    --version "$OPERATOR_VERSION" --namespace console-system --create-namespace \
    --set tenancy.mode=single --set tenancy.targetNamespace=console-sample --wait --timeout 5m

  # kindnet enforces NetworkPolicy after Service address translation. Without an egress rule
  # for the API server endpoint itself, self-initialization cannot reach the API server and
  # the sample stays Initializing with APIServerNetworkReady=Unknown.
  log "Creating the console-bao sample"
  local endpoint
  endpoint=$(kubectl get endpointslice --namespace default -l kubernetes.io/service-name=kubernetes \
    -o jsonpath='{.items[0].endpoints[0].addresses[0]}')
  [ -n "$endpoint" ] || { echo "could not read the API server endpoint address" >&2; exit 1; }
  kubectl patch --local -f examples/kind-cluster.yaml --type=merge -o yaml \
    --patch "{\"spec\":{\"network\":{\"apiServerEndpointIPs\":[\"$endpoint\"]}}}" | kubectl apply -f -

  log "Waiting for console-bao to become Available (up to 10 minutes)"
  if ! kubectl --namespace console-sample wait --for=condition=Available openbaocluster/console-bao --timeout=10m; then
    kubectl --namespace console-sample get openbaocluster console-bao \
      -o jsonpath='{range .status.conditions[*]}{.type}={.status} ({.reason}){"\n"}{end}' >&2
    echo "console-bao did not become Available; the integration test would fail" >&2
    exit 1
  fi

  log "Applying reader RBAC and issuing a one-hour reader credential"
  kubectl apply -f examples/reader-rbac.yaml >/dev/null
  python3 scripts/reader-kubeconfig.py \
    --admin-kubeconfig="$ADMIN_KUBECONFIG" --context="kind-$CLUSTER" \
    --namespace=console-system --service-account=console-reader \
    --duration=1h --output="$READER_KUBECONFIG"

  cat <<EOF

The integration cluster is ready. Run:

  export CONSOLE_TEST_KUBECONFIG="$READER_KUBECONFIG"
  make integration

Remove it with: make integration-cluster-down
EOF
}

down() {
  require kind
  kind delete cluster --name "$CLUSTER" --kubeconfig "$ADMIN_KUBECONFIG"
  rm -f "$READER_KUBECONFIG"
}

case "${1:-}" in
  up) up ;;
  down) down ;;
  *) echo "usage: $0 up|down" >&2; exit 2 ;;
esac
