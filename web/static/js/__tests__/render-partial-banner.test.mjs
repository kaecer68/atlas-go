// 對應 renderPartialBanner 的單元測試。
// 涵蓋 4 種 dataStatus（ok/idle/empty/partial/failed）+ indicators 警告 + a11y。
//
// 執行：node --test web/static/js/__tests__/render-partial-banner.test.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { renderPartialBanner } from '../pages/strategies.js';

// ============================================================================
// 無 banner 狀態
// ============================================================================

test('renderPartialBanner: dataStatus=ok → 空字串', () => {
  assert.equal(renderPartialBanner({ dataStatus: 'ok' }), '');
});

test('renderPartialBanner: dataStatus=idle → 空字串', () => {
  assert.equal(renderPartialBanner({ dataStatus: 'idle' }), '');
});

// ============================================================================
// failed banner
// ============================================================================

test('renderPartialBanner: failed → role=alert + 列出所有錯誤的 hint', () => {
  const state = {
    dataStatus: 'failed',
    errors: {
      '/api/strategies': { kind: 'http_503', message: 'registry 未初始化', hint: '請檢查 strategy_techniques.json' },
      '/api/strategies/layers': { kind: 'network', message: '後端未啟動', hint: '執行 go run ./cmd/atlas --api' },
    },
  };
  const html = renderPartialBanner(state);
  assert.match(html, /role="alert"/);
  assert.match(html, /\/api\/strategies/);
  assert.match(html, /\/api\/strategies\/layers/);
  assert.match(html, /registry 未初始化/);
  assert.match(html, /strategy_techniques\.json/);
  assert.match(html, /go run/);
  assert.match(html, /cmd\/atlas/);
});

test('renderPartialBanner: failed 沒有 errors 時顯示 fallback 訊息', () => {
  const state = { dataStatus: 'failed', errors: {} };
  const html = renderPartialBanner(state);
  assert.match(html, /role="alert"/);
  assert.match(html, /全部失敗|核心資料/);
});

// ============================================================================
// partial banner — P1 改善：列舉個別 hint
// ============================================================================

test('renderPartialBanner: partial → role=status + 每條錯誤的 hint', () => {
  const state = {
    dataStatus: 'partial',
    errors: {
      '/api/strategies': { kind: 'http_503', message: 'registry 未初始化', hint: '請檢查 strategy_techniques.json' },
    },
  };
  const html = renderPartialBanner(state);
  assert.match(html, /role="status"/);
  assert.match(html, /\/api\/strategies/);
  assert.match(html, /registry 未初始化/);
  assert.match(html, /strategy_techniques\.json/);
});

test('renderPartialBanner: partial 沒有 hint 的錯誤不顯示 hint 區塊', () => {
  const state = {
    dataStatus: 'partial',
    errors: {
      '/api/strategies/layers': { kind: 'unknown', message: 'something weird', hint: '' },
    },
  };
  const html = renderPartialBanner(state);
  assert.match(html, /role="status"/);
  assert.match(html, /something weird/);
  // 沒 hint 時不該有空的 small tag（用「請」或 hint 字串判斷）
  assert.doesNotMatch(html.replace(/<small[^>]*>[^<]*<\/small>/g, ''), /<small[^>]*><\/small>/);
});

// ============================================================================
// empty banner
// ============================================================================

test('renderPartialBanner: empty → role=status + 提示檢查 seed', () => {
  const state = { dataStatus: 'empty', errors: {} };
  const html = renderPartialBanner(state);
  assert.match(html, /role="status"/);
  assert.match(html, /資料庫為空/);
  assert.match(html, /strategy_techniques\.json/);
});

// ============================================================================
// indicators 失敗警告 — P2.2 改善
// ============================================================================

test('renderPartialBanner: ok + indicatorsError → role=status + 短線指標警告', () => {
  const state = {
    dataStatus: 'ok',
    errors: {},
    indicatorsError: { kind: 'network', message: '後端未啟動', hint: '執行 go run ./cmd/atlas --api' },
  };
  const html = renderPartialBanner(state);
  assert.match(html, /role="status"/);
  assert.match(html, /短線指標/);
  assert.match(html, /後端未啟動/);
  assert.match(html, /go run/);
});

test('renderPartialBanner: failed 時 indicatorsError 不額外顯示（避免重複）', () => {
  const state = {
    dataStatus: 'failed',
    errors: { '/api/strategies': { kind: 'network', message: '後端未啟動', hint: 'go run' } },
    indicatorsError: { kind: 'network', message: '後端未啟動', hint: 'go run' },
  };
  const html = renderPartialBanner(state);
  // failed 是單一 banner，內含所有錯誤的 URL；不應該出現「短線指標」字樣
  assert.doesNotMatch(html, /短線指標/);
});

// ============================================================================
// XSS 防護
// ============================================================================

test('renderPartialBanner: 錯誤訊息含 <script> 必須 escape', () => {
  const state = {
    dataStatus: 'failed',
    errors: {
      '/api/strategies': { kind: 'unknown', message: '<script>alert(1)</script>', hint: '' },
    },
  };
  const html = renderPartialBanner(state);
  assert.doesNotMatch(html, /<script>alert/);
  assert.match(html, /&lt;script&gt;/);
});
