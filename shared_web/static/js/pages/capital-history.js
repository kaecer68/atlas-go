// shared_web/static/js/pages/capital-history.js
//
// H02：三大法人歷史趨勢圖。
// 從 GET /api/capital-flow/history?days=60 讀取多日 RollingSample，
// 以 Canvas 折線圖呈現外資/投信/自營商等七大勢力的累積買賣超趨勢。

import { silentGetJSON, escapeHtml, renderErrorState } from '../shared/app-utils.js';
import { fmtSafeNumber } from '../shared/format-metric.js';
import { getThemeColor } from '../shared/utils.js';
import { hexToRgba } from '../shared/color-tokens.js';

const RETRY_ID = 'capital-history';

// Dimension display config — ordered as 3+2+2 per E07/E08 spec.
const DIMENSIONS = [
  { key: 'foreign',       label: '外資',       role: 'official',  color: '#e74c3c' },
  { key: 'institutional', label: '投信',       role: 'official',  color: '#3498db' },
  { key: 'dealer',        label: '自營商',     role: 'official',  color: '#2ecc71' },
  { key: 'government',    label: '公股行庫',   role: 'proxy',     color: '#f39c12' },
  { key: 'retail',        label: '散戶',       role: 'proxy',     color: '#9b59b6' },
  { key: 'futures',       label: '外資期貨OI', role: 'indicator', color: '#1abc9c' },
  { key: 'tsm_adr',       label: 'TSM ADR',    role: 'signal',    color: '#e67e22' },
];

// Default visible: official actors only.
const DEFAULT_VISIBLE = new Set(['foreign', 'institutional', 'dealer']);

let currentDays = 60;
let currentData = null;

export async function loadCapitalHistory() {
  const el = document.getElementById('capitalHistoryContent');
  if (!el) return;
  el.classList.add('loading');
  el.innerHTML = '<div class="loading-spinner"></div>';

  try {
    const data = await silentGetJSON('/api/capital-flow/history?days=' + currentDays);
    if (data === null) {
      el.classList.remove('loading');
      el.innerHTML = renderErrorState('三大法人歷史趨勢', RETRY_ID);
      const btn = el.querySelector('[data-retry="' + RETRY_ID + '"]');
      if (btn) btn.addEventListener('click', loadCapitalHistory);
      return;
    }
    currentData = data;
    renderPage(el, data);
  } catch (err) {
    console.error('[capital-history] load failed', err);
    el.classList.remove('loading');
    el.innerHTML = renderErrorState('三大法人歷史趨勢', RETRY_ID);
    const btn = el.querySelector('[data-retry="' + RETRY_ID + '"]');
    if (btn) btn.addEventListener('click', loadCapitalHistory);
  }
}

function renderPage(container, data) {
  container.classList.remove('loading');
  container.innerHTML = '';

  // Controls bar: day range selector + visibility toggles
  const controls = document.createElement('div');
  controls.className = 'capital-history-controls';
  controls.innerHTML =
    '<label>天數：' +
    '<select id="chDays">' +
    '<option value="30"' + (currentDays === 30 ? ' selected' : '') + '>30 天</option>' +
    '<option value="60"' + (currentDays === 60 ? ' selected' : '') + '>60 天</option>' +
    '</select>' +
    '</label>' +
    '<span class="capital-history-toggles" id="chToggles"></span>';
  container.appendChild(controls);

  // Chart canvas
  const chartWrap = document.createElement('div');
  chartWrap.className = 'capital-history-chart-wrap';
  chartWrap.innerHTML = '<canvas id="capitalHistoryCanvas"></canvas>';
  container.appendChild(chartWrap);

  // Dimension toggle chips
  renderToggles();

  // Initial chart render
  renderChart();

  // Day selector
  document.getElementById('chDays').addEventListener('change', function () {
    currentDays = parseInt(this.value, 10);
    loadCapitalHistory();
  });
}

function renderToggles() {
  const toggles = document.getElementById('chToggles');
  if (!toggles) return;
  toggles.innerHTML = '';
  DIMENSIONS.forEach(function (d) {
    const chip = document.createElement('span');
    chip.className = 'capital-history-chip' + (DEFAULT_VISIBLE.has(d.key) ? ' active' : '');
    chip.style.borderColor = d.color;
    chip.style.color = DEFAULT_VISIBLE.has(d.key) ? d.color : '';
    chip.textContent = d.label;
    chip.setAttribute('data-dim', d.key);
    chip.addEventListener('click', function () {
      if (DEFAULT_VISIBLE.has(d.key)) {
        DEFAULT_VISIBLE.delete(d.key);
        chip.classList.remove('active');
        chip.style.color = '';
      } else {
        DEFAULT_VISIBLE.add(d.key);
        chip.classList.add('active');
        chip.style.color = d.color;
      }
      renderChart();
    });
    toggles.appendChild(chip);
  });
}

function renderChart() {
  const canvas = document.getElementById('capitalHistoryCanvas');
  if (!canvas || !currentData) return;

  // Collect visible datasets with at least 1 data point.
  const datasets = [];
  DIMENSIONS.forEach(function (d) {
    if (!DEFAULT_VISIBLE.has(d.key)) return;
    const samples = currentData[d.key] || [];
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

  // Compute global value range across all visible datasets.
  let allVals = [];
  datasets.forEach(function (ds) {
    ds.samples.forEach(function (s) {
      if (Number.isFinite(s.raw_value)) allVals.push(s.raw_value);
    });
  });
  if (allVals.length === 0) return;
  let minV = Math.min.apply(null, allVals);
  let maxV = Math.max.apply(null, allVals);
  const range = (maxV - minV) || 1;
  // Pad range by 10% for visual breathing room.
  minV = minV - range * 0.1;
  maxV = maxV + range * 0.1;
  const vRange = maxV - minV || 1;

  ctx.clearRect(0, 0, W, H);

  // Background
  const bg = getThemeColor('--panel') || '#13161c';
  ctx.fillStyle = hexToRgba(bg, 0.4);
  ctx.beginPath();
  ctx.roundRect(pad.left, pad.top, chartW, chartH, 6);
  ctx.fill();

  // Grid lines
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

  // Zero line (if zero is within visible range)
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

  // Y-axis labels
  ctx.fillStyle = hexToRgba(muted, 0.6);
  ctx.font = '10px system-ui';
  ctx.textAlign = 'right';
  for (let i = 0; i <= 4; i++) {
    const y = pad.top + (chartH / 4) * i;
    const val = maxV - (vRange / 4) * i;
    ctx.fillText(fmtSafeNumber(val, { decimals: 0 }), pad.left - 8, y + 3);
  }

  // Plot each dataset.
  // Use the longest dataset to normalize X axis (some dimensions may have gaps).
  let maxLen = 0;
  datasets.forEach(function (ds) {
    if (ds.samples.length > maxLen) maxLen = ds.samples.length;
  });

  datasets.forEach(function (ds) {
    if (ds.samples.length < 2) return;
    const pts = ds.samples.map(function (s, i) {
      const x = pad.left + (i / Math.max(ds.samples.length - 1, 1)) * chartW;
      const y = pad.top + (1 - (s.raw_value - minV) / vRange) * chartH;
      return { x: x, y: y, label: s.trading_date, value: s.raw_value };
    });

    // Glow + line
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

    // Dots (only when ≤ 30 points for legibility)
    if (pts.length <= 30) {
      ctx.fillStyle = ds.color;
      pts.forEach(function (p) {
        ctx.beginPath();
        ctx.arc(p.x, p.y, 2.5, 0, Math.PI * 2);
        ctx.fill();
      });
    }
  });

  // X-axis labels (use the longest dataset for ticks)
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
      // Show MM-DD only.
      const label = s.trading_date.length >= 10 ? s.trading_date.slice(5) : s.trading_date;
      ctx.fillText(label, x, pad.top + chartH + 16);
    }
  });

  // Unit label
  ctx.fillStyle = hexToRgba(muted, 0.4);
  ctx.font = '9px system-ui';
  ctx.textAlign = 'left';
  ctx.fillText('億股／口數／%', pad.left, pad.top - 6);

  // Legend
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
