/**
 * capital-board.js — 錢潮看板
 *
 * 顯示目前 atlas 啟用模型（含 favored_sectors / avoided_sectors 圖示）。
 * Source: /api/narrative/models
 */

import { silentGetJSON, renderMissingState } from '../shared/app-utils.js';
import { escapeHtml } from '../shared/utils.js';
import { financialColor } from '../shared/color-tokens.js';

const SECTOR_ALIAS = {
  '半導體業': '半導體',
  '半導體產業': '半導體',
  'AI伺服器產業': 'AI',
  'AI伺服器': 'AI',
  '電子零組件業': '電子零組件',
  '金融業': '金融',
  '金融股': '金融',
  '外銷產業': '外銷',
  '傳產股': '傳產',
  '傳統產業': '傳產',
};

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

function canonicalSector(name) {
  return SECTOR_ALIAS[name] || name;
}

function sectorLabel(name) {
  return SECTOR_LABEL[name] || SECTOR_LABEL[String(name).toLowerCase()] || name;
}

export const template = `
<details class="help-details"><summary><strong>💡 如何解讀本頁</strong></summary>
  錢潮看板顯示 atlas narrative engine 目前啟用的模型，每個模型依 weight
  比例列出「看好」與「看壞」的板塊。weight 越高代表該模型對近期方向的影響
  越強。上方圓餅圖與計數卡彙整看多 / 看空 / 中性的板塊數量與權重佔比。
</details>
<section id="cb-summary" class="cb-summary" aria-live="polite"></section>
<section id="cb-chart" class="cb-chart" aria-live="polite"></section>
<section id="cb-grid" class="cb-board" aria-live="polite">載入中…</section>
`;

function aggregateSectors(models) {
  const allSectors = [];
  models.forEach(function (m) {
    const favored = Array.isArray(m.favored_sectors) ? m.favored_sectors : [];
    const avoided = Array.isArray(m.avoided_sectors) ? m.avoided_sectors : [];
    const w = typeof m.weight === 'number' ? m.weight : 0;
    favored.forEach(function (s) { allSectors.push({ name: canonicalSector(s), vote: 'favored', weight: w }); });
    avoided.forEach(function (s) { allSectors.push({ name: canonicalSector(s), vote: 'avoided', weight: w }); });
  });

  if (!allSectors.length) return null;

  const grouped = {};
  allSectors.forEach(function (entry) {
    if (!grouped[entry.name]) grouped[entry.name] = { favored: 0, avoided: 0, total: 0 };
    grouped[entry.name][entry.vote] += entry.weight;
    grouped[entry.name].total += entry.weight;
  });

  const entries = Object.keys(grouped).map(function (name) {
    const g = grouped[name];
    const net = g.favored - g.avoided;
    const verdict = net > 0.05 ? 'bullish' : net < -0.05 ? 'bearish' : 'neutral';
    return {
      name: name,
      label: sectorLabel(name),
      favored: g.favored,
      avoided: g.avoided,
      total: g.total,
      net: net,
      verdict: verdict,
    };
  }).sort(function (a, b) { return Math.abs(b.net) - Math.abs(a.net); });

  const counts = {
    bullish: entries.filter(function (e) { return e.verdict === 'bullish'; }).length,
    bearish: entries.filter(function (e) { return e.verdict === 'bearish'; }).length,
    neutral: entries.filter(function (e) { return e.verdict === 'neutral'; }).length,
  };

  const weights = {
    bullish: entries.filter(function (e) { return e.verdict === 'bullish'; }).reduce(function (s, e) { return s + e.total; }, 0),
    bearish: entries.filter(function (e) { return e.verdict === 'bearish'; }).reduce(function (s, e) { return s + e.total; }, 0),
    neutral: entries.filter(function (e) { return e.verdict === 'neutral'; }).reduce(function (s, e) { return s + e.total; }, 0),
  };

  return { entries: entries, counts: counts, weights: weights };
}

function renderCounts(agg) {
  const total = agg.weights.bullish + agg.weights.bearish + agg.weights.neutral;
  function pct(v) { return total > 0 ? Math.round(v / total * 100) : 0; }
  return (
    '<div class="cb-summary__grid">' +
    '<div class="cb-summary__card cb-summary__card--bullish">' +
    '<div class="cb-summary__num">' + agg.counts.bullish + '</div>' +
    '<div class="cb-summary__label">看多板塊</div>' +
    '<div class="cb-summary__weight">權重 ' + pct(agg.weights.bullish) + '%</div>' +
    '</div>' +
    '<div class="cb-summary__card cb-summary__card--bearish">' +
    '<div class="cb-summary__num">' + agg.counts.bearish + '</div>' +
    '<div class="cb-summary__label">看空板塊</div>' +
    '<div class="cb-summary__weight">權重 ' + pct(agg.weights.bearish) + '%</div>' +
    '</div>' +
    '<div class="cb-summary__card cb-summary__card--neutral">' +
    '<div class="cb-summary__num">' + agg.counts.neutral + '</div>' +
    '<div class="cb-summary__label">中性板塊</div>' +
    '<div class="cb-summary__weight">權重 ' + pct(agg.weights.neutral) + '%</div>' +
    '</div>' +
    '<div class="cb-summary__card cb-summary__card--total">' +
    '<div class="cb-summary__num">' + agg.entries.length + '</div>' +
    '<div class="cb-summary__label">彙總板塊數</div>' +
    '<div class="cb-summary__weight">至少顯示 5 個</div>' +
    '</div>' +
    '</div>'
  );
}

function describeArc(cx, cy, r, startAngle, endAngle) {
  const start = polar(cx, cy, r, endAngle);
  const end = polar(cx, cy, r, startAngle);
  const largeArcFlag = endAngle - startAngle <= 180 ? '0' : '1';
  return [
    'M', cx, cy,
    'L', start[0], start[1],
    'A', r, r, 0, largeArcFlag, 0, end[0], end[1],
    'Z',
  ].join(' ');
}

function polar(cx, cy, r, angleDeg) {
  const rad = (angleDeg - 90) * Math.PI / 180;
  return [cx + r * Math.cos(rad), cy + r * Math.sin(rad)];
}

function renderPieChart(agg) {
  const total = agg.weights.bullish + agg.weights.bearish + agg.weights.neutral;
  if (total <= 0) return '';

  const slices = [
    { key: 'bullish', label: '看多', weight: agg.weights.bullish, color: 'var(--trend-bullish)' },
    { key: 'bearish', label: '看空', weight: agg.weights.bearish, color: 'var(--trend-bearish)' },
    { key: 'neutral', label: '中性', weight: agg.weights.neutral, color: 'var(--muted)' },
  ].filter(function (s) { return s.weight > 0; });

  const cx = 60, cy = 60, r = 50;
  let current = 0;
  const paths = slices.map(function (s) {
    const angle = s.weight / total * 360;
    const d = describeArc(cx, cy, r, current, current + angle);
    const result = '<path d="' + d + '" fill="' + s.color + '" stroke="var(--bg)" stroke-width="1" />';
    current += angle;
    return result;
  }).join('');

  const legend = slices.map(function (s) {
    const pct = Math.round(s.weight / total * 100);
    return (
      '<div class="cb-chart__legend-item">' +
      '<span class="cb-chart__dot" style="background:' + s.color + '"></span>' +
      '<span>' + escapeHtml(s.label) + ' ' + pct + '%</span>' +
      '</div>'
    );
  }).join('');

  return (
    '<div class="cb-chart__wrap">' +
    '<svg viewBox="0 0 120 120" class="cb-chart__svg" role="img" aria-label="板塊權重圓餅圖">' +
    paths +
    '</svg>' +
    '<div class="cb-chart__legend">' + legend + '</div>' +
    '</div>'
  );
}

function renderWeights(models) {
  return models.map(function (m) {
    const w = typeof m.weight === 'number' ? m.weight : 0;
    const pct = Math.round(w * 100);
    const reason = m.rationale ? String(m.rationale) : '';
    return (
      '<div class="cb-model">' +
      '<div class="cb-model__head">' +
      '<span class="cb-model__name">' + escapeHtml(m.name || m.id || '未命名') + '</span>' +
      '<span class="cb-model__weight">權重 ' + pct + '%</span>' +
      '</div>' +
      '<div class="cb-model__bar" aria-hidden="true">' +
      '<div class="cb-model__bar-fill" style="width:' + pct + '%;background:' + financialColor(w >= 1 ? 1 : 0, 'flow') + '"></div>' +
      '</div>' +
      (reason ? '<div class="cb-model__rationale text-muted">' + escapeHtml(reason) + '</div>' : '') +
      '</div>'
    );
  }).join('');
}

function renderSectors(entries) {
  if (!entries || !entries.length) {
    return '<div class="cb-board__sectors text-muted">目前模型未提供 sector 偏好資料</div>';
  }

  return (
    '<div class="cb-board__sectors">' +
    entries.map(function (e) {
      const verdict = e.verdict;
      const verdictLabel = verdict === 'bullish' ? '看好' : verdict === 'bearish' ? '看壞' : '中性';
      const verdictColor = financialColor(verdict === 'bullish' ? 1 : verdict === 'bearish' ? -1 : 0, 'flow');
      return (
        '<div class="cb-sector-row" data-verdict="' + verdict + '">' +
        '<span class="cb-sector-row__name">' + escapeHtml(e.label) + '</span>' +
        '<span class="cb-sector-row__bar" aria-hidden="true">' +
        (e.favored > 0 ? '<span style="width:' + (e.favored / e.total * 100).toFixed(1) + '%;background:rgba(239,68,68,0.45)"></span>' : '') +
        (e.avoided > 0 ? '<span style="width:' + (e.avoided / e.total * 100).toFixed(1) + '%;background:rgba(16,185,129,0.45)"></span>' : '') +
        '</span>' +
        '<span class="cb-sector-row__verdict" style="color:' + verdictColor + '">' + verdictLabel + '</span>' +
        '</div>'
      );
    }).join('') +
    '</div>'
  );
}

async function loadModels() {
  const data = await silentGetJSON('/api/narrative/models');
  const models = (data && Array.isArray(data.models)) ? data.models : [];
  const error = data === null;

  const summary = document.getElementById('cb-summary');
  const chart = document.getElementById('cb-chart');
  const grid = document.getElementById('cb-grid');

  if (error) {
    if (summary) summary.innerHTML = renderMissingState('模型資料', 'api-error');
    if (chart) chart.innerHTML = '';
    if (grid) grid.innerHTML = renderMissingState('錢潮看板', 'api-error');
    return;
  }

  if (!models.length) {
    if (summary) summary.innerHTML = renderMissingState('模型資料', 'no-data');
    if (chart) chart.innerHTML = '';
    if (grid) grid.innerHTML = '<div class="empty">目前無啟用中的模型</div>';
    return;
  }

  const agg = aggregateSectors(models);

  if (summary) summary.innerHTML = '<h3 class="cb-board__title">板塊方向彙總</h3>' + renderCounts(agg);
  if (chart) chart.innerHTML = '<h3 class="cb-board__title">權重佔比圓餅圖</h3>' + renderPieChart(agg);

  grid.innerHTML = (
    '<h3 class="cb-board__title">模型權重</h3>' +
    '<div class="cb-board__weights">' + renderWeights(models) + '</div>' +
    '<h3 class="cb-board__title">板塊看好 / 看壞彙總（' + agg.entries.length + ' 個）</h3>' +
    renderSectors(agg.entries)
  );
}

export async function init() {
  await loadModels();
}
