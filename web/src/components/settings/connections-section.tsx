import { Cable, Pause, Play, Plus } from 'lucide-react'
import { useNavigate } from '@tanstack/react-router'
import type { Configuration, CredentialStatus, ManagedConnection, Snapshot } from '../../api/schema'
import { collectionLabels } from '../../lib/format'
import { CredentialStatusLabel } from '../credential-renewal'
import { EmptyState } from '../page'
import { Timestamp } from '../status'
import { Button } from '../ui/button'

type ObservedConnection = Snapshot['connections'][number]

// collectionBadge describes what the console currently knows about an enrolled cluster.
function collectionBadge(connection: ManagedConnection, observed: ObservedConnection | undefined) {
  if (connection.disabled) return { tone: 'status-unknown', label: 'Paused' }
  const collection = observed?.collection ?? 'pending'
  if (collection === 'current') return { tone: 'status-healthy', label: 'Connected' }
  if (collection === 'pending') return { tone: 'status-unknown', label: collectionLabels.pending }
  return { tone: 'status-attention', label: collectionLabels[collection] }
}

function CollectionDetail({
  connection,
  observed,
}: {
  connection: ManagedConnection
  observed: ObservedConnection | undefined
}) {
  if (connection.disabled) return <small>Collection stopped</small>
  if (!observed?.lastSuccessAt) return <small>No successful collection yet</small>
  return (
    <small>
      Last collected <Timestamp value={observed.lastSuccessAt} />
    </small>
  )
}

export function ConnectionsSection({
  configuration,
  snapshot,
  credentials,
  credentialsUnavailable,
  pending,
  onToggle,
  onEdit,
  onRemove,
}: {
  configuration: Configuration
  snapshot: Snapshot | undefined
  credentials: readonly CredentialStatus[]
  credentialsUnavailable: boolean
  pending: boolean
  onToggle: (connection: ManagedConnection) => void
  onEdit: (connection: ManagedConnection) => void
  onRemove: (connection: ManagedConnection) => void
}) {
  const navigate = useNavigate()
  const count = configuration.connections.length
  return (
    <section>
      <div className="section-heading">
        <h2>
          {count} {count === 1 ? 'cluster' : 'clusters'}
        </h2>
        <Button
          onClick={() => {
            void navigate({ to: '/settings/clusters/new' })
          }}
        >
          <Plus size={15} />
          Add cluster
        </Button>
      </div>
      <div className="settings-list">
        {configuration.connections.map((item) => {
          const observed = snapshot?.connections.find((connection) => connection.id === item.id)
          const credential = credentials.find((status) => status.connectionId === item.id)
          const badge = collectionBadge(item, observed)
          return (
            <article key={item.id} className="settings-row">
              <div>
                <h3>{item.name}</h3>
                <p>
                  {[
                    item.authentication === 'local' ? 'Hosting cluster' : item.server,
                    item.region,
                    item.id,
                  ]
                    .filter(Boolean)
                    .join(' · ')}
                </p>
                <div className="namespace-tags">
                  {item.namespaces.map((namespace) => (
                    <span key={namespace.name}>{namespace.name}</span>
                  ))}
                </div>
              </div>
              <div className="settings-row-state">
                <span className={`badge ${badge.tone}`}>
                  <span className="status-dot" />
                  {badge.label}
                </span>
                <CollectionDetail connection={item} observed={observed} />
                <CredentialStatusLabel status={credential} />
                {item.renewal && credentialsUnavailable && (
                  <small>Renewal status unavailable</small>
                )}
              </div>
              <div className="row-actions">
                <Button
                  size="small"
                  variant="ghost"
                  disabled={pending}
                  aria-label={`${item.disabled ? 'Resume' : 'Pause'} ${item.name}`}
                  onClick={() => {
                    onToggle(item)
                  }}
                >
                  {item.disabled ? <Play size={14} /> : <Pause size={14} />}
                  {item.disabled ? 'Resume' : 'Pause'}
                </Button>
                <Button
                  size="small"
                  onClick={() => {
                    onEdit(item)
                  }}
                >
                  {credential?.state === 'reconnectRequired' ? 'Reconnect' : 'Edit'}
                </Button>
                <Button
                  size="small"
                  variant="ghost"
                  onClick={() => {
                    onRemove(item)
                  }}
                >
                  Remove
                </Button>
              </div>
            </article>
          )
        })}
      </div>
      {count === 0 && (
        <EmptyState title="No clusters connected" icon={Cable}>
          Add a cluster to start collecting OpenBao resources.
        </EmptyState>
      )}
    </section>
  )
}
