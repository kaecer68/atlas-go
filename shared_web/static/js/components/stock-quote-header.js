import { escapeHtml, renderSkeleton } from '../shared/app-utils.js';
import { fmtSafeNumber, fmtSafeSignedPct } from '../shared/format-metric.js';

export function renderHeader(state, quoteResult, chipsResult, coverage) {
  if (state === 'loading') {
    return `<div class="sq-header">${renderSkeleton(3)}</div>`;
  }
  if (state === 'error' || quoteResult.status === 'error' || !quoteResult.data) {
    return `<div class="sq-error-box">報價功能未啟用或暫時無法取得，請洽管理員 (${escapeHtml(quoteResult?.error || '')})</div>`;
  }


  // Out-of-scope: still render the quote (Fugle covers TPEX quotes)
  // but append a small scope badge so users understand chips/fundamentals
  // are not in coverage for this symbol.
  const scopeBadge = coverage && !coverage.covered
    ? `<div class="sq-scope-notice sq-scope-notice--inline">本系統僅涵蓋 TWSE 上市普通股；此代號不在範圍</div>`
    : '';

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
  if (change !== null) {
    if (change > 0) { colorClass = 'sq-price-up'; }
    else if (change < 0) { colorClass = 'sq-price-down'; }
  }

  const changeSign = change !== null
    ? (change > 0 ? '+' : change < 0 ? '' : '')
    : '';
  const absChange = change !== null ? Math.abs(change) : null;

  return `
    <div class="sq-header">
      <div class="sq-header-title">${escapeHtml(name)} (${escapeHtml(symbol)})</div>
      <div class="sq-header-meta">
        <span class="sq-header-price ${colorClass}">${fmtSafeNumber(quote.last, { decimals: 2 })}</span>
        <span class="sq-header-change ${colorClass}">
          ${change !== null ? changeSign + fmtSafeNumber(absChange, { decimals: 2 }) : '—'}
        </span>
        <span class="sq-header-change ${colorClass}">
          ${changePct !== null ? fmtSafeSignedPct(changePct / 100, 2) : '—'}
        </span>
      </div>
      <div class="sq-header-details">
        <span>開 ${fmtSafeNumber(quote.open, { decimals: 2 })}</span>
        <span>高 ${fmtSafeNumber(quote.high, { decimals: 2 })}</span>
        <span>低 ${fmtSafeNumber(quote.low, { decimals: 2 })}</span>
        <span>量 ${volumeLots !== null ? volumeLots.toLocaleString() + ' 張' : '—'}</span>
      </div>
      <div class="sq-header-source">資料來源：Fugle 即時（秒級延遲）</div>
      ${scopeBadge}
    </div>
  `;
}
