// shared_web/static/js/__tests__/login-gate.test.mjs
//
// 登入 gate 單測（Track A：不強迫登入政策，2026-08-21）。
// 邏輯在 client_web/static/js/services/login-gate.js（純函數）。
//
// 執行：cd client_web && npm test

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { PUBLIC_PAGES, GATED_PAGES, pageRequiresLogin, runLoginGate } from '../../../../client_web/static/js/services/login-gate.js';

test('login-gate: 公開頁不需登入（404 + 全部內容頁）', () => {
  const publicPages = [
    'errors/404',
    'home', 'capital_board', 'stock-quote', 'narrative', 'crossmarket',
    'industry', 'retail_sentiment', 'strategies', 'methodology',
    'my-signals', 'capital-causality',
  ];
  assert.deepEqual(PUBLIC_PAGES, publicPages, 'PUBLIC_PAGES 應為完整公開清單');
  for (const p of publicPages) {
    assert.equal(pageRequiresLogin(p), false, `${p} 應允許未登入瀏覽`);
  }
});

test('login-gate: 只有明確 gated 頁（premium）需登入', () => {
  assert.deepEqual(GATED_PAGES, ['premium'], 'GATED_PAGES 應只有需要會員資料/付費的頁');
  assert.equal(pageRequiresLogin('premium'), true, '升級 Premium 應需登入');
  // 組合持倉已遷移 admin（2026-08-23），不再需要 client 登入 gate。
  assert.equal(pageRequiresLogin('portfolio'), false, '組合持倉已遷移 admin，client 不需登入');
});

test('login-gate: gated 頁 + 未登入 → 呼叫導向 member 登入的 redirectFn 並回傳 true', async () => {
  let redirected = 0;
  const gated = await runLoginGate('premium', async () => false, () => { redirected++; });
  assert.equal(gated, true, 'gate 應擋下');
  assert.equal(redirected, 1, '應呼叫一次 redirectFn（導向 go-member 登入）');
});

test('login-gate: gated 頁 + 已登入 → 不導向並回傳 false', async () => {
  let redirected = 0;
  const gated = await runLoginGate('premium', async () => true, () => { redirected++; });
  assert.equal(gated, false, '已登入不應擋下');
  assert.equal(redirected, 0, '不應導向 member');
});

test('login-gate: 公開內容頁（home）不 gate', async () => {
  let switchedTo = null;
  const gated = await runLoginGate('home', async () => false, (id) => { switchedTo = id; });
  assert.equal(gated, false, 'home 不應被 gate');
  assert.equal(switchedTo, null, 'home 不應觸發導向');
});
