// shared_web/static/js/__tests__/methodology-layer2-macro.test.mjs
//
// 2026-08-19 工單 test：methodology 第二層公開數據開放（/api/macro/snapshot/latest）+ 會員鎖住三原則
//  + 原則C「集中板塊 redesign」落地（#md-premium-dashboard）：
//    - 層卡內不再渲染「升級查看即時數值」pill；被鎖即時值（report.* / capital.forces.*）
//      於層卡內只顯示教育標籤「即時數值於 Premium 儀表板」。
//    - 公開數值（cross.* / macro.* / 台積電月營收 / 電子出口）對所有 tier 開放顯示。
//    - #md-premium-dashboard：非 premium 顯示單一門檻；premium 顯示即時數值。
//
// Run: node --test shared_web/static/js/__tests__/methodology-layer2-macro.test.mjs
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { renderMetricCell, renderChain, renderPremiumDashboard } from '../page-shells/methodology.js';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------
const NO_CAPITAL = null;
const NO_HISTORY = null;

// /api/macro/snapshot/latest 實際資料形狀（MacroDataPoint, 公開、無 tier gate）
const MACRO = {
  export_electronics: { symbol: 'TW_EXPORT_ELECTRONICS', value: 2.36, change_pct: -4.8, timestamp: 0 },
  tsmc_revenue:       { symbol: '2330.TW', value: 467580548000, change_pct: 44.7, timestamp: 0 },
};

// /api/cross-market/status（公開、所有 tier 抓）
const CROSS = {
  sox:     { value: 5200.5, change_pct: 1.2 },
  tsm_adr: { value: 180.3, change_pct: -0.5 },
  nvda:    { value: 135.2, change_pct: 2.1 },
  ndx:     { value: 21000, change_pct: 0.4 },
  usd_twd: { value: 31.8, change_pct: -0.3 },
};

// 鏈圖 fake host：node 無 DOM，renderChain 只會寫 innerHTML + querySelectorAll（回傳空）
function makeChainHost() {
  return { innerHTML: '', querySelectorAll: () => [] };
}

// Premium 儀表板 fake host：提供 #md-premium-body 容器
function makePremiumHost() {
  const body = { innerHTML: '' };
  const host = {
    body,
    querySelector(sel) { return sel === '#md-premium-body' ? body : null; },
  };
  return { host, body };
}

// /api/reports/latest 的 premium 範例（市場時期判定 + global + capital + strategy）
function makeReport() {
  return {
    period: {
      market_period: 'downturn',
      period_name_zh: '低迷',
      cash_reserve: 0.45,
      confidence: 0.6,
      conditions_hit: 3,
      conditions_total: 5,
      triggered_indicators: [
        { name: 'US10Y', value: 4.2, relation: '>', threshold: 4.0, hit: true, input_available: true },
      ],
      strategies_detail: [{ id: 'def1', name: '防禦策略', category: 'defensive', priority: 'primary' }],
      allowed_strategies: ['def1'],
    },
    global: {
      status: 'risk_off',
      bond_yield: 4.2,
      usd_index: 104.5,
      jpy: 151.2,
      vix: 18.3,
      summary: '利率高檔壓抑風險資產',
    },
    capital: { resonance: 0.72, quality: 'good' },
    strategy: { active_strategy: 'def1', direction: 'neutral', entry_condition: '等待外資回流' },
    events: { tomorrow: ['Fed 演說'], this_week: ['MSCI 調整'] },
    summary: '利率高檔壓抑風險資產',
  };
}

// ---------------------------------------------------------------------------
// 原則C：層卡內被鎖即時值不再顯示「升級查看即時數值」pill
// ---------------------------------------------------------------------------
test('原則C：report.*（第零層）於層卡內不顯示 pill，改顯示教育標籤', () => {
  const metric = { key: 'us10y', label: '美債殖利率', en: 'US 10Y', source: 'report.global.bond_yield' };
  const html = renderMetricCell(metric, null, null, NO_CAPITAL, NO_HISTORY, MACRO, false);
  assert.ok(!html.includes('升級查看即時數值'), '層卡不得再有 pill');
  assert.ok(html.includes('即時數值於 Premium 儀表板'), '應顯示教育標籤，實際: ' + html);
});

test('原則C：capital.forces.*（第三層外資）於層卡內不顯示 pill，改顯示教育標籤', () => {
  const metric = { key: 'foreign', label: '外資現貨', en: 'Foreign Spot', source: 'capital.forces.foreign', kind: 'force' };
  const html = renderMetricCell(metric, null, null, null, NO_HISTORY, MACRO, false);
  assert.ok(!html.includes('升級查看即時數值'));
  assert.ok(html.includes('即時數值於 Premium 儀表板'));
});

test('原則C：整條鏈（renderChain）不包含「升級查看即時數值」pill，且公開值照常顯示', () => {
  const host = makeChainHost();
  renderChain(host, CROSS, MACRO);
  assert.ok(!host.innerHTML.includes('升級查看即時數值'), '鏈圖不應再有 pill');
  // 公開值（第一層美股）顯示真實值
  assert.ok(host.innerHTML.includes('5,200.5'), 'SOX 公開值顯示，實際: ' + host.innerHTML);
  // 被鎖值改為教育標籤（不出現數值）
  assert.ok(host.innerHTML.includes('即時數值於 Premium 儀表板'), '被鎖值以教育標籤呈現');
  // 資料源未接入仍在
  assert.ok(host.innerHTML.includes('資料源未接入'), '維持資料源未接入佔位');
});

test('公開值在所有 tier 顯示：renderChain 的跨層公開值對 free 也顯示', () => {
  const host = makeChainHost();
  renderChain(host, CROSS, MACRO);
  assert.ok(host.innerHTML.includes('4,675.8'), '台積電月營收公開值顯示');
  assert.ok(host.innerHTML.includes('31.8'), 'USD/TWD 公開值顯示');
  assert.ok(host.innerHTML.includes('2.36'), '電子出口公開值顯示');
});

// ---------------------------------------------------------------------------
// Premium 儀表板：非 premium 顯示單一門檻（原則C）
// ---------------------------------------------------------------------------
test('Premium 儀表板：非 premium 顯示單一門檻「升級 Premium 解鎖即時儀表板」', () => {
  const { host, body } = makePremiumHost();
  renderPremiumDashboard(host, null, null, null, null, false, false);
  assert.ok(body.innerHTML.includes('升級 Premium 解鎖即時儀表板'), '應顯示單一門檻，實際: ' + body.innerHTML);
  assert.ok(!body.innerHTML.includes('升級查看即時數值'), '儀表板門檻不再用卡片內 pill 文案');
  assert.ok(!body.innerHTML.includes('即時數值於 Premium 儀表板'), '非 premium 不顯示任何即時值');
});

// ---------------------------------------------------------------------------
// Premium 儀表板：premium 顯示即時數值
// ---------------------------------------------------------------------------
test('Premium 儀表板：premium 顯示市場時期即時判定、資金共振與策略推薦', () => {
  const report = makeReport();
  const { host, body } = makePremiumHost();
  renderPremiumDashboard(host, report, null, null, CROSS, true, false);

  // 市場時期即時判定
  assert.ok(body.innerHTML.includes('市場時期即時判定'), '含市場時期即時判定標題');
  assert.ok(body.innerHTML.includes('低迷'), 'period_name_zh 顯示');
  assert.ok(body.innerHTML.includes('45%'), 'cash_reserve 顯示 45%');
  assert.ok(body.innerHTML.includes('40-50%'), '憲章建議區間顯示');
  // 資金共振
  assert.ok(body.innerHTML.includes('資金共振'), '含資金共振');
  assert.ok(body.innerHTML.includes('0.72'), '共振係數顯示');
  assert.ok(body.innerHTML.includes('good'), '共振品質顯示');
  // 策略推薦
  assert.ok(body.innerHTML.includes('散戶策略推薦'), '含策略推薦標題');
  assert.ok(body.innerHTML.includes('防禦策略'), '策略 chip 顯示');
});

test('Premium 儀表板：premium 顯示八層即時數值（global/capital.forces/events/USD_TWD）', () => {
  const report = makeReport();
  const capital = {
    forces: [
      { force: 'foreign', z_score: 1.2, trend: 'bullish', data_available: true },
      { force: 'futures', z_score: -0.8, trend: 'bearish', data_available: true },
      { force: 'institutional', z_score: 0.3, trend: 'neutral', data_available: true },
      { force: 'government', z_score: 0.9, trend: 'bullish', data_available: true },
      { force: 'dealer', z_score: -0.4, trend: 'neutral', data_available: true },
      { force: 'retail', z_score: 2.2, trend: 'bullish', data_available: true },
    ],
  };
  const history = {
    foreign: [{ raw_value: 1 }, { raw_value: 2 }, { raw_value: 3 }],
  };
  const { host, body } = makePremiumHost();
  renderPremiumDashboard(host, report, capital, history, CROSS, true, false);

  // 第零層 global
  assert.ok(body.innerHTML.includes('4.2'), 'US10Y 顯示');
  assert.ok(body.innerHTML.includes('104.5'), 'DXY 顯示');
  // 資金勢力 z-score + 趨勢
  assert.ok(body.innerHTML.includes('1.20'), '外資 z-score 顯示');
  assert.ok(body.innerHTML.includes('偏多'), '外資趨勢偏多顯示');
  assert.ok(body.innerHTML.includes('偏空'), '期貨趨勢偏空顯示');
  // sparkline（capitalFlowHistory）
  assert.ok(body.innerHTML.includes('md-sparkline'), '資金勢力 sparkline 顯示');
  // 第七層 events + USD_TWD
  assert.ok(body.innerHTML.includes('Fed 演說'), '明日事件顯示');
  assert.ok(body.innerHTML.includes('MSCI 調整'), '本週事件顯示');
  assert.ok(body.innerHTML.includes('31.8'), 'USD/TWD 顯示');
});

test('Premium 儀表板：premium 但 report 未就緒 → 顯示 empty（不崩潰）', () => {
  const { host, body } = makePremiumHost();
  renderPremiumDashboard(host, null, null, null, CROSS, true, false);
  assert.ok(body.innerHTML.includes('載入失敗') || body.innerHTML.includes('尚未生成'), '應顯示空/錯誤態，實際: ' + body.innerHTML);
});

// ---------------------------------------------------------------------------
// 第二層公開數據：所有 tier（含 free）都顯示真實資料，不 gate
// ---------------------------------------------------------------------------
test('exports（電子出口）接 macro 公開數據，free tier 開放顯示值 + YoY', () => {
  const metric = { key: 'exports', label: '電子出口', en: 'Electronics Export', source: 'macro.export_electronics', numeric: true };
  // isPremium=false（free tier）也應顯示真實值
  const html = renderMetricCell(metric, null, null, NO_CAPITAL, NO_HISTORY, MACRO, false);
  assert.match(html, /2\.36/, 'exports 顯示值 2.36');
  assert.match(html, /−4\.8%|\-4\.8%|4\.8%/, 'exports 顯示 YoY');
  assert.ok(!html.includes('升級查看即時數值'), '公開數據不得顯示 tier gate');
  assert.ok(!html.includes('資料源未接入'), '不得顯示資料源未接入');
});

test('premium tier 的 exports 同樣顯示真實資料', () => {
  const metric = { key: 'exports', label: '電子出口', en: 'Electronics Export', source: 'macro.export_electronics', numeric: true };
  const html = renderMetricCell(metric, null, null, NO_CAPITAL, NO_HISTORY, MACRO, true);
  assert.match(html, /2\.36/);
  assert.ok(!html.includes('升級查看即時數值'));
});

test('tsm_rev（台積電月營收）顯示 億 + YoY，所有 tier 開放', () => {
  const metric = { key: 'tsm_rev', label: '台積電月營收', en: 'TSM Revenue', source: 'macro.tsmc_revenue', format: 'ntd-billions' };
  const html = renderMetricCell(metric, null, null, NO_CAPITAL, NO_HISTORY, MACRO, false);
  // 467580548000 元 → 4,675.8 億（除以 1e8）
  assert.match(html, /4,675\.8/, `期望 4,675.8 億，實際: ${html}`);
  assert.match(html, /億/);
  assert.match(html, /44\.7%|\+44\.7%/, '顯示 YoY +44.7%');
  assert.ok(!html.includes('升級查看即時數值'), '公開數據不得 gate');
  assert.ok(!html.includes('資料源未接入'));
});

test('semi_imp（半導體設備進口）真無資料源 → 維持「資料源未接入」', () => {
  const metric = { key: 'semi_imp', label: '半導體設備進口', en: 'Semi Equip Import', available: false };
  const html = renderMetricCell(metric, null, null, NO_CAPITAL, NO_HISTORY, null, false);
  assert.ok(html.includes('資料源未接入'), '維持資料源未接入佔位');
});

// ---------------------------------------------------------------------------
// macro source 失敗 / 缺欄位：不得編造，顯示「—」
// ---------------------------------------------------------------------------
test('macro 缺欄位（如半導體設備進口）→ 顯示 「—」，不 gate 也不編造', () => {
  // 假設 macro 沒有 semi_equipment_imports 來源的 metric 接進來
  const metric = { key: 'exports', label: '電子出口', en: 'Electronics Export', source: 'macro.export_electronics', numeric: true };
  const emptyMacro = {};
  const html = renderMetricCell(metric, null, null, NO_CAPITAL, NO_HISTORY, emptyMacro, false);
  assert.ok(html.includes('md-metric__value">—<'), `缺資料應顯示 —，實際: ${html}`);
  assert.ok(!html.includes('升級查看即時數值'));
});

test('macro API 整個失敗（macro=null）→ exports 顯示 「—」而非資料獲取失敗/資料源未接入', () => {
  const metric = { key: 'exports', label: '電子出口', en: 'Electronics Export', source: 'macro.export_electronics', numeric: true };
  const html = renderMetricCell(metric, null, null, NO_CAPITAL, NO_HISTORY, null, false);
  assert.ok(html.includes('md-metric__value">—<'));
});

// ---------------------------------------------------------------------------
// 回歸：既有 source 類型不被 macro 分支破壞
// ---------------------------------------------------------------------------
test('回歸：cross.* 來源仍正常（free tier 顯示資料獲取失敗當無資料）', () => {
  const metric = { key: 'sox', label: '費半', en: 'SOX', source: 'cross.sox', numeric: true };
  // empty cross → 資料獲取失敗
  const html = renderMetricCell(metric, null, {}, NO_CAPITAL, NO_HISTORY, MACRO, false);
  assert.ok(html.includes('資料獲取失敗'));
});

test('回歸：report.* 來源於層卡以教育標籤呈現（原則C，不再 gate pill）', () => {
  const metric = { key: 'us10y', label: '美債殖利率', en: 'US 10Y', source: 'report.global.bond_yield' };
  const html = renderMetricCell(metric, null, null, NO_CAPITAL, NO_HISTORY, MACRO, false);
  assert.ok(html.includes('即時數值於 Premium 儀表板'), '層卡顯示教育標籤');
  assert.ok(!html.includes('升級查看即時數值'), '層卡不再顯示 pill');
});

test('回歸：capital.forces.* 來源於層卡以教育標籤呈現（原則C，不再 gate pill）', () => {
  const metric = { key: 'foreign', label: '外資現貨', en: 'Foreign Spot', source: 'capital.forces.foreign', kind: 'force' };
  const html = renderMetricCell(metric, null, null, null, NO_HISTORY, MACRO, false);
  assert.ok(html.includes('即時數值於 Premium 儀表板'));
  assert.ok(!html.includes('升級查看即時數值'));
});
