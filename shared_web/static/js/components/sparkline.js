// sparkline.js — Darwinian weight sparkline + equity curve + agent scoreboard
// Standalone component, importable into dashboard.js or portfolio pages.
import { fmtInt, fmtNTD, getThemeColor } from '../shared/utils.js';
import { renderDualLineChart } from './line-chart.js';
import { fmtSafeNumber, fmtSafePct } from '../shared/format-metric.js';
// formatNumber is used for canvas axis labels to avoid scattered toFixed() calls.
import { pnlProfitColor, pnlLossColor, regimeColor, hexToRgba } from '../shared/color-tokens.js';

export { renderEquityCurve, renderDualEquityCurve, renderAgentScoreboard, renderRegimeContext, renderAllocationGuidance };

/**
 * Renders a canvas-based equity curve sparkline.
 * @param {Array<{value: number, label: string}>} points - Equity data points
 */
function renderEquityCurve(points) {
  const panel = document.getElementById('equityCurvePanel');
  const canvas = document.getElementById('equityChart');
  if (!panel || !canvas) return;
  if (!points || points.length < 2) { panel.style.display = 'none'; return; }
  panel.style.display = '';
  const ctx = canvas.getContext('2d');
  const dpr = window.devicePixelRatio || 1;
  const rect = canvas.parentElement.getBoundingClientRect();
  const W = rect.width - 40, H = 220;
  canvas.width = W * dpr; canvas.height = H * dpr;
  canvas.style.width = W + 'px'; canvas.style.height = H + 'px';
  ctx.scale(dpr, dpr);
  const pad = {top: 20, right: 20, bottom: 28, left: 50};
  const chartW = W - pad.left - pad.right, chartH = H - pad.top - pad.bottom;

  const values = points.map(p => p.value).filter(Number.isFinite);
  if (values.length < 2) { panel.style.display = 'none'; return; }
  const minV = Math.min(...values), maxV = Math.max(...values), range = maxV - minV || 1;
  ctx.clearRect(0, 0, W, H);

  // Background
  ctx.fillStyle = hexToRgba(getThemeColor('--panel') || '#13161c', 0.6);
  ctx.beginPath(); ctx.roundRect(pad.left, pad.top, chartW, chartH, 6); ctx.fill();

  // Subtle grid
  ctx.strokeStyle = hexToRgba(getThemeColor('--text') || '#f0f4f8', 0.05); ctx.lineWidth = 0.5;
  for (let i = 1; i <= 3; i++) {
    const y = pad.top + (chartH / 4) * i;
    ctx.beginPath(); ctx.moveTo(pad.left, y); ctx.lineTo(pad.left + chartW, y); ctx.stroke();
  }

  // Y-axis labels
  ctx.fillStyle = hexToRgba(getThemeColor('--muted') || '#b8c4d0', 0.6); ctx.font = '10px system-ui'; ctx.textAlign = 'right';
  for (let i = 0; i <= 4; i++) {
    const y = pad.top + (chartH / 4) * i;
    ctx.fillText(fmtSafeNumber(maxV - (range / 4) * i, { decimals: 0 }), pad.left - 8, y + 3);
  }

  // Compute points
  const pts = points.map((p, i) => ({
    x: pad.left + (i / (points.length - 1)) * chartW,
    y: pad.top + (1 - (p.value - minV) / range) * chartH
  }));

  const accentColor = getThemeColor('--accent') || '#4fc1ff';

  // Gradient fill under curve
  const gradient = ctx.createLinearGradient(0, pad.top, 0, pad.top + chartH);
  gradient.addColorStop(0, hexToRgba(accentColor, 0.25));
  gradient.addColorStop(1, hexToRgba(accentColor, 0.02));
  ctx.beginPath();
  ctx.moveTo(pts[0].x, pad.top + chartH);
  for (let i = 0; i < pts.length; i++) ctx.lineTo(pts[i].x, pts[i].y);
  ctx.lineTo(pts[pts.length - 1].x, pad.top + chartH);
  ctx.closePath();
  ctx.fillStyle = gradient; ctx.fill();

  // Glow
  ctx.save();
  ctx.shadowColor = hexToRgba(accentColor, 0.4); ctx.shadowBlur = 6;
  ctx.strokeStyle = accentColor;
  ctx.lineWidth = 2.2; ctx.lineJoin = 'round'; ctx.beginPath();
  pts.forEach((p, i) => i === 0 ? ctx.moveTo(p.x, p.y) : ctx.lineTo(p.x, p.y));
  ctx.stroke();
  ctx.restore();

  // Dots on data points
  if (pts.length <= 30) {
    ctx.fillStyle = accentColor;
    pts.forEach(p => { ctx.beginPath(); ctx.arc(p.x, p.y, 2.5, 0, Math.PI * 2); ctx.fill(); });
  }

  // X-axis labels
  ctx.fillStyle = hexToRgba(getThemeColor('--muted') || '#b8c4d0', 0.5); ctx.font = '9px system-ui'; ctx.textAlign = 'center';
  const step = Math.max(1, Math.floor(points.length / 6));
  points.forEach((p, i) => {
    if (i % step === 0 || i === points.length - 1) {
      ctx.fillText(p.label, pad.left + (i / (points.length - 1)) * chartW, pad.top + chartH + 18);
    }
  });
}

/**
 * Renders a dual-curve equity chart showing pre-tax and after-tax values.
 * @param {Array<{value: number, label: string}>} preTaxPoints - Pre-tax equity data points
 * @param {Array<{value: number, label: string}>} afterTaxPoints - After-tax equity data points
 */
function renderDualEquityCurve(preTaxPoints, afterTaxPoints) {
  // SSOT Phase 2 (P2-4)：繪圖邏輯抽至共用 components/line-chart.js；
  // 本函式保留既有對外合約（target #equityCurvePanel / #equityChart，
  // 資料不足時自行隱藏面板），供持倉風控台 S3 與舊呼叫點使用。
  const panel = document.getElementById('equityCurvePanel');
  const canvas = document.getElementById('equityChart');
  if (!panel || !canvas) return;
  if (!preTaxPoints || preTaxPoints.length < 2) { panel.style.display = 'none'; return; }

  const series = [];
  if (afterTaxPoints && afterTaxPoints.length > 0) {
    series.push({ label: '稅後淨值 (After-tax)', color: getThemeColor('--color-warning') || '#f59e0b', points: afterTaxPoints });
  }
  series.push({ label: '稅前淨值 (Pre-tax)', color: getThemeColor('--accent') || '#4fc1ff', points: preTaxPoints });

  const drew = renderDualLineChart({
    canvas: canvas,
    yFormat: fmtNTD,
    series: series,
    height: 220,
  });
  panel.style.display = drew ? '' : 'none';
}

/**
 * Renders agent scoreboard with Darwinian weights, Sharpe ratio, hit rate.
 * @param {object} dw - Darwinian status data { agents: [{agent_id, weight, rolling_sharpe, hit_rate, signal_count}] }
 * @param {function} agentNameFn - Maps agent_id to display name
 */
function renderAgentScoreboard(dw, agentNameFn) {
  const el = document.getElementById('agentScoreboard');
  if (!el) return;
  const nameFn = agentNameFn || (id => id);
  const agents = (dw && dw.agents) || [];
  if (!agents.length) { el.innerHTML = '<div class="muted">No agent data</div>'; return; }

  let html = '<table class="scoreboard-table"><thead><tr>';
  html += '<th>Agent</th><th>Weight</th><th>Sharpe</th><th>Hit Rate</th><th>Signals</th>';
  html += '</tr></thead><tbody>';

  for (const a of agents) {
    const sharpe = a.rolling_sharpe;
    let sharpeColor = 'var(--muted)';
    if (typeof sharpe === 'number') {
      sharpeColor = sharpe > 1 ? pnlProfitColor() : (sharpe < 0 ? pnlLossColor() : 'var(--warn)');
    }
    html += '<tr>';
    html += `<td>${nameFn(a.agent_id)}</td>`;
    html += `<td>${fmtSafeNumber(a.weight, { decimals: 2, useGrouping: true })}</td>`;
    html += `<td style="color:${sharpeColor}">${fmtSafeNumber(sharpe, { decimals: 2, useGrouping: true })}</td>`;
    html += `<td>${fmtSafePct(a.hit_rate)}</td>`;
    html += `<td>${fmtInt(a.signal_count)}</td>`;
    html += '</tr>';
  }
  html += '</tbody></table>';
  el.innerHTML = html;
}

/**
 * Renders the current regime context (RISK_ON/OFF/NEUTRAL).
 * @param {object} ps - Portfolio state { regime, regime_since }
 * @param {object} regimeData - Regime history data
 */
function renderRegimeContext(ps, regimeData) {
  const el = document.getElementById('regimeContext');
  if (!el) return;
  const regime = (ps && ps.regime) || 'UNKNOWN';
  const color = regimeColor(regime);
  let html = `<span class="regime-badge" style="background:${color}">${regime}</span>`;
  if (ps && ps.regime_since) {
    html += ` <span class="regime-since">since ${ps.regime_since}</span>`;
  }
  el.innerHTML = html;
}

/**
 * Renders allocation guidance based on portfolio size tier.
 * @param {object} ps - Portfolio state { total_assets }
 */
function renderAllocationGuidance(ps) {
  const el = document.getElementById('allocationGuidance');
  if (!el) return;
  const assets = (ps && ps.total_assets) || 0;
  let tier, guidance;
  if (assets < 500000) {
    tier = 'Small (<50萬)';
    guidance = 'Concentrated 3-5 positions, focus on conviction';
  } else if (assets < 3000000) {
    tier = 'Medium (50-300萬)';
    guidance = 'Diversified 5-10 positions, sector rotation';
  } else {
    tier = 'Large (>300萬)';
    guidance = 'Index-like 10-20 positions, risk parity';
  }
  el.innerHTML = `
    <div class="guidance-tier">${tier}</div>
    <div class="guidance-text">${guidance}</div>
  `;
}


function showTooltip(e, html) {
  let tooltip = document.getElementById('sparkline-tooltip');
  if (!tooltip) {
    tooltip = document.createElement('div');
    tooltip.id = 'sparkline-tooltip';
    tooltip.style.position = 'absolute';
    tooltip.style.pointerEvents = 'none';
    tooltip.style.background = getThemeColor('--bg', '#13161c');
    tooltip.style.border = `1px solid ${getThemeColor('--border', '#2d333b')}`;
    tooltip.style.color = getThemeColor('--text', '#fff');
    tooltip.style.padding = '8px 12px';
    tooltip.style.borderRadius = '6px';
    tooltip.style.fontSize = '12px';
    tooltip.style.boxShadow = '0 4px 12px rgba(0,0,0,0.5)';
    tooltip.style.zIndex = '9999';
    tooltip.style.lineHeight = '1.4';
    document.body.appendChild(tooltip);
  }
  tooltip.innerHTML = html;
  tooltip.style.display = 'block';
  let x = e.pageX + 15;
  let y = e.pageY + 15;
  if (x + tooltip.offsetWidth > window.innerWidth) x = e.pageX - tooltip.offsetWidth - 15;
  tooltip.style.left = x + 'px';
  tooltip.style.top = y + 'px';
}

function hideTooltip() {
  const tooltip = document.getElementById('sparkline-tooltip');
  if (tooltip) tooltip.style.display = 'none';
}

/**
 * Renders a multi-line comparison chart (e.g., baseline vs candidate monetary values).
 * @param {string} containerId - The ID of the container element
 * @param {Array<{label: string, data: Array<{label: string, value: number}>, color: string, glow: string}>} datasets - Data for lines
 * @param {object} options - Chart options { height: 220 }
 */
export function renderComparisonChart(containerId, datasets, options = {}) {
  const container = document.getElementById(containerId);
  if (!container || !datasets || datasets.length === 0) return;
  
  let canvas = container.querySelector('canvas');
  if (!canvas) {
    canvas = document.createElement('canvas');
    canvas.style.width = '100%';
    canvas.style.height = (options.height || 220) + 'px';
    container.innerHTML = '';
    container.appendChild(canvas);
    
    // Add resize listener
    window.addEventListener('resize', () => renderComparisonChart(containerId, datasets, options));
  }

  const ctx = canvas.getContext('2d');
  const dpr = window.devicePixelRatio || 1;
  const rect = container.getBoundingClientRect();
  const W = rect.width, H = options.height || 220;
  canvas.width = W * dpr; canvas.height = H * dpr;
  ctx.scale(dpr, dpr);
  const pad = {top: 20, right: 20, bottom: 28, left: 60};
  const chartW = W - pad.left - pad.right, chartH = H - pad.top - pad.bottom;

  let allValues = [];
  let maxLen = 0;
  datasets.forEach(ds => {
    if (ds.data.length > maxLen) maxLen = ds.data.length;
    ds.data.forEach(p => {
      if (Number.isFinite(p.value)) allValues.push(p.value);
    });
  });

  if (maxLen < 2 || allValues.length === 0) {
     ctx.fillStyle = getThemeColor('--muted', '#6b7280');
     ctx.font = '12px system-ui';
     ctx.textAlign = 'center';
     ctx.fillText('Not enough data', W/2, H/2);
     return;
  }

  const minV = Math.min(...allValues), maxV = Math.max(...allValues), range = maxV - minV || 1;
  ctx.clearRect(0, 0, W, H);

  // Background
  ctx.fillStyle = hexToRgba(getThemeColor('--panel') || '#13161c', 0.6);
  ctx.beginPath(); ctx.roundRect(pad.left, pad.top, chartW, chartH, 6); ctx.fill();

  // Subtle grid
  ctx.strokeStyle = hexToRgba(getThemeColor('--text') || '#f0f4f8', 0.05); ctx.lineWidth = 0.5;
  for (let i = 1; i <= 3; i++) {
    const y = pad.top + (chartH / 4) * i;
    ctx.beginPath(); ctx.moveTo(pad.left, y); ctx.lineTo(pad.left + chartW, y); ctx.stroke();
  }

  // Y-axis labels
  ctx.fillStyle = hexToRgba(getThemeColor('--muted') || '#b8c4d0', 0.6); ctx.font = '10px system-ui'; ctx.textAlign = 'right';
  for (let i = 0; i <= 4; i++) {
    const y = pad.top + (chartH / 4) * i;
    const val = maxV - (range / 4) * i;
    ctx.fillText(fmtNTD(val), pad.left - 8, y + 3);
  }

  // Points mapping & drawing
  const pointMaps = datasets.map(ds => {
    return ds.data.map((p, i) => ({
      x: pad.left + (i / (ds.data.length - 1)) * chartW,
      y: pad.top + (1 - (p.value - minV) / range) * chartH,
      data: p,
      datasetLabel: ds.label,
      color: ds.color
    }));
  });

  pointMaps.forEach((pts, idx) => {
    const ds = datasets[idx];
    if (pts.length === 0) return;
    
    const rawColor = ds.color.startsWith('var(') ? getThemeColor(ds.color.slice(4, -1)) : ds.color;
    const rawGlow = ds.glow ? (ds.glow.startsWith('var(') ? getThemeColor(ds.glow.slice(4, -1)) : ds.glow) : rawColor;
    
    // Line & Glow
    ctx.save();
    ctx.shadowColor = hexToRgba(rawGlow, 0.4); 
    ctx.shadowBlur = 6;
    ctx.strokeStyle = rawColor;
    ctx.lineWidth = 2.2; ctx.lineJoin = 'round'; ctx.beginPath();
    pts.forEach((p, i) => i === 0 ? ctx.moveTo(p.x, p.y) : ctx.lineTo(p.x, p.y));
    ctx.stroke();
    ctx.restore();

    // Dots
    if (pts.length <= 30) {
      ctx.fillStyle = rawColor;
      pts.forEach(p => { ctx.beginPath(); ctx.arc(p.x, p.y, 2.5, 0, Math.PI * 2); ctx.fill(); });
    }
  });

  // X-axis labels
  ctx.fillStyle = hexToRgba(getThemeColor('--muted') || '#b8c4d0', 0.5); ctx.font = '9px system-ui'; ctx.textAlign = 'center';
  const mainDs = datasets[0].data;
  const step = Math.max(1, Math.floor(mainDs.length / 6));
  mainDs.forEach((p, i) => {
    if (i % step === 0 || i === mainDs.length - 1) {
      ctx.fillText(p.label, pad.left + (i / (mainDs.length - 1)) * chartW, pad.top + chartH + 18);
    }
  });

  // Legend
  ctx.font = '10px system-ui';
  ctx.textAlign = 'left';
  let lx = pad.left + 10;
  datasets.forEach(ds => {
    const rawColor = ds.color.startsWith('var(') ? getThemeColor(ds.color.slice(4, -1)) : ds.color;
    ctx.fillStyle = rawColor;
    ctx.fillRect(lx, pad.top + 10, 10, 10);
    ctx.fillStyle = hexToRgba(getThemeColor('--muted') || '#b8c4d0', 0.8);
    ctx.fillText(ds.label, lx + 15, pad.top + 19);
    lx += ctx.measureText(ds.label).width + 30;
  });

  // Interactive Tooltip
  canvas.onmousemove = (e) => {
    const rect = canvas.getBoundingClientRect();
    const mx = e.clientX - rect.left;
    
    // Find closest index on X axis
    if (mx < pad.left || mx > pad.left + chartW) { hideTooltip(); return; }
    const ratio = (mx - pad.left) / chartW;
    
    let html = '';
    let found = false;
    
    // Collect values from each dataset at this rough X index
    datasets.forEach((ds, idx) => {
       const pts = pointMaps[idx];
       if (!pts.length) return;
       const i = Math.round(ratio * (pts.length - 1));
       if (i >= 0 && i < pts.length) {
         if (!found) {
            html += `<strong>${pts[i].data.label}</strong><br/>`;
            found = true;
         }
         html += `<span style="color:${ds.color}">●</span> ${ds.label}: ${fmtNTD(pts[i].data.value)}<br/>`;
       }
    });
    
    if (found) showTooltip(e, html);
    else hideTooltip();
  };
  canvas.onmouseleave = hideTooltip;
}

/**
 * Renders a radar chart for Agent metrics.
 * @param {string} containerId - Container ID
 * @param {Array<number>} metrics - Array of 5 metrics [0..1]
 * @param {Array<string>} labels - Array of 5 labels
 */
export function renderRadarChart(containerId, metrics, labels) {
  const container = document.getElementById(containerId);
  if (!container || !metrics || metrics.length !== 5) return;
  
  let canvas = container.querySelector('canvas');
  if (!canvas) {
    canvas = document.createElement('canvas');
    canvas.style.width = '100%';
    canvas.style.height = '200px';
    container.innerHTML = '';
    container.appendChild(canvas);
    window.addEventListener('resize', () => renderRadarChart(containerId, metrics, labels));
  }
  
  const ctx = canvas.getContext('2d');
  const dpr = window.devicePixelRatio || 1;
  const rect = container.getBoundingClientRect();
  const W = rect.width, H = 200;
  canvas.width = W * dpr; canvas.height = H * dpr;
  ctx.scale(dpr, dpr);
  ctx.clearRect(0, 0, W, H);
  
  const cx = W / 2;
  const cy = H / 2;
  const radius = Math.min(W, H) / 2 - 30;
  
  const numAxis = 5;
  const angleStep = (Math.PI * 2) / numAxis;
  
  // Draw grid
  ctx.strokeStyle = hexToRgba(getThemeColor('--text') || '#f0f4f8', 0.1);
  ctx.lineWidth = 1;
  for (let r = 1; r <= 4; r++) {
    ctx.beginPath();
    for (let i = 0; i < numAxis; i++) {
      const a = i * angleStep - Math.PI / 2;
      const x = cx + Math.cos(a) * (radius * r / 4);
      const y = cy + Math.sin(a) * (radius * r / 4);
      if (i === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
    }
    ctx.closePath();
    ctx.stroke();
  }
  
  // Draw axes
  ctx.strokeStyle = hexToRgba(getThemeColor('--text') || '#f0f4f8', 0.05);
  for (let i = 0; i < numAxis; i++) {
    const a = i * angleStep - Math.PI / 2;
    ctx.beginPath();
    ctx.moveTo(cx, cy);
    ctx.lineTo(cx + Math.cos(a) * radius, cy + Math.sin(a) * radius);
    ctx.stroke();
  }
  
  // Draw labels
  ctx.fillStyle = getThemeColor('--muted', '#6b7280');
  ctx.font = '10px system-ui';
  ctx.textAlign = 'center';
  ctx.textBaseline = 'middle';
  for (let i = 0; i < numAxis; i++) {
    const a = i * angleStep - Math.PI / 2;
    const x = cx + Math.cos(a) * (radius + 15);
    const y = cy + Math.sin(a) * (radius + 15);
    ctx.fillText(labels[i], x, y);
  }
  
  // Draw Data Polygon
  const accent = getThemeColor('--accent', '#4fc1ff');
  ctx.beginPath();
  for (let i = 0; i < numAxis; i++) {
    const a = i * angleStep - Math.PI / 2;
    const val = Math.max(0, Math.min(1, metrics[i]));
    const x = cx + Math.cos(a) * (radius * val);
    const y = cy + Math.sin(a) * (radius * val);
    if (i === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
  }
  ctx.closePath();
  
  ctx.fillStyle = hexToRgba(accent, 0.2);
  ctx.fill();
  
  ctx.save();
  ctx.shadowColor = accent;
  ctx.shadowBlur = 8;
  ctx.strokeStyle = accent;
  ctx.lineWidth = 2;
  ctx.stroke();
  ctx.restore();
  
  // Tooltip
  canvas.onmousemove = (e) => {
    const rct = canvas.getBoundingClientRect();
    const mx = e.clientX - rct.left;
    const my = e.clientY - rct.top;
    let html = '';
    for (let i = 0; i < numAxis; i++) {
      const a = i * angleStep - Math.PI / 2;
      const val = Math.max(0, Math.min(1, metrics[i]));
      const px = cx + Math.cos(a) * (radius * val);
      const py = cy + Math.sin(a) * (radius * val);
      // If mouse near point
      if (Math.hypot(mx - px, my - py) < 15) {
        html = `<strong>${labels[i]}</strong>: ${fmtSafeNumber(metrics[i], { percent: true, decimals: 1 })}`;
        break;
      }
    }
    if (html) showTooltip(e, html); else hideTooltip();
  };
  canvas.onmouseleave = hideTooltip;
}

/**
 * Renders Regime Timeline combined with Volume/Volatility.
 * @param {string} containerId - Container ID
 * @param {Array<{regime: string, session_id: string, volume: number}>} sessions 
 * @param {Array<number>} volumes - Optional array of normalized volumes [0..1]
 */
export function renderRegimeVolumeChart(containerId, sessions, volumes = []) {
  const container = document.getElementById(containerId);
  if (!container || !sessions || sessions.length === 0) return;
  
  let canvas = container.querySelector('canvas');
  if (!canvas) {
    canvas = document.createElement('canvas');
    canvas.style.width = '100%';
    canvas.style.height = '120px';
    container.innerHTML = '';
    container.appendChild(canvas);
    window.addEventListener('resize', () => renderRegimeVolumeChart(containerId, sessions, volumes));
  }
  
  const ctx = canvas.getContext('2d');
  const dpr = window.devicePixelRatio || 1;
  const rect = container.getBoundingClientRect();
  const W = rect.width, H = 120;
  canvas.width = W * dpr; canvas.height = H * dpr;
  ctx.scale(dpr, dpr);
  
  const pad = { top: 10, right: 10, bottom: 20, left: 10 };
  const chartW = W - pad.left - pad.right, chartH = H - pad.top - pad.bottom;
  
  ctx.clearRect(0, 0, W, H);
  
  const barW = chartW / sessions.length;
  
  sessions.forEach((s, i) => {
    const x = pad.left + i * barW;
    const rc = regimeColor(s.regime);
    const cName = rc.startsWith('var(') ? rc.slice(4, -1) : rc;
    const c = getThemeColor(cName, getThemeColor('--muted', '#6b7280'));
    
    // Volume bar
    let vol = volumes[i] || 0.5; // fallback 0.5
    vol = Math.max(0.1, Math.min(1, vol));
    const barH = chartH * vol;
    const y = pad.top + chartH - barH;
    
    ctx.fillStyle = c;
    ctx.globalAlpha = 0.6;
    ctx.fillRect(x, y, barW - 1, barH);
    
    // Base timeline strip
    ctx.globalAlpha = 1;
    ctx.fillRect(x, pad.top + chartH + 2, barW, 4);
  });
  
  // Tooltip
  canvas.onmousemove = (e) => {
    const rct = canvas.getBoundingClientRect();
    const mx = e.clientX - rct.left;
    if (mx < pad.left || mx > pad.left + chartW) { hideTooltip(); return; }
    
    const i = Math.floor((mx - pad.left) / barW);
    if (i >= 0 && i < sessions.length) {
      const s = sessions[i];
      const vol = volumes[i] || 0.5;
      let html = `<strong>${s.session_id}</strong><br/>`;
      html += `Regime: ${s.regime}<br/>`;
      html += `Intensity: ${fmtSafeNumber(vol, { percent: true, decimals: 0, suffix: '%' })}`;
      showTooltip(e, html);
    } else {
      hideTooltip();
    }
  };
  canvas.onmouseleave = hideTooltip;
}

