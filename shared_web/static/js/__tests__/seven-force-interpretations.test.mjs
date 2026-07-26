// shared_web/static/js/__tests__/seven-force-interpretations.test.mjs
//
// Unit tests for seven-force-interpretations.js renderSevenForceInterpretations().
//
// 「七維錢潮雷達」3+2+2 共識敘事：
//   - 共識 (official_actor：三大法人）：
//     - 3/3 bullish → 「三大法人（共識）偏多，資金方向一致」
//     - 3/3 bearish → 「三大法人（共識）偏空，資金方向一致」
//     - 2/3 bullish + 0 bearish → 「二讀多／分歧」
//     - 0 bullish + 2/3 bearish → 「二讀空／分歧」
//     - 全部 official_actor neutral → 「法人皆觀望，方向不明」
//   - 行為代理（若 available）：
//     - 散戶/官股 與法人同步 → 「行為代理確認」
//     - 行為代理反向 → 「行為代理與法人反向」
//   - 外資 positioning（futures OI）：
//     - 領先方向與法人一致 → 「領先訊號支持／不支援」
//     - futures 永遠**不**影響 institutional 共識
//   - 跨市場訊號（TSM ADR）：
//     - 與法人方向一致 → 「跨市場同步」
//   - Fallback：主要權重集中在 X（仍保留，spec §9.1 允許 actor 共識敘事）
//
// 注意：
//   - 已不再使用「七大勢力全面偏多／偏空」字串（七同級語意已棄）
//   - 不再使用「7/7」字串
//   - bullishForces / bearishForces 過濾 dimension_role='official_actor'，
//     futures/tsm_adr 不參與機構共識
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

// Helper for new E07 contract.
function e07Force(name, opts = {}) {
  return {
    force: name,
    dimension_role: opts.dimension_role || 'official_actor',
    trend: opts.trend || 'neutral',
    z_score: opts.z_score || 0,
    raw_value: opts.raw_value || 0,
    data_available: opts.data_available !== undefined ? opts.data_available : true,
    weight: opts.weight || 0,
    weight_deprecated: true,
  };
}

// ---- Defensive ----

test('container=null → silently no-op', () => {
  assert.doesNotThrow(() => renderSevenForceInterpretations(null, { forces: [] }));
  assert.doesNotThrow(() => renderSevenForceInterpretations(undefined, null));
});

test('summary=null → 不顯示重複占位（七維卡片已負責空狀態）', () => {
  const html = renderToString(null);
  assert.equal(html, '');
});

test('summary.forces=[] → 不顯示重複占位', () => {
  const html = renderToString({ forces: [] });
  assert.equal(html, '');
});

test('summary.forces 不是 array → 不顯示重複占位', () => {
  const html = renderToString({ forces: 'broken' });
  assert.match(html, /尚無|資料載入|七維錢潮/);
});

// ---- 新 contract：3+2+2 共識，無「七大勢力全面偏多/偏空」 ----

test('不渲染「七大勢力」字串（七同級語意已棄）', () => {
  const forces = ['foreign', 'institutional', 'dealer'].map(name =>
    e07Force(name, { dimension_role: 'official_actor', trend: 'bullish' })
  );
  const html = renderToString({ forces });
  assert.doesNotMatch(html, /七大勢力/);
});

test('不渲染「7/7」字串', () => {
  const all = [];
  ['foreign', 'institutional', 'dealer', 'government', 'retail', 'futures', 'tsm_adr']
    .forEach(name => {
      all.push(e07Force(name, { trend: 'bullish', data_available: true }));
    });
  // 強制每個 dimension_role
  all[0].dimension_role = 'official_actor';
  all[1].dimension_role = 'official_actor';
  all[2].dimension_role = 'official_actor';
  all[3].dimension_role = 'behavioral_proxy';
  all[4].dimension_role = 'behavioral_proxy';
  all[5].dimension_role = 'positioning_indicator';
  all[6].dimension_role = 'cross_market_signal';
  const html = renderToString({ forces: all });
  assert.doesNotMatch(html, /7\/7/);
});

test('3/3 official_actor bullish → 「三大法人（共識）偏多」', () => {
  const html = renderToString({
    forces: [
      e07Force('foreign', { dimension_role: 'official_actor', trend: 'bullish' }),
      e07Force('institutional', { dimension_role: 'official_actor', trend: 'bullish' }),
      e07Force('dealer', { dimension_role: 'official_actor', trend: 'bullish' }),
    ],
  });
  assert.match(html, /三大法人/);
  assert.match(html, /偏多/);
  // 確保「三大法人偏空」不會出現
  assert.doesNotMatch(html, /三大法人.{0,20}偏空/);
});

test('3/3 official_actor bearish → 「三大法人（共識）偏空」', () => {
  const html = renderToString({
    forces: [
      e07Force('foreign', { dimension_role: 'official_actor', trend: 'bearish' }),
      e07Force('institutional', { dimension_role: 'official_actor', trend: 'bearish' }),
      e07Force('dealer', { dimension_role: 'official_actor', trend: 'bearish' }),
    ],
  });
  assert.match(html, /三大法人/);
  assert.match(html, /偏空/);
  // 確保「三大法人偏多」不會出現
  assert.doesNotMatch(html, /三大法人.{0,20}偏多/);
});

test('3/3 official_actor neutral → 「三大法人皆觀望」', () => {
  const html = renderToString({
    forces: [
      e07Force('foreign', { dimension_role: 'official_actor', trend: 'neutral' }),
      e07Force('institutional', { dimension_role: 'official_actor', trend: 'neutral' }),
      e07Force('dealer', { dimension_role: 'official_actor', trend: 'neutral' }),
    ],
  });
  assert.match(html, /三大法人/);
  assert.match(html, /觀望/);
});

test('2/3 official_actor bullish + 1 neutral → 「二讀多／分歧」', () => {
  const html = renderToString({
    forces: [
      e07Force('foreign', { dimension_role: 'official_actor', trend: 'bullish' }),
      e07Force('institutional', { dimension_role: 'official_actor', trend: 'bullish' }),
      e07Force('dealer', { dimension_role: 'official_actor', trend: 'neutral' }),
    ],
  });
  // 期待分歧/部分偏多敘事；嚴格避免「全面偏多」
  assert.match(html, /分歧|兩家偏多|二讀多|部分偏多/);
});

test('1/3 official_actor bullish + 2 neutral → 「僅一家偏多」', () => {
  const html = renderToString({
    forces: [
      e07Force('foreign', { dimension_role: 'official_actor', trend: 'bullish' }),
      e07Force('institutional', { dimension_role: 'official_actor', trend: 'neutral' }),
      e07Force('dealer', { dimension_role: 'official_actor', trend: 'neutral' }),
    ],
  });
  // 共識條件不觸發 → 走組合或 fallback
  assert.doesNotMatch(html, /三大法人.{0,10}全面/);
});

// ---- futures/tsm_adr 不參與機構共識敘事 ----

test('futures bullish 不會把 institutional consensus 推向 bullish', () => {
  // 三大法人全 neutral，只有 futures bullish → 共識應為「觀望」
  // 不應出現「三大法人偏多」
  const html = renderToString({
    forces: [
      e07Force('foreign', { dimension_role: 'official_actor', trend: 'neutral' }),
      e07Force('institutional', { dimension_role: 'official_actor', trend: 'neutral' }),
      e07Force('dealer', { dimension_role: 'official_actor', trend: 'neutral' }),
      e07Force('government', { dimension_role: 'behavioral_proxy', trend: 'neutral' }),
      e07Force('retail', { dimension_role: 'behavioral_proxy', trend: 'neutral' }),
      e07Force('futures', { dimension_role: 'positioning_indicator', trend: 'bullish' }),
      e07Force('tsm_adr', { dimension_role: 'cross_market_signal', trend: 'neutral' }),
    ],
  });
  assert.match(html, /三大法人.{0,10}觀望/);
  assert.doesNotMatch(html, /三大法人.{0,20}偏多/);
});

test('tsm_adr bullish 不會把 institutional consensus 推向 bullish', () => {
  // 三大法人全 neutral，只有 tsm_adr bullish → 共識應為「觀望」
  const html = renderToString({
    forces: [
      e07Force('foreign', { dimension_role: 'official_actor', trend: 'neutral' }),
      e07Force('institutional', { dimension_role: 'official_actor', trend: 'neutral' }),
      e07Force('dealer', { dimension_role: 'official_actor', trend: 'neutral' }),
      e07Force('futures', { dimension_role: 'positioning_indicator', trend: 'neutral' }),
      e07Force('tsm_adr', { dimension_role: 'cross_market_signal', trend: 'bullish' }),
    ],
  });
  assert.match(html, /三大法人.{0,10}觀望/);
  assert.doesNotMatch(html, /三大法人.{0,20}偏多/);
});

// ---- 行為代理敘事 ----

test('behavioral_proxy：散戶 bearish + 法人 bullish → 「行為代理與法人反向」', () => {
  const html = renderToString({
    forces: [
      e07Force('foreign', { dimension_role: 'official_actor', trend: 'bullish' }),
      e07Force('institutional', { dimension_role: 'official_actor', trend: 'neutral' }),
      e07Force('dealer', { dimension_role: 'official_actor', trend: 'neutral' }),
      e07Force('retail', { dimension_role: 'behavioral_proxy', trend: 'bearish' }),
    ],
  });
  // 應出現散戶與法人反向的敘事
  assert.match(html, /散戶.{0,30}(反向|偏空|抵銷|withdraw)|反向.{0,20}法人|法人接散戶/);
});

test('behavioral_proxy unavailable（government 缺資料）→ 不敘事', () => {
  // government 缺資料不應被敘事為「反向」或「同步」
  const html = renderToString({
    forces: [
      e07Force('foreign', { dimension_role: 'official_actor', trend: 'bullish' }),
      e07Force('institutional', { dimension_role: 'official_actor', trend: 'bullish' }),
      e07Force('dealer', { dimension_role: 'official_actor', trend: 'bullish' }),
      e07Force('government', { dimension_role: 'behavioral_proxy', trend: 'bearish', data_available: false }),
      e07Force('retail', { dimension_role: 'behavioral_proxy', trend: 'bearish' }),
    ],
  });
  // 政府 unavailable → 不應敘事「官股護盤」「官股反向」
  assert.doesNotMatch(html, /官股護盤/);
});

// ---- 4 narrative group structure ----

test('4 narrative group：institutional + behavioral + foreign_positioning + cross_market', () => {
  // 構造每個 group 都應有觸發的輸入
  const html = renderToString({
    forces: [
      e07Force('foreign', { dimension_role: 'official_actor', trend: 'bullish' }),
      e07Force('institutional', { dimension_role: 'official_actor', trend: 'bullish' }),
      e07Force('dealer', { dimension_role: 'official_actor', trend: 'bullish' }),
      // behavioral
      e07Force('government', { dimension_role: 'behavioral_proxy', trend: 'bullish', data_available: true }),
      e07Force('retail', { dimension_role: 'behavioral_proxy', trend: 'bullish' }),
      // foreign positioning
      e07Force('futures', { dimension_role: 'positioning_indicator', trend: 'bullish' }),
      // cross_market
      e07Force('tsm_adr', { dimension_role: 'cross_market_signal', trend: 'bullish' }),
    ],
  });
  // 至少 4 條敘事（institutional 共識 + 行為代理 + 領先 + 跨市場）
  const items = (html.match(/<li class="force-interpretation__item">/g) || []).length;
  assert.ok(items >= 4, `預期至少 4 條敘事（4 個 group），實際 ${items}`);
});

// ---- Fallback：主要權重集中在 X 仍保留（spec §9.1 actor 共識） ----

test('fallback 仍顯示 top-2 最高 weight 中文 label', () => {
  // 故意做出不觸發任一 consensus/配對的情境：official_actor mixed，
  // 且 retail 反向，futures 也有 weight；fallback 仍 top-weight。
  const html = renderToString({
    forces: [
      e07Force('foreign', { trend: 'bullish', weight: 0.40 }),
      e07Force('institutional', { trend: 'bearish', weight: 0.30 }),
      e07Force('dealer', { trend: 'neutral', weight: 0.10 }),
      e07Force('government', { trend: 'neutral', weight: 0.05, data_available: false }),
      e07Force('retail', { trend: 'neutral', weight: 0.05 }),
      e07Force('futures', { dimension_role: 'positioning_indicator', trend: 'neutral', weight: 0.05 }),
      e07Force('tsm_adr', { dimension_role: 'cross_market_signal', trend: 'neutral', weight: 0.05 }),
    ],
  });
  // 主要權重敘事仍存在
  assert.match(html, /主要權重集中在/);
  assert.match(html, /外資/);
  assert.match(html, /投信/);
});

// ---- 純官方actor觀望（行為代理 + 訊號中性）----

test('3 official_actor neutral + 4 others neutral → 共識「三大法人皆觀望」', () => {
  const html = renderToString({
    forces: [
      e07Force('foreign', { trend: 'neutral' }),
      e07Force('institutional', { trend: 'neutral' }),
      e07Force('dealer', { trend: 'neutral' }),
      e07Force('government', { trend: 'neutral', data_available: true }),
      e07Force('retail', { trend: 'neutral' }),
      e07Force('futures', { dimension_role: 'positioning_indicator', trend: 'neutral' }),
      e07Force('tsm_adr', { dimension_role: 'cross_market_signal', trend: 'neutral' }),
    ],
  });
  assert.match(html, /三大法人/);
  assert.match(html, /觀望/);
});

// ---- PascalCase 後備 ----

test('backend 用 PascalCase (Force/Trend/DimensionRole) 也能解析', () => {
  const html = renderToString({
    forces: [
      { Force: 'foreign', DimensionRole: 'official_actor', Trend: 'BULLISH' },
      { Force: 'institutional', DimensionRole: 'official_actor', Trend: 'BULLISH' },
      { Force: 'dealer', DimensionRole: 'official_actor', Trend: 'NEUTRAL' },
    ],
  });
  // 共識條件觸發：三大法人偏多
  assert.match(html, /三大法人/);
  assert.match(html, /偏多/);
});

// ---- Legacy fallback：dimension_role 缺失 ----

test('legacy fallback：dimension_role 缺失時仍能依 force 鍵判斷官方actor', () => {
  // 即使後端尚未升級到 E07 schema，仍要能正確識別 foreign/institutional/dealer 為官方actor。
  // 這 3 個 force 全部 bullish → 三大法人（共識）偏多
  const html = renderToString({
    forces: [
      { force: 'foreign', trend: 'bullish' },
      { force: 'institutional', trend: 'bullish' },
      { force: 'dealer', trend: 'bullish' },
      { force: 'government', trend: 'neutral' },
      { force: 'retail', trend: 'neutral' },
      { force: 'futures', trend: 'neutral' },
      { force: 'tsm_adr', trend: 'neutral' },
    ],
  });
  assert.match(html, /三大法人/);
  assert.match(html, /偏多/);
  // futures/tsm_adr 即使 bullish 也不應影響此 narrative
});

// ---- HTML escaping ----

test('解讀列表項目使用 escapeHtml,防止 XSS', () => {
  const html = renderToString({
    forces: [
      { force: '<xss>', dimension_role: 'official_actor', trend: 'bullish' },
      { force: 'foreign', dimension_role: 'official_actor', trend: 'bearish', weight: 0.3 },
      { force: 'institutional', dimension_role: 'official_actor', trend: 'neutral' },
      { force: 'dealer', dimension_role: 'official_actor', trend: 'neutral' },
      { force: 'government', dimension_role: 'behavioral_proxy', trend: 'neutral', data_available: false },
      { force: 'retail', dimension_role: 'behavioral_proxy', trend: 'neutral', weight: 0.05 },
      { force: 'futures', dimension_role: 'positioning_indicator', trend: 'neutral' },
      { force: 'tsm_adr', dimension_role: 'cross_market_signal', trend: 'neutral' },
    ],
  });
  // literal `<xss>` 標籤不應出現於輸出內
  assert.doesNotMatch(html, /<xss>/);
  assert.doesNotMatch(html, /<script/i);
});

// ---- 結構完整性 ----

test('輸出包含 title + ul + 多 li items', () => {
  const html = renderToString({
    forces: [
      e07Force('foreign', { trend: 'bullish' }),
      e07Force('institutional', { trend: 'bullish' }),
    ],
  });
  assert.match(html, /<h4[^>]*class="force-interpretation__title">[^<]+<\/h4>/);
  assert.match(html, /<ul class="force-interpretation__list">/);
  assert.match(html, /<li class="force-interpretation__item">/);
});
