// shared_web/static/js/__tests__/home.test.mjs
//
// Home page regression tests: verify the page renders fallback UI when
// dashboard/macro/stress/recommendation APIs fail, instead of crashing or
// leaving the user with a blank/loading screen.

import { test } from 'node:test';
import assert from 'node:assert/strict';

// ============================================================================
// DOM / browser stubs required to render home.js in Node
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
      // Support common selectors used in onboarding.js
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
  dispatchEvent() {},
  addEventListener() {},
  CustomEvent: class CustomEvent extends Event {
    constructor(type, init) {
      super(type, init);
      this.detail = init?.detail ?? null;
    }
  },
};

// All dashboard APIs fail (network/backend down).
global.fetch = async (url) => {
  throw new Error(`Simulated API failure: ${url}`);
};

const { renderHomePage } = await import('../pages/home.js');

// ============================================================================
// Fallback UI tests
// ============================================================================

test('renderHomePage: dashboard API failures render fallback without crashing', async () => {
  const container = { innerHTML: '' };

  await assert.doesNotReject(
    () => renderHomePage(container),
    'renderHomePage must not throw when all dashboard APIs fail'
  );

  assert.ok(container.innerHTML.includes('市場脈動'), 'market pulse section should render');

  const marketGrid = elements.get('home-market-grid');
  assert.ok(marketGrid, 'market pulse grid should exist');
  assert.ok(
    marketGrid.innerHTML.includes('持平') || marketGrid.innerHTML.includes('—'),
    'market pulse should render fallback values when data is unavailable'
  );

  const predictionsContent = elements.get('home-predictions-content');
  assert.ok(predictionsContent, 'predictions content should exist');

  const sevenForceContent = elements.get('home-seven-force-content');
  assert.ok(sevenForceContent, 'seven-force content should exist');

  const trustFooter = elements.get('home-trust-footer');
  assert.ok(trustFooter, 'trust footer container should exist');
  assert.ok(
    trustFooter.innerHTML.includes('不構成投資建議'),
    'trust footer must render even when all dashboard APIs fail'
  );
});

test('renderHomePage: new home sections render after redesign', async () => {
  const container = { innerHTML: '' };
  await renderHomePage(container);

  assert.ok(container.innerHTML.includes('市場脈動'), 'market pulse section should render');
  assert.ok(container.innerHTML.includes('未來 5 日錢潮預測'), 'predictions section should render');
  assert.ok(container.innerHTML.includes('七維錢潮雷達'), 'seven-force section should render');

  const marketGrid = elements.get('home-market-grid');
  assert.ok(marketGrid, 'market pulse grid should exist');
});

test('renderHomePage: nav links use real hrefs, no javascript:void(0) dead links', async () => {
  // UX audit P1-F-d: /client/home 有 2 個 javascript:void(0) 死鏈
  // （「完整看板 →」與時期 chip），必須接上真實路由。
  const container = { innerHTML: '' };
  await renderHomePage(container);
  assert.ok(
    !container.innerHTML.includes('javascript:void(0)'),
    'home page must not render javascript:void(0) dead links'
  );
  assert.ok(
    container.innerHTML.includes('href="/client/capital_board"'),
    '「完整看板 →」should link to /client/capital_board'
  );
});

test('renderHomePage: renders trust footer after unexpected synchronous error', async () => {
  // Simulate an unexpected failure in the date-update path by making the
  // last-update element throw on textContent assignment. The outer try/catch
  // should catch it and still render the trust footer.
  const badEl = {
    ...createElement('home-last-update'),
    set textContent(_value) { throw new Error('unexpected element failure'); },
  };
  elements.set('home-last-update', badEl);

  const container = { innerHTML: '' };
  await assert.doesNotReject(() => renderHomePage(container));

  const trustFooter = elements.get('home-trust-footer');
  assert.ok(trustFooter.innerHTML.includes('不構成投資建議'), 'trust footer must render despite unexpected error');
});

// ============================================================================
// Contract: mockData() field names must match schema / page code expectations
// ============================================================================

// mockData() is not exported; we verify indirectly by checking that mock mode
// (localStorage atlas-mock=true) renders actual indicator values (not "—").
// Phase 0 bug: mockData used { MarketIndex, ChangePct } instead of { taiex,
// foreign_investor_net }, causing pointValue/macro,'taiex' → null → "—".

// Contract test: mockData() macro field names must match schema / page code expectations.
// Phase 0 bug: mockData uses { MarketIndex, ChangePct, ForeignNetBuy } but page code
// calls pointValue(macro,'taiex') and pointChange(macro,'foreign_investor_net').
// This causes indicators to silently fall back to "—" instead of showing mock values.
//
// FIX REQUIRED: In home.js mockData(), rename:
//   MarketIndex → taiex, ChangePct → change_pct, ForeignNetBuy → foreign_investor_net
//
// Until fixed, this test FAILS — proving the contract is violated.
test('mockData: macro field names must match what page code expects (contract)', async () => {
  const realLocalStorage = global.localStorage;
  global.localStorage = { getItem: (k) => (k === 'atlas-mock' ? 'true' : null), setItem() {}, removeItem() {} };

  elements.clear();
  global.fetch = async () => { throw new Error('force-mock'); };

  const container = { innerHTML: '' };
  await renderHomePage(container);
  global.localStorage = realLocalStorage;

  // The market pulse grid must show real numbers from mockData, not "—" fallback.
  // If mockData() field names are correct, TAIEX shows a number.
  // If field names are wrong (MarketIndex instead of taiex), pointValue→null→"—".
  const marketGrid = elements.get('home-market-grid');
  assert.ok(marketGrid, 'market pulse grid must exist');
  const html = marketGrid.innerHTML;

  assert.match(html, /大盤[\s\S]*\d+/,
    'contract: taiex must render (mockData must use "taiex" not "MarketIndex")');
  assert.ok(!html.includes('大盤 —'),
    'contract: taiex must NOT fallback to "—" (indicates wrong field name in mockData)');
});
