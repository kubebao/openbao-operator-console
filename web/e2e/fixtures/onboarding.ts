import { expect } from '@playwright/test'
import type { Page } from '@playwright/test'

interface Enrollment {
  // local selects the in-cluster connection instead of an uploaded reader kubeconfig.
  local?: boolean
  automatic?: boolean
}

// toVerify walks the onboarding wizard to its verify step for Production EU.
export async function toVerify(page: Page, { local = true, automatic = false }: Enrollment = {}) {
  await page.goto('/settings/clusters/new')
  await page.getByRole('textbox', { name: 'Cluster name', exact: true }).fill('Production EU')
  if (local)
    await page.getByRole('combobox', { name: 'Connection', exact: true }).selectOption('local')
  await page.getByRole('button', { name: 'Continue', exact: true }).click()
  await page.getByRole('textbox', { name: 'Namespace 1', exact: true }).fill('team-a')
  if (automatic)
    await page.getByRole('checkbox', { name: 'Renew ServiceAccount tokens automatically' }).check()
  await page.getByRole('button', { name: 'Continue', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Download console-reader.yaml' })).toBeVisible()
  await expect(page.getByRole('region', { name: 'Apply commands' })).toContainText(
    'CONSOLE_CLUSTER_CONTEXT=',
  )
  if (automatic)
    await expect(
      page.getByRole('region', { name: 'Reader RBAC manifests', includeHidden: true }),
    ).toContainText(/serviceaccounts\/token[\s\S]*resourceNames: \[console-reader\]/)
  await page.getByRole('button', { name: 'Continue', exact: true }).click()
  if (!local)
    await page
      .getByRole('textbox', { name: /^Reader kubeconfig/ })
      .fill('fixture-reader-kubeconfig')
  await page.getByRole('button', { name: 'Continue', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Verify Production EU' })).toBeVisible()
}
