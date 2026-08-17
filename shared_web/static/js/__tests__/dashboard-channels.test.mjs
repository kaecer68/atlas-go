// shared_web/static/js/__tests__/dashboard-channels.test.mjs
//
// PR-5: dashboard「信息通道預警」widget 口徑統一測試。
//
// 驗證:
//   1. widget 改吃 /api/dashboard/data-channels (renderOverview 第 7 參數
//      dataChannels, 40 通道) — twse_etf status=error 在 widget 可見
//      (system-health 只涵蓋 22 通道的 18 通道盲區消除)
//   2. inactive 通道 (TEJ) 以「未啟用」呈現, 不計入降級/異常
//   3. widget 的 alert 清單與 data-channels alerts 一致 (由 alerts 渲染)
//   4. null-guard: dataChannels 為 null (fetchWithRetry 失敗) 不 crash
//
// 執行: node --test shared_web/static/js/__tests__/dashboard-channels.test.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';

// ============================================================================
// DOM / browser stubs (與 home.test.mjs 相同模式)
// ============================================================================

const elements = new Map();

function createElement(id) {
  const listeners = [];
  const childMap = new Map();
  const el = {
    id,
    innerHTML: '',
    textContent: '',
    style: {},
    _listeners: listeners,
    _childMap: childMap,
    addEventListener(type, fn) { listeners.push({ type, fn }); },
    dispatchEvent(ev) {
      listeners
        .filter(l => l.type === ev.type)
        .forEach(l => l.fn(ev));
      return true;
    },
    setAttribute() {},
    getAttribute() { return null; },
    classList: {
      add() {},
      remove() {},
      contains() { return false; },
    },
    querySelector(sel) {
      if (sel.startsWith('#')) {
        const childId = sel.slice(1);
        if (!childMap.has(childId)) {
          const child = createElement(childId);
          childMap.set(childId, child);
        }
        return childMap.get(childId);
      }
      return null;
    },
    appendChild(child) {
      if (child && child.id) childMap.set(child.id, child);
      return child;
    },
  };
  elements.set(id, el);
  return el;
}

global.document = {
  getElementById(id) {
    if (!elements.has(id)) createElement(id);
    return elements.get(id);
  },
  createElement(tag) { return createElement(`create-${tag}`); },
  querySelector() { return null; },
  querySelectorAll() { return []; },
  addEventListener() {},
  removeEventListener() {},
  body: {
    appendChild(child) {
      if (child && child.id) elements.set(child.id, child);
      return child;
    },
    addEventListener() {},
  },
};

global.window = {
  matchMedia(query) { return { matches: false, media: query }; },
  switchPage() {},
  openKpiHelp() {},
  dispatchEvent() {},
  addEventListener() {},
  CustomEvent: class CustomEvent extends Event {
    constructor(type, init) {
      super(type, init);
      this.detail = init?.detail ?? null;
    }
  },
};

const { renderOverview } = await import('../pages/dashboard.js');

// 呼叫 renderOverview 並回傳 overviewRisk (信息通道預警 widget 所在 grid)
function renderWidget(dataChannels) {
  renderOverview(
    {},                                   // data (health)
    { scorecards: [] },                   // agentsData
    {},                                   // inbox
    null,                                 // overlap
    {},                                   // narrativeEvents
    { score: 0, regime: 'NEUTRAL' },      // stress
    dataChannels,                         // 7th param: /api/dashboard/data-channels
    null                                  // capitalPhase
  );
  const riskEl = elements.get('overviewRisk');
  return riskEl ? riskEl.innerHTML : '';
}

// ============================================================================
// 1. 40 通道覆蓋 — twse_etf error 在 widget 可見 (18 通道盲區消除)
// ============================================================================

test('widget: twse_etf error (data-channels 40 通道) 顯示為異常', () => {
  const html = renderWidget({
    channels: [
      { channel_id: 'us_yahoo', status: 'ok', status_text: '正常' },
      { channel_id: 'twse_etf', status: 'error', status_text: '異常' },
    ],
    alerts: [{ channel_id: 'twse_etf', status: 'error', error: 'fetch failed' }],
  });

  assert.ok(html.includes('1 筆異常'), 'KPI 應顯示 1 筆異常');
  assert.ok(html.includes('twse_etf'), 'twse_etf 應在 widget 中可見');
  assert.ok(html.includes('發生異常'), 'alert 應標示發生異常');
  assert.ok(!html.includes('所有通道正常'), '有異常時不應顯示所有通道正常');
});

// ============================================================================
// 2. inactive (TEJ) 顯示「未啟用」而非「降級」
// ============================================================================

test('widget: TEJ inactive 顯示未啟用, 不計入降級/異常', () => {
  const html = renderWidget({
    channels: [
      { channel_id: 'us_yahoo', status: 'ok', status_text: '正常' },
      { channel_id: 'tej', status: 'inactive', status_text: '未啟用' },
    ],
    alerts: [],
  });

  assert.ok(html.includes('未啟用'), 'inactive 通道應以未啟用呈現');
  assert.ok(html.includes('text-lg">正常</div>'), 'KPI 值應為正常 (未啟用不計入)');
  assert.ok(!html.includes('降級'), 'inactive 不應顯示為降級');
  assert.ok(!html.includes('異常'), 'inactive 不應顯示為異常');
});

// ============================================================================
// 3. widget alert 清單與 data-channels alerts 一致
// ============================================================================

test('widget: alert 清單由 dataChannels.alerts 渲染且與之相符', () => {
  const alerts = [
    { channel_id: 'twse_etf', status: 'error', error: 'boom' },
    { channel_id: 'fugle', status: 'warn', error: 'stale' },
  ];
  const html = renderWidget({
    channels: [
      { channel_id: 'twse_etf', status: 'error', status_text: '異常' },
      { channel_id: 'fugle', status: 'warn', status_text: '待更新' },
    ],
    alerts,
  });

  assert.ok(html.includes('twse_etf 發生異常'), 'error alert 應渲染');
  assert.ok(html.includes('fugle 資料待更新'), 'warn alert 應渲染');
  assert.equal((html.match(/twse_etf/g) || []).length >= 1, true);
  assert.ok(html.includes('2 筆待更新') || html.includes('1 筆異常'), 'KPI 顯示異常/待更新計數');
});

// ============================================================================
// 4. null-guard — dataChannels 為 null/undefined 不 crash
// ============================================================================

test('widget: dataChannels 為 null (fetch 失敗) 不 crash 且顯示載入失敗', () => {
  assert.doesNotThrow(() => renderWidget(null), 'null dataChannels 不應 throw');
  const html = renderWidget(null);
  assert.ok(html.includes('通道狀態載入失敗'), '應顯示通道狀態載入失敗');
});

test('widget: dataChannels 為 undefined 不 crash', () => {
  assert.doesNotThrow(() => renderWidget(undefined));
});

test('widget: 全部通道正常時顯示所有通道正常', () => {
  const html = renderWidget({
    channels: [
      { channel_id: 'us_yahoo', status: 'ok', status_text: '正常' },
    ],
    alerts: [],
  });
  assert.ok(html.includes('所有通道正常'));
  assert.ok(html.includes('text-lg">正常</div>'));
});
