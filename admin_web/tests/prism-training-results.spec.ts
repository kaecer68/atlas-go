import { test, expect } from '@playwright/test';

test('prism training results page renders table with regime badges and explanation', async ({ page }) => {
  // Mock minimal loadAll endpoints so the page boots
  await page.route('**/api/dashboard/**', r => r.fulfill({ json: {} }));
  await page.route('**/api/taiwan/stress-index', r => r.fulfill({ json: {} }));
  await page.route('**/api/narrative/**', r => r.fulfill({ json: {} }));
  await page.route('**/api/synergy/**', r => r.fulfill({ json: {} }));
  await page.route('**/api/alerts', r => r.fulfill({ json: {} }));
  await page.route('**/api/macro/snapshot/latest', r => r.fulfill({ json: {} }));
  await page.route('**/api/dashboard/sessions', r => r.fulfill({ json: { sessions: [], data_status: 'ok' } }));

  await page.route('**/api/prism/training-results', r => r.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify([
      {
        agent_id: 'semiconductor_v3',
        agent_skill: 'sector',
        regime: 'RISK_ON',
        result: {
          hit_rate: 0.62,
          sharpe_ratio: 1.45,
          max_drawdown: -0.08,
          total_return: 0.23,
          signals_count: 50,
          win_count: 31,
          loss_count: 19,
          error: '',
          duration: '2h 15m',
          synthetic: true,
          explanation: 'PRISM_EXPLANATION_RISK_ON_TEXT'
        }
      },
      {
        agent_id: 'value_yield_v2',
        agent_skill: 'style',
        regime: 'RISK_OFF',
        result: {
          hit_rate: 0.41,
          sharpe_ratio: 0.3,
          max_drawdown: -0.15,
          total_return: -0.05,
          signals_count: 30,
          win_count: 12,
          loss_count: 18,
          error: 'partial_data',
          duration: '1h 40m',
          synthetic: false,
          explanation: 'PRISM_EXPLANATION_RISK_OFF_TEXT'
        }
      }
    ]),
  }));

  await page.goto('/');
  await page.evaluate(() => window.switchPage('prism'));

  const content = page.locator('#prismContent');
  await expect(content).toBeVisible({ timeout: 5000 });

  // Table should contain agent_ids and regime labels
  await expect(content).toContainText('semiconductor_v3');
  await expect(content).toContainText('value_yield_v2');
  await expect(content).toContainText('RISK_ON');
  await expect(content).toContainText('RISK_OFF');
  await expect(content).toContainText('共 2 筆');

  // Error column should highlight error string
  await expect(content).toContainText('partial_data');

  // Expand one of the explanations
  const details = content.locator('details summary');
  await details.first().click();
  await expect(content).toContainText('PRISM_EXPLANATION_RISK_ON_TEXT');

  await page.screenshot({ path: 'test-results/prism-training-results/training-results.png', fullPage: true });
});

test('prism training results shows empty state when array is empty', async ({ page }) => {
  await page.route('**/api/dashboard/**', r => r.fulfill({ json: {} }));
  await page.route('**/api/taiwan/stress-index', r => r.fulfill({ json: {} }));
  await page.route('**/api/narrative/**', r => r.fulfill({ json: {} }));
  await page.route('**/api/synergy/**', r => r.fulfill({ json: {} }));
  await page.route('**/api/alerts', r => r.fulfill({ json: {} }));
  await page.route('**/api/macro/snapshot/latest', r => r.fulfill({ json: {} }));
  await page.route('**/api/dashboard/sessions', r => r.fulfill({ json: { sessions: [], data_status: 'ok' } }));

  await page.route('**/api/prism/training-results', r => r.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify([]),
  }));

  await page.goto('/');
  await page.evaluate(() => window.switchPage('prism'));

  const content = page.locator('#prismContent');
  await expect(content).toBeVisible({ timeout: 5000 });
  await expect(content).toContainText('尚無 PRISM 訓練結果');
});
