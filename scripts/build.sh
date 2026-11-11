#!/usr/bin/env bash
# Builds the application with embedded web assets. Requires Go and pnpm, without make.
set -euo pipefail
cd "$(dirname "$0")/.."

version="${1:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
commit="${2:-$(git rev-parse HEAD)}"
buildinfo=github.com/kubebao/openbao-operator-console/internal/buildinfo

pnpm --dir web build
cp -R web/dist/. internal/httpapi/assets/
mkdir -p bin
CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -X ${buildinfo}.Version=${version} -X ${buildinfo}.Commit=${commit}" \
  -o bin/console ./cmd/console
