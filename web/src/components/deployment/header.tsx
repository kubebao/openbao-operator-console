import { Link } from '@tanstack/react-router'
import { Boxes, Cable, Cpu, Database, Folder, Layers3 } from 'lucide-react'
import type { Deployment, Operator, Snapshot } from '../../api/schema'
import { environmentLabels } from '../../lib/format'
import { EvidenceBadge, HealthBadge, OperatorHealthBadge, Timestamp } from '../status'

// DeploymentHeader identifies a deployment, its responsible operator, and its collection.
export function DeploymentHeader({
  deployment,
  connection,
  operator,
}: {
  deployment: Deployment
  connection: Snapshot['connections'][number] | undefined
  operator: Operator | undefined
}) {
  return (
    <header className="deployment-header">
      <div className="page-title">
        <h1>{deployment.name}</h1>
        <HealthBadge health={deployment.health} />
      </div>
      <ul className="deployment-meta" aria-label="Deployment details">
        <li title="Namespace">
          <Folder size={14} aria-hidden="true" />
          <span className="sr-only">Namespace </span>
          {deployment.namespace}
        </li>
        <li title="Team">
          <Boxes size={14} aria-hidden="true" />
          <span className="sr-only">Team </span>
          {deployment.team}
        </li>
        <li title="Environment">
          <Layers3 size={14} aria-hidden="true" />
          <span className="sr-only">Environment </span>
          {environmentLabels[deployment.environment]}
        </li>
        <li title="Kubernetes cluster">
          <Cable size={14} aria-hidden="true" />
          <span className="sr-only">Kubernetes cluster </span>
          {connection?.name ?? deployment.connectionId}
          {deployment.location && <span className="muted"> · {deployment.location}</span>}
        </li>
        <li title="OpenBao version">
          <Database size={14} aria-hidden="true" />
          OpenBao {deployment.version || 'version not observed'}
        </li>
      </ul>
      <div className="deployment-context">
        <span>
          <Cpu size={14} aria-hidden="true" />
          Managed by{' '}
          {operator ? (
            <Link to="/clusters" className="text-link">
              {operator.id}
            </Link>
          ) : (
            <span>{deployment.operatorId ?? 'no declared operator'}</span>
          )}
          {operator && <OperatorHealthBadge health={operator.health} />}
        </span>
        <span>
          {deployment.evidence.collection === 'current' ? (
            <>
              Last collected: <Timestamp value={deployment.evidence.collectedAt} />
            </>
          ) : (
            <EvidenceBadge collection={deployment.evidence.collection} />
          )}
        </span>
      </div>
    </header>
  )
}
