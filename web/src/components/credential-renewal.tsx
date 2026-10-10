import { useState } from 'react'
import type { CredentialStatus, ManagedConnection } from '../api/schema'
import { generateReaderAccess } from '../api/client'
import { formError } from '../lib/errors'
import { downloadText } from '../lib/download'
import { Field } from './form'
import { Button } from './ui/button'
import { Timestamp } from './status'

export function RenewalFields({
  connection,
  onChange,
}: {
  connection: ManagedConnection
  onChange: (connection: ManagedConnection) => void
}) {
  if (connection.authentication !== 'remote') return null
  return (
    <fieldset className="credential-fields">
      <legend>Credential renewal</legend>
      <label className="checkbox-field">
        <input
          type="checkbox"
          checked={!!connection.renewal}
          onChange={(event) => {
            onChange({
              ...connection,
              renewal: event.target.checked
                ? { namespace: 'console-system', serviceAccount: 'console-reader' }
                : null,
            })
          }}
        />
        Renew ServiceAccount tokens automatically
      </label>
      <p className="onboarding-help">
        Adds a grant that lets the reader request its own short-lived tokens. This changes the
        generated reader RBAC.
      </p>
      {connection.renewal && (
        <>
          <p className="onboarding-help">
            If renewal fails until the current token expires, the cluster must be reconnected.
          </p>
          <div className="form-grid">
            <Field label="Reader namespace">
              <input
                required
                value={connection.renewal.namespace}
                onChange={(event) => {
                  if (connection.renewal)
                    onChange({
                      ...connection,
                      renewal: { ...connection.renewal, namespace: event.target.value },
                    })
                }}
              />
            </Field>
            <Field label="Reader ServiceAccount">
              <input
                required
                value={connection.renewal.serviceAccount}
                onChange={(event) => {
                  if (connection.renewal)
                    onChange({
                      ...connection,
                      renewal: { ...connection.renewal, serviceAccount: event.target.value },
                    })
                }}
              />
            </Field>
          </div>
        </>
      )}
    </fieldset>
  )
}

export function RenewalAccess({ connection }: { connection: ManagedConnection }) {
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)
  if (!connection.renewal) return null
  return (
    <>
      <Button
        type="button"
        size="small"
        disabled={pending}
        onClick={() => {
          setPending(true)
          setError('')
          void generateReaderAccess(connection)
            .then((access) => {
              downloadText(access.rbac, 'console-reader.yaml')
            })
            .catch((error: unknown) => {
              setError(formError(error))
            })
            .finally(() => {
              setPending(false)
            })
        }}
      >
        {pending ? 'Preparing…' : 'Download reader RBAC'}
      </Button>
      <p className="onboarding-help">
        Apply these grants in the reader's cluster before enabling renewal.
      </p>
      {error && (
        <p role="alert" className="form-error">
          {error}
        </p>
      )}
    </>
  )
}

const labels: Record<CredentialStatus['state'], string> = {
  ready: 'Credential ready',
  pending: 'Token renewal pending',
  healthy: 'Token renewal active',
  retrying: 'Token renewal retrying',
  permissionDenied: 'Renewal permission required',
  reconnectRequired: 'Reconnect required',
  paused: 'Token renewal paused',
}

export function CredentialStatusLabel({ status }: { status: CredentialStatus | undefined }) {
  if (!status || status.mode === 'local') return null
  const renewing = status.mode === 'automatic' && status.nextAttemptAt !== null
  return (
    <div className="credential-status">
      <small
        className={
          status.state === 'reconnectRequired' || status.state === 'permissionDenied'
            ? 'credential-warning'
            : ''
        }
      >
        {status.mode === 'manual' ? 'Manual credential' : labels[status.state]}
      </small>
      {(status.expiresAt !== null || renewing) && (
        <small>
          {status.expiresAt && (
            <>
              {status.expirySource === 'declaredToken' ? 'Declared expiry' : 'Expires'}{' '}
              <Timestamp value={status.expiresAt} scheduled />
            </>
          )}
          {status.expiresAt && renewing && ' · '}
          {renewing && (
            <>
              renews <Timestamp value={status.nextAttemptAt} scheduled />
            </>
          )}
        </small>
      )}
    </div>
  )
}
