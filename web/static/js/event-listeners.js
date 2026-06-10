// Atlas Dashboard — Event Listeners (extracted from inline onclick)
// All functions called here are on window.* (set by main.js during module load).

document.addEventListener('DOMContentLoaded', () => {

  // === Sidebar Navigation (22 pages) ===
  document.querySelectorAll('#sidebar nav a[data-page]').forEach(a => {
    a.addEventListener('click', () => window.switchPage(a.dataset.page));
  });

  // === Global/Core ===
  document.querySelector('.theme-toggle').addEventListener('click', () => window.toggleTheme());
  document.getElementById('menuToggle').addEventListener('click', () => window.toggleSidebar());
  document.getElementById('refreshToggle').addEventListener('click', () => window.toggleAutoRefresh());
  document.querySelector('button.refresh').addEventListener('click', () => window.loadAll());

  // === Page: overview — KPI help cards ===
  document.querySelectorAll('#page-overview .kpi-card.clickable[data-kpi]').forEach(card => {
    card.addEventListener('click', () => window.openKpiHelp(card.dataset.kpi));
  });

  // === Page: pipeline ===
  document.getElementById('workflowScreening').addEventListener('click', () => window.toggleWorkflowScreening());
  document.getElementById('filterToggle').addEventListener('click', () => window.toggleFilterPanel());
  document.querySelector('.filter-actions .primary').addEventListener('click', () => window.applyFilters());
  document.querySelector('.filter-actions button:not(.primary)').addEventListener('click', () => window.clearFilters());

  // === Page: reports (backtest) ===
  document.querySelector('#page-reports .primary').addEventListener('click', () => window.runBacktest());

  // === Page: controls ===
  document.querySelector('#page-controls [data-action="pause-agent"]').addEventListener('click', () => window.pauseAgent());
  document.querySelector('#page-controls [data-action="resume-agent"]').addEventListener('click', () => window.resumeAgent());
  document.querySelector('#page-controls [data-action="ban-sector"]').addEventListener('click', () => window.banSector());
  document.querySelector('#page-controls [data-action="unban-sector"]').addEventListener('click', () => window.unbanSector());
  document.querySelector('#page-controls [data-action="promote-experiment"]').addEventListener('click', () => window.promoteExperiment());
  document.querySelector('#page-controls [data-action="revert-experiment"]').addEventListener('click', () => window.revertExperiment());

  // === Page: evolution_panel ===
  document.getElementById('evView-compact').addEventListener('click', () => window._evSwitch('compact'));
  document.getElementById('evView-detailed').addEventListener('click', () => window._evSwitch('detailed'));
  document.getElementById('evView-categorical').addEventListener('click', () => window._evSwitch('categorical'));

  // === Page: datachannels ===
  document.getElementById('btnIngestChannels').addEventListener('click', () => window.triggerChannelsIngest());
  document.querySelector('#page-datachannels [data-action="dc-enable-all"]').addEventListener('click', () => window.dcEnableAll());
  document.querySelector('#page-datachannels [data-action="dc-disable-all"]').addEventListener('click', () => window.dcDisableAll());
  document.querySelector('#page-datachannels [data-action="dc-refresh"]').addEventListener('click', () => window.loadDataChannels());
  // API Key update buttons
  document.querySelectorAll('#page-datachannels [data-provider]').forEach(btn => {
    btn.addEventListener('click', () => window.dcUpdateApiKey(btn.dataset.provider));
  });

  // === Page: alerts ===
  document.querySelector('#page-alerts [data-action="load-alerts"]').addEventListener('click', () => window.loadAlerts());
  document.querySelector('#page-alerts [data-action="show-unacknowledged"]').addEventListener('click', () => window.showUnacknowledgedOnly());
  document.querySelectorAll('#alertFilters .alert-filter-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      if (window.setAlertFilter) window.setAlertFilter(btn.dataset.filter);
    });
  });

  // === Page: industry ===
  document.querySelector('#page-industry .cursor-pointer').addEventListener('click', () => window.toggleCycleLegend());
  document.getElementById('btnRunShockSim').addEventListener('click', () => window.runShockSimulation());

  // === Page: synergy ===
  document.getElementById('btnCalibrateThresholds').addEventListener('click', () => {
    fetch('/api/admin/calibrate-thresholds', { method: 'POST' })
      .then(r => r.json())
      .then(d => alert(d.status || d.error || '完成'))
      .catch(e => alert('校正失敗: ' + e));
  });
  document.querySelector('#page-synergy [data-page="parameters"]').addEventListener('click', () => window.switchPage('parameters'));

  // === Modals: diffModal ===
  document.getElementById('diffModal').addEventListener('click', (e) => {
    if (e.target === e.currentTarget) window.closeModal();
  });
  document.querySelector('#diffModal .control-group button').addEventListener('click', () => window.closeModal());

  // === Modals: promoteModal ===
  document.getElementById('promoteModal').addEventListener('click', (e) => {
    if (e.target === e.currentTarget) window.closePromoteModal();
  });
  document.querySelector('#promoteModal button:not(.primary)').addEventListener('click', () => window.closePromoteModal());
  document.querySelector('#promoteModal .primary').addEventListener('click', () => window.confirmPromote());

  // === Modals: infoModal ===
  document.getElementById('infoModal').addEventListener('click', (e) => {
    if (e.target === e.currentTarget) window.closeInfoModal();
  });
  document.querySelector('#infoModal .control-group button').addEventListener('click', () => window.closeInfoModal());

  // === Modals: industryModal ===
  document.getElementById('industryModal').addEventListener('click', (e) => {
    if (e.target === e.currentTarget) window.closeIndustryModal();
  });
  document.querySelectorAll('#industryTabs .tab-btn').forEach(btn => {
    btn.addEventListener('click', () => window.switchIndustryTab(btn.dataset.tab));
  });
  document.querySelector('#industryModal .control-group button').addEventListener('click', () => window.closeIndustryModal());

  // === Modals: cycleLegendModal ===
  document.getElementById('cycleLegendModal').addEventListener('click', (e) => {
    if (e.target === e.currentTarget) window.closeCycleLegend();
  });
  document.querySelector('#cycleLegendModal .control-group button').addEventListener('click', () => window.closeCycleLegend());

});
