// shared_web/static/js/__tests__/parameters-readonly.test.mjs
//
// P0-D': parameters 頁唯讀回歸測試
//   parameters.js 過去實作 inline-edit（點擊值欄位 → POST /api/parameters 直接改值,
//   無確認/驗證/undo），與 admin_web/static/index.html 的「唯讀檢視」文案矛盾。
//   本測試鎖定唯讀行為：渲染不產生任何編輯 affordance（_paramEdit/_paramMapEdit/
//   param-val-editable/點擊編輯/JSON 批量編輯），且不含 POST 寫入路徑。
//
// 執行：node --test shared_web/static/js/__tests__/parameters-readonly.test.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';

// ============================================================================
// DOM stubs（僅需 renderParametersPage / renderAuditLog 用到的部分）
// ============================================================================

const elements = new Map();

function createElement(id) {
  const el = {
    id,
    innerHTML: '',
    textContent: '',
    style: {},
    classList: { add() {}, remove() {}, contains() { return false; } },
    appendChild() {},
    querySelector() { return null; },
  };
  elements.set(id, el);
  return el;
}

global.document = {
  getElementById(id) {
    if (!elements.has(id)) createElement(id);
    return elements.get(id);
  },
  createElement(tag) { return createElement(`create-${tag}`); },
  querySelector() { return null; },
  querySelectorAll() { return []; },
  addEventListener() {},
};

global.window = {};

const { renderParametersPage } = await import('../pages/parameters.js');

// ============================================================================
// Fixtures
// ============================================================================

const params = {
  'risk.max_drawdown_pct': 15,
  'signal.min_confidence': 0.7,
  'risk.auto_halt': true,
  'sector.map': { '0050': { weight: 0.3, enabled: true }, '2330': { weight: 0.2, enabled: false } },
  'long.string.param': 'x'.repeat(200),
};

const categoriesResp = {
  categories: [{ id: 'risk', name: '風險' }, { id: 'signal', name: '訊號' }],
  keys: { risk: ['risk.max_drawdown_pct', 'risk.auto_halt'], signal: ['signal.min_confidence'] },
};

const metadata = {
  'risk.max_drawdown_pct': { source: 'CLI', todo: ['校準'], last_calibrated: '2026-08-01T00:00:00Z' },
  'risk.auto_halt': { source: 'env', todo: [], last_calibrated: null },
};

const auditLog = {
  changes: [
    { timestamp: '2026-08-16T10:00:00Z', key: 'risk.max_drawdown_pct', old_value: 10, new_value: 15, reason: '壓力測試', user: 'admin' },
  ],
};

// ============================================================================
// 唯讀渲染測試
// ============================================================================

test('renderParametersPage: 渲染不產生任何編輯 affordance', () => {
  renderParametersPage(params, categoriesResp, auditLog, metadata);
  const html = elements.get('parametersContent').innerHTML;

  // 值欄位為純顯示
  assert.match(html, />15</, 'number 值應顯示');
  assert.match(html, /0\.7/, 'float 值應顯示');
  assert.match(html, /true/, 'bool 值應顯示');

  // 分類表格 / 來源 / 進化 / 校準欄位保留
  assert.match(html, /風險/, '分類名稱保留');
  assert.match(html, /CLI/, '來源欄位保留');
  assert.match(html, /✓/, '進化欄位保留');
  assert.match(html, /2026\/08\/01/, '校準日期保留');

  // map 參數仍可展開（唯讀顯示），但無 JSON 批量編輯按鈕
  assert.match(html, /param-map-collapsed/, 'map 參數展開入口保留');
  assert.doesNotMatch(html, /JSON 批量編輯/, '不得有 JSON 批量編輯按鈕');

  // 長字串截斷 + 點擊展開（純顯示）保留
  assert.match(html, /param-val-trunc/, '長值截斷顯示保留');

  // ❌ 禁止的編輯 affordance
  for (const banned of ['_paramEdit', '_paramMapEdit', 'param-val-editable', '點擊編輯', 'title="點擊編輯"', 'onclick="window._paramEdit']) {
    assert.ok(!html.includes(banned), `渲染結果不得包含 ${banned}`);
  }
});

test('renderParametersPage: 值欄位不含 POST 寫入路徑', () => {
  renderParametersPage(params, categoriesResp, auditLog, metadata);
  const html = elements.get('parametersContent').innerHTML;
  assert.ok(!html.includes('/api/parameters'), '不得產生 /api/parameters 寫入呼叫');
  assert.ok(!html.includes('method: POST') && !html.includes("'POST'"), '不得產生 POST 請求');
});

test('renderParametersPage: audit log 顯示保留', () => {
  renderParametersPage(params, categoriesResp, auditLog, metadata);
  const html = elements.get('parametersAuditLog').innerHTML;
  assert.match(html, /risk\.max_drawdown_pct/, 'audit log 參數名稱顯示');
  assert.match(html, /15/, 'audit log 新值顯示');
  assert.match(html, /admin/, 'audit log 操作者顯示');
});

test('renderParametersPage: 無參數時顯示 empty 提示', () => {
  renderParametersPage(null, null, null, null);
  const html = elements.get('parametersContent').innerHTML;
  assert.match(html, /無法載入參數配置/, '無參數時顯示提示');
});
