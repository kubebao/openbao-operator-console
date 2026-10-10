import type { ManagedConnection } from '../../api/schema'
import { connectionIDPattern, withAuthentication } from '../../lib/connection'
import { Field } from '../form'

export function ClusterStep({
  connection,
  onName,
  onID,
  onChange,
}: {
  connection: ManagedConnection
  onName: (name: string) => void
  onID: (id: string) => void
  onChange: (connection: ManagedConnection) => void
}) {
  return (
    <>
      <h2>Which cluster do you want to observe?</h2>
      <Field label="Cluster name">
        <input
          required
          autoFocus
          value={connection.name}
          onChange={(event) => {
            onName(event.target.value)
          }}
          placeholder="Production EU"
        />
      </Field>
      <Field label="Connection">
        <select
          value={connection.authentication}
          onChange={(event) => {
            const authentication = event.target.value
            if (authentication === 'local' || authentication === 'remote')
              onChange(withAuthentication(connection, authentication))
          }}
        >
          <option value="remote">Remote Kubernetes cluster</option>
          <option value="local">Cluster hosting this console</option>
        </select>
      </Field>
      {connection.authentication === 'local' && (
        <p className="onboarding-help">
          Uses the console's Kubernetes ServiceAccount. No credential upload is required.
        </p>
      )}
      <details>
        <summary>Advanced</summary>
        <Field
          label="Cluster identifier"
          help="Stable identity for access rules and history. It cannot be renamed later."
        >
          <input
            required
            pattern={connectionIDPattern}
            value={connection.id}
            onChange={(event) => {
              onID(event.target.value)
            }}
          />
        </Field>
      </details>
    </>
  )
}
