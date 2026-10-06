import { formError } from '../lib/errors'
import { useState } from 'react'
import { Plus, Trash2 } from 'lucide-react'
import type {
  ManagedConnection,
  CredentialUpdate,
  ConnectionCheck,
  ConnectionCheckResult,
} from '../api/schema'
import { checkConnection } from '../api/client'
import { SettingsEditor, Field } from './settings-editor'
import { Button } from './ui/button'
import { RenewalFields, RenewalAccess } from './credential-renewal'
import { CredentialLifetime } from './onboarding-checks'
import { capabilityCheckLabels, operatorCheckLabels } from '../lib/format'
import { Callout } from './page'

export function ConnectionEditor({
  initial,
  existing,
  onClose,
  onSave,
}: {
  initial: ManagedConnection
  existing: boolean
  onClose: () => void
  onSave: (connection: ManagedConnection, credential?: CredentialUpdate) => Promise<void>
}) {
  const [connection, setConnection] = useState(initial)
  const [method, setMethod] = useState('token')
  const [token, setToken] = useState('')
  const [ca, setCA] = useState('')
  const [certificate, setCertificate] = useState('')
  const [key, setKey] = useState('')
  const [kubeconfig, setKubeconfig] = useState('')
  const [context, setContext] = useState('')
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)
  const [check, setCheck] = useState<{ fingerprint: string; result: ConnectionCheckResult } | null>(
    null,
  )
  function input(): ConnectionCheck {
    const result: ConnectionCheck = { connection }
    if (connection.authentication === 'local') return result
    if (method === 'kubeconfig' && kubeconfig) return { ...result, kubeconfig, context }
    if (token || certificate || key || ca || connection.server !== initial.server)
      return {
        ...result,
        credential: {
          server: connection.server,
          ca,
          token: method === 'token' ? token : '',
          certificate: method === 'certificate' ? certificate : '',
          key: method === 'certificate' ? key : '',
        },
      }
    return result
  }
  const currentInput = input()
  const fingerprint = JSON.stringify({
    ...currentInput,
    connection: { ...connection, disabled: false },
  })
  const checkCurrent = check?.fingerprint === fingerprint ? check.result : null
  async function test() {
    setPending(true)
    setError('')
    setCheck(null)
    try {
      setCheck({ fingerprint, result: await checkConnection(currentInput) })
    } catch (error) {
      setError(formError(error))
    } finally {
      setPending(false)
    }
  }
  async function save() {
    setError('')
    if (
      !connection.disabled &&
      (!existing || currentInput.credential || currentInput.kubeconfig) &&
      checkCurrent?.connection.collection !== 'current'
    ) {
      setError(
        'Test the reader successfully before enabling collection, or save the connection paused.',
      )
      return
    }
    setPending(true)
    try {
      let credential: CredentialUpdate | undefined
      if (currentInput.credential)
        credential = { connectionId: connection.id, credential: currentInput.credential }
      else if (currentInput.kubeconfig)
        credential = { connectionId: connection.id, kubeconfig: currentInput.kubeconfig, context }
      await onSave(connection, credential)
      onClose()
    } catch (error) {
      setError(formError(error))
    } finally {
      setPending(false)
    }
  }
  return (
    <SettingsEditor title={existing ? `Edit ${initial.name}` : 'Add cluster'} onClose={onClose}>
      <form
        onSubmit={(event) => {
          event.preventDefault()
          void save()
        }}
      >
        <div className="form-grid">
          <Field label="Name">
            <input
              required
              value={connection.name}
              onChange={(event) => {
                setConnection({ ...connection, name: event.target.value })
              }}
            />
          </Field>
          <Field label="Identifier" help="Stable identity used by access rules and history.">
            <input
              required
              disabled={existing}
              pattern="[a-zA-Z0-9_-]+"
              value={connection.id}
              onChange={(event) => {
                setConnection({ ...connection, id: event.target.value })
              }}
            />
          </Field>
          <Field label="Region">
            <input
              value={connection.region}
              onChange={(event) => {
                setConnection({ ...connection, region: event.target.value })
              }}
            />
          </Field>
          <Field label="Connection">
            <select
              value={connection.authentication}
              onChange={(event) => {
                const value = event.target.value
                if (value === 'local' || value === 'remote')
                  setConnection({
                    ...connection,
                    authentication: value,
                    ...(value === 'local' ? { renewal: null } : {}),
                  })
              }}
            >
              <option value="local">Local Kubernetes ServiceAccount</option>
              <option value="remote">Remote reader credential</option>
            </select>
          </Field>
        </div>
        <RenewalFields
          connection={connection}
          onChange={(next) => {
            setConnection(next)
            if (next.renewal && method === 'certificate') {
              setMethod('token')
              setCertificate('')
              setKey('')
            }
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
            <Field label="Authentication">
              <select
                value={method}
                onChange={(event) => {
                  setMethod(event.target.value)
                }}
              >
                <option value="token">Bearer token</option>
                <option value="certificate" disabled={!!connection.renewal}>
                  Client certificate
                </option>
                <option value="kubeconfig">Import kubeconfig</option>
              </select>
            </Field>
            {method === 'kubeconfig' ? (
              <>
                <Field
                  label="Kubeconfig"
                  help="Embedded credentials only. File references and authentication plugins are rejected."
                >
                  <textarea
                    rows={7}
                    autoComplete="off"
                    value={kubeconfig}
                    onChange={(event) => {
                      setKubeconfig(event.target.value)
                    }}
                  />
                </Field>
                <Field label="Context">
                  <input
                    value={context}
                    onChange={(event) => {
                      setContext(event.target.value)
                    }}
                    placeholder="Current context"
                  />
                </Field>
              </>
            ) : (
              <>
                <Field label="API server">
                  <input
                    type="url"
                    placeholder="https://kubernetes.example:6443"
                    value={connection.server}
                    onChange={(event) => {
                      setConnection({ ...connection, server: event.target.value })
                    }}
                  />
                </Field>
                <Field label="CA certificate" help="Leave empty to use system trust.">
                  <textarea
                    rows={3}
                    value={ca}
                    onChange={(event) => {
                      setCA(event.target.value)
                    }}
                  />
                </Field>
                {method === 'token' ? (
                  <Field label="Bearer token">
                    <input
                      type="password"
                      autoComplete="new-password"
                      value={token}
                      onChange={(event) => {
                        setToken(event.target.value)
                      }}
                    />
                  </Field>
                ) : (
                  <div className="form-grid">
                    <Field label="Client certificate">
                      <textarea
                        rows={4}
                        value={certificate}
                        onChange={(event) => {
                          setCertificate(event.target.value)
                        }}
                      />
                    </Field>
                    <Field label="Private key">
                      <textarea
                        rows={4}
                        autoComplete="off"
                        value={key}
                        onChange={(event) => {
                          setKey(event.target.value)
                        }}
                      />
                    </Field>
                  </div>
                )}
              </>
            )}
          </fieldset>
        )}
        <div className="section-heading">
          <h3>Namespaces</h3>
          <Button
            type="button"
            size="small"
            variant="ghost"
            onClick={() => {
              setConnection({
                ...connection,
                namespaces: [
                  ...connection.namespaces,
                  { name: '', team: '', environment: 'production', location: '' },
                ],
              })
            }}
          >
            <Plus size={14} />
            Add namespace
          </Button>
        </div>
        {connection.namespaces.map((namespace, index) => (
          <div className="namespace-editor" key={index}>
            <div className="form-grid">
              <Field label={`Namespace ${String(index + 1)}`}>
                <input
                  required
                  value={namespace.name}
                  onChange={(event) => {
                    setConnection({
                      ...connection,
                      namespaces: connection.namespaces.map((item, position) =>
                        position === index ? { ...item, name: event.target.value } : item,
                      ),
                    })
                  }}
                />
              </Field>
              <Field label="Team">
                <input
                  value={namespace.team}
                  onChange={(event) => {
                    setConnection({
                      ...connection,
                      namespaces: connection.namespaces.map((item, position) =>
                        position === index ? { ...item, team: event.target.value } : item,
                      ),
                    })
                  }}
                />
              </Field>
              <Field label="Environment">
                <select
                  value={namespace.environment}
                  onChange={(event) => {
                    const value = event.target.value
                    if (
                      value === 'production' ||
                      value === 'staging' ||
                      value === 'development' ||
                      value === 'other'
                    )
                      setConnection({
                        ...connection,
                        namespaces: connection.namespaces.map((item, position) =>
                          position === index ? { ...item, environment: value } : item,
                        ),
                      })
                  }}
                >
                  <option value="production">Production</option>
                  <option value="staging">Staging</option>
                  <option value="development">Development</option>
                  <option value="other">Other</option>
                </select>
              </Field>
              <Field label="Location">
                <input
                  value={namespace.location}
                  onChange={(event) => {
                    setConnection({
                      ...connection,
                      namespaces: connection.namespaces.map((item, position) =>
                        position === index ? { ...item, location: event.target.value } : item,
                      ),
                    })
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
                setConnection({
                  ...connection,
                  namespaces: connection.namespaces.filter((_, position) => position !== index),
                })
              }}
            >
              <Trash2 size={15} />
            </Button>
          </div>
        ))}
        <div className="section-heading">
          <h3>Operators</h3>
          <Button
            type="button"
            size="small"
            variant="ghost"
            onClick={() => {
              setConnection({
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
              {(['id', 'namespace', 'deployment', 'container'] as const).map((field) => (
                <Field
                  key={field}
                  label={
                    field === 'id'
                      ? 'Operator identifier'
                      : field === 'namespace'
                        ? 'Controller namespace'
                        : field === 'deployment'
                          ? 'Deployment name'
                          : 'Container name'
                  }
                >
                  <input
                    required
                    value={operator[field]}
                    onChange={(event) => {
                      setConnection({
                        ...connection,
                        operators: connection.operators.map((item, position) =>
                          position === index ? { ...item, [field]: event.target.value } : item,
                        ),
                      })
                    }}
                  />
                </Field>
              ))}
            </div>
            <div className="checkbox-list">
              {connection.namespaces
                .filter((namespace) => namespace.name)
                .map((namespace) => (
                  <label key={namespace.name}>
                    <input
                      type="checkbox"
                      checked={operator.namespaces.includes(namespace.name)}
                      onChange={(event) => {
                        setConnection({
                          ...connection,
                          operators: connection.operators.map((item, position) =>
                            position === index
                              ? {
                                  ...item,
                                  namespaces: event.target.checked
                                    ? [...item.namespaces, namespace.name]
                                    : item.namespaces.filter((name) => name !== namespace.name),
                                }
                              : item,
                          ),
                        })
                      }}
                    />
                    {namespace.name}
                  </label>
                ))}
            </div>
            <Button
              type="button"
              size="small"
              variant="ghost"
              onClick={() => {
                setConnection({
                  ...connection,
                  operators: connection.operators.filter((_, position) => position !== index),
                })
              }}
            >
              Remove operator
            </Button>
          </fieldset>
        ))}
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
        {checkCurrent && (
          <section className="connection-check">
            <Callout
              tone={checkCurrent.diagnostics.state === 'available' ? 'success' : 'error'}
              title={
                checkCurrent.diagnostics.state === 'available'
                  ? 'Reader access verified'
                  : 'Connection needs attention'
              }
            >
              {checkCurrent.diagnostics.message}
            </Callout>
            <CredentialLifetime
              expiry={checkCurrent.credentialExpiry}
              local={connection.authentication === 'local'}
              renewal={!!connection.renewal}
            />
            <ul>
              {checkCurrent.connection.capabilities.map((capability) => (
                <li key={`${capability.namespace}/${capability.resource}`}>
                  {capability.namespace} / {capability.resource}
                  <span>{capabilityCheckLabels[capability.state]}</span>
                </li>
              ))}
              {checkCurrent.connection.operators.map((operator) => (
                <li key={operator.id}>
                  {operator.namespace}/{operator.name}
                  <span>{operatorCheckLabels[operator.collection]}</span>
                </li>
              ))}
            </ul>
            <details>
              <summary>Reader RBAC manifests</summary>
              <textarea
                aria-label="Reader RBAC manifests"
                readOnly
                rows={8}
                value={checkCurrent.rbac}
              />
            </details>
          </section>
        )}
        {error && (
          <p className="form-error" role="alert">
            {error}
          </p>
        )}
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
