import { test, expect } from '@playwright/test';
import { installAuthMocks } from './auth-mock';


const CAPITAL_FLOW_SUMMARY_PAYLOAD = {
  date: '2026-07-20T00:00:00Z',
  quality_score: 0.75,
  quality_label: 'inflow',
  resonance_dir: 'bullish',
  dominant_force: 'foreign',
  forces: [
    { force: 'foreign', display_name: '外資', z_score: 1.2, trend: 'bullish', raw_value: 15.3 },
    { force: 'institutional', display_name: '投信', z_score: 0.9, trend: 'bullish', raw_value: 8.1 },
    { force: 'dealer', display_name: '自營商', z_score: -0.6, trend: 'bearish', raw_value: -2.4 },
    { force: 'retail', display_name: '散戶', z_score: -0.1, trend: 'neutral', raw_value: -0.5 },
  ],
};

const CAPITAL_FLOW_DAILY_PAYLOAD = {
  date: '2026-07-20T00:00:00Z',
  summary: '官方法人同步偏多，跨市場訊號中性。',
  forces: [
    { force: 'foreign', display_name: '外資', z_score: 1.2, trend: 'bullish', raw_value: 15.3 },
    { force: 'institutional', display_name: '投信', z_score: 0.9, trend: 'bullish', raw_value: 8.1 },
    { force: 'dealer', display_name: '自營商', z_score: -0.6, trend: 'bearish', raw_value: -2.4 },
    { force: 'retail', display_name: '散戶', z_score: -0.1, trend: 'neutral', raw_value: -0.5 },
  ],
};

test('capital board page renders capital-flow summary and force rows', async ({ page }) => {
  await installAuthMocks(page);
  await page.addInitScript(() => { localStorage.setItem('atlas-onboarded', '1'); });
  await page.route('**/api/capital-flow/summary', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(CAPITAL_FLOW_SUMMARY_PAYLOAD) }));
  await page.route('**/api/capital-flow/daily', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(CAPITAL_FLOW_DAILY_PAYLOAD) }));

  await page.route('**/api/dashboard/system-health', route => route.fulfill({ json: {} }));
  await page.route('**/api/macro/snapshot/latest', route => route.fulfill({ json: {} }));
  await page.route('**/api/taiwan/stress-index', route => route.fulfill({ json: { score: 50 } }));
  await page.route('**/api/narrative/bundle', route => route.fulfill({ json: {} }));
  await page.route('**/api/dashboard/retail-sentiment', route => route.fulfill({ json: {} }));
  await page.route('**/api/dashboard/regime-history', route => route.fulfill({ json: {} }));

  await page.goto('/');
  await page.click('a[data-page="capital_board"]');

  const summary = page.locator('#cb-summary');
  await expect(summary).toBeVisible({ timeout: 5000 });
  await expect(summary).toContainText('偏多勢力');
  await expect(summary).toContainText('偏空勢力');
  await expect(summary).toContainText('中性勢力');

  const chart = page.locator('#cb-chart');
  await expect(chart).toBeVisible();
  await expect(chart.locator('svg')).toBeVisible();

  const grid = page.locator('#cb-grid');
  await expect(grid).toBeVisible();
  await expect(grid).toContainText('外資');
  await expect(grid.locator('.cb-sector-row')).toHaveCount(4);
});
