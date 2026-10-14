import { Link } from '@tanstack/react-router'
import { AlertCircle, ArrowRight } from 'lucide-react'
import type { Snapshot } from '../../api/schema'
import { operatorNeedsAttention } from '../../lib/tone'

function plural(count: number, noun: string): string {
  return `${String(count)} ${noun}${count === 1 ? '' : 's'}`
}

// EvidenceBanners warn that findings are incomplete when clusters or controllers lack
// current evidence.
export function EvidenceBanners({ snapshot }: { snapshot: Snapshot }) {
  const incomplete = snapshot.connections.filter(
    (connection) => connection.collection !== 'current',
  )
  const operators = snapshot.connections
    .flatMap((connection) => connection.operators)
    .filter(operatorNeedsAttention)
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
          <Link to="/clusters">
            Inspect clusters <ArrowRight size={14} />
          </Link>
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
          <Link to="/clusters">
            Inspect operators <ArrowRight size={14} />
          </Link>
        </div>
      )}
    </>
  )
}
