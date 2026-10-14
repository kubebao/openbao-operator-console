import { useState } from 'react'
import { Plus, Trash2 } from 'lucide-react'
import type { CredentialUpdate, ManagedConnection } from '../api/schema'
import { useCheckConnection } from '../api/client'
import type { ManagedOperator } from '../lib/connection'
import {
  addNamespace,
  assignNamespace,
  environments,
  isEnvironment,
  namespaceNames,
  removeNamespace,
  updateNamespace,
  updateOperator,
  withAuthentication,
} from '../lib/connection'
import {
  checkKey,
  connectionCheck,
  credentialUpdate,
  emptyCredential,
  withRenewal,
} from '../lib/credential'
import { environmentLabels } from '../lib/format'
import { CredentialFields } from './credential-fields'
import { RenewalAccess, RenewalFields } from './credential-renewal'
import { Field, FormError } from './form'
import { ConnectionCheckSummary } from './onboarding-checks'
import { SettingsEditor } from './settings-editor'
import { Button } from './ui/button'

const operatorFields: readonly {
  field: keyof Omit<ManagedOperator, 'namespaces'>
  label: string
}[] = [
  { field: 'id', label: 'Operator identifier' },
  { field: 'namespace', label: 'Controller namespace' },
  { field: 'deployment', label: 'Deployment name' },
  { field: 'container', label: 'Container name' },
]

// ConnectionEditor edits an enrolled connection. Leaving the credential empty keeps the
// stored reader; a new credential must pass a check before collection is enabled. onSave
// resolves false when the save awaits a confirmation, which keeps the editor open.
export function ConnectionEditor({
  initial,
  onClose,
  onSave,
}: {
  initial: ManagedConnection
  onClose: () => void
  onSave: (connection: ManagedConnection, credential?: CredentialUpdate) => Promise<boolean>
}) {
  const [connection, setConnection] = useState(initial)
  const [draft, setDraft] = useState(() => emptyCredential('token'))
  const [error, setError] = useState<unknown>(null)
  const [saving, setSaving] = useState(false)
  const check = useCheckConnection()
  // An empty credential with the original API server keeps the stored reader.
  const request = connectionCheck(connection, draft, { server: initial.server })
  const result =
    check.isSuccess && checkKey(check.variables) === checkKey(request) ? check.data : null
  const pending = saving || check.isPending
  const replacing = request.credential !== undefined || request.kubeconfig !== undefined

  async function test() {
    setError(null)
    await check.mutateAsync(request).catch(setError)
  }
  async function save() {
    setError(null)
    if (!connection.disabled && replacing && result?.connection.collection !== 'current') {
      setError(
        'Test the reader successfully before enabling collection, or save the connection paused.',
      )
      return
    }
    setSaving(true)
    try {
      if (await onSave(connection, credentialUpdate(request))) onClose()
    } catch (failure) {
      setError(failure)
    } finally {
      setSaving(false)
    }
  }
  return (
    <SettingsEditor title={`Edit ${initial.name}`} onClose={onClose}>
      <form
        onSubmit={(event) => {
          event.preventDefault()
          void save()
        }}
      >
        <ConnectionIdentity connection={connection} onChange={setConnection} />
        <RenewalFields
          connection={connection}
          onChange={(next) => {
            setConnection(next)
            setDraft(withRenewal(draft, !!next.renewal))
          }}
        />
        <RenewalAccess connection={connection} />
        {connection.authentication === 'remote' && (
          <fieldset className="credential-fields">
            <legend>Reader credentials</legend>
            {connection.credentialConfigured && (
              <p>
                Leave credentials empty to keep the current reader. Supply a complete credential to
                replace it.
              </p>
            )}
            <CredentialFields
              connection={connection}
              draft={draft}
              required={false}
              onConnection={setConnection}
              onDraft={setDraft}
              onError={setError}
            />
          </fieldset>
        )}
        <NamespaceEditor connection={connection} onChange={setConnection} />
        <OperatorEditor connection={connection} onChange={setConnection} />
        <label className="checkbox-field">
          <input
            type="checkbox"
            checked={!connection.disabled}
            onChange={(event) => {
              setConnection({ ...connection, disabled: !event.target.checked })
            }}
          />
          Enable collection
        </label>
        {result && <ConnectionCheckSummary result={result} connection={connection} />}
        <FormError error={error} />
        <div className="editor-actions">
          <Button type="button" variant="ghost" onClick={onClose} disabled={pending}>
            Cancel
          </Button>
          <Button
            type="button"
            onClick={() => {
              void test()
            }}
            disabled={pending}
          >
            {pending ? 'Working…' : 'Test connection'}
          </Button>
          <Button type="submit" variant="default" disabled={pending}>
            Save connection
          </Button>
        </div>
      </form>
    </SettingsEditor>
  )
}

function ConnectionIdentity({
  connection,
  onChange,
}: {
  connection: ManagedConnection
  onChange: (connection: ManagedConnection) => void
}) {
  return (
    <div className="form-grid">
      <Field label="Name">
        <input
          required
          value={connection.name}
          onChange={(event) => {
            onChange({ ...connection, name: event.target.value })
          }}
        />
      </Field>
      <Field label="Identifier" help="Stable identity used by access rules and history.">
        <input disabled value={connection.id} />
      </Field>
      <Field label="Region">
        <input
          value={connection.region}
          onChange={(event) => {
            onChange({ ...connection, region: event.target.value })
          }}
        />
      </Field>
      <Field label="Connection">
        <select
          value={connection.authentication}
          onChange={(event) => {
            const value = event.target.value
            if (value === 'local' || value === 'remote')
              onChange(withAuthentication(connection, value))
          }}
        >
          <option value="local">Local Kubernetes ServiceAccount</option>
          <option value="remote">Remote reader credential</option>
        </select>
      </Field>
    </div>
  )
}

function NamespaceEditor({
  connection,
  onChange,
}: {
  connection: ManagedConnection
  onChange: (connection: ManagedConnection) => void
}) {
  return (
    <>
      <div className="section-heading">
        <h3>Namespaces</h3>
        <Button
          type="button"
          size="small"
          variant="ghost"
          onClick={() => {
            onChange(addNamespace(connection))
          }}
        >
          <Plus size={14} />
          Add namespace
        </Button>
      </div>
      {connection.namespaces.map((namespace, index) => (
        // Rows are controlled and have no identity until named, so position is the key.
        <div className="namespace-editor" key={index}>
          <div className="form-grid">
            <Field label={`Namespace ${String(index + 1)}`}>
              <input
                required
                value={namespace.name}
                onChange={(event) => {
                  onChange(updateNamespace(connection, index, { name: event.target.value }))
                }}
              />
            </Field>
            <Field label="Team">
              <input
                value={namespace.team}
                onChange={(event) => {
                  onChange(updateNamespace(connection, index, { team: event.target.value }))
                }}
              />
            </Field>
            <Field label="Environment">
              <select
                value={namespace.environment}
                onChange={(event) => {
                  const environment = event.target.value
                  if (isEnvironment(environment))
                    onChange(updateNamespace(connection, index, { environment }))
                }}
              >
                {environments.map((environment) => (
                  <option key={environment} value={environment}>
                    {environmentLabels[environment]}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="Location">
              <input
                value={namespace.location}
                onChange={(event) => {
                  onChange(updateNamespace(connection, index, { location: event.target.value }))
                }}
              />
            </Field>
          </div>
          <Button
            type="button"
            size="icon"
            variant="ghost"
            aria-label={`Remove namespace ${String(index + 1)}`}
            onClick={() => {
              onChange(removeNamespace(connection, index))
            }}
          >
            <Trash2 size={15} />
          </Button>
        </div>
      ))}
    </>
  )
}

function OperatorEditor({
  connection,
  onChange,
}: {
  connection: ManagedConnection
  onChange: (connection: ManagedConnection) => void
}) {
  return (
    <>
      <div className="section-heading">
        <h3>Operators</h3>
        <Button
          type="button"
          size="small"
          variant="ghost"
          onClick={() => {
            onChange({
              ...connection,
              operators: [
                ...connection.operators,
                { id: '', namespace: '', deployment: '', container: 'manager', namespaces: [] },
              ],
            })
          }}
        >
          <Plus size={14} />
          Add operator
        </Button>
      </div>
      {connection.operators.map((operator, index) => (
        <fieldset key={index} className="operator-editor">
          <legend>Operator {index + 1}</legend>
          <div className="form-grid">
            {operatorFields.map(({ field, label }) => (
              <Field key={field} label={label}>
                <input
                  required
                  value={operator[field]}
                  onChange={(event) => {
                    onChange(updateOperator(connection, index, { [field]: event.target.value }))
                  }}
                />
              </Field>
            ))}
          </div>
          <div className="checkbox-list">
            {namespaceNames(connection).map((name) => (
              <label key={name}>
                <input
                  type="checkbox"
                  checked={operator.namespaces.includes(name)}
                  onChange={(event) => {
                    onChange(assignNamespace(connection, index, name, event.target.checked))
                  }}
                />
                {name}
              </label>
            ))}
          </div>
          <Button
            type="button"
            size="small"
            variant="ghost"
            onClick={() => {
              onChange({
                ...connection,
                operators: connection.operators.filter((_, position) => position !== index),
              })
            }}
          >
            Remove operator
          </Button>
        </fieldset>
      ))}
    </>
  )
}
