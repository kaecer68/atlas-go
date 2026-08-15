/**
 * @typedef {import('./shared/field_types.ts').GuardOutcome} GuardOutcome
 * @typedef {import('./shared/field_types.ts').RecommendationOutcome} RecommendationOutcome
 * @typedef {import('./shared/field_types.ts').RiskSnapshot} RiskSnapshot
 * @typedef {import('./shared/field_types.ts').SessionSummary} SessionSummary
 * @typedef {import('./shared/field_types.ts').Scorecard} Scorecard
 */

import { eventSource } from './services/event-source.js';
import { renderLiveProgress } from './components/live-progress.js';
import { renderToolEvents } from './components/tool-events.js';
import { fmtNTD } from './shared/utils.js';
import { getJSON, silentGetJSON, escapeHtml, parseSessionsList, renderMissingState } from './shared/app-utils.js';
import { injectSharedHead } from './shared/head-config.js';
import { install401Interceptor } from './shared/fetch-wrapper.js';
import { initAuth, invalidateAuth } from './services/auth.js';
injectSharedHead();

export { getJSON, escapeHtml };

const pageLoadStatus = {};
const APP_VERSION = '20260512';

const basePath = (typeof window !== 'undefined')
  ? window.location.pathname.replace(/\/[^/]*$/, '') || ''
  : '';

export function switchPage(id, silent) {
  var pageEl = document.getElementById('page-' + id);
  if (!pageEl) { console.warn('[switchPage] page not found:', id); return; }
  // Skip redundant work only when the page is already active AND its data was
  // already loaded. Without the pageLoadStatus guard, the initial switchPage()
  // for the server-rendered active page (page-home) would early-return here and
  // never run loadPageData(id) — leaving the home scheduler status panel stuck
  // on its static "載入中…" state (the /api/scheduler/status fetch never fired).
  if (pageEl.classList.contains('active') && pageLoadStatus[id]) return;
  document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
  document.getElementById('page-' + id).classList.add('active');
  document.querySelectorAll('#sidebar nav a').forEach(a => a.classList.remove('active'));
  const btn = document.querySelector('#sidebar nav a[data-page="' + id + '"]');
  if (btn) btn.classList.add('active');
  const titles = {
  home: '系統總覽', live: '風控營運台', alerts: '系統警報',
  experiments: '模擬交易',
  datachannels: '資料通道', parameters: '參數管理',
  reports: '最新回測',
  capital_models: '錢潮模型', capital_causality: '錢潮因果', capital_quality: '資料品質',
  metrics: '指標監控', config: '部署配置'
};
  document.getElementById('pageTitle').textContent = titles[id] || id;
  document.getElementById('sidebar').classList.remove('open');
  if (!pageLoadStatus[id]) { pageLoadStatus[id] = true; loadPageData(id); }
  if (!silent) history.pushState({page: id}, '', basePath + '/' + id);
}

export function toggleSidebar() {
  document.getElementById('sidebar').classList.toggle('open');
}

// --- Auto-refresh ---
let autoRefreshEnabled = true, autoRefreshTimer = null, consecutiveFailures = 0;
const MAX_CONSECUTIVE_FAILURES = 3;

function startAutoRefresh() { if (!autoRefreshTimer) autoRefreshTimer = setInterval(loadAll, 30000); }
function stopAutoRefresh() { clearInterval(autoRefreshTimer); autoRefreshTimer = null; }

function showErrorBanner() {
  let b = document.getElementById('errorBanner');
  if (!b) { b = document.createElement('div'); b.id = 'errorBanner'; b.className = 'error-banner'; (document.getElementById('content') || document.body).insertBefore(b, document.getElementById('content').firstChild); }
  b.innerHTML = '<span>⚠ 連續 ' + consecutiveFailures + ' 次更新失敗</span><button class="retry-btn" onclick="retryLoad()">重試</button>';
  b.style.display = 'flex';
}

function hideErrorBanner() { var b = document.getElementById('errorBanner'); if (b) b.style.display = 'none'; }

function retryLoad() { consecutiveFailures = 0; hideErrorBanner(); loadAll(); }

// --- Rendering helpers ---
function renderEmptyState(msg, hint) {
  return '<div style="padding:20px;text-align:center;color:var(--muted)">' + (msg || '尚無資料') + (hint ? '<br><small>' + hint + '</small>' : '') + '</div>';
}

function renderSkeleton(lines) {
  return Array(lines || 4).fill('<div class="skeleton-line"></div>').join('');
}

function showSkeletons() {
  document.querySelectorAll('.skeleton-container').forEach(function(el) { el.innerHTML = renderSkeleton(4); });
}

// --- Per-panel loading / error helpers --------------------------------------
// renderMissingState is imported from shared_web/app-utils.js.
function panelEl(id) { return id ? document.getElementById(id) : null; }

function setPanelLoading(id, label) {
  const el = panelEl(id);
  if (!el) return;
  el.classList.add('loading');
  el.innerHTML = renderMissingState(label, 'loading');
}

function setPanelError(id, label) {
  const el = panelEl(id);
  if (!el) return;
  el.classList.remove('loading');
  el.innerHTML = renderMissingState(label, 'api-error');
}

function setPanelNoData(id, label) {
  const el = panelEl(id);
  if (!el) return;
  el.classList.remove('loading');
  el.innerHTML = renderMissingState(label, 'no-data');
}

function setPanelStale(id, label) {
  const el = panelEl(id);
  if (!el) return;
  el.classList.remove('loading');
  el.innerHTML = renderMissingState(label, 'stale-data');
}

function clearPanelLoading(id) {
  const el = panelEl(id);
  if (!el) return;
  el.classList.remove('loading');
}

// --- Fetch with timeout, retry and per-panel state --------------------------
const DEFAULT_TIMEOUT_MS = 10000;
const MAX_RETRIES = 3;
const BACKOFF_BASE_MS = 800;

function isTransientError(err) {
  if (!err) return false;
  if (err.name === 'AbortError' || err.name === 'TypeError') return true;
  const msg = err.message || '';
  if (/timeout|network|abort|failed to fetch/i.test(msg)) return true;
  const status = err.status || (err.response && err.response.status);
  if (status >= 500 || status === 429) return true;
  return false;
}

async function fetchWithTimeout(url, ms) {
  ms = ms || DEFAULT_TIMEOUT_MS;
  const controller = new AbortController();
  const timer = setTimeout(function() { controller.abort(); }, ms);
  try {
    const res = await fetch(url, { credentials: 'include', signal: controller.signal });
    if (!res.ok) {
      const err = new Error(url + ': ' + res.status);
      err.status = res.status;
      throw err;
    }
    return await res.json();
  } finally {
    clearTimeout(timer);
  }
}

async function fetchWithRetry(url, opts) {
  const options = opts || {};
  const timeoutMs = options.timeoutMs || DEFAULT_TIMEOUT_MS;
  const retries = options.retries != null ? options.retries : MAX_RETRIES;
  const label = options.label;
  const panelId = options.panelId;
  if (panelId) setPanelLoading(panelId, label);
  let lastErr;
  for (let attempt = 0; attempt <= retries; attempt++) {
    try {
      const data = await fetchWithTimeout(url, timeoutMs);
      if (panelId) clearPanelLoading(panelId);
      return data;
    } catch (err) {
      lastErr = err;
      if (attempt >= retries || !isTransientError(err)) break;
      const delay = BACKOFF_BASE_MS * Math.pow(2, attempt) + Math.random() * 500;
      await new Promise(function(resolve) { setTimeout(resolve, delay); });
    }
  }
  console.warn('[fetchWithRetry] failed', url, lastErr);
  if (panelId) setPanelError(panelId, label);
  return null;
}

// Backwards-compatible helper used by lazy page loaders.
function getJSONWithTimeout(url, ms) {
  return fetchWithTimeout(url, ms || DEFAULT_TIMEOUT_MS).catch(function() { return null; });
}

// --- Module Registry (all modules loaded once) ---
var modules = {};

async function loadModules() {
  if (modules._loaded) return modules;
  var imports = [
    import('./pages/dashboard.js'),
    import('./pages/risk.js'),
    import('./pages/narrative.js'),
    import('./pages/backtest.js'),
    import('./pages/inbox.js'),
    import('./pages/experiments.js'),
    import('./pages/alerts.js'),
    import('./pages/metrics.js'),
    import('./pages/datachannels.js'),
    import('./pages/parameters.js'),
    import('./pages/deploy-config.js'),
    import('./pages/capital-models.js'),
    import('./pages/capital-causality.js'),
    import('./pages/capital-quality.js'),
  ];
  var results = await Promise.allSettled(imports);
  var keys = ['dash', 'risk', 'narr', 'back', 'inbox', 'experiments', 'alerts', 'metrics', 'datachannels', 'parameters', 'deployConfig', 'capitalModels', 'capitalCausality', 'capitalQuality'];
  results.forEach(function(r, i) {
    modules[keys[i]] = r.status === 'fulfilled' ? r.value : {};
  });
  modules._loaded = true;
  if (modules.experiments) {
    if (modules.experiments.openInfoHelp) window.openInfoHelp = modules.experiments.openInfoHelp;
    if (modules.experiments.closeInfoModal) window.closeInfoModal = modules.experiments.closeInfoModal;
    if (modules.experiments.openKpiHelp) window.openKpiHelp = modules.experiments.openKpiHelp;
    if (modules.experiments.closeModal) window.closeModal = modules.experiments.closeModal;
    if (modules.experiments.closePromoteModal) window.closePromoteModal = modules.experiments.closePromoteModal;
    if (modules.experiments.confirmPromote) window.confirmPromote = modules.experiments.confirmPromote;
    if (modules.experiments.promoteExperiment) window.promoteExperiment = modules.experiments.promoteExperiment;
    if (modules.experiments.revertExperiment) window.revertExperiment = modules.experiments.revertExperiment;
    if (modules.experiments.pauseAgent) window.pauseAgent = modules.experiments.pauseAgent;
    if (modules.experiments.resumeAgent) window.resumeAgent = modules.experiments.resumeAgent;
    if (modules.experiments.banSector) window.banSector = modules.experiments.banSector;
    if (modules.experiments.unbanSector) window.unbanSector = modules.experiments.unbanSector;
  }
  if (modules.pipe) {
    if (modules.pipe.toggleFilterPanel) window.toggleFilterPanel = modules.pipe.toggleFilterPanel;
    if (modules.pipe.applyFilters) window.applyFilters = modules.pipe.applyFilters;
    if (modules.pipe.clearFilters) window.clearFilters = modules.pipe.clearFilters;
    if (modules.pipe.toggleWorkflowScreening) window.toggleWorkflowScreening = modules.pipe.toggleWorkflowScreening;
  }
  if (modules.back) {
    if (modules.back.runBacktest) window.runBacktest = modules.back.runBacktest;
  }
  if (modules.alerts) {
    if (modules.alerts.loadAlerts) window.loadAlerts = modules.alerts.loadAlerts;
    if (modules.alerts.acknowledgeAlert) window.acknowledgeAlert = modules.alerts.acknowledgeAlert;
    if (modules.alerts.showUnacknowledgedOnly) window.showUnacknowledgedOnly = modules.alerts.showUnacknowledgedOnly;
  }
  if (modules.datachannels) {
    if (modules.datachannels.triggerChannelsIngest) window.triggerChannelsIngest = modules.datachannels.triggerChannelsIngest;
    if (modules.datachannels.enableAllChannels) window.dcEnableAll = modules.datachannels.enableAllChannels;
    if (modules.datachannels.disableAllChannels) window.dcDisableAll = modules.datachannels.disableAllChannels;
    if (modules.datachannels.triggerChannelFetch) window.triggerChannelFetch = modules.datachannels.triggerChannelFetch;
    if (modules.datachannels.toggleChannel) window.toggleChannel = modules.datachannels.toggleChannel;
    if (modules.datachannels.updateApiKey) window.dcUpdateApiKey = modules.datachannels.updateApiKey;
    if (modules.datachannels.loadFetchLogs) window.loadFetchLogs = modules.datachannels.loadFetchLogs;
    if (modules.datachannels.loadDataChannels) window.loadDataChannels = modules.datachannels.loadDataChannels;
    if (modules.datachannels.refreshChannelStatus) window.refreshChannelStatus = modules.datachannels.refreshChannelStatus;
  }
  return modules;
}
// Expose the module-load promise so click handlers installed at DOMContentLoaded
// (admin_web/static/js/event-listeners.js) can `await window.__modulesReady` on
// first click — closes the race window where module imports haven't finished
// and `window.dcEnableAll` etc. are still undefined. The `modules._loaded` guard
// inside loadModules makes subsequent calls a no-op.
window.__modulesReady = loadModules();
// --- Main Data Loader -------------------------------------------------------
// Core endpoints block the initial dashboard render; non-core endpoints update
// their panels independently in the background so a single slow API cannot
// freeze the whole page.

async function fetchCore() {
  setPanelLoading('overviewMarket', '市場環境');
  setPanelLoading('overviewRisk', '風險信號');
  setPanelLoading('overviewSystem', '系統狀態');
  setPanelLoading('sessionSyncAlert', '場次同步');
  setPanelLoading('liveNarrativeStrip', '總經敘事');

  const results = await Promise.all([
    fetchWithRetry('/api/dashboard/system-health', { label: '系統狀態' }),
    fetchWithRetry('/api/dashboard/agent-observatory', { label: 'Agent 觀測' }),
    fetchWithRetry('/api/dashboard/experiment-inbox', { label: '實驗收件匣' }),
    fetchWithRetry('/api/taiwan/stress-index', { label: '壓力指數' }),
    fetchWithRetry('/api/narrative/bundle', { label: '敘事 bundle' }),
    fetchWithRetry('/api/dashboard/data-channels', { label: '資料通道' }),
    fetchWithRetry('/api/dashboard/sessions', { label: '場次同步' }),
    fetchWithRetry('/api/dashboard/capital-phase', { label: '資金階段' }),
  ]);
  const [health, agents, inbox, stress, bundle, dataChannels, sessions, capitalPhase] = results;
  const failures = results.filter(function(v) { return v === null; }).length;

  return { health, agents, inbox, stress, bundle, dataChannels, sessions, capitalPhase, failures, total: results.length };
}

function renderCore(m, core) {
  const health = core.health, agents = core.agents, inbox = core.inbox,
        stress = core.stress, bundle = core.bundle, dataChannels = core.dataChannels,
        sessions = core.sessions, capitalPhase = core.capitalPhase;

  const parsed = parseSessionsList(sessions);
  window.pipelineSessions = parsed.sessions;
  window.pipelineSessionsStatus = parsed.data_status;

  if (health === null) {
    setPanelError('overviewMarket', '市場環境');
    setPanelError('overviewRisk', '風險信號');
    setPanelError('overviewSystem', '系統狀態');
  } else {
    const events = bundle && bundle.events ? { events: bundle.events } : null;
    if (m.dash.renderOverview) {
      m.dash.renderOverview(health, agents, inbox, null, events, stress, dataChannels, capitalPhase);
    }
    clearPanelLoading('overviewMarket');
    clearPanelLoading('overviewRisk');
    clearPanelLoading('overviewSystem');
  }

  if (sessions === null) {
    setPanelError('sessionSyncAlert', '場次同步');
  } else {
    clearPanelLoading('sessionSyncAlert');
  }

  const stripEvents = bundle && bundle.events ? { events: bundle.events } : null;
  const stripModels = bundle && bundle.models ? { models: bundle.models } : null;
  const stripChains = bundle && bundle.chains ? { chains: bundle.chains } : null;
  if (m.narr.renderLiveNarrativeStrip) {
    m.narr.renderLiveNarrativeStrip(stripEvents, stress, stripModels, stripChains);
  }
  clearPanelLoading('liveNarrativeStrip');

  if (m.inbox.renderInbox) m.inbox.renderInbox(inbox);
  if (m.datachannels.renderDataChannels) m.datachannels.renderDataChannels(dataChannels);
}

async function fetchNonCore(m, core) {
  setPanelLoading('macroRadar', '總經雷達');
  setPanelLoading('liveStatus', '即時狀態');
  setPanelLoading('riskCards', '風險指標');
  setPanelLoading('alertsPanel', '系統警報');

  const results = await Promise.all([
    fetchWithRetry('/api/dashboard/macro-radar', { label: '總經雷達' }),
    fetchWithRetry('/api/dashboard/recommendation-pipeline', { label: '推薦管線' }),
    fetchWithRetry('/api/dashboard/live-status', { label: '即時狀態' }),
    fetchWithRetry('/api/dashboard/risk-exposure', { label: '風險曝險' }),
    fetchWithRetry('/api/dashboard/phase3-status', { label: 'Phase 3 狀態' }),
    fetchWithRetry('/api/alerts', { label: '系統警報' }),
  ]);
  const [macro, pipeline, live, riskExposure, phase3, alerts] = results;

  if (macro === null) setPanelError('macroRadar', '總經雷達');
  else if (m.dash.renderMacroRadar) { m.dash.renderMacroRadar(macro, pipeline); clearPanelLoading('macroRadar'); }

  if (live === null) setPanelError('liveStatus', '即時狀態');
  else if (m.risk.renderLiveStatus) { m.risk.renderLiveStatus(live); clearPanelLoading('liveStatus'); }

  if (riskExposure === null) setPanelError('riskCards', '風險指標');
  else if (m.risk.renderRiskCards) { m.risk.renderRiskCards(riskExposure, pipeline, core.capitalPhase); clearPanelLoading('riskCards'); }

  if (alerts === null) setPanelError('alertsPanel', '系統警報');
  else if (m.alerts.renderAlerts) { m.alerts.renderAlerts(alerts); clearPanelLoading('alertsPanel'); }

  if (m.risk.renderRiskCommentary) m.risk.renderRiskCommentary();

  if (phase3 === null) console.warn('[non-core] phase3-status unavailable');
}

async function loadAll() {
  var loadingBar = document.getElementById('loadingBar');
  if (loadingBar) loadingBar.classList.add('active');
  showSkeletons();

  try {
    await loadModules();
    var m = modules;

    const core = await fetchCore();
    if (core.failures > core.total * 0.5) {
      consecutiveFailures++;
      if (consecutiveFailures >= MAX_CONSECUTIVE_FAILURES) showErrorBanner();
    } else {
      consecutiveFailures = 0;
      hideErrorBanner();
    }

    renderCore(m, core);

    // Background non-core updates; do not block the next refresh or the UI.
    fetchNonCore(m, core).catch(function(e) { console.error('[non-core] background update failed', e); });

    // Independent panels that fetch their own data.
    if (m.metrics.loadMetrics) m.metrics.loadMetrics();
    if (m.back.renderBacktestReport) m.back.renderBacktestReport();
    if (m.experiments.loadAuditLog) m.experiments.loadAuditLog();
    if (m.experiments.loadExperimentHistory) m.experiments.loadExperimentHistory();

  } catch (e) {
    console.error(e);
    consecutiveFailures++;
    if (consecutiveFailures >= MAX_CONSECUTIVE_FAILURES) showErrorBanner();
  } finally {
    if (loadingBar) loadingBar.classList.remove('active');
    var refreshTime = document.getElementById('refreshTime');
    if (refreshTime) refreshTime.textContent = new Date().toLocaleString('zh-TW');
  }
}

// --- Lazy page loader ---
async function loadPageData(pageId) {
  await loadModules();
  var m = modules;

  if (pageId === 'home') {
    try {
      var tasks = await silentGetJSON('/api/scheduler/status');
      if (m.dash.renderSchedulerStatus) m.dash.renderSchedulerStatus(tasks);
    } catch (e) {
      var el = document.getElementById('schedulerStatusContent');
      if (el) {
        el.classList.remove('loading');
        el.innerHTML = renderMissingState('排程狀態', 'api-error');
      }
    }
  }
  else if (pageId === 'experiments') {
    try {
      var inbox = await silentGetJSON('/api/dashboard/experiment-inbox');
      var fvrExp = await silentGetJSON('/api/dashboard/forecast-vs-reality');
      if (m.inbox.renderInbox) m.inbox.renderInbox(inbox);
      if (m.experiments.loadAuditLog) m.experiments.loadAuditLog();
      if (m.experiments.loadExperimentHistory) m.experiments.loadExperimentHistory();
      if (m.experiments.renderForecastVsRealitySummary) m.experiments.renderForecastVsRealitySummary(fvrExp);
    } catch(e) { console.error(e); }
  }
  else if (pageId === 'reports') {
    try {
      var signals = await silentGetJSON('/api/backtest/signals');
      var fvrReport = await silentGetJSON('/api/dashboard/forecast-vs-reality');
      if (m.back.renderBacktestReport) m.back.renderBacktestReport();
      if (m.back.renderBacktestSignals) m.back.renderBacktestSignals(signals);
      if (m.back.renderForecastVsRealityTable) m.back.renderForecastVsRealityTable(fvrReport);
    } catch(e) { console.error(e); }
  }
  else if (pageId === 'datachannels') {
    try {
      if (m.datachannels.loadDataChannels) m.datachannels.loadDataChannels();
    } catch(e) { console.error(e); }
  }
  else if (pageId === 'capital_models') {
    try { if (m.capitalModels && m.capitalModels.loadCapitalModels) m.capitalModels.loadCapitalModels(); } catch(e) { console.error(e); }
  }
  else if (pageId === 'capital_causality') {
    try { if (m.capitalCausality && m.capitalCausality.loadCapitalCausality) m.capitalCausality.loadCapitalCausality(); } catch(e) { console.error(e); }
  }
  else if (pageId === 'capital_quality') {
    try { if (m.capitalQuality && m.capitalQuality.loadCapitalQuality) m.capitalQuality.loadCapitalQuality(); } catch(e) { console.error(e); }
  }
  else if (pageId === 'alerts') {
    try { if (m.alerts.loadAlerts) m.alerts.loadAlerts(); } catch(e) { console.error(e); }
  }
  else if (pageId === 'metrics') {
    try { if (m.metrics.loadMetrics) m.metrics.loadMetrics(); } catch(e) { console.error(e); }
  }
  else if (pageId === 'live') {
    try {
      var liveResults = await Promise.all([
        getJSONWithTimeout('/api/dashboard/live-status'),
        getJSONWithTimeout('/api/dashboard/recommendation-pipeline'),
        getJSONWithTimeout('/api/dashboard/risk-exposure'),
        getJSONWithTimeout('/api/dashboard/macro-radar'),
        getJSONWithTimeout('/api/narrative/events'),
        getJSONWithTimeout('/api/taiwan/stress-index'),
        getJSONWithTimeout('/api/narrative/chains'),
        getJSONWithTimeout('/api/narrative/models'),
        getJSONWithTimeout('/api/dashboard/capital-phase'),
        getJSONWithTimeout('/api/dashboard/risk-calibration'),
        getJSONWithTimeout('/api/macro/snapshot/latest'),
        getJSONWithTimeout('/api/dashboard/industry-cycle?industry=semiconductor'),
        getJSONWithTimeout('/api/dashboard/drawdown'),
      ]);
      if (m.risk.renderLiveStatus) m.risk.renderLiveStatus(liveResults[0]);
      if (m.risk.renderRiskCards) m.risk.renderRiskCards(liveResults[2], liveResults[1], liveResults[8]);
      if (m.risk.renderRiskCalibration) m.risk.renderRiskCalibration(liveResults[9]);
      if (m.risk.renderRiskCommentary) m.risk.renderRiskCommentary();
      if (m.dash.renderMacroRadar) m.dash.renderMacroRadar(liveResults[3], liveResults[1]);
      if (m.narr.renderLiveNarrativeStrip) m.narr.renderLiveNarrativeStrip(liveResults[4], liveResults[5], liveResults[7], liveResults[6]);
      if (m.risk.renderSemiconductorSentiment) m.risk.renderSemiconductorSentiment(liveResults[10], liveResults[11]);
      if (m.risk.renderDrawdownPanel) m.risk.renderDrawdownPanel(liveResults[12]);
    } catch(e) { console.error(e); }
  }
  else if (pageId === 'parameters') {
    try {
      var pData = await Promise.all([
        silentGetJSON('/api/parameters'),
        silentGetJSON('/api/parameters/categories'),
        silentGetJSON('/api/parameters/audit-log'),
        silentGetJSON('/api/parameters/metadata')
      ]);
      if (m.parameters && m.parameters.renderParametersPage) {
        m.parameters.renderParametersPage(pData[0], pData[1], pData[2], pData[3]);
      }
    } catch(e) { console.error(e); }
  }
  else if (pageId === 'config') {
    try {
      var cfg = await silentGetJSON('/api/config');
      if (m.deployConfig && m.deployConfig.renderConfigPage) m.deployConfig.renderConfigPage(cfg);
    } catch(e) { console.error(e); }
  }
}

// --- Initialization ---
function initBacktestDates() {
  var today = new Date(), start = new Date(today);
  start.setDate(start.getDate() - 5);
  var s = document.getElementById('backtestStart'), e = document.getElementById('backtestEnd');
  if (s) s.value = start.toISOString().split('T')[0];
  if (e) e.value = today.toISOString().split('T')[0];
}

// --- Global unhandled rejection handler (defense in depth) ---
window.addEventListener('unhandledrejection', function(event) {
  var reason = event.reason || {};
  var msg = reason.message || String(reason);
  console.error('Unhandled Promise rejection: ' + msg, reason.stack || '');
  event.preventDefault();
});

if (typeof window !== "undefined") window.switchPage = switchPage;
if (typeof window !== "undefined") window.toggleSidebar = toggleSidebar;
if (typeof window !== "undefined") window.retryLoad = retryLoad;
if (typeof window !== "undefined") window.fmtNTD = fmtNTD;
if (typeof window !== "undefined") window.loadAll = loadAll;

window.toggleAutoRefresh = function() {
  if (autoRefreshEnabled) {
    stopAutoRefresh();
    autoRefreshEnabled = false;
    var btn = document.getElementById('refreshToggle');
    if (btn) btn.textContent = '▶';
  } else {
    autoRefreshEnabled = true;
    startAutoRefresh();
    var btn = document.getElementById('refreshToggle');
    if (btn) btn.textContent = '⏸';
  }
};

if (typeof window !== 'undefined') {
  install401Interceptor({
    loginPageId: 'login',
    excludedPages: ['login'],
    onUnauthorized: invalidateAuth,
    switchPage: window.switchPage,
  });
  initBacktestDates();
  loadAll();
  startAutoRefresh();
  initEventStream();
  var initialPath = window.location.pathname
    .replace(new RegExp('^' + (basePath || '/') + '/?'), '')
    .replace(/\/$/, '');
  if (!initialPath) {
    history.replaceState({page: 'home'}, '', basePath + '/home');
    switchPage('home', true);
  }
  // Redirect old hash URLs to clean URLs
  if (window.location.hash && window.location.hash.startsWith('#page-')) {
    var pageId = window.location.hash.replace('#page-', '');
    window.location.replace(basePath + '/' + pageId);
  } else if (initialPath && initialPath !== 'home') {
    if (titles[initialPath]) {
      history.replaceState({page: initialPath}, '', basePath + '/' + initialPath);
      switchPage(initialPath, true);
    } else {
      history.replaceState({page: 'home'}, '', basePath + '/home');
      switchPage('home', true);
    }
  } else if (initialPath === 'home') {
    switchPage('home', true);
  }
}

function initEventStream() {
function eventDedupKey(ev) {
  if (ev.payload && ev.payload.event_id) return ev.payload.event_id;
  return ev.id || ev.timestamp || '';
}

const recentEvents = [];
const maxEvents = 20;

  function mapEventToProgress(eventType) {
    if (eventType === 'simulation.start' || eventType === 'system.start') return 'fetching_data';
    if (eventType === 'market.regime.change') return 'regime_detection';
    if (eventType === 'agent.recommendation') return 'agent_recommendations';
    if (eventType === 'guard.outcome') return 'control_filtering';
    if (eventType === 'portfolio.position.update') return 'simulation_running';
    if (eventType === 'simulation.complete' || eventType === 'system.complete') return 'complete';
    return null;
  }

  function updateStatusHint(status, eventCount) {
    const hintEl = document.getElementById('liveStatusHint');
    if (!hintEl) return;
    
    const now = new Date().toLocaleTimeString('zh-TW');
    
    if (status === 'connecting') {
      hintEl.innerHTML = `🟡 連接中... <span style="opacity:0.6">${now}</span>`;
      hintEl.style.color = 'var(--warn)';
    } else if (status === 'error') {
      hintEl.innerHTML = `🔴 連線中斷 <span style="opacity:0.6">${now}</span>`;
      hintEl.style.color = 'var(--color-danger)';
    } else if (status === 'connected' && eventCount === 0) {
      hintEl.innerHTML = `🟢 已連線 · 等待事件 <span style="opacity:0.6">${now}</span>`;
      hintEl.style.color = 'var(--color-success)';
    } else if (status === 'connected') {
      hintEl.innerHTML = `🟢 已連線 · ${eventCount} 個事件 <span style="opacity:0.6">${now}</span>`;
      hintEl.style.color = 'var(--color-success)';
    } else {
      hintEl.innerHTML = `⚪ 未連線 <span style="opacity:0.6">${now}</span>`;
      hintEl.style.color = 'var(--muted)';
    }
  }

  eventSource.on('*', (ev) => {
    var key = eventDedupKey(ev);
    if (key) {
      var dupIdx = recentEvents.findIndex(function(e) { return eventDedupKey(e) === key; });
      if (dupIdx !== -1) recentEvents.splice(dupIdx, 1);
    }
    recentEvents.unshift(ev);
    if (recentEvents.length > maxEvents) {
      recentEvents.pop();
    }
    
    const eventsContainer = document.getElementById('toolEvents');
    if (eventsContainer) renderToolEvents(eventsContainer, recentEvents);

    const newState = mapEventToProgress(ev.type);
    if (newState) {
      const progressContainer = document.getElementById('liveProgress');
      if (progressContainer) renderLiveProgress(progressContainer, newState);
      
      if (newState === 'complete') {
        setTimeout(() => {
          if (progressContainer) renderLiveProgress(progressContainer, 'idle');
        }, 3000);
      }
    }
    
    updateStatusHint('connected', recentEvents.length);
  });

  eventSource.onStatusChange((status) => {
    const pill = document.getElementById('refreshPill');
    if (pill) {
      pill.classList.remove('sse-connected', 'sse-connecting', 'sse-error');
      if (status === 'connected') {
        pill.classList.add('sse-connected');
      } else if (status === 'connecting') {
        pill.classList.add('sse-connecting');
      } else if (status === 'error' || status === 'disconnected') {
        pill.classList.add('sse-error');
      }
    }
    
    updateStatusHint(status, recentEvents.length);
  });

  eventSource.connect();
  
  const progressContainer = document.getElementById('liveProgress');
  const eventsContainer = document.getElementById('toolEvents');
  if (progressContainer) renderLiveProgress(progressContainer, 'idle');
  if (eventsContainer) renderToolEvents(eventsContainer, []);
  updateStatusHint('connecting', 0);
}

if (typeof window !== "undefined") window.toggleTheme = function() {
  var r = document.documentElement;
  r.setAttribute('data-theme', r.getAttribute('data-theme') === 'light' ? 'dark' : 'light');
};


window.addEventListener('popstate', function(e) {
  if (e.state && e.state.page) {
    switchPage(e.state.page, true);
  }
});
