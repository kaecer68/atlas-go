// shared_web/static/js/__tests__/stock-quote-winrate-break-even.test.mjs
//
// 2026-10-13 前端穿透 — 個股勝率卡的「可承受成本」列。
//
// 背景: stockpicker.BreakEvenCostRate（forward 回報中位數）自 migration
//   000028 起持久化於 stock_win_rate / stock_win_rate_by_regime，HTTP
//   GET /api/stock/win_rate 亦已透出 break_even_cost_rate，但前端未消費
//   ⇒ 寫了卻不可見。它是 edge 存亡的直覺指標：勝率跌破 5 成的來回成本上界。
//
// 本檔釘住三件事：
//   1. 有值時顯示「可承受成本 X%」，並對照實際淨成本率 net_cost_rate；
//      低於實際成本者明確標示「不足以覆蓋交易成本」（無 edge 的直覺）。
//   2. 舊列（break_even_cost_rate 缺漏或 0）顯示「—」，**不得**暗示無 edge。
//   3. 降級家族既有語義不受影響（仍顯示量測 + 降級說明）。
// Go 端唯一真相來源: internal/stockpicker/winrate.go BreakEvenCostRate。

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { renderWinRate } from '../components/stock-quote-winrate.js';

const condition = (over = {}) => ({
  condition_id: 'price-volume-bottom-divergence',
  source: 'stockpicker-price-volume-bottom-divergence',
  observations: 42,
  hits: 26,
  win_rate: 0.62,
  wilson_lower: 0.47,
  wilson_upper: 0.75,
  confidence: 0.95,
  calibration_status: 'eligible',
  net_cost_rate: 0.00585,
  avg_forward_return: 0.012,
  break_even_cost_rate: 0.0115,
  updated_at: '2026-10-13T00:00:00Z',
  data_start: '2026-06-01',
  data_end: '2026-08-20',
  ...over,
});

const render = (cond) =>
  renderWinRate('ready', { status: 'ok', data: { found: true, rolling_window: '120d', conditions: [cond] } }, null);

// 1. 有值：顯示可承受成本，並與實際淨成本率對照
test('shows break-even cost when measured', () => {
  const html = render(condition());
  assert.match(html, /可承受成本/);
  assert.match(html, /1\.15%/);
});

// 1b. 可承受成本低於實際 0.585% 來回成本 → 明確標示不足以覆蓋
test('flags break-even below the actual round-trip cost', () => {
  const html = render(condition({ break_even_cost_rate: 0.002 }));
  assert.match(html, /不足以覆蓋交易成本/);
});

// 2. 舊列：0 或缺漏 → 「—」，不得暗示無 edge
test('legacy row without break-even renders a dash, not a verdict', () => {
  for (const cond of [condition({ break_even_cost_rate: 0 }), condition({ break_even_cost_rate: undefined })]) {
    const html = render(cond);
    assert.match(html, /可承受成本/);
    assert.match(html, /—/);
    assert.doesNotMatch(html, /不足以覆蓋交易成本/);
  }
});

// 3. 降級家族語義不受影響
test('demoted family keeps its measurement-only wording', () => {
  const html = render(condition({
    condition_id: 'foreign-3d-net-buy',
    source: 'stockpicker-foreign-3d-net-buy',
    break_even_cost_rate: -0.005,
  }));
  assert.match(html, /僅供量測/);
  assert.match(html, /已移出推薦與排名/);
});
