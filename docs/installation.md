# Install the console

This guide installs the console on Kubernetes with the Helm chart in `charts/openbao-operator-console`. The console runs
as one replica, stores its data in an external PostgreSQL database, and authenticates users through OpenID Connect.

Releases publish a signed image to `ghcr.io/kubebao/openbao-operator-console` and the chart to
`oci://ghcr.io/kubebao/charts/openbao-operator-console`; see [releases](releasing.md#verify-a-release) to verify them.
No release has been published yet. Until then, build the image from source and install the chart from your checkout.

The examples install the release `openbao-operator-console` in the namespace `console-system`. With that release name,
the Deployment, Service, and ServiceAccount are named `openbao-operator-console`.

## Prerequisites

- A Kubernetes cluster for the console. It can be one of the clusters that runs the OpenBao Operator.
- A PostgreSQL database and a user that can create tables in it. CI tests PostgreSQL 18.
- An OpenID Connect provider that supports the authorization code flow with PKCE (`S256`). See [Keycloak](keycloak.md)
  for a worked example.
- A public HTTPS address for the console, for example `https://console.example.com`. Terminate TLS at an ingress
  controller, a Gateway, or the console itself with `--tls-cert` and `--tls-key`.
- Kubernetes 1.33 or later, Helm 3.x or 4, and `kubectl` access to the namespace where the console runs.

## 1. Choose the image

After the first release, use the published image and skip this step. Until then, build and push the image from a
checkout:

```sh
make image IMAGE=registry.example.com/platform/openbao-operator-console:dev
docker push registry.example.com/platform/openbao-operator-console:dev
```

`make image` builds for the local platform. For other platforms, run
`docker buildx build --platform linux/amd64,linux/arm64`. The image runs as user and group 65532 from a distroless base
and listens on port 8090. It contains the license notices in `/licenses`.

## 2. Prepare PostgreSQL

Create a dedicated database and user:

```sql
CREATE USER console WITH PASSWORD '<generated password>';
CREATE DATABASE console OWNER console;
```

The console creates and upgrades its schema at startup, so the user needs privileges to create and alter tables in that
database. Startup applies each pending numbered migration in order, in one transaction under an advisory lock, and
records it in `console_schema_migration`. A failed migration leaves the database unchanged. A startup refuses a database
whose schema version is newer than the binary supports, because downgrades are not supported.

Use a connection URL such as `postgres://console:<password>@db.example.com:5432/console?sslmode=verify-full`.
Use TLS with certificate verification for any database outside the console's cluster network.

When a database operator writes the connection URL to its own Secret, reference that Secret with `database.urlSecret`
and `database.urlKey` instead of copying the URL into the runtime Secret. The Secrets must be in the console's
namespace.

The chart sets `PGSSLMODE` from `database.sslMode`, which defaults to `verify-full`: the server certificate must chain
to a trusted CA and match the host name. Set `database.caSecret` to mount a private server CA certificate as
`PGSSLROOTCERT`; without it, the image's system CA bundle applies. The driver treats these variables as defaults, so an
`sslmode` or `sslrootcert` parameter in the URL takes precedence. A database without TLS needs `sslmode=disable` in the
URL or `database.sslMode: disable`; use that only on a trusted network.

The console reads the URL and CA certificate at startup. Restart it after the password or CA changes:

```sh
kubectl --namespace console-system rollout restart deployment/openbao-operator-console
```

### Example: CloudNativePG

The chart does not depend on [CloudNativePG](https://cloudnative-pg.io). If the operator is installed, create a
`Cluster` in the console's namespace:

```yaml
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: console-db
  namespace: console-system
spec:
  instances: 3
  storage:
    size: 10Gi
  bootstrap:
    initdb:
      database: console
      owner: console
```

CloudNativePG creates the `console-db-app` Secret with a `uri` key and the `console-db-ca` Secret with the server CA
certificate. The `uri` value does not set `sslmode`, so the CA settings apply. Reference both Secrets in the values
file:

```yaml
database:
  urlSecret: console-db-app
  urlKey: uri
  caSecret: console-db-ca
```

The chart mounts only the `ca.crt` key of the CA Secret. Configure CloudNativePG backups for the `Cluster`, and keep
the configuration encryption key from step 3 with them. After a restore into a new `Cluster`, point
`database.urlSecret` and `database.caSecret` at that `Cluster`'s Secrets and keep the same encryption key.

## 3. Generate the configuration encryption key

The console encrypts its configuration and stored Kubernetes credentials with a 32-byte key. Generate it in a private
directory:

```sh
umask 077
openssl rand -hex 32 > console-config-key
```

The console also accepts the key as 32 raw bytes or as base64 text.

Keep this key for the lifetime of the database. Back it up separately from PostgreSQL backups: a restored database
cannot be read without the matching key.

## 4. Register the OpenID Connect client

Create a confidential client with the authorization code flow, PKCE `S256`, and the exact redirect URI
`https://console.example.com/auth/callback`. You need the issuer URL, client ID, and client secret in the next steps,
either in the runtime Secret (automated bootstrap) or in the browser (protected setup).

## 5. Create the runtime Secret

The chart reads deployment settings from an existing Secret. Omit `DATABASE_URL` when `database.urlSecret` references
another Secret. Create the namespace and the Secret from private files so that values do not appear in shell history:

```sh
kubectl create namespace console-system
kubectl --namespace console-system create secret generic console-runtime \
  --from-file=DATABASE_URL=database-url \
  --from-file=OIDC_REDIRECT_URL=redirect-url \
  --from-file=CONFIG_KEY=console-config-key
```

Each file contains only the value. The console ignores surrounding whitespace, including a trailing newline.

| Key | Required | Description |
| --- | --- | --- |
| `DATABASE_URL` | Unless `database.urlSecret` is set | PostgreSQL connection URL. |
| `OIDC_REDIRECT_URL` | Yes | Public HTTPS callback URL ending in `/auth/callback`. |
| `CONFIG_KEY` | Yes | The key from step 3. |
| `OIDC_ISSUER` | For automated bootstrap | HTTPS issuer URL. |
| `OIDC_CLIENT_ID` | For automated bootstrap | Client ID. |
| `OIDC_CLIENT_SECRET` | For automated bootstrap | Client secret. |

Supply all three `OIDC_*` keys or none. Without them, the console starts in protected browser setup.

## 6. Choose how the first administrator is created

Pick one [first-run method](bootstrap.md):

- **Protected browser setup.** Create a setup token Secret and set `setup.tokenSecret`. The first person who enters the
  token and completes a verified OIDC login becomes the administrator.
- **Automated bootstrap.** Put the `OIDC_*` keys in the runtime Secret and list the administrator's OIDC subject in
  `bootstrapAdminSubjects`.

## 7. Write the values file

```yaml
# Only for an image you built. Released charts default to the matching published image.
image:
  repository: registry.example.com/platform/openbao-operator-console
  tag: dev

existingSecret: console-runtime

# Optional: read the database URL and CA from Secrets managed by the database provider.
# database:
#   urlSecret: console-db-app
#   urlKey: uri
#   caSecret: console-db-ca

# Protected browser setup. Remove after the first administrator signs in.
setup:
  tokenSecret: console-setup

# Or automated bootstrap:
# bootstrapAdminSubjects: ["<oidc subject of the first administrator>"]

# Optional: the hosting cluster's OpenBao namespaces and operator Deployment.
# The chart creates namespace-scoped list Roles and a named Deployment get Role.
readerNamespaces: [openbao-team-a]
operatorReaders:
  - namespace: openbao-operator-system
    deployments: [openbao-operator-controller]

ingress:
  enabled: true
  className: nginx
  host: console.example.com
  tlsSecret: console-tls
```

Set `oidcCA.configMap` when the identity provider uses a private certificate authority. The chart's
[values file](../charts/openbao-operator-console/values.yaml) documents every option, and the
[chart README](../charts/openbao-operator-console/README.md) summarizes them.

`readerNamespaces` and `operatorReaders` only grant Kubernetes permissions to the console's ServiceAccount. Enroll the
cluster and its namespaces in the console after the first login, or seed them with `enrollment` (see
[enrollment files](connect-clusters.md)).

## 8. Install the chart

From a release:

```sh
helm upgrade --install openbao-operator-console oci://ghcr.io/kubebao/charts/openbao-operator-console \
  --version <version> --namespace console-system --values console-values.yaml --wait
```

From a checkout:

```sh
helm upgrade --install openbao-operator-console charts/openbao-operator-console \
  --namespace console-system --values console-values.yaml --wait
```

When you use a Gateway instead of an ingress, leave `ingress.enabled` false and route HTTPS traffic to the
`openbao-operator-console` Service on port `http`. Set `networkPolicy.enabled` to limit ingress to the console port; see
the [chart README](../charts/openbao-operator-console/README.md#network-policy).

Check readiness. `/readyz` returns `204` once settings are loaded and PostgreSQL accepts connections:

```sh
curl --fail https://console.example.com/readyz
```

## 9. Sign in and connect clusters

Open the console. With browser setup, it redirects to `/setup`; follow [first administrator setup](bootstrap.md). With
automated bootstrap, sign in as the administrator.

Then add clusters in **Settings → Clusters → Add cluster**. See [connect a Kubernetes cluster](cluster-onboarding.md).

To upgrade or roll back later, follow [upgrade and roll back](upgrading.md). If the console does not start or sign-in
fails, see [troubleshooting](troubleshooting.md).

## Operate the installation

- **Replicas.** Run one replica. Sessions are held in process memory, and a restart requires users to sign in again.
- **Backups.** Back up PostgreSQL and the encryption key. Both are required to restore configuration and history.
- **Upgrades.** Back up the database before upgrading. The new version migrates the schema at startup. An older version
  refuses a migrated database, so rolling back the image also requires restoring the pre-upgrade backup. See
  [administration](administration.md).
- **Retention.** Observation history and activity are kept for seven days by default. Set `--history-retention` or
  `CONSOLE_HISTORY_RETENTION` to change it. An hourly job deletes older rows. Current inventory is not history and is
  kept while its connection is enrolled.
- **Uninstall.** `helm uninstall` leaves PostgreSQL untouched. Remove reader Roles and RoleBindings that you applied in
  other clusters yourself; the console never removes Kubernetes permissions.

## Run without Kubernetes

The same binary can run on a host behind an HTTPS reverse proxy:

```sh
./bin/console --mode=live --listen=127.0.0.1:8090 --config=/etc/console/enrollment.yaml
```

`--mode` defaults to `demo`, which serves fixture data. Pass `--mode=live` for every manual run against real clusters.
The Helm chart sets it for you.

| Setting | Description |
| --- | --- |
| `--mode` | `demo` (default, fixture data, no login) or `live`. |
| `--demo-allow-external` | Let demo mode listen on a non-loopback address, such as in a container. |
| `--listen` | Listen address. Default `127.0.0.1:8090`. |
| `--config` | Optional first-start enrollment YAML. Ignored once the database is initialized. |
| `--tls-cert`, `--tls-key` | Serve HTTPS directly. |
| `--check` | Check enrolled Kubernetes reads and exit. Needs no database or OIDC. |
| `--print-rbac` | Print reader RBAC for a connection ID and exit. |
| `--recover-identity` | Replace the stored identity provider and administrators, then exit. See [administration](administration.md#recover-administrator-login). |
| `--recover-access-rules` | With `--recover-identity` and a different issuer: `remove` or `carryOver` existing user and group access. Required for an issuer change. |
| `--history-retention` | How long observation history and activity are kept, from `1h` to `8760h`. Default `168h` (seven days), or `CONSOLE_HISTORY_RETENTION` when set. |
| `--version` | Print the version and commit, then exit. The startup log also records both. |
| `CONSOLE_DATABASE_URL` | PostgreSQL connection URL. |
| `CONSOLE_CONFIG_KEY_FILE` | Path to the encryption key: 32 bytes, raw or encoded as hexadecimal or base64 text. |
| `CONSOLE_OIDC_REDIRECT_URL` | Public callback URL. |
| `CONSOLE_OIDC_ISSUER`, `CONSOLE_OIDC_CLIENT_ID`, `CONSOLE_OIDC_CLIENT_SECRET` | Automated bootstrap identity. |
| `CONSOLE_OIDC_CA_FILE` | PEM bundle for a private identity provider CA. |
| `CONSOLE_BOOTSTRAP_ADMIN_SUBJECTS` | Comma-separated OIDC subjects for the first administrators. |
| `CONSOLE_SETUP_TOKEN_FILE` | Setup token file for protected browser setup. |
| `CONSOLE_HISTORY_RETENTION` | Default for `--history-retention`, as a Go duration such as `168h`. |
