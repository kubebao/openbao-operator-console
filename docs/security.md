# Security model

The console is a read-only observer. This page describes what it can access, what it stores, and how it protects that
data. Report vulnerabilities as described in [SECURITY.md](../SECURITY.md).

## Kubernetes access

The collector needs only namespace-scoped read access:

| Permission | Scope | Purpose |
| --- | --- | --- |
| `list` on `openbaoclusters`, `openbaorestores`, `openbaorecoveries` | Each enrolled namespace | OpenBao resource evidence |
| `get` on named Deployments | The declared operator Deployment only | Operator health and version |
| `create` on `serviceaccounts/token` | The reader's own ServiceAccount only, when renewal is enabled | Automatic token renewal |

The console never requests permissions to read Secrets, exec into Pods, list Deployments, or mutate any resource. It
does not send requests to OpenBao. The generated reader manifests contain no cluster-wide permissions.

Kubernetes administrators apply and remove these grants. The console generates the manifests but never applies them,
and it never removes RBAC when a cluster is removed.

Token renewal runs in a separate worker. Its only Kubernetes write is a TokenRequest for the enrolled reader
ServiceAccount. A renewed token must pass a scoped collection before the console uses it.

## Uploaded credentials

Kubeconfig import accepts embedded tokens and client certificates. It rejects authentication plugins (exec), file
references, impersonation, proxies, disabled TLS verification, and server-name overrides.

Uploading a credential with broader privileges does not reduce them. Upload only a scoped reader credential.

## Stored data

| Data | Storage |
| --- | --- |
| Configuration (connections, access rules, relationships, policies, identity provider) | PostgreSQL, encrypted with AES-256-GCM |
| Kubernetes reader credentials | PostgreSQL, encrypted per connection with AES-256-GCM and bound to the connection ID |
| Observations, findings, and activity | PostgreSQL, approved fields only; history kept for the configured retention (default seven days) |
| Configuration changes | PostgreSQL: actor, time, revision, and changed area, without credential values |
| Encryption key | Outside PostgreSQL, mounted from a Kubernetes Secret or file |

The collector projects approved fields from Kubernetes objects before storing them. Raw Kubernetes objects, Secret
data, and provider error messages are not stored or returned by the API. Credentials are write-only in the UI.

## Authentication and sessions

- Users sign in through OpenID Connect with the authorization code flow, PKCE (`S256`), and nonce verification.
- The console validates the ID token signature, issuer, and audience.
- The browser receives an opaque `Secure`, `HttpOnly` session cookie. Provider tokens stay on the server and are not
  persisted.
- Sessions expire after eight hours or at ID token expiry, whichever comes first. Sessions live in process memory.
- Logout ends the console session. It does not end the identity provider's single sign-on session.
- A failed login returns to the sign-in page with a fixed reason: expired, no access, failed, or unavailable. Provider
  error details are not shown.
- Requests that change state require the exact public origin.

## Authorization

- Users match by OIDC `sub`. Groups match by exact values in a signed ID token claim.
- Non-administrators see only the clusters and namespaces assigned to them. The API authorizes resources before it
  returns counts, search results, topology, or history.
- Administrators can see all enrolled namespaces and change settings.
- Access rule changes apply to existing sessions on their next request.
- Kubernetes labels and unmapped claims never grant access.

## First-run setup

Protected browser setup requires a deployment-provided token, expires one hour after startup, rate-limits attempts, and
closes permanently once a verified login creates the first administrator. Anyone who holds the setup token can become
the first administrator, so restrict access to it and remove it after setup. See
[first administrator setup](bootstrap.md).

## Demo mode

Demo mode serves fixture data without authentication. It refuses to listen on a non-loopback address and labels every
page as demonstration data. Do not expose it.

## Operational guidance

- Serve the console over HTTPS only.
- Back up the encryption key separately from database backups, and restrict access to both.
- Use TLS with certificate verification for remote PostgreSQL connections.
- Run the provided container as non-root with a read-only root filesystem, as the chart does.
