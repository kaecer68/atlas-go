/**
 * capital-predictions.js — 錢潮預測
 *
 * 顯示未來 5 日 daily prediction + 信心分數熱度圖。
 * Source: /api/events/prediction
 *
 * Backend shape: predictions[] 含 driving_events / predicted_forces; active_events[]
 * 含 affected_industries。本模組用 mapPredictionForDisplay 從 driving_events 推出
 * reasons，並以 name-match 從 active_events 取 union 的 affected_industries。
 */

import { silentGetJSON, renderMissingState, renderErrorState } from '../shared/app-utils.js';
import { escapeHtml } from '../shared/utils.js';
import { financialColor } from '../shared/color-tokens.js';

const RETRY_ID = 'capital-predictions';

const SECTOR_LABEL = {
  semiconductor: '半導體',
  ai_supply_chain: 'AI 供應鏈',
  electronics: '電子',
  financials: '金融',
  consumer: '消費',
  tourism: '觀光',
  pcb: 'PCB',
  thermal: '散熱',
  high_dividend: '高股息',
  etf_rotation: 'ETF 輪動',
  small_cap: '小型股',
  traditional: '傳產',
  'ai伺服器產業': 'AI',
  'ai伺服器': 'AI',
  '半導體業': '半導體',
  '半導體產業': '半導體',
  '電子零組件業': '電子零組件',
  '金融業': '金融',
  '金融股': '金融',
  '外銷產業': '外銷',
  '傳產股': '傳產',
  '傳統產業': '傳產',
};

function sectorLabel(name) {
  return SECTOR_LABEL[name] || SECTOR_LABEL[String(name).toLowerCase()] || name;
}

export const template = `
<details class="help-details"><summary><strong>💡 如何解讀本頁</strong></summary>
  <p>上方「5 日預測卡」列出每日整體資金方向與信心分數；下方「板塊熱度圖」只顯示板塊受影響的方向（▲ 流入 / ▼ 流出），信心分數為每日整體預測，非單一板塊精準值。</p>
</details>
<section id="cp-summary" class="cp-summary" aria-live="polite"></section>
<section class="filter-bar" aria-label="方向篩選">
  <button class="filter-pill active" data-dir="all">全部</button>
  <button class="filter-pill" data-dir="inflow">📈 流入</button>
  <button class="filter-pill" data-dir="outflow">📉 流出</button>
  <button class="filter-pill" data-dir="neutral">➖ 中性</button>
</section>
<section id="cp-predictions" class="cp-predictions" aria-live="polite">載入中…</section>
<section id="cp-heatmap" class="cp-sector-heatmap" aria-live="polite"></section>
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
let _lastError = false;

function dayLabel(prediction, idx) {
  if (prediction && typeof prediction.date === 'string') {
    const d = new Date(prediction.date);
    if (!isNaN(d.getTime())) {
      return (d.getMonth() + 1) + '/' + d.getDate();
    }
  }
  return DAY_LABELS[idx] || 'D' + (idx + 1);
}

function dirLabel(dir) {
  if (dir === 'inflow') return '資金流入';
  if (dir === 'outflow') return '資金流出';
  return '中性';
}

function directionColor(dir) {
  return financialColor(dir === 'inflow' ? 1 : dir === 'outflow' ? -1 : 0, 'flow');
}

function directionSign(dir) {
  if (dir === 'inflow') return '▲';
  if (dir === 'outflow') return '▼';
  return '—';
}

function cellBackground(dir, conf) {
  const pct = Math.round((typeof conf === 'number' ? conf : 0) * 100);
  const colorVar = dir === 'inflow'
    ? 'var(--capital-inflow)'
    : dir === 'outflow'
      ? 'var(--capital-outflow)'
      : 'var(--muted)';
  return 'color-mix(in srgb, ' + colorVar + ' ' + pct + '%, transparent)';
}

function filteredPredictions() {
  if (_activeDir === 'all') return _allPredictions.slice();
  return _allPredictions.filter(function (p) { return (p.direction || 'neutral') === _activeDir; });
}

function renderStatus() {
  const host = document.getElementById('cp-summary');
  if (!host) return;
  if (_lastError) {
    host.innerHTML = renderErrorState('錢潮預測', RETRY_ID);
    const btn = host.querySelector('[data-retry="' + RETRY_ID + '"]');
    if (btn) btn.addEventListener('click', loadPredictions);
    return;
  }
  host.innerHTML = '';
}

function renderPredictionsCard() {
  const host = document.getElementById('cp-predictions');
  if (!host) return;
  const list = filteredPredictions();

  if (_lastError) {
    host.innerHTML = renderMissingState('5 日預測', 'api-error');
    return;
  }
  if (!_allPredictions.length) {
    host.innerHTML = renderMissingState('5 日預測', 'no-data');
    return;
  }
  if (!list.length) {
    host.innerHTML = renderMissingState('5 日預測', 'no-data');
    return;
  }

  host.innerHTML =
    '<h3 class="cp-section-title">5 日資金流向預測</h3>' +
    '<div class="cp-predictions__list">' +
    list.map(function (p) {
      const conf = typeof p.confidence === 'number' ? p.confidence : 0;
      const dir = p.direction || 'neutral';
      const confPct = Math.round(conf * 100);
      const color = directionColor(dir);
      const { reasons } = mapPredictionForDisplay(p, _activeEvents);
      const date = dayLabel(p, _allPredictions.indexOf(p));
      const eventsHtml = reasons.length
        ? '<div class="chip-list">' +
          reasons.map(function (r) { return '<span class="chip">' + escapeHtml(r) + '</span>'; }).join('') +
          '</div>'
        : '<span class="text-muted">無驅動事件</span>';
      return (
        '<div class="cp-prediction" data-idx="' + _allPredictions.indexOf(p) + '">' +
        '<div class="cp-prediction__meta">' +
        '<span class="cp-prediction__day">' + escapeHtml(date) + '</span>' +
        '<span class="cp-prediction__dir" style="color:' + color + '">' + directionSign(dir) + ' ' + escapeHtml(dirLabel(dir)) + '</span>' +
        '<span class="cp-prediction__conf">' + confPct + '%</span>' +
        '</div>' +
        '<div class="cp-prediction__bar" aria-hidden="true"><span style="width:' + confPct + '%;background:' + color + '"></span></div>' +
        '<div class="cp-prediction__events">' + eventsHtml + '</div>' +
        '</div>'
      );
    }).join('') +
    '</div>';

  host.querySelectorAll('.cp-prediction').forEach(function (row) {
    row.addEventListener('click', function () {
      const idx = parseInt(row.getAttribute('data-idx'), 10);
      renderDetail(_allPredictions[idx]);
    });
  });
}

function buildSectorRows() {
  const sectorCount = {};
  const safeEvents = Array.isArray(_activeEvents) ? _activeEvents : [];
  for (const evt of safeEvents) {
    if (!evt || typeof evt !== 'object') continue;
    const industries = Array.isArray(evt.affected_industries) ? evt.affected_industries : [];
    for (const ind of industries) {
      if (typeof ind !== 'string' || !ind.length) continue;
      sectorCount[ind] = (sectorCount[ind] || 0) + 1;
    }
  }
  return Object.keys(sectorCount).sort(function (a, b) {
    const diff = sectorCount[b] - sectorCount[a];
    if (diff !== 0) return diff;
    return a.localeCompare(b, 'zh-Hant');
  });
}

function matchedEventsForSector(prediction, sector) {
  const reasons = Array.isArray(prediction.driving_events) ? prediction.driving_events : [];
  const reasonSet = new Set(reasons);
  const out = [];
  for (const evt of _activeEvents) {
    if (!evt || typeof evt !== 'object') continue;
    const name = typeof evt.name === 'string' ? evt.name : '';
    if (!reasonSet.has(name)) continue;
    const industries = Array.isArray(evt.affected_industries) ? evt.affected_industries : [];
    if (industries.indexOf(sector) !== -1) out.push(name);
  }
  return out;
}

function renderSectorHeatmap() {
  const host = document.getElementById('cp-heatmap');
  if (!host) return;
  const list = filteredPredictions();

  if (_lastError) {
    host.innerHTML = renderMissingState('板塊熱度圖', 'api-error');
    return;
  }
  if (!_allPredictions.length) {
    host.innerHTML = renderMissingState('板塊熱度圖', 'no-data');
    return;
  }

  const sectors = buildSectorRows();
  if (!sectors.length) {
    host.innerHTML = renderMissingState('板塊熱度圖', 'no-data');
    return;
  }

  const headerCells = list.map(function (p, idx) {
    return '<div class="cp-heatmap__colhead">' + escapeHtml(dayLabel(p, _allPredictions.indexOf(p))) + '</div>';
  }).join('');

  const rows = sectors.map(function (sector) {
    const label = sectorLabel(sector);
    const cells = list.map(function (p, colIdx) {
      const allIdx = _allPredictions.indexOf(p);
      const conf = typeof p.confidence === 'number' ? p.confidence : 0;
      const dir = p.direction || 'neutral';
      const matched = matchedEventsForSector(p, sector);
      const active = matched.length > 0;
      const bg = active ? cellBackground(dir, conf) : 'transparent';
      const color = active ? directionColor(dir) : 'var(--muted)';
      const title = active
        ? escapeHtml(dayLabel(p, allIdx) + ' · ' + dirLabel(dir) + ' ' + Math.round(conf * 100) + '%' + '\n' + matched.join('、'))
        : escapeHtml(dayLabel(p, allIdx) + ' · 無顯著驅動事件');
      return (
        '<div class="cp-heatmap__cell' + (active ? ' is-active' : '') + '" ' +
        'title="' + title + '" ' +
        'data-idx="' + allIdx + '" ' +
        'data-sector="' + escapeHtml(sector) + '" ' +
        'style="background:' + bg + ';color:' + color + '"' +
        '>' +
        (active ? directionSign(dir) : '—') +
        '</div>'
      );
    }).join('');

    return (
      '<div class="cp-heatmap__row">' +
      '<div class="cp-heatmap__rowhead">' + escapeHtml(label) + '</div>' +
      '<div class="cp-heatmap__cells">' + cells + '</div>' +
      '</div>'
    );
  }).join('');

  host.innerHTML =
    '<h3 class="cp-section-title">板塊 × 日期 熱度圖</h3>' +
    '<div class="cp-heatmap-table">' +
    '<div class="cp-heatmap__header">' +
    '<div class="cp-heatmap__label">板塊 / 日期</div>' +
    '<div class="cp-heatmap__cols">' + headerCells + '</div>' +
    '</div>' +
    '<div class="cp-heatmap__body">' + rows + '</div>' +
    '</div>';

  host.querySelectorAll('.cp-heatmap__cell.is-active').forEach(function (cell) {
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
  const dirLabelText = dirLabel(dir);
  const color = directionColor(dir);
  const date = prediction.date ? prediction.date.slice(0, 10) : '—';
  const { reasons, sectors } = mapPredictionForDisplay(prediction, _activeEvents);

  host.innerHTML =
    '<div class="cp-detail__card">' +
    '<div class="cp-detail__date">日期：' + escapeHtml(date) + '</div>' +
    '<div class="cp-detail__dir" style="color:' + color + '">方向：' + escapeHtml(dirLabelText) + '（信心 ' + Math.round(conf * 100) + '%）</div>' +
    (reasons.length
      ? '<div class="cp-detail__reasons"><strong>觸發原因</strong><ul>' +
        reasons.map(function (r) { return '<li>' + escapeHtml(typeof r === 'string' ? r : (r.text || r.name || JSON.stringify(r))) + '</li>'; }).join('') +
        '</ul></div>'
      : '<div class="cp-detail__reasons text-muted">無觸發原因</div>') +
    (sectors.length
      ? '<div class="cp-detail__sectors"><strong>影響板塊</strong><div class="chip-list">' +
        sectors.map(function (s) { return '<span class="chip">' + escapeHtml(sectorLabel(typeof s === 'string' ? s : (s.name || s.code || ''))) + '</span>'; }).join('') +
        '</div></div>'
      : '') +
    '</div>';
}

function renderAll() {
  renderStatus();
  renderPredictionsCard();
  renderSectorHeatmap();
}

function wireFilter() {
  document.querySelectorAll('.filter-bar .filter-pill[data-dir]').forEach(function (btn) {
    btn.addEventListener('click', function () {
      _activeDir = btn.getAttribute('data-dir');
      document.querySelectorAll('.filter-bar .filter-pill').forEach(function (b) { b.classList.remove('active'); });
      btn.classList.add('active');
      renderAll();
    });
  });
}

async function loadPredictions() {
  const data = await silentGetJSON('/api/events/prediction');
  _lastError = data === null;
  _allPredictions = (data && Array.isArray(data.predictions)) ? data.predictions : [];
  _activeEvents = (data && Array.isArray(data.active_events)) ? data.active_events : [];
  renderAll();
}

export async function init() {
  wireFilter();
  await loadPredictions();
}
