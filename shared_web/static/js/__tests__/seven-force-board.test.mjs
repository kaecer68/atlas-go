// shared_web/static/js/__tests__/seven-force-board.test.mjs
//
// Unit tests for seven-force-board.js renderSevenForceBoard().
//
// 「七維錢潮雷達」3+2+2 分層渲染：
//   - 官方法人 (官方actortier, 3 forces: foreign/institutional/dealer)
//   - 行為代理 (2 forces: government/retail)
//   - 領先／跨市場訊號 (2 forces: futures/tsm_adr)
//
// 設計語意見 `docs/specs/capital-flow-seven-dimension-spec.md` §4 D-CF-04。
//
// 新 contract：
//   - 完全不渲染 force.weight 字串（避免跨單位裸權重；spec §7.2 / CF-INV-07）
//   - government data_available=false → 顯示「資料不足」badge（**非**「觀望」）
//   - 不渲染「7/7」字串
//   - 不渲染七個同級 card（改為 3+2+2 三個 <section data-tier>）
//
// 向後相容：
//   - response 沒有 dimension_role 時 fallback 到 FORCE_LABELS（既有 7 筆 mapping 仍可運作）。
//   - legacy Weight 欄位仍 parse，但不渲染字串。
//   - PascalCase 仍可（Trend/ZScore/RawValue/Weight 後備欄位）。
//
// 執行：node --test shared_web/static/js/__tests__/seven-force-board.test.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { renderSevenForceBoard } from '../components/seven-force-board.js';

// helper: 跑 render function 後回傳 mock container 的 innerHTML
function renderToString(summary) {
  const container = { innerHTML: '' };
  renderSevenForceBoard(container, summary);
  return container.innerHTML;
}

// Helper for new E07 contract: each force carries dimension_role and
// data_available. Tests below use this helper to avoid repeating boilerplate.
function e07Force(name, opts = {}) {
  return {
    force: name,
    dimension_role: opts.dimension_role || 'official_actor',
    trend: opts.trend || 'neutral',
    z_score: opts.z_score || 0,
    raw_value: opts.raw_value || 0,
    data_available: opts.data_available !== undefined ? opts.data_available : true,
    source_id: opts.source_id || 'SRC-TWSE-T86',
    weight: opts.weight || 0,
    weight_deprecated: true,
  };
}

// ---- Defensive: null container / missing data ----

test('container=null → silently no-op', () => {
  assert.doesNotThrow(() => renderSevenForceBoard(null, { forces: [{ force: 'foreign', dimension_role: 'official_actor', trend: 'bullish' }] }));
  assert.doesNotThrow(() => renderSevenForceBoard(undefined, { forces: [] }));
});

test('summary=null → 顯示「資料載入中」placeholder', () => {
  const html = renderToString(null);
  assert.match(html, /尚無.*資料|資料載入|七維錢潮雷達/);
  assert.match(html, /home-loading-card/);
});

test('summary undefined → placeholder', () => {
  const html = renderToString(undefined);
  assert.match(html, /尚無.*資料|資料載入|七維錢潮雷達/);
});

test('summary.forces 缺失 → placeholder', () => {
  const html = renderToString({ score: 50 });
  assert.match(html, /尚無.*資料|資料載入|七維錢潮雷達/);
});

test('summary.forces 不是 array → placeholder', () => {
  const html = renderToString({ forces: 'not-an-array' });
  assert.match(html, /尚無.*資料|資料載入|七維錢潮雷達/);
});

test('summary.forces 是空 array → placeholder', () => {
  const html = renderToString({ forces: [] });
  assert.match(html, /尚無.*資料|資料載入|七維錢潮雷達/);
});

// ---- 3+2+2 分層 (RED contract for spec §4 D-CF-04) ----

test('3+2+2：7 個 force 在三層 tier 內分組', () => {
  const html = renderToString({
    forces: [
      e07Force('foreign', { dimension_role: 'official_actor', trend: 'bullish', z_score: 1.2 }),
      e07Force('institutional', { dimension_role: 'official_actor', trend: 'bullish', z_score: 0.8 }),
      e07Force('dealer', { dimension_role: 'official_actor', trend: 'neutral', z_score: 0.0 }),
      e07Force('government', { dimension_role: 'behavioral_proxy', trend: 'bearish', z_score: -0.6 }),
      e07Force('retail', { dimension_role: 'behavioral_proxy', trend: 'neutral', z_score: 0.0 }),
      e07Force('futures', { dimension_role: 'positioning_indicator', trend: 'bullish', z_score: 1.1 }),
      e07Force('tsm_adr', { dimension_role: 'cross_market_signal', trend: 'bullish', z_score: 0.9 }),
    ],
  });
  assert.match(html, /<section[^>]*data-tier="official_actor"/);
  assert.match(html, /<section[^>]*data-tier="behavioral_proxy"/);
  assert.match(html, /<section[^>]*data-tier="market_signal"/);
  // 官方法人卡 3 張
  const officer = html.match(/<section[^>]*data-tier="official_actor"[\s\S]*?<\/section>/);
  assert.ok(officer, '官方actortier section 應存在');
  const officerCards = officer[0].match(/class="force-card /g) || [];
  assert.equal(officerCards.length, 3, '官方法人層應有 3 張卡');
  // 行為代理卡 2 張
  const proxy = html.match(/<section[^>]*data-tier="behavioral_proxy"[\s\S]*?<\/section>/);
  assert.ok(proxy, '行為代理 tier section 應存在');
  const proxyCards = proxy[0].match(/class="force-card /g) || [];
  assert.equal(proxyCards.length, 2, '行為代理層應有 2 張卡');
  // 訊號卡 2 張（含 futures + tsm_adr 對齊到同一 tier）
  const signal = html.match(/<section[^>]*data-tier="market_signal"[\s\S]*?<\/section>/);
  assert.ok(signal, '領先／跨市場訊號 tier section 應存在');
  const signalCards = signal[0].match(/class="force-card /g) || [];
  assert.equal(signalCards.length, 2, '訊號層應有 2 張卡');
});

test('各 tier 含中文標題（官方法人／行為代理／領先／跨市場訊號）', () => {
  const html = renderToString({
    forces: [
      e07Force('foreign', { dimension_role: 'official_actor' }),
      e07Force('institutional', { dimension_role: 'official_actor' }),
      e07Force('dealer', { dimension_role: 'official_actor' }),
      e07Force('government', { dimension_role: 'behavioral_proxy' }),
      e07Force('retail', { dimension_role: 'behavioral_proxy' }),
      e07Force('futures', { dimension_role: 'positioning_indicator' }),
      e07Force('tsm_adr', { dimension_role: 'cross_market_signal' }),
    ],
  });
  assert.match(html, /官方法人/);
  assert.match(html, /行為代理/);
  assert.match(html, /領先／跨市場訊號|領先.+跨市場訊號/);
});

test('不渲染 force.weight 字串（zero/deprecated）', () => {
  const html = renderToString({
    forces: [
      e07Force('foreign', { weight: 0.35 }),
      e07Force('institutional', { weight: 0.30 }),
      e07Force('dealer', { weight: 0.20 }),
      e07Force('government', { weight: 0.10 }),
      e07Force('retail', { weight: 0.05 }),
    ],
  });
  assert.doesNotMatch(html, /權重/);
  // 權重 X% 格式（X 為整數百分比，中間含空白），不包含 style "width: X%"
  assert.doesNotMatch(html, /權重\s*\d+%/);
  assert.doesNotMatch(html, />\s*\d+%\s*</);
});

test('不渲染「7/7」字串（替代為層級共識）', () => {
  const html = renderToString({
    forces: [
      e07Force('foreign', { trend: 'bullish' }),
      e07Force('institutional', { trend: 'bullish' }),
      e07Force('dealer', { trend: 'bullish' }),
    ],
  });
  assert.doesNotMatch(html, /7\/7/);
  assert.doesNotMatch(html, /七大勢力/);
});

// ---- 政府 unavailable → 資料不足（spec §4 D-CF-04 / CF-INV 行為代理） ----

test('government data_available=false → 顯示「資料不足」(非「觀望」)', () => {
  const html = renderToString({
    forces: [
      e07Force('foreign', { dimension_role: 'official_actor', trend: 'bullish' }),
      e07Force('institutional', { dimension_role: 'official_actor', trend: 'bullish' }),
      e07Force('dealer', { dimension_role: 'official_actor', trend: 'bullish' }),
      // government 缺資料
      e07Force('government', { dimension_role: 'behavioral_proxy', trend: 'neutral', z_score: 0, raw_value: 0, data_available: false }),
      e07Force('retail', { dimension_role: 'behavioral_proxy', trend: 'neutral' }),
    ],
  });
  // 必須顯示「資料不足」
  assert.match(html, /資料不足/);
  // 不允許在 government card 內以「觀望」作為語意（其他中立 force 不受影響）
  const govSection = html.match(/data-tier="behavioral_proxy"[\s\S]*?<\/section>/);
  assert.ok(govSection);
  // 在 behavioral_proxy section 中，government 那張卡應有「資料不足」標記
  assert.match(govSection[0], /資料不足/);
});

test('官方actor/訊號 data_available=false 也走「資料不足」（行為統一）', () => {
  const html = renderToString({
    forces: [
      e07Force('foreign', { dimension_role: 'official_actor', trend: 'bullish', data_available: true }),
      e07Force('institutional', { dimension_role: 'official_actor', trend: 'neutral', data_available: true }),
      e07Force('dealer', { dimension_role: 'official_actor', trend: 'neutral', data_available: false }),
    ],
  });
  assert.match(html, /資料不足/);
});

// ---- 三種 trend tone ----

test('bullish trend → 偏多 + force-card--positive tone', () => {
  const html = renderToString({
    forces: [e07Force('foreign', { trend: 'bullish', z_score: 1.5, raw_value: 120.3 })],
  });
  assert.match(html, /force-card--positive/);
  assert.match(html, /偏多/);
  assert.match(html, /外資/); // FORCE_LABELS mapping
});

test('bearish trend → 偏空 + force-card--negative tone', () => {
  const html = renderToString({
    forces: [e07Force('retail', { dimension_role: 'behavioral_proxy', trend: 'bearish', z_score: -2.0, raw_value: -50.0 })],
  });
  assert.match(html, /force-card--negative/);
  assert.match(html, /偏空/);
  assert.match(html, /散戶/);
});

test('neutral trend + data_available=true → 觀望 + force-card--neutral tone', () => {
  // 注意：neutral 仍可顯示「觀望」（這是 trend 本身的語意），但
  // data_available=false 的 force 一律顯示「資料不足」。
  const html = renderToString({
    forces: [e07Force('dealer', { trend: 'neutral', z_score: 0, raw_value: 0 })],
  });
  assert.match(html, /force-card--neutral/);
  assert.match(html, /觀望/);
  assert.match(html, /自營商/);
});

test('trend 缺失 → 預設 neutral', () => {
  const html = renderToString({
    forces: [{ force: 'dealer', dimension_role: 'official_actor' }],
  });
  assert.match(html, /force-card--neutral/);
  assert.match(html, /觀望/);
});

test('trend=unknown 字串(非 bullish/bearish) → 落入 neutral', () => {
  const html = renderToString({
    forces: [e07Force('futures', { dimension_role: 'positioning_indicator', trend: 'sideways' })],
  });
  assert.match(html, /force-card--neutral/);
});

// ---- Z-score → strength bar ----
// strength = clamp(|z_score|/3, 0..1) → percent

test('z_score=0 → strength bar 寬度 = 0%', () => {
  const html = renderToString({
    forces: [e07Force('foreign', { trend: 'neutral', z_score: 0 })],
  });
  assert.match(html, /width: 0%/);
});

test('z_score=1.5 → strength bar 寬度 = 50%', () => {
  const html = renderToString({
    forces: [e07Force('foreign', { trend: 'bullish', z_score: 1.5 })],
  });
  assert.match(html, /width: 50%/);
});

test('z_score=-2.4 → strength bar 寬度 = 80% (取絕對值)', () => {
  const html = renderToString({
    forces: [e07Force('foreign', { trend: 'bearish', z_score: -2.4 })],
  });
  assert.match(html, /width: 80%/);
});

test('z_score=3 → strength bar 寬度 = 100% (capped at 1)', () => {
  const html = renderToString({
    forces: [e07Force('foreign', { trend: 'bullish', z_score: 3.0 })],
  });
  assert.match(html, /width: 100%/);
});

test('z_score=10 → strength bar 寬度 = 100% (即使超出也 clamp)', () => {
  const html = renderToString({
    forces: [e07Force('foreign', { trend: 'bullish', z_score: 10.0 })],
  });
  assert.match(html, /width: 100%/);
});

test('z_score 缺失 → bar 寬度 0%', () => {
  const html = renderToString({
    forces: [{ force: 'foreign', dimension_role: 'official_actor', trend: 'bullish' }],
  });
  assert.match(html, /width: 0%/);
});

// ---- raw_value (signed 億) ----

test('raw_value=120.3 → 顯示 +120.3 億', () => {
  const html = renderToString({
    forces: [e07Force('foreign', { trend: 'bullish', raw_value: 120.3 })],
  });
  assert.match(html, /\+120\.3.*億/);
});

test('raw_value=-50.7 → 顯示 −50.7 億 (Unicode minus U+2212 保留)', () => {
  const html = renderToString({
    forces: [e07Force('retail', { dimension_role: 'behavioral_proxy', trend: 'bearish', raw_value: -50.7 })],
  });
  assert.match(html, /−50\.7.*億/);
  assert.doesNotMatch(html, /-50\.7 億/);  // 確保不是 ASCII hyphen
});

test('raw_value=0 → 顯示 0.0 億', () => {
  const html = renderToString({
    forces: [e07Force('government', { dimension_role: 'behavioral_proxy', trend: 'neutral', raw_value: 0, data_available: true })],
  });
  assert.match(html, /0\.0.*億/);
});

test('raw_value 缺失 → 顯示 —', () => {
  const html = renderToString({
    forces: [{ force: 'futures', dimension_role: 'positioning_indicator', trend: 'bullish' }],
  });
  assert.match(html, /force-card__value"[^>]*>—</);
});

// ---- PascalCase 後備欄位 ----

test('backend 用 PascalCase (Trend/ZScore/RawValue/Weight) 也能渲染', () => {
  const html = renderToString({
    forces: [{
      Force: 'foreign',
      DimensionRole: 'official_actor',
      Trend: 'BULLISH',  // 大寫
      ZScore: 1.8,
      RawValue: 88.5,
      Weight: 0.42,
      DataAvailable: true,
    }],
  });
  assert.match(html, /force-card--positive/);
  assert.match(html, /偏多/);
  assert.match(html, /width: 60%/); // 1.8/3 = 0.6 → 60%
  assert.match(html, /\+88\.5.*億/);
  // Weight 仍 parse，但不渲染字串（contract: spec §7.2 / CF-INV-07）
  assert.doesNotMatch(html, /權重/);
  assert.match(html, /外資/);
});

// ---- 既有 7-force legacy fallback 測試（保留） ----

test('legacy fallback：dimension_role 缺失時回退到 7 個 FORCE_LABELS（向後相容）', () => {
  // response 還未升級 E07 schema 時（沒有 dimension_role），仍能正確映射中文 label。
  const allForces = [
    { force: 'foreign' },
    { force: 'institutional' },
    { force: 'dealer' },
    { force: 'retail' },
    { force: 'government' },
    { force: 'futures' },
    { force: 'tsm_adr' },
  ];
  const html = renderToString({ forces: allForces });
  assert.match(html, /外資/);
  assert.match(html, /投信/);
  assert.match(html, /自營商/);
  assert.match(html, /散戶/);
  assert.match(html, /政府\/公股行庫/);
  assert.match(html, /期貨/);
  assert.match(html, /TSM ADR/);
});

test('未知 force 名稱 → 直接顯示原名 (沒有 mapping)', () => {
  const html = renderToString({
    forces: [{ force: 'mystery_force', dimension_role: 'official_actor', trend: 'bullish' }],
  });
  assert.match(html, /mystery_force/);
  assert.match(html, /force-card__name[^>]*>mystery_force</);
});

test('多個 forces 依序渲染,snake_case 容錯', () => {
  const html = renderToString({
    forces: [
      e07Force('foreign', { trend: 'bullish', z_score: 1.0 }),
      e07Force('institutional', { trend: 'bullish', z_score: 1.5 }),
      e07Force('retail', { dimension_role: 'behavioral_proxy', trend: 'bearish', z_score: -0.5 }),
    ],
  });
  // 確認有三張卡
  const matches = html.match(/class="force-card /g) || [];
  assert.equal(matches.length, 3);
});

// ---- HTML escaping ----

test('force label 含 XSS payload → escape 成純文字', () => {
  const html = renderToString({
    forces: [{ force: '<script>alert(1)</script>', dimension_role: 'official_actor', trend: 'bullish' }],
  });
  // escapeHtml 應轉義 < > 與引號
  assert.doesNotMatch(html, /<script>alert/);
  assert.match(html, /&lt;script&gt;/);
});
