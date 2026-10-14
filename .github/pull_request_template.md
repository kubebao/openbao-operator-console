## Summary

<!-- What changed, and why? Keep this focused on reviewer context. -->

## Related issues

<!-- Closes #123 -->

## Type of change

- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change (configuration, database schema, API, Helm values, or RBAC)
- [ ] Documentation
- [ ] Refactor
- [ ] CI, release, or build tooling

## Risk and compatibility

<!-- Describe the impact on the API contract, database schema, Helm values, reader RBAC, security boundaries, or
upgrades. Write "None" if not applicable. -->

## Verification

<!-- Commands and tests you ran. Add screenshots for visible UI changes. -->

## Checklist

- [ ] `make check` passes.
- [ ] I ran `make e2e` or `make e2e-production` for user workflow changes.
- [ ] I ran `make integration` against a PostgreSQL test database for persistence, collection, or renewal changes.
- [ ] I ran `make helm-check` for chart changes.
- [ ] I ran `make generate` and `make check-generated` after changing `api/openapi.yaml` or SQL queries.
- [ ] I ran `make licenses` after changing Go or production web dependencies.
- [ ] I updated documentation and regenerated screenshots (`make screenshots`) for visible changes.
- [ ] The change keeps the console read-only and does not log or expose credentials, tokens, raw resources, or
      Secret data.
- [ ] My commits are signed off (`git commit -s`) under the [DCO](https://developercertificate.org/).
