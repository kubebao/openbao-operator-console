import { useSnapshot } from '../api/client'
import { PageHeading, EmptyState } from '../components/page'
import { RecentActivity } from '../components/recent-activity'

export function ActivityPage() {
  const { data: snapshot } = useSnapshot()
  if (!snapshot) return null
  return (
    <div className="page-enter">
      <PageHeading title="Activity" />
      <div className="activity-workspace">
        {snapshot.activity.length ? (
          <RecentActivity snapshot={snapshot} limit={100} />
        ) : (
          <EmptyState title="No activity recorded">
            Changes appear as the collector observes them. This is a bounded observation history.
          </EmptyState>
        )}
      </div>
      <p className="section-footnote">
        Collection gaps and enrollment time limit history coverage. This view is not a complete
        audit trail.
      </p>
    </div>
  )
}
