// shared_web/static/js/__tests__/zero-metric-audit.test.mjs
//
// P2 regression tests: ensure missing-data values render as '—' (or explicit
// placeholder) instead of misleading '0.0', '0.0%', or '0.00' on audited pages.
//
// Run: node --test shared_web/static/js/__tests__/zero-metric-audit.test.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';

// =============================================================================
// Helpers
// =============================================================================

function assertNoMisleadingZero(html, label) {
  // Matches standalone 0.0 / 0.00 / 0.0% appearing as rendered values.
  // We intentionally do NOT match "0 張" / "NT$0" / "0x" where zero is legitimate.
  const misleading = /(^|[^\d.])0\.0+%?($|[^\d.])/.test(html);
  assert.equal(misleading, false, `${label}: HTML must not contain misleading 0.0/0.0%`);
}

function assertContainsEmDash(html, label) {
  assert.ok(html.includes('—'), `${label}: HTML should contain em-dash for missing data`);
}

// =============================================================================
// Stock quote components (pure functions)
// =============================================================================

import { renderHeader } from '../components/stock-quote-header.js';
import { renderFundamentals } from '../components/stock-quote-fundamentals.js';
import { renderTechnical } from '../components/stock-quote-technical.js';

test('renderHeader: missing quote fields render em-dash, not 0.00', () => {
  const html = renderHeader('loaded', {
    status: 'loaded',
    data: { symbol: '2330', last: null, open: null, high: null, low: null, volume: null },
  }, { status: 'loaded', data: { name: '台積電' } });
  assertNoMisleadingZero(html, 'stock-quote-header missing fields');
  assertContainsEmDash(html, 'stock-quote-header missing fields');
});

test('renderHeader: zero values are legitimate and preserved', () => {
  const html = renderHeader('loaded', {
    status: 'loaded',
    data: { symbol: '2330', last: 0, open: 100, high: 100, low: 100, volume: 0 },
  }, { status: 'loaded', data: { name: '台積電' } });
  // Volume 0 -> 0 張 is acceptable.
  assert.ok(html.includes('0.00') || html.includes('0 張'), 'legitimate zeros preserved');
});

test('renderFundamentals: missing PE/PB/PS render em-dash, not 0.00', () => {
  const html = renderFundamentals('loaded', {
    status: 'loaded',
    data: { PE: null, PB: null, PS: null, DividendYield: null, Sector: null },
  });
  assertNoMisleadingZero(html, 'stock-quote-fundamentals missing fields');
  assertContainsEmDash(html, 'stock-quote-fundamentals missing fields');
});

test('renderTechnical: missing SMA/RSI render em-dash, not 0.00', () => {
  const html = renderTechnical('loaded', {
    status: 'loaded',
    data: { sma20: null, sma50: null, rsi14: null },
  });
  assertNoMisleadingZero(html, 'stock-quote-technical missing fields');
  assertContainsEmDash(html, 'stock-quote-technical missing fields');
});

// =============================================================================
// Industry seasonality (pure function)
// =============================================================================

import { renderSeasonalityTab } from '../pages/industry.js';

test('renderSeasonalityTab: missing pattern metrics render em-dash, not 0.0%', () => {
  const html = renderSeasonalityTab({
    seasonal_patterns: [
      {
        name: '未校準模式',
        start_month: 1, start_day: 1, end_month: 1, end_day: 31,
        historical_accuracy: null,
        avg_market_return: null,
        adjustment_factor: null,
        impact: 'positive',
      },
    ],
  });
  assertNoMisleadingZero(html, 'industry-seasonality missing pattern metrics');
  assertContainsEmDash(html, 'industry-seasonality missing pattern metrics');
});

test('renderSeasonalityTab: legitimate zero accuracy is preserved', () => {
  const html = renderSeasonalityTab({
    seasonal_patterns: [
      {
        name: '零準確度模式',
        start_month: 2, start_day: 1, end_month: 2, end_day: 28,
        historical_accuracy: 0,
        avg_market_return: 0,
        adjustment_factor: 0,
        impact: 'neutral',
      },
    ],
  });
  // Zero accuracy should still render as 0%, because the value is explicitly 0.
  assert.ok(html.includes('0%'), 'legitimate zero accuracy preserved');
});


