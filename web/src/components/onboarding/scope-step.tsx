import { Plus, Trash2 } from 'lucide-react'
import type { ManagedConnection } from '../../api/schema'
import type { ManagedOperator } from '../../lib/connection'
import {
  addNamespace,
  defaultOperatorLocation,
  namespacePattern,
  removeNamespace,
  updateNamespace,
  updateOperator,
} from '../../lib/connection'
import { RenewalFields } from '../credential-renewal'
import { Field } from '../form'
import { Button } from '../ui/button'

const operatorFields: readonly {
  field: keyof Pick<ManagedOperator, 'namespace' | 'deployment' | 'container'>
  label: string
}[] = [
  { field: 'namespace', label: 'Operator namespace' },
  { field: 'deployment', label: 'Operator Deployment' },
  { field: 'container', label: 'Controller container' },
]

// ScopeStep selects the namespaces the reader observes. The optional operator covers all
// of them.
export function ScopeStep({
  connection,
  operatorID,
  onChange,
}: {
  connection: ManagedConnection
  operatorID: string
  onChange: (connection: ManagedConnection) => void
}) {
  return (
    <>
      <h2>Choose namespaces</h2>
      <p className="onboarding-help">The reader only observes the namespaces you enter.</p>
      {connection.namespaces.map((namespace, index) => (
        // Rows are controlled and have no identity until named, so position is the key.
        <div className="onboarding-namespace" key={index}>
          <Field label={`Namespace ${String(index + 1)}`}>
            <input
              required
              pattern={namespacePattern}
              maxLength={63}
              value={namespace.name}
              onChange={(event) => {
                onChange(updateNamespace(connection, index, { name: event.target.value }))
              }}
              placeholder="openbao-production"
            />
          </Field>
          <Button
            type="button"
            size="icon"
            variant="ghost"
            aria-label={`Remove namespace ${String(index + 1)}`}
            disabled={connection.namespaces.length === 1}
            onClick={() => {
              onChange(removeNamespace(connection, index))
            }}
          >
            <Trash2 size={15} />
          </Button>
        </div>
      ))}
      <Button
        type="button"
        size="small"
        onClick={() => {
          onChange(addNamespace(connection))
        }}
      >
        <Plus size={14} />
        Add namespace
      </Button>
      <label className="checkbox-field">
        <input
          type="checkbox"
          checked={connection.operators.length > 0}
          onChange={(event) => {
            onChange({
              ...connection,
              operators: event.target.checked
                ? [{ id: operatorID, ...defaultOperatorLocation, namespaces: [] }]
                : [],
            })
          }}
        />
        Observe the operator for these namespaces
      </label>
      {connection.operators.length > 0 && (
        <p className="onboarding-help">
          Prefilled with the OpenBao Operator chart defaults for the release openbao-operator.
          Change them to match your installation; verification checks the Deployment.
        </p>
      )}
      {connection.operators.map((operator, index) => (
        <div key={operator.id} className="form-grid">
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
      ))}
      <p className="onboarding-help">
        You can assign teams, environments, and additional operators after connecting.
      </p>
      <RenewalFields connection={connection} onChange={onChange} />
    </>
  )
}
