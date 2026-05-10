import { renderDualEquityCurve } from '../components/sparkline.js';

export async function loadPortfolioPage(getJSON, agentNameFn) {
  const kpis = document.getElementById('portfolioKPIs');
  const tableEl = document.getElementById('positionsTable');
  const historyEl = document.getElementById('tradeHistoryContainer');

  if (!kpis || !tableEl || !historyEl) return;

  kpis.innerHTML = '<div style="padding:20px;text-align:center;color:var(--muted)">資料載入中…</div>';
  tableEl.innerHTML = '<div style="padding:20px;text-align:center;color:var(--muted)">資料載入中…</div>';
  historyEl.innerHTML = '<div style="padding:20px;text-align:center;color:var(--muted)">資料載入中…</div>';

  try {
    const [liveData, stateData, taxData, tradeHistory] = await Promise.all([
      getJSON('/api/dashboard/live-status').catch(() => ({})),
      getJSON('/api/dashboard/portfolio-state').catch(() => ({})),
      getJSON('/api/dashboard/tax-snapshot').catch(() => ({})),
      getJSON('/api/dashboard/trade-history').catch(() => ([]))
    ]);

    const state = stateData || {};
    const positions = state.positions || [];
    const tax = taxData || {};
    const trades = Array.isArray(tradeHistory) ? tradeHistory : [];

    const totalTaxPaid = tax.total_tax_paid || 0;
    const afterTaxValue = (state.portfolio_value || 0) - totalTaxPaid;
    const realizedPnL = state.realized_pnl || 0;
    const tradeCount = state.trade_count || trades.length;

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
        <div class="kpi-label">已實現損益</div>
        <div class="kpi-value ${realizedPnL > 0 ? 'text-up' : (realizedPnL < 0 ? 'text-down' : '')}">${window.fmtNTD ? window.fmtNTD(realizedPnL) : realizedPnL.toFixed(0)}</div>
        <div class="kpi-hint">模擬累積已平倉損益</div>
      </div>
      <div class="kpi-card">
        <div class="kpi-label">累積交易數</div>
        <div class="kpi-value">${tradeCount}</div>
        <div class="kpi-hint">交易歷史總筆數</div>
      </div>
      <div class="kpi-card">
        <div class="kpi-label">累積稅負</div>
        <div class="kpi-value text-down">${window.fmtNTD ? window.fmtNTD(totalTaxPaid) : totalTaxPaid.toFixed(0)}</div>
        <div class="kpi-hint">持倉檔數: ${positions.length} | 更新: ${state.snapshot_time ? new Date(state.snapshot_time).toLocaleTimeString() : '-'}</div>
      </div>
    `;

    const equityCurve = state.equity_curve || [];
    const preTaxPoints = equityCurve.map(p => ({ label: p.label, value: p.value }));
    const afterTaxPoints = equityCurve.filter(p => p.after_tax_value !== undefined).map(p => ({ label: p.label, value: p.after_tax_value }));
    renderDualEquityCurve(preTaxPoints, afterTaxPoints);

    if (!positions.length) {
      tableEl.innerHTML = window.emptyState ? window.emptyState('尚無持倉資料', '') : '<div style="padding:20px;text-align:center;color:var(--muted)">尚無持倉資料</div>';
    } else {
      const fmtF = window.fmtFloat || (v => v.toFixed(2));
      const fmtI = window.fmtInt || (v => v.toString());
      const fmtP = window.fmtPct || (v => (v * 100).toFixed(2) + '%');
      const rows = positions.map(pos => {
        const pnl = pos.unrealized_pnl || 0;
        const pct = pos.pnl_pct || 0;
        const costBasis = (pos.average_cost || 0) * (pos.quantity || 0);
        const colorClass = window.pnlColor ? window.pnlColor(pnl) : (pnl > 0 ? 'text-up' : (pnl < 0 ? 'text-down' : ''));

        return `
          <tr>
            <td style="font-weight:600">${pos.symbol}</td>
            <td>—</td>
            <td style="text-align:right">${fmtI(pos.quantity)}</td>
            <td style="text-align:right">${fmtF(pos.average_cost)}</td>
            <td style="text-align:right">${fmtF(costBasis)}</td>
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
                <th style="text-align:right">持倉成本</th>
                <th style="text-align:right">現價</th>
                <th style="text-align:right">市值</th>
                <th style="text-align:right">未實現損益</th>
                <th style="text-align:right">損益率</th>
              </tr>
            </thead>
            <tbody>${rows}</tbody>
          </table>
        </div>
      `;
    }

    if (!trades.length) {
      historyEl.innerHTML = '<div style="padding:20px;text-align:center;color:var(--muted)">尚無交易歷史</div>';
    } else {
      const fmtI = window.fmtInt || (v => v.toString());
      const tradeRows = trades.map(trade => {
        const amount = trade.amount ?? ((trade.quantity || 0) * (trade.price || 0));
        const sideClass = trade.side === 'BUY' ? 'text-up' : 'text-down';
        const sideLabel = trade.side === 'BUY' ? '買入' : '賣出';
        const ts = trade.timestamp ? new Date(trade.timestamp).toLocaleString() : '—';
        return `
          <tr>
            <td>${ts}</td>
            <td style="font-weight:600">${trade.symbol || '—'}</td>
            <td class="${sideClass}">${sideLabel}</td>
            <td style="text-align:right">${fmtI(trade.quantity || 0)}</td>
            <td style="text-align:right">${window.fmtFloat ? window.fmtFloat(trade.price || 0) : (trade.price || 0).toFixed(2)}</td>
            <td style="text-align:right">${window.fmtNTD ? window.fmtNTD(amount) : amount.toFixed(0)}</td>
            <td>${trade.reason || '—'}</td>
          </tr>
        `;
      }).join('');

      historyEl.innerHTML = `
        <div class="table-wrapper">
          <table class="text-sm">
            <thead>
              <tr>
                <th style="text-align:left">時間</th>
                <th style="text-align:left">標的</th>
                <th style="text-align:left">方向</th>
                <th style="text-align:right">數量</th>
                <th style="text-align:right">成交價</th>
                <th style="text-align:right">成交金額</th>
                <th style="text-align:left">原因</th>
              </tr>
            </thead>
            <tbody>${tradeRows}</tbody>
          </table>
        </div>
      `;
    }
  } catch (e) {
    console.error(e);
    kpis.innerHTML = '<div style="padding:20px;text-align:center;color:var(--down)">載入失敗</div>';
    tableEl.innerHTML = '<div style="padding:20px;text-align:center;color:var(--down)">載入失敗</div>';
    historyEl.innerHTML = '<div style="padding:20px;text-align:center;color:var(--down)">載入失敗</div>';
  }
}
