# Security Policy

## Reporting a Vulnerability

Please do **not** open public GitHub issues for security-sensitive reports.

Instead, use GitHub Security Advisories:

- https://github.com/kubebao/openbao-operator-console/security/advisories/new

If you are unable to use GitHub Security Advisories, open a minimal issue that requests a private contact channel and do
not include exploit details.

## Supported Versions

Security fixes are provided for:

- The latest released version.

No release has been published yet. Until then, fixes are made on the `main` branch.

## Supply Chain

From the first release, release artifacts are published to GHCR by the release workflow in this repository:

- the multi-architecture container image `ghcr.io/kubebao/openbao-operator-console`;
- the OCI Helm chart `oci://ghcr.io/kubebao/charts/openbao-operator-console`;
- SPDX SBOMs, `THIRD_PARTY_LICENSES`, and `checksums.txt` attached to the GitHub release.

The image and chart are signed with keyless Sigstore signing through GitHub Actions OIDC, and carry signed SLSA build
provenance attestations. Image SBOMs are also published as signed attestations. `checksums.txt` is signed with a
Sigstore bundle. See [releasing](docs/releasing.md#verify-a-release) for the verification commands.

Dependencies are pinned in `go.sum` and `web/pnpm-lock.yaml`, base images and GitHub Actions are pinned by digest, and
Dependabot proposes updates. CI runs `govulncheck`, `pnpm audit`, CodeQL, dependency review, and OpenSSF Scorecard.

## Scope

The console is designed as a read-only observer. Reports are especially welcome when the console:

- reads Secrets, executes commands in Pods, mutates Kubernetes resources, or contacts OpenBao;
- returns resources, counts, search results, topology, or history outside a user's assigned scope;
- exposes stored credentials, configuration, or the encryption key;
- accepts a kubeconfig that bypasses TLS verification or runs authentication plugins;
- allows a session, setup token, or login flow to be replayed or forged.

See the [security model](docs/security.md) for the intended boundaries.
