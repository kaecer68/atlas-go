import { defineConfig, devices } from '@playwright/test';

/**
 * Backend-penetration e2e config for client_web.
 *
 * Unlike the default playwright.config.ts (which serves static dist with a
 * mockable Python HTTP server), this config starts the real atlas-go server
 * and verifies that backend APIs actually drive the frontend UI.
 */
export default defineConfig({
  testDir: './tests',
  timeout: 60000,
  retries: 1,
  webServer: {
    command: 'cd .. && go run ./cmd/atlas -api',
    port: 18080,
    reuseExistingServer: true,
    timeout: 180 * 1000,
  },
  use: {
    baseURL: 'http://localhost:18080',
    headless: true,
  },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
  ],
});
