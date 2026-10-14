import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { ChevronRight, ListChecks } from 'lucide-react'
import type { Issue, Snapshot } from '../../api/schema'
import { findingLocation, severityRank } from '../../lib/findings'
import { categoryLabels } from '../../lib/format'
import { EmptyState } from '../page'
import { SeverityBadge, Timestamp } from '../status'

type CategoryFilter = Issue['category'] | 'all'
type IssueOrder = 'priority' | 'oldest'

const categories: readonly CategoryFilter[] = [
  'all',
  'availability',
  'operations',
  'backup',
  'visibility',
]
const orders: Record<IssueOrder, (a: Issue, b: Issue) => number> = {
  priority: (a, b) => severityRank[a.severity] - severityRank[b.severity],
  oldest: (a, b) => Date.parse(a.firstSeenAt) - Date.parse(b.firstSeenAt),
}

function inCategory(issue: Issue, category: CategoryFilter): boolean {
  return category === 'all' || issue.category === category
}

function IssueRow({ issue, snapshot }: { issue: Issue; snapshot: Snapshot }) {
  const deployment = snapshot.deployments.find((item) => item.id === issue.deploymentId)
  const target = {
    to: '/deployments/$deploymentId',
    params: { deploymentId: issue.deploymentId },
    search: { tab: findingLocation(issue).tab },
  } as const
  return (
    <tr>
      <td>
        <SeverityBadge severity={issue.severity} />
      </td>
      <td>
        <Link {...target} className="row-main-link">
          {issue.title}
        </Link>
        <div className="row-description" title={issue.detail}>
          {issue.detail}
        </div>
      </td>
      <td data-label="Deployment">
        <strong>{deployment?.name ?? 'Unavailable'}</strong>
        <small>
          {deployment?.connectionId} / {deployment?.namespace}
        </small>
      </td>
      <td data-label="Category">
        <span className="category-text">{categoryLabels[issue.category]}</span>
      </td>
      <td className="time-cell" data-label="First observed">
        <Timestamp value={issue.firstSeenAt} />
      </td>
      <td className="time-cell" data-label="Last checked">
        <Timestamp value={issue.lastCheckedAt} />
      </td>
      <td>
        <Link {...target} aria-label={`Inspect ${issue.title}`}>
          <ChevronRight size={17} />
        </Link>
      </td>
    </tr>
  )
}

// IssueTable lists every finding in the viewer's scope by category and order.
export function IssueTable({ snapshot }: { snapshot: Snapshot }) {
  const [category, setCategory] = useState<CategoryFilter>('all')
  const [order, setOrder] = useState<IssueOrder>('priority')
  const all = snapshot.deployments.flatMap((deployment) => deployment.issues)
  const issues = all.filter((issue) => inCategory(issue, category)).sort(orders[order])
  return (
    <>
      <div className="table-toolbar">
        <div className="segmented" role="group" aria-label="Issue category">
          {categories.map((value) => (
            <button
              key={value}
              type="button"
              aria-pressed={category === value}
              className={category === value ? 'selected' : ''}
              onClick={() => {
                setCategory(value)
              }}
            >
              {value === 'all' ? 'All issues' : categoryLabels[value]}
              <span>{all.filter((issue) => inCategory(issue, value)).length}</span>
            </button>
          ))}
        </div>
        <label className="sort-label">
          Sort by
          <select
            aria-label="Sort issues"
            value={order}
            onChange={(event) => {
              const value = event.target.value
              if (value === 'priority' || value === 'oldest') setOrder(value)
            }}
          >
            <option value="priority">Priority</option>
            <option value="oldest">Oldest first</option>
          </select>
        </label>
      </div>
      <div className="table-surface">
        <table className="attention-table">
          <thead>
            <tr>
              <th>Severity</th>
              <th>Issue</th>
              <th>Affected deployment</th>
              <th>Category</th>
              <th>First observed</th>
              <th>Last checked</th>
              <th>
                <span className="sr-only">Details</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {issues.map((issue) => (
              <IssueRow key={issue.id} issue={issue} snapshot={snapshot} />
            ))}
          </tbody>
        </table>
        {issues.length === 0 && (
          <EmptyState title="No findings in this view" icon={ListChecks}>
            Check collection coverage before assessing the fleet as healthy.
          </EmptyState>
        )}
      </div>
    </>
  )
}
