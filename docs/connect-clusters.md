# Enrollment files and CLI diagnostics

The console collects existing OpenBao resources and named controller Deployments. It does not create OpenBao clusters,
read Secrets, contact OpenBao, or run recovery operations. Live mode uses durable SQLite or PostgreSQL storage and OIDC or local account sign-in.

Most installations add clusters through [guided cluster onboarding](cluster-onboarding.md). This page covers the
alternatives: seeding enrollment from a YAML file on the first start, generating reader RBAC from the command line, and
checking Kubernetes access without a database or identity provider.

## Declare enrollment

Copy `examples/enrollment.yaml` to a private file. For each Kubernetes connection, set a stable `id`, reader kubeconfig,
context, and OpenBao namespaces. Namespace metadata supplies the team, environment, and location shown in the console. A
connection with neither a kubeconfig nor a context uses the application's Kubernetes ServiceAccount.

Add an `operators` entry for each controller installation. Specify the exact Deployment namespace, name, and container.
`operators[].namespaces` declares responsibility for enrolled OpenBao namespaces. Each namespace has at most one
enrolled operator. The console does not discover responsibility from labels, Helm release names, or a shared namespace.

The version comes from the controller Deployment's `app.kubernetes.io/version` label. The container image is shown
separately. A chart label can describe a release while a development image runs different code. Missing labels remain
unknown. Workload readiness does not establish successful reconciliation or OpenBao availability.

## Grant reader access

Download the archive for your platform from a [release](https://github.com/kubebao/openbao-operator-console/releases)
and [verify it](releasing.md#verify-a-release), or build `bin/console` from a checkout with `make build`. Generate RBAC
for one enrolled connection:

```sh
./console --config=/private/enrollment.yaml --print-rbac=local \
  --reader-name=console-reader --reader-namespace=console-system > /private/reader-rbac.yaml
```

The command prints manifests. It does not contact Kubernetes or apply them. Create the reader namespace if required,
then apply the manifests through the administrator context for that connection. Generated Roles grant namespace-scoped
`list` for `openbaoclusters` and `openbaorestores`. Controller Roles grant `get` for named
Deployments. They do not grant Deployment listing, Secret access, or cluster-wide resource discovery.

To create a short-lived credential for testing, without copying administrator credentials into the reader:

```sh
python3 scripts/reader-kubeconfig.py \
  --admin-kubeconfig=/private/admin-kubeconfig --context=kind-console-dev \
  --namespace=console-system --service-account=console-reader \
  --duration=1h --output=/private/reader-kubeconfig
```

The helper uses TokenRequest and writes a separate file with mode `0600`. It never prints the token. For the hosting
cluster, use the console's mounted ServiceAccount token. For remote clusters, enable
[automatic renewal](credential-renewal.md) with the named TokenRequest grant, or replace credentials manually before
expiry. Kubeconfig import rejects authentication plugins. Keep reader kubeconfigs out of Git.

## Check the connection

```sh
./console --config=/private/enrollment.yaml --check
```

This command needs neither PostgreSQL nor OIDC. It prints approved connection, API, controller, and deployment
projections. It exits with status 1 when a connection or enrolled controller read fails. A controller with zero ready
replicas or a sealed OpenBao deployment can still pass the transport check; inspect their separate health fields.

| Result        | Meaning                                  | Action                                                             |
| ------------- | ---------------------------------------- | ------------------------------------------------------------------ |
| `available`   | The namespace list succeeded.            | Inspect workload evidence.                                         |
| `unsupported` | The API is not served, or the adapter does not support it.          | Check the pinned adapter contract before changing the operator. |
| `denied`      | Kubernetes denied the read.              | Check RBAC and credential expiry.                                  |
| `unreachable` | The read or reader configuration failed. | Check the kubeconfig, context, API endpoint, and network.          |
| `notChecked`  | Collection stopped before this API read. | Resolve the preceding failure.                                     |

API availability does not verify an unreleased field schema or an enabled controller feature gate. Absent optional
restore APIs do not block core inventory. Incomplete optional reads remain capability warnings and leave the
affected analysis incomplete. Rejected credentials and failed required reads preserve previous inventory. The native
recovery capability is unsupported by the adapter and has no read timestamp; the console does not request it. A
missing or denied controller Deployment affects operator evidence independently of OpenBao collection.

## Seed enrollment with Helm

Enrollment YAML seeds the first start only. After that, manage connections and access in **Settings**; changes to the
seed are ignored. See the [installation guide](installation.md) for the runtime Secret and OIDC settings.

For Kubernetes, adapt `examples/live-values.yaml`. Set `readerNamespaces` for local resource collection and
`operatorReaders` for the exact controller Deployments, with one `operatorReaders` entry per namespace. Mount remote
reader kubeconfigs with `kubeconfigSecrets`. `examples/multi-cluster-enrollment.yaml` shows two clusters with separate
reader credentials and operator installations.

```sh
helm upgrade --install openbao-operator-console charts/openbao-operator-console \
  --namespace console-system --values /private/live-values.yaml
```

Each connection is collected on its own 30-second schedule, with at most one collection in flight per connection and at
most 16 across the fleet. A slow or unreachable connection does not delay the next collection of another connection. One
broken kubeconfig does not prevent startup or collection from other connections. Evidence becomes stale after two
minutes without a completed collection.
