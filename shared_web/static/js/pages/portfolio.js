import { renderDualEquityCurve } from '../components/sparkline.js';
import { renderPnLAttribution } from '../components/attribution.js';
import { renderBenchmarkComparison } from '../components/benchmark.js';

import { renderRiskPanel } from '../components/risk-panel.js';
import { renderRiskGatePanel } from '../components/risk-gate-panel.js';

import { renderStockCell } from '../names.js';
import { formatMaxDrawdown, formatHHI, fmtSafeNumber, fmtSafeSignedPct, fmtCurrency } from '../shared/format-metric.js';

function isValidNumber(v) {
  return typeof v === 'number' && Number.isFinite(v);
}

function renderActionEmptyState(title, description, pageId, buttonText) {
  // Use data-page so the global click delegation in both app shells handles
  // navigation. Falls back to the strategies page when the preferred target
  // does not exist in the current shell.
  const target = document.getElementById('page-' + pageId) ? pageId : 'strategies';
  const label = target === pageId ? buttonText : '查看投資心法';
  return `
    <div class="action-empty-state">
      <div class="action-empty-state-icon">📂</div>
      <div class="action-empty-state-title">${title}</div>
      <div class="action-empty-state-description">${description}</div>
      <a class="action-empty-state-button" data-page="${target}" href="#">${label}</a>
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

    const portfolioValue = isValidNumber(state.portfolio_value) ? state.portfolio_value : null;
    const totalTaxPaid = isValidNumber(tax.total_tax_paid) ? tax.total_tax_paid : null;
    const afterTaxValue = portfolioValue !== null && totalTaxPaid !== null ? portfolioValue - totalTaxPaid : null;
    const realizedPnL = isValidNumber(state.realized_pnl) ? state.realized_pnl : null;
    // B10 (risk-console Phase 1)：trade_count 缺欄位時顯示 —，
    // 禁止 fallback 成空陣列長度（0）誤導使用者；交易總數以績效報告為準。
    // 2026-09-03：trade_count 改為 trades 表實際成交（下單）筆數（後端與績效
    // 報告同源 SSoT store）；績效報告「總交易數」另含 AI 推薦模擬撮合，兩者
    // 口徑不同，hint 明示避免與績效報告數字混淆。
    const tradeCount = typeof state.trade_count === 'number' ? state.trade_count : null;
    const unrealizedPnLTotal = isValidNumber(state.unrealized_pnl_total) ? state.unrealized_pnl_total : null;
    const concentrationRatio = isValidNumber(state.concentration_ratio) ? state.concentration_ratio : null;
    const maxDrawdown = isValidNumber(state.max_drawdown) ? state.max_drawdown : null;

    const sectorLabels = {
      'semiconductor': '半導體', 'ai_supply_chain': 'AI供應鏈',
      'robotics': '機器人', 'financials': '金融', 'shipping': '航運',
      'energy': '能源', 'electronics': '電子', 'consumer': '消費',
      'industrial': '工業', 'other': '其他'
    };

    function kpiNTD(v) {
      return window.fmtNTD ? window.fmtNTD(v) : fmtCurrency(v, { decimals: 0 });
    }
    function kpiNum(v) {
      return typeof v === 'number' ? v.toString() : '—';
    }
    function pnlToneClass(v) {
      if (!isValidNumber(v)) return '';
      return v > 0 ? 'text-up' : (v < 0 ? 'text-down' : '');
    }

    const hhi = formatHHI(concentrationRatio);
    const hhiLabel = { low: '分散', medium: '中等', high: '集中' }[hhi.level] || '—';

    kpis.innerHTML = `
      <div class="kpi-card">
        <div class="kpi-label">稅前淨值</div>
        <div class="kpi-value">${kpiNTD(portfolioValue)}</div>
        <div class="kpi-hint">可用現金: ${kpiNTD(state.cash)}</div>
      </div>
      <div class="kpi-card">
        <div class="kpi-label">稅後淨值（若今日清倉）</div>
        <div class="kpi-value">${kpiNTD(afterTaxValue)}</div>
        <div class="kpi-hint">已扣除清倉預估稅費</div>
      </div>
      <div class="kpi-card">
        <div class="kpi-label">已實現損益</div>
        <div class="kpi-value ${pnlToneClass(realizedPnL)}">${kpiNTD(realizedPnL)}</div>
        <div class="kpi-hint">模擬累積已平倉損益</div>
      </div>
      <div class="kpi-card">
        <div class="kpi-label">累積交易數</div>
        <div class="kpi-value">${kpiNum(tradeCount)}</div>
        <div class="kpi-hint">${tradeCount === null ? '以績效報告「總交易數」為準' : `實際下單 ${tradeCount.toLocaleString('en-US')} 筆 · 績效報告另計模擬撮合`}</div>
      </div>
      <div class="kpi-card">
        <div class="kpi-label">若今日清倉預估稅費</div>
        <div class="kpi-value text-danger">${kpiNTD(totalTaxPaid)}</div>
        <div class="kpi-hint">清倉試算（非已繳累計）| 持倉檔數: ${positions.length} | 更新: ${state.snapshot_time ? new Date(state.snapshot_time).toLocaleTimeString() : '—'}</div>
      </div>
      <div class="kpi-card">
        <div class="kpi-label">未實現損益</div>
        <div class="kpi-value ${pnlToneClass(unrealizedPnLTotal)}">${kpiNTD(unrealizedPnLTotal)}</div>
        <div class="kpi-hint">持倉未實現總損益</div>
      </div>
      <div class="kpi-card">
        <div class="kpi-label">持倉集中度 (HHI)</div>
        <div class="kpi-value">${hhi.value}</div>
        <div class="kpi-hint">${hhiLabel}</div>
      </div>
      <div class="kpi-card">
        <div class="kpi-label">最大回撤</div>
        <div class="kpi-value ${maxDrawdown === 0 ? '' : 'text-danger'}">${formatMaxDrawdown(maxDrawdown, { asAbsolute: true })}</div>
        <div class="kpi-hint">歷史最大回撤</div>
      </div>
    `;

    const equityCurve = state.equity_curve || [];
    const preTaxPoints = equityCurve.map(p => ({ label: p.label, value: p.value }));
    const afterTaxPoints = equityCurve.filter(p => p.after_tax_value !== undefined).map(p => ({ label: p.label, value: p.after_tax_value }));
    renderDualEquityCurve(preTaxPoints, afterTaxPoints);

    if (!positions.length) {
      tableEl.innerHTML = renderActionEmptyState('尚無持倉資料', '執行一次模擬交易以建立示範持倉', 'strategies', '前往投資心法');
    } else {
      const fmtF = (v) => fmtSafeNumber(v, { decimals: 2 });
      const fmtI = (v) => isValidNumber(v) ? v.toLocaleString('en-US') : '—';
      const fmtP = (v) => fmtSafeSignedPct(v, 2);
      const rows = positions.map(pos => {
        const quantity = isValidNumber(pos.quantity) ? pos.quantity : null;
        const avgCost = isValidNumber(pos.average_cost) ? pos.average_cost : null;
        const currentPrice = isValidNumber(pos.current_price) ? pos.current_price : null;
        const marketValue = isValidNumber(pos.market_value) ? pos.market_value : null;
        const pnl = isValidNumber(pos.unrealized_pnl) ? pos.unrealized_pnl : null;
        const pct = isValidNumber(pos.pnl_pct) ? pos.pnl_pct : null;
        const costBasis = quantity !== null && avgCost !== null ? quantity * avgCost : null;
        const colorClass = pnl !== null ? (pnl > 0 ? 'text-up' : (pnl < 0 ? 'text-down' : '')) : '';

        return `
          <tr>
            <td>${renderStockCell(pos.symbol)}</td>
            <td>${sectorLabels[pos.sector] || pos.sector || '—'}</td>
            <td style="text-align:right">${fmtI(quantity)}</td>
            <td style="text-align:right">${fmtF(avgCost)}</td>
            <td style="text-align:right">${fmtF(costBasis)}</td>
            <td style="text-align:right">${fmtF(currentPrice)}</td>
            <td style="text-align:right">${fmtF(marketValue)}</td>
            <td style="text-align:right" class="${colorClass}">${fmtF(pnl)}</td>
            <td style="text-align:right" class="${colorClass}">${fmtP(pct)}</td>
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
      const fmtI = (v) => isValidNumber(v) ? v.toLocaleString('en-US') : '—';
      const tradeRows = trades.map(trade => {
        const quantity = isValidNumber(trade.quantity) ? trade.quantity : null;
        const price = isValidNumber(trade.price) ? trade.price : null;
        const amount = isValidNumber(trade.amount) ? trade.amount : (quantity !== null && price !== null ? quantity * price : null);
        const sideClass = trade.side === 'BUY' ? 'text-up' : 'text-down';
        const sideLabel = trade.side === 'BUY' ? '買入' : '賣出';
        const ts = trade.timestamp ? new Date(trade.timestamp).toLocaleString() : '—';
        return `
          <tr>
            <td>${ts}</td>
            <td>${trade.symbol ? renderStockCell(trade.symbol) : '—'}</td>
            <td class="${sideClass}">${sideLabel}</td>
            <td style="text-align:right">${fmtI(quantity)}</td>
            <td style="text-align:right">${fmtSafeNumber(price, { decimals: 2 })}</td>
            <td style="text-align:right">${isValidNumber(amount) ? (window.fmtNTD ? window.fmtNTD(amount) : fmtCurrency(amount, { decimals: 0 })) : '—'}</td>
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

