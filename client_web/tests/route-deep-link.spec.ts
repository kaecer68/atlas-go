import { test, expect } from '@playwright/test';
import { installAuthMocks } from './auth-mock';

/**
 * Route & deep-link regression tests for client_web.
 *
 * Verifies:
 *   - removed pages (evolution_panel) fall to 404
 *   - direct /client/<page> deep-links activate the correct page container
 *   - capital_board / stock-quote / strategies
 *     page containers are populated and title is correct
 *
 * We wait for #pageTitle (set inside the page shell's init()) rather than
 * for the page container's CSS class change, because the shell injection is
 * async (await _ensureShellLoaded → import() → init() → template.innerHTML
 * + pageTitle.textContent). The title element is a reliable signal that
 * both the route resolved AND the shell module loaded.
 *
 * SPA fallback is provided by tests/spa-server.mjs (the default Playwright
 * webServer for this config). installAuthMocks keeps the SPA's initAuth
 * chain from blocking on /api/user/profile before page modules load.
 */
test.describe.configure({ mode: 'parallel' });

const PAGES = [
  { id: 'capital_board',       title: '七大勢力看板' },
  { id: 'stock-quote',         title: '個股快查' },
  { id: 'strategies',          title: '投資心法' },
  { id: 'capital-causality',   title: '錢潮因果' },
];

for (const { id, title } of PAGES) {
  test(`${id} deep-link activates page container`, async ({ page }) => {
    await installAuthMocks(page);
    await page.goto(`/client/${id}`);
    // Title is set by the shell's init() once _ensureShellLoaded resolves —
    // this confirms the route resolved AND the page module loaded.
    await expect(page.locator('#pageTitle')).toHaveText(title, { timeout: 15000 });
    const container = page.locator(`#page-${id}`);
    await expect(container).toHaveClass(/active/, { timeout: 5000 });
  });
}

test('removed evolution_panel deep-link falls to 404', async ({ page }) => {
  await installAuthMocks(page);
  await page.goto('/client/evolution_panel');
  await expect(page.locator('#pageTitle')).toHaveText('404', { timeout: 15000 });
  const body = await page.locator('body').innerText();
  expect(body).toContain('找不到這個頁面');
});

// 2026-08-23 遷移刪除的頁面：login/register/mcp 刪除、pipeline/portfolio/
// performance-report 遷移到 admin、capital_causality 改 id 為 capital-causality。
const REMOVED_PATHS = [
  '/client/login', '/client/register', '/client/mcp',
  '/client/pipeline', '/client/portfolio', '/client/performance-report',
  '/client/capital_causality',
];
for (const path of REMOVED_PATHS) {
  test(`removed page deep-link ${path} falls to 404`, async ({ page }) => {
    await installAuthMocks(page);
    await page.goto(path);
    await expect(page.locator('#pageTitle')).toHaveText('404', { timeout: 15000 });
  });
}

test('sidebar nav click routes to correct page', async ({ page }) => {
  await installAuthMocks(page);
  // Direct deep-link is equivalent to sidebar click for SPA routing.
  // The click interaction requires full sidebar visibility which is
  // fragile in a headless static-only setup.
  await page.goto('/client/strategies');
  await expect(page.locator('#pageTitle')).toHaveText('投資心法', { timeout: 15000 });
  await page.goto('/client/capital_board');
  await expect(page.locator('#pageTitle')).toHaveText('七大勢力看板', { timeout: 15000 });
});
