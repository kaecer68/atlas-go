export { fmt, fmtPct, fmtFloat, fmtInt, pnlColor, pnlSign, convColor, escapeHtml, emptyState } from './utils.js';

export function agentName(id) {
  return AGENT_NAME_MAP[id] || id;
}

export function regimeLabel(regime) {
  const m = { 'RISK_ON': '多頭', 'RISK_OFF': '空頭', 'NEUTRAL': '盤整' };
  return m[regime] || regime || '-';
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

export const AGENT_NAME_MAP = {
  'cro-01': '風控長',
  'cio-01': '投資長',
  'taiwan-macro-01': '台灣總經',
  'etf-rotation-01': 'ETF 輪動',
  'value-yield-01': '價值收益',
  'growth-momentum-01': '成長動能',
  'technical-breakout-01': '技術突破',
  'financials-desk-01': '金融產業',
  'semi-desk-01': '半導體產業',
  'ai-desk-01': 'AI 供應鏈',
  'shipping-desk-01': '航運產業',
  'earnings-quality-01': '盈餘品質',
  'leo-satellite-desk-01': '低軌衛星',
  'robotics-desk-01': '機器人產業',
  'mining-desk-01': '礦業/貴金屬',
  'energy-desk-01': '能源產業',
  'electronics-desk-01': '電子零組件',
  'consumer-desk-01': '消費產業',
  'industrial-desk-01': '工業/製造',
  'etf_rotation_desk': 'ETF 輪動',
  'value_yield': '價值收益',
  'growth_momentum': '成長動能',
  'technical_breakout': '技術突破',
  'financials_desk': '金融產業',
  'semiconductor_desk': '半導體產業',
  'ai_supply_chain_desk': 'AI 供應鏈',
  'shipping_desk': '航運產業',
  'earnings_quality': '盈餘品質',
  'leo_satellite_desk': '低軌衛星',
  'robotics_desk': '機器人產業',
  'mining_desk': '礦業/貴金屬',
  'energy_desk': '能源產業',
  'electronics_desk': '電子零組件',
  'consumer_desk': '消費產業',
  'industrial_desk': '工業/製造',
  'cro_risk': '風控長',
  'cio_portfolio': '投資長',
};

export const PAGE_TITLES = {
  overview: '總覽', live: '風控結果', pipeline: '投資管線',
  decision: '決策鏈', agents: 'AI 觀測台', experiments: '模擬交易',
  reports: '最新回測', controls: '控制與稽核', datachannels: '信息通道',
  synergy: '人機協同', alerts: '系統警報', metrics: '指標監控',
  industry: '產業生態系', portfolio: '組合持倉', parameters: '參數管理'
};
