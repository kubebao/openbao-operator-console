import { useMemo } from 'react'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { Boxes, SearchX } from 'lucide-react'
import { useSnapshot } from '../api/client'
import { FleetFilters } from '../components/fleet/fleet-filters'
import { FleetTable } from '../components/fleet/fleet-table'
import { EmptyState, PageHeading } from '../components/page'
import type { FleetFilter } from '../lib/filters'
import { clusterNames, emptyFilter, filterDeployments } from '../lib/filters'
import { plural } from '../lib/format'

export function FleetPage() {
  const { data: snapshot } = useSnapshot()
  const search = useSearch({ from: '/fleet' })
  const navigate = useNavigate()
  const clusters = useMemo(() => clusterNames(snapshot), [snapshot])
  const data = useMemo(
    () => filterDeployments(snapshot?.deployments ?? [], search, clusters),
    [snapshot, search, clusters],
  )
  if (!snapshot) return null
  const update = (field: keyof FleetFilter, value: string) => {
    void navigate({ to: '/fleet', search: { ...search, [field]: value }, replace: true })
  }
  return (
    <div className="page-enter">
      <PageHeading title="Fleet" />
      <div className="summary-line">
        <span className="summary-number">
          <Boxes size={15} />
          {plural(snapshot.deployments.length, 'deployment')}
        </span>
        <span>
          {snapshot.deployments.filter((item) => item.issues.length > 0).length} need attention
        </span>
        <span>{plural(snapshot.connections.length, 'Kubernetes cluster')}</span>
      </div>
      <FleetFilters
        snapshot={snapshot}
        filter={search}
        onChange={update}
        onClear={() => {
          void navigate({ to: '/fleet', search: emptyFilter })
        }}
      />
      <div className="table-surface">
        <FleetTable deployments={data} clusters={clusters} />
        {data.length === 0 && (
          <EmptyState title="No deployments match" icon={SearchX}>
            Adjust your filters to see more of the authorized inventory.
          </EmptyState>
        )}
      </div>
      <div className="table-footer">
        Showing {data.length} of {plural(snapshot.deployments.length, 'deployment')}
      </div>
    </div>
  )
}
