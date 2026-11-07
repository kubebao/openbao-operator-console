import { Link } from '@tanstack/react-router'
import { AlertCircle, ArrowRight } from 'lucide-react'
import type { Snapshot } from '../../api/schema'
import { plural } from '../../lib/format'
import { operatorNeedsAttention } from '../../lib/tone'

// ClusterLink opens the one affected cluster directly, or the topology map for several.
function ClusterLink({
  clusters,
  children,
}: {
  clusters: readonly Snapshot['connections'][number][]
  children: string
}) {
  const [only, ...others] = clusters
  return only && others.length === 0 ? (
    <Link to="/topology/clusters/$clusterId" params={{ clusterId: only.id }}>
      {children} <ArrowRight size={14} />
    </Link>
  ) : (
    <Link to="/topology">
      {children} <ArrowRight size={14} />
    </Link>
  )
}

// EvidenceBanners warn that findings are incomplete when clusters or controllers lack
// current evidence.
export function EvidenceBanners({ snapshot }: { snapshot: Snapshot }) {
  const incomplete = snapshot.connections.filter(
    (connection) => connection.collection !== 'current',
  )
  const operatorClusters = snapshot.connections.filter((connection) =>
    connection.operators.some(operatorNeedsAttention),
  )
  const operators = operatorClusters.flatMap((connection) =>
    connection.operators.filter(operatorNeedsAttention),
  )
  return (
    <>
      {incomplete.length > 0 && (
        <div className="evidence-banner">
          <AlertCircle size={19} />
          <span>
            <strong>Collection needs attention</strong> ·{' '}
            {plural(incomplete.length, 'Kubernetes cluster')} cannot provide current evidence.
            Undiscovered deployments cannot be assessed.
          </span>
          <ClusterLink clusters={incomplete}>Inspect clusters</ClusterLink>
        </div>
      )}
      {operators.length > 0 && (
        <div className="evidence-banner">
          <AlertCircle size={19} />
          <span>
            <strong>Controller workload needs attention</strong> ·{' '}
            {plural(operators.length, 'enrolled operator')} unavailable, progressing, or without
            current evidence.
          </span>
          <ClusterLink clusters={operatorClusters}>Inspect operators</ClusterLink>
        </div>
      )}
    </>
  )
}
