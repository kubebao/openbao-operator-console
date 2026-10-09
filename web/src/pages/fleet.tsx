import { useMemo } from 'react'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import {
  createColumnHelper,
  flexRender,
  getCoreRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table'
import {
  ArrowDown,
  ArrowUp,
  ArrowUpDown,
  Boxes,
  ChevronRight,
  Search,
  SearchX,
  SlidersHorizontal,
} from 'lucide-react'
import { useSnapshot } from '../api/client'
import type { Deployment } from '../api/schema'
import type { FleetFilter } from '../lib/filters'
import { emptyFilter, filterDeployments } from '../lib/filters'
import { collectionLabels, environmentLabels, healthLabels, healthRank } from '../lib/format'
import { PageHeading, EmptyState } from '../components/page'
import { HealthBadge, EvidenceBadge, Timestamp } from '../components/status'
import { Button } from '../components/ui/button'

const column = createColumnHelper<Deployment>()
const columns = [
  column.accessor('name', {
    header: 'Deployment',
    cell: ({ row }) => (
      <Link
        to="/deployments/$deploymentId"
        params={{ deploymentId: row.original.id }}
        className="deployment-cell"
      >
        <Boxes size={19} />
        <span>
          <strong>{row.original.name}</strong>
          <small>{row.original.namespace}</small>
        </span>
      </Link>
    ),
  }),
  column.accessor('team', { header: 'Team' }),
  column.accessor('environment', {
    header: 'Environment',
    cell: ({ getValue }) => environmentLabels[getValue()],
  }),
  column.accessor('connectionId', {
    header: 'Kubernetes cluster',
    cell: ({ row }) => (
      <span>
        <strong className="normal-weight">{row.original.connectionId}</strong>
        <small>{row.original.location}</small>
      </span>
    ),
  }),
  column.accessor('version', {
    header: 'OpenBao',
    cell: ({ getValue }) => getValue() || 'Not observed',
  }),
  column.accessor('health', {
    header: 'State',
    cell: ({ getValue }) => <HealthBadge health={getValue()} />,
    // Severity order, then name, keeps the triage order stable between refreshes.
    sortingFn: (a, b) =>
      healthRank[a.original.health] - healthRank[b.original.health] ||
      a.original.name.localeCompare(b.original.name),
  }),
  column.accessor((row) => row.issues.length, {
    id: 'issues',
    header: 'Findings',
    cell: ({ getValue }) => (
      <span className={getValue() ? 'finding-count' : 'muted'}>{getValue()}</span>
    ),
  }),
  column.accessor((row) => row.evidence.collection, {
    id: 'collection',
    header: 'Collection',
    cell: ({ getValue }) => <EvidenceBadge collection={getValue()} />,
  }),
  column.accessor((row) => row.evidence.collectedAt, {
    id: 'collectedAt',
    header: 'Last collected',
    cell: ({ getValue }) => (
      <span className="time-cell">
        <Timestamp value={getValue()} />
      </span>
    ),
  }),
  column.display({
    id: 'open',
    header: '',
    cell: ({ row }) => (
      <Link
        to="/deployments/$deploymentId"
        params={{ deploymentId: row.original.id }}
        aria-label={`Inspect ${row.original.name}`}
      >
        <ChevronRight size={16} />
      </Link>
    ),
  }),
]

export function FleetPage() {
  'use no memo'
  const { data: snapshot } = useSnapshot()
  const search = useSearch({ from: '/fleet' })
  const navigate = useNavigate()
  const data = useMemo(
    () => filterDeployments(snapshot?.deployments ?? [], search),
    [snapshot, search],
  )
  // React Compiler is not enabled; TanStack owns its mutable table callbacks.
  // oxlint-disable-next-line react/incompatible-library
  const table = useReactTable({
    data,
    columns,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    initialState: { sorting: [{ id: 'health', desc: false }] },
  })
  if (!snapshot) return null
  const update = (field: keyof FleetFilter, value: string) => {
    void navigate({ to: '/fleet', search: { ...search, [field]: value }, replace: true })
  }
  const view = (filter: Partial<FleetFilter>) => {
    void navigate({ to: '/fleet', search: { ...emptyFilter, ...filter } })
  }
  const clear = () => {
    view({})
  }
  const filtered = Object.values(search).some(Boolean)
  const viewClass = (filter: Partial<FleetFilter>) => {
    const target: FleetFilter = { ...emptyFilter, ...filter }
    return (Object.keys(target) as (keyof FleetFilter)[]).every(
      (key) => search[key] === target[key],
    )
      ? 'selected'
      : ''
  }
  const fields: readonly {
    key: Exclude<keyof FleetFilter, 'query'>
    label: string
    options: readonly (readonly [string, string])[]
  }[] = [
    {
      key: 'team',
      label: 'Team',
      options: [...new Set(snapshot.deployments.map((item) => item.team))]
        .sort()
        .map((team) => [team, team] as const),
    },
    {
      key: 'environment',
      label: 'Environment',
      options: Object.entries(environmentLabels),
    },
    {
      key: 'cluster',
      label: 'Cluster',
      options: snapshot.connections.map((connection) => [connection.id, connection.name] as const),
    },
    {
      key: 'health',
      label: 'State',
      options: (['critical', 'attention', 'unknown', 'healthy'] as const).map(
        (health) => [health, healthLabels[health]] as const,
      ),
    },
    {
      key: 'collection',
      label: 'Collection',
      options: [
        ['gaps', 'Any gap'],
        ...(['current', 'stale', 'unreachable', 'denied', 'unsupported'] as const).map(
          (state) => [state, collectionLabels[state]] as const,
        ),
      ],
    },
  ]
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
          <div className="fleet-filters">
            <label className="search-field">
              <Search size={16} />
              <input
                aria-label="Search deployments"
                placeholder="Search deployments…"
                value={search.query}
                onChange={(event) => {
                  update('query', event.target.value)
                }}
              />
            </label>
            {fields.map((field) => (
              <label key={field.key}>
                {field.label}
                <select
                  aria-label={`Filter by ${field.label.toLowerCase()}`}
                  value={search[field.key]}
                  onChange={(event) => {
                    update(field.key, event.target.value)
                  }}
                >
                  <option value="">All</option>
                  {field.options.map(([value, label]) => (
                    <option value={value} key={value}>
                      {label}
                    </option>
                  ))}
                </select>
              </label>
            ))}
            <Button size="small" variant="ghost" onClick={clear} disabled={!filtered}>
              Clear filters
            </Button>
          </div>
          <div className="table-surface">
            <table className="fleet-table">
              <thead>
                {table.getHeaderGroups().map((group) => (
                  <tr key={group.id}>
                    {group.headers.map((header) => (
                      <th
                        key={header.id}
                        aria-sort={
                          header.column.getIsSorted() === 'asc'
                            ? 'ascending'
                            : header.column.getIsSorted() === 'desc'
                              ? 'descending'
                              : undefined
                        }
                      >
                        {header.column.getCanSort() ? (
                          <button onClick={header.column.getToggleSortingHandler()}>
                            {flexRender(header.column.columnDef.header, header.getContext())}
                            {header.column.getIsSorted() === 'asc' ? (
                              <ArrowUp size={12} />
                            ) : header.column.getIsSorted() === 'desc' ? (
                              <ArrowDown size={12} />
                            ) : (
                              <ArrowUpDown size={12} />
                            )}
                          </button>
                        ) : (
                          flexRender(header.column.columnDef.header, header.getContext())
                        )}
                      </th>
                    ))}
                  </tr>
                ))}
              </thead>
              <tbody>
                {table.getRowModel().rows.map((row) => (
                  <tr key={row.original.id}>
                    {row.getVisibleCells().map((cell) => (
                      <td
                        key={cell.id}
                        data-label={
                          typeof cell.column.columnDef.header === 'string'
                            ? cell.column.columnDef.header
                            : undefined
                        }
                      >
                        {flexRender(cell.column.columnDef.cell, cell.getContext())}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
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
        <aside className="saved-views">
          <h2>
            <SlidersHorizontal size={15} />
            Quick views
          </h2>
          <button onClick={clear} className={viewClass({})}>
            All deployments<span>{snapshot.deployments.length}</span>
          </button>
          <button
            className={viewClass({ environment: 'production' })}
            onClick={() => {
              view({ environment: 'production' })
            }}
          >
            Production
            <span>
              {snapshot.deployments.filter((item) => item.environment === 'production').length}
            </span>
          </button>
          <button
            className={viewClass({ health: 'critical' })}
            onClick={() => {
              view({ health: 'critical' })
            }}
          >
            Critical
            <span>{snapshot.deployments.filter((item) => item.health === 'critical').length}</span>
          </button>
          <button
            className={viewClass({ collection: 'gaps' })}
            onClick={() => {
              view({ collection: 'gaps' })
            }}
          >
            Collection gaps
            <span>
              {snapshot.deployments.filter((item) => item.evidence.collection !== 'current').length}
            </span>
          </button>
          <p>Filters are kept in the URL so a view can be bookmarked or shared.</p>
        </aside>
      </div>
    </div>
  )
}
