import { expect, test } from '@playwright/test'
import { z } from 'zod'
import type { SetupStatus } from '../src/api/schema'
import { problem } from './fixtures/problem'

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
      await route.fulfill(problem(401, 'invalid_token', 'The setup token was not accepted.'))
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
    route.fulfill(problem(400, 'invalid', 'Identity discovery failed. Check the issuer.')),
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
    route.fulfill(problem(401, 'unauthenticated', 'Sign in.')),
  )
  await page.goto('/fleet')
  await expect(page.getByRole('heading', { name: 'OpenBao Operator Console' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Fleet evidence unavailable' })).not.toBeVisible()
  await expect(page.getByText('Platform team', { exact: true })).not.toBeVisible()
  await expect(page.getByRole('link', { name: 'Sign in with single sign-on' })).toHaveAttribute(
    'href',
    /returnTo=/,
  )
})
