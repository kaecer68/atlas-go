export async function renderBenchmarkComparison(container, getJSON) {
  const data = await getJSON('/api/dashboard/benchmark-comparison').catch(() => null);
  if (!data || data.session_count < 1) {
    container.innerHTML = '<div style="padding:20px;text-align:center;color:var(--muted)">暫無基準比較資料</div>';
    return;
  }

  const fmtNTD = window.fmtNTD || (v => v.toFixed(0));
  const fmtP = window.fmtPct || (v => (v * 100).toFixed(2) + '%');
  const fmtF = window.fmtFloat || (v => v.toFixed(3));

  const kpis = [
    { label: '投組累積報酬', value: fmtP(data.portfolio_return || 0) },
    { label: 'TAIEX 報酬', value: fmtP(data.taiex_return || 0) },
    { label: '超額報酬', value: fmtP(data.outperformance || 0), cls: data.outperformance > 0 ? 'text-up' : 'text-down' },
    { label: 'Alpha', value: fmtP(data.alpha || 0), cls: data.alpha > 0 ? 'text-up' : 'text-down' },
    { label: 'Beta', value: fmtF(data.beta || 0) },
    { label: 'Tracking Error', value: fmtP(data.tracking_error || 0) },
    { label: 'Sharpe Ratio', value: data.sharpe_ratio == null ? 'N/A' : fmtF(data.sharpe_ratio) },
    { label: 'Info Ratio', value: fmtF(data.info_ratio || 0) },
  ];

  const kpiCards = kpis.map(k =>
    `<div class="kpi-card"><div class="kpi-label">${k.label}</div><div class="kpi-value ${k.cls || ''}">${k.value}</div></div>`
  ).join('');

  const curve = data.equity_curve || [];
  const curveRows = curve.map(p => {
    const outCls = (p.outperf || 0) > 0 ? 'text-up' : 'text-down';
    return `<tr><td>${p.label}</td><td style="text-align:right">${fmtP(p.portfolio || 0)}</td><td style="text-align:right">${fmtP(p.benchmark || 0)}</td><td style="text-align:right" class="${outCls}">${(p.outperf > 0 ? '+' : '')}${fmtP(p.outperf || 0)}</td></tr>`;
  }).join('');

  container.innerHTML = `
    <div class="panel-content">
      <div class="section-title">基準比較指標</div>
      <div class="kpi-grid" style="grid-template-columns:repeat(4,1fr)">${kpiCards}</div>
      <div class="section-title" style="margin-top:16px">權益曲線：投組 vs TAIEX</div>
      <div class="table-wrapper"><table class="text-sm"><thead><tr><th>日期</th><th>投組</th><th>TAIEX</th><th>差額</th></tr></thead><tbody>${curveRows}</tbody></table></div>
    </div>`;
}