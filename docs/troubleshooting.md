# Troubleshooting

This page lists common problems with a live console and how to diagnose them. The examples use the Helm release
`openbao-operator-console` in the namespace `console-system`.

The sidebar shows the console version and source revision when available. Include them when reporting a problem.
They also appear in the authenticated snapshot API under `build` and in `console --version` output.

## Logs

The console writes JSON log lines to standard error, one object per line with `time`, `level`, and `msg` fields. In
Kubernetes, read them with:

```sh
kubectl --namespace console-system logs deployment/openbao-operator-console
```

A successful start logs `Console started` with the mode and listen address. Background workers log
`Persist collection failed`, `Prune history failed`, `Remove observations of removed connections failed`, and
`Credential renewal could not be stored`, with the connection ID where one applies. Logs never contain credentials,
tokens, raw provider errors, or Kubernetes resources. Collection failures are not logged; they appear as collection
states in the console (see [collection states](#collection-states)).

A startup failure is printed as one plain-text line, and the process exits with status 1. Kubernetes then restarts the
container and eventually reports `CrashLoopBackOff`. Read the failed container's output with:

```sh
kubectl --namespace console-system logs deployment/openbao-operator-console --previous
```

## Health endpoints

| Endpoint | Returns | Used by the chart for |
| --- | --- | --- |
| `/healthz` | `204` while the HTTP server runs. It checks nothing else. | Startup and liveness probes |
| `/readyz` | `204` when the console can read the fleet from PostgreSQL within three seconds, otherwise `503` with `Database unavailable`. | Readiness probe |

Both endpoints are unauthenticated and are available during browser setup. A database outage after startup makes the Pod
unready without restarting it. Neither endpoint checks the identity provider or Kubernetes collection.

## Startup failures

The console checks its configuration in this order: command-line options, the database URL and connection, schema
migrations, the encryption key, stored configuration, then the identity provider or browser setup. The first failure
stops startup.

An invalid `historyRetention` value fails first with `CONSOLE_HISTORY_RETENTION must be a duration such as 168h` or
`history retention must be between 1h and 8760h`.

### Database connection and TLS

| Message contains | Cause and fix |
| --- | --- |
| `CONSOLE_DATABASE_URL is required in live mode` | The URL is empty. Check `existingSecret`, `database.urlSecret`, and `database.urlKey`. |
| `server certificate is not signed by a trusted CA` | Verification is enabled but the server CA is unknown. Set `database.caSecret`, or add `sslrootcert` to the URL. |
| `server certificate does not match the host name` | With `verify-full`, the URL's host must appear in the server certificate. Use the host name from the certificate, such as the CloudNativePG `<cluster>-rw` Service. |
| `server certificate is expired or not yet valid` | Renew the server certificate, or check the clock of the node. |
| `host name lookup failed` | The host in the URL does not resolve from the console Pod. |
| `connection refused`, `connection attempt timed out`, `database is unreachable` | Network path, port, or a NetworkPolicy that restricts egress. |
| `database is starting up` | PostgreSQL accepted the connection but is still starting. |

The console retries these transport failures for up to 30 seconds before it fails, so the message names the cause of the
last attempt. A database that rejects the login or the database name fails immediately; check the user, password, and
database name.

The chart exports `PGSSLMODE` from `database.sslMode` (default `verify-full`) and `PGSSLROOTCERT` from
`database.caSecret`. Parameters in the URL take precedence over both. A database without TLS fails with the default
mode; set `sslmode=disable` in the URL or `database.sslMode: disable` only on a trusted network. The console reads the
URL and CA at startup, so restart it after either changes.

### Schema version

| Message contains | Cause and fix |
| --- | --- |
| `database schema version N is newer than version M supported by this console` | The database was migrated by a newer console. Run that version or newer, or restore the pre-upgrade backup. See [upgrade and roll back](upgrading.md). |
| `apply schema migration N` | A migration failed and was rolled back. The database is unchanged. Check that the database user can create and alter tables. |

### Encryption key

| Message contains | Cause and fix |
| --- | --- |
| `CONSOLE_CONFIG_KEY_FILE must reference a readable configuration key` | The key file is missing or unreadable. Check the `CONFIG_KEY` key in the runtime Secret. |
| `configuration key must contain 32 bytes` | The key is not 32 random bytes in a supported form. Generate it with `openssl rand -hex 32`. Raw bytes, hexadecimal, and base64 are accepted; surrounding whitespace is ignored. A 32-character password is rejected. |
| `configuration cannot be decrypted with the supplied key` | The key does not match the database. Use the key that belongs to this database or backup. A new key cannot read existing configuration. |

### Browser setup and the setup token

These checks apply only to an empty database without a seeded identity provider.

| Message contains | Cause and fix |
| --- | --- |
| `CONSOLE_SETUP_TOKEN_FILE must reference a readable private setup token` | The database is empty and the runtime Secret has no `SETUP_TOKEN` key. Add it, or supply the `OIDC_*` keys for automated bootstrap. The kubelet updates the mounted file within a minute or two; then restart the console. |
| `setup token file must be a regular file without permissions for other users` | The file is readable by other users. The chart mounts it with mode `0440`; for a binary, use `chmod 600`. |
| `setup token file must contain at most 256 bytes` | The file contains more than the token. |
| `setup token must contain 32 random bytes encoded as 64 hexadecimal or 43 URL-safe base64 characters` | Generate the token with `openssl rand -hex 32`. Surrounding whitespace is ignored. |
| `OIDC callback must be an HTTPS URL ending in /auth/callback` | Set the chart's `publicURL` to the console's HTTPS address, or fix `CONSOLE_OIDC_REDIRECT_URL` when running the binary. |
| `bootstrap identity requires OIDC issuer, client ID, and client secret` | Only some of the three `OIDC_*` keys are set. Supply all three or none. |

During setup, the browser shows these responses:

- **The setup window expired.** The window lasts one hour from process start. Create a new token, update the Secret, and
  restart the console while the database is still uninitialized.
- **Too many setup attempts. Retry in one minute.** Setup accepts five attempts per client and 64 attempts in total per
  minute. Without `trustedProxies`, all browsers behind one ingress count as one client; see
  [trusted proxies](../charts/openbao-operator-console/README.md#trusted-proxies).
- **Setup is already open in several browsers.** At most eight unlocked setup sessions can exist. Wait up to 15 minutes
  for them to expire, or restart the console.

An initialized database never reopens setup. A setup token mounted after initialization is ignored.

## Identity provider and sign-in

### The console does not start after initialization

`authentication startup failed: discover OIDC provider` means the console could not read the provider's discovery
document at startup. Check that the issuer URL is reachable from the Pod over HTTPS, that its certificate is trusted
(set `oidcCA.configMap` or the private CA in **Settings → Identity provider**), and that the `issuer` value in the
discovery document equals the configured issuer exactly, including any trailing slash. A provider that stays unavailable
prevents startup; use [administrator recovery](administration.md#recover-administrator-login) to switch providers.

### Redirect URI errors

The provider rejects the login when the callback URL is not registered exactly. Register the chart's
`publicURL` followed by `/auth/callback`, such as `https://console.example.com/auth/callback`. The Helm release notes
print the exact value. Open the console at the same scheme and host:
the console derives the allowed request origin from the callback URL, and rejects setup and sign-out requests from any
other origin with an invalid origin error. Session cookies are `Secure`, so the console must be served over HTTPS.

### Sign-in fails or is denied

After a failed login, the browser returns to the sign-in page with a message. The `signin` parameter in its URL names
the reason:

| `signin` | Cause |
| --- | --- |
| `expired` | The login took longer than allowed or was started in another tab. Start a new login. |
| `failed` | The code exchange, ID token signature, issuer, audience, nonce, or groups claim did not verify. Check the client ID and secret, and that the groups claim is a top-level array of strings. |
| `denied` | The verified subject and groups match no console user or group. An administrator must add an access rule. |
| `unavailable` | No identity provider is active. Sign in with a local account, or add a provider in **Settings**. |

### Local sign-in fails

| Message | Cause and fix |
| --- | --- |
| `The username, password, or code is incorrect.` | One of the three values is wrong. The message does not say which. Check that the authenticator app shows the code for this console, and that the device clock is correct. A code works once; wait for the next code before signing in again. |
| `Too many failed sign-ins for this username. Retry in N seconds.` | Five or more recent failures delay further attempts, up to 15 minutes. Wait, then sign in with the correct values. The account is not locked. |
| `Too many failed sign-ins. Retry in one minute.` | The client address reached 30 failures in a minute. Behind an ingress without `trustedProxies`, all browsers share one address. |

A lost authenticator: sign in with a recovery code. Without recovery codes, another administrator resets the
authenticator, or use [administrator recovery](administration.md#recover-administrator-login).

### Clock skew

ID tokens are rejected once their expiry time has passed on the console's clock, and a `nbf` (not before) time is
accepted up to five minutes early. Sessions also end at the ID token's expiry. If sign-in fails with `failed` right
after a successful provider login, or sessions end early, check that the console nodes and the provider synchronize
their clocks with NTP.

## Collection states

Each connection, and each enrolled operator Deployment, shows the result of its latest collection in **Settings →
Clusters** and on the deployment pages. Collection runs every 30 seconds per connection. When collection fails, the
console keeps the last known evidence and raises the **Collection is incomplete** finding instead of treating missing
evidence as healthy.

| State | Meaning | Action |
| --- | --- | --- |
| `pending` | No collection has completed since the connection or operator was enrolled or changed. | Wait for the first collection. |
| `current` | The latest collection succeeded within the last two minutes. | None. |
| `stale` | No collection completed in the last two minutes, or the connection is paused. | Check the console logs and resource limits, or resume the connection. |
| `denied` | Kubernetes rejected the reader with 401 or 403. | Apply the generated reader RBAC, and replace an expired credential. |
| `unreachable` | A transport, TLS, DNS, timeout, or reader configuration error. | Check that the API server address is reachable from the console Pod, its CA, and any egress NetworkPolicy. |
| `unsupported` | The `OpenBaoCluster` API is not served by the cluster. | Install the OpenBao Operator CRDs. |
| `notFound` (operator) | The named operator Deployment does not exist. | Check the operator namespace and Deployment name. |
| `invalid` (operator) | The declared container or its image reference was not found in the Deployment. | Check the container name. |

To test a reader credential without the database or identity provider, use
[`console --check`](connect-clusters.md#check-the-connection).
