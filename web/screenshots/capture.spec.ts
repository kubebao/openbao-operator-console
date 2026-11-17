import type { Page } from '@playwright/test'
import { snapshotSchema } from '../src/api/schema'
import { expect, test } from '@playwright/test'
import { administrator } from '../e2e/fixtures/administrator'
import { diagnosticGaps } from '../e2e/fixtures/diagnostic-gaps'
import { conditionFindings } from '../e2e/fixtures/condition-findings'

const output = '../docs/images'

test('operator condition findings documentation screenshot', async ({ page }) => {
  await conditionFindings(page)
  await page.goto('/deployments/customer-x?tab=conditions')
  await expect(page.locator('.condition').filter({ hasText: 'TLSReady' })).toHaveClass(
    /finding-high/,
  )
  await capture(page, 'operator-conditions')
})

test('API group enrollment documentation screenshot', async ({ page }) => {
  // Fictional administrator enrollment; the API fixture does not contact Kubernetes.
  await administrator(page)
  await page.goto('/settings/clusters/new')
  await page.getByRole('textbox', { name: 'Cluster name', exact: true }).fill('Example cluster')
  await expect(page.getByRole('combobox', { name: /^Operator API group/ })).toHaveValue(
    'kubebao.org',
  )
  await page.evaluate(() => document.fonts.ready)
  await page.screenshot({ path: `${output}/api-group-enrollment.png` })
})

async function capture(
  page: Page,
  name: string,
  theme: 'light' | 'dark' = 'light',
  expandGaps = false,
) {
  await page.evaluate((value) => {
    localStorage.setItem('console-theme', value)
  }, theme)
  await page.reload()
  await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
  await expect(page.getByRole('main')).not.toContainText('Loading')
  if (expandGaps) {
    await page.getByText(/^View incomplete checks/).click()
    await expect(page.locator('details[open]')).toBeVisible()
  }
  await page.screenshot({ path: `${output}/${name}.png` })
}

test('documentation screenshots', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Attention', exact: true })).toBeVisible()
  await capture(page, 'attention')
  await capture(page, 'attention-dark', 'dark')

  await page.goto('/fleet')
  await expect(page.getByText('Showing 12 of 12 deployments')).toBeVisible()
  await capture(page, 'fleet')

  await page.goto('/deployments/identity')
  await expect(page.getByRole('heading', { name: 'identity-bao', exact: true })).toBeVisible()
  await capture(page, 'deployment')

  await page.goto('/deployments/customer-x?tab=backups')
  await expect(page.getByRole('heading', { name: 'Backup activity, last 7 days' })).toBeVisible()
  await capture(page, 'backup-evidence')

  await page.goto('/deployments/identity?tab=history')
  await expect(page.getByRole('heading', { name: 'Recorded changes' })).toBeVisible()
  await capture(page, 'history')

  await page.goto('/topology')
  await expect(page.locator('.map-edge')).toHaveCount(2)
  await capture(page, 'topology')

  await page.goto('/topology/clusters/prod-eu-1')
  await expect(page.getByRole('heading', { name: 'prod-eu-1', exact: true })).toBeVisible()
  await capture(page, 'clusters')

  await page.goto('/topology/relationships/payments-recovery')
  await expect(page.getByText('Snapshot repository', { exact: true })).toBeVisible()
  await capture(page, 'recovery-path')
})

test('incomplete analysis documentation screenshot', async ({ page }) => {
  await page.route('**/api/v1/snapshot', async (route) => {
    const response = await route.fetch()
    const snapshot = snapshotSchema.parse(await response.json())
    const deployments = snapshot.deployments.map((deployment) =>
      deployment.id === 'customer-x' && deployment.backup
        ? {
            ...deployment,
            evidence: {
              ...deployment.evidence,
              analysisIncomplete: true,
              analysisGaps: [
                {
                  rule: 'backup-failed',
                  reasons: ['missing_field' as const],
                  collectionOutcome: 'complete' as const,
                  checkedAt: snapshot.generatedAt,
                },
              ],
            },
            backup: { ...deployment.backup, consecutiveFailures: null },
          }
        : deployment,
    )
    await route.fulfill({ response, json: { ...snapshot, deployments } })
  })
  await page.goto('/deployments/customer-x?tab=backups')
  await expect(
    page.getByText('Some diagnostic rules could not be evaluated.', { exact: false }),
  ).toBeVisible()
  await capture(page, 'analysis-incomplete', 'light', true)
})

test('generation evidence documentation screenshot', async ({ page }) => {
  await diagnosticGaps(page)
  await page.goto('/deployments/customer-x?tab=lifecycle')
  await expect(page.getByText('Generation 3 of 4', { exact: true })).toBeVisible()
  await capture(page, 'generation-evidence')
})
