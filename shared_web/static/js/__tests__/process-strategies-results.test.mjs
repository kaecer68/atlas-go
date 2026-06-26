// 對應 processStrategiesResults 的單元測試。
// 涵蓋 5 種失敗模式（A/B/C/D/E）+ indicators 失敗 + schema mismatch。
//
// 執行：node --test web/static/js/__tests__/process-strategies-results.test.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { processStrategiesResults } from '../pages/strategies.js';

const okStrategies = { strategies: [{ id: 'L1-A', layer: 'L1' }] };
const okLayers = { layers: [{ layer: 'L1', count: 1 }] };
const okChain = { core_indicators: { foreign_capital_net_twd: 1e9 } };

function fulfilled(value) {
  return { status: 'fulfilled', value };
}

function rejected(reason) {
  return { status: 'rejected', reason };
}

function networkErr(url) {
  const e = new TypeError('Failed to fetch');
  return e;
}

// ============================================================================
// Happy path
// ============================================================================

test('processStrategiesResults: 全部 OK → dataStatus=ok', () => {
  const r = processStrategiesResults([
    fulfilled(okStrategies),
    fulfilled(okLayers),
    fulfilled(okChain),
  ]);
  assert.equal(r.dataStatus, 'ok');
  assert.equal(r.strategies.length, 1);
  assert.equal(r.layers.length, 1);
  assert.equal(r.indicatorsError, null);
  assert.deepEqual(r.errors, {});
});

test('processStrategiesResults: strategies=[] → dataStatus=empty', () => {
  const r = processStrategiesResults([
    fulfilled({ strategies: [] }),
    fulfilled(okLayers),
    fulfilled(okChain),
  ]);
  assert.equal(r.dataStatus, 'empty');
  assert.equal(r.strategies.length, 0);
});

// ============================================================================
// 模式 A: backend 完全不可達（TypeError）
// ============================================================================

test('processStrategiesResults: strategies rejected TypeError → partial (1 core failure)', () => {
  const r = processStrategiesResults([
    rejected(networkErr('/api/strategies')),
    fulfilled(okLayers),
    fulfilled(okChain),
  ]);
  assert.equal(r.dataStatus, 'partial');
  assert.equal(r.strategies.length, 0);
  assert.equal(r.errors['/api/strategies'].kind, 'network');
});

// ============================================================================
// 模式 C: 部分失敗
// ============================================================================

test('processStrategiesResults: layers rejected → partial', () => {
  const r = processStrategiesResults([
    fulfilled(okStrategies),
    rejected(networkErr('/api/strategies/layers')),
    fulfilled(okChain),
  ]);
  assert.equal(r.dataStatus, 'partial');
  assert.equal(r.errors['/api/strategies/layers'].kind, 'network');
  assert.equal(r.strategies.length, 1);
});

test('processStrategiesResults: strategies + layers 都 reject → failed (2 core failures)', () => {
  const r = processStrategiesResults([
    rejected(networkErr('/api/strategies')),
    rejected(networkErr('/api/strategies/layers')),
    fulfilled(okChain),
  ]);
  assert.equal(r.dataStatus, 'failed');
});

// ============================================================================
// 模式 B: HTTP 503
// ============================================================================

test('processStrategiesResults: HTTP 503 → http_503 kind + partial', () => {
  const e503 = new Error('HTTP 503');
  e503.status = 503;
  const r = processStrategiesResults([
    rejected(e503),
    fulfilled(okLayers),
    fulfilled(okChain),
  ]);
  assert.equal(r.dataStatus, 'partial');
  assert.equal(r.errors['/api/strategies'].kind, 'http_503');
});

// ============================================================================
// 模式 E: schema mismatch（200 但缺少預期 key）
// ============================================================================

test('processStrategiesResults: strategies 缺少 .strategies key → schema error', () => {
  const r = processStrategiesResults([
    fulfilled({ data: okStrategies.strategies }), // schema 改成 data
    fulfilled(okLayers),
    fulfilled(okChain),
  ]);
  assert.equal(r.errors['/api/strategies'].kind, 'schema');
  assert.equal(r.dataStatus, 'partial'); // 1 core failure
  assert.equal(r.strategies.length, 0);
});

test('processStrategiesResults: layers 缺少 .layers key → schema error', () => {
  const r = processStrategiesResults([
    fulfilled(okStrategies),
    fulfilled({ counts: [] }), // schema 改成 counts
    fulfilled(okChain),
  ]);
  assert.equal(r.errors['/api/strategies/layers'].kind, 'schema');
  assert.equal(r.dataStatus, 'partial');
});

test('processStrategiesResults: strategies + layers 都 schema 錯 → failed', () => {
  const r = processStrategiesResults([
    fulfilled({ data: [] }),
    fulfilled({ counts: [] }),
    fulfilled(okChain),
  ]);
  assert.equal(r.dataStatus, 'failed');
  assert.equal(r.errors['/api/strategies'].kind, 'schema');
  assert.equal(r.errors['/api/strategies/layers'].kind, 'schema');
});

// ============================================================================
// decision-chain 失敗處理
// ============================================================================

test('processStrategiesResults: decision-chain 失敗但核心 OK → ok + indicatorsError', () => {
  const r = processStrategiesResults([
    fulfilled(okStrategies),
    fulfilled(okLayers),
    rejected(networkErr('/api/dashboard/decision-chain')),
  ]);
  assert.equal(r.dataStatus, 'ok');
  assert.ok(r.indicatorsError);
  assert.equal(r.indicatorsError.kind, 'network');
  assert.equal(r.coreIndicators, null);
});

test('processStrategiesResults: decision-chain 失敗 + 核心失敗 → partial/failed，不覆蓋 indicatorsError', () => {
  const r = processStrategiesResults([
    fulfilled(okStrategies),
    rejected(networkErr('/api/strategies/layers')),
    rejected(networkErr('/api/dashboard/decision-chain')),
  ]);
  assert.equal(r.dataStatus, 'partial');
  assert.equal(r.indicatorsError, null); // 不該遮蓋 partial banner
});

test('processStrategiesResults: decision-chain 無 .core_indicators → coreIndicators=null', () => {
  const r = processStrategiesResults([
    fulfilled(okStrategies),
    fulfilled(okLayers),
    fulfilled({ unrelated_field: 42 }),
  ]);
  assert.equal(r.dataStatus, 'ok');
  assert.equal(r.coreIndicators, null);
  assert.equal(r.indicatorsError, null);
});

// ============================================================================
// 結構完整性
// ============================================================================

test('processStrategiesResults: 回傳永遠包含所有 STATE 欄位', () => {
  const r = processStrategiesResults([
    rejected(networkErr('/x')),
    rejected(networkErr('/y')),
    rejected(networkErr('/z')),
  ]);
  assert.ok('strategies' in r);
  assert.ok('layers' in r);
  assert.ok('coreIndicators' in r);
  assert.ok('errors' in r);
  assert.ok('indicatorsError' in r);
  assert.ok('dataStatus' in r);
  assert.equal(typeof r.strategies, 'object');
  assert.equal(Array.isArray(r.strategies), true);
  assert.equal(Array.isArray(r.layers), true);
});
