import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { KeyRound } from 'lucide-react'
import { signInLocal } from '../../api/client'
import type { LocalSignInResult } from '../../api/sign-in'
import { Field, FormError } from '../form'
import { secretFieldProps } from '../../lib/credential'
import { Button } from '../ui/button'

// RecoveryNotice tells the viewer that a recovery code was used and how many remain.
function RecoveryNotice({
  result,
  onContinue,
}: {
  result: LocalSignInResult
  onContinue: () => void
}) {
  return (
    <div className="local-sign-in" role="status">
      <p>
        You signed in with a recovery code. {String(result.recoveryCodesLeft)} of 10 codes remain.
        {result.recoveryCodesLeft <= 3 && ' Ask an administrator to reset your authenticator soon.'}
      </p>
      <Button type="button" variant="default" className="sign-in-button" onClick={onContinue}>
        Continue
      </Button>
    </div>
  )
}

// LocalSignInForm signs in with a username, password, and an authenticator or recovery
// code. Secrets stay in component state and are cleared after each attempt.
export function LocalSignInForm({ returnTo, primary }: { returnTo: string; primary: boolean }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const proceed = () => {
    window.location.assign(returnTo)
  }
  const signIn = useMutation({
    gcTime: 0,
    mutationFn: () => signInLocal(username, password, code),
    onSuccess: (result) => {
      if (!result.recoveryCodeUsed) proceed()
    },
    onSettled: () => {
      setPassword('')
      setCode('')
    },
  })
  if (signIn.data?.recoveryCodeUsed)
    return <RecoveryNotice result={signIn.data} onContinue={proceed} />
  return (
    <form
      className="local-sign-in settings-form"
      aria-label="Sign in with a local account"
      onSubmit={(event) => {
        event.preventDefault()
        signIn.mutate()
      }}
    >
      <FormError error={signIn.error} />
      <Field label="Username">
        <input
          value={username}
          onChange={(event) => {
            setUsername(event.target.value)
          }}
          maxLength={64}
          autoComplete="username"
          {...{ spellCheck: false, autoCapitalize: 'off', autoCorrect: 'off' }}
          required
        />
      </Field>
      <Field label="Password">
        <input
          type="password"
          value={password}
          onChange={(event) => {
            setPassword(event.target.value)
          }}
          maxLength={1024}
          autoComplete="current-password"
          required
        />
      </Field>
      <Field label="Authenticator code" help="Or one of your recovery codes.">
        <input
          {...secretFieldProps}
          value={code}
          onChange={(event) => {
            setCode(event.target.value)
          }}
          maxLength={64}
          autoComplete="one-time-code"
          required
        />
      </Field>
      <Button
        type="submit"
        variant={primary ? 'default' : 'outline'}
        className="sign-in-button"
        disabled={signIn.isPending || signIn.isSuccess}
      >
        <KeyRound size={16} aria-hidden="true" />
        {signIn.isPending ? 'Signing in…' : 'Sign in'}
      </Button>
    </form>
  )
}
