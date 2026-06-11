/**
 * @typedef {import('./shared/field_types.ts').GuardOutcome} GuardOutcome
 * @typedef {import('./shared/field_types.ts').RecommendationOutcome} RecommendationOutcome
 * @typedef {import('./shared/field_types.ts').RiskSnapshot} RiskSnapshot
 * @typedef {import('./shared/field_types.ts').SessionSummary} SessionSummary
 * @typedef {import('./shared/field_types.ts').Scorecard} Scorecard
 */

import { loadReasoningTrace } from './components/reasoning-trace.js';
import { eventSource } from './services/event-source.js';
import { renderLiveProgress } from './components/live-progress.js';
import { renderToolEvents } from './components/tool-events.js';
import { fmtNTD } from './shared/utils.js';
import { getJSON, silentGetJSON, escapeHtml } from './shared/app-utils.js';

export { getJSON, escapeHtml };

const pageLoadStatus = {};
const APP_VERSION = '20260512';

export function switchPage(id, silent) {
  if (document.getElementById('page-' + id).classList.contains('active')) return;
  document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
  document.getElementById('page-' + id).classList.add('active');
  document.querySelectorAll('#sidebar nav a').forEach(a => a.classList.remove('active'));
  const btn = document.querySelector('#sidebar nav a[data-page="' + id + '"]');
  if (btn) btn.classList.add('active');
  const titles = {
    overview: '總覽', narrative: '宏觀敘事', live: '風控結果',
    pipeline: '投資管線', decision: '決策鏈', agents: 'AI 觀測台',
    'reasoning-trace': '決策追蹤',
    experiments: '模擬交易', reports: '最新回測', controls: '控制與稽核',
    datachannels: '信息通道', synergy: '人機協同', alerts: '系統警報',
    metrics: '指標監控', industry: '產業生態系', portfolio: '組合持倉', parameters: '參數管理', config: '部署配置',
    evolution_panel: '演化透視', strategies: '投資心法',
  swarm: 'Swarm 模擬', crossmarket: '美台連動監控'
  };
  document.getElementById('pageTitle').textContent = titles[id] || id;
  document.getElementById('sidebar').classList.remove('open');
  if (!pageLoadStatus[id]) { pageLoadStatus[id] = true; loadPageData(id); }
  if (!silent) history.pushState({page: id}, '', '/' + id);
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

// --- Module Registry (all modules loaded once) ---
var modules = {};

async function loadModules() {
  if (modules._loaded) return modules;
  var imports = [
    import('./pages/dashboard.js'),
    import('./pages/pipeline.js'),
    import('./pages/risk.js'),
    import('./pages/narrative.js'),
    import('./pages/backtest.js'),
    import('./pages/inbox.js'),
    import('./pages/experiments.js'),
    import('./pages/alerts.js'),
    import('./pages/metrics.js'),
    import('./pages/industry.js'),
    import('./pages/datachannels.js'),
    import('./pages/parameters.js'),
    import('./pages/deploy-config.js'),
    import('./pages/synergy.js'),
    import('./pages/evolution_panel.js'),
    import('./pages/decision-chain.js'),
    import('./pages/strategies.js'),
  import('./pages/swarm.js'),
    import('./pages/crossmarket.js'),
  ];
  var results = await Promise.allSettled(imports);
  var keys = ['dash', 'pipe', 'risk', 'narr', 'back', 'inbox', 'experiments', 'alerts', 'metrics', 'industry', 'datachannels', 'parameters', 'deployConfig', 'synergy', 'evolution_panel', 'decision', 'strategies', 'swarm', 'crossmarket'];
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
  return modules;
}
// --- Main Data Loader ---
async function loadAll() {
  var loadingBar = document.getElementById('loadingBar');
  if (loadingBar) loadingBar.classList.add('active');
  showSkeletons();

  try {
    var results = await Promise.all([
      silentGetJSON('/api/dashboard/system-health'),
      silentGetJSON('/api/dashboard/macro-radar'),
      silentGetJSON('/api/dashboard/agent-observatory'),
      silentGetJSON('/api/dashboard/recommendation-pipeline'),
      silentGetJSON('/api/dashboard/live-status'),
      silentGetJSON('/api/dashboard/risk-exposure'),
      silentGetJSON('/api/dashboard/experiment-inbox'),
      silentGetJSON('/api/dashboard/universe-overlap'),
      silentGetJSON('/api/taiwan/stress-index'),
      silentGetJSON('/api/narrative/bundle'),
      silentGetJSON('/api/macro/snapshot/latest'),
      silentGetJSON('/api/dashboard/data-channels'),
      silentGetJSON('/api/dashboard/sessions'),
      silentGetJSON('/api/dashboard/phase3-status'),
      silentGetJSON('/api/alerts'),
      silentGetJSON('/api/dashboard/retail-sentiment'),
      silentGetJSON('/api/dashboard/capital-phase'),
      silentGetJSON('/api/dashboard/tax-snapshot'),
      silentGetJSON('/api/dashboard/regime-history'),
      silentGetJSON('/api/synergy/darwinian-trend'),
      silentGetJSON('/api/synergy/darwinian-status'),
      silentGetJSON('/api/dashboard/risk-calibration'),
    ]);

    var health = results[0], macro = results[1], agents = results[2], pipeline = results[3], live = results[4],
        riskExposure = results[5], inbox = results[6], overlap = results[7], stress = results[8], bundle = results[9],
        snapshot = results[10], dataChannels = results[11],
        sessions = results[12], phase3 = results[13], alerts = results[14], retailSentiment = results[15],
        capitalPhase = results[16], taxSnapshot = results[17], regimeHistory = results[18],
        darwinianTrend = results[19], darwinianStatus = results[20], riskCalibration = results[21];

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

    window.pipelineSessions = sessions && sessions.sessions ? sessions.sessions : [];

    await loadModules();
    var m = modules;

    if (m.dash.renderOverview) m.dash.renderOverview(health, agents, inbox, overlap, events, stress, dataChannels, capitalPhase);
    if (m.dash.renderMacroRadar) m.dash.renderMacroRadar(macro, pipeline);
    if (m.dash.renderAgentObservatory) m.dash.renderAgentObservatory(agents, overlap, darwinianTrend);
    if (m.dash.renderUniverseOverlap) m.dash.renderUniverseOverlap(overlap);
    if (m.dash.renderAIEvolution) m.dash.renderAIEvolution(inbox, phase3, darwinianStatus, darwinianTrend, agents, macro, stress);

    if (m.pipe.renderPipeline) m.pipe.renderPipeline(pipeline, false, '');

    if (m.narr.renderLiveNarrativeStrip) m.narr.renderLiveNarrativeStrip(events, stress, models, chains);
    if (m.narr.renderNarrativePage) m.narr.renderNarrativePage(snapshot, stress, events, chains, models, templates, retailSentiment, seasonal);

    if (m.risk.renderLiveStatus) m.risk.renderLiveStatus(live);
    if (m.risk.renderRiskCards) m.risk.renderRiskCards(riskExposure, pipeline, capitalPhase);
    if (m.risk.renderRiskCalibration) m.risk.renderRiskCalibration(riskCalibration);

    if (m.inbox.renderInbox) m.inbox.renderInbox(inbox);
    if (m.datachannels.renderDataChannels) m.datachannels.renderDataChannels(dataChannels);
    if (m.alerts.renderAlerts) m.alerts.renderAlerts(alerts);
    if (m.metrics.loadMetrics) m.metrics.loadMetrics();
    if (m.industry.loadIndustryData) m.industry.loadIndustryData();

    if (m.back.renderBacktestReport) m.back.renderBacktestReport();

    if (m.experiments.loadOverrides) m.experiments.loadOverrides();
    if (m.experiments.loadAuditLog) m.experiments.loadAuditLog();
    if (m.experiments.loadExperimentHistory) m.experiments.loadExperimentHistory();
    if (m.synergy.renderSynergyPage) m.synergy.renderSynergyPage(darwinianStatus, darwinianTrend, inbox);

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

  if (pageId === 'narrative') {
    try {
      var results = await Promise.all([
        silentGetJSON('/api/macro/snapshot/latest'),
        silentGetJSON('/api/taiwan/stress-index'),
        silentGetJSON('/api/narrative/events'),
        silentGetJSON('/api/narrative/chains'),
        silentGetJSON('/api/narrative/models'),
        silentGetJSON('/api/narrative/templates'),
        silentGetJSON('/api/dashboard/retail-sentiment'),
        silentGetJSON('/api/narrative/seasonal'),
      ]);
      if (m.narr.renderNarrativePage) m.narr.renderNarrativePage(results[0], results[1], results[2], results[3], results[4], results[5], results[6], results[7]);
    } catch(e) { console.error(e); }
  }
  else if (pageId === 'pipeline') {
    try {
      var p = await silentGetJSON('/api/dashboard/recommendation-pipeline');
      if (m.pipe.renderPipeline) m.pipe.renderPipeline(p, false, '');
    } catch(e) { console.error(e); }
  }
  else if (pageId === 'decision') {
    try {
      if (m.decision && m.decision.loadDecisionChain) m.decision.loadDecisionChain();
    } catch(e) { console.error(e); }
  }
  else if (pageId === 'reasoning-trace') {
    loadReasoningTrace(window._currentSessionId);
  }
  else if (pageId === 'agents') {
    try {
      var a = await Promise.all([
        silentGetJSON('/api/dashboard/agent-observatory'),
        silentGetJSON('/api/dashboard/universe-overlap'),
      ]);
      if (m.dash.renderAgentObservatory) m.dash.renderAgentObservatory(a[0], a[1]);
      if (m.dash.renderUniverseOverlap) m.dash.renderUniverseOverlap(a[1]);
    } catch(e) { console.error(e); }
  }
  else if (pageId === 'experiments') {
    try {
      var inbox = await silentGetJSON('/api/dashboard/experiment-inbox');
      if (m.inbox.renderInbox) m.inbox.renderInbox(inbox);
      if (m.experiments.loadAuditLog) m.experiments.loadAuditLog();
      if (m.experiments.loadExperimentHistory) m.experiments.loadExperimentHistory();
    } catch(e) { console.error(e); }
  }
  else if (pageId === 'reports') {
    try { if (m.back.renderBacktestReport) m.back.renderBacktestReport(); } catch(e) { console.error(e); }
  }
  else if (pageId === 'controls') {
    try { if (m.experiments.loadOverrides) m.experiments.loadOverrides(); } catch(e) { console.error(e); }
  }
  else if (pageId === 'datachannels') {
    try {
      var dc = await silentGetJSON('/api/dashboard/data-channels');
      if (m.datachannels.renderDataChannels) m.datachannels.renderDataChannels(dc);
      if (m.datachannels.loadFetchLogs) m.datachannels.loadFetchLogs();
    } catch(e) { console.error(e); }
  }
  else if (pageId === 'synergy') {
    try {
      var s = await Promise.all([
        silentGetJSON('/api/synergy/darwinian-status'),
        silentGetJSON('/api/synergy/darwinian-trend'),
        silentGetJSON('/api/dashboard/experiment-inbox')
      ]);
      if (m.synergy && m.synergy.renderSynergyPage) m.synergy.renderSynergyPage(s[0], s[1], s[2]);
    } catch(e) { console.error(e); }
  }
  else if (pageId === 'alerts') {
    try { if (m.alerts.loadAlerts) m.alerts.loadAlerts(); } catch(e) { console.error(e); }
  }
  else if (pageId === 'metrics') {
    try { if (m.metrics.loadMetrics) m.metrics.loadMetrics(); } catch(e) { console.error(e); }
  }
  else if (pageId === 'industry') {
    try { if (m.industry.loadIndustryData) m.industry.loadIndustryData(); } catch(e) { console.error(e); }
  }
  else if (pageId === 'live') {
    try {
      var liveResults = await Promise.all([
        silentGetJSON('/api/dashboard/live-status'),
        silentGetJSON('/api/dashboard/recommendation-pipeline'),
        silentGetJSON('/api/dashboard/risk-exposure'),
        silentGetJSON('/api/dashboard/macro-radar'),
        silentGetJSON('/api/narrative/events'),
        silentGetJSON('/api/taiwan/stress-index'),
        silentGetJSON('/api/narrative/chains'),
        silentGetJSON('/api/narrative/models'),
        silentGetJSON('/api/dashboard/capital-phase'),
        silentGetJSON('/api/dashboard/risk-calibration'),
      ]);
      if (m.risk.renderLiveStatus) m.risk.renderLiveStatus(liveResults[0]);
      if (m.risk.renderRiskCards) m.risk.renderRiskCards(liveResults[2], liveResults[1], liveResults[8]);
      if (m.risk.renderRiskCalibration) m.risk.renderRiskCalibration(liveResults[9]);
      if (m.dash.renderMacroRadar) m.dash.renderMacroRadar(liveResults[3], liveResults[1]);
      if (m.narr.renderLiveNarrativeStrip) m.narr.renderLiveNarrativeStrip(liveResults[4], liveResults[5], liveResults[7], liveResults[6]);
    } catch(e) { console.error(e); }
  }
  else if (pageId === 'portfolio') {
    try {
      var portfolioModule = await import('./pages/portfolio.js').catch(function(err) {
        console.error('[Dynamic import] portfolio module load failed:', err);
        return null;
      });
      if (portfolioModule) portfolioModule.loadPortfolioPage(getJSON, window.agentNameEsm || function(id) { return id; });
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
  else if (pageId === 'strategies') {
    try { if (m.strategies && m.strategies.renderStrategiesPage) m.strategies.renderStrategiesPage(); } catch(e) { console.error(e); }
  }
  else if (pageId === 'swarm') {
    try { if (m.swarm && m.swarm.loadSwarmData) m.swarm.loadSwarmData(); } catch(e) { console.error(e); }
  }
  else if (pageId === 'crossmarket') {
    try { if (m.crossmarket && m.crossmarket.loadCrossMarketData) m.crossmarket.loadCrossMarketData(); } catch(e) { console.error(e); }
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
function populateAgentSelect() {
  var select = document.getElementById('agentSelect');
  if (!select) return;
}

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
  populateAgentSelect();
  initBacktestDates();
  loadAll();
  startAutoRefresh();
  initEventStream();
  var initialPath = window.location.pathname.replace(/^\//, '');
  if (!initialPath) {
    history.replaceState({page: 'overview'}, '', '/overview');
  }
  // Redirect old hash URLs to clean URLs
  if (window.location.hash && window.location.hash.startsWith('#page-')) {
    var pageId = window.location.hash.replace('#page-', '');
    window.location.replace('/' + pageId);
  } else if (initialPath && initialPath !== 'overview') {
    history.replaceState({page: initialPath}, '', '/' + initialPath);
    switchPage(initialPath, true);
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


// datachannels globals
import('./pages/datachannels.js').then(function(m) {
  if (typeof window === 'undefined') return;
  if (m.triggerChannelsIngest) window.triggerChannelsIngest = m.triggerChannelsIngest;
  if (m.enableAllChannels) window.dcEnableAll = m.enableAllChannels;
  if (m.disableAllChannels) window.dcDisableAll = m.disableAllChannels;
  if (m.triggerChannelFetch) window.triggerChannelFetch = m.triggerChannelFetch;
  if (m.toggleChannel) window.toggleChannel = m.toggleChannel;
  if (m.updateApiKey) window.dcUpdateApiKey = m.updateApiKey;
  if (m.loadFetchLogs) window.loadFetchLogs = m.loadFetchLogs;
  if (m.loadDataChannels) window.loadDataChannels = m.loadDataChannels;
  if (m.refreshChannelStatus) window.refreshChannelStatus = m.refreshChannelStatus;
}).catch(function(err) {
  console.error('[Dynamic import] datachannels module load failed:', err);
});

window.addEventListener('popstate', function(e) {
  if (e.state && e.state.page) {
    switchPage(e.state.page, true);
  }
});
