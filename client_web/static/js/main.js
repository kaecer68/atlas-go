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
import { initAuth, isLoggedIn, invalidateAuth, renderNavState, getTier } from './services/auth.js';
import { metricCard } from './components/metric-card.js';
import { fmtSignedPct } from './shared/format-metric.js';
import { renderHomeTierSections } from './components/home-tier-sections.js';
import './modals/modal.js';
import { injectSharedHead } from './shared/head-config.js';
import { install401Interceptor } from './shared/fetch-wrapper.js';
injectSharedHead();

const SHELL_LOADERS = {
  narrative: () => import('./page-shells/narrative.js'),
  pipeline: () => import('./page-shells/pipeline.js'),
  portfolio: () => import('./page-shells/portfolio.js'),
  crossmarket: () => import('./page-shells/crossmarket.js'),
  industry: () => import('./page-shells/industry.js'),
  strategies: () => import('./page-shells/strategies.js'),
  capital_board: () => import('./pages/capital_board.js'),
  login: () => import('./page-shells/login.js'),
  register: () => import('./page-shells/register.js'),
  premium: () => import('./page-shells/premium.js'),
  mcp: () => import('./page-shells/mcp.js'),
  'stock-quote': () => import('./page-shells/stock-quote.js'),
  'performance-report': () => import('./page-shells/performance-report.js'),
  methodology: () => import('./page-shells/methodology.js'),
  'my-signals': () => import('./pages/my-signals.js'),
  retail_sentiment: () => import('./pages/retail_sentiment.js'),
};
const _shellsLoaded = new Set();

async function _ensureShellLoaded(id) {
  if (_shellsLoaded.has(id)) return;
  const loader = SHELL_LOADERS[id];
  if (!loader) return;
  try {
    const mod = await loader();
    const el = document.getElementById('page-' + id);
    if (el && typeof mod.template === 'string') el.innerHTML = mod.template;
    if (typeof mod.init === 'function') {
      await mod.init();
    }
    _shellsLoaded.add(id);
  } catch (e) {
    _shellsLoaded.delete(id);
    console.warn('[switchPage] shell load failed:', id, e);
    const el = document.getElementById('page-' + id);
    if (el && !el.innerHTML) el.innerHTML = '<div class="empty">頁面載入失敗，請重新整理</div>';
  }
}

export { getJSON, escapeHtml };

const pageLoadStatus = {};
const APP_VERSION = '20260512';

const basePath = (typeof window !== 'undefined')
  ? (window.location.pathname.match(/^\/(?:client|admin)/)?.[0] || '')
  : '';

export async function switchPage(id, silent) {
  // Alias /client/quote URL path to 'stock-quote' page module.
  if (id === 'quote') id = 'stock-quote';
  // Unknown page — fallback to 404. 'home' is a loadAll-rendered page, not a shell.
  if (!SHELL_LOADERS[id] && id !== 'errors/404' && id !== 'home' && id !== 'stock-quote') {
    return switchPage('errors/404', silent);
  }
  var pageEl = document.getElementById('page-' + id);
  if (!pageEl) { console.warn('[switchPage] page not found:', id); return; }
  await _ensureShellLoaded(id);
  const wasActive = pageEl.classList.contains('active');
  document.querySelectorAll('.page').forEach(p => { if (p !== pageEl) p.classList.remove('active'); });
  pageEl.classList.add('active');
  if (wasActive && !silent) return;
  document.querySelectorAll('#sidebar nav a').forEach(a => a.classList.remove('active'));
  const btn = document.querySelector('#sidebar nav a[data-page="' + id + '"]');
  if (btn) btn.classList.add('active');
  const titles = {
    home: '今日判讀', narrative: '全球宏觀', live: '風險總覽',
    crossmarket: '美台連動', industry: '產業熱力圖',
    pipeline: '投資管線', portfolio: '組合持倉',
    'performance-report': '績效報告',
    strategies: '投資心法',
    capital_board: '七大勢力看板',
      login: '登入', register: '註冊', premium: '升級 Premium',
      mcp: 'MCP 整合', 'errors/404': '404', 'stock-quote': '個股快查',
      methodology: '方法論：為什麼', 'my-signals': '我的追蹤',
      retail_sentiment: '散戶情緒'
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
  b.innerHTML = '<span>連續 ' + consecutiveFailures + ' 次更新失敗</span><button class="retry-btn" onclick="retryLoad()">重試</button>';
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
    import('./pages/home.js'),
    import('./pages/dashboard.js'),
    import('./pages/risk.js'),
    import('./pages/narrative.js'),
    import('./pages/pipeline.js'),
    import('./pages/inbox.js'),
    import('./pages/experiments.js'),
    import('./pages/industry.js'),
    import('./pages/strategies.js'),
    import('./pages/crossmarket.js'),
  ];
  var results = await Promise.allSettled(imports);
  var keys = ['home', 'dash', 'risk', 'narr', 'pipe', 'inbox', 'experiments', 'industry', 'strategies', 'crossmarket'];
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
// --- Main Data Loader (investor-facing, Phase A slimmed to 6 core APIs) ---
async function loadAll() {
  var loadingBar = document.getElementById('loadingBar');
  if (loadingBar) loadingBar.classList.add('active');
  showSkeletons();

  try {
    var results = await Promise.all([
      getJSONWithTimeout('/api/dashboard/system-health'),
      getJSONWithTimeout('/api/macro/snapshot/latest'),
      getJSONWithTimeout('/api/taiwan/stress-index'),
      getJSONWithTimeout('/api/narrative/bundle'),
      getJSONWithTimeout('/api/dashboard/regime-history'),
    ]);

    var health = results[0], snapshot = results[1], stress = results[2], bundle = results[3],
        regimeHistory = results[4];

    // Unwrap narrative bundle
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

    await loadModules();
    var m = modules;
    if (m.narr.renderLiveNarrativeStrip) m.narr.renderLiveNarrativeStrip(events, stress, models, chains);
    if (m.narr.renderNarrativePage) m.narr.renderNarrativePage(snapshot, stress, events, chains, models, templates, seasonal);
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
        getJSONWithTimeout('/api/macro/snapshot/latest'),
        getJSONWithTimeout('/api/taiwan/stress-index'),
        getJSONWithTimeout('/api/narrative/events'),
        getJSONWithTimeout('/api/narrative/chains'),
        getJSONWithTimeout('/api/narrative/models'),
        getJSONWithTimeout('/api/narrative/templates'),
        getJSONWithTimeout('/api/narrative/seasonal'),
      ]);
      if (m.narr.renderNarrativePage) m.narr.renderNarrativePage(results[0], results[1], results[2], results[3], results[4], results[5], results[6]);

    } catch(e) { console.warn(e); }
  }
  else if (pageId === 'pipeline') {
    try {
      // Fetch the session list explicitly: renderPipeline's session dropdown
      // reads window.pipelineSessions, which nothing else populates on this page.
      var pipelineResults = await Promise.all([
        silentGetJSON('/api/dashboard/recommendation-pipeline'),
        silentGetJSON('/api/dashboard/sessions'),
      ]);
      var p = pipelineResults[0];
      var sessResp = pipelineResults[1];
      if (sessResp && Array.isArray(sessResp.sessions)) {
        window.pipelineSessions = sessResp.sessions;
      }
      if (m.pipe.renderPipeline) m.pipe.renderPipeline(p, false, '');
    } catch(e) { console.warn(e); }
  }
  else if (pageId === 'reasoning-trace') {
    loadReasoningTrace(window._currentSessionId);
  }
  
  
  
  
  
  
  
  
  
  else if (pageId === 'home') {
    try {
      if (m.home && m.home.renderHomePage) {
        await m.home.renderHomePage(document.getElementById('page-home'));
        await renderHomeTierSections();
      }
    } catch(e) { console.warn('[home] load failed:', e); }
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
      ]);
      if (m.risk.renderLiveStatus) m.risk.renderLiveStatus(liveResults[0]);
      if (m.risk.renderRiskCards) m.risk.renderRiskCards(liveResults[2], liveResults[1], liveResults[8]);
      if (m.risk.renderRiskCalibration) m.risk.renderRiskCalibration(liveResults[9]);
      try {
        if (m.risk.renderRiskCommentary) await m.risk.renderRiskCommentary();
      } catch (err) {
        var rc = document.getElementById('risk-commentary');
        if (rc) rc.innerHTML = '<div class="empty-state"><h4>風險評論暫時無法載入</h4><p>資料源可能正在更新，請稍後重試。</p></div>';
        console.warn('[live] risk commentary unavailable:', err.message || err);
      }
      if (m.dash.renderMacroRadar) m.dash.renderMacroRadar(liveResults[3], liveResults[1]);
      if (m.narr.renderLiveNarrativeStrip) m.narr.renderLiveNarrativeStrip(liveResults[4], liveResults[5], liveResults[7], liveResults[6]);
    } catch(e) { console.warn(e); }
  }
  else if (pageId === 'industry') {
    try { if (m.industry && m.industry.loadIndustryData) m.industry.loadIndustryData(); } catch(e) { console.warn(e); }
  }
  else if (pageId === 'stock-quote') {
    try {
      var sqModule = await import('./pages/stock-quote.js').catch(function(err) {
        console.warn('[Dynamic import] stock-quote module load failed:', err);
        return null;
      });
      if (sqModule && sqModule.renderPage) {
        await sqModule.renderPage(document.getElementById('page-stock-quote'));
      }
    } catch(e) { console.warn(e); }
  }
  else if (pageId === 'portfolio') {
    try {
      var portfolioModule = await import('./pages/portfolio.js').catch(function(err) {
        console.warn('[Dynamic import] portfolio module load failed:', err);
        return null;
      });
      if (portfolioModule) portfolioModule.loadPortfolioPage(getJSON, window.agentNameEsm || function(id) { return id; });
    } catch(e) { console.warn(e); }
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
    } catch(e) { console.warn(e); }
  }
  
  else if (pageId === 'strategies') {
    try { if (m.strategies && m.strategies.renderStrategiesPage) m.strategies.renderStrategiesPage(document.getElementById('page-strategies')); } catch(e) { console.warn(e); }
  }

  else if (pageId === 'methodology') {
    try {
      import('./page-shells/methodology.js').then(function(mod) {
        if (mod.loadMethodologyData) mod.loadMethodologyData();
      }).catch(function(err) {
        console.warn('[Dynamic import] methodology module load failed:', err);
      });
    } catch(e) { console.warn(e); }
  }
  
  else if (pageId === 'crossmarket') {
    try { if (m.crossmarket && m.crossmarket.loadCrossMarketData) m.crossmarket.loadCrossMarketData(); } catch(e) { console.warn(e); }
  }
  
  else if (pageId === 'performance-report') {
    try {
      import('./pages/performance-report.js').then(function(mod) {
        if (mod.loadPerformanceReport) mod.loadPerformanceReport();
      }).catch(function(err) {
        console.warn('[Dynamic import] performance-report module load failed:', err);
      });
    } catch(e) { console.warn(e); }
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
    var label = btn && btn.querySelector('.refresh-toggle__label');
    if (label) label.textContent = '繼續';
  } else {
    autoRefreshEnabled = true;
    startAutoRefresh();
    var btn = document.getElementById('refreshToggle');
    var label = btn && btn.querySelector('.refresh-toggle__label');
    if (label) label.textContent = '暫停';
  }
};

if (typeof window !== 'undefined') {
  populateAgentSelect();
  initBacktestDates();
  startAutoRefresh();
  initEventStream();

  // Auth: check JWT validity before loading data; wrap fetch for 401 detection
  install401Interceptor({
    loginPageId: 'login',
    excludedPages: ['login', 'register'],
    onUnauthorized: invalidateAuth,
    switchPage: window.switchPage,
  });

  initAuth().then(function() {
    renderNavState();
    (async () => {
    loadAll().catch(e => console.warn('[init] loadAll failed:', e));
    var initialPath = window.location.pathname
      .replace(new RegExp('^' + (basePath || '/') + '/?'), '')
      .replace(/\/$/, '');
    if (!initialPath) {
      history.replaceState({page: 'home'}, '', basePath + '/home');
      switchPage('home', true);
    } else if (initialPath === 'home') {
      switchPage('home', true);
    } else if (initialPath) {
      const query = window.location.search || '';
      history.replaceState({page: initialPath}, '', basePath + '/' + initialPath + query);
      switchPage(initialPath, true);
    }
    // Redirect old hash URLs to clean URLs
    if (window.location.hash && window.location.hash.startsWith('#page-')) {
      var pageId = window.location.hash.replace('#page-', '');
      window.location.replace(basePath + '/' + pageId);
    }
  })();
  });
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
      hintEl.innerHTML = `<span class="sse-dot sse-dot--warning"></span>連接中… <span style="opacity:0.6">${now}</span>`;
      hintEl.style.color = 'var(--warn)';
    } else if (status === 'error') {
      hintEl.innerHTML = `<span class="sse-dot sse-dot--danger"></span>連線中斷 <span style="opacity:0.6">${now}</span>`;
      hintEl.style.color = 'var(--color-danger)';
    } else if (status === 'connected' && eventCount === 0) {
      hintEl.innerHTML = `<span class="sse-dot sse-dot--success"></span>已連線 · 等待事件 <span style="opacity:0.6">${now}</span>`;
      hintEl.style.color = 'var(--color-success)';
    } else if (status === 'connected') {
      hintEl.innerHTML = `<span class="sse-dot sse-dot--success"></span>已連線 · ${eventCount} 個事件 <span style="opacity:0.6">${now}</span>`;
      hintEl.style.color = 'var(--color-success)';
    } else {
      hintEl.innerHTML = `<span class="sse-dot sse-dot--idle"></span>未連線 <span style="opacity:0.6">${now}</span>`;
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
