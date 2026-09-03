// shared_web/static/js/__tests__/period-matrix.test.mjs
//
// Unit tests for pages/period-matrix.js (CF Phase 2 PR-2c heatmap renderer).
//   - <30 samples → 「資料不足」grey cell (API status=insufficient_data),
//     numeric fields never rendered for insufficient cells
//   - ≥30 samples → win_rate + Sharpe rendered, colored by win rate
//   - colorForWinRate boundaries: 0.5 neutral, 1.0 green, 0.0 red
//
// 執行：node --test shared_web/static/js/__tests__/period-matrix.test.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  buildPeriodMatrixHtml,
  colorForWinRate,
  cellLabel,
  PERIOD_LABELS,
} from '../pages/period-matrix.js';

function sampleData() {
  return {
    source: 'postgres',
    degraded: false,
    min_samples: 30,
    periods: ['downturn', 'turnaround_up', 'bull', 'plateau', 'consolidation', 'turnaround_down', 'black_swan'],
    cells: [
      { agent_id: 'agent-a', market_period: 'bull', sample_count: 30, win_rate: 0.9, sharpe: 1.2, avg_return: 0.02, status: 'ok' },
      { agent_id: 'agent-a', market_period: 'plateau', sample_count: 12, win_rate: null, sharpe: null, avg_return: null, status: 'insufficient_data' },
      { agent_id: 'agent-a', market_period: 'downturn', sample_count: 0, win_rate: null, sharpe: null, avg_return: null, status: 'insufficient_data' },
      { agent_id: 'agent-b', market_period: 'bull', sample_count: 40, win_rate: 0.45, sharpe: -0.2, avg_return: -0.01, status: 'ok' },
    ],
  };
}

test('PERIOD_LABELS covers the seven canonical periods', () => {
  assert.equal(Object.keys(PERIOD_LABELS).length, 7);
  assert.equal(PERIOD_LABELS.black_swan, '黑天鵝');
  assert.equal(PERIOD_LABELS.turnaround_up, '轉折開高');
});

test('colorForWinRate: neutral at 0.5, green at 1.0, red at 0.0', () => {
  const neutral = colorForWinRate(0.5);
  const green = colorForWinRate(1.0);
  const red = colorForWinRate(0.0);
  assert.match(neutral, /^hsl\(60, 55%, /);
  assert.match(green, /^hsl\(120, 55%, /);
  assert.match(red, /^hsl\(0, 55%, /);
  assert.equal(colorForWinRate(-1), 'transparent');
});

test('cellLabel: insufficient cells show 資料不足, ok cells show win rate', () => {
  assert.equal(cellLabel({ status: 'insufficient_data', sample_count: 5 }), '資料不足');
  assert.equal(cellLabel({ status: 'ok', win_rate: 0.875 }), '87.5%');
});

test('buildPeriodMatrixHtml: header has 7 period columns + agent rows', () => {
  const html = buildPeriodMatrixHtml(sampleData());
  assert.match(html, /<table class="pm-heat-table">/);
  assert.match(html, /低迷/);
  assert.match(html, /黑天鵝/);
  assert.match(html, /agent-a/);
  assert.match(html, /agent-b/);
  // 2 agents × (1 corner + 7 periods) table: agents appear in thead? no, in rows
  assert.ok((html.match(/<tr/g) || []).length >= 2);
});

test('buildPeriodMatrixHtml: ok cell shows win rate and greys insufficient', () => {
  const html = buildPeriodMatrixHtml(sampleData());
  // agent-a/bull ok cell: 90% win rate with background color and Sharpe.
  const okIdx = html.indexOf('data-agent="agent-a" data-period="bull"');
  assert.ok(okIdx >= 0, 'ok cell exists');
  const okCell = html.slice(Math.max(0, okIdx - 60), okIdx + 420);
  assert.match(okCell, /90\.0%/);
  assert.match(okCell, /S 1\.20/);
  assert.match(okCell, /background-color:hsl\(120, 55%, /);

  // insufficient cell: 資料不足 with truthful n, no win_rate/sharpe numbers.
  const insIdx = html.indexOf('data-agent="agent-a" data-period="plateau"');
  assert.ok(insIdx >= 0, 'insufficient cell exists');
  const insCell = html.slice(Math.max(0, insIdx - 60), insIdx + 400);
  assert.match(insCell, /pm-cell-insufficient/);
  assert.match(insCell, /資料不足/);
  assert.match(insCell, /n=12/);
  assert.ok(!insCell.includes('90.0%'), 'insufficient cell must not render win rate');
});

test('buildPeriodMatrixHtml: empty (no outcome) cell renders — without numbers', () => {
  const html = buildPeriodMatrixHtml(sampleData());
  const idx = html.indexOf('data-agent="agent-a" data-period="downturn"');
  assert.ok(idx >= 0, 'zero-sample cell exists');
  const cell = html.slice(Math.max(0, idx - 60), idx + 300);
  assert.match(cell, /pm-cell-insufficient/);
  assert.match(cell, /資料不足/);
  assert.match(cell, /n=0/);
});

test('buildPeriodMatrixHtml: null data → empty string', () => {
  assert.equal(buildPeriodMatrixHtml(null), '');
  assert.equal(buildPeriodMatrixHtml({}), '');
});
