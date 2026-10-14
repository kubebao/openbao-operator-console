import type { Deployment, Snapshot } from '../api/schema'

export interface FleetFilter {
  query: string
  environment: string
  team: string
  cluster: string
  health: string
  collection: string
}

const filterKeys: readonly (keyof FleetFilter)[] = [
  'query',
  'environment',
  'team',
  'cluster',
  'health',
  'collection',
]

// collectionGaps selects every deployment without current evidence.
export const collectionGaps = 'gaps'

// Values the fleet filters accept from a URL. Teams and clusters depend on the snapshot.
export const healthFilters = ['critical', 'attention', 'unknown', 'healthy'] as const
export const collectionFilters = [
  collectionGaps,
  'current',
  'stale',
  'unreachable',
  'denied',
  'unsupported',
] as const

// sameFilter reports whether a filter equals a view with unset fields left empty.
export function sameFilter(filter: FleetFilter, view: Partial<FleetFilter>): boolean {
  const target: FleetFilter = { ...emptyFilter, ...view }
  return filterKeys.every((key) => filter[key] === target[key])
}

export const emptyFilter: FleetFilter = {
  query: '',
  environment: '',
  team: '',
  cluster: '',
  health: '',
  collection: '',
}

// ClusterNames maps connection identifiers to their display names.
export type ClusterNames = ReadonlyMap<string, string>

export function clusterNames(snapshot: Pick<Snapshot, 'connections'> | undefined): ClusterNames {
  return new Map(snapshot?.connections.map((connection) => [connection.id, connection.name]))
}

// matchesQuery searches a deployment's name, namespace, team, and its cluster's name and
// identifier.
export function matchesQuery(deployment: Deployment, query: string, clusters: ClusterNames) {
  const term = query.trim().toLocaleLowerCase()
  if (!term) return true
  return [
    deployment.name,
    deployment.namespace,
    deployment.team,
    deployment.connectionId,
    clusters.get(deployment.connectionId) ?? '',
  ].some((value) => value.toLocaleLowerCase().includes(term))
}

export function filterDeployments(
  deployments: readonly Deployment[],
  filter: FleetFilter,
  clusters: ClusterNames,
): Deployment[] {
  return deployments.filter(
    (deployment) =>
      matchesQuery(deployment, filter.query, clusters) &&
      (!filter.environment || deployment.environment === filter.environment) &&
      (!filter.team || deployment.team === filter.team) &&
      (!filter.cluster || deployment.connectionId === filter.cluster) &&
      (!filter.health || deployment.health === filter.health) &&
      (!filter.collection ||
        (filter.collection === collectionGaps
          ? deployment.evidence.collection !== 'current'
          : deployment.evidence.collection === filter.collection)),
  )
}
