import { expect } from '@playwright/test'
import type { Page, Route } from '@playwright/test'
import { z } from 'zod'
import type {
  Configuration,
  CredentialStatus,
  ManagedConnection,
  Snapshot,
} from '../../src/api/schema'
import { managedConnectionSchema, settingsUpdateSchema, snapshotSchema } from '../../src/api/schema'

// This fixture fakes the administrator API for onboarding and renewal UI workflows. It
// does not exercise the Go server's validation or collection.

interface BackendState {
  revision: number
  configuration: Configuration
  checks: number
  optionalUnavailable: boolean
  denied: boolean
  empty: boolean
  renewalState: CredentialStatus['state']
}

const connectionRequestSchema = z.object({ connection: managedConnectionSchema })

const identity = {
  issuer: 'https://identity.example',
  clientId: 'console',
  ca: '',
  groupsClaim: 'groups',
  secretConfigured: true,
}

function initialState(): BackendState {
  return {
    revision: 1,
    configuration: {
      connections: [],
      users: [{ subject: 'local-demo', name: 'Administrator', admin: true, scopes: [] }],
      groups: [],
      relationships: [],
      maxBackupAgeHours: 24,
      startupGraceMinutes: 10,
    },
    checks: 0,
    optionalUnavailable: true,
    denied: false,
    empty: false,
    renewalState: 'healthy',
  }
}

function connectionStatuses(state: BackendState): Snapshot['connections'] {
  return state.configuration.connections.map((connection) => ({
    id: connection.id,
    name: connection.name,
    region: connection.region,
    collection: connection.disabled ? 'stale' : 'pending',
    lastAttemptAt: new Date().toISOString(),
    lastSuccessAt: null,
    namespaces: connection.namespaces.map((namespace) => namespace.name),
    capabilities: [],
    operators: [],
  }))
}

function credentialMode(connection: ManagedConnection): CredentialStatus['mode'] {
  if (connection.renewal) return 'automatic'
  return connection.authentication === 'local' ? 'local' : 'manual'
}

function credentialStatus(state: BackendState, connection: ManagedConnection): CredentialStatus {
  const renewed = Boolean(connection.renewal)
  const now = new Date().toISOString()
  let current: CredentialStatus['state'] = 'ready'
  if (connection.disabled) current = 'paused'
  else if (renewed) current = state.renewalState
  return {
    connectionId: connection.id,
    mode: credentialMode(connection),
    state: current,
    expiresAt: renewed ? '2030-01-01T00:00:00Z' : null,
    expirySource: renewed ? 'tokenRequest' : 'unknown',
    lastAttemptAt: renewed ? now : null,
    lastSuccessAt: renewed ? now : null,
    nextAttemptAt:
      renewed && state.renewalState !== 'reconnectRequired' ? '2029-12-31T08:00:00Z' : null,
  }
}

function readerManifests(connection: ManagedConnection): string {
  const serviceAccount =
    'apiVersion: v1\nkind: ServiceAccount\nmetadata:\n  name: console-reader\n  namespace: console-system\n'
  if (!connection.renewal) return serviceAccount
  return `${serviceAccount}---\nkind: Role\nrules:\n  - resources: [serviceaccounts/token]\n    verbs: [create]\n    resourceNames: [console-reader]\n`
}

async function fulfillSnapshot(route: Route, state: BackendState) {
  const response = await route.fetch()
  const snapshot = snapshotSchema.parse(await response.json())
  await route.fulfill({
    response,
    json: {
      ...snapshot,
      mode: 'live',
      viewer: { ...snapshot.viewer, admin: true },
      connections: connectionStatuses(state),
      deployments: [],
      relationships: [],
    },
  })
}

async function fulfillSettings(route: Route, state: BackendState) {
  if (route.request().method() === 'PUT') {
    const request = settingsUpdateSchema.parse(route.request().postDataJSON())
    expect(request.revision).toBe(state.revision)
    state.configuration = request.configuration
    state.revision += 1
  }
  await route.fulfill({
    status: 200,
    json: {
      revision: state.revision,
      configuration: state.configuration,
      identity,
      changes: [],
    },
  })
}

async function fulfillReaderAccess(route: Route) {
  const { connection } = connectionRequestSchema.parse(route.request().postDataJSON())
  expect(connection.namespaces.length).toBeGreaterThan(0)
  await route.fulfill({
    status: 200,
    json: {
      rbac: readerManifests(connection),
      serviceAccount: connection.renewal?.serviceAccount ?? 'console-reader',
      serviceAccountNamespace: connection.renewal?.namespace ?? 'console-system',
    },
  })
}

async function fixtureDeployment(page: Page) {
  // The page route intentionally filters the workspace, so use the fixture API directly
  // for its typed projection.
  const response = await page.request.get('/api/v1/snapshot')
  const deployment = snapshotSchema.parse(await response.json()).deployments[0]
  if (!deployment) throw new Error('Missing deployment fixture')
  return deployment
}

async function fulfillCheck(route: Route, page: Page, state: BackendState) {
  state.checks += 1
  const { connection } = connectionRequestSchema.parse(route.request().postDataJSON())
  const deployment = await fixtureDeployment(page)
  const { denied } = state
  const now = new Date().toISOString()
  await route.fulfill({
    status: 200,
    json: {
      connection: {
        id: connection.id,
        name: connection.name,
        region: '',
        collection: denied ? 'denied' : 'current',
        lastAttemptAt: now,
        lastSuccessAt: denied ? null : now,
        namespaces: ['team-a'],
        operators: [],
        capabilities: [
          {
            namespace: 'team-a',
            resource: 'openbaoclusters',
            state: denied ? 'denied' : 'available',
            checkedAt: now,
          },
          {
            namespace: 'team-a',
            resource: 'openbaorecoveries',
            state: state.optionalUnavailable ? 'unsupported' : 'available',
            checkedAt: now,
          },
        ],
      },
      rbac: '',
      deployments:
        denied || state.empty
          ? []
          : [{ ...deployment, name: 'onboarding-bao', namespace: 'team-a' }],
      diagnostics: {
        state: denied ? 'forbidden' : 'available',
        message: denied ? 'Reader RBAC is missing.' : 'OpenBaoCluster access verified.',
      },
      credentialExpiry: { expiresAt: null, source: 'unknown' },
    },
  })
}

// administrator routes the settings and onboarding API to an in-memory fixture backend
// and returns controls for the test.
export async function administrator(page: Page) {
  const state = initialState()
  await page.route('**/api/v1/snapshot', (route) => fulfillSnapshot(route, state))
  await page.route('**/api/v1/settings', (route) => fulfillSettings(route, state))
  await page.route('**/api/v1/settings/connections/credentials', (route) =>
    route.fulfill({
      status: 200,
      json: {
        credentials: state.configuration.connections.map((connection) =>
          credentialStatus(state, connection),
        ),
      },
    }),
  )
  await page.route('**/api/v1/settings/connections/rbac', fulfillReaderAccess)
  await page.route('**/api/v1/settings/connections/check', (route) =>
    fulfillCheck(route, page, state),
  )
  return {
    configuration: () => state.configuration,
    checks: () => state.checks,
    renewal: (renewalState: CredentialStatus['state']) => {
      state.renewalState = renewalState
    },
    deny: () => {
      state.denied = true
    },
    allow: () => {
      state.denied = false
      state.optionalUnavailable = true
    },
    empty: () => {
      state.empty = true
    },
  }
}
