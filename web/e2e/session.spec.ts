import { expect, test } from '@playwright/test'
import type { Page } from '@playwright/test'
import { administrator } from './fixtures/administrator'
import { toVerify } from './fixtures/onboarding'
import { problem } from './fixtures/problem'

// Pending fixture requests are abandoned when a test ends.
test.afterEach(async ({ page }) => {
  await page.unrouteAll({ behavior: 'ignoreErrors' })
})

// failSnapshot makes later snapshot requests fail with the given status until restored.
async function failSnapshot(page: Page, status: number) {
  const state = { failing: false }
  await page.route('**/api/v1/snapshot', async (route) => {
    if (!state.failing) {
      await route.fallback()
      return
    }
    await route.fulfill(
      problem(status, status === 401 ? 'unauthenticated' : 'unavailable', 'Unavailable.'),
    )
  })
  return state
}

test('a session that expires after load leads to sign-in', async ({ page }) => {
  const snapshot = await failSnapshot(page, 401)
  await page.goto('/fleet?query=payments')
  await expect(page.getByRole('heading', { name: 'Fleet', exact: true })).toBeVisible()
  snapshot.failing = true
  await page
    .getByRole('navigation', { name: 'Main navigation' })
    .getByRole('link', { name: /^Attention/ })
    .click()
  await page.getByRole('button', { name: 'Refresh' }).click()
  const signIn = page.getByRole('link', { name: 'Sign in with single sign-on' })
  await expect(signIn).toBeVisible()
  expect(
    new URL((await signIn.getAttribute('href')) ?? '', page.url()).searchParams.get('returnTo'),
  ).toBe('/')
})

test('a transient refresh failure keeps the open editor and its input', async ({ page }) => {
  await page.clock.install()
  await administrator(page)
  await toVerify(page, { local: false })
  const snapshot = await failSnapshot(page, 503)
  const kubeconfig = 'fixture-reader-kubeconfig'
  await page.getByRole('button', { name: 'Back', exact: true }).click()
  await expect(page.getByRole('textbox', { name: /^Reader kubeconfig/ })).toHaveValue(kubeconfig)
  snapshot.failing = true
  // The snapshot refreshes every 30 seconds and retries twice before reporting a failure.
  await page.clock.fastForward(31_000)
  await page.clock.fastForward(5_000)
  await expect(page.getByText('The console could not refresh.')).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Fleet evidence unavailable' })).toHaveCount(0)
  await expect(page.getByRole('textbox', { name: /^Reader kubeconfig/ })).toHaveValue(kubeconfig)
  snapshot.failing = false
  await page.getByRole('button', { name: 'Retry now' }).click()
  await expect(page.getByText('The console could not refresh.')).toBeHidden()
  await expect(page.getByRole('textbox', { name: /^Reader kubeconfig/ })).toHaveValue(kubeconfig)
})
