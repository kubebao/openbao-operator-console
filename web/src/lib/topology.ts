import type { Deployment, Operator, Relationship, Snapshot } from '../api/schema'
import { healthRank } from './format'

type Connection = Snapshot['connections'][number]
type Capability = Connection['capabilities'][number]

// An operator group holds the deployments one enrolled operator is responsible for. A null
// operator holds deployments in namespaces that no enrolled operator declares.
export interface OperatorGroup {
  operator: Operator | null
  deployments: Deployment[]
}

// mapLimit is the number of deployments one operator shows on the map before the rest
// collapse into a health tally.
export const mapLimit = 8

const byTriage = (a: Deployment, b: Deployment) =>
  healthRank[a.health] - healthRank[b.health] || a.name.localeCompare(b.name)

// operatorGroups assigns each deployment of a cluster to the operator that enrollment declares
// responsible for it. Responsibility is never inferred: a deployment whose operator is not
// enrolled on the same cluster belongs to the unmanaged group.
export function operatorGroups(connection: Connection, deployments: readonly Deployment[]) {
  const groups: OperatorGroup[] = connection.operators.map((operator) => ({
    operator,
    deployments: [],
  }))
  const unmanaged: OperatorGroup = { operator: null, deployments: [] }
  for (const deployment of deployments) {
    if (deployment.connectionId !== connection.id) continue
    const group = groups.find((item) => item.operator?.id === deployment.operatorId)
    ;(group ?? unmanaged).deployments.push(deployment)
  }
  if (unmanaged.deployments.length > 0) groups.push(unmanaged)
  for (const group of groups) group.deployments.sort(byTriage)
  return groups
}

// linkedDeployments lists every deployment that is an endpoint of a relationship.
export function linkedDeployments(relationships: readonly Relationship[]): ReadonlySet<string> {
  return new Set(relationships.flatMap((item) => [item.sourceId, item.targetId]))
}

// visibleOnMap keeps an operator's deployments visible up to the limit. Above it, only
// deployments in a relationship stay, so every line keeps its endpoints.
export function visibleOnMap(
  deployments: readonly Deployment[],
  linked: ReadonlySet<string>,
  limit = mapLimit,
): { visible: Deployment[]; hidden: Deployment[] } {
  if (deployments.length <= limit) return { visible: [...deployments], hidden: [] }
  return {
    visible: deployments.filter((item) => linked.has(item.id)),
    hidden: deployments.filter((item) => !linked.has(item.id)),
  }
}

export interface CapabilityGroup {
  resources: string[]
  state: Capability['state']
  namespaces: string[]
}

// groupCapabilities writes one row per access result and namespace set, listing the resource
// APIs that share it. A cluster-wide failure becomes one row instead of one per namespace and API.
export function groupCapabilities(capabilities: readonly Capability[]): CapabilityGroup[] {
  const perResource = new Map<
    string,
    { resource: string; state: Capability['state']; namespaces: string[] }
  >()
  for (const capability of capabilities) {
    const key = `${capability.resource}/${capability.state}`
    const entry = perResource.get(key)
    if (entry) entry.namespaces.push(capability.namespace)
    else
      perResource.set(key, {
        resource: capability.resource,
        state: capability.state,
        namespaces: [capability.namespace],
      })
  }
  const groups = new Map<string, CapabilityGroup>()
  for (const entry of perResource.values()) {
    const key = `${entry.state}/${entry.namespaces.join(',')}`
    const group = groups.get(key)
    if (group) group.resources.push(entry.resource)
    else
      groups.set(key, {
        resources: [entry.resource],
        state: entry.state,
        namespaces: entry.namespaces,
      })
  }
  return [...groups.values()]
}

export interface Box {
  left: number
  right: number
  top: number
  bottom: number
}

export type Point = readonly [number, number]

export interface Route {
  points: Point[]
  // The straight segment that can carry a label.
  labelSegment: readonly [Point, Point]
  ports: readonly [Point, Point]
}

// routeEdge connects two deployment rows through the gaps between their cluster cards. The
// line leaves the source card's side, runs along the row gap above a card row when the cards
// are not neighbours, and enters the target card's side. It never crosses a card. Offsets
// separate lines that share a gap.
export function routeEdge(
  source: { card: Box; y: number },
  target: { card: Box; y: number },
  gap: { column: number; row: number },
  offset: { out: number; into: number; lane: number } = { out: 0, into: 0, lane: 0 },
): Route {
  const leftward = target.card.left < source.card.left - 1
  const exitX = leftward ? source.card.left : source.card.right
  const rightward = target.card.left > source.card.left + 1
  const enterX = rightward ? target.card.left : target.card.right
  const out = exitX + ((leftward ? -1 : 1) * gap.column) / 2 + offset.out
  const into = enterX + ((rightward ? -1 : 1) * gap.column) / 2 + offset.into
  const sameRow = Math.abs(source.card.top - target.card.top) < 1
  const start: Point = [exitX, source.y]
  const end: Point = [enterX, target.y]
  if (sameRow && Math.abs(out - offset.out - (into - offset.into)) < 1) {
    const points: Point[] = [start, [out, source.y], [out, target.y], end]
    return { points, labelSegment: [points[1] ?? start, points[2] ?? end], ports: [start, end] }
  }
  const laneTop = target.card.top > source.card.top + 1 ? target.card.top : source.card.top
  const lane = laneTop - gap.row / 2 + offset.lane
  const points: Point[] = [start, [out, source.y], [out, lane], [into, lane], [into, target.y], end]
  return { points, labelSegment: [points[2] ?? start, points[3] ?? end], ports: [start, end] }
}

// roundedPath draws an orthogonal polyline as an SVG path with rounded corners.
export function roundedPath(points: readonly Point[], radius = 8): string {
  const [first, ...rest] = points
  if (!first) return ''
  let path = `M ${String(first[0])} ${String(first[1])}`
  for (let index = 0; index < rest.length; index += 1) {
    const corner = rest[index]
    const previous = index === 0 ? first : rest[index - 1]
    const next = rest[index + 1]
    if (!corner || !previous) continue
    if (!next) {
      path += ` L ${String(corner[0])} ${String(corner[1])}`
      continue
    }
    const r = Math.min(
      radius,
      Math.hypot(corner[0] - previous[0], corner[1] - previous[1]) / 2,
      Math.hypot(next[0] - corner[0], next[1] - corner[1]) / 2,
    )
    const ax = corner[0] - Math.sign(corner[0] - previous[0]) * r
    const ay = corner[1] - Math.sign(corner[1] - previous[1]) * r
    const bx = corner[0] + Math.sign(next[0] - corner[0]) * r
    const by = corner[1] + Math.sign(next[1] - corner[1]) * r
    path += ` L ${String(ax)} ${String(ay)} Q ${String(corner[0])} ${String(corner[1])} ${String(bx)} ${String(by)}`
  }
  return path
}
