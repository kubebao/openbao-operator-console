import { formError } from '../lib/errors'
import { useState } from 'react'
import type { Deployment, ManagedRelationship } from '../api/schema'
import type { EditMode } from '../lib/settings'
import { SettingsEditor } from './settings-editor'
import { Field } from './form'
import { Button } from './ui/button'

// ref binds a relationship end to the observed resource identity.
function ref(deployment: Deployment): ManagedRelationship['source'] {
  return {
    connectionId: deployment.connectionId,
    namespace: deployment.namespace,
    name: deployment.name,
    uid: deployment.uid,
  }
}

export function RelationshipEditor({
  initial,
  deployments,
  mode,
  onSave,
  onClose,
}: {
  initial: ManagedRelationship | null
  deployments: readonly Deployment[]
  mode: EditMode
  onSave: (relationship: ManagedRelationship) => Promise<void>
  onClose: () => void
}) {
  const [id, setID] = useState(initial?.id ?? '')
  const [kind, setKind] = useState<ManagedRelationship['kind']>(initial?.kind ?? 'snapshotRecovery')
  const [sourceID, setSource] = useState(
    deployments.find(
      (deployment) =>
        deployment.uid === initial?.source.uid &&
        deployment.connectionId === initial.source.connectionId,
    )?.id ?? '',
  )
  const [targetID, setTarget] = useState(
    deployments.find(
      (deployment) =>
        deployment.uid === initial?.target.uid &&
        deployment.connectionId === initial.target.connectionId,
    )?.id ?? '',
  )
  const [repository, setRepository] = useState(initial?.repository ?? '')
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)
  const source = deployments.find((deployment) => deployment.id === sourceID)
  const target = deployments.find((deployment) => deployment.id === targetID)
  async function save() {
    if (!source || !target || source.id === target.id) {
      setError('Select two different observed deployments.')
      return
    }
    setPending(true)
    setError('')
    try {
      // Only snapshot recovery has a repository. A value typed before switching kind is dropped.
      const declared = kind === 'snapshotRecovery' ? repository : ''
      await onSave({ id, kind, repository: declared, source: ref(source), target: ref(target) })
      onClose()
    } catch (error) {
      setError(formError(error))
    } finally {
      setPending(false)
    }
  }
  return (
    <SettingsEditor
      title={mode === 'edit' ? 'Edit relationship' : 'Add relationship'}
      onClose={onClose}
    >
      <form
        onSubmit={(event) => {
          event.preventDefault()
          void save()
        }}
      >
        <Field label="Identifier">
          <input
            required
            disabled={mode === 'edit'}
            value={id}
            onChange={(event) => {
              setID(event.target.value)
            }}
          />
        </Field>
        <Field label="Relationship">
          <select
            value={kind}
            onChange={(event) => {
              const value = event.target.value
              if (value === 'replication' || value === 'snapshotRecovery') setKind(value)
            }}
          >
            <option value="snapshotRecovery">Snapshot recovery</option>
            <option value="replication">Remote read replica</option>
          </select>
        </Field>
        <div className="form-grid">
          {[
            { label: 'Source deployment', value: sourceID, change: setSource },
            { label: 'Target deployment', value: targetID, change: setTarget },
          ].map((field) => (
            <Field label={field.label} key={field.label}>
              <select
                required
                value={field.value}
                onChange={(event) => {
                  field.change(event.target.value)
                }}
              >
                <option value="">Select deployment</option>
                {deployments.map((deployment) => (
                  <option key={deployment.id} value={deployment.id}>
                    {deployment.connectionId} / {deployment.namespace}/{deployment.name}
                  </option>
                ))}
              </select>
            </Field>
          ))}
        </div>
        {kind === 'snapshotRecovery' && (
          <Field
            label="Snapshot repository"
            help="Display location only. Do not include credentials or signed URL parameters."
          >
            <input
              value={repository}
              onChange={(event) => {
                setRepository(event.target.value)
              }}
              placeholder="s3://backups"
            />
          </Field>
        )}
        <p className="muted">
          The relationship is bound to these resource identities. Replacing a deployment requires
          selecting its new identity.
        </p>
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
            Save relationship
          </Button>
        </div>
      </form>
    </SettingsEditor>
  )
}
