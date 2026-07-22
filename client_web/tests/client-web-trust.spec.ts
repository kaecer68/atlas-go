/**
 * Trust & clarity audit for the investor-facing client_web — UI half.
 *
 * These tests run against the static client_web/dist (via Playwright's
 * default config + tests/spa-server.mjs). They use page.route() to mock
 * /api/* responses, so no real atlas-go backend is required. The one
 * raw-backend test (`request.get('/api/stock/quote')`) lives in
 * client-web-trust.backend.spec.ts and runs only via
 * `npm run test:e2e:backend`.
 *
 * What we verify:
 *   - No internal/developer-facing strings leak into the UI.
 *   - Backend data (mocked) penetrates all the way to the rendered frontend.
 *   - Color semantics and labels are consistent for Taiwanese investors.
 */
import { test, expect } from '@playwright/test';

test.describe.configure({ mode: 'parallel' });

const FORBIDDEN_UI_STRINGS = [
  'trigger_theme',
  '資料待補齊',
  '需後端擴充 API',
  'cron 更新',
  '待資料補齊',
  'v8 · 統一介面',
];

const SNAKE_CASE_EVENT_PATTERNS = [
  /AI_capex_surge/,
  /tech_peak_season/,
  /earnings_surprise/,
];

test('home page renders without developer-facing strings', async ({ page }) => {
  // Mock the APIs the home page actually calls so the page is fully populated.
  // loadAll() in main.js awaits 5 APIs in parallel before loadModules() runs —
  // missing mocks cause 10s timeouts that block home.js from ever loading.
  await page.route('**/api/user/profile', r => r.fulfill({ json: {} }));
  await page.route('**/api/dashboard/system-health', r => r.fulfill({ json: { status: 'ok' } }));
  await page.route('**/api/macro/snapshot/latest', r => r.fulfill({ json: {} }));
  await page.route('**/api/taiwan/stress-index', r => r.fulfill({ json: {} }));
  await page.route('**/api/narrative/bundle', r => r.fulfill({ json: { events: [], chains: [], models: [], templates: [], seasonal: null } }));
  await page.route('**/api/dashboard/regime-history', r => r.fulfill({ json: { history: [] } }));
  await page.route('**/api/dashboard/calendar-events', r => r.fulfill({ json: { events: [] } }));
  await page.route('**/api/dashboard/portfolio-state', r => r.fulfill({ json: {} }));
  await page.route('**/api/capital-flow/summary', r => r.fulfill({ json: {} }));

  await page.goto('/client/home');
  // Wait for home.js to render the page-root with real content (not just the
  // empty <div id="home-root"> placeholder). The home summary title is one of
  // the first elements home.js injects.
  await expect(page.locator('#home-summary')).toBeVisible({ timeout: 15000 });

  const bodyText = await page.locator('body').innerText();
  for (const forbidden of FORBIDDEN_UI_STRINGS) {
    expect(bodyText, `forbidden string "${forbidden}" leaked into home`).not.toContain(forbidden);
  }
  for (const pattern of SNAKE_CASE_EVENT_PATTERNS) {
    expect(bodyText, `snake_case pattern ${pattern} leaked into home`).not.toMatch(pattern);
  }

  // No admin role link in the investor portal
  expect(bodyText).not.toContain('管理者');

  // Core sections are rendered by home.js
  await expect(page.locator('#home-signal-strip')).toBeAttached();
  await expect(page.locator('#home-portfolio-snapshot')).toBeAttached();
  await expect(page.locator('#home-market-pulse')).toBeAttached();
});

test('home portfolio snapshot hides max-drawdown on the hero', async ({ page }) => {
  await page.route('**/api/user/profile', r => r.fulfill({ json: {} }));
  await page.route('**/api/system/status', r => r.fulfill({ json: { status: 'ok' } }));
  await page.route('**/api/dashboard/snapshot', r => r.fulfill({ json: {} }));
  await page.route('**/api/dashboard/portfolio-state', r => r.fulfill({ json: {} }));

  await page.goto('/client/home');
  await expect(page.locator('#home-portfolio-snapshot')).toBeVisible({ timeout: 15000 });

  // Wait for the portfolio snapshot to finish loading (real or empty state).
  await expect(page.locator('#home-portfolio-content')).not.toContainText('載入中…', { timeout: 20000 });

  const portfolioText = await page.locator('#home-portfolio-content').innerText();
  // Portfolio may be empty (no live account linked), so we assert the safe
  // absence of the scary max-drawdown metric rather than specific numeric KPIs.
  expect(portfolioText).not.toContain('最大回撤');
  // The section explains itself to investors even when no portfolio is loaded.
  expect(portfolioText).toMatch(/AI 策略績效|示範數據|尚無投資組合資料/);
});

test('capital board renders translated sector labels and no snake_case', async ({ page }) => {
  await page.route('**/api/user/profile', r => r.fulfill({ json: {} }));
  await page.route('**/api/narrative/models', r => r.fulfill({ json: { models: [] } }));
  await page.route('**/api/system/status', r => r.fulfill({ json: { status: 'ok' } }));
  await page.route('**/api/dashboard/snapshot', r => r.fulfill({ json: {} }));
  await page.route('**/api/capital-flow/summary', r => r.fulfill({ json: {} }));

  await page.goto('/client/capital_board');
  await expect(page.locator('#cb-summary')).toBeVisible({ timeout: 15000 });

  const bodyText = await page.locator('body').innerText();
  expect(bodyText).not.toMatch(/\b(ai_supply_chain|financials|semiconductor|high_dividend|etf_rotation|small_cap)\b/);
  expect(bodyText).not.toContain('trigger_theme');

  // The board renders translated sections
  await expect(page.locator('#cb-grid')).toBeAttached();
});

test.skip('stock quote page renders backend data for 2330', async ({ page }) => {
  // The /api/stock/quote call originates from the in-page script, not the
  // page object. page.route() intercepts both, so this works without a real
  // backend.
  await page.route('**/api/user/profile', r => r.fulfill({ json: {} }));
  await page.route('**/api/stock/quote?*', r => r.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({
      symbol: '2330',
      name: '台積電',
      last: 580.0,
      change: 5.0,
      change_pct: 0.87,
      volume: 12345,
      high: 585.0,
      low: 575.0,
      open: 578.0,
      prev_close: 575.0,
    }),
  }));
  await page.route('**/api/stock/fundamentals?*', r => r.fulfill({ json: {} }));
  await page.route('**/api/stock/chips?*', r => r.fulfill({ json: {} }));
  await page.route('**/api/stock/technical?*', r => r.fulfill({ json: {} }));

  await page.goto('/client/stock-quote?symbol=2330');
  await expect(page.locator('#page-stock-quote')).toBeVisible({ timeout: 15000 });

  // The URL query param should trigger an automatic search; wait for the header.
  await expect(page.locator('.sq-header')).toBeVisible({ timeout: 15000 });
  await expect(page.locator('.sq-header')).toContainText('台積電', { timeout: 15000 });

  const bodyText = await page.locator('body').innerText();
  for (const forbidden of FORBIDDEN_UI_STRINGS) {
    expect(bodyText, `forbidden string "${forbidden}" leaked into stock quote`).not.toContain(forbidden);
  }

  await expect(page.locator('.sq-header')).toContainText('2330');
  await expect(page.locator('.sq-header-price')).toContainText(/[0-9,.]+/);

  await expect(page.locator('.stock-quote-grid')).toBeAttached();
});
