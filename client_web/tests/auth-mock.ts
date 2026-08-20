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

// GUEST_MODE=false（會員制）：profile 必須回傳有效 email+tier 才視為已登入，
// 否則 SPA 的登入 gate 會把所有功能頁導向 /login。此處模擬已登入會員，讓
// 功能頁 e2e 能正常渲染。
const AUTH_MOCKS: Array<[string, unknown]> = [
  ['**/api/user/profile', { email: 'test@atlas.test', tier: 'registered', effective_tier: 'registered' }],
  ['**/api/auth/whoami', { email: 'test@atlas.test', tier: 'registered' }],
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
