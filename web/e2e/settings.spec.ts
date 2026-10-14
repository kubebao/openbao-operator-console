import { expect, test } from '@playwright/test'
import type { Page } from '@playwright/test'
import { z } from 'zod'
import type { Configuration } from '../src/api/schema'
import { settingsUpdateSchema, snapshotSchema } from '../src/api/schema'
import { problem } from './fixtures/problem'

// These tests fake the administrator settings API in the browser. They exercise the UI
// workflows and request shapes, not the Go server's validation.

// Pending fixture requests are abandoned when a test ends.
test.afterEach(async ({ page }) => {
  await page.unrouteAll({ behavior: 'ignoreErrors' })
})

interface Backend {
  revision: number
  configuration: Configuration
  // conflict makes the next save fail as if another administrator saved first.
  conflict: boolean
}

function initialConfiguration(): Configuration {
  return {
    connections: [
      {
        id: 'prod-eu-1',
        name: 'Production EU 1',
        region: 'eu-west',
        disabled: false,
        authentication: 'local',
        server: '',
        credentialConfigured: false,
        namespaces: [
          { name: 'payments', team: 'Payments', environment: 'production', location: '' },
          { name: 'identity', team: 'Identity', environment: 'production', location: '' },
        ],
        operators: [],
      },
    ],
    users: [
      { subject: 'local-demo', name: 'Administrator', admin: true, scopes: [] },
      {
        subject: 'engineer',
        name: 'Engineer',
        admin: false,
        scopes: [{ connectionId: 'prod-eu-1', namespaces: ['payments', 'identity'] }],
      },
    ],
    groups: [],
    relationships: [],
    maxBackupAgeHours: 24,
    startupGraceMinutes: 10,
  }
}

async function settingsBackend(page: Page) {
  const backend: Backend = { revision: 1, configuration: initialConfiguration(), conflict: false }
  const settings = () => ({
    revision: backend.revision,
    configuration: backend.configuration,
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
      json: { ...snapshot, viewer: { ...snapshot.viewer, admin: true } },
    })
  })
  await page.route('**/api/v1/settings/connections/credentials', (route) =>
    route.fulfill({ status: 200, json: { credentials: [] } }),
  )
  await page.route('**/api/v1/settings', async (route) => {
    if (route.request().method() === 'PUT') {
      const request = settingsUpdateSchema.parse(route.request().postDataJSON())
      if (backend.conflict || request.revision !== backend.revision) {
        backend.conflict = false
        backend.revision += 1
        await route.fulfill(
          problem(
            409,
            'conflict',
            'Settings changed since they were loaded. Reload and try again.',
          ),
        )
        return
      }
      backend.configuration = request.configuration
      backend.revision += 1
    }
    await route.fulfill({ status: 200, json: settings() })
  })
  return backend
}

function sections(page: Page, name: string) {
  return page
    .getByRole('navigation', { name: 'Settings sections' })
    .getByRole('link', { name, exact: true })
}

test('administrators grant a group namespace access and remove a user', async ({ page }) => {
  const backend = await settingsBackend(page)
  await page.goto('/settings?section=access')
  await page.getByRole('button', { name: 'Add group', exact: true }).click()
  const editor = page.getByRole('dialog', { name: 'Add group' })
  await editor.getByRole('textbox', { name: /OIDC group/ }).fill('payments-oncall')
  await editor.getByRole('checkbox', { name: 'payments', exact: true }).check()
  await editor.getByRole('button', { name: 'Save access' }).click()
  await expect(editor).toBeHidden()
  await expect(page.getByRole('heading', { name: 'payments-oncall' })).toBeVisible()
  expect(backend.configuration.groups).toEqual([
    {
      name: 'payments-oncall',
      admin: false,
      scopes: [{ connectionId: 'prod-eu-1', namespaces: ['payments'] }],
    },
  ])

  const engineer = page.locator('.settings-row', { hasText: 'Engineer' })
  await engineer.getByRole('button', { name: 'Remove' }).click()
  const confirm = page.getByRole('dialog', { name: 'Remove Engineer?' })
  await confirm.getByRole('button', { name: 'Cancel' }).click()
  await expect(confirm).toBeHidden()
  expect(backend.configuration.users).toHaveLength(2)
  await engineer.getByRole('button', { name: 'Remove' }).click()
  await page
    .getByRole('dialog', { name: 'Remove Engineer?' })
    .getByRole('button', { name: 'Remove' })
    .click()
  await expect(page.getByRole('heading', { name: 'Engineer', exact: true })).toBeHidden()
  expect(backend.configuration.users.map((user) => user.subject)).toEqual(['local-demo'])
})

test('administrators declare a relationship from observed deployments', async ({ page }) => {
  const backend = await settingsBackend(page)
  await page.goto('/settings?section=relationships')
  await page.getByRole('button', { name: 'Add relationship', exact: true }).click()
  const editor = page.getByRole('dialog', { name: 'Add relationship' })
  await editor.getByRole('textbox', { name: 'Identifier' }).fill('payments-replica')
  await editor.getByRole('textbox', { name: /Snapshot repository/ }).fill('s3://backups')
  // Switching to replication drops the repository typed for snapshot recovery.
  await editor.getByRole('combobox', { name: 'Relationship' }).selectOption('replication')
  await expect(editor.getByRole('textbox', { name: /Snapshot repository/ })).toBeHidden()
  await editor
    .getByRole('combobox', { name: 'Source deployment' })
    .selectOption({ label: 'prod-eu-1 / payments/payments-bao' })
  await editor
    .getByRole('combobox', { name: 'Target deployment' })
    .selectOption({ label: 'prod-eu-1 / identity/identity-bao' })
  await editor.getByRole('button', { name: 'Save relationship' }).click()
  await expect(editor).toBeHidden()
  await expect(page.getByText('Remote read replica · payments-replica')).toBeVisible()
  expect(backend.configuration.relationships).toEqual([
    {
      id: 'payments-replica',
      kind: 'replication',
      repository: '',
      source: expect.objectContaining({ connectionId: 'prod-eu-1', name: 'payments-bao' }),
      target: expect.objectContaining({ connectionId: 'prod-eu-1', name: 'identity-bao' }),
    },
  ])
})

test('removing a namespace shows the access it removes before saving', async ({ page }) => {
  const backend = await settingsBackend(page)
  await page.goto('/settings?section=connections')
  await page.getByRole('button', { name: 'Edit', exact: true }).click()
  const editor = page.getByRole('dialog', { name: 'Edit Production EU 1' })
  await editor.getByRole('button', { name: 'Remove namespace 2' }).click()
  await editor.getByRole('button', { name: 'Save connection' }).click()
  const confirm = page.getByRole('dialog', { name: 'Remove access and relationships?' })
  await expect(confirm.getByRole('list', { name: 'Access removed' })).toContainText(
    'Engineer (user) loses prod-eu-1/identity',
  )
  expect(backend.revision).toBe(1)
  await confirm.getByRole('button', { name: 'Save and remove' }).click()
  await expect(confirm).toBeHidden()
  await expect(editor).toBeHidden()
  expect(backend.configuration.users[1]?.scopes).toEqual([
    { connectionId: 'prod-eu-1', namespaces: ['payments'] },
  ])
})

test('a concurrent change is reported and the settings can be reloaded', async ({ page }) => {
  const backend = await settingsBackend(page)
  await page.goto('/settings?section=policies')
  backend.conflict = true
  await page.getByRole('spinbutton', { name: 'Maximum backup age (hours)' }).fill('12')
  await page.getByRole('button', { name: 'Save policies' }).click()
  await expect(page.getByRole('alert')).toContainText('Settings changed since they were loaded')
  expect(backend.configuration.maxBackupAgeHours).toBe(24)
  await page.getByRole('button', { name: 'Reload settings' }).first().click()
  await sections(page, 'Policies').click()
  await page.getByRole('spinbutton', { name: 'Maximum backup age (hours)' }).fill('12')
  await page.getByRole('button', { name: 'Save policies' }).click()
  await expect(page.getByRole('status')).toHaveText('Saved.')
  expect(backend.configuration.maxBackupAgeHours).toBe(12)
})

test('an empty policy value is rejected instead of saved', async ({ page }) => {
  const backend = await settingsBackend(page)
  await page.goto('/settings?section=policies')
  const hours = page.getByRole('spinbutton', { name: 'Maximum backup age (hours)' })
  await hours.fill('')
  await expect(hours).toHaveValue('')
  await page.getByRole('button', { name: 'Save policies' }).click()
  expect(await hours.evaluate((input: HTMLInputElement) => input.validity.valueMissing)).toBe(true)
  expect(backend.revision).toBe(1)
})

test('a candidate identity provider is verified through a provider login', async ({ page }) => {
  await settingsBackend(page)
  let trial: unknown = null
  await page.route('**/api/v1/settings/identity/trial', async (route) => {
    trial = route.request().postDataJSON()
    await route.fulfill({
      status: 200,
      json: { url: 'https://identity.example/authorize?state=fixture' },
    })
  })
  await page.route('https://identity.example/**', (route) =>
    route.fulfill({ status: 200, contentType: 'text/html', body: '<h1>Fixture provider</h1>' }),
  )
  await page.goto('/settings?section=identity')
  await page
    .getByRole('textbox', { name: 'Issuer', exact: true })
    .fill('https://identity.example/realms/new')
  await page.getByLabel('Client secret').fill('fixture-client-secret')
  await page.getByRole('button', { name: 'Review access changes' }).click()
  const decision = page.getByRole('dialog', { name: 'Choose access for the new issuer' })
  await decision.getByRole('radio', { name: /Remove existing access/ }).check()
  await decision.getByRole('button', { name: 'Verify login and activate' }).click()
  await expect(page.getByRole('heading', { name: 'Fixture provider' })).toBeVisible()
  expect(
    z
      .object({
        revision: z.literal(1),
        accessRules: z.literal('remove'),
        identity: z.object({ issuer: z.string(), clientSecret: z.string() }),
      })
      .parse(trial).identity,
  ).toEqual(
    expect.objectContaining({
      issuer: 'https://identity.example/realms/new',
      clientSecret: 'fixture-client-secret',
    }),
  )
})

test('a failed trial keeps the identity form and reports the reason', async ({ page }) => {
  await settingsBackend(page)
  await page.route('**/api/v1/settings/identity/trial', (route) =>
    route.fulfill(problem(400, 'invalid', 'Identity discovery failed. Check the issuer.')),
  )
  await page.goto('/settings?section=identity')
  await page.getByRole('button', { name: 'Verify login and activate' }).click()
  await expect(page.getByRole('alert')).toHaveText('Identity discovery failed. Check the issuer.')
  await expect(page.getByRole('button', { name: 'Verify login and activate' })).toBeEnabled()
})
