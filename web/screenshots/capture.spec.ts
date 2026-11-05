import type { Page } from '@playwright/test'
import { expect, test } from '@playwright/test'

const output = '../docs/images'

async function capture(page: Page, name: string, theme: 'light' | 'dark' = 'light') {
  await page.evaluate((value) => {
    localStorage.setItem('console-theme', value)
  }, theme)
  await page.reload()
  await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
  await expect(page.getByRole('main')).not.toContainText('Loading')
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
  await page.getByRole('button', { name: /Inspect.*payments\/payments-bao/ }).click()
  await page.getByRole('button', { name: /Select.*recovery\/payments-dr/ }).click()
  await expect(page.getByText('Snapshot storage', { exact: true })).toBeVisible()
  await capture(page, 'topology')

  await page.goto('/clusters')
  await expect(page.getByRole('heading', { name: 'Clusters', exact: true })).toBeVisible()
  await capture(page, 'clusters')
})
