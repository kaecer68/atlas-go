// shared_web/static/js/__tests__/degraded-reporting.test.mjs
//
// P1-B：API 失敗上報 choke point 改造 — degraded 狀態追蹤測試。
//
// 驗證：
//   1. getJSON / silentGetJSON / getJSONWithTimeout 失敗 → reportDegraded 被呼叫
//      （endpoint 進入 getDegradedEndpoints() + 通知訂閱者 + 派發 'atlas:degraded'）
//   2. dedupe：同 endpoint 於 DEGRADED_REPORT_WINDOW_MS 內重複失敗只通知一次
//   3. 恢復：後續成功自動清除 degraded 狀態（badge 消失的依據）
//   4. 4xx 為確定性錯誤 → 不誤報 degraded（避免 badge 永不消失）
//   5. crossmarket 頁：4 個 endpoint 全失敗 → 6 個 section 顯示降級文案取代空殼
//
// 執行：node --test shared_web/static/js/__tests__/degraded-reporting.test.mjs

import { test, afterEach } from 'node:test';
import assert from 'node:assert/strict';
import {
  getJSON, silentGetJSON, getJSONWithTimeout,
  reportDegraded, clearDegraded, getDegradedEndpoints,
  onDegradedChange, setDegradedReportWindowMs,
} from '../shared/app-utils.js';
import { loadCrossMarketData } from '../pages/crossmarket.js';

const originalFetch = globalThis.fetch;
const originalDocument = globalThis.document;
const originalWindow = globalThis.window;
let unsubscribers = [];

function setFetch(mock) { globalThis.fetch = mock; }
function restoreFetch() { globalThis.fetch = originalFetch; }

function okJsonResponse(body, status = 200) {
  return new Response(JSON.stringify(body), {
    status: status,
    headers: { 'Content-Type': 'application/json' },
  });
}

// 失敗 fetch：永遠 network error（retryable，會 retry 一次後拋出）
function failingFetch() {
  return async function () {
    throw new TypeError('Failed to fetch');
  };
}

afterEach(() => {
  unsubscribers.forEach(fn => fn());
  unsubscribers = [];
  getDegradedEndpoints().forEach(clearDegraded);
  restoreFetch();
  if (originalDocument === undefined) delete globalThis.document;
  else globalThis.document = originalDocument;
  if (originalWindow === undefined) delete globalThis.window;
  else globalThis.window = originalWindow;
  setDegradedReportWindowMs(5 * 60 * 1000);
});

function subscribe() {
  const events = [];
  unsubscribers.push(onDegradedChange(eps => events.push(eps)));
  return events;
}

// ============================================================================
// 1. 失敗 → reportDegraded（getJSON / silentGetJSON / getJSONWithTimeout）
// ============================================================================

test('getJSON 503 失敗 → endpoint 進入 degraded 狀態並通知一次', async () => {
  const events = subscribe();
  setFetch(async function () { return okJsonResponse({}, 503); });

  await assert.rejects(
    getJSON('/api/dashboard/x', { retryBackoffMs: 1 }),
    err => /503/.test(err.message)
  );

  assert.deepEqual(getDegradedEndpoints(), ['/api/dashboard/x']);
  assert.equal(events.length, 1, '一次失敗應只觸發一次通知');
  assert.deepEqual(events[0], ['/api/dashboard/x']);
});

test('silentGetJSON 失敗 → 回傳 null 且 endpoint 進入 degraded 狀態', async () => {
  const events = subscribe();
  setFetch(failingFetch());

  const data = await silentGetJSON('/api/silent-down');

  assert.equal(data, null);
  assert.deepEqual(getDegradedEndpoints(), ['/api/silent-down']);
  assert.equal(events.length, 1);
});

test('getJSONWithTimeout 逾時 → endpoint 進入 degraded 狀態', async () => {
  const events = subscribe();
  setFetch(function (url, init) {
    // 永遠 hang；abort 時 reject（silentGetJSON 底層會吃掉）
    return new Promise(function (_, reject) {
      init.signal.addEventListener('abort', function () {
        const err = new Error('aborted');
        err.name = 'AbortError';
        reject(err);
      });
    });
  });

  const data = await getJSONWithTimeout('/api/slow-endpoint', 60, { retry: 0 });

  assert.equal(data, null);
  assert.deepEqual(getDegradedEndpoints(), ['/api/slow-endpoint']);
  assert.equal(events.length, 1, 'timeout 分支與 abort 路徑 dedupe 後只通知一次');
});

test('4xx 失敗 → 不進入 degraded 狀態（確定性錯誤，避免 badge 永不消失）', async () => {
  const events = subscribe();
  setFetch(async function () { return okJsonResponse({}, 404); });

  await assert.rejects(
    getJSON('/api/gone', { retry: 0 }),
    err => /404/.test(err.message)
  );

  assert.deepEqual(getDegradedEndpoints(), []);
  assert.equal(events.length, 0);
});

// ============================================================================
// 2. dedupe：同 endpoint N 分鐘內只報一次
// ============================================================================

test('dedupe：同 endpoint 於 window 內重複失敗只通知一次', async () => {
  const events = subscribe();
  setFetch(failingFetch());

  await silentGetJSON('/api/flaky');
  await silentGetJSON('/api/flaky');
  await silentGetJSON('/api/flaky');

  assert.deepEqual(getDegradedEndpoints(), ['/api/flaky']);
  assert.equal(events.length, 1, 'window 內重複失敗不應重複通知');
});

test('dedupe 過期：window 結束後再次失敗會再次通知', async () => {
  const events = subscribe();
  setFetch(failingFetch());
  setDegradedReportWindowMs(1); // 強制 window 過期

  await silentGetJSON('/api/flaky2');
  await new Promise(r => setTimeout(r, 5));
  await silentGetJSON('/api/flaky2');

  assert.equal(events.length, 2, 'window 過期後再次失敗應重新通知');
});

// ============================================================================
// 3. 恢復：成功自動清除
// ============================================================================

test('成功後自動清除 degraded 狀態（恢復可見性消失）', async () => {
  const events = subscribe();
  let shouldFail = true;
  setFetch(async function () {
    if (shouldFail) throw new TypeError('Failed to fetch');
    return okJsonResponse({ ok: true });
  });

  await silentGetJSON('/api/recovering');
  assert.deepEqual(getDegradedEndpoints(), ['/api/recovering']);

  shouldFail = false;
  await silentGetJSON('/api/recovering');

  assert.deepEqual(getDegradedEndpoints(), [], '成功後應自動清除');
  assert.equal(events.length, 2, '清除時應再通知一次（badge 隱藏）');
  assert.deepEqual(events[1], []);
});

test('clearDegraded：只清除指定 endpoint，其餘保留', async () => {
  setFetch(failingFetch());
  await silentGetJSON('/api/a');
  await silentGetJSON('/api/b');
  assert.deepEqual(getDegradedEndpoints(), ['/api/a', '/api/b']);

  clearDegraded('/api/a');
  assert.deepEqual(getDegradedEndpoints(), ['/api/b']);
});

// ============================================================================
// 4. CustomEvent：瀏覽器環境派發 'atlas:degraded'
// ============================================================================

test('失敗時在 window 上派發 atlas:degraded CustomEvent', async () => {
  const dispatched = [];
  globalThis.window = {
    dispatchEvent: function (ev) { dispatched.push(ev); },
  };
  setFetch(failingFetch());

  await silentGetJSON('/api/event-check');

  assert.equal(dispatched.length, 1);
  assert.equal(dispatched[0].type, 'atlas:degraded');
  assert.deepEqual(dispatched[0].detail, ['/api/event-check']);
});

test('onDegradedChange 取消訂閱後不再收到通知', async () => {
  const events = [];
  const unsub = onDegradedChange(eps => events.push(eps));
  setFetch(failingFetch());

  await silentGetJSON('/api/unsub');
  unsub();
  await silentGetJSON('/api/unsub2');

  assert.equal(events.length, 1, '取消訂閱後不應再收到通知');
});

// ============================================================================
// 5. crossmarket：全失敗 → 6 個 section 顯示降級文案取代空殼
// ============================================================================

const CM_IDS = [
  'cm-crisis', 'cm-us-indices', 'cm-tech-stocks',
  'cm-macro', 'cm-correlation', 'cm-correlation-matrix',
];
const DEGRADED_TEXT = '美台連動資料源暫時不可用，系統已記錄並將自動恢復';

function makeDocStub() {
  const els = {};
  CM_IDS.forEach(id => { els[id] = { innerHTML: '' }; });
  return {
    getElementById: (id) => els[id] || null,
    querySelector: function () {
      // renderStaleBanner / renderDegradedBanner 的前置檢查用
      return { parentNode: null };
    },
    _els: els,
  };
}

test('crossmarket：4 endpoint 全失敗 → 6 section 顯示降級文案', async () => {
  const stub = makeDocStub();
  globalThis.document = stub;
  setFetch(failingFetch());

  await loadCrossMarketData();

  CM_IDS.forEach(id => {
    assert.ok(
      stub._els[id].innerHTML.includes(DEGRADED_TEXT),
      id + ' 應顯示降級文案，實際: ' + stub._els[id].innerHTML.slice(0, 120)
    );
  });
  // 失敗已進入全域 degraded 狀態（badge 依據）
  assert.ok(getDegradedEndpoints().includes('/api/cross-market/status'));
  assert.ok(getDegradedEndpoints().includes('/api/dashboard/us-indices'));
  assert.ok(getDegradedEndpoints().includes('/api/cross-market/correlation'));
  assert.ok(getDegradedEndpoints().includes('/api/dashboard/correlation-matrix'));
});

test('crossmarket：全部成功 → 正常渲染且無降級文案', async () => {
  const stub = makeDocStub();
  globalThis.document = stub;
  const status = {
    data_status: 'ok',
    spx: { symbol: '^GSPC', value: 5000.12, change_pct: 0.5, timestamp: '2026-08-17T00:00:00Z' },
    ndx: { symbol: '^IXIC', value: 16000.34, change_pct: -0.2, timestamp: '2026-08-17T00:00:00Z' },
    dji: { symbol: '^DJI', value: 39000.56, change_pct: 0.1, timestamp: '2026-08-17T00:00:00Z' },
    sox: { symbol: '^SOX', value: 5100.78, change_pct: 1.2, timestamp: '2026-08-17T00:00:00Z' },
    nvda: { symbol: 'NVDA', value: 120.5, change_pct: 2.0, timestamp: '2026-08-17T00:00:00Z' },
    aapl: { symbol: 'AAPL', value: 220.1, change_pct: -0.5, timestamp: '2026-08-17T00:00:00Z' },
    msft: { symbol: 'MSFT', value: 430.2, change_pct: 0.3, timestamp: '2026-08-17T00:00:00Z' },
    tsm_adr: { symbol: 'TSM', value: 180.9, change_pct: 1.5, timestamp: '2026-08-17T00:00:00Z' },
    vix: { symbol: '^VIX', value: 15.2, change_pct: -3.0, timestamp: '2026-08-17T00:00:00Z' },
    dxy: { symbol: 'DXY', value: 104.5, change_pct: 0.2, timestamp: '2026-08-17T00:00:00Z' },
    usd_twd: { symbol: 'USD/TWD', value: 31.5, change_pct: 0.1, timestamp: '2026-08-17T00:00:00Z' },
    us10y: { symbol: 'US10Y', value: 4.2, change_pct: 1.0, timestamp: '2026-08-17T00:00:00Z' },
    crisis_active: false,
    correlation_spx_twse: 0.55,
    generated_at: '2026-08-17T00:00:00Z',
  };
  const correlation = { correlation: 0.55, observations: 30, window_size: 30, computed_at: '2026-08-17T00:00:00Z', is_fallback: false };
  const matrix = { symbols: ['半導體'], labels: ['半導體'], matrix: [[1]] };

  setFetch(async function (url) {
    if (url === '/api/cross-market/status') return okJsonResponse(status);
    if (url === '/api/dashboard/us-indices') return okJsonResponse({ indices: [] });
    if (url === '/api/cross-market/correlation') return okJsonResponse(correlation);
    if (url === '/api/dashboard/correlation-matrix') return okJsonResponse(matrix);
    return okJsonResponse({});
  });

  await loadCrossMarketData();

  CM_IDS.forEach(id => {
    assert.ok(
      !stub._els[id].innerHTML.includes(DEGRADED_TEXT),
      id + ' 不應顯示降級文案'
    );
  });
  assert.ok(stub._els['cm-us-indices'].innerHTML.includes('S&amp;P 500') || stub._els['cm-us-indices'].innerHTML.includes('S&P 500'));
  assert.ok(stub._els['cm-correlation-matrix'].innerHTML.includes('半導體'));
  assert.deepEqual(getDegradedEndpoints(), [], '全部成功後不應有 degraded endpoint');
});
