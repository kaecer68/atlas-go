// shared_web/static/js/__tests__/methodology-layer2-macro.test.mjs
//
// 2026-08-19 工單 test：methodology 第二層公開數據開放（/api/macro/snapshot/latest）+ 會員鎖住三原則。
// - exports（電子出口）/ tsm_rev（台積電月營收）接 macro 公開數據，對所有 tier 開放顯示
// - semi_imp（半導體設備進口）真無資料源 → 維持「資料源未接入」
// - macro source 不破壞既有 source 類型（report.* / cross.* / capital.forces.*）
//
// Run: node --test shared_web/static/js/__tests__/methodology-layer2-macro.test.mjs
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { renderMetricCell } from '../page-shells/methodology.js';

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

test('回歸：report.* 來源對 free tier 仍 tier-gated（只鎖非公開數據）', () => {
  const metric = { key: 'us10y', label: '美債殖利率', en: 'US 10Y', source: 'report.global.bond_yield' };
  const html = renderMetricCell(metric, null, null, NO_CAPITAL, NO_HISTORY, MACRO, false);
  assert.ok(html.includes('升級查看即時數值'), 'report.* 非公開數據維持 gate');
});

test('回歸：capital.forces.* 來源對 free tier 仍 tier-gated', () => {
  const metric = { key: 'foreign', label: '外資現貨', en: 'Foreign Spot', source: 'capital.forces.foreign', kind: 'force' };
  const html = renderMetricCell(metric, null, null, null, NO_HISTORY, MACRO, false);
  assert.ok(html.includes('升級查看即時數值'), 'capital.forces.* 非 premium 維持 gate');
});
