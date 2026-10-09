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
| `internal/httpapi` | HTTP API, authorization, and embedded web assets |
| `internal/renewal` | ServiceAccount token renewal worker |
| `internal/settings` | Versioned, encrypted console configuration |
| `internal/store` | PostgreSQL persistence, migrations, and sqlc queries |
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
| `make helm-check` | Helm lint and template rendering |
| `make screenshots` | Regenerates the README screenshots in `docs/images` from demo data |

Install the test browser before the first browser run:

```sh
PLAYWRIGHT_SKIP_BROWSER_GC=1 pnpm --dir web exec playwright install chromium
```

The workflow in `.github/workflows/check.yml` runs generated-code drift, `make check`, PostgreSQL integration, the build,
and browser tests. It does not publish an image.

## Generated code

Go transport types, TypeScript API types, and sqlc queries are generated. After changing `api/openapi.yaml` or
`internal/store/queries.sql`, run:

```sh
make generate
make check-generated
```

## Local PostgreSQL

The Compose database listens on loopback. Its credentials are disposable development values.

```sh
docker compose up -d --wait
export CONSOLE_TEST_DATABASE_URL='postgres://console:console-local-only@127.0.0.1:55439/console_test?sslmode=disable'
make integration
```

Integration tests require a database named `console_test` and reject other names. `docker compose down` keeps the
named volume; remove it only when its data is disposable.

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

The test reads only `console-sample` and expects the `console-bao` resource to declare OpenBao 2.7.0 and one replica.
It does not run a restore or recovery. Delete the kind cluster when you are done:

```sh
kind delete cluster --name console-dev --kubeconfig .local/kubeconfig
```

### Credential renewal

To run the real renewal check, generate RBAC with renewal enabled for a dedicated `console-renewal-test` ServiceAccount
in `console-system`, apply it on the `kind-console-dev` context, and issue its reader kubeconfig. Pass the file in
`CONSOLE_TEST_RENEWAL_KUBECONFIG` when running `make integration`.

## Keycloak

`compose.keycloak.yaml` runs a disposable Keycloak 26.8.0 instance with HTTPS on loopback. A helper generates one
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
docker build -t openbao-operator-console:local .
```

The multi-stage build compiles the web application and Go binary from pinned base images and produces a distroless,
non-root image.
