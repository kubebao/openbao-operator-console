# Install the console

This guide installs the console on Kubernetes with the Helm chart in `charts/openbao-operator-console`. The console runs
as one replica, stores its data in an external PostgreSQL database, and authenticates users through OpenID Connect.

There is no published container image or chart repository yet. Build the image from source and install the chart from
your checkout.

## Prerequisites

- A Kubernetes cluster for the console. It can be one of the clusters that runs the OpenBao Operator.
- A PostgreSQL database and a user that can create tables in it. CI tests PostgreSQL 18.
- An OpenID Connect provider that supports the authorization code flow with PKCE (`S256`). See [Keycloak](keycloak.md)
  for a worked example.
- A public HTTPS address for the console, for example `https://console.example.com`. Terminate TLS at an ingress
  controller, a Gateway, or the console itself with `--tls-cert` and `--tls-key`.
- Helm 3 and `kubectl` access to the namespace where the console runs.

## 1. Build the image

```sh
docker build -t registry.example.com/platform/openbao-operator-console:0.1.0 .
docker push registry.example.com/platform/openbao-operator-console:0.1.0
```

The image runs as a non-root user from a distroless base and listens on port 8090.

## 2. Prepare PostgreSQL

Create a dedicated database and user:

```sql
CREATE USER console WITH PASSWORD '<generated password>';
CREATE DATABASE console OWNER console;
```

The console creates and migrates its schema at startup under a transaction lock, so the user needs privileges to create
tables in that database. A startup refuses a database schema newer than the binary supports.

Use a connection URL such as `postgres://console:<password>@db.example.com:5432/console?sslmode=verify-full`.
Use TLS with certificate verification for any database outside the console's cluster network.

## 3. Generate the configuration encryption key

The console encrypts its configuration and stored Kubernetes credentials with a 32-byte key. Generate it in a private
directory:

```sh
umask 077
openssl rand 32 > console-config-key
```

Keep this key for the lifetime of the database. Back it up separately from PostgreSQL backups: a restored database
cannot be read without the matching key.

## 4. Register the OpenID Connect client

Create a confidential client with the authorization code flow, PKCE `S256`, and the exact redirect URI
`https://console.example.com/auth/callback`. You need the issuer URL, client ID, and client secret in the next steps,
either in the runtime Secret (automated bootstrap) or in the browser (protected setup).

## 5. Create the runtime Secret

The chart reads deployment settings from an existing Secret. Create the namespace and the Secret from private files so
that values do not appear in shell history:

```sh
kubectl create namespace openbao-console
kubectl --namespace openbao-console create secret generic console-runtime \
  --from-file=DATABASE_URL=database-url \
  --from-file=OIDC_REDIRECT_URL=redirect-url \
  --from-file=CONFIG_KEY=console-config-key
```

Each file contains only the value, without a trailing newline.

| Key | Required | Description |
| --- | --- | --- |
| `DATABASE_URL` | Yes | PostgreSQL connection URL. |
| `OIDC_REDIRECT_URL` | Yes | Public HTTPS callback URL ending in `/auth/callback`. |
| `CONFIG_KEY` | Yes | The 32 raw bytes from step 3. |
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
image:
  repository: registry.example.com/platform/openbao-operator-console
  tag: "0.1.0"

existingSecret: console-runtime

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

```sh
helm upgrade --install console charts/openbao-operator-console \
  --namespace openbao-console --values console-values.yaml --wait
```

When you use a Gateway instead of an ingress, leave `ingress.enabled` false and route HTTPS traffic to the `console`
Service on port `http`.

Check readiness:

```sh
curl --fail https://console.example.com/readyz
```

## 9. Sign in and connect clusters

Open the console. With browser setup, it redirects to `/setup`; follow [first administrator setup](bootstrap.md).
With automated bootstrap, sign in as the administrator.

Then add clusters in **Settings → Clusters → Add cluster**. See [connect a Kubernetes cluster](cluster-onboarding.md).

## Operate the installation

- **Replicas.** Run one replica. Sessions are held in process memory, and a restart requires users to sign in again.
- **Backups.** Back up PostgreSQL and the encryption key. Both are required to restore configuration and history.
- **Upgrades.** Back up the database before upgrading. A schema migration can make the database unreadable for older
  versions, so rolling back the image may also require restoring the database. See [administration](administration.md).
- **Retention.** Observations are kept for seven days.
- **Uninstall.** `helm uninstall` leaves PostgreSQL untouched. Remove reader Roles and RoleBindings that you applied in
  other clusters yourself; the console never removes Kubernetes permissions.

## Run without Kubernetes

The same binary can run on a host behind an HTTPS reverse proxy:

```sh
./bin/console --mode=live --listen=127.0.0.1:8090 --config=/etc/console/enrollment.yaml
```

| Setting | Description |
| --- | --- |
| `--mode` | `demo` (default, loopback only, fixture data) or `live`. |
| `--listen` | Listen address. Default `127.0.0.1:8090`. |
| `--config` | Optional first-start enrollment YAML. Ignored once the database is initialized. |
| `--tls-cert`, `--tls-key` | Serve HTTPS directly. |
| `--check` | Check enrolled Kubernetes reads and exit. Needs no database or OIDC. |
| `--print-rbac` | Print reader RBAC for a connection ID and exit. |
| `--recover-identity` | Replace the stored identity provider and administrators, then exit. See [administration](administration.md). |
| `CONSOLE_DATABASE_URL` | PostgreSQL connection URL. |
| `CONSOLE_CONFIG_KEY_FILE` | Path to the 32-byte encryption key. |
| `CONSOLE_OIDC_REDIRECT_URL` | Public callback URL. |
| `CONSOLE_OIDC_ISSUER`, `CONSOLE_OIDC_CLIENT_ID`, `CONSOLE_OIDC_CLIENT_SECRET` | Automated bootstrap identity. |
| `CONSOLE_OIDC_CA_FILE` | PEM bundle for a private identity provider CA. |
| `CONSOLE_BOOTSTRAP_ADMIN_SUBJECTS` | Comma-separated OIDC subjects for the first administrators. |
| `CONSOLE_SETUP_TOKEN_FILE` | Setup token file for protected browser setup. |
