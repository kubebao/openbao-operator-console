import { Link, useParams } from '@tanstack/react-router'
import { AlertTriangle, ArrowLeft, ArrowRight, CheckCircle2, Cpu } from 'lucide-react'
import { useSnapshot } from '../api/client'
import type { Deployment, Operator, Snapshot } from '../api/schema'
import { Breadcrumbs } from '../components/breadcrumbs'
import { InfoTip } from '../components/info-tip'
import { OperatorRows } from '../components/operator-evidence'
import { PageHeading } from '../components/page'
import {
  EvidenceBadge,
  HealthBadge,
  OperatorCollectionBadge,
  OperatorHealthBadge,
  Timestamp,
} from '../components/status'
import { useDocumentTitle } from '../components/use-document-title'
import { capabilityCheckLabels, environmentLabels, plural } from '../lib/format'
import type { CapabilityGroup } from '../lib/topology'
import { groupCapabilities, operatorGroups } from '../lib/topology'

type Connection = Snapshot['connections'][number]

// An available API needs no explanation; the other states say what to check.
const capabilityHelp = {
  available: '',
  unsupported: 'This API is not served. Install a compatible operator feature build if needed.',
  denied: 'Check reader RBAC and credential expiry. Previous inventory is retained.',
  unreachable: 'Check API reachability and the reader credential.',
  notChecked: 'A preceding read failed or collection has not started.',
} as const

// Each kind reads from the deployment's side: [as source, as target].
const verbs = {
  replication: ['Replicates to', 'Replica of'],
  snapshotRecovery: ['Recovers to', 'Recovers from'],
} as const

function RelationshipLinks({
  deployment,
  connection,
  snapshot,
}: {
  deployment: Deployment
  connection: Connection
  snapshot: Snapshot
}) {
  const relationships = snapshot.relationships.filter(
    (item) => item.sourceId === deployment.id || item.targetId === deployment.id,
  )
  if (relationships.length === 0) return <span className="muted">—</span>
  return (
    <ul className="relationship-stubs">
      {relationships.map((relationship) => {
        const outgoing = relationship.sourceId === deployment.id
        const other = snapshot.deployments.find(
          (item) => item.id === (outgoing ? relationship.targetId : relationship.sourceId),
        )
        const otherCluster = snapshot.connections.find((item) => item.id === other?.connectionId)
        const verb = verbs[relationship.kind][outgoing ? 0 : 1]
        return (
          <li key={relationship.id}>
            <Link
              to="/topology/relationships/$relationshipId"
              params={{ relationshipId: relationship.id }}
              className="text-link"
            >
              {verb} {other ? `${other.namespace}/${other.name}` : 'an unavailable deployment'}
              {otherCluster && otherCluster.id !== connection.id && ` (${otherCluster.name})`}
            </Link>
          </li>
        )
      })}
    </ul>
  )
}

function DeploymentTable({
  deployments,
  connection,
  snapshot,
}: {
  deployments: readonly Deployment[]
  connection: Connection
  snapshot: Snapshot
}) {
  // Namespace order groups the table; the namespace is written once per group.
  const rows = [...deployments].sort((a, b) => a.namespace.localeCompare(b.namespace))
  return (
    <div className="table-surface cluster-deployments">
      <table>
        <thead>
          <tr>
            <th>Namespace</th>
            <th>Deployment</th>
            <th>State</th>
            <th>Team</th>
            <th>Environment</th>
            <th>OpenBao</th>
            <th>Findings</th>
            <th>Relationships</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((deployment, index) => {
            const first = rows[index - 1]?.namespace !== deployment.namespace
            return (
              <tr
                key={deployment.id}
                className={first && index > 0 ? 'namespace-start' : undefined}
              >
                <td data-label="Namespace" className="muted">
                  {first ? deployment.namespace : ''}
                </td>
                <td>
                  <Link
                    to="/deployments/$deploymentId"
                    params={{ deploymentId: deployment.id }}
                    className="row-main-link"
                  >
                    {deployment.name}
                  </Link>
                </td>
                <td data-label="State">
                  <HealthBadge health={deployment.health} />
                </td>
                <td data-label="Team">{deployment.team}</td>
                <td data-label="Environment">{environmentLabels[deployment.environment]}</td>
                <td data-label="OpenBao">{deployment.version || 'Not observed'}</td>
                <td data-label="Findings">
                  <span className={deployment.issues.length ? 'finding-count' : 'muted'}>
                    {deployment.issues.length}
                  </span>
                </td>
                <td data-label="Relationships">
                  <RelationshipLinks
                    deployment={deployment}
                    connection={connection}
                    snapshot={snapshot}
                  />
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

function OperatorPanel({
  operator,
  deployments,
  connection,
  snapshot,
}: {
  operator: Operator | null
  deployments: readonly Deployment[]
  connection: Connection
  snapshot: Snapshot
}) {
  return (
    <section className="cluster-panel">
      <div className="cluster-panel-heading">
        {operator ? (
          <>
            <h2>
              <Cpu size={16} />
              {operator.id}
            </h2>
            <InfoTip label="About operator evidence">
              {operator.collection !== 'current' ? 'Workload facts are last known. ' : ''}
              Deployment readiness does not prove successful reconciliation. The version label does
              not verify the binary.
            </InfoTip>
            <OperatorHealthBadge operator={operator} />
            {/* An uncollected cluster already says so in its heading. */}
            {operator.collection !== 'current' && connection.collection === 'current' && (
              <OperatorCollectionBadge collection={operator.collection} />
            )}
            <span className="muted">
              {plural(deployments.length, 'deployment')} in {operator.namespaces.join(', ')}
            </span>
          </>
        ) : (
          <>
            <h2>
              <AlertTriangle size={16} />
              No enrolled operator
            </h2>
            <span className="muted">
              No enrolled operator declares these namespaces, so responsibility is unknown.
            </span>
          </>
        )}
      </div>
      {deployments.length > 0 ? (
        <DeploymentTable deployments={deployments} connection={connection} snapshot={snapshot} />
      ) : (
        <p className="cluster-panel-empty">No OpenBao deployments observed in its namespaces.</p>
      )}
      {operator && (
        <details className="controller-details">
          <summary>Controller details</summary>
          <OperatorRows operator={operator} />
        </details>
      )}
    </section>
  )
}

function ResourceNames({ resources }: { resources: readonly string[] }) {
  return resources.map((resource, index) => (
    <span key={resource}>
      {index > 0 && ', '}
      <code>{resource}</code>
    </span>
  ))
}

function scope(group: CapabilityGroup, connection: Connection): string {
  return group.namespaces.length === connection.namespaces.length &&
    connection.namespaces.length > 1
    ? 'All namespaces'
    : group.namespaces.join(', ')
}

function ResourceAccess({ connection }: { connection: Connection }) {
  const groups = groupCapabilities(connection.capabilities)
  if (groups.length === 0) return null
  const problems = groups.filter((group) => group.state !== 'available')
  const accessible = groups.filter((group) => group.state === 'available')
  return (
    <section className="cluster-panel">
      <div className="cluster-panel-heading">
        <h2>Resource API access</h2>
      </div>
      {problems.length > 0 && (
        <div className="table-surface cluster-access">
          <table>
            <thead>
              <tr>
                <th>Resource APIs</th>
                <th>Access</th>
                <th>Namespaces</th>
                <th>Next step</th>
              </tr>
            </thead>
            <tbody>
              {problems.map((group) => (
                <tr key={`${group.state}/${group.namespaces.join(',')}`}>
                  <td>
                    <ResourceNames resources={group.resources} />
                  </td>
                  <td className="capability-state">{capabilityCheckLabels[group.state]}</td>
                  <td>{scope(group, connection)}</td>
                  <td className="muted">{capabilityHelp[group.state]}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {accessible.map((group) => (
        <p key={group.namespaces.join(',')} className="cluster-access-ok">
          <CheckCircle2 size={15} />
          <span>
            Accessible: <ResourceNames resources={group.resources} />
            {/* The namespaces only matter when access differs between them. */}
            {(problems.length > 0 || accessible.length > 1) &&
              ` in ${scope(group, connection).toLocaleLowerCase()}`}
          </span>
        </p>
      ))}
    </section>
  )
}

// TopologyClusterPage is the second topology level: one Kubernetes cluster, its operators and
// their deployments, the relationships that leave it, and the reader's API access.
export function TopologyClusterPage() {
  const { clusterId } = useParams({ from: '/topology/clusters/$clusterId' })
  const { data: snapshot } = useSnapshot()
  const connection = snapshot?.connections.find((item) => item.id === clusterId)
  useDocumentTitle(connection?.name ?? 'Cluster')
  if (!snapshot) return null
  if (!connection)
    return (
      <div className="page-enter">
        <Link to="/topology" className="back-link">
          <ArrowLeft size={14} />
          Back to topology
        </Link>
        <PageHeading
          title="Cluster unavailable"
          description="The cluster is not enrolled or is outside your access scope."
        />
      </div>
    )
  const deployments = snapshot.deployments.filter((item) => item.connectionId === connection.id)
  const collected = connection.collection === 'current'
  return (
    <div className="page-enter">
      <Breadcrumbs>
        {[
          <Link key="topology" to="/topology">
            Topology
          </Link>,
          connection.name,
        ]}
      </Breadcrumbs>
      <PageHeading
        title={connection.name}
        status={collected ? undefined : <EvidenceBadge collection={connection.collection} />}
        actions={
          snapshot.viewer.admin && snapshot.mode === 'live' ? (
            <Link to="/settings" search={{ section: 'connections' }} className="text-link">
              Manage cluster
              <ArrowRight size={14} />
            </Link>
          ) : undefined
        }
      />
      <div className="summary-line">
        {connection.region && <span>{connection.region}</span>}
        <span>{plural(connection.namespaces.length, 'namespace')}</span>
        <span>{plural(deployments.length, 'deployment')}</span>
        <span>
          Collected <Timestamp value={connection.lastSuccessAt} />
        </span>
      </div>
      {!collected && (
        <div className="notice notice-warning" role="status">
          <AlertTriangle size={17} aria-hidden="true" />
          <span>
            The console cannot collect from this cluster. The states below are last known.
          </span>
        </div>
      )}
      {operatorGroups(connection, deployments).map((group) => (
        <OperatorPanel
          key={group.operator?.id ?? 'unmanaged'}
          operator={group.operator}
          deployments={group.deployments}
          connection={connection}
          snapshot={snapshot}
        />
      ))}
      {connection.operators.length === 0 && deployments.length === 0 && (
        <p className="cluster-panel-empty">
          No operator is enrolled and no deployments are observed.
        </p>
      )}
      <ResourceAccess connection={connection} />
    </div>
  )
}
