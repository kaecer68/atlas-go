/**
 * Helper: installAuthMocks
 *
 * The client_web SPA fires `/api/user/profile` during `initAuth()` (called
 * unconditionally on every page load) and `isLoggedIn()` uses non-silent
 * `getJSON()` which throws on 404. Without a successful response, the entire
 * init chain stalls — `loadAll()` never runs, `loadModules()` never imports
 * `pages/home.js`, and the page stays empty.
 *
 * Tests that don't actually exercise auth flows should call this helper
 * before `page.goto()` to keep the SPA's init chain moving.
 */
import type { Page, BrowserContext } from '@playwright/test';

const AUTH_MOCKS: Array<[string, unknown]> = [
  ['**/api/user/profile', {}],
  ['**/api/auth/whoami', {}],
];

export async function installAuthMocks(target: Page | BrowserContext): Promise<void> {
  for (const [pattern, body] of AUTH_MOCKS) {
    await target.route(pattern, r => r.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(body),
    }));
  }
}
