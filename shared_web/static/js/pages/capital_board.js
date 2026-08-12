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

// ============================================================
// 近期歷史趨勢（client 專用，5 個 dimension，與 admin 的 7 個區隔）
// ============================================================
// 與 admin /admin/capital_history 共用 backend endpoint：
//   GET /api/capital-flow/history?days=30|60
// 與 admin 的差異：
//   - 不含 `government` 維度（spec §6.1：公股口徑未定，operator-imported，
//     畫出來是全 0 線，會誤導 client 用戶認為「無變化」實為「無資料」）
//   - 共用 admin 既有圖表邏輯（Canvas 折線、零線、軸、圖例），
//     state 全部 closure 化，與 admin 模組變數完全隔離

import { getThemeColor } from '../shared/utils.js';
import { hexToRgba } from '../shared/color-tokens.js';
import { fmtSafeNumber } from '../shared/format-metric.js';

const CBH_DIMENSIONS = [
  { key: 'foreign',       label: '外資',       color: '#e74c3c' },
  { key: 'institutional', label: '投信',       color: '#3498db' },
  { key: 'dealer',        label: '自營商',     color: '#2ecc71' },
  { key: 'retail',        label: '散戶',       color: '#9b59b6' },
  { key: 'futures',       label: '外資期貨OI', color: '#1abc9c' },
  { key: 'tsm_adr',       label: 'TSM ADR',    color: '#e67e22' },
];
const CBH_DEFAULT_VISIBLE = new Set(['foreign', 'institutional', 'dealer']);
const CBH_RETRY_ID = 'cb-capital-history';

function cbhUpdateAllChip(allChip) {
  const allOn = CBH_DEFAULT_VISIBLE.size === CBH_DIMENSIONS.length;
  if (allOn) {
    allChip.classList.add('active');
    allChip.style.color = 'var(--accent, #5b8def)';
    allChip.style.borderColor = 'var(--accent, #5b8def)';
  } else {
    allChip.classList.remove('active');
    allChip.style.color = '';
    allChip.style.borderColor = '';
  }
}

function cbhRenderToggles(root) {
  const toggles = root.querySelector('#cbhToggles');
  if (!toggles) return;
  toggles.innerHTML = '';

  // 「全部」chip：點擊在「全亮」與「預設官方三方」之間切換
  const allChip = document.createElement('span');
  allChip.className = 'cbh-chip cbh-chip--all';
  allChip.setAttribute('data-dim', '__all__');
  allChip.textContent = '全部';
  allChip.addEventListener('click', function () {
    const allOn = CBH_DEFAULT_VISIBLE.size === CBH_DIMENSIONS.length;
    CBH_DEFAULT_VISIBLE.clear();
    if (!allOn) {
      CBH_DIMENSIONS.forEach(function (d) { CBH_DEFAULT_VISIBLE.add(d.key); });
    } else {
      // 從全亮回到 admin 預設（官方三方）
      ['foreign', 'institutional', 'dealer'].forEach(function (k) { CBH_DEFAULT_VISIBLE.add(k); });
    }
    // 重繪整排 chip
    cbhRenderToggles(root);
    cbhRenderChart();
  });
  toggles.appendChild(allChip);
  cbhUpdateAllChip(allChip);

  CBH_DIMENSIONS.forEach(function (d) {
    const chip = document.createElement('span');
    const on = CBH_DEFAULT_VISIBLE.has(d.key);
    chip.className = 'cbh-chip' + (on ? ' active' : '');
    chip.style.borderColor = d.color;
    chip.style.color = on ? d.color : '';
    chip.textContent = d.label;
    chip.setAttribute('data-dim', d.key);
    chip.addEventListener('click', function () {
      if (CBH_DEFAULT_VISIBLE.has(d.key)) {
        CBH_DEFAULT_VISIBLE.delete(d.key);
        chip.classList.remove('active');
        chip.style.color = '';
      } else {
        CBH_DEFAULT_VISIBLE.add(d.key);
        chip.classList.add('active');
        chip.style.color = d.color;
      }
      cbhUpdateAllChip(allChip);
      cbhRenderChart();
    });
    toggles.appendChild(chip);
  });
}

function cbhRenderChart() {
  const canvas = document.getElementById('cbhCanvas');
  if (!canvas || !cbhState.data) return;

  const datasets = [];
  CBH_DIMENSIONS.forEach(function (d) {
    if (!CBH_DEFAULT_VISIBLE.has(d.key)) return;
    const samples = cbhState.data[d.key] || [];
    if (samples.length < 2) return;
    datasets.push({ key: d.key, label: d.label, color: d.color, samples: samples });
  });
  if (datasets.length === 0) {
    canvas.style.display = 'none';
    return;
  }
  canvas.style.display = '';

  const ctx = canvas.getContext('2d');
  const dpr = window.devicePixelRatio || 1;
  const parent = canvas.parentElement;
  const W = parent.clientWidth;
  const H = 320;
  canvas.width = W * dpr;
  canvas.height = H * dpr;
  canvas.style.width = W + 'px';
  canvas.style.height = H + 'px';
  ctx.scale(dpr, dpr);

  const pad = { top: 20, right: 24, bottom: 40, left: 60 };
  const chartW = W - pad.left - pad.right;
  const chartH = H - pad.top - pad.bottom;

  const allVals = [];
  datasets.forEach(function (ds) {
    ds.samples.forEach(function (s) {
      if (Number.isFinite(s.raw_value)) allVals.push(s.raw_value);
    });
  });
  if (allVals.length === 0) return;
  let minV = Math.min.apply(null, allVals);
  let maxV = Math.max.apply(null, allVals);
  const range = (maxV - minV) || 1;
  minV = minV - range * 0.1;
  maxV = maxV + range * 0.1;
  const vRange = maxV - minV || 1;

  ctx.clearRect(0, 0, W, H);

  const bg = getThemeColor('--panel') || '#13161c';
  ctx.fillStyle = hexToRgba(bg, 0.4);
  ctx.beginPath();
  ctx.roundRect(pad.left, pad.top, chartW, chartH, 6);
  ctx.fill();

  const muted = getThemeColor('--muted') || '#b8c4d0';
  ctx.strokeStyle = hexToRgba(muted, 0.08);
  ctx.lineWidth = 0.5;
  for (let i = 1; i <= 3; i++) {
    const y = pad.top + (chartH / 4) * i;
    ctx.beginPath();
    ctx.moveTo(pad.left, y);
    ctx.lineTo(pad.left + chartW, y);
    ctx.stroke();
  }

  if (minV < 0 && maxV > 0) {
    const zeroY = pad.top + (1 - (0 - minV) / vRange) * chartH;
    ctx.strokeStyle = hexToRgba(muted, 0.25);
    ctx.lineWidth = 1;
    ctx.setLineDash([4, 4]);
    ctx.beginPath();
    ctx.moveTo(pad.left, zeroY);
    ctx.lineTo(pad.left + chartW, zeroY);
    ctx.stroke();
    ctx.setLineDash([]);
  }

  ctx.fillStyle = hexToRgba(muted, 0.6);
  ctx.font = '10px system-ui';
  ctx.textAlign = 'right';
  for (let i = 0; i <= 4; i++) {
    const y = pad.top + (chartH / 4) * i;
    const val = maxV - (vRange / 4) * i;
    ctx.fillText(fmtSafeNumber(val, { decimals: 0 }), pad.left - 8, y + 3);
  }

  datasets.forEach(function (ds) {
    if (ds.samples.length < 2) return;
    const pts = ds.samples.map(function (s, i) {
      const x = pad.left + (i / Math.max(ds.samples.length - 1, 1)) * chartW;
      const y = pad.top + (1 - (s.raw_value - minV) / vRange) * chartH;
      return { x: x, y: y, label: s.trading_date, value: s.raw_value };
    });

    ctx.save();
    ctx.shadowColor = hexToRgba(ds.color, 0.35);
    ctx.shadowBlur = 5;
    ctx.strokeStyle = ds.color;
    ctx.lineWidth = 2;
    ctx.lineJoin = 'round';
    ctx.beginPath();
    pts.forEach(function (p, i) {
      if (i === 0) ctx.moveTo(p.x, p.y);
      else ctx.lineTo(p.x, p.y);
    });
    ctx.stroke();
    ctx.restore();

    if (pts.length <= 30) {
      ctx.fillStyle = ds.color;
      pts.forEach(function (p) {
        ctx.beginPath();
        ctx.arc(p.x, p.y, 2.5, 0, Math.PI * 2);
        ctx.fill();
      });
    }
  });

  const longest = datasets.reduce(function (a, b) {
    return a.samples.length >= b.samples.length ? a : b;
  });
  ctx.fillStyle = hexToRgba(muted, 0.5);
  ctx.font = '9px system-ui';
  ctx.textAlign = 'center';
  const step = Math.max(1, Math.floor(longest.samples.length / 6));
  longest.samples.forEach(function (s, i) {
    if (i % step === 0 || i === longest.samples.length - 1) {
      const x = pad.left + (i / Math.max(longest.samples.length - 1, 1)) * chartW;
      const label = s.trading_date.length >= 10 ? s.trading_date.slice(5) : s.trading_date;
      ctx.fillText(label, x, pad.top + chartH + 16);
    }
  });

  ctx.fillStyle = hexToRgba(muted, 0.4);
  ctx.font = '9px system-ui';
  ctx.textAlign = 'left';
  ctx.fillText('億股／口數／%', pad.left, pad.top - 6);

  ctx.font = '10px system-ui';
  ctx.textAlign = 'left';
  let lx = pad.left + 8;
  datasets.forEach(function (ds) {
    ctx.fillStyle = ds.color;
    ctx.fillRect(lx, pad.top + chartH + 28, 10, 10);
    ctx.fillStyle = hexToRgba(muted, 0.8);
    ctx.fillText(ds.label, lx + 14, pad.top + chartH + 37);
    lx += ctx.measureText(ds.label).width + 28;
  });
}

const cbhState = { days: 60, data: null };

async function cbhLoad() {
  const root = document.getElementById('cbhRoot');
  if (!root) return;
  root.classList.add('loading');
  try {
    const data = await silentGetJSON('/api/capital-flow/history?days=' + cbhState.days);
    if (data === null) {
      root.classList.remove('loading');
      root.innerHTML = renderMissingState('近期歷史趨勢', CBH_RETRY_ID);
      const btn = root.querySelector('[data-retry="' + CBH_RETRY_ID + '"]');
      if (btn) btn.addEventListener('click', cbhLoad);
      return;
    }
    cbhState.data = data;
    root.classList.remove('loading');
    root.innerHTML =
      '<div class="cbh-controls">' +
        '<label>天數：' +
          '<select id="cbhDays">' +
            '<option value="30"' + (cbhState.days === 30 ? ' selected' : '') + '>30 天</option>' +
            '<option value="60"' + (cbhState.days === 60 ? ' selected' : '') + '>60 天</option>' +
          '</select>' +
        '</label>' +
        '<span class="cbh-toggles" id="cbhToggles"></span>' +
      '</div>' +
      '<div class="cbh-chart-wrap"><canvas id="cbhCanvas"></canvas></div>';
    cbhRenderToggles(root);
    cbhRenderChart();
    const sel = document.getElementById('cbhDays');
    if (sel) sel.addEventListener('change', function () {
      cbhState.days = parseInt(this.value, 10);
      cbhLoad();
    });
  } catch (err) {
    console.error('[cbh] load failed', err);
    root.classList.remove('loading');
    root.innerHTML = renderMissingState('近期歷史趨勢', CBH_RETRY_ID);
    const btn = root.querySelector('[data-retry="' + CBH_RETRY_ID + '"]');
    if (btn) btn.addEventListener('click', cbhLoad);
  }
}
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
<section id="cb-history" class="cb-history">
  <h3 class="cb-board__title">近期歷史趨勢</h3>
  <div id="cbhRoot" class="cbh-root">載入中…</div>
</section>
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
  // 觸發近期歷史趨勢載入（不 await：與 summary/daily 並行，失敗不影響主面板）
  cbhLoad();
}

export async function init() {
  await loadBoard();
}
