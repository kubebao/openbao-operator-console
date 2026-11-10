import { useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { completeEnrollment, openEnrollment } from '../api/local-accounts'
import type { EnrollmentStart } from '../api/sign-in'
import { Brand } from '../components/brand'
import { Field, FormError } from '../components/form'
import { RecoveryCodes } from '../components/local/recovery-codes'
import { AuthenticatorStep } from '../components/setup/authenticator-step'
import { Button } from '../components/ui/button'
import { useDocumentTitle } from '../components/use-document-title'
import { absolute } from '../lib/format'
import { minimumPasswordLength } from '../lib/local-account'

// The link carries its token in the fragment, which browsers never send to the server.
function linkToken(): string {
  return new URLSearchParams(window.location.hash.slice(1)).get('token') ?? ''
}

function EnrollmentForm({
  token,
  start,
  onEnrolled,
}: {
  token: string
  start: EnrollmentStart
  onEnrolled: (codes: readonly string[]) => void
}) {
  const [password, setPassword] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [code, setCode] = useState('')
  const [mismatch, setMismatch] = useState(false)
  const complete = useMutation({
    gcTime: 0,
    mutationFn: () => completeEnrollment(token, password, code),
    onSuccess: (codes) => {
      setPassword('')
      setConfirmation('')
      onEnrolled(codes)
    },
    onSettled: () => {
      setCode('')
    },
  })
  return (
    <form
      className="settings-form"
      onSubmit={(event) => {
        event.preventDefault()
        setMismatch(password !== confirmation)
        if (password === confirmation) complete.mutate()
      }}
    >
      <p className="onboarding-help">
        Set up the local account <strong>{start.username}</strong>
        {start.name && ` for ${start.name}`}. This link works once and expires{' '}
        {absolute(start.expiresAt)}.
      </p>
      <FormError error={mismatch ? 'The passwords do not match.' : complete.error} />
      <Field
        label="Password"
        help={`At least ${String(minimumPasswordLength)} characters. A passphrase of several words works well.`}
      >
        <input
          type="password"
          value={password}
          onChange={(event) => {
            setPassword(event.target.value)
          }}
          minLength={minimumPasswordLength}
          maxLength={1024}
          autoComplete="new-password"
          required
        />
      </Field>
      <Field label="Confirm password">
        <input
          type="password"
          value={confirmation}
          onChange={(event) => {
            setConfirmation(event.target.value)
          }}
          maxLength={1024}
          autoComplete="new-password"
          required
        />
      </Field>
      <h2>Authenticator app</h2>
      <AuthenticatorStep
        enrollment={start.authenticator}
        preparing={false}
        disabled={complete.isPending}
        code={code}
        onCode={setCode}
        onPrepare={() => undefined}
      />
      <div className="form-actions">
        <Button type="submit" variant="default" disabled={complete.isPending}>
          {complete.isPending ? 'Setting up…' : 'Set up account'}
        </Button>
      </div>
    </form>
  )
}

// EnrollPage completes a local account from a one-time link that an administrator issued.
export function EnrollPage() {
  const [token] = useState(linkToken)
  const [codes, setCodes] = useState<readonly string[] | null>(null)
  const [copyError, setCopyError] = useState('')
  const start = useQuery({
    queryKey: ['enrollment', token],
    queryFn: () => openEnrollment(token),
    enabled: token !== '' && codes === null,
    retry: false,
    gcTime: 0,
    staleTime: Number.POSITIVE_INFINITY,
    refetchOnWindowFocus: false,
  })
  useDocumentTitle(codes ? 'Account ready' : 'Set up your account')
  return (
    <main className="setup-page">
      <div className="setup-panel">
        <div className="entry-brand">
          <Brand />
        </div>
        <header className="setup-header">
          <h1>{codes ? 'Account ready' : 'Set up your account'}</h1>
        </header>
        <FormError error={copyError} />
        {codes && (
          <RecoveryCodes codes={codes} onError={setCopyError}>
            {(confirmed) =>
              confirmed ? (
                <a href="/" className="button button-primary">
                  Continue to the console
                </a>
              ) : (
                <button type="button" className="button button-primary" disabled>
                  Continue to the console
                </button>
              )
            }
          </RecoveryCodes>
        )}
        {!codes && token === '' && (
          <p role="alert" className="form-error">
            Open the complete enrollment link that your administrator shared.
          </p>
        )}
        {!codes && start.isPending && token !== '' && <p role="status">Opening the link…</p>}
        {!codes && start.isError && <FormError error={start.error} />}
        {!codes && start.data && (
          <EnrollmentForm token={token} start={start.data} onEnrolled={setCodes} />
        )}
      </div>
    </main>
  )
}
