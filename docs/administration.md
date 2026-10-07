# Console administration

Console administrators manage cluster connections, user and group access, declared relationships, attention policies,
and OpenID Connect (OIDC) settings in **Settings**. PostgreSQL stores this configuration. Enabling automatic credential
renewal starts named ServiceAccount TokenRequests. Settings do not change workload configuration or OpenBao.

## Initialize the console

Provide the database URL, public HTTPS callback URL, and configuration encryption key through the deployment secret
mechanism. Choose [protected browser setup or automated identity bootstrap](bootstrap.md).
`CONSOLE_CONFIG_KEY_FILE` must reference exactly 32 raw random bytes.
Generate a key in a private directory:

```sh
umask 077
openssl rand 32 > /private/console-config-key
```

For Helm, put those bytes in the runtime Secret's `CONFIG_KEY` key. Keep the key across upgrades. Back it up separately
from PostgreSQL; a database backup cannot restore configuration without the matching key.

For automated identity bootstrap, provide a confidential OIDC client and enroll at least one administrator.
Set `bootstrapAdminSubjects` in Helm or `CONSOLE_BOOTSTRAP_ADMIN_SUBJECTS` to a
comma-separated list of OIDC subject identifiers. An optional enrollment YAML file can seed connections, users, groups,
relationships, and policies. Set `admin: true` on a seed user or group that must manage the console.

Browser setup enrolls the first administrator through a verified provider login. Automated first start imports the seed
configuration once. Later starts load PostgreSQL and ignore seed enrollment and OIDC
values. The database URL, encryption-key file, public callback URL, listen address, and TLS termination remain deployment
settings. Changing seed YAML does not overwrite changes made in the console.

## Add a Kubernetes cluster

Open **Settings → Clusters → Add cluster**. The [onboarding flow](cluster-onboarding.md) guides you through the cluster,
namespace scope, reader permissions, credentials, and verification. RBAC is available before uploading credentials.
Review the deployment preview, then connect the cluster or save it paused. Configure team, environment, and location
metadata after enrollment. The console does not apply the reader manifests.

Kubeconfig import accepts embedded tokens and client certificates. It rejects authentication plugins, file references,
impersonation, proxies, TLS verification bypass, and server-name overrides. Use a reader credential with namespace-scoped
list access and named Deployment get access. Uploading an administrator credential does not reduce its Kubernetes privileges.

Stored credentials are write-only in the UI. Leave credential fields empty to retain them. Supply a complete credential
to replace them. Remote ServiceAccount tokens support [opt-in automatic renewal](credential-renewal.md). Other remote
credentials require manual replacement before expiration.

Collection uses the saved configuration without restarting the application. A change triggers the next bounded collection
round. In-flight results from the old revision cannot be persisted. Pausing retains last-known inventory and marks collection
stale. Removing a connection hides its inventory and access scopes immediately; retained observations expire with normal
history retention. The UI removes relationships and scope grants that reference a removed namespace or connection.

## Manage access and relationships

Administrators can view all enrolled namespaces and change console settings. Other users receive only their assigned
connection and namespace scopes. Match users by OIDC `sub`, and groups by an exact signed ID-token group value.
The configured groups claim must contain a top-level array of strings. Unmapped groups grant no access. A user who
signs in without a matching subject or group returns to the sign-in page with a message to ask an administrator for
access.

Console access-rule changes affect existing sessions on their next request. Group membership is captured during login;
identity-provider membership changes take effect after a new login or session expiration. Sessions expire no later than
the ID token. Sessions are process-local; the chart runs one replica and restarts end existing console sessions.

Add a relationship by selecting observed source and target deployments. The console stores both resource UIDs.
A recreated resource cannot inherit the old relationship. Snapshot storage is a display value, not a backup catalog
or a verified recovery point. The topology view and recovery details display available evidence for each selected path.

## Change the identity provider

1. Enroll an administrator user or group recognized by the candidate provider before changing OIDC settings.
2. Open **Settings → Identity provider**. Enter issuer, client ID, client secret, groups claim, and any private CA.
3. Select **Verify login and activate** and sign in through the candidate provider.

Activation requires a verified candidate identity that resolves to a current console administrator, a still-authorized
initiating session, and an unchanged configuration revision. A discovery failure, denied login, expired trial, or concurrent
settings change leaves the active provider unchanged. Activation ends old console sessions. The verified administrator
receives a new session.

## Recover administrator login

If the active provider is unavailable or access mappings lock out administrators, use the deployment's local recovery
command. This requires the existing database URL and encryption key, bootstrap OIDC settings, the public callback URL,
and `CONSOLE_BOOTSTRAP_ADMIN_SUBJECTS` for a known administrator on the recovery provider.

1. Stop the serving console process. In Kubernetes, scale the application Deployment to zero.
2. Run a one-off process or Job using the same application image, database, and encryption key, with the recovery OIDC
   environment and administrator subject. Run `console --mode=live --recover-identity`.
3. Verify the command exits successfully. It checks provider discovery, records the recovery change, and exits without
   starting the collector or HTTP server. It preserves cluster connections and existing access rules.
4. Start the console again and sign in as the recovery administrator.

Recovery bypasses the browser login trial because an unavailable provider cannot complete it. Limit access to the database,
encryption key, and recovery process to deployment administrators.

## Concurrent edits and upgrades

Every save uses a configuration revision. A stale edit returns a conflict. Reload settings before applying the change again.
**Settings → Changes** records the actor, time, revision, and changed area without credential values.

Back up PostgreSQL and the encryption key before an upgrade. A newer binary can migrate the schema at startup,
and an older binary refuses a newer schema. Rollback across a schema change therefore requires the matching pre-upgrade
database backup as well as the older image; a Helm rollback alone is insufficient. A database restore also restores the access rules,
identity settings, and retained observations from that backup.

The console requires one serving process. Shared sessions, coordinated collectors, key rotation tooling, and a separate
migration role are not implemented yet.
