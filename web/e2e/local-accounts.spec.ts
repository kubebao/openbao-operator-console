import { expect, test } from '@playwright/test'
import { z } from 'zod'
import type { SetupStatus } from '../src/api/schema'
import { problem } from './fixtures/problem'

const recoveryCodes = Array.from(
  { length: 10 },
  (_, i) => `ABCD-EFGH-IJKL-MN${String(i).padStart(2, '0')}`,
)
const password = 'maple river lantern 42'

test('setup creates a local administrator with an authenticator and recovery codes', async ({
  page,
}) => {
  let status: SetupStatus = {
    state: 'unlocked',
    callbackUrl: 'https://console.example/auth/callback',
    sessionExpiresAt: new Date(Date.now() + 900_000).toISOString(),
  }
  let created: unknown = null
  await page.route('**/api/v1/setup', (route) => route.fulfill({ status: 200, json: status }))
  await page.route('**/api/v1/setup/local/authenticator', (route) =>
    route.fulfill({
      status: 200,
      json: {
        key: 'JBSWY3DPEHPK3PXP',
        uri: 'otpauth://totp/OpenBao%20Operator%20Console:console.example?secret=JBSWY3DPEHPK3PXP',
        qrSize: 29,
        qrPath: 'M4 4h7v1h-7zM18 4h7v1h-7z',
      },
    }),
  )
  await page.route('**/api/v1/setup/local', async (route) => {
    created = route.request().postDataJSON()
    status = { ...status, state: 'complete', sessionExpiresAt: null }
    await route.fulfill({ status: 200, json: { recoveryCodes } })
  })
  await page.goto('/setup')
  await expect(page.getByRole('radio', { name: /Identity provider/ })).toBeChecked()
  await page.getByRole('radio', { name: /Local administrator/ }).check()
  await page.getByLabel(/^Username/).fill('Admin')
  await expect(page.getByLabel(/^Username/)).toHaveValue('admin')
  await page.getByLabel(/^Display name/).fill('First administrator')
  await page.getByLabel(/^Password/).fill(password)
  await page.getByLabel('Confirm password', { exact: true }).fill('a different value here')
  await expect(page.getByRole('button', { name: 'Create administrator' })).toBeDisabled()
  await page.getByRole('button', { name: 'Add an authenticator app' }).click()
  await expect(page.getByRole('img', { name: 'QR code for your authenticator app' })).toBeVisible()
  await expect(page.getByText('JBSWY3DPEHPK3PXP', { exact: true })).toBeVisible()
  await page.getByLabel(/^Authenticator code/).fill('123456')
  await page.screenshot({ path: '../.local/screenshots/setup-local-light.png', fullPage: true })
  await page.getByRole('button', { name: 'Create administrator' }).click()
  await expect(page.getByRole('alert')).toHaveText('The passwords do not match.')
  expect(created).toBeNull()
  await page.getByLabel('Confirm password', { exact: true }).fill(password)
  await page.getByLabel(/^Authenticator code/).fill('123456')
  await page.getByRole('button', { name: 'Create administrator' }).click()
  await expect(page.getByRole('heading', { name: 'Save your recovery codes' })).toBeVisible()
  expect(created).toEqual({
    username: 'admin',
    name: 'First administrator',
    password,
    code: '123456',
  })
  await expect(page.getByRole('region', { name: 'Recovery codes', exact: true })).toContainText(
    recoveryCodes[0] ?? '',
  )
  await expect(page.getByRole('button', { name: 'Connect your first cluster' })).toBeDisabled()
  await page
    .getByRole('checkbox', { name: 'I stored these recovery codes in a safe place' })
    .check()
  await expect(page.getByRole('link', { name: 'Connect your first cluster' })).toHaveAttribute(
    'href',
    '/settings/clusters/new',
  )
  await page.screenshot({
    path: '../.local/screenshots/setup-local-recovery-light.png',
    fullPage: true,
  })
  const storage = await page.evaluate(() =>
    JSON.stringify([Object.values(localStorage), Object.values(sessionStorage), location.href]),
  )
  expect(storage).not.toContain(password)
  expect(storage).not.toContain(recoveryCodes[0])
})

test('sign-in offers a local account beside single sign-on and reports recovery codes', async ({
  page,
}) => {
  await page.route('**/api/v1/snapshot', (route) =>
    route.fulfill(problem(401, 'unauthenticated', 'Sign in.')),
  )
  await page.route('**/api/v1/sign-in', (route) =>
    route.fulfill({ status: 200, json: { singleSignOn: true, localAccounts: true } }),
  )
  const attempts: unknown[] = []
  await page.route('**/api/v1/sign-in/local', async (route) => {
    const input = z
      .object({ username: z.string(), password: z.string(), code: z.string() })
      .parse(route.request().postDataJSON())
    attempts.push(input)
    if (input.password !== password) {
      await route.fulfill(
        problem(401, 'unauthenticated', 'The username, password, or code is incorrect.'),
      )
      return
    }
    await route.fulfill({ status: 200, json: { recoveryCodeUsed: true, recoveryCodesLeft: 2 } })
  })
  await page.goto('/fleet')
  await expect(page.getByRole('link', { name: 'Sign in with single sign-on' })).toBeVisible()
  const form = page.getByRole('form', { name: 'Sign in with a local account' })
  await form.getByLabel(/^Username/).fill('admin')
  await form.getByLabel(/^Password/).fill('wrong password value')
  await form.getByLabel(/^Authenticator code/).fill('123456')
  await form.getByRole('button', { name: 'Sign in' }).click()
  await expect(form.getByRole('alert')).toHaveText('The username, password, or code is incorrect.')
  await expect(form.getByLabel(/^Password/)).toBeEmpty()
  await form.getByLabel(/^Password/).fill(password)
  await form.getByLabel(/^Authenticator code/).fill(recoveryCodes[0] ?? '')
  await form.getByRole('button', { name: 'Sign in' }).click()
  await expect(page.getByRole('status')).toContainText('2 of 10 codes remain')
  await expect(page.getByRole('status')).toContainText('reset your authenticator soon')
  expect(attempts).toHaveLength(2)
  await page.screenshot({ path: '../.local/screenshots/sign-in-local-light.png', fullPage: true })
})

test('a console with local accounts only hides single sign-on', async ({ page }) => {
  await page.route('**/api/v1/snapshot', (route) =>
    route.fulfill(problem(401, 'unauthenticated', 'Sign in.')),
  )
  await page.route('**/api/v1/sign-in', (route) =>
    route.fulfill({ status: 200, json: { singleSignOn: false, localAccounts: true } }),
  )
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/fleet')
  await expect(page.getByRole('form', { name: 'Sign in with a local account' })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Sign in with single sign-on' })).toHaveCount(0)
  await expect(page.getByText('or use a local account')).toHaveCount(0)
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
})
