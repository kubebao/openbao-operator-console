# Releases

This page describes how maintainers publish releases and edge builds, and how users verify the artifacts. The console
has no stable release. The first stable release uses version `0.1.0`.

Both publishing channels are disabled by default. `CONSOLE_RELEASES_ENABLED` controls Release Please and the tagged
Release workflow. `CONSOLE_EDGE_ENABLED` controls Publish Edge. Each variable must equal `true`, and the repository must
be public. Unset or other values disable the channel. Opening the source does not approve artifact publication. The
three publication workflows are also manually disabled. Enable each workflow only when its channel is approved.

## Versioning

Releases use [semantic versioning](https://semver.org/) without a `v` prefix, for example `0.1.0`. The tag, the image
tag, the chart version, and the chart `appVersion` are the same value.
[Release Please](https://github.com/googleapis/release-please) derives the next version from
[Conventional Commits](https://www.conventionalcommits.org/) on `main`:

| Commits since the last release | Next version before 1.0 |
| --- | --- |
| `fix:` or `perf:` only | Patch, such as `0.1.0` to `0.1.1` |
| At least one `feat:` | Minor, such as `0.1.1` to `0.2.0` |
| A breaking change (`feat!:` or a `BREAKING CHANGE:` footer) | Minor, such as `0.2.0` to `0.3.0` |

Features, fixes, performance improvements, reverts, documentation, and dependency updates appear in `CHANGELOG.md`.
Refactoring, tests, build, CI, and chores are omitted.

## Release flow

1. When tagged publication is enabled, every push to `main` runs the **Release Please** workflow. It opens or updates a release pull request that bumps
   `.release-please-manifest.json`, the chart `version` and `appVersion`, the image reference in the chart's Artifact
   Hub annotation, and `CHANGELOG.md`.
2. A maintainer reviews the release pull request, including the changelog, and merges it after the checks pass.
3. Release Please creates the tag and a draft GitHub release, then calls the **Release** workflow
   (`.github/workflows/release.yml`).
4. The Release workflow checks that the chart version matches the tag, then:
   - builds `linux/amd64` and `linux/arm64` images with Buildx and pushes them to
     `ghcr.io/kubebao/openbao-operator-console:<version>`;
   - records SLSA build provenance and an SPDX SBOM for each platform as signed GitHub attestations in the registry;
   - signs the image with keyless [cosign](https://docs.sigstore.dev/) through the workflow's OIDC identity;
   - packages the chart, pushes it to `oci://ghcr.io/kubebao/charts/openbao-operator-console`, records its provenance,
     and signs it;
   - builds static binary archives for Linux and macOS on `amd64` and `arm64` with `make dist` and records their
     provenance;
   - uploads the SBOMs, the chart package, the binary archives, `THIRD_PARTY_LICENSES`, and a signed `checksums.txt` to
     the GitHub release;
   - publishes the draft release.

Only the jobs that sign or attest receive `id-token: write`, and only the jobs that push receive `packages: write`.

If a publication step fails, fix the cause and run the **Release** workflow manually with the existing tag. It replaces
the release assets and republishes the same version. Do not move a published tag; release a new patch version instead.

### First release

The documentation describes the pre-release state in a few places. In the release pull request for `0.1.0`, remove:

- the **Early development** warning in `README.md` and the pre-release checkout command in its quick start;
- the pre-release note at the top of `docs/installation.md` and its **Build the image before the first release**
  section;
- the pre-release installation guidance in `charts/openbao-operator-console/README.md`;
- the "no stable release yet" sentence at the top of this page and in `SECURITY.md`.

Search for them with `git grep -n -i "first.*release\|no.*release"`.

### Repository setup

- **Release App.** Install the organization-owned `openbao-operator-console-release` GitHub App on this repository
  only. Grant it repository **Contents**, **Issues**, and **Pull requests** permissions with read and write access.
  Store its client ID in the `RELEASE_APP_CLIENT_ID` Actions variable and its PEM private key in the
  `RELEASE_APP_PRIVATE_KEY` Actions secret. The workflow creates a temporary installation token scoped to this
  repository and revokes it when the job ends. Release PRs created with this token trigger CI without manual workflow
  approval. The workflow does not use a personal access token or fall back to `GITHUB_TOKEN` for Release Please.
- **Publication credentials.** GHCR publishing uses the job's `GITHUB_TOKEN`. Signing and attestations use GitHub
  Actions OIDC. No separate registry password or signing key is required.
- **Package visibility.** After each package's first publication, set it to public and link it to this repository.
  The packages are `openbao-operator-console`, `charts/openbao-operator-console`, and
  `charts-edge/openbao-operator-console`. Check that anonymous image and chart pulls work.
- **Artifact Hub.** Register the OCI repository `oci://ghcr.io/kubebao/charts/openbao-operator-console`, add the
  assigned `repositoryID` to `artifacthub-repo.yml`, and push the metadata once with [ORAS](https://oras.land/):

  ```sh
  oras push ghcr.io/kubebao/charts/openbao-operator-console:artifacthub.io \
    --config /dev/null:application/vnd.cncf.artifacthub.config.v1+yaml \
    artifacthub-repo.yml:application/vnd.cncf.artifacthub.repository-metadata.layer.v1.yaml
  ```

### Public repository controls

The current private-repository plan does not provide branch protection or secret scanning. Private vulnerability
reporting also requires a public repository. After changing visibility, complete these steps before merging a release
pull request or publishing artifacts. These commands do not change repository visibility.

1. Require pull requests and the current CI and security checks on `main`:

   ```sh
   gh api --method PUT repos/kubebao/openbao-operator-console/branches/main/protection \
     --input .github/main-protection.json
   ```

   The configuration requires up-to-date branches, checks from GitHub Actions and the DCO app, resolved review threads,
   and linear history. It protects administrators and blocks force pushes and deletion. It sets no minimum approval count for
   this single-maintainer repository. Keep the check names aligned with the workflow job names.
   **Edge candidate provenance** is excluded because it runs only on `main` after PR qualification.

2. Enable secret scanning and push protection, then enable private vulnerability reporting:

   ```sh
   gh api --method PATCH repos/kubebao/openbao-operator-console --input - <<'JSON'
   {
     "security_and_analysis": {
       "secret_scanning": { "status": "enabled" },
       "secret_scanning_push_protection": { "status": "enabled" }
     }
   }
   JSON
   gh api --method PUT repos/kubebao/openbao-operator-console/private-vulnerability-reporting
   ```

3. Verify that the settings took effect and the security reporting link in `SECURITY.md` opens the private report form:

   ```sh
   gh api repos/kubebao/openbao-operator-console/branches/main/protection
   gh api repos/kubebao/openbao-operator-console --jq .security_and_analysis
   gh api repos/kubebao/openbao-operator-console/private-vulnerability-reporting
   gh api repos/kubebao/openbao-operator-console/secret-scanning/alerts \
     --jq 'map({number, state, secret_type_display_name})'
   ```

4. Run **Check**, **CodeQL**, and **Scorecard** on the same current `main` commit. All three workflows support manual
   runs. Confirm successful runs; private-repository skips do not qualify the public repository. Test
   **Dependency Review** on a new pull request. Keep both publication variables false while qualifying the public
   repository. **Publish Edge** also requires its separate opt-in and an enabled workflow.

The publication workflows use `anchore/sbom-action` and `actions/attest`, pinned to approved commits. The SBOM action
generates local files without uploading release assets, workflow artifacts, or dependency snapshots. Existing
publication steps sign, attest, and upload those files. Do not broaden the organization Actions allowlist to repair a
workflow that can use these approved actions.

## Edge builds

Edge is an opt-in development channel for testing qualified `main` commits before a stable release. It includes the
multi-architecture image, a Helm chart, and binary archives for Linux and macOS on `amd64` and `arm64`. Edge builds
can change configuration and schema. Use a disposable database, or follow the offline schema upgrade procedure and
keep the matching encryption key in a separate backup. A rollback across a schema change requires the pre-upgrade database backup; see
[upgrading](upgrading.md).

The **Check** workflow builds the OCI image once, builds all four binary archives, and packages the chart with that
image's digest. It tests the native archive's version and installs the packaged chart with the default SQLite PVC and
optional PostgreSQL 18 in a disposable kind cluster. It verifies retained SQLite schema after Pod replacement. It checks the chart's image reference, readiness, embedded UI, and content security policy.
This test does not qualify a real OpenBao Operator, external identity provider, or remote credential renewal.

After all Check jobs pass, CI attests the candidate bundle, chart, binary archives, and source metadata. **Publish
Edge** requires `CONSOLE_EDGE_ENABLED=true` and waits for successful **Check** and **CodeQL** runs on the same current `main` commit. It accepts only pushes
and manual runs in the public canonical repository. The publisher verifies the CI attestation before unpacking,
validates the source identity and every artifact checksum, and imports the tested OCI image without rebuilding it.
It signs and verifies the image and chart, publishes SBOM attestations, verifies the binary and chart CI provenance,
and signs the download checksums before publishing the prerelease.

An edge build has these references:

- Image: `ghcr.io/kubebao/openbao-operator-console:0.0.0-edge.<run>.<attempt>.g<sha>`.
- Moving image tag: `ghcr.io/kubebao/openbao-operator-console:edge`. It advances after publication only if the commit
  is still the head of `main`.
- Chart: `oci://ghcr.io/kubebao/charts-edge/openbao-operator-console`, with the same `0.0.0-edge...` version. Its
  default image is pinned by digest.
- Downloads: an **Edge** GitHub prerelease tagged `edge-<run>.<attempt>.g<sha>`, including all four binary archives,
  the chart, SBOMs, license notices, `metadata.json`, `checksums.txt`, and `checksums.txt.sigstore.json`.

Here `<sha>` is the first 12 characters of the full source commit. `metadata.json` records the full commit, producer
run and attempt, image digests, and candidate file hashes. Each published version remains unchanged. Retries can
resume a draft prerelease, but do not replace published download assets or conflicting image/chart content. Edge
Git tags use a non-SemVer prefix so Release Please does not treat them as stable releases. Edge publication does not
change the release manifest, changelog, or chart sources. The edge chart repository is not registered in Artifact Hub.

### Install an edge build

Select a published **Edge** prerelease from [GitHub releases](https://github.com/kubebao/openbao-operator-console/releases).
Set `VERSION` to its chart version and `TAG` to its GitHub prerelease tag. Verify the image and chart signatures:

```sh
IDENTITY=https://github.com/kubebao/openbao-operator-console/.github/workflows/publish-edge.yml@refs/heads/main
ISSUER=https://token.actions.githubusercontent.com
cosign verify ghcr.io/kubebao/openbao-operator-console:$VERSION \
  --certificate-identity "$IDENTITY" --certificate-oidc-issuer "$ISSUER"
cosign verify ghcr.io/kubebao/charts-edge/openbao-operator-console:$VERSION \
  --certificate-identity "$IDENTITY" --certificate-oidc-issuer "$ISSUER"
```

Follow the database and Secret setup in [installation](installation.md), then install the verified chart:

```sh
helm upgrade --install openbao-operator-console oci://ghcr.io/kubebao/charts-edge/openbao-operator-console \
  --version "$VERSION" --namespace console-system --values console-values.yaml --wait
```

For binaries, download the archive for your platform, the metadata, and the signed checksums. This example uses
Linux `amd64`:

```sh
gh release download "$TAG" --repo kubebao/openbao-operator-console \
  --pattern "openbao-operator-console_${VERSION}_linux_amd64.tar.gz" \
  --pattern metadata.json --pattern 'checksums.txt*'
cosign verify-blob checksums.txt --bundle checksums.txt.sigstore.json \
  --certificate-identity "$IDENTITY" --certificate-oidc-issuer "$ISSUER"
sha256sum --check --ignore-missing checksums.txt
gh attestation verify "openbao-operator-console_${VERSION}_linux_amd64.tar.gz" \
  --repo kubebao/openbao-operator-console \
  --signer-workflow kubebao/openbao-operator-console/.github/workflows/check.yml \
  --source-ref refs/heads/main --source-digest "$(jq -r .commit metadata.json)"
```

Image and chart publication provenance and per-platform SBOM attestations use the **Publish Edge** signer. Verify
them with the stable verification commands below, using the edge image/chart references and `publish-edge.yml`
signer identity. Binary and chart archive build provenance uses the **Check** signer shown above.

### Enable or retry edge publication

Merge the workflows, make the repository public, and enable Actions. Enable **Check** and **CodeQL** if GitHub leaves
them disabled. Both support manual runs on `main`, so the first edge build does not need an empty commit. Trigger both
workflows for the same commit; the last one to succeed starts publication. Pull requests build and install the
candidate without publishing or receiving registry credentials.

If qualification fails, fix the failure and rerun Check. A failed-job rerun can reuse a successful candidate from an
earlier attempt of that run. If publication fails, rerun the failed Publish Edge run while the selected Check artifact
is retained (seven days). After it expires, run Check and CodeQL on current `main` again. Published edge versions and
their registry digests have no automatic deletion policy.

## Third-party licenses

`THIRD_PARTY_LICENSES` lists the Go modules linked into the binary, the Go standard library, and the production web
dependencies, including the DM Sans and DM Mono fonts, with their license texts. The image contains it at
`/licenses/THIRD_PARTY_LICENSES`. Regenerate it after changing dependencies:

```sh
make licenses
```

CI fails when the committed file is out of date.

## Verify a release

The examples use `VERSION=0.1.0`. Install [cosign](https://docs.sigstore.dev/cosign/system_config/installation/) 2.4 or
later and the [GitHub CLI](https://cli.github.com/).

```sh
VERSION=0.1.0
IDENTITY='^https://github\.com/kubebao/openbao-operator-console/\.github/workflows/release\.yml@refs/'
ISSUER=https://token.actions.githubusercontent.com
```

Verify the image and chart signatures:

```sh
cosign verify ghcr.io/kubebao/openbao-operator-console:$VERSION \
  --certificate-identity-regexp "$IDENTITY" --certificate-oidc-issuer "$ISSUER"

cosign verify ghcr.io/kubebao/charts/openbao-operator-console:$VERSION \
  --certificate-identity-regexp "$IDENTITY" --certificate-oidc-issuer "$ISSUER"
```

Verify the build provenance of the image and the chart:

```sh
gh attestation verify oci://ghcr.io/kubebao/openbao-operator-console:$VERSION \
  --repo kubebao/openbao-operator-console
gh attestation verify oci://ghcr.io/kubebao/charts/openbao-operator-console:$VERSION \
  --repo kubebao/openbao-operator-console
```

SBOM attestations belong to each platform image. Look up the platform digest, then verify it:

```sh
docker buildx imagetools inspect ghcr.io/kubebao/openbao-operator-console:$VERSION
gh attestation verify oci://ghcr.io/kubebao/openbao-operator-console@sha256:<linux/amd64 digest> \
  --repo kubebao/openbao-operator-console --predicate-type https://spdx.dev/Document/v2.3
```

Verify downloaded release assets:

```sh
cosign verify-blob checksums.txt --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp "$IDENTITY" --certificate-oidc-issuer "$ISSUER"
sha256sum --check --ignore-missing checksums.txt
gh attestation verify openbao-operator-console_${VERSION}_linux_amd64.tar.gz --repo kubebao/openbao-operator-console
```

Install by digest to make sure the cluster runs the image you verified. Set `image.digest` in the chart values.
