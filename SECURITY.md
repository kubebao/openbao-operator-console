# Security policy

## Report a vulnerability

Do **not** open public GitHub issues for security-sensitive reports.

Instead, use GitHub Security Advisories:

- https://github.com/kubebao/openbao-operator-console/security/advisories/new

If you are unable to use GitHub Security Advisories, open a minimal issue that requests a private contact channel and do
not include exploit details.

## Supported versions

Security fixes are provided for:

- The latest stable release.

No stable release has been published yet. Until then, fixes are made on the `main` branch. Edge builds receive fixes
through new builds of `main`; older edge builds are not maintained.

## Supply chain

No release artifacts are published yet. Tagged and edge publication require separate opt-in
variables and public-repository checks. The prepared release workflows produce:

- the multi-architecture container image `ghcr.io/kubebao/openbao-operator-console`;
- the stable OCI Helm chart `oci://ghcr.io/kubebao/charts/openbao-operator-console` and development charts in
  `oci://ghcr.io/kubebao/charts-edge/openbao-operator-console`;
- SPDX SBOMs, `THIRD_PARTY_LICENSES`, and `checksums.txt` attached to the GitHub release.

The publication workflows sign the image and chart through GitHub Actions OIDC and attach signed SLSA build provenance
and image SBOM attestations. They sign `checksums.txt` with a Sigstore bundle. The unsigned distribution rehearsal does
not perform these publication steps. See [releasing](docs/releasing.md#verify-a-release) for the verification commands.

Dependencies are pinned in `go.sum` and `web/pnpm-lock.yaml`, base images and GitHub Actions are pinned by digest, and
Dependabot proposes updates. CI runs `govulncheck`, `pnpm audit`, and a redacted Git-history secret scan. CodeQL,
dependency review, and OpenSSF Scorecard run for public repositories; private-repository skips do not qualify those
security checks.

## Scope

The console is designed as a read-only observer. Reports are especially welcome when the console:

- reads Secrets, executes commands in Pods, mutates Kubernetes resources, or contacts OpenBao;
- returns resources, counts, search results, topology, or history outside a user's assigned scope;
- exposes stored credentials, configuration, or the encryption key;
- accepts a kubeconfig that bypasses TLS verification or runs authentication plugins;
- allows a session, setup token, or login flow to be replayed or forged.

See the [security model](docs/security.md) for the intended boundaries.
