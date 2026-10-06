#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT
cp -R internal/contract "$scratch/contract"
cp -R internal/store/database "$scratch/database"
cp web/src/api/generated.ts "$scratch/generated.ts"
make generate
diff -ru "$scratch/contract" internal/contract
diff -ru "$scratch/database" internal/store/database
diff -u "$scratch/generated.ts" web/src/api/generated.ts
