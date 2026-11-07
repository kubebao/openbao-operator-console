import { Link } from '@tanstack/react-router'
import { ArrowRight } from 'lucide-react'
import type { Deployment, Snapshot } from '../../api/schema'
import { environments } from '../../lib/connection'
import { environmentLabels, healthLabels, plural } from '../../lib/format'

const healthStates = ['healthy', 'attention', 'critical', 'unknown'] as const

function share(deployments: readonly Deployment[], health: Deployment['health']): number {
  if (deployments.length === 0) return 0
  return (deployments.filter((item) => item.health === health).length / deployments.length) * 100
}

export function FleetOverview({ snapshot }: { snapshot: Snapshot }) {
  return (
    <section>
      <div className="section-heading">
        <div>
          <h2>Fleet overview</h2>
          <p>
            {plural(snapshot.deployments.length, 'deployment')} across{' '}
            {plural(snapshot.connections.length, 'Kubernetes cluster')}
          </p>
        </div>
        <Link to="/fleet" className="text-link">
          View fleet
          <ArrowRight size={15} />
        </Link>
      </div>
      <div className="health-legend">
        {healthStates.map((health) => (
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
          // An environment without deployments adds an empty bar and no information.
          if (deployments.length === 0) return null
          return (
            <div key={environment}>
              <span className="environment-label">{environmentLabels[environment]}</span>
              <div className="stacked-bar">
                {healthStates.map((health) => (
                  <span
                    key={health}
                    className={`bar-${health}`}
                    style={{ width: `${String(share(deployments, health))}%` }}
                  />
                ))}
              </div>
              <strong>{deployments.length}</strong>
            </div>
          )
        })}
      </div>
    </section>
  )
}
