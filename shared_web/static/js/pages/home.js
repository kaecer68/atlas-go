/**
 * Home page for retail investors.
 * Renders an editorial dashboard: market summary, recommendation,
 * portfolio snapshot, and trust elements.
 */

import { getJSON, silentGetJSON, escapeHtml } from '../shared/app-utils.js';
import { metricCard } from '../components/metric-card.js';
import { trustFooter } from '../components/trust-footer.js';
import { renderRiskBadge } from '../components/risk-badge.js';
import { fmtSignedPct, fmtDrawdown, fmtHHI, riskLevelLabel, formatNumber } from '../shared/format-metric.js';
import { getDemoPortfolio } from '../services/demo-data.js';

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
      <div class="home-hero__actions">
        <button class="btn btn--primary" id="home-view-market">查看市場詳情</button>
        <button class="btn btn--secondary" id="home-view-portfolio">我的組合</button>
      </div>
    </section>

    <section class="home-section" id="home-market-pulse">
      <div class="home-section__header">
        <h2>市場脈動</h2>
        <span class="home-section__subtitle">三大觀察指標</span>
      </div>
      <div class="home-grid home-grid--3" id="home-market-grid">
        <div class="home-loading-card">載入中…</div>
      </div>
    </section>

    <section class="home-section" id="home-recommendation">
      <div class="home-section__header">
        <h2>今日建議</h2>
        <span class="home-section__subtitle">模型輸出與信心分數</span>
      </div>
      <div class="home-recommendation__card" id="home-rec-card">
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
  const now = new Date();
  document.getElementById('home-last-update').textContent =
    `最後更新：${now.getHours().toString().padStart(2, '0')}:${now.getMinutes().toString().padStart(2, '0')}`;

  try {
    const [health, macro, stress, pipeline, alerts] = await Promise.all([
      silentGetJSON('/api/dashboard/system-health'),
      silentGetJSON('/api/macro/snapshot/latest'),
      silentGetJSON('/api/taiwan/stress-index'),
      silentGetJSON('/api/dashboard/recommendation-pipeline'),
      silentGetJSON('/api/alerts'),
    ]);

    renderHero(macro, stress, alerts);
    renderMarketPulse(health, macro, stress);
    renderRecommendation(pipeline, stress);
  } catch (err) {
    console.warn('[home] failed to load dashboard data:', err);
    renderHero(null, null, null);
    renderMarketPulse(null, null, null);
    renderRecommendation(null, null);
  }

  // Portfolio snapshot is loaded independently; it may be empty/demo.
  await loadPortfolioSnapshot();
  renderTrustFooter();
}

function renderHero(macro, stress, alerts) {
  const summaryEl = document.getElementById('home-summary');
  const badgeEl = document.getElementById('home-risk-badge');

  let risk = 'unknown';
  let summary = '市場資料載入中，請稍候。';

  if (alerts && Array.isArray(alerts.items) && alerts.items.length > 0) {
    const top = alerts.items[0];
    summary = top.title || '市場出現值得關注的訊號。';
    risk = top.severity || 'medium';
  } else if (stress && typeof stress.level === 'number') {
    if (stress.level >= 0.7) {
      summary = '市場壓力偏高，建議降低曝險、保留現金。';
      risk = 'high';
    } else if (stress.level >= 0.4) {
      summary = '市場處於中性震盪，建議觀望並控制倉位。';
      risk = 'medium';
    } else {
      summary = '市場壓力偏低，可留意配置機會。';
      risk = 'low';
    }
  } else if (macro && macro.foreign_capital_signal) {
    summary = `外資動向：${macro.foreign_capital_signal}。`;
    risk = 'medium';
  }

  summaryEl.textContent = summary;
  badgeEl.innerHTML = renderRiskBadge(risk, riskLevelLabel(risk));
}

function renderMarketPulse(health, macro, stress) {
  const grid = document.getElementById('home-market-grid');

  const marketTrend = (macro && macro.taiwan_ex_direction) || '—';
  const marketTrendValue = marketTrend === 'up' ? '+偏多' : marketTrend === 'down' ? '-偏空' : '持平';
  const marketTrendClass = marketTrend === 'up' ? 'positive' : marketTrend === 'down' ? 'negative' : 'neutral';

  const foreign = (macro && macro.foreign_investor_net_billions) || null;
  const foreignText = foreign !== null ? fmtSignedPct(foreign / 100, true) : '—';

  const stressLevel = (stress && stress.level) || 0;
  const stressLabel = riskLevelLabel(stressLevel >= 0.7 ? 'high' : stressLevel >= 0.4 ? 'medium' : 'low');

  grid.innerHTML = [
    metricCard({
      label: '大盤趨勢',
      value: marketTrendValue,
      delta: macro && macro.taiwan_ex_change_pct ? fmtSignedPct(macro.taiwan_ex_change_pct) : null,
      tone: marketTrendClass,
      tooltip: '加權指數近期趨勢方向，資料來自 replay 與市場資料。'
    }),
    metricCard({
      label: '外資動向',
      value: foreignText,
      delta: null,
      tone: foreign > 0 ? 'positive' : foreign < 0 ? 'negative' : 'neutral',
      tooltip: '外資近一交易日淨買賣超（億元）。'
    }),
    metricCard({
      label: '市場壓力',
      value: stressLabel,
      delta: stressLevel ? `${(stressLevel * 100).toFixed(0)}%` : null,
      tone: stressLevel >= 0.7 ? 'negative' : stressLevel >= 0.4 ? 'warning' : 'positive',
      tooltip: '綜合波動、資金流與信用風險的壓力指數。'
    })
  ].join('');

}

function renderRecommendation(pipeline, stress) {
  const card = document.getElementById('home-rec-card');

  let action = '觀望';
  let reason = '目前資料不足以產生明確建議，請確認模擬已執行或查看市場頁面。';
  let confidence = 0;
  let tone = 'neutral';

  if (pipeline && typeof pipeline.score === 'number') {
    const score = pipeline.score;
    if (score >= 0.6) {
      action = '配置';
      tone = 'positive';
    } else if (score <= -0.6) {
      action = '減碼';
      tone = 'negative';
    } else {
      action = '觀望';
      tone = 'neutral';
    }
    confidence = Math.min(100, Math.max(0, Math.round(Math.abs(score) * 100)));
    reason = pipeline.rationale || `模型綜合評分 ${score.toFixed(2)}，對應建議為「${action}」。`;
  }

  card.innerHTML = `
    <div class="home-recommendation__action">
      <span class="home-recommendation__label">建議行動</span>
      <span class="home-recommendation__value home-recommendation__value--${tone}">${escapeHtml(action)}</span>
    </div>
    <div class="home-recommendation__confidence">
      <span class="home-recommendation__label">信心分數</span>
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
    const data = await silentGetJSON('/api/portfolio/current');
    if (!data || !data.positions || data.positions.length === 0) {
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
  const total = data.total_value || 0;
  const pnl = data.total_pnl || 0;
  const pnlPct = total > 0 ? (pnl / (total - pnl)) * 100 : 0;
  const hhi = data.hhi || 0;

  container.innerHTML = `
    <div class="home-portfolio-summary">
      <div class="home-portfolio-summary__item">
        <span class="home-portfolio-summary__label">總市值</span>
        <span class="home-portfolio-summary__value">${escapeHtml(fmtNTD(total))}</span>
      </div>
      <div class="home-portfolio-summary__item">
        <span class="home-portfolio-summary__label">損益</span>
        <span class="home-portfolio-summary__value ${pnl >= 0 ? 'positive' : 'negative'}">${fmtSignedPct(pnlPct)}</span>
      </div>
      <div class="home-portfolio-summary__item">
        <span class="home-portfolio-summary__label">集中度</span>
        <span class="home-portfolio-summary__value">${escapeHtml(fmtHHI(hhi))}</span>
      </div>
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
    <div class="home-portfolio-summary home-portfolio-summary--demo">
      <div class="home-portfolio-summary__item">
        <span class="home-portfolio-summary__label">示範總市值 <span class="badge demo">DEMO</span></span>
        <span class="home-portfolio-summary__value">${fmtNTD(totalValue)}</span>
      </div>
      <div class="home-portfolio-summary__item">
        <span class="home-portfolio-summary__label">損益</span>
        <span class="home-portfolio-summary__value ${totalPnl >= 0 ? 'positive' : 'negative'}">${fmtSignedPct(pnlPct)}</span>
      </div>
      <div class="home-portfolio-summary__item">
        <span class="home-portfolio-summary__label">持倉檔數</span>
        <span class="home-portfolio-summary__value">${positions.length}</span>
      </div>
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
