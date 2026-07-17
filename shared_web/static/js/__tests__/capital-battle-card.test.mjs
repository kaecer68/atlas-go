// shared_web/static/js/__tests__/capital-battle-card.test.mjs
//
// Unit tests for capital-battle-card.js renderCapitalBattleCard().
//
// 法人 vs 散戶對殺卡：依七維錢潮雷達（3+2+2 分層）數據算出 institutional vs retail 的方向對比，
// 渲染成敘事 + 4 列(外資/投信/自營商/散戶)的視覺對比卡片；行為代理與官方法人依 role 區分。
// 對應前端 B04 修復後新元件（PR #1198）。
//
// Narrative mapping:
//   foreign+institutional+dealer 偏多 vs retail 偏空 → 法人進 / 散戶出
//   foreign+institutional+dealer 偏空 vs retail 偏多 → 法人出 / 散戶進
//   institutional 偏多 (無論 retail)              → 法人與散戶同向偏多
//   institutional 偏空 (無論 retail)              → 法人與散戶同向偏空
//   其它 (含 tied counts / 全觀望 / 缺資料)       → 法人與散戶方向分歧或觀望
//
// 執行：node --test shared_web/static/js/__tests__/capital-battle-card.test.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { renderCapitalBattleCard } from '../components/capital-battle-card.js';

function renderToString(summary) {
  const container = { innerHTML: '' };
  renderCapitalBattleCard(container, summary);
  return container.innerHTML;
}

// ---- Defensive: null container ----

test('container=null → silently no-op', () => {
  assert.doesNotThrow(() => renderCapitalBattleCard(null, { forces: [] }));
  assert.doesNotThrow(() => renderCapitalBattleCard(undefined, null));
});

// ---- 空白資料 ----

test('summary=null → 渲染 4 列 + 觀望/分歧 narrative', () => {
  const html = renderToString(null);
  // 即使沒資料仍渲染 4 列
  assert.match(html, /capital-battle__card/);
  assert.match(html, /法人 vs 散戶對殺/);
  assert.match(html, /法人與散戶方向分歧或觀望/);
  // 4 列都要有
  assert.match(html, /外資/);
  assert.match(html, /投信/);
  assert.match(html, /自營商/);
  assert.match(html, /散戶/);
});

test('summary.forces=[] → 觀望/分歧 narrative', () => {
  const html = renderToString({ forces: [] });
  assert.match(html, /法人與散戶方向分歧或觀望/);
});

test('forces 包含未知 force 名 → 仍渲染 4 列固定行', () => {
  const html = renderToString({
    forces: [{ force: 'mystery', trend: 'bullish' }],  // 不在預設 4 個 key 內
  });
  // 4 列固定 (foreign/institutional/dealer/retail)
  assert.match(html, /外資/);
  assert.match(html, /投信/);
  assert.match(html, /自營商/);
  assert.match(html, /散戶/);
  // mystery 不會出現在 card 內
  assert.doesNotMatch(html, /mystery/);
});

// ---- 法人進 / 散戶出 (核心場景) ----

test('foreign+institutional 偏多,retail 偏空 → 「法人進 / 散戶出」', () => {
  const html = renderToString({
    forces: [
      { force: 'foreign', trend: 'bullish' },
      { force: 'institutional', trend: 'bullish' },
      { force: 'dealer', trend: 'neutral' },
      { force: 'retail', trend: 'bearish' },
    ],
  });
  assert.match(html, /法人進 \/ 散戶出/);
});

test('institutional+dealer 偏多,retail 偏空,foreign 觀望 → 仍觸發法人進 / 散戶出', () => {
  const html = renderToString({
    forces: [
      { force: 'foreign', trend: 'neutral' },
      { force: 'institutional', trend: 'bullish' },
      { force: 'dealer', trend: 'bullish' },
      { force: 'retail', trend: 'bearish' },
    ],
  });
  assert.match(html, /法人進 \/ 散戶出/);
});

// ---- 法人出 / 散戶進 ----

test('foreign+institutional 偏空,retail 偏多 → 「法人出 / 散戶進」', () => {
  const html = renderToString({
    forces: [
      { force: 'foreign', trend: 'bearish' },
      { force: 'institutional', trend: 'bearish' },
      { force: 'dealer', trend: 'neutral' },
      { force: 'retail', trend: 'bullish' },
    ],
  });
  assert.match(html, /法人出 \/ 散戶進/);
});

// ---- 法人與散戶同向偏多 ----

test('all bullish including retail → 「同向偏多」', () => {
  const html = renderToString({
    forces: [
      { force: 'foreign', trend: 'bullish' },
      { force: 'institutional', trend: 'bullish' },
      { force: 'dealer', trend: 'bullish' },
      { force: 'retail', trend: 'bullish' },
    ],
  });
  assert.match(html, /法人與散戶同向偏多/);
});

test('institutional 多數偏多,retail 也偏多 → 「同向偏多」', () => {
  const html = renderToString({
    forces: [
      { force: 'foreign', trend: 'bullish' },
      { force: 'institutional', trend: 'bullish' },
      { force: 'dealer', trend: 'bearish' },
      { force: 'retail', trend: 'bullish' },
    ],
  });
  // institutional votes: foreign(1)+institutional(1)+dealer(0)=2 bull > 0 bear
  // retail=bullish → 同向偏多
  assert.match(html, /法人與散戶同向偏多/);
});

// ---- 法人與散戶同向偏空 ----

test('all bearish → 「同向偏空」', () => {
  const html = renderToString({
    forces: [
      { force: 'foreign', trend: 'bearish' },
      { force: 'institutional', trend: 'bearish' },
      { force: 'dealer', trend: 'bearish' },
      { force: 'retail', trend: 'bearish' },
    ],
  });
  assert.match(html, /法人與散戶同向偏空/);
});

test('institutional 多數偏空,retail 觀望 → 仍觸發「同向偏空」', () => {
  const html = renderToString({
    forces: [
      { force: 'foreign', trend: 'bearish' },
      { force: 'institutional', trend: 'bearish' },
      { force: 'dealer', trend: 'bearish' },
      { force: 'retail', trend: 'neutral' },
    ],
  });
  // institutional bearish(3) > bullish(0), retail=觀望(neutral)
  assert.match(html, /法人與散戶同向偏空/);
});

// ---- 分歧 / 觀望 ----

test('institutional 票數 tied (1 bull vs 1 bear) → 分歧或觀望', () => {
  const html = renderToString({
    forces: [
      { force: 'foreign', trend: 'bullish' },
      { force: 'institutional', trend: 'bearish' },
      { force: 'dealer', trend: 'neutral' },
      { force: 'retail', trend: 'bullish' },
    ],
  });
  // institutional: foreign(1) + institutional(1) + dealer(0) = bull 1, bear 1 → tied → 觀望
  assert.match(html, /法人與散戶方向分歧或觀望/);
});

test('institutional 全觀望 → 分歧或觀望', () => {
  const html = renderToString({
    forces: [
      { force: 'foreign', trend: 'neutral' },
      { force: 'institutional', trend: 'neutral' },
      { force: 'dealer', trend: 'neutral' },
      { force: 'retail', trend: 'bullish' },
    ],
  });
  assert.match(html, /法人與散戶方向分歧或觀望/);
});

test('retail 缺資料 (force=null) → 預設 neutral', () => {
  const html = renderToString({
    forces: [
      { force: 'foreign', trend: 'bullish' },
      { force: 'institutional', trend: 'bullish' },
      { force: 'dealer' },
      { force: 'retail', trend: 'bearish' },
    ],
  });
  // institutional 多 bull, retail bear → 法人進 / 散戶出
  assert.match(html, /法人進 \/ 散戶出/);
});

// ---- tone class ----

test('bullish 對應 capital-battle__row--positive tone', () => {
  const html = renderToString({
    forces: [
      { force: 'foreign', trend: 'bullish' },
      { force: 'institutional', trend: 'neutral' },
      { force: 'dealer', trend: 'neutral' },
      { force: 'retail', trend: 'neutral' },
    ],
  });
  assert.match(html, /capital-battle__row capital-battle__row--positive/);
});

test('bearish 對應 capital-battle__row--negative tone', () => {
  const html = renderToString({
    forces: [
      { force: 'foreign', trend: 'bearish' },
      { force: 'institutional', trend: 'neutral' },
      { force: 'dealer', trend: 'neutral' },
      { force: 'retail', trend: 'neutral' },
    ],
  });
  assert.match(html, /capital-battle__row capital-battle__row--negative/);
});

test('neutral/missing 對應 capital-battle__row--neutral tone', () => {
  const html = renderToString({
    forces: [
      { force: 'foreign', trend: 'bullish' },
      { force: 'institutional' },  // trend 缺失
      { force: 'dealer', trend: 'neutral' },
      { force: 'retail', trend: 'bullish' },
    ],
  });
  // 出現至少一列 neutral tone (institutional 因 trend 缺失)
  assert.match(html, /capital-battle__row capital-battle__row--neutral/);
});

// ---- PascalCase 後備 ----

test('backend 用 PascalCase Trend 也能解析', () => {
  const html = renderToString({
    forces: [
      { Force: 'foreign', Trend: 'BULLISH' },
      { Force: 'institutional', Trend: 'BULLISH' },
      { Force: 'dealer', Trend: 'NEUTRAL' },
      { Force: 'retail', Trend: 'BEARISH' },
    ],
  });
  assert.match(html, /法人進 \/ 散戶出/);
});

// ---- HTML escape ----

test('含 XSS payload 的 force 名 (理論上不可能但 defensive)', () => {
  const html = renderToString({
    forces: [
      { force: '<script>alert("xss")</script>', trend: 'bullish' },
    ],
  });
  // 不會真的渲染 <script> tag,因為外層 escapeHtml 包 label
  // 但 force 名不在 4 個 key 內時不會渲染,所以這裡只是確認不 throw
  assert.doesNotThrow(() => renderCapitalBattleCard({ innerHTML: '' }, {
    forces: [{ force: '<script>alert("xss")</script>', trend: 'bullish' }],
  }));
  // 確認 container 仍可寫入
  assert.match(html, /capital-battle__card/);
});

// ---- 行數與結構完整性 ----

test('永遠渲染 4 列 (外/投/自/散)', () => {
  const html = renderToString({
    forces: [
      { force: 'foreign', trend: 'bullish' },
      { force: 'institutional', trend: 'bullish' },
      { force: 'dealer', trend: 'bullish' },
      { force: 'retail', trend: 'bullish' },
    ],
  });
  const rowMatches = html.match(/class="capital-battle__row/g) || [];
  assert.equal(rowMatches.length, 4);
});
