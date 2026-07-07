import { renderDualEquityCurve } from '../components/sparkline.js';
import { renderPnLAttribution } from '../components/attribution.js';
import { renderBenchmarkComparison } from '../components/benchmark.js';

import { renderRiskPanel } from '../components/risk-panel.js';
import { renderRiskGatePanel } from '../components/risk-gate-panel.js';

import { renderStockCell } from '../names.js';
import { formatMaxDrawdown, formatHHI } from '../shared/format-metric.js';

function renderActionEmptyState(title, description, pageId, buttonText) {
  return `
    <div class="action-empty-state">
      <div class="action-empty-state-icon">📂</div>
      <div class="action-empty-state-title">${title}</div>
      <div class="action-empty-state-description">${description}</div>
      <button class="action-empty-state-button" data-nav="${pageId}" type="button">${buttonText}</button>
    </div>
  `;
}

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
    const unrealizedPnLTotal = state.unrealized_pnl_total || 0;
    const concentrationRatio = state.concentration_ratio || 0;
    const currentDrawdown = state.current_drawdown || 0;

    const sectorLabels = {
      'semiconductor': '半導體', 'ai_supply_chain': 'AI供應鏈',
      'robotics': '機器人', 'financials': '金融', 'shipping': '航運',
      'energy': '能源', 'electronics': '電子', 'consumer': '消費',
      'industrial': '工業', 'other': '其他'
    };

    function kpiNTD(v) {
  if (v == null) return '—';
  return window.fmtNTD ? window.fmtNTD(v) : v.toFixed(0);
}
function kpiPct(v) {
  if (v == null) return '—';
  return window.fmtPct ? window.fmtPct(v) : (v * 100).toFixed(2) + '%';
}
function kpiNum(v) {
  if (v == null) return '—';
  return v.toString();
}

    const hhi = formatHHI(concentrationRatio);
    const hhiLabel = { low: '分散', medium: '中等', high: '集中' }[hhi.level] || '—';

    kpis.innerHTML = `
      <div class="kpi-card">
        <div class="kpi-label">稅前淨值</div>
        <div class="kpi-value">${kpiNTD(state.portfolio_value)}</div>
        <div class="kpi-hint">可用現金: ${kpiNTD(state.cash)}</div>
      </div>
      <div class="kpi-card">
        <div class="kpi-label">稅後淨值</div>
        <div class="kpi-value">${kpiNTD(afterTaxValue)}</div>
        <div class="kpi-hint">已扣除累積稅負</div>
      </div>
      <div class="kpi-card">
        <div class="kpi-label">已實現損益</div>
        <div class="kpi-value ${realizedPnL > 0 ? 'text-up' : (realizedPnL < 0 ? 'text-down' : '')}">${kpiNTD(realizedPnL)}</div>
        <div class="kpi-hint">模擬累積已平倉損益</div>
      </div>
      <div class="kpi-card">
        <div class="kpi-label">累積交易數</div>
        <div class="kpi-value">${kpiNum(tradeCount)}</div>
        <div class="kpi-hint">交易歷史總筆數</div>
      </div>
      <div class="kpi-card">
        <div class="kpi-label">累積稅負</div>
        <div class="kpi-value text-danger">${kpiNTD(totalTaxPaid)}</div>
        <div class="kpi-hint">持倉檔數: ${positions.length} | 更新: ${state.snapshot_time ? new Date(state.snapshot_time).toLocaleTimeString() : '—'}</div>
      </div>
      <div class="kpi-card">
        <div class="kpi-label">未實現損益</div>
        <div class="kpi-value ${unrealizedPnLTotal > 0 ? 'text-up' : (unrealizedPnLTotal < 0 ? 'text-down' : '')}">${kpiNTD(unrealizedPnLTotal)}</div>
        <div class="kpi-hint">持倉未實現總損益</div>
      </div>
      <div class="kpi-card">
        <div class="kpi-label">持倉集中度 (HHI)</div>
        <div class="kpi-value">${hhi.value}</div>
        <div class="kpi-hint">${hhiLabel}</div>
      </div>
      <div class="kpi-card">
        <div class="kpi-label">最大回撤</div>
        <div class="kpi-value text-danger">${formatMaxDrawdown(currentDrawdown, { asAbsolute: true })}</div>
        <div class="kpi-hint">歷史最大回撤</div>
      </div>
    `;

    const equityCurve = state.equity_curve || [];
    const preTaxPoints = equityCurve.map(p => ({ label: p.label, value: p.value }));
    const afterTaxPoints = equityCurve.filter(p => p.after_tax_value !== undefined).map(p => ({ label: p.label, value: p.after_tax_value }));
    renderDualEquityCurve(preTaxPoints, afterTaxPoints);

    if (!positions.length) {
      tableEl.innerHTML = renderActionEmptyState('尚無持倉資料', '執行一次模擬交易以建立示範持倉', 'evolution_panel', '前往策略演化');
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
            <td>${renderStockCell(pos.symbol)}</td>
            <td>${sectorLabels[pos.sector] || pos.sector || '—'}</td>
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
            <td>${trade.symbol ? renderStockCell(trade.symbol) : '—'}</td>
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

    const attrContainer = document.getElementById('pnlAttribution');
    if (attrContainer) { renderPnLAttribution(attrContainer, getJSON); }
    const benchContainer = document.getElementById('benchmarkComparison');
    if (benchContainer) { renderBenchmarkComparison(benchContainer, getJSON); }
    const riskContainer = document.getElementById('riskPanel');
    if (riskContainer) { renderRiskPanel(riskContainer, getJSON); }
    const riskGateContainer = document.getElementById('riskGatePanel');
    if (riskGateContainer) { renderRiskGatePanel(riskGateContainer, getJSON); }
  } catch (e) {
    console.warn(e);
    kpis.innerHTML = '<div style="padding:20px;text-align:center;color:var(--color-danger)">載入失敗</div>';
    tableEl.innerHTML = '<div style="padding:20px;text-align:center;color:var(--color-danger)">載入失敗</div>';
    historyEl.innerHTML = '<div style="padding:20px;text-align:center;color:var(--color-danger)">載入失敗</div>';
  }
}

