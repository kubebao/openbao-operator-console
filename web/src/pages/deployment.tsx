import * as Tabs from '@radix-ui/react-tabs'
import { Link, useParams, useSearch, useNavigate } from '@tanstack/react-router'
import {
  AlertCircle,
  AlertTriangle,
  ArrowLeft,
  ArrowRight,
  Boxes,
  Clock3,
  Cpu,
  Database,
  Folder,
  GitBranch,
  Layers3,
  SearchX,
  ShieldCheck,
  Wrench,
} from 'lucide-react'
import { useHistory, useSnapshot } from '../api/client'
import { OperatorEvidence } from '../components/operator-evidence'
import type { Deployment, Issue } from '../api/schema'
import {
  age,
  bytes,
  cleanupStates,
  collectionLabels,
  environmentLabels,
  recoveryPhases,
  restoreOutcomes,
  roleLabels,
} from '../lib/format'
import type { DeploymentTab } from '../lib/findings'
import { fieldSeverity, findingLocation, tabSeverity } from '../lib/findings'
import { DetailRows, EmptyState, PageHeading } from '../components/page'
import { EvidenceBadge, HealthBadge, SeverityBadge, Timestamp } from '../components/status'

const tabs: readonly { value: DeploymentTab; label: string }[] = [
  { value: 'overview', label: 'Overview' },
  { value: 'conditions', label: 'Conditions' },
  { value: 'operations', label: 'Operations' },
  { value: 'backup', label: 'Backup evidence' },
  { value: 'history', label: 'Observation history' },
]
const tabLabels = Object.fromEntries(tabs.map((tab) => [tab.value, tab.label])) as Record<
  DeploymentTab,
  string
>
const severityWords: Record<Issue['severity'], string> = {
  critical: 'critical finding',
  high: 'finding',
  info: 'note',
}

function HistoryPanel({ id }: { id: string }) {
  const history = useHistory(id)
  if (history.isPending) return <p className="muted">Loading retained observations…</p>
  if (history.isError)
    return (
      <EmptyState title="History unavailable" icon={AlertCircle}>
        The console cannot read retained observations.
      </EmptyState>
    )
  return (
    <section className="detail-section">
      <h2>Retained observations</h2>
      <p className="muted">
        Bounded history of collected projections. Missing periods are not proof of unchanged state.
      </p>
      <div className="table-surface">
        <table className="conditions-table">
          <thead>
            <tr>
              <th>Recorded</th>
              <th>State</th>
              <th>Collection</th>
              <th>Resource version</th>
            </tr>
          </thead>
          <tbody>
            {history.data.observations.map((observation, index) => (
              <tr key={`${observation.recordedAt}-${String(index)}`}>
                <td>
                  <Timestamp value={observation.recordedAt} />
                </td>
                <td>
                  <HealthBadge health={observation.deployment.health} />
                </td>
                <td>
                  <EvidenceBadge collection={observation.deployment.evidence.collection} />
                </td>
                <td>
                  <code>{observation.deployment.resourceVersion}</code>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  )
}

function OperationsPanel({ deployment }: { deployment: Deployment }) {
  const recovery = deployment.recovery
  const upgrade = deployment.upgradeFailure
  if (!recovery && !upgrade)
    return (
      <EmptyState title="No operations observed" icon={Wrench}>
        This view displays existing execution evidence. Lifecycle changes are performed through the
        operator and your runbooks.
      </EmptyState>
    )
  return (
    <>
      {upgrade && (
        <section className="detail-section">
          <h2>Upgrade failure</h2>
          <DetailRows
            rows={[
              {
                label: 'Reason',
                value: <code>{upgrade.reason || 'Not observed'}</code>,
                finding: fieldSeverity(deployment, 'upgrade'),
              },
              { label: 'Recorded', value: <Timestamp value={upgrade.at} /> },
              {
                label: 'Requested version',
                value: deployment.desiredVersion || 'Not observed',
              },
              { label: 'Observed version', value: deployment.version || 'Not observed' },
            ]}
          />
          <p className="section-footnote">
            The operator records this failure. Inspect its post-upgrade checks before retrying.
          </p>
        </section>
      )}
      {recovery && (
        <section className="detail-section">
          <h2>Recorded recovery request</h2>
          <DetailRows
            rows={[
              { label: 'Request', value: recovery.requestName },
              {
                label: 'Mechanism',
                value: recovery.mechanism === 'snapshot' ? 'Snapshot' : 'Native',
              },
              {
                label: 'Phase',
                value: recoveryPhases[recovery.phase],
                finding:
                  recovery.phase === 'failed' || recovery.phase === 'unknown'
                    ? fieldSeverity(deployment, 'recovery')
                    : null,
              },
              { label: 'Execution stage', value: recovery.stage || 'Not observed' },
              {
                label: 'Target application confirmed',
                value: <Timestamp value={recovery.applicationConfirmedAt} />,
              },
              {
                label: 'Cleanup',
                value: cleanupStates[recovery.cleanup],
                finding:
                  recovery.cleanup === 'failed' ? fieldSeverity(deployment, 'recovery') : null,
              },
              { label: 'Completion receipt', value: <Timestamp value={recovery.completedAt} /> },
            ]}
          />
          <p className="section-footnote">
            Historical completion does not establish current client availability. Unknown execution
            requires investigation.
          </p>
        </section>
      )}
    </>
  )
}

export function BackupPanel({ deployment }: { deployment: Deployment }) {
  const backup = deployment.backup
  if (!backup)
    return (
      <EmptyState title="No backup summary">
        Backup evidence is not configured or not supplied for this deployment role.
      </EmptyState>
    )
  return (
    <div className="detail-two-column">
      <section className="detail-section">
        <h2>
          <Database size={17} />
          Backup evidence
        </h2>
        <DetailRows
          rows={[
            {
              label: 'Last successful completion',
              value: <Timestamp value={backup.lastSuccessAt} />,
              finding: fieldSeverity(deployment, 'backupSuccess'),
            },
            {
              label: 'Next scheduled backup',
              value: <Timestamp value={backup.nextAt} scheduled missing="Not observed" />,
            },
            { label: 'Latest object key', value: <code>{backup.key ?? 'Not observed'}</code> },
            { label: 'Uploaded size', value: bytes(backup.sizeBytes) },
            {
              label: 'Consecutive failures',
              value: backup.consecutiveFailures,
              finding: fieldSeverity(deployment, 'backupFailures'),
            },
            {
              label: 'Uploaded digest',
              value: <code className="digest">{backup.digest ?? 'Not observed'}</code>,
            },
          ]}
        />
        <p className="section-footnote">
          Completion age is not measured RPO. A past upload does not establish current repository
          readability.
        </p>
      </section>
      <section className="detail-section">
        <h2>
          <ShieldCheck size={17} />
          Restore-test evidence
        </h2>
        {backup.restoreTest ? (
          <>
            <DetailRows
              rows={[
                {
                  label: 'Latest outcome',
                  value: restoreOutcomes[backup.restoreTest.outcome],
                  finding: fieldSeverity(deployment, 'restoreTest'),
                },
                {
                  label: 'Result recorded',
                  value: <Timestamp value={backup.restoreTest.finishedAt} />,
                },
                {
                  label: 'Last successful test',
                  value: <Timestamp value={backup.restoreTest.lastSuccessAt} />,
                },
                { label: 'Reason', value: backup.restoreTest.reason || 'Not observed' },
                {
                  label: 'Tested object key',
                  value: <code>{backup.restoreTest.key ?? 'Not observed'}</code>,
                },
              ]}
            />
            <p className="section-footnote">
              A pass covers a pinned snapshot and disposable target in the same Kubernetes cluster.
              Client logins and secret reads are not tested.
            </p>
          </>
        ) : (
          <p className="muted">No restore-test result supplied.</p>
        )}
      </section>
    </div>
  )
}

export function DeploymentPage() {
  const { deploymentId } = useParams({ from: '/deployments/$deploymentId' })
  const search = useSearch({ from: '/deployments/$deploymentId' })
  const navigate = useNavigate()
  const { data: snapshot } = useSnapshot()
  const deployment = snapshot?.deployments.find((item) => item.id === deploymentId)
  if (!deployment)
    return (
      <EmptyState title="Deployment not found" icon={SearchX}>
        This deployment is unavailable or outside your access scope.
      </EmptyState>
    )
  const issue =
    deployment.issues.find((item) => item.severity === 'critical') ?? deployment.issues[0]
  const issueTab = issue ? findingLocation(issue).tab : 'overview'
  const sealed = deployment.conditions.find((condition) => condition.type === 'OpenBaoSealed')
  const operator = snapshot?.connections
    .find((connection) => connection.id === deployment.connectionId)
    ?.operators.find((item) => item.id === deployment.operatorId)
  const selectTab = (tab: DeploymentTab) => {
    void navigate({
      to: '/deployments/$deploymentId',
      params: { deploymentId },
      search: { tab },
      replace: true,
    })
  }
  return (
    <div className="page-enter">
      <Link to="/fleet" className="back-link">
        <ArrowLeft size={14} />
        Back to fleet
      </Link>
      <PageHeading title={deployment.name} status={<HealthBadge health={deployment.health} />} />
      <div className="identity-strip">
        <div>
          <Folder size={17} />
          <span>
            Namespace<strong>{deployment.namespace}</strong>
          </span>
        </div>
        <div>
          <Boxes size={17} />
          <span>
            Team<strong>{deployment.team}</strong>
          </span>
        </div>
        <div>
          <Layers3 size={17} />
          <span>
            Environment<strong>{environmentLabels[deployment.environment]}</strong>
          </span>
        </div>
        <div>
          <GitBranch size={17} />
          <span>
            Kubernetes cluster<strong>{deployment.connectionId}</strong>
            <small>{deployment.location}</small>
          </span>
        </div>
        <div>
          <Database size={17} />
          <span>
            OpenBao version<strong>{deployment.version || 'Not observed'}</strong>
          </span>
        </div>
        <div>
          <Cpu size={17} />
          <span>
            Operator version label<strong>{deployment.operatorVersion ?? 'Not observed'}</strong>
          </span>
        </div>
      </div>
      {deployment.evidence.collection !== 'current' && (
        <div className="notice notice-warning">
          <AlertTriangle size={17} />
          <span>
            <strong>Current state is uncertain.</strong> Collection result:{' '}
            {collectionLabels[deployment.evidence.collection].toLocaleLowerCase()}. Last successful
            collection {age(deployment.evidence.collectedAt)}.
          </span>
        </div>
      )}
      <Tabs.Root
        value={search.tab}
        onValueChange={(value) => {
          const tab = tabs.find((item) => item.value === value)
          if (tab) selectTab(tab.value)
        }}
        className="detail-tabs"
      >
        <Tabs.List aria-label="Deployment details">
          {tabs.map((tab) => {
            const severity = tabSeverity(deployment, tab.value)
            return (
              <Tabs.Trigger value={tab.value} key={tab.value}>
                {tab.label}
                {severity && (
                  <span
                    className={`tab-marker marker-${severity}`}
                    aria-label={`Has ${severityWords[severity]}`}
                    role="img"
                  />
                )}
              </Tabs.Trigger>
            )
          })}
        </Tabs.List>
        <Tabs.Content value="overview">
          {issue && (
            <div className={`primary-finding severity-${issue.severity}`}>
              {issue.severity === 'critical' ? (
                <AlertCircle size={24} />
              ) : (
                <AlertTriangle size={24} />
              )}
              <div>
                <strong>{issue.title}</strong>
                <p>{issue.detail}</p>
                {issueTab !== 'overview' && (
                  <button
                    type="button"
                    className="text-link"
                    onClick={() => {
                      selectTab(issueTab)
                    }}
                  >
                    View {tabLabels[issueTab].toLocaleLowerCase()}
                    <ArrowRight size={14} />
                  </button>
                )}
              </div>
              <SeverityBadge severity={issue.severity} />
            </div>
          )}
          <div className="detail-two-column">
            <section className="detail-section">
              <h2>
                <Database size={17} />
                Current status
              </h2>
              <DetailRows
                rows={[
                  {
                    label: 'Observed workload state',
                    value: <HealthBadge health={deployment.health} />,
                  },
                  {
                    label: 'Ready / desired replicas',
                    value: `${String(deployment.readyReplicas ?? '—')} / ${String(deployment.desiredReplicas ?? '—')}`,
                    finding: fieldSeverity(deployment, 'replicas'),
                  },
                  {
                    label: 'Sealed condition',
                    value: sealed ? sealed.status : 'Not observed',
                    finding: fieldSeverity(deployment, 'sealed'),
                  },
                  ...(deployment.replica
                    ? [
                        { label: 'Role', value: roleLabels[deployment.role] },
                        {
                          label: 'Replica history',
                          value: deployment.replica.historyConflict
                            ? 'Conflict recorded'
                            : 'No conflict recorded',
                          finding: fieldSeverity(deployment, 'replicaHistory'),
                        },
                      ]
                    : []),
                  {
                    label: 'Requested version',
                    value: deployment.desiredVersion || 'Not observed',
                  },
                  { label: 'Resource generation', value: deployment.generation },
                ]}
              />
            </section>
            <section className="detail-section">
              <h2>
                <Clock3 size={17} />
                Evidence freshness
              </h2>
              <DetailRows
                rows={[
                  {
                    label: 'Collection result',
                    value: <EvidenceBadge collection={deployment.evidence.collection} />,
                    finding: fieldSeverity(deployment, 'collection'),
                  },
                  {
                    label: 'Last successful collection',
                    value: <Timestamp value={deployment.evidence.collectedAt} />,
                  },
                  {
                    label: 'Last attempted collection',
                    value: <Timestamp value={deployment.evidence.lastAttemptAt} />,
                  },
                  {
                    label: 'Controller observation',
                    value: (
                      <span title="This operator contract supplies no general observation heartbeat.">
                        Freshness unknown
                      </span>
                    ),
                  },
                  ...(deployment.replica
                    ? [
                        {
                          label: 'Replica observed',
                          value: <Timestamp value={deployment.replica.observedAt} />,
                        },
                      ]
                    : []),
                  {
                    label: 'Backup completion',
                    value: <Timestamp value={deployment.backup?.lastSuccessAt ?? null} />,
                  },
                ]}
              />
            </section>
          </div>
          {operator && <OperatorEvidence operator={operator} />}
          {deployment.issues.length > 1 && (
            <section className="detail-section">
              <h2>Other findings</h2>
              {deployment.issues
                .filter((item) => item.id !== issue?.id)
                .map((item) => {
                  const tab = findingLocation(item).tab
                  return (
                    <div className="finding-row" key={item.id}>
                      <SeverityBadge severity={item.severity} />
                      <div>
                        <strong>{item.title}</strong>
                        <p>{item.detail}</p>
                      </div>
                      {tab !== 'overview' && (
                        <button
                          type="button"
                          className="text-link"
                          onClick={() => {
                            selectTab(tab)
                          }}
                        >
                          View {tabLabels[tab].toLocaleLowerCase()}
                          <ArrowRight size={14} />
                        </button>
                      )}
                    </div>
                  )
                })}
            </section>
          )}
        </Tabs.Content>
        <Tabs.Content value="conditions">
          <section className="detail-section">
            <h2>Operator conditions</h2>
            <div className="table-surface">
              <table className="conditions-table">
                <thead>
                  <tr>
                    <th>Type</th>
                    <th>Status</th>
                    <th>Reason</th>
                    <th>Generation</th>
                    <th>Last transition</th>
                  </tr>
                </thead>
                <tbody>
                  {deployment.conditions.map((condition) => {
                    const finding =
                      condition.type === 'OpenBaoSealed'
                        ? fieldSeverity(deployment, 'sealed')
                        : null
                    return (
                      <tr
                        key={condition.type}
                        className={finding ? `finding-${finding}` : undefined}
                      >
                        <td>
                          <strong>{condition.type}</strong>
                        </td>
                        <td>{condition.status}</td>
                        <td>
                          <code>{condition.reason || 'Not observed'}</code>
                          {condition.message && <small>{condition.message}</small>}
                        </td>
                        <td>
                          {condition.observedGeneration} / {deployment.generation}
                        </td>
                        <td>
                          <Timestamp value={condition.lastTransitionAt} />
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
              {deployment.conditions.length === 0 && (
                <EmptyState title="No conditions reported">
                  The operator has not published status conditions for this resource.
                </EmptyState>
              )}
            </div>
          </section>
        </Tabs.Content>
        <Tabs.Content value="operations">
          <OperationsPanel deployment={deployment} />
        </Tabs.Content>
        <Tabs.Content value="backup">
          <BackupPanel deployment={deployment} />
        </Tabs.Content>
        <Tabs.Content value="history">
          <HistoryPanel id={deployment.id} />
        </Tabs.Content>
      </Tabs.Root>
    </div>
  )
}
