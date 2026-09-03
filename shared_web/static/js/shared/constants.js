export { fmt, fmtPct, fmtFloat, fmtInt, pnlColor, pnlSign, convColor, escapeHtml, emptyState } from './utils.js';
export { AGENT_NAME_MAP, agentName } from '../names.js';

export function regimeLabel(regime) {
  if (!regime) return '-';
  const m = { 'RISK_ON': '多頭', 'RISK_OFF': '空頭', 'NEUTRAL': '盤整' };
  return m[regime] || '-';
}

// Narrative event theme labels (frontend canonical source of truth).
// Keep in sync with the 26 theme codes in `internal/narrative/lifecycle.go`.
// NOTE: A separate Go-side map in `internal/eventbus/eventbus.go` enriches SSE
// events with longer impact-analysis descriptions — intentionally separate.
// Unknown codes fall through to the raw snake_case value (null → "-").
export const NARRATIVE_THEME_LABELS = {
  AI_capex_surge: 'AI 資本支出加速',
  US_rates_up: '美債殖利率走升',
  US_rates_down: '美債殖利率走低',
  JPY_carry_unwind: '日圓套利平倉潮',
  geopolitical_risk_spike: '地緣政治風險升溫',
  oil_price_shock: '油價劇烈波動',
  taiwan_political_risk: '兩岸地緣政治風險',
  taiwan_export_boom: '台灣出口暢旺',
  semiconductor_downturn: '半導體景氣下行',
  semiconductor_cycle_peak: '半導體週期高峰',
  USD_TWD_volatility: '台幣匯率劇烈波動',
  retail_institutional_divergence: '散戶與法人看法分歧',
  gold_rally: '黃金多頭行情',
  dollar_surge: '美元指數急升',
  inflation_spike: '通膨升溫',
  earnings_surprise: '企業財報驚喜',
  spring_festival_season: '農曆春節效應',
  election_cycle: '選舉行情不確定性',
  earnings_blackout: '財報空窗期',
  tech_peak_season: '科技業出貨旺季',
  year_end_window_dressing: '年底作帳行情',
  Fed_emergency_cut: '聯準會緊急降息',
  china_slowdown: '中國經濟放緩',
  shipping_rate_spike: '運價飆升',
  dividend_season: '除權息旺季',
  tariff_shock: '關稅衝擊',
};

export function narrativeThemeLabel(theme) {
  if (!theme) return '-';
  return NARRATIVE_THEME_LABELS[theme] || theme;
}

export const PAGE_TITLES = {
  overview: '總覽', live: '風控結果', pipeline: '投資管線',
  decision: '決策鏈', agents: 'AI 觀測台', experiments: '模擬交易',
  reports: '最新回測', controls: '控制與稽核', datachannels: '信息通道',
  synergy: '人機協同', alerts: '系統警報', metrics: '指標監控',
  industry: '產業生態系', portfolio: '組合持倉', parameters: '參數管理'
};


// ============================================================================
// 列舉值中文化（risk-console UX refactor Phase 1, B5）
// 後端 enum 一律經前端映射表翻譯；對應 Go 定義：
//   - capital phase: internal/domain/types.go CapitalPhase
//   - business/inventory/capex cycle + trend: internal/industry/cycle.go
// 未列出的值原樣回傳（避免資料遺失），null/undefined 回 '-'。
// ============================================================================

export const CAPITAL_PHASE_LABELS = {
  simulation: '模擬',
  paper: '紙上交易',
  live: '實盤',
  full: '全額實盤',
};

export const BUSINESS_CYCLE_LABELS = {
  recovery: '復甦',
  expansion: '擴張',
  mature: '成熟',
  recession: '衰退',
};

export const INVENTORY_CYCLE_LABELS = {
  active_restocking: '主動補庫存',
  passive_restocking: '被動補庫存',
  active_destocking: '主動去庫存',
  passive_destocking: '被動去庫存',
};

export const CAPEX_CYCLE_LABELS = {
  expansion: '資本支出擴張',
  maintenance: '維護性支出',
  contraction: '資本支出緊縮',
};

export const TREND_LABELS = {
  up: '上升',
  down: '下降',
  stable: '持平',
};

/** 依映射表翻譯列舉值；未知值保留原文字串，null/undefined 回 '-'。 */
export function enumLabel(map, value) {
  if (value === null || value === undefined || value === '') return '-';
  return map[value] ?? String(value);
}

export function capitalPhaseLabel(value) {
  return enumLabel(CAPITAL_PHASE_LABELS, value);
}
export function businessCycleLabel(value) {
  return enumLabel(BUSINESS_CYCLE_LABELS, value);
}
export function inventoryCycleLabel(value) {
  return enumLabel(INVENTORY_CYCLE_LABELS, value);
}
export function capexCycleLabel(value) {
  return enumLabel(CAPEX_CYCLE_LABELS, value);
}
export function trendLabel(value) {
  return enumLabel(TREND_LABELS, value);
}
