import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { trialIdentity } from '../../api/client'
import type { AccessRules, IdentityInput, Settings } from '../../api/schema'
import { issuerChanges } from '../../lib/identity'
import { Field, FormError } from '../form'
import { secretFieldProps } from '../../lib/credential'
import { Button } from '../ui/button'
import { AccessRulesDialog } from './access-rules-dialog'

interface Trial {
  identity: IdentityInput
  accessRules: AccessRules | null
}

// IdentitySettings activates a candidate provider only after an administrator signs in
// through it. An issuer change first asks what happens to existing user and group access.
export function IdentitySettings({ settings }: { settings: Settings }) {
  const [identity, setIdentity] = useState<IdentityInput>({
    issuer: settings.identity.issuer,
    clientId: settings.identity.clientId,
    ca: settings.identity.ca,
    groupsClaim: settings.identity.groupsClaim,
  })
  const [secret, setSecret] = useState('')
  const [confirming, setConfirming] = useState(false)
  const trial = useMutation({
    gcTime: 0,
    mutationFn: (input: Trial) =>
      trialIdentity(settings.revision, input.identity, input.accessRules),
    onSuccess: (url) => {
      window.location.assign(url)
    },
  })
  // A successful trial navigates away, so the form stays disabled.
  const pending = trial.isPending || trial.isSuccess
  const field = (key: keyof IdentityInput) => (value: string) => {
    setIdentity({ ...identity, [key]: value })
  }
  const candidate = secret ? { ...identity, clientSecret: secret } : identity
  const issuerChanged = issuerChanges(settings.identity.issuer, identity.issuer)
  const submitLabel = issuerChanged ? 'Review access changes' : 'Verify login and activate'
  // The dialog renders outside the form: portal events still bubble through the React tree.
  const form = (
    <form
      className="settings-form"
      onSubmit={(event) => {
        event.preventDefault()
        trial.reset()
        if (issuerChanged) setConfirming(true)
        else trial.mutate({ identity: candidate, accessRules: null })
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
        <input
          type="password"
          {...secretFieldProps}
          autoComplete="new-password"
          value={secret}
          onChange={(event) => {
            setSecret(event.target.value)
          }}
        />
      </Field>
      <Field label="Groups claim" help="Top-level token claim containing an array of group names.">
        <Input value={identity.groupsClaim} onChange={field('groupsClaim')} />
      </Field>
      <Field label="CA certificate" help="Leave empty to use system trust.">
        <textarea
          rows={4}
          {...secretFieldProps}
          value={identity.ca}
          onChange={(event) => {
            field('ca')(event.target.value)
          }}
        />
      </Field>
      <p className="muted">
        {issuerChanged
          ? 'The issuer changes. Before you sign in, choose whether existing user and group access carries over to the new issuer or is removed.'
          : 'Sign in through the candidate provider as an enrolled administrator. Existing user and group access stays with this issuer.'}{' '}
        Activation follows a successful login and ends existing console sessions.
      </p>
      {!confirming && <FormError error={trial.error} />}
      <Button type="submit" variant="default" disabled={pending}>
        {pending ? 'Checking…' : submitLabel}
      </Button>
    </form>
  )
  return (
    <>
      {form}
      {confirming && (
        <AccessRulesDialog
          activeIssuer={settings.identity.issuer}
          candidateIssuer={identity.issuer}
          configuration={settings.configuration}
          pending={pending}
          error={trial.error}
          onConfirm={(accessRules) => {
            trial.mutate({ identity: candidate, accessRules })
          }}
          onClose={() => {
            setConfirming(false)
          }}
        />
      )}
    </>
  )
}

function Input({
  value,
  onChange,
  type,
  required = false,
}: {
  value: string
  onChange: (value: string) => void
  type?: string
  required?: boolean
}) {
  return (
    <input
      type={type}
      required={required}
      value={value}
      onChange={(event) => {
        onChange(event.target.value)
      }}
    />
  )
}
