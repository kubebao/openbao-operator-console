import { defineConfig, devices } from '@playwright/test'

// Captures documentation screenshots from the compiled application in fixture mode.
// Run through `make screenshots`, which builds the binary first.
export default defineConfig({
  testDir: './screenshots',
  fullyParallel: false,
  retries: 0,
  reporter: 'list',
  use: {
    ...devices['Desktop Chrome'],
    baseURL: 'http://127.0.0.1:5178',
    viewport: { width: 1280, height: 800 },
    deviceScaleFactor: 2,
    reducedMotion: 'reduce',
  },
  webServer: {
    command: './bin/console --mode=demo --listen=127.0.0.1:5178',
    cwd: '..',
    url: 'http://127.0.0.1:5178/healthz',
    reuseExistingServer: false,
  },
})
