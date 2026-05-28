export { fmt, fmtPct, fmtFloat, fmtInt, pnlColor, pnlSign, convColor, escapeHtml, emptyState } from './utils.js';
export { FIELD } from './field_names.js';

export function agentName(id) {
  return AGENT_NAME_MAP[id] || id;
}

export function regimeLabel(regime) {
  const m = { 'RISK_ON': '多頭', 'RISK_OFF': '空頭', 'NEUTRAL': '盤整' };
  return m[regime] || regime || '-';
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
  'etf_rotation_desk': 'ETF 輪動',
  'value_yield': '價值收益',
  'growth_momentum': '成長動能',
  'technical_breakout': '技術突破',
  'financials_desk': '金融產業',
  'semiconductor_desk': '半導體產業',
  'ai_supply_chain_desk': 'AI 供應鏈',
  'shipping_desk': '航運產業',
  'earnings_quality': '盈餘品質',
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
