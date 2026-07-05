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

const DASHBOARD_VERSION = 'v0.0.0.24';
const DATA_SOURCES = ['TWSE', 'Fugle', 'Replay 資料'];

let homeLoaded = false;

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
    <section class="home-hero" id="home-hero">
      <div class="home-hero__kicker">Atlas-Go 每日市場摘要</div>
      <h1 class="home-hero__title" id="home-summary">載入市場摘要…</h1>
      <div class="home-hero__meta">
        <span id="home-risk-badge"></span>
        <span class="home-hero__update" id="home-last-update">最後更新：--</span>
      </div>
      <div class="home-hero__rec" id="home-rec-content">
        <div class="home-loading-card">載入中…</div>
      </div>
      <div class="home-hero__actions">
        <button class="btn btn--primary" id="home-view-market">查看市場詳情</button>
        <button class="btn btn--secondary" id="home-view-portfolio">我的組合</button>
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
        <h2>我的組合快覽</h2>
        <span class="home-section__subtitle">持倉摘要或示範資料</span>
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
      <div class="home-grid home-grid--4" id="home-market-grid">
        <div class="home-loading-card">載入中…</div>
      </div>
    </section>

    <section class="home-section" id="home-event-calendar">
      <div class="home-section__header">
        <h2>市場行事曆</h2>
        <span class="home-section__subtitle">近期除權息、法說會、財報等重要事件</span>
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
  document.getElementById('home-view-portfolio').addEventListener('click', () => {
    window.switchPage('portfolio');
  });

  await loadHomeData();
  homeLoaded = true;
}

async function loadHomeData() {
  try {
    const now = new Date();
    document.getElementById('home-last-update').textContent =
      `最後更新：${now.getHours().toString().padStart(2, '0')}:${now.getMinutes().toString().padStart(2, '0')}`;

    try {
      const [health, macro, stress, pipeline, bundle, crossStatus, calData] = await Promise.all([
        silentGetJSON('/api/dashboard/system-health'),
        silentGetJSON('/api/macro/snapshot/latest'),
        silentGetJSON('/api/taiwan/stress-index'),
        silentGetJSON('/api/dashboard/recommendation-pipeline'),
        silentGetJSON('/api/narrative/bundle'),
        silentGetJSON('/api/dashboard/us-indices'),
        silentGetJSON('/api/dashboard/calendar-events'),
      ]);

      const events = bundle && bundle.events ? bundle.events : [];
      renderHero(macro, stress, pipeline, events, crossStatus);
      renderSignalStrip(events);
      renderMarketPulse(macro, stress, crossStatus);
      renderRecommendation(pipeline, stress);

      // Event calendar — fetched independently, non-blocking
      const calContainer = document.getElementById('home-calendar-content');
      if (calContainer) {
        renderEventCalendar(calContainer).catch(err =>
          console.warn('[home] calendar render failed:', err));
      }
    } catch (err) {
      console.warn('[home] failed to load dashboard data:', err);
      renderHero(null, null, null, [], null);
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

function stressSummary(stress) {
  const score = pointValue(stress, 'score');
  if (score === null) return { summary: '市場資料載入中，請稍候。', risk: 'unknown' };
  const risk = pickRiskFromStress(stress);
  if (risk === 'high') {
    return { summary: '市場壓力偏高，建議降低曝險、保留現金。', risk };
  }
  if (risk === 'medium') {
    return { summary: '市場處於中性震盪，建議觀望並控制倉位。', risk };
  }
  return { summary: '市場壓力偏低，可留意配置機會。', risk };
}

function renderHero(macro, stress, pipeline, events, crossStatus) {
  const summaryEl = document.getElementById('home-summary');
  const badgeEl = document.getElementById('home-risk-badge');

  let { summary, risk } = stressSummary(stress);

  // Use narrative events to refine hero: pick the highest severity active event
  const activeEvents = events.filter(e => e && e.status === 'active');
  if (activeEvents.length > 0) {
    const topEvent = activeEvents.reduce((best, e) =>
      ((e.confidence || 0) >= (best.confidence || 0) ? e : best), activeEvents[0]);
    const label = getThemeLabel(topEvent.theme);
    const sent = (topEvent.sentiment || 0) >= 0 ? '偏多' : '偏空';
    const sevMap = { low: '輕微', medium: '中等', high: '重大', critical: '緊急' };
    const sevLabel = sevMap[topEvent.severity] || '';
    summary = `${sevLabel}信號「${label}」${sent} — ${topEvent.confidence >= 0.7 ? '多項指標方向一致' : '建議觀察後續發展'}`;
    risk = pickRiskFromStress(stress) === 'high' ? 'high'
         : (topEvent.sentiment || 0) < 0 ? 'medium'
         : 'low';
  }

  // Fallback: pipeline view
  if (!activeEvents.length && pipeline && Array.isArray(pipeline.items) && pipeline.items.length > 0) {
    const top = pipeline.items[0];
    const side = top.side || '';
    if (side.toLowerCase() === 'buy') {
      summary = '模型觀察到偏多訊號，可留意配置機會。';
      risk = pickRiskFromStress(stress) === 'high' ? 'high' : 'low';
    } else if (side.toLowerCase() === 'sell') {
      summary = '模型觀察到偏空訊號，建議降低曝險。';
      risk = 'high';
    }
  }

  // Fallback: macro data loaded
  if (!activeEvents.length && !(pipeline && Array.isArray(pipeline.items) && pipeline.items.length > 0)
      && risk === 'unknown' && macro && pointValue(macro, 'foreign_investor_net') !== null) {
    summary = '外資與大盤資料已載入，請觀察下方指標。';
    risk = 'medium';
  }

  summaryEl.textContent = summary;
  badgeEl.innerHTML = renderRiskBadge(risk, riskLevelLabel(risk));
}

function pointValue(obj, key) {
  if (!obj) return null;
  if (obj[key] !== undefined && obj[key] !== null) {
    const n = Number(obj[key]);
    return Number.isNaN(n) ? null : n;
  }
  if (obj[key] && typeof obj[key] === 'object' && obj[key].value !== undefined) {
    const n = Number(obj[key].value);
    return Number.isNaN(n) ? null : n;
  }
  return null;
}

function pointChange(obj, key) {
  if (!obj || !obj[key] || typeof obj[key] !== 'object') return null;
  const n = Number(obj[key].change_pct);
  return Number.isNaN(n) ? null : n;
}

function marketTrendDirection(changePct) {
  if (changePct === null) return { value: '持平', tone: 'neutral' };
  return changePct > 0
    ? { value: '+偏多', tone: 'positive' }
    : changePct < 0
      ? { value: '-偏空', tone: 'negative' }
      : { value: '持平', tone: 'neutral' };
}

function renderMarketPulse(macro, stress, crossStatus) {
  const grid = document.getElementById('home-market-grid');

  // TAIEX
  const taiexChange = pointChange(macro, 'taiex');
  const trend = marketTrendDirection(taiexChange);

  // Foreign investor
  const foreign = pointValue(macro, 'foreign_investor_net');
  const foreignText = foreign !== null ? fmtSignedPct(foreign / 100, true) : '—';

  // Stress
  const stressScore = pointValue(stress, 'score') || 0;
  const stressRisk = stressScore >= 70 ? 'high' : stressScore >= 40 ? 'medium' : 'low';
  const stressLabel = riskLevelLabel(stressRisk);

  // Cross-market helpers (macro snapshot may have these or fallback to crossStatus)
  function cmField(obj, cross, key) {
    const v = pointValue(obj, key);
    if (v !== null) return v;
    if (cross && cross[key]) {
      const raw = cross[key];
      if (raw && typeof raw === 'object' && raw.value != null) return Number(raw.value);
    }
    return null;
  }
  function cmChange(obj, cross, key) {
    const c = pointChange(obj, key);
    if (c !== null) return c;
    if (cross && cross[key] && typeof cross[key] === 'object') {
      const n = Number(cross[key].change_pct);
      return Number.isNaN(n) ? null : n;
    }
    return null;
  }

  const tsmChange = cmChange(macro, crossStatus, 'tsm_adr');
  const soxChange = cmChange(macro, crossStatus, 'sox');
  const ndxChange = cmChange(macro, crossStatus, 'ndx');
  const usdtwd = cmField(macro, crossStatus, 'usd_twd');
  const vixVal = cmField(macro, crossStatus, 'vix');
  const marginVal = pointValue(macro, 'retail_margin_balance');

  const cards = [
    metricCard({ label: '大盤', value: trend.value, delta: taiexChange !== null ? fmtSignedPct(taiexChange) : null, tone: trend.tone, tooltip: '加權指數近期漲跌幅。' }),
    metricCard({ label: '外資', value: foreignText, tone: foreign > 0 ? 'positive' : foreign < 0 ? 'negative' : 'neutral', tooltip: '外資近一交易日淨買賣超（億元）。' }),
    metricCard({ label: 'TSM ADR', value: tsmChange !== null ? fmtSignedPct(tsmChange) : '—', tone: tsmChange >= 0 ? 'positive' : 'negative', tooltip: '台積電 ADR 漲跌幅，領先台股現貨。' }),
    metricCard({ label: 'SOX 半導體', value: soxChange !== null ? fmtSignedPct(soxChange) : '—', tone: soxChange >= 0 ? 'positive' : 'negative', tooltip: '費城半導體指數，台股科技股先行指標。' }),
    metricCard({ label: 'NASDAQ', value: ndxChange !== null ? fmtSignedPct(ndxChange) : '—', tone: ndxChange >= 0 ? 'positive' : 'negative', tooltip: '那斯達克指數漲跌幅。' }),
    metricCard({ label: 'USD/TWD', value: usdtwd !== null ? usdtwd.toFixed(2) : '—', tone: 'neutral', tooltip: '美元兌台幣匯率，影響外資進出意願。' }),
    metricCard({ label: 'VIX', value: vixVal !== null ? vixVal.toFixed(1) : '—', tone: vixVal >= 25 ? 'negative' : vixVal >= 20 ? 'warning' : 'positive', tooltip: '恐慌指數，>20 風險升高、>25 警戒。' }),
    metricCard({ label: '融資餘額', value: marginVal !== null ? `${(marginVal / 100).toFixed(0)} 億` : '—', tone: 'neutral', tooltip: '散戶融資餘額（億元），反映市場熱度。' }),
  ];

  grid.innerHTML = cards.join('');
}

function renderSignalStrip(events) {
  const el = document.getElementById('home-signal-strip-content');
  if (!el) return;

  const active = events.filter(e => e && e.status === 'active');
  if (!active.length) {
    el.innerHTML = '<div class="home-signal-empty">尚無主動信號 — 市場處於平靜期</div>';
    return;
  }

  el.innerHTML = active.map(e => {
    const label = escapeHtml(getThemeLabel(e.theme));
    const sent = (e.sentiment || 0) >= 0 ? 'bullish' : 'bearish';
    const conf = e.confidence ? `${(e.confidence * 100).toFixed(0)}%` : '—';
    const sev = e.severity || 'low';
    return `<div class="signal-chip signal-chip--${sent} signal-chip--sev-${sev}" title="信心: ${conf} | 嚴重度: ${sev}">
      <span class="signal-chip__label">${label}</span>
      <span class="signal-chip__meta">${sent === 'bullish' ? '↑利多' : '↓利空'} · ${conf}</span>
    </div>`;
  }).join('');
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
      ${metricCard({ label: '損益', value: fmtSignedPct(pnlPct), tone: pnl >= 0 ? 'positive' : 'negative' })}
      ${metricCard({ label: '最大回撤', value: fmtDrawdown(drawdown), tone: 'neutral', extraClasses: 'advanced-only' })}
    </div>
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
    <div class="kpi-grid">
      ${metricCard({ label: '示範總市值', value: fmtNTD(totalValue), tone: 'neutral', tooltip: 'DEMO 資料' })}
      ${metricCard({ label: '損益', value: fmtSignedPct(pnlPct), tone: totalPnl >= 0 ? 'positive' : 'negative' })}
      ${metricCard({ label: '持倉檔數', value: positions.length, tone: 'neutral' })}
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
              <span class="home-demo-position__pnl ${pnl >= 0 ? 'positive' : 'negative'}">${fmtSignedPct(pnlPct)}</span>
            </div>
          </div>
        `;
      }).join('')}
    </div>
    <button class="btn btn--secondary" id="home-portfolio-detail">查看完整持倉</button>
  `;

  document.getElementById('home-portfolio-detail').addEventListener('click', () => window.switchPage('portfolio'));
}

function renderTrustFooter() {
  const container = document.getElementById('home-trust-footer');
  container.innerHTML = trustFooter({
    version: DASHBOARD_VERSION,
    sources: DATA_SOURCES,
    disclaimer: '本平台為研究模擬用途，不構成投資建議。投資人應獨立判斷並自負風險。'
  });
}

export function initHomePage() {
  if (homeLoaded) return;
  const container = document.getElementById('page-home');
  if (!container) return;
  renderHomePage(container).catch(err => console.error('[home] init failed:', err));
}
