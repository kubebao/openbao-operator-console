import { useActivity, useSnapshot } from '../api/client'
import { PageHeading, EmptyState } from '../components/page'
import { RecentActivity } from '../components/recent-activity'
import { Button } from '../components/ui/button'

export function ActivityPage() {
  const { data: snapshot } = useSnapshot()
  const activity = useActivity(Boolean(snapshot))
  let content = (
    <EmptyState title="No activity recorded">
      Changes appear as the collector observes them.
    </EmptyState>
  )
  if (activity.isPending) content = <p className="muted">Loading recorded activity…</p>
  if (activity.isError)
    content = (
      <EmptyState title="Activity unavailable">
        The console cannot read recorded activity.
      </EmptyState>
    )
  if (activity.data?.length) content = <RecentActivity events={activity.data} />
  if (!snapshot) return null
  return (
    <div className="page-enter">
      <PageHeading title="Activity" />
      <div className="activity-workspace">{content}</div>
      {activity.isError && (
        <div role="status">
          {activity.data && (
            <p>Activity could not be refreshed. Previously loaded records remain visible.</p>
          )}
          <Button
            onClick={() => {
              void activity.refetch()
            }}
          >
            Try again
          </Button>
        </div>
      )}
      {activity.hasNextPage && (
        <Button
          disabled={activity.isFetchingNextPage}
          onClick={() => {
            void activity.fetchNextPage()
          }}
        >
          {activity.isFetchingNextPage ? 'Loading older activity…' : 'Load older activity'}
        </Button>
      )}
      <p className="section-footnote">
        Collection gaps and enrollment time limit history coverage. This view is not a complete
        audit trail.
      </p>
    </div>
  )
}
