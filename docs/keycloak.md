# Configure Keycloak

The console uses standard OpenID Connect (OIDC) discovery and authorization code login with PKCE. Keycloak does not
require a provider-specific adapter. Other OIDC providers use the same configuration.

## Register the client

In the intended Keycloak realm, create an OpenID Connect client with these settings:

| Setting | Value |
| --- | --- |
| Client ID | `openbao-console`, or your chosen ID |
| Client authentication | On |
| Standard flow | On |
| Implicit flow | Off |
| Direct access grants | Off |
| Service accounts | Off |
| PKCE method | `S256` |
| Valid redirect URI | Exact HTTPS console URL ending in `/auth/callback` |

The issuer is `https://keycloak.example/realms/<realm>`. The console discovers authorization, token, and signing-key
endpoints from the realm's discovery document. See
[Keycloak's OIDC endpoint documentation](https://www.keycloak.org/securing-apps/oidc-layers).

Set `CONSOLE_OIDC_ISSUER`, `CONSOLE_OIDC_CLIENT_ID`, `CONSOLE_OIDC_CLIENT_SECRET`, and `CONSOLE_OIDC_REDIRECT_URL`
through your runtime secret mechanism. The client secret stays on the server. Browser sessions use an opaque cookie.

If Keycloak uses a private certificate authority, set `CONSOLE_OIDC_CA_FILE` to its PEM certificate bundle. The console
adds that bundle to system roots for OIDC discovery, code exchange, and key retrieval. It keeps TLS verification
enabled. Restart the console after changing provider settings or the CA bundle. The Kubernetes chart can mount the
bundle from `oidcCA.configMap` and `oidcCA.key`.

Use an issuer URL reachable by both the browser and backend. An internal container service name is not a browser issuer.
Configure Keycloak's public hostname and TLS so discovery returns the same HTTPS issuer and endpoints. See
[Keycloak TLS configuration](https://www.keycloak.org/server/enabletls).

## Enroll access

Use the user's OIDC `sub` value in `users[].subject`. For the default Keycloak subject mapping, this is the user's ID,
which is visible in the realm's user details. Confirm the mapping when using a custom subject protocol mapper. A
username, email address, group, or Kubernetes label does not grant access. Subjects and groups apply only to the realm's
issuer. Moving to another realm or provider changes the issuer and requires a decision about existing access; see
[changing the identity provider](administration.md#change-the-identity-provider).

Add namespace scopes for each permitted connection. `examples/multi-cluster-enrollment.yaml` shows two independent
Kubernetes clusters with separate reader credentials and operator installations. YAML seeds the first start. Use
**Settings → Clusters** and **Settings → Access** to manage later enrollment and scopes.

The console supports one OIDC issuer per application instance. Administrators can import reader credentials and change
access in Settings. Human OIDC sessions and Kubernetes reader token renewal have separate lifecycles.

Logout ends the console session. It does not end the Keycloak SSO session. Selecting Sign in can reuse that SSO session.
There is no back-channel logout or immediate provider revocation check; local session expiry is bounded by ID token
expiry.

For a disposable local Keycloak used in development and integration tests, see [development](development.md#keycloak).
