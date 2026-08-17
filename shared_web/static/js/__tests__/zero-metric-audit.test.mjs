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



// =============================================================================
// Benchmark comparison (async component)
// =============================================================================

import { renderBenchmarkComparison } from '../components/benchmark.js';

test('renderBenchmarkComparison: missing KPIs render em-dash, not 0.0%', async () => {
  const container = { innerHTML: '' };
  const getJSON = async () => ({
    session_count: 1,
    portfolio_return: null,
    taiex_return: null,
    outperformance: null,
    alpha: null,
    beta: null,
    tracking_error: null,
    sharpe_ratio: null,
    info_ratio: null,
    equity_curve: [],
  });
  await renderBenchmarkComparison(container, getJSON);
  assertNoMisleadingZero(container.innerHTML, 'benchmark-comparison missing KPIs');
  assertContainsEmDash(container.innerHTML, 'benchmark-comparison missing KPIs');
});

test('renderBenchmarkComparison: legitimate zeros are preserved', async () => {
  const container = { innerHTML: '' };
  const getJSON = async () => ({
    session_count: 1,
    portfolio_return: 0,
    taiex_return: 0,
    outperformance: 0,
    alpha: 0,
    beta: 1,
    tracking_error: 0.05,
    sharpe_ratio: 1.5,
    info_ratio: 0.2,
    equity_curve: [],
  });
  await renderBenchmarkComparison(container, getJSON);
  assert.ok(container.innerHTML.includes('0.0%'), 'benchmark zero returns preserved');
});

// =============================================================================
// Risk panel (async component)
// =============================================================================

import { renderRiskPanel } from '../components/risk-panel.js';

test('renderRiskPanel: missing risk metrics render em-dash, not 0.0%', async () => {
  const container = { innerHTML: '' };
  const getJSON = async (url) => {
    if (url === '/api/dashboard/portfolio-state') {
      return { max_drawdown: null, concentration_ratio: null, portfolio_value: null, cash: null, positions_count: null };
    }
    return null;
  };
  await renderRiskPanel(container, getJSON);
  assertNoMisleadingZero(container.innerHTML, 'risk-panel missing metrics');
  assertContainsEmDash(container.innerHTML, 'risk-panel missing metrics');
});

test('renderRiskPanel: empty portfolio hides 20x20 matrix and shows unlock guidance (P1-E)', async () => {
  const container = { innerHTML: '' };
  const getJSON = async (url) => {
    if (url === '/api/dashboard/portfolio-state') {
      return { max_drawdown: -0.05, concentration_ratio: 0.2, portfolio_value: 100000, cash: 100000, positions_count: 0, positions: [] };
    }
    if (url === '/api/dashboard/correlation-matrix') {
      // 20x20 matrix would be returned by the backend even with no positions
      return { symbols: ['A', 'B'], labels: ['A', 'B'], matrix: [[1, 0.5], [0.5, 1]] };
    }
    return null;
  };
  await renderRiskPanel(container, getJSON);
  assert.ok(
    container.innerHTML.includes('建立持倉後解鎖持倉相關性分析'),
    'empty portfolio should show unlock guidance instead of the matrix'
  );
  assert.ok(
    !container.innerHTML.includes('corr-matrix'),
    'correlation matrix must not render when there are no positions'
  );
});

test('renderRiskPanel: portfolio with positions renders correlation matrix (P1-E)', async () => {
  const container = { innerHTML: '' };
  const getJSON = async (url) => {
    if (url === '/api/dashboard/portfolio-state') {
      return { max_drawdown: -0.05, concentration_ratio: 0.3, portfolio_value: 100000, cash: 30000, positions_count: 2, positions: [{ symbol: '2330' }, { symbol: '2317' }] };
    }
    if (url === '/api/dashboard/correlation-matrix') {
      return { symbols: ['2330', '2317'], labels: ['2330', '2317'], matrix: [[1, 0.5], [0.5, 1]] };
    }
    return null;
  };
  await renderRiskPanel(container, getJSON);
  assert.ok(
    container.innerHTML.includes('corr-matrix'),
    'correlation matrix should render when positions exist'
  );
  assert.ok(
    !container.innerHTML.includes('建立持倉後解鎖持倉相關性分析'),
    'unlock guidance should not render when positions exist'
  );
});

// =============================================================================
// PnL attribution (async component)
// =============================================================================

import { renderPnLAttribution } from '../components/attribution.js';

test('renderPnLAttribution: missing attribution values render em-dash, not 0.0%', async () => {
  const container = { innerHTML: '' };
  const getJSON = async () => ({
    agent_attribution: [{ agent_id: 'a1', agent_name: 'Test', avg_return: null, count: 0 }],
    sector_attribution: [{ sector: 'tech', sector_label: '科技', avg_return: null, count: 0 }],
    factor_attribution: {
      momentum: { avg_score: null, avg_return: null, contribution: null },
      value: {},
      quality: {},
      agent: {},
    },
  });
  await renderPnLAttribution(container, getJSON);
  assertNoMisleadingZero(container.innerHTML, 'pnl-attribution missing values');
  assertContainsEmDash(container.innerHTML, 'pnl-attribution missing values');
});

// =============================================================================
// Risk gate panel (async component)
// =============================================================================

import { renderRiskGatePanel } from '../components/risk-gate-panel.js';

test('renderRiskGatePanel: missing VaR/drawdown render em-dash, not 0.0%', async () => {
  const container = { innerHTML: '' };
  const getJSON = async () => ({
    risk_snapshot: { var_95: null, max_drawdown_pct: null },
    gate_mode: 'normal',
  });
  await renderRiskGatePanel(container, getJSON);
  assertNoMisleadingZero(container.innerHTML, 'risk-gate-panel missing metrics');
  assertContainsEmDash(container.innerHTML, 'risk-gate-panel missing metrics');
});

// =============================================================================
// Strategies: pure result processor
// =============================================================================

import { processStrategiesResults } from '../pages/strategies.js';

test('processStrategiesResults: surfaces schema errors without throwing', () => {
  const results = [
    { status: 'fulfilled', value: {} },
    { status: 'fulfilled', value: {} },
    { status: 'fulfilled', value: {} },
  ];
  const out = processStrategiesResults(results);
  assert.equal(out.dataStatus, 'failed');
  assert.ok(Object.keys(out.errors).length > 0);
});

// =============================================================================
// Pipeline: pure helpers (P2 batch3 migration regression)
// =============================================================================

import {
  renderConvictionBreakdown,
  buildPipelineStatusBanner,
  countFilteredItems,
} from '../pages/pipeline.js';

test('renderConvictionBreakdown: missing breakdown returns placeholder', () => {
  const html = renderConvictionBreakdown(null);
  assert.ok(html.includes('無計算明細'));
  assertNoMisleadingZero(html, 'pipeline-conviction-breakdown');
});

test('buildPipelineStatusBanner: unknown status renders escaped text without zero', () => {
  const html = buildPipelineStatusBanner({ status: 'weird', status_message: 'custom msg' });
  assert.ok(html.includes('未知的管線狀態'));
  assert.ok(html.includes('weird'));
  assertNoMisleadingZero(html, 'pipeline-status-banner');
});

test('countFilteredItems: empty list returns 0 as legitimate count', () => {
  assert.equal(countFilteredItems([]), 0);
  assert.equal(countFilteredItems(null), 0);
});

// =============================================================================
// Experiments: forecast-vs-reality summary (P2 batch3 migration regression)
// =============================================================================

import { renderForecastVsRealitySummary } from '../pages/experiments.js';

test('renderForecastVsRealitySummary: missing hit flags render em-dash, not 0.0%', () => {
  const el = { classList: { remove: () => {} }, innerHTML: '' };
  const originalDocument = globalThis.document;
  globalThis.document = { getElementById: () => el };
  try {
    renderForecastVsRealitySummary({ symbol_predictions: [{ symbol: '2330', hit: null, passed_guards: null }] });
    assertNoMisleadingZero(el.innerHTML, 'experiments-missing-hits');
    assertContainsEmDash(el.innerHTML, 'experiments-missing-hits');
  } finally {
    if (originalDocument === undefined) {
      delete globalThis.document;
    } else {
      globalThis.document = originalDocument;
    }
  }
});
