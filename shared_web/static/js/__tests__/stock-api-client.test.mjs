// shared_web/static/js/__tests__/stock-api-client.test.mjs
//
// 驗證 2026-08-07 新增的 fetchStockMonthlyRevenue client method：
//   - URL 組裝（symbol 必要、year/month optional）
//   - TTL 7 天（monthly-revenue cache key）
//   - coverage 例外：TPEX symbol 不回 NOT_COVERED（後端行為，client 只是透傳）
//
// 執行：node --test shared_web/static/js/__tests__/stock-api-client.test.mjs

import { test, afterEach } from 'node:test';
import assert from 'node:assert/strict';
import { fetchStockMonthlyRevenue } from '../services/stock-api-client.js';

const originalFetch = globalThis.fetch;

// Node test runner has no jsdom — stub localStorage (mirrors
// onboarding.test.mjs pattern) so fetchCached's cache read/write works.
const memStorage = new Map();
global.localStorage = {
  getItem: (k) => (memStorage.has(k) ? memStorage.get(k) : null),
  setItem: (k, v) => { memStorage.set(k, String(v)); },
  removeItem: (k) => { memStorage.delete(k); },
  clear: () => { memStorage.clear(); },
};

function setFetch(mock) { globalThis.fetch = mock; }
function restoreFetch() { globalThis.fetch = originalFetch; }

afterEach(restoreFetch);

// 每個 test 前清空 cache,避免 symbol 跨 test 污染
function clearStorage() { memStorage.clear(); }

function okJsonResponse(body, status = 200) {
  return new Response(JSON.stringify(body), {
    status: status,
    headers: { 'Content-Type': 'application/json' },
  });
}

// 用 symbol 當 cache key 隔離,避免 test 間 localStorage 污染
const STORAGE_PREFIX = 'atlas_stock_cache::';

// ---- fetchStockMonthlyRevenue ----

test('fetchStockMonthlyRevenue 組裝 symbol-only URL', async () => {
  let capturedUrl = null;
  setFetch(async (url) => {
    capturedUrl = url;
    return okJsonResponse({ symbol: '2330.TW', value: 25000000000, change_pct: 12.5 });
  });
  const data = await fetchStockMonthlyRevenue('2330');
  assert.equal(capturedUrl, '/api/stock/monthly_revenue?symbol=2330');
  assert.equal(data.value, 25000000000);
});

test('fetchStockMonthlyRevenue 組裝含 year/month URL', async () => {
  clearStorage();
  let capturedUrl = null;
  setFetch(async (url) => {
    capturedUrl = url;
    return okJsonResponse({ symbol: '3131.TW', value: 631051000, change_pct: 12.7 });
  });
  const data = await fetchStockMonthlyRevenue('3131', 2026, 7);
  assert.equal(capturedUrl, '/api/stock/monthly_revenue?symbol=3131&year=2026&month=7');
  assert.equal(data.symbol, '3131.TW');
});

test('fetchStockMonthlyRevenue TPEX 資料不回 NOT_COVERED（透傳後端 200）', async () => {
  clearStorage();
  setFetch(async () => okJsonResponse({ symbol: '6640.TW', value: 340792000, change_pct: 8.9 }));
  const data = await fetchStockMonthlyRevenue('6640');
  // 重點：後端對 TPEX 回 200 + 真實資料（不是 coverage_note），
  // client 只是透傳 — 這裡斷言 data 不帶 coverage_note。
  assert.equal(data.coverage_note, undefined);
  assert.equal(data.value, 340792000);
});

test('fetchStockMonthlyRevenue 503（quota exhausted）時 reject 帶 message', async () => {
  clearStorage();
  setFetch(async () => okJsonResponse({ error: 'finmind daily quota nearly exhausted, retry tomorrow' }, 503));
  await assert.rejects(
    () => fetchStockMonthlyRevenue('3131'),
    /quota/
  );
});

test('monthly-revenue TTL 是 7 天（對位 TTL_MS 設計）', () => {
  // 直接驗證 localStorage cache 的 expiresAt 是 7 天後
  clearStorage();
  const ttlMs = 7 * 24 * 60 * 60 * 1000;
  setFetch(async () => okJsonResponse({ symbol: '2330.TW', value: 1 }));
  return fetchStockMonthlyRevenue('2330').then(() => {
    const raw = localStorage.getItem(`${STORAGE_PREFIX}monthly-revenue::2330`);
    assert.ok(raw, 'cache entry should exist');
    const entry = JSON.parse(raw);
    const expectedExpiry = Date.now() + ttlMs;
    // 允許 ±5s 誤差（fetch 本身耗時）
    assert.ok(
      Math.abs(entry.expiresAt - expectedExpiry) < 5000,
      `expiresAt ${entry.expiresAt} should be ~7 days from now`
    );
  });
});
