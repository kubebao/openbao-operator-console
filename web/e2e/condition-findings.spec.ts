import { expect, test } from '@playwright/test'
import { conditionFindings } from './fixtures/condition-findings'

test('operator findings open and highlight the matching conditions', async ({ page }) => {
  await conditionFindings(page)
  await page.goto('/')
  await page.getByRole('link', { name: 'Operator condition: TLSReady', exact: true }).click()
  await expect(page.getByRole('tab', { name: /^Conditions/ })).toHaveAttribute(
    'data-state',
    'active',
  )
  await expect(page.locator('.condition').filter({ hasText: 'TLSReady' })).toHaveClass(
    /finding-high/,
  )
  await expect(page.locator('.condition').filter({ hasText: 'Available' })).toHaveClass(
    /finding-high/,
  )
  await expect(page.locator('.condition').filter({ hasText: 'StorageConfigured' })).not.toHaveClass(
    /finding-high/,
  )
  await page.goto('/deployments/customer-x')
  await expect(page.getByRole('button', { name: /^Availability.*Open Conditions$/ })).toHaveClass(
    /tile-high/,
  )
})
