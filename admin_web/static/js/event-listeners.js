// Atlas Dashboard — Event Listeners (extracted from inline onclick)
// All functions called here are on window.* (set by main.js during module load).

document.addEventListener('DOMContentLoaded', () => {

  // === Sidebar Navigation (22 pages) ===
  document.querySelectorAll('#sidebar nav a[data-page]').forEach(a => {
    a.addEventListener('click', () => window.switchPage(a.dataset.page));
  });

  // === Global page navigation (inline links with data-page, e.g. alert health panel)
  document.addEventListener('click', (e) => {
    const link = e.target.closest('a[data-page]');
    if (link && !link.closest('#sidebar nav')) {
      e.preventDefault();
      window.switchPage(link.dataset.page);
    }
  });

  // === Global/Core ===
  var qs___theme_toggle_ = document.querySelector('.theme-toggle'); if (qs___theme_toggle_) qs___theme_toggle_.addEventListener('click', () => window.toggleTheme());
  var el_menuToggle = document.getElementById('menuToggle'); if (el_menuToggle) el_menuToggle.addEventListener('click', () => window.toggleSidebar());
  var el_refreshToggle = document.getElementById('refreshToggle'); if (el_refreshToggle) el_refreshToggle.addEventListener('click', () => window.toggleAutoRefresh());
  var qs__button_refresh_ = document.querySelector('button.refresh'); if (qs__button_refresh_) qs__button_refresh_.addEventListener('click', () => window.loadAll());

  // === Page: home — KPI help cards ===
  document.querySelectorAll('#page-home .kpi-card.clickable[data-kpi]').forEach(card => {
    card.addEventListener('click', () => window.openKpiHelp(card.dataset.kpi));
  });

  // === Page: reports (backtest) ===
  var qs___page_reports__primary_ = document.querySelector('#page-reports .primary'); if (qs___page_reports__primary_) qs___page_reports__primary_.addEventListener('click', () => window.runBacktest());

  // === Page: datachannels ===
  var el_btnIngestChannels = document.getElementById('btnIngestChannels'); if (el_btnIngestChannels) el_btnIngestChannels.addEventListener('click', () => window.triggerChannelsIngest());
  // safeCall bridges the race between DOMContentLoaded (when these listeners
  // attach) and module-load completion (when window.dcEnableAll et al. are set
  // in main.js loadModules). Without it, a fast click hits
  // `TypeError: window.dcEnableAll is not a function` and the user perceives the
  // button as broken.
  const safeCall = async (fnName, ...args) => {
    if (typeof window[fnName] === 'function') return window[fnName](...args);
    await window.__modulesReady;
    const fn = window[fnName];
    if (typeof fn !== 'function') throw new Error(`${fnName} not available after module load`);
    return fn(...args);
  };
  var qs___page_datachannels__data_acti = document.querySelector('#page-datachannels [data-action="dc-enable-all"]'); if (qs___page_datachannels__data_acti) qs___page_datachannels__data_acti.addEventListener('click', () => safeCall('dcEnableAll'));
  var qs___page_datachannels__data_acti = document.querySelector('#page-datachannels [data-action="dc-disable-all"]'); if (qs___page_datachannels__data_acti) qs___page_datachannels__data_acti.addEventListener('click', () => safeCall('dcDisableAll'));
  var qs___page_datachannels__data_acti = document.querySelector('#page-datachannels [data-action="dc-refresh"]'); if (qs___page_datachannels__data_acti) qs___page_datachannels__data_acti.addEventListener('click', () => safeCall('refreshChannelStatus'));
  // API Key update buttons
  document.querySelectorAll('#page-datachannels [data-provider]').forEach(btn => {
    btn.addEventListener('click', () => window.dcUpdateApiKey(btn.dataset.provider));
  });

  // === Page: alerts ===
  var qs___page_alerts__data_action__lo = document.querySelector('#page-alerts [data-action="load-alerts"]'); if (qs___page_alerts__data_action__lo) qs___page_alerts__data_action__lo.addEventListener('click', () => window.loadAlerts());
  var qs___page_alerts__data_action__lo = document.querySelector('#page-alerts [data-action="load-all-alerts"]'); if (qs___page_alerts__data_action__lo) qs___page_alerts__data_action__lo.addEventListener('click', () => window.loadAllAlerts());
  var qs___page_alerts__data_action__sh = document.querySelector('#page-alerts [data-action="show-unacknowledged"]'); if (qs___page_alerts__data_action__sh) qs___page_alerts__data_action__sh.addEventListener('click', () => window.showUnacknowledgedOnly());
  document.querySelectorAll('#alertFilters .view-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      if (window.setAlertFilter) window.setAlertFilter(btn.dataset.filter);
    });
  });

  // === Modals: diffModal ===
  var el_diffModal = document.getElementById('diffModal'); if (el_diffModal) el_diffModal.addEventListener('click', (e) => {
    if (e.target === e.currentTarget) window.closeModal();
  });
  var qs___diffModal__control_group_but = document.querySelector('#diffModal .control-group button'); if (qs___diffModal__control_group_but) qs___diffModal__control_group_but.addEventListener('click', () => window.closeModal());

  // === Modals: promoteModal ===
  var el_promoteModal = document.getElementById('promoteModal'); if (el_promoteModal) el_promoteModal.addEventListener('click', (e) => {
    if (e.target === e.currentTarget) window.closePromoteModal();
  });
  var qs___promoteModal__button_not_primary_ = document.querySelector('#promoteModal button:not(.primary)'); if (qs___promoteModal__button_not_primary_) qs___promoteModal__button_not_primary_.addEventListener('click', () => window.closePromoteModal());
  var qs___promoteModal__primary_ = document.querySelector('#promoteModal .primary'); if (qs___promoteModal__primary_) qs___promoteModal__primary_.addEventListener('click', () => window.confirmPromote());

  // === Modals: infoModal ===
  var el_infoModal = document.getElementById('infoModal'); if (el_infoModal) el_infoModal.addEventListener('click', (e) => {
    if (e.target === e.currentTarget) window.closeInfoModal();
  });
  var qs___infoModal__control_group_but = document.querySelector('#infoModal .control-group button'); if (qs___infoModal__control_group_but) qs___infoModal__control_group_but.addEventListener('click', () => window.closeInfoModal());

});
