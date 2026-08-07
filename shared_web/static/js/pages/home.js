/**
 * Home page for retail investors.
 * Renders an editorial dashboard: market pulse, capital-flow radar,
 * event calendar, and trust elements.
 */

import { getJSON, silentGetJSON, getJSONWithTimeout, escapeHtml, renderMissingState } from '../shared/app-utils.js';
import { metricCard } from '../components/metric-card.js';
import { trustFooter } from '../components/trust-footer.js';
import { renderEventCalendar, gatherCalFilterOptions, EVENT_TYPE_LABELS } from '../components/event-calendar.js';
import {
  riskLevelLabel,
  fmtSafeSigned, fmtSafeNumber, fmtSafeSignedPct, fmtSafeLargeNumber,
} from '../shared/format-metric.js';
import { getThemeLabel } from '../shared/theme-labels.js';
import { displayZH as sectorLabel } from '../shared/sector-display.js';
import { initOnboarding } from '../components/onboarding.js';
import { scrollToSection } from '../shared/scroll-utils.js';
import { getDisclosureState, setDisclosureState } from '../shared/disclosure-state.js';
import { dataQualityBadge, buildChannelMap } from '../components/data-quality-badge.js';
import { renderSevenForceBoard } from '../components/seven-force-board.js';
import { renderSevenForceInterpretations } from '../components/seven-force-interpretations.js';

window.scrollToSection = scrollToSection;

const DASHBOARD_VERSION = 'v0.0.0.24';
const DATA_SOURCES = ['TWSE', 'Fugle', 'Replay 資料'];

const CHANNELS_BY_SECTION = {
  marketPulse: ['us_yahoo', 'us_spx', 'us_ndx', 'us_dji', 'sox_index', 'us_nvda', 'us_aapl', 'us_msft', 'tsm_adr', 'frankfurter_fx', 'twse_margin', 'taiex_index', 'tw_vol', 'export_statistics', 'twse_capital_flow'],
  predictions: ['twse_capital_flow', 'geopolitical', 'tsmc_revenue'],
  sevenForce: ['twse_capital_flow', 'frankfurter_fx', 'us_yahoo', 'tsm_adr'],
  calendar: ['twse_replay', 'us_yahoo', 'tsmc_revenue'],
};

let _homeChannelMap = {};

function renderDataBadges() {
  const set = (id, channelIds) => {
    const el = document.getElementById(id);
    if (!el) return;
    el.innerHTML = dataQualityBadge(_homeChannelMap, channelIds);
  };
  set('market-pulse-data-badge', CHANNELS_BY_SECTION.marketPulse);
  set('predictions-data-badge', CHANNELS_BY_SECTION.predictions);
  set('seven-force-data-badge', CHANNELS_BY_SECTION.sevenForce);
  set('calendar-data-badge', CHANNELS_BY_SECTION.calendar);
}


let homeLoaded = false;
let calActiveCategories = [];
let calActiveFilters = { triggerThemes: [], sectors: [] };
let calDateRange = { start: '', end: '' };
let lastCalendarEvents = [];

function prefersReducedMotion() {
  return window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

function animateValue(el, target, suffix = '', duration = 800) {
  if (target === null || target === undefined || Number.isNaN(target)) {
    el.textContent = '—';
    return;
  }
  const decimals = Number.isInteger(target) ? 0 : 1;
  if (prefersReducedMotion()) {
    el.textContent = fmtSafeNumber(target, { decimals, suffix });
    return;
  }
  const start = 0;
  const startTime = performance.now();
  function step(now) {
    const progress = Math.min((now - startTime) / duration, 1);
    const current = start + (target - start) * (progress * (2 - progress));
    el.textContent = fmtSafeNumber(current, { decimals, suffix });
    if (progress < 1) requestAnimationFrame(step);
  }
  requestAnimationFrame(step);
}

export async function renderHomePage(container) {
  container.innerHTML = `
    <section class="home-section" id="home-market-pulse">
      <div class="home-section__header">
        <h2>市場脈動</h2>
        <span class="home-section__subtitle">核心觀察指標</span>
        <span class="home-section__period-badge" id="home-period-badge" style="display:none"></span>
        <span class="home-section__data-badge" id="market-pulse-data-badge"></span>
      </div>
      <div class="home-grid home-grid--4" id="home-market-grid" data-disclosure-section="market-pulse" data-disclosure-state="collapsed">
        <div class="home-loading-card">載入中…</div>
      </div>
      <button class="disclosure-toggle" id="market-pulse-toggle" type="button" aria-expanded="false" aria-controls="home-market-grid" aria-label="展開 6 張進階指標">
        <span class="disclosure-toggle__label">展開進階指標</span>
        <span class="disclosure-toggle__icon disclosure-toggle__icon--down" aria-hidden="true"></span>
      </button>
      <span class="sr-only" id="market-pulse-status" aria-live="polite" aria-atomic="true"></span>
    </section>

    <section class="home-section" id="home-predictions">
      <div class="home-section__header">
        <h2>未來 5 日錢潮預測</h2>
        <span class="home-section__subtitle">事件驅動的資金流向預測</span>
        <span class="home-section__data-badge" id="predictions-data-badge"></span>
        <a class="home-section__nav-link" href="javascript:void(0)" onclick="switchPage('capital_predictions')">完整預測 →</a>
      </div>
      <div id="home-predictions-content" class="home-predictions__content">
        <div class="home-loading-card">載入中…</div>
      </div>
      <button class="disclosure-toggle" id="predictions-toggle" type="button" aria-expanded="false" aria-controls="home-predictions-content" aria-label="展開未來 5 日錢潮預測">
        <span class="disclosure-toggle__label">展開錢潮預測</span>
        <span class="disclosure-toggle__icon disclosure-toggle__icon--down" aria-hidden="true"></span>
      </button>
    </section>

    <section class="home-section" id="home-seven-force">
      <div class="home-section__header">
        <h2>七維錢潮雷達（3+2+2 分層）</h2>
        <span class="home-section__subtitle">官方法人 / 行為代理 / 領先＋跨市場訊號；語意見 <code>docs/specs/capital-flow-seven-dimension-spec.md</code> §4 D-CF-04</span>
        <span class="home-section__data-badge" id="seven-force-data-badge"></span>
        <a class="home-section__nav-link" href="javascript:void(0)" onclick="switchPage('capital_board')">完整看板 →</a>
      </div>
      <div id="home-seven-force-content">
        <div class="home-loading-card">載入中…</div>
      </div>
      <div id="home-seven-force-interpretations"></div>
    </section>

    <section class="home-section" id="home-event-calendar">
      <div class="home-section__header">
        <h2>市場行事曆（全年事件）</h2>
        <span class="home-section__subtitle">近期除權息、法說會、財報等重要事件</span>
        <span class="home-section__data-badge" id="calendar-data-badge"></span>
      </div>
      <div class="cal-filter-bar" id="cal-filter-bar">
        <button class="cal-filter-pill active" data-category="">全部</button>
        <button class="cal-filter-pill" data-category="除權息">除權息</button>
        <button class="cal-filter-pill" data-category="法說會/財報">法說會</button>
        <button class="cal-filter-pill" data-category="總經事件">總經</button>
        <select class="cal-filter-select" id="cal-filter-trigger-theme" data-filter-type="trigger_theme" aria-label="依事件類型篩選">
          <option value="">所有事件類型</option>
        </select>
        <select class="cal-filter-select" id="cal-filter-sector" data-filter-type="sector" aria-label="依產業篩選">
          <option value="">所有產業</option>
        </select>
        <label class="cal-filter-date">
          <span>開始</span>
          <input type="date" id="cal-filter-start" aria-label="開始日期">
        </label>
        <label class="cal-filter-date">
          <span>結束</span>
          <input type="date" id="cal-filter-end" aria-label="結束日期">
        </label>
      </div>
      <div id="home-calendar-content">
        <div class="home-loading-card">載入中…</div>
      </div>
    </section>

    <section class="home-section home-section--transparent" id="home-trust">
      <div id="home-trust-footer"></div>
    </section>
  `;

  const calContainer = document.getElementById('home-calendar-content');
  const filterBar = document.getElementById('cal-filter-bar');
  const applyCalFiltersAndRender = () => {
    if (!calContainer) return;
    renderEventCalendar(calContainer, calActiveCategories, calActiveFilters, calDateRange, lastCalendarEvents)
      .catch(err => console.warn('[home] calendar render failed:', err));
  };
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
      document.querySelectorAll('#cal-filter-bar .cal-filter-pill').forEach(p => {
        const c = p.dataset.category;
        p.classList.toggle('active', c === '' ? calActiveCategories.length === 0 : calActiveCategories.includes(c));
      });
      applyCalFiltersAndRender();
    });

    document.querySelectorAll('#cal-filter-bar .cal-filter-select').forEach(sel => {
      sel.addEventListener('change', () => {
        const key = sel.dataset.filterType === 'trigger_theme' ? 'triggerThemes' : 'sectors';
        const value = sel.value;
        calActiveFilters[key] = value ? [value] : [];
        applyCalFiltersAndRender();
      });
    });

    const startInput = document.getElementById('cal-filter-start');
    const endInput = document.getElementById('cal-filter-end');
    if (startInput) {
      startInput.addEventListener('change', () => {
        calDateRange.start = startInput.value;
        applyCalFiltersAndRender();
      });
    }
    if (endInput) {
      endInput.addEventListener('change', () => {
        calDateRange.end = endInput.value;
        applyCalFiltersAndRender();
      });
    }
  }

  bindPredictionsDisclosure();

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
      renderMarketPulse(m.macro, m.stress, null);
      renderPredictionsCard(m.narrative);
      renderSevenForceBoard(document.getElementById('home-seven-force-content'), m.portfolio);
      renderSevenForceInterpretations(document.getElementById('home-seven-force-interpretations'), m.portfolio);
      const calContainer = document.getElementById('home-calendar-content');
      renderEventCalendar(calContainer, calActiveCategories, calActiveFilters, calDateRange, m.narrative);

      renderTrustFooter(['Fugle', 'TWSE', 'FRED']);
      return;
    }

    try {
      const [health, macro, stress, pipeline, bundle, calData, predictionData, capitalFlowSummary] = await Promise.all([
        getJSONWithTimeout('/api/dashboard/system-health', 5000),
        getJSONWithTimeout('/api/macro/snapshot/latest', 5000),
        getJSONWithTimeout('/api/taiwan/stress-index', 5000),
        getJSONWithTimeout('/api/dashboard/recommendation-pipeline', 5000),
        getJSONWithTimeout('/api/narrative/bundle', 5000),
        getJSONWithTimeout('/api/dashboard/calendar-events', 5000),
        getJSONWithTimeout('/api/events/prediction', 5000),
        getJSONWithTimeout('/api/capital-flow/summary', 5000),
      ]);

      // 市場時期 badge（/api/regime/history 無 tier gate，所有人可見）
      try {
        const regimeData = await silentGetJSON('/api/regime/history');
        const sessions = regimeData && (Array.isArray(regimeData) ? regimeData
          : (regimeData.sessions || regimeData.Sessions || null));
        const periodBadge = document.getElementById('home-period-badge');
        if (periodBadge && sessions && sessions.length > 0) {
          const latest = sessions[sessions.length - 1];
          const mp = latest.market_period;
          if (mp) {
            const PERIOD_LABEL = {
              downturn: { zh: '低迷' }, turnaround_up: { zh: '轉折開高' },
              bull: { zh: '上升' }, plateau: { zh: '高原' },
              consolidation: { zh: '盤整' }, turnaround_down: { zh: '轉折下壓' },
              black_swan: { zh: '黑天鵝' },
            };
            const zh = (PERIOD_LABEL[mp] && PERIOD_LABEL[mp].zh) || mp;
            periodBadge.style.display = '';
            periodBadge.innerHTML = `<a class="home-period-chip" data-period="${escapeHtml(mp)}" href="javascript:void(0)" onclick="switchPage('methodology')" title="目前市場時期：${escapeHtml(zh)}，點擊查看方法論">時期：${escapeHtml(zh)} →</a>`;
          }
        }
      } catch (_) { /* 不影響首頁主流程 */ }

      const events = bundle && bundle.events ? bundle.events : [];
      _homeChannelMap = buildChannelMap(health && Array.isArray(health.data_channels) ? health.data_channels : null);
      renderDataBadges();

      renderMarketPulse(macro, stress);
      renderPredictionsCard(predictionData);
      renderSevenForceBoard(document.getElementById('home-seven-force-content'), capitalFlowSummary);
      renderSevenForceInterpretations(document.getElementById('home-seven-force-interpretations'), capitalFlowSummary);

      // Event calendar — fetched once and shared with banner + filters
      const calContainer = document.getElementById('home-calendar-content');
      lastCalendarEvents = calData && Array.isArray(calData.events) ? calData.events : [];
      populateCalFilterSelects(lastCalendarEvents);
      if (calContainer) {
        renderEventCalendar(calContainer, calActiveCategories, calActiveFilters, calDateRange, lastCalendarEvents).catch(err =>
          console.warn('[home] calendar render failed:', err));
      }

      // Portfolio snapshot / banner removed from home per UI refresh request.
    } catch (err) {
      console.warn('[home] failed to load dashboard data:', err);
      renderMarketPulse(null, null);
      renderPredictionsCard(null);
      renderSevenForceBoard(document.getElementById('home-seven-force-content'), null);
      renderSevenForceInterpretations(document.getElementById('home-seven-force-interpretations'), null);
    }

    // Portfolio snapshot is loaded independently; it may be empty/demo.
    // Removed from home per UI refresh request.

  } catch (err) {
    console.error('[home] unexpected error loading home data:', err);
  } finally {
    renderTrustFooter();
  }
}

function collectEventTypes(events) {
  const set = new Set();
  for (const evt of events) {
    if (evt.event_type) set.add(evt.event_type);
  }
  return Array.from(set).sort();
}

function populateCalFilterSelects(events) {
  const opts = gatherCalFilterOptions(events);
  const themeSel = document.getElementById('cal-filter-trigger-theme');
  const sectorSel = document.getElementById('cal-filter-sector');
  const eventTypes = collectEventTypes(events);
  if (themeSel) {
    themeSel.innerHTML = '<option value="">所有事件類型</option>' +
      eventTypes.map(t => `<option value="${escapeHtml(t)}">${escapeHtml(EVENT_TYPE_LABELS[t] || t)}</option>`).join('');
  }
  if (sectorSel && opts.sectors.length) {
    sectorSel.innerHTML = '<option value="">所有產業</option>' +
      opts.sectors.map(s => `<option value="${escapeHtml(s)}">${escapeHtml(sectorLabel(s))}</option>`).join('');
  }
}

function predictionDirectionLabel(dir) {
  if (dir === 'inflow') return '資金流入';
  if (dir === 'outflow') return '資金流出';
  return '中性觀望';
}

function predictionDirectionClass(dir) {
  if (dir === 'inflow') return 'positive';
  if (dir === 'outflow') return 'negative';
  return 'neutral';
}

function fmtPredictionDate(d) {
  if (!d) return '—';
  const date = new Date(d);
  if (Number.isNaN(date.getTime())) return '—';
  return `${date.getMonth() + 1}/${date.getDate()}`;
}

/**
 * 渲染單一預測列的「方向機率分佈」3-bar stack + 3 個 label。
 * 對應 C03 修復後新增的 inflow / neutral / outflow 三段進度條。
 *
 * @param {{inflow?: number, neutral?: number, outflow?: number}|null|undefined} distribution
 * @returns {string} 內含 pred-row__dist + pred-row__dist-label 兩個區塊的 HTML 字串
 */
export function renderDistributionSegments(distribution) {
  const dist = distribution && typeof distribution === 'object' ? distribution : {};
  const inflow = typeof dist.inflow === 'number' && dist.inflow >= 0 ? dist.inflow : 0;
  const neutral = typeof dist.neutral === 'number' && dist.neutral >= 0 ? dist.neutral : 0;
  const outflow = typeof dist.outflow === 'number' && dist.outflow >= 0 ? dist.outflow : 0;
  const inflowPct = Math.round(inflow * 100);
  const neutralPct = Math.round(neutral * 100);
  const outflowPct = Math.round(outflow * 100);
  return `
        <div class="pred-row__dist" aria-hidden="true">
          <div class="pred-row__dist-segment pred-row__dist-segment--inflow" style="width:${inflowPct}%"></div>
          <div class="pred-row__dist-segment pred-row__dist-segment--neutral" style="width:${neutralPct}%"></div>
          <div class="pred-row__dist-segment pred-row__dist-segment--outflow" style="width:${outflowPct}%"></div>
        </div>
        <div class="pred-row__dist-label">
          <span>流入 ${inflowPct}%</span>
          <span>觀望 ${neutralPct}%</span>
          <span>流出 ${outflowPct}%</span>
        </div>`;
}

function renderPredictionsCard(data) {
  const container = document.getElementById('home-predictions-content');
  if (!container) return;

  const predictions = data && Array.isArray(data.predictions) ? data.predictions : [];
  if (!predictions.length) {
    container.innerHTML = renderMissingState('未來 5 日錢潮預測', 'no-data');
    return;
  }

  const rows = predictions.slice(0, 5).map((p, idx) => {
    const conf = typeof p.confidence === 'number' ? p.confidence : 0;
    const dir = p.direction || 'neutral';
    const width = Math.round(Math.min(1, Math.max(0, conf)) * 100);
    const drivers = Array.isArray(p.driving_events) ? p.driving_events : [];
    const driverText = drivers.length ? drivers.slice(0, 2).map(e => escapeHtml(e)).join('、') : '無顯著事件';
    const dist = p.distribution && typeof p.distribution === 'object' ? p.distribution : {};
    return `
      <div class="pred-row" data-index="${idx}">
        <div class="pred-row__meta">
          <span class="pred-row__date">${escapeHtml(fmtPredictionDate(p.date))}</span>
          <span class="pred-row__dir ${predictionDirectionClass(dir)}">${escapeHtml(predictionDirectionLabel(dir))}</span>
          <span class="pred-row__conf">${width}%</span>
        </div>
        <div class="pred-row__bar" aria-hidden="true">
          <div class="pred-row__bar-fill pred-row__bar-fill--${dir}" style="width:${width}%"></div>
        </div>
        ${renderDistributionSegments(dist)}
        <div class="pred-row__drivers">${driverText}</div>
      </div>
    `;
  }).join('');

  // 誠實聲明 (product positioning §9): 預測是機率性陳述, 呈現時附歷史命中率
  // 而非確定承諾。樣本不足時顯示「校準中」, 不顯示誤導的百分比。
  const hitRateBadge = renderHitRateBadge(data && data.historical_hit_rate);

  container.innerHTML = `<div class="pred-card">${hitRateBadge}${rows}</div>`;
}

/**
 * 渲染歷史命中率徽章。輸入為 /api/events/prediction 的 historical_hit_rate
 * 欄位 (可能為 null):
 *   - null / undefined → 無 store (不顯示, 保持舊 UI)
 *   - calibrated=true   → 「過去 N 天命中率 X% (H/T)」
 *   - calibrated=false  → 「校準中 (樣本 S/30)」 — 對齊 §6 校準語意
 */
function renderHitRateBadge(hhr) {
  if (!hhr || typeof hhr !== 'object') return '';
  const samples = typeof hhr.samples === 'number' ? hhr.samples : 0;
  const hits = typeof hhr.hits === 'number' ? hhr.hits : 0;
  if (samples === 0) {
    return `<div class="pred-hitrate pred-hitrate--calibrating" role="status">校準中（樣本 0/30）</div>`;
  }
  const pct = typeof hhr.hit_rate === 'number' ? Math.round(hhr.hit_rate * 100) : 0;
  if (hhr.calibrated === true) {
    return `<div class="pred-hitrate pred-hitrate--calibrated" role="status">過去 ${hhr.window_days || 60} 天命中率 ${pct}%（${hits}/${samples}）</div>`;
  }
  return `<div class="pred-hitrate pred-hitrate--calibrating" role="status">校準中（樣本 ${samples}/30）</div>`;
}

function isValidMacroPoint(v) {
  return v && typeof v === 'object' && v.symbol;
}

function pointValue(obj, key) {
  if (!obj) return null;
  const v = obj[key];
  if (v === null || v === undefined) return null;
  if (typeof v === 'object') {
    if (!isValidMacroPoint(v)) return null;
    const n = Number(v.value);
    return Number.isNaN(n) ? null : n;
  }
  const n = Number(v);
  return Number.isNaN(n) ? null : n;
}

function pointChange(obj, key) {
  if (!obj || !obj[key] || typeof obj[key] !== 'object') return null;
  const pt = obj[key];
  if (!isValidMacroPoint(pt)) return null;
  const n = Number(pt.change_pct);
  return Number.isNaN(n) ? null : n;
}

// Format annualized volatility (decimal, e.g. 0.18) as percentage (e.g. "18.0%").
function formatVolatility(val) {
  return fmtSafeNumber(val, { decimals: 1, suffix: '%', percent: true });
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

  // Foreign investor — values are absolute net buy/sell in 億元, not ratios.
  const foreign = pointValue(macro, 'foreign_investor_net');
  const foreignText = fmtSafeSigned(foreign, { decimals: 1, suffix: ' 億', forceSign: true });
  const fundVal = pointValue(macro, 'domestic_fund_net');
  const fundText = fmtSafeSigned(fundVal, { decimals: 1, suffix: ' 億', forceSign: true });
  const dealerVal = pointValue(macro, 'dealer_net');
  const dealerText = fmtSafeSigned(dealerVal, { decimals: 1, suffix: ' 億', forceSign: true });

  // Stress
  const stressScore = pointValue(stress, 'score');
  const stressRisk = stressScore === null ? null : stressScore >= 70 ? 'high' : stressScore >= 40 ? 'medium' : 'low';
  const stressLabel = stressRisk === null ? '—' : riskLevelLabel(stressRisk);

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
  const retailText = retailChange !== null && !Number.isNaN(retailChange)
    ? (retailChange >= 0 ? '偏多 ' : '偏空 ') + fmtSafeSignedPct(retailChange, 0)
    : '—';

  const cards = [
    metricCard({ id: 'market-taiwan', label: '大盤', value: trend.value, delta: taiexChange !== null ? fmtSafeSignedPct(taiexChange) : null, tone: trend.tone, tooltip: '加權指數近期漲跌幅。', extraClasses: 'card-priority-high disclosure-tier-core' }),
    metricCard({ id: 'market-foreign', label: '外資', value: foreignText, tone: foreign > 0 ? 'positive' : foreign < 0 ? 'negative' : 'neutral', tooltip: '外資近一交易日淨買賣超（億元）。', extraClasses: 'card-priority-high disclosure-tier-core' }),
    metricCard({ id: 'market-tsm', label: 'TSM ADR', value: fmtSafeSignedPct(tsmChange), tone: tsmChange === null ? 'neutral' : tsmChange >= 0 ? 'positive' : 'negative', tooltip: '台積電 ADR 漲跌幅，領先台股現貨。', extraClasses: 'card-priority-high disclosure-tier-core' }),
    metricCard({ id: 'market-sox', label: 'SOX 半導體', value: fmtSafeSignedPct(soxChange), tone: soxChange === null ? 'neutral' : soxChange >= 0 ? 'positive' : 'negative', tooltip: '費城半導體指數，台股科技股先行指標。', extraClasses: 'card-priority-medium disclosure-tier-core' }),
    metricCard({ id: 'market-nasdaq', label: 'NASDAQ', value: fmtSafeSignedPct(ndxChange), tone: ndxChange === null ? 'neutral' : ndxChange >= 0 ? 'positive' : 'negative', tooltip: '那斯達克指數漲跌幅。', extraClasses: 'card-priority-medium disclosure-tier-advanced' }),
    metricCard({ id: 'market-usdtwd', label: 'USD/TWD', value: fmtSafeNumber(usdtwd, { decimals: 2 }), tone: 'neutral', tooltip: '美元兌台幣匯率，影響外資進出意願。', extraClasses: 'card-priority-medium disclosure-tier-core' }),
    metricCard({ label: 'VIX', value: fmtSafeNumber(vixVal, { decimals: 1 }), tone: vixVal === null ? 'neutral' : vixVal >= 25 ? 'negative' : vixVal >= 20 ? 'warning' : 'positive', tooltip: '恐慌指數，>20 風險升高、>25 警戒。', extraClasses: 'card-priority-low disclosure-tier-advanced' }),
    metricCard({ label: '融資餘額', value: marginVal !== null && !Number.isNaN(marginVal) ? `${fmtSafeLargeNumber(marginVal)} 億` : '—', tone: 'neutral', tooltip: '散戶融資餘額（億元），反映市場熱度。', extraClasses: 'card-priority-low disclosure-tier-advanced' }),
    metricCard({ label: '投信動向', value: fundText, tone: fundVal > 0 ? 'positive' : fundVal < 0 ? 'negative' : 'neutral', tooltip: '投信近一交易日買賣超（億元）。', extraClasses: 'card-priority-low disclosure-tier-advanced' }),
    metricCard({ label: '自營商', value: dealerText, tone: dealerVal > 0 ? 'positive' : dealerVal < 0 ? 'negative' : 'neutral', tooltip: '自營商近一交易日買賣超（億元）。', extraClasses: 'card-priority-low disclosure-tier-advanced' }),
    metricCard({ label: '歷史波動', value: formatVolatility(pointValue(macro, 'historical_volatility')), tone: volatilityTone(pointValue(macro, 'historical_volatility')), tooltip: 'TAIEX 20 日年化波動率。<20% 低波動、20-30% 中等、>30% 高波動警戒。', extraClasses: 'card-priority-low disclosure-tier-advanced' }),
    metricCard({ label: '散戶情緒', value: retailText, tone: retailChange === null ? 'neutral' : retailChange >= 0 ? 'positive' : 'negative', tooltip: '散戶融資餘額變化 — 偏多表示融資增加（槓桿意願高），偏空表示融資減少。', extraClasses: 'card-priority-low disclosure-tier-advanced' }),
  ];

  // Progressive disclosure lazy-render: collapsed state only emits the 5
  // core cards; the 7 advanced cards are appended on first expand.
  const advancedCards = cards.filter(c => c.includes('disclosure-tier-advanced'));
  const coreCards = cards.filter(c => c.includes('disclosure-tier-core'));

  if (grid) {
    const initialState = getDisclosureState('market-pulse', 'expanded');
    grid.setAttribute('data-disclosure-state', initialState);
    // TODO(api-lazy-fetch): collapsed 狀態目前仍計算全部 12 張 metricCard 物件,
    // 只是不寫入 DOM;真正的 API lazy-fetch 待 /api/macro/snapshot 支援 fields
    // 過濾後,改為 collapsed 只 evaluate coreCards、展開時再 evaluate advanced。
    grid.innerHTML = (initialState === 'expanded'
      ? coreCards.concat(advancedCards)
      : coreCards
    ).join('');
    bindMarketPulseDisclosure(advancedCards.join(''));
  }
}

let _marketPulseDisclosureBound = false;

function bindMarketPulseDisclosure(advancedCardsHTML) {
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
      if (iconEl) { iconEl.classList.remove('disclosure-toggle__icon--down'); iconEl.classList.add('disclosure-toggle__icon--up'); }
      btn.setAttribute('aria-label', '收合 7 張進階指標');
      if (statusEl) statusEl.textContent = '已展開 7 張進階指標';
    } else {
      if (labelEl) labelEl.textContent = '展開進階指標';
      if (iconEl) { iconEl.classList.remove('disclosure-toggle__icon--up'); iconEl.classList.add('disclosure-toggle__icon--down'); }
      btn.setAttribute('aria-label', '展開 7 張進階指標');
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
    if (next === 'expanded') {
      if (!grid.querySelector('.disclosure-tier-advanced') && advancedCardsHTML) {
        grid.insertAdjacentHTML('beforeend', advancedCardsHTML);
      }
    } else {
      grid.querySelectorAll('.disclosure-tier-advanced').forEach(el => el.remove());
    }
  });
}

let _predictionsDisclosureBound = false;

function bindPredictionsDisclosure() {
  if (_predictionsDisclosureBound) return;
  const btn = document.getElementById('predictions-toggle');
  const content = document.getElementById('home-predictions-content');
  if (!btn || !content) return;
  _predictionsDisclosureBound = true;

  content.setAttribute('data-disclosure-state', 'expanded');

  const updateButton = (state) => {
    btn.setAttribute('aria-expanded', state === 'expanded' ? 'true' : 'false');
    const labelEl = btn.querySelector('.disclosure-toggle__label');
    const iconEl = btn.querySelector('.disclosure-toggle__icon');
    if (state === 'expanded') {
      if (labelEl) labelEl.textContent = '收合錢潮預測';
      if (iconEl) { iconEl.classList.remove('disclosure-toggle__icon--down'); iconEl.classList.add('disclosure-toggle__icon--up'); }
      btn.setAttribute('aria-label', '收合未來 5 日錢潮預測');
    } else {
      if (labelEl) labelEl.textContent = '展開錢潮預測';
      if (iconEl) { iconEl.classList.remove('disclosure-toggle__icon--up'); iconEl.classList.add('disclosure-toggle__icon--down'); }
      btn.setAttribute('aria-label', '展開未來 5 日錢潮預測');
    }
  };

  updateButton('expanded');

  btn.addEventListener('click', () => {
    const current = content.getAttribute('data-disclosure-state') || 'collapsed';
    const next = current === 'expanded' ? 'collapsed' : 'expanded';
    content.setAttribute('data-disclosure-state', next);
    updateButton(next);
  });
}

function explainFromEvent(e) {
  if (e.description && typeof e.description === 'string' && e.description.trim()) {
    return truncate(e.description.trim(), 50);
  }
  const label = getThemeLabel(e.theme);
  const bullish = typeof e.sentiment === 'number' ? e.sentiment >= 0 : true;
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
