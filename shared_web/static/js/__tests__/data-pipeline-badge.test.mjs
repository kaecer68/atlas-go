// shared_web/static/js/__tests__/data-pipeline-badge.test.mjs
//
// PR-6 (data-pipeline 詞彙對齊) 回歸測試。
//
// 背景: 後端 data_pipeline.go 的 status 契約是 ok/warn/error/unknown，
// 但前端 STATUS_BADGE 只認 fresh/stale/error/paused，且未知狀態 fallback
// 到 STATUS_BADGE.stale → 所有正常來源 (ok) 都被誤標「延遲」。
// 修復: STATUS_BADGE 直接對齊後端 enum (ok→最新 / warn→延遲 / error→異常 /
// unknown→未知)，fallback 改為 unknown，未知狀態顯示「未知」而非「延遲」。
//
// 驗證:
//   1. 8 個來源全部 status=ok → 頁面顯示「最新」且完全不見「延遲」
//   2. warn/error/unknown 各映射到 延遲/異常/未知
//   3. 未定義的 status 不再 fallback 到 stale(延遲)，而是「未知」
//   4. 原始碼層級: STATUS_BADGE 含 ok/warn/unknown，且無 `|| STATUS_BADGE.stale`
//
// 執行: node --test shared_web/static/js/__tests__/data-pipeline-badge.test.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = resolve(__dirname, '../../../../');
const DATACHANNELS_SRC = readFileSync(
  resolve(REPO_ROOT, 'shared_web/static/js/pages/datachannels.js'),
  'utf8'
);

// ============================================================================
// 最小 DOM stub（renderDataPipeline 只用到 #dataPipelineContent 的 classList/innerHTML）
// ============================================================================

class FakeClassList {
  constructor() { this._set = new Set(); }
  add(...cls) { cls.forEach(c => this._set.add(c)); }
  remove(...cls) { cls.forEach(c => this._set.delete(c)); }
  contains(c) { return this._set.has(c); }
}

const pipelineEl = {
  innerHTML: '',
  classList: new FakeClassList(),
};

// escapeHtml 用 document.createElement('div').textContent/innerHTML 脫逸
globalThis.document = {
  getElementById(id) {
    if (id === 'dataPipelineContent') return pipelineEl;
    return null;
  },
  createElement(tag) {
    return {
      tagName: String(tag).toUpperCase(),
      _text: '',
      get textContent() { return this._text; },
      set textContent(v) { this._text = String(v); },
      get innerHTML() {
        // 簡單跳脫，與瀏覽器行為一致（本測試只驗證標籤/文字，不需完整 HTML 序列化）
        return this._text
          .replace(/&/g, '&amp;')
          .replace(/</g, '&lt;')
          .replace(/>/g, '&gt;');
      },
    };
  },
};

const { renderDataPipeline } = await import('../pages/datachannels.js');

function renderSources(statuses) {
  const sources = statuses.map((s, i) => ({
    source_id: 'src_' + i,
    producer: 'producer_' + i,
    consumer: 'consumer_' + i,
    last_produced: '2026-08-17T00:00:00Z',
    last_consumed: '2026-08-17T00:00:01Z',
    status: s,
    lag_human: '-',
    file_path: '/tmp/x',
  }));
  renderDataPipeline({ sources });
  return pipelineEl.innerHTML;
}

function countBadge(html, cls, label) {
  return (html.match(new RegExp(cls + '">' + label, 'g')) || []).length;
}

test('8 個來源 status=ok → 全部顯示「最新」badge，無任何「延遲」badge', () => {
  const html = renderSources(['ok', 'ok', 'ok', 'ok', 'ok', 'ok', 'ok', 'ok']);
  assert.equal(countBadge(html, 'tier-badge--bullish', '最新'), 8, '8 個 ok 來源都應標「最新」');
  assert.equal(countBadge(html, 'tier-badge--warn', '延遲'), 0, 'ok 來源不得再被誤標為「延遲」');
});

test('STATUS_BADGE 對齊後端契約: ok→最新 / warn→延遲 / error→異常 / unknown→未知', () => {
  const html = renderSources(['ok', 'warn', 'error', 'unknown']);
  assert.equal(countBadge(html, 'tier-badge--bullish', '最新'), 1, 'ok → 最新');
  assert.equal(countBadge(html, 'tier-badge--warn', '延遲'), 1, 'warn → 延遲');
  assert.equal(countBadge(html, 'tier-badge--bearish', '異常'), 1, 'error → 異常');
  assert.equal(countBadge(html, 'tier-badge--neutral', '未知'), 1, 'unknown → 未知');
});

test('未定義 status 不再 fallback 到 stale(延遲)，而是顯示「未知」', () => {
  const html = renderSources(['bogus']);
  assert.equal(countBadge(html, 'tier-badge--neutral', '未知'), 1, '未知 status 應顯示「未知」');
  assert.equal(countBadge(html, 'tier-badge--warn', '延遲'), 0, '未知 status 不得誤標為「延遲」');
});

test('原始碼層級: STATUS_BADGE 含 ok/warn/unknown，且無 `|| STATUS_BADGE.stale` fallback', () => {
  const badgeBlock = DATACHANNELS_SRC.slice(
    DATACHANNELS_SRC.indexOf('const STATUS_BADGE'),
    DATACHANNELS_SRC.indexOf('const rows = sources.map')
  );
  assert.ok(badgeBlock.includes("ok:"), 'STATUS_BADGE 應含 ok key');
  assert.ok(badgeBlock.includes("warn:"), 'STATUS_BADGE 應含 warn key');
  assert.ok(badgeBlock.includes("unknown:"), 'STATUS_BADGE 應含 unknown key');
  assert.ok(!DATACHANNELS_SRC.includes('|| STATUS_BADGE.stale'),
    '不得再有 `|| STATUS_BADGE.stale` 誤標 fallback');
  assert.ok(DATACHANNELS_SRC.includes('|| STATUS_BADGE.unknown'),
    'fallback 應改為 STATUS_BADGE.unknown');
});
