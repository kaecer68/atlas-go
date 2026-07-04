/**
 * Narrative event theme → 中文標籤對映表
 * 與 internal/narrative/templates.go 24 個主題同步
 * 未知主題會回傳原始 theme ID 並 console.warn
 */

export const THEME_LABELS = {
  // 美國宏觀
  US_rates_up:    '美升息／鷹派',
  US_rates_down:  '美降息／鴿派',
  dollar_surge:   '美元強勢',
  inflation_spike: '通膨升溫',

  // 日本
  JPY_carry_unwind: '日圓套利平倉',

  // AI / 科技
  AI_capex_surge: 'AI 資本支出激增',
  tech_peak_season: '科技旺季',
  semiconductor_downturn: '半導體週期下行',
  earnings_surprise: '財報驚喜',
  earnings_blackout: '財報空窗期',

  // 地緣政治
  geopolitical_risk_spike: '地緣風險飆升',
  taiwan_political_risk: '台海地緣風險',
  tariff_shock: '關稅衝擊',

  // 原物料
  oil_price_shock: '油價衝擊',
  gold_rally: '黃金避險',
  shipping_rate_spike: '運價飆升',

  // 台灣季節性
  spring_festival_season: '春節行情',
  dividend_season: '除權息旺季',
  year_end_window_dressing: '年底作帳',
  election_cycle: '選舉週期',

  // 台灣市場
  taiwan_export_boom: '出口強勁',
  retail_institutional_divergence: '散戶機構分歧',
  USD_TWD_volatility: '台幣劇烈波動',

  // 中國
  china_slowdown: '中國經濟放緩',
};

/**
 * getThemeLabel — 取得主題中文標籤，含未知主題 fallback
 * @param {string} theme - narrative event theme (snake_case)
 * @returns {string} 中文標籤
 */
export function getThemeLabel(theme) {
  if (!theme) return '未知事件';
  const label = THEME_LABELS[theme];
  if (label) return label;
  console.warn('[getThemeLabel] unknown theme:', theme);
  return theme;
}
