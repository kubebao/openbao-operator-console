import { useState } from 'react'
import { useSearch } from '@tanstack/react-router'
import { useMutation } from '@tanstack/react-query'
import { Check } from 'lucide-react'
import { lockSetup, startSetupIdentity, unlockSetup, useSetup, useSetupStep } from '../api/client'
import type { IdentityInput, SetupStatus } from '../api/schema'
import { Brand } from '../components/brand'
import { FormError } from '../components/form'
import { IdentityForm } from '../components/setup/identity-form'
import { LocalAdministratorForm } from '../components/setup/local-administrator-form'
import { UnlockForm } from '../components/setup/unlock-form'
import { RecoveryCodes } from '../components/local/recovery-codes'
import { Button } from '../components/ui/button'
import { useDocumentTitle } from '../components/use-document-title'
import { RefreshBanner } from '../components/workspace/refresh-banner'

const resultMessages = {
  failed:
    'Login verification failed. Check the provider settings and retry. No identity configuration was saved.',
  expired: 'The verification link expired. Start a new login to continue.',
} as const

function SetupProgress({
  state,
  result,
}: {
  state: 'required' | 'unlocked'
  result: '' | keyof typeof resultMessages
}) {
  return (
    <>
      <ol className="setup-progress" aria-label="Setup steps">
        <li aria-current={state === 'required' ? 'step' : undefined}>
          {state === 'unlocked' ? <Check size={16} aria-hidden="true" /> : <span>1</span>}
          Unlock setup
        </li>
        <li aria-current={state === 'unlocked' ? 'step' : undefined}>
          <span>2</span> First administrator
        </li>
      </ol>
      {result && <FormError error={resultMessages[result]} />}
    </>
  )
}

function SetupClosed({ status, onCheck }: { status: SetupStatus; onCheck: () => void }) {
  if (status.state === 'complete')
    return (
      <>
        <p>Continue to connect a Kubernetes cluster.</p>
        <a href="/settings/clusters/new" className="button button-primary">
          Connect your first cluster
        </a>
      </>
    )
  return (
    <>
      <h2>The setup window expired</h2>
      <p>Generate a new setup token and restart the console. No configuration was saved.</p>
      <Button onClick={onCheck}>Check again</Button>
    </>
  )
}

// SetupLoading reports the first load, or a failed refresh that keeps the last known status.
function SetupLoading({
  loaded,
  failed,
  retrying,
  onRetry,
}: {
  loaded: boolean
  failed: boolean
  retrying: boolean
  onRetry: () => void
}) {
  if (!loaded) return <p role="status">Loading setup…</p>
  return failed ? <RefreshBanner retrying={retrying} onRetry={onRetry} /> : null
}

type Method = 'identity' | 'local'

const methods: readonly { value: Method; title: string; detail: string }[] = [
  {
    value: 'identity',
    title: 'Identity provider',
    detail:
      'Recommended. Sign in through OpenID Connect, with the multi-factor rules of your provider.',
  },
  {
    value: 'local',
    title: 'Local administrator',
    detail:
      'A password and an authenticator app, stored by the console. Suits evaluation and small teams.',
  },
]

function MethodChoice({
  method,
  onChange,
}: {
  method: Method
  onChange: (method: Method) => void
}) {
  return (
    <fieldset className="setup-methods">
      <legend>How does the first administrator sign in?</legend>
      {methods.map((choice) => (
        <label key={choice.value} className="setup-method">
          <input
            type="radio"
            name="setup-method"
            value={choice.value}
            checked={method === choice.value}
            onChange={() => {
              onChange(choice.value)
            }}
          />
          <span>
            <strong>{choice.title}</strong>
            <small>{choice.detail}</small>
          </span>
        </label>
      ))}
    </fieldset>
  )
}

// SetupRecovery shows the recovery codes of the new local administrator once.
function SetupRecovery({
  codes,
  onError,
}: {
  codes: readonly string[]
  onError: (message: string) => void
}) {
  return (
    <RecoveryCodes codes={codes} onError={onError}>
      {(confirmed) =>
        confirmed ? (
          <a href="/settings/clusters/new" className="button button-primary">
            Connect your first cluster
          </a>
        ) : (
          <button type="button" className="button button-primary" disabled>
            Connect your first cluster
          </button>
        )
      }
    </RecoveryCodes>
  )
}

const emptyIdentity: IdentityInput = {
  issuer: '',
  clientId: '',
  clientSecret: '',
  ca: '',
  groupsClaim: 'groups',
}

// SetupState reports a closed setup or the progress of an open one.
function SetupState({
  status,
  result,
  onCheck,
}: {
  status: SetupStatus | undefined
  result: '' | keyof typeof resultMessages
  onCheck: () => void
}) {
  if (status?.state === 'complete' || status?.state === 'expired')
    return <SetupClosed status={status} onCheck={onCheck} />
  if (status?.state === 'required' || status?.state === 'unlocked')
    return <SetupProgress state={status.state} result={result} />
  return null
}

// UnlockedSetup lets the setup holder choose how the first administrator signs in.
function UnlockedSetup({
  callbackURL,
  identity,
  pending,
  onIdentity,
  onVerify,
  onCreated,
  onLock,
  onError,
}: {
  callbackURL: string
  identity: IdentityInput
  pending: boolean
  onIdentity: (identity: IdentityInput) => void
  onVerify: () => void
  onCreated: (codes: readonly string[]) => void
  onLock: () => void
  onError: (error: unknown) => void
}) {
  const [method, setMethod] = useState<Method>('identity')
  return (
    <>
      <MethodChoice
        method={method}
        onChange={(next) => {
          onError(null)
          setMethod(next)
        }}
      />
      {method === 'local' ? (
        <LocalAdministratorForm
          pending={pending}
          onCreated={onCreated}
          onError={onError}
          onLock={onLock}
        />
      ) : (
        <IdentityForm
          callbackURL={callbackURL}
          identity={identity}
          pending={pending}
          onChange={onIdentity}
          onError={onError}
          onVerify={onVerify}
          onLock={onLock}
        />
      )}
    </>
  )
}

// SetupPage unlocks first-run setup with the installation token and creates the first
// administrator: through a verified identity provider login, or as a local account.
// Secrets never leave component state.
export function SetupPage() {
  const query = useSetup()
  const { result } = useSearch({ from: '/setup' })
  const [identity, setIdentity] = useState(emptyIdentity)
  const [recoveryCodes, setRecoveryCodes] = useState<readonly string[] | null>(null)
  const [error, setError] = useState<unknown>(null)
  const refetch = () => {
    void query.refetch()
  }
  const unlock = useSetupStep(unlockSetup)
  const lock = useSetupStep(lockSetup)
  const verify = useMutation({
    gcTime: 0,
    mutationFn: (input: IdentityInput) => startSetupIdentity(input),
    onSuccess: (url) => {
      setIdentity((current) => ({ ...current, clientSecret: '' }))
      window.location.assign(url)
    },
  })
  // A successful verification navigates away, so the form stays disabled.
  const pending = unlock.isPending || lock.isPending || verify.isPending || verify.isSuccess
  const fail = (failure: unknown) => {
    setError(failure)
    refetch()
  }
  const status = query.data
  const complete = status?.state === 'complete' || recoveryCodes !== null
  useDocumentTitle(complete ? 'Setup complete' : 'Set up your console')
  return (
    <main className="setup-page">
      <div className="setup-panel">
        <div className="entry-brand">
          <Brand />
        </div>
        <header className="setup-header">
          <h1>{complete ? 'Setup complete' : 'Set up your console'}</h1>
        </header>
        <FormError error={error} />
        {recoveryCodes ? (
          <SetupRecovery codes={recoveryCodes} onError={setError} />
        ) : (
          <>
            <SetupLoading
              loaded={status !== undefined}
              failed={query.isError}
              retrying={query.isFetching}
              onRetry={refetch}
            />
            <SetupState status={status} result={result} onCheck={refetch} />
            {status?.state === 'required' && (
              <UnlockForm
                pending={pending}
                onUnlock={(token) => {
                  setError(null)
                  unlock.mutate(token, { onError: fail })
                }}
              />
            )}
            {status?.state === 'unlocked' && (
              <UnlockedSetup
                callbackURL={status.callbackUrl}
                identity={identity}
                pending={pending}
                onIdentity={setIdentity}
                onVerify={() => {
                  setError(null)
                  verify.mutate(identity, { onError: fail })
                }}
                onCreated={(codes) => {
                  setRecoveryCodes(codes)
                  refetch()
                }}
                onLock={() => {
                  setError(null)
                  lock.mutate(undefined, {
                    onSuccess: () => {
                      setIdentity((current) => ({ ...current, clientSecret: '', ca: '' }))
                    },
                    onError: setError,
                  })
                }}
                onError={setError}
              />
            )}
          </>
        )}
      </div>
    </main>
  )
}
