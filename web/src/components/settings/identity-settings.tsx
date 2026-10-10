import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { trialIdentity } from '../../api/client'
import type { IdentityInput, Settings } from '../../api/schema'
import { Field, FormError } from '../form'
import { Button } from '../ui/button'

// IdentitySettings activates a candidate provider only after an enrolled administrator
// signs in through it.
export function IdentitySettings({ settings }: { settings: Settings }) {
  const [identity, setIdentity] = useState<IdentityInput>({
    issuer: settings.identity.issuer,
    clientId: settings.identity.clientId,
    ca: settings.identity.ca,
    groupsClaim: settings.identity.groupsClaim,
  })
  const [secret, setSecret] = useState('')
  const trial = useMutation({
    mutationFn: (input: IdentityInput) => trialIdentity(settings.revision, input),
    onSuccess: (url) => {
      window.location.assign(url)
    },
  })
  // A successful trial navigates away, so the form stays disabled.
  const pending = trial.isPending || trial.isSuccess
  const field = (key: keyof IdentityInput) => (value: string) => {
    setIdentity({ ...identity, [key]: value })
  }
  return (
    <form
      className="settings-form"
      onSubmit={(event) => {
        event.preventDefault()
        trial.mutate(secret ? { ...identity, clientSecret: secret } : identity)
      }}
    >
      <h2>OpenID Connect</h2>
      <Field label="Issuer">
        <Input type="url" required value={identity.issuer} onChange={field('issuer')} />
      </Field>
      <Field label="Client ID">
        <Input required value={identity.clientId} onChange={field('clientId')} />
      </Field>
      <Field
        label="Client secret"
        help={
          settings.identity.secretConfigured
            ? 'Leave empty to keep the secret for this issuer and client.'
            : 'Supply the confidential client secret.'
        }
      >
        <Input type="password" autoComplete="new-password" value={secret} onChange={setSecret} />
      </Field>
      <Field label="Groups claim" help="Top-level token claim containing an array of group names.">
        <Input value={identity.groupsClaim} onChange={field('groupsClaim')} />
      </Field>
      <Field label="CA certificate" help="Leave empty to use system trust.">
        <textarea
          rows={4}
          value={identity.ca}
          onChange={(event) => {
            field('ca')(event.target.value)
          }}
        />
      </Field>
      <p className="muted">
        Sign in through the candidate provider as an enrolled administrator. Activation follows a
        successful login and ends existing console sessions.
      </p>
      <FormError error={trial.error} />
      <Button type="submit" variant="default" disabled={pending}>
        {pending ? 'Checking…' : 'Verify login and activate'}
      </Button>
    </form>
  )
}

function Input({
  value,
  onChange,
  type,
  required = false,
  autoComplete,
}: {
  value: string
  onChange: (value: string) => void
  type?: string
  required?: boolean
  autoComplete?: string
}) {
  return (
    <input
      type={type}
      required={required}
      autoComplete={autoComplete}
      value={value}
      onChange={(event) => {
        onChange(event.target.value)
      }}
    />
  )
}
