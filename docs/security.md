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
| Configuration (connections, access rules, relationships, policies, identity provider) | PostgreSQL, encrypted with AES-256-GCM and bound to its revision |
| Kubernetes reader credentials | PostgreSQL, encrypted per connection with AES-256-GCM and bound to the connection ID and credential revision |
| Local account credentials (Argon2id password hash, TOTP secret, recovery code digests) | PostgreSQL, encrypted per account with AES-256-GCM and bound to the account ID and credential revision |
| Observations, findings, and activity | PostgreSQL, approved fields only; history kept for the configured retention (default seven days) |
| Configuration changes | PostgreSQL: actor, time, revision, and changed area, without credential values |
| Encryption key | Outside PostgreSQL, mounted from a Kubernetes Secret or file |

The collector projects approved fields from Kubernetes objects before storing them. Raw Kubernetes objects, Secret
data, and provider error messages are not stored or returned by the API. Credentials are write-only in the UI.

## Authentication and sessions

- Users sign in through OpenID Connect with the authorization code flow, PKCE (`S256`), and nonce verification, or with
  a [local account](#local-accounts).
- The console validates the ID token signature, issuer, and audience.
- The browser receives an opaque `Secure`, `HttpOnly` session cookie. Provider tokens stay on the server and are not
  persisted.
- Sessions expire after eight hours or at ID token expiry, whichever comes first. Sessions live in process memory. Each
  sign-in issues a new session ID and ends the session the browser already had.
- A login in progress lives in a `Secure`, `HttpOnly` cookie sealed with AES-256-GCM under a random per-process key. It
  expires after five minutes, and its state is accepted once. Unauthenticated login requests keep no server state, so
  they cannot block other sign-ins. A restart cancels logins that are in progress.
- Logout ends the console session. It does not end the identity provider's single sign-on session.
- A failed login returns to the sign-in page with a fixed reason: expired, no access, failed, or unavailable. Provider
  error details are not shown.
- The console logs each failed sign-in as a security event with a reason class and the client address. It adds the
  subject only after the provider verified it, and never logs tokens, authorization codes, state values, or provider
  messages. Failed API requests log the method, route pattern, and a cause class, without driver messages.
- Requests that change state require the public origin. The console compares origins case-insensitively and ignores
  an explicit default port.
- API requests authenticate with the `__Host-console-session` cookie. Only the setup endpoints work without it: setup
  status, unlock, and lock are public, and identity and local administrator setup require the `__Host-console-setup`
  cookie that a valid setup token opens. Sign-in options and local sign-in are public. `api/openapi.yaml` declares both cookies as security schemes.

## Local accounts

Local accounts sign in with a username, a password, and a TOTP code from an authenticator app, or one of ten
single-use recovery codes. OpenID Connect with multi-factor authentication at the provider remains the recommended
sign-in for teams.

- Passwords follow NIST SP 800-63B: at least 15 characters, up to 1024 bytes, Unicode NFKC normalization, no
  composition rules, and a check against common, repetitive, keyboard-pattern, and context-specific values.
- Passwords are hashed with Argon2id (19 MiB, two passes, one lane). At most four hashes run at once, which bounds
  memory. Unknown usernames cost the same hashing work as known ones.
- TOTP follows RFC 6238 (SHA-1, six digits, 30 seconds) and accepts one step of clock drift. A code is accepted once:
  each sign-in stores the accepted step with compare-and-swap, so a replayed or concurrent code fails. Recovery codes
  carry 80 random bits, are stored as SHA-256 digests, and are removed when used.
- Every failed sign-in returns the same message, whether the username, password, or code was wrong.
- After five failures for a username, each further attempt waits 30 seconds, doubling up to 15 minutes. The account is
  never locked, so guessing cannot lock out an administrator. Unknown usernames are delayed the same way. Each client
  address also has a limit of 30 failures per minute; see [trusted proxies](../charts/openbao-operator-console/README.md#trusted-proxies)
  for clients behind an ingress. Failure counters live in process memory.
- Local accounts match only the reserved `console:local` issuer. They are independent of the identity provider and its
  issuer-scoped access rules, and they stay when the provider changes.
- Credentials are keyed by a random account ID. A deleted account's credential cannot apply to a new account with the
  same username; startup removes such leftover credentials.

## Authorization

- Users match by OIDC `sub`. Groups match by exact values in a signed ID token claim.
- User and group access is issuer-scoped. The configuration stores the issuer that its users and groups belong to, and a
  sign-in matches only when the verified ID token's `iss` equals it. A subject or group name from another issuer grants
  nothing.
- Changing to a provider with a different issuer, in Settings or through local recovery, requires an explicit decision
  to remove existing users and groups or to carry them over to the new issuer. Same-issuer changes keep them. See
  [changing the identity provider](administration.md#change-the-identity-provider).
- Non-administrators see only the clusters and namespaces assigned to them. The API authorizes resources before it
  returns counts, search results, topology, or history.
- Administrators can see all enrolled namespaces and change settings.
- Access rule changes apply to existing sessions on their next request.
- Kubernetes labels and unmapped claims never grant access.

## First-run setup

Protected browser setup requires a deployment-provided token, expires one hour after startup, rate-limits rejected
tokens, and closes permanently once a verified login or a local administrator with a verified authenticator code
creates the first administrator. The valid token is never
throttled, so failed guesses through a shared proxy address cannot lock out its holder. The console logs each rejected
token with the client address, never the submitted value. Anyone who holds the setup token can become the first
administrator, so restrict access to it and remove it after setup. See [first administrator setup](bootstrap.md).

## Browser protections

Every response carries `Cache-Control: no-store`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`,
and this content security policy:

```text
default-src 'self'; script-src 'self'; style-src 'self' 'nonce-<random>'; img-src 'self' data:; font-src 'self';
connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'
```

- Scripts load only from the console origin. Inline scripts and `eval` are blocked.
- Stylesheets load only from the console origin. Each application shell response carries a new random nonce, which
  the frontend uses for the style elements that dialogs inject to lock page scrolling. Other responses carry no nonce.
  Because the shell is never cached, a page never reuses an earlier nonce.
- Inline style attributes are blocked. The frontend applies dynamic sizes and positions through the CSS object model,
  which the policy does not restrict.
- Other sites cannot frame the console.

## Demo mode

Demo mode serves fixture data without authentication and labels every page as demonstration data. It refuses to listen
on a non-loopback address unless `--demo-allow-external` is set. Use that flag only inside a container whose published
port is bound to the host's loopback interface. Do not expose demo mode.

## Operational guidance

- Serve the console over HTTPS only. When the public callback URL uses HTTPS, every response carries
  `Strict-Transport-Security: max-age=31536000`. The console does not set `includeSubDomains` or `preload`.
- Back up the encryption key separately from database backups, and restrict access to both.
- Use TLS with certificate verification for remote PostgreSQL connections.
- Run the provided container as non-root with a read-only root filesystem, as the chart does.
