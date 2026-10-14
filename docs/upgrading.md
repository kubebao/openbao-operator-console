# Upgrade and roll back

This page describes how to upgrade a Helm installation of the console and how to return to the previous version. The
examples use the release `openbao-operator-console` in the namespace `console-system`, the runtime Secret
`console-runtime`, and a values file `console-values.yaml`.

## How upgrades change the database

At startup, the console applies the pending numbered schema migrations of its version in one transaction and records
each version in `console_schema_migration`. A failed migration rolls back completely, and the console exits without
serving. A console refuses to start against a database with a newer schema version than it supports. A Helm rollback
therefore restores the previous image and values, but not the previous schema. Rolling back across a schema change
requires the database backup taken before the upgrade.

The encryption key in `CONFIG_KEY` does not change during an upgrade. Keep the same key for every version and every
restored backup.

Stored configurations from before issuer-scoped access bind their existing users and groups to the identity provider
already stored with them. Loading this format preserves the configuration revision, reader credentials, and change
history. The next settings save persists the issuer binding. A declared binding that differs from the stored provider
is rejected.

The pagination migration replaces the observation and activity indexes without deleting their records. Older activity
records remain visible as earlier activity with unavailable structured details. The console does not infer structured
changes from their old text.

## Before you upgrade

1. Read the release notes in [CHANGELOG.md](../CHANGELOG.md) for breaking changes to configuration, Helm values, or
   reader RBAC. Releases before 1.0 can contain breaking changes in minor versions.
2. Verify the new image and chart as described in [releases](releasing.md#verify-a-release).
3. Record the installed revision and values:

   ```sh
   helm --namespace console-system history openbao-operator-console
   helm --namespace console-system get values openbao-operator-console > previous-values.yaml
   ```

4. Record the current schema version, so that you know later whether the upgrade migrated the database:

   ```sh
   psql "$DATABASE_URL" --command 'SELECT max(version) FROM console_schema_migration'
   ```

5. Back up PostgreSQL. Use your database provider's backup, such as a CloudNativePG `Backup`, or `pg_dump`:

   ```sh
   umask 077
   pg_dump --format=custom --file=console-before-upgrade.dump "$DATABASE_URL"
   ```

6. Confirm that you have a copy of the encryption key from the runtime Secret, stored separately from the database
   backup. A backup cannot be read without it.

## Upgrade

Upgrade the release with the new chart version and your values file:

```sh
helm upgrade openbao-operator-console oci://ghcr.io/kubebao/charts/openbao-operator-console \
  --version <new version> --namespace console-system --values console-values.yaml --wait
```

The chart uses the `Recreate` strategy, so the old Pod stops before the new one starts. Expect a short interruption.
Sessions are held in process memory, so users sign in again afterwards. Check the result:

```sh
kubectl --namespace console-system rollout status deployment/openbao-operator-console
kubectl --namespace console-system logs deployment/openbao-operator-console
curl --fail https://console.example.com/readyz
```

If the Pod restarts, read the log of the failed container with `kubectl logs --previous`. A startup error is the last
line before the process exits. See [troubleshooting](troubleshooting.md).

## Roll back

First check whether the upgrade changed the schema version, with the query from step 4.

**The schema version is unchanged.** Roll back the release:

```sh
helm --namespace console-system rollback openbao-operator-console <previous revision> --wait
```

**The schema version increased.** The previous version refuses the migrated database and reports both versions in its
startup error. Restore the pre-upgrade backup into an empty database, then roll back:

1. Stop the console:

   ```sh
   kubectl --namespace console-system scale deployment/openbao-operator-console --replicas=0
   ```

2. Restore the backup into an empty database. Recreate the database, or restore into a new database or CloudNativePG
   `Cluster`, rather than restoring over the migrated schema:

   ```sh
   pg_restore --no-owner --dbname "$EMPTY_DATABASE_URL" console-before-upgrade.dump
   ```

3. If the database URL changed, update `DATABASE_URL` in the runtime Secret or `database.urlSecret` in
   `previous-values.yaml`.
4. Roll back the release, which also restores the replica count:

   ```sh
   helm --namespace console-system rollback openbao-operator-console <previous revision> --wait
   ```

A restore also returns access rules, identity settings, cluster enrollment, and history to their state at backup time.
Changes made in **Settings** after the backup are lost; check **Settings → Changes** and repeat them if needed.
Kubernetes credentials that were renewed after the backup may have expired; reconnect affected clusters.

If the migration itself failed, the database is unchanged. Roll back the release without restoring the backup.

## Pre-release charts

Charts from before the first release used different selector labels and resource names. Kubernetes does not allow a
Deployment's selector to change, so `helm upgrade` from such a chart can fail. Uninstall the old release and install the
new chart with the same runtime Secret and database; `helm uninstall` does not touch PostgreSQL.
