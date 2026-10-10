import { useState } from 'react'
import { useSearch } from '@tanstack/react-router'
import { useMutation } from '@tanstack/react-query'
import { Check } from 'lucide-react'
import { lockSetup, startSetupIdentity, unlockSetup, useSetup, useSetupStep } from '../api/client'
import type { IdentityInput, SetupStatus } from '../api/schema'
import { Brand } from '../components/brand'
import { FormError } from '../components/form'
import { IdentityForm } from '../components/setup/identity-form'
import { UnlockForm } from '../components/setup/unlock-form'
import { Button } from '../components/ui/button'

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
          <span>2</span> Identity provider
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

const emptyIdentity: IdentityInput = {
  issuer: '',
  clientId: '',
  clientSecret: '',
  ca: '',
  groupsClaim: 'groups',
}

// SetupPage unlocks first-run setup with the installation token and verifies the first
// identity provider through a real login. Secrets never leave component state.
export function SetupPage() {
  const query = useSetup()
  const { result } = useSearch({ from: '/setup' })
  const [identity, setIdentity] = useState(emptyIdentity)
  const [error, setError] = useState<unknown>(null)
  const refetch = () => {
    void query.refetch()
  }
  const unlock = useSetupStep(unlockSetup)
  const lock = useSetupStep(lockSetup)
  const verify = useMutation({
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
        {(status?.state === 'complete' || status?.state === 'expired') && (
          <SetupClosed status={status} onCheck={refetch} />
        )}
        {(status?.state === 'required' || status?.state === 'unlocked') && (
          <SetupProgress state={status.state} result={result} />
        )}
        <FormError error={error} />
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
          <IdentityForm
            callbackURL={status.callbackUrl}
            identity={identity}
            pending={pending}
            onChange={setIdentity}
            onError={setError}
            onVerify={() => {
              setError(null)
              verify.mutate(identity, { onError: fail })
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
          />
        )}
      </div>
    </main>
  )
}
