/**
 * @typedef {import('./shared/field_types.d.ts').GuardOutcome} GuardOutcome
 * @typedef {import('./shared/field_types.d.ts').RecommendationOutcome} RecommendationOutcome
 * @typedef {import('./shared/field_types.d.ts').RiskSnapshot} RiskSnapshot
 * @typedef {import('./shared/field_types.d.ts').SessionSummary} SessionSummary
 * @typedef {import('./shared/field_types.d.ts').Scorecard} Scorecard
 */

import { loadReasoningTrace } from './components/reasoning-trace.js';
import { eventSource } from './services/event-source.js';
import { renderLiveProgress } from './components/live-progress.js';
import { renderToolEvents } from './components/tool-events.js';
import { fmtNTD } from './shared/utils.js';

const pageLoadStatus = {};
const APP_VERSION = '20260509';

export function switchPage(id) {
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
    metrics: '指標監控', industry: '產業生態系', portfolio: '組合持倉', parameters: '參數管理',
    evolution_panel: '演化透視'
  };
  document.getElementById('pageTitle').textContent = titles[id] || id;
  document.getElementById('sidebar').classList.remove('open');
  if (!pageLoadStatus[id]) { pageLoadStatus[id] = true; loadPageData(id); }
}

export function toggleSidebar() {
  document.getElementById('sidebar').classList.toggle('open');
}

export async function getJSON(url) {
  const res = await fetch(url);
  if (!res.ok) throw new Error(url + ': ' + res.status);
  return res.json();
}

async function postJSON(url, body) {
  const res = await fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
  return res.json();
}

function notify(msg, type) { console.log('[' + (type || 'info') + '] ' + msg); }

export function escapeHtml(text) {
  if (!text) return '';
  return String(text).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
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
  document.querySelectorAll('.skeleton-container, .empty.loading').forEach(function(el) { el.innerHTML = renderSkeleton(4); });
}

// --- Module Registry (all modules loaded once) ---
var modules = {};

async function loadModules() {
  if (modules._loaded) return modules;
  var imports = [
    import('./pages/dashboard.js?v=' + APP_VERSION),
    import('./pages/pipeline.js?v=' + APP_VERSION),
    import('./pages/risk.js?v=' + APP_VERSION),
    import('./pages/narrative.js?v=' + APP_VERSION),
    import('./pages/backtest.js?v=' + APP_VERSION),
    import('./pages/inbox.js?v=' + APP_VERSION),
    import('./pages/experiments.js?v=' + APP_VERSION),
    import('./pages/alerts.js?v=' + APP_VERSION),
    import('./pages/metrics.js?v=' + APP_VERSION),
    import('./pages/industry.js?v=' + APP_VERSION),
    import('./pages/datachannels.js?v=' + APP_VERSION),
    import('./pages/parameters.js?v=' + APP_VERSION),
    import('./pages/synergy.js?v=' + APP_VERSION),
    import('./pages/evolution_panel.js?v=' + APP_VERSION),
  ];
  var results = await Promise.allSettled(imports);
  var keys = ['dash', 'pipe', 'risk', 'narr', 'back', 'inbox', 'experiments', 'alerts', 'metrics', 'industry', 'datachannels', 'parameters', 'synergy', 'evolution_panel'];
  results.forEach(function(r, i) {
    modules[keys[i]] = r.status === 'fulfilled' ? r.value : {};
  });
  modules._loaded = true;
  return modules;
}

// --- Main Data Loader ---
async function loadAll() {
  var loadingBar = document.getElementById('loadingBar');
  if (loadingBar) loadingBar.classList.add('active');
  showSkeletons();

  try {
    var results = await Promise.all([
      getJSON('/api/dashboard/system-health').catch(function() { return null; }),
      getJSON('/api/dashboard/macro-radar').catch(function() { return null; }),
      getJSON('/api/dashboard/agent-observatory').catch(function() { return null; }),
      getJSON('/api/dashboard/recommendation-pipeline').catch(function() { return null; }),
      getJSON('/api/dashboard/live-status').catch(function() { return null; }),
      getJSON('/api/dashboard/risk-exposure').catch(function() { return null; }),
      getJSON('/api/dashboard/experiment-inbox').catch(function() { return null; }),
      getJSON('/api/dashboard/universe-overlap').catch(function() { return null; }),
      getJSON('/api/taiwan/stress-index').catch(function() { return null; }),
      getJSON('/api/narrative/events').catch(function() { return null; }),
      getJSON('/api/narrative/chains').catch(function() { return null; }),
      getJSON('/api/narrative/models').catch(function() { return null; }),
      getJSON('/api/narrative/templates').catch(function() { return null; }),
      getJSON('/api/macro/snapshot/latest').catch(function() { return null; }),
      getJSON('/api/dashboard/data-channels').catch(function() { return null; }),
      getJSON('/api/dashboard/sessions').catch(function() { return null; }),
      getJSON('/api/dashboard/phase3-status').catch(function() { return null; }),
      getJSON('/api/alerts').catch(function() { return null; }),
      getJSON('/api/dashboard/retail-sentiment').catch(function() { return null; }),
      getJSON('/api/dashboard/capital-phase').catch(function() { return null; }),
      getJSON('/api/dashboard/tax-snapshot').catch(function() { return null; }),
      getJSON('/api/narrative/seasonal').catch(function() { return null; }),
      getJSON('/api/dashboard/regime-history').catch(function() { return null; }),
      getJSON('/api/synergy/darwinian-trend').catch(function() { return null; }),
      getJSON('/api/health/data-integrity').catch(function() { return null; }),
      getJSON('/api/synergy/darwinian-status').catch(function() { return null; }),
    ]);

    var health = results[0], macro = results[1], agents = results[2], pipeline = results[3], live = results[4],
        riskExposure = results[5], inbox = results[6], overlap = results[7], stress = results[8], events = results[9], chains = results[10],
        models = results[11], templates = results[12], snapshot = results[13], dataChannels = results[14],
        sessions = results[15], phase3 = results[16], alerts = results[17], retailSentiment = results[18],
        capitalPhase = results[19], taxSnapshot = results[20], seasonal = results[21], regimeHistory = results[22],
        darwinianTrend = results[23], dataIntegrity = results[24], darwinianStatus = results[25];

    var allNull = [health, macro, agents, pipeline, live, inbox, overlap].every(function(v) { return v === null; });
    if (allNull) {
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
    if (m.dash.renderAIEvolution) m.dash.renderAIEvolution(inbox, phase3);

    if (m.pipe.renderPipeline) m.pipe.renderPipeline(pipeline, false, '');
    if (m.pipe.renderDecisionChain) m.pipe.renderDecisionChain(pipeline, macro, agents, stress, events, chains, models, inbox, phase3, taxSnapshot, regimeHistory);

    if (m.narr.renderLiveNarrativeStrip) m.narr.renderLiveNarrativeStrip(events, stress, models, chains);
    if (m.narr.renderNarrativePage) m.narr.renderNarrativePage(snapshot, stress, events, chains, models, templates, retailSentiment, seasonal);

    if (m.risk.renderLiveStatus) m.risk.renderLiveStatus(live);
    if (m.risk.renderRiskCards) m.risk.renderRiskCards(riskExposure, pipeline, capitalPhase);

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
  }
}

// --- Lazy page loader ---
async function loadPageData(pageId) {
  await loadModules();
  var m = modules;

  if (pageId === 'narrative') {
    try {
      var results = await Promise.all([
        getJSON('/api/macro/snapshot/latest').catch(function() { return null; }),
        getJSON('/api/taiwan/stress-index').catch(function() { return null; }),
        getJSON('/api/narrative/events').catch(function() { return null; }),
        getJSON('/api/narrative/chains').catch(function() { return null; }),
        getJSON('/api/narrative/models').catch(function() { return null; }),
        getJSON('/api/narrative/templates').catch(function() { return null; }),
        getJSON('/api/dashboard/retail-sentiment').catch(function() { return null; }),
        getJSON('/api/narrative/seasonal').catch(function() { return null; }),
      ]);
      if (m.narr.renderNarrativePage) m.narr.renderNarrativePage(results[0], results[1], results[2], results[3], results[4], results[5], results[6], results[7]);
    } catch(e) { console.error(e); }
  }
  else if (pageId === 'pipeline') {
    try {
      var p = await getJSON('/api/dashboard/recommendation-pipeline').catch(function() { return null; });
      if (m.pipe.renderPipeline) m.pipe.renderPipeline(p, false, '');
    } catch(e) { console.error(e); }
  }
  else if (pageId === 'decision') {
    try {
      var d = await Promise.all([
        getJSON('/api/dashboard/recommendation-pipeline').catch(function() { return null; }),
        getJSON('/api/dashboard/macro-radar').catch(function() { return null; }),
        getJSON('/api/dashboard/agent-observatory').catch(function() { return null; }),
        getJSON('/api/taiwan/stress-index').catch(function() { return null; }),
        getJSON('/api/narrative/events').catch(function() { return null; }),
        getJSON('/api/narrative/chains').catch(function() { return null; }),
        getJSON('/api/narrative/models').catch(function() { return null; }),
        getJSON('/api/dashboard/experiment-inbox').catch(function() { return null; }),
        getJSON('/api/dashboard/phase3-status').catch(function() { return null; }),
        getJSON('/api/dashboard/tax-snapshot').catch(function() { return null; }),
        getJSON('/api/dashboard/regime-history').catch(function() { return null; }),
      ]);
      if (m.pipe.renderDecisionChain) m.pipe.renderDecisionChain(d[0], d[1], d[2], d[3], d[4], d[5], d[6], d[7], d[8], d[9], d[10]);
    } catch(e) { console.error(e); }
  }
  else if (pageId === 'reasoning-trace') {
    loadReasoningTrace(window._currentSessionId);
  }
  else if (pageId === 'agents') {
    try {
      var a = await Promise.all([
        getJSON('/api/dashboard/agent-observatory').catch(function() { return null; }),
        getJSON('/api/dashboard/universe-overlap').catch(function() { return null; }),
      ]);
      if (m.dash.renderAgentObservatory) m.dash.renderAgentObservatory(a[0], a[1], window.darwinianTrend);
      if (m.dash.renderUniverseOverlap) m.dash.renderUniverseOverlap(a[1]);
    } catch(e) { console.error(e); }
  }
  else if (pageId === 'experiments') {
    try {
      var inbox = await getJSON('/api/dashboard/experiment-inbox').catch(function() { return null; });
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
      var dc = await getJSON('/api/dashboard/data-channels').catch(function() { return null; });
      if (m.datachannels.renderDataChannels) m.datachannels.renderDataChannels(dc);
    } catch(e) { console.error(e); }
  }
  else if (pageId === 'synergy') {
    try {
      var s = await Promise.all([
        getJSON('/api/synergy/darwinian-status').catch(function() { return null; }),
        getJSON('/api/synergy/darwinian-trend').catch(function() { return null; }),
        getJSON('/api/dashboard/experiment-inbox').catch(function() { return null; })
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
        getJSON('/api/dashboard/live-status').catch(function() { return null; }),
        getJSON('/api/dashboard/recommendation-pipeline').catch(function() { return null; }),
        getJSON('/api/dashboard/risk-exposure').catch(function() { return null; }),
        getJSON('/api/dashboard/macro-radar').catch(function() { return null; }),
        getJSON('/api/narrative/events').catch(function() { return null; }),
        getJSON('/api/taiwan/stress-index').catch(function() { return null; }),
        getJSON('/api/narrative/chains').catch(function() { return null; }),
        getJSON('/api/narrative/models').catch(function() { return null; }),
        getJSON('/api/dashboard/capital-phase').catch(function() { return null; }),
      ]);
      if (m.risk.renderLiveStatus) m.risk.renderLiveStatus(liveResults[0]);
      if (m.risk.renderRiskCards) m.risk.renderRiskCards(liveResults[2], liveResults[1], liveResults[8]);
      if (m.dash.renderMacroRadar) m.dash.renderMacroRadar(liveResults[3], liveResults[1]);
      if (m.narr.renderLiveNarrativeStrip) m.narr.renderLiveNarrativeStrip(liveResults[4], liveResults[5], liveResults[7], liveResults[6]);
    } catch(e) { console.error(e); }
  }
  else if (pageId === 'portfolio') {
    try {
      var portfolioModule = await import('./pages/portfolio.js?v=' + APP_VERSION).catch(function() { return null; });
      if (portfolioModule) portfolioModule.loadPortfolioPage(getJSON, window.agentNameEsm || function(id) { return id; });
    } catch(e) { console.error(e); }
  }
  else if (pageId === 'parameters') {
    try {
      var pData = await Promise.all([
        getJSON('/api/parameters').catch(function() { return null; }),
        getJSON('/api/parameters/categories').catch(function() { return null; }),
        getJSON('/api/parameters/audit-log').catch(function() { return null; })
      ]);
      if (m.parameters && m.parameters.renderParametersPage) {
        m.parameters.renderParametersPage(pData[0], pData[1], pData[2]);
      }
    } catch(e) { console.error(e); }
  }
  else if (pageId === 'evolution_panel') {
    try {
      import('./pages/evolution_panel.js?v=' + APP_VERSION).then(function(evo) {
        if (evo.loadEvolutionData) evo.loadEvolutionData();
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
  start.setMonth(start.getMonth() - 1);
  var s = document.getElementById('backtestStartDate'), e = document.getElementById('backtestEndDate');
  if (s) s.value = start.toISOString().split('T')[0];
  if (e) e.value = today.toISOString().split('T')[0];
}

if (typeof window !== "undefined") window.switchPage = switchPage;
if (typeof window !== "undefined") window.toggleSidebar = toggleSidebar;
if (typeof window !== "undefined") window.retryLoad = retryLoad;
if (typeof window !== "undefined") window.fmtNTD = fmtNTD;

if (typeof window !== 'undefined') {
  populateAgentSelect();
  initBacktestDates();
  loadAll();
  startAutoRefresh();
  initEventStream();
}

function initEventStream() {
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

  eventSource.on('*', (ev) => {
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
        }, 2000);
      }
    }
  });

  eventSource.onStatusChange((status) => {
    const pill = document.getElementById('refreshPill');
    if (!pill) return;
    
    pill.classList.remove('sse-connected', 'sse-connecting', 'sse-error');
    if (status === 'connected') {
      pill.classList.add('sse-connected');
    } else if (status === 'connecting') {
      pill.classList.add('sse-connecting');
    } else if (status === 'error' || status === 'disconnected') {
      pill.classList.add('sse-error');
    }
  });

  eventSource.connect();
  
  const progressContainer = document.getElementById('liveProgress');
  const eventsContainer = document.getElementById('toolEvents');
  if (progressContainer) renderLiveProgress(progressContainer, 'idle');
  if (eventsContainer) renderToolEvents(eventsContainer, []);
}

if (typeof window !== "undefined") window.toggleTheme = function() {
  var r = document.documentElement;
  r.setAttribute('data-theme', r.getAttribute('data-theme') === 'light' ? 'dark' : 'light');
};
if (typeof window !== "undefined") window.showUnacknowledgedOnly = function() { console.log('showUnacknowledgedOnly: not yet reimplemented'); };

// datachannels globals
import('./pages/datachannels.js?v=' + APP_VERSION).then(function(m) {
  if (m.triggerChannelsIngest && typeof window !== 'undefined') window.triggerChannelsIngest = m.triggerChannelsIngest;
});
