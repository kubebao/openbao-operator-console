# Command-line and environment reference

The console is one binary. The Helm chart sets these flags and variables for you; use this page when you run the binary
directly or need to know what the chart configures.

## Run without Kubernetes

The binary can run on a host behind an HTTPS reverse proxy:

```sh
./bin/console --mode=live --listen=127.0.0.1:8090 --config=/etc/console/enrollment.yaml
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
| `--check` | Check enrolled Kubernetes reads and exit. Needs no database or OIDC. See [enrollment files and CLI](connect-clusters.md). |
| `--print-rbac` | Print reader RBAC for a connection ID and exit. |
| `--reader-name`, `--reader-namespace` | ServiceAccount used in RBAC that `--print-rbac` and the onboarding wizard generate for the hosting cluster. |
| `--recover-identity` | Replace the stored identity provider and administrators, then exit. See [administration](administration.md#recover-administrator-login). |
| `--recover-access-rules` | With `--recover-identity` and a different issuer: `remove` or `carryOver` existing user and group access. Required for an issuer change. |
| `--history-retention` | How long observation history and activity are kept, from `1h` to `8760h`. Default `168h` (seven days), or `CONSOLE_HISTORY_RETENTION` when set. |
| `--version` | Print the version and commit, then exit. The startup log also records both. |

## Environment

The console removes surrounding whitespace from these values, so files with a trailing newline work.

| Variable | Description |
| --- | --- |
| `CONSOLE_DATABASE_URL` | PostgreSQL connection URL. Required in live mode. |
| `CONSOLE_CONFIG_KEY_FILE` | Path to the encryption key: 32 bytes, raw or encoded as hexadecimal or base64 text. Required in live mode. |
| `CONSOLE_OIDC_REDIRECT_URL` | Public HTTPS callback URL ending in `/auth/callback`. The chart derives it from `publicURL`. |
| `CONSOLE_OIDC_ISSUER`, `CONSOLE_OIDC_CLIENT_ID`, `CONSOLE_OIDC_CLIENT_SECRET` | Automated bootstrap identity. Supply all three or none. |
| `CONSOLE_OIDC_CA_FILE` | PEM bundle for a private identity provider CA. |
| `CONSOLE_BOOTSTRAP_ADMIN_SUBJECTS` | Comma-separated OIDC subjects for the first administrators. |
| `CONSOLE_SETUP_TOKEN_FILE` | Setup token file for protected browser setup. Read only while the database is uninitialized. |
| `CONSOLE_HISTORY_RETENTION` | Default for `--history-retention`, as a Go duration such as `168h`. |
| `PGSSLMODE`, `PGSSLROOTCERT` | PostgreSQL TLS defaults. Parameters in the connection URL take precedence. |
