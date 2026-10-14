import { Link, useParams } from '@tanstack/react-router'
import { ArrowLeft } from 'lucide-react'
import { useSnapshot } from '../api/client'
import { PageHeading } from '../components/page'
import { useDocumentTitle } from '../components/use-document-title'
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
  useDocumentTitle('Recovery path')
  if (!snapshot || !route || !source || !target)
    return (
      <div className="page-enter">
        <Link to="/topology" className="back-link">
          <ArrowLeft size={14} />
          Back to topology
        </Link>
        <PageHeading
          title="Recovery path unavailable"
          description="The path is missing or outside your access scope."
        />
      </div>
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
