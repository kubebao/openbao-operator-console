# Shared client compatibility and qualification

The console pins the public client release `v0.2.0`, commit
`e64f20ca5d5e13d97d31f9adf06c0347e04a50df`. It uses evidence schema `4` for new collection and keeps its existing persisted
fleet projections, deployment IDs, routes, and history. No local Go workspace or replacement is required.

## Selected API contracts

Each connection selects one API group. Existing enrollment and stored configuration without `apiGroup` retain
`openbao.org`. New browser enrollment starts with `kubebao.org` and offers both groups. The reader does not discover
another API or fall back after a failed read.

| API group | Inspected client contract | Operator integration baseline |
| --- | --- | --- |
| `openbao.org` | `openbao.org/v1alpha1@9671c306/maintenance-pools-v1` | Released operator `0.6.1` |
| `kubebao.org` | `kubebao.org/v1alpha1@22894cdc/maintenance-pools-v1` | Native operator candidate `22894cdc0a4aa5c442a07f2bf9902fe28ce02ea1` |

Native qualification covers that candidate. Matching API discovery does not qualify another build or establish support
for a released native operator. The operator's workload metadata retains `openbao.org/*` keys in both groups.

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

The isolated operator checks use Kubernetes 1.36.1 on Linux/arm64 with OpenBao 2.7.2, self-initialization, the Development
profile, one voter, and enabled admission policies. The console reads through a short-lived reader ServiceAccount token.
Permission checks allow only namespace cluster/restore LISTs and the enrolled controller GET; they deny cluster GET,
other-group LISTs, Pod LISTs, Secret GETs, workload PATCHes, restore creation, other controllers, and cluster-wide LISTs.

`TestKindOperatorProjection` verifies UID, versions, replicas, conditions, current selected analysis, healthy state, and
controller identity. `TestKindPostgresAndOIDCSession` combines real operator reads, PostgreSQL persistence, a signed test
identity authority, authorization, connection preview, and the HTTP contract. This is not external Keycloak qualification.

SQLite and PostgreSQL tests cover legacy settings backfill, retained history, connection removal/re-enrollment, restart,
rejected edits without committed changes, and encrypted settings/credential persistence. Browser tests cover both API
choices through onboarding and edit. Generated-contract checks verify OpenAPI and sqlc outputs against their inputs.

A real empty restore LIST does not qualify restore execution or nonempty restore correlation. Native recovery,
replication, backups, upgrades, restore-test execution, HA, and production operation remain outside this runtime check.
Unit and fixture results remain separate from operator integration. Exact candidate, image, CRD, command, and result
identities belong in the qualification pull request and release notes.
