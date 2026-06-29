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
    // === L2.4 schedule panel buttons (synergy page) ===
    const btn = e.target.closest('button');
    if (btn && btn.id && btn.id.startsWith('l24')) {
      const panel = document.getElementById('synergyL24Schedule');
      const showError = (msg) => {
        let errDiv = panel ? panel.querySelector('.error-msg') : null;
        if (!errDiv && panel) {
          errDiv = document.createElement('div');
          errDiv.className = 'error-msg';
          errDiv.style.color = 'var(--color-danger)';
          errDiv.style.marginTop = '8px';
          errDiv.style.fontSize = '12px';
          panel.appendChild(errDiv);
        }
        if (errDiv) {
          errDiv.textContent = msg;
          setTimeout(() => errDiv.remove(), 5000);
        }
      };
      const doReq = async (url, method, body) => {
        const opts = { method };
        if (body !== undefined && body !== null) {
          opts.headers = { 'Content-Type': 'application/json' };
          opts.body = JSON.stringify(body);
        }
        const res = await fetch(url, opts);
        if (!res.ok) {
          const text = await res.text();
          throw new Error(url + ': ' + res.status + ' ' + text.slice(0, 200));
        }
        return res.json();
      };
      (async () => {
        try {
          if (btn.id === 'l24StartBtn') await doReq('/api/synergy/l2-4-schedule/start', 'POST');
          else if (btn.id === 'l24StopBtn') await doReq('/api/synergy/l2-4-schedule/stop', 'POST');
          else if (btn.id === 'l24ResetBtn') await doReq('/api/synergy/l2-4-schedule/reset', 'POST');
          else if (btn.id === 'l24SaveBtn') {
            const t = (document.getElementById('l24OverrideStart') || {}).value || '';
            const p = parseInt((document.getElementById('l24OverridePeriod') || {}).value, 10);
            if (!t) throw new Error('時間不可為空');
            if (isNaN(p) || p < 1 || p > 30) throw new Error('週期必須在 1-30 之間');
            await doReq('/api/synergy/l2-4-schedule', 'PUT', { override_start_time: t, override_period_days: p });
          }
          const state = await doReq('/api/synergy/l2-4-schedule', 'GET');
          if (state && window.m && window.m.synergy && window.m.synergy.renderL24Schedule) {
            window.m.synergy.renderL24Schedule(state);
          }
          if (panel) {
            panel.classList.remove('l24-flash');
            void panel.offsetWidth;
            panel.classList.add('l24-flash');
          }
        } catch (err) {
          showError(err.message);
        }
      })();
    }
  });

  // === Global/Core ===
  var qs___theme_toggle_ = document.querySelector('.theme-toggle'); if (qs___theme_toggle_) qs___theme_toggle_.addEventListener('click', () => window.toggleTheme());
  var el_menuToggle = document.getElementById('menuToggle'); if (el_menuToggle) el_menuToggle.addEventListener('click', () => window.toggleSidebar());
  var el_refreshToggle = document.getElementById('refreshToggle'); if (el_refreshToggle) el_refreshToggle.addEventListener('click', () => window.toggleAutoRefresh());
  var qs__button_refresh_ = document.querySelector('button.refresh'); if (qs__button_refresh_) qs__button_refresh_.addEventListener('click', () => window.loadAll());

  // === Page: overview — KPI help cards ===
  document.querySelectorAll('#page-overview .kpi-card.clickable[data-kpi]').forEach(card => {
    card.addEventListener('click', () => window.openKpiHelp(card.dataset.kpi));
  });

  // === Page: reports (backtest) ===
  var qs___page_reports__primary_ = document.querySelector('#page-reports .primary'); if (qs___page_reports__primary_) qs___page_reports__primary_.addEventListener('click', () => window.runBacktest());

  // === Page: controls ===
  var qs___page_controls__data_action__ = document.querySelector('#page-controls [data-action="pause-agent"]'); if (qs___page_controls__data_action__) qs___page_controls__data_action__.addEventListener('click', () => window.pauseAgent());
  var qs___page_controls__data_action__ = document.querySelector('#page-controls [data-action="resume-agent"]'); if (qs___page_controls__data_action__) qs___page_controls__data_action__.addEventListener('click', () => window.resumeAgent());
  var qs___page_controls__data_action__ = document.querySelector('#page-controls [data-action="ban-sector"]'); if (qs___page_controls__data_action__) qs___page_controls__data_action__.addEventListener('click', () => window.banSector());
  var qs___page_controls__data_action__ = document.querySelector('#page-controls [data-action="unban-sector"]'); if (qs___page_controls__data_action__) qs___page_controls__data_action__.addEventListener('click', () => window.unbanSector());
  var qs___page_controls__data_action__ = document.querySelector('#page-controls [data-action="promote-experiment"]'); if (qs___page_controls__data_action__) qs___page_controls__data_action__.addEventListener('click', () => window.promoteExperiment());
  var qs___page_controls__data_action__ = document.querySelector('#page-controls [data-action="revert-experiment"]'); if (qs___page_controls__data_action__) qs___page_controls__data_action__.addEventListener('click', () => window.revertExperiment());

  // === Page: evolution_panel ===
    var el_evView_compact = document.getElementById('evView-compact'); if (el_evView_compact) el_evView_compact.addEventListener('click', () => window._evSwitch('compact'));
    var el_evView_ai_analysis = document.getElementById('evView-ai-analysis'); if (el_evView_ai_analysis) el_evView_ai_analysis.addEventListener('click', () => window._evSwitch('ai-analysis'));

  // === Page: datachannels ===
  var el_btnIngestChannels = document.getElementById('btnIngestChannels'); if (el_btnIngestChannels) el_btnIngestChannels.addEventListener('click', () => window.triggerChannelsIngest());
  var qs___page_datachannels__data_acti = document.querySelector('#page-datachannels [data-action="dc-enable-all"]'); if (qs___page_datachannels__data_acti) qs___page_datachannels__data_acti.addEventListener('click', () => window.dcEnableAll());
  var qs___page_datachannels__data_acti = document.querySelector('#page-datachannels [data-action="dc-disable-all"]'); if (qs___page_datachannels__data_acti) qs___page_datachannels__data_acti.addEventListener('click', () => window.dcDisableAll());
  var qs___page_datachannels__data_acti = document.querySelector('#page-datachannels [data-action="dc-refresh"]'); if (qs___page_datachannels__data_acti) qs___page_datachannels__data_acti.addEventListener('click', () => window.loadDataChannels());
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

  // === Page: industry ===
  var qs___page_industry__cursor_pointe = document.querySelector('#page-industry .cursor-pointer'); if (qs___page_industry__cursor_pointe) qs___page_industry__cursor_pointe.addEventListener('click', () => window.toggleCycleLegend());
  var el_btnRunShockSim = document.getElementById('btnRunShockSim'); if (el_btnRunShockSim) el_btnRunShockSim.addEventListener('click', () => window.runShockSimulation());

  // === Page: synergy ===
  var el_btnCalibrateThresholds = document.getElementById('btnCalibrateThresholds'); if (el_btnCalibrateThresholds) el_btnCalibrateThresholds.addEventListener('click', () => {
    fetch('/api/admin/calibrate-thresholds', { method: 'POST' })
      .then(r => r.json())
      .then(d => alert(d.status || d.error || '完成'))
      .catch(e => alert('校正失敗: ' + e));
  });
  var qs___page_synergy__data_page__par = document.querySelector('#page-synergy [data-page="parameters"]'); if (qs___page_synergy__data_page__par) qs___page_synergy__data_page__par.addEventListener('click', () => window.switchPage('parameters'));

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

  // === Modals: industryModal ===
  var el_industryModal = document.getElementById('industryModal'); if (el_industryModal) el_industryModal.addEventListener('click', (e) => {
    if (e.target === e.currentTarget) window.closeIndustryModal();
  });
  document.querySelectorAll('#industryTabs .tab-btn').forEach(btn => {
    btn.addEventListener('click', () => window.switchIndustryTab(btn.dataset.tab));
  });
  var qs___industryModal__control_group = document.querySelector('#industryModal .control-group button'); if (qs___industryModal__control_group) qs___industryModal__control_group.addEventListener('click', () => window.closeIndustryModal());

  // === Modals: cycleLegendModal ===
  var el_cycleLegendModal = document.getElementById('cycleLegendModal'); if (el_cycleLegendModal) el_cycleLegendModal.addEventListener('click', (e) => {
    if (e.target === e.currentTarget) window.closeCycleLegend();
  });
  var qs___cycleLegendModal__control_gr = document.querySelector('#cycleLegendModal .control-group button'); if (qs___cycleLegendModal__control_gr) qs___cycleLegendModal__control_gr.addEventListener('click', () => window.closeCycleLegend());

});
