import type { Configuration } from '../src/api/schema'
import { expect, test } from '@playwright/test'
import { snapshotSchema, settingsUpdateSchema } from '../src/api/schema'
import { problem } from './fixtures/problem'

test('attention drills into deployment evidence and history', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Attention', exact: true })).toBeVisible()
  await expect(page.getByText('Illustrative evidence · no live clusters connected')).toBeVisible()
  await page.getByRole('link', { name: 'OpenBao is sealed', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'identity-bao', exact: true })).toBeVisible()
  await page.getByRole('tab', { name: /^Conditions/ }).click()
  await expect(page.getByRole('tab', { name: 'Conditions Has critical finding' })).toBeVisible()
  await expect(page.getByText('OpenBaoSealed', { exact: true })).toBeVisible()
  await page.reload()
  await expect(page.getByRole('tab', { name: /^Conditions/ })).toHaveAttribute(
    'data-state',
    'active',
  )
  await page.getByRole('tab', { name: 'History', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Recorded changes' })).toBeVisible()
})

test('fleet filters are shareable and sorting works', async ({ page }) => {
  await page.goto('/fleet')
  await page.getByRole('textbox', { name: 'Search deployments' }).fill('payments')
  await expect(page.getByText('Showing 3 of 12 deployments')).toBeVisible()
  await expect(page).toHaveURL(/query=payments/)
  await page.reload()
  await expect(page.getByRole('textbox', { name: 'Search deployments' })).toHaveValue('payments')
  await page.getByRole('button', { name: 'Clear filters' }).click()
  const rows = page.locator('.fleet-table tbody tr')
  const deploymentHeader = page.getByRole('columnheader', { name: 'Deployment' })
  await deploymentHeader.getByRole('button').click()
  await expect(deploymentHeader).toHaveAttribute('aria-sort', 'ascending')
  await expect(rows.first()).toContainText('customer-x-bao')
  await expect(rows.last()).toContainText('staging-bao')
  await deploymentHeader.getByRole('button').click()
  await expect(deploymentHeader).toHaveAttribute('aria-sort', 'descending')
  await expect(rows.first()).toContainText('staging-bao')
  await page.getByRole('textbox', { name: 'Search deployments' }).fill('payments')
  await expect(rows).toHaveCount(3)
  await expect(rows.first()).toContainText('payments-read')
  await expect(deploymentHeader).toHaveAttribute('aria-sort', 'descending')
  await page.getByRole('button', { name: 'Clear filters' }).click()
  const stateHeader = page.getByRole('columnheader', { name: 'State', exact: true })
  await stateHeader.getByRole('button').click()
  await expect(rows.first()).toContainText('identity-bao')
  await stateHeader.getByRole('button').click()
  await expect(stateHeader).toHaveAttribute('aria-sort', 'descending')
  await expect(rows.last()).toContainText('identity-bao')
  await page.getByRole('combobox', { name: 'Filter by state' }).selectOption('critical')
  await expect(page.getByText('Showing 1 of 12 deployments')).toBeVisible()
  await expect(page.getByRole('link', { name: 'identity-bao identity', exact: true })).toBeVisible()
})

test('topology zooms from the map to a cluster and a relationship', async ({ page }) => {
  await page.goto('/topology')
  await expect(page.getByRole('heading', { name: 'Topology', exact: true })).toBeVisible()
  // Each declared relationship is drawn between its deployments on a wide screen.
  await expect(page.locator('.map-edge')).toHaveCount(2)
  await expect(page.getByText('operator-prod-eu-1', { exact: true })).toBeVisible()
  await page.screenshot({ path: '../.local/screenshots/topology-light.png', fullPage: true })
  await page.getByRole('link', { name: /payments\/payments-bao → payments\/payments-read/ }).click()
  await expect(page.getByRole('heading', { name: 'Read replica', exact: true })).toBeVisible()
  await expect(page.getByText('9007199254741007', { exact: true })).toBeVisible()
  const breadcrumb = page.getByRole('navigation', { name: 'Breadcrumb' })
  await breadcrumb.getByRole('link', { name: 'prod-eu-1', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'prod-eu-1', exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'operator-prod-eu-1' })).toBeVisible()
  await page.getByRole('link', { name: 'Recovers to recovery/payments-dr (dr-eu-1)' }).click()
  await expect(page.getByRole('heading', { name: 'Recovery path', exact: true })).toBeVisible()
  await expect(page.getByText('Measured RPO', { exact: true })).toBeVisible()
  await expect(page.getByText('Readability unknown', { exact: true })).toBeVisible()
  await expect(page.getByText('Snapshot repository', { exact: true })).toBeVisible()
  await page
    .getByRole('navigation', { name: 'Breadcrumb' })
    .getByRole('link', { name: 'Topology' })
    .click()
  await page.getByRole('link', { name: /^prod-eu-2/ }).click()
  await expect(page.getByRole('heading', { name: 'prod-eu-2', exact: true })).toBeVisible()
})

test('dark mode persists and global search is keyboard accessible', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'light' })
  await page.goto('/')
  await page.getByRole('button', { name: 'Switch to dark mode' }).click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await page.reload()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  // The shortcut is available once the workspace has loaded.
  await expect(page.getByRole('heading', { name: 'Attention', exact: true })).toBeVisible()
  await page.keyboard.press('ControlOrMeta+k')
  await page.getByRole('combobox', { name: 'Search your fleet' }).fill('finance')
  await expect(page.getByRole('option', { name: /finance-bao/ })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await page.keyboard.press('Enter')
  await expect(page.getByRole('heading', { name: 'finance-bao', exact: true })).toBeVisible()
})

test('keyboard users skip to content and navigation focuses the new page', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Attention', exact: true })).toBeVisible()
  await expect(page).toHaveTitle('Attention · OpenBao Operator Console')
  await page.keyboard.press('Tab')
  await expect(page.getByRole('link', { name: 'Skip to main content' })).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('main')).toBeFocused()
  await page
    .getByRole('navigation', { name: 'Main navigation' })
    .getByRole('link', { name: 'Fleet', exact: true })
    .click()
  await expect(page.getByRole('heading', { name: 'Fleet', level: 1 })).toBeFocused()
  await expect(page).toHaveTitle('Fleet · OpenBao Operator Console')
  // Filters change only the search, so focus stays in the filter bar.
  const state = page.getByRole('combobox', { name: 'Filter by state' })
  await state.focus()
  await state.selectOption('critical')
  await expect(page).toHaveURL(/health=critical/)
  await expect(state).toBeFocused()
  await page.keyboard.press('ControlOrMeta+K')
  await page.getByRole('combobox', { name: 'Search your fleet' }).fill('finance')
  await page.keyboard.press('Enter')
  await expect(page.getByRole('heading', { name: 'finance-bao', level: 1 })).toBeFocused()
  await expect(page).toHaveTitle('finance-bao · OpenBao Operator Console')
})

test('mobile navigation and reduced motion retain working content', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Attention', exact: true })).toBeVisible()
  await page.getByRole('link', { name: 'Fleet', exact: true }).click()
  await expect(page.getByRole('textbox', { name: 'Search deployments' })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  )
})

test('sign-in explains a refused login and keeps the return path', async ({ page }) => {
  await page.route('**/api/v1/snapshot', (route) =>
    route.fulfill(problem(401, 'unauthenticated', 'Sign in to view this fleet.')),
  )
  await page.goto('/fleet?query=payments&signin=denied')
  await expect(page.getByRole('heading', { name: 'OpenBao Operator Console' })).toBeVisible()
  await expect(page.getByRole('alert')).toContainText('no access to this console')
  const login = await page
    .getByRole('link', { name: 'Sign in with single sign-on' })
    .getAttribute('href')
  const returnTo = new URL(login ?? '', page.url()).searchParams.get('returnTo') ?? ''
  expect(returnTo).toMatch(/^\/fleet\?query=payments/)
  expect(returnTo).not.toContain('signin')
  await page.goto('/?signin=signed-out')
  await expect(page.getByRole('status')).toHaveText('You have signed out.')
  await page.goto('/?signin=unknown')
  await expect(page.getByRole('alert')).toHaveCount(0)
  await expect(page.getByRole('status')).toHaveCount(0)
})

test('unavailable evidence never displays a healthy fleet', async ({ page }) => {
  await page.route('**/api/v1/snapshot', (route) =>
    route.fulfill(problem(503, 'unavailable', 'Unavailable')),
  )
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Fleet evidence unavailable' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Attention', exact: true })).not.toBeVisible()
})

test('a denied first collection remains visible without deployment inventory', async ({ page }) => {
  await page.route('**/api/v1/snapshot', async (route) => {
    const response = await route.fetch()
    const snapshot = snapshotSchema.parse(await response.json())
    await route.fulfill({
      response,
      json: {
        ...snapshot,
        mode: 'live',
        deployments: [],
        relationships: [],
        connections: snapshot.connections.map((connection) => ({
          ...connection,
          collection: 'denied',
          lastSuccessAt: null,
        })),
      },
    })
  })
  await page.goto('/')
  await expect(page.getByText('Collection needs attention', { exact: true })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Inspect clusters' })).toBeVisible()
  await expect(page.getByText('0 deployments observed')).toBeVisible()
})

test('cluster views explain API access and enrolled controller evidence', async ({ page }) => {
  await page.goto('/topology')
  // An uncollected cluster says so on the map and keeps its last known states.
  await expect(page.getByText('States are last known.')).toBeVisible()
  await page.getByRole('link', { name: /^edge-dc-1/ }).click()
  await expect(page.getByRole('heading', { name: 'edge-dc-1', exact: true })).toBeVisible()
  await expect(page.getByText(/cannot collect from this cluster/)).toBeVisible()
  // One row covers every API that fails the same way in the same namespaces.
  await expect(page.getByRole('cell', { name: 'Unreachable', exact: true })).toHaveCount(1)
  await page.goto('/topology/clusters/prod-eu-1')
  await expect(page.getByRole('cell', { name: 'API absent', exact: true })).toBeVisible()
  await page.getByText('Controller details', { exact: true }).click()
  await expect(
    page.getByText('example.invalid/openbao-operator:development', { exact: true }),
  ).toBeVisible()
  // A healthy operator is context on the deployment page; its details stay on the cluster view.
  await page.goto('/deployments/payments')
  await page.getByRole('link', { name: 'operator-prod-eu-1', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'prod-eu-1', exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Operator needs attention' })).toHaveCount(0)
  // An operator without current evidence explains why the deployment's evidence is suspect.
  await page.goto('/deployments/edge')
  await expect(page.getByRole('heading', { name: 'Operator needs attention' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'operator-edge', exact: true })).toBeVisible()
})

test('administrators add a paused cluster, edit policies, and inspect changes', async ({
  page,
}) => {
  await page.emulateMedia({ colorScheme: 'dark' })
  let revision = 1
  let configuration: Configuration = {
    connections: [],
    users: [{ subject: 'local-demo', name: 'Administrator', admin: true, scopes: [] }],
    groups: [],
    relationships: [],
    maxBackupAgeHours: 24,
    startupGraceMinutes: 10,
  }
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
    localSignIn: true,
    localAccounts: [],
    changes: [
      {
        revision,
        actor: 'local-demo',
        summary: revision > 1 ? 'Updated configuration' : 'Initial configuration imported',
        at: new Date().toISOString(),
      },
    ],
  })
  await page.route('**/api/v1/snapshot', async (route) => {
    const response = await route.fetch()
    const snapshot = snapshotSchema.parse(await response.json())
    await route.fulfill({
      response,
      json: { ...snapshot, viewer: { ...snapshot.viewer, admin: true } },
    })
  })
  await page.route('**/api/v1/settings', async (route) => {
    if (route.request().method() === 'PUT') {
      // Runtime validation keeps the browser test on the published configuration contract.
      const request = settingsUpdateSchema.parse(route.request().postDataJSON())
      expect(request.revision).toBe(revision)
      configuration = request.configuration
      revision += 1
    }
    await route.fulfill({ status: 200, contentType: 'application/json', json: settings() })
  })
  await page.route('**/api/v1/settings/connections/rbac', (route) =>
    route.fulfill({
      status: 200,
      json: {
        rbac: '',
        serviceAccount: 'console-reader',
        serviceAccountNamespace: 'console-system',
      },
    }),
  )
  await page.goto('/settings')
  await page.getByRole('button', { name: 'Add cluster', exact: true }).click()
  await page.getByRole('textbox', { name: 'Cluster name', exact: true }).fill('Demo cluster')
  await page.getByRole('combobox', { name: 'Connection', exact: true }).selectOption('local')
  await page.getByRole('button', { name: 'Continue', exact: true }).click()
  await page.getByLabel('Namespace 1', { exact: true }).fill('team-a')
  await page.getByRole('button', { name: 'Continue', exact: true }).click()
  await page.getByRole('button', { name: 'Continue', exact: true }).click()
  await page.getByRole('button', { name: 'Continue', exact: true }).click()
  await page.getByRole('button', { name: 'Save paused', exact: true }).click()
  await page.getByRole('link', { name: 'Open cluster settings', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Demo cluster', exact: true })).toBeVisible()
  await expect(page.getByText('Paused', { exact: true })).toBeVisible()
  await page.screenshot({ path: '../.local/screenshots/settings-dark.png', fullPage: true })
  const section = (name: string) =>
    page
      .getByRole('navigation', { name: 'Settings sections' })
      .getByRole('link', { name, exact: true })
  await section('Policies').click()
  await expect(section('Policies')).toHaveAttribute('aria-current', 'page')
  await page.getByRole('spinbutton', { name: 'Maximum backup age (hours)' }).fill('12')
  await page.getByRole('button', { name: 'Save policies' }).click()
  await expect(page.getByRole('status')).toHaveText('Saved.')
  await expect.poll(() => configuration.maxBackupAgeHours).toBe(12)
  expect(revision).toBe(3)
  await section('Changes').click()
  await expect(page.getByText('Updated configuration', { exact: true })).toBeVisible()
  await section('Clusters').click()
  await page.getByRole('button', { name: 'Edit', exact: true }).click()
  await page.screenshot({ path: '../.local/screenshots/settings-editor.png', fullPage: true })
})

test('findings lead to their highlighted evidence', async ({ page }) => {
  await page.goto('/deployments/customer-x')
  await expect(page.getByRole('tab', { name: 'Backups Has finding' })).toBeVisible()
  await expect(page.getByRole('button', { name: /^Backups.*Open Backups$/ })).toHaveClass(
    /tile-high/,
  )
  await page.getByRole('button', { name: 'View backups', exact: true }).click()
  await expect(page).toHaveURL(/tab=backups/)
  const failures = page.locator('.summary-stat', { hasText: 'Consecutive failures' })
  await expect(failures).toHaveClass(/finding-high/)
  await expect(
    page.getByRole('img', { name: /backups completed and 2 failures observed/ }),
  ).toBeVisible()
  await page.goto('/deployments/staging')
  await page.getByRole('tab', { name: /^Lifecycle/ }).click()
  await expect(page.getByRole('heading', { name: 'Version' })).toBeVisible()
  await expect(page.getByText('PodNotReady', { exact: true })).toBeVisible()
  await expect(page.locator('.detail-rows > div', { hasText: 'Upgrade' }).first()).toHaveClass(
    /finding-high/,
  )
})

test('history explains recorded changes in plain language', async ({ page }) => {
  await page.goto('/deployments/identity')
  await expect(
    page.getByRole('heading', { name: 'Recent changes' }).locator('..').locator('..'),
  ).toContainText('State Attention → Critical')
  await page.getByRole('button', { name: 'View history' }).click()
  await expect(page).toHaveURL(/tab=history/)
  await expect(
    page.getByRole('img', { name: /Recorded state from .*Healthy, then Attention, then Critical/ }),
  ).toBeVisible()
  await expect(page.getByText('OpenBaoSealed reported True', { exact: true })).toBeVisible()
  await expect(page.getByText('Ready replicas 3 → 1', { exact: true })).toBeVisible()
  await page.getByRole('tab', { name: /^Conditions/ }).click()
  await expect(page.locator('.condition', { hasText: 'OpenBaoSealed' })).toHaveClass(
    /finding-critical/,
  )
  // Caveats stay available as the info button's description instead of page text.
  await expect(page.getByRole('button', { name: 'About conditions' })).toHaveAccessibleDescription(
    /Conditions from an older resource generation are not current evidence/,
  )
})

test('fleet lists the most severe deployments first', async ({ page }) => {
  await page.goto('/fleet')
  const rows = page.locator('.fleet-table tbody tr')
  await expect(rows.first()).toContainText('identity-bao')
  await expect(rows.nth(1)).toContainText('Attention')
  await expect(page.getByRole('columnheader', { name: /State/ })).toHaveAttribute(
    'aria-sort',
    'ascending',
  )
  await page.getByRole('combobox', { name: 'Filter by collection' }).selectOption('gaps')
  await expect(page.getByText('Showing 1 of 12 deployments')).toBeVisible()
  await expect(page).toHaveURL(/collection=gaps/)
})

test('settings require an administrator', async ({ page }) => {
  await page.goto('/settings')
  await expect(page.getByRole('heading', { name: 'Administrator access required' })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Settings', exact: true })).toHaveCount(0)
  await page.getByRole('link', { name: 'View cluster coverage' }).click()
  await expect(page.getByRole('heading', { name: 'Topology', exact: true })).toBeVisible()
})

test('mobile navigation shows every destination and tables fit as cards', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/fleet')
  const navigation = page.getByRole('navigation', { name: 'Main navigation' })
  for (const name of ['Attention', 'Fleet', 'Topology', 'Activity'])
    await expect(navigation.getByRole('link', { name: new RegExp(`^${name}`) })).toBeInViewport()
  await expect(page.getByRole('link', { name: 'identity-bao identity', exact: true })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  for (const path of ['/', '/topology', '/topology/clusters/prod-eu-1']) {
    await page.goto(path)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  }
})

test('incomplete analysis shows unknown counters and unresolved findings', async ({ page }) => {
  await page.route('**/api/v1/snapshot', async (route) => {
    const response = await route.fetch()
    const snapshot = snapshotSchema.parse(await response.json())
    const deployments = snapshot.deployments.map((deployment) =>
      deployment.id === 'customer-x' && deployment.backup
        ? {
            ...deployment,
            evidence: { ...deployment.evidence, analysisIncomplete: true },
            backup: { ...deployment.backup, consecutiveFailures: null },
          }
        : deployment,
    )
    await route.fulfill({ response, json: { ...snapshot, deployments } })
  })
  await page.goto('/deployments/customer-x')
  await expect(
    page.getByText('Some diagnostic rules could not be evaluated.', { exact: false }),
  ).toBeVisible()
  await expect(page.getByRole('heading', { name: 'customer-x-bao', exact: true })).toBeVisible()
  await expect(page.getByText('Backup failed', { exact: true }).first()).toBeVisible()
  await page.getByRole('tab', { name: /^Backups/ }).click()
  await expect(page.locator('.backup-summary')).toContainText('Unknown')
})
