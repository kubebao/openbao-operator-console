import { Link } from '@tanstack/react-router'
import {
  createColumnHelper,
  flexRender,
  getCoreRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table'
import type { SortDirection } from '@tanstack/react-table'
import { ArrowDown, ArrowUp, ArrowUpDown, Boxes, ChevronRight } from 'lucide-react'
import type { Deployment } from '../../api/schema'
import { environmentLabels, healthRank } from '../../lib/format'
import { EvidenceBadge, HealthBadge, Timestamp } from '../status'

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
    header: () => <span className="sr-only">Details</span>,
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

const ariaSort = { asc: 'ascending', desc: 'descending' } as const

function SortIcon({ direction }: { direction: SortDirection | false }) {
  if (direction === 'asc') return <ArrowUp size={12} />
  if (direction === 'desc') return <ArrowDown size={12} />
  return <ArrowUpDown size={12} />
}

export function FleetTable({ deployments }: { deployments: Deployment[] }) {
  // React Compiler is not enabled; TanStack owns its mutable table callbacks.
  // oxlint-disable-next-line react/incompatible-library
  const table = useReactTable({
    data: deployments,
    columns,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    initialState: { sorting: [{ id: 'health', desc: false }] },
  })
  return (
    <table className="fleet-table">
      <thead>
        {table.getHeaderGroups().map((group) => (
          <tr key={group.id}>
            {group.headers.map((header) => {
              const sorted = header.column.getIsSorted()
              const heading = flexRender(header.column.columnDef.header, header.getContext())
              return (
                <th key={header.id} aria-sort={sorted ? ariaSort[sorted] : undefined}>
                  {header.column.getCanSort() ? (
                    <button onClick={header.column.getToggleSortingHandler()}>
                      {heading}
                      <SortIcon direction={sorted} />
                    </button>
                  ) : (
                    heading
                  )}
                </th>
              )
            })}
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
  )
}
