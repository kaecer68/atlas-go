// Methodology page — 對應 docs/ATLAS_METHODOLOGY.md 第二章（因果傳導鏈）與
// 第五章（策略矩陣）。所有 8 層結構、文字、策略優先級皆來自憲章；數值
// 區接真 API（/api/reports/latest、/api/cross-market/status、/api/regime/history），
// 沒資料源的指標顯示「資料源未接入」佔位，**禁止編造數值**。
//
// API 失敗處理：每個區塊（period card / regime band / chain / recommend）
// 獨立擁有自己的 loading / error / empty 三態，並各自掛重試按鈕。

import { silentGetJSON, escapeHtml } from '../shared/app-utils.js';
import { getTier } from '../services/auth.js';
import { fmtSafeNumber, fmtSafeSignedPct } from '../shared/format-metric.js';

// ---------------------------------------------------------------------------
// 策略分類輔助（後端 strategies_detail 為唯一真理源頭）
// ---------------------------------------------------------------------------
const CATEGORY_LABEL = {
  defensive: '防禦',
  aggressive: '攻擊',
  tactical: '戰術',
};
function categoryLabel(cat) {
  return CATEGORY_LABEL[cat] || escapeHtml(String(cat || '—'));
}
function categoryClass(cat) {
  if (cat === 'defensive' || cat === 'aggressive' || cat === 'tactical') return cat;
  return 'unknown';
}
function renderStrategyChips(strategiesDetail, allowedStrategies) {
  if (Array.isArray(strategiesDetail) && strategiesDetail.length > 0) {
    return strategiesDetail.map((s, i) => {
      const priority = s.priority === 'primary' ? 'primary' : 'secondary';
      const name = escapeHtml(s.name || s.id || '—');
      const badge = `<span class="md-strategy-chip__badge md-strategy-chip__badge--${categoryClass(s.category)}">${categoryLabel(s.category)}</span>`;
      return `<span class="md-strategy-chip md-strategy-chip--${priority}" data-rank="${i + 1}">${name}${badge}</span>`;
    }).join('');
  }
  // Fallback: raw ID chips without category badges when strategies_detail is missing.
  console.warn('[methodology] strategies_detail missing or empty; falling back to allowed_strategies IDs');
  const ids = Array.isArray(allowedStrategies) ? allowedStrategies : [];
  if (ids.length === 0) {
    return '<span class="badge muted">當前時期無特殊策略限制</span>';
  }
  return ids.map(id => `<span class="md-strategy-chip md-strategy-chip--secondary">${escapeHtml(String(id))}</span>`).join('');
}
const CASH_RANGES = {
  downturn: '40-50%', turnaround_up: '10-20%', bull: '5-10%',
  plateau: '20-30%', consolidation: '30-40%', turnaround_down: '50-70%', black_swan: '80-100%',
};
const PERIOD_LABEL = {
  downturn:        { zh: '低迷',       en: 'Downturn' },
  turnaround_up:   { zh: '轉折開高',   en: 'Turnaround Up' },
  bull:            { zh: '上升',       en: 'Bull' },
  plateau:         { zh: '高原',       en: 'Plateau' },
  consolidation:   { zh: '盤整',       en: 'Consolidation' },
  turnaround_down: { zh: '轉折下壓',   en: 'Turnaround Down' },
  black_swan:      { zh: '黑天鵝',     en: 'Black Swan' },
};
const STATUS_TO_BADGE = {
  risk_on: { cls: 'ok',   label: '利多' },
  risk_off:{ cls: 'err',  label: '利空' },
  neutral: { cls: 'warn', label: '中性' },
};

// ---------------------------------------------------------------------------
// 8 層鏈圖定義 — 結構與憲章第二章完全一致，不得合併改名
// ---------------------------------------------------------------------------
const LAYERS = [
  {
    num: '第零層', title: '全球資金總開關',
    charter: '二・第〇層',
    oneliner: '全球資金總開關決定新興市場方向（美債殖利率、DXY、日圓、VIX）',
    metrics: [
      { key: 'us10y',  label: '美債殖利率', en: 'US 10Y',     source: 'report.global.bond_yield' },
      { key: 'dxy',    label: '美元指數',   en: 'DXY',         source: 'report.global.usd_index' },
      { key: 'jpy',    label: '日圓',       en: 'JPY',         source: 'report.global.jpy' },
      { key: 'vix',    label: '恐慌指數',   en: 'VIX',         source: 'report.global.vix' },
    ],
  },
  {
    num: '第一層', title: '美股科技估值',
    charter: '二・第一層',
    oneliner: '美股科技估值決定台股基本面動能（費半、TSM ADR、NVDA、NASDAQ-100）',
    metrics: [
      { key: 'sox',     label: '費半',     en: 'SOX',       source: 'cross.sox',     numeric: true },
      { key: 'tsm_adr', label: '台積電 ADR', en: 'TSM',     source: 'cross.tsm_adr', numeric: true },
      { key: 'nvda',    label: '輝達',     en: 'NVDA',      source: 'cross.nvda',    numeric: true },
      { key: 'ndx',     label: '那斯達克 100', en: 'NDX',   source: 'cross.ndx',     numeric: true },
    ],
  },
  {
    num: '第二層', title: '台灣出口與半導體景氣',
    charter: '二・第二層',
    oneliner: '基本面方向訊號（出口訂單、設備進口、台積電月營收）',
    metrics: [
      { key: 'exports',   label: '出口訂單',       en: 'MOEA Exports',     available: false },
      { key: 'semi_imp',  label: '半導體設備進口', en: 'Semi Equip Import',available: false },
      { key: 'tsm_rev',   label: '台積電月營收',   en: 'TSM Revenue',      available: false },
    ],
  },
  {
    num: '第三層', title: '外資資金流與匯率',
    charter: '二・第三層',
    oneliner: '外資是方向制定者，行動最領先（外資現貨、外資期貨、新台幣匯率）',
    metrics: [
      { key: 'foreign',   label: '外資現貨',       en: 'Foreign Spot',   source: 'capital.forces.foreign', kind: 'force' },
      { key: 'futures',   label: '外資期貨未平倉', en: 'Foreign Futures',source: 'capital.forces.futures', kind: 'force' },
      { key: 'usd_twd',   label: '新台幣匯率',     en: 'USD/TWD',        source: 'cross.usd_twd', numeric: true },
    ],
    resonance: true,
  },
  {
    num: '第四層', title: '台股大盤量能',
    charter: '二・第四層',
    oneliner: '價格方向 + 市場參與熱度 + 散戶槓桿水位',
    metrics: [
      { key: 'taiex',       label: '加權指數',   en: 'TAIEX',        available: false },
      { key: 'volume',      label: '集中市場成交量', en: 'Volume',   available: false },
      { key: 'margin',      label: '融資餘額',   en: 'Margin Balance',available: false },
      { key: 'daytrade',    label: '當沖交易佔比', en: 'Day-Trade',  available: false },
    ],
  },
  {
    num: '第五層', title: '內資勢力反應',
    charter: '二・第五層',
    oneliner: '內資共識可對抗外資賣壓（投信、公股、自營商、壽險、公司派）',
    metrics: [
      { key: 'inst',    label: '投信',           en: 'Investment Trust', source: 'capital.forces.institutional', kind: 'force' },
      { key: 'gov',     label: '公股券商',       en: 'Government',       source: 'capital.forces.government',    kind: 'force' },
      { key: 'dealer',  label: '自營商',         en: 'Dealer',           source: 'capital.forces.dealer',        kind: 'force' },
      { key: 'insur',   label: '壽險／銀行',     en: 'Insurance/Bank',   available: false },
      { key: 'insider', label: '公司派／內部人', en: 'Insider',          available: false },
    ],
    resonance: true,
  },
  {
    num: '第六層', title: '散戶情緒與籌碼',
    charter: '二・第六層',
    oneliner: '散戶情緒是反向指標（融資高點≈頭部、融資斷頭潮≈底部）',
    metrics: [
      { key: 'retail',   label: '散戶動向 Z-score', en: 'Retail',        source: 'capital.forces.retail', kind: 'force' },
      { key: 'margin_m', label: '融資維持率',       en: 'Margin Ratio',  available: false },
      { key: 'gtrends',  label: 'Google Trends',    en: 'Google Trends', available: false },
    ],
    resonance: true,
  },
  {
    num: '第七層', title: '可捕捉的資金事件與錯價',
    charter: '二・第七層',
    oneliner: '可預測事件（ETF 調整、MSCI、營收、除權息）創造可捕捉的短期錯價',
    metrics: [
      { key: 'tomorrow', label: '近期事件（明日）', en: 'Tomorrow',  source: 'report.events.tomorrow' },
      { key: 'thisweek', label: '近期事件（本週）', en: 'This Week', source: 'report.events.this_week' },
    ],
  },
];

// 圖層白話卡片的「對決策含義」— 引用憲章原文，**禁止自由發揮**
const LAYER_IMPLICATION = {
  '第零層': '美台資金開關是所有後續判斷的起點；當 US10Y 急升或 DXY 走強，資金撤出新興市場，台股難逃。',
  '第一層': 'SOX 在 50 日線下不做多；NVDA、TSM ADR 突破是科技股動能延續至台股的必要條件。',
  '第二層': '當出口訂單連續三個月年增率轉負，第二層將對第三層（外資）形成基本面逆風。',
  '第三層': '外資連續買超 + 期貨多單增加 + 新台幣升值 = 三重確認做多信號；任一反轉即應減碼。',
  '第四層': '融資餘額暴增 + 當沖佔比 > 35% = 過熱警訊；融資大減 + 量縮 = 底部訊號。',
  '第五層': '內資共識（投信+公股同步買超）可對抗外資賣壓；當內資共識瓦解，底部信號失效。',
  '第六層': '散戶反向指標：融資高點≈頭部、融資斷頭潮≈底部。Google Trends 同步轉熱 = 散戶進場 = 警訊。',
  '第七層': '事件套利持倉 < 5 日、單一事件曝險 < 10%；事件過後迅速獲利了結。',
};

// ---------------------------------------------------------------------------
// template（DOM 骨架）— 純靜態，所有資料由 init() 注入。
// ---------------------------------------------------------------------------
export const template = `
  <div class="md-page">
    <div class="md-page-header">
      <h2>ATLAS 方法論：因果傳導鏈</h2>
      <p>八層由上而下、由外而內的資金傳導鏈，對齊 <code>docs/ATLAS_METHODOLOGY.md</code> 憲章第二章與第五章策略矩陣。</p>
    </div>

    <details class="md-helper">
      <summary>💡 如何閱讀本頁</summary>
      <p>由上至下讀 8 層因果鏈（全球 → 美股 → 台灣出口 → 外資 → 大盤 → 內資 → 散戶 → 事件），每一層點擊可展開「憲章引用 + 指標清單 + 對決策的含義」。最下方的「策略推薦」依當前時期套用憲章第五章策略矩陣。</p>
      <p>非 premium 用戶仍可看到完整結構與憲章內容，僅即時數值以「升級查看即時數值」標示。</p>
    </details>

    <!-- 1. 當前時期卡片 -->
    <section id="md-period-host" class="md-period-card" data-period="" aria-label="當前市場時期">
      <div class="md-state md-state--loading">載入當前時期…</div>
    </section>

    <!-- 2. 三態歷史色帶 -->
    <section id="md-regime-host" class="md-history-card" aria-label="三態歷史色帶">
      <h3>📊 風險狀態（三態）歷史</h3>
      <div class="md-state md-state--loading">載入歷史紀錄…</div>
    </section>

    <!-- 3. 因果傳導鏈 -->
    <section class="md-chain-section">
      <h3>🔗 因果傳導鏈（八層）</h3>
      <p class="md-chain-section__intro">由上而下、由外而內，<strong>不可反向推導</strong>。點任一層卡片查看憲章引用與指標清單。</p>
      <div id="md-chain-host" class="md-chain">
        <div class="md-state md-state--loading">載入鏈圖…</div>
      </div>
    </section>

    <!-- 4. 策略推薦輸出節點 -->
    <section id="md-recommend-host" class="md-recommend" aria-label="散戶策略推薦">
      <div class="md-state md-state--loading">載入策略推薦…</div>
    </section>
  </div>
`;

// ---------------------------------------------------------------------------
// 工具函式
// ---------------------------------------------------------------------------
function isNum(v) { return typeof v === 'number' && Number.isFinite(v); }

// 安全取得欄位：null/undefined/空字串/NaN 視為缺失
function safeGet(obj, path) {
  if (!obj) return null;
  const parts = path.split('.');
  let cur = obj;
  for (const p of parts) {
    if (cur == null) return null;
    cur = cur[p];
  }
  if (cur == null || cur === '') return null;
  if (typeof cur === 'number' && Number.isNaN(cur)) return null;
  return cur;
}

function tierGatePill() {
  return '<span class="md-tier-gate" title="升級 Premium 以查看即時數值">升級查看即時數值</span>';
}

function renderError(label, retryId) {
  return `<div class="md-state md-state--error">
    <div>⚠️ ${escapeHtml(label)} 載入失敗</div>
    <button class="md-state__retry" type="button" data-retry="${escapeHtml(retryId || '')}">重試</button>
  </div>`;
}

function renderEmpty(msg) {
  return `<div class="md-state md-state--empty">${escapeHtml(msg || '無資料')}</div>`;
}

// SVG 下向箭頭（鏈圖層與層之間）
function arrowSvg() {
  return '<svg width="20" height="20" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">'
    + '<path d="M10 18 L2 7 L18 7 Z"/></svg>';
}

// 解析 cross-market/status 欄位：欄位可能為 {value, change_pct, symbol, timestamp} 或純值
function getCrossField(cross, key) {
  if (!cross) return { value: null, changePct: null, available: false };
  const raw = cross[key];
  if (raw && typeof raw === 'object') {
    // 僅以 value 是否有效作為可用性判斷；symbol 只作 debug 提示，不阻塞顯示。
    const value = raw.value;
    const available = value != null && !Number.isNaN(Number(value));
    return { value, changePct: raw.change_pct, available };
  }
  if (isNum(raw) || typeof raw === 'string') {
    return { value: raw, changePct: null, available: true };
  }
  return { value: null, changePct: null, available: false };
}

// 從 capital-flow summary/daily 的 forces 陣列找單一 force
function getForce(capital, name) {
  if (!capital) return null;
  const arr = Array.isArray(capital.forces) ? capital.forces : [];
  return arr.find(f => f && f.force === name) || null;
}

// 渲染單一 metric cell
function renderMetricCell(metric, report, cross, capital, isPremium) {
  if (metric.available === false) {
    return `<div class="md-metric" data-key="${escapeHtml(metric.key)}">
      <div class="md-metric__label">${escapeHtml(metric.label)} <code>${escapeHtml(metric.en)}</code></div>
      <div class="md-metric__value md-metric__value--muted">資料源未接入</div>
    </div>`;
  }

  // 解析來源
  let value = null;
  let changePct = null;
  let isGated = false;
  let extraBadge = '';
  const src = metric.source || '';
  if (metric.kind === 'force') {
    // 來自 /api/capital-flow/summary 或 /api/capital-flow/daily
    const forceName = src.slice('capital.forces.'.length);
    const f = getForce(capital, forceName);
    if (!isPremium) {
      isGated = true;
    } else if (!f || f.data_available === false) {
      return `<div class="md-metric" data-key="${escapeHtml(metric.key)}">
        <div class="md-metric__label">${escapeHtml(metric.label)} <code>${escapeHtml(metric.en)}</code></div>
        <div class="md-metric__value md-metric__value--muted">資料源未接入</div>
      </div>`;
    } else {
      value = isNum(f.z_score) ? Number(f.z_score) : null;
      const trend = f.trend || (value > 0.5 ? 'bullish' : value < -0.5 ? 'bearish' : 'neutral');
      extraBadge = `<span class="badge ${trend === 'bullish' ? 'up' : trend === 'bearish' ? 'down' : 'muted'}">${escapeHtml(trend === 'bullish' ? '偏多' : trend === 'bearish' ? '偏空' : '中性')}</span>`;
    }
  } else if (src.startsWith('report.')) {
    // 來自 /api/reports/latest — tier-gated
    if (!isPremium) {
      isGated = true;
    } else {
      value = safeGet(report, src.slice('report.'.length));
    }
  } else if (src.startsWith('cross.')) {
    // 來自 /api/cross-market/status
    const field = getCrossField(cross, src.slice('cross.'.length));
    value = field.value;
    changePct = field.changePct;
    if (!field.available) {
      return `<div class="md-metric" data-key="${escapeHtml(metric.key)}">
        <div class="md-metric__label">${escapeHtml(metric.label)} <code>${escapeHtml(metric.en)}</code></div>
        <div class="md-metric__value md-metric__value--muted">資料獲取失敗</div>
      </div>`;
    }
  }

  if (isGated) {
    return `<div class="md-metric" data-key="${escapeHtml(metric.key)}">
      <div class="md-metric__label">${escapeHtml(metric.label)} <code>${escapeHtml(metric.en)}</code></div>
      <div class="md-metric__value md-metric__value--muted">${tierGatePill()}</div>
    </div>`;
  }

  // 數值 / 字串呈現
  const isNumeric = metric.numeric || metric.kind === 'force' || isNum(changePct);
  let display = '—';
  if (isNumeric && isNum(value)) {
    display = fmtSafeNumber(value, { decimals: 2, useGrouping: true });
    if (isNum(changePct)) {
      const cp = Number(changePct);
      const cls = cp > 0 ? 'md-metric__value--bullish' : cp < 0 ? 'md-metric__value--bearish' : '';
      display += ` <span class="md-metric__change ${cls}">${fmtSafeSignedPct(cp)}</span>`;
    }
  } else if (typeof value === 'string') {
    display = escapeHtml(value);
  } else if (Array.isArray(value)) {
    // events.tomorrow / events.this_week：摺疊顯示前 2 條
    const shown = value.slice(0, 2).map(escapeHtml).join('、');
    const more = value.length > 2 ? ' …' : '';
    display = shown + more;
  }
  return `<div class="md-metric" data-key="${escapeHtml(metric.key)}">
    <div class="md-metric__label">${escapeHtml(metric.label)} <code>${escapeHtml(metric.en)}</code>${extraBadge}</div>
    <div class="md-metric__value">${display}</div>
  </div>`;
}

// ---------------------------------------------------------------------------
// 渲染：1. 當前時期卡片
// ---------------------------------------------------------------------------
function renderPeriodCard(host, report, isPremium, error) {
  if (error) {
    host.innerHTML = renderError('當前時期資料', 'period');
    return rebindRetry('md-period-host', 'period', isPremium);
  }
  if (!isPremium) {
    // 非 premium 仍顯示結構，但數值區以「升級查看即時數值」呈現
    host.dataset.period = '';
    host.innerHTML = `
      <div class="md-period-card__header">
        <span class="md-period-name-zh">${tierGatePill()}</span>
        <span class="md-period-market-period">當前市場時期需 Premium 才能查看即時判定</span>
      </div>
      <div class="md-period-card__body">
        <div class="md-period-summary">${escapeHtml(LAYER_IMPLICATION['第零層'])}</div>
        <div class="md-period-cash">
          <span class="md-period-cash__label">現金部位建議</span>
          <span class="md-period-cash__value">—</span>
          <div class="md-period-cash__bar"><div class="md-period-cash__fill" style="width:0%"></div></div>
          ${tierGatePill()}
        </div>
      </div>
      <div class="md-allowed-strategies"><span class="md-tier-gate">升級查看可用策略</span></div>
    `;
    return;
  }
  if (!report || !report.period) {
    host.dataset.period = '';
    host.innerHTML = renderEmpty('當前時期資料尚未生成，請稍候或重新整理。');
    return;
  }
  const p = report.period;
  const periodId = p.market_period || '';
  const label = PERIOD_LABEL[periodId] || { zh: periodId || '—', en: '' };
  const status = STATUS_TO_BADGE[report.global && report.global.status] || null;
  const cash = isNum(p.cash_reserve) ? Number(p.cash_reserve) : null;
  const cashPct = cash == null ? null : Math.max(0, Math.min(100, Math.round(cash)));
  const summary = (report.global && report.global.summary) || (report.summary) || '—';

  host.dataset.period = periodId;
  host.innerHTML = `
    <div class="md-period-card__header">
      <span class="md-period-name-zh">${escapeHtml(p.period_name_zh || label.zh)}</span>
      <span class="md-period-market-period">${escapeHtml(periodId || label.en || '')}</span>
      ${status ? `<span class="badge ${status.cls}">${escapeHtml(status.label)}</span>` : ''}
    </div>
    <div class="md-period-card__body">
      <div class="md-period-summary">${escapeHtml(summary)}</div>
      <div class="md-period-cash">
        <span class="md-period-cash__label">現金部位（cash_reserve）</span>
        <span class="md-period-cash__value">${cash == null ? '—' : (cashPct + '%')}</span>
        <div class="md-period-cash__bar"><div class="md-period-cash__fill" style="width:${cash == null ? 0 : cashPct}%"></div></div>
        <div class="md-period-cash__hint">${periodId && CASH_RANGES[periodId] ? '憲章建議區間：' + escapeHtml(CASH_RANGES[periodId]) : ''}</div>
      </div>
    </div>
    <div class="md-allowed-strategies">
      ${renderStrategyChips(p.strategies_detail, p.allowed_strategies)}
    </div>
  `;
}

// ---------------------------------------------------------------------------
// 渲染：2. 三態歷史色帶
// ---------------------------------------------------------------------------
function renderRegimeHistory(host, data, error) {
  if (error) {
    host.style.display = '';
    host.innerHTML = renderError('三態歷史紀錄', 'regime');
    return rebindRetry('md-regime-host', 'regime');
  }
  // 無資料時整區塊隱藏（提示詞 empty 態），禁止畫假色帶；error 態仍顯示重試。
  const sessions = data && Array.isArray(data) ? data
                 : (data && Array.isArray(data.sessions) ? data.sessions
                 : (data && Array.isArray(data.Sessions) ? data.Sessions : null));
  if (!sessions || sessions.length === 0) {
    host.style.display = 'none';
    host.innerHTML = '';
    return;
  }
  host.style.display = '';
  const cells = sessions.map(s => {
    const regime = s.regime || s.Regime || '';
    const date = s.date || s.Date || s.recorded_at || '';
    return `<div class="md-regime-cell" data-regime="${escapeHtml(regime)}" title="${escapeHtml(date)} · ${escapeHtml(regime)}"></div>`;
  }).join('');
  host.innerHTML = `
    <h3>📊 風險狀態（三態）歷史</h3>
    <div class="md-regime-band" aria-label="regime 歷史色帶">${cells}</div>
    <p class="md-history-card__hint">七時期歷史軸待後端 <code>period_history</code> 提供（API 目前僅回傳三態）。</p>
    <div class="md-regime-legend">
      <span class="md-regime-legend__item"><span class="md-regime-legend__swatch md-regime-legend__swatch--risk-on"></span>RISK_ON</span>
      <span class="md-regime-legend__item"><span class="md-regime-legend__swatch md-regime-legend__swatch--risk-off"></span>RISK_OFF</span>
      <span class="md-regime-legend__item"><span class="md-regime-legend__swatch md-regime-legend__swatch--neutral"></span>NEUTRAL</span>
      <span class="md-regime-legend__item"><span class="md-regime-legend__swatch md-regime-legend__swatch--transitional"></span>TRANSITIONAL</span>
    </div>
  `;
}

// ---------------------------------------------------------------------------
// 渲染：3. 鏈圖（8 層）
// ---------------------------------------------------------------------------
function renderChain(host, report, cross, capital, isPremium) {
  const layerHtml = LAYERS.map((layer, idx) => {
    const isLast = idx === LAYERS.length - 1;
    const cards = layer.metrics.map(m => renderMetricCell(m, report, cross, capital, isPremium)).join('');
    const resonance = layer.resonance ? renderResonance(report, isPremium) : '';
    return `
      <div class="md-layer-card" data-layer-num="${escapeHtml(layer.num)}" tabindex="0" role="button" aria-label="展開 ${escapeHtml(layer.title)} 詳細">
        <div class="md-layer-card__head">
          <span class="md-layer-card__num">${escapeHtml(layer.num)}</span>
          <span class="md-layer-card__title">${escapeHtml(layer.title)}</span>
          <span class="md-layer-card__chevron">›</span>
        </div>
        <div class="md-layer-card__oneliner">${escapeHtml(layer.oneliner)}</div>
        <div class="md-layer-card__metrics">${cards}</div>
        ${resonance}
      </div>
      ${isLast ? '' : `<div class="md-arrow">${arrowSvg()}</div>`}
    `;
  }).join('');
  host.innerHTML = layerHtml;
  // 綁定 click / Enter 開 Modal
  host.querySelectorAll('.md-layer-card').forEach(card => {
    const open = () => openLayerModal(card);
    card.addEventListener('click', open);
    card.addEventListener('keydown', (ev) => {
      if (ev.key === 'Enter' || ev.key === ' ') { ev.preventDefault(); open(); }
    });
  });
}

function renderResonance(report, isPremium) {
  if (!isPremium) {
    return `<div class="md-layer-card__resonance">
      <span>資金共振</span>
      <div class="md-progress"><div class="md-progress__fill" style="width:0%"></div></div>
      <span>${tierGatePill()}</span>
    </div>`;
  }
  const r = report && report.capital ? report.capital : null;
  const resonance = r && isNum(r.resonance) ? Math.max(0, Math.min(1, Number(r.resonance))) : null;
  const quality = r ? r.quality : null;
  const qualityBadge = quality
    ? `<span class="badge ${quality === 'high' || quality === 'good' ? 'ok' : quality === 'low' || quality === 'bad' ? 'err' : 'warn'}">${escapeHtml(quality)}</span>`
    : '<span class="badge muted">—</span>';
  return `<div class="md-layer-card__resonance">
    <span>資金共振</span>
    <div class="md-progress" title="資金共振係數（0–1）"><div class="md-progress__fill" style="width:${resonance == null ? 0 : Math.round(resonance * 100)}%"></div></div>
    <span class="md-resonance-value">${resonance == null ? '—' : resonance.toFixed(2)}</span>
    <span>品質</span>
    ${qualityBadge}
  </div>`;
}

// ---------------------------------------------------------------------------
// 渲染：4. 策略推薦
// ---------------------------------------------------------------------------
function renderRecommend(host, report, isPremium, error) {
  if (error) {
    host.innerHTML = renderError('策略推薦', 'recommend');
    return rebindRetry('md-recommend-host', 'recommend', isPremium);
  }
  if (!isPremium) {
    host.innerHTML = `
      <div class="md-recommend__header">
        <h3 class="md-recommend__title">📌 散戶策略推薦</h3>
      </div>
      <div class="md-period-summary">${tierGatePill()} 升級 Premium 後，策略推薦將依當前市場時期自動套用憲章第五章策略矩陣。</div>
    `;
    return;
  }
  if (!report || !report.period || !report.period.market_period) {
    host.innerHTML = renderEmpty('策略推薦需先有當前市場時期；period 資料未就緒。');
    return;
  }
  const p = report.period;
  const periodId = p.market_period;
  const cashRange = CASH_RANGES[periodId] || '—';
  const activeStrategy = (report.strategy && report.strategy.active_strategy) || '';
  const direction = (report.strategy && report.strategy.direction) || '';
  const entryCond = (report.strategy && report.strategy.entry_condition) || '';

  const chipsHtml = renderStrategyChips(p.strategies_detail, p.allowed_strategies);

  host.innerHTML = `
    <div class="md-recommend__header">
      <h3 class="md-recommend__title">📌 散戶策略推薦</h3>
      <span class="md-recommend__active">當前：${escapeHtml(p.period_name_zh || periodId)}${activeStrategy ? ' · ' + escapeHtml(String(activeStrategy)) : ''}</span>
    </div>
    <div class="md-recommend__strategies">
      <div class="md-recommend__chips">${chipsHtml}</div>
      <div class="md-period-cash" style="margin-top:var(--space-sm)">
        <span class="md-period-cash__label">憲章建議現金部位</span>
        <span class="md-period-cash__value">${escapeHtml(cashRange)}</span>
      </div>
      ${entryCond ? `<div class="md-period-summary" style="margin-top:var(--space-sm)"><strong>進場條件：</strong>${escapeHtml(entryCond)}${direction ? '　·　方向：' + escapeHtml(direction) : ''}</div>` : ''}
    </div>
  `;
}

// ---------------------------------------------------------------------------
// Modal（本頁自管）
// ---------------------------------------------------------------------------
let _activeOverlay = null;
let _activeKeyHandler = null;

function closeModal() {
  if (_activeOverlay && _activeOverlay.parentNode) {
    _activeOverlay.parentNode.removeChild(_activeOverlay);
  }
  _activeOverlay = null;
  if (_activeKeyHandler) {
    document.removeEventListener('keydown', _activeKeyHandler);
    _activeKeyHandler = null;
  }
}

function openLayerModal(card) {
  const num = card.getAttribute('data-layer-num');
  const layer = LAYERS.find(l => l.num === num);
  if (!layer) return;
  closeModal();
  const overlay = document.createElement('div');
  overlay.className = 'md-modal-overlay show';
  overlay.setAttribute('role', 'dialog');
  overlay.setAttribute('aria-modal', 'true');
  overlay.innerHTML = `
    <div class="md-modal" role="document">
      <div class="md-modal__charter-ref">憲章 ${escapeHtml(layer.charter)}</div>
      <h3 class="md-modal__title">${escapeHtml(layer.num)} · ${escapeHtml(layer.title)}</h3>

      <div class="md-modal__section">
        <h4>白話說明（憲章原文）</h4>
        <p>${escapeHtml(layer.oneliner)}</p>
      </div>

      <div class="md-modal__section">
        <h4>對決策的含義（憲章原文）</h4>
        <p>${escapeHtml(LAYER_IMPLICATION[layer.num] || '—')}</p>
      </div>

      <div class="md-modal__section">
        <h4>指標清單與即時值</h4>
        <div class="md-modal__metrics">
          ${layer.metrics.map(m => {
            const cell = card.querySelector('.md-metric[data-key="' + m.key + '"]');
            const valueHtml = cell ? cell.querySelector('.md-metric__value').outerHTML : '<div class="md-metric__value md-metric__value--muted">—</div>';
            return `<div class="md-metric">
              <div class="md-metric__label">${escapeHtml(m.label)} <code>${escapeHtml(m.en)}</code></div>
              ${valueHtml}
            </div>`;
          }).join('')}
        </div>
      </div>

      <div class="md-modal__close-row">
        <button class="md-state__retry" type="button" data-close>關閉</button>
      </div>
    </div>
  `;
  document.body.appendChild(overlay);
  _activeOverlay = overlay;
  // 點 overlay 背景（非 modal 本體）關閉
  overlay.addEventListener('click', (ev) => {
    if (ev.target === overlay) closeModal();
  });
  overlay.querySelector('[data-close]').addEventListener('click', closeModal);
  _activeKeyHandler = (ev) => { if (ev.key === 'Escape') closeModal(); };
  document.addEventListener('keydown', _activeKeyHandler);
}

// ---------------------------------------------------------------------------
// 重試綁定（per-block）
// ---------------------------------------------------------------------------
function rebindRetry(hostId, section, isPremium) {
  const fresh = document.getElementById(hostId);
  if (!fresh) return;
  const btn = fresh.querySelector('[data-retry]');
  if (!btn) return;
  btn.addEventListener('click', () => reloadSection(section, isPremium));
}

async function reloadSection(section, isPremium) {
  if (section === 'period' || section === 'recommend') {
    if (!isPremium) {
      // 非 premium 重新渲染結構（佔位）
      if (section === 'period') {
        const host = document.getElementById('md-period-host');
        if (host) renderPeriodCard(host, null, false, false);
      } else {
        const host = document.getElementById('md-recommend-host');
        if (host) renderRecommend(host, null, false, false);
      }
      return;
    }
    const [r, capital] = await Promise.all([
      silentGetJSON('/api/reports/latest'),
      silentGetJSON('/api/capital-flow/summary'),
    ]);
    let capitalData = capital;
    if (capitalData && !Array.isArray(capitalData.forces)) {
      const daily = await silentGetJSON('/api/capital-flow/daily');
      if (daily && Array.isArray(daily.forces)) capitalData = daily;
    }
    // 連動更新 chain，因為 chain 的 force metric 來自 capital-flow
    const chainHost = document.getElementById('md-chain-host');
    const cross = await silentGetJSON('/api/cross-market/status');
    if (chainHost) renderChain(chainHost, r, cross, capitalData, true);
    if (section === 'period') {
      const host = document.getElementById('md-period-host');
      if (host) renderPeriodCard(host, r, true, !r);
    } else {
      const host = document.getElementById('md-recommend-host');
      if (host) renderRecommend(host, r, true, !r);
    }
  } else if (section === 'regime') {
    const r = await silentGetJSON('/api/regime/history');
    // 兼容 backend 同時提供 /api/dashboard/regime-history
    const sessions = r && (Array.isArray(r) ? r : (r.sessions || r.Sessions || null));
    if (!sessions) {
      const fallback = await silentGetJSON('/api/dashboard/regime-history');
      if (fallback) {
        const merged = Array.isArray(fallback) ? fallback : (fallback.sessions || fallback.Sessions || fallback);
        if (merged) {
          const host = document.getElementById('md-regime-host');
          if (host) renderRegimeHistory(host, merged, false);
          return;
        }
      }
    }
    const host = document.getElementById('md-regime-host');
    if (host) renderRegimeHistory(host, r, !r);
  }
}

// ---------------------------------------------------------------------------
// 資料載入與渲染（供 init 與 main.js 的 auto-refresh / 頁面切換重用）
// ---------------------------------------------------------------------------
export async function loadMethodologyData() {
  const tier = await getTier();
  const isPremium = tier === 'premium';

  // 四個獨立 API；任一失敗不拖垮其他（silentGetJSON 吞錯誤回傳 null）
  const [report, regime, cross, capital] = await Promise.all([
    isPremium ? silentGetJSON('/api/reports/latest') : Promise.resolve(null),
    silentGetJSON('/api/regime/history'),
    silentGetJSON('/api/cross-market/status'),
    isPremium ? silentGetJSON('/api/capital-flow/summary') : Promise.resolve(null),
  ]);

  // capital-flow fallback：summary 若無 forces，嘗試 daily
  let capitalData = capital;
  if (isPremium && capitalData && !Array.isArray(capitalData.forces)) {
    const daily = await silentGetJSON('/api/capital-flow/daily');
    if (daily && Array.isArray(daily.forces)) capitalData = daily;
  }

  // Regime 兼容路徑：若 /api/regime/history 沒資料，fallback 到 /api/dashboard/regime-history
  let regimeData = regime;
  const hasRegimeSessions = regimeData && (
    Array.isArray(regimeData) ||
    Array.isArray(regimeData.sessions) ||
    Array.isArray(regimeData.Sessions)
  );
  if (!hasRegimeSessions) {
    const fallback = await silentGetJSON('/api/dashboard/regime-history');
    if (fallback) {
      const merged = Array.isArray(fallback) ? fallback : (fallback.sessions || fallback.Sessions || fallback);
      if (merged && (Array.isArray(merged) || Array.isArray(merged.sessions) || Array.isArray(merged.Sessions))) {
        regimeData = merged;
      }
    }
  }

  // 1. 時期卡片
  const periodHost = document.getElementById('md-period-host');
  if (periodHost) renderPeriodCard(periodHost, report, isPremium, isPremium && !report);

  // 2. Regime 歷史色帶
  const regimeHost = document.getElementById('md-regime-host');
  if (regimeHost) renderRegimeHistory(regimeHost, regimeData, !regimeData);

  // 3. 鏈圖（結構全 tier 可見；數值區獨立 error/empty/tier-gate）
  const chainHost = document.getElementById('md-chain-host');
  if (chainHost) renderChain(chainHost, report, cross, capitalData, isPremium);

  // 4. 策略推薦
  const recHost = document.getElementById('md-recommend-host');
  if (recHost) renderRecommend(recHost, report, isPremium, isPremium && !report);
}

// ---------------------------------------------------------------------------
// init（esbuild SHELL_LOADERS 入口）
// ---------------------------------------------------------------------------
export async function init() {
  await loadMethodologyData();
}
