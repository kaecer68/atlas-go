import { escapeHtml, renderEmptyState, renderSkeleton } from '../shared/app-utils.js';

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

  const change = quote.last - quote.open;
  const changePct = quote.open ? (change / quote.open * 100) : 0;
  const volumeLots = Math.floor(quote.volume / 1000);

  let colorClass = 'sq-price-neutral';
  let sign = '';
  if (change > 0) { colorClass = 'sq-price-up'; sign = '▲'; }
  else if (change < 0) { colorClass = 'sq-price-down'; sign = '▼'; }

  const formatNum = n => typeof n === 'number' ? n.toLocaleString(undefined, { maximumFractionDigits: 2 }) : '—';

  return `
    <div class="sq-header">
      <div class="sq-header-title">${escapeHtml(name)} (${escapeHtml(symbol)})</div>
      <div class="sq-header-price ${colorClass}">${formatNum(quote.last)}</div>
      <div class="sq-header-details ${colorClass}">
        <span>${sign} ${formatNum(Math.abs(change))}</span>
        <span>${sign} ${formatNum(Math.abs(changePct))}%</span>
      </div>
      <div class="sq-header-details" style="margin-top:var(--spacing-2)">
        <span>開: ${formatNum(quote.open)}</span>
        <span>高: ${formatNum(quote.high)}</span>
        <span>低: ${formatNum(quote.low)}</span>
        <span>量: ${volumeLots.toLocaleString()} 張</span>
      </div>
      <div style="font-size:var(--text-xs);color:var(--text-tertiary)">資料: Fugle 即時 (秒級延遲)</div>
    </div>
  `;
}
