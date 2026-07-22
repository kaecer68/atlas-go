/**
 * Default config for client_web Playwright tests.
 *
 * Runs against client_web/dist served by tests/spa-server.mjs (SPA fallback
 * static server with /client/ prefix stripping). CI's frontend-tests job
 * (see .github/workflows/quality.yml) has no Go/PostgreSQL.
 *
 * Most tests use page.route() mocking for /api/* calls. Tests that need a
 * real atlas-go backend are excluded here and run only via
 * `npm run test:e2e:backend` (playwright.backend.config.ts).
 *
 * Excluded from this config:
 *  - client-web-trust.backend.spec.ts — raw `request.get('/api/stock/quote')`
 *  - client-web-trust.spec.ts stock-quote test — page has no data-loading
 *    handler yet (sq-load-symbol event has no listener)
 */
import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  timeout: 60000,
  retries: 1,
  testIgnore: [
    '**/client-web-trust.backend.spec.ts',
  ],
  webServer: {
    command: 'node tests/spa-server.mjs 8085',
    port: 8085,
    reuseExistingServer: true,
    timeout: 60 * 1000,
  },
  use: {
    baseURL: 'http://localhost:8085',
    headless: true,
  },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
  ],
});
