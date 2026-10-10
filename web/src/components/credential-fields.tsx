import { Fragment } from 'react'
import type { ManagedConnection } from '../api/schema'
import type { CredentialDraft } from '../lib/credential'
import { isCredentialMethod } from '../lib/credential'
import { Field } from './form'

// Kubeconfig uploads above this size are rejected before they are read.
const maxKubeconfigBytes = 800_000

// CredentialFields collects a remote reader credential: an imported kubeconfig, a bearer
// token, or a client certificate. Token renewal requires a bearer token.
export function CredentialFields({
  connection,
  draft,
  required,
  onConnection,
  onDraft,
  onError,
}: {
  connection: ManagedConnection
  draft: CredentialDraft
  required: boolean
  onConnection: (connection: ManagedConnection) => void
  onDraft: (draft: CredentialDraft) => void
  onError: (message: string) => void
}) {
  const change = (field: keyof Omit<CredentialDraft, 'method'>) => (value: string) => {
    onDraft({ ...draft, [field]: value })
  }
  return (
    <>
      <Field label="Credential method">
        <select
          value={draft.method}
          onChange={(event) => {
            const method = event.target.value
            if (isCredentialMethod(method)) onDraft({ ...draft, method })
          }}
        >
          <option value="kubeconfig">Import reader kubeconfig</option>
          <option value="token">Bearer token</option>
          <option value="certificate" disabled={!!connection.renewal}>
            Client certificate
          </option>
        </select>
      </Field>
      {draft.method === 'kubeconfig' ? (
        <Fragment key="kubeconfig">
          <Field label="Kubeconfig file">
            <input
              type="file"
              accept=".yaml,.yml,.conf,.kubeconfig"
              onChange={(event) => {
                const file = event.target.files?.[0]
                if (!file) return
                if (file.size > maxKubeconfigBytes) {
                  onError('Kubeconfig must be smaller than 800 KiB.')
                  return
                }
                void file
                  .text()
                  .then(change('kubeconfig'))
                  .catch(() => {
                    onError('The kubeconfig file could not be read.')
                  })
              }}
            />
          </Field>
          <Field
            label="Reader kubeconfig"
            help="Embedded token or certificate only. Authentication plugins and file references are rejected."
          >
            <TextArea
              required={required}
              rows={8}
              value={draft.kubeconfig}
              onChange={change('kubeconfig')}
            />
          </Field>
          <Field label="Kubeconfig context" help="Leave empty to use its current context.">
            <TextInput value={draft.context} onChange={change('context')} />
          </Field>
        </Fragment>
      ) : (
        <Fragment key="manual">
          <Field label="API server">
            <input
              required={required}
              type="url"
              placeholder="https://kubernetes.example:6443"
              value={connection.server}
              onChange={(event) => {
                onConnection({ ...connection, server: event.target.value })
              }}
            />
          </Field>
          <Field label="CA certificate" help="Leave empty to use system trust.">
            <TextArea rows={3} value={draft.ca} onChange={change('ca')} />
          </Field>
          {draft.method === 'token' ? (
            <Field label="Bearer token">
              <input
                required={required}
                type="password"
                autoComplete="new-password"
                value={draft.token}
                onChange={(event) => {
                  change('token')(event.target.value)
                }}
              />
            </Field>
          ) : (
            <div className="form-grid">
              <Field label="Client certificate">
                <TextArea
                  required={required}
                  rows={5}
                  value={draft.certificate}
                  onChange={change('certificate')}
                />
              </Field>
              <Field label="Private key">
                <TextArea required={required} rows={5} value={draft.key} onChange={change('key')} />
              </Field>
            </div>
          )}
        </Fragment>
      )}
    </>
  )
}

function TextArea({
  value,
  rows,
  required = false,
  onChange,
}: {
  value: string
  rows: number
  required?: boolean
  onChange: (value: string) => void
}) {
  return (
    <textarea
      required={required}
      autoComplete="off"
      rows={rows}
      value={value}
      onChange={(event) => {
        onChange(event.target.value)
      }}
    />
  )
}

function TextInput({ value, onChange }: { value: string; onChange: (value: string) => void }) {
  return (
    <input
      value={value}
      onChange={(event) => {
        onChange(event.target.value)
      }}
    />
  )
}
