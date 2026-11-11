import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: Boolean(process.env['CI']),
  retries: 0,
  reporter: 'list',
  use: { baseURL: 'http://127.0.0.1:5177', trace: 'retain-on-failure' },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 1040 } },
    },
  ],
  webServer:
    process.env['CONSOLE_E2E_PRODUCTION'] === '1'
      ? [
          {
            command: './bin/console --mode=demo --listen=127.0.0.1:5177',
            cwd: '..',
            url: 'http://127.0.0.1:5177/healthz',
            reuseExistingServer: false,
          },
        ]
      : [
          {
            command:
              process.env['CONSOLE_E2E_PREBUILT'] === '1'
                ? './bin/console --mode=demo --listen=127.0.0.1:8097'
                : 'go run ./cmd/console --mode=demo --listen=127.0.0.1:8097',
            cwd: '..',
            url: 'http://127.0.0.1:8097/healthz',
            reuseExistingServer: false,
          },
          {
            command: 'pnpm dev --port 5177 --strictPort',
            url: 'http://127.0.0.1:5177',
            env: { CONSOLE_DEV_API_URL: 'http://127.0.0.1:8097' },
            reuseExistingServer: false,
          },
        ],
})
