# OpenBao Operator Console Helm chart

Installs the [OpenBao Operator Console](https://github.com/kubebao/openbao-operator-console), a read-only fleet console
for the [OpenBao Operator](https://github.com/kubebao/openbao-operator).

The chart runs one console replica with SQLite on a persistent volume by default. PostgreSQL is optional. It does not install PostgreSQL or an identity provider. See the
[installation guide](https://github.com/kubebao/openbao-operator-console/blob/main/docs/installation.md) for the full
procedure, including the runtime Secret and first-run setup.

## Prerequisites

- Kubernetes 1.33 or later
- Helm 3.x or 4
- A local or block filesystem StorageClass for SQLite, or an external PostgreSQL database
- An OpenID Connect provider is recommended; a local administrator with an authenticator app works without one

## Install

Releases publish the chart as a signed OCI artifact. Before the first stable release, install from a checkout with an
image you built, or use a qualified [edge build](../../docs/releasing.md#edge-builds). Edge charts include the tested
image digest and use a separate OCI repository.

```sh
helm upgrade --install openbao-operator-console oci://ghcr.io/kubebao/charts/openbao-operator-console \
  --version <version> --namespace console-system --values console-values.yaml --wait
```

Verify the chart signature before installing it; see
[releasing](https://github.com/kubebao/openbao-operator-console/blob/main/docs/releasing.md#verify-a-release). To
upgrade or roll back, follow
[upgrading](https://github.com/kubebao/openbao-operator-console/blob/main/docs/upgrading.md).

Resource names default to the release name, followed by the chart name unless the release name already contains it. With
the release name `openbao-operator-console`, the Deployment, Service, and ServiceAccount are all named
`openbao-operator-console`.

## Values

| Value | Default | Description |
| --- | --- | --- |
| `nameOverride` | `""` | Override the chart name in resource names and labels. |
| `fullnameOverride` | `""` | Override the full resource name. |
| `image.repository` | `ghcr.io/kubebao/openbao-operator-console` | Console image repository. |
| `image.tag` | `""` | Image tag. Defaults to the chart `appVersion`. |
| `image.digest` | `""` | Image digest, such as `sha256:<hex>`. Takes precedence over `image.tag`. |
| `image.pullPolicy` | `IfNotPresent` | Image pull policy. |
| `imagePullSecrets` | `[]` | Secrets for pulling the image from a private registry. |
| `existingSecret` | `""` | **Required.** Secret with `CONFIG_KEY`, PostgreSQL `DATABASE_URL` unless `database.urlSecret` is set, and optional `SETUP_TOKEN`, `OIDC_ISSUER`, `OIDC_CLIENT_ID`, and `OIDC_CLIENT_SECRET`. |
| `publicURL` | `""` | Public HTTPS address without a path, such as `https://console.example.com`. The OIDC callback is this address followed by `/auth/callback`. Defaults to `https://<ingress.host>` when the Ingress is enabled; required otherwise. |
| `database.engine` | `sqlite` | SQLite state on a PVC, or `postgres` for an external database. |
| `persistence.enabled` | `true` | Persist SQLite state. `false` uses an ephemeral `emptyDir` and loses state on Pod replacement. |
| `persistence.existingClaim` | `""` | Use an existing SQLite state PVC instead of creating one. |
| `persistence.size` | `1Gi` | Requested state volume size. |
| `persistence.storageClass` | `""` | StorageClass. Empty uses the cluster default. Use SQLite-compatible local or block storage; NFS is unsupported. |
| `persistence.accessModes` | `[ReadWriteOnce]` | One writer: `ReadWriteOnce` or `ReadWriteOncePod`. |
| `database.urlSecret` | `""` | Secret with the PostgreSQL connection URL, for example a CloudNativePG `<cluster>-app` Secret. Defaults to `existingSecret`. |
| `database.urlKey` | `DATABASE_URL` | Key of the connection URL. Use `uri` for CloudNativePG. |
| `database.sslMode` | `verify-full` | Default TLS mode, exported as `PGSSLMODE`. An `sslmode` parameter in the URL takes precedence. `""` uses the driver default, `prefer`, which does not verify the server. |
| `database.caSecret` | `""` | Secret with the PostgreSQL server CA certificate, exported as `PGSSLROOTCERT`. Without it, verification uses the system CA bundle in the image. |
| `database.caKey` | `ca.crt` | Key of the CA certificate in `database.caSecret`. |
| `historyRetention` | `""` | How long observation history and activity are kept, as a Go duration from `1h` to `8760h`. Empty uses the console default of `168h`. Exported as `CONSOLE_HISTORY_RETENTION`. |
| `setup.tokenSecret` | `""` | Secret with the setup token for protected browser setup on an empty database. Defaults to `existingSecret`. The mount is optional. |
| `setup.tokenKey` | `SETUP_TOKEN` | Key of the setup token. Remove the key after setup. |
| `bootstrapAdminSubjects` | `[]` | OIDC subjects granted administrator access on the first start. |
| `oidcCA.configMap` | `""` | ConfigMap with a private CA bundle for the identity provider. |
| `oidcCA.key` | `ca.crt` | Key of the CA bundle in `oidcCA.configMap`. |
| `serviceAccount.create` | `true` | Create the console ServiceAccount. |
| `serviceAccount.name` | `""` | ServiceAccount name. Defaults to the full resource name. Required when `serviceAccount.create` is false. |
| `serviceAccount.annotations` | `{}` | ServiceAccount annotations. |
| `readerNamespaces` | `[]` | Hosting-cluster namespaces where the console may list legacy `openbao.org` clusters and restores. |
| `readerScopes` | `[]` | Hosting-cluster LIST grants with required `namespace` and `apiGroup` (`kubebao.org` or `openbao.org`). |
| `operatorReaders` | `[]` | Named operator Deployments on the hosting cluster that the console may `get`, one entry per namespace. |
| `enrollment` | see `values.yaml` | Optional first-start seed: connections, users, groups, relationships, and policies. Ignored after initialization. |
| `kubeconfigSecrets` | `[]` | Secrets with remote reader kubeconfigs, mounted read-only with mode `0440` for the first-start seed. |
| `podAnnotations` | `{}` | Pod annotations. |
| `podLabels` | `{}` | Additional Pod labels. Do not override the `app.kubernetes.io/*` labels. |
| `resources` | 100m / 128Mi requests, 1 CPU / 512Mi limits | Container resources. |
| `nodeSelector` | `{}` | Pod node selector. |
| `tolerations` | `[]` | Pod tolerations. |
| `affinity` | `{}` | Pod affinity. |
| `tls.secretName` | `""` | `kubernetes.io/tls` Secret for HTTPS served by the console itself. The Service port then carries HTTPS. Leave empty when an ingress or Gateway terminates TLS. |
| `service.type` | `ClusterIP` | Service type. |
| `service.port` | `8090` | Service port. |
| `service.annotations` | `{}` | Service annotations. |
| `ingress.enabled` | `false` | Create an Ingress. |
| `ingress.className` | `""` | Ingress class name. |
| `ingress.annotations` | `{}` | Ingress annotations. |
| `ingress.host` | `""` | Public host name. Required when the Ingress is enabled. |
| `ingress.tlsSecret` | `""` | TLS Secret for the host. Required when the Ingress is enabled. |
| `trustedProxies` | `[]` | Proxy addresses or CIDR prefixes whose `X-Forwarded-For` identifies the client for sign-in rate limits and security logs. Catch-all prefixes are refused. See [trusted proxies](#trusted-proxies). |
| `networkPolicy.enabled` | `false` | Create a NetworkPolicy for the console Pod. |
| `networkPolicy.ingressFrom` | `[]` | NetworkPolicy peers allowed to reach the console port. Empty allows every source on that port only. |
| `networkPolicy.egress` | `[]` | NetworkPolicy egress rules. Empty leaves egress unrestricted. |

`values.schema.json` validates these values. Helm rejects unknown keys and invalid enumerations.

## SQLite persistence

The chart mounts state at `/var/lib/console`. It retains the generated PVC on uninstall. A reinstall with the same
release name can reuse this state. Do not mount the database directory in another serving process or remove its lock
file. Back up the database and CONFIG_KEY separately. Follow [schema upgrades](../../docs/upgrading.md) when the
candidate binary reports pending changes. Changing `database.engine` does not transfer data between backends.

## PostgreSQL TLS

The console uses the pgx driver, which reads libpq environment variables as defaults. Parameters in the connection URL
take precedence: a URL with `sslmode=disable` or `sslrootcert=...` overrides `database.sslMode` and `database.caSecret`.

With the default `verify-full`, the server certificate must be signed by a trusted CA and match the host name in the
URL. Set `database.caSecret` when the server uses a private CA, such as the `<cluster>-ca` Secret of CloudNativePG.

## Network policy

With `networkPolicy.enabled`, the chart creates one NetworkPolicy for the console Pod. Ingress is limited to the console
port. List the ingress controller or Gateway namespace in `networkPolicy.ingressFrom` to limit the sources:

```yaml
networkPolicy:
  enabled: true
  ingressFrom:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: ingress-nginx
```

Egress stays unrestricted unless `networkPolicy.egress` contains rules. Restricted egress must allow DNS, PostgreSQL,
the identity provider, and the API server of every enrolled Kubernetes cluster, including the hosting cluster.

## Trusted proxies

Behind an ingress controller or Gateway, every request arrives from the proxy's address. Rate limits for setup and
sign-in then treat all browsers as one client, and security logs show the proxy. List the proxy addresses in
`trustedProxies` to use the client address from `X-Forwarded-For` instead:

```yaml
trustedProxies: [10.42.0.0/16]
networkPolicy:
  enabled: true
  ingressFrom:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: ingress-nginx
```

The console reads the header from right to left and uses the first address that is not a trusted proxy, so entries a
client adds itself are ignored. The address is used for rate limits and logs only, never for access decisions.

- Only list networks whose every address is a proxy you operate. Any Pod in a trusted range could otherwise send a
  forged header. Limit ingress to those proxies with `networkPolicy.ingressFrom`.
- The proxy must see the real client address. With a cloud load balancer, set `externalTrafficPolicy: Local` on the
  ingress controller Service or enable the PROXY protocol; otherwise the header carries node addresses.

## Created resources

- A Deployment and Service for the console, and a ConfigMap with the first-start enrollment.
- A ServiceAccount for the console, unless `serviceAccount.create` is false.
- For each namespace declared by `readerNamespaces` or `readerScopes`, one Role and RoleBinding that allow `list` on
  `openbaoclusters` and `openbaorestores` in only the declared API groups. `readerNamespaces` contributes legacy
  `openbao.org` only; observing both groups requires both grants to be declared.
- For each `operatorReaders` entry, a Role and RoleBinding that allow `get` on the named Deployments only.
- An Ingress when `ingress.enabled` is true, and a NetworkPolicy when `networkPolicy.enabled` is true.

The container runs as non-root with a read-only root filesystem and no added capabilities.
