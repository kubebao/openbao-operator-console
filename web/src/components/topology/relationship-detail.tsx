import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { ArrowRight, Database, HardDrive } from 'lucide-react'
import type { Deployment, Relationship, Snapshot } from '../../api/schema'
import { provenanceLabels, recoveryPhases, relationshipKindLabels } from '../../lib/format'
import { DetailRows } from '../page'
import { HealthBadge, Timestamp } from '../status'

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
  const connection = snapshot.connections.find((item) => item.id === deployment.connectionId)
  return (
    <button
      className={`relationship-node ${selected ? 'relationship-node-selected' : ''}`}
      onClick={onSelect}
      aria-pressed={selected}
    >
      <span className="sr-only">Inspect </span>
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

interface Rows {
  label: string
  value: ReactNode
}

// pathRows adds the evidence each relationship kind depends on.
function pathRows(
  relationship: Relationship,
  selected: Deployment,
  source: Deployment,
  target: Deployment,
): Rows[] {
  if (relationship.kind === 'snapshotRecovery')
    return [
      {
        label: 'Last successful backup',
        value: <Timestamp value={source.backup?.lastSuccessAt ?? null} />,
      },
      {
        label: 'Target restore',
        value: target.recovery ? recoveryPhases[target.recovery.phase] : 'Not observed',
      },
    ]
  if (!selected.replica) return []
  return [
    { label: 'Replica observed', value: <Timestamp value={selected.replica.observedAt} /> },
    { label: 'Local Raft index', value: <code>{selected.replica.localIndex ?? 'Unknown'}</code> },
    { label: 'Source Raft index', value: <code>{selected.replica.sourceIndex ?? 'Unknown'}</code> },
  ]
}

function Inspector({
  relationship,
  selected,
  source,
  target,
}: {
  relationship: Relationship
  selected: Deployment
  source: Deployment
  target: Deployment
}) {
  return (
    <div className="relationship-inspector">
      <div>
        <h3>
          {selected.namespace}/{selected.name}
        </h3>
        <DetailRows
          rows={[
            { label: 'State', value: <HealthBadge health={selected.health} /> },
            { label: 'OpenBao version', value: selected.version || 'Unknown' },
            { label: 'Last collected', value: <Timestamp value={selected.evidence.collectedAt} /> },
            ...pathRows(relationship, selected, source, target),
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
  )
}

export function RelationshipDetail({
  relationship,
  snapshot,
  source,
  target,
  selected,
  onSelect,
}: {
  relationship: Relationship
  snapshot: Snapshot
  source: Deployment
  target: Deployment
  selected: Deployment
  onSelect: (id: string) => void
}) {
  const replication = relationship.kind === 'replication'
  return (
    <section className="relationship-detail">
      <div className="section-heading">
        <h2>{relationshipKindLabels[relationship.kind]}</h2>
        <span className="muted">{provenanceLabels[relationship.provenance]}</span>
      </div>
      <div className="relationship-path">
        <DeploymentNode
          deployment={source}
          snapshot={snapshot}
          label="Source"
          selected={selected.id === source.id}
          onSelect={() => {
            onSelect(source.id)
          }}
        />
        <div className="relationship-arrow">
          <span>{replication ? 'Replication' : 'Snapshot'}</span>
          <ArrowRight />
        </div>
        {!replication && (
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
          label={replication ? 'Read replica' : 'Restore target'}
          selected={selected.id === target.id}
          onSelect={() => {
            onSelect(target.id)
          }}
        />
      </div>
      <Inspector relationship={relationship} selected={selected} source={source} target={target} />
    </section>
  )
}
