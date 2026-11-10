#!/usr/bin/env bash
# Publishes qualified edge artifacts. The workflow verifies their CI attestation first.
set -euo pipefail
cd "$(dirname "$0")/.."
command="$1"
candidate="$(cd "$2" && pwd)"
metadata="${candidate}/metadata.json"
image="$(jq -er .image.repository "${metadata}")"
digest="$(jq -er .image.digest "${metadata}")"
version="$(jq -er .version "${metadata}")"
tag="$(jq -er .releaseTag "${metadata}")"
commit="$(jq -er .commit "${metadata}")"
chart="ghcr.io/${GITHUB_REPOSITORY_OWNER,,}/charts-edge/openbao-operator-console"
work="$(mktemp -d)"
trap 'rm -rf "${work}"' EXIT

# A failed lookup must not turn an authentication or network failure into an overwrite.
existing_digest() {
  if regctl image digest "$1" > "${work}/digest" 2> "${work}/error"; then
    cat "${work}/digest"
  elif grep -Eq 'not found|404' "${work}/error"; then
    return 0
  else
    echo "registry lookup failed; refusing publication" >&2
    return 1
  fi
}

case "${command}" in
  image)
    existing="$(existing_digest "${image}:${version}")"
    if [[ -n "${existing}" && "${existing}" != "${digest}" ]]; then
      echo "edge image version already points to different content" >&2
      exit 1
    fi
    if [[ -z "${existing}" ]]; then
      regctl image import "${image}:${version}" "${candidate}/image.tar"
    fi
    test "$(regctl image digest "${image}:${version}")" = "${digest}"
    ;;
  chart)
    chart_digest="$(existing_digest "${chart}:${version}")"
    if [[ -z "${chart_digest}" ]]; then
      helm push "${candidate}/openbao-operator-console-${version}.tgz" "oci://ghcr.io/${GITHUB_REPOSITORY_OWNER,,}/charts-edge"
      chart_digest="$(regctl image digest "${chart}:${version}")"
    fi
    helm pull "oci://${chart}@${chart_digest}" --destination "${work}"
    shopt -s nullglob
    archives=("${work}"/*.tgz)
    test "${#archives[@]}" -eq 1
    cmp "${candidate}/openbao-operator-console-${version}.tgz" "${archives[0]}"
    printf 'digest=%s\n' "${chart_digest}" >> "${GITHUB_OUTPUT}"
    ;;
  release)
    # These non-SemVer Git tags are separate from the stable release tag namespace.
    if gh api "repos/${GITHUB_REPOSITORY}/git/ref/tags/${tag}" > "${work}/ref.json" 2> "${work}/error"; then
      test "$(jq -er .object.type "${work}/ref.json")" = commit
      test "$(jq -er .object.sha "${work}/ref.json")" = "${commit}"
    elif grep -q '404' "${work}/error"; then
      gh api --method POST "repos/${GITHUB_REPOSITORY}/git/refs" -f "ref=refs/tags/${tag}" -f "sha=${commit}" > /dev/null
    else
      echo "edge tag lookup failed" >&2; exit 1
    fi
    if gh release view "${tag}" --repo "${GITHUB_REPOSITORY}" --json isDraft,isPrerelease > "${work}/release.json" 2> "${work}/error"; then
      test "$(jq -er .isPrerelease "${work}/release.json")" = true
      if [[ "$(jq -r .isDraft "${work}/release.json")" == false ]]; then
        echo "edge release is already published; assets are unchanged"
        exit 0
      fi
    elif grep -Eq 'release not found|404' "${work}/error"; then
      # shellcheck disable=SC2016 # Backticks are Markdown code spans.
      printf 'Development preview of `%s`.\n\nImage: `%s@%s`\n\nChart: `oci://%s --version %s`\n\nIncludes Linux and macOS binaries for amd64 and arm64. This version can migrate the database; use a disposable database or back it up with its encryption key before upgrading.\n\nSee the [edge verification instructions](https://github.com/%s/blob/%s/docs/releasing.md#edge-builds).\n' \
        "${commit}" "${image}" "${digest}" "${chart}" "${version}" "${GITHUB_REPOSITORY}" "${commit}" > "${work}/notes.md"
      gh release create "${tag}" --repo "${GITHUB_REPOSITORY}" --verify-tag --prerelease --draft --latest=false \
        --title "Edge ${version}" --notes-file "${work}/notes.md"
    else
      echo "edge release lookup failed" >&2; exit 1
    fi
    assets=("${candidate}"/*.tar.gz "${candidate}"/*.tgz "${candidate}"/*.spdx.json
      "${candidate}/LICENSE" "${candidate}/THIRD_PARTY_LICENSES" "${metadata}"
      "${candidate}/checksums.txt" "${candidate}/checksums.txt.sigstore.json")
    gh release upload "${tag}" --repo "${GITHUB_REPOSITORY}" --clobber "${assets[@]}"
    gh release edit "${tag}" --repo "${GITHUB_REPOSITORY}" --draft=false --prerelease --latest=false
    ;;
  alias)
    # A slow run must not replace edge after a newer commit reached main.
    current="$(gh api "repos/${GITHUB_REPOSITORY}/branches/main" --jq .commit.sha)"
    if [[ "${current}" == "${commit}" ]]; then
      regctl image copy "${image}@${digest}" "${image}:edge"
      test "$(regctl image digest "${image}:edge")" = "${digest}"
    else
      echo "main has advanced; leaving the moving edge tag unchanged"
    fi
    ;;
  *) echo "expected image, chart, release, or alias" >&2; exit 1 ;;
esac
