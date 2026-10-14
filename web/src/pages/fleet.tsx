import { useMemo } from 'react'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { Boxes, SearchX } from 'lucide-react'
import { useSnapshot } from '../api/client'
import { FleetFilters } from '../components/fleet/fleet-filters'
import { FleetTable } from '../components/fleet/fleet-table'
import { QuickViews } from '../components/fleet/quick-views'
import { EmptyState, PageHeading } from '../components/page'
import type { FleetFilter } from '../lib/filters'
import { clusterNames, emptyFilter, filterDeployments } from '../lib/filters'

export function FleetPage() {
  const { data: snapshot } = useSnapshot()
  const search = useSearch({ from: '/fleet' })
  const navigate = useNavigate()
  const data = useMemo(
    () => filterDeployments(snapshot?.deployments ?? [], search, clusterNames(snapshot)),
    [snapshot, search],
  )
  if (!snapshot) return null
  const update = (field: keyof FleetFilter, value: string) => {
    void navigate({ to: '/fleet', search: { ...search, [field]: value }, replace: true })
  }
  const view = (filter: Partial<FleetFilter>) => {
    void navigate({ to: '/fleet', search: { ...emptyFilter, ...filter } })
  }
  return (
    <div className="page-enter">
      <PageHeading title="Fleet" />
      <div className="summary-line">
        <span className="summary-number">
          <Boxes size={15} />
          {snapshot.deployments.length} deployments
        </span>
        <span>
          {snapshot.deployments.filter((item) => item.issues.length > 0).length} need attention
        </span>
        <span>{snapshot.connections.length} Kubernetes clusters</span>
      </div>
      <div className="fleet-layout">
        <div>
          <FleetFilters
            snapshot={snapshot}
            filter={search}
            onChange={update}
            onClear={() => {
              view({})
            }}
          />
          <div className="table-surface">
            <FleetTable deployments={data} />
            {data.length === 0 && (
              <EmptyState title="No deployments match" icon={SearchX}>
                Adjust your filters to see more of the authorized inventory.
              </EmptyState>
            )}
          </div>
          <div className="table-footer">
            Showing {data.length} of {snapshot.deployments.length} deployments
          </div>
        </div>
        <QuickViews deployments={snapshot.deployments} filter={search} onView={view} />
      </div>
    </div>
  )
}
