#!/usr/bin/env bash
# Builds the archives and chart around an OCI image exported by Buildx.
set -euo pipefail
cd "$(dirname "$0")/.."
candidate="$(cd "$1" && pwd)"
identity=(--repository "${GITHUB_REPOSITORY}" --sha "${GITHUB_SHA}" --run-id "${GITHUB_RUN_ID}"
  --run-attempt "${GITHUB_RUN_ATTEMPT}" --source-ref "${GITHUB_REF}")
version="$(python3 scripts/edge-bundle.py identity "${candidate}" "${identity[@]}" | sed -n 's/^version=//p')"
DIST_DIR="${candidate}" make dist VERSION="${version}" COMMIT="${GITHUB_SHA}"
cp LICENSE THIRD_PARTY_LICENSES "${candidate}/"
python3 scripts/edge-bundle.py create "${candidate}" "${identity[@]}"
./scripts/check-edge-install.sh "${candidate}"
python3 scripts/edge-bundle.py validate "${candidate}" "${identity[@]}"
tar -C "${candidate}" -cf "${candidate}.tar" .
