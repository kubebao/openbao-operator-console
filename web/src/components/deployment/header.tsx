import { Link } from '@tanstack/react-router'
import { Boxes, Cable, Cpu, Database, Folder, Layers3 } from 'lucide-react'
import type { Deployment, Operator, Snapshot } from '../../api/schema'
import { environmentLabels, producerFreshness, producerFreshnessLabels } from '../../lib/format'
import { producerFreshnessTone } from '../../lib/tone'
import {
  EvidenceBadge,
  HealthBadge,
  OperatorCollectionBadge,
  OperatorHealthBadge,
  StatusBadge,
  Timestamp,
} from '../status'

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
            <Link
              to="/topology/clusters/$clusterId"
              params={{ clusterId: deployment.connectionId }}
              className="text-link"
            >
              {operator.id}
            </Link>
          ) : (
            <span>{deployment.operatorId ?? 'no declared operator'}</span>
          )}
          {operator && <OperatorHealthBadge operator={operator} />}
          {operator && operator.collection !== 'current' && (
            <OperatorCollectionBadge collection={operator.collection} />
          )}
        </span>
        <span>
          {deployment.evidence.collection === 'current' ? (
            <>
              Collected <Timestamp value={deployment.evidence.collectedAt} />
            </>
          ) : (
            <>
              Collection <EvidenceBadge collection={deployment.evidence.collection} />
            </>
          )}
        </span>
        <ProducerEvidence evidence={deployment.evidence} />
      </div>
    </header>
  )
}

// ProducerEvidence shows when the operator last reported status, separately from when the
// console collected it. Without a reported time, freshness is unknown, and the missing time
// says so on its own.
function ProducerEvidence({ evidence }: { evidence: Deployment['evidence'] }) {
  const freshness = producerFreshness(evidence)
  if (evidence.producerObservedAt === null)
    return (
      <span>
        <StatusBadge tone={producerFreshnessTone[freshness]}>
          Producer time not reported
        </StatusBadge>
      </span>
    )
  return (
    <span>
      Producer reported <Timestamp value={evidence.producerObservedAt} />
      <StatusBadge tone={producerFreshnessTone[freshness]}>
        {producerFreshnessLabels[freshness]}
      </StatusBadge>
    </span>
  )
}
