# Upgrade and roll back

The console initializes a genuinely empty database at startup. It opens an existing database only when its schema
matches the binary. Startup never upgrades an existing schema or resets an unrecognized database.

Schema versions are independent of application versions. `console_schema_migration` records each consecutive version,
its baseline identity, and its SQL checksum. Both backends start at version 1 with the same logical schema and separate
DDL. A changed checksum, missing history, legacy development schema, or newer version stops startup.

> [!IMPORTANT]
> This baseline replaces the three unreleased development migrations. Recreate those development databases deliberately.
> There is no supported upgrade from them. Do not delete a database that contains state you need to retain.

Keep the configuration encryption key for the lifetime of the database. Store its backup separately. Restored settings,
reader credentials, local passwords, authenticators, and recovery codes require the matching key.

## Inspect an upgrade

Read the release notes and verify the candidate binary, image, and chart as described in [releases](releasing.md).
Use the candidate binary with the same storage environment as the installed console:

```sh
./console --mode=live --schema=inspect
```

The command prints JSON with `current`, `supported`, `pending`, and `empty`. It reads schema history without creating
or changing it. For SQLite, set `CONSOLE_SQLITE_PATH` to the existing file. For PostgreSQL, set `CONSOLE_DATABASE_URL`.
The command requires no configuration key or Kubernetes credentials.

When `pending` is empty, upgrade the image and chart normally. A restart ends process-local sessions. Users sign in again.

## Upgrade an existing schema

Schema changes require an interruption. Stop every console process that uses this database before running the command.
The server and maintenance commands enforce the same ownership lock. SQLite uses a sibling `.lock` file; PostgreSQL
uses a session advisory lock scoped to the database and schema. Do not remove a lock file to bypass ownership.

1. Record the installed image, chart revision, values, and schema inspection result.
2. Stop the console. For Helm, scale its Deployment to zero and wait for its Pod to terminate:

   ```sh
   kubectl --namespace console-system scale deployment/openbao-operator-console --replicas=0
   kubectl --namespace console-system wait --for=delete pod \
     --selector=app.kubernetes.io/instance=openbao-operator-console --timeout=180s
   ```

3. Confirm that you have the matching encryption key in a separate backup.
4. Run the candidate binary with an unused backup path on durable storage:

   ```sh
   ./console --mode=live --schema=upgrade --backup=/backups/console-before-upgrade
   ```

5. Confirm that inspection reports the supported version and no pending changes. Start the candidate console and check
   `/readyz`, sign-in, enrolled clusters, and retained history.

The command acquires ownership, validates migration history, creates and validates the backup, and applies all pending
steps and their history records in one transaction. Failed SQL or validation rolls back the complete chain. The backup
remains available after failure. An existing backup file is never overwritten. There are no down migrations.

For SQLite, the command uses `VACUUM INTO` to produce a standalone database containing committed WAL data. It checks
integrity, foreign keys, and schema history in the backup. It checks integrity and foreign keys before committing the
upgrade. Use a local or block filesystem with SQLite locking support. WAL does not support NFS or sharing the database
across hosts. Do not copy only the main database file while a process is running.

For PostgreSQL, install `pg_dump` and `pg_restore` compatible with the server. The command exports a consistent snapshot,
dumps the console's current schema in custom format, and validates the complete archive through `pg_restore`. It does
not restore the archive into another server. Qualify restoration separately with your PostgreSQL backup tooling before
an operational upgrade. The runtime image does not contain these tools; run maintenance from an operator workstation
or a purpose-built maintenance image with the same database access and CA trust.

To create a backup without changing the schema, stop the console and run:

```sh
./console --mode=live --schema=backup --backup=/backups/console-state
```

For Kubernetes SQLite installations, run the maintenance binary in a temporary Pod that mounts the existing state PVC
at `/var/lib/console` and a separate backup volume at `/backups`. Set `CONSOLE_SQLITE_PATH=/var/lib/console/console.db`.
Use the candidate image and its non-root UID 65532. Stop and remove this Pod before restarting the Deployment. Copy the
backup to independent storage; another file on the state PVC does not protect against loss of that PVC.

## Roll back

If the schema version did not change, restore the previous binary or Helm revision and values.

If it changed, stop the console and restore the pre-upgrade backup before starting the previous binary. For SQLite,
restore to a new directory and select that file; do not leave old `-wal` or `-shm` files beside a replacement database.
Set file ownership for UID 65532 in Kubernetes and restrict access to the state directory and files. For PostgreSQL,
restore into an empty database:

```sh
pg_restore --no-owner --dbname "$EMPTY_DATABASE_URL" /backups/console-before-upgrade
```

Restore the matching encryption key. Update the database URL or PVC reference if it changed. A Helm rollback restores
the image and values; it does not restore database contents.

A restore returns access rules, identity settings, cluster enrollment, authentication replay state, and history to the
backup time. Changes after that time are lost. Renewed Kubernetes credentials may have expired. Invalidate or reset
local credentials if restoring their previous replay state would be unsafe for your environment.

## Migration maintenance

After the first public build that persists this baseline, including edge builds, released migrations are immutable.
Append a numbered migration only when stored structure must change. Implement equivalent behavior for both engines.
Keep every step transactional. Do not include transaction control, PostgreSQL `CREATE INDEX CONCURRENTLY`, or SQLite
`VACUUM` in migration SQL. Test populated upgrades, failure rollback, newer-version refusal, backup restoration, and
credential revisions with both backends. Changes to an encrypted payload format need a separate compatibility review.

There is no automatic conversion between SQLite and PostgreSQL. Select the backend before enrolling a console, or
start a separate console and enroll it deliberately.
