# openbao-operator-console Helm chart

Installs the [OpenBao Operator Console](../../README.md), a read-only fleet console for the OpenBao Operator.

The chart runs one console replica. It does not install PostgreSQL or an identity provider. See the
[installation guide](../../docs/installation.md) for the full procedure, including the runtime Secret and first-run
setup.

```sh
helm upgrade --install console charts/openbao-operator-console \
  --namespace openbao-console --values console-values.yaml --wait
```

## Values

| Value | Default | Description |
| --- | --- | --- |
| `image.repository` | `openbao-operator-console` | Console image repository. |
| `image.tag` | `local` | Image tag. Set this to the image you built. |
| `image.pullPolicy` | `IfNotPresent` | Image pull policy. |
| `existingSecret` | `""` | **Required.** Secret with `DATABASE_URL`, `OIDC_REDIRECT_URL`, and `CONFIG_KEY`, plus optional `OIDC_ISSUER`, `OIDC_CLIENT_ID`, and `OIDC_CLIENT_SECRET`. |
| `setup.tokenSecret` | `""` | Secret with a setup token for protected browser setup on an empty database. Remove after setup. |
| `setup.tokenKey` | `setup-token` | Key of the setup token in `setup.tokenSecret`. |
| `bootstrapAdminSubjects` | `[]` | OIDC subjects granted administrator access on the first start. |
| `oidcCA.configMap` | `""` | ConfigMap with a private CA bundle for the identity provider. |
| `oidcCA.key` | `ca.crt` | Key of the CA bundle in `oidcCA.configMap`. |
| `readerNamespaces` | `[]` | Namespaces on the hosting cluster where the console's ServiceAccount may list OpenBao resources. |
| `operatorReaders` | `[]` | Named operator Deployments on the hosting cluster that the console may `get`, one entry per namespace. |
| `enrollment` | see `values.yaml` | Optional first-start seed: connections, users, groups, relationships, and policies. Ignored after initialization. |
| `kubeconfigSecrets` | `[]` | Secrets with remote reader kubeconfigs, mounted for the first-start seed. |
| `resources` | 100m / 128Mi requests, 1 CPU / 512Mi limits | Container resources. |
| `ingress.enabled` | `false` | Create an Ingress. |
| `ingress.className` | `""` | Ingress class. |
| `ingress.host` | `""` | Public host name. Required when the Ingress is enabled. |
| `ingress.tlsSecret` | `""` | TLS Secret for the host. Required when the Ingress is enabled. |

## Created resources

- A Deployment and Service for the console, and a ConfigMap with the first-start enrollment.
- A ServiceAccount for the console.
- For each `readerNamespaces` entry, a Role and RoleBinding that allow `list` on `openbaoclusters`, `openbaorestores`,
  and `openbaorecoveries`.
- For each `operatorReaders` entry, a Role and RoleBinding that allow `get` on the named Deployments only.
- An Ingress when `ingress.enabled` is true.

The container runs as non-root with a read-only root filesystem and no added capabilities.
