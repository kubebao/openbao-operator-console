# Console administration

Console administrators manage cluster connections, user and group access, declared relationships, attention policies,
and OpenID Connect (OIDC) settings in **Settings**. PostgreSQL stores this configuration. Enabling automatic credential
renewal starts named ServiceAccount TokenRequests. Settings do not change workload configuration or OpenBao.

## Initialize the console

Provide the database URL, public HTTPS callback URL, and configuration encryption key through the deployment secret
mechanism. Choose [protected browser setup or automated identity bootstrap](bootstrap.md). `CONSOLE_CONFIG_KEY_FILE`
must reference 32 random bytes, stored raw or encoded as hexadecimal or base64 text. Generate a key in a private
directory:

```sh
umask 077
openssl rand -hex 32 > /private/console-config-key
```

For Helm, put the key in the runtime Secret's `CONFIG_KEY` key. Keep the key across upgrades. Back it up separately
from PostgreSQL; a database backup cannot restore configuration without the matching key.

For automated identity bootstrap, provide a confidential OIDC client and enroll at least one administrator. Set
`bootstrapAdminSubjects` in Helm or `CONSOLE_BOOTSTRAP_ADMIN_SUBJECTS` to a comma-separated list of OIDC subject
identifiers. An optional enrollment YAML file can seed connections, users, groups, relationships, and policies. Set
`admin: true` on a seed user or group that must manage the console.

Browser setup enrolls the first administrator through a verified provider login. Automated first start imports the seed
configuration once. Later starts load PostgreSQL and ignore seed enrollment and OIDC values. The database URL,
encryption-key file, public callback URL, listen address, and TLS termination remain deployment settings. Changing seed
YAML does not overwrite changes made in the console.

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

Collection uses the saved configuration without restarting the application. Each connection is collected on its own
30-second schedule. A new or changed connection is collected immediately. In-flight results from the old revision cannot
be persisted. Pausing retains last-known inventory and marks collection stale. Removing a connection hides its inventory
and access scopes immediately. The collector then deletes its stored inventory, history, and activity, so a connection
enrolled again under the same ID starts without old evidence. The UI removes relationships and scope grants that
reference a removed namespace or connection.

## Manage access and relationships

Administrators can view all enrolled namespaces and change console settings. Other users receive only their assigned
connection and namespace scopes. Match users by OIDC `sub`, and groups by an exact signed ID-token group value. User
and group access belongs to the active provider's issuer. A sign-in matches only when the ID token's `iss` equals that
issuer, so the same subject or group name from another issuer grants nothing. The configured groups claim must contain a
top-level array of strings. Unmapped groups grant no access. A user who signs in without a matching subject or group
returns to the sign-in page with a message to ask an administrator for access.

Activity applies the current namespace scopes before each page. Previously issued page cursors do not retain access
after a rule changes. Events for a deleted deployment remain visible while their connection and namespace stay enrolled
and authorized, until retention removes them.

Console access-rule changes affect existing sessions on their next request. Group membership is captured during login;
identity-provider membership changes take effect after a new login or session expiration. Sessions expire no later than
the ID token. Sessions are process-local; the chart runs one replica and restarts end existing console sessions.

Add a relationship by selecting observed source and target deployments. The console stores both resource UIDs. A
recreated resource cannot inherit the old relationship. Snapshot storage is a display value, not a backup catalog or a
verified recovery point. The topology view and recovery details display available evidence for each selected path.

## Change the identity provider

Open **Settings → Identity provider**. Enter issuer, client ID, client secret, groups claim, and any private CA. What
happens next depends on whether the issuer changes.

**Same issuer.** Rotating the client secret or changing the client ID, groups claim, or CA keeps the issuer. Existing
user and group access stays. Select **Verify login and activate** and sign in through the candidate provider as a
current console administrator.

**New issuer.** Subjects and group names are only unique within one issuer. Another provider can reuse them for
different people; GitLab, for example, uses numeric subjects. Access therefore never moves to a new issuer silently:

1. Select **Review access changes**. The console lists every user and group that belongs to the current issuer.
2. Choose what happens to them:
   - **Remove existing access** deletes every user and group. Only the account you sign in with keeps access.
   - **Carry over to the new issuer** keeps every user and group and applies them to the new issuer. Choose this only
     when the new provider issues the same subjects and group names for the same people.
3. Select **Verify login and activate** and sign in through the candidate provider. The account you sign in with
   becomes a console administrator under the new issuer.

The API enforces the same rule: a trial with a different issuer requires `accessRules` (`carryOver` or `remove`), and a
trial with the same issuer rejects it.

Activation requires a verified candidate login, a still-authorized initiating session, and an unchanged configuration
revision. With the same issuer, the candidate login must also resolve to a current console administrator. A discovery
failure, denied login, expired trial, or concurrent settings change leaves the active provider and access unchanged.
Activation ends old console sessions and records the access decision in **Settings → Changes**. The verified
administrator receives a new session.

## Recover administrator login

If the active provider is unavailable or access mappings lock out administrators, use the deployment's local recovery
command. This requires the existing database URL and encryption key, bootstrap OIDC settings, the public callback URL,
and `CONSOLE_BOOTSTRAP_ADMIN_SUBJECTS` for a known administrator on the recovery provider.

1. Stop the serving console process. In Kubernetes, scale the application Deployment to zero.
2. Run a one-off process or Job using the same application image, database, and encryption key, with the recovery OIDC
   environment and administrator subject. Run `console --mode=live --recover-identity`. If the recovery issuer differs
   from the stored issuer, also choose what happens to existing user and group access: add
   `--recover-access-rules=remove` to delete it, or `--recover-access-rules=carryOver` to apply it to the recovery
   issuer. Without this flag, recovery to a different issuer fails and changes nothing. With the same issuer, omit the
   flag.
3. Verify the command exits successfully. It checks provider discovery, records the recovery change, and exits without
   starting the collector or HTTP server. It preserves cluster connections. Existing access rules stay with the same
   issuer and follow the `--recover-access-rules` decision for a new issuer. The subjects in
   `CONSOLE_BOOTSTRAP_ADMIN_SUBJECTS` become administrators under the recovery issuer.
4. Start the console again and sign in as the recovery administrator.

Recovery bypasses the browser login trial because an unavailable provider cannot complete it. Limit access to the
database, encryption key, and recovery process to deployment administrators.

### Example: recovery Job

This example assumes the Helm release `openbao-operator-console` in `console-system`, the runtime Secret
`console-runtime`, and a database URL in that Secret. Create a separate Secret with the recovery provider and the
administrator subject from private files:

```sh
kubectl --namespace console-system create secret generic console-recovery \
  --from-file=OIDC_ISSUER=recovery-issuer \
  --from-file=OIDC_CLIENT_ID=recovery-client-id \
  --from-file=OIDC_CLIENT_SECRET=recovery-client-secret \
  --from-file=ADMIN_SUBJECTS=recovery-admin-subjects
kubectl --namespace console-system scale deployment/openbao-operator-console --replicas=0
```

Use the image of the stopped Deployment. A newer image would also apply its schema migrations. Copy the database
settings from the Deployment if they differ, for example `database.urlSecret`, `PGSSLMODE`, or `PGSSLROOTCERT` with its
volume.

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: console-recover-identity
  namespace: console-system
spec:
  backoffLimit: 0
  ttlSecondsAfterFinished: 3600
  template:
    spec:
      restartPolicy: Never
      automountServiceAccountToken: false
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        runAsGroup: 65532
        fsGroup: 65532
        seccompProfile:
          type: RuntimeDefault
      containers:
        - name: recover
          image: ghcr.io/kubebao/openbao-operator-console:<version of the Deployment>
          args: ["--mode=live", "--recover-identity"]
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities:
              drop: ["ALL"]
          env:
            - name: CONSOLE_CONFIG_KEY_FILE
              value: /etc/console/key/config-key
            - name: PGSSLMODE
              value: verify-full
            - name: CONSOLE_DATABASE_URL
              valueFrom:
                secretKeyRef: {name: console-runtime, key: DATABASE_URL}
            - name: CONSOLE_OIDC_REDIRECT_URL
              value: https://console.example.com/auth/callback
            - name: CONSOLE_OIDC_ISSUER
              valueFrom:
                secretKeyRef: {name: console-recovery, key: OIDC_ISSUER}
            - name: CONSOLE_OIDC_CLIENT_ID
              valueFrom:
                secretKeyRef: {name: console-recovery, key: OIDC_CLIENT_ID}
            - name: CONSOLE_OIDC_CLIENT_SECRET
              valueFrom:
                secretKeyRef: {name: console-recovery, key: OIDC_CLIENT_SECRET}
            - name: CONSOLE_BOOTSTRAP_ADMIN_SUBJECTS
              valueFrom:
                secretKeyRef: {name: console-recovery, key: ADMIN_SUBJECTS}
          volumeMounts:
            - name: configuration-key
              mountPath: /etc/console/key
              readOnly: true
      volumes:
        - name: configuration-key
          secret:
            secretName: console-runtime
            defaultMode: 0440
            items:
              - key: CONFIG_KEY
                path: config-key
```

When the recovery provider has a different issuer, append `"--recover-access-rules=remove"` or
`"--recover-access-rules=carryOver"` to `args`. For a provider with a private CA, mount its PEM bundle and set
`CONSOLE_OIDC_CA_FILE`. Apply the Job and check its result, then start the console and delete the recovery Secret:

```sh
kubectl --namespace console-system apply -f console-recover-identity.yaml
kubectl --namespace console-system wait --for=condition=complete job/console-recover-identity --timeout=120s
kubectl --namespace console-system logs job/console-recover-identity
kubectl --namespace console-system scale deployment/openbao-operator-console --replicas=1
kubectl --namespace console-system delete secret console-recovery
```

The command prints nothing on success. On failure, it prints the reason and exits with status 1, and the Job fails. If
you changed the identity provider permanently, update the `OIDC_*` keys in the runtime Secret as well; stored
configuration takes precedence over them on later starts.

## Concurrent edits and upgrades

Every save uses a configuration revision. A stale edit returns a conflict. Reload settings before applying the change again.
**Settings → Changes** records the actor, time, revision, and changed area without credential values.

Back up PostgreSQL and the encryption key before an upgrade. At startup, a newer binary applies its pending numbered schema
migrations in one transaction and records each version in `console_schema_migration`. A failed migration rolls back
completely and the console does not start. An older binary refuses a database with a newer schema version and names both
versions in the error. Rollback across a schema change therefore requires the matching pre-upgrade database backup as
well as the older image; a Helm rollback alone is insufficient. A database restore also restores the access rules,
identity settings, and retained observations from that backup.

The console requires one serving process. Shared sessions, coordinated collectors, key rotation tooling, and a separate
migration role are not implemented yet.
