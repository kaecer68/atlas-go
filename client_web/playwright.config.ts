import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  timeout: 30000,
  retries: 1,
  webServer: {
    command: 'python3 -m http.server 8085 --directory dist',
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
