import { expect, test } from '@playwright/test'
import type { Page } from '@playwright/test'
import { administrator } from './fixtures/administrator'

// Only the compiled Go application sends the content security policy. Run these checks
// with make e2e-production.
test.skip(
  process.env['CONSOLE_E2E_PRODUCTION'] !== '1',
  'The development server does not send the content security policy.',
)

const violationsKey = 'csp-violations'

// recordViolations collects securitypolicyviolation events and browser console reports.
// The events survive full navigations in session storage.
async function recordViolations(page: Page): Promise<() => Promise<string[]>> {
  const reported: string[] = []
  page.on('console', (message) => {
    if (message.text().includes('Content Security Policy')) reported.push(message.text())
  })
  await page.addInitScript((key) => {
    document.addEventListener('securitypolicyviolation', (event) => {
      const recorded = sessionStorage.getItem(key) ?? ''
      sessionStorage.setItem(key, `${recorded}${event.effectiveDirective} ${event.blockedURI}\n`)
    })
  }, violationsKey)
  return async () => {
    const recorded = await page.evaluate((key) => sessionStorage.getItem(key) ?? '', violationsKey)
    return [...recorded.split('\n').filter(Boolean), ...reported]
  }
}

test('the shell sends a fresh style nonce and no inline style allowance', async ({ page }) => {
  const nonces = new Set<string>()
  for (const path of ['/', '/fleet']) {
    const response = await page.goto(path)
    const policy = (await response?.allHeaders())?.['content-security-policy'] ?? ''
    expect(policy).not.toContain('unsafe-inline')
    const nonce = /style-src 'self' 'nonce-([A-Za-z0-9]+)'/.exec(policy)?.[1] ?? ''
    expect(nonce).not.toBe('')
    await expect(page.locator('meta[name="csp-nonce"]')).toHaveAttribute('content', nonce)
    nonces.add(nonce)
  }
  expect(nonces.size).toBe(2)
})

test('dialogs, menus, and charts render without policy violations', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' })
  const violations = await recordViolations(page)
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Attention', exact: true })).toBeVisible()
  await page.keyboard.press('ControlOrMeta+k')
  await page.getByRole('combobox', { name: 'Search your fleet' }).fill('payments')
  await expect(page.getByRole('option', { name: /payments-bao/ })).toBeVisible()
  // The open dialog locks scrolling with an injected style element that carries the nonce.
  const injected = await page.evaluate(() =>
    [...document.head.querySelectorAll('style')].map((style) => style.nonce),
  )
  expect(injected.length).toBeGreaterThan(0)
  expect(injected).not.toContain('')
  await page.keyboard.press('Escape')
  await page.goto('/fleet')
  await page.getByRole('combobox', { name: 'Filter by state' }).selectOption('critical')
  await page.goto('/clusters')
  await page.locator('.cluster-toggle').first().click()
  await page.goto('/deployments/payments?tab=history')
  await expect(page.getByRole('heading', { name: 'Recorded changes' })).toBeVisible()
  await page.goto('/topology')
  await page.getByRole('button', { name: /Inspect.*payments\/payments-bao/ }).click()
  await administrator(page)
  await page.goto('/settings?section=access')
  await page.getByRole('button', { name: 'Add user', exact: true }).click()
  await expect(page.getByRole('dialog', { name: 'Add user' })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog')).toHaveCount(0)
  expect(await violations()).toEqual([])

  // The recorder must see a style element without the nonce and an inline style attribute.
  await page.evaluate(() => {
    const style = document.createElement('style')
    style.textContent = 'body { outline: 1px solid red; }'
    document.head.append(style)
    const element = document.createElement('div')
    element.setAttribute('style', 'color: red')
    document.body.append(element)
  })
  await expect.poll(async () => (await violations()).join('\n')).toContain('style-src-elem')
  await expect.poll(async () => (await violations()).join('\n')).toContain('style-src-attr')
})
