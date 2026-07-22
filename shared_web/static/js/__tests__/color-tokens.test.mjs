// Color token tests — covers existing 9 helpers + Phase 2b-A 9 new chart/accent helpers.
// Each helper must return a `var(--...)` string (not a raw hex) so CSS variable
// theming continues to work.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  financialColor,
  regimeColor,
  severityColor,
  confidenceColor,
  pnlProfitColor,
  pnlLossColor,
  inflowColor,
  outflowColor,
  hexToRgba,
  // Phase 2b-A additions:
  chartAxisColor,
  chartBackgroundColor,
  chartTextColor,
  mutedTextColor,
  accentBlueColor,
  accentPurpleColor,
  accentTealColor,
  neutralTextColor,
  overlayColor,
} from '../shared/color-tokens.js';

test('financialColor: positive value returns bullish token', () => {
  assert.equal(financialColor(100, 'pnl'), 'var(--pnl-profit)');
  assert.equal(financialColor(0, 'trend'), 'var(--trend-bullish)');
  assert.equal(financialColor(50, 'capital'), 'var(--capital-inflow)');
  assert.equal(financialColor(10, 'signal'), 'var(--signal-bullish)');
});

test('financialColor: negative value returns bearish token', () => {
  assert.equal(financialColor(-1, 'pnl'), 'var(--pnl-loss)');
  assert.equal(financialColor(-50, 'trend'), 'var(--trend-bearish)');
});

test('financialColor: null/undefined/NaN returns empty string', () => {
  assert.equal(financialColor(null, 'pnl'), '');
  assert.equal(financialColor(undefined, 'pnl'), '');
  assert.equal(financialColor(NaN, 'pnl'), '');
});

test('regimeColor: maps known regimes', () => {
  assert.equal(regimeColor('RISK_ON'), 'var(--trend-bullish)');
  assert.equal(regimeColor('RISK_OFF'), 'var(--trend-bearish)');
  assert.equal(regimeColor('NEUTRAL'), 'var(--warn)');
  assert.equal(regimeColor('TRANSITIONAL'), 'var(--warn)');
  // unknown regime falls back to muted
  assert.equal(regimeColor('UNKNOWN'), 'var(--muted)');
});

test('severityColor: maps severity levels (low/medium/high/critical)', () => {
  assert.equal(severityColor('low'), 'var(--metric-good)');
  assert.equal(severityColor('medium'), 'var(--warn)');
  assert.equal(severityColor('high'), 'var(--risk-high)');
  assert.equal(severityColor('critical'), 'var(--color-danger)');
  // unknown severity falls back to muted
  assert.equal(severityColor('info'), 'var(--muted)');
  assert.equal(severityColor('warn'), 'var(--muted)');
});

test('confidenceColor: returns gradient token for any value 0..1', () => {
  const c0 = confidenceColor(0);
  const c05 = confidenceColor(0.5);
  const c1 = confidenceColor(1);
  assert.match(c0, /^var\(--/);
  assert.match(c05, /^var\(--/);
  assert.match(c1, /^var\(--/);
});

test('pnl/inflow/outflow: direct token accessors', () => {
  assert.equal(pnlProfitColor(), 'var(--pnl-profit)');
  assert.equal(pnlLossColor(), 'var(--pnl-loss)');
  assert.equal(inflowColor(), 'var(--capital-inflow)');
  assert.equal(outflowColor(), 'var(--capital-outflow)');
});

test('hexToRgba: converts 6-digit hex to rgba', () => {
  assert.equal(hexToRgba('#ff0000', 0.5), 'rgba(255, 0, 0, 0.5)');
  assert.equal(hexToRgba('#00ff00', 1), 'rgba(0, 255, 0, 1)');
});

test('hexToRgba: converts 3-digit hex shorthand', () => {
  assert.equal(hexToRgba('#f00', 0.5), 'rgba(255, 0, 0, 0.5)');
});

test('hexToRgba: returns input as-is when missing # prefix', () => {
  assert.equal(hexToRgba('red', 0.5), 'red');
});

test('hexToRgba: returns rgba with default alpha=1 when only hex passed', () => {
  assert.equal(hexToRgba('#000000'), 'rgba(0, 0, 0, 1)');
});

// --- Phase 2b-A new helpers ---

test('Phase 2b-A: chartAxisColor returns --muted (#b8c4d0 base)', () => {
  assert.equal(chartAxisColor(), 'var(--muted)');
});

test('Phase 2b-A: chartBackgroundColor returns --panel (#13161c base)', () => {
  assert.equal(chartBackgroundColor(), 'var(--panel)');
});

test('Phase 2b-A: chartTextColor returns --text (#f0f4f8 base)', () => {
  assert.equal(chartTextColor(), 'var(--text)');
});

test('Phase 2b-A: mutedTextColor returns --status-unknown (#9ca3af base)', () => {
  assert.equal(mutedTextColor(), 'var(--status-unknown)');
});

test('Phase 2b-A: accentBlueColor returns --accent (#4fc1ff base)', () => {
  assert.equal(accentBlueColor(), 'var(--accent)');
});

test('Phase 2b-A: accentPurpleColor returns --accent-secondary (#8b5cf6 base)', () => {
  assert.equal(accentPurpleColor(), 'var(--accent-secondary)');
});

test('Phase 2b-A: accentTealColor returns --accent-tertiary (#10b981 base)', () => {
  assert.equal(accentTealColor(), 'var(--accent-tertiary)');
});

test('Phase 2b-A: neutralTextColor returns --status-unknown', () => {
  assert.equal(neutralTextColor(), 'var(--status-unknown)');
});

test('Phase 2b-A: overlayColor returns --bg', () => {
  assert.equal(overlayColor(), 'var(--bg)');
});

test('Phase 2b-A: all 9 new helpers return var() references (no hardcoded hex)', () => {
  // This is the design invariant: helpers must never return raw hex,
  // otherwise the whole point of tokenization is defeated.
  const helpers = [
    chartAxisColor(),
    chartBackgroundColor(),
    chartTextColor(),
    mutedTextColor(),
    accentBlueColor(),
    accentPurpleColor(),
    accentTealColor(),
    neutralTextColor(),
    overlayColor(),
  ];
  for (const v of helpers) {
    assert.match(v, /^var\(--[a-zA-Z0-9-]+\)$/, `expected var() form, got ${v}`);
  }
});
