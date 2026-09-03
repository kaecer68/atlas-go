import { fmtSafePct, fmtSafeNumber } from '../shared/format-metric.js';

// B1 (risk-console Phase 1)：權益曲線 186 列無分頁 → 每頁 20 列分頁
// （沿用現成 table-pagination.css 樣式）；指數點位（基期 100）不再進
// fmtSafePct（會被 ×100 誤顯示成 12039.0%）。
const PAGE_SIZE = 20;

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
  const totalItems = curve.length;
  const totalPages = Math.max(1, Math.ceil(totalItems / PAGE_SIZE));
  // 單元測試環境（node）沒有 window/document — 分頁互動只在瀏覽器啟用。
  const hasBrowser = typeof window !== 'undefined' && typeof document !== 'undefined';

  // 指數點位格式（基期 100 的原值，不進 percent 格式化）
  const fmtIndexPoint = (v) => fmtSafeNumber(v, { decimals: 2, useGrouping: true });

  const state = hasBrowser && window._benchmarkCurveState ? window._benchmarkCurveState : { page: 0 };
  if (hasBrowser) window._benchmarkCurveState = state;

  function pageRows() {
    const page = Math.max(0, Math.min(state.page, totalPages - 1));
    const startIdx = page * PAGE_SIZE;
    const endIdx = Math.min(startIdx + PAGE_SIZE, totalItems);
    return { page, startIdx, endIdx, slice: curve.slice(startIdx, endIdx) };
  }

  function renderTable() {
    const { page, startIdx, endIdx, slice } = pageRows();
    const rowsHtml = slice.map(p => {
      const outCls = signedCls(p.outperf);
      return `<tr><td>${p.label}</td><td style="text-align:right">${fmtIndexPoint(p.portfolio)}</td><td style="text-align:right">${fmtIndexPoint(p.benchmark)}</td><td style="text-align:right" class="${outCls}">${fmtIndexPoint(p.outperf)}</td></tr>`;
    }).join('');

    const pagination = totalItems > PAGE_SIZE ? `
      <div class="table-pagination" style="margin-top:10px">
        <span>顯示 <strong>${startIdx + 1}-${endIdx}</strong> / 共 <strong>${totalItems}</strong> 筆</span>
        <div style="display:flex;gap:6px;align-items:center">
          <button onclick="window._benchmarkCurveState.page=0;renderBenchmarkTable()" ${page === 0 ? 'disabled' : ''}>« 首頁</button>
          <button onclick="window._benchmarkCurveState.page=${page - 1};renderBenchmarkTable()" ${page === 0 ? 'disabled' : ''}>‹ 上一頁</button>
          <span style="padding:0 8px">第 ${page + 1} / ${totalPages} 頁</span>
          <button onclick="window._benchmarkCurveState.page=${page + 1};renderBenchmarkTable()" ${page >= totalPages - 1 ? 'disabled' : ''}>下一頁 ›</button>
          <button onclick="window._benchmarkCurveState.page=${totalPages - 1};renderBenchmarkTable()" ${page >= totalPages - 1 ? 'disabled' : ''}>末頁 »</button>
        </div>
      </div>
    ` : '';

    const tableEl = hasBrowser ? document.getElementById('benchmarkCurveTable') : null;
    if (tableEl) {
      tableEl.innerHTML = `
        <div class="table-wrapper">
          <table class="text-sm">
            <thead><tr><th>日期</th><th style="text-align:right">投組指數</th><th style="text-align:right">TAIEX 指數</th><th style="text-align:right">差額(點)</th></tr></thead>
            <tbody>${rowsHtml}</tbody>
          </table>
        </div>
        ${pagination}
      `;
    }
  }

  container.innerHTML = `
    <div class="panel-content">
      <div class="section-title">基準比較指標</div>
      <div class="kpi-grid" style="grid-template-columns:repeat(4,1fr)">${kpiCards}</div>
      <div class="section-title" style="margin-top:16px">權益曲線：投組 vs TAIEX（指數點位 · 基期 = 100）</div>
      <div id="benchmarkCurveTable"></div>
    </div>`;

  // 全域點擊代理：分頁按鈕不需重新 fetch API，只重繪表格本體。
  if (hasBrowser) window.renderBenchmarkTable = renderTable;
  renderTable();
}
