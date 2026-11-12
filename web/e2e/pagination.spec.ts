import { expect, test } from '@playwright/test'
import { activityFixture } from '../src/test/activity-fixtures'
import { deploymentFixture, snapshotFixture } from '../src/test/fixtures'
import { problem } from './fixtures/problem'
import type { ChartHistory } from '../src/api/schema'

// These browser fixtures exercise paging and failed reads, not live fleet qualification.

test('backup charts include older completions without loading observation pages', async ({
  page,
}) => {
  const now = Date.now()
  const at = (hours: number) => new Date(now - hours * 3_600_000).toISOString()
  const backup = {
    lastSuccessAt: at(1),
    nextAt: at(-1),
    key: 'fixture-snapshot',
    digest: null,
    sizeBytes: 1024,
    consecutiveFailures: 0,
    restoreTest: null,
  }
  const current = deploymentFixture({ backup })
  await page.route('**/api/v1/snapshot', (route) =>
    route.fulfill({ json: snapshotFixture({ generatedAt: at(0), deployments: [current] }) }),
  )
  let historyReads = 0
  await page.route('**/api/v1/deployments/fixture/history*', (route) => {
    historyReads++
    expect(new URL(route.request().url()).searchParams.has('cursor')).toBe(false)
    return route.fulfill({
      json: {
        observations: [{ recordedAt: at(1), deployment: current }],
        baseline: null,
        nextCursor: 'older-page',
      },
    })
  })
  const charts: ChartHistory = {
    start: at(168),
    end: at(0),
    baseline: null,
    observations: [
      { recordedAt: at(1), deployment: current },
      {
        recordedAt: at(120),
        deployment: deploymentFixture({ backup: { ...backup, lastSuccessAt: at(120) } }),
      },
    ],
  }
  await page.route('**/api/v1/deployments/fixture/charts', (route) =>
    route.fulfill({ json: charts }),
  )
  await page.goto('/deployments/fixture?tab=backups')
  await expect(page.getByText('Completed (2)', { exact: true })).toBeVisible()
  await expect(page.getByText('unchanged across 2 backups', { exact: false })).toBeVisible()
  await expect(page.getByText('Older observations are available.', { exact: false })).toHaveCount(0)
  expect(historyReads).toBe(1)
})

test('chart failures keep the observation list and can be retried', async ({ page }) => {
  const now = Date.now()
  const at = (seconds: number) => new Date(now - seconds * 1000).toISOString()
  let failed = true
  await page.route('**/api/v1/snapshot', (route) =>
    route.fulfill({ json: snapshotFixture({ generatedAt: at(0) }) }),
  )
  await page.route('**/api/v1/deployments/fixture/history*', (route) =>
    route.fulfill({
      json: {
        observations: [{ recordedAt: at(60), deployment: deploymentFixture() }],
        baseline: null,
        nextCursor: 'more',
      },
    }),
  )
  await page.route('**/api/v1/deployments/fixture/charts', (route) =>
    failed
      ? route.fulfill(problem(503, 'unavailable', 'Chart evidence unavailable.'))
      : route.fulfill({
          json: {
            start: at(7 * 86400),
            end: at(0),
            baseline: { recordedAt: at(8 * 86400), deployment: deploymentFixture() },
            observations: [
              { recordedAt: at(3600), deployment: deploymentFixture({ health: 'critical' }) },
              { recordedAt: at(3690), deployment: deploymentFixture({ health: 'attention' }) },
            ],
          },
        }),
  )
  await page.goto('/deployments/fixture?tab=history')
  await expect(page.getByText('Chart evidence is unavailable.', { exact: false })).toBeVisible({
    timeout: 10000,
  })
  await expect(page.locator('.timeline-entry')).toHaveCount(1)
  failed = false
  await page.getByRole('button', { name: 'Retry charts', exact: true }).click()
  const marker = page.locator('.state-incident.band-attention')
  await expect(marker).toHaveCount(1)
  const attention = await marker.boundingBox()
  const critical = await page.locator('.state-incident.band-critical').boundingBox()
  expect(attention?.width).toBeGreaterThanOrEqual(3)
  expect((attention?.y ?? 0) + (attention?.height ?? 0)).toBeLessThanOrEqual(critical?.y ?? 0)
  await expect(page.getByText('2 recorded incidents', { exact: false })).toBeVisible()
  await page.getByText('2 recorded incidents', { exact: false }).click()
  await expect(page.locator('.chart-incidents li')).toHaveCount(2)
  await expect(page.locator('.timeline-entry')).toHaveCount(1)
})

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

test('complete charts stay unchanged while observations load older pages', async ({ page }) => {
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
  let chartReads = 0
  await page.route('**/api/v1/deployments/fixture/charts', (route) => {
    chartReads++
    return route.fulfill({
      json: {
        start: at(168),
        end: at(0),
        baseline: { recordedAt: at(192), deployment: deploymentFixture() },
        observations: [
          { recordedAt: at(1), deployment: current },
          { recordedAt: at(120), deployment: deploymentFixture({ health: 'attention' }) },
        ],
      },
    })
  })
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
  await expect(page.locator('.state-band-track .band-unloaded')).toHaveCount(0)
  await expect(page.locator('.state-segment.band-attention')).toHaveCount(1)
  const bandBefore = await page.locator('.state-band-track').getAttribute('aria-label')
  await expect(page.getByText('Earlier observations not loaded', { exact: true })).toBeVisible()
  await expect(page.getByText('Console fixture-version', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Load older observations', exact: true }).click()
  await expect(page.locator('.state-band-track .band-unloaded')).toHaveCount(0)
  await expect(page.locator('.state-band-track')).toHaveAttribute('aria-label', bandBefore ?? '')
  expect(chartReads).toBe(1)
  await expect(page.getByText('State Healthy → Attention', { exact: true })).toBeVisible()
  await expect(page.getByText('State Attention → Critical', { exact: true })).toBeVisible()
  await expect(page.getByText('Earliest retained observation', { exact: true })).toHaveCount(0)
  await expect(
    page.getByRole('button', { name: 'Load older observations', exact: true }),
  ).toHaveCount(0)
})
