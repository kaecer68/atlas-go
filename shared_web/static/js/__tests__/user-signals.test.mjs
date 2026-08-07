// shared_web/static/js/__tests__/user-signals.test.mjs
//
// Gap 3-R4「我的追蹤」service 驗證:
//   - signalStatusMeta: 四態 (null / new / ack / dismissed) 映射
//   - renderSignalsList: 空狀態、按鈕組依狀態切換、XSS escape
//   - API 呼叫: list/ack/dismiss/reset 的 URL + method 正確
//
// 執行: node --test shared_web/static/js/__tests__/user-signals.test.mjs
//       (client_web: npm test)

import { test, afterEach } from 'node:test';
import assert from 'node:assert/strict';
import {
  listSignals,
  ackSignal,
  dismissSignal,
  resetSignal,
  signalStatusMeta,
  renderSignalsList,
} from '../services/user-signals.js';

const originalFetch = globalThis.fetch;
function setFetch(mock) { globalThis.fetch = mock; }
function restoreFetch() { globalThis.fetch = originalFetch; }
afterEach(restoreFetch);

function okJsonResponse(body, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

// ---- signalStatusMeta: 四態映射 ----

test('signalStatusMeta: null → 新訊號', () => {
  const meta = signalStatusMeta(null);
  assert.equal(meta.label, '新訊號');
  assert.equal(meta.className, 'badge--new');
});

test('signalStatusMeta: 無 acknowledged_at → 新訊號', () => {
  const meta = signalStatusMeta({ signal_key: 'x', dismissed: false });
  assert.equal(meta.label, '新訊號');
  assert.equal(meta.className, 'badge--new');
});

test('signalStatusMeta: acknowledged_at 存在 → 已讀', () => {
  const meta = signalStatusMeta({
    signal_key: 'x',
    acknowledged_at: '2026-08-07T00:00:00Z',
    dismissed: false,
  });
  assert.equal(meta.label, '已讀');
  assert.equal(meta.className, 'badge--ack');
});

test('signalStatusMeta: dismissed → 已忽略 (即使有 acknowledged_at)', () => {
  const meta = signalStatusMeta({
    signal_key: 'x',
    acknowledged_at: '2026-08-07T00:00:00Z',
    dismissed: true,
  });
  assert.equal(meta.label, '已忽略');
  assert.equal(meta.className, 'badge--dismissed');
});

// ---- renderSignalsList ----

test('renderSignalsList: 空陣列 → 空狀態說明', () => {
  const html = renderSignalsList([]);
  assert.match(html, /還沒有追蹤紀錄/);
});

test('renderSignalsList: null → 空狀態', () => {
  assert.match(renderSignalsList(null), /還沒有追蹤紀錄/);
});

test('renderSignalsList: 新訊號 → 標記已讀 + 不再顯示', () => {
  const html = renderSignalsList([{ signal_key: 'foreign-3day-inflow', dismissed: false }]);
  assert.match(html, /foreign-3day-inflow/);
  assert.match(html, /標記已讀/);
  assert.match(html, /不再顯示/);
  assert.doesNotMatch(html, /恢復顯示/);
  assert.match(html, /badge--new/);
});

test('renderSignalsList: 已讀 → 無標記已讀,有不再顯示', () => {
  const html = renderSignalsList([{
    signal_key: 'margin-balance-extreme',
    acknowledged_at: '2026-08-07T00:00:00Z',
    dismissed: false,
  }]);
  assert.match(html, /已讀/);
  assert.doesNotMatch(html, /標記已讀/);
  assert.match(html, /不再顯示/);
  assert.match(html, /badge--ack/);
});

test('renderSignalsList: 已忽略 → 恢復顯示', () => {
  const html = renderSignalsList([{ signal_key: 'sig-x', dismissed: true }]);
  assert.match(html, /恢復顯示/);
  assert.doesNotMatch(html, /標記已讀/);
  assert.match(html, /badge--dismissed/);
});

test('renderSignalsList: signal_key 含 HTML 被 escape (XSS)', () => {
  const html = renderSignalsList([{ signal_key: '<script>alert(1)</script>', dismissed: false }]);
  assert.doesNotMatch(html, /<script>/);
  assert.match(html, /&lt;script&gt;/);
});

// ---- API 呼叫 URL/method ----

test('listSignals: GET /api/user/signals', async () => {
  let seen;
  setFetch(async (url) => {
    seen = { url: String(url), method: 'GET' };
    return okJsonResponse({ signals: [], count: 0 });
  });
  const payload = await listSignals();
  assert.deepEqual(seen, { url: '/api/user/signals', method: 'GET' });
  assert.equal(payload.count, 0);
});

test('ackSignal: PUT /api/user/signals/{key}/ack', async () => {
  let seen;
  setFetch(async (url, opts) => {
    seen = { url: String(url), method: opts.method };
    return okJsonResponse({ signal_key: 'k1', acknowledged_at: '2026-08-07T00:00:00Z' });
  });
  await ackSignal('k1');
  assert.deepEqual(seen, { url: '/api/user/signals/k1/ack', method: 'PUT' });
});

test('dismissSignal: PUT /api/user/signals/{key}/dismiss', async () => {
  let seen;
  setFetch(async (url, opts) => {
    seen = { url: String(url), method: opts.method };
    return okJsonResponse({ signal_key: 'k1', dismissed: true });
  });
  await dismissSignal('k1');
  assert.deepEqual(seen, { url: '/api/user/signals/k1/dismiss', method: 'PUT' });
});

test('resetSignal: DELETE /api/user/signals/{key}', async () => {
  let seen;
  setFetch(async (url, opts) => {
    seen = { url: String(url), method: opts.method };
    return okJsonResponse({ signal_key: 'k1' });
  });
  await resetSignal('k1');
  assert.deepEqual(seen, { url: '/api/user/signals/k1', method: 'DELETE' });
});

test('signal key 含特殊字元被 encodeURIComponent', async () => {
  let seen;
  setFetch(async (url, opts) => {
    seen = { url: String(url), method: opts.method };
    return okJsonResponse({});
  });
  await ackSignal('a/b?c');
  assert.equal(seen.url, '/api/user/signals/a%2Fb%3Fc/ack');
});
