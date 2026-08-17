// shared_web/static/js/__tests__/alerts-single-source.test.mjs
//
// PR-4 regression guard: alerts 雙寫競態修復 (29↔45 跳動, 漏 16 筆 INFO)。
//
// 背景: admin main.js 的 fetchNonCore 原本用「無過濾 /api/alerts」覆寫
// alertsPanel 渲染, 與 lazy loader 的 loadAlerts() (status=triggered) 雙寫
// 競態, 造成「需決策」數字跳動。修復後 alerts 資料統一由
// loadPageData('alerts') → loadAlerts() (status=triggered&page_size=50)
// 單一來源供應。
//
// 本測試為來源層級護欄 (main.js 依賴瀏覽器模組, 無法直接 node import):
// 1. fetchNonCore 不再 fetch /api/alerts、不再操作 alertsPanel、
//    不再呼叫 renderAlerts。
// 2. alerts 頁單一來源仍在: pages/alerts.js loadAlerts 使用
//    status=triggered&page_size=50, 且 main.js lazy loader 仍路由到它。
//
// 执行: node --test shared_web/static/js/__tests__/alerts-single-source.test.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));

// __tests__ -> shared_web/static/js -> shared_web/static -> shared_web -> repo root
const REPO_ROOT = resolve(__dirname, '../../../../');
const MAIN_JS = readFileSync(resolve(REPO_ROOT, 'admin_web/static/js/main.js'), 'utf8');
const ALERTS_PAGE_JS = readFileSync(resolve(REPO_ROOT, 'shared_web/static/js/pages/alerts.js'), 'utf8');

function fetchNonCoreSource() {
  const start = MAIN_JS.indexOf('async function fetchNonCore');
  assert.ok(start >= 0, 'main.js must define fetchNonCore');
  // fetchNonCore ends at the closing brace of its own block: find the next
  // top-level `async function` / `function` / `export` after it.
  const tail = MAIN_JS.slice(start);
  const end = tail.search(/\n(?:async function|function|export function|\/\/ ---)/);
  return end === -1 ? tail : tail.slice(0, end);
}

test('fetchNonCore: 不再 fetch 無過濾 /api/alerts', () => {
  const fn = fetchNonCoreSource();
  assert.ok(!fn.includes('/api/alerts'), 'fetchNonCore 不得再抓取 /api/alerts (無過濾雙寫來源)');
});

test('fetchNonCore: 不再操作 alertsPanel (setPanelLoading/setPanelError/renderAlerts)', () => {
  const fn = fetchNonCoreSource();
  assert.ok(!fn.includes('alertsPanel'), 'fetchNonCore 不得再操作 alertsPanel');
  assert.ok(!fn.includes('renderAlerts'), 'fetchNonCore 不得再呼叫 renderAlerts');
});

test('fetchNonCore: 其餘非核心面板 fetch 保留 (macro/live/risk/phase3)', () => {
  const fn = fetchNonCoreSource();
  for (const ep of [
    '/api/dashboard/macro-radar',
    '/api/dashboard/recommendation-pipeline',
    '/api/dashboard/live-status',
    '/api/dashboard/risk-exposure',
    '/api/dashboard/phase3-status',
  ]) {
    assert.ok(fn.includes(ep), `fetchNonCore 應保留 ${ep}`);
  }
  assert.ok(!fn.includes('alerts'), 'fetchNonCore 不得再有 alerts 變數/解構');
});

test('alerts 頁單一來源: pages/alerts.js loadAlerts 用 status=triggered&page_size=50', () => {
  assert.ok(
    ALERTS_PAGE_JS.includes("'/api/alerts?status=triggered&page_size=50'"),
    'loadAlerts 必須以 status=triggered&page_size=50 作為單一資料來源'
  );
});

test('main.js lazy loader 仍路由 alerts 頁到 m.alerts.loadAlerts()', () => {
  const loader = MAIN_JS.slice(MAIN_JS.indexOf("pageId === 'alerts'"));
  assert.ok(loader.includes('m.alerts.loadAlerts'), 'loadPageData("alerts") 必須呼叫 m.alerts.loadAlerts()');
  // 單一來源: main.js 全檔不得再有其他 /api/alerts 抓取
  assert.ok(
    !MAIN_JS.includes("'/api/alerts'") && !MAIN_JS.includes('"/api/alerts"'),
    'main.js 全檔不得再有 /api/alerts 抓取 (單一來源在 pages/alerts.js)'
  );
});
