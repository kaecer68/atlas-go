// web/static/js/__tests__/data-visibility.test.mjs
//
// P0 級別回歸測試：投資管線頁面在「孤兒 session」(summary.json 缺失) 情境下
// 必須正確顯示推薦明細，不得靜默回傳 0/0、不得顯示 Go zero time "1/1/1"。
//
// 對應 AGENTS.md 規範的 4 層資料可見性 (L1-L4)：前端 (L4) 必須在資料缺失
// 或不一致時主動暴露，絕不以零值掩蓋。
//
// 執行：node --test web/static/js/__tests__/data-visibility.test.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { computePipelineSummary } from '../pages/dashboard.js';
import { formatDate } from '../shared/app-utils.js';
import { regimeLabel as regimeLabelFromNames } from '../names.js';
import { regimeLabel as regimeLabelFromConstants } from '../shared/constants.js';

// ============================================================================
// P0-D: computePipelineSummary 必須在 guard_outcomes 為空時從 items 回退
// ============================================================================

test('computePipelineSummary: 標準情境 (有 guard_outcomes) 行為不變', () => {
  const guards = [
    { guard_id: 'cro_risk', input_count: 70, output_count: 5, passed: true },
    { guard_id: 'cio_portfolio', input_count: 5, output_count: 5, passed: true },
  ];
  const result = computePipelineSummary(guards);
  assert.equal(result.rawInputs, 70, 'rawInputs 應為第一個 guard 的 input_count');
  assert.equal(result.finalOutputs, 5, 'finalOutputs 應為最後一個 guard 的 output_count');
  assert.equal(result.filteredCount, 65, 'filteredCount 應為 70-5');
  assert.equal(result.guard.length, 2);
});

test('computePipelineSummary: 孤兒 session 情境 (guard_outcomes 空) 必須從 items 回退', () => {
  // 重現 user 報告的 bug: session-20260614-daily 沒有 summary.json，
  // guard_outcomes 為空陣列，但有 70 筆 recommendation_outcomes.jsonl 資料。
  // 修復前：回傳 {rawInputs:0, finalOutputs:0, filteredCount:0} → 頁面顯示「0 筆推薦、0 筆放行」
  // 修復後：必須從 items 推導正確數字，頁面才能顯示真實資料。
  const items = [
    { symbol: '2330.TW', passed_guards: true },
    { symbol: '2454.TW', passed_guards: true },
    { symbol: '2891.TW', passed_guards: true },
    { symbol: '3008.TW', passed_guards: true },
    { symbol: '6770.TW', passed_guards: false },
    { symbol: '2884.TW', passed_guards: false },
    { symbol: '2885.TW', passed_guards: false },
  ];
  const result = computePipelineSummary([], items);
  assert.equal(result.rawInputs, 7, 'rawInputs 必須是 items.length (總推薦數)');
  assert.equal(result.finalOutputs, 4, 'finalOutputs 必須是 items 中 passed_guards=true 的數量');
  assert.equal(result.filteredCount, 3, 'filteredCount 必須是 rawInputs - finalOutputs');
});

test('computePipelineSummary: 同時不傳 items 也不傳 guards 必須回傳 0/0 但不崩潰', () => {
  const result = computePipelineSummary([], []);
  assert.equal(result.rawInputs, 0);
  assert.equal(result.finalOutputs, 0);
  assert.equal(result.filteredCount, 0);
});

test('computePipelineSummary: items 為 undefined 時 (向後相容) 不崩潰', () => {
  // 既有呼叫端 (renderMacroRadar) 只傳 guard_outcomes，必須保持向後相容
  const result = computePipelineSummary([{ guard_id: 'g1', input_count: 10, output_count: 8, passed: true }]);
  assert.equal(result.rawInputs, 10);
  assert.equal(result.finalOutputs, 8);
});

test('computePipelineSummary: items 中 passed_guards 為 undefined 視為 true (legacy 資料)', () => {
  // 對齊 backend pipeline.go:621-623 的 legacy fallback 邏輯
  const items = [
    { symbol: 'A', passed_guards: true },
    { symbol: 'B' }, // 沒有 passed_guards 欄位
    { symbol: 'C', passed_guards: false },
  ];
  const result = computePipelineSummary([], items);
  assert.equal(result.rawInputs, 3);
  assert.equal(result.finalOutputs, 2, 'undefined passed_guards 視為 true');
});

// ============================================================================
// P0-C: formatDate 必須過濾 Go zero time (避免顯示 "1/1/1 上午8:00:00")
// ============================================================================

test('formatDate: 空值或 null 回傳 "-"', () => {
  assert.equal(formatDate(null), '-');
  assert.equal(formatDate(undefined), '-');
  assert.equal(formatDate(''), '-');
});

test('formatDate: Go zero time 字串 "0001-01-01T00:00:00Z" 必須回傳 "-"', () => {
  // Go time.Time zero value 序列化後是 "0001-01-01T00:00:00Z"，
  // 對應 zh-TW locale 顯示為 "1/1/1 上午8:00:00" → 對使用者無意義。
  assert.equal(formatDate('0001-01-01T00:00:00Z'), '-');
});

test('formatDate: Go zero time 帶時區 "0001-01-01T08:06:00+08:00" 必須回傳 "-"', () => {
  // 對齊 user 報告的 "1/1/1 上午8:06:00" 字串
  assert.equal(formatDate('0001-01-01T08:06:00+08:00'), '-');
});

test('formatDate: 合法日期字串正常顯示', () => {
  const result = formatDate('2026-06-14T04:00:00Z');
  assert.notEqual(result, '-', '合法日期不應回傳 "-"');
  // 不強制完整字串比對（locale 行為因環境而異），只確認非 fallback
  assert.ok(result.length > 5);
});

test('formatDate: 無效字串不崩潰，回傳 "-"', () => {
  assert.equal(formatDate('not-a-date'), '-');
  assert.equal(formatDate('9999-99-99'), '-');
});

// ============================================================================
// P0-E: regimeLabel 統一處理 unknown/empty
// ============================================================================

test('regimeLabel (names.js): 已知 regime 加上中文標籤', () => {
  assert.equal(regimeLabelFromNames('RISK_ON'), '風險趨向（RISK_ON）');
  assert.equal(regimeLabelFromNames('RISK_OFF'), '風險趨避（RISK_OFF）');
  assert.equal(regimeLabelFromNames('NEUTRAL'), '中性（NEUTRAL）');
});

test('regimeLabel (names.js): 未知 regime 必須回傳 "-" 而非 raw 字串', () => {
  // 修復前：regimeLabel('unknown') 回傳 'unknown'，regimeLabel('') 回傳 ''
  // 修復後：必須統一回傳 "-" (與其他空值處理一致)
  assert.equal(regimeLabelFromNames('unknown'), '-');
  assert.equal(regimeLabelFromNames(''), '-');
  assert.equal(regimeLabelFromNames(null), '-');
  assert.equal(regimeLabelFromNames(undefined), '-');
});

test('regimeLabel (constants.js): 簡化版標籤行為對齊 names.js', () => {
  // constants.js 的 regimeLabel 是另一個版本 (dashboard.js:404 inline) 的正名
  // 必須與 names.js 對齊，否則會出現同一頁面不同顯示
  assert.equal(regimeLabelFromConstants('RISK_ON'), '多頭');
  assert.equal(regimeLabelFromConstants('RISK_OFF'), '空頭');
  assert.equal(regimeLabelFromConstants('NEUTRAL'), '盤整');
  // 對齊 names.js 行為：unknown/empty 必須回傳 "-"
  assert.equal(regimeLabelFromConstants('unknown'), '-');
  assert.equal(regimeLabelFromConstants(''), '-');
  assert.equal(regimeLabelFromConstants(null), '-');
  assert.equal(regimeLabelFromConstants(undefined), '-');
});

// ============================================================================
// P1-A: buildPipelineStatusBanner 必須處理全部 5 種 status + is_fallback_session
// ============================================================================

import { buildPipelineStatusBanner } from '../pages/pipeline.js';

test('buildPipelineStatusBanner: status=ok 必須回傳空字串 (無 banner)', () => {
  const html = buildPipelineStatusBanner({ status: 'ok', status_message: 'ok' });
  assert.equal(html, '', 'ok 狀態不應顯示任何 banner');
});

test('buildPipelineStatusBanner: status=undefined 必須回傳空字串 (向後相容)', () => {
  // 既有 API response 沒有 status 欄位時,不得崩潰
  assert.equal(buildPipelineStatusBanner({}), '');
  assert.equal(buildPipelineStatusBanner(null), '');
  assert.equal(buildPipelineStatusBanner(undefined), '');
});

test('buildPipelineStatusBanner: status=degraded 必須顯示資料不完整 banner', () => {
  const html = buildPipelineStatusBanner({
    status: 'degraded',
    status_message: '控制層過濾記錄未載入（summary.json 缺失），推薦清單仍可用',
  });
  assert.match(html, /資料不完整/, '必須包含「資料不完整」徽章');
  assert.match(html, /控制層過濾記錄未載入/, '必須顯示後端 status_message');
  assert.match(html, /summary\.json/, '訊息必須包含後端具體原因');
});

test('buildPipelineStatusBanner: status=minimal 必須顯示「尚無推薦產出」banner', () => {
  // 修復前:pipeline.js 只判斷 'degraded',minimal 狀態被靜默忽略,
  // 頁面直接空白,使用者以為系統壞掉。
  const html = buildPipelineStatusBanner({
    status: 'minimal',
    status_message: '本場次尚無推薦產出記錄',
  });
  assert.match(html, /尚無推薦產出|無資料/, '必須明確告知使用者本場次無推薦');
  assert.match(html, /本場次尚無推薦產出記錄/, '必須包含後端 status_message');
});

test('buildPipelineStatusBanner: status=no_session 必須顯示「尚未執行任何場次」banner', () => {
  const html = buildPipelineStatusBanner({
    status: 'no_session',
    status_message: '尚未執行任何回測場次，請先執行回測',
  });
  assert.match(html, /尚未執行|請先執行/, '必須明確告知尚未執行回測');
  assert.match(html, /回測/, '必須提及回測');
});

test('buildPipelineStatusBanner: status=error 必須顯示錯誤 banner', () => {
  const html = buildPipelineStatusBanner({
    status: 'error',
    status_message: '載入推薦管線資料時發生錯誤',
  });
  assert.match(html, /錯誤|失敗/, '必須明確標示錯誤狀態');
  assert.match(html, /載入推薦管線資料時發生錯誤/, '必須包含後端錯誤訊息');
});

test('buildPipelineStatusBanner: is_fallback_session=true 必須顯示 fallback banner', () => {
  // fallback 與 degraded 是兩個獨立維度,可同時為 true。
  // 修復前:fallbackBanner 和 degradedBanner 是兩個獨立變數,可能只渲染其中一個。
  const html = buildPipelineStatusBanner({
    status: 'degraded',
    status_message: 'summary 缺失',
    is_fallback_session: true,
    fallback_message: '最新場次 session-X 尚無數據，已自動切換至 session-Y',
  });
  assert.match(html, /已自動切換/, '必須包含 fallback_message');
  assert.match(html, /資料不完整/, 'degraded 徽章也必須保留');
});

test('buildPipelineStatusBanner: 完整資料 (status=ok) 不得渲染任何 banner', () => {
  // 重要回歸測試:正常 session 不得被錯誤降級。
  const html = buildPipelineStatusBanner({
    session_id: 'session-20260610-daily',
    status: 'ok',
    status_message: '',
    is_fallback_session: false,
    fallback_message: '',
  });
  assert.equal(html, '', '正常 session 必須無 banner');
});
