import AxeBuilder from '@axe-core/playwright'
import { expect, test } from '@playwright/test'
import type { Page } from '@playwright/test'
import { administrator } from './fixtures/administrator'
import { problem } from './fixtures/problem'

// Automated checks find a subset of accessibility problems: missing names, invalid ARIA,
// insufficient contrast, and page structure such as a missing main heading. They do not
// replace keyboard and screen reader review.

async function violations(page: Page) {
  const results = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice'])
    .analyze()
  return results.violations.map((violation) => ({
    impact: violation.impact,
    rule: violation.id,
    targets: violation.nodes.map((node) => node.target.join(' ')),
  }))
}

const pages = [
  { path: '/', heading: 'Attention' },
  { path: '/fleet', heading: 'Fleet' },
  { path: '/topology', heading: 'Topology' },
  { path: '/activity', heading: 'Activity' },
  { path: '/clusters', heading: 'Clusters' },
  { path: '/deployments/payments', heading: 'payments-bao' },
  { path: '/deployments/identity?tab=history', heading: 'identity-bao' },
  { path: '/recovery/payments-recovery', heading: 'Recovery path' },
  // The demonstration viewer is not an administrator.
  { path: '/settings', heading: 'Settings' },
  { path: '/settings/clusters/new', heading: 'Add cluster' },
  { path: '/deployments/missing', heading: 'Deployment not found' },
  { path: '/recovery/missing', heading: 'Recovery path unavailable' },
  { path: '/missing', heading: 'Page not found' },
]

for (const scheme of ['light', 'dark'] as const)
  for (const { path, heading } of pages)
    test(`${path} has no accessibility violations in ${scheme} mode`, async ({ page }) => {
      await page.emulateMedia({ colorScheme: scheme, reducedMotion: 'reduce' })
      await page.goto(path)
      await expect(page.getByRole('heading', { name: heading, level: 1 })).toBeVisible()
      expect(await violations(page)).toEqual([])
    })

test('settings and the cluster editor have no accessibility violations', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await administrator(page)
  await page.goto('/settings')
  await expect(page.getByRole('heading', { name: 'Settings', level: 1 })).toBeVisible()
  expect(await violations(page)).toEqual([])
  await page.goto('/settings/clusters/new')
  await expect(page.getByRole('heading', { name: 'Add cluster', level: 1 })).toBeVisible()
  expect(await violations(page)).toEqual([])
  await page.goto('/settings?section=access')
  await page.getByRole('button', { name: 'Add user', exact: true }).click()
  await expect(page.getByRole('dialog', { name: 'Add user' })).toBeVisible()
  expect(await violations(page)).toEqual([])
})

test('sign-in and setup have no accessibility violations', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await page.route('**/api/v1/snapshot', (route) =>
    route.fulfill(problem(401, 'unauthenticated', 'Sign in.')),
  )
  await page.goto('/?signin=denied')
  await expect(page.getByRole('link', { name: 'Sign in with single sign-on' })).toBeVisible()
  expect(await violations(page)).toEqual([])
  await page.route('**/api/v1/setup', (route) =>
    route.fulfill({
      status: 200,
      json: {
        state: 'required',
        callbackUrl: 'https://console.example/auth/callback',
        sessionExpiresAt: null,
      },
    }),
  )
  await page.goto('/setup')
  await expect(page.getByLabel('Setup token', { exact: true })).toBeVisible()
  expect(await violations(page)).toEqual([])
})

test('navigation focuses the heading of pages without content', async ({ page }) => {
  await page.goto('/settings')
  await expect(page.getByRole('heading', { name: 'Administrator access required' })).toBeVisible()
  await page.getByRole('link', { name: 'View cluster coverage' }).click()
  await expect(page.getByRole('heading', { name: 'Clusters', level: 1 })).toBeFocused()
  await page.goBack()
  await expect(page.getByRole('heading', { name: 'Settings', level: 1 })).toBeFocused()
  await page.goto('/deployments/missing')
  await page.getByRole('link', { name: 'Back to fleet' }).click()
  await expect(page.getByRole('heading', { name: 'Fleet', level: 1 })).toBeFocused()
  await page.goBack()
  await expect(page.getByRole('heading', { name: 'Deployment not found', level: 1 })).toBeFocused()
})
