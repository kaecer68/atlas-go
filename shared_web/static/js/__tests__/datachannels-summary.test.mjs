// shared_web/static/js/__tests__/datachannels-summary.test.mjs
//
// fix/20260924-channel-status-truth — /admin/datachannels 摘要統計 + 狀態徽章。
//
// 修復前: 「正常 = total - error - warn」，且 statusLight/statusClass 只認
// ok/warn/error。實證 bug: twse_oddlot 的 record 是 ok、但 17 天沒更新
// (後端 derived status=stale、「資料過期」)，本頁仍顯示「正常」。
//
// 驗證:
//   1. stale / degraded 各自計入「資料過期/降級」統計，不計入「正常」
//   2. stale 徽章 = warn(amber) + 「資料過期」，不是 err(紅)
//   3. inactive → muted「未啟用」；未知狀態 → muted「未知」，且不被算進正常
//   4. 「需要關注的通道」用 SSOT label（stale → 資料過期、非紅）
//
// 執行: node --test shared_web/static/js/__tests__/datachannels-summary.test.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';

// ============================================================================
// DOM stub（renderDataChannels 只用到 #dataChannels 的 classList + innerHTML，
// 以及 escapeHtml 用的 document.createElement('div')）
// ============================================================================

const elements = new Map();

function makeEl(id) {
  const el = {
    id,
    innerHTML: '',
    textContent: '',
    style: {},
    classList: { add() {}, remove() {}, contains() { return false; } },
    addEventListener() {},
    querySelector() { return null; },
  };
  elements.set(id, el);
  return el;
}

globalThis.document = {
  getElementById(id) {
    if (!elements.has(id)) makeEl(id);
    return elements.get(id);
  },
  createElement() {
    return {
      _t: '',
      get textContent() { return this._t; },
      set textContent(v) { this._t = String(v); },
      get innerHTML() {
        return this._t.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
      },
    };
  },
  querySelector() { return null; },
  querySelectorAll() { return []; },
  addEventListener() {},
};

const { renderDataChannels } = await import('../pages/datachannels.js');

function channel(id, status, statusText, extra) {
  return Object.assign({
    channel_id: id,
    country: '台灣',
    platform: id + ' 平台',
    api_format: 'REST JSON',
    path: '/x',
    storage: 'db',
    status,
    status_text: statusText,
    updated_at: '2026-09-24 10:00:00',
    enabled: true,
  }, extra || {});
}

function render(channels, alerts) {
  renderDataChannels({ channels, alerts: alerts || [], generated: '2026-09-24 12:00:00' });
  return elements.get('dataChannels').innerHTML;
}

// 把 control-summary 的 tile 抓成 { 標籤: 數值 } map
function tiles(html) {
  const out = {};
  const re = /<div class="label">([^<]+)<\/div>\s*<div class="value"[^>]*>([^<]*)<\/div>/g;
  let m;
  while ((m = re.exec(html)) !== null) out[m[1]] = m[2];
  return out;
}

// ============================================================================
// 1. stale / degraded 不計入「正常」
// ============================================================================

test('stale 通道：不計入正常，改計入「資料過期/降級」', () => {
  const html = render([
    channel('us_yahoo', 'ok', '正常'),
    channel('twse_oddlot', 'stale', '資料過期'),
  ]);
  const t = tiles(html);
  assert.equal(t['總通道'], '2');
  assert.equal(t['正常'], '1', '只有 ok 通道算正常，實際: ' + JSON.stringify(t));
  assert.equal(t['資料過期/降級'], '1');
  assert.equal(t['異常'], '0');
  assert.equal(t['待更新'], '0');
});

test('degraded 通道：不計入正常，計入「資料過期/降級」', () => {
  const html = render([
    channel('us_yahoo', 'ok', '正常'),
    channel('twse_oddlot', 'degraded', '降級'),
  ]);
  const t = tiles(html);
  assert.equal(t['正常'], '1');
  assert.equal(t['資料過期/降級'], '1');
});

test('8 個 ok 通道 → 全部算正常（回歸：不得因新統計而少算）', () => {
  const channels = [];
  for (let i = 0; i < 8; i++) channels.push(channel('ch' + i, 'ok', '正常'));
  const t = tiles(render(channels));
  assert.equal(t['總通道'], '8');
  assert.equal(t['正常'], '8');
  assert.equal(t['資料過期/降級'], '0');
  assert.equal(t['異常'], '0');
});

// ============================================================================
// 2. 徽章顏色 / 文字
// ============================================================================

test('stale 徽章 = warn(amber) + 「資料過期」，不是 err(紅)', () => {
  const html = render([channel('twse_oddlot', 'stale', '資料過期')]);
  assert.ok(html.includes('<span class="badge warn">資料過期</span>'), '實際: ' + html.slice(0, 400));
  assert.ok(!html.includes('<span class="badge err">資料過期</span>'), 'stale 不得用紅 badge');
  assert.ok(html.includes('var(--status-stale)'), 'stale 燈號應使用 --status-stale token');
});

test('degraded 徽章 = warn(amber) + 「降級」', () => {
  const html = render([channel('twse_oddlot', 'degraded', '降級')]);
  assert.ok(html.includes('<span class="badge warn">降級</span>'), '實際: ' + html.slice(0, 400));
});

test('error 徽章仍是 err(紅)（不得被 SSOT 改成 amber）', () => {
  const html = render([channel('twse_etf', 'error', '異常')]);
  assert.ok(html.includes('<span class="badge err">異常</span>'));
});

test('inactive → muted「未啟用」；未知狀態 → muted「未知」', () => {
  const html = render([
    channel('tej', 'inactive', '未啟用'),
    channel('mystery', 'no-such-status', ''),
  ]);
  assert.ok(html.includes('<span class="badge muted">未啟用</span>'), '實際: ' + html.slice(0, 400));
  assert.ok(html.includes('<span class="badge muted">未知</span>'), '缺 status_text 時 fallback 到 SSOT label');
});

// ============================================================================
// 3. inactive / unknown 不計入正常，且明示未計入
// ============================================================================

test('inactive / 未知狀態既不計入正常也不計入異常，並在頁面明示', () => {
  const t = tiles(render([
    channel('us_yahoo', 'ok', '正常'),
    channel('tej', 'inactive', '未啟用'),
    channel('mystery', 'no-such-status', '未知'),
  ]));
  assert.equal(t['正常'], '1', '實際: ' + JSON.stringify(t));
  assert.equal(t['異常'], '0');
  assert.equal(t['待更新'], '0');
  const html = render([
    channel('us_yahoo', 'ok', '正常'),
    channel('tej', 'inactive', '未啟用'),
    channel('mystery', 'no-such-status', '未知'),
  ]);
  assert.ok(html.includes('未啟用 1'), '應明示未啟用 1，實際: ' + html.slice(0, 400));
  assert.ok(html.includes('未知 1'), '應明示未知 1');
});

// ============================================================================
// 4. 「需要關注的通道」用 SSOT label
// ============================================================================

test('需要關注的通道：stale alert 顯示「資料過期」且用 amber（非紅）', () => {
  const html = render(
    [channel('twse_oddlot', 'stale', '資料過期')],
    [{ channel_id: 'twse_oddlot', status: 'stale', error: '' }],
  );
  assert.ok(html.includes('需要關注的通道'));
  assert.ok(html.includes('twse_oddlot</strong>：<span style="color:var(--warn)">資料過期</span>'), '實際: ' + html.slice(-600));
  assert.ok(html.includes('border-left:3px solid var(--warn)'), 'stale-only 區塊不得用紅色框，實際: ' + html.slice(-600));
  assert.ok(!html.includes('border-left:3px solid var(--color-danger)'), 'stale 不得觸發紅色區塊');
});

test('需要關注的通道：error alert 仍是紅色「異常」', () => {
  const html = render(
    [channel('twse_etf', 'error', '異常')],
    [{ channel_id: 'twse_etf', status: 'error', error: '' }],
  );
  assert.ok(html.includes('<span style="color:var(--color-danger)">異常</span>'), '實際: ' + html.slice(-600));
});

test('需要關注的通道：後端有 error 文字時以 error 文字優先', () => {
  const html = render(
    [channel('twse_etf', 'error', '異常')],
    [{ channel_id: 'twse_etf', status: 'error', error: 'fetch failed' }],
  );
  assert.ok(html.includes('fetch failed'));
});

test('需要關注的通道：混有 error 時區塊維持紅色（最嚴重者決定）', () => {
  const html = render(
    [channel('twse_etf', 'error', '異常'), channel('twse_oddlot', 'stale', '資料過期')],
    [
      { channel_id: 'twse_etf', status: 'error', error: '' },
      { channel_id: 'twse_oddlot', status: 'stale', error: '' },
    ],
  );
  assert.ok(html.includes('border-left:3px solid var(--color-danger)'), '實際: ' + html.slice(-700));
  assert.ok(html.includes('<span style="color:var(--warn)">資料過期</span>'), 'stale 該行仍是 amber');
  assert.ok(html.includes('<span style="color:var(--color-danger)">異常</span>'), 'error 該行仍是紅');
});
