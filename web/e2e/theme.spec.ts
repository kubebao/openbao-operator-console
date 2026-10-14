import { expect, test } from '@playwright/test'
import type { Page } from '@playwright/test'

// recordFirstPaintTheme records the theme attribute when the parser creates the body. No
// content can paint earlier, and the application has not started yet.
async function recordFirstPaintTheme(page: Page) {
  await page.addInitScript(() => {
    new MutationObserver((_, observer) => {
      if (!document.querySelector('body')) return
      observer.disconnect()
      Reflect.set(window, 'firstPaintTheme', document.documentElement.dataset['theme'] ?? 'none')
    }).observe(document, { childList: true, subtree: true })
  })
}

async function firstPaintTheme(page: Page): Promise<unknown> {
  return page.evaluate((): unknown => Reflect.get(window, 'firstPaintTheme'))
}

test('the system theme applies before the first paint', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'dark' })
  await recordFirstPaintTheme(page)
  await page.goto('/')
  expect(await firstPaintTheme(page)).toBe('dark')
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
})

test('a stored theme overrides the system theme before the first paint', async ({ page }) => {
  await recordFirstPaintTheme(page)
  for (const [system, stored] of [
    ['light', 'dark'],
    ['dark', 'light'],
  ] as const) {
    await page.emulateMedia({ colorScheme: system })
    await page.goto('/')
    await page.evaluate((theme) => {
      localStorage.setItem('console-theme', theme)
    }, stored)
    await page.reload()
    expect(await firstPaintTheme(page)).toBe(stored)
    await expect(page.locator('html')).toHaveAttribute('data-theme', stored)
  }
})
