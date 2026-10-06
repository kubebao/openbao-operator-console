# First administrator setup

An empty database supports either seeded identity configuration or protected
browser setup. An initialized database always loads its stored configuration.
Changing bootstrap environment variables or mounting a setup token cannot reopen
setup.

The deployment must provide PostgreSQL, a persistent 32-byte encryption key, and
an HTTPS callback URL ending in `/auth/callback`. Back up both PostgreSQL and the
matching key. Store the key separately from database backups. The key is required
to read a restored configuration.

## Browser setup

1. Create the runtime Secret with `DATABASE_URL`, `CONFIG_KEY`, and
   `OIDC_REDIRECT_URL`. The callback URL must match the public console address,
   for example `https://console.example.com/auth/callback`.
2. Generate a private setup token with 32 random bytes. Store the encoded token
   in a separate Kubernetes Secret. The following commands keep it out of shell
   arguments and terminal output:

   ```sh
   umask 077
   openssl rand -hex 32 > console-setup-token
   kubectl --context=your-console-context --namespace openbao-console create secret generic console-setup \
     --from-file=setup-token=console-setup-token
   ```

3. Set `setup.tokenSecret: console-setup` in the Helm values. The chart mounts
   the token and sets `CONSOLE_SETUP_TOKEN_FILE`. When running the binary directly,
   set that variable to the token file path.
4. Open the public console address. The console redirects to `/setup`. Enter
   the token through the password field. Use a private local method to read the
   token file; do not place it in a URL or a shared command transcript.
5. Register the displayed callback URL in an OpenID Connect client. Configure
   client authentication and the authorization code flow with PKCE S256.
6. Enter the HTTPS issuer, client ID, and client secret. If the provider uses a
   private certificate authority, enter its PEM certificate under **Private CA
   and group claim**. TLS verification remains enabled.
7. Select **Verify with login**. Complete the provider login with the account
   that must become the first console administrator.

The console verifies the token signature, issuer, audience, nonce, and PKCE
exchange before writing any identity configuration. The verified subject becomes
the first administrator. Configuration initialization uses a database revision
compare-and-swap, so a second setup claim cannot overwrite the first one.

The successful callback creates the normal console session and opens guided
cluster onboarding. The collector accepts subsequent enrollment without a restart.
Remove `setup.tokenSecret` from the Helm values, delete the setup Secret, and
remove the private token file after verifying administrator access. Keep the
configuration encryption key.

### Keycloak

Use a confidential OpenID Connect client with **Client authentication** and
**Standard flow** enabled. Register the exact callback URL in **Valid redirect
URIs** and configure PKCE S256. Copy the secret from **Credentials**. The issuer
is the realm URL, such as `https://identity.example.com/realms/platform`.

The groups claim is optional for the first administrator. Configure a top-level
ID-token claim containing an array of group names before adding group access in
the console.

### Expiry and retries

The setup window lasts one hour from process startup. Unlocking it creates a
Secure, HttpOnly, SameSite=Lax browser ticket that lasts at most 15 minutes.
Restarting an uninitialized process starts a new setup window. Rotate the token
before restarting it. Identity verification links expire after five minutes and
can be used once.
Starting a replacement trial invalidates earlier trials in the same setup
session. **Lock setup** invalidates that session and its unfinished trials.

If login fails or its link expires, return to `/setup` and start a new login.
Unfinished candidate credentials remain in bounded process memory; an expired
login cannot activate them. Failed candidates are not written to PostgreSQL. If
the browser ticket expires, enter the setup token again. If the one-hour window expires, generate a
new token and restart the console while the database is still uninitialized.

Requests are limited to five attempts per direct network peer per minute and 64
attempts globally per minute. The console does not trust forwarded address
headers for this limit. A shared ingress can cause browsers to share a peer
limit. Wait one minute after a rate-limit response.

All setup changes require the exact public request origin. Setup tokens are
accepted only in JSON POST bodies. The console retains their digest, never logs
them, and does not put them in browser storage. The token authorizes the initial
claim; anyone who has it can become the first administrator through a verified
provider login. Restrict access to the token and the installation.

## Automated first start

Supply `OIDC_ISSUER`, `OIDC_CLIENT_ID`, and `OIDC_CLIENT_SECRET` in the runtime
Secret. Enroll at least one administrator with `bootstrapAdminSubjects` or an
enrollment user with `admin: true`. An optional enrollment file can also contain
the complete `identity` object. A partially supplied environment identity is an
installation error.

The application imports this seed once and immediately starts normal
authentication. Browser setup remains closed. Later access and identity changes
use **Settings**. Deployment inputs still include PostgreSQL, the encryption key,
and the callback URL.

An unreadable setup token does not affect an initialized database. Stored
configuration takes precedence over seed files and bootstrap identity variables.
For lost administrator access or an unavailable provider, use the local recovery
procedure in [Console administration](administration.md).
