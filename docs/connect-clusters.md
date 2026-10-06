# Enrollment files and CLI diagnostics

The console collects existing OpenBao resources and named controller Deployments. It does not create OpenBao clusters,
read Secrets, contact OpenBao, or run recovery operations. Use PostgreSQL and an OIDC provider to serve a live workspace.

Most installations add clusters through [guided cluster onboarding](cluster-onboarding.md). This page covers the
alternatives: seeding enrollment from a YAML file on the first start, generating reader RBAC from the command line, and
checking Kubernetes access without a database or identity provider.

## Declare enrollment

Copy `examples/enrollment.yaml` to a private file. For each Kubernetes connection, set a stable `id`, reader kubeconfig,
context, and OpenBao namespaces. Namespace metadata supplies the team, environment, and location shown in the console.
A connection with neither a kubeconfig nor a context uses the application's Kubernetes ServiceAccount.

Add an `operators` entry for each controller installation. Specify the exact Deployment namespace, name, and container.
`operators[].namespaces` declares responsibility for enrolled OpenBao namespaces. Each namespace has at most one enrolled
operator. The console does not discover responsibility from labels, Helm release names, or a shared namespace.

The version comes from the controller Deployment's `app.kubernetes.io/version` label. The container image is shown
separately. A chart label can describe a release while a development image runs different code. Missing labels remain
unknown. Workload readiness does not establish successful reconciliation or OpenBao availability.

## Grant reader access

Build the executable with `make build`. Generate RBAC for one enrolled connection:

```sh
./bin/console --config=/private/enrollment.yaml --print-rbac=local \
  --reader-name=console-reader --reader-namespace=console-system > /private/reader-rbac.yaml
```

The command prints manifests. It does not contact Kubernetes or apply them. Create the reader namespace if required,
then apply the manifests through the administrator context for that connection. Generated Roles grant namespace-scoped
`list` for `openbaoclusters`, `openbaorestores`, and `openbaorecoveries`. Controller Roles grant `get` for named Deployments.
They do not grant Deployment listing, Secret access, or cluster-wide resource discovery.

To create a short-lived credential for testing, without copying administrator credentials into the reader:

```sh
python3 scripts/reader-kubeconfig.py \
  --admin-kubeconfig=/private/admin-kubeconfig --context=kind-console-dev \
  --namespace=console-system --service-account=console-reader \
  --duration=1h --output=/private/reader-kubeconfig
```

The helper uses TokenRequest and writes a separate file with mode `0600`. It never prints the token. For the hosting
cluster, use the console's mounted ServiceAccount token. For remote clusters, enable [automatic renewal](credential-renewal.md)
with the named TokenRequest grant, or replace credentials manually before expiry. Kubeconfig import rejects authentication
plugins. Keep reader kubeconfigs out of Git.

## Check the connection

```sh
./bin/console --config=/private/enrollment.yaml --check
```

This command needs neither PostgreSQL nor OIDC. It prints approved connection, API, controller, and deployment projections.
It exits with status 1 when a connection or enrolled controller read fails. A controller with zero ready replicas or a sealed
OpenBao deployment can still pass the transport check; inspect their separate health fields.

| Result | Meaning | Action |
| --- | --- | --- |
| `available` | The namespace list succeeded. | Inspect workload evidence. |
| `unsupported` | The resource API is not served. | Install a compatible operator feature build if that API is needed. |
| `denied` | Kubernetes denied the read. | Check RBAC and credential expiry. |
| `unreachable` | The read or reader configuration failed. | Check the kubeconfig, context, API endpoint, and network. |
| `notChecked` | Collection stopped before this API read. | Resolve the preceding failure. |

API availability does not verify an unreleased field schema or an enabled controller feature gate. Absent optional restore
or recovery APIs do not block core inventory. Forbidden optional reads remain capability warnings and leave the affected
evidence unknown. Authentication or transport failures still fail collection and preserve previous inventory.
A missing or denied controller Deployment affects operator evidence independently of OpenBao collection.

## Seed enrollment with Helm

Enrollment YAML seeds the first start only. After that, manage connections and access in **Settings**; changes to the
seed are ignored. See the [installation guide](installation.md) for the runtime Secret and OIDC settings.

For Kubernetes, adapt `examples/live-values.yaml`. Set `readerNamespaces` for local resource collection and
`operatorReaders` for the exact controller Deployments, with one `operatorReaders` entry per namespace. Mount remote
reader kubeconfigs with `kubeconfigSecrets`. `examples/multi-cluster-enrollment.yaml` shows two clusters with separate
reader credentials and operator installations.

```sh
helm upgrade --install console charts/openbao-operator-console \
  --namespace console-system --values /private/live-values.yaml
```

Collection runs every 30 seconds with at most four connections in flight. Rounds do not overlap. One broken kubeconfig
does not prevent startup or collection from other connections. Evidence becomes stale after two minutes without a
completed collection.
