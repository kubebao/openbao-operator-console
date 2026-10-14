import { SlidersHorizontal } from 'lucide-react'
import type { Deployment } from '../../api/schema'
import type { FleetFilter } from '../../lib/filters'
import { collectionGaps, sameFilter } from '../../lib/filters'

const views: readonly {
  label: string
  filter: Partial<FleetFilter>
  matches: (deployment: Deployment) => boolean
}[] = [
  { label: 'All deployments', filter: {}, matches: () => true },
  {
    label: 'Production',
    filter: { environment: 'production' },
    matches: (deployment) => deployment.environment === 'production',
  },
  {
    label: 'Critical',
    filter: { health: 'critical' },
    matches: (deployment) => deployment.health === 'critical',
  },
  {
    label: 'Collection gaps',
    filter: { collection: collectionGaps },
    matches: (deployment) => deployment.evidence.collection !== 'current',
  },
]

export function QuickViews({
  deployments,
  filter,
  onView,
}: {
  deployments: readonly Deployment[]
  filter: FleetFilter
  onView: (filter: Partial<FleetFilter>) => void
}) {
  return (
    <aside className="saved-views" aria-labelledby="quick-views-heading">
      <h2 id="quick-views-heading">
        <SlidersHorizontal size={15} />
        Quick views
      </h2>
      <div role="group" aria-labelledby="quick-views-heading">
        {views.map((view) => {
          const selected = sameFilter(filter, view.filter)
          return (
            <button
              key={view.label}
              type="button"
              aria-pressed={selected}
              className={selected ? 'selected' : ''}
              onClick={() => {
                onView(view.filter)
              }}
            >
              {view.label}
              <span>{deployments.filter(view.matches).length}</span>
            </button>
          )
        })}
      </div>
      <p>Filters are kept in the URL so a view can be bookmarked or shared.</p>
    </aside>
  )
}
