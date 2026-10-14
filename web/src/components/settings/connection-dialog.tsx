import { useState } from 'react'
import type { Configuration, CredentialUpdate, ManagedConnection } from '../../api/schema'
import type { PrunedReferences } from '../../lib/settings'
import { hasPrunedReferences, prunedReferences, pruneScopes } from '../../lib/settings'
import { ConnectionEditor } from '../connection-editor'
import { FormError } from '../form'
import { SettingsEditor } from '../settings-editor'
import { Button } from '../ui/button'
import { PrunedReferenceList } from './pruned-references'

interface PendingSave {
  configuration: Configuration
  credentials: readonly CredentialUpdate[]
  references: PrunedReferences
}

// ConnectionDialog edits an enrolled connection. A change that drops a namespace still
// referenced by access grants or relationships is confirmed before those are removed; the
// editor and its input stay open until the save completes or is abandoned.
export function ConnectionDialog({
  connection,
  configuration,
  pending,
  commit,
  onClose,
}: {
  connection: ManagedConnection
  configuration: Configuration
  pending: boolean
  commit: (configuration: Configuration, credentials?: readonly CredentialUpdate[]) => Promise<void>
  onClose: () => void
}) {
  const [confirming, setConfirming] = useState<PendingSave | null>(null)
  const [error, setError] = useState<unknown>(null)
  const cancel = () => {
    setConfirming(null)
    setError(null)
  }
  return (
    <>
      <ConnectionEditor
        initial={connection}
        onClose={onClose}
        onSave={async (next, credential) => {
          const connections = configuration.connections.map((item) =>
            item.id === next.id ? next : item,
          )
          const updated = { ...configuration, connections }
          const credentials = credential ? [credential] : []
          const references = prunedReferences(updated)
          if (hasPrunedReferences(references)) {
            setConfirming({ configuration: pruneScopes(updated), credentials, references })
            return false
          }
          await commit(updated, credentials)
          return true
        }}
      />
      {confirming && (
        <SettingsEditor title="Remove access and relationships?" onClose={cancel}>
          <p>
            Saving this cluster removes namespaces that the following access grants and
            relationships still use. They are removed with the change.
          </p>
          <PrunedReferenceList references={confirming.references} />
          <FormError error={error} />
          <div className="editor-actions">
            <Button variant="ghost" onClick={cancel}>
              Keep editing
            </Button>
            <Button
              variant="danger"
              disabled={pending}
              onClick={() => {
                setError(null)
                void commit(confirming.configuration, confirming.credentials).then(
                  onClose,
                  setError,
                )
              }}
            >
              Save and remove
            </Button>
          </div>
        </SettingsEditor>
      )}
    </>
  )
}
