// shared_web/static/js/__tests__/app-utils.test.mjs
//
// 驗證 Stage 6 PR#1 加進 app-utils.js 的 timeout + AbortController + 1 retry 邏輯。
//
// 設計：原本 app-utils.js 是裸 fetch 沒有 timeout/retry。Stage 6 升級成：
//   - 預設 timeout 8000ms
//   - 預設 1 retry（總共 2 次）
//   - 預設 500ms backoff
//   - 重試觸發：timeout / network / 5xx / 429
//
// 測試透過替換 globalThis.fetch 模擬：timeout、network error、5xx、4xx、JSON ok。
//
// 執行：node --test shared_web/static/js/__tests__/app-utils.test.mjs

import { test, afterEach } from 'node:test';
import assert from 'node:assert/strict';
import {
  getJSON, silentGetJSON, postJSON, putJSON,
  DEFAULT_TIMEOUT_MS, DEFAULT_RETRY, DEFAULT_RETRY_BACKOFF_MS,
} from '../shared/app-utils.js';

// Minimal localStorage mock for API-key header tests in Node.js.
const store = new Map();
global.localStorage = {
  getItem: (k) => store.get(k) ?? null,
  setItem: (k, v) => store.set(k, String(v)),
  removeItem: (k) => store.delete(k),
};

const originalFetch = globalThis.fetch;

function setFetch(mock) { globalThis.fetch = mock; }
function restoreFetch() { globalThis.fetch = originalFetch; }

afterEach(restoreFetch);

// 構一個成功 response 物件的 helper
function okJsonResponse(body, status = 200) {
  return new Response(JSON.stringify(body), {
    status: status,
    headers: { 'Content-Type': 'application/json' },
  });
}

// ---- 預設值 sanity check ----

test('DEFAULT_* 常數是文件期望值', () => {
  assert.equal(DEFAULT_TIMEOUT_MS, 8000);
  assert.equal(DEFAULT_RETRY, 1);
  assert.equal(DEFAULT_RETRY_BACKOFF_MS, 500);
});

// ---- Happy path ----

test('getJSON 200 直接回傳 JSON body，不 retry', async () => {
  let calls = 0;
  setFetch(async function () {
    calls++;
    return okJsonResponse({ hello: 'world' });
  });

  const data = await getJSON('/api/x');
  assert.deepEqual(data, { hello: 'world' });
  assert.equal(calls, 1);
});

test('postJSON 帶 body + Content-Type header', async () => {
  let captured = null;
  setFetch(async function (url, init) {
    captured = { url: url, init: init };
    return okJsonResponse({ ok: true });
  });

  await postJSON('/api/x', { a: 1 });
  assert.equal(captured.url, '/api/x');
  assert.equal(captured.init.method, 'POST');
  assert.equal(captured.init.headers['Content-Type'], 'application/json');
  assert.equal(captured.init.body, JSON.stringify({ a: 1 }));
  assert.equal(captured.init.credentials, 'include');
});

test('putJSON 同上但 method = PUT', async () => {
  let captured = null;
  setFetch(async function (url, init) {
    captured = { url: url, init: init };
    return okJsonResponse({ ok: true });
  });

  await putJSON('/api/x', { b: 2 });
  assert.equal(captured.init.method, 'PUT');
  assert.equal(captured.init.body, JSON.stringify({ b: 2 }));
});

// ---- Timeout ----

test('timeout 觸發 → AbortError 拋出', async () => {
  setFetch(async function (url, init) {
    // 永遠 hang，模擬 backend 卡住
    return new Promise(function (_, reject) {
      init.signal.addEventListener('abort', function () {
        const err = new Error('aborted');
        err.name = 'AbortError';
        reject(err);
      });
    });
  });

  // 設一個很短的 timeoutMs 加速測試
  await assert.rejects(
    getJSON('/api/hang', { timeoutMs: 50, retry: 0 }),
    function (err) { return err.name === 'AbortError'; }
  );
});

// ---- Retry 行為 ----

test('5xx 觸發 retry，第 2 次成功', async () => {
  let calls = 0;
  setFetch(async function () {
    calls++;
    if (calls === 1) return okJsonResponse({}, 503);
    return okJsonResponse({ recovered: true });
  });

  const data = await getJSON('/api/x', { retryBackoffMs: 1 });
  assert.deepEqual(data, { recovered: true });
  assert.equal(calls, 2);
});

test('network error (TypeError) 觸發 retry', async () => {
  let calls = 0;
  setFetch(async function () {
    calls++;
    if (calls === 1) throw new TypeError('Failed to fetch');
    return okJsonResponse({ ok: true });
  });

  const data = await getJSON('/api/x', { retryBackoffMs: 1 });
  assert.deepEqual(data, { ok: true });
  assert.equal(calls, 2);
});

test('4xx 不 retry（呼叫端拿到的就是 404 錯誤）', async () => {
  let calls = 0;
  setFetch(async function () {
    calls++;
    return okJsonResponse({}, 404);
  });

  await assert.rejects(
    getJSON('/api/x', { retryBackoffMs: 1 }),
    function (err) { return /404/.test(err.message); }
  );
  assert.equal(calls, 1, '4xx 不應 retry');
});

test('retry 用盡：連 2 次 503 → 拋出 503 錯誤', async () => {
  let calls = 0;
  setFetch(async function () {
    calls++;
    return okJsonResponse({}, 503);
  });

  await assert.rejects(
    getJSON('/api/x', { retryBackoffMs: 1 }),
    function (err) { return /503/.test(err.message); }
  );
  assert.equal(calls, 2);
});

test('retry=0 關閉 retry 機制', async () => {
  let calls = 0;
  setFetch(async function () {
    calls++;
    return okJsonResponse({}, 503);
  });

  await assert.rejects(
    getJSON('/api/x', { retry: 0 }),
    function (err) { return /503/.test(err.message); }
  );
  assert.equal(calls, 1);
});

// ---- silent 系列吞錯誤 ----

test('silentGetJSON 4xx 錯誤 → 回傳 null（不阻塞 UI）', async () => {
  setFetch(async function () { return okJsonResponse({}, 404); });

  const data = await silentGetJSON('/api/x');
  assert.equal(data, null);
});

test('silentGetJSON 成功 → 回傳資料', async () => {
  setFetch(async function () { return okJsonResponse({ v: 1 }); });

  const data = await silentGetJSON('/api/x');
  assert.deepEqual(data, { v: 1 });
});

// ---- Backoff timing ----

test('retry 間隔約等於設定的 backoffMs（容忍 5ms 抖動）', async () => {
  let calls = 0;
  setFetch(async function () {
    calls++;
    if (calls === 1) return okJsonResponse({}, 503);
    return okJsonResponse({ ok: true });
  });

  const start = Date.now();
  await getJSON('/api/x', { retry: 1, retryBackoffMs: 80 });
  const elapsed = Date.now() - start;
  assert.ok(elapsed >= 80, 'elapsed too small: ' + elapsed);
  assert.ok(elapsed < 80 + 50, 'elapsed too large: ' + elapsed);
});

// ---- ATLAS_API_KEY header for mutating methods ----

test('postJSON 帶有 localStorage ATLAS_API_KEY 時會附加 X-API-Key header', async () => {
  localStorage.setItem('ATLAS_API_KEY', 'secret-key');
  let captured = null;
  setFetch(async function (url, init) {
    captured = { url: url, init: init };
    return okJsonResponse({ ok: true });
  });

  await postJSON('/api/scheduler/toggle', { enabled: true });
  assert.equal(captured.init.headers['X-API-Key'], 'secret-key');
  assert.equal(captured.init.headers['Content-Type'], 'application/json');
});

test('postJSON 未設定 ATLAS_API_KEY 時不附加 X-API-Key header', async () => {
  localStorage.removeItem('ATLAS_API_KEY');
  let captured = null;
  setFetch(async function (url, init) {
    captured = { url: url, init: init };
    return okJsonResponse({ ok: true });
  });

  await postJSON('/api/scheduler/toggle', { enabled: true });
  assert.equal(captured.init.headers['X-API-Key'], undefined);
});

test('getJSON 有 ATLAS_API_KEY 時附加 X-API-Key header（GET 靜默帶 key）', async () => {
  localStorage.setItem('ATLAS_API_KEY', 'secret-key');
  let captured = null;
  setFetch(async function (url, init) {
    captured = { url: url, init: init };
    return okJsonResponse({ ok: true });
  });

  await getJSON('/api/dashboard/status');
  assert.equal(captured.init.headers['X-API-Key'], 'secret-key');
});

test('getJSON 未設定 ATLAS_API_KEY 時不 prompt 也不附加 X-API-Key header', async () => {
  localStorage.removeItem('ATLAS_API_KEY');
  let captured = null;
  let promptCalls = 0;
  const originalWindow = global.window;
  global.window = {
    prompt: function () { promptCalls++; return 'should-not-be-used'; },
  };
  setFetch(async function (url, init) {
    captured = { url: url, init: init };
    return okJsonResponse({ ok: true });
  });

  try {
    await getJSON('/api/dashboard/status');
    assert.equal(captured.init.headers, undefined, '無 key 的 GET 不應附加 header');
    assert.equal(promptCalls, 0, '無 key 的 GET 不得觸發 prompt');
  } finally {
    if (originalWindow === undefined) {
      delete global.window;
    } else {
      global.window = originalWindow;
    }
  }
});

test('silentGetJSON 有 ATLAS_API_KEY 時附加 X-API-Key header', async () => {
  localStorage.setItem('ATLAS_API_KEY', 'secret-key');
  let captured = null;
  setFetch(async function (url, init) {
    captured = { url: url, init: init };
    return okJsonResponse({ v: 1 });
  });

  const data = await silentGetJSON('/api/deployment/dashboard');
  assert.deepEqual(data, { v: 1 });
  assert.equal(captured.init.headers['X-API-Key'], 'secret-key');
});
