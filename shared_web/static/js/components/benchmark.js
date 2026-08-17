import { fmtSafePct, fmtSafeNumber } from '../shared/format-metric.js';

export async function renderBenchmarkComparison(container, getJSON) {
  // 失敗已由 app-utils choke point 回報 (reportDegraded)，此處 .catch 僅為 null 預設值。
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
    { label: '投組累積報酬', value: fmtSafePct(data.portfolio_return) },
    { label: 'TAIEX 報酬', value: fmtSafePct(data.taiex_return) },
    { label: '超額報酬', value: fmtSafePct(data.outperformance), cls: signedCls(data.outperformance) },
    { label: 'Alpha', value: fmtSafePct(data.alpha), cls: signedCls(data.alpha) },
    { label: 'Beta', value: fmtSafeNumber(data.beta, { decimals: 2, useGrouping: true }) },
    { label: 'Tracking Error', value: fmtSafePct(data.tracking_error) },
    { label: 'Sharpe Ratio', value: fmtSafeNumber(data.sharpe_ratio, { decimals: 2, useGrouping: true }) },
    { label: 'Info Ratio', value: fmtSafeNumber(data.info_ratio, { decimals: 2, useGrouping: true }) },
  ];

  const kpiCards = kpis.map(k =>
    `<div class="kpi-card"><div class="kpi-label">${k.label}</div><div class="kpi-value ${k.cls || ''}">${k.value}</div></div>`
  ).join('');

  const curve = data.equity_curve || [];
  const curveRows = curve.map(p => {
    const outCls = signedCls(p.outperf);
    return `<tr><td>${p.label}</td><td style="text-align:right">${fmtSafePct(p.portfolio)}</td><td style="text-align:right">${fmtSafePct(p.benchmark)}</td><td style="text-align:right" class="${outCls}">${fmtSafePct(p.outperf)}</td></tr>`;
  }).join('');

  container.innerHTML = `
    <div class="panel-content">
      <div class="section-title">基準比較指標</div>
      <div class="kpi-grid" style="grid-template-columns:repeat(4,1fr)">${kpiCards}</div>
      <div class="section-title" style="margin-top:16px">權益曲線：投組 vs TAIEX</div>
      <div class="table-wrapper"><table class="text-sm"><thead><tr><th>日期</th><th>投組</th><th>TAIEX</th><th>差額</th></tr></thead><tbody>${curveRows}</tbody></table></div>
    </div>`;
}