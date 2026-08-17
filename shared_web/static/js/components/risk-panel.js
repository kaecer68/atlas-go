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

  // 持倉相關性矩陣只在有持倉時才有意義：沒持倉時不顯示 20×20 矩陣
  // （避免「我沒有持倉，為什麼有相關性矩陣？」的邏輯矛盾），改為引導文案 + CTA。
  const hasPositions = Number.isFinite(state.positions_count)
    ? state.positions_count > 0
    : Array.isArray(state.positions) && state.positions.length > 0;

  let matrixHtml = '';
  if (!hasPositions) {
    matrixHtml = `
      <div class="section-title">相關性矩陣</div>
      <div class="empty-state-guidance">
        <div class="icon">📊</div>
        <div class="title">建立持倉後解鎖持倉相關性分析</div>
        <div class="desc">相關性矩陣分析你持倉之間的連動關係；建立持倉後即可查看。</div>
        <div class="empty-actions">
          <a class="btn btn--primary btn-sm" data-page="strategies" href="/client/strategies">前往投資心法</a>
        </div>
      </div>`;
  } else if (corr && corr.matrix && corr.matrix.length > 0) {
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
