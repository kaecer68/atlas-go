// Risk Control Page - Enhanced Risk Indicators
// Extracted from index.html - DO NOT EDIT inline
import { sectorName } from '../names.js';

export function renderLiveStatus(data) {
  const el = document.getElementById('liveStatus');
  el.classList.remove('loading');
  if (!data || !data.circuit_breaker) {
    el.innerHTML = '<div class="empty">即時狀態暫無資料</div>';
    return;
  }
  const cb = data.circuit_breaker;
  const pf = data.portfolio || {};
  const pnl = pf.unrealized_pnl || 0;
  const dayPnl = pf.day_pnl || 0;
  const pnlClass = pnl >= 0 ? 'up' : 'down';
  const dayPnlClass = dayPnl >= 0 ? 'up' : 'down';
  const cbState = cb.state === 'tripped' ? '已觸發' : '正常';
  const cbStateClass = cb.state === 'tripped' ? 'err' : 'ok';

  let cooldownInfo = '';
  if (cb.cooldown_until && cb.cooldown_until !== '0001-01-01T00:00:00Z') {
    const cd = new Date(cb.cooldown_until);
    const now = new Date();
    if (cd > now) {
      const mins = Math.ceil((cd - now) / 60000);
      cooldownInfo = `<div class="metric"><div class="label">冷卻中</div><div class="value warn">${mins} 分鐘</div></div>`;
    }
  }

  let slWarning = '';
  if (cb.consecutive_sl > 0) {
    slWarning = `<div class="metric"><div class="label">連續止損</div><div class="value ${cb.consecutive_sl >= 3 ? 'err' : 'warn'}">${cb.consecutive_sl} 次</div></div>`;
  }

  el.innerHTML = `
    <div class="metric"><div class="label">熔斷機制</div><div class="value ${cbStateClass}">${cbState}</div></div>
    <div class="metric"><div class="label">現金</div><div class="value">${(pf.cash || 0).toLocaleString()}</div></div>
    <div class="metric"><div class="label">持倉市值</div><div class="value">${(pf.total_exposure || 0).toLocaleString()}</div></div>
    <div class="metric"><div class="label">持倉數</div><div class="value">${pf.positions_count || 0}</div></div>
    <div class="metric"><div class="label">未實現損益</div><div class="value ${pnlClass}">${pnl.toLocaleString()}</div></div>
    <div class="metric"><div class="label">當日損益</div><div class="value ${dayPnlClass}">${dayPnl.toLocaleString()}</div></div>
    ${slWarning}
    ${cooldownInfo}
  `;
}

export function renderRiskCards(riskExposure, pipelineData, capitalPhase) {
  const el = document.getElementById('riskCards');
  const panel = document.getElementById('riskCardsPanel');
  const posEl = document.getElementById('riskPositionConcentration');
  const sectorEl = document.getElementById('riskSectorDistribution');

  if (!el || !riskExposure) {
    if (panel) panel.style.display = 'none';
    return;
  }

  panel.style.display = '';
  panel.classList.remove('loading');
  el.classList.remove('loading');

  const re = riskExposure;
  const insufficient = re.insufficient_data || (re.data_points < 30);
  const fmtPct = v => (typeof v === 'number' && !isNaN(v)) ? (v * 100).toFixed(1) + '%' : '—';

  const cp = capitalPhase || {};
  const phaseLabel = { advance: '🚀 推進', reduce: '🔻 縮減', standby: '⏸️ 觀望' };
  const phase = phaseLabel[cp.phase] || cp.phase || '—';
  const rollingSharpe = (cp.rolling_sharpe != null && !isNaN(cp.rolling_sharpe)) ? cp.rolling_sharpe.toFixed(2) : null;
  const consecLosses = cp.consecutive_losses || 0;
  const daysInPhase = cp.days_in_phase || 0;
  const canAdvance = cp.can_advance;

  let concentrationHtml = '';
  const conc = re.concentration || [];
  if (conc.length > 0) {
    const top5Weight = conc.reduce((s, c) => s + (c.weight || 0), 0);
    const top1Weight = conc.length > 0 ? (conc[0].weight || 0) : 0;
    const top3Weight = conc.slice(0, 3).reduce((s, c) => s + (c.weight || 0), 0);

    const rows = conc.map((c, idx) => {
      const w = ((c.weight || 0) * 100).toFixed(1);
      return `<tr><td style="padding:3px 8px;font-size:12px">${idx + 1}</td><td style="padding:3px 8px;font-size:12px">${c.symbol}</td><td style="padding:3px 8px;font-size:12px;text-align:right">${w}%</td><td style="padding:3px 8px;font-size:12px;text-align:right">${(c.market_value || 0).toLocaleString()}</td></tr>`;
    }).join('');

    concentrationHtml = `
      <div style="display:flex;gap:16px;flex-wrap:wrap;margin-top:12px">
        <div style="flex:1;min-width:180px">
          <div style="font-size:12px;color:var(--muted);margin-bottom:6px">持倉集中度（市值）</div>
          <div style="font-size:20px;font-weight:700;color:${top5Weight > 0.6 ? 'var(--down)' : (top5Weight > 0.4 ? 'var(--warn)' : 'var(--up)')}">${(top5Weight * 100).toFixed(1)}%</div>
          <div style="font-size:11px;color:var(--muted);margin-top:4px">前 3 大 ${(top3Weight * 100).toFixed(1)}% · 最大 ${(top1Weight * 100).toFixed(1)}%</div>
        </div>
        <div style="flex:2;min-width:300px">
          <table style="width:100%;font-size:12px;border-collapse:collapse">
            <thead><tr style="border-bottom:1px solid var(--border)"><th style="text-align:left;padding:4px 8px">#</th><th style="text-align:left;padding:4px 8px">標的</th><th style="text-align:right;padding:4px 8px">權重</th><th style="text-align:right;padding:4px 8px">市值</th></tr></thead>
            <tbody>${rows}</tbody>
          </table>
        </div>
      </div>
    `;
  } else {
    concentrationHtml = `<div style="font-size:12px;color:var(--muted);margin-top:12px">暫無持倉資料</div>`;
  }

  let sectorHtml = '';
  const sectors = (re.sector_exposure || [])
    .filter(s => s.weight > 0)
    .sort((a, b) => (b.weight || 0) - (a.weight || 0));

  if (sectors.length > 0) {
    const maxW = Math.max(...sectors.map(s => s.weight || 0), 0.01);
    const sectorBars = sectors.map(s => {
      const w = s.weight || 0;
      const pct = (w * 100).toFixed(1);
      const barPct = ((w / maxW) * 100).toFixed(1);
      const color = w > 0.3 ? 'var(--accent)' : (w > 0.15 ? 'var(--warn)' : 'var(--muted)');
      return `
        <div style="margin:4px 0">
          <div style="display:flex;justify-content:space-between;font-size:12px;margin-bottom:2px">
            <span>${sectorName(s.sector) || s.sector}</span>
            <span>${pct}%</span>
          </div>
          <div style="width:100%;height:6px;background:var(--bg);border-radius:3px;overflow:hidden">
            <div style="width:${barPct}%;height:100%;background:${color};border-radius:3px;transition:width 0.3s"></div>
          </div>
        </div>
      `;
    }).join('');

    sectorHtml = `
      <div style="margin-top:16px">
        <div style="font-size:13px;font-weight:700;margin-bottom:8px">板塊曝險分布（市值權重）</div>
        ${sectorBars}
      </div>
    `;
  } else {
    sectorHtml = `<div style="font-size:12px;color:var(--muted);margin-top:16px">暫無板塊曝險資料</div>`;
  }

  const cashRatioPct = re.cash_ratio != null ? (re.cash_ratio * 100).toFixed(1) : null;
  const portfolioValue = re.portfolio_value ? re.portfolio_value.toLocaleString() : '—';
  const deployedCapital = cp.deployed_capital || 0;
  const totalCapital = cp.total_capital || 0;
  const exposureRatio = totalCapital > 0 ? ((deployedCapital / totalCapital) * 100).toFixed(1) : null;

  el.innerHTML = `
    <div class="panel" style="text-align:center">
      <div class="kpi-label">VaR 95%</div>
      <div class="kpi-value" style="color:var(--down)">${insufficient ? '資料不足' : fmtPct(re.var_95)}</div>
      <div class="kpi-hint">日頻 · 95% 信賴水準</div>
    </div>
    <div class="panel" style="text-align:center">
      <div class="kpi-label">VaR 99%</div>
      <div class="kpi-value" style="color:var(--down)">${insufficient ? '資料不足' : fmtPct(re.var_99)}</div>
      <div class="kpi-hint">日頻 · 極端事件壓力</div>
    </div>
    <div class="panel" style="text-align:center">
      <div class="kpi-label">CVaR 95%</div>
      <div class="kpi-value" style="color:var(--down)">${insufficient ? '資料不足' : fmtPct(re.cvar_95)}</div>
      <div class="kpi-hint">95% 條件期望虧損</div>
    </div>
    <div class="panel" style="text-align:center">
      <div class="kpi-label">最大回撤</div>
      <div class="kpi-value" style="color:var(--warn)">${insufficient ? '資料不足' : fmtPct(re.max_drawdown_pct)}</div>
      <div class="kpi-hint">歷史峰值回撤幅度</div>
    </div>
    <div class="panel" style="text-align:center">
      <div class="kpi-label">Rolling Sharpe</div>
      <div class="kpi-value" style="color:${rollingSharpe !== null ? (rollingSharpe > 0.5 ? 'var(--up)' : (rollingSharpe < 0 ? 'var(--down)' : 'var(--warn)')) : 'var(--muted)'}">${rollingSharpe !== null ? rollingSharpe : '—'}</div>
      <div class="kpi-hint">${rollingSharpe !== null ? '風險調整後收益' : '尚無資金階段資料'}</div>
    </div>
    <div class="panel" style="text-align:center">
      <div class="kpi-label">投組淨值</div>
      <div class="kpi-value">${portfolioValue}</div>
      <div class="kpi-hint">${cashRatioPct !== null ? '現金 ' + cashRatioPct + '%' : ''}${exposureRatio !== null ? ' · 曝險 ' + exposureRatio + '%' : ''}</div>
    </div>
    <div class="panel" style="text-align:center">
      <div class="kpi-label">資金階段</div>
      <div class="kpi-value" style="font-size:16px">${phase}</div>
      <div class="kpi-hint">${daysInPhase > 0 ? '持續 ' + daysInPhase + ' 天' : ''}${consecLosses > 0 ? ' · 連續虧損 ' + consecLosses + ' 次' : ''}${canAdvance ? ' · 可推進' : ''}</div>
    </div>
    <div class="panel" style="text-align:center">
      <div class="kpi-label">持倉數</div>
      <div class="kpi-value">${re.position_count || 0}</div>
      <div class="kpi-hint">${re.data_points >= 30 ? '資料點 ' + re.data_points + ' · 可信' : '資料點 ' + (re.data_points || 0) + ' · 統計不足'}</div>
    </div>
    <div class="panel" style="text-align:center">
      <div class="kpi-label">保留現金</div>
      <div class="kpi-value">${cp.reserve_cash ? cp.reserve_cash.toLocaleString() : '—'}</div>
      <div class="kpi-hint">總資本 ${totalCapital ? totalCapital.toLocaleString() : '—'}</div>
    </div>
  `;

  if (posEl) posEl.innerHTML = `
    <div style="border-top:1px solid var(--border);padding-top:12px">
      <div style="font-size:13px;font-weight:700;margin-bottom:6px">倉位集中度分析</div>
      ${concentrationHtml}
    </div>
  `;

  if (sectorEl) sectorEl.innerHTML = sectorHtml;
}

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
