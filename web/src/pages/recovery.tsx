import { Link, useParams } from '@tanstack/react-router'
import { ArrowLeft, ArrowRight, Database, HardDrive, SearchX } from 'lucide-react'
import { useSnapshot } from '../api/client'
import { DetailRows, EmptyState, PageHeading } from '../components/page'
import { HealthBadge, Timestamp } from '../components/status'
import { provenanceLabels } from '../lib/format'

export function RecoveryPage() {
  const { relationshipId } = useParams({ from: '/recovery/$relationshipId' })
  const { data: snapshot } = useSnapshot()
  const route = snapshot?.relationships.find(
    (relationship) =>
      relationship.id === relationshipId && relationship.kind === 'snapshotRecovery',
  )
  const source = snapshot?.deployments.find((deployment) => deployment.id === route?.sourceId)
  const target = snapshot?.deployments.find((deployment) => deployment.id === route?.targetId)
  if (!route || !source || !target)
    return (
      <EmptyState title="Recovery path unavailable" icon={SearchX}>
        The path is missing or outside your access scope.
      </EmptyState>
    )
  const test = source.backup?.restoreTest
  return (
    <div className="page-enter">
      <Link to="/topology" className="back-link">
        <ArrowLeft size={14} />
        Back to topology
      </Link>
      <PageHeading title="Recovery path" />
      <div className="recovery-layout">
        <div>
          <div className="recovery-path">
            <div className="recovery-deployment">
              <span className="path-role">Source</span>
              <Database size={25} />
              <h2>
                {source.namespace}/{source.name}
              </h2>
              <p>
                {snapshot?.connections.find((item) => item.id === source.connectionId)?.name ??
                  source.connectionId}
              </p>
              <HealthBadge health={source.health} />
            </div>
            <ArrowRight className="path-arrow" size={25} />
            <div className="recovery-repository">
              <HardDrive size={27} />
              <h3>Snapshot repository</h3>
              <code>{route.repository ?? 'Configured storage'}</code>
              <small>Current readability unknown</small>
            </div>
            <ArrowRight className="path-arrow dashed" size={25} />
            <div className="recovery-deployment">
              <span className="path-role">Restore target</span>
              <Database size={25} />
              <h2>
                {target.namespace}/{target.name}
              </h2>
              <p>
                {snapshot?.connections.find((item) => item.id === target.connectionId)?.name ??
                  target.connectionId}
              </p>
              <HealthBadge health={target.health} />
            </div>
          </div>
          <section className="detail-section">
            <h2>Recorded evidence</h2>
            <DetailRows
              rows={[
                {
                  label: 'Source backup completed',
                  value: <Timestamp value={source.backup?.lastSuccessAt ?? null} />,
                },
                {
                  label: 'Source snapshot digest',
                  value: source.backup?.digest ? (
                    <code>{source.backup.digest}</code>
                  ) : (
                    'Not observed'
                  ),
                },
                {
                  label: 'Target application confirmed',
                  value: <Timestamp value={target.recovery?.applicationConfirmedAt ?? null} />,
                },
                {
                  label: 'Target execution completed',
                  value: <Timestamp value={target.recovery?.completedAt ?? null} />,
                },
              ]}
            />
            <details className="evidence-help">
              <summary>How to interpret this evidence</summary>
              <p>
                These observations can refer to different snapshots. The operator does not supply a
                shared recovery receipt or a measured recovery point objective.
              </p>
            </details>
          </section>
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
                target leadership in one Kubernetes cluster. They do not check client login or
                secret reads.
              </p>
            </details>
          </section>
        </div>
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
                value: <Timestamp value={test?.lastSuccessAt ?? null} />,
              },
              { label: 'Target request phase', value: target.recovery?.phase ?? 'Not observed' },
              {
                label: 'Application receipt',
                value: <Timestamp value={target.recovery?.applicationConfirmedAt ?? null} />,
              },
              {
                label: 'Source exclusion',
                value:
                  target.recovery?.exclusionAcknowledged === true
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
      </div>
    </div>
  )
}
