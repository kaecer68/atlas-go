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
  const el = {
    id,
    innerHTML: '',
    textContent: '',
    style: {},
    _listeners: listeners,
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

  const recCard = elements.get('home-rec-card');
  assert.ok(recCard, 'recommendation card should exist');
  assert.ok(
    recCard.innerHTML.includes('觀望'),
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
