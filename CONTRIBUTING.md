# Contributing

Thank you for your interest in the OpenBao Operator Console. This guide covers the workflow and conventions for changes.
See [development](docs/development.md) for test environments in more detail.

## Set up

Requirements: Go 1.27, Node.js 26, pnpm 11, and Docker for integration tests. Use the pnpm version from the
`packageManager` field in `web/package.json`.

```sh
make install
make dev
```

`make dev` starts the API in demo mode and the Vite development server at <http://127.0.0.1:5173>. No database,
Kubernetes cluster, or identity provider is required.

## Check your change

Run the full check before opening a pull request:

```sh
make check
```

It runs golangci-lint, Go race tests, strict TypeScript checking, oxlint, Prettier, unit tests, and the frontend build.
Depending on the change, also run:

| Change | Command |
| --- | --- |
| API contract or SQL queries | `make generate`, then `make check-generated` |
| User workflows | `make e2e` (browser tests through the development server) |
| Production build or content security policy | `make e2e-production` |
| Persistence, collection, or renewal | `make integration` with a PostgreSQL test database |
| Helm chart | `make helm-check` |
| Go or production web dependencies | `make licenses`, then commit `THIRD_PARTY_LICENSES` |
| Dockerfile | `make image` |
| Visible UI changes shown in the README | `make screenshots` |

CI runs these checks as parallel jobs on every pull request, together with `govulncheck`, `pnpm audit`, CodeQL,
dependency review, and a multi-architecture image build. See [development](docs/development.md#continuous-integration).

## Project boundaries

These rules keep the console safe to run against production clusters. Changes that cross them will not be accepted.

- The console is a read-only observer. Do not add Kubernetes mutations, Secret reads, Pod exec, or OpenBao requests to
  the collector.
- Credential renewal lives in `internal/renewal`. Its only Kubernetes write is a TokenRequest for the enrolled
  ServiceAccount.
- Operator observation reads only enrolled, named controller Deployments. Do not list controller workloads across
  namespaces or infer responsibility from labels.
- Do not import operator internals. Interpret public resource fields through named adapters.
- Keep Kubernetes JSON and provider data at the integration boundary. Project approved fields before persistence or HTTP
  output.
- Keep collection time, producer evidence time, and state transition time separate. Missing evidence stays unknown.
- Authorize resources before returning counts, search results, topology, or history.
- Keep fixtures visibly identified as demonstration data.

## Code conventions

- Go transport types and TypeScript API types are generated from `api/openapi.yaml`. Never edit generated files.
- TypeScript runs in strict mode with unchecked index access and exact optional properties. Do not use `any`, non-null
  assertions, or casts to bypass validation. Validate data at external boundaries with runtime schemas.
- Prefer concrete types, bounded enums, and narrow interfaces owned by their consumer. Avoid generic repositories and
  speculative abstractions.
- Handle errors with context. Never log credentials, raw provider errors, raw resources, or Secret data.
- Test evidence interpretation, authorization, collection failures, and durable history. Test user workflows in a
  browser.
- Keep dependencies pinned in lockfiles and generated artifacts reproducible.

## Commits and pull requests

- Use conventional branch names, such as `feat/topology-filters` or `fix/session-expiry`.
- Write commit subjects as scoped [Conventional Commits](https://www.conventionalcommits.org/), for example
  `fix(onboarding): keep namespaces when going back`.
- Sign off your commits (`git commit -s`) to certify the
  [Developer Certificate of Origin](https://developercertificate.org/).
- Keep pull requests focused. Describe the user-visible change and how you tested it. Include screenshots for UI
  changes.
- Commit types drive releases: `feat` and `fix` appear in the changelog and select the next version. See
  [releases](docs/releasing.md).

## AI-assisted contributions

AI-assisted contributions are welcome. You are responsible for what you submit: understand the change, follow the
boundaries above, and make sure the checks pass. Agent instructions for this repository are in [AGENTS.md](AGENTS.md).

## Security issues

Do not report vulnerabilities in public issues. Follow [SECURITY.md](SECURITY.md).
