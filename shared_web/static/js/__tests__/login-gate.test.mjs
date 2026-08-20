// shared_web/static/js/__tests__/login-gate.test.mjs
//
// 會員制 (GUEST_MODE=false) 登入 gate 單測：頁面分類 + 未登入導向 login。
// 邏輯在 client_web/static/js/services/login-gate.js（純函數）。
//
// 執行：cd client_web && npm test

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { PUBLIC_PAGES, pageRequiresLogin, runLoginGate } from '../../../../client_web/static/js/services/login-gate.js';

test('login-gate: 公開頁 = login / register / errors-404 不需登入', () => {
  assert.deepEqual(PUBLIC_PAGES, ['login', 'register', 'errors/404']);
  for (const p of PUBLIC_PAGES) {
    assert.equal(pageRequiresLogin(p), false, `${p} 應為公開頁`);
  }
});

test('login-gate: 所有功能頁（含 home 今日判讀）都需登入', () => {
  const functionalPages = [
    'home', 'capital_board', 'stock-quote', 'narrative', 'crossmarket',
    'industry', 'retail_sentiment', 'pipeline', 'portfolio', 'strategies',
    'methodology', 'mcp', 'performance-report', 'my-signals', 'premium',
  ];
  for (const p of functionalPages) {
    assert.equal(pageRequiresLogin(p), true, `${p} 應需登入`);
  }
});

test('login-gate: 需登入頁 + 未登入 → 導向 login 並回傳 true', async () => {
  let switchedTo = null;
  const gated = await runLoginGate('home', async () => false, (id) => { switchedTo = id; });
  assert.equal(gated, true, 'gate 應擋下');
  assert.equal(switchedTo, 'login', '應導向 login');
});

test('login-gate: 需登入頁 + 已登入 → 不導向並回傳 false', async () => {
  let switchedTo = null;
  const gated = await runLoginGate('portfolio', async () => true, (id) => { switchedTo = id; });
  assert.equal(gated, false, '已登入不應擋下');
  assert.equal(switchedTo, null, '不應導向 login');
});

test('login-gate: 公開頁（login）不 gate', async () => {
  let switchedTo = null;
  const gated = await runLoginGate('login', async () => false, (id) => { switchedTo = id; });
  assert.equal(gated, false, 'login 頁不應被 gate');
  assert.equal(switchedTo, null, 'login 頁不應觸發導向');
});
