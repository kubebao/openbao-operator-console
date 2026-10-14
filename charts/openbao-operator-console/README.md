# OpenBao Operator Console Helm chart

Installs the [OpenBao Operator Console](https://github.com/kubebao/openbao-operator-console), a read-only fleet console
for the [OpenBao Operator](https://github.com/kubebao/openbao-operator).

The chart runs one console replica. It does not install PostgreSQL or an identity provider. See the
[installation guide](https://github.com/kubebao/openbao-operator-console/blob/main/docs/installation.md) for the full
procedure, including the runtime Secret and first-run setup.

## Prerequisites

- Kubernetes 1.33 or later
- Helm 3.x or 4
- An external PostgreSQL database and an OpenID Connect provider

## Install

Releases publish the chart as a signed OCI artifact. Until the first release, install the chart from a checkout and set
`image.repository` and `image.tag` to an image you built.

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
| `existingSecret` | `""` | **Required.** Secret with `OIDC_REDIRECT_URL` and `CONFIG_KEY`, `DATABASE_URL` unless `database.urlSecret` is set, plus optional `OIDC_ISSUER`, `OIDC_CLIENT_ID`, and `OIDC_CLIENT_SECRET`. |
| `database.urlSecret` | `""` | Secret with the PostgreSQL connection URL, for example a CloudNativePG `<cluster>-app` Secret. Defaults to `existingSecret`. |
| `database.urlKey` | `DATABASE_URL` | Key of the connection URL. Use `uri` for CloudNativePG. |
| `database.sslMode` | `verify-full` | Default TLS mode, exported as `PGSSLMODE`. An `sslmode` parameter in the URL takes precedence. `""` uses the driver default, `prefer`, which does not verify the server. |
| `database.caSecret` | `""` | Secret with the PostgreSQL server CA certificate, exported as `PGSSLROOTCERT`. Without it, verification uses the system CA bundle in the image. |
| `database.caKey` | `ca.crt` | Key of the CA certificate in `database.caSecret`. |
| `historyRetention` | `""` | How long observation history and activity are kept, as a Go duration from `1h` to `8760h`. Empty uses the console default of `168h`. Exported as `CONSOLE_HISTORY_RETENTION`. |
| `setup.tokenSecret` | `""` | Secret with a setup token for protected browser setup on an empty database. Remove after setup. |
| `setup.tokenKey` | `setup-token` | Key of the setup token in `setup.tokenSecret`. |
| `bootstrapAdminSubjects` | `[]` | OIDC subjects granted administrator access on the first start. |
| `oidcCA.configMap` | `""` | ConfigMap with a private CA bundle for the identity provider. |
| `oidcCA.key` | `ca.crt` | Key of the CA bundle in `oidcCA.configMap`. |
| `serviceAccount.create` | `true` | Create the console ServiceAccount. |
| `serviceAccount.name` | `""` | ServiceAccount name. Defaults to the full resource name. Required when `serviceAccount.create` is false. |
| `serviceAccount.annotations` | `{}` | ServiceAccount annotations. |
| `readerNamespaces` | `[]` | Namespaces on the hosting cluster where the console's ServiceAccount may list OpenBao resources. |
| `operatorReaders` | `[]` | Named operator Deployments on the hosting cluster that the console may `get`, one entry per namespace. |
| `enrollment` | see `values.yaml` | Optional first-start seed: connections, users, groups, relationships, and policies. Ignored after initialization. |
| `kubeconfigSecrets` | `[]` | Secrets with remote reader kubeconfigs, mounted read-only with mode `0440` for the first-start seed. |
| `podAnnotations` | `{}` | Pod annotations. |
| `podLabels` | `{}` | Additional Pod labels. Do not override the `app.kubernetes.io/*` labels. |
| `resources` | 100m / 128Mi requests, 1 CPU / 512Mi limits | Container resources. |
| `nodeSelector` | `{}` | Pod node selector. |
| `tolerations` | `[]` | Pod tolerations. |
| `affinity` | `{}` | Pod affinity. |
| `service.type` | `ClusterIP` | Service type. |
| `service.port` | `8090` | Service port. |
| `service.annotations` | `{}` | Service annotations. |
| `ingress.enabled` | `false` | Create an Ingress. |
| `ingress.className` | `""` | Ingress class name. |
| `ingress.annotations` | `{}` | Ingress annotations. |
| `ingress.host` | `""` | Public host name. Required when the Ingress is enabled. |
| `ingress.tlsSecret` | `""` | TLS Secret for the host. Required when the Ingress is enabled. |
| `networkPolicy.enabled` | `false` | Create a NetworkPolicy for the console Pod. |
| `networkPolicy.ingressFrom` | `[]` | NetworkPolicy peers allowed to reach the console port. Empty allows every source on that port only. |
| `networkPolicy.egress` | `[]` | NetworkPolicy egress rules. Empty leaves egress unrestricted. |

`values.schema.json` validates these values. Helm rejects unknown keys and invalid enumerations.

## Database TLS

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

## Created resources

- A Deployment and Service for the console, and a ConfigMap with the first-start enrollment.
- A ServiceAccount for the console, unless `serviceAccount.create` is false.
- For each `readerNamespaces` entry, a Role and RoleBinding that allow `list` on `openbaoclusters`, `openbaorestores`,
  and `openbaorecoveries`.
- For each `operatorReaders` entry, a Role and RoleBinding that allow `get` on the named Deployments only.
- An Ingress when `ingress.enabled` is true, and a NetworkPolicy when `networkPolicy.enabled` is true.

The container runs as non-root with a read-only root filesystem and no added capabilities.
