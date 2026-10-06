import { Cpu } from 'lucide-react'
import type { Operator } from '../api/schema'
import { operatorHealthLabels } from '../lib/format'
import { DetailRows } from './page'
import { EvidenceBadge, Timestamp } from './status'

const healthStyle = {
  available: 'healthy',
  progressing: 'attention',
  unavailable: 'critical',
  unknown: 'unknown',
} as const

export function OperatorEvidence({ operator }: { operator: Operator }) {
  return (
    <section className="operator-evidence">
      <div className="operator-heading">
        <h3>
          <Cpu size={16} />
          {operator.id}
        </h3>
        <span className={`badge status-${healthStyle[operator.health]}`}>
          <span className="status-dot" />
          {operatorHealthLabels[operator.health]}
        </span>
        {operator.collection === 'notFound' || operator.collection === 'invalid' ? (
          <span className="evidence evidence-unsupported">
            {operator.collection === 'notFound'
              ? 'Deployment missing'
              : 'Check container enrollment'}
          </span>
        ) : (
          <EvidenceBadge collection={operator.collection} />
        )}
      </div>
      <DetailRows
        rows={[
          { label: 'Controller Deployment', value: `${operator.namespace} / ${operator.name}` },
          { label: 'Declared responsibility', value: operator.namespaces.join(', ') },
          { label: 'Version label', value: operator.version ?? 'Not supplied' },
          { label: 'Container image', value: <code>{operator.image ?? 'Not observed'}</code> },
          {
            label: 'Ready / desired replicas',
            value: `${String(operator.readyReplicas ?? '—')} / ${String(operator.desiredReplicas ?? '—')}`,
          },
          {
            label: 'Observed / requested generation',
            value: `${String(operator.observedGeneration ?? '—')} / ${String(operator.generation)}`,
          },
          {
            label: 'Last successful observation',
            value: <Timestamp value={operator.lastSuccessAt} />,
          },
        ]}
      />
      <p className="section-footnote">
        {operator.collection !== 'current' ? 'Workload facts are last known. ' : ''}
        Deployment readiness does not prove successful reconciliation. The version label does not
        verify the binary.
      </p>
    </section>
  )
}
