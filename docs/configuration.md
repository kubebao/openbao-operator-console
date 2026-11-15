# Command-line and environment reference

The console is one binary. The Helm chart sets these flags and variables for you; use this page when you run the binary
directly or need to know what the chart configures.

## Operator API selection

Each enrolled connection selects one operator API group: `kubebao.org` or legacy `openbao.org`, both at `v1alpha1`.
Set `apiGroup` in enrollment YAML. Omitted fields in existing files, stored settings, and API requests select
`openbao.org`. New browser enrollments default to `kubebao.org`; select the group installed in the cluster.

The connection check and collector use only the selected API. They do not discover or fall back to another group.
Generated reader Roles grant namespace LIST permission for that group only. Named controller Deployment GETs and
credential-renewal grants remain separate.

On the hosting cluster, use Helm `readerScopes` entries with `namespace` and `apiGroup` to grant the selected access.
Existing `readerNamespaces` entries grant only `openbao.org`. Declaring both grants for one namespace permits both API
groups; remove an obsolete declaration when that access is no longer needed. RBAC grants do not select the enrollment's
API group: configure `apiGroup` on the connection separately.

An enrollment identifier stays bound to its API group after removal and restart. To observe the other group, add a
connection with a new identifier and grant its reader access. Existing history and routes retain the original identity;
the console does not join migrated objects across API groups.

## Run without Kubernetes

Releases publish archives for Linux and macOS on `amd64` and `arm64`. Each contains the static `console` binary with the
web application embedded; see [verify a release](releasing.md#verify-a-release). `make build` writes the same binary to
`bin/console`.

The binary can run on a host behind an HTTPS reverse proxy:

```sh
./console --mode=live --listen=127.0.0.1:8090 --config=/etc/console/enrollment.yaml
```

`--mode` defaults to `demo`, which serves fixture data. Pass `--mode=live` for every manual run against real clusters.

## Flags

| Flag | Description |
| --- | --- |
| `--mode` | `demo` (default, fixture data, no login) or `live`. |
| `--listen` | Listen address. Default `127.0.0.1:8090`. |
| `--demo-allow-external` | Let demo mode listen on a non-loopback address, such as in a container. Demo mode has no login. |
| `--config` | Optional first-start enrollment YAML. Ignored once the database is initialized. |
| `--tls-cert`, `--tls-key` | Serve HTTPS directly. |
| `--schema` | `inspect`, `backup`, or `upgrade` in live mode, without serving. See [schema upgrades](upgrading.md). |
| `--backup` | Unused destination file for an offline schema backup or upgrade. |
| `--check` | Check enrolled Kubernetes reads and exit. Needs no database or OIDC. See [enrollment files and CLI](connect-clusters.md). |
| `--print-rbac` | Print reader RBAC for a connection ID and exit. |
| `--reader-name`, `--reader-namespace` | ServiceAccount used in RBAC that `--print-rbac` and the onboarding wizard generate for the hosting cluster. |
| `--recover-identity` | Replace the stored identity provider and administrators, then exit. See [administration](administration.md#recover-administrator-login). |
| `--recover-local-administrator` | Create or reset this local administrator, turn local sign-in on, print a one-time enrollment link, and exit. See [administration](administration.md#recover-with-a-local-administrator). |
| `--recover-access-rules` | With `--recover-identity` and a different issuer: `remove` or `carryOver` existing user and group access. Required for an issuer change. |
| `--trusted-proxies` | Comma-separated proxy addresses or CIDR prefixes whose `X-Forwarded-For` identifies the client for rate limits and logs. Default from `CONSOLE_TRUSTED_PROXIES`. |
| `--history-retention` | How long observation history and activity are kept, from `1h` to `8760h`. Default `168h` (seven days), or `CONSOLE_HISTORY_RETENTION` when set. |
| `--version` | Print the version and commit, then exit. The startup log also records both. |

## Environment

The console removes surrounding whitespace from these values, so files with a trailing newline work.

| Variable | Description |
| --- | --- |
| `CONSOLE_STORAGE` | `sqlite` or `postgres`. Defaults to SQLite when no PostgreSQL URL is set, otherwise PostgreSQL. Conflicting backend settings are refused. |
| `CONSOLE_SQLITE_PATH` | Local SQLite file. Defaults to `data/console.db` relative to the working directory. The parent directory is created with mode 0700. Use a durable local or block filesystem and back up the state. |
| `CONSOLE_DATABASE_URL` | PostgreSQL connection URL. Required when `CONSOLE_STORAGE=postgres`. Never falls back to SQLite if PostgreSQL is unavailable. |
| `CONSOLE_CONFIG_KEY_FILE` | Path to the encryption key: 32 bytes, raw or encoded as hexadecimal or base64 text. Required in live mode. |
| `CONSOLE_OIDC_REDIRECT_URL` | Public HTTPS callback URL ending in `/auth/callback`. The chart derives it from `publicURL`. |
| `CONSOLE_OIDC_ISSUER`, `CONSOLE_OIDC_CLIENT_ID`, `CONSOLE_OIDC_CLIENT_SECRET` | Automated bootstrap identity. Supply all three or none. |
| `CONSOLE_OIDC_CA_FILE` | PEM bundle for a private identity provider CA. |
| `CONSOLE_BOOTSTRAP_ADMIN_SUBJECTS` | Comma-separated OIDC subjects for the first administrators. |
| `CONSOLE_SETUP_TOKEN_FILE` | Setup token file for protected browser setup. Read only while the database is uninitialized. |
| `CONSOLE_TRUSTED_PROXIES` | Default for `--trusted-proxies`. |
| `CONSOLE_HISTORY_RETENTION` | Default for `--history-retention`, as a Go duration such as `168h`. |
| `PGSSLMODE`, `PGSSLROOTCERT` | PostgreSQL TLS defaults. Parameters in the connection URL take precedence. |
