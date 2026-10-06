import { expect, test } from '@playwright/test'
import type { Page } from '@playwright/test'
import { z } from 'zod'
import type { Configuration, CredentialStatus, Snapshot, SetupStatus } from '../src/api/schema'
import { managedConnectionSchema, settingsUpdateSchema, snapshotSchema } from '../src/api/schema'

async function administrator(page: Page) {
  let revision = 1
  let configuration: Configuration = {
    connections: [],
    users: [{ subject: 'local-demo', name: 'Administrator', admin: true, scopes: [] }],
    groups: [],
    relationships: [],
    maxBackupAgeHours: 24,
    startupGraceMinutes: 10,
  }
  let checks = 0
  let optionalUnavailable = true
  let denied = false
  let empty = false
  let renewalState: CredentialStatus['state'] = 'healthy'
  const statusConnection = (): Snapshot['connections'] =>
    configuration.connections.map((connection) => ({
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
  const settings = () => ({
    revision,
    configuration,
    identity: {
      issuer: 'https://identity.example',
      clientId: 'console',
      ca: '',
      groupsClaim: 'groups',
      secretConfigured: true,
    },
    changes: [],
  })
  await page.route('**/api/v1/snapshot', async (route) => {
    const response = await route.fetch()
    const snapshot = snapshotSchema.parse(await response.json())
    await route.fulfill({
      response,
      json: {
        ...snapshot,
        mode: 'live',
        viewer: { ...snapshot.viewer, admin: true },
        connections: statusConnection(),
        deployments: [],
        relationships: [],
        activity: [],
      },
    })
  })
  await page.route('**/api/v1/settings', async (route) => {
    if (route.request().method() === 'PUT') {
      const request = settingsUpdateSchema.parse(route.request().postDataJSON())
      expect(request.revision).toBe(revision)
      configuration = request.configuration
      revision += 1
    }
    await route.fulfill({ status: 200, json: settings() })
  })
  await page.route('**/api/v1/settings/connections/credentials', (route) =>
    route.fulfill({
      status: 200,
      json: {
        credentials: configuration.connections.map((connection): CredentialStatus => ({
          connectionId: connection.id,
          mode: connection.renewal
            ? 'automatic'
            : connection.authentication === 'local'
              ? 'local'
              : 'manual',
          state: connection.disabled ? 'paused' : connection.renewal ? renewalState : 'ready',
          expiresAt: connection.renewal ? '2030-01-01T00:00:00Z' : null,
          expirySource: connection.renewal ? 'tokenRequest' : 'unknown',
          lastAttemptAt: connection.renewal ? new Date().toISOString() : null,
          lastSuccessAt: connection.renewal ? new Date().toISOString() : null,
          nextAttemptAt:
            connection.renewal && renewalState !== 'reconnectRequired'
              ? '2029-12-31T08:00:00Z'
              : null,
        })),
      },
    }),
  )
  await page.route('**/api/v1/settings/connections/rbac', async (route) => {
    const input = z
      .object({ connection: managedConnectionSchema })
      .parse(route.request().postDataJSON())
    expect(input.connection.namespaces.length).toBeGreaterThan(0)
    await route.fulfill({
      status: 200,
      json: {
        rbac: `apiVersion: v1\nkind: ServiceAccount\nmetadata:\n  name: console-reader\n  namespace: console-system\n${input.connection.renewal ? '---\nkind: Role\nrules:\n  - resources: [serviceaccounts/token]\n    verbs: [create]\n    resourceNames: [console-reader]\n' : ''}`,
        serviceAccount: input.connection.renewal?.serviceAccount ?? 'console-reader',
        serviceAccountNamespace: input.connection.renewal?.namespace ?? 'console-system',
      },
    })
  })
  await page.route('**/api/v1/settings/connections/check', async (route) => {
    checks += 1
    const input = z
      .object({ connection: managedConnectionSchema })
      .parse(route.request().postDataJSON())
    const response = await page.request.get('/api/v1/snapshot')
    const fixture = snapshotSchema.parse(await response.json())
    // The page route intentionally filters the workspace, so use the fixture API
    // directly for its typed projection. This is a UI workflow fixture.
    const deployment = fixture.deployments[0]
    if (!deployment) throw new Error('Missing deployment fixture')
    const now = new Date().toISOString()
    await route.fulfill({
      status: 200,
      json: {
        connection: {
          id: input.connection.id,
          name: input.connection.name,
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
              state: optionalUnavailable ? 'unsupported' : 'available',
              checkedAt: now,
            },
          ],
        },
        rbac: '',
        deployments:
          denied || empty ? [] : [{ ...deployment, name: 'onboarding-bao', namespace: 'team-a' }],
        diagnostics: {
          state: denied ? 'forbidden' : 'available',
          message: denied ? 'Reader RBAC is missing.' : 'OpenBaoCluster access verified.',
        },
        credentialExpiry: { expiresAt: null, source: 'unknown' },
      },
    })
  })
  return {
    configuration: () => configuration,
    checks: () => checks,
    renewal: (state: CredentialStatus['state']) => {
      renewalState = state
    },
    deny: () => {
      denied = true
    },
    allow: () => {
      denied = false
      optionalUnavailable = true
    },
    empty: () => {
      empty = true
    },
  }
}

async function toVerify(page: Page, local = true, automatic = false) {
  await page.goto('/settings/clusters/new')
  await page.getByRole('textbox', { name: 'Cluster name', exact: true }).fill('Production EU')
  if (local)
    await page.getByRole('combobox', { name: 'Connection', exact: true }).selectOption('local')
  await page.getByRole('button', { name: 'Continue', exact: true }).click()
  await page.getByRole('textbox', { name: 'Namespace 1', exact: true }).fill('team-a')
  if (automatic)
    await page.getByRole('checkbox', { name: 'Renew ServiceAccount tokens automatically' }).check()
  await page.getByRole('button', { name: 'Continue', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Download console-reader.yaml' })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Apply commands' })).toContainText(
    'CONSOLE_CLUSTER_CONTEXT=',
  )
  if (automatic)
    await expect(
      page.getByRole('region', { name: 'Reader RBAC manifests', includeHidden: true }),
    ).toContainText(/serviceaccounts\/token[\s\S]*resourceNames: \[console-reader\]/)
  await page.getByRole('button', { name: 'Continue', exact: true }).click()
  if (!local) {
    await page
      .getByRole('textbox', { name: /^Reader kubeconfig/ })
      .fill('fixture-reader-kubeconfig')
  }
  await page.getByRole('button', { name: 'Continue', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Verify Production EU' })).toBeVisible()
}

test('automatic renewal is prepared before credential upload and saved with enrollment', async ({
  page,
}) => {
  await page.emulateMedia({ reducedMotion: 'reduce' })
  const state = await administrator(page)
  await toVerify(page, false, true)
  await page.getByRole('button', { name: 'Verify connection', exact: true }).click()
  await expect(
    page.getByText('Automatic renewal replaces this token after the cluster is connected.', {
      exact: false,
    }),
  ).toBeVisible()
  await expect(page.getByText('not renewed automatically', { exact: false })).toHaveCount(0)
  await page.getByRole('button', { name: 'Connect cluster', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Cluster connected' })).toBeVisible()
  expect(state.configuration().connections[0]?.renewal).toEqual({
    namespace: 'console-system',
    serviceAccount: 'console-reader',
  })
  await page.goto('/settings?section=connections')
  await expect(page.getByText('Token renewal active', { exact: true })).toBeVisible()
  await page.screenshot({
    path: '../.local/screenshots/credential-renewal-light.png',
    fullPage: true,
  })
})

test('renewal status refresh preserves an open edit and makes reconnection visible on mobile', async ({
  page,
}) => {
  await page.clock.install()
  const state = await administrator(page)
  await toVerify(page, false, true)
  await page.getByRole('button', { name: 'Verify connection', exact: true }).click()
  await page.getByRole('button', { name: 'Connect cluster', exact: true }).click()
  await page.goto('/settings?section=connections')
  await page.getByRole('button', { name: 'Edit', exact: true }).click()
  await page.getByRole('textbox', { name: 'Name', exact: true }).fill('Production EU renamed')
  state.renewal('reconnectRequired')
  await page.clock.fastForward(16_000)
  await expect(page.getByRole('textbox', { name: 'Name', exact: true })).toHaveValue(
    'Production EU renamed',
  )
  await page.getByRole('button', { name: 'Save connection', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Reconnect', exact: true })).toBeVisible()
  expect(state.configuration().connections[0]?.name).toBe('Production EU renamed')
  await page.setViewportSize({ width: 390, height: 844 })
  await page.emulateMedia({ colorScheme: 'dark', reducedMotion: 'reduce' })
  // Without an explicit choice, the console follows the system color scheme.
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await expect(page.getByText('Reconnect required', { exact: true })).toHaveCSS(
    'color',
    'rgb(255, 146, 156)',
  )
  await page.screenshot({
    path: '../.local/screenshots/credential-reconnect-mobile-dark.png',
    fullPage: true,
  })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.getByRole('button', { name: 'Reconnect', exact: true }).click()
  await expect(page.getByRole('textbox', { name: 'Bearer token', exact: true })).toBeVisible()
})

test('first use offers cluster onboarding instead of an empty healthy fleet', async ({ page }) => {
  await administrator(page)
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Connect your first cluster' })).toBeVisible()
  await page.getByRole('link', { name: 'Connect cluster', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Add cluster', exact: true })).toBeVisible()
})

test('reader preparation precedes credentials and optional APIs do not block preview', async ({
  page,
}) => {
  const state = await administrator(page)
  await toVerify(page)
  expect(state.checks()).toBe(0)
  await expect(page.getByRole('button', { name: 'Connect cluster', exact: true })).toBeDisabled()
  await page.getByRole('button', { name: 'Verify connection', exact: true }).click()
  await expect(page.getByText('Reader access verified', { exact: true })).toBeVisible()
  await expect(page.getByText('onboarding-bao', { exact: true })).toBeVisible()
  await expect(
    page.getByText('Some optional APIs are unavailable or denied.', { exact: false }),
  ).toBeVisible()
  await page.screenshot({
    path: '../.local/screenshots/onboarding-verify-light.png',
    fullPage: true,
  })
  await page.getByRole('button', { name: 'Connect cluster', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Cluster connected' })).toBeVisible()
  await expect(page.getByText('Waiting for first collection…')).toBeVisible()
  expect(state.configuration().connections[0]?.disabled).toBe(false)
})

test('denied reader can retry and an empty successful preview can connect', async ({ page }) => {
  const state = await administrator(page)
  state.deny()
  await toVerify(page)
  await page.getByRole('button', { name: 'Verify connection', exact: true }).click()
  await expect(page.getByText('Reader RBAC is missing.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Connect cluster', exact: true })).toBeDisabled()
  state.allow()
  state.empty()
  await page.getByRole('button', { name: 'Check again', exact: true }).click()
  await expect(
    page.getByText('No OpenBao deployments found in the enrolled namespaces.', { exact: false }),
  ).toBeVisible()
  await expect(page.getByRole('button', { name: 'Connect cluster', exact: true })).toBeEnabled()
})

test('scope changes invalidate a successful check and mobile onboarding fits', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.emulateMedia({ colorScheme: 'dark', reducedMotion: 'reduce' })
  await administrator(page)
  await toVerify(page)
  await page.getByRole('button', { name: 'Verify connection', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Connect cluster', exact: true })).toBeEnabled()
  await page.screenshot({
    path: '../.local/screenshots/onboarding-mobile-dark.png',
    fullPage: true,
  })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  await page.getByRole('button', { name: 'Back', exact: true }).click()
  await page.getByRole('button', { name: 'Back', exact: true }).click()
  await page.getByRole('button', { name: 'Back', exact: true }).click()
  await page.getByRole('textbox', { name: 'Namespace 1', exact: true }).fill('team-b')
  await page.getByRole('button', { name: 'Continue', exact: true }).click()
  await page.getByRole('button', { name: 'Continue', exact: true }).click()
  await page.getByRole('button', { name: 'Continue', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Connect cluster', exact: true })).toBeDisabled()
  await expect(page.getByText('Reader access verified', { exact: true })).not.toBeVisible()
})

test('setup is a protected standalone flow with retry and no browser token storage', async ({
  page,
}) => {
  let status: SetupStatus = {
    state: 'required',
    callbackUrl: 'https://console.example/auth/callback',
    sessionExpiresAt: null,
  }
  const token = '0123456789abcdef'.repeat(4)
  await page.route('**/api/v1/setup', (route) => route.fulfill({ status: 200, json: status }))
  await page.route('**/api/v1/setup/unlock', async (route) => {
    const request = z.object({ token: z.string() }).parse(route.request().postDataJSON())
    if (request.token !== token) {
      await route.fulfill({
        status: 401,
        json: { code: 'invalid_token', message: 'The setup token was not accepted.' },
      })
      return
    }
    status = {
      ...status,
      state: 'unlocked',
      sessionExpiresAt: new Date(Date.now() + 900_000).toISOString(),
    }
    await route.fulfill({ status: 200, json: status })
  })
  await page.route('**/api/v1/setup/identity', (route) =>
    route.fulfill({
      status: 400,
      json: { code: 'invalid', message: 'Identity discovery failed. Check the issuer.' },
    }),
  )
  await page.route('**/api/v1/setup/lock', (route) => {
    status = { ...status, state: 'required', sessionExpiresAt: null }
    return route.fulfill({ status: 200, json: status })
  })
  await page.goto('/')
  await expect(page).toHaveURL(/\/setup/)
  await expect(page.getByRole('navigation', { name: 'Main navigation' })).not.toBeVisible()
  await page.getByLabel('Setup token', { exact: true }).fill('incorrect')
  await page.getByRole('button', { name: 'Unlock setup', exact: true }).click()
  await expect(page.getByRole('alert')).toHaveText('The setup token was not accepted.')
  await expect(page.getByLabel('Setup token', { exact: true })).toBeEmpty()
  await page.getByLabel('Setup token', { exact: true }).fill(token)
  await page.getByRole('button', { name: 'Unlock setup', exact: true }).click()
  await expect(page.getByLabel('Callback URL', { exact: true })).toHaveValue(status.callbackUrl)
  await page
    .getByLabel('Issuer URL', { exact: false })
    .fill('https://identity.example/realms/platform')
  await page.getByLabel('Client ID', { exact: true }).fill('console')
  await page.getByLabel('Client secret', { exact: true }).fill('fixture-client-secret')
  await page.getByRole('button', { name: 'Verify with login', exact: true }).click()
  await expect(page.getByRole('alert')).toHaveText('Identity discovery failed. Check the issuer.')
  await page.screenshot({ path: '../.local/screenshots/setup-identity-light.png', fullPage: true })
  const storage = await page.evaluate(() => ({
    local: Object.values(localStorage),
    session: Object.values(sessionStorage),
    url: location.href,
  }))
  expect(JSON.stringify(storage)).not.toContain(token)
  expect(JSON.stringify(storage)).not.toContain('fixture-client-secret')
  await page.getByRole('button', { name: 'Lock setup', exact: true }).click()
  await expect(page.getByLabel('Setup token', { exact: true })).toBeVisible()
})

test('signed-out console presents sign in without an outage or fake profile', async ({ page }) => {
  await page.route('**/api/v1/snapshot', (route) =>
    route.fulfill({ status: 401, json: { code: 'unauthenticated', message: 'Sign in.' } }),
  )
  await page.goto('/fleet')
  await expect(page.getByRole('heading', { name: 'Sign in', exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Fleet evidence unavailable' })).not.toBeVisible()
  await expect(page.getByText('Platform team', { exact: true })).not.toBeVisible()
  await expect(page.getByRole('link', { name: 'Sign in', exact: true })).toHaveAttribute(
    'href',
    /returnTo=/,
  )
})
