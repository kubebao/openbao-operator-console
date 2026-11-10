import { formError } from '../lib/errors'
import { useState } from 'react'
import type { Configuration, Scope } from '../api/schema'
import type { AccessGrant, EditMode, GrantKind } from '../lib/settings'
import { SettingsEditor } from './settings-editor'
import { Field } from './form'
import { Button } from './ui/button'

export function AccessEditor({
  initial,
  kind,
  mode,
  configuration,
  onSave,
  onClose,
}: {
  initial: AccessGrant
  kind: GrantKind
  mode: EditMode
  configuration: Configuration
  onSave: (grant: AccessGrant) => Promise<void>
  onClose: () => void
}) {
  const [grant, setGrant] = useState(initial)
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)
  return (
    <SettingsEditor title={`${mode === 'edit' ? 'Edit' : 'Add'} ${kind}`} onClose={onClose}>
      <form
        onSubmit={(event) => {
          event.preventDefault()
          setPending(true)
          setError('')
          void onSave(grant)
            .then(onClose)
            .catch((error: unknown) => {
              setError(formError(error))
            })
            .finally(() => {
              setPending(false)
            })
        }}
      >
        <Field
          label={kind === 'user' ? 'OIDC subject' : 'OIDC group'}
          help={
            kind === 'user'
              ? 'The stable subject from the identity provider.'
              : 'Exact group value in the configured signed token claim.'
          }
        >
          <input
            required
            disabled={mode === 'edit'}
            value={grant.key}
            onChange={(event) => {
              setGrant({ ...grant, key: event.target.value })
            }}
          />
        </Field>
        {kind === 'user' && (
          <Field label="Display name">
            <input
              required
              value={grant.name}
              onChange={(event) => {
                setGrant({ ...grant, name: event.target.value })
              }}
            />
          </Field>
        )}
        <label className="checkbox-field">
          <input
            type="checkbox"
            checked={grant.admin}
            onChange={(event) => {
              setGrant({ ...grant, admin: event.target.checked })
            }}
          />
          Console administrator
        </label>
        {grant.admin ? (
          <p className="muted">
            Administrators can manage settings and view all enrolled namespaces.
          </p>
        ) : (
          <ScopePicker
            configuration={configuration}
            scopes={grant.scopes}
            onChange={(scopes) => {
              setGrant({ ...grant, scopes })
            }}
          />
        )}
        {error && (
          <p role="alert" className="form-error">
            {error}
          </p>
        )}
        <div className="editor-actions">
          <Button type="button" variant="ghost" onClick={onClose} disabled={pending}>
            Cancel
          </Button>
          <Button type="submit" disabled={pending}>
            {pending ? 'Saving…' : 'Save access'}
          </Button>
        </div>
      </form>
    </SettingsEditor>
  )
}

export function ScopePicker({
  configuration,
  scopes,
  onChange,
}: {
  configuration: Configuration
  scopes: readonly Scope[]
  onChange: (scopes: readonly Scope[]) => void
}) {
  return (
    <fieldset className="scope-picker">
      <legend>Visible namespaces</legend>
      {configuration.connections.length === 0 && <p>Add a cluster before assigning namespaces.</p>}
      {configuration.connections.map((connection) => (
        <div key={connection.id}>
          <strong>{connection.name}</strong>
          {connection.namespaces.map((namespace) => {
            const checked = scopes.some(
              (scope) =>
                scope.connectionId === connection.id && scope.namespaces.includes(namespace.name),
            )
            return (
              <label key={namespace.name}>
                <input
                  type="checkbox"
                  checked={checked}
                  onChange={(event) => {
                    const remaining = scopes.filter((scope) => scope.connectionId !== connection.id)
                    const names = scopes
                      .filter((scope) => scope.connectionId === connection.id)
                      .flatMap((scope) => scope.namespaces)
                      .filter((name) => name !== namespace.name)
                    if (event.target.checked) names.push(namespace.name)
                    onChange(
                      names.length
                        ? [...remaining, { connectionId: connection.id, namespaces: names }]
                        : remaining,
                    )
                  }}
                />
                {namespace.name}
              </label>
            )
          })}
        </div>
      ))}
    </fieldset>
  )
}
