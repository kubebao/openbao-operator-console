import { Link } from '@tanstack/react-router'
import { ArrowRight, RefreshCw } from 'lucide-react'
import { useActivity, useSnapshot } from '../api/client'
import type { Snapshot } from '../api/schema'
import { plural } from '../lib/format'
import { EvidenceBanners } from '../components/attention/evidence-banners'
import { FleetOverview } from '../components/attention/fleet-overview'
import { IssueTable } from '../components/attention/issue-table'
import { PageHeading } from '../components/page'
import { RecentActivity } from '../components/recent-activity'
import { Button } from '../components/ui/button'

// A live console without enrolled connections offers onboarding instead of an empty,
// apparently healthy fleet.
function InitialFleet({ snapshot }: { snapshot: Snapshot }) {
  const admin = snapshot.viewer.admin
  return (
    <div className="page-enter initial-fleet">
      <PageHeading title={admin ? 'Connect your first cluster' : 'No namespaces assigned'} />
      <p>
        {admin
          ? 'Add a Kubernetes cluster to observe OpenBao deployments.'
          : 'Ask a console administrator to assign namespace access.'}
      </p>
      {admin && (
        <Link to="/settings/clusters/new" className="button button-primary">
          Connect cluster <ArrowRight size={16} />
        </Link>
      )}
    </div>
  )
}

export function AttentionPage() {
  const { data: snapshot, refetch } = useSnapshot()
  const activity = useActivity(Boolean(snapshot))
  let activityStatus = 'No changes recorded.'
  if (activity.isPending) activityStatus = 'Loading recorded activity…'
  if (activity.isError) activityStatus = 'Recorded activity is unavailable.'
  if (!snapshot) return null
  if (snapshot.mode === 'live' && snapshot.connections.length === 0)
    return <InitialFleet snapshot={snapshot} />
  const findings = snapshot.deployments.flatMap((deployment) => deployment.issues)
  const affected = new Set(findings.map((issue) => issue.deploymentId)).size
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
          {plural(findings.length, 'finding')}
        </span>
        <span>{plural(affected, 'affected deployment')}</span>
        <span>{plural(snapshot.deployments.length, 'deployment')} observed</span>
      </div>
      <EvidenceBanners snapshot={snapshot} />
      <IssueTable snapshot={snapshot} />
      <div className="attention-support">
        <FleetOverview snapshot={snapshot} />
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
          {activity.data?.length ? (
            <RecentActivity events={activity.data.slice(0, 5)} />
          ) : (
            <p className="muted">{activityStatus}</p>
          )}
        </section>
      </div>
    </div>
  )
}
