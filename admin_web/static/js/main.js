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
import { getJSON, silentGetJSON, escapeHtml, parseSessionsList } from './shared/app-utils.js';
import { injectSharedHead } from './shared/head-config.js';
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
  if (pageEl.classList.contains('active')) return;
  document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
  document.getElementById('page-' + id).classList.add('active');
  document.querySelectorAll('#sidebar nav a').forEach(a => a.classList.remove('active'));
  const btn = document.querySelector('#sidebar nav a[data-page="' + id + '"]');
  if (btn) btn.classList.add('active');
  const titles = {
  home: '系統總覽', live: '風控營運台', alerts: '系統警報',
  evolution_panel: '策略演化', experiments: '模擬交易',
  datachannels: '資料通道', parameters: '參數管理',
  reports: '最新回測',
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

// Fetch helper: avoid a single slow endpoint blocking the whole dashboard.
function getJSONWithTimeout(url, ms) {
  ms = ms || 10000;
  return Promise.race([
    silentGetJSON(url),
    new Promise(function(resolve) {
      setTimeout(function() { console.warn('[timeout]', url); resolve(null); }, ms);
    })
  ]);
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
    import('./pages/evolution_panel.js'),
  ];
  var results = await Promise.allSettled(imports);
  var keys = ['dash', 'risk', 'narr', 'back', 'inbox', 'experiments', 'alerts', 'metrics', 'datachannels', 'parameters', 'deployConfig', 'evolution_panel'];
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
// --- Main Data Loader ---
async function loadAll() {
  var loadingBar = document.getElementById('loadingBar');
  if (loadingBar) loadingBar.classList.add('active');
  showSkeletons();

  try {
    var results = await Promise.all([
      getJSONWithTimeout('/api/dashboard/system-health'),
      getJSONWithTimeout('/api/dashboard/macro-radar'),
      getJSONWithTimeout('/api/dashboard/agent-observatory'),
      getJSONWithTimeout('/api/dashboard/recommendation-pipeline'),
      getJSONWithTimeout('/api/dashboard/live-status'),
      getJSONWithTimeout('/api/dashboard/risk-exposure'),
      getJSONWithTimeout('/api/dashboard/experiment-inbox'),
      getJSONWithTimeout('/api/taiwan/stress-index'),
      getJSONWithTimeout('/api/narrative/bundle'),
      getJSONWithTimeout('/api/dashboard/data-channels'),
      getJSONWithTimeout('/api/dashboard/sessions'),
      getJSONWithTimeout('/api/dashboard/phase3-status'),
      getJSONWithTimeout('/api/alerts'),
      getJSONWithTimeout('/api/dashboard/capital-phase'),
    ]);

    var health = results[0], macro = results[1], agents = results[2], pipeline = results[3], live = results[4],
        riskExposure = results[5], inbox = results[6], stress = results[7], bundle = results[8],
        dataChannels = results[9], sessions = results[10], phase3 = results[11], alerts = results[12],
        capitalPhase = results[13];

    // Unwrap narrative bundle into backwards-compatible shapes.
    var events = bundle && bundle.events ? { events: bundle.events } : null;
    var chains = bundle && bundle.chains ? { chains: bundle.chains } : null;
    var models = bundle && bundle.models ? { models: bundle.models } : null;
    var templates = bundle && bundle.templates ? { templates: bundle.templates } : null;
    var seasonal = bundle && bundle.seasonal ? bundle.seasonal : null;

    var failures = results.filter(function(v) { return v === null; }).length;
    if (failures > results.length * 0.5) {
      consecutiveFailures++;
      if (consecutiveFailures >= MAX_CONSECUTIVE_FAILURES) showErrorBanner();
    } else {
      consecutiveFailures = 0; hideErrorBanner();
    }

    var parsed = parseSessionsList(sessions);
    window.pipelineSessions = parsed.sessions;
    window.pipelineSessionsStatus = parsed.data_status;

    await loadModules();
    var m = modules;

    if (m.dash.renderOverview) m.dash.renderOverview(health, agents, inbox, events, stress, dataChannels, capitalPhase);
    if (m.dash.renderMacroRadar) m.dash.renderMacroRadar(macro, pipeline);
    if (m.narr.renderLiveNarrativeStrip) m.narr.renderLiveNarrativeStrip(events, stress, models, chains);

    if (m.risk.renderLiveStatus) m.risk.renderLiveStatus(live);
    if (m.risk.renderRiskCards) m.risk.renderRiskCards(riskExposure, pipeline, capitalPhase);
    if (m.risk.renderRiskCommentary) m.risk.renderRiskCommentary();

    if (m.inbox.renderInbox) m.inbox.renderInbox(inbox);
    if (m.datachannels.renderDataChannels) m.datachannels.renderDataChannels(dataChannels);
    if (m.alerts.renderAlerts) m.alerts.renderAlerts(alerts);
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
        el.innerHTML = '<div class="empty error">排程狀態載入失敗</div>';
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
  else if (pageId === 'evolution_panel') {
    try {
      import('./pages/evolution_panel.js').then(function(evo) {
        if (evo.loadEvolutionData) evo.loadEvolutionData();
      }).catch(function(err) {
        console.error('[Dynamic import] evolution_panel module load failed:', err);
      });
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
