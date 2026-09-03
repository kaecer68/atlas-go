// shared_web/static/js/__tests__/format-metric.test.mjs
//
// format-metric.js unit tests covering zero-value / missing-data / signed edge cases.
// Run: node --test shared_web/static/js/__tests__/format-metric.test.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  fmtCurrency,
  fmtPct,
  fmtSignedPct,
  fmtLargeNumber,
  fmtDrawdown,
  formatNumber,
  formatSigned,
  formatMaxDrawdown,
  isEmptyMetric,
  fmtSafeSigned,
  fmtSafeNumber,
  fmtSafePct,
  fmtSafeSignedPct,
  fmtSafeDrawdown,
  fmtSafeCurrency,
  fmtSafeLargeNumber,
} from '../shared/format-metric.js';

test('fmtCurrency formats with thousands grouping and defaults to NTD', () => {
  assert.equal(fmtCurrency(1234567.89), 'NT$1,234,568');
  assert.equal(fmtCurrency(0), 'NT$0');
  assert.equal(fmtCurrency(-1000), 'NT$-1,000');
  assert.equal(fmtCurrency(1234.5, { decimals: 2 }), 'NT$1,234.50');
});

test('fmtCurrency respects options', () => {
  assert.equal(fmtCurrency(1234.5, { decimals: 2, prefix: '$', useGrouping: false }), '$1234.50');
});

test('fmtCurrency returns em-dash for invalid inputs', () => {
  assert.equal(fmtCurrency(null), '—');
  assert.equal(fmtCurrency(undefined), '—');
  assert.equal(fmtCurrency(NaN), '—');
});

test('fmtPct converts ratio to percent without sign', () => {
  assert.equal(fmtPct(0.1523, 1), '15.2%');
  assert.equal(fmtPct(0, 1), '0.0%');
});

test('fmtSignedPct adds sign and avoids -0.0%', () => {
  assert.equal(fmtSignedPct(0.36, 1), '+0.4%');
  assert.equal(fmtSignedPct(-0.36, 1), '−0.4%');
  assert.equal(fmtSignedPct(0, 1), '0.0%');
  assert.equal(fmtSignedPct(-0.0001, 1), '0.0%');
});

test('fmtLargeNumber scales to 萬 / 億 and groups', () => {
  assert.equal(fmtLargeNumber(150000000), '1.50 億');
  assert.equal(fmtLargeNumber(25000), '2.5 萬');
  assert.equal(fmtLargeNumber(999), '999');
  assert.equal(fmtLargeNumber(null), '—');
});

test('fmtDrawdown never shows positive sign', () => {
  assert.equal(fmtDrawdown(0.15), '−15.0%');
  assert.equal(fmtDrawdown(-0.15), '−15.0%');
  assert.equal(fmtDrawdown(0), '0.0%');
  assert.equal(fmtDrawdown(null), '—');
});

test('formatNumber returns em-dash for null/undefined/NaN', () => {
  assert.equal(formatNumber(null), '—');
  assert.equal(formatNumber(undefined), '—');
  assert.equal(formatNumber(NaN), '—');
});

test('formatNumber supports grouping', () => {
  assert.equal(formatNumber(1234567.89, { decimals: 0, useGrouping: true }), '1,234,568');
});

// B3 (risk-console Phase 1)：percent:true 未指定 suffix 時自動帶 '%'，
// 避免 live/portfolio 頁百分比無單位（集中度 47.8、現金比 52.2…）。
test('formatNumber percent:true appends % suffix automatically', () => {
  assert.equal(formatNumber(0.1523, { percent: true, decimals: 1 }), '15.2%');
  assert.equal(formatNumber(0.522, { percent: true, decimals: 1 }), '52.2%');
  assert.equal(formatNumber(-0.0048, { percent: true, decimals: 1 }), '-0.5%');
  // 明確給 suffix 時不受影響
  assert.equal(formatNumber(0.1523, { percent: true, decimals: 1, suffix: '%' }), '15.2%');
});

test('formatSigned handles tiny negative values', () => {
  assert.equal(formatSigned(-0.0001, { decimals: 1, suffix: '%' }), '0.0%');
  assert.equal(formatSigned(0, { forceSign: true }), '0.00');
});

test('formatMaxDrawdown returns absolute when requested', () => {
  assert.equal(formatMaxDrawdown(-0.125, { asAbsolute: true }), '12.5%');
});


// ============================================================================
// P2 safe-format wrappers
// ============================================================================

test('isEmptyMetric identifies missing-data sentinels', () => {
  assert.equal(isEmptyMetric(null), true);
  assert.equal(isEmptyMetric(undefined), true);
  assert.equal(isEmptyMetric(''), true);
  assert.equal(isEmptyMetric(NaN), true);
  assert.equal(isEmptyMetric(0), false);
  assert.equal(isEmptyMetric(false), false);
});

test('fmtSafeSigned returns em-dash for invalid inputs', () => {
  assert.equal(fmtSafeSigned(null), '—');
  assert.equal(fmtSafeSigned(undefined), '—');
  assert.equal(fmtSafeSigned(0, { forceSign: true }), '0.00');
  assert.equal(fmtSafeSigned(-12.3, { decimals: 1, suffix: ' 億' }), '−12.3 億');
});

test('fmtSafeNumber mirrors formatNumber and preserves zero', () => {
  assert.equal(fmtSafeNumber(0), '0.00');
  assert.equal(fmtSafeNumber(null), '—');
  assert.equal(fmtSafeNumber(undefined), '—');
  assert.equal(fmtSafeNumber(1234.567, { decimals: 1, suffix: 'x' }), '1234.6x');
});

test('fmtSafePct mirrors fmtPct and preserves zero', () => {
  assert.equal(fmtSafePct(0.15), '15.0%');
  assert.equal(fmtSafePct(0), '0.0%');
  assert.equal(fmtSafePct(null), '—');
});

test('fmtSafeSignedPct mirrors fmtSignedPct and preserves zero', () => {
  assert.equal(fmtSafeSignedPct(0.36), '+0.4%');
  assert.equal(fmtSafeSignedPct(0), '0.0%');
  assert.equal(fmtSafeSignedPct(null), '—');
});

test('fmtSafeDrawdown mirrors fmtDrawdown and preserves zero', () => {
  assert.equal(fmtSafeDrawdown(0.15), '−15.0%');
  assert.equal(fmtSafeDrawdown(0), '0.0%');
  assert.equal(fmtSafeDrawdown(null), '—');
});

test('fmtSafeCurrency mirrors fmtCurrency and preserves zero', () => {
  assert.equal(fmtSafeCurrency(0), 'NT$0');
  assert.equal(fmtSafeCurrency(null), '—');
});

test('fmtSafeLargeNumber mirrors fmtLargeNumber and preserves zero', () => {
  assert.equal(fmtSafeLargeNumber(0), '0');
  assert.equal(fmtSafeLargeNumber(null), '—');
});
