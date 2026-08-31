import { test, expect } from '@playwright/test';

/**
 * Scheduler status badge regression test (UX backlog: admin 右上角紅塊).
 *
 * Root cause: auth-form.css 的 premium 角標 ribbon 樣式寫成裸 `.tier-badge`
 * （position:absolute; top:-12px; right:16px），使全站 tier-badge
 * （scheduler 狀態啟用/停用/逾期等）全部脫離文件流、疊到頁面右上角，
 * 形成內容無法辨識的小紅塊。
 *
 * 此測試鎖定兩件事：
 *  1. scheduler 狀態 badge 渲染在表格 cell 內（position 非 absolute）
 *  2. badge 的 bounding box 落在 scheduler 表格內（不再漂浮到頁面右上角）
 */

async function mockCommonEndpoints(page) {
  await page.route('**/api/dashboard/**', r => r.fulfill({ json: {} }));
  await page.route('**/api/taiwan/stress-index', r => r.fulfill({ json: {} }));
  await page.route('**/api/narrative/**', r => r.fulfill({ json: {} }));
  await page.route('**/api/synergy/**', r => r.fulfill({ json: {} }));
  await page.route('**/api/alerts', r => r.fulfill({ json: {} }));
  await page.route('**/api/macro/snapshot/latest', r => r.fulfill({ json: {} }));
  await page.route('**/api/dashboard/sessions', r => r.fulfill({ json: { sessions: [], data_status: 'ok' } }));
}

// 真實 API 回傳 bare array（internal/apigateway.TaskStatus），不是 {tasks: []}
// 2026-08-31 (#1776 audit): home scheduler panel now shows ONLY abnormal rows
// (archived / overdue / failing). Mock: one healthy enabled task (must NOT
// appear), one failing enabled task, one archived disabled task.
const MOCK_TASKS = [
  { name: 'prism_auto_balancer', channel_id: '-', enabled: true, interval: 300000000000, last_run: '2026-08-17T08:00:00Z', next_run: '2026-08-17T08:05:00Z', consecutive_failures: 0 },
  { name: 'risk_gate_calibrate', channel_id: '-', enabled: true, interval: 86400000000000, last_run: '2026-08-17T08:00:00Z', next_run: '2026-08-18T08:00:00Z', consecutive_failures: 3, last_error: 'self_calibrate: no sessions available' },
  { name: 'ml_retrain', channel_id: '-', enabled: false, interval: 86400000000000, last_run: '2026-08-16T08:00:00Z', next_run: '2026-08-24T08:00:00Z', consecutive_failures: 0 },
];

test('scheduler status tier badges render inside table cells, not floating at viewport top-right', async ({ page }) => {
  await mockCommonEndpoints(page);

  await page.route('**/api/scheduler/status', r => r.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify(MOCK_TASKS),
  }));
  await page.route('**/api/dashboard/task-liveness', r => r.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({ tasks: [] }),
  }));

  await page.goto('/');
  await page.waitForFunction(() => typeof window.switchPage === 'function');

  const table = page.locator('#schedulerStatusContent table.ranker-table');
  await expect(table).toBeVisible({ timeout: 8000 });

  // 只列異常：健康任務不出現；失敗與歸檔各一列
  await expect(page.locator('#schedulerStatusContent')).toContainText('risk_gate_calibrate');
  await expect(page.locator('#schedulerStatusContent')).toContainText('ml_retrain');
  await expect(page.locator('#schedulerStatusContent')).not.toContainText('prism_auto_balancer');
  await expect(page.locator('#schedulerStatusContent .tier-badge--bearish')).toHaveCount(1);
  await expect(page.locator('#schedulerStatusContent .tier-badge--neutral')).toHaveCount(1);
  await expect(page.locator('#schedulerStatusContent')).toContainText('連續失敗 3 次');
  await expect(page.locator('#schedulerStatusContent')).toContainText('歸檔 · 等待啟用');

  // 每個 badge 都必須是非 absolute，且落在表格 bounding box 內
  const tableBox = await table.boundingBox();
  const badgeInfo = await page.$$eval('#schedulerStatusContent .tier-badge', els =>
    els.map(el => {
      const cs = getComputedStyle(el);
      const r = el.getBoundingClientRect();
      return { pos: cs.position, top: r.top, left: r.left, width: r.width, height: r.height };
    })
  );
  expect(badgeInfo.length).toBeGreaterThan(0);
  for (const b of badgeInfo) {
    expect(b.pos).not.toBe('absolute');
    expect(b.top).toBeGreaterThanOrEqual(tableBox.y - 1);
    expect(b.top + b.height).toBeLessThanOrEqual(tableBox.y + tableBox.height + 1);
  }

  await page.screenshot({ path: 'test-results/scheduler-badge/in-table.png', fullPage: true });
});
