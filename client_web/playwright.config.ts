import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  timeout: 30000,
  retries: 1,
  webServer: {
    command: 'node scripts/ci-spa-server.mjs',
    port: 8085,
    reuseExistingServer: true,
  },
  use: {
    baseURL: 'http://localhost:8085',
    headless: true,
  },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
  ],
});
