import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import {
  AlertCircle,
  ArrowRight,
  ChevronRight,
  ListChecks,
  RefreshCw,
  ShieldCheck,
} from 'lucide-react'
import { useSnapshot } from '../api/client'
import { categoryLabels, environmentLabels, healthLabels } from '../lib/format'
import { findingLocation } from '../lib/findings'
import { PageHeading, EmptyState } from '../components/page'
import { SeverityBadge, Timestamp } from '../components/status'
import { Button } from '../components/ui/button'
import type { Issue, Snapshot } from '../api/schema'

const severityOrder = { critical: 0, high: 1, info: 2 }

export function RecentActivity({ snapshot, limit = 5 }: { snapshot: Snapshot; limit?: number }) {
  return (
    <ul className="activity-list">
      {snapshot.activity.slice(0, limit).map((event) => (
        <li key={event.id}>
          <span className={`activity-symbol activity-${event.outcome}`}>
            {event.outcome === 'success' ? <ShieldCheck size={17} /> : <AlertCircle size={17} />}
          </span>
          <div>
            <strong>{event.title}</strong>
            <small>{event.detail}</small>
          </div>
          <Timestamp value={event.at} />
        </li>
      ))}
    </ul>
  )
}

export function AttentionPage() {
  const { data: snapshot, refetch } = useSnapshot()
  const [category, setCategory] = useState<Issue['category'] | 'all'>('all')
  const [sort, setSort] = useState('priority')
  if (!snapshot) return null
  const all = snapshot.deployments.flatMap((deployment) => deployment.issues)
  const issues = all
    .filter((issue) => category === 'all' || issue.category === category)
    .sort((a, b) =>
      sort === 'priority'
        ? severityOrder[a.severity] - severityOrder[b.severity]
        : Date.parse(a.firstSeenAt) - Date.parse(b.firstSeenAt),
    )
  const incomplete = snapshot.connections.filter(
    (connection) => connection.collection !== 'current',
  )
  const affected = new Set(all.map((issue) => issue.deploymentId)).size
  const operatorsNeedingAttention = snapshot.connections
    .flatMap((connection) => connection.operators)
    .filter((operator) => operator.collection !== 'current' || operator.health !== 'available')
  if (snapshot.mode === 'live' && snapshot.connections.length === 0)
    return (
      <div className="page-enter initial-fleet">
        <PageHeading
          title={snapshot.viewer.admin ? 'Connect your first cluster' : 'No namespaces assigned'}
        />
        <p>
          {snapshot.viewer.admin
            ? 'Add a Kubernetes cluster to observe OpenBao deployments.'
            : 'Ask a console administrator to assign namespace access.'}
        </p>
        {snapshot.viewer.admin && (
          <Link to="/settings/clusters/new" className="button button-primary">
            Connect cluster <ArrowRight size={16} />
          </Link>
        )}
      </div>
    )
  const environments = ['production', 'staging', 'development', 'other'] as const
  const categories: (Issue['category'] | 'all')[] = [
    'all',
    'availability',
    'operations',
    'backup',
    'visibility',
  ]
  return (
    <div className="page-enter">
      <PageHeading
        title="Attention"

        actions={
          <Button
            size="small"
            onClick={() => {
              void refetch()
            }}
          >
            <RefreshCw size={14} />
            Refresh
          </Button>
        }
      />
      <div className="summary-line">
        <span className="summary-number">
          <span className="dot-critical" />
          {all.length} {all.length === 1 ? 'finding' : 'findings'}
        </span>
        <span>{affected} affected deployments</span>
        <span>{snapshot.deployments.length} deployments observed</span>
      </div>
      {incomplete.length > 0 && (
        <div className="evidence-banner">
          <AlertCircle size={19} />
          <span>
            <strong>Collection needs attention</strong> · {incomplete.length} Kubernetes cluster
            {incomplete.length === 1 ? '' : 's'} cannot provide current evidence. Undiscovered
            deployments cannot be assessed.
          </span>
          <Link to="/clusters">
            Inspect clusters <ArrowRight size={14} />
          </Link>
        </div>
      )}
      {operatorsNeedingAttention.length > 0 && (
        <div className="evidence-banner">
          <AlertCircle size={19} />
          <span>
            <strong>Controller workload needs attention</strong> ·{' '}
            {operatorsNeedingAttention.length} enrolled operator
            {operatorsNeedingAttention.length === 1 ? '' : 's'} unavailable, progressing, or without
            current evidence.
          </span>
          <Link to="/clusters">
            Inspect operators <ArrowRight size={14} />
          </Link>
        </div>
      )}
      <div className="table-toolbar">
        <div className="segmented" aria-label="Issue category">
          {categories.map((value) => (
            <button
              key={value}
              className={category === value ? 'selected' : ''}
              onClick={() => {
                setCategory(value)
              }}
            >
              {value === 'all' ? 'All issues' : categoryLabels[value]}
              <span>
                {all.filter((issue) => value === 'all' || issue.category === value).length}
              </span>
            </button>
          ))}
        </div>
        <label className="sort-label">
          Sort by
          <select
            aria-label="Sort issues"
            value={sort}
            onChange={(event) => {
              setSort(event.target.value)
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
              <th aria-label="Details" />
            </tr>
          </thead>
          <tbody>
            {issues.map((issue) => {
              const deployment = snapshot.deployments.find((item) => item.id === issue.deploymentId)
              return (
                <tr key={issue.id}>
                  <td>
                    <SeverityBadge severity={issue.severity} />
                  </td>
                  <td>
                    <Link
                      to="/deployments/$deploymentId"
                      params={{ deploymentId: issue.deploymentId }}
                      search={{ tab: findingLocation(issue).tab }}
                      className="row-main-link"
                    >
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
                    <Link
                      to="/deployments/$deploymentId"
                      params={{ deploymentId: issue.deploymentId }}
                      search={{ tab: findingLocation(issue).tab }}
                      aria-label={`Inspect ${issue.title}`}
                    >
                      <ChevronRight size={17} />
                    </Link>
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
        {issues.length === 0 && (
          <EmptyState title="No findings in this view" icon={ListChecks}>
            Check collection coverage before assessing the fleet as healthy.
          </EmptyState>
        )}
      </div>
      <div className="attention-support">
        <section>
          <div className="section-heading">
            <div>
              <h2>Fleet overview</h2>
              <p>
                {snapshot.deployments.length} deployments across {snapshot.connections.length}{' '}
                Kubernetes clusters
              </p>
            </div>
            <Link to="/fleet" className="text-link">
              View fleet
              <ArrowRight size={15} />
            </Link>
          </div>
          <div className="health-legend">
            {(['healthy', 'attention', 'critical', 'unknown'] as const).map((health) => (
              <span key={health}>
                <span className={`legend-dot status-${health}`} />
                {healthLabels[health]}
                <strong>
                  {snapshot.deployments.filter((deployment) => deployment.health === health).length}
                </strong>
              </span>
            ))}
          </div>
          <div className="environment-bars">
            {environments.map((environment) => {
              const deployments = snapshot.deployments.filter(
                (item) => item.environment === environment,
              )
              return (
                <div key={environment}>
                  <span className="environment-label">{environmentLabels[environment]}</span>
                  <div className="stacked-bar">
                    {(['healthy', 'attention', 'critical', 'unknown'] as const).map((health) => (
                      <span
                        key={health}
                        className={`bar-${health}`}
                        style={{
                          width: `${String(deployments.length ? (deployments.filter((item) => item.health === health).length / deployments.length) * 100 : 0)}%`,
                        }}
                      />
                    ))}
                  </div>
                  <strong>{deployments.length}</strong>
                </div>
              )
            })}
          </div>
        </section>
        <section>
          <div className="section-heading">
            <div>
              <h2>Recent activity</h2>
            </div>
            <Link to="/activity" className="text-link">
              View all
              <ArrowRight size={15} />
            </Link>
          </div>
          {snapshot.activity.length ? (
            <RecentActivity snapshot={snapshot} />
          ) : (
            <p className="muted">No changes recorded.</p>
          )}
        </section>
      </div>
    </div>
  )
}
