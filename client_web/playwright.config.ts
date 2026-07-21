/**
 * Default config for client_web Playwright tests.
 *
 * Uses the real atlas-go backend (playwright.backend.config.ts logic) so that
 * API calls like /api/stock/quote return JSON, not the SPA index.html fallback.
 *
 * For pure static-dist tests that should not require the backend, explicitly
 * override with: npx playwright test --config playwright.static.config.ts
 */
import { defineConfig, devices } from '@playwright/test';
import backendConfig from './playwright.backend.config.ts';

export default defineConfig({
  ...backendConfig,
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
