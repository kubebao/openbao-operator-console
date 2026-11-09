# Releases

This page describes how maintainers publish a release and how users verify the published artifacts. The console has no
release yet; the first release will be `0.1.0`.

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

1. Every push to `main` runs the **Release Please** workflow. It opens or updates a release pull request that bumps
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

- the **Early development** warning in `README.md`, and the "Before the first release" note in its quick start;
- the pre-release note at the top of `docs/installation.md` and its **Build the image before the first release**
  section;
- the "Until the first release" sentence in `charts/openbao-operator-console/README.md`;
- the "no release yet" sentence at the top of this page.

Search for them with `git grep -n -i "first release\|no release"`.

### Repository setup

- **Actions permissions.** Allow GitHub Actions to create pull requests, or store a fine-grained token or GitHub App
  token with contents, issues, and pull request write access as the `RELEASE_PLEASE_TOKEN` secret. Pull requests that
  the default token opens do not trigger the **Check** workflow; close and reopen the release pull request to run them,
  or use the secret.
- **Package visibility.** After the first release, set the `openbao-operator-console` image package and the
  `charts/openbao-operator-console` chart package in GHCR to public and link them to this repository.
- **Artifact Hub.** Register the OCI repository `oci://ghcr.io/kubebao/charts/openbao-operator-console`, add the
  assigned `repositoryID` to `artifacthub-repo.yml`, and push the metadata once with [ORAS](https://oras.land/):

  ```sh
  oras push ghcr.io/kubebao/charts/openbao-operator-console:artifacthub.io \
    --config /dev/null:application/vnd.cncf.artifacthub.config.v1+yaml \
    artifacthub-repo.yml:application/vnd.cncf.artifacthub.repository-metadata.layer.v1.yaml
  ```

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
