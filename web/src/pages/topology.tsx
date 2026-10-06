import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { ArrowRight, Database, GitBranch, HardDrive } from 'lucide-react'
import { useSnapshot } from '../api/client'
import type { Deployment, Snapshot } from '../api/schema'
import { PageHeading, DetailRows, EmptyState } from '../components/page'
import { HealthBadge, Timestamp } from '../components/status'
import { provenanceLabels, recoveryPhases } from '../lib/format'

function DeploymentNode({
  deployment,
  snapshot,
  label,
  selected,
  onSelect,
}: {
  deployment: Deployment
  snapshot: Snapshot
  label: string
  selected: boolean
  onSelect: () => void
}) {
  const connection = snapshot.connections.find(
    (connection) => connection.id === deployment.connectionId,
  )
  return (
    <button
      className={`relationship-node ${selected ? 'relationship-node-selected' : ''}`}
      onClick={onSelect}
      aria-label={`Inspect ${deployment.namespace}/${deployment.name}`}
      aria-pressed={selected}
    >
      <span className="path-role">{label}</span>
      <Database size={22} />
      <strong>
        {deployment.namespace}/{deployment.name}
      </strong>
      <small>{connection?.name ?? deployment.connectionId}</small>
      <div className="node-state">
        <HealthBadge health={deployment.health} />
        <span>{deployment.version}</span>
      </div>
    </button>
  )
}
export function TopologyPage() {
  const { data: snapshot } = useSnapshot()
  const search = useSearch({ from: '/topology' })
  const navigate = useNavigate()
  if (!snapshot) return null
  const relationships = snapshot.relationships.filter(
    (relationship) => search.kind === 'all' || relationship.kind === search.kind,
  )
  const relationship =
    relationships.find((relationship) => relationship.id === search.relationship) ??
    relationships[0]
  const source = snapshot.deployments.find((deployment) => deployment.id === relationship?.sourceId)
  const target = snapshot.deployments.find((deployment) => deployment.id === relationship?.targetId)
  const selected =
    [source, target].find((deployment) => deployment?.id === search.deployment) ?? source
  const select = (id: string) => {
    void navigate({
      to: '/topology',
      search: { ...search, relationship: relationship?.id ?? '', deployment: id },
    })
  }
  return (
    <div className="page-enter">
      <PageHeading title="Topology" />
      <div className="table-toolbar">
        <div className="segmented" aria-label="Relationship type">
          {(
            [
              { kind: 'all', label: 'All relationships' },
              { kind: 'replication', label: 'Read replicas' },
              { kind: 'snapshotRecovery', label: 'Recovery' },
            ] as const
          ).map((filter) => (
            <button
              key={filter.kind}
              className={search.kind === filter.kind ? 'selected' : ''}
              onClick={() => {
                void navigate({
                  to: '/topology',
                  search: { kind: filter.kind, relationship: '', deployment: '' },
                })
              }}
            >
              {filter.label}
              <span>
                {
                  snapshot.relationships.filter(
                    (relationship) => filter.kind === 'all' || relationship.kind === filter.kind,
                  ).length
                }
              </span>
            </button>
          ))}
        </div>
        {snapshot.viewer.admin && (
          <Link to="/settings" search={{ section: 'relationships' }} className="text-link">
            Manage relationships
            <ArrowRight size={14} />
          </Link>
        )}
      </div>
      {relationship && source && target ? (
        <div className="relationship-workspace">
          <nav className="relationship-list" aria-label="Relationships">
            {relationships.map((item) => {
              const from = snapshot.deployments.find(
                (deployment) => deployment.id === item.sourceId,
              )
              const to = snapshot.deployments.find((deployment) => deployment.id === item.targetId)
              return (
                <button
                  key={item.id}
                  aria-label={`Select ${item.id}`}
                  className={item.id === relationship.id ? 'selected' : ''}
                  onClick={() => {
                    void navigate({
                      to: '/topology',
                      search: { ...search, relationship: item.id, deployment: '' },
                    })
                  }}
                >
                  <strong>
                    {from?.namespace}/{from?.name}
                    <ArrowRight size={14} />
                    {to?.namespace}/{to?.name}
                  </strong>
                  <small>
                    {item.kind === 'replication' ? 'Remote read replica' : 'Snapshot recovery'} ·{' '}
                    {provenanceLabels[item.provenance]}
                  </small>
                </button>
              )
            })}
          </nav>
          <section className="relationship-detail">
            <div className="section-heading">
              <h2>
                {relationship.kind === 'replication' ? 'Remote read replica' : 'Snapshot recovery'}
              </h2>
              <span className="muted">{provenanceLabels[relationship.provenance]}</span>
            </div>
            <div className="relationship-path">
              <DeploymentNode
                deployment={source}
                snapshot={snapshot}
                label="Source"
                selected={selected?.id === source.id}
                onSelect={() => {
                  select(source.id)
                }}
              />
              <div className="relationship-arrow">
                <span>{relationship.kind === 'replication' ? 'Replication' : 'Snapshot'}</span>
                <ArrowRight />
              </div>
              {relationship.kind === 'snapshotRecovery' && (
                <>
                  <div className="relationship-storage">
                    <HardDrive size={24} />
                    <strong>Snapshot storage</strong>
                    <code>{relationship.repository ?? 'Location not supplied'}</code>
                    <small>Readability unknown</small>
                  </div>
                  <div className="relationship-arrow">
                    <span>Restore</span>
                    <ArrowRight />
                  </div>
                </>
              )}
              <DeploymentNode
                deployment={target}
                snapshot={snapshot}
                label={relationship.kind === 'replication' ? 'Read replica' : 'Restore target'}
                selected={selected?.id === target.id}
                onSelect={() => {
                  select(target.id)
                }}
              />
            </div>
            {selected && (
              <div className="relationship-inspector">
                <div>
                  <h3>
                    {selected.namespace}/{selected.name}
                  </h3>
                  <DetailRows
                    rows={[
                      { label: 'State', value: <HealthBadge health={selected.health} /> },
                      { label: 'OpenBao version', value: selected.version || 'Unknown' },
                      {
                        label: 'Last collected',
                        value: <Timestamp value={selected.evidence.collectedAt} />,
                      },
                      ...(relationship.kind === 'replication' && selected.replica
                        ? [
                            {
                              label: 'Replica observed',
                              value: <Timestamp value={selected.replica.observedAt} />,
                            },
                            {
                              label: 'Local Raft index',
                              value: <code>{selected.replica.localIndex ?? 'Unknown'}</code>,
                            },
                            {
                              label: 'Source Raft index',
                              value: <code>{selected.replica.sourceIndex ?? 'Unknown'}</code>,
                            },
                          ]
                        : relationship.kind === 'snapshotRecovery'
                          ? [
                              {
                                label: 'Last successful backup',
                                value: <Timestamp value={source.backup?.lastSuccessAt ?? null} />,
                              },
                              {
                                label: 'Target restore',
                                value: target.recovery
                                  ? recoveryPhases[target.recovery.phase]
                                  : 'Not observed',
                              },
                            ]
                          : []),
                    ]}
                  />
                </div>
                <div className="inspector-links">
                  <Link
                    to="/deployments/$deploymentId"
                    params={{ deploymentId: selected.id }}
                    className="text-link"
                  >
                    View deployment
                    <ArrowRight size={14} />
                  </Link>
                  {relationship.kind === 'snapshotRecovery' && (
                    <Link
                      to="/recovery/$relationshipId"
                      params={{ relationshipId: relationship.id }}
                      className="text-link"
                    >
                      Inspect recovery path
                      <ArrowRight size={14} />
                    </Link>
                  )}
                </div>
              </div>
            )}
          </section>
        </div>
      ) : (
        <EmptyState title="No relationships configured" icon={GitBranch}>
          {snapshot.viewer.admin ? (
            <Link to="/settings" search={{ section: 'relationships' }}>
              Add a relationship
            </Link>
          ) : (
            'No relationships are configured within your access scope.'
          )}
        </EmptyState>
      )}
    </div>
  )
}
