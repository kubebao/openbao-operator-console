#!/usr/bin/env bash
# Use the CI service's matching PostgreSQL tools, with backup files on the runner.
set -euo pipefail
mkdir -p "$1"
cat > "$1/pg_dump" <<'DUMP'
#!/usr/bin/env bash
set -euo pipefail
args=()
output=""
while (($#)); do
  if [[ "$1" == --file ]]; then output="$2"; shift 2; else args+=("$1"); shift; fi
done
test -n "$output"
exec docker exec -i -e PGPASSWORD "$CONSOLE_POSTGRES_CONTAINER" pg_dump "${args[@]}" > "$output"
DUMP
cat > "$1/pg_restore" <<'RESTORE'
#!/usr/bin/env bash
set -euo pipefail
archive="${!#}"
args=("${@:1:$#-1}")
exec docker exec -i -e PGPASSWORD "$CONSOLE_POSTGRES_CONTAINER" pg_restore "${args[@]}" < "$archive"
RESTORE
chmod +x "$1/pg_dump" "$1/pg_restore"
