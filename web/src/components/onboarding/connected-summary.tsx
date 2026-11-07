import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { AlertCircle, Check, CircleDashed, Loader2, Pause, Plus } from 'lucide-react'
import { snapshotOptions } from '../../api/client'
import type { Deployment, Snapshot } from '../../api/schema'
import { emptyFilter } from '../../lib/filters'
import { collectionLabels } from '../../lib/format'
import { PageHeading } from '../page'
import { EvidenceBadge, HealthBadge } from '../status'
import { Button } from '../ui/button'

type ObservedConnection = Snapshot['connections'][number]

// Poll quickly until the collector reports its first result for the new cluster.
const firstCollectionInterval = 3_000

function collected(observed: ObservedConnection | undefined): observed is ObservedConnection {
  return observed !== undefined && observed.collection !== 'pending'
}

function FirstCollection({ observed }: { observed: ObservedConnection | undefined }) {
  if (!collected(observed))
    return (
      <li className="progress-active">
        <Loader2 size={16} className="spin" />
        Waiting for first collection…
      </li>
    )
  if (observed.collection === 'current')
    return (
      <li className="progress-done">
        <Check size={16} />
        First collection completed
      </li>
    )
  return (
    <li className="progress-failed">
      <AlertCircle size={16} />
      First collection: {collectionLabels[observed.collection].toLocaleLowerCase()}. Review the
      reader in cluster settings.
    </li>
  )
}

function Inventory({ count, collected }: { count: number; collected: boolean }): ReactNode {
  if (!collected)
    return (
      <li>
        <CircleDashed size={16} />
        Deployment inventory
      </li>
    )
  return (
    <li className="progress-done">
      <Check size={16} />
      {`${String(count)} ${count === 1 ? 'deployment' : 'deployments'} observed`}
    </li>
  )
}

export function ConnectedSummary({
  id,
  name,
  paused,
  preview,
}: {
  id: string
  name: string
  paused: boolean
  preview: readonly Deployment[]
}) {
  const { data: snapshot } = useQuery({
    ...snapshotOptions,
    refetchInterval: (query) => {
      const observed = query.state.data?.connections.find((item) => item.id === id)
      return paused || collected(observed)
        ? snapshotOptions.refetchInterval
        : firstCollectionInterval
    },
  })
  const observed = snapshot?.connections.find((item) => item.id === id)
  const done = collected(observed)
  const deployments = done
    ? (snapshot?.deployments.filter((item) => item.connectionId === id) ?? [])
    : preview
  return (
    <div className="page-enter onboarding-layout">
      <PageHeading title={paused ? 'Cluster saved paused' : 'Cluster connected'} />
      <section className="onboarding-summary">
        <h2>{name}</h2>
        <ol className="progress-list" role="status">
          <li className="progress-done">
            <Check size={16} />
            Configuration saved
          </li>
          {paused ? (
            <li>
              <Pause size={16} />
              Collection paused. Verify reader credentials in Settings before enabling collection.
            </li>
          ) : (
            <>
              <FirstCollection observed={observed} />
              <Inventory count={deployments.length} collected={done} />
            </>
          )}
        </ol>
        {deployments.length > 0 && (
          <section className="onboarding-preview">
            <h3>{done ? 'Observed deployments' : 'Verification preview'}</h3>
            <ul className="onboarding-result-list">
              {deployments.map((deployment) => (
                <li key={deployment.id}>
                  <span>
                    <strong>{deployment.name}</strong>
                    <small>{deployment.namespace}</small>
                  </span>
                  {done ? (
                    <HealthBadge health={deployment.health} />
                  ) : (
                    <EvidenceBadge collection="pending" />
                  )}
                </li>
              ))}
            </ul>
          </section>
        )}
        <div className="onboarding-actions">
          <Button asChild variant="ghost">
            <Link to="/settings/clusters/new" reloadDocument>
              <Plus size={15} />
              Add another cluster
            </Link>
          </Button>
          <Button asChild variant={paused ? 'default' : 'outline'}>
            {paused ? (
              <Link to="/settings">Open cluster settings</Link>
            ) : (
              <Link to="/topology/clusters/$clusterId" params={{ clusterId: id }}>
                View cluster
              </Link>
            )}
          </Button>
          {!paused && (
            <Button asChild variant="default">
              <Link to="/fleet" search={{ ...emptyFilter, cluster: id }}>
                View deployments
              </Link>
            </Button>
          )}
        </div>
      </section>
    </div>
  )
}
