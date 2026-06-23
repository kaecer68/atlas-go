import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  webServer: {
    command: 'python3 -m http.server 8085 --directory static',
    port: 8085,
    reuseExistingServer: true,
  },
  use: {
    baseURL: 'http://localhost:8085',
  },
});
