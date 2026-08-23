// client_web/tests/capital-causality.spec.ts
// 錢潮因果（2026-08-23 由 admin_web 遷移到 client_web，/client/capital-causality）。
// 沿用原 admin capital-pages.spec.js 的 causality 測例，mock API 不依賴 backend。

import { test, expect } from '@playwright/test';
import { installAuthMocks } from './auth-mock';

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

async function setupMocks(page) {
  await installAuthMocks(page);
  await page.route('**/api/narrative/models', (route) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ models: [] }) }));
  await page.route('**/api/narrative/templates', (route) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_TEMPLATES) }));
}

test('capital-causality renders 24 templates and theme filter works', async ({ page }) => {
  await setupMocks(page);
  await page.goto('/client/capital-causality');
  await page.waitForFunction(() => typeof window.switchPage === 'function');
  await page.waitForSelector('#capitalCausalityContent .cc-item', { timeout: 8000 });

  await expect(page.locator('.cc-item')).toHaveCount(24);
  await expect(page.locator('text=共 24 個模板')).toBeVisible();

  // filter by theme
  await page.selectOption('#cc-theme-filter', 'AI_capex_surge');
  await expect(page.locator('.cc-item')).toHaveCount(12);

  // expand first item and verify steps bullets
  const firstItem = page.locator('.cc-item').first();
  await firstItem.locator('summary').click();
  await expect(firstItem.locator('.cc-item__steps li')).toHaveCount(2);
});
