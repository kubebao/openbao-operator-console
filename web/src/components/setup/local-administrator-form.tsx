import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { createSetupAdministrator, startSetupAuthenticator } from '../../api/client'
import { Field } from '../form'
import { localUsernamePattern, minimumPasswordLength } from '../../lib/local-account'
import { Button } from '../ui/button'
import { AuthenticatorStep } from './authenticator-step'

// LocalAdministratorForm creates the first local administrator with a password and an
// authenticator app. Passwords stay in component state and are cleared after use.
export function LocalAdministratorForm({
  pending,
  onCreated,
  onError,
  onLock,
}: {
  pending: boolean
  onCreated: (recoveryCodes: readonly string[]) => void
  onError: (error: unknown) => void
  onLock: () => void
}) {
  const [username, setUsername] = useState('')
  const [name, setName] = useState('')
  const [password, setPassword] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [code, setCode] = useState('')
  const authenticator = useMutation({ gcTime: 0, mutationFn: startSetupAuthenticator })
  const create = useMutation({
    gcTime: 0,
    mutationFn: createSetupAdministrator,
    onSuccess: (codes) => {
      setPassword('')
      setConfirmation('')
      onCreated(codes)
    },
  })
  const busy = pending || authenticator.isPending || create.isPending
  return (
    <form
      className="settings-form"
      onSubmit={(event) => {
        event.preventDefault()
        onError(null)
        if (password !== confirmation) {
          onError('The passwords do not match.')
          return
        }
        create.mutate({ username, name, password, code }, { onError })
        setCode('')
      }}
    >
      <p className="onboarding-help">
        A local administrator signs in with a password and an authenticator app. You can add an
        identity provider later in Settings; it remains the recommended sign-in for teams.
      </p>
      <Field label="Username" help="Lowercase letters, digits, dots, hyphens, or underscores.">
        <input
          value={username}
          onChange={(event) => {
            setUsername(event.target.value.toLowerCase())
          }}
          pattern={localUsernamePattern}
          maxLength={64}
          autoComplete="username"
          {...{ spellCheck: false, autoCapitalize: 'off', autoCorrect: 'off' }}
          required
        />
      </Field>
      <Field label="Display name" help="Optional. Shown in the console and the change history.">
        <input
          value={name}
          onChange={(event) => {
            setName(event.target.value)
          }}
          maxLength={256}
          autoComplete="name"
        />
      </Field>
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
          {...{ spellCheck: false }}
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
        enrollment={authenticator.data}
        preparing={authenticator.isPending}
        disabled={busy}
        code={code}
        onCode={setCode}
        onPrepare={() => {
          onError(null)
          authenticator.mutate(undefined, { onError })
        }}
      />
      <div className="form-actions">
        <Button type="button" disabled={busy} onClick={onLock}>
          Lock setup
        </Button>
        <Button type="submit" variant="default" disabled={busy || !authenticator.data}>
          {create.isPending ? 'Creating administrator…' : 'Create administrator'}
        </Button>
      </div>
    </form>
  )
}
