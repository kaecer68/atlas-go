// sparkline.js — Darwinian weight sparkline + equity curve + agent scoreboard
// Standalone component, importable into dashboard.js or portfolio pages.
import { fmt, fmtPct, fmtFloat, fmtNTD } from '../shared/utils.js';

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

  const values = points.map(p => p.value);
  const minV = Math.min(...values), maxV = Math.max(...values), range = maxV - minV || 1;
  ctx.clearRect(0, 0, W, H);

  // Background
  ctx.fillStyle = 'rgba(19,22,28,0.6)';
  ctx.beginPath(); ctx.roundRect(pad.left, pad.top, chartW, chartH, 6); ctx.fill();

  // Subtle grid
  ctx.strokeStyle = 'rgba(255,255,255,0.05)'; ctx.lineWidth = 0.5;
  for (let i = 1; i <= 3; i++) {
    const y = pad.top + (chartH / 4) * i;
    ctx.beginPath(); ctx.moveTo(pad.left, y); ctx.lineTo(pad.left + chartW, y); ctx.stroke();
  }

  // Y-axis labels
  ctx.fillStyle = 'rgba(184,196,208,0.6)'; ctx.font = '10px system-ui'; ctx.textAlign = 'right';
  for (let i = 0; i <= 4; i++) {
    const y = pad.top + (chartH / 4) * i;
    ctx.fillText((maxV - (range / 4) * i).toFixed(0), pad.left - 8, y + 3);
  }

  // Compute points
  const pts = points.map((p, i) => ({
    x: pad.left + (i / (points.length - 1)) * chartW,
    y: pad.top + (1 - (p.value - minV) / range) * chartH
  }));

  // Gradient fill under curve
  const gradient = ctx.createLinearGradient(0, pad.top, 0, pad.top + chartH);
  gradient.addColorStop(0, 'rgba(79,193,255,0.25)');
  gradient.addColorStop(1, 'rgba(79,193,255,0.02)');
  ctx.beginPath();
  ctx.moveTo(pts[0].x, pad.top + chartH);
  for (let i = 0; i < pts.length; i++) ctx.lineTo(pts[i].x, pts[i].y);
  ctx.lineTo(pts[pts.length - 1].x, pad.top + chartH);
  ctx.closePath();
  ctx.fillStyle = gradient; ctx.fill();

  // Glow
  ctx.save();
  ctx.shadowColor = 'rgba(79,193,255,0.4)'; ctx.shadowBlur = 6;
  ctx.strokeStyle = getComputedStyle(document.documentElement).getPropertyValue('--accent').trim() || '#4fc1ff';
  ctx.lineWidth = 2.2; ctx.lineJoin = 'round'; ctx.beginPath();
  pts.forEach((p, i) => i === 0 ? ctx.moveTo(p.x, p.y) : ctx.lineTo(p.x, p.y));
  ctx.stroke();
  ctx.restore();

  // Dots on data points
  if (pts.length <= 30) {
    ctx.fillStyle = '#4fc1ff';
    pts.forEach(p => { ctx.beginPath(); ctx.arc(p.x, p.y, 2.5, 0, Math.PI * 2); ctx.fill(); });
  }

  // X-axis labels
  ctx.fillStyle = 'rgba(184,196,208,0.5)'; ctx.font = '9px system-ui'; ctx.textAlign = 'center';
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
  const panel = document.getElementById('equityCurvePanel');
  const canvas = document.getElementById('equityChart');
  if (!panel || !canvas) return;
  if (!preTaxPoints || preTaxPoints.length < 2) { panel.style.display = 'none'; return; }
  panel.style.display = '';
  
  const hasAfterTax = afterTaxPoints && afterTaxPoints.length > 0;
  
  const ctx = canvas.getContext('2d');
  const dpr = window.devicePixelRatio || 1;
  const rect = canvas.parentElement.getBoundingClientRect();
  const W = rect.width - 40, H = 220;
  canvas.width = W * dpr; canvas.height = H * dpr;
  canvas.style.width = W + 'px'; canvas.style.height = H + 'px';
  ctx.scale(dpr, dpr);
  const pad = {top: 20, right: 20, bottom: 28, left: 80}; // Wider left pad for NT$ labels
  const chartW = W - pad.left - pad.right, chartH = H - pad.top - pad.bottom;

  const preTaxValues = preTaxPoints.map(p => p.value);
  const afterTaxValues = hasAfterTax ? afterTaxPoints.map(p => p.value) : [];
  const allValues = [...preTaxValues, ...afterTaxValues];
  
  const minV = Math.min(...allValues), maxV = Math.max(...allValues), range = maxV - minV || 1;
  ctx.clearRect(0, 0, W, H);

  ctx.fillStyle = 'rgba(19,22,28,0.6)';
  ctx.beginPath(); ctx.roundRect(pad.left, pad.top, chartW, chartH, 6); ctx.fill();

  ctx.strokeStyle = 'rgba(255,255,255,0.05)'; ctx.lineWidth = 0.5;
  for (let i = 1; i <= 3; i++) {
    const y = pad.top + (chartH / 4) * i;
    ctx.beginPath(); ctx.moveTo(pad.left, y); ctx.lineTo(pad.left + chartW, y); ctx.stroke();
  }

  ctx.fillStyle = 'rgba(184,196,208,0.6)'; ctx.font = '10px system-ui'; ctx.textAlign = 'right';
  for (let i = 0; i <= 4; i++) {
    const y = pad.top + (chartH / 4) * i;
    const val = maxV - (range / 4) * i;
    ctx.fillText(fmtNTD(val), pad.left - 8, y + 3);
  }

  function drawCurve(points, colorRGB, glowColor) {
    if (!points || points.length === 0) return;
    const pts = points.map((p, i) => ({
      x: pad.left + (i / (points.length - 1)) * chartW,
      y: pad.top + (1 - (p.value - minV) / range) * chartH
    }));

    const gradient = ctx.createLinearGradient(0, pad.top, 0, pad.top + chartH);
    gradient.addColorStop(0, `rgba(${colorRGB},0.25)`);
    gradient.addColorStop(1, `rgba(${colorRGB},0.02)`);
    ctx.beginPath();
    ctx.moveTo(pts[0].x, pad.top + chartH);
    for (let i = 0; i < pts.length; i++) ctx.lineTo(pts[i].x, pts[i].y);
    ctx.lineTo(pts[pts.length - 1].x, pad.top + chartH);
    ctx.closePath();
    ctx.fillStyle = gradient; ctx.fill();

    ctx.save();
    ctx.shadowColor = `rgba(${colorRGB},0.4)`; ctx.shadowBlur = 6;
    ctx.strokeStyle = glowColor;
    ctx.lineWidth = 2.2; ctx.lineJoin = 'round'; ctx.beginPath();
    pts.forEach((p, i) => i === 0 ? ctx.moveTo(p.x, p.y) : ctx.lineTo(p.x, p.y));
    ctx.stroke();
    ctx.restore();

    if (pts.length <= 30) {
      ctx.fillStyle = glowColor;
      pts.forEach(p => { ctx.beginPath(); ctx.arc(p.x, p.y, 2.5, 0, Math.PI * 2); ctx.fill(); });
    }
  }

  if (hasAfterTax) {
    drawCurve(afterTaxPoints, '255,165,0', '#ffa500'); 
  }
  drawCurve(preTaxPoints, '79,193,255', '#4fc1ff'); 

  ctx.fillStyle = 'rgba(184,196,208,0.5)'; ctx.font = '9px system-ui'; ctx.textAlign = 'center';
  const step = Math.max(1, Math.floor(preTaxPoints.length / 6));
  preTaxPoints.forEach((p, i) => {
    if (i % step === 0 || i === preTaxPoints.length - 1) {
      ctx.fillText(p.label, pad.left + (i / (preTaxPoints.length - 1)) * chartW, pad.top + chartH + 18);
    }
  });

  if (hasAfterTax) {
    ctx.font = '10px system-ui';
    ctx.textAlign = 'left';
    
    ctx.fillStyle = '#4fc1ff';
    ctx.fillRect(pad.left + 10, pad.top + 10, 10, 10);
    ctx.fillStyle = 'rgba(184,196,208,0.8)';
    ctx.fillText('稅前淨值 (Pre-tax)', pad.left + 25, pad.top + 19);
    
    ctx.fillStyle = '#ffa500';
    ctx.fillRect(pad.left + 130, pad.top + 10, 10, 10);
    ctx.fillStyle = 'rgba(184,196,208,0.8)';
    ctx.fillText('稅後淨值 (After-tax)', pad.left + 145, pad.top + 19);
  }
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
    const sharpe = a.rolling_sharpe || 0;
    const sharpeColor = sharpe > 1 ? 'var(--up)' : (sharpe < 0 ? 'var(--down)' : 'var(--warn)');
    html += '<tr>';
    html += `<td>${nameFn(a.agent_id)}</td>`;
    html += `<td>${fmtFloat(a.weight)}</td>`;
    html += `<td style="color:${sharpeColor}">${sharpe.toFixed(2)}</td>`;
    html += `<td>${fmtPct(a.hit_rate)}</td>`;
    html += `<td>${a.signal_count || 0}</td>`;
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
  const colors = { RISK_ON: 'var(--up)', RISK_OFF: 'var(--down)', NEUTRAL: 'var(--warn)' };
  const color = colors[regime] || 'var(--muted)';
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
