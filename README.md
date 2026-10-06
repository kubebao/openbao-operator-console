<div align="center">

# OpenBao Operator Console

**A read-only fleet console for the [OpenBao Operator](https://github.com/dc-tec/openbao-operator).**

[![Check](https://github.com/dc-tec/openbao-operator-console/actions/workflows/check.yml/badge.svg)](https://github.com/dc-tec/openbao-operator-console/actions/workflows/check.yml)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

[Quick start](#quick-start) • [Install](docs/installation.md) • [Documentation](#documentation) • [Contributing](CONTRIBUTING.md)

</div>

> [!WARNING]
> **Early development.** The console has no tagged release, published image, or published chart yet. Build from
> source. Some evidence (restore tests, native recovery, remote replicas) depends on operator features that are still in
> development. Interfaces and the database schema may change before the first release.

![Attention workspace showing findings across the fleet](docs/images/attention.png)

The console is a companion to the OpenBao Operator, maintained by the same project. It gives platform teams one place
to see which OpenBao deployments need attention across many Kubernetes clusters, and why. It reads the operator's
public Kubernetes resources, keeps a history of what it observed, and explains the evidence behind each finding.

It never changes your clusters. The console does not create or modify resources, read Secrets, exec into Pods, or send
requests to OpenBao. Lifecycle operations stay with the operator and your runbooks.

## Features

- **Attention first.** Findings for unavailable replicas, sealed deployments, failed or overdue backups, failed restore
  tests, failed upgrades, recovery problems, and collection gaps, ordered by severity.
- **Evidence you can trace.** Each finding links to the tab and the exact evidence rows that produced it. Collection
  time, producer time, and state transitions stay separate, and missing evidence is shown as unknown, never as healthy.
- **Deployment health at a glance.** Availability, backups, restore tests, and lifecycle on one overview; backup age
  against policy; interpreted operator conditions; and upgrade and recovery progress.
- **Change history.** Seven days of recorded state and a plain-language timeline of what changed, such as findings
  raised and resolved, replica counts, versions, conditions, and backups.
- **Fleet inventory.** Search, filters you can share as URLs, and severity sorting across clusters, teams, and
  environments.
- **Topology and recovery paths.** Declared replication and snapshot recovery relationships, bound to resource UIDs,
  with the evidence available for each path.
- **Multi-cluster collection.** Namespace-scoped readers per cluster, named operator Deployment observation, and opt-in
  automatic ServiceAccount token renewal for remote clusters.
- **Guided onboarding.** Generated least-privilege reader RBAC, connection checks, and deployment previews before a
  cluster is saved.
- **Scoped access.** OpenID Connect login with PKCE, per-user and per-group namespace scopes, and administrator-only
  settings with a change history.
- **Built for operators.** Light and dark themes, keyboard search (<kbd>⌘</kbd>/<kbd>Ctrl</kbd> + <kbd>K</kbd>),
  responsive layout, and reduced-motion support.

## Screenshots

| Fleet inventory | Deployment overview |
| --- | --- |
| ![Fleet table sorted by severity](docs/images/fleet.png) | ![Deployment overview with evidence tiles and findings](docs/images/deployment.png) |
| **Backup health** | **Change history** |
| ![Backup age against policy, activity over seven days, and restore-test evidence](docs/images/backup-evidence.png) | ![State over seven days and a timeline of recorded changes](docs/images/history.png) |
| **Recovery path** | **Cluster coverage** |
| ![Topology view of a snapshot recovery path](docs/images/topology.png) | ![Cluster collection and operator status](docs/images/clusters.png) |

A [dark theme](docs/images/attention-dark.png) is included. The screenshots show the built-in demonstration data;
regenerate them with `make screenshots`.

## Quick start

Try the console locally with demonstration data. No Kubernetes cluster, database, or identity provider is required.

Requirements: Go 1.27, Node.js 24 or later, and pnpm 11.

```sh
git clone https://github.com/dc-tec/openbao-operator-console.git
cd openbao-operator-console
make install
make build
./bin/console --mode=demo
```

Open <http://127.0.0.1:8090>. Demo mode listens on loopback only, needs no login, and labels every page as
demonstration data.

To run the console against real clusters, follow the [installation guide](docs/installation.md).

## How it works

```mermaid
flowchart LR
  browser[Browser] --> api[Console API<br/>authorization and sessions]
  api --> database[(PostgreSQL)]
  collector[Collector] --> database
  collector -- namespace-scoped reads --> kubernetes[Kubernetes clusters<br/>with the OpenBao Operator]
  renewal[Token renewal] -. optional TokenRequest .-> kubernetes
  api -. OIDC .-> idp[Identity provider]
```

One Go binary serves the web application and API, runs the collectors, and stores projected observations, findings,
and encrypted configuration in PostgreSQL. See [architecture](docs/architecture.md) and the
[security model](docs/security.md).

## Requirements

| Component | Requirement |
| --- | --- |
| OpenBao Operator | Clusters managed by the [OpenBao Operator](https://github.com/dc-tec/openbao-operator) (`v1alpha1` APIs) |
| Kubernetes | A namespace-scoped reader per cluster; the hosting cluster can use the console's ServiceAccount |
| PostgreSQL | An external database; CI tests PostgreSQL 18 |
| Identity provider | An OpenID Connect provider with authorization code flow and PKCE, such as Keycloak |
| Network | HTTPS in front of the console, through an ingress, a gateway, or direct TLS |

A compatibility matrix with operator releases will be published with the first console release.

## Documentation

| Guide | Contents |
| --- | --- |
| [Installation](docs/installation.md) | Helm installation, runtime Secret, PostgreSQL, encryption key, and HTTPS |
| [First administrator setup](docs/bootstrap.md) | Protected browser setup or automated identity bootstrap |
| [Connect a Kubernetes cluster](docs/cluster-onboarding.md) | Guided onboarding, reader RBAC, verification, and credential replacement |
| [Credential renewal](docs/credential-renewal.md) | Automatic ServiceAccount token renewal for remote clusters |
| [Administration](docs/administration.md) | Access rules, relationships, identity provider changes, recovery, and upgrades |
| [Keycloak](docs/keycloak.md) | Configure Keycloak as the OpenID Connect provider |
| [Enrollment files and CLI](docs/connect-clusters.md) | First-start enrollment YAML, `--print-rbac`, and `--check` diagnostics |
| [Attention rules](docs/attention-rules.md) | What each finding means and what it does not prove |
| [Security model](docs/security.md) | Permissions, stored data, sessions, and encryption |
| [Architecture](docs/architecture.md) | Components, evidence handling, and design boundaries |
| [Development](docs/development.md) | Local development, tests, and integration environments |

## Contributing

Contributions are welcome. Read [CONTRIBUTING.md](CONTRIBUTING.md) for the development workflow and conventions.
Report security issues privately as described in [SECURITY.md](SECURITY.md).

## License

Licensed under the [Apache License 2.0](LICENSE).
