import { Link, useParams } from '@tanstack/react-router'
import { ArrowLeft, SearchX } from 'lucide-react'
import { useSnapshot } from '../api/client'
import { EmptyState, PageHeading } from '../components/page'
import {
  PathEvidence,
  RecordedEvidence,
  RecoveryDiagram,
  RestoreTestEvidence,
} from '../components/recovery/recovery-evidence'

export function RecoveryPage() {
  const { relationshipId } = useParams({ from: '/recovery/$relationshipId' })
  const { data: snapshot } = useSnapshot()
  const route = snapshot?.relationships.find(
    (relationship) =>
      relationship.id === relationshipId && relationship.kind === 'snapshotRecovery',
  )
  const source = snapshot?.deployments.find((deployment) => deployment.id === route?.sourceId)
  const target = snapshot?.deployments.find((deployment) => deployment.id === route?.targetId)
  if (!snapshot || !route || !source || !target)
    return (
      <EmptyState title="Recovery path unavailable" icon={SearchX}>
        The path is missing or outside your access scope.
      </EmptyState>
    )
  return (
    <div className="page-enter">
      <Link to="/topology" className="back-link">
        <ArrowLeft size={14} />
        Back to topology
      </Link>
      <PageHeading title="Recovery path" />
      <div className="recovery-layout">
        <div>
          <RecoveryDiagram route={route} source={source} target={target} snapshot={snapshot} />
          <RecordedEvidence source={source} target={target} />
          <RestoreTestEvidence source={source} />
        </div>
        <PathEvidence route={route} source={source} target={target} />
      </div>
    </div>
  )
}
