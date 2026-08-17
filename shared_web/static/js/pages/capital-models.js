// shared_web/static/js/pages/capital-models.js
//
// Stage 6 PR#2：admin_web「錢潮模型」頁。
// 從 GET /api/narrative/models 讀 narrative engine 的 active models，
// 以卡片呈現權重、近期誤差、命中率與最近訊號；點擊卡片展開詳細推理。

import { silentGetJSON, escapeHtml, renderEmptyState, renderErrorState } from '../shared/app-utils.js';
import { modelName, sectorName } from '../names.js';
import { fmtSafePct, fmtSafeNumber } from '../shared/format-metric.js';

const RETRY_ID = 'capital-models';

export async function loadCapitalModels() {
  const el = document.getElementById('capitalModelsContent');
  if (!el) return;
  try {
    const data = await silentGetJSON('/api/narrative/models');
    if (data === null) {
      el.classList.remove('loading');
      el.innerHTML = renderErrorState('錢潮模型', RETRY_ID);
      const btn = el.querySelector('[data-retry="' + RETRY_ID + '"]');
      if (btn) btn.addEventListener('click', loadCapitalModels);
      return;
    }
    renderCapitalModels(data);
  } catch (err) {
    console.error('[capital-models] load failed', err);
    el.classList.remove('loading');
    el.innerHTML = renderErrorState('錢潮模型', RETRY_ID);
    const btn = el.querySelector('[data-retry="' + RETRY_ID + '"]');
    if (btn) btn.addEventListener('click', loadCapitalModels);
  }
}

export function renderCapitalModels(data) {
  const el = document.getElementById('capitalModelsContent');
  if (!el) return;
  const models = data && Array.isArray(data.models) ? data.models : [];
  if (models.length === 0) {
    el.classList.remove('loading');
    el.innerHTML = renderEmptyState('目前沒有 narrative model 啟用');
    return;
  }

  const totalWeight = models.reduce(function (sum, m) { return sum + (typeof m.weight === 'number' ? m.weight : 0); }, 0);

  const cards = models.map(function (m, idx) {
    const weight = typeof m.weight === 'number' ? m.weight : 0;
    const pct = totalWeight > 0 ? (weight / totalWeight) * 100 : 0;
    const hitRate = m.hit_rate;
    const hitDisplay = isNoDataHitRate(hitRate)
      ? '<span class="text-muted">無資料</span>'
      : fmtSafePct(hitRate, 1);
    const lastSignal = signalLabel(m.recent_prediction);
    const recentError = fmtSafeNumber(m.recent_error, { decimals: 3 });

    return (
      '<div class="cm-card" data-idx="' + idx + '">'
      + '<div class="cm-card__head">'
      +   '<span class="cm-card__name">' + escapeHtml(modelName(m.name || m.id || '未命名')) + '</span>'
      +   '<span class="cm-card__weight" title="各模型權重佔總權重比例（合計 100%）">相對權重 ' + pct.toFixed(1) + '%</span>'
      + '</div>'
      + '<div class="cm-card__bar">'
      +   '<div class="cm-card__bar-fill" style="width:' + pct.toFixed(1) + '%;background:var(--accent)"></div>'
      + '</div>'
      + '<div class="cm-card__metrics">'
      +   '<div class="cm-card__metric">'
      +     '<div class="cm-card__metric-label">近期誤差</div>'
      +     '<div class="cm-card__metric-value">' + recentError + '</div>'
      +   '</div>'
      +   '<div class="cm-card__metric">'
      +     '<div class="cm-card__metric-label">歷史命中率</div>'
      +     '<div class="cm-card__metric-value">' + hitDisplay + '</div>'
      +   '</div>'
      +   '<div class="cm-card__metric">'
      +     '<div class="cm-card__metric-label">最近訊號</div>'
      +     '<div class="cm-card__metric-value">' + lastSignal + '</div>'
      +   '</div>'
      + '</div>'
      + (typeof m.sample_count === 'number' && m.sample_count < 20
          ? '<div class="cm-card__badge">樣本不足（' + m.sample_count + ' 筆）</div>'
          : '')
      + '<div class="cm-card__detail" id="cm-detail-' + idx + '">'
      +   '<div class="cm-card__rationale">' + escapeHtml(m.rationale || '尚無推理依據') + '</div>'
      +   (m.description ? '<div class="text-muted">' + escapeHtml(m.description) + '</div>' : '')
      +   '<h4>看好板塊</h4>'
      +   renderSectorChips(m.favored_sectors)
      +   '<h4>迴避板塊</h4>'
      +   renderSectorChips(m.avoided_sectors)
      +   (m.active_themes && m.active_themes.length ? '<h4>活躍主題</h4>' + renderThemeChips(m.active_themes) : '')
      + '</div>'
      + '</div>'
    );
  }).join('');

  el.classList.remove('loading');
  el.innerHTML = (
    '<div class="cm-models">' + cards + '</div>'
    + '<div class="cm-sum">絕對權重合計：<strong>' + fmtSafePct(totalWeight, 1) + '</strong>（共 ' + models.length + ' 個模型）</div>'
  );

  el.querySelectorAll('.cm-card').forEach(function (card) {
    card.addEventListener('click', function () {
      const idx = card.getAttribute('data-idx');
      const detail = document.getElementById('cm-detail-' + idx);
      if (detail) detail.classList.toggle('open');
    });
  });
  wireThemeChips(el);
}

function signalLabel(value) {
  if (typeof value !== 'number' || Number.isNaN(value)) return '—';
  if (value > 0) return '看漲';
  if (value < 0) return '看跌';
  return '中性';
}

function isNoDataHitRate(hitRate) {
  if (hitRate === null || hitRate === undefined || Number.isNaN(hitRate)) return true;
  if (typeof hitRate === 'number' && (hitRate === 0 || hitRate === 0 / 1)) return true;
  if (typeof hitRate === 'string' && (hitRate === '0' || hitRate === '0/0')) return true;
  return false;
}

function renderSectorChips(sectors) {
  if (!Array.isArray(sectors) || sectors.length === 0) {
    return '<span class="text-muted">—</span>';
  }
  return sectors.map(function (s) {
    return '<span class="badge info">' + escapeHtml(sectorName(s)) + '</span>';
  }).join(' ');
}

function renderThemeChips(themes) {
  if (!Array.isArray(themes) || themes.length === 0) return '';
  return themes.map(function (t) {
    return '<a href="javascript:void(0)" class="cm-theme-chip badge muted" data-theme="' + escapeHtml(t) + '" title="前往錢潮因果查看此主題">' + escapeHtml(t) + '</a>';
  }).join(' ');
}

// navigateToCausalityTheme jumps to the causality page pre-filtered by a
// trigger theme. The theme is stashed on window._pendingCausalityFilter so
// loadCapitalCausality can apply it after the page switch.
function navigateToCausalityTheme(theme) {
  window._pendingCausalityFilter = theme;
  if (typeof window.switchPage === 'function') {
    window.switchPage('capital_causality');
  }
}

// wireThemeChips attaches click handlers after the models list renders.
function wireThemeChips(container) {
  if (!container) return;
  container.querySelectorAll('.cm-theme-chip').forEach(function (chip) {
    chip.addEventListener('click', function (ev) {
      ev.stopPropagation(); // don't toggle the card detail
      navigateToCausalityTheme(chip.getAttribute('data-theme'));
    });
  });
}
