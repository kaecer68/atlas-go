// main.js - Application shell: routing, data loading, auto-refresh, initialization
// ES Module for Atlas dashboard orchestration

// --- Page state ---
const pageLoadStatus = {};

// --- Page routing ---
function switchPage(id) {
  document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
  document.getElementById('page-' + id).classList.add('active');
  document.querySelectorAll('#sidebar nav a').forEach(a => a.classList.remove('active'));
  const btn = document.querySelector('#sidebar nav a[data-page="' + id + '"]');
  if (btn) btn.classList.add('active');
  const titles = {
    overview: '總覽',
    narrative: '宏觀敘事',
    live: '風控結果',
    pipeline: '投資管線',
    decision: '決策鏈',
    agents: 'AI 觀測台',
    experiments: '模擬交易',
    reports: '最新回測',
    controls: '控制與稽核',
    datachannels: '信息通道',
    synergy: '人機協同',
    alerts: '系統警報',
    metrics: '指標監控',
    industry: '產業生態系',
    portfolio: '組合持倉',
    parameters: '參數管理'
  };
  document.getElementById('pageTitle').textContent = titles[id] || id;
  // Close mobile sidebar
  document.getElementById('sidebar').classList.remove('open');
  // Lazy load page data
  if (!pageLoadStatus[id]) {
    pageLoadStatus[id] = true;
    loadPageData(id);
  }
}

function toggleSidebar() {
  document.getElementById('sidebar').classList.toggle('open');
}

// --- Utilities ---
function escapeHtml(text) {
  if (typeof text !== 'string') return text;
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

const apiErrors = [];

function showAPIErrorBanner() {
  const existing = document.getElementById('apiErrorBanner');
  if (existing) existing.remove();
  if (apiErrors.length === 0) return;
  const latest = apiErrors.slice(-5);
  const banner = document.createElement('div');
  banner.id = 'apiErrorBanner';
  banner.style.cssText = 'position:fixed;top:0;left:0;right:0;z-index:9999;background:#dc2626;color:#fff;padding:8px 16px;font-size:12px;line-height:1.5;cursor:pointer';
  banner.title = '點擊關閉';
  banner.innerHTML = `<strong>⚠️ ${apiErrors.length} API 錯誤</strong> ${latest.map(e => e.url.split('/').pop() + ': ' + e.msg).join(' | ')}`;
  banner.onclick = () => { banner.remove(); apiErrors.length = 0; };
  document.body.prepend(banner);
}

async function getJSON(url) {
  try {
    const r = await fetch(url);
    if (!r.ok) throw new Error(`HTTP ${r.status}`);
    return r.json();
  } catch (err) {
    const msg = err.message || String(err);
    if (!msg.includes('404')) {
      apiErrors.push({url, msg, time: new Date()});
      showAPIErrorBanner();
      notify(`載入失敗: ${url.split('/').pop()} - ${msg}`, 'error');
    }
    throw err;
  }
}

async function postJSON(url, body) {
  try {
    const r = await fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
    if (!r.ok) throw new Error(`HTTP ${r.status}`);
    return r.json();
  } catch (err) {
    notify(`操作失敗: ${url.split('/').pop()} - ${err.message}`, 'error');
    throw err;
  }
}

function notify(msg, type='info') {
  const nc = document.getElementById('notificationCenter');
  const n = document.createElement('div');
  const typeClass = type === 'error' ? 'type-error' : type === 'warning' ? 'type-warning' : type === 'success' ? 'type-success' : 'type-info';
  const icon = type === 'error' ? '✕' : type === 'warning' ? '⚠' : type === 'success' ? '✓' : 'ℹ';
  n.className = `notification ${typeClass}`;
  n.innerHTML = `<span class="notif-icon">${icon}</span><span class="close" onclick="this.parentElement.remove()">×</span><div>${msg}</div>`;
  nc.appendChild(n);
  setTimeout(() => n.remove(), 8000);
}

function formatDate(d) { return d ? new Date(d).toLocaleString('zh-TW') : '-'; }
function fmt(num, digits=3) { return (num ?? 0).toFixed(digits); }

function initBacktestDates() {
  const today = new Date();
  const mostRecentTradingDay = new Date(today);
  while (mostRecentTradingDay.getDay() === 0 || mostRecentTradingDay.getDay() === 6) {
    mostRecentTradingDay.setDate(mostRecentTradingDay.getDate() - 1);
  }
  const fiveTradingDaysAgo = new Date(mostRecentTradingDay);
  let daysBack = 0;
  while (daysBack < 5) {
    fiveTradingDaysAgo.setDate(fiveTradingDaysAgo.getDate() - 1);
    if (fiveTradingDaysAgo.getDay() !== 0 && fiveTradingDaysAgo.getDay() !== 6) {
      daysBack++;
    }
  }
  const fmt2 = (d) => d.toISOString().split('T')[0];
  const startEl = document.getElementById('backtestStart');
  const endEl = document.getElementById('backtestEnd');
  if (startEl) startEl.value = fmt2(fiveTradingDaysAgo);
  if (endEl) endEl.value = fmt2(mostRecentTradingDay);
}

// CSV export utility
function exportTableToCSV(tableId, filename) {
  const table = document.getElementById(tableId);
  if (!table) { notify('找不到表格', 'error'); return; }
  const rows = table.querySelectorAll('tr');
  if (!rows.length) { notify('表格無資料', 'warning'); return; }
  let csv = [];
  rows.forEach(row => {
    const cells = row.querySelectorAll('th, td');
    const vals = [];
    cells.forEach(cell => {
      let text = cell.textContent.trim().replace(/"/g, '""');
      vals.push(`"${text}"`);
    });
    csv.push(vals.join(','));
  });
  const blob = new Blob(['\uFEFF' + csv.join('\n')], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename || 'export.csv';
  a.click();
  URL.revokeObjectURL(url);
  notify('CSV 匯出完成', 'success');
}

// Table pagination utility
function paginateTable(containerId, rows, pageSize=50) {
  const container = document.getElementById(containerId);
  if (!container || rows.length <= pageSize) return null;
  const totalPages = Math.ceil(rows.length / pageSize);
  const paginationDiv = document.createElement('div');
  paginationDiv.className = 'table-pagination';
  paginationDiv.innerHTML = `
    <span>顯示 <strong>1-${pageSize}</strong> / 共 <strong>${rows.length}</strong> 筆</span>
    <div style="display:flex;gap:6px">
      <button disabled id="prevPageBtn">‹ 上一頁</button>
      <span id="pageInfo" style="line-height:24px">1 / ${totalPages}</span>
      <button id="nextPageBtn">下一頁 ›</button>
    </div>
  `;
  let currentPage = 0;
  const updatePage = (page) => {
    currentPage = page;
    const start = page * pageSize;
    const end = Math.min(start + pageSize, rows.length);
    const visibleRows = rows.slice(start, end);
    container.innerHTML = '';
    visibleRows.forEach(r => container.appendChild(r));
    const info = document.getElementById('pageInfo');
    const prevBtn = document.getElementById('prevPageBtn');
    const nextBtn = document.getElementById('nextPageBtn');
    if (info) info.textContent = `${page + 1} / ${totalPages}`;
    if (prevBtn) { prevBtn.disabled = page === 0; prevBtn.onclick = () => updatePage(page - 1); }
    if (nextBtn) { nextBtn.disabled = page >= totalPages - 1; nextBtn.onclick = () => updatePage(page + 1); }
    const span = paginationDiv.querySelector('span');
    if (span) span.innerHTML = `顯示 <strong>${start + 1}-${end}</strong> / 共 <strong>${rows.length}</strong> 筆`;
  };
  paginationDiv.querySelector('#prevPageBtn').onclick = () => updatePage(currentPage - 1);
  paginationDiv.querySelector('#nextPageBtn').onclick = () => updatePage(currentPage + 1);
  updatePage(0);
  return paginationDiv;
}

// Simple markdown-to-HTML
function mdToHtml(md) {
  let html = md.replace(/</g, '&lt;');
  html = html.replace(/^# (.*$)/gim, '<h1>$1</h1>');
  html = html.replace(/^## (.*$)/gim, '<h2>$1</h2>');
  html = html.replace(/^### (.*$)/gim, '<h3>$1</h3>');
  html = html.replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>');
  html = html.replace(/`([^`]+)`/g, '<code>$1</code>');
  html = html.replace(/\n/g, '<br>');
  return html;
}

// --- Empty state & skeleton ---
function renderEmptyState(message, action, hint) {
  return `<div class="empty-state-guidance">
    <div class="icon">📭</div>
    <div class="title">${message}</div>
    ${hint ? `<div class="desc">${hint}</div>` : ''}
    ${action ? `<div class="action">${action}</div>` : ''}
  </div>`;
}

function renderSkeleton(lines=4) {
  let html = '';
  for (let i = 0; i < lines; i++) {
    const w = Math.random() * 30 + 50;
    html += `<div class="skeleton skeleton-line" style="width:${w}%"></div>`;
  }
  return `<div class="skeleton-block"></div><div style="padding:8px">${html}</div>`;
}

function showSkeletons() {
  const panels = [
    { id: 'recommendationPipeline', lines: 6 },
    { id: 'decisionChain', lines: 8 },
    { id: 'agentObservatory', lines: 5 },
    { id: 'universeOverlap', lines: 4 },
    { id: 'experimentInbox', lines: 4 },
    { id: 'backtestReport', lines: 6 },
    { id: 'aiEvolution', lines: 4 },
    { id: 'liveStatus', lines: 4 },
    { id: 'narrativeMacro', lines: 3 },
    { id: 'narrativeStress', lines: 3 },
    { id: 'narrativeEvents', lines: 5 },
    { id: 'narrativeChains', lines: 5 },
    { id: 'narrativeModels', lines: 4 },
    { id: 'dataChannels', lines: 4 },
    { id: 'alerts', lines: 4 },
  ];
  panels.forEach(p => {
    const el = document.getElementById(p.id);
    if (el && el.classList.contains('loading')) {
      el.innerHTML = renderSkeleton(p.lines);
      el.classList.remove('loading');
    }
  });
}

// --- Agent select population ---
function populateAgentSelect() {
  const agents = [
    {id:'taiwan-macro-01',name:'台灣總經'},
    {id:'foreign-flow-01',name:'外援流向'},
    {id:'semi-desk-01',name:'半導體產業桌'},
    {id:'ai-desk-01',name:'AI 供應鏈產業桌'},
    {id:'growth-momentum-01',name:'成長動能'},
    {id:'value-yield-01',name:'價值股息'},
    {id:'cro-01',name:'風控長'},
    {id:'cio-01',name:'投資長'},
    {id:'super-dru-01',name:'Druckenmiller 超級投資者'}
  ];
  const sel = document.getElementById('agentSelect');
  sel.innerHTML = agents.map(a => `<option value="${a.id}">${a.name}</option>`).join('');
}

// --- Pipeline sessions state ---
let pipelineSessions = [];

// --- Auto-refresh state ---
let autoRefreshEnabled = true;
let autoRefreshTimer = null;
let consecutiveFailures = 0;
const MAX_CONSECUTIVE_FAILURES = 3;

function toggleAutoRefresh() {
  autoRefreshEnabled = !autoRefreshEnabled;
  const pill = document.getElementById('refreshPill');
  const btn = document.getElementById('refreshToggle');
  if (autoRefreshEnabled) {
    pill.classList.remove('paused');
    btn.textContent = '⏸';
    startAutoRefresh();
    notify('自動刷新已恢復', 'success');
  } else {
    pill.classList.add('paused');
    btn.textContent = '▶';
    stopAutoRefresh();
    notify('自動刷新已暫停', 'warning');
  }
}

function startAutoRefresh() {
  stopAutoRefresh();
  autoRefreshTimer = setInterval(loadAll, 30000);
}

function stopAutoRefresh() {
  if (autoRefreshTimer) {
    clearInterval(autoRefreshTimer);
    autoRefreshTimer = null;
  }
}

function updateRefreshPill(status) {
  const pill = document.getElementById('refreshPill');
  const timeEl = document.getElementById('refreshTime');
  if (!pill || !timeEl) return;
  const now = new Date().toLocaleTimeString('zh-TW', {hour:'2-digit',minute:'2-digit',second:'2-digit'});
  timeEl.textContent = `上次更新 ${now}`;
  pill.classList.remove('paused', 'error');
  const btn = document.getElementById('refreshToggle');
  if (status === 'error') {
    pill.classList.add('error');
    timeEl.textContent = '連線異常';
    if (btn) btn.textContent = '🔄';
  } else if (!autoRefreshEnabled) {
    pill.classList.add('paused');
    timeEl.textContent = '已暫停';
    if (btn) btn.textContent = '▶';
  } else {
    if (btn) btn.textContent = '⏸';
  }
}

// --- Error banner ---
function showErrorBanner() {
  let banner = document.getElementById('errorBanner');
  if (!banner) {
    banner = document.createElement('div');
    banner.id = 'errorBanner';
    banner.className = 'error-banner';
    const content = document.getElementById('content');
    if (content) content.insertBefore(banner, content.firstChild);
  }
  banner.innerHTML = `<span>⚠ 連續 ${consecutiveFailures} 次更新失敗，請檢查網路連線</span><button class="retry-btn" onclick="retryLoad()">重試</button>`;
  banner.style.display = 'flex';
}

function hideErrorBanner() {
  const banner = document.getElementById('errorBanner');
  if (banner) banner.style.display = 'none';
}

function retryLoad() {
  consecutiveFailures = 0;
  hideErrorBanner();
  loadAll();
}

// --- Main data loader ---
async function loadAll() {
  const loadingBar = document.getElementById('loadingBar');
  if (loadingBar) loadingBar.classList.add('active');

  showSkeletons();

  try {
    const [health, macro, agents, pipeline, live, inbox, overlap, stress, events, chains, models, templates, snapshot, dataChannels, sessions, phase3, alerts, retailSentiment, capitalPhase, taxSnapshot, seasonal, regimeHistory, darwinianTrend, dataIntegrity] = await Promise.all([
      getJSON('/api/dashboard/system-health').catch(() => null),
      getJSON('/api/dashboard/macro-radar').catch(() => null),
      getJSON('/api/dashboard/agent-observatory').catch(() => null),
      getJSON('/api/dashboard/recommendation-pipeline').catch(() => null),
      getJSON('/api/dashboard/live-status').catch(() => null),
      getJSON('/api/dashboard/experiment-inbox').catch(() => null),
      getJSON('/api/dashboard/universe-overlap').catch(() => null),
      getJSON('/api/taiwan/stress-index').catch(() => null),
      getJSON('/api/narrative/events').catch(() => null),
      getJSON('/api/narrative/chains').catch(() => null),
      getJSON('/api/narrative/models').catch(() => null),
      getJSON('/api/narrative/templates').catch(() => null),
      getJSON('/api/macro/snapshot/latest').catch(() => null),
      getJSON('/api/dashboard/data-channels').catch(() => null),
      getJSON('/api/dashboard/sessions').catch(() => null),
      getJSON('/api/dashboard/phase3-status').catch(() => null),
      getJSON('/api/alerts').catch(() => null),
      getJSON('/api/dashboard/retail-sentiment').catch(() => null),
      getJSON('/api/dashboard/capital-phase').catch(() => null),
      getJSON('/api/dashboard/tax-snapshot').catch(() => null),
      getJSON('/api/narrative/seasonal').catch(() => null),
      getJSON('/api/dashboard/regime-history').catch(() => null),
      getJSON('/api/synergy/darwinian-trend').catch(() => null),
      getJSON('/api/health/data-integrity').catch(() => null),
    ]);

    const allNull = [health, macro, agents, pipeline, live, inbox, overlap, stress, events, chains, models, templates, snapshot, dataChannels, sessions, phase3, alerts, retailSentiment, capitalPhase, taxSnapshot, seasonal, regimeHistory, darwinianTrend, dataIntegrity].every(v => v === null);
    if (allNull) {
      consecutiveFailures++;
      if (consecutiveFailures >= MAX_CONSECUTIVE_FAILURES) {
        updateRefreshPill('error');
        showErrorBanner();
      }
    } else {
      consecutiveFailures = 0;
      hideErrorBanner();
      updateRefreshPill('ok');
    }

    if (dataIntegrity && dataIntegrity.overall !== 'ok') {
      const badge = dataIntegrity.overall === 'failing' ? '🔴' : '🟡';
      const warnings = (dataIntegrity.warnings || []).join('; ');
      notify(`${badge} 資料完整性: ${dataIntegrity.overall} — ${warnings || '請查看 /api/health/data-integrity'}`, 'warn');
    }

    if (sessions && sessions.sessions) {
      pipelineSessions = sessions.sessions;
    }

    // Import page modules dynamically
    const [overviewModule, narrativeModule, liveModule, pipelineModule, decisionModule, agentsModule, experimentsModule, reportsModule, controlsModule, datachannelsModule, synergyModule, alertsModule, metricsModule, industryModule] = await Promise.all([
      import('./js/pages/overview.js').catch(() => ({ renderOverview: () => {} })),
      import('./js/pages/narrative.js').catch(() => ({ renderNarrativePage: () => {} })),
      import('./js/pages/live.js').catch(() => ({ renderLiveStatus: () => {}, renderRiskCards: () => {} })),
      import('./js/pages/pipeline.js').catch(() => ({ renderPipeline: () => {} })),
      import('./js/pages/decision.js').catch(() => ({ renderDecisionChain: () => {} })),
      import('./js/pages/agents.js').catch(() => ({ renderAgentObservatory: () => {}, renderUniverseOverlap: () => {} })),
      import('./js/pages/experiments.js').catch(() => ({ renderInbox: () => {} })),
      import('./js/pages/reports.js').catch(() => ({ renderBacktestReport: () => {} })),
      import('./js/pages/controls.js').catch(() => ({ loadOverrides: () => {} })),
      import('./js/pages/datachannels.js').catch(() => ({ renderDataChannels: () => {} })),
      import('./js/pages/synergy.js').catch(() => ({ loadSynergyCronStatus: () => {} })),
      import('./js/pages/alerts.js').catch(() => ({ renderAlerts: () => {} })),
      import('./js/pages/metrics.js').catch(() => ({ loadMetrics: () => {} })),
      import('./js/pages/industry.js').catch(() => ({ loadIndustryData: () => {} })),
    ]);

    // Call render functions from page modules
    if (overviewModule.renderOverview) overviewModule.renderOverview(health, agents, inbox, overlap, events, stress, dataChannels, capitalPhase);
    if (macroModule?.renderMacroRadar) macroModule.renderMacroRadar(macro, pipeline);
    if (liveModule?.renderLiveNarrativeStrip) liveModule.renderLiveNarrativeStrip(events, stress, models, chains);
    if (agentsModule?.renderAgentObservatory) agentsModule.renderAgentObservatory(agents, overlap);
    if (agentsModule?.renderUniverseOverlap) agentsModule.renderUniverseOverlap(overlap);
    if (pipelineModule?.renderPipeline) pipelineModule.renderPipeline(pipeline, false, '');
    if (decisionModule?.renderDecisionChain) decisionModule.renderDecisionChain(pipeline, macro, agents, stress, events, chains, models, inbox, phase3, taxSnapshot, regimeHistory);
    if (experimentsModule?.renderAIEvolution) experimentsModule.renderAIEvolution(inbox, phase3);
    if (liveModule?.renderLiveStatus) liveModule.renderLiveStatus(live);
    if (experimentsModule?.renderInbox) experimentsModule.renderInbox(inbox);
    if (narrativeModule?.renderNarrativePage) narrativeModule.renderNarrativePage(snapshot, stress, events, chains, models, templates, retailSentiment, seasonal);
    if (datachannelsModule?.renderDataChannels) datachannelsModule.renderDataChannels(dataChannels);
    if (alertsModule?.renderAlerts) alertsModule.renderAlerts(alerts);
    if (metricsModule?.loadMetrics) metricsModule.loadMetrics();
    if (industryModule?.loadIndustryData) industryModule.loadIndustryData();

    if (document.getElementById('page-synergy')?.classList.contains('active')) {
      const synergyModule = await import('./js/pages/synergy.js').catch(() => ({ loadSynergyCronStatus: () => {}, loadTaskHistory: () => {} }));
      if (synergyModule.loadSynergyCronStatus) synergyModule.loadSynergyCronStatus();
      if (synergyModule.loadTaskHistory) synergyModule.loadTaskHistory();
    }

    // Pass pipelineData to renderRiskCards: CRITICAL requirement
    if (liveModule?.renderRiskCards) liveModule.renderRiskCards(live, pipeline);

  } catch (e) {
    console.error(e);
    consecutiveFailures++;
    if (consecutiveFailures >= MAX_CONSECUTIVE_FAILURES) {
      updateRefreshPill('error');
      showErrorBanner();
    }
  } finally {
    if (loadingBar) loadingBar.classList.remove('active');
  }

  // Background loads
  const reportsModule = await import('./js/pages/reports.js').catch(() => ({ renderBacktestReport: () => {} }));
  if (reportsModule?.renderBacktestReport) reportsModule.renderBacktestReport();

  const controlsModule = await import('./js/pages/controls.js').catch(() => ({ loadOverrides: () => {}, loadAuditLog: () => {}, loadExperimentHistory: () => {} }));
  if (controlsModule?.loadOverrides) controlsModule.loadOverrides();
  if (controlsModule?.loadAuditLog) controlsModule.loadAuditLog();
  if (controlsModule?.loadExperimentHistory) controlsModule.loadExperimentHistory();
}

// --- Lazy page data loader ---
async function loadPageData(pageId) {
  switch(pageId) {
    case 'narrative':
      try {
        const [snapshot, stress, events, chains, models, templates, retailSentiment, seasonal] = await Promise.all([
          getJSON('/api/macro/snapshot/latest').catch(() => null),
          getJSON('/api/taiwan/stress-index').catch(() => null),
          getJSON('/api/narrative/events').catch(() => null),
          getJSON('/api/narrative/chains').catch(() => null),
          getJSON('/api/narrative/models').catch(() => null),
          getJSON('/api/narrative/templates').catch(() => null),
          getJSON('/api/dashboard/retail-sentiment').catch(() => null),
          getJSON('/api/narrative/seasonal').catch(() => null),
        ]);
        const narrativeModule = await import('./js/pages/narrative.js').catch(() => ({ renderNarrativePage: () => {} }));
        if (narrativeModule?.renderNarrativePage) narrativeModule.renderNarrativePage(snapshot, stress, events, chains, models, templates, retailSentiment, seasonal);
      } catch (e) { console.error('Failed to load narrative:', e); }
      break;
    case 'pipeline':
      try {
        const pipeline = await getJSON('/api/dashboard/recommendation-pipeline').catch(() => null);
        const pipelineModule = await import('./js/pages/pipeline.js').catch(() => ({ renderPipeline: () => {} }));
        if (pipelineModule?.renderPipeline) pipelineModule.renderPipeline(pipeline, false, '');
      } catch (e) { console.error('Failed to load pipeline:', e); }
      break;
    case 'decision':
      try {
        const [pipeline, macro, agents, stress, events, chains, models, inbox, phase3, taxSnapshot, regimeHistory] = await Promise.all([
          getJSON('/api/dashboard/recommendation-pipeline').catch(() => null),
          getJSON('/api/dashboard/macro-radar').catch(() => null),
          getJSON('/api/dashboard/agent-observatory').catch(() => null),
          getJSON('/api/taiwan/stress-index').catch(() => null),
          getJSON('/api/narrative/events').catch(() => null),
          getJSON('/api/narrative/chains').catch(() => null),
          getJSON('/api/narrative/models').catch(() => null),
          getJSON('/api/dashboard/experiment-inbox').catch(() => null),
          getJSON('/api/dashboard/phase3-status').catch(() => null),
          getJSON('/api/dashboard/tax-snapshot').catch(() => null),
          getJSON('/api/dashboard/regime-history').catch(() => null),
        ]);
        const decisionModule = await import('./js/pages/decision.js').catch(() => ({ renderDecisionChain: () => {} }));
        if (decisionModule?.renderDecisionChain) decisionModule.renderDecisionChain(pipeline, macro, agents, stress, events, chains, models, inbox, phase3, taxSnapshot, regimeHistory);
      } catch (e) { console.error('Failed to load decision chain:', e); }
      break;
    case 'agents':
      try {
        const [agents, overlap] = await Promise.all([
          getJSON('/api/dashboard/agent-observatory').catch(() => null),
          getJSON('/api/dashboard/universe-overlap').catch(() => null),
        ]);
        const agentsModule = await import('./js/pages/agents.js').catch(() => ({ renderAgentObservatory: () => {}, renderUniverseOverlap: () => {} }));
        if (agentsModule?.renderAgentObservatory) agentsModule.renderAgentObservatory(agents, overlap);
        if (agentsModule?.renderUniverseOverlap) agentsModule.renderUniverseOverlap(overlap);
      } catch (e) { console.error('Failed to load agents:', e); }
      break;
    case 'experiments':
      try {
        const inbox = await getJSON('/api/dashboard/experiment-inbox').catch(() => null);
        const experimentsModule = await import('./js/pages/experiments.js').catch(() => ({ renderInbox: () => {}, loadAuditLog: () => {}, loadExperimentHistory: () => {} }));
        if (experimentsModule?.renderInbox) experimentsModule.renderInbox(inbox);
        if (experimentsModule?.loadAuditLog) experimentsModule.loadAuditLog();
        if (experimentsModule?.loadExperimentHistory) experimentsModule.loadExperimentHistory();
      } catch (e) { console.error('Failed to load experiments:', e); }
      break;
    case 'reports':
      try {
        const reportsModule = await import('./js/pages/reports.js').catch(() => ({ renderBacktestReport: () => {} }));
        if (reportsModule?.renderBacktestReport) reportsModule.renderBacktestReport();
      } catch (e) { console.error('Failed to load reports:', e); }
      break;
    case 'controls':
      try {
        const controlsModule = await import('./js/pages/controls.js').catch(() => ({ loadOverrides: () => {} }));
        if (controlsModule?.loadOverrides) controlsModule.loadOverrides();
      } catch (e) { console.error('Failed to load controls:', e); }
      break;
    case 'datachannels':
      try {
        const dataChannels = await getJSON('/api/dashboard/data-channels').catch(() => null);
        const datachannelsModule = await import('./js/pages/datachannels.js').catch(() => ({ renderDataChannels: () => {} }));
        if (datachannelsModule?.renderDataChannels) datachannelsModule.renderDataChannels(dataChannels);
      } catch (e) { console.error('Failed to load data channels:', e); }
      break;
    case 'synergy':
      try {
        const synergyModule = await import('./js/pages/synergy.js').catch(() => ({ loadSynergyCronStatus: () => {}, loadTaskHistory: () => {} }));
        if (synergyModule?.loadSynergyCronStatus) synergyModule.loadSynergyCronStatus();
        if (synergyModule?.loadTaskHistory) synergyModule.loadTaskHistory();
      } catch (e) { console.error('Failed to load synergy status:', e); }
      break;
    case 'alerts':
      try {
        const alerts = await getJSON('/api/alerts').catch(() => null);
        const alertsModule = await import('./js/pages/alerts.js').catch(() => ({ renderAlerts: () => {} }));
        if (alertsModule?.renderAlerts) alertsModule.renderAlerts(alerts);
      } catch (e) { console.error('Failed to load alerts:', e); }
      break;
    case 'metrics':
      try {
        const metricsModule = await import('./js/pages/metrics.js').catch(() => ({ loadMetrics: () => {} }));
        if (metricsModule?.loadMetrics) metricsModule.loadMetrics();
      } catch (e) { console.error('Failed to load metrics:', e); }
      break;
    case 'industry':
      try {
        const industryModule = await import('./js/pages/industry.js').catch(() => ({ loadIndustryData: () => {} }));
        if (industryModule?.loadIndustryData) industryModule.loadIndustryData();
      } catch (e) { console.error('Failed to load industry:', e); }
      break;
    case 'live':
      try {
        const [live, risk] = await Promise.all([
          getJSON('/api/dashboard/live-status').catch(() => null),
          getJSON('/api/dashboard/risk').catch(() => null),
        ]);
        const liveModule = await import('./js/pages/live.js').catch(() => ({ renderLiveStatus: () => {}, renderRiskCards: () => {} }));
        if (liveModule?.renderLiveStatus) liveModule.renderLiveStatus(live);
        if (liveModule?.renderRiskCards) liveModule.renderRiskCards(live, risk);
      } catch (e) { console.error('Failed to load live status:', e); }
      break;
    case 'portfolio':
      try {
        const portfolioModule = await import('./js/pages/portfolio.js').catch(() => ({ loadPortfolioPage: () => {} }));
        if (portfolioModule?.loadPortfolioPage) portfolioModule.loadPortfolioPage(getJSON, agentName);
      } catch (e) { console.error('Failed to load portfolio:', e); }
      break;
    case 'parameters':
      try {
        const paramsModule = await import('./js/pages/parameters.js').catch(() => ({ loadParameters: () => {}, loadSnapshots: () => {}, loadParamAuditLog: () => {} }));
        if (paramsModule?.loadParameters) paramsModule.loadParameters();
        if (paramsModule?.loadSnapshots) paramsModule.loadSnapshots();
        if (paramsModule?.loadParamAuditLog) paramsModule.loadParamAuditLog();
      } catch (e) { console.error('Failed to load parameters:', e); }
      break;
  }
}

// --- Initialization ---
populateAgentSelect();
initBacktestDates();
loadAll();
startAutoRefresh();

// --- Expose to window for onclick handlers in HTML ---
window.switchPage = switchPage;
window.toggleSidebar = toggleSidebar;
window.retryLoad = retryLoad;

// Export for use by other modules
export {
  pageLoadStatus,
  switchPage,
  toggleSidebar,
  loadAll,
  loadPageData,
  showErrorBanner,
  hideErrorBanner,
  retryLoad,
  consecutiveFailures,
  MAX_CONSECUTIVE_FAILURES,
  showAPIErrorBanner,
  updateRefreshPill,
  startAutoRefresh,
  stopAutoRefresh,
  toggleAutoRefresh,
  populateAgentSelect,
  initBacktestDates,
  pipelineSessions,
  renderEmptyState,
  renderSkeleton,
  showSkeletons,
  notify,
  exportTableToCSV,
  paginateTable,
  mdToHtml,
};
