import { useState } from 'react'
import { Check, Copy } from 'lucide-react'
import type { IdentityInput } from '../../api/schema'
import { Field } from '../form'
import { Button } from '../ui/button'

function CallbackURL({ url, onError }: { url: string; onError: (message: string) => void }) {
  const [copied, setCopied] = useState(false)
  return (
    <div className="setup-callback">
      <Field label="Callback URL" help="Register this exact URL in your identity provider client.">
        <input value={url} readOnly aria-label="Callback URL" />
      </Field>
      <Button
        type="button"
        size="icon"
        aria-label={copied ? 'Callback URL copied' : 'Copy callback URL'}
        onClick={() => {
          void navigator.clipboard
            .writeText(url)
            .then(() => {
              setCopied(true)
            })
            .catch(() => {
              onError('Select the callback URL and copy it manually.')
            })
        }}
      >
        {copied ? <Check size={16} /> : <Copy size={16} />}
      </Button>
    </div>
  )
}

function KeycloakHelp() {
  return (
    <details className="setup-help">
      <summary>Configure Keycloak</summary>
      <ol>
        <li>
          Create an OpenID Connect client with client authentication and the standard authorization
          code flow enabled.
        </li>
        <li>Register the callback URL above as a valid redirect URI. Use PKCE with S256.</li>
        <li>Copy the client secret from Credentials. Use the realm URL as the issuer.</li>
      </ol>
    </details>
  )
}

// IdentityForm configures the first identity provider. The verification login also
// enrolls the first administrator.
export function IdentityForm({
  callbackURL,
  identity,
  pending,
  onChange,
  onVerify,
  onLock,
  onError,
}: {
  callbackURL: string
  identity: IdentityInput
  pending: boolean
  onChange: (identity: IdentityInput) => void
  onVerify: () => void
  onLock: () => void
  onError: (message: string) => void
}) {
  const field = (key: keyof IdentityInput) => (event: { target: { value: string } }) => {
    onChange({ ...identity, [key]: event.target.value })
  }
  return (
    <form
      className="settings-form"
      onSubmit={(event) => {
        event.preventDefault()
        onVerify()
      }}
    >
      <p>The account you use for the verification login becomes the first console administrator.</p>
      <CallbackURL url={callbackURL} onError={onError} />
      <Field
        label="Issuer URL"
        help="Use the HTTPS issuer from your provider's OpenID Connect discovery document."
      >
        <input
          type="url"
          value={identity.issuer}
          onChange={field('issuer')}
          placeholder="https://identity.example.com/realms/platform"
          autoComplete="url"
          required
          maxLength={4096}
        />
      </Field>
      <Field label="Client ID">
        <input value={identity.clientId} onChange={field('clientId')} required maxLength={1024} />
      </Field>
      <Field label="Client secret">
        <input
          type="password"
          value={identity.clientSecret ?? ''}
          onChange={field('clientSecret')}
          autoComplete="off"
          required
          maxLength={65536}
        />
      </Field>
      <details className="setup-help">
        <summary>Private CA and group claim</summary>
        <Field
          label="CA certificate"
          help="Optional PEM certificate for a private certificate authority. TLS verification stays enabled."
        >
          <textarea
            rows={4}
            value={identity.ca}
            onChange={field('ca')}
            spellCheck={false}
            maxLength={262144}
          />
        </Field>
        <Field
          label="Groups claim"
          help="Top-level claim containing an array of group names. Group access can be configured after setup."
        >
          <input value={identity.groupsClaim} onChange={field('groupsClaim')} maxLength={1024} />
        </Field>
      </details>
      <KeycloakHelp />
      <div className="form-actions">
        <Button type="button" disabled={pending} onClick={onLock}>
          Lock setup
        </Button>
        <Button type="submit" variant="default" disabled={pending}>
          {pending ? 'Checking provider…' : 'Verify with login'}
        </Button>
      </div>
    </form>
  )
}
