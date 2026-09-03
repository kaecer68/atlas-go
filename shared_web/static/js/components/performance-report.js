import { fmtNTD, fmtInt, escapeHtml } from '../shared/utils.js';
import { fmtSafePct, fmtSafeNumber, fmtSafeDrawdown } from '../shared/format-metric.js';
import { agentName as agentNameEsm, regimeLabel as regimeLabelEsm } from '../shared/constants.js';

export function renderPerformanceReport(container) {
  if (typeof container === 'string') container = document.getElementById(container);
  if (!container) { console.error('performance-report: container not found'); return; }
  container.innerHTML = `
    <div class="pr-toolbar">
      <div class="pr-period-selector" id="prPeriodSelector">
        <button class="pr-period-btn active" data-period="30d">30 天</button>
        <button class="pr-period-btn" data-period="90d">90 天</button>
        <button class="pr-period-btn" data-period="1y">1 年</button>
        <button class="pr-period-btn" data-period="all">全部期間</button>
      </div>
      <button class="pr-export-btn" id="prExportBtn">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path><polyline points="7 10 12 15 17 10"></polyline><line x1="12" y1="15" x2="12" y2="3"></line></svg>
        匯出 Markdown
      </button>
    </div>
    
    <div class="pr-summary-header" id="prDateRange">載入中…</div>
    
    <div id="prKpiGrid" class="pr-grid">
      <!-- KPI cards -->
    </div>
    
    <div class="pr-section-title">🏆 最佳貢獻 AI</div>
    <div class="pr-table-container">
      <table class="pr-table">
        <thead>
          <tr>
            <th>AI 名稱</th>
            <th class="num">累積遠期報酬 (pt)</th>
            <th class="num">勝率</th>
            <th class="num">夏普值</th>
          </tr>
        </thead>
        <tbody id="prAgentsBody">
          <!-- agents -->
        </tbody>
      </table>
    </div>
    
    <div class="pr-section-title">📊 市場狀態績效</div>
    <div class="pr-table-container">
      <table class="pr-table">
        <thead>
          <tr>
            <th>市場狀態</th>
            <th class="num">場次數</th>
            <th class="num">平均報酬</th>
            <th class="num">勝率</th>
          </tr>
        </thead>
        <tbody id="prRegimesBody">
          <!-- regimes -->
        </tbody>
      </table>
    </div>
    
    <div class="pr-section-title">📅 月度報酬</div>
    <div class="pr-table-container">
      <table class="pr-table">
        <thead>
          <tr>
            <th>月份</th>
            <th class="num">報酬</th>
            <th class="num">累積</th>
          </tr>
        </thead>
        <tbody id="prMonthsBody">
          <!-- months -->
        </tbody>
      </table>
    </div>
  `;

  const selector = container.querySelector('#prPeriodSelector');
  selector.addEventListener('click', (e) => {
    if (e.target.tagName === 'BUTTON') {
      selector.querySelectorAll('button').forEach(b => b.classList.remove('active'));
      e.target.classList.add('active');
      fetchPerformanceReport(e.target.dataset.period);
    }
  });

  container.querySelector('#prExportBtn').addEventListener('click', () => {
    const activePeriod = selector.querySelector('.active').dataset.period;
    exportPerformanceReport('md', activePeriod);
  });

  fetchPerformanceReport('30d');
}

async function fetchPerformanceReport(period) {
  const kpiGrid = document.getElementById('prKpiGrid');
  if (!kpiGrid) return;
  
  kpiGrid.innerHTML = '<div class="pr-loading">載入報告資料中…</div>';
  
  try {
    const res = await fetch(`/api/dashboard/performance-report?period=${period}`);
    if (!res.ok) throw new Error('無法取得績效報告');
    const data = await res.json();
    renderReportData(data);
  } catch (err) {
    kpiGrid.innerHTML = `<div class="pr-loading" style="color:var(--color-danger)">錯誤：${err.message}</div>`;
  }
}

function exportPerformanceReport(format, period) {
  window.open(`/api/dashboard/performance-report/export?format=${format}&period=${period}`, '_blank');
}

function pnlColorStyle(v) {
  if (typeof v !== 'number' || !Number.isFinite(v) || v === 0) return 'var(--text)';
  return v > 0 ? 'var(--up)' : 'var(--down)';
}

function drawdownSign(v) {
  if (typeof v !== 'number' || !Number.isFinite(v) || v === 0) return 0;
  return v > 0 ? -1 : 1;
}

function renderReportData(data) {
  document.getElementById('prDateRange').textContent = `${(data.start_date || '--').slice(0,10)} ～ ${(data.end_date || '--').slice(0,10)}`;

  // 期間中文標籤（KPI 口徑 hint 用）
  const periodZh = { '30d': '近 30 日', '90d': '近 90 日', '1y': '近 1 年', 'all': '全部期間' }[data.period] || '本期間';

  const kpis = [
    { label: '總報酬', value: fmtSafePct(data.total_return), sign: data.total_return },
    { label: '年化報酬', value: fmtSafePct(data.annualized_return), sign: data.annualized_return },
    { label: '夏普比率', value: fmtSafeNumber(data.sharpe_ratio, { decimals: 2, useGrouping: true }), hint: '口徑：期間日報酬（AI 表 sharpe_like 不同）' },
    { label: '最大回撤', value: fmtSafeDrawdown(data.max_drawdown), sign: drawdownSign(data.max_drawdown), hint: `${periodZh} · 期間內最大回撤` },
    { label: '稅後價值', value: fmtNTD(data.after_tax_value) },
    { label: '已繳稅額', value: fmtNTD(data.total_tax_paid), hint: '累積' },
    { label: '勝率', value: fmtSafePct(data.win_rate), sign: data.win_rate },
    { label: '總交易數', value: fmtInt(data.total_trades) }
  ];

  const gridHtml = kpis.map(function(k) {
    var cls = '';
    if (k.sign > 0) cls = 'positive';
    else if (k.sign < 0) cls = 'negative';
    return `<div class="pr-card">
      <div class="pr-card-label">${k.label}</div>
      <div class="pr-card-value ${cls}">${k.value}</div>
      ${k.hint ? '<div class="pr-card-hint">' + k.hint + '</div>' : ''}
    </div>`;
  }).join('');
  
  document.getElementById('prKpiGrid').innerHTML = gridHtml;

  const agentsBody = document.getElementById('prAgentsBody');
  if (data.top_agents && data.top_agents.length > 0) {
    agentsBody.innerHTML = data.top_agents.map(function(a) {
      // aggregate_forward_return 是多筆決策遠期報酬的加總（fraction），
      // 不是單一報酬率：×100 後以「點 (pt)」呈現，避免誤讀成 3610.9%。
      var ret = a.aggregate_forward_return;
      var retPt = fmtSafeNumber(ret * 100, { decimals: 1, useGrouping: true }) + ' pt';
      var sharpeCell = fmtSafeNumber(a.sharpe_like, { decimals: 2, useGrouping: true });
      return '<tr>' +
        '<td>' + escapeHtml(a.display_name || agentNameEsm(a.agent_id) || a.agent_id) + '</td>' +
        '<td class="num" style="color:' + pnlColorStyle(ret) + '">' + retPt + '</td>' +
        '<td class="num">' + fmtSafePct(a.win_rate) + '</td>' +
        '<td class="num">' + sharpeCell + '</td>' +
        '</tr>';
    }).join('');
  } else {
    agentsBody.innerHTML = '<tr><td colspan="4" class="pr-loading">尚無 AI 績效資料</td></tr>';
  }

  const regimesBody = document.getElementById('prRegimesBody');
  var regimeMap = (data.regime_breakdown && data.regime_breakdown.regimes) ? data.regime_breakdown.regimes : {};
  var regimes = Object.entries(regimeMap);
  if (regimes.length > 0) {
    regimesBody.innerHTML = regimes.map(function(entry) {
      var id = entry[0], r = entry[1];
      var avgRet = r.avg_return;
      var sessions = r.session_count;
      return '<tr>' +
        '<td>' + escapeHtml(regimeLabelEsm(id) || id) + '</td>' +
        '<td class="num">' + fmtInt(sessions) + '</td>' +
        '<td class="num" style="color:' + pnlColorStyle(avgRet) + '">' + fmtSafePct(avgRet) + '</td>' +
        '<td class="num">' + fmtSafePct(r.win_rate) + '</td>' +
        '</tr>';
    }).join('');
  } else {
    regimesBody.innerHTML = '<tr><td colspan="4" class="pr-loading">尚無市場狀態績效資料</td></tr>';
  }

  const monthsBody = document.getElementById('prMonthsBody');
  if (data.monthly_returns && data.monthly_returns.length > 0) {
    monthsBody.innerHTML = data.monthly_returns.map(function(m) {
      var ret = m.return;
      return '<tr>' +
        '<td>' + escapeHtml(m.label || m.month || '--') + '</td>' +
        '<td class="num" style="color:' + pnlColorStyle(ret) + '">' + fmtSafePct(ret) + '</td>' +
        '<td class="num">' + fmtSafePct(m.cumulative) + '</td>' +
        '</tr>';
    }).join('');
  } else {
    monthsBody.innerHTML = '<tr><td colspan="3" class="pr-loading">尚無月度報酬資料</td></tr>';
  }
}
