import { expect, test } from '@playwright/test'
import { diagnosticGaps } from './fixtures/diagnostic-gaps'

test('resource acknowledgement and incomplete diagnostic causes remain separate', async ({
  page,
}) => {
  await diagnosticGaps(page)
  await page.goto('/deployments/customer-x')
  const lifecycle = page.getByRole('button', { name: /^Lifecycle.*Open Lifecycle$/ })
  await expect(lifecycle).toContainText('Upgrade state unknown')
  await expect(lifecycle).toContainText('Operator at generation 3 of 4')
  await expect(page.getByRole('button', { name: /^Availability.*Open Conditions$/ })).toContainText(
    'Seal state not current',
  )
  await page.getByText('View incomplete checks (2)', { exact: true }).click()
  await expect(page.getByRole('listitem').filter({ hasText: 'Recovery evidence:' })).toContainText(
    'Read access denied. Check the enrolled reader permissions.',
  )
  await expect(page.getByRole('listitem').filter({ hasText: 'Upgrade evidence:' })).toContainText(
    'Operator acknowledgement does not match the resource generation.',
  )
  await page.getByRole('tab', { name: /^Lifecycle/ }).click()
  await expect(page.getByText('Generation 3 of 4', { exact: true })).toBeVisible()
})

test('a current resource acknowledgement cannot make stale conditions current', async ({
  page,
}) => {
  await diagnosticGaps(page, 4)
  await page.goto('/deployments/customer-x')
  const lifecycle = page.getByRole('button', { name: /^Lifecycle.*Open Lifecycle$/ })
  await expect(lifecycle).toContainText('Resource generation acknowledged')
  await expect(lifecycle).toContainText('Upgrade state unknown')
  await expect(page.getByRole('button', { name: /^Availability.*Open Conditions$/ })).toContainText(
    'Seal state not current',
  )
  await page.getByRole('tab', { name: /^Conditions/ }).click()
  await expect(page.locator('.condition').filter({ hasText: 'Upgrading' })).toContainText(
    'Not current',
  )
})

test('missing resource acknowledgement is not filled from current conditions', async ({ page }) => {
  await diagnosticGaps(page, null)
  await page.goto('/deployments/customer-x?tab=lifecycle')
  await expect(page.getByText('Resource acknowledgement unknown', { exact: true })).toBeVisible()
  await expect(page.getByText('Generation 4 (current)', { exact: true })).toHaveCount(0)
})
