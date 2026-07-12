import { escapeHtml, renderEmptyState, renderSkeleton } from '../shared/app-utils.js';
import { fmtSafeNumber, fmtSafeSignedPct } from '../shared/format-metric.js';

export function renderHeader(state, quoteResult, chipsResult) {
  if (state === 'loading') {
    return `<div class="sq-header">${renderSkeleton(3)}</div>`;
  }
  if (state === 'error' || quoteResult.status === 'error' || !quoteResult.data) {
    return `<div class="sq-error-box">報價功能未啟用或暫時無法取得,請洽管理員 (${escapeHtml(quoteResult?.error || '')})</div>`;
  }

  const quote = quoteResult.data;
  const symbol = quote.symbol;
  const name = (chipsResult && chipsResult.status === 'loaded' && chipsResult.data && chipsResult.data.name) ? chipsResult.data.name : '';

  const hasLast = typeof quote.last === 'number' && Number.isFinite(quote.last);
  const hasOpen = typeof quote.open === 'number' && Number.isFinite(quote.open) && quote.open !== 0;
  const change = hasLast && hasOpen ? quote.last - quote.open : null;
  const changePct = hasLast && hasOpen ? (change / quote.open * 100) : null;
  const volumeLots = typeof quote.volume === 'number' && Number.isFinite(quote.volume)
    ? Math.floor(quote.volume / 1000)
    : null;

  let colorClass = 'sq-price-neutral';
  let sign = '';
  if (change !== null) {
    if (change > 0) { colorClass = 'sq-price-up'; sign = '▲'; }
    else if (change < 0) { colorClass = 'sq-price-down'; sign = '▼'; }
  }

  return `
    <div class="sq-header">
      <div class="sq-header-title">${escapeHtml(name)} (${escapeHtml(symbol)})</div>
      <div class="sq-header-price ${colorClass}">${fmtSafeNumber(quote.last, { decimals: 2 })}</div>
      <div class="sq-header-details ${colorClass}">
        <span>${change !== null ? sign + ' ' + fmtSafeNumber(Math.abs(change), { decimals: 2 }) : '—'}</span>
        <span>${changePct !== null ? fmtSafeSignedPct(changePct / 100, 2) : '—'}</span>
      </div>
      <div class="sq-header-details" style="margin-top:var(--spacing-2)">
        <span>開: ${fmtSafeNumber(quote.open, { decimals: 2 })}</span>
        <span>高: ${fmtSafeNumber(quote.high, { decimals: 2 })}</span>
        <span>低: ${fmtSafeNumber(quote.low, { decimals: 2 })}</span>
        <span>量: ${volumeLots !== null ? volumeLots.toLocaleString() + ' 張' : '—'}</span>
      </div>
      <div style="font-size:var(--text-xs);color:var(--text-tertiary)">資料: Fugle 即時 (秒級延遲)</div>
    </div>
  `;
}
