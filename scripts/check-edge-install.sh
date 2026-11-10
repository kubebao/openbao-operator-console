#!/usr/bin/env bash
# Installs the packaged edge chart and the exact OCI image in a disposable kind cluster.
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/edge-tools.env

candidate="$(cd "$1" && pwd)"
version="$(jq -er .version "${candidate}/metadata.json")"
commit="$(jq -er .commit "${candidate}/metadata.json")"
digest="$(jq -er .image.digest "${candidate}/metadata.json")"
work="$(mktemp -d)"
cluster="console-edge-$(date +%s)-$$"
registry="${cluster}-registry"
forward_pid=""
created_cluster=false
cleanup() {
  if [[ -n "${forward_pid}" ]]; then kill "${forward_pid}" 2>/dev/null || true; fi
  if [[ "${created_cluster}" == true ]]; then kind delete cluster --name "${cluster}" --kubeconfig "${work}/kubeconfig"; fi
  docker rm -f -v "${registry}" >/dev/null 2>&1 || true
  rm -rf "${work}"
}
trap cleanup EXIT

# Check the native archive, including its embedded version and source commit.
platform="$(go env GOOS)_$(go env GOARCH)"
archive="openbao-operator-console_${version}_${platform}"
tar -xzf "${candidate}/${archive}.tar.gz" -C "${work}"
test "$("${work}/${archive}/console" --version)" = "openbao-operator-console ${version} (commit ${commit})"

docker run -d --name "${registry}" -p 127.0.0.1::5000 "${REGISTRY_IMAGE}" >/dev/null
address="localhost:$(docker port "${registry}" 5000/tcp | sed 's/.*://')"
image="${address}/openbao-operator-console"
regctl --host "reg=${address},tls=disabled" image import "${image}:${version}" "${candidate}/image.tar"
test "$(regctl --host "reg=${address},tls=disabled" image digest "${image}:${version}")" = "${digest}"

created_cluster=true
kind create cluster --name "${cluster}" --image "${KIND_IMAGE}" --kubeconfig "${work}/kubeconfig" --wait 120s
docker network connect kind "${registry}"
# Configure only the new node's registry mirror. Existing clusters are unchanged.
for node in $(kind get nodes --name "${cluster}"); do
  docker exec "${node}" mkdir -p "/etc/containerd/certs.d/${address}"
  printf '[host."http://%s:5000"]\n' "${registry}" |
    docker exec -i "${node}" sh -c 'cat > "$1"' sh "/etc/containerd/certs.d/${address}/hosts.toml"
done

export KUBECONFIG="${work}/kubeconfig"
kubectl create namespace console-edge
umask 077
openssl rand -hex 32 > "${work}/key"
openssl rand -hex 32 > "${work}/setup-token"
printf '%s' 'postgres://console:console-edge-ci-only@postgres:5432/console_test?sslmode=disable' > "${work}/database-url"
kubectl -n console-edge create secret generic console-runtime --from-file=CONFIG_KEY="${work}/key" \
  --from-file=SETUP_TOKEN="${work}/setup-token" --from-file=DATABASE_URL="${work}/database-url"
kubectl -n console-edge apply -f - <<'YAML'
apiVersion: v1
kind: Service
metadata:
  name: postgres
spec:
  selector: {app: postgres}
  ports: [{port: 5432}]
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: postgres
spec:
  replicas: 1
  selector:
    matchLabels: {app: postgres}
  template:
    metadata:
      labels: {app: postgres}
    spec:
      containers:
        - name: postgres
          image: postgres:18-alpine@sha256:77f585114c32fbca283dc835b0596f4e52b51b4c6662d7810b2f4084f60a1873
          env:
            - {name: POSTGRES_DB, value: console_test}
            - {name: POSTGRES_USER, value: console}
            - {name: POSTGRES_PASSWORD, value: console-edge-ci-only}
          readinessProbe:
            exec:
              command: [pg_isready, -U, console, -d, console_test]
YAML
kubectl -n console-edge rollout status deployment/postgres --timeout=180s
helm install console "${candidate}/openbao-operator-console-${version}.tgz" --namespace console-edge \
  --set existingSecret=console-runtime --set publicURL=https://console.example.test \
  --set database.sslMode=disable --set image.repository="${image}" --wait --timeout=180s
test "$(kubectl -n console-edge get deployment console-openbao-operator-console -o jsonpath='{.spec.template.spec.containers[0].image}')" = "${image}@${digest}"
kubectl -n console-edge port-forward service/console-openbao-operator-console :8090 --address 127.0.0.1 > "${work}/forward.log" 2>&1 &
forward_pid=$!
for _ in {1..30}; do
  if grep -q 'Forwarding from 127.0.0.1:' "${work}/forward.log"; then break; fi
  sleep 1
done
port="$(sed -nE 's/^Forwarding from 127.0.0.1:([0-9]+).*/\1/p' "${work}/forward.log" | head -n 1)"
test -n "${port}"
test "$(curl --silent --show-error --fail -o /dev/null -w '%{http_code}' "http://127.0.0.1:${port}/readyz")" = 204
curl --silent --show-error --fail -D "${work}/headers" "http://127.0.0.1:${port}/" -o "${work}/index.html"
grep -qi '^Content-Security-Policy:' "${work}/headers"
grep -q '<html' "${work}/index.html"
printf 'Qualified packaged edge chart %s with image %s\n' "${version}" "${digest}"
