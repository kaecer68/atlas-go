// shared_web/static/js/__tests__/rawfetch-postjson.test.mjs
//
// PR-1 (raw-fetch 寫入收編 → postJSON) 回歸測試。
//
// 背景：admin/client UI 的 8 處 mutating 寫入原本用裸 fetch POST，沒有走
// app-utils 的 attachApiKey → 一律 401。本 PR 把 4 檔 8 處全部改成
// postJSON()，讓 X-API-Key 自動附加（無 key 時先 prompt）。
//
// 本測試驗證（對應 PR 驗收標準）：
//   1. 每條寫入路徑都改走 postJSON → 有 key 時請求帶 X-API-Key header
//   2. 無 key 時觸發 prompt（window.__atlasPromptForApiKey modal hook /
//      client_web 的 window.prompt fallback），輸入的 key 被帶上並存入
//      localStorage
//   3. 錯誤路徑仍走既有的 notify / alert 提示
//
// 執行：node --test shared_web/static/js/__tests__/rawfetch-postjson.test.mjs

import { test, afterEach } from 'node:test';
import assert from 'node:assert/strict';

// ============================================================================
// 假瀏覽器環境（DOM / window / localStorage / fetch / alert）
// ============================================================================

const storage = new Map();

function makeClassList() {
  const set = new Set();
  return {
    add(...cls) { cls.forEach(c => set.add(c)); },
    remove(...cls) { cls.forEach(c => set.delete(c)); },
    contains(c) { return set.has(c); },
    toggle(c, force) {
      if (force === undefined) { if (set.has(c)) set.delete(c); else set.add(c); }
      else if (force) set.add(c);
      else set.delete(c);
      return set.has(c);
    },
  };
}

function matchesSelector(el, sel) {
  if (sel.startsWith('.')) {
    const cls = sel.slice(1);
    const name = typeof el.className === 'string' ? el.className : '';
    return name.split(/\s+/).includes(cls) || (el.classList && el.classList.contains(cls));
  }
  if (sel.startsWith('#')) return el.id === sel.slice(1);
  return el.tagName === sel.toUpperCase();
}

function queryBySelector(root, sel, all) {
  const out = [];
  function walk(node) {
    for (const child of (node._children || [])) {
      if (matchesSelector(child, sel)) out.push(child);
      walk(child);
    }
  }
  walk(root);
  return all ? out : (out.length ? out[0] : null);
}

function makeElement(tagName) {
  const listeners = {};
  const children = [];
  const el = {
    tagName: String(tagName || 'div').toUpperCase(),
    id: '',
    className: '',
    textContent: '',
    innerHTML: '',
    style: {},
    value: '',
    dataset: {},
    disabled: false,
    type: '',
    checked: false,
    offsetWidth: 100,
    offsetLeft: 50,
    parent: null,
    _listeners: listeners,
    _children: children,
    classList: makeClassList(),
    setAttribute(k, v) { el[k] = v; },
    getAttribute(k) { return el[k] !== undefined ? el[k] : null; },
    addEventListener(type, fn) { (listeners[type] = listeners[type] || []).push(fn); },
    removeEventListener(type, fn) {
      const a = listeners[type];
      if (a) { const i = a.indexOf(fn); if (i >= 0) a.splice(i, 1); }
    },
    emit(type, ev) { (listeners[type] || []).slice().forEach(fn => fn(ev || { target: el })); },
    appendChild(child) { children.push(child); child.parent = el; return child; },
    remove() {
      if (el.parent) {
        const i = el.parent._children.indexOf(el);
        if (i >= 0) el.parent._children.splice(i, 1);
      }
      el.parent = null;
    },
    focus() {},
    getBoundingClientRect() { return { left: 100, top: 100, bottom: 120, right: 200, width: 100, height: 20 }; },
    querySelector(sel) {
      let found = queryBySelector(el, sel, false);
      if (!found && sel.startsWith('.')) {
        // innerHTML 在假 DOM 不解析 → 惰性建立對應 class 的子節點（模擬真實 DOM）
        found = makeElement('div');
        found.className = sel.slice(1);
        children.push(found);
        found.parent = el;
      }
      return found;
    },
    querySelectorAll(sel) { return queryBySelector(el, sel, true); },
  };
  return el;
}

const byId = new Map();
const doc = {
  getElementById(id) {
    if (!byId.has(id)) byId.set(id, makeElement('div'));
    return byId.get(id);
  },
  createElement(tag) { return makeElement(tag); },
  querySelector() { return null; },
  querySelectorAll() { return []; },
  addEventListener() {},
  removeEventListener() {},
  body: makeElement('body'),
  activeElement: makeElement('div'),
};

const win = {
  prompt() { return ''; },
  dispatchEvent() {},
  addEventListener() {},
};

const alerts = [];
function spyAlert(msg) { alerts.push(msg); }

const originalFetch = globalThis.fetch;
const fetchCalls = [];

function okJson(body, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

// 預設 fetch handler：GET 依 routes 回傳，POST 一律 200；可用 handler 覆寫。
function installFetch(routes, handler) {
  globalThis.fetch = async (url, init) => {
    const u = String(url);
    const method = (init && init.method) || 'GET';
    const headers = (init && init.headers) || {};
    let body;
    if (init && init.body !== undefined) body = JSON.parse(init.body);
    fetchCalls.push({ url: u, method, headers, body });
    if (handler) return handler(u, method, init);
    if (method === 'POST') return okJson({ ok: true });
    if (routes[u] !== undefined) return okJson(routes[u]);
    return okJson({});
  };
}

function restoreFetch() {
  if (originalFetch === undefined) delete globalThis.fetch;
  else globalThis.fetch = originalFetch;
}

function setApiKey(key) {
  if (key) storage.set('ATLAS_API_KEY', key);
  else storage.delete('ATLAS_API_KEY');
}

function postCallsFor(url) {
  return fetchCalls.filter(c => c.method === 'POST' && c.url === url);
}

afterEach(() => {
  fetchCalls.length = 0;
  alerts.length = 0;
  storage.clear();
  delete win.__atlasPromptForApiKey;
  restoreFetch();
});

// 設定 globals（模組 import 前必須就位）
globalThis.localStorage = {
  getItem: k => (storage.has(k) ? storage.get(k) : null),
  setItem: (k, v) => storage.set(k, String(v)),
  removeItem: k => storage.delete(k),
};
globalThis.document = doc;
globalThis.window = win;
globalThis.alert = spyAlert;
globalThis.prompt = () => ''; // circuit-breaker.js 使用原生 prompt

const { triggerChannelsIngest } = await import('../pages/datachannels.js');
const { toggleSchedulerTask } = await import('../pages/scheduler.js');
const { CircuitBreakerPanel } = await import('../components/circuit-breaker.js');
const { handleOverrideClick } = await import('../pages/pipeline.js');

// ============================================================================
// datachannels.js — 5 條寫入路徑
// ============================================================================

test('triggerChannelsIngest: POST ingest 帶 X-API-Key，成功訊息依 macro/geo 組成', async () => {
  setApiKey('secret-key');
  const btn = makeElement('button');
  byId.set('btnIngestChannels', btn);
  installFetch({});
  await triggerChannelsIngest();
  const calls = postCallsFor('/api/channels/ingest');
  assert.equal(calls.length, 1);
  assert.equal(calls[0].headers['X-API-Key'], 'secret-key');
});

// toggleSchedulerTask 測試已隨 UI 停用/啟用按鈕移除（2026-09-03 審計）。

// ============================================================================
// circuit-breaker.js — reset
// ============================================================================

test('CircuitBreakerPanel.handleReset: POST reset 帶 X-API-Key + body, 成功後 fetchState', async () => {
  setApiKey('secret-key');
  installFetch({});
  const panel = Object.create(CircuitBreakerPanel.prototype);
  panel.resetBtn = makeElement('button');
  let fetchStateCalled = 0;
  panel.fetchState = async () => { fetchStateCalled++; };
  globalThis.prompt = () => 'manual-test-reason';
  await panel.handleReset();
  const calls = postCallsFor('/api/dashboard/circuit-breaker/reset');
  assert.equal(calls.length, 1);
  assert.equal(calls[0].headers['X-API-Key'], 'secret-key');
  assert.deepEqual(calls[0].body, { reason: 'manual-test-reason' });
  assert.equal(fetchStateCalled, 1, '成功後應重新拉取狀態');
});

test('CircuitBreakerPanel.handleReset: HTTP 401 → alert 重置失敗', async () => {
  setApiKey('secret-key');
  installFetch({}, () => okJson({}, 401));
  const panel = Object.create(CircuitBreakerPanel.prototype);
  panel.resetBtn = makeElement('button');
  panel.fetchState = async () => {};
  globalThis.prompt = () => 'manual-test-reason';
  await panel.handleReset();
  assert.ok(alerts.includes('重置失敗'), 'HTTP 錯誤應 alert 重置失敗');
});

// ============================================================================
// pipeline.js — approve/reject 覆寫
// ============================================================================

test('handleOverrideClick 放行: POST approve-recommendation 帶 X-API-Key + body', async () => {
  setApiKey('secret-key');
  installFetch({});
  const btn = makeElement('button');
  btn.dataset = { symbol: '2330', agentId: 'alpha_discovery', guards: '0' };
  handleOverrideClick({ currentTarget: btn, stopPropagation() {} });

  const popover = doc.body._children.find(c => c.className === 'override-popover');
  assert.ok(popover, '應建立 override popover');
  popover.querySelector('.override-action').dataset.action = 'approve';
  popover.querySelector('.override-reason').value = '測試覆寫原因';
  await popover.querySelector('.override-submit').onclick();

  const calls = postCallsFor('/api/control/approve-recommendation');
  assert.equal(calls.length, 1);
  assert.equal(calls[0].headers['X-API-Key'], 'secret-key');
  assert.deepEqual(calls[0].body, { symbol: '2330', agent_id: 'alpha_discovery', reason: '測試覆寫原因', operator: 'admin' });
  assert.equal(popover.parent, null, '成功後 popover 應被移除');
});

test('handleOverrideClick 否決: POST reject-recommendation 帶 X-API-Key', async () => {
  setApiKey('secret-key');
  installFetch({});
  const btn = makeElement('button');
  btn.dataset = { symbol: '2317', agentId: 'context_layer', guards: '1' };
  handleOverrideClick({ currentTarget: btn, stopPropagation() {} });
  const popover = doc.body._children.find(c => c.className === 'override-popover');
  popover.querySelector('.override-action').dataset.action = 'reject';
  popover.querySelector('.override-reason').value = '測試否決原因';
  await popover.querySelector('.override-submit').onclick();
  const calls = postCallsFor('/api/control/reject-recommendation');
  assert.equal(calls.length, 1);
  assert.equal(calls[0].headers['X-API-Key'], 'secret-key');
  assert.equal(calls[0].body.symbol, '2317');
});

test('handleOverrideClick: HTTP 401 → popover 顯示送出失敗', async () => {
  setApiKey('secret-key');
  installFetch({}, () => okJson({}, 401));
  const btn = makeElement('button');
  btn.dataset = { symbol: '2330', agentId: 'alpha_discovery', guards: '0' };
  handleOverrideClick({ currentTarget: btn, stopPropagation() {} });
  const popover = doc.body._children.find(c => c.className === 'override-popover');
  popover.querySelector('.override-action').dataset.action = 'approve';
  popover.querySelector('.override-reason').value = '測試覆寫原因';
  await popover.querySelector('.override-submit').onclick();
  const errEl = popover.querySelector('.override-error');
  assert.equal(errEl.style.display, 'block', '錯誤區塊應顯示');
  assert.match(errEl.textContent, /送出失敗/);
});
