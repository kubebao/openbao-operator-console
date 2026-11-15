# Source-first publication

Opening this repository publishes its source and development documentation. It does not publish a release, an edge
build, or a package. Keep the early-development warning and source-build instructions until a separate artifact
release is approved. Do not deploy the demo as part of this procedure.

## Prepare while private

1. Keep `CONSOLE_EDGE_ENABLED=false` and `CONSOLE_RELEASES_ENABLED=false` in repository Actions variables. Keep
   Publish Edge, Release Please, and Release manually disabled. The workflow conditions also require their channel's
   variable to equal `true` and the canonical repository to be public.
2. Review all remote branches, tags, and advertised pull-request refs. Scan their Git history with pinned Gitleaks,
   inspect findings, and review documentation, examples, screenshots, discussions, retained Actions logs, and artifact
   inventories. Store raw reports outside the repository. Do not put credentials or private incident material in an
   audit report.
3. Run `make check`, `make secret-scan`, and `make check-release-tools`. CI also checks generated artifacts, licenses,
   dependencies, browser workflows, database persistence, and the packaged installation. Require a successful current
   Check run. Private CodeQL, Dependency Review, and Scorecard skips do not establish public security qualification.
4. Keep `LICENSE`, dependency notices, contribution guidance, the security policy, and the operator qualification
   record current. Inspect `.github/main-protection.json` against the actual job names and application IDs.

`make secret-scan` scans all refs available in the local checkout. CI fetches full history. For the publication audit,
use a fresh mirror of the remote repository, which also includes advertised pull-request refs. The six entries in
`.gitleaksignore` identify reviewed historical parser/setup test fixtures by commit, file, rule, and line. They do not
exclude a file or a directory. Two identify the same fixtures in a retained local pre-consolidation commit. New
findings require review; do not generate a blanket baseline.

## Apply controls at the approved visibility change

Changing visibility is a separate action. The commands below do not change visibility. Once the repository is public,
apply and verify these controls before accepting contributions:

```sh
gh api --method PUT repos/kubebao/openbao-operator-console/branches/main/protection \
  --input .github/main-protection.json
gh api --method PATCH repos/kubebao/openbao-operator-console --input .github/public-security.json
gh api --method PUT repos/kubebao/openbao-operator-console/private-vulnerability-reporting
gh api --method PUT repos/kubebao/openbao-operator-console/vulnerability-alerts
```

The protection configuration requires current CI and security checks, DCO sign-off, resolved conversations, and linear
history. It blocks force pushes and branch deletion and also applies to administrators. This single-maintainer
repository sets no minimum approval count.

Run Check, CodeQL, and Scorecard on the same current `main` commit. Test Dependency Review with a pull request. Inspect
secret-scanning alerts and confirm that the private reporting link in `SECURITY.md` opens a private form. Keep both
publication variables false and the three publication workflows disabled throughout these checks.

Verify anonymous source access with a fresh checkout that has no GitHub credentials. Confirm that the documented
source build works. Recheck visibility, branch protection, security settings, publisher states, and the absence of
releases and tags. Record the verified source commit and workflow results. These controls do not qualify a running
operator, external identity provider, production capacity, or HA operation.

## Approve an artifact channel separately

For tagged releases, review the first-release documentation changes and release pull request before setting
`CONSOLE_RELEASES_ENABLED=true` and enabling Release Please and Release. For edge artifacts, approve the development
channel before setting `CONSOLE_EDGE_ENABLED=true` and enabling Publish Edge. These settings are independent. Follow
the [release procedure](releasing.md), including signature verification, package visibility, and anonymous pulls.
