import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { Lock } from 'lucide-react'
import { useSnapshot } from '../api/client'
import { changeAccountPassword, replaceRecoveryCodes } from '../api/local-accounts'
import { Field, FormError } from '../components/form'
import { RecoveryCodes } from '../components/local/recovery-codes'
import { EmptyState, PageHeading } from '../components/page'
import { Button } from '../components/ui/button'
import { useDocumentTitle } from '../components/use-document-title'
import { secretFieldProps } from '../lib/credential'
import { minimumPasswordLength } from '../lib/local-account'

function SecretField({
  label,
  value,
  onChange,
  autoComplete,
  help,
  minLength,
}: {
  label: string
  value: string
  onChange: (value: string) => void
  autoComplete: string
  help?: string
  minLength?: number
}) {
  return (
    <Field label={label} help={help}>
      <input
        type="password"
        value={value}
        onChange={(event) => {
          onChange(event.target.value)
        }}
        minLength={minLength}
        maxLength={1024}
        autoComplete={autoComplete}
        required
      />
    </Field>
  )
}

function CodeField({ value, onChange }: { value: string; onChange: (value: string) => void }) {
  return (
    <Field label="Authenticator code" help="Or one of your recovery codes.">
      <input
        {...secretFieldProps}
        value={value}
        onChange={(event) => {
          onChange(event.target.value)
        }}
        maxLength={64}
        autoComplete="one-time-code"
        required
      />
    </Field>
  )
}

function PasswordForm() {
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [code, setCode] = useState('')
  const [mismatch, setMismatch] = useState(false)
  const change = useMutation({
    gcTime: 0,
    mutationFn: () => changeAccountPassword(current, next, code),
    onSuccess: () => {
      setNext('')
      setConfirmation('')
    },
    onSettled: () => {
      setCurrent('')
      setCode('')
    },
  })
  return (
    <form
      className="settings-form account-form"
      aria-label="Change password"
      onSubmit={(event) => {
        event.preventDefault()
        setMismatch(next !== confirmation)
        if (next === confirmation) change.mutate()
      }}
    >
      <h2>Change password</h2>
      <p className="muted">Your other sessions end. This session stays signed in.</p>
      <FormError error={mismatch ? 'The new passwords do not match.' : change.error} />
      {change.isSuccess && (
        <p role="status" className="save-notice">
          Password changed.
        </p>
      )}
      <SecretField
        label="Current password"
        value={current}
        onChange={setCurrent}
        autoComplete="current-password"
      />
      <SecretField
        label="New password"
        value={next}
        onChange={setNext}
        autoComplete="new-password"
        minLength={minimumPasswordLength}
        help={`At least ${String(minimumPasswordLength)} characters.`}
      />
      <SecretField
        label="Confirm new password"
        value={confirmation}
        onChange={setConfirmation}
        autoComplete="new-password"
      />
      <CodeField value={code} onChange={setCode} />
      <div className="form-actions">
        <Button type="submit" variant="default" disabled={change.isPending}>
          {change.isPending ? 'Changing…' : 'Change password'}
        </Button>
      </div>
    </form>
  )
}

function RecoveryCodesForm() {
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [copyError, setCopyError] = useState('')
  const replace = useMutation({
    gcTime: 0,
    mutationFn: () => replaceRecoveryCodes(password, code),
    onSettled: () => {
      setPassword('')
      setCode('')
    },
  })
  if (replace.data)
    return (
      <div className="account-form">
        <FormError error={copyError} />
        <RecoveryCodes codes={replace.data} onError={setCopyError}>
          {(confirmed) => (
            <Button
              variant="default"
              disabled={!confirmed}
              onClick={() => {
                replace.reset()
              }}
            >
              Done
            </Button>
          )}
        </RecoveryCodes>
      </div>
    )
  return (
    <form
      className="settings-form account-form"
      aria-label="Replace recovery codes"
      onSubmit={(event) => {
        event.preventDefault()
        replace.mutate()
      }}
    >
      <h2>Recovery codes</h2>
      <p className="muted">
        Replace your recovery codes when few remain or you lost them. The previous codes stop
        working.
      </p>
      <FormError error={replace.error} />
      <SecretField
        label="Password"
        value={password}
        onChange={setPassword}
        autoComplete="current-password"
      />
      <CodeField value={code} onChange={setCode} />
      <div className="form-actions">
        <Button type="submit" disabled={replace.isPending}>
          {replace.isPending ? 'Replacing…' : 'Replace recovery codes'}
        </Button>
      </div>
    </form>
  )
}

// AccountPage lets a local account change its password and recovery codes. Identity
// provider accounts manage their credentials at the provider.
export function AccountPage() {
  const { data: snapshot } = useSnapshot()
  useDocumentTitle('Your account')
  if (!snapshot) return null
  if (!snapshot.viewer.localAccount)
    return (
      <div className="page-enter">
        <PageHeading title="Your account" />
        <EmptyState title="Managed by your identity provider" icon={Lock}>
          You signed in with single sign-on. Change your password and multi-factor settings at your
          identity provider.
        </EmptyState>
      </div>
    )
  return (
    <div className="page-enter">
      <PageHeading title="Your account" />
      <p className="muted">
        Signed in as {snapshot.viewer.name} ({snapshot.viewer.subject.replace(/^local:/, '')}), a
        local account.
      </p>
      <PasswordForm />
      <RecoveryCodesForm />
    </div>
  )
}
