import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './e2e-storage',
  workers: 1,
  retries: 0,
  timeout: 90_000,
  reporter: 'list',
  use: {
    ...devices['Desktop Chrome'],
    baseURL: 'https://127.0.0.1:5178',
    ignoreHTTPSErrors: true,
    trace: 'retain-on-failure',
  },
})
