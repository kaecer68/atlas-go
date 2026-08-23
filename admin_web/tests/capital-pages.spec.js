// admin_web/tests/capital-pages.spec.js
// Stage 6 PR#2：錢潮方向三頁面 smoke test（mock API，不依賴 backend）。

import { test, expect } from '@playwright/test';

const MOCK_MODELS = {
  models: [
    {
      id: 'hawkish_fed_model',
      name: '鷹派聯準會模型',
      weight: 0.5,
      recent_error: 0.05,
      hit_rate: 0.65,
      last_signal: 0.12,
      rationale: '美國升息週期下，外資回流美國，台股資金面緊縮。',
      favored_sectors: ['financials', 'high_dividend'],
      avoided_sectors: ['ai_supply_chain'],
      active_themes: ['US_rates_up'],
    },
    {
      id: 'ai_supercycle_model',
      name: 'AI 超級週期模型',
      weight: 0.3,
      recent_error: 0.1,
      hit_rate: 0,
      recent_prediction: -0.05,
      rationale: 'AI 資本支出上修，台灣供應鏈受惠。',
      favored_sectors: ['ai_supply_chain', 'semiconductor'],
      avoided_sectors: ['consumer'],
      active_themes: ['AI_capex_surge'],
    },
    {
      id: 'earnings_surprise_model',
      name: '財報驚喜驅動',
      weight: 0.2,
      recent_error: 0.02,
      hit_rate: 0.5,
      last_signal: 0,
      rationale: '科技股財報超預期，法人調升目標價。',
      favored_sectors: ['semiconductor'],
      avoided_sectors: ['traditional'],
      active_themes: ['earnings_surprise'],
    },
  ],
};

const MOCK_TEMPLATES = {
  templates: Array.from({ length: 24 }, (_, i) => ({
    id: `template-${i}`,
    name: `模板 ${i + 1}`,
    trigger_theme: i % 2 === 0 ? 'AI_capex_surge' : 'US_rates_up',
    historical_hit_rate: 0.55 + (i % 10) / 100,
    rationale: `模板 ${i + 1} 的投資邏輯說明。`,
    required_region: 'TW',
    source_references: ['Source A', 'Source B'],
    steps: [
      { description: '步驟一說明', affected: ['半導體'] },
      { description: '步驟二說明', affected: ['AI供應鏈'] },
    ],
  })),
};

const MOCK_CHANNELS = {
  channels: [
    { channel_id: 'twse_capital_flow', platform: 'TWSE 三大法人', country: '台灣', status_text: '正常', updated_at: new Date().toISOString(), last_error: '' },
    { channel_id: 'finmind', platform: 'FinMind', country: '台灣', status_text: '異常', updated_at: new Date(Date.now() - 7 * 60 * 60 * 1000).toISOString(), last_error: 'finmind fetch: status 400' },
    { channel_id: 'us_yahoo', platform: 'Yahoo Finance', country: '美國', status_text: '異常', updated_at: '上次失敗: circuit breaker open', last_error: 'circuit breaker open for channel us_yahoo' },
  ],
  alerts: [],
};

const MOCK_ALERTS = {
  alerts: [
    { id: 'a1', severity: 'ERROR', message: 'Task channel_health_finmind failed', timestamp: new Date().toISOString(), rule: 'background_task' },
  ],
  total: 1,
};

async function setupMocks(page) {
  await page.route('**/api/dashboard/**', (route) => route.fulfill({ json: {} }));
  await page.route('**/api/taiwan/stress-index', (route) => route.fulfill({ json: {} }));
  await page.route('**/api/narrative/**', (route) => route.fulfill({ json: {} }));
  await page.route('**/api/synergy/**', (route) => route.fulfill({ json: {} }));
  await page.route('**/api/macro/snapshot/latest', (route) => route.fulfill({ json: {} }));
  await page.route('**/api/dashboard/sessions', (route) => route.fulfill({ json: { sessions: [], data_status: 'ok' } }));

  await page.route('/api/narrative/models', (route) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_MODELS) }));
  await page.route('/api/narrative/templates', (route) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_TEMPLATES) }));
  await page.route('/api/dashboard/data-channels', (route) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_CHANNELS) }));
  await page.route('/api/alerts', (route) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_ALERTS) }));
  await page.route('/api/health/aggregate', (route) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ tiers: {}, overall: { ok: true } }) }));
}

async function navigateTo(page, pageId) {
  await page.goto('/');
  await page.click('a[data-page="' + pageId + '"]');
}

test.describe('capital pages', () => {
  test('capital_models renders 3 model cards with weight bars and expandable details', async ({ page }) => {
    await setupMocks(page);
    await navigateTo(page, 'capital_models');

    await expect(page.locator('.cm-card')).toHaveCount(3);
    await expect(page.locator('.cm-card__bar-fill')).toHaveCount(3);
    await expect(page.locator('text=鷹派聯準會模型')).toBeVisible();

    // hit_rate 0/0 shows "no data"
    await expect(page.locator('#capitalModelsContent').getByText('無資料')).toBeVisible();

    // click first card expands details
    const firstCard = page.locator('.cm-card').first();
    await firstCard.click();
    await expect(firstCard.locator('.cm-card__detail.open')).toBeVisible();
    await expect(firstCard.getByText('看好板塊')).toBeVisible();

    // weight sum note
    await expect(page.locator('#capitalModelsContent').getByText('權重合計')).toBeVisible();
  });

  test('capital_quality renders channels with staleness colors and error details', async ({ page }) => {
    await setupMocks(page);
    await navigateTo(page, 'capital_quality');

    await expect(page.locator('.cq-row')).toHaveCount(3);

    // critical channel row colored
    await expect(page.locator('.cq-row.critical')).toHaveCount(2);

    // click error row shows error detail
    await page.locator('.cq-row.critical').first().click();
    await expect(page.locator('.cq-error.open')).toBeVisible();

    // critical alerts integrated
    await expect(page.locator('#capitalQualityContent').getByText('🔥 嚴重警報')).toBeVisible();
    await expect(page.locator('.cq-alert')).toHaveCount(1);
  });
});
