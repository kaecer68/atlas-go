// shared_web/static/js/__tests__/alerts-channel-health.test.mjs
//
// fix/20260924-channel-status-truth — alerts 頁「資料通道健康」摘要。
//
// 修復前: `status !== 'ok' && status !== 'inactive'` 一律掛紅色 err badge，
// 且統計只認 ok/warn/error → stale（資料過期，即最後一次成功抓取超過合約
// freshness window）被畫成紅色「異常」，degraded / unknown 完全不出現。
//
// 驗證:
//   1. stale → amber warn badge + 「資料過期」，不是 err(紅)
//   2. stale / degraded / unknown 都計入非正常（ok 分母不包含它們）
//   3. degraded → 「降級」；unknown → muted「未知」
//   4. last_error 成為 badge tooltip（derived verdict 的原因字串就放在 last_error）
//   5. error 仍是紅色「異常」；inactive 不掛 badge
//
// 執行: node --test shared_web/static/js/__tests__/alerts-channel-health.test.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';

const els = new Map();
function makeEl(id) {
  const el = {
    id,
    innerHTML: '',
    textContent: '',
    style: {},
    classList: { add() {}, remove() {}, contains() { return false; } },
    addEventListener() {},
    appendChild() {},
    querySelector() { return null; },
  };
  els.set(id, el);
  return el;
}

const originalDocument = globalThis.document;
const originalFetch = globalThis.fetch;

function installDom() {
  globalThis.document = {
    getElementById(id) {
      if (!els.has(id)) makeEl(id);
      return els.get(id);
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
    body: { appendChild() {}, addEventListener() {} },
  };
}

function okJson(body) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  });
}

async function renderHealth(channels) {
  installDom();
  globalThis.fetch = async function (url) {
    if (url === '/api/dashboard/channel-health') {
      return okJson({ channels, updated_at: '2026-09-24T12:00:00Z' });
    }
    if (String(url).startsWith('/api/alerts?status=triggered')) return okJson({ alerts: [] });
    if (url === '/api/alerts/stats') return okJson({ triggered: 0 });
    return okJson({});
  };
  const { loadAlerts } = await import('../pages/alerts.js');
  await loadAlerts();
  // renderHealthSummary 在 .then 內渲染，等它落地
  for (let i = 0; i < 5; i++) await new Promise(r => setTimeout(r, 5));
  return els.get('alertHealthSummary').innerHTML;
}

const CH = (id, status, extra) => Object.assign({ channel_id: id, status }, extra || {});

test('stale 通道 → amber warn badge +「資料過期」，不是紅色異常', async () => {
  const html = await renderHealth([
    CH('us_yahoo', 'ok'),
    CH('twse_oddlot', 'stale', { last_error: '資料已 17 天 6 小時 未更新，超過合約更新窗口 48 小時' }),
  ]);
  assert.ok(html.includes('1/2 正常'), 'stale 不得計入正常，實際: ' + html);
  assert.ok(html.includes('1 資料過期'), '應顯示資料過期統計，實際: ' + html);
  assert.ok(html.includes('badge warn') && html.includes('twse_oddlot 資料過期'), '實際: ' + html);
  assert.ok(!html.includes('badge err'), 'stale 不得使用紅色 err badge，實際: ' + html);
  assert.ok(html.includes('超過合約更新窗口 48 小時'), 'last_error 應成為 tooltip');
});

test('degraded 通道 →「降級」amber，計入非正常', async () => {
  const html = await renderHealth([
    CH('us_yahoo', 'ok'),
    CH('fugle', 'degraded'),
  ]);
  assert.ok(html.includes('1/2 正常'));
  assert.ok(html.includes('1 降級'), '實際: ' + html);
  assert.ok(html.includes('fugle 降級'));
  assert.ok(!html.includes('badge err'));
});

test('未知狀態 → muted「未知」badge，且不算正常', async () => {
  const html = await renderHealth([
    CH('us_yahoo', 'ok'),
    CH('mystery', 'no-such-status'),
  ]);
  assert.ok(html.includes('1/2 正常'), '實際: ' + html);
  assert.ok(html.includes('badge muted') && html.includes('mystery 未知'), '實際: ' + html);
});

test('error 仍為紅色「異常」；inactive 不掛 badge', async () => {
  const html = await renderHealth([
    CH('us_yahoo', 'ok'),
    CH('twse_etf', 'error'),
    CH('tej', 'inactive'),
  ]);
  assert.ok(html.includes('1 異常'), '實際: ' + html);
  assert.ok(html.includes('badge err') && html.includes('twse_etf 異常'));
  assert.ok(!html.includes('tej'), 'inactive 不應出現在需注意清單，實際: ' + html);
});

test('全部正常時只顯示 2/2 正常（與修復前一致：不需注意時不渲染 badge 列）', async () => {
  const html = await renderHealth([CH('us_yahoo', 'ok'), CH('finmind', 'ok')]);
  assert.ok(html.includes('2/2 正常'));
  assert.ok(!html.includes('badge '), '無需注意通道時不應有 badge，實際: ' + html);
  for (const label of ['資料過期', '降級', '警告', '異常']) {
    assert.ok(!html.includes(label), '實際: ' + html);
  }
});

test('清理：還原 document / fetch', () => {
  if (originalDocument === undefined) delete globalThis.document;
  else globalThis.document = originalDocument;
  globalThis.fetch = originalFetch;
  assert.ok(true);
});
