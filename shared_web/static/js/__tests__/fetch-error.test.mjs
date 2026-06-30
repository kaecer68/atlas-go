// shared_web/static/js/__tests__/fetch-error.test.mjs
//
// 對應 INVESTIGATION.md「模式 A: backend 沒起」與「模式 B: registry 503」的修復。
// classifyFetchError 必須把 raw fetch / fetchJSON 拋出的錯誤轉成
// { kind, message, recoverable, hint } 結構，讓前端可以顯示可行動訊息。
//
// 執行：node --test shared_web/static/js/__tests__/fetch-error.test.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { classifyFetchError } from '../shared/fetch-error.js';

// ============================================================================
// 模式 A：Backend 完全不可達（瀏覽器拋 TypeError: Failed to fetch）
// ============================================================================

test('classifyFetchError: TypeError "Failed to fetch" → network + 啟動指令', () => {
  const err = new TypeError('Failed to fetch');
  const result = classifyFetchError(err, '/api/strategies');

  assert.equal(result.kind, 'network');
  assert.equal(result.recoverable, true);
  assert.match(result.message, /後端/);
  assert.match(result.hint, /go run/);
  assert.match(result.hint, /cmd\/atlas/);
});

test('classifyFetchError: TypeError 即使 URL 不同也要分類為 network', () => {
  const err = new TypeError('fetch failed');
  const result = classifyFetchError(err, '/api/dashboard/system-health');
  assert.equal(result.kind, 'network');
  assert.equal(result.recoverable, true);
});

// ============================================================================
// 模式 B：Backend 起來但 registry 未初始化（HTTP 503）
// ============================================================================

test('classifyFetchError: HTTP 503 → http_503 + seed 檔案路徑', () => {
  const err = new Error('HTTP 503');
  err.status = 503;
  const result = classifyFetchError(err, '/api/strategies');

  assert.equal(result.kind, 'http_503');
  assert.equal(result.recoverable, true);
  assert.match(result.message, /registry/i);
  assert.match(result.hint, /strategy_techniques\.json/);
});

// ============================================================================
// 模式 E：Schema/欄位錯誤（HTTP 4xx）
// ============================================================================

test('classifyFetchError: HTTP 404 → http_404', () => {
  const err = new Error('HTTP 404');
  err.status = 404;
  const result = classifyFetchError(err, '/api/strategies/unknown-id');
  assert.equal(result.kind, 'http_404');
  assert.equal(result.recoverable, false);
});

test('classifyFetchError: HTTP 400 → http_4xx', () => {
  const err = new Error('HTTP 400');
  err.status = 400;
  const result = classifyFetchError(err, '/api/strategies');
  assert.equal(result.kind, 'http_4xx');
  assert.equal(result.recoverable, false);
});

// ============================================================================
// 其他 5xx
// ============================================================================

test('classifyFetchError: HTTP 500 → http_5xx', () => {
  const err = new Error('HTTP 500');
  err.status = 500;
  const result = classifyFetchError(err, '/api/strategies');
  assert.equal(result.kind, 'http_5xx');
  assert.equal(result.recoverable, true);
  assert.match(result.hint, /稍後/);
});

test('classifyFetchError: HTTP 502/504 也歸類為 5xx', () => {
  for (const status of [502, 504]) {
    const err = new Error(`HTTP ${status}`);
    err.status = status;
    const result = classifyFetchError(err, '/api/strategies');
    assert.equal(result.kind, 'http_5xx');
    assert.equal(result.recoverable, true);
  }
});

// ============================================================================
// 410 Gone（既有 fetchJSON 邏輯）
// ============================================================================

test('classifyFetchError: HTTP 410 → http_410', () => {
  const err = new Error('HTTP 410');
  err.status = 410;
  const result = classifyFetchError(err, '/api/strategies/old-id');
  assert.equal(result.kind, 'http_410');
  assert.equal(result.recoverable, false);
});

// ============================================================================
// Fallback：無 status 屬性的 Error
// ============================================================================

test('classifyFetchError: 無 status 的 Error → unknown', () => {
  const err = new Error('something weird');
  const result = classifyFetchError(err, '/api/strategies');
  assert.equal(result.kind, 'unknown');
  assert.equal(result.recoverable, false);
  assert.match(result.message, /something weird/);
});

test('classifyFetchError: null/undefined → unknown', () => {
  assert.equal(classifyFetchError(null, '/x').kind, 'unknown');
  assert.equal(classifyFetchError(undefined, '/x').kind, 'unknown');
});

// ============================================================================
// 結構完整性
// ============================================================================

test('classifyFetchError: 回傳結構永遠包含 4 個欄位', () => {
  const cases = [
    new TypeError('x'),
    Object.assign(new Error('x'), { status: 503 }),
    Object.assign(new Error('x'), { status: 404 }),
    Object.assign(new Error('x'), { status: 500 }),
    new Error('x'),
  ];
  for (const err of cases) {
    const r = classifyFetchError(err, '/api/strategies');
    assert.ok('kind' in r, 'kind 必須存在');
    assert.ok('message' in r, 'message 必須存在');
    assert.ok('recoverable' in r, 'recoverable 必須存在');
    assert.ok('hint' in r, 'hint 必須存在');
    assert.equal(typeof r.kind, 'string');
    assert.equal(typeof r.message, 'string');
    assert.equal(typeof r.recoverable, 'boolean');
    assert.equal(typeof r.hint, 'string');
  }
});
