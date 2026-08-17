import { fmtInt } from '../shared/utils.js';
import { fmtSafeNumber, fmtSafePct, fmtSafeDrawdown } from '../shared/format-metric.js';

export async function renderRiskPanel(container, getJSON) {
  // 1. 讀取 portfolio-state 取得 current_drawdown, concentration_ratio
  // 失敗已由 app-utils choke point 回報 (reportDegraded)，此處 .catch 僅為 null 預設值。
  const state = await getJSON('/api/dashboard/portfolio-state').catch(() => ({}));

  // 2. 讀取 correlation-matrix (if available)
  const corr = await getJSON('/api/dashboard/correlation-matrix').catch(() => null);

  const dd = state.max_drawdown;
  const conc = state.concentration_ratio;

  let leverage = null;
  if (
    Number.isFinite(state.portfolio_value) &&
    Number.isFinite(state.cash) &&
    state.cash !== 0
  ) {
    leverage = (state.portfolio_value - state.cash) / state.cash;
  }

  let matrixHtml = '';
  if (corr && corr.matrix && corr.matrix.length > 0) {
    const labels = corr.labels || corr.symbols || [];
    const header = '<th></th>' + labels.map(l => `<th class="corr-header">${l}</th>`).join('');
    const rows = corr.matrix.map((row, i) => {
      const cells = row.map((v, j) => {
        let color = 'inherit';
        if (typeof v !== 'number') {
          return '<td class="corr-cell">—</td>';
        }
        if (i === j) color = 'var(--muted)';
        else if (v > 0.7) color = 'var(--color-danger)';
        else if (v > 0.4) color = 'var(--warn)';
        else if (v < 0) color = 'var(--color-success)';
        return `<td class="corr-cell" style="color:${color}">${fmtSafeNumber(v, { decimals: 2 })}</td>`;
      }).join('');
      return `<tr><td class="corr-header">${labels[i]}</td>${cells}</tr>`;
    }).join('');
    matrixHtml = `<div class="section-title">相關性矩陣</div><div class="corr-matrix-container"><table class="corr-matrix"><thead><tr>${header}</tr></thead><tbody>${rows}</tbody></table></div>`;
  }

  container.innerHTML = `
    <div class="panel-content">
      <div class="section-title">風險指標</div>
      <div class="kpi-grid" style="grid-template-columns:repeat(4,1fr)">
        <div class="kpi-card"><div class="kpi-label">最大回撤</div><div class="kpi-value" style="color:var(--color-danger)">${fmtSafeDrawdown(dd)}</div></div>
        <div class="kpi-card"><div class="kpi-label">持倉集中度</div><div class="kpi-value">${fmtSafePct(conc)}</div><div class="kpi-hint">HHI 指數</div></div>
        <div class="kpi-card"><div class="kpi-label">部位數</div><div class="kpi-value">${fmtInt(state.positions_count)}</div></div>
        <div class="kpi-card"><div class="kpi-label">槓桿率</div><div class="kpi-value">${fmtSafePct(leverage)}</div></div>
      </div>
      ${matrixHtml}
    </div>`;
}
