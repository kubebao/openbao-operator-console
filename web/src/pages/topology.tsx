import { Link } from '@tanstack/react-router'
import { ArrowRight, Cable } from 'lucide-react'
import { useSnapshot } from '../api/client'
import { EmptyState, PageHeading } from '../components/page'
import { FleetMap } from '../components/topology/fleet-map'
import { plural } from '../lib/format'

function Legend({ admin }: { admin: boolean }) {
  return (
    <div className="map-legend">
      <span>
        <svg width="28" height="8" aria-hidden="true">
          <line x1="0" y1="4" x2="28" y2="4" className="legend-replication" />
        </svg>
        Read replica
      </span>
      <span>
        <svg width="28" height="8" aria-hidden="true">
          <line x1="0" y1="4" x2="28" y2="4" className="legend-recovery" />
        </svg>
        Snapshot recovery
      </span>
      {admin && (
        <Link to="/settings" search={{ section: 'relationships' }} className="text-link">
          Manage relationships
          <ArrowRight size={14} />
        </Link>
      )}
    </div>
  )
}

// TopologyPage is the top level of the topology: clusters, their operators, the deployments
// they manage, and the declared relationships between deployments.
export function TopologyPage() {
  const { data: snapshot } = useSnapshot()
  if (!snapshot) return null
  const admin = snapshot.viewer.admin
  return (
    <div className="page-enter">
      <PageHeading title="Topology" actions={<Legend admin={admin} />} />
      <div className="summary-line">
        <span className="summary-number">
          <Cable size={15} />
          {plural(snapshot.connections.length, 'Kubernetes cluster')}
        </span>
        <span>
          {plural(
            snapshot.connections.reduce((total, item) => total + item.operators.length, 0),
            'operator',
          )}
        </span>
        <span>{plural(snapshot.deployments.length, 'deployment')}</span>
        <span>{plural(snapshot.relationships.length, 'relationship')}</span>
      </div>
      {snapshot.connections.length === 0 ? (
        <EmptyState title="No clusters enrolled" icon={Cable}>
          {admin && snapshot.mode === 'live' ? (
            <Link to="/settings/clusters/new">Add a Kubernetes cluster</Link>
          ) : (
            'Ask a console administrator to assign namespace access.'
          )}
        </EmptyState>
      ) : (
        <FleetMap snapshot={snapshot} />
      )}
    </div>
  )
}
