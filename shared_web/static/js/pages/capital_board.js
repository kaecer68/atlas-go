/**
 * capital-board.js — 錢潮看板
 *
 * 顯示七維資金流模型的即時摘要：
 *   - 模型資料：GET /api/capital-flow/summary（quality_score、dominant_force、resonance）
 *   - 錢潮看板：GET /api/capital-flow/daily（各勢力 Z-score、原始值、趨勢）
 */

import { silentGetJSON, renderMissingState } from '../shared/app-utils.js';
import { escapeHtml } from '../shared/utils.js';
import { financialColor } from '../shared/color-tokens.js';

const FORCE_LABEL = {
  foreign: '外資',
  futures: '外資期貨',
  tsm_adr: 'TSM ADR',
  institutional: '投信',
  dealer: '自營商',
  retail: '散戶',
  margin: '融資',
  short: '融券',
  dxy: '美元指數',
  vix: 'VIX',
  taiex: '加權指數',
};

const QUALITY_LABEL = {
  strong_inflow: '強勁流入',
  inflow: '流入',
  neutral: '中性',
  outflow: '流出',
  strong_outflow: '強勁流出',
};

function forceLabel(f) {
  return FORCE_LABEL[f.force] || f.display_name || f.force || '—';
}

function qualityLabel(value) {
  const key = String(value).toLowerCase();
  return QUALITY_LABEL[key] || value || '—';
}

export const template = `
<details class="help-details"><summary><strong>💡 如何解讀本頁</strong></summary>
  錢潮看板顯示七維資金流模型的即時摘要：上方「模型資料」卡片統計偏多 / 偏空 / 中性的勢力數量與
  Z-score 權重佔比，圓餅圖視覺化整體力道分佈；下方「錢潮看板」表格列出各勢力（外資、投信、自營商、
  散戶、跨市場訊號等）的 Z-score、原始值與趨勢，並標示主導勢力與市場品質分數。
</details>
<section id="cb-summary" class="cb-summary" aria-live="polite"></section>
<section id="cb-chart" class="cb-chart" aria-live="polite"></section>
<section id="cb-grid" class="cb-board" aria-live="polite">載入中…</section>
`;

function aggregateForces(data) {
  const forces = Array.isArray(data && data.forces) ? data.forces : [];
  if (!forces.length) return null;

  const entries = forces.map(function (f) {
    const z = typeof f.z_score === 'number' ? f.z_score : 0;
    const trend = f.trend || (z > 0.5 ? 'bullish' : z < -0.5 ? 'bearish' : 'neutral');
    return {
      name: f.force || '',
      label: forceLabel(f),
      z: z,
      trend: trend,
      weight: Math.abs(z),
    };
  });

  const counts = {
    bullish: entries.filter(function (e) { return e.trend === 'bullish'; }).length,
    bearish: entries.filter(function (e) { return e.trend === 'bearish'; }).length,
    neutral: entries.filter(function (e) { return e.trend === 'neutral'; }).length,
  };

  const weights = {
    bullish: entries.filter(function (e) { return e.trend === 'bullish'; }).reduce(function (s, e) { return s + e.weight; }, 0),
    bearish: entries.filter(function (e) { return e.trend === 'bearish'; }).reduce(function (s, e) { return s + e.weight; }, 0),
    neutral: entries.filter(function (e) { return e.trend === 'neutral'; }).reduce(function (s, e) { return s + e.weight; }, 0),
  };

  return { entries: entries, counts: counts, weights: weights };
}

function renderCounts(agg, summary) {
  const total = agg.weights.bullish + agg.weights.bearish + agg.weights.neutral;
  function pct(v) { return total > 0 ? Math.round(v / total * 100) : 0; }

  const qualityScore = summary && typeof summary.quality_score === 'number' ? summary.quality_score.toFixed(2) : '—';
  const qualityText = qualityLabel(summary && summary.quality_label);
  const dominant = summary && summary.dominant_force ? (FORCE_LABEL[summary.dominant_force] || summary.dominant_force) : '—';

  return (
    '<div class="cb-summary__grid">' +
    '<div class="cb-summary__card cb-summary__card--bullish">' +
    '<div class="cb-summary__num">' + agg.counts.bullish + '</div>' +
    '<div class="cb-summary__label">偏多勢力</div>' +
    '<div class="cb-summary__weight">權重 ' + pct(agg.weights.bullish) + '%</div>' +
    '</div>' +
    '<div class="cb-summary__card cb-summary__card--bearish">' +
    '<div class="cb-summary__num">' + agg.counts.bearish + '</div>' +
    '<div class="cb-summary__label">偏空勢力</div>' +
    '<div class="cb-summary__weight">權重 ' + pct(agg.weights.bearish) + '%</div>' +
    '</div>' +
    '<div class="cb-summary__card cb-summary__card--neutral">' +
    '<div class="cb-summary__num">' + agg.counts.neutral + '</div>' +
    '<div class="cb-summary__label">中性勢力</div>' +
    '<div class="cb-summary__weight">權重 ' + pct(agg.weights.neutral) + '%</div>' +
    '</div>' +
    '<div class="cb-summary__card cb-summary__card--total">' +
    '<div class="cb-summary__num">' + agg.entries.length + '</div>' +
    '<div class="cb-summary__label">彙總勢力數</div>' +
    '<div class="cb-summary__weight">主導：' + escapeHtml(dominant) + '<br>品質：' + escapeHtml(qualityText) + '（' + qualityScore + '）</div>' +
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
    { key: 'bullish', label: '偏多', weight: agg.weights.bullish, color: 'var(--trend-bullish)' },
    { key: 'bearish', label: '偏空', weight: agg.weights.bearish, color: 'var(--trend-bearish)' },
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
    '<svg viewBox="0 0 120 120" class="cb-chart__svg" role="img" aria-label="力道分佈圓餅圖">' + paths + '</svg>' +
    '<div class="cb-chart__legend">' + legend + '</div>' +
    '</div>'
  );
}

function renderForceRows(entries) {
  if (!entries || !entries.length) {
    return '<div class="cb-board__sectors text-muted">目前無勢力資料</div>';
  }

  const maxWeight = Math.max.apply(null, entries.map(function (e) { return e.weight; }).concat([1]));

  return (
    '<div class="cb-board__sectors">' +
    entries.map(function (e) {
      const verdict = e.trend;
      const verdictLabel = verdict === 'bullish' ? '偏多' : verdict === 'bearish' ? '偏空' : '中性';
      const verdictColor = financialColor(verdict === 'bullish' ? 1 : verdict === 'bearish' ? -1 : 0, 'trend');
      const width = Math.min(100, e.weight / maxWeight * 100).toFixed(1);
      const colorVar = verdict === 'bullish' ? 'var(--trend-bullish)' : verdict === 'bearish' ? 'var(--trend-bearish)' : 'var(--muted)';
      return (
        '<div class="cb-sector-row" data-verdict="' + verdict + '" title="Z-score ' + e.z.toFixed(2) + '">' +
        '<span class="cb-sector-row__name">' + escapeHtml(e.label) + '</span>' +
        '<span class="cb-sector-row__bar" aria-hidden="true">' +
        '<span style="width:' + width + '%;background:color-mix(in srgb, ' + colorVar + ' 45%, transparent)"></span>' +
        '</span>' +
        '<span class="cb-sector-row__verdict" style="color:' + verdictColor + '">' + verdictLabel + '</span>' +
        '</div>'
      );
    }).join('') +
    '</div>'
  );
}

function renderModelMeta(summary, daily) {
  const resonance = summary && summary.resonance_dir ? summary.resonance_dir : (daily && daily.resonance_dir ? daily.resonance_dir : '—');
  return (
    '<div class="text-muted" style="font-size:12px;margin-bottom:8px">' +
    '共振方向：' + escapeHtml(resonance) + ' · ' +
    '資料時間：' + escapeHtml((summary && summary.date) || (daily && daily.date) || '—') +
    '</div>'
  );
}

async function loadBoard() {
  const results = await Promise.all([
    silentGetJSON('/api/capital-flow/summary'),
    silentGetJSON('/api/capital-flow/daily'),
  ]);
  const summary = results[0];
  const daily = results[1];

  const summaryEl = document.getElementById('cb-summary');
  const chartEl = document.getElementById('cb-chart');
  const gridEl = document.getElementById('cb-grid');

  if (!summaryEl || !chartEl || !gridEl) return;

  if (summary === null && daily === null) {
    summaryEl.innerHTML = renderMissingState('模型資料', 'api-error');
    chartEl.innerHTML = '';
    gridEl.innerHTML = renderMissingState('錢潮看板', 'api-error');
    return;
  }

  const data = (daily && Array.isArray(daily.forces) && daily.forces.length)
    ? daily
    : (summary && Array.isArray(summary.forces) && summary.forces.length)
      ? summary
      : null;

  if (!data) {
    summaryEl.innerHTML = renderMissingState('模型資料', 'no-data');
    chartEl.innerHTML = '';
    gridEl.innerHTML = '<div class="empty">目前無資金流資料</div>';
    return;
  }

  const agg = aggregateForces(data);
  if (!agg) {
    summaryEl.innerHTML = renderMissingState('模型資料', 'no-data');
    chartEl.innerHTML = '';
    gridEl.innerHTML = '<div class="empty">目前無資金流資料</div>';
    return;
  }

  summaryEl.innerHTML = '<h3 class="cb-board__title">模型資料</h3>' + renderModelMeta(summary, daily) + renderCounts(agg, summary || daily);
  chartEl.innerHTML = '<h3 class="cb-board__title">權重佔比圓餅圖</h3>' + renderPieChart(agg);
  gridEl.innerHTML = '<h3 class="cb-board__title">錢潮看板</h3>' + renderForceRows(agg.entries);
}

export async function init() {
  await loadBoard();
}
