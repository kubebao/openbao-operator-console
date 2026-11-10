#!/usr/bin/env bash
# Creates, opens, or removes a disposable kind cluster that runs the OpenBao Operator, one
# Development OpenBao cluster, persistent SQLite state, and the console from this checkout.
# Usage: scripts/evaluation.sh up|open|down
# It touches only its own kind cluster and keeps its kubeconfig and secrets in .local/evaluation.
set -euo pipefail
cd "$(dirname "$0")/.."

CLUSTER=console-evaluation
STATE=.local/evaluation
OPERATOR_VERSION=${OPENBAO_OPERATOR_VERSION:-0.6.0}
IMAGE=openbao-operator-console:evaluation
PORT=${CONSOLE_EVALUATION_PORT:-8443}
export KUBECONFIG="$PWD/$STATE/kubeconfig"

log() { printf '\n==> %s\n' "$*"; }

require() {
  for tool in "$@"; do
    command -v "$tool" >/dev/null || { echo "$tool is required" >&2; exit 1; }
  done
}

# secret writes a random value to a private file once, so reruns keep the same database
# password, encryption key, and setup token.
secret() {
  local file="$STATE/$1"
  [ -s "$file" ] || (umask 077 && openssl rand -hex 32 > "$file")
}

apply_secret() {
  kubectl --namespace console-system create secret generic "$1" "${@:2}" --dry-run=client -o yaml | kubectl apply -f - >/dev/null
}

tls_secret() {
  if [ ! -s "$STATE/tls.crt" ]; then
    if command -v mkcert >/dev/null; then
      mkcert -cert-file "$STATE/tls.crt" -key-file "$STATE/tls.key" localhost 127.0.0.1 >/dev/null 2>&1
    else
      (umask 077 && openssl req -x509 -newkey rsa:2048 -nodes -days 30 -subj /CN=localhost \
        -addext subjectAltName=DNS:localhost,IP:127.0.0.1 \
        -keyout "$STATE/tls.key" -out "$STATE/tls.crt" >/dev/null 2>&1)
      touch "$STATE/self-signed"
    fi
  fi
  kubectl --namespace console-system create secret tls console-tls \
    --cert="$STATE/tls.crt" --key="$STATE/tls.key" --dry-run=client -o yaml | kubectl apply -f - >/dev/null
}

up() {
  require kind kubectl helm docker openssl
  mkdir -p "$STATE" && chmod 700 "$STATE"
  if ! kind get clusters 2>/dev/null | grep -qx "$CLUSTER"; then
    log "Creating the kind cluster $CLUSTER"
    kind create cluster --name "$CLUSTER" --kubeconfig "$KUBECONFIG" ${KIND_NODE_IMAGE:+--image "$KIND_NODE_IMAGE"}
  fi

  log "Installing the OpenBao Operator $OPERATOR_VERSION for namespace openbao-demo"
  kubectl create namespace openbao-demo --dry-run=client -o yaml | kubectl apply -f - >/dev/null
  helm upgrade --install openbao-operator oci://ghcr.io/kubebao/charts/openbao-operator \
    --version "$OPERATOR_VERSION" --namespace openbao-operator-system --create-namespace \
    --set tenancy.mode=single --set tenancy.targetNamespace=openbao-demo --wait --timeout 5m
  # kindnet enforces NetworkPolicy after Service address translation, so OpenBao needs an
  # egress rule for the API server endpoint itself, which differs per kind cluster.
  local endpoint
  endpoint=$(kubectl get endpointslice --namespace default -l kubernetes.io/service-name=kubernetes \
    -o jsonpath='{.items[0].endpoints[0].addresses[0]}')
  kubectl patch --local -f examples/evaluation/openbao-cluster.yaml --type=merge -o yaml \
    --patch "{\"spec\":{\"network\":{\"apiServerEndpointIPs\":[\"$endpoint\"]}}}" | kubectl apply -f -

  log "Building the console image from this checkout"
  make image IMAGE="$IMAGE"
  kind load docker-image "$IMAGE" --name "$CLUSTER"

  log "Preparing persistent SQLite state"
  kubectl create namespace console-system --dry-run=client -o yaml | kubectl apply -f - >/dev/null
  secret config-key
  secret setup-token
  apply_secret console-runtime --from-file=CONFIG_KEY="$STATE/config-key" \
    --from-file=SETUP_TOKEN="$STATE/setup-token"

  log "Installing the console"
  tls_secret
  helm upgrade --install openbao-operator-console charts/openbao-operator-console \
    --namespace console-system --values examples/evaluation/console-values.yaml \
    --set image.repository=openbao-operator-console --set image.tag=evaluation --set image.pullPolicy=Never \
    --set "publicURL=https://localhost:$PORT" --wait --timeout 3m >/dev/null
  # A rebuilt image keeps its tag, so restart to run it.
  kubectl --namespace console-system rollout restart deployment/openbao-operator-console >/dev/null
  kubectl --namespace console-system rollout status deployment/openbao-operator-console --timeout=3m

  log "Waiting for the OpenBao cluster (up to 10 minutes)"
  kubectl --namespace openbao-demo wait --for=condition=Available openbaocluster/evaluation --timeout=10m ||
    echo "The OpenBao cluster is not available yet. The console shows its progress." >&2

  cat <<EOF

The evaluation is ready. Run:

  make evaluation-open

Then open https://localhost:$PORT, enter the setup token from $STATE/setup-token, and choose
Local administrator. The fleet shows the evaluation OpenBao cluster after the first collection.
EOF
}

open_console() {
  require kubectl
  [ -s "$KUBECONFIG" ] || { echo "Run make evaluation first." >&2; exit 1; }
  if [ -e "$STATE/self-signed" ]; then
    echo "The certificate is self-signed; accept the browser warning for localhost. Install mkcert for a trusted one."
  fi
  echo "Setup token file: $STATE/setup-token"
  echo "Console: https://localhost:$PORT (stop with Ctrl-C)"
  kubectl --namespace console-system port-forward service/openbao-operator-console "$PORT:8090"
}

down() {
  require kind
  kind delete cluster --name "$CLUSTER" --kubeconfig "$KUBECONFIG" 2>/dev/null || true
  rm -rf "$STATE"
  echo "Removed the evaluation cluster and $STATE."
}

case "${1:-}" in
  up) up ;;
  open) open_console ;;
  down) down ;;
  *)
    echo "usage: $0 up|open|down" >&2
    exit 2
    ;;
esac
