import * as Tabs from '@radix-ui/react-tabs'
import { useState } from 'react'
import { Link, useNavigate, useParams, useSearch } from '@tanstack/react-router'
import { AlertTriangle, ArrowLeft } from 'lucide-react'
import { useHistory, useSnapshot } from '../api/client'
import type { Issue, Snapshot } from '../api/schema'
import { BackupsTab } from '../components/deployment/backups'
import { ConditionsTab } from '../components/deployment/conditions'
import { DeploymentHeader } from '../components/deployment/header'
import { HistoryTab } from '../components/deployment/history'
import { LifecycleTab } from '../components/deployment/lifecycle'
import { OverviewTab } from '../components/deployment/overview'
import { PageHeading } from '../components/page'
import { useDocumentTitle } from '../components/use-document-title'
import type { DeploymentTab } from '../lib/findings'
import { deploymentTabs, tabLabels, tabSeverity } from '../lib/findings'
import { age, collectionLabels } from '../lib/format'

const severityWords: Record<Issue['severity'], string> = {
  critical: 'critical finding',
  high: 'finding',
  info: 'note',
}

export function DeploymentPage() {
  const { data: snapshot } = useSnapshot()
  if (!snapshot) return null
  return <DeploymentDetails snapshot={snapshot} />
}

function DeploymentDetails({ snapshot }: { snapshot: Snapshot }) {
  const { deploymentId } = useParams({ from: '/deployments/$deploymentId' })
  const search = useSearch({ from: '/deployments/$deploymentId' })
  const navigate = useNavigate()
  // Anchor the window to verified server time and keep its cursor query stable while
  // fleet polling refreshes the snapshot.
  const [since] = useState(() =>
    new Date(Date.parse(snapshot.generatedAt) - 7 * 24 * 60 * 60 * 1000).toISOString(),
  )
  const history = useHistory(deploymentId, since)
  const deployment = snapshot.deployments.find((item) => item.id === deploymentId)
  useDocumentTitle(deployment?.name ?? 'Deployment not found')
  if (!deployment)
    return (
      <div className="page-enter">
        <Link to="/fleet" className="back-link">
          <ArrowLeft size={14} />
          Back to fleet
        </Link>
        <PageHeading
          title="Deployment not found"
          description="This deployment is unavailable or outside your access scope."
        />
      </div>
    )
  const connection = snapshot.connections.find((item) => item.id === deployment.connectionId)
  const operator = connection?.operators.find((item) => item.id === deployment.operatorId)
  const observations = history.data
    ? [
        ...(!history.hasNextPage && history.data.baseline ? [history.data.baseline] : []),
        ...history.data.observations,
      ]
    : null
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
      <DeploymentHeader deployment={deployment} connection={connection} operator={operator} />
      {history.hasNextPage && (
        <p className="notice">
          Older observations are available. Load them in History to extend the charts.
        </p>
      )}
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
          const tab = deploymentTabs.find((item) => item === value)
          if (tab) selectTab(tab)
        }}
        className="detail-tabs"
      >
        <Tabs.List aria-label="Deployment sections">
          {deploymentTabs.map((tab) => {
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
            earlierUnloaded={history.hasNextPage}
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
            now={Date.parse(snapshot.generatedAt)}
          />
        </Tabs.Content>
        <Tabs.Content value="conditions">
          <ConditionsTab deployment={deployment} observations={observations} />
        </Tabs.Content>
        <Tabs.Content value="history">
          <HistoryTab id={deployment.id} since={since} now={Date.parse(snapshot.generatedAt)} />
        </Tabs.Content>
      </Tabs.Root>
    </div>
  )
}
