# Renew Kubernetes reader credentials

Remote Kubernetes connections can renew their ServiceAccount bearer tokens automatically. Enable renewal per connection.
The hosting cluster uses the mounted ServiceAccount token managed by Kubernetes. Client certificates and other remote
credential types require manual replacement.

## Enable renewal

For a new remote cluster, select **Renew ServiceAccount tokens automatically** in the onboarding **Scope** step.
Enter the reader namespace and ServiceAccount before generating RBAC. Apply the generated grants through the cluster's
administrator context, then issue and upload the initial scoped reader credential.

For an existing connection:

1. Open **Settings → Clusters** and select **Edit**.
2. Select **Renew ServiceAccount tokens automatically**. Enter the existing reader namespace and ServiceAccount.
3. Download reader RBAC and apply it in that cluster through an administrator context.
4. Save the connection. Check that its status becomes **Token renewal active**.

The token's declared subject must match `system:serviceaccount:<namespace>:<serviceAccount>`. Kubernetes authenticates
and authorizes each request. A parsed token subject does not prove its authenticity.

The additional namespaced Role rule is:

```yaml
apiGroups: [""]
resources: [serviceaccounts/token]
verbs: [create]
resourceNames: [console-reader]
```

The RoleBinding references that reader ServiceAccount. No Secret reads, ServiceAccount creation, Pod access, or workload
mutation permissions are added. Kubernetes administrators own grant application and removal.

## Timing and outages

Renewal requests a 24-hour token. Kubernetes can grant a shorter lifetime. The console uses the API's
`expirationTimestamp`, schedules renewal after about one third of the granted lifetime, and caps that delay at eight hours.
Small scheduling jitter moves attempts earlier. Pending and overdue readers are checked every 15 seconds, with at most
four requests in flight. Each attempt has a 45-second deadline, including candidate validation.

Failed requests retain the current token and retry with backoff from 30 seconds to five minutes. A candidate must pass
scoped workload collection before activation. It is encrypted and committed to PostgreSQL before replacing the active
reader. Restart preserves the credential and next attempt time. Background updates do not invalidate an open settings form.

With a granted 24-hour lifetime, an uninterrupted connection renews about every 7.2–8 hours. The token then has roughly
16–24 hours remaining when an outage starts, depending on the last successful renewal. This is a timing example,
not an outage guarantee: a shorter granted lifetime, previous failures, revocation, or pausing reduces that window.

If the token expires while the console or Kubernetes API is unavailable, automatic renewal cannot authenticate. Issue a
fresh reader credential through an administrator context and reconnect.

## Resolve a status

| Status | Action |
| --- | --- |
| Token renewal pending | Wait for the first attempt. |
| Token renewal active | Review the server-issued expiry and next renewal time. |
| Token renewal retrying | Check API connectivity, TLS trust, and scoped workload permissions. The current token remains active. |
| Renewal permission required | Apply the named TokenRequest grant. Retries continue while the token remains valid. |
| Reconnect required | Issue a new scoped reader credential. Select **Reconnect**, upload it, test the connection, and save. |
| Token renewal paused | Resume collection before expiry, or reconnect with a fresh credential. Pausing also stops renewal. |

Renewal stops after a known token expiry or a TokenRequest authentication rejection. Disabling renewal leaves the current
token available for collection until Kubernetes rejects it. Removing the connection deletes its stored credential;
administrators must remove obsolete Kubernetes grants separately.

## Storage

Reader credentials are stored separately from editable configuration. Each credential has its own revision and
persisted renewal state, so background renewals do not change the configuration revision or invalidate an open
settings form. AES-256-GCM binds each ciphertext to its connection identifier, and the encryption key stays outside
PostgreSQL. Renewal never changes the API server, CA, client certificate, or enrollment scope.

Back up PostgreSQL and the matching encryption key together. See [administration](administration.md) for upgrades.
