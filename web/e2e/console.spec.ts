import type { Configuration } from '../src/api/schema'
import { expect, test } from '@playwright/test'
import { snapshotSchema, settingsUpdateSchema } from '../src/api/schema'

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
  await page.getByRole('combobox', { name: 'Filter by state' }).selectOption('critical')
  await expect(page.getByText('Showing 1 of 12 deployments')).toBeVisible()
  await expect(page.getByRole('link', { name: 'identity-bao identity', exact: true })).toBeVisible()
})

test('topology selection opens the recovery evidence path', async ({ page }) => {
  await page.goto('/topology')
  await expect(page.getByRole('heading', { name: 'Topology', exact: true })).toBeVisible()
  await expect(page.getByText('Replication', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Inspect payments/payments-read', exact: true }).click()
  await expect(page.getByText('9007199254741007', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Inspect payments/payments-bao', exact: true }).click()
  await page.getByRole('button', { name: 'Select payments-recovery' }).click()
  await page.screenshot({ path: '../.local/screenshots/topology-light.png', fullPage: true })
  await page.reload()
  await expect(page.getByText('Snapshot storage', { exact: true })).toBeVisible()
  await page.getByRole('link', { name: 'Inspect recovery path' }).click()
  await expect(page.getByRole('heading', { name: 'Recovery path', exact: true })).toBeVisible()
  await expect(page.getByText('Measured RPO', { exact: true })).toBeVisible()
  await expect(page.getByText('Current readability unknown', { exact: true })).toBeVisible()
})

test('dark mode persists and global search is keyboard accessible', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'light' })
  await page.goto('/')
  await page.getByRole('button', { name: 'Switch to dark mode' }).click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await page.reload()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await page.keyboard.press('ControlOrMeta+k')
  await page.getByRole('combobox', { name: 'Search your fleet' }).fill('finance')
  await expect(page.getByRole('option', { name: /finance-bao/ })).toHaveAttribute(
    'aria-selected',
    'true',
  )
  await page.keyboard.press('Enter')
  await expect(page.getByRole('heading', { name: 'finance-bao', exact: true })).toBeVisible()
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

test('unavailable evidence never displays a healthy fleet', async ({ page }) => {
  await page.route('**/api/v1/snapshot', (route) =>
    route.fulfill({
      status: 503,
      contentType: 'application/json',
      body: JSON.stringify({ code: 'unavailable', message: 'Unavailable' }),
    }),
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
        activity: [],
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

test('clusters explain API access and enrolled controller evidence', async ({ page }) => {
  await page.goto('/clusters')
  await expect(page.getByRole('heading', { name: 'Clusters', exact: true })).toBeVisible()
  // Clusters with collection gaps start expanded; healthy clusters stay collapsed.
  await expect(page.getByRole('button', { name: /^edge-dc-1/ })).toHaveAttribute(
    'aria-expanded',
    'true',
  )
  const toggle = page.getByRole('button', { name: /^prod-eu-1/ })
  await expect(toggle).toHaveAttribute('aria-expanded', 'false')
  await toggle.click()
  const details = page.locator('#cluster-prod-eu-1')
  await expect(
    details.getByRole('heading', { name: 'operator-prod-eu-1', exact: true }),
  ).toBeVisible()
  await expect(details.getByText('API absent', { exact: true }).first()).toBeVisible()
  await expect(
    details.getByText('example.invalid/openbao-operator:development', { exact: true }),
  ).toBeVisible()
  await page
    .getByRole('row', { name: /prod-eu-1/ })
    .getByRole('link', { name: '4' })
    .click()
  await expect(page).toHaveURL(/cluster=prod-eu-1/)
  await expect(page.getByText('Showing 4 of 12 deployments')).toBeVisible()
  // A healthy operator is context on the deployment page; its details stay on Clusters.
  await page.goto('/deployments/payments')
  await expect(page.getByRole('link', { name: 'operator-prod-eu-1', exact: true })).toBeVisible()
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
  await page.getByRole('button', { name: 'Policies', exact: true }).click()
  await page.getByRole('spinbutton', { name: 'Maximum backup age (hours)' }).fill('12')
  await page.getByRole('button', { name: 'Save policies' }).click()
  await expect(page.getByRole('status')).toHaveText('Saved.')
  await expect.poll(() => configuration.maxBackupAgeHours).toBe(12)
  expect(revision).toBe(3)
  await page.getByRole('button', { name: 'Changes', exact: true }).click()
  await expect(page.getByText('Updated configuration', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Clusters', exact: true }).click()
  await page.getByRole('button', { name: 'Edit', exact: true }).click()
  await page.screenshot({ path: '../.local/screenshots/settings-editor.png', fullPage: true })
})

test('findings lead to their highlighted evidence', async ({ page }) => {
  await page.goto('/deployments/customer-x')
  await expect(page.getByRole('tab', { name: 'Backups Has finding' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Backups: open Backups' })).toHaveClass(/tile-high/)
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
  await expect(page.getByRole('button', { name: /Collection gaps/ })).toHaveClass(/selected/)
})

test('settings require an administrator', async ({ page }) => {
  await page.goto('/settings')
  await expect(page.getByRole('heading', { name: 'Administrator access required' })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Settings', exact: true })).toHaveCount(0)
  await page.getByRole('link', { name: 'View cluster coverage' }).click()
  await expect(page.getByRole('heading', { name: 'Clusters', exact: true })).toBeVisible()
})

test('mobile navigation shows every destination and tables fit as cards', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/fleet')
  const navigation = page.getByRole('navigation', { name: 'Main navigation' })
  for (const name of ['Attention', 'Fleet', 'Topology', 'Activity', 'Clusters'])
    await expect(navigation.getByRole('link', { name: new RegExp(`^${name}`) })).toBeInViewport()
  await expect(page.getByRole('link', { name: 'identity-bao identity', exact: true })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  for (const path of ['/', '/clusters']) {
    await page.goto(path)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  }
})
