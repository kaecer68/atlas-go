export function renderPerformanceReport(container) {
  container.innerHTML = `
    <div class="pr-toolbar">
      <div class="pr-period-selector" id="prPeriodSelector">
        <button class="pr-period-btn active" data-period="30d">30 Days</button>
        <button class="pr-period-btn" data-period="90d">90 Days</button>
        <button class="pr-period-btn" data-period="1y">1 Year</button>
        <button class="pr-period-btn" data-period="all">All Time</button>
      </div>
      <button class="pr-export-btn" id="prExportBtn">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path><polyline points="7 10 12 15 17 10"></polyline><line x1="12" y1="15" x2="12" y2="3"></line></svg>
        Export MD
      </button>
    </div>
    
    <div class="pr-summary-header" id="prDateRange">Loading...</div>
    
    <div id="prKpiGrid" class="pr-grid">
      <!-- KPIs rendered here -->
    </div>
    
    <div class="pr-section-title">Top Contributing Agents</div>
    <div class="pr-table-container">
      <table class="pr-table">
        <thead>
          <tr>
            <th>Agent Name</th>
            <th>Contribution</th>
            <th>Win Rate</th>
            <th>Sharpe</th>
          </tr>
        </thead>
        <tbody id="prAgentsBody">
          <!-- Agents rendered here -->
        </tbody>
      </table>
    </div>
    
    <div class="pr-section-title">Regime Performance</div>
    <div class="pr-table-container">
      <table class="pr-table">
        <thead>
          <tr>
            <th>Regime</th>
            <th>Sessions</th>
            <th>Avg Return</th>
            <th>Win Rate</th>
          </tr>
        </thead>
        <tbody id="prRegimesBody">
          <!-- Regimes rendered here -->
        </tbody>
      </table>
    </div>
    
    <div class="pr-section-title">Monthly Returns</div>
    <div class="pr-table-container">
      <table class="pr-table">
        <thead>
          <tr>
            <th>Month</th>
            <th>Return</th>
            <th>Cumulative</th>
          </tr>
        </thead>
        <tbody id="prMonthsBody">
          <!-- Months rendered here -->
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
  
  kpiGrid.innerHTML = '<div class="pr-loading">Loading report data...</div>';
  
  try {
    const res = await fetch(`/api/dashboard/performance-report?period=${period}`);
    if (!res.ok) throw new Error('Failed to fetch performance report');
    const data = await res.json();
    renderReportData(data);
  } catch (err) {
    kpiGrid.innerHTML = `<div class="pr-loading" style="color:var(--down)">Error: ${err.message}</div>`;
  }
}

function exportPerformanceReport(format, period) {
  window.open(`/api/dashboard/performance-report/export?format=${format}&period=${period}`, '_blank');
}

function renderReportData(data) {
  const { fmtNTD, fmtPct, fmtFloat, agentNameEsm, regimeLabelEsm } = window;

  document.getElementById('prDateRange').textContent = `${data.start_date || '--'} to ${data.end_date || '--'}`;

  const kpis = [
    { label: 'Total Return', value: fmtPct ? fmtPct(data.total_return || 0) : `${((data.total_return||0)*100).toFixed(2)}%`, sign: data.total_return },
    { label: 'Annualized', value: fmtPct ? fmtPct(data.annualized_return || 0) : `${((data.annualized_return||0)*100).toFixed(2)}%`, sign: data.annualized_return },
    { label: 'Sharpe Ratio', value: fmtFloat ? fmtFloat(data.sharpe_ratio || 0, 2) : (data.sharpe_ratio||0).toFixed(2) },
    { label: 'Max Drawdown', value: fmtPct ? fmtPct(data.max_drawdown || 0) : `${((data.max_drawdown||0)*100).toFixed(2)}%`, sign: -1 },
    { label: 'After-Tax Value', value: fmtNTD ? fmtNTD(data.after_tax_value || 0) : (data.after_tax_value||0).toFixed(0) },
    { label: 'Total Tax Paid', value: fmtNTD ? fmtNTD(data.total_tax_paid || 0) : (data.total_tax_paid||0).toFixed(0), hint: 'Cumulative' },
    { label: 'Win Rate', value: fmtPct ? fmtPct(data.win_rate || 0) : `${((data.win_rate||0)*100).toFixed(1)}%` },
    { label: 'Total Trades', value: data.total_trades || 0 }
  ];

  const gridHtml = kpis.map(k => `
    <div class="pr-card">
      <div class="pr-card-label">${k.label}</div>
      <div class="pr-card-value ${k.sign > 0 ? 'positive' : (k.sign < 0 ? 'negative' : '')}">${k.value}</div>
      ${k.hint ? `<div class="pr-card-hint">${k.hint}</div>` : ''}
    </div>
  `).join('');
  
  document.getElementById('prKpiGrid').innerHTML = gridHtml;

  const agentsBody = document.getElementById('prAgentsBody');
  if (data.top_agents && data.top_agents.length > 0) {
    agentsBody.innerHTML = data.top_agents.map(a => `
      <tr>
        <td>${agentNameEsm ? agentNameEsm(a.id) : a.id}</td>
        <td class="${a.contribution > 0 ? 'positive' : (a.contribution < 0 ? 'negative' : '')}" style="color: ${a.contribution > 0 ? 'var(--up)' : 'var(--down)'}">
          ${fmtPct ? fmtPct(a.contribution) : `${(a.contribution*100).toFixed(2)}%`}
        </td>
        <td>${fmtPct ? fmtPct(a.win_rate) : `${(a.win_rate*100).toFixed(1)}%`}</td>
        <td>${fmtFloat ? fmtFloat(a.sharpe, 2) : (a.sharpe||0).toFixed(2)}</td>
      </tr>
    `).join('');
  } else {
    agentsBody.innerHTML = '<tr><td colspan="4" class="pr-loading">No agent data available</td></tr>';
  }

  const regimesBody = document.getElementById('prRegimesBody');
  const regimes = Object.entries(data.regime_breakdown || {});
  if (regimes.length > 0) {
    regimesBody.innerHTML = regimes.map(([id, r]) => `
      <tr>
        <td>${regimeLabelEsm ? regimeLabelEsm(id) : id}</td>
        <td>${r.sessions || 0}</td>
        <td style="color: ${(r.avg_return || 0) > 0 ? 'var(--up)' : 'var(--down)'}">
          ${fmtPct ? fmtPct(r.avg_return || 0) : `${((r.avg_return||0)*100).toFixed(2)}%`}
        </td>
        <td>${fmtPct ? fmtPct(r.win_rate || 0) : `${((r.win_rate||0)*100).toFixed(1)}%`}</td>
      </tr>
    `).join('');
  } else {
    regimesBody.innerHTML = '<tr><td colspan="4" class="pr-loading">No regime data available</td></tr>';
  }

  // Monthly Returns
  const monthsBody = document.getElementById('prMonthsBody');
  if (data.monthly_returns && data.monthly_returns.length > 0) {
    monthsBody.innerHTML = data.monthly_returns.map(m => `
      <tr>
        <td>${m.month || '--'}</td>
        <td style="color: ${(m.return || 0) > 0 ? 'var(--up)' : 'var(--down)'}">
          ${fmtPct ? fmtPct(m.return || 0) : `${((m.return||0)*100).toFixed(2)}%`}
        </td>
        <td>${fmtPct ? fmtPct(m.cumulative || 0) : `${((m.cumulative||0)*100).toFixed(2)}%`}</td>
      </tr>
    `).join('');
  } else {
    monthsBody.innerHTML = '<tr><td colspan="3" class="pr-loading">No monthly data available</td></tr>';
  }
}
