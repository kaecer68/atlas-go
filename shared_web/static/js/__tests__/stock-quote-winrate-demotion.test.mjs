// shared_web/static/js/__tests__/stock-quote-winrate-demotion.test.mjs
//
// fix/20261006-demote-no-edge-signal-families — 個股勝率卡（前端呈現）。
//
// 背景: stock_signal_outcomes 顯示兩個 stockpicker 家族的 5 日淨成本期望值為
//   負（foreign-3d-net-buy −0.517%，t=−11.4；momentum-20d-positive −0.987%，
//   t=−21.3，成本 0.585%）。它們必須退出所有使用者可見的推薦/建議路徑，
//   但**量測保留**：outcome 寫入與 win-rate 聚合照舊，卡片仍要顯示數字。
//
// 本檔釘住卡片那一半：降級家族的數字仍顯示（量測），但徽章不得再寫「可參考」，
// 且要出現具名期望值的降級說明；未降級的家族完全不受影響。
// Go 端唯一真相來源: internal/config/stockpicker_edge.go。
//
// 執行: node --test shared_web/static/js/__tests__/stock-quote-winrate-demotion.test.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { renderWinRate } from '../components/stock-quote-winrate.js';

const condition = (id, winRate, avgForwardReturn) => ({
  condition_id: id,
  source: `stockpicker-${id}`,
  observations: 42,
  hits: 26,
  win_rate: winRate,
  wilson_lower: 0.47,
  wilson_upper: 0.75,
  confidence: 0.95,
  calibration_status: 'eligible',
  net_cost_rate: 0.00585,
  avg_forward_return: avgForwardReturn,
  updated_at: '2026-08-28T00:00:00Z',
  data_start: '2026-06-01',
  data_end: '2026-08-20',
});

const DEMOTED_ONE = 'foreign-3d-net-buy';
const DEMOTED_TWO = 'momentum-20d-positive';
const RETAINED = 'price-volume-bottom-divergence';

function renderCard() {
  const data = {
    symbol: '2330',
    rolling_window: '120d',
    found: true,
    conditions: [
      condition(DEMOTED_ONE, 0.619, 0.0081),
      condition(DEMOTED_TWO, 0.51, -0.004),
      condition(RETAINED, 0.66, 0.012),
    ],
  };
  return renderWinRate('ready', { status: 'ok', data }, null);
}

// One block per rendered condition, so an assertion on a demoted row cannot be
// satisfied by a retained row elsewhere in the card.
function blockFor(html, id) {
  const blocks = html.split('<div class="sq-winrate-condition">').slice(1);
  const block = blocks.find(b => b.includes(id));
  assert.ok(block, `no rendered block for ${id}`);
  return block;
}

test('降級家族的數字仍然顯示（量測保留）', () => {
  const html = renderCard();
  const one = blockFor(html, DEMOTED_ONE);
  assert.ok(one.includes('61.9%'), 'win_rate 應仍顯示: ' + one);
  assert.ok(one.includes('42 次'), 'observations 應仍顯示');
  assert.ok(one.includes('47% ~ 75%'), 'Wilson 區間應仍顯示');
  assert.ok(blockFor(html, DEMOTED_TWO).includes('51.0%'), '第二個降級家族的勝率應仍顯示');
});

test('降級家族不再標示「可參考」（推薦語意）', () => {
  const html = renderCard();
  for (const id of [DEMOTED_ONE, DEMOTED_TWO]) {
    const block = blockFor(html, id);
    assert.ok(!block.includes('可參考'), `${id} 不得標示 可參考: ` + block);
    assert.ok(block.includes('僅供量測'), `${id} 應標示 僅供量測`);
  }
});

test('降級家族附上具名期望值的降級說明', () => {
  const html = renderCard();
  assert.ok(blockFor(html, DEMOTED_ONE).includes('sq-winrate-note--demoted'));
  assert.ok(blockFor(html, DEMOTED_ONE).includes('−0.517%'));
  assert.ok(blockFor(html, DEMOTED_ONE).includes('已移出推薦與排名'), '說明需點明已移出推薦與排名');
  assert.ok(blockFor(html, DEMOTED_TWO).includes('−0.987%'));
});

test('e2e 契約不變：eligible 校準徽章類別仍在', () => {
  const html = renderCard();
  assert.ok(blockFor(html, DEMOTED_ONE).includes('sq-winrate-badge--eligible'));
  assert.ok(blockFor(html, DEMOTED_ONE).includes('sq-winrate-badge--demoted'));
});

test('未降級家族完全不受影響', () => {
  const block = blockFor(renderCard(), RETAINED);
  assert.ok(block.includes('可參考'), '保留家族應維持 可參考');
  assert.ok(!block.includes('僅供量測'));
  assert.ok(!block.includes('sq-winrate-note--demoted'));
});
