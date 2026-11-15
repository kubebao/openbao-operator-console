# Development

This guide covers local development and the test environments. See [CONTRIBUTING.md](../CONTRIBUTING.md) for the
workflow and conventions, and [installation](installation.md) for running the console against real clusters.

## Repository layout

| Path | Contents |
| --- | --- |
| `cmd/console` | Executable entry point, CLI flags, and first-start configuration |
| `internal/auth` | OpenID Connect login and sessions |
| `internal/collector` | Shared client orchestration and mappings; named controller observation |
| `internal/config` | Enrollment, credential validation, and reader RBAC generation |
| `internal/contract` | Generated API transport types |
| `internal/demo` | Demonstration fixtures |
| `internal/evidence` | Shared finding mappings, product health, and activity transitions |
| `internal/fleet` | Domain model for deployments, connections, and controllers; defines the stored observation format |
| `internal/httpapi` | HTTP API, contract projection, and embedded web assets |
| `internal/kube` | Kubernetes REST clients for embedded reader credentials; never follow redirects |
| `internal/renewal` | ServiceAccount token renewal worker |
| `internal/settings` | Versioned, encrypted console configuration |
| `internal/store` | SQLite and PostgreSQL persistence, schemas, and sqlc queries |
| `internal/view` | Reconciles retained observations with current enrollment and marks stale evidence |
| `api/openapi.yaml` | API contract; Go and TypeScript types are generated from it |
| `web/` | React application, unit tests (`src`), browser tests (`e2e`), and screenshot capture (`screenshots`) |
| `charts/openbao-operator-console` | Helm chart |
| `examples/` | Enrollment, Helm values, and RBAC examples |

## Demo workspace

```sh
make install
make dev
```

Open <http://127.0.0.1:5173>. `make dev` builds the API, runs it in demo mode on port 8090, and starts the Vite
development server. Demo mode runs on loopback, shows a demonstration notice, and needs no PostgreSQL or Kubernetes.

To run the compiled application with embedded assets:

```sh
make build
./bin/console --mode=demo
```

Open <http://127.0.0.1:8090>.

## Checks

| Command | Coverage |
| --- | --- |
| `make check` | `make lint`, Go race tests; TypeScript strict checking; oxlint with type-aware rules; Prettier; unit tests; frontend build |
| `make lint` | golangci-lint with the pinned version and `.golangci.yml`, including gofumpt, goimports, and golines |
| `make lint-fix` | Applies the Go formatters and automatic golangci-lint fixes |
| `make fuzz` | Fuzzes authentication redirects and forwarded client addresses for 30 seconds per target; set `FUZZ_TIME` to change the duration |
| `make secret-scan` | Pinned Gitleaks scan of all reachable local Git history; output is redacted |
| `make check-generated` | OpenAPI Go/TypeScript and sqlc output drift |
| `make e2e` | Chromium workflows through the development server |
| `make e2e-production` | The same workflows through the compiled Go application and its content security policy |
| `make e2e-storage` | Real SQLite browser setup, restart, and backup restore with recovery-code replay checks |
| `make integration` | SQLite and PostgreSQL persistence; optional Kubernetes, Keycloak, and renewal checks |
| `make helm-check` | Strict Helm lint and rendering with each `charts/openbao-operator-console/ci/*-values.yaml` file, and values schema rejection checks |
| `make screenshots` | Regenerates the README screenshots in `docs/images` from demo data |
| `make build` | Web assets and `bin/console`, with the version from `git describe` and the commit embedded |
| `make dist` | Release archives in `dist/` for Linux and macOS; set `DIST_PLATFORMS` to choose platforms |
| `make image` | Container image for the local platform; set `IMAGE` to choose the tag |
| `make licenses` | Regenerates `THIRD_PARTY_LICENSES` from Go and production web dependencies |
| `make check-licenses` | Fails when `THIRD_PARTY_LICENSES` is out of date |
| `make check-release-tools` | Edge source/artifact boundary tests, publication failure tests, ShellCheck, and pinned actionlint |
| `make vulncheck` | `govulncheck` for reachable vulnerabilities in Go code and the standard library |
| `make audit` | `pnpm audit` for production web dependencies |

Install the test browser before the first browser run:

```sh
PLAYWRIGHT_SKIP_BROWSER_GC=1 pnpm --dir web exec playwright install chromium
```

## Continuous integration

`.github/workflows/check.yml` runs on every pull request and push to `main`. A newer push to a pull request cancels the
running checks. The jobs run in parallel:

| Job | Runs |
| --- | --- |
| Go | `make lint`, one uncached Go race test run with SQLite and PostgreSQL 18 persistence integration, and 10 seconds of fuzzing per target |
| Web | Type checking, oxlint, Prettier, unit tests, and the production build |
| E2E (development) | Browser workflows through the development server |
| E2E (production) | One application build, compiled-application browser workflows, content security policy, and SQLite restart/restore |
| Generated code | `make check-generated` |
| Third-party licenses | `make check-licenses` |
| Helm (v3 and v4) | `make helm-check` and chart-testing `ct lint` with `.github/ct.yaml` |
| Go vulnerabilities | `make vulncheck` |
| Web dependency audit | `make audit` |
| Container image | A `linux/amd64` and `linux/arm64` Buildx build without push |

The browser jobs use the official Playwright image, pinned by digest, with Chromium and its system libraries
preinstalled. They do not install Ubuntu packages or restore a browser cache. Keep the image version in both jobs
aligned with `@playwright/test` in `web/package.json` when updating Playwright.
Each container trusts only the checked-out workspace path for Git operations. The development job compiles the API
before launching Playwright and selects it with `CONSOLE_E2E_PREBUILT=1`, so a cold Go build does not consume the
server-readiness timeout. Local development browser tests keep using `go run` by default.
The production job builds once with `scripts/build.sh` and reuses the binary for both browser suites. `make build`
uses the same script for local builds. The Go job includes persistence integration in its full test run, so it does
not rerun those packages separately.

The Docker build and CI bootstrap Corepack with `npm ci` and the integrity hashes in
`tools/corepack/package-lock.json`. Corepack verifies the pnpm archive against the version and SHA-512 hash in
`web/package.json`. Dependabot updates the Corepack manifest and lockfile with the other npm dependencies.

Fuzz seed inputs run with the regular Go tests. The Go CI job also generates inputs for 10 seconds per target.
`.github/workflows/fuzz.yml` runs each target for two minutes weekly and supports manual runs. Both fuzzing targets
use two workers. The scheduled workflow uploads failing inputs for reproduction.

The Container image job also builds edge archives and a digest-pinned chart, then installs it in a disposable kind
cluster with SQLite persistence and optional PostgreSQL. `scripts/edge-tools.env` pins its registry, kind, and fixture versions. Release tooling checks
require Python 3.11 or later and ShellCheck. `make check-release-tools` runs actionlint through its pinned Go module.

Other workflows run CodeQL for Go and TypeScript, OpenSSF Scorecard, and dependency review. These jobs run only while
the repository is public: CodeQL and dependency review require the unavailable private-repository security features,
and Scorecard publishes public results. Every
action is pinned to a commit SHA, and workflows default to read-only repository permissions. Dependabot proposes weekly
updates for Go modules, web dependencies, actions, and base images.

Dependabot groups patch and minor action updates separately from publishing actions. Publishing action updates need
validation of their publication path before merging. Playwright updates have separate pull requests because the
package version and both pinned browser images must change together. Major action and npm updates also have separate
pull requests.

Releases are automated with Release Please and publish signed images and charts. See [releases](releasing.md).

## Generated code

Go transport types, TypeScript API types, and sqlc queries are generated. After changing `api/openapi.yaml` or
`internal/store/queries.sql`, run:

```sh
make generate
make check-generated
```

## Styles

The web application uses plain CSS in `web/src/styles`, with one file per area. `index.css` imports them, and its import
order is the cascade order. `reset.css` holds the element reset in a cascade layer, so every unlayered rule overrides
it. Use the color, type, radius, and stacking tokens in `tokens.css` instead of repeating literal values.

## Storage tests

SQLite persistence and recovery tests run in temporary directories during `go test -race ./...`. With
`CONSOLE_TEST_DATABASE_URL` set, the same storage contract also runs against isolated PostgreSQL schemas. Schema tests
require `pg_dump` and `pg_restore` compatible with the PostgreSQL server. The CI service supplies its matching tools.
`sqlc.yaml` generates separate query adapters for the engine-specific DDL. Never edit either generated package.

## Local PostgreSQL

The Compose database listens on loopback. Its credentials are disposable development values.

```sh
docker compose up -d --wait
export CONSOLE_TEST_DATABASE_URL='postgres://console:console-local-only@127.0.0.1:55439/console_test?sslmode=disable'
make integration
```

Integration tests require a database named `console_test` and reject other names. `docker compose down` keeps the named
volume; remove it only when its data is disposable.

## Isolated Kubernetes test

`make integration-cluster` creates a dedicated kind cluster, `console-dev`, with its own kubeconfig in `.local`, so the
tests never touch your other clusters. It needs Docker, kind, kubectl, Helm 3.x or 4, and Python 3. The script:

1. Creates the cluster from the node image pinned in `scripts/edge-tools.env`.
2. Installs the [OpenBao Operator](https://github.com/kubebao/openbao-operator) chart and image `0.6.1` in single-tenant
   mode as the Helm release `console-operator` in `console-system`, targeting `console-sample`. Set
   `OPENBAO_OPERATOR_VERSION` to test another version.
3. Applies `examples/kind-cluster.yaml` with `spec.network.apiServerEndpointIPs` set to the kind API server address.
   Without it, kindnet blocks the API server and the sample stays `Initializing`.
4. Waits up to 10 minutes for `console-bao` to report `Available`, and fails with its conditions if it does not.
5. Applies `examples/reader-rbac.yaml` and issues a one-hour reader kubeconfig in `.local/reader-kubeconfig`.

```sh
make integration-cluster
export CONSOLE_TEST_KUBECONFIG="$PWD/.local/reader-kubeconfig"
make integration
```

For another isolated cluster, set `CONSOLE_TEST_CONTEXT` to its kubeconfig context name. Set
`CONSOLE_TEST_API_GROUP=kubebao.org` for a native operator; omission keeps the legacy API. Set
`CONSOLE_TEST_OPENBAO_VERSION` when the fixture declares another version.

The test reads only `console-sample` and expects `console-bao` to declare OpenBao 2.7.0 and one replica, and to be
healthy. It does not run a restore or recovery. The reader credential expires after one hour; run
`make integration-cluster` again to reuse the cluster and issue a new one. Delete the cluster when you are done:

```sh
make integration-cluster-down
```

### Nonempty restore projection

`TestNonemptyRestoreProjection` reads an existing UID-confirmed restore through a caller-supplied reader credential.
Provision the target and request separately. The test creates no Kubernetes resources and uses only namespace cluster
and restore LISTs. It persists the collected projection to temporary SQLite storage and verifies it after reopening the
database. It does not require an enrolled controller Deployment.

Declare the namespace, target and request identities, reported phase, and expected console finding before running it:

```sh
CONSOLE_TEST_KUBECONFIG=/path/to/reader-kubeconfig \
CONSOLE_TEST_CONTEXT=qualification \
CONSOLE_TEST_API_GROUP=kubebao.org \
CONSOLE_TEST_RESTORE_NAMESPACE=restore-qualification \
CONSOLE_TEST_RESTORE_TARGET=restore-target \
CONSOLE_TEST_RESTORE_TARGET_UID=target-uid \
CONSOLE_TEST_RESTORE_REQUEST=restore-request \
CONSOLE_TEST_RESTORE_UID=request-uid \
CONSOLE_TEST_RESTORE_PHASE=Unknown \
CONSOLE_TEST_RESTORE_FINDING=high \
GOWORK=off go test -race -count=1 ./internal/collector -run '^TestNonemptyRestoreProjection$'
```

Replace the identity values with the caller-provisioned records. The phase must be `Pending`, `Validating`, `Running`,
`Completed`, `Failed`, or `Unknown`. The finding expectation must be `none`, `info`, `high`, or `critical`. Declare the
finding separately because a `Completed` phase does not exclude a recorded cleanup concern. The test skips when the
restore tuple is unset and rejects partial or unsupported settings. A passing check verifies the declared observation;
it does not establish end-to-end recovery or qualify a different operator build.

### Credential renewal

To run the real renewal check, generate RBAC with renewal enabled for a dedicated `console-renewal-test` ServiceAccount
in `console-system`, apply it on the `kind-console-dev` context, and issue its reader kubeconfig. Pass the file in
`CONSOLE_TEST_RENEWAL_KUBECONFIG` when running `make integration`.

## Keycloak

`compose.keycloak.yaml` runs a disposable Keycloak 26.8.2-2 instance with HTTPS on loopback. A helper generates one
development realm, one confidential PKCE client, one test user, and a seven-day certificate. Passwords and secrets are
written to the ignored `.local/keycloak` directory with mode `0700`. The helper refuses to overwrite an existing setup
and does not change workstation certificate trust.

```sh
python3 scripts/keycloak-dev.py
docker compose --env-file .local/keycloak/compose.env -f compose.keycloak.yaml up -d
curl --cacert .local/keycloak/server.crt \
  https://localhost:8543/realms/console-dev/.well-known/openid-configuration
```

Run the provider login check with SQLite:

```sh
export CONSOLE_TEST_KEYCLOAK_CONFIG="$PWD/.local/keycloak/credentials.json"
go test -race -count=1 ./internal/httpapi -run '^TestKeycloakSession$'
```

The `Keycloak integration` workflow runs this check when the provider fixture, authentication code, HTTP API, or Go
dependencies change. It checks the authorization-code flow with PKCE, the enrolled identity, and console logout over
verified TLS. It does not require Kubernetes or PostgreSQL and does not qualify an existing realm migration or
provider-wide SSO logout.

With the local PostgreSQL and the kind reader credential configured, also run the combined reader and login checks:

```sh
make integration
```

The test uses port `8554` for its HTTPS callback. It signs in through Keycloak's login form, exchanges the code with
PKCE, reads persisted observations through the scoped API, and verifies logout, without password grants or skipped TLS
checks. Stop the development identity provider with:

```sh
docker compose --env-file .local/keycloak/compose.env -f compose.keycloak.yaml down
```

## Container image

```sh
make image
docker buildx build --platform linux/amd64,linux/arm64 .
```

The multi-stage build compiles the web application and Go binary from digest-pinned base images and produces a
distroless image that runs as user and group 65532. The build stages run on the build platform and cross-compile for the
target platform, so a multi-platform build needs no emulation. Corepack installs the pnpm version from the
`packageManager` field in `web/package.json`. The `VERSION` and `COMMIT` build arguments set the embedded build
information and the OCI image labels.
