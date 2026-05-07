// sparkline.js — Darwinian weight sparkline + equity curve + agent scoreboard
// Standalone component, importable into dashboard.js or portfolio pages.
import { fmt, fmtPct, fmtFloat } from '../shared/utils.js';

export { renderEquityCurve, renderAgentScoreboard, renderRegimeContext, renderAllocationGuidance };

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
  canvas.width = (rect.width - 40) * dpr; canvas.height = 200 * dpr;
  canvas.style.width = (rect.width - 40) + 'px'; canvas.style.height = '200px';
  ctx.scale(dpr, dpr);
  const w = rect.width - 40, h = 200;
  const pad = {top: 20, right: 20, bottom: 30, left: 60};
  const chartW = w - pad.left - pad.right, chartH = h - pad.top - pad.bottom;
  const values = points.map(p => p.value);
  const minV = Math.min(...values), maxV = Math.max(...values), range = maxV - minV || 1;
  ctx.clearRect(0, 0, w, h);
  // Grid lines
  ctx.strokeStyle = '#333'; ctx.lineWidth = 0.5;
  for (let i = 0; i <= 4; i++) {
    const y = pad.top + (chartH / 4) * i;
    ctx.beginPath(); ctx.moveTo(pad.left, y); ctx.lineTo(w - pad.right, y); ctx.stroke();
    ctx.fillStyle = '#888'; ctx.font = '10px sans-serif'; ctx.textAlign = 'right';
    ctx.fillText((maxV - (range / 4) * i).toFixed(0), pad.left - 5, y + 3);
  }
  // Trend line
  ctx.strokeStyle = getComputedStyle(document.documentElement).getPropertyValue('--accent').trim() || '#3b82f6';
  ctx.lineWidth = 2; ctx.beginPath();
  points.forEach((p, i) => {
    const x = pad.left + (i / (points.length - 1)) * chartW;
    const y = pad.top + (1 - (p.value - minV) / range) * chartH;
    i === 0 ? ctx.moveTo(x, y) : ctx.lineTo(x, y);
  });
  ctx.stroke();
  // X-axis labels
  ctx.fillStyle = '#888'; ctx.font = '9px sans-serif'; ctx.textAlign = 'center';
  const step = Math.max(1, Math.floor(points.length / 6));
  points.forEach((p, i) => {
    if (i % step === 0 || i === points.length - 1) ctx.fillText(p.label, pad.left + (i / (points.length - 1)) * chartW, h - 5);
  });
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
