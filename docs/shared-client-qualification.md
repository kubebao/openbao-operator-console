# Shared client compatibility and qualification

The console pins the public client candidate `v0.2.1-0.20261114115514-8981014162d0`, commit
`8981014162d00a902584dff0e5a2b9d5fd519e5d`. Evidence shapes and diagnostic behavior follow the pinned client release.
Captures and policies have no independent evidence, rule-set, or adapter version checks. The existing persisted
fleet projections, deployment IDs, routes, and history remain unchanged. No database migration, local Go workspace, or
replacement is required.

## Selected API contracts

Each connection selects one API group. Existing enrollment and stored configuration without `apiGroup` retain
`openbao.org`. New browser enrollment starts with `kubebao.org` and offers both groups. The reader does not discover
another API or fall back after a failed read.

| API group | Inspected client contract | Operator integration baseline |
| --- | --- | --- |
| `openbao.org` | `openbao.org/v1alpha1` | Released operator `0.6.1` |
| `kubebao.org` | `kubebao.org/v1alpha1` | Enrollment/controller baseline `22894cdc`; nonempty restore observations at `587dd163` |

The native enrollment and controller baseline is candidate `22894cdc0a4aa5c442a07f2bf9902fe28ce02ea1`. The separate
nonempty restore checks cover native build `587dd16386bc5a234520e22eec1bbedb904fa6dd` under the inspected field contract.
The client's native adapter now identifies operator `0.7.0-rc.1`, source
`7b9bbf8b1a2aa3a62e2235607073df3e912643b5`, as its inspected source. The console integration baselines remain those
listed above; the client source declaration does not expand their qualification.
Matching API discovery does not qualify another build or establish support for a released native operator. The
operator's workload metadata retains `openbao.org/*` keys in both groups.

The library also provides `openbao.org/v1alpha1/unqualified` and `kubebao.org/v1alpha1/unqualified` projections. They
leave omission-derived values and interpreted restore UID bindings unknown. The console selects the inspected adapters
in the table at the read boundary. Diagnosis validates typed evidence, API identity, and collection scope.

## Read scopes

The console reads namespace LISTs of clusters and restores, another cluster LIST for target verification, and GETs of
enrolled named controller Deployments. It does not add named cluster GETs, Pod reads, Secret reads, OpenBao requests,
or workload mutations. Credential renewal remains a separate TokenRequest permission.

Generated reader RBAC names only the selected group. Helm `readerScopes` grants the declared namespace/group pairs;
legacy `readerNamespaces` remains legacy-only. Declaring both groups in one namespace grants both LIST scopes, so an
administrator must declare each group it needs. Named controller GET grants remain separate.

## Durable identity and upgrade compatibility

An enrolled connection ID cannot change API group. Private bindings in encrypted settings retain that restriction after
removal and restart. They do not enter HTTP settings responses. Older settings without bindings infer retained connection
IDs as legacy before accepting edits. Loading them does not rewrite ciphertext, revision, or audit history.

Enroll a different API under a new connection ID. Migration annotations, shared names, or copied UIDs do not join history
across API groups. Invalid or inconsistent declared bindings fail validation. See [rollback guidance](upgrading.md#roll-back)
before running an older console against state that contains native enrollment or bindings.

## Validation boundary

### Enrollment and controller baseline

Enrollment and controller checks cover legacy operator `0.6.1` and native candidate `22894cdc`. They use Kubernetes 1.36.1
on Linux/arm64 with OpenBao 2.7.2, self-initialization, the Development profile, one voter, and enabled admission policies.
The console reads through a short-lived reader ServiceAccount token.
Permission checks allow only namespace cluster/restore LISTs and the enrolled controller GET; they deny cluster GET,
other-group LISTs, Pod LISTs, Secret GETs, workload PATCHes, restore creation, other controllers, and cluster-wide LISTs.

`TestKindOperatorProjection` verifies UID, versions, replicas, conditions, current selected analysis, healthy state, and
controller identity. `TestKindPostgresAndOIDCSession` combines real operator reads, PostgreSQL persistence, a signed test
identity authority, authorization, connection preview, and the HTTP contract. This is not external Keycloak qualification.

SQLite and PostgreSQL tests cover legacy settings backfill, retained history, connection removal/re-enrollment, restart,
rejected edits without committed changes, and encrypted settings/credential persistence. Browser tests cover both API
choices through onboarding and edit. Generated-contract checks verify OpenAPI and sqlc outputs against their inputs.

### Backup failure-count evidence

The operator omits a zero `status.backup.consecutiveFailures`. The inspected adapter implies zero only when backup
status reports both `lastBackupTime` and `lastAttemptTime`, with the success at or after the attempt. A later attempt,
missing timestamps, or invalid values leave the counter unknown. Reported counts keep their reported values.
The console accepts that evidence through the shared backup rules and its existing nullable counter projection.

Synthetic collector tests cover both API groups: a successful latest attempt permits Healthy state, an overdue success
raises the backup-age finding, and a later unresolved attempt keeps analysis incomplete and health Unknown.
Read-only checks against native operator build `587dd163` verify the same success interpretation on the demo's three
running deployments with current backups. The restore target retains its historical restore association gap and its
backup-age finding. These checks do not execute a backup or qualify other operator builds.

### Nonempty restore observations

`TestNonemptyRestoreProjection` verifies the declared request and target UIDs, their documented association, selected
console recovery projection, and finding severity. It persists the real collected batch to temporary SQLite storage and
verifies it after reopening the database. It uses only namespace cluster and restore LISTs.

Native checks use operator build `587dd16386bc5a234520e22eec1bbedb904fa6dd` in the migrated demo, Kubernetes
1.36.1+k3s1 on Linux/amd64, and OpenBao 2.7.2. They cover:

- A historical UID-confirmed `Unknown` request, rendered with high severity. Three older unbound requests keep
  diagnostic coverage incomplete.
- A held disposable request reporting `Unknown`, successful execution, target application, and pending cleanup within
  its configured retention interval. Diagnosis renders this as informational and preserves the reported `Unknown`
  phase.

The legacy check uses released operator `0.6.1` on isolated Kind Kubernetes 1.36.1, Linux/arm64, and OpenBao 2.7.2. A
fixture verifies a sample KV value, takes a backup, changes the value, and reads the snapshot value after restoring into
its own source. The operator retains `Unknown` and a hold; the console preserves the high-severity recovery concern.
The sample-data proof does not replace operator completion evidence. The legacy observer permission matrix permits
only its declared LISTs and denies Secret reads, workload mutations, restore creation, and the other API group.

Native and legacy synthetic tests cover finding identity and first-seen time through incomplete reads, restart, request
UID replacement, and resolution. They also verify that a retained `Completed` receipt cannot recreate a deleted target
or rewrite its history. The separate library runtime check of a terminal receipt after disposable target cleanup does
not qualify a current console deployment for an absent target.

Other restore failure and identity-conflict branches remain unit and fixture coverage. PostgreSQL, signed-session HTTP,
browser, and controller qualification retain their separate enrollment baseline. Native recovery, replication,
upgrades, scheduled restore-test execution, HA, and production operation remain outside these restore checks. Exact candidate, image, CRD, command, and
result identities belong in the qualification pull request and release notes.
