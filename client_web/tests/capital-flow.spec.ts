import { test, expect } from '@playwright/test';
import { installAuthMocks } from './auth-mock';


const PREDICTIONS_PAYLOAD = {
  predictions: [
    { date: '2026-07-16T00:00:00Z', direction: 'inflow', confidence: 0.88, driving_events: ['法說會旺季', '配息資金回流'], predicted_forces: ['foreign', 'institutional'] },
    { date: '2026-07-17T00:00:00Z', direction: 'inflow', confidence: 0.75, driving_events: ['法說會旺季'], predicted_forces: ['foreign'] },
    { date: '2026-07-18T00:00:00Z', direction: 'neutral', confidence: 0.45, driving_events: ['期貨結算日'], predicted_forces: ['dealer'] },
    { date: '2026-07-19T00:00:00Z', direction: 'outflow', confidence: 0.60, driving_events: ['除權息旺季'], predicted_forces: ['retail'] },
    { date: '2026-07-20T00:00:00Z', direction: 'inflow', confidence: 0.70, driving_events: ['法說會旺季', '配息資金回流'], predicted_forces: ['foreign', 'institutional'] },
  ],
  active_events: [
    { name: '法說會旺季', event_type: 'investor_conference', affected_industries: ['semiconductor', 'ai_supply_chain', 'electronics'], confidence: 0.6, direction: 'bullish' },
    { name: '配息資金回流', event_type: 'dividend_payout', affected_industries: ['financials', 'consumer'], confidence: 0.5, direction: 'bullish' },
    { name: '除權息旺季', event_type: 'ex_dividend', affected_industries: ['financials', 'consumer'], confidence: 0.7, direction: 'mixed' },
    { name: '期貨結算日', event_type: 'futures_settlement', affected_industries: ['financials', 'electronics'], confidence: 0.6, direction: 'bearish' },
  ],
  summary: '未來 5 天資金偏流入。',
  window: '5-day forward',
};

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

const MODELS_PAYLOAD = {
  models: [
    {
      id: 'hawkish_fed_model',
      name: '鷹派聯準會模型',
      weight: 0.5,
      hit_rate: 1,
      recent_error: 0,
      rationale: '偏好防禦型板塊。',
      favored_sectors: ['financials', 'high_dividend'],
      avoided_sectors: ['ai_supply_chain', 'small_cap'],
    },
    {
      id: 'ai_supercycle_model',
      name: 'AI 超級週期模型',
      weight: 0.3,
      hit_rate: 0,
      recent_error: 1,
      rationale: '偏好科技供應鏈。',
      favored_sectors: ['ai_supply_chain', 'semiconductor', 'pcb', 'thermal'],
      avoided_sectors: ['consumer', 'tourism'],
    },
    {
      id: 'earnings_surprise_model',
      name: '財報驚喜驅動',
      weight: 0.2,
      hit_rate: 0.5,
      recent_error: 0.5,
      rationale: '科技股財報超預期。',
      favored_sectors: ['semiconductor', 'ai_supply_chain'],
      avoided_sectors: ['traditional'],
    },
  ],
};

test('capital predictions page renders 5-day cards and sector heatmap', async ({ page }) => {
  await installAuthMocks(page);
  await page.addInitScript(() => { localStorage.setItem('atlas-onboarded', '1'); });
  await page.route('**/api/events/prediction', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(PREDICTIONS_PAYLOAD) }));
  await page.route('**/api/narrative/models', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MODELS_PAYLOAD) }));

  // Silence background dashboard calls so they don't pollute the test log.
  await page.route('**/api/dashboard/system-health', route => route.fulfill({ json: {} }));
  await page.route('**/api/macro/snapshot/latest', route => route.fulfill({ json: {} }));
  await page.route('**/api/taiwan/stress-index', route => route.fulfill({ json: { score: 50 } }));
  await page.route('**/api/narrative/bundle', route => route.fulfill({ json: {} }));
  await page.route('**/api/dashboard/retail-sentiment', route => route.fulfill({ json: {} }));
  await page.route('**/api/dashboard/regime-history', route => route.fulfill({ json: {} }));

  await page.goto('/');
  await page.click('a[data-page="capital_predictions"]');

  const predictions = page.locator('#cp-predictions');
  await expect(predictions).toBeVisible({ timeout: 5000 });
  await expect(predictions).toContainText('資金流入');
  await expect(predictions).toContainText('88%');
  await expect(predictions.locator('.cp-prediction')).toHaveCount(5);

  const heatmap = page.locator('#cp-heatmap');
  await expect(heatmap).toBeVisible({ timeout: 5000 });
  await expect(heatmap).toContainText('半導體');
  await expect(heatmap.locator('.cp-heatmap__row')).toHaveCount(5);
  // 5 sectors × 5 days, but only sectors touched by that day's driving events are active.
  await expect(heatmap.locator('.cp-heatmap__cell.is-active')).toHaveCount(17);

  // Click first day card to open detail panel.
  await predictions.locator('.cp-prediction').first().click();
  const detail = page.locator('#cp-detail');
  await expect(detail).toBeVisible();
  await expect(detail).toContainText('觸發原因');
});

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
