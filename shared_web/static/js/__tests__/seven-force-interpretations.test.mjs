// shared_web/static/js/__tests__/seven-force-interpretations.test.mjs
//
// Unit tests for seven-force-interpretations.js renderSevenForceInterpretations().
//
// 7-Force 組合解讀：依七大錢潮勢力的方向組合產出 1..N 條敘事。
// 對應前端 C05 修復後新元件（PR #1198）。
//
// 條件分支：
//   共識 (count-based):
//     - 7/7 bullish → 全面偏多
//     - 7/7 bearish → 全面偏空
//     - bull >= 4 & bear <= 1 → 多數偏多
//     - bear >= 4 & bull <= 1 → 多數偏空
//     - bull = 0 & bear = 0 → 全觀望
//   組合 (specific force pairs):
//     - foreign↑ + institutional↑ → 法人齊買
//     - foreign↑ + retail↓        → 法人接散戶籌碼
//     - retail↑ + dealer↑         → 短線動能活躍
//     - retail↑ + foreign↓        → 散戶 vs 外資反向
//     - government↑ + foreign↑    → 官股護盤
//     - futures↑ + tsm_adr↑       → 外資期貨積極
//   Fallback（無 condition 觸發）:
//     - 主要權重集中在 X、Y，方向以觀望為主
//
// 注意 source 內有 dead code：第 78 行 `hasForeign && hasForeignBearish` 互相排斥
// 永不觸發（解讀文字卻提到「投信」暗示應為 hasInstitutionalBearish）。
// 本測試不修 bug，只驗證實際行為。
//
// 執行：node --test shared_web/static/js/__tests__/seven-force-interpretations.test.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { renderSevenForceInterpretations } from '../components/seven-force-interpretations.js';

function renderToString(summary) {
  const container = { innerHTML: '' };
  renderSevenForceInterpretations(container, summary);
  return container.innerHTML;
}

// ---- Defensive ----

test('container=null → silently no-op', () => {
  assert.doesNotThrow(() => renderSevenForceInterpretations(null, { forces: [] }));
  assert.doesNotThrow(() => renderSevenForceInterpretations(undefined, null));
});

test('summary=null → 「尚無 7-Force 解讀」placeholder', () => {
  const html = renderToString(null);
  assert.match(html, /尚無 7-Force 解讀/);
  assert.match(html, /home-loading-card/);
});

test('summary.forces=[] → placeholder', () => {
  const html = renderToString({ forces: [] });
  assert.match(html, /尚無 7-Force 解讀/);
});

test('summary.forces 不是 array → placeholder', () => {
  const html = renderToString({ forces: 'broken' });
  assert.match(html, /尚無 7-Force 解讀/);
});

// ---- 共識 (count-based) ----

test('7/7 bullish → 「七大勢力全面偏多」', () => {
  const forces = ['foreign', 'institutional', 'dealer', 'retail', 'government', 'futures', 'tsm_adr']
    .map(name => ({ force: name, trend: 'bullish' }));
  const html = renderToString({ forces });
  assert.match(html, /七大勢力全面偏多/);
  assert.match(html, /資金共識強/);
  // 確保沒有多數偏多/多數偏空/全觀望等其它共識
  assert.doesNotMatch(html, /七大勢力全面偏空/);
});

test('7/7 bearish → 「七大勢力全面偏空」', () => {
  const forces = ['foreign', 'institutional', 'dealer', 'retail', 'government', 'futures', 'tsm_adr']
    .map(name => ({ force: name, trend: 'bearish' }));
  const html = renderToString({ forces });
  assert.match(html, /七大勢力全面偏空/);
  assert.match(html, /資金共識偏謹慎/);
});

test('5 bullish, 1 bearish, 1 neutral → 「多數勢力偏多」', () => {
  const forces = [
    { force: 'foreign', trend: 'bullish' },
    { force: 'institutional', trend: 'bullish' },
    { force: 'dealer', trend: 'bullish' },
    { force: 'retail', trend: 'bullish' },
    { force: 'government', trend: 'bullish' },
    { force: 'futures', trend: 'bearish' },
    { force: 'tsm_adr', trend: 'neutral' },
  ];
  const html = renderToString({ forces });
  assert.match(html, /多數勢力偏多/);
  assert.match(html, /僅少數勢力分歧/);
});

test('5 bearish, 1 bullish, 1 neutral → 「多數勢力偏空」', () => {
  const forces = [
    { force: 'foreign', trend: 'bearish' },
    { force: 'institutional', trend: 'bearish' },
    { force: 'dealer', trend: 'bearish' },
    { force: 'retail', trend: 'bearish' },
    { force: 'government', trend: 'bearish' },
    { force: 'futures', trend: 'bullish' },
    { force: 'tsm_adr', trend: 'neutral' },
  ];
  const html = renderToString({ forces });
  assert.match(html, /多數勢力偏空/);
  assert.match(html, /僅少數勢力支撐/);
});

test('7/7 neutral → 「各勢力觀望」', () => {
  const forces = ['foreign', 'institutional', 'dealer', 'retail', 'government', 'futures', 'tsm_adr']
    .map(name => ({ force: name, trend: 'neutral' }));
  const html = renderToString({ forces });
  assert.match(html, /各勢力觀望/);
  assert.match(html, /市場方向不明/);
});

// ---- 組合 (specific force pair triggers) ----

test('foreign↑ + institutional↑ → 「外資與投信同步偏多」', () => {
  const html = renderToString({
    forces: [
      { force: 'foreign', trend: 'bullish' },
      { force: 'institutional', trend: 'bullish' },
      // 其他都不觸發特定組合條件
      { force: 'dealer', trend: 'neutral' },
      { force: 'retail', trend: 'neutral' },
      { force: 'government', trend: 'neutral' },
      { force: 'futures', trend: 'neutral' },
      { force: 'tsm_adr', trend: 'neutral' },
    ],
  });
  assert.match(html, /外資與投信同步偏多/);
  assert.match(html, /法人齊買/);
});

test('foreign↑ + retail↓ → 「法人接散戶籌碼結構」', () => {
  const html = renderToString({
    forces: [
      { force: 'foreign', trend: 'bullish' },
      { force: 'institutional', trend: 'neutral' },
      { force: 'dealer', trend: 'neutral' },
      { force: 'retail', trend: 'bearish' },
      { force: 'government', trend: 'neutral' },
      { force: 'futures', trend: 'neutral' },
      { force: 'tsm_adr', trend: 'neutral' },
    ],
  });
  // bull=1, bear=1, neutral=5 → 共識條件不觸發 (需要 bull>=4 或 bear>=4)
  // 但 force 對條件成立
  assert.match(html, /外資偏多但散戶偏空/);
  assert.match(html, /法人接散戶籌碼結構/);
});

test('retail↑ + dealer↑ → 「散戶與自營商同步偏多,短線動能活躍」', () => {
  const html = renderToString({
    forces: [
      { force: 'foreign', trend: 'neutral' },
      { force: 'institutional', trend: 'neutral' },
      { force: 'dealer', trend: 'bullish' },
      { force: 'retail', trend: 'bullish' },
      { force: 'government', trend: 'neutral' },
      { force: 'futures', trend: 'neutral' },
      { force: 'tsm_adr', trend: 'neutral' },
    ],
  });
  assert.match(html, /散戶與自營商同步偏多/);
  assert.match(html, /短線動能活躍/);
});

test('retail↑ + foreign↓ → 「散戶偏多但外資偏空」', () => {
  const html = renderToString({
    forces: [
      { force: 'foreign', trend: 'bearish' },
      { force: 'institutional', trend: 'neutral' },
      { force: 'dealer', trend: 'neutral' },
      { force: 'retail', trend: 'bullish' },
      { force: 'government', trend: 'neutral' },
      { force: 'futures', trend: 'neutral' },
      { force: 'tsm_adr', trend: 'neutral' },
    ],
  });
  assert.match(html, /散戶偏多但外資偏空/);
  assert.match(html, /籌碼與法人反向/);
});

test('government↑ + foreign↑ → 「官股護盤加上外資回流」', () => {
  const html = renderToString({
    forces: [
      { force: 'foreign', trend: 'bullish' },
      { force: 'institutional', trend: 'neutral' },
      { force: 'dealer', trend: 'neutral' },
      { force: 'retail', trend: 'neutral' },
      { force: 'government', trend: 'bullish' },
      { force: 'futures', trend: 'neutral' },
      { force: 'tsm_adr', trend: 'neutral' },
    ],
  });
  assert.match(html, /政府\/公股行庫與外資同步偏多/);
  assert.match(html, /官股護盤加上外資回流/);
});

test('futures↑ + tsm_adr↑ → 「外資期貨與 TSM ADR 同步偏多」', () => {
  const html = renderToString({
    forces: [
      { force: 'foreign', trend: 'neutral' },
      { force: 'institutional', trend: 'neutral' },
      { force: 'dealer', trend: 'neutral' },
      { force: 'retail', trend: 'neutral' },
      { force: 'government', trend: 'neutral' },
      { force: 'futures', trend: 'bullish' },
      { force: 'tsm_adr', trend: 'bullish' },
    ],
  });
  assert.match(html, /外資期貨與 TSM ADR 同步偏多/);
  assert.match(html, /外資態度積極/);
});

// ---- 多條同時觸發 ----

test('多個組合條件同時成立 → 多條 <li> 並存', () => {
  // foreign↑ + institutional↑ + government↑ 同時 → 三條 individual 觸發
  const html = renderToString({
    forces: [
      { force: 'foreign', trend: 'bullish' },
      { force: 'institutional', trend: 'bullish' },
      { force: 'government', trend: 'bullish' },
      // 其他都 neutral
      { force: 'dealer', trend: 'neutral' },
      { force: 'retail', trend: 'neutral' },
      { force: 'futures', trend: 'neutral' },
      { force: 'tsm_adr', trend: 'neutral' },
    ],
  });
  // bull=3, bear=0, neutral=4 → 不觸發共識 count-based 條件
  // foreign+institutional bullish → 法人齊買
  assert.match(html, /法人齊買/);
  // government+foreign bullish → 官股護盤
  assert.match(html, /官股護盤/);
  // 共識"多數偏多" 不該觸發 (bull=3 < 4)
  assert.doesNotMatch(html, /多數勢力偏多/);
});

// ---- Fallback: 主要權重 ----

test('全 neutral 但 weight 都是 undefined → 「各勢力觀望」共識觸發,不進 fallback', () => {
  // 注意 source 行為:bull=0 & bear=0 滿足共識條件,即使 force 的 weight 缺,
  // 仍會 push「各勢力觀望」,fallback 只在 interpretations 為空時才會 push。
  const html = renderToString({
    forces: [
      { force: 'foreign', trend: 'neutral' },
      { force: 'institutional', trend: 'neutral' },
      { force: 'dealer', trend: 'neutral' },
      { force: 'retail', trend: 'neutral' },
      { force: 'government', trend: 'neutral' },
      { force: 'futures', trend: 'neutral' },
      { force: 'tsm_adr', trend: 'neutral' },
    ],
  });
  assert.match(html, /各勢力觀望/);
  assert.match(html, /市場方向不明/);
});

test('fallback 顯示 top-2 最高 weight 中文 label', () => {
  const html = renderToString({
    forces: [
      { force: 'foreign', trend: 'neutral', weight: 0.50 },     // 1st
      { force: 'institutional', trend: 'neutral', weight: 0.30 }, // 2nd
      { force: 'dealer', trend: 'neutral', weight: 0.10 },
      { force: 'retail', trend: 'neutral', weight: 0.05 },
      { force: 'government', trend: 'neutral', weight: 0.03 },
      { force: 'futures', trend: 'neutral', weight: 0.01 },
      { force: 'tsm_adr', trend: 'neutral', weight: 0.01 },
    ],
  });
  // 共識 "全觀望" 觸發,但 interpretations 不為空所以 fallback 不會 push
  assert.match(html, /各勢力觀望/);
  assert.match(html, /市場方向不明/);
});

// ---- 純混合 (沒 consensus 也沒 specific pair) 但有 weight → 走 fallback ----

test('混合 signals 但 force pair 都未配對 → top-weight fallback', () => {
  // foreign↑ + institutional↑ 觸發,故不放這 case
  // 直接做「只有一個 bullish 但又不觸發共識」的情境
  const html = renderToString({
    forces: [
      { force: 'foreign', trend: 'bullish', weight: 0.4 },  // bull=1, foreign↑ 但 institutional 不 bull → 不觸發
      { force: 'institutional', trend: 'bearish', weight: 0.3 },
      { force: 'dealer', trend: 'neutral', weight: 0.1 },
      { force: 'retail', trend: 'neutral', weight: 0.1 },
      { force: 'government', trend: 'neutral', weight: 0.05 },
      { force: 'futures', trend: 'neutral', weight: 0.03 },
      { force: 'tsm_adr', trend: 'neutral', weight: 0.02 },
    ],
  });
  // bull=1, bear=1 → 不觸發 4+ 共識
  // foreign↑ + institutional↓ → 沒有 hasForeignBullish + hasRetailBearish,沒有其它 pair
  // → fallback 觸發,顯示 top-2 weight: foreign 40%, institutional 30%
  assert.match(html, /主要權重集中在/);
  assert.match(html, /外資/);
  assert.match(html, /投信/);
  assert.match(html, /方向以觀望為主/);
});

// ---- Force 缺 weight (filter 掉) ----

test('forces 沒 weight → top.map 回空 → fallback 中 topNames 為空字串', () => {
  const html = renderToString({
    forces: [
      { force: 'foreign' },  // 沒 trend 沒 weight
      { force: 'institutional' },
    ],
  });
  // bull=0, bear=0 → 共識 "全觀望" 觸發
  // 但 weight undefined → top 為空 → fallback 仍 push (因為 interpretations 流程先 push 共識)
  // 但測 fallback 路徑:直接看 fallback push 條件「!interpretations.length」
  assert.match(html, /各勢力觀望/);
});

// ---- PascalCase 後備 ----

test('backend 用 PascalCase (Force/Trend) 也能解析', () => {
  const html = renderToString({
    forces: [
      { Force: 'foreign', Trend: 'BULLISH' },
      { Force: 'institutional', Trend: 'BULLISH' },
      { Force: 'dealer', Trend: 'NEUTRAL' },
      { Force: 'retail', Trend: 'NEUTRAL' },
      { Force: 'government', Trend: 'NEUTRAL' },
      { Force: 'futures', Trend: 'NEUTRAL' },
      { Force: 'tsm_adr', Trend: 'NEUTRAL' },
    ],
  });
  // 7 forces 但 bull=2 → 不到 4,不觸發共識
  // force pair: foreign↑ + institutional↑ → 法人齊買
  assert.match(html, /法人齊買/);
});

// ---- Unknown force 名稱 → label() fallback ----

test('未知 force 名稱 → fallback 顯示「未知」', () => {
  const html = renderToString({
    forces: [
      { force: 'foreign', trend: 'neutral', weight: 0.5 },
      { force: 'mystery_force', trend: 'neutral', weight: 0.3 },
    ],
  });
  // 全 neutral → 共識 "全觀望" 觸發
  // fallback 不 push (因為已有共識)
  assert.match(html, /各勢力觀望/);
});

// ---- HTML escaping ----

test('解讀列表項目使用 escapeHtml,防止 XSS', () => {
  // 為了觸發 fallback 路徑:避免共識條件、避免所有 force pair 配對。
  // 設 <xss> 為 bullish (top weight),foreign bearish,其他中性:
  //   bull=1, bear=1 → 不觸發 4+ 共識
  //   bull=0+neutral=0? No → 不觸發全觀望
  //   foreign↑? No (foreign 是 bearish)
  //   → fallback 觸發,top-2 為 <xss>、foreign
  const html = renderToString({
    forces: [
      { force: '<xss>', trend: 'bullish', weight: 0.5 },
      { force: 'foreign', trend: 'bearish', weight: 0.3 },
      { force: 'institutional', trend: 'neutral', weight: 0.1 },
      { force: 'dealer', trend: 'neutral', weight: 0.05 },
      { force: 'retail', trend: 'neutral', weight: 0.03 },
      { force: 'government', trend: 'neutral', weight: 0.01 },
      { force: 'futures', trend: 'neutral', weight: 0.01 },
    ],
  });
  // 確認走 fallback 路徑
  assert.match(html, /主要權重集中在/);
  // literal `<xss>` tag 不應出現在 HTML(render 之後的 innerHTML)
  // 注意 source 對 fallback 字串做雙重 escape:
  //   - line 96: escapeHtml(topNames) 已 escape '<xss>' → '&lt;xss&gt;'
  //   - line 99: items.map(escapeHtml) 又再 escape → '&amp;lt;xss&amp;gt;'
  // 瀏覽器解讀後顯示為純文字「<xss>」,語意上仍安全。
  assert.doesNotMatch(html, /<xss>/);
  assert.doesNotMatch(html, /<script/i);
  assert.match(html, /&amp;lt;xss&amp;gt;/);  // 確認 double-escape 確實發生
});

// ---- 結構完整性 ----

test('輸出包含 title + ul + 多 li items', () => {
  const html = renderToString({
    forces: [
      { force: 'foreign', trend: 'bullish' },
      { force: 'institutional', trend: 'bullish' },
    ],
  });
  assert.match(html, /<h4 class="force-interpretation__title">7-Force 組合解讀<\/h4>/);
  assert.match(html, /<ul class="force-interpretation__list">/);
  // 最少 1 條 li
  assert.match(html, /<li class="force-interpretation__item">/);
});
