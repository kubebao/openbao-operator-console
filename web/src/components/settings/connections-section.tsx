import { Cable, Pause, Play, Plus } from 'lucide-react'
import { useNavigate } from '@tanstack/react-router'
import type { Configuration, CredentialStatus, ManagedConnection } from '../../api/schema'
import { plural } from '../../lib/format'
import { CredentialStatusLabel } from '../credential-renewal'
import { EmptyState } from '../page'
import { StatusBadge } from '../status'
import { Button } from '../ui/button'

export function ConnectionsSection({
  configuration,
  credentials,
  credentialsUnavailable,
  pending,
  onToggle,
  onEdit,
  onRemove,
}: {
  configuration: Configuration
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
        <h2>{plural(count, 'cluster')}</h2>
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
          const credential = credentials.find((status) => status.connectionId === item.id)
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
              {/* Collection status lives on the Clusters page; this row edits configuration. */}
              <div className="settings-row-state">
                {item.disabled && <StatusBadge tone="unknown">Paused</StatusBadge>}
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
