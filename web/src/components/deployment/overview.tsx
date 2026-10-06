import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import {
  AlertCircle,
  AlertTriangle,
  ArrowRight,
  CheckCircle2,
  Database,
  GitBranch,
  HeartPulse,
  History as HistoryIcon,
  ShieldCheck,
  Wrench,
} from 'lucide-react'
import type { Deployment, Issue, Operator, Snapshot } from '../../api/schema'
import type { DeploymentTab, EvidenceDimension } from '../../lib/findings'
import { dimensionSeverity, findingLocation, tabLabels } from '../../lib/findings'
import type { Observation } from '../../lib/history'
import { describeHistory } from '../../lib/history'
import { observedGeneration, upgradeLabels, upgradeState } from '../../lib/lifecycle'
import { restoreOutcomes } from '../../lib/format'
import { OperatorEvidence } from '../operator-evidence'
import { SeverityBadge, Timestamp } from '../status'
import { AgeMeter, ReplicaDots } from './charts'

const severityRank: Record<Issue['severity'], number> = { critical: 0, high: 1, info: 2 }

function EvidenceTile({
  label,
  icon,
  dimension,
  deployment,
  tab,
  onSelect,
  children,
  footer,
}: {
  label: string
  icon: ReactNode
  dimension: EvidenceDimension
  deployment: Deployment
  tab: DeploymentTab
  onSelect: (tab: DeploymentTab) => void
  children: ReactNode
  footer?: ReactNode
}) {
  const severity = dimensionSeverity(deployment, dimension)
  return (
    <button
      type="button"
      className={`evidence-tile${severity ? ` tile-${severity}` : ''}`}
      onClick={() => {
        onSelect(tab)
      }}
      aria-label={`${label}: open ${tabLabels[tab]}`}
    >
      <span className="tile-label">
        {icon}
        {label}
        {severity === 'critical' ? (
          <AlertCircle size={15} className="tile-status" aria-label="Critical finding" />
        ) : severity ? (
          <AlertTriangle size={15} className="tile-status" aria-label="Finding" />
        ) : null}
      </span>
      <span className="tile-body">{children}</span>
      {footer && <span className="tile-footer">{footer}</span>}
    </button>
  )
}

function sealedState(deployment: Deployment): string {
  const sealed = deployment.conditions.find((condition) => condition.type === 'OpenBaoSealed')
  if (!sealed) return 'Seal state not reported'
  return sealed.status === 'True'
    ? 'Sealed'
    : sealed.status === 'False'
      ? 'Unsealed'
      : 'Seal state unknown'
}

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
  const backup = deployment.backup
  const test = backup?.restoreTest ?? null
  const upgrade = upgradeState(deployment)
  const generation = observedGeneration(deployment)
  const findings = [...deployment.issues].sort(
    (a, b) => severityRank[a.severity] - severityRank[b.severity],
  )
  const recent = observations ? describeHistory(observations).slice(0, 5) : null
  const relationships = snapshot.relationships.filter(
    (relationship) =>
      relationship.sourceId === deployment.id || relationship.targetId === deployment.id,
  )
  // Ages are measured from the snapshot's collection time, not the browser clock.
  const collectedAt = Date.parse(snapshot.collectedAt)
  const backupAge = backup?.lastSuccessAt
    ? (collectedAt - Date.parse(backup.lastSuccessAt)) / 3_600_000
    : null
  const operatorProblem =
    operator !== undefined && (operator.collection !== 'current' || operator.health !== 'available')
  return (
    <>
      <div className="evidence-tiles">
        <EvidenceTile
          label="Availability"
          icon={<HeartPulse size={15} />}
          dimension="availability"
          deployment={deployment}
          tab="conditions"
          onSelect={onSelect}
          footer={sealedState(deployment)}
        >
          {deployment.readyReplicas === null || deployment.desiredReplicas === null ? (
            <span className="muted">Readiness not reported</span>
          ) : (
            <>
              <ReplicaDots ready={deployment.readyReplicas} desired={deployment.desiredReplicas} />
              <strong>
                {deployment.readyReplicas} of {deployment.desiredReplicas} ready
              </strong>
            </>
          )}
        </EvidenceTile>
        <EvidenceTile
          label="Backups"
          icon={<Database size={15} />}
          dimension="backups"
          deployment={deployment}
          tab="backups"
          onSelect={onSelect}
          footer={
            backup ? (
              backup.consecutiveFailures > 0 ? (
                `${String(backup.consecutiveFailures)} consecutive ${backup.consecutiveFailures === 1 ? 'failure' : 'failures'}`
              ) : backup.nextAt ? (
                <>
                  Next <Timestamp value={backup.nextAt} scheduled />
                </>
              ) : (
                'No schedule reported'
              )
            ) : undefined
          }
        >
          {backup ? (
            <>
              <strong>
                {backup.lastSuccessAt ? (
                  <>
                    Completed <Timestamp value={backup.lastSuccessAt} />
                  </>
                ) : (
                  'No success recorded'
                )}
              </strong>
              {backupAge !== null && (
                <AgeMeter hours={backupAge} limitHours={snapshot.policy.maxBackupAgeHours} />
              )}
            </>
          ) : (
            <span className="muted">Not configured</span>
          )}
        </EvidenceTile>
        <EvidenceTile
          label="Restore test"
          icon={<ShieldCheck size={15} />}
          dimension="restoreTest"
          deployment={deployment}
          tab="backups"
          onSelect={onSelect}
          footer={test ? 'Pinned snapshot, same cluster' : undefined}
        >
          {test ? (
            <strong>
              {restoreOutcomes[test.outcome]}
              {test.finishedAt && (
                <>
                  {' '}
                  <Timestamp value={test.finishedAt} />
                </>
              )}
            </strong>
          ) : (
            <span className="muted">No result reported</span>
          )}
        </EvidenceTile>
        <EvidenceTile
          label="Lifecycle"
          icon={<Wrench size={15} />}
          dimension="lifecycle"
          deployment={deployment}
          tab="lifecycle"
          onSelect={onSelect}
          footer={
            generation === null
              ? 'Reconciliation not reported'
              : generation >= deployment.generation
                ? 'Operator caught up with spec'
                : `Operator at generation ${String(generation)} of ${String(deployment.generation)}`
          }
        >
          <strong>
            {deployment.version || 'Version not observed'}
            {upgrade === 'upgrading' && deployment.desiredVersion !== deployment.version
              ? ` → ${deployment.desiredVersion}`
              : ''}
          </strong>
          <span className={`upgrade-state upgrade-${upgrade}`}>
            Upgrade {upgradeLabels[upgrade].toLocaleLowerCase()}
          </span>
        </EvidenceTile>
      </div>

      <div className="overview-columns">
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
      </div>

      {relationships.length > 0 && (
        <section className="overview-section relationships-line">
          <h2>
            <GitBranch size={16} />
            Relationships
          </h2>
          <ul>
            {relationships.map((relationship) => {
              const otherId =
                relationship.sourceId === deployment.id
                  ? relationship.targetId
                  : relationship.sourceId
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
      )}

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
