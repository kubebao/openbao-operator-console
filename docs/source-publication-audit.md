# Source publication audit

Audit date: 2026-11-14. Source baseline: `82354bd519de52792790db17eb0cd7b54d4d5fbf`.
This record covers preparation for public source access. It does not authorize a visibility change or artifact release.

## History and public-facing material

A fresh remote mirror contained 151 reachable commits, eight branch refs, and 42 advertised pull-request refs. Pinned
Gitleaks `v8.30.1` identified four deterministic parser/setup test fixtures. Their exact historical fingerprints are
reviewed in `.gitleaksignore`. A separate local-history scan reviewed two equivalent fixtures from a retained local
pre-consolidation commit. No file or directory is excluded. A temporary negative probe confirmed that a new
credential-shaped value in the same test path still fails the scan.

Review covered documentation, examples, fixture screenshot sources, selected rendered screenshots, and historical
file paths. Targeted private-host, local-path, and customer-reference searches found only the declared public maintainer
contact in chart metadata. No live project credential or private operational endpoint was identified in this review.

The GitHub review included 63 issue, pull-request, review, and comment records and all 220 retrievable log archives
among 242 recorded workflow runs. Unavailable log archives could not be inspected. Across this repository and the CLI,
the text scan's 390 log findings were reviewed: 358 were public Git commit identifiers matched by the scanner's legacy
code-search token pattern, and 32 were expired signed download URLs for public container-registry content. No finding was
left unclassified. No remote history, discussion, or workflow run was deleted or rewritten.

Artifact review covered inventories and workflow upload paths: 75 retained console artifacts were candidate bundles
or Docker build records. Candidate construction validates its file inventory and checksums. Docker build inputs
exclude local state and credentials. This review does not substitute for artifact signature or runtime qualification.
Raw scan reports and downloaded private logs remain outside the source repository.

## Preparation controls

- `CONSOLE_EDGE_ENABLED` and `CONSOLE_RELEASES_ENABLED` are false. Publish Edge, Release Please, and Release are
  manually disabled. Workflow conditions also require their channel opt-in and a public canonical repository.
- Check includes a redacted full-history secret scan. Edge candidate provenance depends on that job as well as the
  existing qualification jobs.
- `.github/main-protection.json` and `.github/public-security.json` define the controls applied at the approved
  visibility change. The private repository plan does not provide those public security controls.
- README instructions build from source and retain the early-development warning. No release, tag, package, or demo
  deployment was created by preparation.

## Validation and remaining boundary

Local `make check` passes, including Go race tests, TypeScript checks, lint, 131 web tests, and the frontend build.
`make secret-scan` and `make check-release-tools` pass. Release-tool tests cover a disabled edge gate making no network
requests and an enabled gate still requiring qualified runs on current `main`.

The preparation changes still require current hosted Check results. At visibility change, verify branch protection,
secret scanning, push protection, private vulnerability reporting, actual CodeQL/Scorecard results, Dependency Review
on a pull request, and anonymous source access. Follow [source publication](source-publication.md). Keep artifact
publication held throughout. Refresh this audit when new source, history, or public-facing material changes.
