import { Link } from '@tanstack/react-router'
import { ArrowRight, Database, HardDrive } from 'lucide-react'
import type { Deployment, Relationship, Snapshot } from '../../api/schema'
import { provenanceLabels } from '../../lib/format'
import { DetailRows } from '../page'
import { HealthBadge, Timestamp } from '../status'

function PathDeployment({
  role,
  deployment,
  snapshot,
}: {
  role: string
  deployment: Deployment
  snapshot: Snapshot
}) {
  const connection = snapshot.connections.find((item) => item.id === deployment.connectionId)
  return (
    <div className="recovery-deployment">
      <span className="path-role">{role}</span>
      <Database size={25} />
      <h2>
        {deployment.namespace}/{deployment.name}
      </h2>
      <p>{connection?.name ?? deployment.connectionId}</p>
      <HealthBadge health={deployment.health} />
    </div>
  )
}

export function RecoveryDiagram({
  route,
  source,
  target,
  snapshot,
}: {
  route: Relationship
  source: Deployment
  target: Deployment
  snapshot: Snapshot
}) {
  return (
    <div className="recovery-path">
      <PathDeployment role="Source" deployment={source} snapshot={snapshot} />
      <ArrowRight className="path-arrow" size={25} />
      <div className="recovery-repository">
        <HardDrive size={27} />
        <h3>Snapshot repository</h3>
        <code>{route.repository ?? 'Configured storage'}</code>
        <small>Current readability unknown</small>
      </div>
      <ArrowRight className="path-arrow dashed" size={25} />
      <PathDeployment role="Restore target" deployment={target} snapshot={snapshot} />
    </div>
  )
}

export function RecordedEvidence({ source, target }: { source: Deployment; target: Deployment }) {
  const backup = source.backup
  const recovery = target.recovery
  return (
    <section className="detail-section">
      <h2>Recorded evidence</h2>
      <DetailRows
        rows={[
          {
            label: 'Source backup completed',
            value: <Timestamp value={backup?.lastSuccessAt ?? null} />,
          },
          {
            label: 'Source snapshot digest',
            value: backup?.digest ? <code>{backup.digest}</code> : 'Not observed',
          },
          {
            label: 'Target application confirmed',
            value: <Timestamp value={recovery?.applicationConfirmedAt ?? null} />,
          },
          {
            label: 'Target execution completed',
            value: <Timestamp value={recovery?.completedAt ?? null} />,
          },
        ]}
      />
      <details className="evidence-help">
        <summary>How to interpret this evidence</summary>
        <p>
          These observations can refer to different snapshots. The operator does not supply a shared
          recovery receipt or a measured recovery point objective.
        </p>
      </details>
    </section>
  )
}

export function RestoreTestEvidence({ source }: { source: Deployment }) {
  const test = source.backup?.restoreTest
  return (
    <section className="detail-section">
      <h2>Latest restore test</h2>
      <DetailRows
        rows={[
          { label: 'Result', value: test?.outcome ?? 'Not observed' },
          { label: 'Finished', value: <Timestamp value={test?.finishedAt ?? null} /> },
          {
            label: 'Last successful test',
            value: <Timestamp value={test?.lastSuccessAt ?? null} />,
          },
        ]}
      />
      <details className="evidence-help">
        <summary>Test coverage</summary>
        <p>
          Scheduled disposable tests check pinned snapshot bytes, source identity, and healthy
          target leadership in one Kubernetes cluster. They do not check client login or secret
          reads.
        </p>
      </details>
    </section>
  )
}

export function PathEvidence({
  route,
  source,
  target,
}: {
  route: Relationship
  source: Deployment
  target: Deployment
}) {
  const recovery = target.recovery
  return (
    <aside className="topology-inspector">
      <h2>Path evidence</h2>
      <DetailRows
        rows={[
          { label: 'Relationship', value: provenanceLabels[route.provenance] },
          {
            label: 'Backup completion age',
            value: <Timestamp value={source.backup?.lastSuccessAt ?? null} />,
          },
          { label: 'Measured RPO', value: 'Not supplied' },
          {
            label: 'Latest restore-test pass',
            value: <Timestamp value={source.backup?.restoreTest?.lastSuccessAt ?? null} />,
          },
          { label: 'Target request phase', value: recovery?.phase ?? 'Not observed' },
          {
            label: 'Application receipt',
            value: <Timestamp value={recovery?.applicationConfirmedAt ?? null} />,
          },
          {
            label: 'Source exclusion',
            value:
              recovery?.exclusionAcknowledged === true
                ? 'Administrator acknowledgment'
                : 'No observed evidence',
          },
          { label: 'Physical fencing', value: 'Not observed' },
          { label: 'Client cutover', value: 'Not observed' },
        ]}
      />
      <Link
        to="/deployments/$deploymentId"
        params={{ deploymentId: target.id }}
        className="text-link"
      >
        Inspect target
        <ArrowRight size={15} />
      </Link>
    </aside>
  )
}
