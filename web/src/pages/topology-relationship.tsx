import { Link, useParams } from '@tanstack/react-router'
import { ArrowLeft, ArrowRight } from 'lucide-react'
import { useSnapshot } from '../api/client'
import type { Deployment, Snapshot } from '../api/schema'
import { Breadcrumbs } from '../components/breadcrumbs'
import { ReplicationSection } from '../components/deployment/lifecycle'
import { PageHeading } from '../components/page'
import {
  PathDeployment,
  PathEvidence,
  RecordedEvidence,
  RecoveryDiagram,
  RestoreTestEvidence,
} from '../components/recovery/recovery-evidence'
import { useDocumentTitle } from '../components/use-document-title'
import { provenanceLabels, relationshipKindLabels } from '../lib/format'

const titles = { replication: 'Read replica', snapshotRecovery: 'Recovery path' } as const

function ClusterCrumb({ deployment, snapshot }: { deployment: Deployment; snapshot: Snapshot }) {
  const connection = snapshot.connections.find((item) => item.id === deployment.connectionId)
  return (
    <Link to="/topology/clusters/$clusterId" params={{ clusterId: deployment.connectionId }}>
      {connection?.name ?? deployment.connectionId}
    </Link>
  )
}

function ReplicaView({
  source,
  target,
  snapshot,
}: {
  source: Deployment
  target: Deployment
  snapshot: Snapshot
}) {
  return (
    <>
      <div className="recovery-path replica-path">
        <PathDeployment role="Source" deployment={source} snapshot={snapshot} />
        <ArrowRight className="path-arrow" size={25} />
        <PathDeployment role="Read replica" deployment={target} snapshot={snapshot} />
      </div>
      {target.replica ? (
        <ReplicationSection deployment={target} replica={target.replica} />
      ) : (
        <section className="detail-section">
          <h2>Replication</h2>
          <p className="muted">The read replica reports no replication evidence.</p>
        </section>
      )}
    </>
  )
}

// TopologyRelationshipPage is the third topology level: one declared relationship and the
// evidence its kind depends on.
export function TopologyRelationshipPage() {
  const { relationshipId } = useParams({ from: '/topology/relationships/$relationshipId' })
  const { data: snapshot } = useSnapshot()
  const relationship = snapshot?.relationships.find((item) => item.id === relationshipId)
  const source = snapshot?.deployments.find((item) => item.id === relationship?.sourceId)
  const target = snapshot?.deployments.find((item) => item.id === relationship?.targetId)
  useDocumentTitle(relationship ? titles[relationship.kind] : 'Relationship')
  if (!snapshot) return null
  if (!relationship || !source || !target)
    return (
      <div className="page-enter">
        <Link to="/topology" className="back-link">
          <ArrowLeft size={14} />
          Back to topology
        </Link>
        <PageHeading
          title="Relationship unavailable"
          description="The relationship is missing or outside your access scope."
        />
      </div>
    )
  const crossCluster = source.connectionId !== target.connectionId
  return (
    <div className="page-enter">
      <Breadcrumbs>
        {[
          <Link key="topology" to="/topology">
            Topology
          </Link>,
          <span key="clusters" className="breadcrumb-clusters">
            <ClusterCrumb deployment={source} snapshot={snapshot} />
            {crossCluster && (
              <>
                <ArrowRight size={12} aria-label="to" />
                <ClusterCrumb deployment={target} snapshot={snapshot} />
              </>
            )}
          </span>,
          relationshipKindLabels[relationship.kind],
        ]}
      </Breadcrumbs>
      <PageHeading title={titles[relationship.kind]} />
      <div className="summary-line">
        <span>
          {source.namespace}/{source.name} → {target.namespace}/{target.name}
        </span>
        <span>{provenanceLabels[relationship.provenance]}</span>
      </div>
      {relationship.kind === 'snapshotRecovery' ? (
        <div className="recovery-layout">
          <div>
            <RecoveryDiagram
              route={relationship}
              source={source}
              target={target}
              snapshot={snapshot}
            />
            <RecordedEvidence source={source} target={target} />
            <RestoreTestEvidence source={source} />
          </div>
          <PathEvidence route={relationship} target={target} />
        </div>
      ) : (
        <ReplicaView source={source} target={target} snapshot={snapshot} />
      )}
    </div>
  )
}
