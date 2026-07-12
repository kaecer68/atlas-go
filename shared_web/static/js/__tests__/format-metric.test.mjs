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

test('formatSigned handles tiny negative values', () => {
  assert.equal(formatSigned(-0.0001, { decimals: 1, suffix: '%' }), '0.0%');
  assert.equal(formatSigned(0, { forceSign: true }), '0.00');
});

test('formatMaxDrawdown returns absolute when requested', () => {
  assert.equal(formatMaxDrawdown(-0.125, { asAbsolute: true }), '12.5%');
});
