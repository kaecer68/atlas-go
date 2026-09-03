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
  'robotics-desk-01': '機器人產業桌',
  'mining-desk-01': '礦業/貴金屬產業桌',
  'energy-desk-01': '能源產業桌',
  'electronics-desk-01': '電子零組件產業桌',
  'consumer-desk-01': '消費產業桌',
  'industrial-desk-01': '工業/製造產業桌',
  'etf-rotation-01': 'ETF 輪動',
  'cro-01': '風控長',
  'cio-01': '投資長',
  'super-dru-01': 'Druckenmiller 超級投資者',
  'super-asc-01': 'Aschenbrenner 超級投資者',
  'super-bak-01': 'Baker 超級投資者',
  'super-ack-01': 'Ackman 超級投資者',
  'alpha_discovery': '阿爾法發現',
  'us-macro-spx-01': '美股 S&P 500 宏觀監控',
  'us-macro-tech-01': '美股科技/半導體宏觀監控',
  'us-crossmarket-01': '美台跨市場傳導監控',
  'odm-channel-01': 'ODM 通道監控',
  'odm-data-01': 'ODM 資料提供者'
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

/**
 * 統一股票代號表格 cell 渲染：<td>{symbol} {companyName}</td>
 * 自動處理 .TW 後綴、有/無簡稱的 fallback、XSS 防護。
 *
 * @param {string} symbol - 股票代號（可含/不含 .TW）
 * @param {object} [opts]
 * @param {string} [opts.variant='inline'] - 'inline'（同行）或 'stacked'（兩行）
 * @param {string} [opts.className=''] - 額外 class
 * @param {boolean} [opts.escape=true] - 自動 escape symbol（防 XSS）
 * @returns {string} HTML 片段
 */
export function renderStockCell(symbol, opts = {}) {
  const { variant = 'inline', className = '', escape = true } = opts;
  if (symbol == null) return '';
  const sym = String(symbol);
  const safeSym = escape ? sym.replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c])) : sym;
  const pureSym = safeSym.replace('.TW', '');
  const linkHref = `/client/quote?symbol=${pureSym}`;
  const name = stockName(sym);
  const cls = ['stock-cell', className].filter(Boolean).join(' ');
  if (!name) return `<a href="${linkHref}" class="${cls}" style="text-decoration:none;"><span class="stock-cell-symbol" style="color:var(--color-primary);">${safeSym}</span></a>`;
  if (variant === 'stacked') {
    return `<a href="${linkHref}" class="${cls} stock-cell-stacked" style="text-decoration:none;">
      <span class="stock-cell-symbol" style="color:var(--color-primary);">${safeSym}</span>
      <span class="stock-cell-name">${name}</span>
    </a>`;
  }
  return `<a href="${linkHref}" class="${cls}" style="text-decoration:none;"><span class="stock-cell-symbol" style="color:var(--color-primary);">${safeSym}</span> <span class="stock-cell-name">${name}</span></a>`;
}

// Regime name mapping
export const REGIME_NAME_MAP = {
  'NEUTRAL': '中性',
  'RISK_ON': '風險趨向',
  'RISK_OFF': '風險趨避'
};

export function regimeLabel(r) {
  if (!r) return '-';
  return REGIME_NAME_MAP[r] ? `${REGIME_NAME_MAP[r]}（${r}）` : '-';
}

// Stress name mapping
export const STRESS_NAME_MAP = {
  'low': '低壓',
  'alert': '警戒',
  'high': '高壓',
  'crisis': '危機'
};

export function stressLabel(r) { return STRESS_NAME_MAP[r] || r; }

// Confidence source name mapping
export const CONFIDENCE_SOURCE_NAME_MAP = {
  'deviation_based_v1': '偏離度模型 v1',
  'calendar_seasonal': '季節性日曆',
  'calendar_political': '政治日曆'
};

export function confidenceSourceName(cs) { return CONFIDENCE_SOURCE_NAME_MAP[cs] || cs; }

// Severity name mapping
export const SEVERITY_NAME_MAP = {
  'low': '低',
  'medium': '中',
  'high': '高'
};

export function severityName(s) { return SEVERITY_NAME_MAP[s] || s; }

// Status name mapping
export const STATUS_NAME_MAP = {
  'active': '進行中',
  'confirmed': '已確認',
  'expired': '已過期'
};

export function statusName(s) { return STATUS_NAME_MAP[s] || s; }

// Region name mapping
export const REGION_NAME_MAP = {
  'US': '美國',
  'Global': '全球',
  'Asia': '亞洲',
  'JP': '日本',
  'Middle East': '中東',
  'TW': '台灣',
  'COM': '國際商品'
};

export function regionName(r) { return REGION_NAME_MAP[r] || r; }

// Capital flow name mapping
export const CAPITAL_FLOW_NAME_MAP = {
  'flight_to_USD': '美元避險',
  'risk_off': '風險趨避流出',
  'risk_on': '風險偏好流入',
  'tech_capex_inflow': '科技資本支出流入',
  'inflation_reprice': '通膨重新定價',
  'global_liquidity_drain': '全球流動性收緊',
  'tech_capex_slowdown': '科技資本支出放緩',
  'seasonal_rotation': '季節性輪動',
  'policy_uncertainty': '政策不確定性',
  'pre_earnings_positioning': '財報前佈局',
  'institutional_rebalancing': '機構再平衡',
  'crowding_risk': '擁擠風險',
  'flight_to_gold': '黃金避險',
  'fx_driven_outflow': '匯率驅動流出',
  'earnings_beat': '財報優於預期',
  'earnings_miss': '財報不如預期'
};

export function capitalFlowName(cf) { return CAPITAL_FLOW_NAME_MAP[cf] || cf; }

// Model name mapping
export const MODEL_NAME_MAP = {
  '鷹派聯準會模型': '鷹派聯準會模型',
  'AI 超級週期模型': 'AI 超級週期模型',
  '地緣政治避險模型': '地緣政治避險模型',
  '台灣地緣風險模型': '台灣地緣風險模型',
  '半導體週期模型': '半導體週期模型',
  '季節性輪動模型': '季節性輪動模型',
  '選舉週期模型': '選舉週期模型',
  '散戶與法人背離': '散戶與法人背離',
  '財報驚喜驅動': '財報驚喜驅動'
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
  // risk-console UX Phase 1 B5：sector other 未翻譯直接上畫面
  'other': '其他',
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
  'defensive': '防禦性板塊',
  'technology': '科技板塊',
  'traditional': '傳產板塊',
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
  'foreign_flow_TW': '外資流向_台股',
  '新興市場貨幣': '新興市場貨幣',
  '外資流向_台股': '外資流向(台股)',
  '台股大盤': '台股大盤',
  '全球流動性': '全球流動性',
  '全球股市': '全球股市',
  '美國科技資本支出': '美國科技資本支出',
  '半導體設備': '半導體設備',
  '材料': '材料',
  '地緣政治風險指數': '地緣政治風險指數',
  '原油': '原油',
  '通膨預期': '通膨預期',
  '實質利率': '實質利率',
  '內需': '內需',
  '傳產': '傳產',
  '綠能': '綠能',
  '基建': '基建',
  '國防': '國防',
  '台海風險': '台海風險',
  '軍事不確定性': '軍事不確定性',
  '出口導向': '出口導向',
  '進口成本': '進口成本',
  '電子零組件': '電子零組件',
  '散戶情緒': '散戶情緒',
  '融資餘額': '融資餘額',
  '外公流向': '外資流向',
  '投信流向': '投信流向',
  '主題股': '主題股',
  '貴金屬': '貴金屬',
  '貴金屬ETF': '貴金屬ETF',
  '原物料': '原物料',
  '出口股': '出口股',
  '進口商': '進口商',
  '集團股': '集團股',
  '零售': '零售',
  '證券': '證券',
  '貨櫃航運': '貨櫃航運',
  '散裝航運': '散裝航運',
  '工業金屬': '工業金屬'
};

export function sectorName(s) { return SECTOR_NAME_MAP[s] || s; }

// Time window name mapping
export const TIME_WINDOW_NAME_MAP = {
  'immediate': '即時',
  '1_week': '1 週',
  '1_month': '1 個月',
  '2_months': '2 個月'
};

export function timeWindowName(tw) { return TIME_WINDOW_NAME_MAP[tw] || tw; }
