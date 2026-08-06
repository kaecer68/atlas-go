import { renderEmptyState, renderSkeleton } from '../shared/app-utils.js';
import { fmtSafeNumber } from '../shared/format-metric.js';

function isValidNumber(v) {
  return typeof v === 'number' && Number.isFinite(v);
}

export function renderTechnical(state, techResult, coverage) {
  if (state === 'loading') {
    return `<div class="sq-card"><h3 class="sq-card__title">技術指標（90 日）</h3>${renderSkeleton(3)}</div>`;
  }
  // Out-of-scope: render neutral scope notice instead of misleading
  // "資料尚未回填" copy that confuses users into thinking the system
  // is broken. See docs/manifests/2026-08-06-stock-coverage-notice.md.
  if (coverage && !coverage.covered) {
    return `<div class="sq-card"><h3 class="sq-card__title">技術指標（90 日）</h3>
      <div class="sq-scope-notice">此股票代號不在本系統技術指標涵蓋範圍（涵蓋 TWSE 上市普通股，約 1070 隻）<br><small>${escapeHtml(coverage.reason || '')}</small></div>
    </div>`;
  }
  if (state === 'error' || techResult.status === 'error') {
    const isDataMissing = techResult.error && /insufficient historical quote data/i.test(techResult.error);
    const message = isDataMissing
      ? '歷史股價資料尚未回填，無法計算技術指標。'
      : '技術指標暫時無法取得';
    return `<div class="sq-error-box">${message}</div>`;
  }
  if (!techResult.data) {
    return renderEmptyState('無技術指標資料');
  }

  const data = techResult.data;
  const sma20 = isValidNumber(data.sma20) ? data.sma20 : null;
  const sma50 = isValidNumber(data.sma50) ? data.sma50 : null;
  const rsi = isValidNumber(data.rsi14) ? data.rsi14 : isValidNumber(data.rsi) ? data.rsi : null;

  let smaSignal = '無訊號';
  let smaColor = 'sq-price-neutral';
  if (sma20 !== null && sma50 !== null) {
    if (sma20 > sma50) {
      smaSignal = '短期偏多';
      smaColor = 'sq-price-up';
    } else if (sma20 < sma50) {
      smaSignal = '短期偏空';
      smaColor = 'sq-price-down';
    }
  }

  let rsiZone = '—';
  let rsiColor = 'sq-price-neutral';
  if (rsi !== null) {
    rsiZone = '中性區';
    if (rsi >= 70) {
      rsiZone = '超買區';
      rsiColor = 'risk-badge risk-high';
    } else if (rsi <= 30) {
      rsiZone = '超賣區';
      rsiColor = 'risk-badge risk-low';
    }
  }

  const today = new Date().toISOString().slice(0, 10);

  return `
    <div class="sq-card">
      <h3 class="sq-card__title">技術指標（90 日）</h3>
      <div class="sq-tech-list">
        <div class="sq-tech-row">
          <div class="sq-tech-row__label">SMA20</div>
          <div class="sq-tech-row__value">${fmtSafeNumber(sma20, { decimals: 2 })}</div>
          <div class="sq-tech-row__hint ${smaColor}">${smaSignal}</div>
        </div>
        <div class="sq-tech-row">
          <div class="sq-tech-row__label">SMA50</div>
          <div class="sq-tech-row__value">${fmtSafeNumber(sma50, { decimals: 2 })}</div>
          <div class="sq-tech-row__hint ${smaColor}">${sma20 !== null && sma50 !== null ? (sma20 > sma50 ? '中期偏多' : '中期偏空') : '—'}</div>
        </div>
        <div class="sq-tech-row">
          <div class="sq-tech-row__label">RSI14</div>
          <div class="sq-tech-row__value">${fmtSafeNumber(rsi, { decimals: 2 })}</div>
          <div class="sq-tech-row__hint"><span class="${rsiColor}">${rsiZone}</span></div>
        </div>
      </div>
      <div class="sq-card__source">計算基準：${today} 收盤</div>
    </div>
  `;
}
