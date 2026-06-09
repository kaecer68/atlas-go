export async function renderRiskPanel(container, getJSON) {
  // 1. 讀取 portfolio-state 取得 current_drawdown, concentration_ratio
  const state = await getJSON('/api/dashboard/portfolio-state').catch(() => ({}));

  // 2. 讀取 correlation-matrix (if available)
  const corr = await getJSON('/api/dashboard/correlation-matrix').catch(() => null);

  const fmtP = window.fmtPct || (v => (v * 100).toFixed(2) + '%');
  const dd = state.current_drawdown || 0;
  const conc = state.concentration_ratio || 0;

  let matrixHtml = '';
  if (corr && corr.matrix && corr.matrix.length > 0) {
    const labels = corr.labels || corr.symbols || [];
    const header = '<th></th>' + labels.map(l => `<th class="corr-header">${l}</th>`).join('');
    const rows = corr.matrix.map((row, i) => {
      const cells = row.map((v, j) => {
        let color = 'inherit';
        if (i === j) color = 'var(--muted)';
        else if (v > 0.7) color = 'var(--color-danger)';
        else if (v > 0.4) color = 'var(--warn)';
        else if (v < 0) color = 'var(--color-success)';
        return `<td class="corr-cell" style="color:${color}">${v.toFixed(2)}</td>`;
      }).join('');
      return `<tr><td class="corr-header">${labels[i]}</td>${cells}</tr>`;
    }).join('');
    matrixHtml = `<div class="section-title">相關性矩陣</div><div class="corr-matrix-container"><table class="corr-matrix"><thead><tr>${header}</tr></thead><tbody>${rows}</tbody></table></div>`;
  }

  container.innerHTML = `
    <div class="panel-content">
      <div class="section-title">風險指標</div>
      <div class="kpi-grid" style="grid-template-columns:repeat(4,1fr)">
        <div class="kpi-card"><div class="kpi-label">最大回撤</div><div class="kpi-value" style="color:var(--color-danger)">${fmtP(dd)}</div></div>
        <div class="kpi-card"><div class="kpi-label">持倉集中度</div><div class="kpi-value">${fmtP(conc)}</div><div class="kpi-hint">HHI 指數</div></div>
        <div class="kpi-card"><div class="kpi-label">部位數</div><div class="kpi-value">${state.positions_count || 0}</div></div>
        <div class="kpi-card"><div class="kpi-label">槓桿率</div><div class="kpi-value">${fmtP((state.portfolio_value || 0) > 0 ? (state.portfolio_value - state.cash) / state.cash : 0)}</div></div>
      </div>
      ${matrixHtml}
    </div>`;
}
