import { expect, test } from '@playwright/test'
import type { Page, Route } from '@playwright/test'
import { z } from 'zod'
import type { LocalAccount } from '../src/api/sign-in'
import { administrator } from './fixtures/administrator'
import { problem } from './fixtures/problem'

const codes = Array.from({ length: 10 }, (_, i) => `WXYZ-2345-6789-AB${String(i).padStart(2, '0')}`)

interface LocalBackend {
  revision: number
  localSignIn: boolean
  accounts: LocalAccount[]
}

function settingsJSON(backend: LocalBackend) {
  return {
    revision: backend.revision,
    configuration: {
      connections: [],
      users: [{ subject: 'oidc-admin', name: 'Administrator', admin: true, scopes: [] }],
      groups: [],
      relationships: [],
      maxBackupAgeHours: 24,
      startupGraceMinutes: 10,
    },
    identity: {
      issuer: 'https://identity.example',
      clientId: 'console',
      ca: '',
      groupsClaim: 'groups',
      secretConfigured: true,
    },
    localSignIn: backend.localSignIn,
    localAccounts: backend.accounts,
    changes: [],
  }
}

function enrollment(id: string) {
  return {
    accountId: id,
    enrollmentUrl: `https://console.example/enroll#token=${id}.secret`,
    expiresAt: '2030-01-02T03:04:05Z',
  }
}

async function routeLocalAccounts(page: Page, backend: LocalBackend) {
  const settings = (route: Route) => route.fulfill({ status: 200, json: settingsJSON(backend) })
  await page.route('**/api/v1/settings', settings)
  await page.route('**/api/v1/settings/local-accounts', async (route) => {
    const input = z
      .object({ revision: z.number(), username: z.string(), name: z.string(), admin: z.boolean() })
      .parse(route.request().postDataJSON())
    expect(input.revision).toBe(backend.revision)
    backend.revision += 1
    backend.accounts.push({
      id: `${input.username}-id`,
      username: input.username,
      name: input.name,
      admin: input.admin,
      scopes: [],
      state: 'enrollmentPending',
      enrollmentExpiresAt: '2030-01-02T03:04:05Z',
    })
    await route.fulfill({ status: 200, json: enrollment(`${input.username}-id`) })
  })
  await page.route(/\/api\/v1\/settings\/local-accounts\/[^/]+\/enrollment$/, (route) =>
    route.fulfill({ status: 200, json: enrollment('reader-id') }),
  )
  await page.route(/\/api\/v1\/settings\/local-accounts\/[^/?]+(\?.*)?$/, async (route) => {
    const id = new URL(route.request().url()).pathname.split('/').at(-1)
    expect(route.request().method()).toBe('DELETE')
    backend.accounts = backend.accounts.filter((account) => account.id !== id)
    backend.revision += 1
    await settings(route)
  })
  await page.route('**/api/v1/settings/local-sign-in', async (route) => {
    backend.localSignIn = z
      .object({ enabled: z.boolean() })
      .parse(route.request().postDataJSON()).enabled
    backend.revision += 1
    await settings(route)
  })
}

test('administrators add, reset, and remove local accounts', async ({ page }) => {
  await administrator(page)
  const backend: LocalBackend = {
    revision: 7,
    localSignIn: true,
    accounts: [
      {
        id: 'admin-id',
        username: 'admin',
        name: 'Break glass',
        admin: true,
        scopes: [],
        state: 'active',
        enrollmentExpiresAt: null,
      },
    ],
  }
  await routeLocalAccounts(page, backend)
  await page.goto('/settings?section=local')
  await expect(page.getByRole('heading', { name: 'Local accounts' })).toBeVisible()
  await expect(page.getByText('admin · Active')).toBeVisible()
  await page.getByRole('button', { name: 'Add local account' }).click()
  const editor = page.getByRole('dialog', { name: 'Add local account' })
  await editor.getByLabel(/^Username/).fill('Reader')
  await editor.getByLabel('Display name', { exact: true }).fill('Read-only user')
  await editor.getByRole('button', { name: 'Create account' }).click()
  const link = page.getByRole('dialog', { name: 'Enrollment link for reader' })
  await expect(link.getByRole('region', { name: 'Enrollment link' })).toContainText(
    'https://console.example/enroll#token=reader-id.secret',
  )
  await page.screenshot({
    path: '../.local/screenshots/local-account-link-light.png',
    fullPage: true,
  })
  await link.getByRole('button', { name: 'Done' }).click()
  await expect(page.getByText(/reader · Enrollment link valid until/)).toBeVisible()
  const reader = page.locator('.settings-row', { hasText: 'Read-only user' })
  await reader.getByRole('button', { name: 'Reset' }).click()
  await page.getByRole('button', { name: 'Reset account' }).click()
  await expect(page.getByRole('dialog', { name: 'Enrollment link for reader' })).toBeVisible()
  await page.getByRole('button', { name: 'Done' }).click()
  await reader.getByRole('button', { name: 'Remove' }).click()
  await page.getByRole('button', { name: 'Remove account' }).click()
  await expect(page.getByText('Read-only user')).toHaveCount(0)
  await page.getByRole('button', { name: 'Turn local sign-in off' }).click()
  await expect(page.getByRole('heading', { name: 'Local sign-in off' })).toBeVisible()
  expect(backend.localSignIn).toBe(false)
  await page.screenshot({ path: '../.local/screenshots/local-accounts-light.png', fullPage: true })
})

test('an enrollment link sets the password and authenticator once', async ({ page }) => {
  let completed: unknown = null
  await page.route('**/api/v1/enrollment', async (route) => {
    const { token } = z.object({ token: z.string() }).parse(route.request().postDataJSON())
    if (token !== 'reader-id.secret') {
      await route.fulfill(
        problem(401, 'invalid_token', 'This enrollment link is invalid, used, or expired.'),
      )
      return
    }
    await route.fulfill({
      status: 200,
      json: {
        username: 'reader',
        name: 'Read-only user',
        expiresAt: '2030-01-02T03:04:05Z',
        authenticator: {
          key: 'JBSWY3DPEHPK3PXP',
          uri: 'otpauth://totp/OpenBao%20Operator%20Console:reader@console.example?secret=JBSWY3DPEHPK3PXP',
          qrSize: 29,
          qrPath: 'M4 4h7v1h-7z',
        },
      },
    })
  })
  await page.route('**/api/v1/enrollment/complete', async (route) => {
    completed = route.request().postDataJSON()
    await route.fulfill({ status: 200, json: { recoveryCodes: codes } })
  })
  await page.goto('/enroll#token=forged.value')
  await expect(page.getByRole('alert')).toHaveText(
    'This enrollment link is invalid, used, or expired.',
  )
  await page.goto('/enroll#token=reader-id.secret')
  await page.reload()
  await expect(page.getByText('Set up the local account reader for Read-only user')).toBeVisible()
  await expect(page.getByRole('navigation', { name: 'Main navigation' })).toHaveCount(0)
  await page.getByLabel(/^Password/).fill('silver orchard winter path')
  await page.getByLabel('Confirm password', { exact: true }).fill('silver orchard winter path')
  await page.getByLabel(/^Authenticator code/).fill('123456')
  await page.getByRole('button', { name: 'Set up account' }).click()
  await expect(page.getByRole('heading', { name: 'Account ready' })).toBeVisible()
  expect(completed).toEqual({
    token: 'reader-id.secret',
    password: 'silver orchard winter path',
    code: '123456',
  })
  await page
    .getByRole('checkbox', { name: 'I stored these recovery codes in a safe place' })
    .check()
  await expect(page.getByRole('link', { name: 'Continue to the console' })).toHaveAttribute(
    'href',
    '/',
  )
})

test('local accounts change their password and recovery codes', async ({ page }) => {
  await page.route('**/api/v1/snapshot', async (route) => {
    const response = await route.fetch()
    const snapshot = z.looseObject({ viewer: z.looseObject({}) }).parse(await response.json())
    await route.fulfill({
      response,
      json: {
        ...snapshot,
        mode: 'live',
        viewer: { ...snapshot.viewer, name: 'Reader', subject: 'local:reader', localAccount: true },
      },
    })
  })
  const passwords: unknown[] = []
  await page.route('**/api/v1/account/password', async (route) => {
    passwords.push(route.request().postDataJSON())
    await route.fulfill({ status: 204 })
  })
  await page.route('**/api/v1/account/recovery-codes', (route) =>
    route.fulfill({ status: 200, json: { recoveryCodes: codes } }),
  )
  await page.goto('/')
  await page.getByRole('link', { name: 'Your account' }).click()
  await expect(page.getByRole('heading', { name: 'Your account' })).toBeVisible()
  const form = page.getByRole('form', { name: 'Change password' })
  await form.getByLabel('Current password', { exact: true }).fill('maple river lantern 42')
  await form.getByLabel(/^New password/).fill('copper kettle morning tide')
  await form.getByLabel('Confirm new password', { exact: true }).fill('another value entirely')
  await form.getByLabel(/^Authenticator code/).fill('123456')
  await form.getByRole('button', { name: 'Change password' }).click()
  await expect(form.getByRole('alert')).toHaveText('The new passwords do not match.')
  expect(passwords).toHaveLength(0)
  await form.getByLabel('Confirm new password', { exact: true }).fill('copper kettle morning tide')
  await form.getByRole('button', { name: 'Change password' }).click()
  await expect(form.getByRole('status')).toHaveText('Password changed.')
  expect(passwords).toEqual([
    {
      currentPassword: 'maple river lantern 42',
      newPassword: 'copper kettle morning tide',
      code: '123456',
    },
  ])
  const recovery = page.getByRole('form', { name: 'Replace recovery codes' })
  await recovery.getByLabel('Password', { exact: true }).fill('copper kettle morning tide')
  await recovery.getByLabel(/^Authenticator code/).fill('234567')
  await recovery.getByRole('button', { name: 'Replace recovery codes' }).click()
  await expect(page.getByRole('region', { name: 'Recovery codes', exact: true })).toContainText(
    codes[0] ?? '',
  )
  await page.screenshot({
    path: '../.local/screenshots/local-account-page-light.png',
    fullPage: true,
  })
})
