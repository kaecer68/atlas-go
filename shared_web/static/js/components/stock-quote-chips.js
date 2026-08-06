import { escapeHtml, renderEmptyState, renderSkeleton } from '../shared/app-utils.js';

function isValidNumber(v) {
  return typeof v === 'number' && Number.isFinite(v);
}

function renderBar(label, value, maxAbs) {
  const hasValue = isValidNumber(value);
  const isPositive = hasValue && value > 0;
  const isNegative = hasValue && value < 0;
  const colorClass = isPositive ? 'sq-chip-inflow' : (isNegative ? 'sq-chip-outflow' : '');
  const widthPct = hasValue && maxAbs ? (Math.abs(value) / maxAbs * 100) : 0;
  const displayValue = hasValue
    ? `${isPositive ? '+' : ''}${Math.round(Math.abs(value)).toLocaleString()} 張`
    : '—';

  return `
    <div class="sq-chip-row">
      <div class="sq-chip-label">${escapeHtml(label)}</div>
      <div class="sq-chip-bar-container">
        <div class="sq-chip-bar ${colorClass}" style="width: ${widthPct}%"></div>
      </div>
      <div class="sq-chip-value ${isPositive ? 'sq-price-up' : (isNegative ? 'sq-price-down' : 'sq-price-neutral')}">
        ${displayValue}
      </div>
    </div>
  `;
}

export function renderChips(state, chipsResult, coverage) {
  if (state === 'loading') {
    return `<div class="sq-card"><h3 class="sq-card__title">籌碼</h3>${renderSkeleton(4)}</div>`;
  }
  // Out-of-scope: chips/fundamentals/technical for TPEX symbols return
  // 200 + coverage_note via the stocktools backend (see
  // docs/manifests/2026-08-06-stock-coverage-notice.md). Show a neutral
  // badge — NOT an error banner — so investors immediately understand
  // that quote works but the other axes do not.
  if (coverage && !coverage.covered) {
    return `<div class="sq-card"><h3 class="sq-card__title">籌碼</h3>
      <div class="sq-scope-notice">此股票代號不在本系統 chips 涵蓋範圍（涵蓋 TWSE 上市普通股，約 1070 隻）<br><small>${escapeHtml(coverage.reason || '')}</small></div>
    </div>`;
  }
  if (state === 'error' || chipsResult.status === 'error') {
    return `<div class="sq-error-box">籌碼資料當日無更新，請稍後再試</div>`;
  }
  if (!chipsResult.data) {
    return renderEmptyState('無籌碼資料');
  }

  const data = chipsResult.data;
  const foreign = isValidNumber(data.foreign_investor_net) ? data.foreign_investor_net : null;
  const domestic = isValidNumber(data.domestic_fund_net) ? data.domestic_fund_net : null;
  const dealer = isValidNumber(data.dealer_net) ? data.dealer_net : null;
  const total = foreign !== null && domestic !== null && dealer !== null
    ? foreign + domestic + dealer
    : null;

  const values = [foreign, domestic, dealer, total].filter(isValidNumber);
  const maxAbs = values.length > 0 ? Math.max(...values.map(Math.abs)) : 1;

  let dateDisplay = data.date ? escapeHtml(data.date) : '未知';
  if (dateDisplay.length === 8) {
    dateDisplay = `${dateDisplay.substring(0, 4)}-${dateDisplay.substring(4, 6)}-${dateDisplay.substring(6, 8)}`;
  }

  return `
    <div class="sq-card">
      <h3 class="sq-card__title">籌碼</h3>
      ${renderBar('外資', foreign, maxAbs)}
      ${renderBar('投信', domestic, maxAbs)}
      ${renderBar('自營商', dealer, maxAbs)}
      <hr style="margin:var(--spacing-1) 0; border:none; border-top:1px solid var(--border-color);" />
      ${renderBar('合計', total, maxAbs)}
      <div class="sq-card__source">資料日期：${dateDisplay}（資料來源：TWSE）</div>
    </div>
  `;
}
