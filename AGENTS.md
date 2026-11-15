# Engineering rules

Write clear technical English for developers and operators. Follow the Google developer documentation style guide and
the detailed rules in [.agents/rules/documentation.md](.agents/rules/documentation.md).

## Boundaries

- The console is a read-only observer. Never add Kubernetes mutations, Secret reads, Pod exec, or OpenBao requests to its reader.
- Remote credential renewal belongs in `internal/renewal`. Its only Kubernetes write is TokenRequest for the enrolled ServiceAccount. Keep that permission separate from workload observation.
- Operator observation reads only enrolled, named controller Deployments. Never list controller workloads across namespaces or infer responsibility from labels.
- Keep operator internals out of this repository. Interpret public resource fields through named adapters.
- Keep Kubernetes JSON and provider data at the integration boundary. Project approved fields before persistence or HTTP output.
- Separate collection time, producer evidence time, and state transition time. Missing evidence remains unknown.
- Authorize resources before returning counts, search results, topology, or history.
- Keep fixtures visibly identified. Do not claim production qualification from fixture tests.

## Code

- Use concrete types, bounded enums, and narrow interfaces owned by their consumer.
- Go transport types and TypeScript API types are generated from `api/openapi.yaml`. Never edit generated files.
- TypeScript must pass strict checking, including unchecked index access and exact optional properties. Do not use `any`, non-null assertions, or casts to bypass validation.
- Use runtime validation at external boundaries. Keep core logic typed.
- Prefer focused packages and components. Avoid generic repositories, service locators, and speculative frameworks.
- Handle errors with context. Do not log credentials, raw provider errors, raw resources, or Secret data.
- Test evidence interpretation, authorization, collection failures, and durable history. Test user workflows in a browser.
- Run `make check` before handing work back. Run integration checks when persistence or collection changes.

## Local work

- Use conventional branch names such as `feat/initial-console` and scoped Conventional Commits.
- Preserve other repositories and existing local environments. Create isolated test clusters.
- Keep generated artifacts reproducible and dependencies pinned in lockfiles.
- Update user-facing docs and regenerate screenshots (`make screenshots`) when visible behavior changes.
