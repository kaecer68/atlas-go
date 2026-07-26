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
  steel: '鋼鐵',
  shipping: '航運',
  cement: '水泥',
  food: '食品',
  plastics: '塑膠',
  textiles: '紡織',
  machinery: '電機機械',
  chemicals: '化學',
  biotech: '生技醫療',
  construction: '建材營造',
  retail: '貿易百貨',
  energy: '油電燃氣',
  auto: '汽車',
  optoelectronics: '光電',
  telecom: '通信網路',
  other_electronics: '其他電子',
};

function sectorLabel(name) {
  return SECTOR_LABEL[name] || SECTOR_LABEL[String(name).toLowerCase()] || name;
}

export const template = `
<details class="help-details"><summary><strong>💡 如何解讀本頁</strong></summary>
  <p>上方「5 日預測卡」列出每日整體資金方向與信心分數；下方「板塊熱度圖」只顯示板塊受影響的方向（▲ 流入 / ▼ 流出），信心分數為每日整體預測，非單一板塊精準值。「ETF 換股估計」列出未來 ETF rebalance 預估加碼/減碼標的與預估金額。</p>
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
<section id="cp-etf-estimates" class="cp-etf-estimates" aria-live="polite"></section>
<section id="cp-sector-predictions" class="cp-sector-predictions" aria-live="polite"></section>
<section id="cp-detail" class="cp-detail" hidden></section>`;

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
let _etfEstimates = [];
let _sectorPredictions = [];
let _activeDir = 'all';
let _lastError = false;
let _summary = '';

let _sectorPredictionsExpanded = false;
let _showAllSectors = false;
try {
  _sectorPredictionsExpanded = localStorage.getItem('cp_sector_predictions_expanded') === 'true';
} catch (e) {}

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
  if (_summary) {
    host.innerHTML = '<div style="padding:10px 12px;background:var(--panel);border:1px solid var(--border);border-radius:8px;font-size:13px;line-height:1.6;color:var(--text);margin-bottom:10px;">' + escapeHtml(_summary) + '</div>';
  } else {
    host.innerHTML = '';
  }
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

/**
 * 為 C06 新增：純函式版本，將 etf_estimates 陣列渲染為 HTML 字串。
 * 不接觸 DOM,讓單元測試可不依賴 jsdom 直接呼叫。
 *
 * @param {Array<{etf_name?: string, stock_symbol?: string, stock_name?: string, direction?: string, est_weight?: number, etf_aum?: number, est_flow?: number}>} estimates
 * @returns {string} HTML,空陣列回傳空字串
 */
export function renderETFEstimatesTable(estimates) {
  if (!Array.isArray(estimates) || estimates.length === 0) return '';

  const sorted = estimates.slice().sort(function (a, b) {
    const ea = (a && a.etf_name) || '';
    const eb = (b && b.etf_name) || '';
    if (ea !== eb) return ea.localeCompare(eb, 'zh-Hant');
    return (Number(b.est_flow) || 0) - (Number(a.est_flow) || 0);
  });

  const rows = sorted.map(function (e) {
    const dir = (e && e.direction === 'add') ? '加碼' : (e && e.direction === 'remove') ? '減碼' : '—';
    const dirClass = (e && e.direction === 'add') ? 'cp-etf__row--add' : (e && e.direction === 'remove') ? 'cp-etf__row--remove' : '';
    const symbol = (e && e.stock_symbol) ? escapeHtml(e.stock_symbol) : '—';
    const name = (e && e.stock_name) ? ' ' + escapeHtml(e.stock_name) : '';
    const etf = (e && e.etf_name) ? escapeHtml(e.etf_name) : '—';
    const weightPct = (typeof e.est_weight === 'number') ? (e.est_weight * 100).toFixed(1) + '%' : '—';
    const aumStr = (typeof e.etf_aum === 'number') ? formatAUM(e.etf_aum) : '—';
    const flowStr = (typeof e.est_flow === 'number') ? formatNTDMillions(e.est_flow) : '—';
    return (
      '<tr class="' + dirClass + '">' +
      '<td>' + etf + '</td>' +
      '<td class="cp-etf__sym">' + symbol + '<span class="cp-etf__name">' + name + '</span></td>' +
      '<td>' + dir + '</td>' +
      '<td class="cp-etf__num">' + weightPct + '</td>' +
      '<td class="cp-etf__num">' + aumStr + '</td>' +
      '<td class="cp-etf__num">' + flowStr + '</td>' +
      '</tr>'
    );
  }).join('');

  return (
    '<h3 class="cp-section-title">ETF 換股估計</h3>' +
    '<p class="cp-section-help">未來 14 日內 ETF 季配/年中/年底 rebalance 預估加碼/減碼標的與預估淨流量（NT$ 百萬 × AUM 兆權重）。</p>' +
    '<div class="cp-etf__table-wrap"><table class="cp-etf__table">' +
    '<thead><tr>' +
    '<th>ETF</th><th>標的</th><th>方向</th><th>估計權重</th><th>AUM</th><th>預估淨流量' +
    '</th></tr></thead>' +
    '<tbody>' + rows + '</tbody>' +
    '</table></div>'
  );
}

function formatAUM(billion) {
  if (billion >= 1000) return (billion / 1000).toFixed(1) + ' 兆';
  if (billion >= 1) return billion.toFixed(0) + ' 億';
  return (billion * 100).toFixed(0) + ' 百萬';
}

function formatNTDMillions(million) {
  const abs = Math.abs(million);
  if (abs >= 1000) return (million / 1000).toFixed(2) + ' 億';
  return million.toFixed(0) + ' 百萬';
}

function renderETFEstimates() {
  const host = document.getElementById('cp-etf-estimates');
  if (!host) return;

  if (_lastError) {
    host.innerHTML = renderMissingState('ETF 換股估計', 'api-error');
    return;
  }

  const html = renderETFEstimatesTable(_etfEstimates);
  host.innerHTML = html;
}

function renderSectorDetail(dateStr, cellData) {
  const host = document.getElementById('cp-detail');
  if (!cellData) {
    host.hidden = true;
    host.innerHTML = '';
    return;
  }
  host.hidden = false;
  const conf = typeof cellData.confidence === 'number' ? cellData.confidence : 0;
  const dir = cellData.direction || 'neutral';
  const dirLabelText = dirLabel(dir);
  const color = directionColor(dir);
  const date = dateStr ? dateStr.slice(0, 10) : '—';
  const name = cellData.sector_name || sectorLabel(cellData.sector_id);
  const drivers = Array.isArray(cellData.drivers) ? cellData.drivers : [];

  let distHtml = '';
  if (cellData.distribution) {
      const inflowPct = Math.round((cellData.distribution.inflow || 0) * 100);
      const neutralPct = Math.round((cellData.distribution.neutral || 0) * 100);
      const outflowPct = Math.round((cellData.distribution.outflow || 0) * 100);
      distHtml = '<div class="cp-detail__distribution">分佈：流入 ' + inflowPct + '% / 觀望 ' + neutralPct + '% / 流出 ' + outflowPct + '%</div>';
  }

  host.innerHTML =
    '<div class="cp-detail__card">' +
    '<div class="cp-detail__date">日期：' + escapeHtml(date) + '</div>' +
    '<div class="cp-detail__sector-name">板塊：' + escapeHtml(name) + '</div>' +
    '<div class="cp-detail__dir" style="color:' + color + '">方向：' + escapeHtml(dirLabelText) + '（信心 ' + Math.round(conf * 100) + '%）</div>' +
    distHtml +
    (drivers.length
      ? '<div class="cp-detail__reasons cp-detail__drivers-title"><strong>驅動因子</strong><ul class="cp-detail__drivers-list">' +
        drivers.map(function (r) { return '<li>' + escapeHtml(typeof r === 'string' ? r : JSON.stringify(r)) + '</li>'; }).join('') +
        '</ul></div>'
      : '<div class="cp-detail__reasons text-muted cp-detail__no-drivers">無驅動因子</div>') +
    '</div>';

  host.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
}

export const MUST_WATCH_SECTORS = ['semiconductor', 'electronics', 'financials', 'shipping', 'steel'];

export function _setStateForTest(allPreds, actEvents, secPreds, lastErr, showAll, expanded, summary) {
  _allPredictions = allPreds;
  _activeEvents = actEvents;
  _sectorPredictions = secPreds;
  _lastError = lastErr;
  _showAllSectors = showAll;
  _sectorPredictionsExpanded = expanded;
  _summary = summary || '';
}

export function renderSectorPredictions() {
  const host = document.getElementById('cp-sector-predictions');
  if (!host) return;

  if (_lastError) {
    host.innerHTML = renderMissingState('板塊方向預測', 'api-error');
    return;
  }
  if (!_sectorPredictions.length) {
    host.innerHTML = '<div class="cp-sp-empty"><div class="state-missing__text text-muted">尚無板塊預測資料</div></div>';
    return;
  }

  const day1 = _sectorPredictions[0];
  let bullish = 0, bearish = 0, neutral = 0;
  if (day1 && Array.isArray(day1.sectors)) {
    for (const sec of day1.sectors) {
      if (MUST_WATCH_SECTORS.indexOf(sec.sector_id) !== -1) {
        if (sec.direction === 'inflow') bullish++;
        else if (sec.direction === 'outflow') bearish++;
        else neutral++;
      }
    }
  }

  const badgeText = '5 個必須看板塊中 ' + bullish + ' 個偏多 / ' + bearish + ' 個偏空 / ' + neutral + ' 個觀望';

  const allSectorIds = new Set();
  for (const day of _sectorPredictions) {
    if (Array.isArray(day.sectors)) {
      for (const s of day.sectors) {
        allSectorIds.add(s.sector_id);
      }
    }
  }

  const sortedSectors = [];
  for (const id of MUST_WATCH_SECTORS) {
    if (allSectorIds.has(id)) {
      sortedSectors.push(id);
      allSectorIds.delete(id);
    }
  }
  const rest = Array.from(allSectorIds).sort();

  const sectorsToRender = _showAllSectors ? sortedSectors.concat(rest) : sortedSectors;

  const headerCells = _sectorPredictions.slice(0, 5).map(function (p, idx) {
    return '<div class="cp-heatmap__colhead">' + escapeHtml(dayLabel(p, idx)) + '</div>';
  }).join('');

  const getSectorCell = function(dayData, sectorId) {
     if (!dayData || !Array.isArray(dayData.sectors)) return null;
     for (let i = 0; i < dayData.sectors.length; i++) {
        if (dayData.sectors[i].sector_id === sectorId) return dayData.sectors[i];
     }
     return null;
  };

  const rows = sectorsToRender.map(function (sectorId) {
    const firstMatch = getSectorCell(day1, sectorId);
    const label = firstMatch && firstMatch.sector_name ? firstMatch.sector_name : sectorLabel(sectorId);

    const cells = _sectorPredictions.slice(0, 5).map(function (p, colIdx) {
      const cellData = getSectorCell(p, sectorId);
      if (!cellData) {
        return '<div class="cp-heatmap__cell" style="color:var(--muted)">—</div>';
      }

      const dir = cellData.direction || 'neutral';
      const conf = typeof cellData.confidence === 'number' ? cellData.confidence : 0;
      const confPct = Math.round(conf * 100);
      const color = directionColor(dir);

      const drivers = Array.isArray(cellData.drivers) ? cellData.drivers : [];
      let tooltipText = dirLabel(dir) + ' ' + confPct + '%';
      if (drivers.length > 0) {
        tooltipText += '\n' + drivers.slice(0, 2).map(function(s) { return escapeHtml(s); }).join('、');
      }

      return (
        '<div class="cp-heatmap__cell is-active sector-cell" ' +
        'title="' + tooltipText + '" ' +
        'data-dayidx="' + colIdx + '" ' +
        'data-sectorid="' + escapeHtml(sectorId) + '" ' +
        'style="color:' + color + '"' +
        '>' +
        directionSign(dir) + ' <span class="cp-heatmap__pct">' + confPct + '%</span>' +
        '</div>'
      );
    }).join('');

    return (
      '<div class="cp-heatmap__row">' +
      '<div class="cp-heatmap__rowhead" title="' + escapeHtml(label) + '">' + escapeHtml(label) + '</div>' +
      '<div class="cp-heatmap__cells">' + cells + '</div>' +
      '</div>'
    );
  }).join('');

  const contentDisplay = _sectorPredictionsExpanded ? 'block' : 'none';
  const toggleIcon = _sectorPredictionsExpanded ? '▼' : '▶';
  const switchChecked = _showAllSectors ? 'checked' : '';

  host.innerHTML =
    '<div class="cp-sp-header" style="display:' + (contentDisplay === 'block' ? 'flex' : 'flex') + '">' +
      '<div class="cp-sp-header__title-wrap">' +
        '<span class="cp-sp-toggle-icon">' + toggleIcon + '</span>' +
        '<strong class="cp-sp-title">板塊方向預測</strong>' +
        '<span class="chip">' + escapeHtml(badgeText) + '</span>' +
      '</div>' +
      '<div class="cp-sp-switch-container" style="display:' + contentDisplay + ';">' +
        '<label class="cp-sp-switch-label">' +
          '<input type="checkbox" id="cp-sp-show-all" ' + switchChecked + '>' +
          '顯示全部 20 板塊' +
        '</label>' +
      '</div>' +
    '</div>' +
    '<div class="cp-sp-content" style="display:' + contentDisplay + ';">' +
      '<div class="cp-heatmap-table">' +
        '<div class="cp-heatmap__header">' +
          '<div class="cp-heatmap__label">板塊 / 日期</div>' +
          '<div class="cp-heatmap__cols">' + headerCells + '</div>' +
        '</div>' +
        '<div class="cp-heatmap__body">' + rows + '</div>' +
      '</div>' +
    '</div>';

  const headerEl = host.querySelector('.cp-sp-header');
  if (headerEl) {
    headerEl.addEventListener('click', function() {
      _sectorPredictionsExpanded = !_sectorPredictionsExpanded;
      try { localStorage.setItem('cp_sector_predictions_expanded', String(_sectorPredictionsExpanded)); } catch(e) {}
      renderSectorPredictions();
    });
  }

  const switchContainer = host.querySelector('.cp-sp-switch-container');
  if (switchContainer) {
    switchContainer.addEventListener('click', function(e) {
      e.stopPropagation();
    });
  }

  const showAllEl = host.querySelector('#cp-sp-show-all');
  if (showAllEl) {
    showAllEl.addEventListener('change', function(e) {
      _showAllSectors = e.target.checked;
      renderSectorPredictions();
    });
    showAllEl.addEventListener('click', function(e) {
      e.stopPropagation();
    });
  }

  host.querySelectorAll('.sector-cell').forEach(function(cell) {
    cell.addEventListener('click', function(e) {
      e.stopPropagation();
      const dayIdx = parseInt(cell.getAttribute('data-dayidx'), 10);
      const sectorId = cell.getAttribute('data-sectorid');
      const dayData = _sectorPredictions[dayIdx];
      const cellData = getSectorCell(dayData, sectorId);
      renderSectorDetail(dayData.date, cellData);
    });
  });
}

function renderAll() {
  renderStatus();
  renderPredictionsCard();
  renderSectorHeatmap();
  renderETFEstimates();
  renderSectorPredictions();
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
  _etfEstimates = (data && Array.isArray(data.etf_estimates)) ? data.etf_estimates : [];
  _sectorPredictions = (data && Array.isArray(data.sector_predictions)) ? data.sector_predictions : [];
  _summary = (data && typeof data.summary === 'string') ? data.summary : '';
  renderAll();
}

export async function init() {
  wireFilter();
  await loadPredictions();
}
