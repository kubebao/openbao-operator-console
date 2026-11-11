# Shared client migration qualification

The console pins the public client release `v0.1.0`, commit `1ab29173f8b94b9607058b7ccc04e98f35f59224`.
The [final candidate qualification](https://github.com/kubebao/openbao-operator-console/pull/26) records local
application, browser, SQLite/PostgreSQL, scoped operator, and distribution checks against that commit.
The [tagged dependency update](https://github.com/kubebao/openbao-operator-console/pull/27) records independent public
module consumption, local checks, and tagged distribution verification. Hosted CI did not pass for those updates;
their local results do not qualify hosted publication.

The migration record below covers the earlier client commit `7926f09468de82a04e5322b2062e1d579f9fbdf4`, published as
`v0.0.0-20261111084856-7926f09468de`. Validation ran locally on 11 November 2026 with Go 1.27.4.
It does not qualify lifecycle operations, native recovery, remote replication, or another operator API group.

## Local checks

| Check | Result |
| --- | --- |
| `make check` | Go lint and race tests, strict TypeScript, web lint/format, 125 web unit tests, and production asset build pass |
| `make check-generated` | Go/TypeScript OpenAPI and sqlc outputs match their sources |
| `make integration` | SQLite and PostgreSQL 18 persistence, collection, renewal, and HTTP tests pass |
| Isolated operator integration | Scoped collection and the PostgreSQL/HTTP session workflow pass with the signed TLS test identity provider |
| `make e2e` | 82 development browser tests pass; two production-only content security policy checks are excluded |
| `make e2e-production` | 84 compiled application browser workflows and content security policy checks pass |
| `make e2e-storage` | Real SQLite browser setup, restart, backup restore, and recovery-code replay checks pass |
| `make screenshots` | Documentation captures pass, including incomplete analysis and an unknown backup counter |
| `make vulncheck` | No reachable vulnerabilities; one vulnerability exists in an unused required module |
| `make licenses`, `make check-licenses` | Dependency notices regenerated and verified |
| `make helm-check` | All chart variants render and lint; invalid values are rejected |
| `make image IMAGE=openbao-operator-console:client-m4` | Local image builds with the public client dependency |
| `GOWORK=off go build -trimpath -o .local/client-m4/console-independent ./cmd/console` | Independent module build passes; the client has no replacement directive |

External Keycloak and real credential-renewal integration tests remain optional and were not run. The renewal unit
suite validates issued credentials through the migrated collector. Existing environments were not used.

## Isolated operator evidence

A disposable kind cluster named `console-client-m4` runs Kubernetes 1.36.1 on Linux/arm64. The Helm release
`console-operator` uses chart 0.6.1 and image `ghcr.io/kubebao/openbao-operator:0.6.1` in single-tenant mode for
`console-sample`. The chart digest is
`sha256:03bc763ade61590f3616bff2738b379453554084cee12aa61035503c3d9fc338`.
The running image ID is
`ghcr.io/kubebao/openbao-operator@sha256:8a73e5a7c0f13d6786e7ca6d65af20cb36620d4464dc07a6e8210e59d17bcb97`.

Installed CRD spec hashes use SHA-256 over sorted, compact JSON of each `spec`:

| CRD | SHA-256 |
| --- | --- |
| `openbaoclusters.openbao.org` | `7c3659f144c375092d7d1629d65004c15cc30ec41dd81e04a400842ada82ed14` |
| `openbaorestores.openbao.org` | `d3c781479988c032e00e45c6ec4c29f66aba1d11413f65f11f98dd0901b0ca64` |

The console reads through a short-lived `console-system/console-reader` ServiceAccount credential. Its Roles permit
namespace LISTs of clusters and restores, plus GET of the enrolled controller Deployment. Access checks confirm that
cluster LIST and the named Deployment GET are allowed, while cluster GET and Pod LIST are denied.

`TestKindOperatorProjection` confirms the real resource UID, versions, replicas, conditions, controller identity,
current collection, complete selected analysis, and healthy state after the operator reports one ready replica at
generation 1. The final verification uses another namespace LIST. No named cluster GET or Pod read is added.
`TestKindPostgresAndOIDCSession` exercises collection, persistence, authorization, and the HTTP contract together.

This qualifies the exercised namespace collection, basic cluster projection, generation verification, selected analysis
for that cluster, and named controller observation. An empty restore LIST does not qualify restore execution or restore
association against a live request. Backups, upgrades, and restore tests have synthetic coverage only in this run.
