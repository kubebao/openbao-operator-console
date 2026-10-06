import { createRootRoute, createRoute, createRouter } from '@tanstack/react-router'
import type { SearchSchemaInput } from '@tanstack/react-router'
import type { FleetFilter } from './lib/filters'
import { z } from 'zod'
import { AlertCircle, SearchX } from 'lucide-react'
import { Shell } from './components/shell'
import { AttentionPage } from './pages/attention'
import { FleetPage } from './pages/fleet'
import { DeploymentPage } from './pages/deployment'
import { ActivityPage } from './pages/activity'
import { ClustersPage } from './pages/clusters'
import { SettingsPage } from './pages/settings'
import { TopologyPage } from './pages/topology'
import { RecoveryPage } from './pages/recovery'
import { SetupPage } from './pages/setup'
import { ClusterOnboardingPage } from './pages/cluster-onboarding'
import { EmptyState } from './components/page'

const root = createRootRoute({
  component: Shell,
  errorComponent: () => (
    <EmptyState title="This view could not load" icon={AlertCircle}>
      Refresh the page to retry. Other fleet views remain available from the navigation.
    </EmptyState>
  ),
  notFoundComponent: () => (
    <EmptyState title="Page not found" icon={SearchX}>
      Use the workspace navigation to return to your fleet.
    </EmptyState>
  ),
})
const attention = createRoute({ getParentRoute: () => root, path: '/', component: AttentionPage })
const setup = createRoute({
  getParentRoute: () => root,
  path: '/setup',
  validateSearch: (search: SearchSchemaInput & { result?: string }) =>
    z.object({ result: z.enum(['', 'failed', 'expired']).catch('') }).parse(search),
  component: SetupPage,
})
const clusterOnboarding = createRoute({
  getParentRoute: () => root,
  path: '/settings/clusters/new',
  component: ClusterOnboardingPage,
})
const filterSchema = z.object({
  query: z.string().catch(''),
  environment: z.string().catch(''),
  team: z.string().catch(''),
  cluster: z.string().catch(''),
  health: z.string().catch(''),
  collection: z.string().catch(''),
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
        tab: z
          .enum(['overview', 'conditions', 'operations', 'backup', 'history'])
          .catch('overview'),
      })
      .parse(search),
  component: DeploymentPage,
})
const activity = createRoute({
  getParentRoute: () => root,
  path: '/activity',
  component: ActivityPage,
})
const clusters = createRoute({
  getParentRoute: () => root,
  path: '/clusters',
  component: ClustersPage,
})
const settings = createRoute({
  getParentRoute: () => root,
  path: '/settings',
  validateSearch: (search: SearchSchemaInput & { section?: string }) =>
    z
      .object({
        section: z
          .enum(['connections', 'access', 'relationships', 'policies', 'identity', 'changes'])
          .catch('connections'),
      })
      .parse(search),
  component: SettingsPage,
})
const topology = createRoute({
  getParentRoute: () => root,
  path: '/topology',
  validateSearch: (
    search: SearchSchemaInput & { relationship?: string; deployment?: string; kind?: string },
  ) =>
    z
      .object({
        relationship: z.string().catch(''),
        deployment: z.string().catch(''),
        kind: z.enum(['all', 'replication', 'snapshotRecovery']).catch('all'),
      })
      .parse(search),
  component: TopologyPage,
})
const recovery = createRoute({
  getParentRoute: () => root,
  path: '/recovery/$relationshipId',
  component: RecoveryPage,
})

export const router = createRouter({
  routeTree: root.addChildren([
    attention,
    setup,
    clusterOnboarding,
    fleet,
    deployment,
    activity,
    clusters,
    settings,
    topology,
    recovery,
  ]),
  defaultPreload: 'intent',
  scrollRestoration: true,
})
declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}
