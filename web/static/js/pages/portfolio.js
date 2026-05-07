// Portfolio page module — extracted from index.html
// Renders the strategy cockpit: KPIs, regime, allocation, agents, positions, equity curve
import { fmt, fmtPct, fmtFloat, fmtInt, pnlColor, pnlSign, convColor } from '../shared/utils.js';
import { FIELD } from '../shared/field_names.js';

// Re-export render functions for direct page loading
export { renderPortfolioKPIs, renderEquityCurve, renderPositionsTable, renderSuggestedPositions,
         renderRegimeContext, renderAllocationGuidance, renderAgentScoreboard };

// Main entry point for portfolio page — fetches all data and renders
export async function loadPortfolioPage(getJSON, agentNameFn) {
  const agentName = agentNameFn || (id => id);
  try {
    const [ps, dw, regimeData, pipeline] = await Promise.all([
      getJSON('/api/dashboard/portfolio-state').catch(() => null),
      getJSON('/api/synergy/darwinian-status').catch(() => null),
      getJSON('/api/dashboard/regime-history').catch(() => null),
      getJSON('/api/dashboard/recommendation-pipeline').catch(() => null),
    ]);
    renderPortfolioKPIs(ps, pipeline);
    renderRegimeContext(ps, regimeData);
    renderAllocationGuidance(ps, regimeData);
    renderAgentScoreboard(dw, agentName);
    renderEquityCurve((ps && ps.equity_curve) || []);
    renderPositionsTable((ps && ps.positions) || [], pipeline, agentName);
  } catch (e) { console.error('Failed to load portfolio:', e); }
}

function renderPortfolioKPIs(data, pipeline) {
  const el = document.getElementById('portfolioKPIs');
  if (!el) return;
  if (!data) { el.innerHTML = '<div class="empty">尚無組合資料</div>'; return; }
  const dd = data.current_drawdown;
  const ddPct = dd > 0 ? (dd * 100).toFixed(1) + '%' : '0%';
  const ddColor = dd > 0.2 ? 'var(--down)' : (dd > 0.1 ? 'var(--warn)' : 'var(--up)');
  const positionsCount = data.positions_count || 0;
  let positionLabel = positionsCount + ' 檔';
  if (positionsCount === 0 && pipeline && pipeline.items) {
    const passed = pipeline.items.filter(it => it.passed_guards !== false).length;
    if (passed > 0) positionLabel = `0 檔 <span style="font-size:10px;color:var(--muted)">（${passed} 筆建議）</span>`;
  }
  el.innerHTML = `
    <div class="panel" style="text-align:center"><div class="kpi-label">總資產</div><div class="kpi-value">${fmt(data.portfolio_value)}</div></div>
    <div class="panel" style="text-align:center"><div class="kpi-label">可用現金</div><div class="kpi-value">${fmt(data.cash)}</div></div>
    <div class="panel" style="text-align:center"><div class="kpi-label">目前回撤</div><div class="kpi-value" style="color:${ddColor}">${ddPct}</div></div>
    <div class="panel" style="text-align:center"><div class="kpi-label">累計損益</div><div class="kpi-value" style="color:${pnlColor(data.cumulative_pnl_pct)}">${fmtPct(data.cumulative_pnl_pct)}</div></div>
    <div class="panel" style="text-align:center"><div class="kpi-label">持倉數</div><div class="kpi-value">${positionLabel}</div></div>
  `;
}

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
  ctx.strokeStyle = '#333'; ctx.lineWidth = 0.5;
  for (let i = 0; i <= 4; i++) {
    const y = pad.top + (chartH / 4) * i;
    ctx.beginPath(); ctx.moveTo(pad.left, y); ctx.lineTo(w - pad.right, y); ctx.stroke();
    ctx.fillStyle = '#888'; ctx.font = '10px sans-serif'; ctx.textAlign = 'right';
    ctx.fillText((maxV - (range / 4) * i).toFixed(0), pad.left - 5, y + 3);
  }
  ctx.strokeStyle = getComputedStyle(document.documentElement).getPropertyValue('--accent').trim() || '#3b82f6';
  ctx.lineWidth = 2; ctx.beginPath();
  points.forEach((p, i) => {
    const x = pad.left + (i / (points.length - 1)) * chartW;
    const y = pad.top + (1 - (p.value - minV) / range) * chartH;
    i === 0 ? ctx.moveTo(x, y) : ctx.lineTo(x, y);
  });
  ctx.stroke();
  ctx.fillStyle = '#888'; ctx.font = '9px sans-serif'; ctx.textAlign = 'center';
  const step = Math.max(1, Math.floor(points.length / 6));
  points.forEach((p, i) => {
    if (i % step === 0 || i === points.length - 1) ctx.fillText(p.label, pad.left + (i / (points.length - 1)) * chartW, h - 5);
  });
}

function renderPositionsTable(positions, pipeline, agentName) {
  const el = document.getElementById('positionsTable');
  if (!el) return;
  if (!positions || positions.length === 0) {
    const items = (pipeline && pipeline.items) || [];
    const passedItems = items.filter(it => it.passed_guards !== false);
    if (passedItems.length > 0) { el.innerHTML = renderSuggestedPositions(passedItems, pipeline, agentName); return; }
    el.innerHTML = '<div style="padding:20px;text-align:center;color:var(--muted)">目前無持倉，且尚無 pipeline 建議資料</div>';
    return;
  }
  el.innerHTML = `<div class="table-wrapper"><table><thead><tr><th>符號</th><th>數量</th><th>成本</th><th>現價</th><th>市值</th><th>損益</th><th>損益%</th><th>推薦 AI</th><th>信心</th></tr></thead>
    <tbody>${positions.map(p => `<tr>
      <td><strong>${p.symbol}</strong></td><td>${fmtInt(p.quantity)}</td><td>${fmtFloat(p.average_cost)}</td><td>${fmtFloat(p.current_price)}</td><td>${fmtFloat(p.market_value)}</td>
      <td style="color:${pnlColor(p.unrealized_pnl)};font-weight:700">${pnlSign(p.unrealized_pnl)}${fmtFloat(p.unrealized_pnl)}</td>
      <td style="color:${pnlColor(p.pnl_pct)};font-weight:700">${pnlSign(p.pnl_pct)}${(p.pnl_pct * 100).toFixed(2)}%</td>
      <td style="font-size:11px">${(agentName(p.agent_id)) || p.agent_name || '-'}</td>
      <td style="color:${convColor(p.conviction)};font-size:11px">${p.conviction ? (p.conviction * 100).toFixed(0) + '%' : '-'}</td>
    </tr>`).join('')}</tbody></table></div>`;
}

function renderSuggestedPositions(items, pipeline, agentName) {
  const sessionID = (pipeline && pipeline.session_id) || '-';
  const regime = (pipeline && pipeline.regime) || '-';
  return `<div style="margin-bottom:8px;font-size:11px;color:var(--warn)">⚠️ 目前無 live 持倉資料，以下為最新模擬場次（${sessionID}｜${regime}）的 <strong>AI 建議持倉</strong></div>
    <div class="table-wrapper"><table><thead><tr><th>標的</th><th>方向</th><th>推薦 AI</th><th>信心</th><th>目標價</th><th>停損價</th><th>理由</th></tr></thead>
    <tbody>${items.map(i => {
      const sideLabel = i.side === 'BUY' ? '買入' : (i.side === 'SELL' ? '賣出' : i.side);
      const sideColor = i.side === 'BUY' ? 'var(--up)' : 'var(--down)';
      const conv = i.conviction || 0;
      return `<tr><td><strong>${i.symbol || '-'}</strong></td>
        <td style="color:${sideColor};font-weight:600">${sideLabel}</td>
        <td style="font-size:11px">${agentName(i.agent_id) || i.skill || '-'}</td>
        <td style="color:${convColor(conv)};font-weight:600">${(conv * 100).toFixed(0)}%</td>
        <td>${i.target_price ? i.target_price.toFixed(1) : '-'}</td>
        <td>${i.stop_loss_price ? i.stop_loss_price.toFixed(1) : '-'}</td>
        <td style="font-size:11px;max-width:200px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap" title="${(i.reason || '-').replace(/"/g,'&quot;')}">${(i.reason || '-').substring(0, 50)}</td></tr>`;
    }).join('')}</tbody></table></div>
    <div style="font-size:10px;color:var(--muted);margin-top:4px">共 ${items.length} 筆通過控制層審核的推薦（來自 ${pipeline.session_id || '?'}）</div>`;
}

function renderRegimeContext(ps, regimeData) {
  const el = document.getElementById('regimeContext');
  if (!el) return;
  const regime = (ps && ps.regime) || '-';
  const regimeLabel = (ps && ps.regime_label) || regime;
  const transitions = (regimeData && regimeData.transitions) || [];
  const sessions = (regimeData && regimeData.sessions) || [];
  const counts = {};
  sessions.forEach(s => { const r = s.regime || '-'; counts[r] = (counts[r] || 0) + 1; });
  let advice = regime === 'RISK_ON' ? '市場處於多頭格局，適合提高曝險、偏重動能與成長型策略。'
    : (regime === 'RISK_OFF' ? '市場處於空頭格局，應降低曝險、增持現金、偏好防禦型策略。'
    : (regime === 'NEUTRAL' ? '市場處於盤整格局，方向不明朗，建議適度曝險、分散配置。'
    : '體制資料尚未載入，系統將以中性配置運行。'));
  const regimeColor = regime === 'RISK_ON' ? 'var(--up)' : (regime === 'RISK_OFF' ? 'var(--down)' : 'var(--warn)');
  let transitionHtml = '';
  if (transitions.length > 0) {
    const last = transitions[transitions.length - 1];
    transitionHtml = `<div style="font-size:10px;color:var(--muted);margin-top:6px">最近轉變：${last.from || '-'} → ${last.to || '-'}（${last.session_id || ''}）</div>`;
  }
  el.innerHTML = `<div style="text-align:center"><div class="kpi-value" style="color:${regimeColor};font-size:22px">${regimeLabel}</div><div class="kpi-label" style="margin-top:2px">${regime}</div></div>
    <div style="font-size:12px;color:var(--muted);margin-top:8px;line-height:1.6">${advice}</div>${transitionHtml}
    <div style="margin-top:8px;display:flex;gap:8px;font-size:11px;color:var(--muted)"><span>🟢 ${counts['RISK_ON'] || 0} 天</span><span>🟡 ${counts['NEUTRAL'] || 0} 天</span><span>🔴 ${counts['RISK_OFF'] || 0} 天</span></div>`;
}

function renderAllocationGuidance(ps) {
  const el = document.getElementById('allocationGuidance');
  if (!el) return;
  const cash = (ps && ps.cash) || 0;
  const portfolioValue = (ps && ps.portfolio_value) || cash;
  const regime = (ps && ps.regime) || 'NEUTRAL';
  let tier, suggestedCashPct, maxPositionPct, diversificationNote;
  if (portfolioValue < 500000) {
    tier = '小型資金（<50 萬）'; suggestedCashPct = regime === 'RISK_ON' ? 0.05 : (regime === 'RISK_OFF' ? 0.30 : 0.15);
    maxPositionPct = 0.40; diversificationNote = '建議 3～5 檔集中配置，追求資本成長';
  } else if (portfolioValue < 3000000) {
    tier = '中型資金（50～300 萬）'; suggestedCashPct = regime === 'RISK_ON' ? 0.10 : (regime === 'RISK_OFF' ? 0.35 : 0.20);
    maxPositionPct = 0.20; diversificationNote = '建議 6～10 檔適度分散，平衡成長與風控';
  } else {
    tier = '大型資金（>300 萬）'; suggestedCashPct = regime === 'RISK_ON' ? 0.20 : (regime === 'RISK_OFF' ? 0.45 : 0.30);
    maxPositionPct = 0.10; diversificationNote = '建議 10～20 檔全市場配置，資本保值優先';
  }
  const currentCashPct = portfolioValue > 0 ? cash / portfolioValue : 1;
  const cashStatus = currentCashPct >= suggestedCashPct ? '✅' : '⚠️';
  const exposurePct = 1 - currentCashPct, suggestedExposurePct = 1 - suggestedCashPct;
  el.innerHTML = `<div style="font-size:11px;color:var(--muted);margin-bottom:4px">${tier}</div>
    <div style="display:flex;justify-content:space-between;margin-bottom:6px">
      <div><span style="font-size:18px;font-weight:700">${(exposurePct * 100).toFixed(0)}%</span><br><span style="font-size:10px;color:var(--muted)">目前曝險</span></div>
      <div><span style="font-size:18px;color:var(--muted)">→</span></div>
      <div><span style="font-size:18px;font-weight:700;color:var(--accent)">${(suggestedExposurePct * 100).toFixed(0)}%</span><br><span style="font-size:10px;color:var(--muted)">建議曝險</span></div></div>
    <div style="font-size:11px;color:var(--muted);line-height:1.5">
      <div>${cashStatus} 現金比例：${(currentCashPct * 100).toFixed(0)}%（建議 ≥ ${(suggestedCashPct * 100).toFixed(0)}%）</div>
      <div>📌 單一持倉上限：${(maxPositionPct * 100).toFixed(0)}%</div><div style="margin-top:2px">${diversificationNote}</div></div>`;
}

function renderAgentScoreboard(dw, agentName) {
  const el = document.getElementById('agentScoreboard');
  if (!el) return;
  if (!dw || !dw.agents || dw.status !== 'ok') {
    el.innerHTML = '<div style="padding:12px;text-align:center;color:var(--muted)">Darwinian 權重資料尚未載入</div>';
    return;
  }
  const agents = Object.values(dw.agents).sort((a, b) => (b.rolling_sharpe || 0) - (a.rolling_sharpe || 0));
  el.innerHTML = `<div style="display:flex;flex-wrap:wrap;gap:6px">${agents.map(a => {
    const w = a.weight || 1, sharpe = a.rolling_sharpe || 0, hitRate = (a.hit_rate || 0) * 100;
    const barW = Math.max(1, Math.round(w / 2.5 * 100));
    const color = w >= 2.0 ? 'var(--up)' : (w >= 1.0 ? 'var(--accent)' : (w >= 0.5 ? 'var(--warn)' : 'var(--down)'));
    return `<div style="flex:1;min-width:180px;max-width:240px;background:var(--bg);border:1px solid var(--border);border-radius:6px;padding:8px">
      <div style="display:flex;align-items:center;gap:6px;margin-bottom:4px"><div style="width:${barW}%;max-width:100%;height:3px;background:${color};border-radius:2px;flex-shrink:0" title="權重 ${w.toFixed(2)}"></div>
      <span style="font-size:10px;font-weight:700;white-space:nowrap">${agentName(a.agent_id)}</span></div>
      <div style="font-size:10px;color:var(--muted);display:flex;gap:8px"><span>S:${sharpe.toFixed(1)}</span><span>Hit:${hitRate.toFixed(0)}%</span><span>N:${a.total_signals || 0}</span><span style="font-weight:700;color:${color}">×${w.toFixed(2)}</span></div></div>`;
  }).join('')}</div>
  <div style="font-size:10px;color:var(--muted);margin-top:6px">Darwinian 權重範圍：0.3～2.5× | 基於近 20 日滾動績效自動調整 | 更新時間：${dw.last_computed || '-'}</div>`;
}
