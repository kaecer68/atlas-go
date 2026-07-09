import { escapeHtml, renderEmptyState, renderSkeleton } from '../shared/app-utils.js';

function renderBar(label, value, maxAbs) {
  const isPositive = value > 0;
  const sign = isPositive ? '▲ +' : (value < 0 ? '▼ ' : '');
  const colorClass = isPositive ? 'sq-chip-inflow' : (value < 0 ? 'sq-chip-outflow' : '');
  const widthPct = maxAbs ? (Math.abs(value) / maxAbs * 100) : 0;
  
  return `
    <div class="sq-chip-row">
      <div class="sq-chip-label">${escapeHtml(label)}</div>
      <div class="sq-chip-bar-container">
        <div class="sq-chip-bar ${colorClass}" style="width: ${widthPct}%"></div>
      </div>
      <div class="sq-chip-value ${isPositive ? 'sq-price-up' : (value < 0 ? 'sq-price-down' : 'sq-price-neutral')}">
        ${sign}${Math.abs(value).toLocaleString()} 張
      </div>
    </div>
  `;
}

export function renderChips(state, chipsResult) {
  if (state === 'loading') {
    return `<div>${renderSkeleton(4)}</div>`;
  }
  if (state === 'error' || chipsResult.status === 'error') {
    return `<div class="sq-error-box">籌碼資料當日無更新,請稍後再試</div>`;
  }
  if (!chipsResult.data) {
    return renderEmptyState('無籌碼資料');
  }

  const data = chipsResult.data;
  const foreign = data.foreign_investor_net || 0;
  const domestic = data.domestic_fund_net || 0;
  const dealer = data.dealer_net || 0;
  const total = foreign + domestic + dealer;

  const maxAbs = Math.max(Math.abs(foreign), Math.abs(domestic), Math.abs(dealer), Math.abs(total)) || 1;

  let dateDisplay = data.date ? escapeHtml(data.date) : '未知';
  if (dateDisplay.length === 8) {
    dateDisplay = `${dateDisplay.substring(0,4)}-${dateDisplay.substring(4,6)}-${dateDisplay.substring(6,8)}`;
  }

  return `
    <div class="card">
      <div class="card-header">
        <h3 class="card-title">籌碼 (三大法人)</h3>
      </div>
      <div class="card-body">
        ${renderBar('外資', foreign, maxAbs)}
        ${renderBar('投信', domestic, maxAbs)}
        ${renderBar('自營商', dealer, maxAbs)}
        <hr style="margin:var(--spacing-2) 0; border:none; border-top:1px solid var(--border-color);" />
        ${renderBar('合計', total, maxAbs)}
      </div>
      <div style="font-size:var(--text-xs);color:var(--text-tertiary);margin-top:var(--spacing-2)">
        資料日期: ${dateDisplay} (資料來源: TWSE)
      </div>
    </div>
  `;
}
