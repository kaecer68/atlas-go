import { test, expect } from '@playwright/test';
import { skipIfAtlasOffline } from '../../tests-shared/atlas-check';

/**
 * Trust & clarity audit for the investor-facing client_web.
 *
 * These tests run against the real atlas-go backend (via playwright.backend.config.ts)
 * to verify that:
 *   - No internal/developer-facing strings leak into the UI.
 *   - Backend data penetrates all the way to the rendered frontend.
 *   - Color semantics and labels are consistent for Taiwanese investors.
 */

test.beforeAll(async () => { await skipIfAtlasOffline(test); });
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

test('backend API /api/stock/quote penetrates to frontend', async ({ request }) => {
  const res = await request.get('/api/stock/quote?symbol=2330');
  expect(res.status()).toBe(200);
  const body = await res.json();
  expect(body).toHaveProperty('symbol', '2330');
  expect(body).toHaveProperty('last');
});

test('home page renders without developer-facing strings', async ({ page }) => {
  await page.goto('/client/home');
  await expect(page.locator('#page-home')).toBeVisible({ timeout: 15000 });

  const bodyText = await page.locator('body').innerText();
  for (const forbidden of FORBIDDEN_UI_STRINGS) {
    expect(bodyText).not.toContain(forbidden);
  }
  for (const pattern of SNAKE_CASE_EVENT_PATTERNS) {
    expect(bodyText).not.toMatch(pattern);
  }

  // No admin role link in the investor portal
  expect(bodyText).not.toContain('管理者');

  // Key backend-driven sections are visible
  // Hero starts with a loading placeholder; wait for it to resolve into a
  // concrete recommendation driven by the real backend.
  await expect(page.locator('#home-summary')).not.toContainText('載入市場摘要…', { timeout: 20000 });
  const heroText = await page.locator('#home-hero').innerText();
  expect(heroText).toMatch(/偏多|偏空|觀望/);
  await expect(page.locator('#home-signal-strip')).toBeVisible();
  await expect(page.locator('#home-portfolio-snapshot')).toBeVisible();
  await expect(page.locator('#home-market-pulse')).toBeVisible();
});

test('home portfolio snapshot hides max-drawdown on the hero', async ({ page }) => {
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
  await page.goto('/client/capital_board');
  await expect(page.locator('#page-capital_board')).toBeVisible({ timeout: 15000 });

  const bodyText = await page.locator('body').innerText();
  expect(bodyText).not.toMatch(/\b(ai_supply_chain|financials|semiconductor|high_dividend|etf_rotation|small_cap)\b/);
  expect(bodyText).not.toContain('trigger_theme');

  // Backend-driven sections render
  await expect(page.locator('#cb-summary')).toContainText('板塊方向彙總');
  await expect(page.locator('#cb-grid')).toContainText('模型權重');
  await expect(page.locator('#cb-grid')).toContainText('板塊看好');
});

test('stock quote page renders real backend data for 2330', async ({ page }) => {
  await page.goto('/client/stock-quote?symbol=2330');
  await expect(page.locator('#page-stock-quote')).toBeVisible({ timeout: 15000 });

  // The URL query param should trigger an automatic search; wait for the header.
  await expect(page.locator('.sq-header')).toBeVisible({ timeout: 15000 });
  await expect(page.locator('.sq-header')).toContainText('台積電', { timeout: 15000 });

  const bodyText = await page.locator('body').innerText();
  for (const forbidden of FORBIDDEN_UI_STRINGS) {
    expect(bodyText).not.toContain(forbidden);
  }

  await expect(page.locator('.sq-header')).toContainText('2330');
  await expect(page.locator('.sq-header-price')).toContainText(/[0-9,.]+/);

  await expect(page.locator('.stock-quote-grid')).toContainText('基本面');
  await expect(page.locator('.stock-quote-grid')).toContainText('籌碼');
  await expect(page.locator('.stock-quote-grid')).toContainText('技術指標');

  // No developer-facing placeholder in the technical section
  await expect(page.locator('.stock-quote-grid')).not.toContainText('需後端擴充 API');
});
