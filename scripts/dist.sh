#!/usr/bin/env bash
# Builds release archives of the console binary with embedded web assets.
# Usage: scripts/dist.sh <version> <commit> <os/arch>...
set -euo pipefail
cd "$(dirname "$0")/.."

version="$1"
commit="$2"
shift 2
buildinfo=github.com/kubebao/openbao-operator-console/internal/buildinfo

pnpm --dir web build
cp -R web/dist/. internal/httpapi/assets/

mkdir -p dist
work="$(mktemp -d)"
trap 'rm -rf "${work}"' EXIT
for platform in "$@"; do
  os="${platform%/*}"
  arch="${platform#*/}"
  name="openbao-operator-console_${version}_${os}_${arch}"
  mkdir -p "${work}/${name}"
  CGO_ENABLED=0 GOOS="${os}" GOARCH="${arch}" go build -trimpath \
    -ldflags "-s -w -X ${buildinfo}.Version=${version} -X ${buildinfo}.Commit=${commit}" \
    -o "${work}/${name}/console" ./cmd/console
  cp LICENSE THIRD_PARTY_LICENSES "${work}/${name}/"
  tar -C "${work}" -czf "dist/${name}.tar.gz" "${name}"
  echo "dist/${name}.tar.gz"
done
