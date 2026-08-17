import { test, expect } from '@playwright/test';

async function mockCommonEndpoints(page) {
  await page.route('**/api/dashboard/**', r => r.fulfill({ json: {} }));
  await page.route('**/api/taiwan/stress-index', r => r.fulfill({ json: {} }));
  await page.route('**/api/narrative/**', r => r.fulfill({ json: {} }));
  await page.route('**/api/synergy/**', r => r.fulfill({ json: {} }));
  await page.route('**/api/alerts', r => r.fulfill({ json: {} }));
  await page.route('**/api/macro/snapshot/latest', r => r.fulfill({ json: {} }));
  await page.route('**/api/dashboard/sessions', r => r.fulfill({ json: { sessions: [], data_status: 'ok' } }));
}

const MOCK_MODELS = {
  models: [
    { id: 'm1', name: 'macro_flow', weight: 0.5, recent_error: 0.02, hit_rate: 0.72, last_signal: 'inflow', rationale: '外資動量' },
    { id: 'm2', name: 'retail_sentiment', weight: 0.3, recent_error: 0.05, hit_rate: 0, last_signal: 'neutral', rationale: '散戶情緒' },
    { id: 'm3', name: 'event_tilt', weight: 0.2, recent_error: 0.01, hit_rate: 0.88, last_signal: 'outflow', rationale: '事件驅動' },
  ],
};

test('capital models page renders model cards with weights, hit rates and expandable details', async ({ page }) => {
  await mockCommonEndpoints(page);

  await page.route('**/api/narrative/models', r => r.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify(MOCK_MODELS),
  }));

  await page.goto('/');
  await page.waitForFunction(() => typeof window.switchPage === 'function');
  await page.evaluate(() => window.switchPage('capital_models'));

  const content = page.locator('#capitalModelsContent');
  await expect(content).toBeVisible({ timeout: 5000 });

  // 3 model cards
  await expect(content.locator('.cm-card')).toHaveCount(3);
  await expect(content.locator('.cm-card__bar-fill')).toHaveCount(3);

  // names
  await expect(content).toContainText('macro_flow');
  await expect(content).toContainText('retail_sentiment');
  await expect(content).toContainText('event_tilt');

  // hit rates
  await expect(content).toContainText('72.0%');
  await expect(content).toContainText('88.0%');

  // hit_rate 0/0 shows "no data"
  await expect(content).toContainText('無資料');

  // rationale in expanded detail
  await content.locator('.cm-card').first().click();
  await expect(content.locator('.cm-card__detail.open')).toBeVisible();
  await expect(content).toContainText('外資動量');

  // weight sum note
  await expect(content).toContainText('權重合計');
  await expect(content).toContainText('100.0%');

  await page.screenshot({ path: 'test-results/capital-models/cards.png', fullPage: true });
});
// 非歸一權重測例：weights 合計 = 0.342 (非 1.0)，驗證卡片為「相對權重」
// (各模型權重 / 總權重，合計 100%)、合計行為「絕對權重」(原始權重合計)。
// 卡片與合計行必須有明確單位標註，且兩者口徑不同。
const MOCK_MODELS_NON_NORMALIZED = {
  models: [
    { id: 'n1', name: 'model_alpha', weight: 0.152, recent_error: 0.02, hit_rate: 0.7, rationale: '測試 A' },
    { id: 'n2', name: 'model_beta', weight: 0.118, recent_error: 0.03, hit_rate: 0.6, rationale: '測試 B' },
    { id: 'n3', name: 'model_gamma', weight: 0.072, recent_error: 0.04, hit_rate: 0.5, rationale: '測試 C' },
  ],
}; // 0.152 + 0.118 + 0.072 = 0.342

test('capital models page labels relative vs absolute weights when weights are not normalized (sum=0.342)', async ({ page }) => {
  await mockCommonEndpoints(page);

  await page.route('**/api/narrative/models', r => r.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify(MOCK_MODELS_NON_NORMALIZED),
  }));

  await page.goto('/');
  await page.waitForFunction(() => typeof window.switchPage === 'function');
  await page.evaluate(() => window.switchPage('capital_models'));

  const content = page.locator('#capitalModelsContent');
  await expect(content).toBeVisible({ timeout: 5000 });
  await expect(content.locator('.cm-card')).toHaveCount(3);

  // 卡片 = 相對權重: 0.152/0.342=44.4%, 0.118/0.342=34.5%, 0.072/0.342=21.1%
  await expect(content).toContainText('相對權重 44.4%');
  await expect(content).toContainText('相對權重 34.5%');
  await expect(content).toContainText('相對權重 21.1%');
  // 卡片不得顯示原始絕對值 (會誤導為相對占比)
  await expect(content).not.toContainText('相對權重 15.2%');
  await expect(content).not.toContainText('相對權重 11.8%');

  // 合計行 = 絕對權重: 原始權重合計 0.342 → 34.2% (非 100.0%)
  await expect(content).toContainText('絕對權重合計');
  await expect(content).toContainText('34.2%');
  await expect(content).not.toContainText('100.0%');

  await page.screenshot({ path: 'test-results/capital-models/non-normalized-weights.png', fullPage: true });
});


test('capital models page shows graceful error state when atlas-go is unreachable', async ({ page }) => {
  await mockCommonEndpoints(page);

  await page.route('**/api/narrative/models', r => r.fulfill({ status: 503, body: 'Service Unavailable' }));

  await page.goto('/');
  await page.waitForFunction(() => typeof window.switchPage === 'function');
  await page.evaluate(() => window.switchPage('capital_models'));

  const content = page.locator('#capitalModelsContent');
  await expect(content).toBeVisible({ timeout: 5000 });
  await expect(content).toContainText('資料暫時無法載入');
  await expect(content).toContainText('重試');

  // Retry should re-fetch and succeed after switching the mock to valid data.
  await page.route('**/api/narrative/models', r => r.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify(MOCK_MODELS),
  }));

  await page.click('#capitalModelsContent button[data-retry="capital-models"]');
  await expect(content.locator('.cm-card')).toHaveCount(3, { timeout: 5000 });

  await page.screenshot({ path: 'test-results/capital-models/error-state.png', fullPage: true });
});
