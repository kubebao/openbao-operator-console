import { expect, test } from '@playwright/test'
import { administrator } from './fixtures/administrator'
import { toVerify } from './fixtures/onboarding'

test('automatic renewal is prepared before credential upload and saved with enrollment', async ({
  page,
}) => {
  await page.emulateMedia({ reducedMotion: 'reduce' })
  const state = await administrator(page)
  await toVerify(page, { local: false, automatic: true })
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
  await toVerify(page, { local: false, automatic: true })
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

test('operator observation starts from the operator chart defaults', async ({ page }) => {
  await administrator(page)
  await page.goto('/settings/clusters/new')
  await page.getByRole('textbox', { name: 'Cluster name', exact: true }).fill('Production EU')
  await page.getByRole('combobox', { name: 'Connection', exact: true }).selectOption('local')
  await page.getByRole('button', { name: 'Continue', exact: true }).click()
  await page.getByRole('textbox', { name: 'Namespace 1', exact: true }).fill('team-a')
  await page.getByRole('checkbox', { name: 'Observe the operator for these namespaces' }).check()
  await expect(page.getByRole('textbox', { name: 'Operator namespace', exact: true })).toHaveValue(
    'openbao-operator-system',
  )
  await expect(page.getByRole('textbox', { name: 'Operator Deployment', exact: true })).toHaveValue(
    'openbao-operator-controller',
  )
  await expect(
    page.getByRole('textbox', { name: 'Controller container', exact: true }),
  ).toHaveValue('manager')
  await page.getByRole('button', { name: 'Continue', exact: true }).click()
  await expect(
    page.getByText('readerNamespaces and operatorReaders already grant', { exact: false }),
  ).toBeVisible()
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
