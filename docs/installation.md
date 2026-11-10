# Install the console

This guide installs the console on Kubernetes with the Helm chart. The console runs as one replica, stores its data in
PostgreSQL, and signs users in through OpenID Connect or local accounts. The installation takes four steps: a database,
one Secret, the chart, and the first administrator.

Releases publish a signed image to `ghcr.io/kubebao/openbao-operator-console` and the chart to
`oci://ghcr.io/kubebao/charts/openbao-operator-console`; see [releases](releasing.md#verify-a-release) to verify them.

> [!NOTE]
> No stable release has been published yet. [Build the image](#build-the-image-before-the-first-release) and install
> from a checkout, or use a qualified [edge build](releasing.md#edge-builds).

To try the console first, [evaluate it on a local cluster](evaluation.md).

The examples install the release `openbao-operator-console` in the namespace `console-system`. With that release name,
the Deployment, Service, and ServiceAccount are named `openbao-operator-console`.

## Prerequisites

- A Kubernetes 1.33 or later cluster for the console. It can be one of the clusters that runs the OpenBao Operator.
- Helm 3.x or 4, and `kubectl` access to the console's namespace.
- PostgreSQL. CI tests PostgreSQL 18. [CloudNativePG](#use-cloudnativepg) is the shortest path on Kubernetes.
- An OpenID Connect provider that supports the authorization code flow with PKCE (`S256`), recommended for teams. See
  [Keycloak](keycloak.md) for a worked example. You register the client during setup. Without a provider, start with a
  local administrator and add the provider later.
- A public HTTPS address, such as `https://console.example.com`, served by an ingress controller or a Gateway. The
  console can also serve HTTPS itself with `tls.secretName`.

## 1. Prepare PostgreSQL

Create a dedicated database and a user that owns it:

```sql
CREATE USER console WITH PASSWORD '<generated password>';
CREATE DATABASE console OWNER console;
```

The console creates and migrates its schema at startup. The connection URL looks like
`postgres://console:<password>@db.example.com:5432/console?sslmode=verify-full`. When a database operator manages the
credentials, skip this step and reference its Secret instead; see [use CloudNativePG](#use-cloudnativepg).

## 2. Create the runtime Secret

Generate the configuration encryption key and a one-time setup token in a private directory. Write the database URL to
a file named `database-url`, then create the Secret:

```sh
umask 077
openssl rand -hex 32 > console-config-key
openssl rand -hex 32 > console-setup-token

kubectl create namespace console-system
kubectl --namespace console-system create secret generic console-runtime \
  --from-file=CONFIG_KEY=console-config-key \
  --from-file=SETUP_TOKEN=console-setup-token \
  --from-file=DATABASE_URL=database-url
```

The files keep the values out of shell history. The console ignores surrounding whitespace, including a trailing
newline.

| Key | Required | Description |
| --- | --- | --- |
| `CONFIG_KEY` | Yes | Encrypts configuration and stored Kubernetes credentials: 32 random bytes, raw or encoded as hexadecimal or base64 text. |
| `DATABASE_URL` | Unless `database.urlSecret` is set | PostgreSQL connection URL. |
| `SETUP_TOKEN` | For browser setup | Unlocks first administrator setup on an empty database. Remove it after setup. |
| `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET` | For automated bootstrap | Seed the identity provider instead of browser setup. Supply all three or none. |

> [!IMPORTANT]
> Back up `console-config-key` separately from PostgreSQL backups and keep it for the lifetime of the database. A
> restored database cannot be read without the matching key.

## 3. Install the chart

Write `console-values.yaml`:

```yaml
existingSecret: console-runtime

ingress:
  enabled: true
  className: nginx
  host: console.example.com
  tlsSecret: console-tls
```

The chart derives the public address and the OIDC callback URL, `https://console.example.com/auth/callback`, from the
Ingress host. With a Gateway, leave `ingress.enabled` false, set `publicURL: https://console.example.com`, and route
HTTPS traffic to the `openbao-operator-console` Service on port `http`.

Install from a release:

```sh
helm upgrade --install openbao-operator-console oci://ghcr.io/kubebao/charts/openbao-operator-console \
  --version <version> --namespace console-system --values console-values.yaml --wait
```

Or from a checkout:

```sh
helm upgrade --install openbao-operator-console charts/openbao-operator-console \
  --namespace console-system --values console-values.yaml --wait
```

The release notes print the console address, the callback URL to register, and the next steps. `/readyz` returns `204`
once settings are loaded and PostgreSQL accepts connections:

```sh
curl --fail https://console.example.com/readyz
```

## 4. Create the first administrator

Open the console. On an empty database it redirects to `/setup`:

1. Enter the setup token from `console-setup-token`.
2. Choose how the first administrator signs in:
   - **Identity provider** (recommended): register the displayed callback URL in a confidential OpenID Connect client
     with the authorization code flow and PKCE `S256`. Enter the issuer, client ID, and client secret, then select
     **Verify with login** and sign in with the account that becomes the first administrator.
   - **Local administrator**: enter a username and a password of at least 15 characters, add an authenticator app, and
     save the recovery codes. No identity provider is needed. You can add one later in **Settings**.

The setup window lasts one hour from startup. After you have signed in, remove the token. No Helm upgrade is required:

```sh
kubectl --namespace console-system patch secret console-runtime --type=json \
  --patch='[{"op":"remove","path":"/data/SETUP_TOKEN"}]'
```

The console then opens guided cluster onboarding. Continue with
[connect a Kubernetes cluster](cluster-onboarding.md). [First administrator setup](bootstrap.md) describes the setup
security model, retries, and automated bootstrap.

If the console does not start or sign-in fails, see [troubleshooting](troubleshooting.md).

## Options

### Use CloudNativePG

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
certificate. Omit `DATABASE_URL` from the runtime Secret and reference both Secrets in the values file:

```yaml
database:
  urlSecret: console-db-app
  urlKey: uri
  caSecret: console-db-ca
```

The chart mounts only the `ca.crt` key of the CA Secret. Configure CloudNativePG backups for the `Cluster`, and keep the
configuration encryption key with them. After a restore into a new `Cluster`, point `database.urlSecret` and
`database.caSecret` at that `Cluster`'s Secrets and keep the same encryption key.

### Database TLS

The chart sets `PGSSLMODE` from `database.sslMode`, which defaults to `verify-full`: the server certificate must chain
to a trusted CA and match the host name. Set `database.caSecret` to mount a private server CA certificate as
`PGSSLROOTCERT`; without it, the image's system CA bundle applies. An `sslmode` or `sslrootcert` parameter in the URL
takes precedence. A database without TLS needs `sslmode=disable` in the URL or `database.sslMode: disable`; use that only
on a trusted network.

The console reads the URL and CA certificate at startup. Restart it after the password or CA changes:

```sh
kubectl --namespace console-system rollout restart deployment/openbao-operator-console
```

Startup applies each pending numbered schema migration in order, in one transaction under an advisory lock, and records
it in `console_schema_migration`. A failed migration leaves the database unchanged. Startup refuses a database whose
schema version is newer than the binary supports, because downgrades are not supported.

### Automated bootstrap

To skip browser setup, register the OIDC client first, put `OIDC_ISSUER`, `OIDC_CLIENT_ID`, and `OIDC_CLIENT_SECRET`
in the runtime Secret instead of `SETUP_TOKEN`, and list the administrator's OIDC subject in the values file:

```yaml
bootstrapAdminSubjects: ["<oidc subject of the first administrator>"]
```

Set `oidcCA.configMap` when the identity provider uses a private certificate authority. See
[first administrator setup](bootstrap.md#automated-first-start).

### Declarative cluster enrollment

The chart can grant the console's ServiceAccount read access on the hosting cluster and seed enrollment on the first
start, for GitOps installations:

```yaml
readerNamespaces: [openbao-team-a]
operatorReaders:
  - namespace: openbao-operator-system
    deployments: [openbao-operator-controller]
```

`readerNamespaces` and `operatorReaders` only grant Kubernetes permissions. Enroll the cluster in the console, or seed
it with `enrollment`; see [enrollment files](connect-clusters.md). When these values already grant access, skip the
RBAC commands in the onboarding wizard.

### Network policy and trusted proxies

Set `networkPolicy.enabled` to limit ingress to the console port; see the
[chart README](../charts/openbao-operator-console/README.md#network-policy). Behind an ingress controller, list its
addresses in `trustedProxies` so that rate limits and security logs use the client address; see
[trusted proxies](../charts/openbao-operator-console/README.md#trusted-proxies). The chart's
[values file](../charts/openbao-operator-console/values.yaml) documents every option.

### Build the image before the first release

Build and push the image from a checkout:

```sh
make image IMAGE=registry.example.com/platform/openbao-operator-console:dev
docker push registry.example.com/platform/openbao-operator-console:dev
```

Then set it in the values file:

```yaml
image:
  repository: registry.example.com/platform/openbao-operator-console
  tag: dev
```

`make image` builds for the local platform. For other platforms, run
`docker buildx build --platform linux/amd64,linux/arm64`. The image runs as user and group 65532 from a distroless base
and listens on port 8090. It contains the license notices in `/licenses`.

### Run without Kubernetes

The same binary runs on a host behind an HTTPS reverse proxy. See the
[command-line and environment reference](configuration.md).

## Operate the installation

- **Replicas.** Run one replica. Sessions are held in process memory, and a restart requires users to sign in again.
- **Backups.** Back up PostgreSQL and the encryption key. Both are required to restore configuration and history.
- **Upgrades.** Back up the database before upgrading. The new version migrates the schema at startup. An older version
  refuses a migrated database, so rolling back the image also requires restoring the pre-upgrade backup. See
  [upgrade and roll back](upgrading.md).
- **Retention.** Observation history and activity are kept for seven days by default. Set `historyRetention` to change
  it. An hourly job deletes older rows. Current inventory is not history and is kept while its connection is enrolled.
- **Uninstall.** `helm uninstall` leaves PostgreSQL untouched. Remove reader Roles and RoleBindings that you applied in
  other clusters yourself; the console never removes Kubernetes permissions.
