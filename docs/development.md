# Development

This guide covers local development and the test environments. See [CONTRIBUTING.md](../CONTRIBUTING.md) for the
workflow and conventions, and [installation](installation.md) for running the console against real clusters.

## Repository layout

| Path | Contents |
| --- | --- |
| `cmd/console` | Executable entry point, CLI flags, and first-start configuration |
| `internal/auth` | OpenID Connect login and sessions |
| `internal/collector` | Kubernetes collection and evidence adapters |
| `internal/config` | Enrollment, credential validation, and reader RBAC generation |
| `internal/contract` | Generated API transport types |
| `internal/demo` | Demonstration fixtures |
| `internal/evidence` | Attention rules |
| `internal/fleet` | Domain model for deployments, connections, and controllers; defines the stored observation format |
| `internal/httpapi` | HTTP API, contract projection, and embedded web assets |
| `internal/kube` | Kubernetes REST clients for embedded reader credentials; never follow redirects |
| `internal/renewal` | ServiceAccount token renewal worker |
| `internal/settings` | Versioned, encrypted console configuration |
| `internal/store` | PostgreSQL persistence, schema, and sqlc queries |
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
| `make check-generated` | OpenAPI Go/TypeScript and sqlc output drift |
| `make e2e` | Chromium workflows through the development server |
| `make e2e-production` | The same workflows through the compiled Go application and its content security policy |
| `make integration` | PostgreSQL persistence; optional Kubernetes, Keycloak, and renewal checks |
| `make helm-check` | Strict Helm lint and rendering with each `charts/openbao-operator-console/ci/*-values.yaml` file, and values schema rejection checks |
| `make screenshots` | Regenerates the README screenshots in `docs/images` from demo data |
| `make build` | Web assets and `bin/console`, with the version from `git describe` and the commit embedded |
| `make image` | Container image for the local platform; set `IMAGE` to choose the tag |
| `make licenses` | Regenerates `THIRD_PARTY_LICENSES` from Go and production web dependencies |
| `make check-licenses` | Fails when `THIRD_PARTY_LICENSES` is out of date |
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
| Go | `make lint`, `go test -race ./...`, and `make integration` against a PostgreSQL 18 service |
| Web | Type checking, oxlint, Prettier, unit tests, and the production build |
| E2E (development) | `make e2e` |
| E2E (production) | `make e2e-production` |
| Generated code | `make check-generated` |
| Third-party licenses | `make check-licenses` |
| Helm (v3 and v4) | `make helm-check` and chart-testing `ct lint` with `.github/ct.yaml` |
| Go vulnerabilities | `make vulncheck` |
| Web dependency audit | `make audit` |
| Container image | A `linux/amd64` and `linux/arm64` Buildx build without push |

Other workflows run CodeQL for Go and TypeScript, OpenSSF Scorecard, and dependency review on pull requests. Every
action is pinned to a commit SHA, and workflows default to read-only repository permissions. Dependabot proposes weekly
updates for Go modules, web dependencies, actions, and base images.

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

Use a dedicated kind cluster and a separate kubeconfig so tests never touch your other clusters:

```sh
mkdir -p .local
kind create cluster --name console-dev --image kindest/node:v1.36.1 --kubeconfig .local/kubeconfig
```

Install the [OpenBao Operator](https://github.com/kubebao/openbao-operator) in single-tenant mode with `console-sample`
as the target namespace. Use an operator image and chart from the same version so admission policies match. Then apply
`examples/kind-cluster.yaml`, which creates one disposable development OpenBao deployment with a 1 GiB volume.

Apply `examples/reader-rbac.yaml` and issue a short-lived reader credential:

```sh
python3 scripts/reader-kubeconfig.py \
  --admin-kubeconfig=.local/kubeconfig --context=kind-console-dev \
  --namespace=console-system --service-account=console-reader \
  --duration=1h --output=.local/reader-kubeconfig
export CONSOLE_TEST_KUBECONFIG="$PWD/.local/reader-kubeconfig"
make integration
```

The test reads only `console-sample` and expects the `console-bao` resource to declare OpenBao 2.7.0 and one replica. It
does not run a restore or recovery. Delete the kind cluster when you are done:

```sh
kind delete cluster --name console-dev --kubeconfig .local/kubeconfig
```

### Credential renewal

To run the real renewal check, generate RBAC with renewal enabled for a dedicated `console-renewal-test` ServiceAccount
in `console-system`, apply it on the `kind-console-dev` context, and issue its reader kubeconfig. Pass the file in
`CONSOLE_TEST_RENEWAL_KUBECONFIG` when running `make integration`.

## Keycloak

`compose.keycloak.yaml` runs a disposable Keycloak 26.8.1-2 instance with HTTPS on loopback. A helper generates one
development realm, one confidential PKCE client, one test user, and a seven-day certificate. Passwords and secrets are
written to the ignored `.local/keycloak` directory with mode `0700`. The helper refuses to overwrite an existing setup
and does not change workstation certificate trust.

```sh
python3 scripts/keycloak-dev.py
docker compose --env-file .local/keycloak/compose.env -f compose.keycloak.yaml up -d
curl --cacert .local/keycloak/server.crt \
  https://localhost:8543/realms/console-dev/.well-known/openid-configuration
```

With the local PostgreSQL and the kind reader credential configured, run the real login check:

```sh
export CONSOLE_TEST_KEYCLOAK_CONFIG="$PWD/.local/keycloak/credentials.json"
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
