import { defineConfig, devices } from '@playwright/test';

/**
 * Backend-penetration e2e config for client_web.
 *
 * Unlike the default playwright.config.ts (which serves client_web/dist via
 * tests/spa-server.mjs with page.route() mocking), this config starts the
 * real atlas-go server so backend APIs actually drive the frontend UI.
 *
 * Local dev only — CI's frontend-tests job does not provide Go or PostgreSQL.
 * Run with: `npm run test:e2e:backend` (in client_web/).
 *
 * Includes the raw-backend test in client-web-trust.backend.spec.ts; the rest
 * of client-web-trust.spec.ts (page-route mocks) is duplicated to verify the
 * UI half still passes against the real backend.
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
