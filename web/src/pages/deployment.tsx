import * as Tabs from '@radix-ui/react-tabs'
import { Link, useNavigate, useParams, useSearch } from '@tanstack/react-router'
import {
  AlertTriangle,
  ArrowLeft,
  Boxes,
  Cable,
  Cpu,
  Database,
  Folder,
  Layers3,
  SearchX,
} from 'lucide-react'
import { useHistory, useSnapshot } from '../api/client'
import type { Issue } from '../api/schema'
import { BackupsTab } from '../components/deployment/backups'
import { ConditionsTab } from '../components/deployment/conditions'
import { HistoryTab } from '../components/deployment/history'
import { LifecycleTab } from '../components/deployment/lifecycle'
import { OverviewTab } from '../components/deployment/overview'
import { EmptyState } from '../components/page'
import { EvidenceBadge, HealthBadge, OperatorHealthBadge, Timestamp } from '../components/status'
import type { DeploymentTab } from '../lib/findings'
import { tabLabels, tabSeverity } from '../lib/findings'
import { age, collectionLabels, environmentLabels } from '../lib/format'

const tabs: readonly DeploymentTab[] = ['overview', 'lifecycle', 'backups', 'conditions', 'history']

const severityWords: Record<Issue['severity'], string> = {
  critical: 'critical finding',
  high: 'finding',
  info: 'note',
}

export function DeploymentPage() {
  const { deploymentId } = useParams({ from: '/deployments/$deploymentId' })
  const search = useSearch({ from: '/deployments/$deploymentId' })
  const navigate = useNavigate()
  const { data: snapshot } = useSnapshot()
  const history = useHistory(deploymentId)
  const deployment = snapshot?.deployments.find((item) => item.id === deploymentId)
  if (!snapshot || !deployment)
    return (
      <EmptyState title="Deployment not found" icon={SearchX}>
        This deployment is unavailable or outside your access scope.
      </EmptyState>
    )
  const connection = snapshot.connections.find((item) => item.id === deployment.connectionId)
  const operator = connection?.operators.find((item) => item.id === deployment.operatorId)
  const observations = history.data?.observations ?? null
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
      <header className="deployment-header">
        <div className="page-title">
          <h1>{deployment.name}</h1>
          <HealthBadge health={deployment.health} />
        </div>
        <ul className="deployment-meta" aria-label="Deployment details">
          <li title="Namespace">
            <Folder size={14} aria-hidden="true" />
            <span className="sr-only">Namespace </span>
            {deployment.namespace}
          </li>
          <li title="Team">
            <Boxes size={14} aria-hidden="true" />
            <span className="sr-only">Team </span>
            {deployment.team}
          </li>
          <li title="Environment">
            <Layers3 size={14} aria-hidden="true" />
            <span className="sr-only">Environment </span>
            {environmentLabels[deployment.environment]}
          </li>
          <li title="Kubernetes cluster">
            <Cable size={14} aria-hidden="true" />
            <span className="sr-only">Kubernetes cluster </span>
            {connection?.name ?? deployment.connectionId}
            {deployment.location && <span className="muted"> · {deployment.location}</span>}
          </li>
          <li title="OpenBao version">
            <Database size={14} aria-hidden="true" />
            OpenBao {deployment.version || 'version not observed'}
          </li>
        </ul>
        <div className="deployment-context">
          <span>
            <Cpu size={14} aria-hidden="true" />
            Managed by{' '}
            {operator ? (
              <Link to="/clusters" className="text-link">
                {operator.id}
              </Link>
            ) : (
              <span>{deployment.operatorId ?? 'no declared operator'}</span>
            )}
            {operator && <OperatorHealthBadge health={operator.health} />}
          </span>
          <span>
            {deployment.evidence.collection === 'current' ? (
              <>
                Last collected: <Timestamp value={deployment.evidence.collectedAt} />
              </>
            ) : (
              <EvidenceBadge collection={deployment.evidence.collection} />
            )}
          </span>
        </div>
      </header>
      {deployment.evidence.collection !== 'current' && (
        <div className="notice notice-warning">
          <AlertTriangle size={17} />
          <span>
            <strong>Current state is uncertain.</strong> Collection result:{' '}
            {collectionLabels[deployment.evidence.collection].toLocaleLowerCase()}. Last successful
            collection {age(deployment.evidence.collectedAt)}. The values below are last known.
          </span>
        </div>
      )}
      <Tabs.Root
        value={search.tab}
        onValueChange={(value) => {
          const tab = tabs.find((item) => item === value)
          if (tab) selectTab(tab)
        }}
        className="detail-tabs"
      >
        <Tabs.List aria-label="Deployment sections">
          {tabs.map((tab) => {
            const severity = tabSeverity(deployment, tab)
            return (
              <Tabs.Trigger value={tab} key={tab}>
                {tabLabels[tab]}
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
          <OverviewTab
            deployment={deployment}
            snapshot={snapshot}
            operator={operator}
            observations={observations}
            onSelect={selectTab}
          />
        </Tabs.Content>
        <Tabs.Content value="lifecycle">
          <LifecycleTab deployment={deployment} operator={operator} />
        </Tabs.Content>
        <Tabs.Content value="backups">
          <BackupsTab
            deployment={deployment}
            maxBackupAgeHours={snapshot.policy.maxBackupAgeHours}
            observations={observations}
            now={Date.parse(snapshot.collectedAt)}
          />
        </Tabs.Content>
        <Tabs.Content value="conditions">
          <ConditionsTab deployment={deployment} observations={observations} />
        </Tabs.Content>
        <Tabs.Content value="history">
          <HistoryTab id={deployment.id} now={Date.parse(snapshot.collectedAt)} />
        </Tabs.Content>
      </Tabs.Root>
    </div>
  )
}
