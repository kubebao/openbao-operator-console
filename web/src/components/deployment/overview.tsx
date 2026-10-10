import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { ArrowRight, CheckCircle2, GitBranch, History as HistoryIcon } from 'lucide-react'
import type { Deployment, Issue, Operator, Snapshot } from '../../api/schema'
import type { DeploymentTab } from '../../lib/findings'
import { findingLocation, tabLabels } from '../../lib/findings'
import type { Observation } from '../../lib/history'
import { describeHistory } from '../../lib/history'
import { OperatorEvidence } from '../operator-evidence'
import { SeverityBadge, Timestamp } from '../status'
import { EvidenceTiles } from './evidence-tiles'

const severityRank: Record<Issue['severity'], number> = { critical: 0, high: 1, info: 2 }

function relationshipLabel(
  relationship: Snapshot['relationships'][number],
  deployment: Deployment,
  other: Deployment | undefined,
): ReactNode {
  const name = other ? `${other.namespace}/${other.name}` : 'an unavailable deployment'
  if (relationship.sourceId === deployment.id)
    return relationship.kind === 'replication' ? (
      <>Replicates to {name}</>
    ) : (
      <>Snapshot recovery to {name}</>
    )
  return relationship.kind === 'replication' ? (
    <>Read replica of {name}</>
  ) : (
    <>Recovers from {name}</>
  )
}

function FindingsSection({
  deployment,
  onSelect,
}: {
  deployment: Deployment
  onSelect: (tab: DeploymentTab) => void
}) {
  const findings = [...deployment.issues].sort(
    (a, b) => severityRank[a.severity] - severityRank[b.severity],
  )
  return (
    <section className="overview-section">
      <h2>
        Findings <span className="count">{findings.length}</span>
      </h2>
      {findings.length === 0 ? (
        <p className="no-findings">
          <CheckCircle2 size={16} />
          Current evidence matches no attention rule.
        </p>
      ) : (
        <ul className="finding-list">
          {findings.map((issue) => {
            const tab = findingLocation(issue).tab
            return (
              <li key={issue.id} className={`finding-item finding-${issue.severity}`}>
                <SeverityBadge severity={issue.severity} />
                <div>
                  <strong>{issue.title}</strong>
                  <p>{issue.detail}</p>
                  <small>
                    First observed <Timestamp value={issue.firstSeenAt} />
                  </small>
                </div>
                {tab !== 'overview' && (
                  <button
                    type="button"
                    className="text-link"
                    onClick={() => {
                      onSelect(tab)
                    }}
                  >
                    View {tabLabels[tab].toLocaleLowerCase()}
                    <ArrowRight size={14} />
                  </button>
                )}
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}

function RecentChanges({
  observations,
  onSelect,
}: {
  observations: readonly Observation[] | null
  onSelect: (tab: DeploymentTab) => void
}) {
  const recent = observations
    ? describeHistory(observations)
        .filter((entry) => !entry.quiet)
        .slice(0, 5)
    : null
  return (
    <section className="overview-section">
      <div className="section-heading">
        <h2>
          <HistoryIcon size={16} />
          Recent changes
        </h2>
        <button
          type="button"
          className="text-link"
          onClick={() => {
            onSelect('history')
          }}
        >
          View history
          <ArrowRight size={14} />
        </button>
      </div>
      {recent === null ? (
        <p className="muted">Loading recorded changes…</p>
      ) : (
        <ol className="recent-changes">
          {recent.map((entry) => {
            const [first, ...rest] = entry.changes
            return (
              <li key={entry.at}>
                <span className={`tone-dot tone-${first?.tone ?? 'neutral'}`} />
                <span>
                  {first?.title}
                  {rest.length > 0 && <small> +{rest.length} more</small>}
                </span>
                <Timestamp value={entry.at} />
              </li>
            )
          })}
        </ol>
      )}
    </section>
  )
}

function RelationshipsLine({
  deployment,
  snapshot,
}: {
  deployment: Deployment
  snapshot: Snapshot
}) {
  const relationships = snapshot.relationships.filter(
    (relationship) =>
      relationship.sourceId === deployment.id || relationship.targetId === deployment.id,
  )
  if (relationships.length === 0) return null
  return (
    <section className="overview-section relationships-line">
      <h2>
        <GitBranch size={16} />
        Relationships
      </h2>
      <ul>
        {relationships.map((relationship) => {
          const otherId =
            relationship.sourceId === deployment.id ? relationship.targetId : relationship.sourceId
          const other = snapshot.deployments.find((item) => item.id === otherId)
          return (
            <li key={relationship.id}>
              <Link
                to="/topology"
                search={{ relationship: relationship.id, deployment: '', kind: 'all' }}
                className="text-link"
              >
                {relationshipLabel(relationship, deployment, other)}
                <ArrowRight size={14} />
              </Link>
            </li>
          )
        })}
      </ul>
    </section>
  )
}

export function OverviewTab({
  deployment,
  snapshot,
  operator,
  observations,
  onSelect,
}: {
  deployment: Deployment
  snapshot: Snapshot
  operator: Operator | undefined
  observations: readonly Observation[] | null
  onSelect: (tab: DeploymentTab) => void
}) {
  const operatorProblem =
    operator !== undefined && (operator.collection !== 'current' || operator.health !== 'available')
  return (
    <>
      <EvidenceTiles
        deployment={deployment}
        collectedAt={snapshot.collectedAt}
        maxBackupAgeHours={snapshot.policy.maxBackupAgeHours}
        onSelect={onSelect}
      />
      <div className="overview-columns">
        <FindingsSection deployment={deployment} onSelect={onSelect} />
        <RecentChanges observations={observations} onSelect={onSelect} />
      </div>
      <RelationshipsLine deployment={deployment} snapshot={snapshot} />
      {operatorProblem && (
        <section className="overview-section">
          <h2>Operator needs attention</h2>
          <p className="muted">
            The operator that manages this deployment is not reporting normally, so its evidence may
            be outdated.
          </p>
          <OperatorEvidence operator={operator} />
        </section>
      )}
      {deployment.operatorId && !operator && (
        <p className="section-footnote">
          The declared operator {deployment.operatorId} is not visible in your access scope.
        </p>
      )}
    </>
  )
}
