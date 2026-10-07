import { useState } from 'react'
import { useSearch } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { Check, Copy, LockKeyhole } from 'lucide-react'
import { useSetup, unlockSetup, startSetupIdentity, lockSetup } from '../api/client'
import type { IdentityInput } from '../api/schema'
import { Brand } from '../components/brand'
import { Field } from '../components/settings-editor'
import { Button } from '../components/ui/button'
import { formError } from '../lib/errors'

export function SetupPage() {
  const query = useSetup()
  const client = useQueryClient()
  const { result } = useSearch({ from: '/setup' })
  const [token, setToken] = useState('')
  const [identity, setIdentity] = useState<IdentityInput>({
    issuer: '',
    clientId: '',
    clientSecret: '',
    ca: '',
    groupsClaim: 'groups',
  })
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)
  const [copied, setCopied] = useState(false)

  async function unlock() {
    setError('')
    setPending(true)
    try {
      const status = await unlockSetup(token)
      setToken('')
      client.setQueryData(['setup'], status)
    } catch (failure) {
      setToken('')
      setError(formError(failure))
      await query.refetch()
    } finally {
      setPending(false)
    }
  }

  async function verify() {
    setError('')
    setPending(true)
    try {
      const url = await startSetupIdentity(identity)
      setIdentity({ ...identity, clientSecret: '' })
      window.location.assign(url)
    } catch (failure) {
      setError(formError(failure))
      await query.refetch()
      setPending(false)
    }
  }

  async function close() {
    setError('')
    setPending(true)
    try {
      const status = await lockSetup()
      setIdentity({ ...identity, clientSecret: '', ca: '' })
      client.setQueryData(['setup'], status)
    } catch (failure) {
      setError(formError(failure))
    } finally {
      setPending(false)
    }
  }

  const status = query.data
  return (
    <main className="setup-page">
      <div className="setup-panel">
        <div className="entry-brand">
          <Brand />
        </div>
        <header className="setup-header">
          <h1>{status?.state === 'complete' ? 'Setup complete' : 'Set up your console'}</h1>
        </header>
        {!status && <p role="status">Loading setup…</p>}
        {status?.state === 'complete' && (
          <>
            <p>Continue to connect a Kubernetes cluster.</p>
            <a href="/settings/clusters/new" className="button button-primary">
              Connect your first cluster
            </a>
          </>
        )}
        {status?.state === 'expired' && (
          <>
            <h2>The setup window expired</h2>
            <p>Generate a new setup token and restart the console. No configuration was saved.</p>
            <Button
              onClick={() => {
                void query.refetch()
              }}
            >
              Check again
            </Button>
          </>
        )}
        {(status?.state === 'required' || status?.state === 'unlocked') && (
          <>
            <ol className="setup-progress" aria-label="Setup steps">
              <li aria-current={status.state === 'required' ? 'step' : undefined}>
                {status.state === 'unlocked' ? (
                  <Check size={16} aria-hidden="true" />
                ) : (
                  <span>1</span>
                )}
                Unlock setup
              </li>
              <li aria-current={status.state === 'unlocked' ? 'step' : undefined}>
                <span>2</span> Identity provider
              </li>
            </ol>
            {result === 'failed' && (
              <p className="form-error" role="alert">
                Login verification failed. Check the provider settings and retry. No identity
                configuration was saved.
              </p>
            )}
            {result === 'expired' && (
              <p className="form-error" role="alert">
                The verification link expired. Start a new login to continue.
              </p>
            )}
          </>
        )}
        {error && (
          <p className="form-error" role="alert">
            {error}
          </p>
        )}
        {status?.state === 'required' && (
          <form
            className="settings-form"
            onSubmit={(event) => {
              event.preventDefault()
              void unlock()
            }}
          >
            <p>Enter the private setup token supplied with this installation.</p>
            <Field label="Setup token">
              <input
                type="password"
                value={token}
                onChange={(event) => {
                  setToken(event.target.value)
                }}
                autoComplete="off"
                spellCheck={false}
                required
                maxLength={256}
                autoFocus
              />
            </Field>
            <div className="form-actions">
              <Button type="submit" variant="default" disabled={pending}>
                <LockKeyhole size={16} aria-hidden="true" />
                {pending ? 'Opening setup…' : 'Unlock setup'}
              </Button>
            </div>
          </form>
        )}
        {status?.state === 'unlocked' && (
          <form
            className="settings-form"
            onSubmit={(event) => {
              event.preventDefault()
              void verify()
            }}
          >
            <p>
              The account you use for the verification login becomes the first console
              administrator.
            </p>
            <div className="setup-callback">
              <Field
                label="Callback URL"
                help="Register this exact URL in your identity provider client."
              >
                <input value={status.callbackUrl} readOnly aria-label="Callback URL" />
              </Field>
              <Button
                type="button"
                size="icon"
                aria-label={copied ? 'Callback URL copied' : 'Copy callback URL'}
                onClick={() => {
                  void navigator.clipboard
                    .writeText(status.callbackUrl)
                    .then(() => {
                      setCopied(true)
                    })
                    .catch(() => {
                      setError('Select the callback URL and copy it manually.')
                    })
                }}
              >
                {copied ? <Check size={16} /> : <Copy size={16} />}
              </Button>
            </div>
            <Field
              label="Issuer URL"
              help="Use the HTTPS issuer from your provider's OpenID Connect discovery document."
            >
              <input
                type="url"
                value={identity.issuer}
                onChange={(event) => {
                  setIdentity({ ...identity, issuer: event.target.value })
                }}
                placeholder="https://identity.example.com/realms/platform"
                autoComplete="url"
                required
                maxLength={4096}
              />
            </Field>
            <Field label="Client ID">
              <input
                value={identity.clientId}
                onChange={(event) => {
                  setIdentity({ ...identity, clientId: event.target.value })
                }}
                required
                maxLength={1024}
              />
            </Field>
            <Field label="Client secret">
              <input
                type="password"
                value={identity.clientSecret ?? ''}
                onChange={(event) => {
                  setIdentity({ ...identity, clientSecret: event.target.value })
                }}
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
                  onChange={(event) => {
                    setIdentity({ ...identity, ca: event.target.value })
                  }}
                  spellCheck={false}
                  maxLength={262144}
                />
              </Field>
              <Field
                label="Groups claim"
                help="Top-level claim containing an array of group names. Group access can be configured after setup."
              >
                <input
                  value={identity.groupsClaim}
                  onChange={(event) => {
                    setIdentity({ ...identity, groupsClaim: event.target.value })
                  }}
                  maxLength={1024}
                />
              </Field>
            </details>
            <details className="setup-help">
              <summary>Configure Keycloak</summary>
              <ol>
                <li>
                  Create an OpenID Connect client with client authentication and the standard
                  authorization code flow enabled.
                </li>
                <li>
                  Register the callback URL above as a valid redirect URI. Use PKCE with S256.
                </li>
                <li>Copy the client secret from Credentials. Use the realm URL as the issuer.</li>
              </ol>
            </details>
            <div className="form-actions">
              <Button
                type="button"
                disabled={pending}
                onClick={() => {
                  void close()
                }}
              >
                Lock setup
              </Button>
              <Button type="submit" variant="default" disabled={pending}>
                {pending ? 'Checking provider…' : 'Verify with login'}
              </Button>
            </div>
          </form>
        )}
      </div>
    </main>
  )
}
