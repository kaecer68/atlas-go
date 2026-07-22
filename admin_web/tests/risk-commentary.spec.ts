import { test, expect } from '@playwright/test';

test('risk commentary panel renders on page-live with confidence_commentary', async ({ page }) => {
  // Minimal mocks for other loadAll fetches so page boots without errors.
  await page.route('**/api/dashboard/system-health', r => r.fulfill({ json: {} }));
  await page.route('**/api/dashboard/macro-radar', r => r.fulfill({ json: {} }));
  await page.route('**/api/dashboard/agent-observatory', r => r.fulfill({ json: { scorecards: [] } }));
  await page.route('**/api/dashboard/recommendation-pipeline', r => r.fulfill({ json: {} }));
  await page.route('**/api/dashboard/live-status', r => r.fulfill({ json: {} }));
  await page.route('**/api/dashboard/risk-exposure', r => r.fulfill({ json: {} }));
  await page.route('**/api/dashboard/experiment-inbox', r => r.fulfill({ json: {} }));
  await page.route('**/api/dashboard/universe-overlap', r => r.fulfill({ json: {} }));
  await page.route('**/api/taiwan/stress-index', r => r.fulfill({ json: {} }));
  await page.route('**/api/narrative/bundle', r => r.fulfill({ json: {} }));
  await page.route('**/api/macro/snapshot/latest', r => r.fulfill({ json: {} }));
  await page.route('**/api/dashboard/data-channels', r => r.fulfill({ json: {} }));
  await page.route('**/api/dashboard/sessions', r => r.fulfill({ json: { sessions: [], data_status: 'ok' } }));
  await page.route('**/api/dashboard/phase3-status', r => r.fulfill({ json: {} }));
  await page.route('**/api/alerts', r => r.fulfill({ json: {} }));
  await page.route('**/api/dashboard/retail-sentiment', r => r.fulfill({ json: {} }));
  await page.route('**/api/dashboard/capital-phase', r => r.fulfill({ json: {} }));
  await page.route('**/api/dashboard/tax-snapshot', r => r.fulfill({ json: {} }));
  await page.route('**/api/dashboard/regime-history', r => r.fulfill({ json: {} }));
  await page.route('**/api/synergy/darwinian-trend', r => r.fulfill({ json: {} }));
  await page.route('**/api/synergy/darwinian-status', r => r.fulfill({ json: {} }));
  await page.route('**/api/dashboard/risk-calibration', r => r.fulfill({ json: {} }));

  await page.route('**/api/risk/commentary', r => r.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({
      phase: 'INTRA_SESSION',
      verdict: 'BLOCK',
      reason: '外資本週累計賣超 800 億',
      action_type: 'reduce_exposure',
      action_description: '砍倉半導體 30%',
      mode: 'live',
      symbol: '2330.TW',
      recorded_at: new Date().toISOString(),
      confidence_commentary: 'RISK_COMMENTARY_LLM_TEXT_FOR_TEST',
      generated: true,
    }),
  }));

  await page.goto('/');
  await page.click('a[data-page="live"]');

  const panel = page.locator('#liveRiskCommentaryPanel');
  await expect(panel).toBeVisible({ timeout: 5000 });
  await expect(panel).toContainText('🛑 阻擋');
  await expect(panel).toContainText('RISK_COMMENTARY_LLM_TEXT_FOR_TEST');
  await expect(panel).toContainText('外資本週累計賣超 800 億');
  await expect(panel).toContainText('2330.TW');

  await page.screenshot({ path: 'test-results/risk-commentary/commentary.png', fullPage: true });
});

test('risk commentary shows empty state when generated=false', async ({ page }) => {
  await page.route('**/api/dashboard/**', r => r.fulfill({ json: {} }));
  await page.route('**/api/taiwan/stress-index', r => r.fulfill({ json: {} }));
  await page.route('**/api/narrative/**', r => r.fulfill({ json: {} }));
  await page.route('**/api/synergy/**', r => r.fulfill({ json: {} }));
  await page.route('**/api/alerts', r => r.fulfill({ json: {} }));
  await page.route('**/api/macro/snapshot/latest', r => r.fulfill({ json: {} }));
  await page.route('**/api/dashboard/sessions', r => r.fulfill({ json: { sessions: [], data_status: 'ok' } }));

  await page.route('**/api/risk/commentary', r => r.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({ generated: false }),
  }));

  await page.goto('/');
  await page.click('a[data-page="live"]');

  const el = page.locator('#liveRiskCommentary');
  await expect(el).toBeVisible({ timeout: 5000 });
  await expect(el).toContainText('尚無風控長評語');
});
