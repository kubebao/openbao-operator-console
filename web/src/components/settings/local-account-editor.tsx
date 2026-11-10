import { useState } from 'react'
import type { Configuration } from '../../api/schema'
import type { LocalAccount } from '../../api/sign-in'
import type { LocalAccountFields } from '../../api/local-accounts'
import { ScopePicker } from '../access-editor'
import { Field, FormError } from '../form'
import { SettingsEditor } from '../settings-editor'
import { Button } from '../ui/button'
import { localUsernamePattern } from '../../lib/local-account'

// LocalAccountEditor adds an account or edits its name and access. The username of an
// existing account cannot change.
export function LocalAccountEditor({
  account,
  configuration,
  pending,
  error,
  onSave,
  onClose,
}: {
  account: LocalAccount | null
  configuration: Configuration
  pending: boolean
  error: unknown
  onSave: (username: string, fields: LocalAccountFields) => void
  onClose: () => void
}) {
  const [username, setUsername] = useState(account?.username ?? '')
  const [fields, setFields] = useState<LocalAccountFields>({
    name: account?.name ?? '',
    admin: account?.admin ?? false,
    scopes: account?.scopes ?? [],
  })
  let submitLabel = account ? 'Save account' : 'Create account'
  if (pending) submitLabel = 'Saving…'
  return (
    <SettingsEditor
      title={account ? `Edit ${account.username}` : 'Add local account'}
      description={
        account
          ? undefined
          : 'The account owner sets a password and an authenticator app through a one-time link.'
      }
      onClose={onClose}
    >
      <form
        onSubmit={(event) => {
          event.preventDefault()
          onSave(username, fields)
        }}
      >
        <Field label="Username" help="Lowercase letters, digits, dots, hyphens, or underscores.">
          <input
            required
            disabled={account !== null}
            value={username}
            pattern={localUsernamePattern}
            maxLength={64}
            {...{ spellCheck: false, autoCapitalize: 'off', autoCorrect: 'off' }}
            onChange={(event) => {
              setUsername(event.target.value.toLowerCase())
            }}
          />
        </Field>
        <Field label="Display name">
          <input
            value={fields.name}
            maxLength={256}
            onChange={(event) => {
              setFields({ ...fields, name: event.target.value })
            }}
          />
        </Field>
        <label className="checkbox-field">
          <input
            type="checkbox"
            checked={fields.admin}
            onChange={(event) => {
              setFields({ ...fields, admin: event.target.checked })
            }}
          />
          Console administrator
        </label>
        {fields.admin ? (
          <p className="muted">
            Administrators can manage settings and view all enrolled namespaces.
          </p>
        ) : (
          <ScopePicker
            configuration={configuration}
            scopes={fields.scopes}
            onChange={(scopes) => {
              setFields({ ...fields, scopes })
            }}
          />
        )}
        <FormError error={error} />
        <div className="editor-actions">
          <Button type="button" variant="ghost" onClick={onClose} disabled={pending}>
            Cancel
          </Button>
          <Button type="submit" variant="default" disabled={pending}>
            {submitLabel}
          </Button>
        </div>
      </form>
    </SettingsEditor>
  )
}
