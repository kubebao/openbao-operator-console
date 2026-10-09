# Security Policy

## Reporting a Vulnerability

Please do **not** open public GitHub issues for security-sensitive reports.

Instead, use GitHub Security Advisories:

- https://github.com/kubebao/openbao-operator-console/security/advisories/new

If you are unable to use GitHub Security Advisories, open a minimal issue that requests a private contact channel and
do not include exploit details.

## Supported Versions

The console has no tagged release yet. Security fixes are made on the `main` branch. After the first release, fixes
will be provided for the latest released version.

## Scope

The console is designed as a read-only observer. Reports are especially welcome when the console:

- reads Secrets, executes commands in Pods, mutates Kubernetes resources, or contacts OpenBao;
- returns resources, counts, search results, topology, or history outside a user's assigned scope;
- exposes stored credentials, configuration, or the encryption key;
- accepts a kubeconfig that bypasses TLS verification or runs authentication plugins;
- allows a session, setup token, or login flow to be replayed or forged.

See the [security model](docs/security.md) for the intended boundaries.
