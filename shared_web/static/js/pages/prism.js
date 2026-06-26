// PRISM 訓練結果頁面 — Regime-Specific Training Results
// 顯示 PRISMManager.GetCompletedResults() 回傳的 CompletedTrainingResult 陣列
import { escapeHtml } from '../shared/utils.js';

const REGIME_COLORS = {
  RISK_ON: 'var(--color-success)',
  RISK_OFF: 'var(--color-danger)',
  NEUTRAL: 'var(--color-warning)',
};
const REGIME_DEFAULT_COLOR = 'var(--muted)';

function fmtPct(v) {
  return (typeof v === 'number' && !isNaN(v)) ? (v * 100).toFixed(2) + '%' : '—';
}
function fmtNum(v, digits) {
  return (typeof v === 'number' && !isNaN(v)) ? v.toFixed(digits != null ? digits : 2) : '—';
}

function renderRow(r, idx) {
  const res = r.result || {};
  const regime = r.regime || 'UNKNOWN';
  const regimeColor = REGIME_COLORS[regime] || REGIME_DEFAULT_COLOR;
  const hasError = !!res.error;
  const winRatio = (res.signals_count > 0)
    ? ((res.win_count / res.signals_count) * 100).toFixed(1) + '%'
    : '—';
  const explanation = res.explanation
    ? escapeHtml(res.explanation)
    : '<span style="color:var(--muted)">（無說明）</span>';

  return `
    <tr style="border-bottom:1px solid var(--border)">
      <td style="padding:8px;font-family:monospace;font-size:12px">${escapeHtml(r.agent_id || '—')}</td>
      <td style="padding:8px;font-size:12px">${escapeHtml(r.agent_skill || '—')}</td>
      <td style="padding:8px;text-align:center">
        <span style="padding:2px 8px;border-radius:3px;font-size:11px;font-weight:700;background:color-mix(in srgb, ${regimeColor} 15%, transparent);color:${regimeColor}">${escapeHtml(regime)}</span>
      </td>
      <td style="padding:8px;text-align:right;font-size:12px">${fmtPct(res.hit_rate)}</td>
      <td style="padding:8px;text-align:right;font-size:12px">${fmtNum(res.sharpe_ratio)}</td>
      <td style="padding:8px;text-align:right;font-size:12px;color:var(--down)">${fmtPct(res.max_drawdown)}</td>
      <td style="padding:8px;text-align:right;font-size:12px">${fmtPct(res.total_return)}</td>
      <td style="padding:8px;text-align:right;font-size:12px">${res.win_count || 0}/${res.signals_count || 0} (${winRatio})</td>
      <td style="padding:8px;text-align:right;font-size:12px;color:${hasError ? 'var(--color-danger)' : 'var(--muted)'}">${escapeHtml(res.error || (res.duration || '—'))}</td>
    </tr>
    <tr>
      <td colspan="9" style="padding:4px 8px 12px 8px;background:var(--panel-l2);font-size:12px;color:var(--text);line-height:1.5">
        <details>
          <summary style="cursor:pointer;color:var(--muted)">📝 訓練說明（Explanation）</summary>
          <div style="margin-top:6px;padding:6px 10px;border-left:3px solid ${regimeColor};background:var(--bg-elev)">${explanation}</div>
        </details>
      </td>
    </tr>
  `;
}

export function renderTrainingResults(containerOrData) {
  const el = typeof containerOrData === 'string'
    ? document.getElementById(containerOrData)
    : (containerOrData || document.getElementById('prismContent'));
  if (!el) return;

  // Direct render mode (data passed in)
  if (containerOrData && typeof containerOrData !== 'string' && !Array.isArray(containerOrData) && containerOrData.tagName) {
    // element passed, need to fetch
  }
}

export async function loadPrismData() {
  const el = document.getElementById('prismContent');
  if (!el) return;
  el.classList.remove('loading');

  let data = null;
  try {
    const resp = await fetch('/api/prism/training-results');
    if (!resp.ok) {
      el.innerHTML = '<div class="empty">PRISM 訓練結果取得失敗 (' + resp.status + ')</div>';
      return;
    }
    data = await resp.json();
  } catch (err) {
    console.error('[prism] fetch failed', err);
    el.innerHTML = '<div class="empty">PRISM 訓練結果連線錯誤</div>';
    return;
  }

  if (!Array.isArray(data) || data.length === 0) {
    el.innerHTML = '<div class="empty">尚無 PRISM 訓練結果（等待 regime-specific 訓練完成）</div>';
    return;
  }

  const rows = data.map(renderRow).join('');
  el.innerHTML = `
    <div class="panel wide" style="margin-bottom:12px">
      <div style="font-size:13px;color:var(--muted)">共 <strong>${data.length}</strong> 筆 regime-specific 訓練結果</div>
    </div>
    <div class="panel wide" style="overflow-x:auto">
      <table style="width:100%;border-collapse:collapse;font-size:12px">
        <thead>
          <tr style="background:var(--panel-l2);border-bottom:2px solid var(--border)">
            <th style="padding:8px;text-align:left">Agent</th>
            <th style="padding:8px;text-align:left">Skill</th>
            <th style="padding:8px;text-align:center">Regime</th>
            <th style="padding:8px;text-align:right">Hit Rate</th>
            <th style="padding:8px;text-align:right">Sharpe</th>
            <th style="padding:8px;text-align:right">Max DD</th>
            <th style="padding:8px;text-align:right">Total Return</th>
            <th style="padding:8px;text-align:right">Wins/Signals</th>
            <th style="padding:8px;text-align:right">Error / Duration</th>
          </tr>
        </thead>
        <tbody>${rows}</tbody>
      </table>
    </div>
  `;
}
