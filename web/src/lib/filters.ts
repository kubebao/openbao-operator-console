import type { Deployment } from '../api/schema'

export interface FleetFilter {
  query: string
  environment: string
  team: string
  cluster: string
  health: string
  collection: string
}

export const emptyFilter: FleetFilter = {
  query: '',
  environment: '',
  team: '',
  cluster: '',
  health: '',
  collection: '',
}

export function filterDeployments(
  deployments: readonly Deployment[],
  filter: FleetFilter,
): Deployment[] {
  const query = filter.query.trim().toLocaleLowerCase()
  return deployments.filter(
    (deployment) =>
      (!query ||
        [deployment.name, deployment.namespace, deployment.connectionId, deployment.team].some(
          (value) => value.toLocaleLowerCase().includes(query),
        )) &&
      (!filter.environment || deployment.environment === filter.environment) &&
      (!filter.team || deployment.team === filter.team) &&
      (!filter.cluster || deployment.connectionId === filter.cluster) &&
      (!filter.health || deployment.health === filter.health) &&
      (!filter.collection ||
        (filter.collection === 'gaps'
          ? deployment.evidence.collection !== 'current'
          : deployment.evidence.collection === filter.collection)),
  )
}
