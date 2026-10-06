import { Fragment, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { Cable, ChevronDown, ChevronRight, Plus } from 'lucide-react'
import { useSnapshot } from '../api/client'
import { emptyFilter } from '../lib/filters'
import { operatorHealthLabels } from '../lib/format'
import type { Operator, Snapshot } from '../api/schema'
import { EmptyState, PageHeading } from '../components/page'
import { EvidenceBadge, Timestamp } from '../components/status'
import { OperatorEvidence } from '../components/operator-evidence'
import { Button } from '../components/ui/button'

type Connection = Snapshot['connections'][number]

const capabilityLabels = {
  available: 'Available',
  unsupported: 'API absent',
  denied: 'Access denied',
  unreachable: 'Unreachable',
  notChecked: 'Not checked',
} as const
const capabilityHelp = {
  available: 'Namespace list succeeded.',
  unsupported: 'This API is not served. Install a compatible operator feature build if needed.',
  denied: 'Check reader RBAC and credential expiry. Previous inventory is retained.',
  unreachable: 'Check API reachability and the reader credential.',
  notChecked: 'A preceding read failed or collection has not started.',
} as const

function operatorNeedsAttention(operator: Operator): boolean {
  return operator.collection !== 'current' || operator.health !== 'available'
}

function needsAttention(connection: Connection): boolean {
  return (
    connection.collection !== 'current' ||
    // An absent optional API is a capability limit, not a collection gap.
    connection.capabilities.some(
      (capability) => capability.state === 'denied' || capability.state === 'unreachable',
    ) ||
    connection.operators.some(operatorNeedsAttention)
  )
}

function OperatorSummary({ operators }: { operators: readonly Operator[] }) {
  if (operators.length === 0) return <span className="muted">Not enrolled</span>
  const failing = operators.filter(operatorNeedsAttention)
  const first = failing[0]
  if (!first)
    return (
      <span className="badge status-healthy">
        <span className="status-dot" />
        {operators.length === 1 ? 'Available' : `${String(operators.length)} available`}
      </span>
    )
  return (
    <span className="badge status-attention">
      <span className="status-dot" />
      {failing.length === 1
        ? first.collection === 'current'
          ? operatorHealthLabels[first.health]
          : 'Evidence missing'
        : `${String(failing.length)} need attention`}
    </span>
  )
}

function ClusterDetails({ connection }: { connection: Connection }) {
  return (
    <div className="cluster-details">
      {connection.capabilities.length > 0 && (
        <section>
          <h3>Resource API access</h3>
          <div className="capability-table">
            <table>
              <thead>
                <tr>
                  <th>Namespace</th>
                  <th>Resource API</th>
                  <th>Access</th>
                  <th>Result</th>
                </tr>
              </thead>
              <tbody>
                {connection.capabilities.map((capability) => (
                  <tr
                    key={`${capability.namespace}/${capability.resource}`}
                    className={capability.state === 'available' ? '' : 'capability-problem'}
                  >
                    <td>{capability.namespace}</td>
                    <td>
                      <code>{capability.resource}</code>
                    </td>
                    <td>{capabilityLabels[capability.state]}</td>
                    <td className="muted">{capabilityHelp[capability.state]}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}
      {connection.operators.length > 0 ? (
        connection.operators.map((operator) => (
          <OperatorEvidence key={operator.id} operator={operator} />
        ))
      ) : (
        <p className="section-footnote">
          No controller Deployment is enrolled. Operator health and association remain unknown.
        </p>
      )}
    </div>
  )
}

export function ClustersPage() {
  const { data: snapshot } = useSnapshot()
  // Clusters with gaps start expanded so their cause is visible without a click.
  const [expanded, setExpanded] = useState<ReadonlySet<string> | null>(null)
  if (!snapshot) return null
  const open =
    expanded ??
    new Set(snapshot.connections.filter(needsAttention).map((connection) => connection.id))
  const toggle = (id: string) => {
    const next = new Set(open)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    setExpanded(next)
  }
  const gaps = snapshot.connections.filter(needsAttention).length
  return (
    <div className="page-enter">
      <PageHeading
        title="Clusters"
        actions={
          snapshot.viewer.admin && snapshot.mode === 'live' ? (
            <Button asChild>
              <Link to="/settings/clusters/new">
                <Plus size={15} />
                Add cluster
              </Link>
            </Button>
          ) : undefined
        }
      />
      <div className="summary-line">
        <span className="summary-number">
          <Cable size={15} />
          {snapshot.connections.length} Kubernetes{' '}
          {snapshot.connections.length === 1 ? 'cluster' : 'clusters'}
        </span>
        <span>{gaps} need attention</span>
        <span>
          {snapshot.connections.reduce((total, item) => total + item.namespaces.length, 0)}{' '}
          authorized namespaces
        </span>
      </div>
      {snapshot.connections.length === 0 ? (
        <EmptyState title="No clusters enrolled" icon={Cable}>
          {snapshot.viewer.admin
            ? 'Add a Kubernetes cluster to start collecting OpenBao resources.'
            : 'Ask a console administrator to assign namespace access.'}
        </EmptyState>
      ) : (
        <div className="table-surface">
          <table className="clusters-table">
            <thead>
              <tr>
                <th>Cluster</th>
                <th>Collection</th>
                <th>Namespaces</th>
                <th>Deployments</th>
                <th>Operator</th>
                <th>Last collected</th>
                <th aria-label="Details" />
              </tr>
            </thead>
            <tbody>
              {snapshot.connections.map((connection) => {
                const isOpen = open.has(connection.id)
                return (
                  <Fragment key={connection.id}>
                    <tr className={isOpen ? 'cluster-row expanded' : 'cluster-row'}>
                      <td>
                        <button
                          type="button"
                          className="cluster-toggle"
                          aria-expanded={isOpen}
                          aria-controls={`cluster-${connection.id}`}
                          onClick={() => {
                            toggle(connection.id)
                          }}
                        >
                          {isOpen ? <ChevronDown size={15} /> : <ChevronRight size={15} />}
                          <span>
                            <strong>{connection.name}</strong>
                            <small>{connection.region || connection.id}</small>
                          </span>
                        </button>
                      </td>
                      <td data-label="Collection">
                        <EvidenceBadge collection={connection.collection} />
                      </td>
                      <td data-label="Namespaces">
                        <span className="namespace-list">{connection.namespaces.join(', ')}</span>
                      </td>
                      <td data-label="Deployments">
                        <Link
                          to="/fleet"
                          search={{ ...emptyFilter, cluster: connection.id }}
                          className="text-link"
                        >
                          {
                            snapshot.deployments.filter(
                              (item) => item.connectionId === connection.id,
                            ).length
                          }
                        </Link>
                      </td>
                      <td data-label="Operator">
                        <OperatorSummary operators={connection.operators} />
                      </td>
                      <td data-label="Last collected" className="time-cell">
                        <Timestamp value={connection.lastSuccessAt} />
                      </td>
                      <td />
                    </tr>
                    {isOpen && (
                      <tr className="cluster-detail-row" id={`cluster-${connection.id}`}>
                        <td colSpan={7}>
                          <ClusterDetails connection={connection} />
                        </td>
                      </tr>
                    )}
                  </Fragment>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
      <p className="section-footnote">
        {snapshot.mode === 'demo'
          ? 'These clusters are illustrative fixtures. Use live enrollment to collect real evidence.'
          : 'The reader projects approved metadata and status. Credentials remain on the server.'}
      </p>
    </div>
  )
}
