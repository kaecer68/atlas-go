/**
 * Home page for retail investors.
 * Renders an editorial dashboard: market summary, recommendation,
 * portfolio snapshot, and trust elements.
 */

import { getJSON, silentGetJSON, escapeHtml } from '../shared/app-utils.js';
import { metricCard } from '../components/metric-card.js';
import { trustFooter } from '../components/trust-footer.js';
import { renderRiskBadge } from '../components/risk-badge.js';
import { renderTooltip } from '../components/tooltip.js';
import { renderEventCalendar } from '../components/event-calendar.js';
import { fmtSignedPct, fmtDrawdown, riskLevelLabel, formatNumber } from '../shared/format-metric.js';
import { getDemoPortfolio } from '../services/demo-data.js';
import { getThemeLabel } from '../shared/theme-labels.js';
import { initOnboarding } from '../components/onboarding.js';
import { scrollToSection } from '../shared/scroll-utils.js';
import { getDisclosureState, setDisclosureState } from '../shared/disclosure-state.js';

window.scrollToSection = scrollToSection;

const DASHBOARD_VERSION = 'v0.0.0.24';
const DATA_SOURCES = ['TWSE', 'Fugle', 'Replay 資料'];

let homeLoaded = false;
let calActiveCategories = [];

function prefersReducedMotion() {
  return window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

function animateValue(el, target, suffix = '', duration = 800) {
  if (prefersReducedMotion() || target === null || target === undefined || Number.isNaN(target)) {
    el.textContent = target + suffix;
    return;
  }
  const start = 0;
  const startTime = performance.now();
  function step(now) {
    const progress = Math.min((now - startTime) / duration, 1);
    const current = start + (target - start) * (progress * (2 - progress));
    el.textContent = (Number.isInteger(target) ? Math.round(current) : current.toFixed(1)) + suffix;
    if (progress < 1) requestAnimationFrame(step);
  }
  requestAnimationFrame(step);
}

export async function renderHomePage(container) {
  container.innerHTML = `
    <section class="home-today-summary card-priority-high" id="home-hero">
      <div class="home-today-summary__top">
        <h1 class="home-today-summary__title" id="home-summary">載入市場摘要…</h1>
        <span class="home-today-summary__help" data-tip="建議方向基於 AI 資本支出、外資流向、壓力指數、與多個市場信號的一致性計算。分數越高，信號一致性越好。若資料不足或信號互相矛盾，會顯示觀望。"></span>
        <span id="home-risk-badge"></span>
        <span class="home-today-summary__update" id="home-last-update">最後更新：--</span>
      </div>
      <p class="home-today-summary__reason" id="home-summary-reason"></p>
      <div class="home-today-summary__indicators" id="home-today-indicators">
        <div class="home-loading-card">載入中…</div>
      </div>
      <div class="home-today-summary__actions">
        <button class="btn btn--primary" id="home-view-market">查看市場詳情</button>
      </div>
    </section>

    <section class="home-section" id="home-signal-strip">
      <div class="home-section__header">
        <h2>今日信號</h2>
        <span class="home-section__subtitle">主動偵測的市場事件</span>
      </div>
      <div class="home-signal-strip" id="home-signal-strip-content">
        <div class="home-loading-card">載入中…</div>
      </div>
    </section>

    <section class="home-section" id="home-portfolio-snapshot">
      <div class="home-section__header">
        <h2>AI 策略績效（300 萬示範組合）</h2>
        <span class="home-section__subtitle">此為模擬交易績效展示，非真實帳戶</span>
      </div>
      <div id="home-portfolio-content">
        <div class="home-loading-card">載入中…</div>
      </div>
    </section>

    <section class="home-section" id="home-market-pulse">
      <div class="home-section__header">
        <h2>市場脈動</h2>
        <span class="home-section__subtitle">核心觀察指標</span>
      </div>
      <div class="home-grid home-grid--4" id="home-market-grid" data-disclosure-section="market-pulse" data-disclosure-state="collapsed">
        <div class="home-loading-card">載入中…</div>
      </div>
      <button class="disclosure-toggle" id="market-pulse-toggle" type="button" aria-expanded="false" aria-controls="home-market-grid" aria-label="展開 6 張進階指標">
        <span class="disclosure-toggle__label">展開進階指標</span>
        <span class="disclosure-toggle__icon" aria-hidden="true">▼</span>
      </button>
      <span class="sr-only" id="market-pulse-status" aria-live="polite" aria-atomic="true"></span>
    </section>

    <section class="home-section" id="home-event-calendar">
      <div class="home-section__header">
        <h2>市場行事曆</h2>
        <span class="home-section__subtitle">近期除權息、法說會、財報等重要事件</span>
      </div>
      <div class="cal-filter-bar" id="cal-filter-bar">
        <button class="cal-filter-pill active" data-category="">全部</button>
        <button class="cal-filter-pill" data-category="除權息">除權息</button>
        <button class="cal-filter-pill" data-category="法說會/財報">法說會</button>
        <button class="cal-filter-pill" data-category="總經事件">總經</button>
      </div>
      <div id="home-calendar-content">
        <div class="home-loading-card">載入中…</div>
      </div>
    </section>

    <section class="home-section home-section--transparent" id="home-trust">
      <div id="home-trust-footer"></div>
    </section>
  `;

  document.getElementById('home-view-market').addEventListener('click', () => {
    window.switchPage('crossmarket');
  });

  const calContainer = document.getElementById('home-calendar-content');
  const filterBar = document.getElementById('cal-filter-bar');
  if (filterBar && calContainer) {
    filterBar.addEventListener('click', (e) => {
      const pill = e.target.closest('.cal-filter-pill');
      if (!pill) return;
      const category = pill.dataset.category;
      if (category === '') {
        calActiveCategories = [];
      } else {
        const idx = calActiveCategories.indexOf(category);
        if (idx >= 0) {
          calActiveCategories.splice(idx, 1);
        } else {
          calActiveCategories.push(category);
        }
      }
      filterBar.querySelectorAll('.cal-filter-pill').forEach(p => {
        const c = p.dataset.category;
        p.classList.toggle('active', c === '' ? calActiveCategories.length === 0 : calActiveCategories.includes(c));
      });
      renderEventCalendar(calContainer, calActiveCategories).catch(err =>
        console.warn('[home] calendar render failed:', err));
    });
  }

  await loadHomeData();
  homeLoaded = true;
  initOnboarding();
}

async function loadHomeData() {
  try {
    const now = new Date();
    try {
      document.getElementById('home-last-update').textContent =
        `最後更新：${now.getHours().toString().padStart(2, '0')}:${now.getMinutes().toString().padStart(2, '0')}`;
    } catch (_e) {}

    if (isMockMode()) {
      const m = mockData();
      renderTodaySummary(m.macro, m.stress, m.pipeline, m.narrative, null);
      renderSignalStrip(m.narrative);
      renderMarketPulse(m.macro, m.stress, null);
      renderRecommendation(m.pipeline, m.stress);
      loadPortfolioSnapshot();

      renderTrustFooter(['Fugle', 'TWSE', 'FRED']);
      return;
    }

    try {
      const [health, macro, stress, pipeline, bundle, calData] = await Promise.all([
        silentGetJSON('/api/dashboard/system-health'),
        silentGetJSON('/api/macro/snapshot/latest'),
        silentGetJSON('/api/taiwan/stress-index'),
        silentGetJSON('/api/dashboard/recommendation-pipeline'),
        silentGetJSON('/api/narrative/bundle'),
        silentGetJSON('/api/dashboard/calendar-events'),
      ]);

      const events = bundle && bundle.events ? bundle.events : [];
      renderTodaySummary(macro, stress, pipeline, events);
      renderSignalStrip(events);
      renderMarketPulse(macro, stress);
      renderRecommendation(pipeline, stress);

      // Event calendar — fetched independently, non-blocking
      const calContainer = document.getElementById('home-calendar-content');
      if (calContainer) {
        renderEventCalendar(calContainer, calActiveCategories).catch(err =>
          console.warn('[home] calendar render failed:', err));
      }
    } catch (err) {
      console.warn('[home] failed to load dashboard data:', err);
      renderTodaySummary(null, null, null, [], null);
      renderSignalStrip([]);
      renderMarketPulse(null, null, null);
      renderRecommendation(null, null);
    }

    // Portfolio snapshot is loaded independently; it may be empty/demo.
    await loadPortfolioSnapshot();
  } catch (err) {
    console.error('[home] unexpected error loading home data:', err);
  } finally {
    renderTrustFooter();
  }
}

function pickRiskFromStress(stress) {
  const score = pointValue(stress, 'score');
  if (score === null) return 'unknown';
  // Stress index is 0–100; higher score = more stress.
  if (score >= 70) return 'high';
  if (score >= 40) return 'medium';
  return 'low';
}

function heroRecommendation(stress, pipeline, events) {
  const activeEvents = events.filter(e => e && e.status === 'active');
  const hasPipeline = pipeline && Array.isArray(pipeline.items) && pipeline.items.length > 0;
  const hasMacro = stress && pointValue(stress, 'score') !== null;

  if (!activeEvents.length && !hasPipeline && !hasMacro) {
    return { rec: '觀望', reason: '資料更新中 — 請稍後再訪', risk: 'unknown', hasRec: false };
  }

  const stressRisk = pickRiskFromStress(stress);

  if (activeEvents.length > 0) {
    const topEvent = activeEvents.reduce((best, e) =>
      ((e.confidence || 0) >= (best.confidence || 0) ? e : best), activeEvents[0]);
    const confidence = topEvent.confidence || 0;
    const bullish = (topEvent.sentiment || 0) >= 0;

    if (confidence < 0.7) {
      return { rec: '觀望', reason: '市場方向不明，建議觀望', risk: stressRisk, hasRec: true };
    }
    if (bullish) {
      const reason = stressRisk === 'low' ? '信號一致看多，壓力低位' : '偏多訊號出現，留意機會';
      return { rec: '偏多', reason, risk: stressRisk, hasRec: true };
    }
    const reason = stressRisk === 'high' ? '壓力偏高，降低曝險' : '偏空訊號出現，保守因應';
    return { rec: '偏空', reason, risk: stressRisk, hasRec: true };
  }

  if (hasPipeline) {
    const top = pipeline.items[0];
    const conviction = typeof top.conviction === 'number' ? top.conviction : 0;
    const side = (top.side || '').toLowerCase();

    if (conviction < 0.7) {
      return { rec: '觀望', reason: '市場方向不明，建議觀望', risk: stressRisk, hasRec: true };
    }
    if (side === 'buy') {
      const reason = stressRisk === 'low' ? '信號一致看多，壓力低位' : '模型偏多，控制倉位';
      return { rec: '偏多', reason, risk: stressRisk, hasRec: true };
    }
    if (side === 'sell') {
      const reason = stressRisk === 'high' ? '壓力偏高，降低曝險' : '模型偏空，降低曝險';
      return { rec: '偏空', reason, risk: 'high', hasRec: true };
    }
    return { rec: '觀望', reason: '市場方向不明，建議觀望', risk: stressRisk, hasRec: true };
  }

  return { rec: '觀望', reason: '資料已載入，等待明確信號', risk: stressRisk, hasRec: false };
}

function renderTodaySummary(macro, stress, pipeline, events) {
  const summaryEl = document.getElementById('home-summary');
  const reasonEl = document.getElementById('home-summary-reason');
  const badgeEl = document.getElementById('home-risk-badge');
  const indicatorsEl = document.getElementById('home-today-indicators');

  const { rec, reason, risk } = heroRecommendation(stress, pipeline, events);

  summaryEl.textContent = rec;
  reasonEl.textContent = reason;
  if (badgeEl) badgeEl.innerHTML = renderRiskBadge(risk, riskLevelLabel(risk));

  if (indicatorsEl) {
    const dirTone = rec === '偏多' ? 'bullish' : rec === '偏空' ? 'bearish' : 'neutral';
    const dirLabel = rec === '偏多' ? '↑偏多' : rec === '偏空' ? '↓偏空' : '→觀望';
    const stressScoreVal = pointValue(stress, 'score');
    const stressVal = stressScoreVal !== null ? stressScoreVal.toFixed(0) : '—';
    const stressLabel = stressVal;
    const foreignChange = macro ? pointChange(macro, 'foreign_investor_net') : null;
    const foreignLabel = foreignChange !== null ? fmtSignedPct(foreignChange) : '—';
    const taiexVal = macro ? pointValue(macro, 'taiex') : null;
    const taiexLabel = taiexVal !== null ? formatNumber(taiexVal) : '—';

    indicatorsEl.innerHTML = [
      `<div class="home-today-indicator home-today-indicator--${dirTone}" title="今日建議方向">${dirLabel}</div>`,
      `<div class="home-today-indicator" title="壓力指數">壓力 ${stressLabel}</div>`,
      `<div class="home-today-indicator" title="外資買賣超">外資 ${foreignLabel}</div>`,
      `<div class="home-today-indicator" title="加權指數">加權 ${taiexLabel}</div>`,
    ].join('');
  }
}

function pointValue(obj, key) {
  if (!obj) return null;
  const v = obj[key];
  if (v === null || v === undefined) return null;
  if (typeof v === 'object' && v.value !== undefined) {
    const n = Number(v.value);
    return Number.isNaN(n) ? null : n;
  }
  const n = Number(v);
  return Number.isNaN(n) ? null : n;
}

function pointChange(obj, key) {
  if (!obj || !obj[key] || typeof obj[key] !== 'object') return null;
  const n = Number(obj[key].change_pct);
  return Number.isNaN(n) ? null : n;
}

// Format annualized volatility (decimal, e.g. 0.18) as percentage (e.g. "18.0%").
function formatVolatility(val) {
  if (val == null) return '—';
  const n = Number(val);
  if (Number.isNaN(n)) return '—';
  return (n * 100).toFixed(1) + '%';
}

// Tone mapping for volatility:
//   < 20% → positive (low vol, calm market)
//   20-30% → warning (elevated vol)
//   >= 30% → negative (high vol, risk regime)
function volatilityTone(val) {
  if (val == null) return 'neutral';
  const n = Number(val);
  if (Number.isNaN(n)) return 'neutral';
  if (n >= 0.30) return 'negative';
  if (n >= 0.20) return 'warning';
  return 'positive';
}

function marketTrendDirection(changePct) {
  if (changePct === null) return { value: '持平', tone: 'neutral' };
  return changePct > 0
    ? { value: '+偏多', tone: 'positive' }
    : changePct < 0
      ? { value: '-偏空', tone: 'negative' }
      : { value: '持平', tone: 'neutral' };
}

function renderMarketPulse(macro, stress) {
  const grid = document.getElementById('home-market-grid');
  if (!grid) return;

  // TAIEX
  const taiexChange = pointChange(macro, 'taiex');
  const trend = marketTrendDirection(taiexChange);

  // Foreign investor
  // Foreign investor
  const foreign = pointValue(macro, 'foreign_investor_net');
  const foreignText = foreign !== null ? fmtSignedPct(foreign / 100, true) : '—';
  const fundVal = pointValue(macro, 'domestic_fund_net');
  const fundText = fundVal !== null ? fmtSignedPct(fundVal / 100, true) : '—';
  const dealerVal = pointValue(macro, 'dealer_net');
  const dealerText = dealerVal !== null ? fmtSignedPct(dealerVal / 100, true) : '—';

  // Stress
  const stressScore = pointValue(stress, 'score') || 0;
  const stressRisk = stressScore >= 70 ? 'high' : stressScore >= 40 ? 'medium' : 'low';
  const stressLabel = riskLevelLabel(stressRisk);

  // Cross-market helpers — all fields are available from MacroDataSnapshot.
  function cmField(obj, key) {
    return pointValue(obj, key);
  }
  function cmChange(obj, key) {
    return pointChange(obj, key);
  }

  const tsmChange = cmChange(macro, 'tsm_adr');
  const soxChange = cmChange(macro, 'sox_index');
  const ndxChange = cmChange(macro, 'ndx_index');
  const usdtwd = cmField(macro, 'usd_twd');
  const vixVal = cmField(macro, 'vix');
  const marginVal = pointValue(macro, 'retail_margin_balance');
  const retailChange = pointChange(macro, 'retail_margin_balance');
  const retailText = retailChange !== null
    ? (retailChange >= 0 ? '偏多 ' : '偏空 ') + fmtSignedPct(retailChange, 0)
    : '—';

  const cards = [
    metricCard({ id: 'market-taiwan', label: '大盤', value: trend.value, delta: taiexChange !== null ? fmtSignedPct(taiexChange) : null, tone: trend.tone, tooltip: '加權指數近期漲跌幅。', extraClasses: 'card-priority-high disclosure-tier-core' }),
    metricCard({ id: 'market-foreign', label: '外資', value: foreignText, tone: foreign > 0 ? 'positive' : foreign < 0 ? 'negative' : 'neutral', tooltip: '外資近一交易日淨買賣超（億元）。', extraClasses: 'card-priority-high disclosure-tier-core' }),
    metricCard({ id: 'market-tsm', label: 'TSM ADR', value: tsmChange !== null ? fmtSignedPct(tsmChange) : '—', tone: tsmChange >= 0 ? 'positive' : 'negative', tooltip: '台積電 ADR 漲跌幅，領先台股現貨。', extraClasses: 'card-priority-high disclosure-tier-core' }),
    metricCard({ id: 'market-sox', label: 'SOX 半導體', value: soxChange !== null ? fmtSignedPct(soxChange) : '—', tone: soxChange >= 0 ? 'positive' : 'negative', tooltip: '費城半導體指數，台股科技股先行指標。', extraClasses: 'card-priority-medium disclosure-tier-core' }),
    metricCard({ id: 'market-nasdaq', label: 'NASDAQ', value: ndxChange !== null ? fmtSignedPct(ndxChange) : '—', tone: ndxChange >= 0 ? 'positive' : 'negative', tooltip: '那斯達克指數漲跌幅。', extraClasses: 'card-priority-medium disclosure-tier-advanced' }),
    metricCard({ id: 'market-usdtwd', label: 'USD/TWD', value: usdtwd !== null ? usdtwd.toFixed(2) : '—', tone: 'neutral', tooltip: '美元兌台幣匯率，影響外資進出意願。', extraClasses: 'card-priority-medium disclosure-tier-core' }),
    metricCard({ label: 'VIX', value: vixVal !== null ? vixVal.toFixed(1) : '—', tone: vixVal >= 25 ? 'negative' : vixVal >= 20 ? 'warning' : 'positive', tooltip: '恐慌指數，>20 風險升高、>25 警戒。', extraClasses: 'card-priority-low disclosure-tier-advanced' }),
    metricCard({ label: '融資餘額', value: marginVal !== null ? `${(marginVal / 100).toFixed(0)} 億` : '—', tone: 'neutral', tooltip: '散戶融資餘額（億元），反映市場熱度。', extraClasses: 'card-priority-low disclosure-tier-advanced' }),
    metricCard({ label: '投信動向', value: fundText, tone: fundVal > 0 ? 'positive' : fundVal < 0 ? 'negative' : 'neutral', tooltip: '投信近一交易日買賣超（億元）。', extraClasses: 'card-priority-low disclosure-tier-advanced' }),
    metricCard({ label: '自營商', value: dealerText, tone: dealerVal > 0 ? 'positive' : dealerVal < 0 ? 'negative' : 'neutral', tooltip: '自營商近一交易日買賣超（億元）。', extraClasses: 'card-priority-low disclosure-tier-advanced' }),
    metricCard({ label: '歷史波動', value: formatVolatility(pointValue(macro, 'historical_volatility')), tone: volatilityTone(pointValue(macro, 'historical_volatility')), tooltip: 'TAIEX 20 日年化波動率。<20% 低波動、20-30% 中等、>30% 高波動警戒。', extraClasses: 'card-priority-low disclosure-tier-advanced' }),
    metricCard({ label: '散戶情緒', value: retailText, tone: retailChange === null ? 'neutral' : retailChange >= 0 ? 'positive' : 'negative', tooltip: '散戶融資餘額變化 — 偏多表示融資增加（槓桿意願高），偏空表示融資減少。', extraClasses: 'card-priority-low disclosure-tier-advanced' }),
  ];

  if (grid) {
    const initialState = getDisclosureState('market-pulse', 'collapsed');
    grid.setAttribute('data-disclosure-state', initialState);
    // TODO(lazy-load): 目前 11 張卡一次 render; collapsed 狀態 CSS 隱藏 6 張進階卡
    // 但 data 已 fetch 進來。等後端 /api/macro/snapshot 支援 fields=core 過濾後,
    // 改為 collapsed 只 render 5 張,展開時再 fetch advanced 資料 (見 disclosure-state.js header)。
    grid.innerHTML = cards.join('');
    bindMarketPulseDisclosure();
  }
}

let _marketPulseDisclosureBound = false;

function bindMarketPulseDisclosure() {
  if (_marketPulseDisclosureBound) return;
  const btn = document.getElementById('market-pulse-toggle');
  const grid = document.getElementById('home-market-grid');
  if (!btn || !grid) return;
  _marketPulseDisclosureBound = true;

  const updateButton = (state) => {
    btn.setAttribute('aria-expanded', state === 'expanded' ? 'true' : 'false');
    const labelEl = btn.querySelector('.disclosure-toggle__label');
    const iconEl = btn.querySelector('.disclosure-toggle__icon');
    const statusEl = document.getElementById('market-pulse-status');
    if (state === 'expanded') {
      if (labelEl) labelEl.textContent = '收合進階指標';
      if (iconEl) iconEl.textContent = '▲';
      btn.setAttribute('aria-label', '收合 6 張進階指標');
      if (statusEl) statusEl.textContent = '已展開 6 張進階指標';
    } else {
      if (labelEl) labelEl.textContent = '展開進階指標';
      if (iconEl) iconEl.textContent = '▼';
      btn.setAttribute('aria-label', '展開 6 張進階指標');
      if (statusEl) statusEl.textContent = '';
    }
  };

  updateButton(grid.getAttribute('data-disclosure-state') || 'collapsed');

  btn.addEventListener('click', () => {
    const current = grid.getAttribute('data-disclosure-state') || 'collapsed';
    const next = current === 'expanded' ? 'collapsed' : 'expanded';
    grid.setAttribute('data-disclosure-state', next);
    setDisclosureState('market-pulse', next);
    updateButton(next);
  });
}

function explainFromEvent(e) {
  if (e.description && typeof e.description === 'string' && e.description.trim()) {
    return truncate(e.description.trim(), 50);
  }
  const label = getThemeLabel(e.theme);
  const bullish = (e.sentiment || 0) >= 0;
  return bullish ? `${label}面向正面，可關注相關持股` : `${label}面臨壓力，留意風險`;
}

function linkFromEvent(e) {
  if (!e.affected_industries) return '';
  const industries = Array.isArray(e.affected_industries)
    ? e.affected_industries
    : [e.affected_industries];
  const valid = industries.filter(x => x && typeof x === 'string').map(x => x.trim()).filter(Boolean);
  if (!valid.length) return '';
  return `相關：${valid.join('、')}`;
}

function truncate(str, max) {
  if (str.length <= max) return str;
  return str.slice(0, max - 1) + '…';
}

function renderSignalStrip(events) {
  const el = document.getElementById('home-signal-strip-content');
  if (!el) return;
  if (!el) return;

  const active = events.filter(e => e && e.status === 'active');
  if (!active.length) {
if (!el) return;
  el.innerHTML = '<div class="home-signal-empty">尚無主動信號 — 市場處於平靜期</div>';
  return;
 }
 if (!el) return;
 el.innerHTML = active.map(e => {
    const label = escapeHtml(getThemeLabel(e.theme));
    const sent = (e.sentiment || 0) >= 0 ? 'bullish' : 'bearish';
    const conf = e.confidence ? `${(e.confidence * 100).toFixed(0)}%` : '—';
    const sev = e.severity || 'low';
    const explain = escapeHtml(explainFromEvent(e));
    const link = escapeHtml(linkFromEvent(e));
    const marketId = signalToMarket(e.theme);
    const onclick = marketId ? ` onclick="window.scrollToSection?.('%23${marketId}')"` : '';
    return `<div class="signal-chip signal-chip--${sent} signal-chip--sev-${sev}"${onclick} title="信心:${conf}|嚴重度:${sev}${marketId?' |點擊查看⇣':''}">
      <div class="signal-chip__row">
        <span class="signal-chip__label">${label}</span>
        <span class="signal-chip__meta">${sent === 'bullish' ? '↑利多' : '↓利空'} · ${conf}</span>
      </div>
      <span class="signal-chip__explain">${explain}</span>
      ${link ? `<span class="signal-chip__link">${link}</span>` : ''}
    </div>`;
  }).join('');
}

function signalToMarket(theme) {
  const map = {
    ai_capex: 'market-tsm',
    yen_carry: 'market-foreign',
    earnings_surprise: 'market-taiwan',
    tech_cycle: 'market-sox',
    macro_risk: 'market-usdtwd',
    sentiment_shift: 'market-taiwan',
  };
  return map[theme] || null;
}

function renderRecommendation(pipeline, stress) {
  const card = document.getElementById('home-rec-content');

  let action = '觀望';
  let reason = '目前資料不足以產生明確建議，請確認模擬已執行或查看市場頁面。';
  let confidence = 0;
  let tone = 'neutral';

  if (pipeline && Array.isArray(pipeline.items) && pipeline.items.length > 0) {
    const top = pipeline.items[0];
    const conviction = typeof top.conviction === 'number' ? top.conviction : 0;
    const side = (top.side || '').toLowerCase();

    if (side === 'buy') {
      action = '配置';
      tone = 'positive';
    } else if (side === 'sell') {
      action = '減碼';
      tone = 'negative';
    } else {
      action = '觀望';
      tone = 'neutral';
    }

    confidence = Math.min(100, Math.max(0, Math.round(conviction * 100)));
    reason = top.reason || `模型對 ${escapeHtml(top.symbol || '市場')} 的綜合評估為「${action}」，信心 ${confidence}%。`;
  }

  if (card) {
    card.innerHTML = `
      <div class="home-recommendation__action">
      <span class="home-recommendation__label">${renderTooltip('建議行動', '模型根據當日市場壓力、外資流向與趨勢評分，給出的今日操作傾向（配置 / 觀望 / 減碼）。')}</span>
      <span class="home-recommendation__value home-recommendation__value--${tone}">${escapeHtml(action)}</span>
    </div>
    <div class="home-recommendation__confidence">
      <span class="home-recommendation__label">${renderTooltip('信心分數', '模型對今日建議的把握程度（0–100%）。數值越高代表多項指標方向一致，越值得參考。')}</span>
      <div class="home-confidence-bar" aria-label="信心分數 ${confidence}%" role="img">
        <div class="home-confidence-bar__fill home-confidence-bar__fill--${tone}" style="width: ${confidence}%"></div>
      </div>
      <span class="home-recommendation__score">${confidence}%</span>
    </div>
    <p class="home-recommendation__reason">${escapeHtml(reason)}</p>
    <div class="home-recommendation__actions">
      <button class="btn btn--primary" id="home-rec-detail">查看決策鏈</button>
      <button class="btn btn--secondary" id="home-rec-portfolio">調整組合</button>
    </div>
  `;

  document.getElementById('home-rec-detail').addEventListener('click', () => window.switchPage('decision'));
  document.getElementById('home-rec-portfolio').addEventListener('click', () => window.switchPage('portfolio'));
  }
}

async function loadPortfolioSnapshot() {
  const container = document.getElementById('home-portfolio-content');
  try {
    const data = await silentGetJSON('/api/dashboard/portfolio-state');
    if (!data || !Array.isArray(data.positions) || data.positions.length === 0) {
      renderDemoPortfolio(container);
      return;
    }
    renderRealPortfolio(container, data);
  } catch (err) {
    console.warn('[home] portfolio snapshot unavailable:', err);
    renderDemoPortfolioWithData(container);
  }
}

function pnlMetricCard(label, value, isProfit) {
  const cls = isProfit ? 'home-pnl-profit' : 'home-pnl-loss';
  const card = metricCard({ label, value, tone: 'neutral' });
  return card.replace(/class="kpi-value\s*"/, `class="kpi-value ${cls} "`);
}

function portfolioReassurance(pnl) {
  if (pnl >= 0) return '';
  return '<p class="home-portfolio-note">短期回撤 — 策略持續運作中，過去績效不代表未來結果</p>';
}

function renderRealPortfolio(container, data) {
  const total = data.portfolio_value || 0;
  const pnl = data.cumulative_pnl || 0;
  const pnlPct = typeof data.cumulative_pnl_pct === 'number'
    ? data.cumulative_pnl_pct
    : (total > 0 && total !== pnl ? (pnl / (total - pnl)) : 0);
  const drawdown = data.current_drawdown || 0;

  container.innerHTML = `
    <div class="kpi-grid">
      ${metricCard({ label: '總市值', value: fmtNTD(total), tone: 'neutral' })}
      ${pnlMetricCard('損益', fmtSignedPct(pnlPct * 100), pnl >= 0)}
      ${metricCard({ label: '最大回撤', value: fmtDrawdown(drawdown), tone: 'neutral' })}
    </div>
    ${portfolioReassurance(pnl)}
    <button class="btn btn--secondary" id="home-portfolio-detail">查看完整持倉</button>
  `;
  document.getElementById('home-portfolio-detail').addEventListener('click', () => window.switchPage('portfolio'));
}

function fmtNTD(value) {
  if (value === null || value === undefined || Number.isNaN(value)) return '—';
  return `NT$ ${formatNumber(value, { decimals: 0 })}`;
}

function renderDemoPortfolio(container) {
  container.innerHTML = `
    <div class="action-card">
      <div class="action-card__icon">📋</div>
      <h3 class="action-card__title">尚無投資組合資料</h3>
      <p class="action-card__message">您可載入示範組合，快速體驗平台如何呈現持倉、風險與建議。</p>
      <div class="action-card__actions">
        <button class="btn btn--primary" id="home-load-demo">載入示範組合</button>
        <button class="btn btn--secondary" id="home-goto-portfolio">前往組合頁面</button>
      </div>
    </div>
  `;

  document.getElementById('home-load-demo').addEventListener('click', () => {
    window.dispatchEvent(new CustomEvent('atlas:load-demo-portfolio'));
    loadPortfolioSnapshot();
  });
  document.getElementById('home-goto-portfolio').addEventListener('click', () => window.switchPage('portfolio'));
}

function renderDemoPortfolioWithData(container) {
  const positions = getDemoPortfolio();
  const totalValue = positions.reduce((sum, p) => sum + p.shares * p.price, 0);
  const totalCost = positions.reduce((sum, p) => sum + p.shares * p.avgCost, 0);
  const totalPnl = totalValue - totalCost;
  const pnlPct = totalCost > 0 ? totalPnl / totalCost : 0;
  const topPositions = positions.slice(0, 3);

  container.innerHTML = `
    <div class="home-demo-badge">示範數據</div>
    <div class="kpi-grid">
      ${metricCard({ label: '示範總市值', value: fmtNTD(totalValue), tone: 'neutral', tooltip: 'DEMO 資料' })}
      ${pnlMetricCard('損益', fmtSignedPct(pnlPct * 100), totalPnl >= 0)}
      ${metricCard({ label: '最大回撤', value: '−8.5%', tone: 'neutral', tooltip: '示範組合歷史最大回撤' })}
      ${metricCard({ label: '夏普比率', value: '1.62', tone: 'neutral', tooltip: '風險調整後報酬' })}
      ${metricCard({ label: '勝率', value: '62%', tone: 'neutral', tooltip: '示範組合交易勝率' })}
    </div>
    <div class="home-demo-positions">
      ${topPositions.map(p => {
        const mkt = p.shares * p.price;
        const pnl = p.price - p.avgCost;
        const pnlPct = p.avgCost > 0 ? pnl / p.avgCost : 0;
        return `
          <div class="home-demo-position">
            <div class="home-demo-position__meta">
              <span class="home-demo-position__symbol">${escapeHtml(p.symbol)}</span>
              <span class="home-demo-position__name">${escapeHtml(p.name)}</span>
              <span class="home-demo-position__sector">${escapeHtml(p.sector)}</span>
            </div>
            <div class="home-demo-position__values">
              <span class="home-demo-position__weight">${formatNumber(p.weight * 100, { decimals: 0 })}%</span>
              <span class="home-demo-position__pnl ${pnl >= 0 ? 'home-pnl-profit' : 'home-pnl-loss'}">${fmtSignedPct(pnlPct * 100)}</span>
            </div>
          </div>
        `;
      }).join('')}
    </div>
    ${portfolioReassurance(totalPnl)}
    <button class="btn btn--secondary" id="home-portfolio-detail">查看完整持倉</button>
  `;

  document.getElementById('home-portfolio-detail').addEventListener('click', () => window.switchPage('portfolio'));
}

function renderTrustFooter(availableSources = []) {
  const container = document.getElementById('home-trust-footer');
  if (!container) return;
  const sources = availableSources.length > 0
    ? DATA_SOURCES.filter(s => availableSources.includes(s.key))
    : DATA_SOURCES;
  container.innerHTML = trustFooter({
    version: DASHBOARD_VERSION,
    sources,
    disclaimer: '本平台為研究模擬用途，不構成投資建議。投資人應獨立判斷並自負風險。'
  });
}

function isMockMode() {
  try { if (new URLSearchParams(window.location.search).has('mock')) return true; } catch (_e) {}
  try { if (localStorage.getItem('atlas-mock') === 'true') return true; } catch (_e) {}
  return false;
}

function mockData() {
  return {
    // Field names must match what pointValue/pointChange read:
    // pointValue(macro,'taiex')         → macro['taiex'] (object with .value OR bare number)
    // pointChange(macro,'foreign_investor_net') → macro['foreign_investor_net'].change_pct (object required)
    macro: {
      taiex: { symbol: 'TAIEX', value: 23100.5, change_pct: 0.35, timestamp: new Date().toISOString() },
      foreign_investor_net: { symbol: 'FOREIGN', value: 45.2, change_pct: 0.012, timestamp: new Date().toISOString() },
      tsm_adr: { symbol: 'TSM', value: 162.8, change_pct: -0.008, timestamp: new Date().toISOString() },
      sox_index: { symbol: 'SOX', value: 5820.1, change_pct: 0.022, timestamp: new Date().toISOString() },
      ndx_index: { symbol: 'NDX', value: 21450.3, change_pct: 0.018, timestamp: new Date().toISOString() },
      retail_margin_balance: { symbol: 'MARGIN', value: 3250.0, change_pct: 0.05, timestamp: new Date().toISOString() },
      vix: { symbol: 'VIX', value: 14.3, change_pct: -0.03, timestamp: new Date().toISOString() },
      usd_twd: { symbol: 'USD/TWD', value: 32.15, change_pct: 0.001, timestamp: new Date().toISOString() },
    },
    pipeline: {
      items: [
        { theme: 'ai_capex', sentiment: 0.92, confidence: 0.88, severity: 'high', affected_industries: ['半導體', 'AI伺服器'] },
        { theme: 'yen_carry', sentiment: 0.87, confidence: 0.82, severity: 'medium', affected_industries: ['金融', '外銷'] },
        { theme: 'earnings_surprise', sentiment: 0.85, confidence: 0.90, severity: 'high', affected_industries: ['半導體', '電子零組件'] },
        { theme: 'stress_low', sentiment: 0.90, confidence: 0.85, severity: 'low', affected_industries: [] },
      ],
    },
    stress: { score: 28.5, regime: 'risk_on' },
    narrative: [
      { theme: '除權息旺季', sentiment: 0.6, confidence: 0.7, severity: 'medium' },
      { theme: '台指期結算', sentiment: 0.3, confidence: 0.8, severity: 'low' },
      { theme: '台積電法說會', sentiment: 0.7, confidence: 0.85, severity: 'high' },
    ],
    portfolio: { totalValue: 3028000, cumulativePnL: 0.032, cumulativePnLAmount: 96000, monthlyPnL: 0.015, sharpeRatio: 1.8, maxDrawdown: -0.021, winRate: 0.62 },
  };
}

export function initHomePage() {
  if (homeLoaded) return;
  const container = document.getElementById('page-home');
  if (!container) return;

  if (isMockMode()) {
    container.setAttribute('data-mock', 'true');
  }

  renderHomePage(container).catch(err => console.error('[home] init failed:', err));
}
