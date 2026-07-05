// shared_web/static/js/__tests__/scroll-utils.test.mjs
// Tests for scroll-utils.js: scrollToSection

import { test } from 'node:test';
import assert from 'node:assert/strict';

// Stub scrollIntoView to record invocations
let lastScrollCall = null;
const mockElement = {
  scrollIntoView(options) {
    lastScrollCall = options;
  },
};

// Default stubs: reduced motion OFF, element found
let matchMediaResult = { matches: false };
global.window = {
  matchMedia(query) {
    if (query === '(prefers-reduced-motion: reduce)') {
      return matchMediaResult;
    }
    return { matches: false };
  },
};
global.document = {
  querySelector(selector) {
    if (selector === '#market-card-2330') return mockElement;
    return null;
  },
};

const { scrollToSection } = await import('../shared/scroll-utils.js');

// ---------------------------------------------------------------------------
// Test: scrollToSection calls scrollIntoView with smooth behavior when reduced
// motion is NOT preferred
// ---------------------------------------------------------------------------
test('scrollToSection: smooth scroll when prefers-reduced-motion is false', () => {
  matchMediaResult = { matches: false };
  lastScrollCall = null;

  scrollToSection('#market-card-2330');

  assert.deepStrictEqual(lastScrollCall, { behavior: 'smooth', block: 'start' });
});

// ---------------------------------------------------------------------------
// Test: scrollToSection uses instant scroll when reduced motion IS preferred
// ---------------------------------------------------------------------------
test('scrollToSection: instant scroll when prefers-reduced-motion is true', () => {
  matchMediaResult = { matches: true };
  lastScrollCall = null;

  scrollToSection('#market-card-2330');

  assert.deepStrictEqual(lastScrollCall, { behavior: 'instant', block: 'start' });
});

// ---------------------------------------------------------------------------
// Test: scrollToSection does not throw when target element does not exist
// ---------------------------------------------------------------------------
test('scrollToSection: no throw for non-existent element', () => {
  matchMediaResult = { matches: false };
  lastScrollCall = null;

  assert.doesNotThrow(() => scrollToSection('#non-existent-id'));
  assert.strictEqual(lastScrollCall, null);
});
