// Risk Control Page - Enhanced Risk Indicators
// Extracted from index.html - DO NOT EDIT inline
import { sectorName } from '../names.js';

export function renderLiveStatus(data) {
  const el = document.getElementById('liveStatus');
  if (!data || !data.circuit_breaker) {
    el.innerHTML = '<div class="empty">即時狀態暫無資料</div>';
    return;
  }
  const cb = data.circuit_breaker;
  const pnl = (data.portfolio || {}).unrealized_pnl || 0;
  const pnlClass = pnl >= 0 ? 'up' : 'down';
  const cbState = cb.state === 'tripped' ? '已觸發' : '正常';
  el.innerHTML = `
    <div class="metric"><div class="label">熔斷機制</div><div class="value ${cb.state === 'tripped' ? 'err' : 'ok'}">${cbState}</div></div>
    <div class="metric"><div class="label">現金</div><div class="value">${((data.portfolio || {}).cash || 0).toLocaleString()}</div></div>
    <div class="metric"><div class="label">今日損益</div><div class="value ${pnlClass}">${pnl.toLocaleString()}</div></div>
    <div class="metric"><div class="label">持倉數</div><div class="value">${(data.portfolio || {}).positions_count || 0}</div></div>
  `;
}

export function renderRiskCards(data, pipelineData) {
  const el = document.getElementById('riskCards');
  const panel = document.getElementById('riskCardsPanel');
  const posEl = document.getElementById('riskPositionConcentration');
  const sectorEl = document.getElementById('riskSectorDistribution');
  if (!el || !data || !data.risk_snapshot) {
    if (panel) panel.style.display = 'none';
    return;
  }
  panel.style.display = '';
  const rs = data.risk_snapshot;
  const insufficient = rs.insufficient_data || rs.data_points < 30;
  const fmtPct = v => (typeof v === 'number' && !isNaN(v)) ? (v * 100).toFixed(1) + '%' : '-';

  // --- Calculate portfolio risk indicators (from pipelineData) ---
  const items = (pipelineData && pipelineData.items) || [];
  const passedItems = items.filter(it => it.passed_guards);

  // 1. Portfolio Sharpe (simple: mean return / std dev)
  let portfolioSharpe = null;
  if (passedItems.length >= 3) {
    const returns = passedItems.map(it => it.forward_return || 0);
    const avg = returns.reduce((a, b) => a + b, 0) / returns.length;
    const variance = returns.reduce((sum, r) => sum + Math.pow(r - avg, 2), 0) / returns.length;
    const std = Math.sqrt(variance);
    portfolioSharpe = std > 0 ? (avg / std) : 0;
  }

  // 2. Position concentration (top 5 holdings)
  let concentrationHtml = '';
  if (passedItems.length > 0) {
    const totalConviction = passedItems.reduce((sum, it) => sum + (it.conviction || 0), 0);
    const sorted = passedItems.slice().sort((a, b) => (b.conviction || 0) - (a.conviction || 0));
    const top5 = sorted.slice(0, 5);
    const top5Weight = totalConviction > 0
      ? top5.reduce((sum, it) => sum + (it.conviction || 0), 0) / totalConviction
      : 0;
    const top1Weight = totalConviction > 0 ? (sorted[0].conviction || 0) / totalConviction : 0;

    const rows = top5.map((it, idx) => {
      const weight = totalConviction > 0 ? ((it.conviction || 0) / totalConviction * 100).toFixed(1) : '0.0';
      return `<tr><td style="padding:3px 8px;font-size:12px">${idx + 1}</td><td style="padding:3px 8px;font-size:12px">${it.symbol}</td><td style="padding:3px 8px;font-size:12px;text-align:right">${weight}%</td></tr>`;
    }).join('');

    concentrationHtml = `
      <div style="display:flex;gap:16px;flex-wrap:wrap;margin-top:12px">
        <div style="flex:1;min-width:200px">
          <div style="font-size:12px;color:var(--muted);margin-bottom:6px">前 5 大持倉集中度</div>
          <div style="font-size:20px;font-weight:700;color:${top5Weight > 0.6 ? 'var(--down)' : (top5Weight > 0.4 ? 'var(--warn)' : 'var(--up)')}">${(top5Weight * 100).toFixed(1)}%</div>
          <div style="font-size:11px;color:var(--muted)">最高單一持倉 ${(top1Weight * 100).toFixed(1)}%</div>
        </div>
        <div style="flex:2;min-width:280px">
          <table style="width:100%;font-size:12px;border-collapse:collapse">
            <thead><tr style="border-bottom:1px solid var(--border)"><th style="text-align:left;padding:4px 8px">#</th><th style="text-align:left;padding:4px 8px">標的</th><th style="text-align:right;padding:4px 8px">權重</th></tr></thead>
            <tbody>${rows}</tbody>
          </table>
        </div>
      </div>
    `;
  } else {
    concentrationHtml = `<div style="font-size:12px;color:var(--muted);margin-top:12px">暫無持倉資料（本場次無控制層放行標的）</div>`;
  }

  // 3. Sector distribution (inferred from agent_id / layer)
  let sectorHtml = '';
  if (passedItems.length > 0) {
    const sectorMap = {};
    passedItems.forEach(it => {
      const sector = inferSectorFromAgent(it.agent_id, it.layer) || '其他';
      if (!sectorMap[sector]) sectorMap[sector] = { count: 0, conviction: 0 };
      sectorMap[sector].count++;
      sectorMap[sector].conviction += (it.conviction || 0);
    });

    const totalConv = passedItems.reduce((sum, it) => sum + (it.conviction || 0), 0);
    const sectors = Object.entries(sectorMap)
      .map(([name, data]) => ({ name, ...data, weight: totalConv > 0 ? data.conviction / totalConv : 0 }))
      .sort((a, b) => b.weight - a.weight);

    const sectorBars = sectors.map(s => {
      const pct = (s.weight * 100).toFixed(1);
      const color = s.weight > 0.3 ? 'var(--accent)' : (s.weight > 0.15 ? 'var(--accent-secondary)' : 'var(--muted)');
      return `
        <div style="margin:4px 0">
          <div style="display:flex;justify-content:space-between;font-size:12px;margin-bottom:2px">
            <span>${sectorName(s.name) || s.name}</span>
            <span>${pct}% (${s.count} 檔)</span>
          </div>
          <div style="width:100%;height:6px;background:var(--bg);border-radius:3px;overflow:hidden">
            <div style="width:${pct}%;height:100%;background:${color};border-radius:3px;transition:width 0.3s"></div>
          </div>
        </div>
      `;
    }).join('');

    sectorHtml = `
      <div style="margin-top:16px">
        <div style="font-size:13px;font-weight:700;margin-bottom:8px">板塊配置分布</div>
        ${sectorBars}
      </div>
    `;
  } else {
    sectorHtml = `<div style="font-size:12px;color:var(--muted);margin-top:16px">暫無板塊分布資料</div>`;
  }

  // Render main risk indicator cards
  el.innerHTML = `
    <div class="panel" style="text-align:center"><div class="kpi-label">VaR 95%</div><div class="kpi-value" style="color:var(--down)">${insufficient ? '資料不足' : fmtPct(rs.var_95)}</div><div class="kpi-hint">95% 信賴區間最大虧損</div></div>
    <div class="panel" style="text-align:center"><div class="kpi-label">CVaR 95%</div><div class="kpi-value" style="color:var(--down)">${insufficient ? '資料不足' : fmtPct(rs.cvar_95)}</div><div class="kpi-hint">95% 條件期望虧損</div></div>
    <div class="panel" style="text-align:center"><div class="kpi-label">最大回撤</div><div class="kpi-value" style="color:var(--warn)">${insufficient ? '資料不足' : fmtPct(rs.max_drawdown_pct)}</div><div class="kpi-hint">歷史最大回撤幅度</div></div>
    <div class="panel" style="text-align:center"><div class="kpi-label">投組 Sharpe</div><div class="kpi-value" style="color:${portfolioSharpe !== null ? (portfolioSharpe > 1 ? 'var(--up)' : (portfolioSharpe < 0 ? 'var(--down)' : 'var(--warn)')) : 'var(--muted)'}">${portfolioSharpe !== null ? portfolioSharpe.toFixed(2) : 'N/A'}</div><div class="kpi-hint">風險調整後收益比</div></div>
    <div class="panel" style="text-align:center"><div class="kpi-label">放行標的數</div><div class="kpi-value">${passedItems.length}</div><div class="kpi-hint">控制層放行進模擬投組</div></div>
    <div class="panel" style="text-align:center"><div class="kpi-label">資料點數</div><div class="kpi-value" style="color:${rs.data_points < 30 ? 'var(--warn)' : 'var(--up)'}">${rs.data_points || 0}</div><div class="kpi-hint">統計可信度 ${rs.data_points >= 30 ? '充足' : '不足'}</div></div>
  `;

  // Render position concentration
  if (posEl) posEl.innerHTML = `
    <div style="border-top:1px solid var(--border);padding-top:12px">
      <div style="font-size:13px;font-weight:700;margin-bottom:6px">倉位集中度分析</div>
      ${concentrationHtml}
    </div>
  `;

  // Render sector distribution
  if (sectorEl) sectorEl.innerHTML = sectorHtml;
}

// Simple sector inference from agent ID and layer
export function inferSectorFromAgent(agentID, layer) {
  const agentSectorMap = {
    'semiconductor': 'semiconductor',
    'ai_supply_chain': 'ai_supply_chain',
    'financials': 'financials',
    'shipping': 'shipping',
    'value_yield': 'high_dividend',
    'etf_rotation': 'etf_rotation',
    'technical_breakout': 'small_cap',
    'growth_momentum': 'small_cap',
    'macro': 'TAIEX',
    'cro': 'control',
    'cio': 'control'
  };
  return agentSectorMap[agentID] || (layer === 'sector' ? agentID : null);
}
