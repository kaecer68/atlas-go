import { renderDualEquityCurve } from '../components/sparkline.js';
import { stockName } from '../names.js';

const SYMBOL_SECTOR_MAP = {
  '0050.TW': 'ETF', '0056.TW': 'ETF', '00878.TW': 'ETF',
  '1301.TW': '石化', '1303.TW': '石化', '1326.TW': '石化',
  '2303.TW': '半導體', '2330.TW': '半導體', '2454.TW': '半導體', '3034.TW': '半導體', '3037.TW': '半導體',
  '2308.TW': '電子零組件', '2382.TW': 'AI 供應鏈', '6669.TW': 'AI 供應鏈',
  '2317.TW': '電子組裝',
  '2603.TW': '航運', '2609.TW': '航運', '2615.TW': '航運',
  '2881.TW': '金融', '2882.TW': '金融', '2886.TW': '金融', '2891.TW': '金融', '2892.TW': '金融',
  '3008.TW': '光學', '3017.TW': '散熱'
};

function getSector(symbol) {
  return SYMBOL_SECTOR_MAP[symbol] || SYMBOL_SECTOR_MAP[symbol?.replace('.TW', '')] || '—';
}

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
        <div class="kpi-label">投組稅前淨值</div>
        <div class="kpi-value">${window.fmtNTD ? window.fmtNTD(state.portfolio_value || 0) : (state.portfolio_value || 0).toFixed(0)}</div>
        <div class="kpi-hint">含現金與持倉市值 | 可用現金: ${window.fmtNTD ? window.fmtNTD(state.cash || 0) : (state.cash || 0).toFixed(0)}</div>
      </div>
      <div class="kpi-card">
        <div class="kpi-label">投組稅後淨值</div>
        <div class="kpi-value">${window.fmtNTD ? window.fmtNTD(afterTaxValue) : afterTaxValue.toFixed(0)}</div>
        <div class="kpi-hint">已扣除累積稅負 ${window.fmtNTD ? window.fmtNTD(totalTaxPaid) : totalTaxPaid.toFixed(0)}</div>
      </div>
      <div class="kpi-card">
        <div class="kpi-label">累積稅負</div>
        <div class="kpi-value text-down">${window.fmtNTD ? window.fmtNTD(totalTaxPaid) : totalTaxPaid.toFixed(0)}</div>
        <div class="kpi-hint">持倉 ${positions.length} 檔 | 更新: ${state.snapshot_time ? new Date(state.snapshot_time).toLocaleTimeString() : '-'}</div>
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
      let pnl = pos.unrealized_pnl || 0;
      let pct = pos.pnl_pct || 0;
      if (pnl === 0 && pos.average_cost && pos.current_price && pos.quantity) {
        const costBasis = pos.average_cost * pos.quantity;
        const marketValue = pos.current_price * pos.quantity;
        pnl = marketValue - costBasis;
        pct = pos.average_cost > 0 ? (pos.current_price - pos.average_cost) / pos.average_cost : 0;
      }
      const colorClass = pnl > 0 ? 'text-up' : (pnl < 0 ? 'text-down' : '');
      const fmtF = window.fmtFloat || (v => v.toFixed(2));
      const fmtI = window.fmtInt || (v => v.toString());
      const fmtP = window.fmtPct || (v => (v*100).toFixed(2) + '%');
      const name = stockName(pos.symbol);
      const sector = getSector(pos.symbol);
      
      return `
        <tr>
          <td>
            <div style="font-weight:600">${pos.symbol}</div>
            <div style="font-size:12px;color:var(--muted)">${name}</div>
          </td>
          <td>${sector}</td>
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
