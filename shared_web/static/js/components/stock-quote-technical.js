import { renderEmptyState, renderSkeleton } from '../shared/app-utils.js';

export function renderTechnical(state, techResult) {
  if (state === 'loading') {
    return `<div>${renderSkeleton(3)}</div>`;
  }
  if (state === 'error' || techResult.status === 'error') {
    return `<div class="sq-error-box">技術指標暫時無法取得</div>`;
  }
  if (!techResult.data) {
    return renderEmptyState('無技術指標資料');
  }

  const data = techResult.data;
  const sma20 = data.sma20 || 0;
  const sma50 = data.sma50 || 0;
  const rsi = data.rsi14 || data.rsi || 0;

  let smaSignal = '無訊號';
  let smaColor = 'sq-price-neutral';
  if (sma20 > sma50) {
    smaSignal = '短期偏多';
    smaColor = 'sq-price-up';
  } else if (sma20 < sma50) {
    smaSignal = '短期偏空';
    smaColor = 'sq-price-down';
  }

  let rsiZone = '中性區';
  let rsiColor = 'sq-price-neutral';
  if (rsi >= 70) {
    rsiZone = '超買區';
    rsiColor = 'risk-badge risk-high'; // Using risk-high for warning, not red
  } else if (rsi <= 30) {
    rsiZone = '超賣區';
    rsiColor = 'risk-badge risk-low';
  }

  const today = new Date().toISOString().slice(0, 10);

  return `
    <div class="card sq-full-width">
      <div class="card-header" style="display:flex;justify-content:space-between;align-items:center;">
        <h3 class="card-title">技術指標 (90 日)</h3>
        <span style="font-size:var(--text-xs);color:var(--text-tertiary)">計算基準: ${today} 收盤</span>
      </div>
      <div class="sq-tech-cards">
        <div class="metric-card">
          <div class="metric-card__label">SMA20</div>
          <div class="metric-card__value">${sma20.toFixed(2)}</div>
          <div class="${smaColor}" style="margin-top:var(--spacing-1);font-size:var(--text-sm)">${smaSignal}</div>
        </div>
        <div class="metric-card">
          <div class="metric-card__label">SMA50</div>
          <div class="metric-card__value">${sma50.toFixed(2)}</div>
          <div class="${smaColor}" style="margin-top:var(--spacing-1);font-size:var(--text-sm)">${sma20 > sma50 ? '中期偏多' : '中期偏空'}</div>
        </div>
        <div class="metric-card">
          <div class="metric-card__label">RSI14</div>
          <div class="metric-card__value">${rsi.toFixed(2)}</div>
          <div style="margin-top:var(--spacing-1);font-size:var(--text-sm)"><span class="${rsiColor}">${rsiZone}</span></div>
        </div>
      </div>
      <div style="margin-top:var(--spacing-6);padding:var(--spacing-4);background:var(--bg-tertiary);border-radius:var(--radius-sm);text-align:center;color:var(--text-secondary)">
        [歷史走勢 sparkline — 需後端擴充 API, 暫以 quote 近似]
      </div>
    </div>
  `;
}
