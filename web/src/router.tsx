import { createRootRoute, createRoute, createRouter } from '@tanstack/react-router'
import type { SearchSchemaInput } from '@tanstack/react-router'
import type { FleetFilter } from './lib/filters'
import { collectionFilters, healthFilters } from './lib/filters'
import { environments } from './lib/connection'
import { deploymentTabs } from './lib/findings'
import { z } from 'zod'
import { Shell } from './components/shell'
import { AttentionPage } from './pages/attention'
import { FleetPage } from './pages/fleet'
import { DeploymentPage } from './pages/deployment'
import { ActivityPage } from './pages/activity'
import { SettingsPage } from './pages/settings'
import { TopologyPage } from './pages/topology'
import { TopologyClusterPage } from './pages/topology-cluster'
import { TopologyRelationshipPage } from './pages/topology-relationship'
import { SetupPage } from './pages/setup'
import { EnrollPage } from './pages/enroll'
import { AccountPage } from './pages/account'
import { ClusterOnboardingPage } from './pages/cluster-onboarding'
import { RouteError, RouteNotFound } from './components/route-states'

const root = createRootRoute({
  component: Shell,
  errorComponent: RouteError,
  notFoundComponent: RouteNotFound,
})
const attention = createRoute({ getParentRoute: () => root, path: '/', component: AttentionPage })
const setup = createRoute({
  getParentRoute: () => root,
  path: '/setup',
  validateSearch: (search: SearchSchemaInput & { result?: string }) =>
    z.object({ result: z.enum(['', 'failed', 'expired']).catch('') }).parse(search),
  component: SetupPage,
})
const enroll = createRoute({ getParentRoute: () => root, path: '/enroll', component: EnrollPage })
const account = createRoute({
  getParentRoute: () => root,
  path: '/account',
  component: AccountPage,
})
const clusterOnboarding = createRoute({
  getParentRoute: () => root,
  path: '/settings/clusters/new',
  component: ClusterOnboardingPage,
})
// An unknown enumerated filter value is dropped instead of filtering silently. Teams and
// clusters depend on the snapshot; the filter bar shows an unobserved value as selected.
const knownOrEmpty = <T extends readonly [string, ...string[]]>(values: T) =>
  z.union([z.literal(''), z.enum(values)]).catch('')
const filterSchema = z.object({
  query: z.string().catch(''),
  environment: knownOrEmpty(environments),
  team: z.string().catch(''),
  cluster: z.string().catch(''),
  health: knownOrEmpty(healthFilters),
  collection: knownOrEmpty(collectionFilters),
})
const fleet = createRoute({
  getParentRoute: () => root,
  path: '/fleet',
  validateSearch: (search: SearchSchemaInput & Partial<FleetFilter>) => filterSchema.parse(search),
  component: FleetPage,
})
const deployment = createRoute({
  getParentRoute: () => root,
  path: '/deployments/$deploymentId',
  validateSearch: (search: SearchSchemaInput & { tab?: string }) =>
    z
      .object({
        tab: z.enum(deploymentTabs).catch('overview'),
      })
      .parse(search),
  component: DeploymentPage,
})
const activity = createRoute({
  getParentRoute: () => root,
  path: '/activity',
  component: ActivityPage,
})
const settings = createRoute({
  getParentRoute: () => root,
  path: '/settings',
  validateSearch: (search: SearchSchemaInput & { section?: string }) =>
    z
      .object({
        section: z
          .enum([
            'connections',
            'access',
            'local',
            'relationships',
            'policies',
            'identity',
            'changes',
          ])
          .catch('connections'),
      })
      .parse(search),
  component: SettingsPage,
})
const topology = createRoute({
  getParentRoute: () => root,
  path: '/topology',
  component: TopologyPage,
})
const topologyCluster = createRoute({
  getParentRoute: () => root,
  path: '/topology/clusters/$clusterId',
  component: TopologyClusterPage,
})
const topologyRelationship = createRoute({
  getParentRoute: () => root,
  path: '/topology/relationships/$relationshipId',
  component: TopologyRelationshipPage,
})

export const router = createRouter({
  routeTree: root.addChildren([
    attention,
    setup,
    enroll,
    account,
    clusterOnboarding,
    fleet,
    deployment,
    activity,
    settings,
    topology,
    topologyCluster,
    topologyRelationship,
  ]),
  defaultPreload: 'intent',
  scrollRestoration: true,
})
declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}
