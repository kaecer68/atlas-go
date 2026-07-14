/**
 * capital-predictions.js — 錢潮預測
 *
 * 顯示未來 5 日 daily prediction + 信心分數熱度圖。
 * Source: /api/events/prediction
 *
 * Backend shape: predictions[] 含 driving_events / predicted_forces;active_events[]
 * 含 affected_industries。本模組用 mapPredictionForDisplay 從 driving_events 推出
 * reasons,並以 name-match 從 active_events 取 union 的 affected_industries。
 */

import { silentGetJSON } from '../shared/app-utils.js';
import { escapeHtml } from '../shared/utils.js';
import { financialColor } from '../shared/color-tokens.js';

export const template = `
<details class="help-details"><summary><strong>💡 如何解讀本頁</strong></summary>
  5 日 daily prediction 信心分數熱度圖：每一格代表一天的板塊方向預測；
  顏色深度 = 信心分數（深色 = 信心高、方向明確；淺色 = 信心低、偏中性）。
  點任一格可在下方顯示細節。
</details>
<section class="filter-bar" aria-label="方向篩選">
  <button class="filter-pill active" data-dir="all">全部</button>
  <button class="filter-pill" data-dir="inflow">📈 流入</button>
  <button class="filter-pill" data-dir="outflow">📉 流出</button>
  <button class="filter-pill" data-dir="neutral">➖ 中性</button>
</section>
<section id="cp-grid" class="cp-heatmap" aria-live="polite">載入中…</section>
<section id="cp-detail" class="cp-detail" hidden></section>
`;

/**
 * @param {object|null|undefined} prediction backend FlowPrediction
 * @param {Array<object>|null|undefined} activeEvents backend active_events
 * @returns {{ reasons: string[], sectors: string[] }}
 */
export function mapPredictionForDisplay(prediction, activeEvents) {
  if (!prediction || typeof prediction !== 'object') {
    return { reasons: [], sectors: [] };
  }
  const reasons = Array.isArray(prediction.driving_events)
    ? prediction.driving_events.filter(function (s) { return typeof s === 'string' && s.length > 0; })
    : [];
  const safeEvents = Array.isArray(activeEvents) ? activeEvents : [];
  const drivingSet = new Set(reasons);
  const sectorSet = new Set();
  for (const evt of safeEvents) {
    if (!evt || typeof evt !== 'object') continue;
    const name = typeof evt.name === 'string' ? evt.name : '';
    if (!drivingSet.has(name)) continue;
    const industries = Array.isArray(evt.affected_industries) ? evt.affected_industries : [];
    for (const ind of industries) {
      if (typeof ind === 'string' && ind.length > 0) sectorSet.add(ind);
    }
  }
  return { reasons, sectors: Array.from(sectorSet) };
}

const DAY_LABELS = ['明', '二', '三', '四', '五'];

let _allPredictions = [];
let _activeEvents = [];
let _activeDir = 'all';

function dayLabel(prediction, idx) {
  if (prediction && typeof prediction.date === 'string') {
    const d = new Date(prediction.date);
    if (!isNaN(d.getTime())) {
      return (d.getMonth() + 1) + '/' + d.getDate();
    }
  }
  return DAY_LABELS[idx] || 'D' + (idx + 1);
}

function confidenceColor(conf) {
  if (typeof conf !== 'number') return 'rgba(120,120,120,0.15)';
  const alpha = Math.min(0.85, Math.max(0.12, conf * 0.85));
  if (conf >= 0.7) return 'rgba(34,139,34,' + alpha.toFixed(3) + ')';
  if (conf >= 0.4) return 'rgba(255,165,0,' + alpha.toFixed(3) + ')';
  return 'rgba(178,34,34,' + alpha.toFixed(3) + ')';
}

function renderGrid() {
  const grid = document.getElementById('cp-grid');
  const filtered = _activeDir === 'all'
    ? _allPredictions
    : _allPredictions.filter(function (p) { return (p.direction || 'neutral') === _activeDir; });

  if (!filtered.length) {
    grid.innerHTML = '<div class="empty">' + escapeHtml(_activeDir === 'all' ? '目前無 5 日預測資料' : '此方向無資料（試試切換方向）') + '</div>';
    return;
  }

  grid.innerHTML = filtered.map(function (p, idx) {
    const conf = typeof p.confidence === 'number' ? p.confidence : 0;
    const dir = p.direction || 'neutral';
    const bg = confidenceColor(conf);
    const dirLabel = dir === 'inflow' ? '流入' : dir === 'outflow' ? '流出' : '中性';
    const dataIdx = _allPredictions.indexOf(p);
    return (
      '<div class="cp-cell" data-idx="' + dataIdx + '" ' +
      'style="background:' + bg + '" ' +
      'title="' + escapeHtml(dirLabel) + ' · 信心 ' + Math.round(conf * 100) + '%">' +
      '<div class="cp-cell__day">' + escapeHtml(dayLabel(p, idx)) + '</div>' +
      '<div class="cp-cell__dir">' + escapeHtml(dirLabel) + '</div>' +
      '<div class="cp-cell__conf">' + Math.round(conf * 100) + '%</div>' +
      '</div>'
    );
  }).join('');

  grid.querySelectorAll('.cp-cell').forEach(function (cell) {
    cell.addEventListener('click', function () {
      const idx = parseInt(cell.getAttribute('data-idx'), 10);
      renderDetail(_allPredictions[idx]);
    });
  });
}

function renderDetail(prediction) {
  const host = document.getElementById('cp-detail');
  if (!prediction) {
    host.hidden = true;
    host.innerHTML = '';
    return;
  }
  host.hidden = false;
  const conf = typeof prediction.confidence === 'number' ? prediction.confidence : 0;
  const dir = prediction.direction || 'neutral';
  const dirLabel = dir === 'inflow' ? '資金流入' : dir === 'outflow' ? '資金流出' : '中性';
  const dirColor = financialColor(dir === 'inflow' ? 1 : dir === 'outflow' ? -1 : 0, 'flow');
  const date = prediction.date ? prediction.date.slice(0, 10) : '—';
  const { reasons, sectors } = mapPredictionForDisplay(prediction, _activeEvents);

  host.innerHTML =
    '<div class="cp-detail__card">' +
    '<div class="cp-detail__date">日期：' + escapeHtml(date) + '</div>' +
    '<div class="cp-detail__dir" style="color:' + dirColor + '">方向：' + escapeHtml(dirLabel) + '（信心 ' + Math.round(conf * 100) + '%）</div>' +
    (reasons.length
      ? '<div class="cp-detail__reasons"><strong>觸發原因</strong><ul>' +
        reasons.map(function (r) { return '<li>' + escapeHtml(typeof r === 'string' ? r : (r.text || r.name || JSON.stringify(r))) + '</li>'; }).join('') +
        '</ul></div>'
      : '<div class="cp-detail__reasons text-muted">無觸發原因</div>') +
    (sectors.length
      ? '<div class="cp-detail__sectors"><strong>影響板塊</strong><div class="chip-list">' +
        sectors.map(function (s) { return '<span class="chip">' + escapeHtml(typeof s === 'string' ? s : (s.name || s.code || '')) + '</span>'; }).join('') +
        '</div></div>'
      : '') +
    '</div>';
}

async function loadPredictions() {
  const data = await silentGetJSON('/api/events/prediction');
  _allPredictions = (data && Array.isArray(data.predictions)) ? data.predictions : [];
  _activeEvents = (data && Array.isArray(data.active_events)) ? data.active_events : [];
  renderGrid();
}

function wireFilter() {
  document.querySelectorAll('.filter-bar .filter-pill[data-dir]').forEach(function (btn) {
    btn.addEventListener('click', function () {
      _activeDir = btn.getAttribute('data-dir');
      document.querySelectorAll('.filter-bar .filter-pill').forEach(function (b) { b.classList.remove('active'); });
      btn.classList.add('active');
      renderGrid();
    });
  });
}

export async function init() {
  wireFilter();
  await loadPredictions();
}
