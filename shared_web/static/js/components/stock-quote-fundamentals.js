import { escapeHtml, renderEmptyState, renderSkeleton } from '../shared/app-utils.js';
import { fmtSafeNumber } from '../shared/format-metric.js';

export function renderFundamentals(state, fundResult) {
  if (state === 'loading') {
    return `<div>${renderSkeleton(4)}</div>`;
  }
  if (state === 'error' || fundResult.status === 'error') {
    return `<div class="sq-error-box">基本面資料暫時無法取得</div>`;
  }
  if (!fundResult.data || Object.keys(fundResult.data).length === 0) {
    return renderEmptyState('無基本面資料');
  }

  const data = fundResult.data;
  
  let peDisplay = '—';
  if (data.PE === 0) {
    peDisplay = '<span class="sq-price-down" title="公司目前為虧損狀態">虧損</span>';
  } else if (data.PE) {
    peDisplay = fmtSafeNumber(data.PE, { decimals: 2 });
  }

  const pbDisplay = fmtSafeNumber(data.PB, { decimals: 2 });
  const psDisplay = data.PS ? fmtSafeNumber(data.PS, { decimals: 2 }) : '—';
  const divDisplay = data.DividendYield ? fmtSafeNumber(data.DividendYield, { decimals: 2, suffix: '%' }) : '—';
  const sectorDisplay = data.Sector ? escapeHtml(data.Sector) : '—';

  return `
    <div class="card">
      <div class="card-header">
        <h3 class="card-title">基本面</h3>
      </div>
      <table class="sq-table">
        <tbody>
          <tr>
            <th>PE (本益比)</th>
            <td>${peDisplay}</td>
          </tr>
          <tr>
            <th>PB (股價淨值比)</th>
            <td>${pbDisplay}</td>
          </tr>
          <tr>
            <th>PS (市銷率)</th>
            <td>${psDisplay}</td>
          </tr>
          <tr>
            <th>殖利率</th>
            <td>${divDisplay}</td>
          </tr>
          <tr>
            <th>產業分類</th>
            <td><span class="badge">${sectorDisplay}</span></td>
          </tr>
        </tbody>
      </table>
      <div style="font-size:var(--text-xs);color:var(--text-tertiary);margin-top:var(--spacing-2)">
        資料日期: T-1
      </div>
    </div>
  `;
}
