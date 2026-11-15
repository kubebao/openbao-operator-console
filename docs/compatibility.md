# Compatibility

This page lists the operator API contracts the console reads, the reads it performs, and how each contract is
validated. Exact candidate, image, CRD, command, and result identities for each qualification belong in the
qualification pull request and release notes.

## Shared client

The console pins the public client candidate `v0.2.1-0.20261114115514-8981014162d0`, commit
`8981014162d00a902584dff0e5a2b9d5fd519e5d`. Evidence shapes and diagnostic behavior follow the pinned client release.
Captures and policies have no separate evidence, rule-set, or adapter version checks. See the client's
[compatibility document](https://github.com/kubebao/openbao-operator-client/blob/8981014162d00a902584dff0e5a2b9d5fd519e5d/docs/compatibility.md)
for its adapter sources and validation levels.

## Kubernetes

The console runs on Kubernetes 1.33 or later. The observed-cluster validation below uses Kubernetes 1.36.1. It does not
cover other Kubernetes versions or platforms.

## API contracts

Each connection selects one API group. Stored configuration without `apiGroup` uses `openbao.org`. Browser enrollment
starts with `kubebao.org` and offers both groups. The reader does not discover another API or fall back after a failed
read.

| API group | Client contract | Operator integration baseline |
| --- | --- | --- |
| `openbao.org` | `openbao.org/v1alpha1` | Released operator `0.6.1` |
| `kubebao.org` | `kubebao.org/v1alpha1` | Enrollment and controller baseline `22894cdc`; nonempty restore observations at `587dd163` |

The native enrollment and controller baseline is candidate `22894cdc0a4aa5c442a07f2bf9902fe28ce02ea1`. The nonempty
restore checks cover native build `587dd16386bc5a234520e22eec1bbedb904fa6dd`. The client's native adapter identifies
operator `0.7.0-rc.1`, source `7b9bbf8b1a2aa3a62e2235607073df3e912643b5`, as its inspected source. That declaration does
not extend the console baselines in the table. Matching API discovery does not qualify another build or establish
support for a released native operator. The operator's workload metadata uses `openbao.org/*` keys in both groups.

The client also provides `openbao.org/v1alpha1/unqualified` and `kubebao.org/v1alpha1/unqualified` projections. They
leave omission-derived values and interpreted restore UID bindings unknown. The console selects the inspected adapters
in the table at the read boundary. Diagnosis validates typed evidence, API identity, and collection scope.

An enrolled connection ID cannot change API group. Enroll a different API under a new connection ID. Migration
annotations, shared names, or copied UIDs do not join history across API groups. Invalid or inconsistent declared
bindings fail validation. Read [roll back](upgrading.md#roll-back) before you run an older console against state that
contains native enrollment or bindings.

## Read scopes

The console reads namespace LISTs of clusters and restores, a second cluster LIST for target verification, and GETs of
enrolled named controller Deployments. It does not perform named cluster GETs, Pod reads, Secret reads, OpenBao
requests, or workload mutations. Credential renewal uses a separate TokenRequest permission.

Generated reader RBAC names only the selected group. Helm `readerScopes` grants the declared namespace and group pairs.
`readerNamespaces` grants only the legacy group. Declaring both groups in one namespace grants both LIST scopes, so you
must declare each group you need. Named controller GET grants are separate.

## Validation levels

### Enrollment and controller baseline

Enrollment and controller checks cover legacy operator `0.6.1` and native candidate `22894cdc`. They use Kubernetes
1.36.1 on Linux/arm64 with OpenBao 2.7.2, self-initialization, the Development profile, one voter, and enabled admission
policies. The console reads through a short-lived reader ServiceAccount token. Permission checks allow only namespace
cluster and restore LISTs and the enrolled controller GET. They deny cluster GET, other-group LISTs, Pod LISTs, Secret
GETs, workload PATCHes, restore creation, other controllers, and cluster-wide LISTs.

`TestKindOperatorProjection` verifies UID, versions, replicas, conditions, current selected analysis, healthy state, and
controller identity. `TestKindPostgresAndOIDCSession` combines real operator reads, PostgreSQL persistence, a signed
test identity authority, authorization, connection preview, and the HTTP contract. This is not external Keycloak
qualification.

SQLite and PostgreSQL tests cover settings without API bindings, retained history, connection removal and
re-enrollment, restart, rejected edits without committed changes, and encrypted settings and credential persistence.
Browser tests cover both API choices through onboarding and edit. Generated-contract checks verify OpenAPI and sqlc
outputs against their inputs.

### Backup failure-count evidence

The operator omits a zero `status.backup.consecutiveFailures`. The inspected adapter implies zero only when backup
status reports both `lastBackupTime` and `lastAttemptTime`, with the success at or after the attempt. A later attempt,
missing timestamps, or invalid values leave the counter unknown. Reported counts keep their reported values.

Synthetic collector tests cover both API groups. A successful latest attempt permits Healthy state, an overdue success
raises the backup-age finding, and a later unresolved attempt keeps analysis incomplete and health Unknown. Read-only
checks against native operator build `587dd163` verify the same success interpretation on three running deployments
with current backups. These checks do not execute a backup or qualify other operator builds.

### Nonempty restore observations

`TestNonemptyRestoreProjection` verifies the declared request and target UIDs, their documented association, the
selected recovery projection, and finding severity. It persists the collected batch to temporary SQLite storage and
verifies it after reopening the database. It uses only namespace cluster and restore LISTs.

Native checks use operator build `587dd16386bc5a234520e22eec1bbedb904fa6dd` on Kubernetes 1.36.1+k3s1, Linux/amd64, and
OpenBao 2.7.2. They cover:

- A UID-confirmed `Unknown` request, rendered with high severity. Older requests without UID bindings keep diagnostic
  coverage incomplete.
- A held disposable request that reports `Unknown`, successful execution, target application, and pending cleanup
  within its configured retention interval. Diagnosis renders it as informational and preserves the reported `Unknown`
  phase.

The legacy check uses released operator `0.6.1` on isolated Kind Kubernetes 1.36.1, Linux/arm64, and OpenBao 2.7.2. A
fixture verifies a sample KV value, takes a backup, changes the value, and reads the snapshot value after restoring into
its own source. The operator reports `Unknown` and a hold, and the console reports a high-severity recovery concern. The
sample-data check does not replace operator completion evidence. The legacy permission matrix permits only the declared
LISTs and denies Secret reads, workload mutations, restore creation, and the other API group.

Native and legacy synthetic tests cover finding identity and first-seen time through incomplete reads, restart, request
UID replacement, and resolution. They also verify that a retained `Completed` receipt cannot recreate a deleted target
or rewrite its history. Other restore failure and identity-conflict branches have unit and fixture coverage only.

## Unsupported

- The supported operator contract has no public API for native recovery or remote replication. The console shows that
  evidence as unknown.
- Upgrades, scheduled restore-test execution, HA, and production operation are not qualified.
- Lifecycle operations are outside the console's read-only collection scope.
