// Name maps and lookup functions for the trading system

// Agent name mapping
export const AGENT_NAME_MAP = {
  'taiwan-macro-01': '台灣總經',
  'foreign-flow-01': '外資流向',
  'semi-desk-01': '半導體產業桌',
  'ai-desk-01': 'AI 供應鏈產業桌',
  'growth-momentum-01': '成長動能',
  'value-yield-01': '價值股息',
  'earnings-quality-01': '獲利品質',
  'technical-breakout-01': '技術突破',
  'financials-desk-01': '金融產業桌',
  'shipping-desk-01': '航運產業桌',
  'leo-satellite-desk-01': '低軌衛星',
  'etf-rotation-01': 'ETF 輪動',
  'cro-01': '風控長',
  'cio-01': '投資長',
  'super-dru-01': 'Druckenmiller 超級投資者',
  'super-asc-01': 'Aschenbrenner 超級投資者',
  'super-bak-01': 'Baker 超級投資者',
  'super-ack-01': 'Ackman 超級投資者',
  'alpha_discovery': '阿爾法發現'
};

export function agentName(id) { return AGENT_NAME_MAP[id] || id; }

// Stock name mapping
export const STOCK_NAME_MAP = {
  '0050.TW': '元大台灣50',
  '0056.TW': '元大高股息',
  '00878.TW': '國泰永續高股息',
  '1301.TW': '台塑',
  '1303.TW': '南亞',
  '1326.TW': '台化',
  '2303.TW': '聯電',
  '2308.TW': '台達電',
  '2317.TW': '鴻海',
  '2330.TW': '台積電',
  '2382.TW': '廣達',
  '2454.TW': '聯發科',
  '2603.TW': '長榮',
  '2609.TW': '陽明',
  '2615.TW': '萬海',
  '2881.TW': '富邦金',
  '2882.TW': '國泰金',
  '2886.TW': '兆豐金',
  '2891.TW': '中信金',
  '3008.TW': '大立光',
  '3017.TW': '奇鋐',
  '3034.TW': '聯詠',
  '3037.TW': '欣興',
  '6669.TW': '緯穎',
  // 常見個股無後綴映射
  '0050': '元大台灣50',
  '0056': '元大高股息',
  '00878': '國泰永續高股息',
  '1301': '台塑',
  '1303': '南亞',
  '1326': '台化',
  '2303': '聯電',
  '2308': '台達電',
  '2317': '鴻海',
  '2330': '台積電',
  '2382': '廣達',
  '2454': '聯發科',
  '2603': '長榮',
  '2609': '陽明',
  '2615': '萬海',
  '2881': '富邦金',
  '2882': '國泰金',
  '2886': '兆豐金',
  '2891': '中信金',
  '3008': '大立光',
  '3017': '奇鋐',
  '3034': '聯詠',
  '3037': '欣興',
  '6669': '緯穎'
};

export function stockName(symbol) { return STOCK_NAME_MAP[symbol] || (STOCK_NAME_MAP[(symbol || '').replace('.TW','')] || ''); }

// Regime name mapping
export const REGIME_NAME_MAP = {
  'NEUTRAL': '中性',
  'RISK_ON': '風險趨向',
  'RISK_OFF': '風險趨避'
};

export function regimeLabel(r) { return REGIME_NAME_MAP[r] ? `${REGIME_NAME_MAP[r]}（${r}）` : r; }

// Stress name mapping
export const STRESS_NAME_MAP = {
  'low': '低壓',
  'alert': '警戒',
  'high': '高壓',
  'crisis': '危機'
};

export function stressLabel(r) { return STRESS_NAME_MAP[r] || r; }

// Event name mapping
export const EVENT_NAME_MAP = {
  'US_rates_up': '美國升息',
  'US_rates_down': '美國降息',
  'AI_capex_surge': 'AI 資本支出激增',
  'geopolitical_tension': '地緣政治緊張',
  'geopolitical_risk_spike': '地緣政治風險飆升',
  'china_slowdown': '中國經濟放緩',
  'taiwan_export_boom': '台灣出口強勁',
  'semiconductor_cycle_peak': '半導體週期高峰',
  'financials_deregulation': '金融去管制',
  'shipping_rate_spike': '運價飆升',
  'oil_price_shock': '油價衝擊',
  'JPY_carry_unwind': '日圓套利平倉',
  'middle_east_escalation': '中東衝突升級'
};

export function eventName(theme) { return EVENT_NAME_MAP[theme] || theme; }

// Region name mapping
export const REGION_NAME_MAP = {
  'US': '美國',
  'Global': '全球',
  'Asia': '亞洲',
  'JP': '日本',
  'Middle East': '中東'
};

export function regionName(r) { return REGION_NAME_MAP[r] || r; }

// Capital flow name mapping
export const CAPITAL_FLOW_NAME_MAP = {
  'flight_to_USD': '美元避險',
  'risk_off': '風險趨避流出',
  'risk_on': '風險偏好流入',
  'tech_capex_inflow': '科技資本支出流入',
  'inflation_reprice': '通膨重新定價',
  'global_liquidity_drain': '全球流動性收緊'
};

export function capitalFlowName(cf) { return CAPITAL_FLOW_NAME_MAP[cf] || cf; }

// Model name mapping
export const MODEL_NAME_MAP = {
  '鷹派聯準會模型': '鷹派聯準會模型',
  'AI 超級週期模型': 'AI 超級週期模型',
  '地緣政治避險模型': '地緣政治避險模型'
};

export function modelName(m) { return MODEL_NAME_MAP[m] || m; }

// Template name mapping
export const TEMPLATE_NAME_MAP = {
  '美國升息 / 鷹派聯準會': '美國升息 / 鷹派聯準會',
  '日圓套利平倉': '日圓套利平倉',
  'AI 資本支出激增': 'AI 資本支出激增',
  '地緣政治風險飆升': '地緣政治風險飆升',
  '油價衝擊': '油價衝擊',
  '中東衝突升級': '中東衝突升級'
};

export function templateName(t) { return TEMPLATE_NAME_MAP[t] || t; }

// Sector name mapping
export const SECTOR_NAME_MAP = {
  'semiconductor': '半導體',
  'ai_supply_chain': 'AI 供應鏈',
  'financials': '金融',
  'shipping': '航運',
  'leo_satellite': '低軌衛星',
  'mining': '礦業/貴金屬',
  'high_dividend': '高股息',
  'etf_rotation': 'ETF 輪動',
  'small_cap': '小型股',
  'foundry': '晶圓代工',
  'pcb': 'PCB',
  'thermal': '散熱',
  'semi_equipment': '半導體設備',
  'materials': '材料',
  'petrochemicals': '石化',
  'consumer': '消費',
  'tourism': '觀光',
  'DXY': '美元指數',
  'gold': '黃金',
  'JPY': '日圓',
  'oil': '原油',
  'VIX': '波動率指數',
  'global_liquidity': '全球流動性',
  'global_equities': '全球股市',
  'US_tech_capex': '美國科技資本支出',
  'breakeven_inflation': '通膨預期',
  'Fed_funds_futures': '聯準會基金期貨',
  'US_rates': '美國利率',
  'TAIEX': '台股大盤',
  'GPR_index': '地緣政治風險指數',
  'EM_currencies': '新興市場貨幣',
  'foreign_flow_TW': '外資流向_台股'
};

export function sectorName(s) { return SECTOR_NAME_MAP[s] || s; }

// Time window name mapping
export const TIME_WINDOW_NAME_MAP = {
  'immediate': '即時',
  '1_week': '1 週',
  '1_month': '1 個月'
};

export function timeWindowName(tw) { return TIME_WINDOW_NAME_MAP[tw] || tw; }
