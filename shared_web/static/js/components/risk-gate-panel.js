// Risk Gate Panel — 風控閘道狀態顯示（操作面板）
// 與 risk-panel.js（分析展示）分工明確

import { fmtInt } from '../shared/utils.js';
import { fmtSafePct, fmtSafeDrawdown } from '../shared/format-metric.js';

export function renderRiskGatePanel(container, getJSON) {
  container.innerHTML = '<div class="loading">載入風控狀態…</div>';

  // 失敗已由 app-utils choke point 回報 (reportDegraded)，此處 .catch 僅為 null 預設值。
  return Promise.all([
    getJSON('/api/dashboard/risk').catch(() => null),
    getJSON('/api/dashboard/risk-exposure').catch(() => null),
  ])
    .then(([risk, exposure]) => {
      if (!risk || risk.message || !risk.risk_snapshot) {
        container.innerHTML = '<div class="empty-state">尚無風險數據</div>';
        return;
      }

      const snap = risk.risk_snapshot || {};
      // B9 (risk-console Phase 1)：與 live 風險指標的 252 交易日觀察期門檻對齊 —
      // insufficient_data / var 未達觀察門檻時顯示「觀察期中」，不再照顯 -32.1% 原值。
      const varObserving = !!(exposure
        ? (exposure.insufficient_data || exposure.var_available === false)
        : (snap.insufficient_data === 1 || (typeof snap.data_points === 'number' && snap.data_points < 252)));
      const var95 = varObserving ? null : fmtSafePct(snap.var_95);
      const drawdown = fmtSafeDrawdown(snap.max_drawdown_pct);
      const positions = fmtInt(exposure && exposure.position_count);
      const gateMode = typeof risk.gate_mode === 'string' && risk.gate_mode ? risk.gate_mode : '—';
      const gateModeClass =
        gateMode === '—'
          ? 'risk-gate-mode'
          : `risk-gate-mode risk-gate-mode--${gateMode.toLowerCase()}`;

      const ddNum = snap.max_drawdown_pct;
      const ddCls =
        typeof ddNum === 'number'
          ? ddNum > 0.15
            ? 'critical'
            : ddNum > 0.08
            ? 'warning'
            : 'normal'
          : 'normal';
      const varCls = varObserving ? 'observing' : 'normal';
      const varHtml = varObserving
        ? '<span class="metric-value observing" style="color:var(--status-unknown)">觀察期中</span>'
        : `<span class="metric-value ${varCls}">${var95}</span>`;

      container.innerHTML = `
        <div class="risk-gate-grid" style="display:grid;grid-template-columns:1fr 1fr;gap:12px;padding:12px">
          <div class="risk-gate-metric">
            <span class="metric-label">VaR 95%</span>
            ${varHtml}
          </div>
          <div class="risk-gate-metric">
            <span class="metric-label">最大回撤</span>
            <span class="metric-value ${ddCls}">${drawdown}</span>
          </div>
          <div class="risk-gate-metric">
            <span class="metric-label">持倉數</span>
            <span class="metric-value">${positions}</span>
          </div>
          <div class="risk-gate-metric">
            <span class="metric-label">風控閘道模式</span>
            <span class="${gateModeClass}">${gateMode}</span>
          </div>
        </div>
      `;
    })
    .catch(() => {
      container.innerHTML = '<div class="error">無法載入風險數據</div>';
    });
}
