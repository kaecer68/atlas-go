import { renderDualEquityCurve } from '../components/sparkline.js';

export async function loadPortfolioPage(getJSON, agentNameFn) {
  const kpis = document.getElementById('portfolioKPIs');
  const tableEl = document.getElementById('positionsTable');
  
  if (!kpis || !tableEl) return;
  
  kpis.innerHTML = '<div style="padding:20px;text-align:center;color:var(--muted)">資料載入中…</div>';
  tableEl.innerHTML = '<div style="padding:20px;text-align:center;color:var(--muted)">資料載入中…</div>';
  
  try {
    const [liveData, stateData, taxData] = await Promise.all([
      getJSON('/api/dashboard/live-status').catch(() => ({})),
      getJSON('/api/dashboard/portfolio-state').catch(() => ({})),
      getJSON('/api/dashboard/tax-snapshot').catch(() => ({}))
    ]);
    
    const p = liveData?.portfolio || {};
    const state = stateData || {};
    const positions = state.positions || [];
    const tax = taxData || {};

    const totalTaxPaid = tax.total_tax_paid || 0;
    const afterTaxValue = (state.portfolio_value || 0) - totalTaxPaid;
    
    kpis.innerHTML = `
      <div class="kpi-card">
        <div class="kpi-label">稅前淨值</div>
        <div class="kpi-value">${window.fmtNTD ? window.fmtNTD(state.portfolio_value || 0) : (state.portfolio_value || 0).toFixed(0)}</div>
        <div class="kpi-hint">可用現金: ${window.fmtNTD ? window.fmtNTD(state.cash || 0) : (state.cash || 0).toFixed(0)}</div>
      </div>
      <div class="kpi-card">
        <div class="kpi-label">稅後淨值</div>
        <div class="kpi-value">${window.fmtNTD ? window.fmtNTD(afterTaxValue) : afterTaxValue.toFixed(0)}</div>
        <div class="kpi-hint">已扣除累積稅負</div>
      </div>
      <div class="kpi-card">
        <div class="kpi-label">累積稅負</div>
        <div class="kpi-value text-down">${window.fmtNTD ? window.fmtNTD(totalTaxPaid) : totalTaxPaid.toFixed(0)}</div>
        <div class="kpi-hint">持倉檔數: ${positions.length} | 更新: ${state.snapshot_time ? new Date(state.snapshot_time).toLocaleTimeString() : '-'}</div>
      </div>
    `;

    // Render dual-curve equity chart with pre-tax and after-tax lines
    const equityCurve = state.equity_curve || [];
    const preTaxPoints = equityCurve.map(p => ({ label: p.label, value: p.value }));
    const afterTaxPoints = equityCurve
      .filter(p => p.after_tax_value !== undefined)
      .map(p => ({ label: p.label, value: p.after_tax_value }));
    renderDualEquityCurve(preTaxPoints, afterTaxPoints);

    if (!positions || positions.length === 0) {
      tableEl.innerHTML = window.emptyState ? window.emptyState('尚無持倉資料', '') : '<div style="padding:20px;text-align:center;color:var(--muted)">尚無持倉資料</div>';
      return;
    }

    const rows = positions.map(pos => {
      const pnl = pos.unrealized_pnl || 0;
      const pct = pos.pnl_pct || 0;
      const colorClass = window.pnlColor ? window.pnlColor(pnl) : (pnl > 0 ? 'text-up' : (pnl < 0 ? 'text-down' : ''));
      const fmtF = window.fmtFloat || (v => v.toFixed(2));
      const fmtI = window.fmtInt || (v => v.toString());
      const fmtP = window.fmtPct || (v => (v*100).toFixed(2) + '%');
      
      return `
        <tr>
          <td style="font-weight:600">${pos.symbol}</td>
          <td>—</td>
          <td style="text-align:right">${fmtI(pos.quantity)}</td>
          <td style="text-align:right">${fmtF(pos.average_cost)}</td>
          <td style="text-align:right">${fmtF(pos.current_price)}</td>
          <td style="text-align:right">${fmtF(pos.market_value)}</td>
          <td style="text-align:right" class="${colorClass}">${pnl > 0 ? '+' : ''}${fmtF(pnl)}</td>
          <td style="text-align:right" class="${colorClass}">${pnl > 0 ? '+' : ''}${fmtP(pct)}</td>
        </tr>
      `;
    }).join('');

    tableEl.innerHTML = `
      <div class="table-wrapper">
        <table class="text-sm">
          <thead>
            <tr>
              <th style="text-align:left">標的</th>
              <th style="text-align:left">產業板塊</th>
              <th style="text-align:right">數量 (股)</th>
              <th style="text-align:right">平均成本</th>
              <th style="text-align:right">現價</th>
              <th style="text-align:right">市值</th>
              <th style="text-align:right">未實現損益</th>
              <th style="text-align:right">損益率</th>
            </tr>
          </thead>
          <tbody>
            ${rows}
          </tbody>
        </table>
      </div>
    `;

  } catch (e) {
    console.error(e);
    kpis.innerHTML = '<div style="padding:20px;text-align:center;color:var(--down)">載入失敗</div>';
    tableEl.innerHTML = '<div style="padding:20px;text-align:center;color:var(--down)">載入失敗</div>';
  }
}
