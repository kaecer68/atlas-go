// shared_web/static/js/pages/capital-causality.js
//
// Stage 6 PR#2：admin_web「錢潮因果」頁。
// 從 GET /api/narrative/templates 讀因果模板，依 trigger_theme 篩選，
// 以 <details> 展開每個 template 的因果步驟。

import { silentGetJSON, escapeHtml, renderEmptyState, renderErrorState } from '../shared/app-utils.js';
import { templateName } from '../names.js';
import { narrativeThemeLabel } from '../shared/constants.js';
import { fmtSafePct } from '../shared/format-metric.js';

let _allTemplates = [];
let _allModels = [];
let _activeTheme = 'all';
const RETRY_ID = 'capital-causality';

// 從 URL ?theme= 讀初始主題（admin capital_models 跨 SPA 跳轉時帶入）。
function readInitialThemeFromUrl() {
  try {
    const params = new URLSearchParams(window.location.search || '');
    return params.get('theme') || null;
  } catch (e) {
    return null;
  }
}

export async function loadCapitalCausality() {
  const el = document.getElementById('capitalCausalityContent');
  if (!el) return;

  // Cross-page navigation from capital_models theme chip: apply the
  // requested theme as the initial filter, then clear the pending flag.
  if (window._pendingCausalityFilter) {
    _activeTheme = window._pendingCausalityFilter;
    delete window._pendingCausalityFilter;
  } else if (readInitialThemeFromUrl()) {
    _activeTheme = readInitialThemeFromUrl();
  }

  try {
    // Fetch templates + active models in parallel so the causality page can
    // show which trigger themes have a corresponding investment model.
    const results = await Promise.all([
      silentGetJSON('/api/narrative/templates'),
      silentGetJSON('/api/narrative/models'),
    ]);
    const data = results[0];
    if (data === null) {
      el.classList.remove('loading');
      el.innerHTML = renderErrorState('錢潮因果', RETRY_ID);
      const btn = el.querySelector('[data-retry="' + RETRY_ID + '"]');
      if (btn) btn.addEventListener('click', loadCapitalCausality);
      return;
    }
    const models = results[1];
    renderCapitalCausality(data, models);
  } catch (err) {
    console.error('[capital-causality] load failed', err);
    el.classList.remove('loading');
    el.innerHTML = renderErrorState('錢潮因果', RETRY_ID);
    const btn = el.querySelector('[data-retry="' + RETRY_ID + '"]');
    if (btn) btn.addEventListener('click', loadCapitalCausality);
  }
}

export function renderCapitalCausality(data, modelsData) {
  const el = document.getElementById('capitalCausalityContent');
  if (!el) return;
  _allTemplates = data && Array.isArray(data.templates) ? data.templates : [];
  _allModels = modelsData && Array.isArray(modelsData.models) ? modelsData.models : [];
  if (_allTemplates.length === 0) {
    el.classList.remove('loading');
    el.innerHTML = renderEmptyState('尚無因果模板');
    return;
  }

  const themes = collectThemes(_allTemplates);
  const filterHtml = buildThemeFilter(themes, _activeTheme);

  el.classList.remove('loading');
  el.innerHTML = filterHtml + '<div id="cc-list" class="cc-list"></div>';
  renderTemplateList();

  const select = el.querySelector('#cc-theme-filter');
  if (select) {
    select.addEventListener('change', function () {
      _activeTheme = select.value;
      renderTemplateList();
    });
  }
}

function collectThemes(templates) {
  const set = new Set();
  templates.forEach(function (t) {
    if (t.trigger_theme) set.add(t.trigger_theme);
  });
  return Array.from(set).sort();
}

function buildThemeFilter(themes, active) {
  const options = themes.map(function (theme) {
    return '<option value="' + escapeHtml(theme) + '"' + (theme === active ? ' selected' : '') + '>'
      + escapeHtml(narrativeThemeLabel(theme)) + '</option>';
  }).join('');
  return (
    '<div class="cc-filter">'
    + '<label for="cc-theme-filter">主題篩選</label>'
    + '<select id="cc-theme-filter">'
    +   '<option value="all"' + (active === 'all' ? ' selected' : '') + '>全部</option>'
    +   options
    + '</select>'
    + '<span class="text-muted text-sm">共 ' + _allTemplates.length + ' 個模板</span>'
    + '</div>'
  );
}

function renderTemplateList() {
  const list = document.getElementById('cc-list');
  if (!list) return;

  const filtered = _activeTheme === 'all'
    ? _allTemplates
    : _allTemplates.filter(function (t) { return t.trigger_theme === _activeTheme; });

  if (filtered.length === 0) {
    list.innerHTML = '<div class="cc-empty">此主題尚無模板</div>';
    return;
  }

  // modelsByTheme: trigger theme → active model names (active_themes match).
  const modelsByTheme = {};
  _allModels.forEach(function (m) {
    (m.active_themes || []).forEach(function (theme) {
      if (!modelsByTheme[theme]) modelsByTheme[theme] = [];
      if (modelsByTheme[theme].indexOf(m.name) === -1) modelsByTheme[theme].push(m.name);
    });
  });

  list.innerHTML = filtered.map(function (t) {
    const hitRate = t.hit_rate != null ? t.hit_rate : t.historical_hit_rate;
    const steps = Array.isArray(t.steps) && t.steps.length > 0
      ? t.steps.map(function (s) {
          const label = typeof s === 'string'
            ? s
            : escapeHtml(s.description || s.label || s.text || JSON.stringify(s));
          const affected = Array.isArray(s.affected) && s.affected.length
            ? ' <span class="text-muted">→ ' + s.affected.map(function (a) { return escapeHtml(a); }).join('、') + '</span>'
            : '';
          return '<li>' + label + affected + '</li>';
        }).join('')
      : '<li>（未定義步驟）</li>';
    const modelNames = (t.trigger_theme && modelsByTheme[t.trigger_theme]) || [];
    const modelBadge = modelNames.length
      ? '<span class="badge cc-model-badge" title="此主題有對應投資模型，可跳回錢潮模型檢視">'
          + '對應模型：' + escapeHtml(modelNames.join('、'))
          + '</span>'
      : '';
    return (
      '<details class="cc-item">'
      + '<summary class="cc-item__summary">'
      +   '<span>' + escapeHtml(templateName(t.name || t.id || '-')) + '</span>'
      +   '<span class="badge info">命中率 ' + fmtSafePct(hitRate, 1) + '</span>'
      +   modelBadge
      + '</summary>'
      + '<div class="cc-item__body">'
      +   '<div class="text-muted">' + escapeHtml(t.rationale || '尚無說明') + '</div>'
      +   '<h4 style="margin:12px 0 4px;font-size:12px;color:var(--muted)">因果步驟</h4>'
      +   '<ol class="cc-item__steps">' + steps + '</ol>'
      +   '<div class="cc-item__meta">'
      +     (t.trigger_theme ? '<span class="badge muted">' + escapeHtml(narrativeThemeLabel(t.trigger_theme)) + '</span>' : '')
      +     (t.required_region ? '<span class="badge muted">地區：' + escapeHtml(t.required_region) + '</span>' : '')
      +     (Array.isArray(t.source_references) && t.source_references.length
            ? '<span class="badge muted">來源：' + escapeHtml(t.source_references.join('、')) + '</span>'
            : '')
      +   '</div>'
      + '</div>'
      + '</details>'
    );
  }).join('');

  // Wire model badge clicks: jump back to capital_models page.
  list.querySelectorAll('.cc-model-badge').forEach(function (badge) {
    badge.addEventListener('click', function () {
      if (typeof window.switchPage === 'function') {
        window.switchPage('capital_models');
      }
    });
  });
}
