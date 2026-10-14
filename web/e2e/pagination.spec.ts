import { expect, test } from '@playwright/test'
import { activityFixture } from '../src/test/activity-fixtures'
import { deploymentFixture, snapshotFixture } from '../src/test/fixtures'
import { problem } from './fixtures/problem'

// These browser fixtures exercise paging and failed reads, not live fleet qualification.
test('activity retains loaded events after a failed older page and can retry', async ({ page }) => {
  let failOlder = true
  await page.route('**/api/v1/activity*', (route) => {
    const cursor = new URL(route.request().url()).searchParams.get('cursor')
    if (cursor && failOlder)
      return route.fulfill(problem(503, 'unavailable', 'Activity unavailable.'))
    return route.fulfill({
      json: cursor
        ? {
            events: [
              activityFixture({
                id: 'older',
                kind: 'collectionChanged',
                health: null,
                collection: { from: 'current', to: 'unreachable' },
              }),
            ],
            nextCursor: null,
          }
        : { events: [activityFixture({ id: 'newest' })], nextCursor: 'older-fixture-page' },
    })
  })
  await page.goto('/activity')
  await expect(page.getByText('State Healthy → Critical', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Load older activity', exact: true }).click()
  await expect(page.getByRole('status')).toContainText('Previously loaded records remain visible')
  await expect(page.getByText('State Healthy → Critical', { exact: true })).toBeVisible()
  failOlder = false
  await page.getByRole('button', { name: 'Load older activity', exact: true }).click()
  await expect(page.getByText('Collection Collected → Unreachable', { exact: true })).toBeVisible()
  await expect(page.locator('.activity-list > li')).toHaveCount(2)
  await expect(page.getByRole('button', { name: 'Load older activity', exact: true })).toHaveCount(
    0,
  )
})

test('history pages distinguish unloaded evidence and use a pre-window baseline', async ({
  page,
}) => {
  const now = Date.now()
  const at = (hours: number) => new Date(now - hours * 3_600_000).toISOString()
  const current = deploymentFixture({ health: 'critical' })
  let since: string | null = null
  await page.route('**/api/v1/snapshot', (route) =>
    route.fulfill({
      json: snapshotFixture({
        generatedAt: at(0),
        deployments: [current],
        build: { version: 'fixture-version', commit: 'fixture-source-revision' },
      }),
    }),
  )
  await page.route('**/api/v1/deployments/fixture/history*', (route) => {
    const query = new URL(route.request().url()).searchParams
    expect(query.get('since')).toBe(at(168))
    since ??= query.get('since')
    expect(query.get('since')).toBe(since)
    return route.fulfill({
      json: query.has('cursor')
        ? {
            observations: [
              { recordedAt: at(120), deployment: deploymentFixture({ health: 'attention' }) },
            ],
            baseline: null,
            nextCursor: null,
          }
        : {
            observations: [{ recordedAt: at(1), deployment: current }],
            baseline: { recordedAt: at(192), deployment: deploymentFixture() },
            nextCursor: 'older-fixture-observations',
          },
    })
  })
  await page.goto('/deployments/fixture?tab=history')
  await expect(page.locator('.state-band-track .band-unloaded')).toHaveCount(1)
  await expect(page.getByText('Earlier observations not loaded', { exact: true })).toBeVisible()
  await expect(page.getByText('Console fixture-version', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Load older observations', exact: true }).click()
  await expect(page.locator('.state-band-track .band-unloaded')).toHaveCount(0)
  await expect(page.getByText('State Healthy → Attention', { exact: true })).toBeVisible()
  await expect(page.getByText('State Attention → Critical', { exact: true })).toBeVisible()
  await expect(page.getByText('Earliest retained observation', { exact: true })).toHaveCount(0)
  await expect(
    page.getByRole('button', { name: 'Load older observations', exact: true }),
  ).toHaveCount(0)
})
