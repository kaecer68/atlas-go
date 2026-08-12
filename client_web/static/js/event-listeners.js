// Atlas Dashboard — Event Listeners (extracted from inline onclick)
// All functions called here are on window.* (set by main.js during module load).

import { logout, renderNavState } from './services/auth.js';

document.addEventListener('DOMContentLoaded', () => {

  // === Sidebar Navigation (22 pages) ===
  document.querySelectorAll('#sidebar nav a[data-page]').forEach(a => {
    a.addEventListener('click', (e) => {
      e.preventDefault();
      window.switchPage(a.dataset.page);
    });
  });

  // === Sidebar Logout ===
  const logoutBtn = document.getElementById('navLogoutBtn');
  if (logoutBtn) {
    logoutBtn.addEventListener('click', async (e) => {
      e.preventDefault();
      await logout();
      await renderNavState();
      window.location.hash = '#home';
    });
  }

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

  // === Page: pipeline ===
  var el_workflowScreening = document.getElementById('workflowScreening'); if (el_workflowScreening) el_workflowScreening.addEventListener('click', () => window.toggleWorkflowScreening());
  var el_filterToggle = document.getElementById('filterToggle'); if (el_filterToggle) el_filterToggle.addEventListener('click', () => window.toggleFilterPanel());
  var qs___filter_actions__primary_ = document.querySelector('.filter-actions .primary'); if (qs___filter_actions__primary_) qs___filter_actions__primary_.addEventListener('click', () => window.applyFilters());
  var qs___filter_actions__button_not_primary_ = document.querySelector('.filter-actions button:not(.primary)'); if (qs___filter_actions__button_not_primary_) qs___filter_actions__button_not_primary_.addEventListener('click', () => window.clearFilters());

  // === Page: industry ===
  var qs___page_industry__cursor_pointe = document.querySelector('#page-industry .cursor-pointer'); if (qs___page_industry__cursor_pointe) qs___page_industry__cursor_pointe.addEventListener('click', () => window.toggleCycleLegend());
  var el_btnRunShockSim = document.getElementById('btnRunShockSim'); if (el_btnRunShockSim) el_btnRunShockSim.addEventListener('click', () => window.runShockSimulation());

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
