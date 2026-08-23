// admin_web/tests/pipeline-migration.spec.ts
// 投資管線 client → admin 遷移驗證（2026-08-23）：
//   1) /admin/pipeline 可開啟（page div + loadPageData 接線）
//   2) 「前往【最新回測】啟動回測 →」按鈕（client 端原本失效）在 admin 端
//      正確切到 page-reports（backtest 啟動頁）
//   3) 進階篩選 toggle / 套用篩選按鈕有接線（window.applyFilters 可用）
import { test, expect } from '@playwright/test';

// 有 session_id 但零 items → renderPipeline 走 emptyStateWithAction 分支
// （只有「有場次但無標的」時才出現「前往【最新回測】啟動回測 →」按鈕）。
const PIPELINE_EMPTY_SESSION = {
  session_id: 'mock-s1',
  regime: 'RISK_ON',
  recorded_at: '2026-08-23T00:00:00Z',
  items: [],
  guard_outcomes: [],
};

async function mockCommonEndpoints(page) {
  await page.route('**/api/dashboard/**', r => r.fulfill({ json: {} }));
  await page.route('**/api/dashboard/recommendation-pipeline', r => r.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(PIPELINE_EMPTY_SESSION) }));
  await page.route('**/api/taiwan/stress-index', r => r.fulfill({ json: {} }));
  await page.route('**/api/narrative/**', r => r.fulfill({ json: {} }));
  await page.route('**/api/synergy/**', r => r.fulfill({ json: {} }));
  await page.route('**/api/alerts', r => r.fulfill({ json: {} }));
  await page.route('**/api/macro/snapshot/latest', r => r.fulfill({ json: {} }));
  await page.route('**/api/dashboard/sessions', r => r.fulfill({ json: { sessions: [], data_status: 'ok' } }));
  await page.route('**/api/backtest/**', r => r.fulfill({ json: {} }));
}

test('pipeline page opens on admin and 前往最新回測 button routes to reports', async ({ page }) => {
  await mockCommonEndpoints(page);
  await page.goto('/');
  await page.waitForFunction(() => typeof window.switchPage === 'function');
  await page.evaluate(() => window.switchPage('pipeline', true));

  // page-pipeline active + shell content rendered
  await expect(page.locator('#page-pipeline')).toHaveClass(/active/, { timeout: 5000 });
  await expect(page.locator('#page-pipeline .workflow')).toBeVisible();
  await expect(page.locator('#recommendationPipeline')).toBeVisible();

  // 按鈕修復：空態 CTA（有場次但無標的）存在，點擊後應切到 page-reports。
  const navBtn = page.locator('#page-pipeline button', { hasText: '前往【最新回測】啟動回測' });
  await expect(navBtn).toHaveCount(1, { timeout: 5000 });
  await navBtn.click();
  await expect(page.locator('#page-reports')).toHaveClass(/active/, { timeout: 5000 });
  await expect(page.locator('#pageTitle')).toHaveText('最新回測');
});

test('pipeline filter panel toggles and applyFilters is wired', async ({ page }) => {
  await mockCommonEndpoints(page);
  await page.goto('/');
  await page.waitForFunction(() => typeof window.switchPage === 'function');
  await page.evaluate(() => window.switchPage('pipeline', true));

  const filterPanel = page.locator('#filterPanel');
  // 初始無 open class（max-height:0 + opacity:0 收合）
  await expect(filterPanel).not.toHaveClass(/open/);
  await page.click('#filterToggle');
  await expect(filterPanel).toHaveClass(/open/);

  // applyFilters / clearFilters 已接線（window 層面）
  const wired = await page.evaluate(() => typeof window.applyFilters === 'function' && typeof window.clearFilters === 'function');
  expect(wired).toBe(true);
});

test('portfolio and performance-report pages open on admin without login gate', async ({ page }) => {
  await mockCommonEndpoints(page);
  await page.goto('/');
  await page.waitForFunction(() => typeof window.switchPage === 'function');

  await page.evaluate(() => window.switchPage('portfolio', true));
  await expect(page.locator('#page-portfolio')).toHaveClass(/active/, { timeout: 5000 });
  await expect(page.locator('#portfolioPageTitle')).toHaveText('📂 組合持倉');

  await page.evaluate(() => window.switchPage('performance-report', true));
  await expect(page.locator('#page-performance-report')).toHaveClass(/active/, { timeout: 5000 });
  await expect(page.locator('#performanceReportContainer')).toBeVisible();
});
