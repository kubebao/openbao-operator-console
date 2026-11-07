import { Cpu } from 'lucide-react'
import type { Operator } from '../api/schema'
import type { DetailRow } from './page'
import { DetailRows } from './page'
import { InfoTip } from './info-tip'
import { operatorHealthLabels } from '../lib/format'
import { OperatorCollectionBadge, OperatorHealthBadge, Timestamp } from './status'

export function OperatorEvidence({ operator }: { operator: Operator }) {
  return (
    <section className="operator-evidence">
      <div className="operator-heading">
        <h3>
          <Cpu size={16} />
          {operator.id}
        </h3>
        <OperatorHealthBadge operator={operator} />
        <OperatorCollectionBadge collection={operator.collection} />
        <InfoTip label="About operator evidence">
          {operator.collection !== 'current' ? 'Workload facts are last known. ' : ''}
          Deployment readiness does not prove successful reconciliation. The version label does not
          verify the binary.
        </InfoTip>
      </div>
      <OperatorRows operator={operator} />
    </section>
  )
}

// OperatorRows lists the controller facts collected from the enrolled Deployment.
export function OperatorRows({ operator }: { operator: Operator }) {
  const rows: DetailRow[] = [
    { label: 'Controller Deployment', value: `${operator.namespace} / ${operator.name}` },
    { label: 'Declared responsibility', value: operator.namespaces.join(', ') },
    ...(operator.collection === 'current'
      ? []
      : [{ label: 'Last reported health', value: operatorHealthLabels[operator.health] }]),
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
  ]
  return <DetailRows rows={rows} />
}
