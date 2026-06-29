// 對應 classifyFetchError 的 timeout kind 測試。
// 對應 P2.1：fetchJSON 使用 AbortController，30 秒逾時後拋 AbortError。
//
// 執行：node --test shared_web/static/js/__tests__/fetch-error-timeout.test.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { classifyFetchError } from '../shared/fetch-error.js';

// ============================================================================
// Timeout（AbortError）
// ============================================================================

test('classifyFetchError: AbortError → timeout + recoverable', () => {
  const err = new DOMException('The user aborted a request', 'AbortError');
  const result = classifyFetchError(err, '/api/strategies');
  assert.equal(result.kind, 'timeout');
  assert.equal(result.recoverable, true);
  assert.match(result.message, /逾時|timeout/);
});

test('classifyFetchError: 帶 code=ABORT_ERR 的 error → timeout', () => {
  const err = new Error('aborted');
  err.code = 'ABORT_ERR';
  err.name = 'AbortError';
  const result = classifyFetchError(err, '/api/strategies');
  assert.equal(result.kind, 'timeout');
});

test('classifyFetchError: 結構完整性（timeout 也包含 4 個欄位）', () => {
  const err = new DOMException('aborted', 'AbortError');
  const r = classifyFetchError(err, '/api/strategies');
  assert.ok('kind' in r);
  assert.ok('message' in r);
  assert.ok('recoverable' in r);
  assert.ok('hint' in r);
  assert.equal(typeof r.kind, 'string');
  assert.equal(typeof r.message, 'string');
  assert.equal(typeof r.recoverable, 'boolean');
  assert.equal(typeof r.hint, 'string');
});
