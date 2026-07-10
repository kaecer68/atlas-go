import { fmtPct, fmtFloat } from '../shared/utils.js';

export async function renderBenchmarkComparison(container, getJSON) {
  const data = await getJSON('/api/dashboard/benchmark-comparison').catch(() => null);
  if (!data || data.session_count < 1) {
    container.innerHTML = '<div style="padding:20px;text-align:center;color:var(--muted)">暫無基準比較資料</div>';
    return;
  }

  const signedCls = (v) => {
    if (typeof v !== 'number') return '';
    return v > 0 ? 'text-up' : v < 0 ? 'text-down' : '';
  };

  const kpis = [
    { label: '投組累積報酬', value: fmtPct(data.portfolio_return) },
    { label: 'TAIEX 報酬', value: fmtPct(data.taiex_return) },
    { label: '超額報酬', value: fmtPct(data.outperformance), cls: signedCls(data.outperformance) },
    { label: 'Alpha', value: fmtPct(data.alpha), cls: signedCls(data.alpha) },
    { label: 'Beta', value: fmtFloat(data.beta) },
    { label: 'Tracking Error', value: fmtPct(data.tracking_error) },
    { label: 'Sharpe Ratio', value: fmtFloat(data.sharpe_ratio) },
    { label: 'Info Ratio', value: fmtFloat(data.info_ratio) },
  ];

  const kpiCards = kpis.map(k =>
    `<div class="kpi-card"><div class="kpi-label">${k.label}</div><div class="kpi-value ${k.cls || ''}">${k.value}</div></div>`
  ).join('');

  const curve = data.equity_curve || [];
  const curveRows = curve.map(p => {
    const outCls = signedCls(p.outperf);
    return `<tr><td>${p.label}</td><td style="text-align:right">${fmtPct(p.portfolio)}</td><td style="text-align:right">${fmtPct(p.benchmark)}</td><td style="text-align:right" class="${outCls}">${fmtPct(p.outperf)}</td></tr>`;
  }).join('');

  container.innerHTML = `
    <div class="panel-content">
      <div class="section-title">基準比較指標</div>
      <div class="kpi-grid" style="grid-template-columns:repeat(4,1fr)">${kpiCards}</div>
      <div class="section-title" style="margin-top:16px">權益曲線：投組 vs TAIEX</div>
      <div class="table-wrapper"><table class="text-sm"><thead><tr><th>日期</th><th>投組</th><th>TAIEX</th><th>差額</th></tr></thead><tbody>${curveRows}</tbody></table></div>
    </div>`;
}