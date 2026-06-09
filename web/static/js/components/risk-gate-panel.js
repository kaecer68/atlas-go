// Risk Gate Panel — 風控閘道狀態顯示（操作面板）
// 與 risk-panel.js（分析展示）分工明確

export function renderRiskGatePanel(container, getJSON) {
  container.innerHTML = '<div class="loading">載入風控狀態…</div>';

  getJSON('/api/dashboard/risk')
    .then(data => {
      if (!data || data.message) {
        container.innerHTML = '<div class="empty-state">尚無風險數據</div>';
        return;
      }

      const var95 = data.var_95 ?? '-';
      const drawdown = data.max_drawdown ?? '-';
      const concentration = data.concentration_score ?? '-';
      const positions = data.position_count ?? 0;

      container.innerHTML = `
        <div class="risk-gate-grid" style="display:grid;grid-template-columns:1fr 1fr;gap:12px;padding:12px">
          <div class="risk-gate-metric">
            <span class="metric-label">VaR 95%</span>
            <span class="metric-value ${var95 < -0.05 ? 'critical' : 'normal'}">${typeof var95 === 'number' ? (var95 * 100).toFixed(1) + '%' : var95}</span>
          </div>
          <div class="risk-gate-metric">
            <span class="metric-label">最大回撤</span>
            <span class="metric-value ${drawdown > 0.15 ? 'critical' : drawdown > 0.08 ? 'warning' : 'normal'}">${typeof drawdown === 'number' ? (drawdown * 100).toFixed(1) + '%' : drawdown}</span>
          </div>
          <div class="risk-gate-metric">
            <span class="metric-label">集中度 (HHI)</span>
            <span class="metric-value ${concentration > 0.25 ? 'warning' : 'normal'}">${typeof concentration === 'number' ? (concentration * 100).toFixed(1) + '%' : concentration}</span>
          </div>
          <div class="risk-gate-metric">
            <span class="metric-label">持倉數</span>
            <span class="metric-value">${positions}</span>
          </div>
        </div>
        <div style="padding:8px 12px;font-size:12px;color:var(--muted);border-top:1px solid var(--border)">
          風控閘道模式：<span class="risk-gate-mode">NORMAL</span>
        </div>
      `;
    })
    .catch(() => {
      container.innerHTML = '<div class="error">無法載入風控數據</div>';
    });
}
