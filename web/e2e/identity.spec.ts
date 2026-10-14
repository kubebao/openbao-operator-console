import type { Page } from '@playwright/test'
import { expect, test } from '@playwright/test'
import type { IdentityTrial, Settings } from '../src/api/schema'
import { identityTrialSchema, snapshotSchema } from '../src/api/schema'

// This fixture fakes the settings API to exercise the identity activation flow. It does
// not exercise the Go server's trial or access rules decision.

const settings: Settings = {
  revision: 4,
  configuration: {
    connections: [],
    users: [
      { subject: '42', name: 'Ada Administrator', admin: true, scopes: [] },
      { subject: '7', name: 'Reader', admin: false, scopes: [] },
    ],
    groups: [{ name: 'platform', admin: true, scopes: [] }],
    relationships: [],
    maxBackupAgeHours: 24,
    startupGraceMinutes: 10,
  },
  identity: {
    issuer: 'https://keycloak.example/realms/platform',
    clientId: 'console',
    ca: '',
    groupsClaim: 'groups',
    secretConfigured: true,
  },
  changes: [],
}

async function identityBackend(page: Page) {
  const trials: IdentityTrial[] = []
  await page.route('**/api/v1/snapshot', async (route) => {
    const response = await route.fetch()
    const snapshot = snapshotSchema.parse(await response.json())
    await route.fulfill({
      response,
      json: { ...snapshot, viewer: { ...snapshot.viewer, admin: true } },
    })
  })
  await page.route('**/api/v1/settings', (route) =>
    route.fulfill({ status: 200, contentType: 'application/json', json: settings }),
  )
  await page.route('**/api/v1/settings/identity/trial', async (route) => {
    // Runtime validation keeps the browser test on the published trial contract.
    trials.push(identityTrialSchema.parse(route.request().postDataJSON()))
    await route.fulfill({ status: 200, json: { url: providerLogin } })
  })
  await page.route(`${providerLogin}*`, (route) =>
    route.fulfill({ contentType: 'text/html', body: '<title>Fixture provider login</title>' }),
  )
  return trials
}

const providerLogin = 'https://provider.example/authorize'

test('an issuer change shows affected access and requires a decision', async ({ page }) => {
  const trials = await identityBackend(page)
  await page.goto('/settings?section=identity')
  await page.getByRole('textbox', { name: 'Issuer', exact: true }).fill('https://gitlab.example')
  await page.getByRole('button', { name: 'Review access changes' }).click()

  const dialog = page.getByRole('dialog', { name: 'Choose access for the new issuer' })
  await expect(dialog).toBeVisible()
  await expect(dialog).toContainText('belongs to https://keycloak.example/realms/platform')
  const affected = dialog.getByRole('list', { name: 'Affected users and groups (3)' })
  await expect(affected.getByRole('listitem')).toHaveCount(3)
  await expect(affected).toContainText('Ada Administrator')
  await expect(affected).toContainText('Administrator group')

  // The decision is required; Escape closes the dialog and returns focus to the form.
  await dialog.getByRole('button', { name: 'Verify login and activate' }).click()
  expect(trials).toHaveLength(0)
  await page.keyboard.press('Escape')
  await expect(dialog).toBeHidden()
  await expect(page.getByRole('button', { name: 'Review access changes' })).toBeFocused()

  await page.getByRole('button', { name: 'Review access changes' }).click()
  await dialog.getByRole('radio', { name: /Carry over to the new issuer/ }).check()
  await page.screenshot({ path: '../.local/screenshots/identity-access-rules.png', fullPage: true })
  await dialog.getByRole('button', { name: 'Verify login and activate' }).click()
  await page.waitForURL(providerLogin)
  expect(trials).toEqual([
    {
      revision: 4,
      identity: {
        issuer: 'https://gitlab.example',
        clientId: 'console',
        ca: '',
        groupsClaim: 'groups',
      },
      accessRules: 'carryOver',
    },
  ])
})

test('a same-issuer change keeps access without a decision', async ({ page }) => {
  const trials = await identityBackend(page)
  await page.goto('/settings?section=identity')
  await page.getByLabel('Client secret').fill('rotated-secret')
  await page.getByRole('button', { name: 'Verify login and activate' }).click()
  await page.waitForURL(providerLogin)
  expect(trials).toHaveLength(1)
  expect(trials[0]?.accessRules).toBeUndefined()
  expect(trials[0]?.identity.clientSecret).toBe('rotated-secret')
})
