import type { ManagedConnection } from '../../api/schema'
import type { CredentialDraft } from '../../lib/credential'
import { CredentialFields } from '../credential-fields'

export function CredentialsStep({
  connection,
  draft,
  onConnection,
  onDraft,
  onError,
}: {
  connection: ManagedConnection
  draft: CredentialDraft
  onConnection: (connection: ManagedConnection) => void
  onDraft: (draft: CredentialDraft) => void
  onError: (message: string) => void
}) {
  if (connection.authentication === 'local')
    return (
      <>
        <h2>Use the hosting cluster identity</h2>
        <p>
          The console uses its mounted Kubernetes ServiceAccount token. Continue to check its access
          to the enrolled namespaces.
        </p>
      </>
    )
  return (
    <>
      <h2>Add reader credentials</h2>
      <CredentialFields
        connection={connection}
        draft={draft}
        required
        onConnection={onConnection}
        onDraft={onDraft}
        onError={onError}
      />
    </>
  )
}
