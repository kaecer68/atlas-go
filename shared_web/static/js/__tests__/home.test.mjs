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

  assert.ok(container.innerHTML.includes('載入市場摘要'), 'hero section should render');

  const marketGrid = elements.get('home-market-grid');
  assert.ok(marketGrid, 'market pulse grid should exist');
  assert.ok(
    marketGrid.innerHTML.includes('持平') || marketGrid.innerHTML.includes('—'),
    'market pulse should render fallback values when data is unavailable'
  );

  const recContent = elements.get('home-rec-content');
  assert.ok(recContent, 'recommendation content should exist inside hero');
  assert.ok(
    recContent.innerHTML.includes('觀望'),
    'recommendation should default to 觀望 when pipeline data is unavailable'
  );

  const portfolioContent = elements.get('home-portfolio-content');
  assert.ok(portfolioContent, 'portfolio snapshot container should exist');
  assert.ok(
    portfolioContent.innerHTML.includes('尚無投資組合資料') ||
      portfolioContent.innerHTML.includes('示範總市值'),
    'portfolio snapshot should render demo or empty-state fallback'
  );

  const trustFooter = elements.get('home-trust-footer');
  assert.ok(trustFooter, 'trust footer container should exist');
  assert.ok(
    trustFooter.innerHTML.includes('不構成投資建議'),
    'trust footer must render even when all dashboard APIs fail'
  );
});

test('renderHomePage: hero has exactly 1 primary CTA', async () => {
  const container = { innerHTML: '' };
  await renderHomePage(container);

  // Primary CTA button must exist and be registered
  const marketBtn = elements.get('home-view-market');
  assert.ok(marketBtn, 'primary CTA button must exist');
  assert.ok(marketBtn._listeners.some(l => l.type === 'click'), 'primary CTA must have click listener');

  // Exactly 1 CTA button in today-summary actions
  const actionsMatch = container.innerHTML.match(/class="home-today-summary__actions"([\s\S]*?)<\/div>/);
  assert.ok(actionsMatch, 'today-summary actions section must exist');
  const btnCount = (actionsMatch[1].match(/<button/g) || []).length;
  assert.strictEqual(btnCount, 1, 'hero must have exactly 1 CTA button');

  // Remaining CTA is 查看市場詳情 linking to crossmarket
  assert.ok(
    container.innerHTML.includes('查看市場詳情') && container.innerHTML.includes('home-view-market'),
    'primary CTA text must be 查看市場詳情'
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

  // The indicators section must show real numbers, not "—" fallback.
  // If mockData() field names are correct, TAIEX shows a number.
  // If field names are wrong (MarketIndex instead of taiex), pointValue→null→"—".
  const indicatorsEl = elements.get('home-today-indicators');
  assert.ok(indicatorsEl, 'today-indicators must exist');
  const html = indicatorsEl.innerHTML;

  // Assert correct behavior: TAIEX must show a real number
  assert.match(html, /加權.*\d+/,
    'contract: taiex must render (mockData must use "taiex" not "MarketIndex")');
  assert.ok(!html.includes('加權 —'),
    'contract: taiex must NOT fallback to "—" (indicates wrong field name in mockData)');
});
