import { useLayoutEffect, useRef, useState } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { AlertTriangle, ChevronRight, Cpu } from 'lucide-react'
import type { Deployment, Relationship, Snapshot } from '../../api/schema'
import {
  age,
  collectionLabels,
  healthLabels,
  operatorHealthText,
  plural,
  relationshipKindLabels,
} from '../../lib/format'
import { operatorNeedsAttention } from '../../lib/tone'
import type { Box, Point } from '../../lib/topology'
import {
  linkedDeployments,
  operatorGroups,
  roundedPath,
  routeEdge,
  visibleOnMap,
} from '../../lib/topology'
import { StatusBadge } from '../status'

type Connection = Snapshot['connections'][number]

const tallyOrder = ['critical', 'attention', 'unknown', 'healthy'] as const
const edgeLabels: Record<Relationship['kind'], string> = {
  replication: 'Replica',
  snapshotRecovery: 'Recovery',
}

interface DrawnEdge {
  relationship: Relationship
  path: string
  ports: readonly [Point, Point]
  label: { x: number; y: number; width: number } | null
}

function HealthTally({ deployments }: { deployments: readonly Deployment[] }) {
  return (
    <span className="health-tally">
      {tallyOrder.map((health) => {
        const count = deployments.filter((item) => item.health === health).length
        if (count === 0) return null
        return (
          <span key={health} title={healthLabels[health]}>
            <span className={`legend-dot status-${health}`} aria-hidden="true" />
            <span className="sr-only">{healthLabels[health]} </span>
            {count}
          </span>
        )
      })}
    </span>
  )
}

function DeploymentRow({
  deployment,
  linked,
  hot,
}: {
  deployment: Deployment
  linked: boolean
  hot: boolean
}) {
  return (
    <Link
      to="/deployments/$deploymentId"
      params={{ deploymentId: deployment.id }}
      data-deployment-id={deployment.id}
      className={`map-deployment${linked ? ' linked' : ''}${hot ? ' hot' : ''}`}
    >
      <span className={`legend-dot status-${deployment.health}`} aria-hidden="true" />
      <span className="sr-only">{healthLabels[deployment.health]} </span>
      <span className="map-deployment-name">{deployment.name}</span>
      <span className="map-deployment-namespace">{deployment.namespace}</span>
    </Link>
  )
}

function ClusterCard({
  connection,
  deployments,
  linked,
  hot,
}: {
  connection: Connection
  deployments: readonly Deployment[]
  linked: ReadonlySet<string>
  hot: ReadonlySet<string>
}) {
  const collected = connection.collection === 'current'
  const groups = operatorGroups(connection, deployments)
  return (
    <article className={`map-cluster${collected ? '' : ' map-cluster-gap'}`}>
      <Link
        to="/topology/clusters/$clusterId"
        params={{ clusterId: connection.id }}
        className="map-cluster-head"
      >
        <span>
          <h2>{connection.name}</h2>
          <small>{connection.region || connection.id}</small>
        </span>
        {collected ? (
          <ChevronRight size={15} aria-hidden="true" />
        ) : (
          <StatusBadge tone="attention">{collectionLabels[connection.collection]}</StatusBadge>
        )}
      </Link>
      {!collected && (
        <p className="map-cluster-note">
          Last collected {age(connection.lastSuccessAt).toLocaleLowerCase()}. States are last known.
        </p>
      )}
      {groups.map(({ operator, deployments: managed }) => {
        const { visible, hidden } = visibleOnMap(managed, linked)
        // On an uncollected cluster the card already says so; the operator line stays quiet.
        const problem = operator !== null && collected && operatorNeedsAttention(operator)
        return (
          <section key={operator?.id ?? 'unmanaged'} className="map-operator">
            {operator ? (
              <h3 className={problem ? 'map-operator-problem' : undefined}>
                <Cpu size={12} aria-hidden="true" />
                {operator.id}
                {problem && <span> · {operatorHealthText(operator)}</span>}
              </h3>
            ) : (
              <h3 className="map-operator-problem">
                <AlertTriangle size={12} aria-hidden="true" />
                No enrolled operator
              </h3>
            )}
            {visible.map((deployment) => (
              <DeploymentRow
                key={deployment.id}
                deployment={deployment}
                linked={linked.has(deployment.id)}
                hot={hot.has(deployment.id)}
              />
            ))}
            {hidden.length > 0 && (
              <Link
                to="/topology/clusters/$clusterId"
                params={{ clusterId: connection.id }}
                className="map-collapsed"
              >
                {visible.length > 0 ? '+ ' : ''}
                {plural(hidden.length, 'deployment')}
                <HealthTally deployments={hidden} />
              </Link>
            )}
            {managed.length === 0 && <p className="map-empty">No deployments observed.</p>}
          </section>
        )
      })}
      {groups.length === 0 && <p className="map-empty">No operator enrolled.</p>}
    </article>
  )
}

// measureEdges routes every relationship whose endpoints are on the map. Lines that share a
// gap or lane are offset so they stay distinguishable.
function measureEdges(map: HTMLElement, relationships: readonly Relationship[]): DrawnEdge[] {
  const grid = map.querySelector('.map-grid')
  if (!(grid instanceof HTMLElement)) return []
  const style = getComputedStyle(grid)
  // One column leaves no gap beside the cards; the relationship list remains.
  if (style.gridTemplateColumns.split(' ').length < 2) return []
  const origin = map.getBoundingClientRect()
  const box = (element: Element): Box => {
    const rect = element.getBoundingClientRect()
    return {
      left: rect.left - origin.left,
      right: rect.right - origin.left,
      top: rect.top - origin.top,
      bottom: rect.bottom - origin.top,
    }
  }
  const gap = { column: parseFloat(style.columnGap), row: parseFloat(style.rowGap) }
  const used = new Map<string, number>()
  const slot = (key: string) => {
    const count = used.get(key) ?? 0
    used.set(key, count + 1)
    return (count % 2 ? -1 : 1) * Math.ceil(count / 2) * 7
  }
  const endpoint = (id: string) => {
    const row = map.querySelector(`[data-deployment-id="${CSS.escape(id)}"]`)
    const card = row?.closest('.map-cluster')
    if (!row || !card) return null
    const rowBox = box(row)
    return { card: box(card), y: (rowBox.top + rowBox.bottom) / 2 }
  }
  const edges: DrawnEdge[] = []
  for (const relationship of relationships) {
    const source = endpoint(relationship.sourceId)
    const target = endpoint(relationship.targetId)
    if (!source || !target) continue
    const plain = routeEdge(source, target, gap)
    const [, outPoint] = plain.points
    const intoPoint = plain.points.at(-2)
    const lanePoint = plain.points.length > 4 ? plain.points[2] : undefined
    const route = routeEdge(source, target, gap, {
      out: slot(`x:${String(Math.round(outPoint?.[0] ?? 0))}`),
      into: plain.points.length > 4 ? slot(`x:${String(Math.round(intoPoint?.[0] ?? 0))}`) : 0,
      lane: lanePoint ? slot(`y:${String(Math.round(lanePoint[1]))}`) : 0,
    })
    const [from, to] = route.labelSegment
    const text = edgeLabels[relationship.kind]
    const width = text.length * 5.6 + 14
    // A label needs a straight run longer than itself; short links rely on line style.
    const run = Math.hypot(to[0] - from[0], to[1] - from[1])
    edges.push({
      relationship,
      path: roundedPath(route.points),
      ports: route.ports,
      label:
        run >= width + 40 ? { x: (from[0] + to[0]) / 2, y: (from[1] + to[1]) / 2, width } : null,
    })
  }
  return edges
}

type HotHandler = (relationshipId: string | null) => void

function MapEdges({
  edges,
  size,
  hot,
  onHot,
}: {
  edges: readonly DrawnEdge[]
  size: { width: number; height: number }
  hot: string | null
  onHot: HotHandler
}) {
  const navigate = useNavigate()
  return (
    <svg
      className="map-edges"
      aria-hidden="true"
      viewBox={`0 0 ${String(size.width)} ${String(size.height)}`}
    >
      <defs>
        <marker
          id="map-arrow"
          viewBox="0 0 8 8"
          refX="7"
          refY="4"
          markerWidth="7"
          markerHeight="7"
          orient="auto-start-reverse"
        >
          <path d="M0 0 L8 4 L0 8 z" />
        </marker>
      </defs>
      {edges.map(({ relationship, path, label }) => (
        <g
          key={relationship.id}
          className={`map-edge map-edge-${relationship.kind}${hot === relationship.id ? ' hot' : ''}`}
          onMouseEnter={() => {
            onHot(relationship.id)
          }}
          onMouseLeave={() => {
            onHot(null)
          }}
          onClick={() => {
            void navigate({
              to: '/topology/relationships/$relationshipId',
              params: { relationshipId: relationship.id },
            })
          }}
        >
          <path className="map-edge-line" d={path} markerEnd="url(#map-arrow)" />
          <path className="map-edge-hit" d={path} />
          {label && (
            <g transform={`translate(${String(label.x)} ${String(label.y)})`}>
              <rect x={-label.width / 2} y={-9} width={label.width} height={18} rx={9} />
              <text textAnchor="middle" y={3.5}>
                {edgeLabels[relationship.kind]}
              </text>
            </g>
          )}
        </g>
      ))}
      {edges.flatMap(({ relationship, ports }) =>
        ports.map(([x, y]) => (
          <circle
            key={`${relationship.id}-${String(x)}-${String(y)}`}
            className="map-port"
            cx={x}
            cy={y}
            r={3.5}
          />
        )),
      )}
    </svg>
  )
}

// RelationshipList names every relationship as a link. It is the keyboard and narrow-screen
// route to the relationship views; the drawn lines are decoration.
function RelationshipList({
  snapshot,
  hot,
  onHot,
}: {
  snapshot: Snapshot
  hot: string | null
  onHot: HotHandler
}) {
  const find = (id: string) => snapshot.deployments.find((item) => item.id === id)
  const clusterName = (deployment: Deployment | undefined) =>
    snapshot.connections.find((item) => item.id === deployment?.connectionId)?.name ??
    deployment?.connectionId
  return (
    <section className="map-relationships" aria-labelledby="map-relationships-heading">
      <h2 id="map-relationships-heading">Relationships</h2>
      {snapshot.relationships.length === 0 ? (
        <p className="muted">No relationships are declared within your access scope.</p>
      ) : (
        <ul>
          {snapshot.relationships.map((relationship) => {
            const source = find(relationship.sourceId)
            const target = find(relationship.targetId)
            const enter = () => {
              onHot(relationship.id)
            }
            const leave = () => {
              onHot(null)
            }
            return (
              <li key={relationship.id}>
                <Link
                  to="/topology/relationships/$relationshipId"
                  params={{ relationshipId: relationship.id }}
                  className={hot === relationship.id ? 'hot' : undefined}
                  onMouseEnter={enter}
                  onMouseLeave={leave}
                  onFocus={enter}
                  onBlur={leave}
                >
                  <span>
                    {source?.namespace}/{source?.name} → {target?.namespace}/{target?.name}
                  </span>
                  <small>
                    {relationshipKindLabels[relationship.kind]} · {clusterName(source)}
                    {source?.connectionId !== target?.connectionId &&
                      ` → ${clusterName(target) ?? ''}`}
                  </small>
                </Link>
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}

// FleetMap shows each Kubernetes cluster with its enrolled operators and the OpenBao
// deployments they manage. Declared relationships connect deployments across clusters.
export function FleetMap({ snapshot }: { snapshot: Snapshot }) {
  const mapRef = useRef<HTMLDivElement>(null)
  const [edges, setEdges] = useState<DrawnEdge[]>([])
  const [size, setSize] = useState({ width: 0, height: 0 })
  const [hot, setHot] = useState<string | null>(null)
  const linked = linkedDeployments(snapshot.relationships)
  const hotRelationship = snapshot.relationships.find((item) => item.id === hot)
  const hotDeployments = new Set(
    hotRelationship ? [hotRelationship.sourceId, hotRelationship.targetId] : [],
  )

  useLayoutEffect(() => {
    const map = mapRef.current
    if (!map) return
    const measure = () => {
      setSize({ width: map.clientWidth, height: map.clientHeight })
      setEdges(measureEdges(map, snapshot.relationships))
    }
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(map)
    return () => {
      observer.disconnect()
    }
  }, [snapshot])

  return (
    <>
      <div className="fleet-map" ref={mapRef}>
        <div className="map-grid">
          {snapshot.connections.map((connection) => (
            <ClusterCard
              key={connection.id}
              connection={connection}
              deployments={snapshot.deployments}
              linked={linked}
              hot={hotDeployments}
            />
          ))}
        </div>
        <MapEdges edges={edges} size={size} hot={hot} onHot={setHot} />
      </div>
      <RelationshipList snapshot={snapshot} hot={hot} onHot={setHot} />
    </>
  )
}
