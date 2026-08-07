import { renderEmptyState, renderSkeleton } from '../shared/app-utils.js';
import { fmtSafeNumber } from '../shared/format-metric.js';

// STRUCTURAL_ORDER_YOY_THRESHOLD mirrors SK-31 §4 散戶解讀: monthly
// revenue YoY > 30% = 結構性訂單 signal. Kept in sync with the SK-31
// wiki doc — if the threshold changes there, update here.
const STRUCTURAL_ORDER_YOY_THRESHOLD = 30;

function isValidNumber(v) {
  return typeof v === 'number' && Number.isFinite(v);
}

// escapeHtml is a minimal HTML escaper for API-provided strings (symbol
// comes from user input via URL query; defense-in-depth against XSS even
// though the handler echoes it back as JSON).
function escapeHtml(s) {
  return String(s ?? '')
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

// formatSignedPct renders a signed percentage with explicit +/− prefix.
function formatSignedPct(v) {
  if (!isValidNumber(v)) return '—';
  const sign = v > 0 ? '+' : '';
  return `${sign}${v.toFixed(1)}%`;
}

// pctColorClass maps a signed percentage to the stock-quote color token.
// Positive → green (sq-price-up), negative → red (sq-price-down).
function pctColorClass(v) {
  if (!isValidNumber(v) || v === 0) return 'sq-price-neutral';
  return v > 0 ? 'sq-price-up' : 'sq-price-down';
}

export function renderRevenue(state, revenueResult) {
  if (state === 'loading') {
    return `<div class="sq-card"><h3 class="sq-card__title">月營收</h3>${renderSkeleton(3)}</div>`;
  }
  if (state === 'error' || revenueResult.status === 'error') {
    // Quota-exhausted 503 (FinMind daily budget) is transient — render a
    // neutral "稍後再試" instead of an error banner so the page doesn't
    // look broken. All other errors share the same copy.
    return `<div class="sq-card"><h3 class="sq-card__title">月營收</h3>
      <div class="sq-error-box">月營收資料暫時無法取得，請稍後再試。</div>
    </div>`;
  }
  if (!revenueResult.data) {
    return `<div class="sq-card"><h3 class="sq-card__title">月營收</h3>${renderEmptyState('無月營收資料')}</div>`;
  }

  const data = revenueResult.data;
  const revenue = isValidNumber(data.value) ? data.value : null;
  const yoy = isValidNumber(data.change_pct) ? data.change_pct : null;
  const mom = isValidNumber(data.mom_pct) ? data.mom_pct : null;
  const symbol = data.symbol ? escapeHtml(data.symbol) : '';

  const revenueDisplay = revenue !== null
    ? fmtSafeNumber(revenue, { maxFractionDigits: 0 })
    : '—';

  const isStructuralOrder = yoy !== null && yoy > STRUCTURAL_ORDER_YOY_THRESHOLD;
  const badge = isStructuralOrder
    ? `<span class="sq-revenue-badge sq-revenue-badge--signal">結構性訂單信號</span>`
    : '';

  return `<div class="sq-card">
    <h3 class="sq-card__title">月營收${symbol ? `（${symbol}）` : ''}${badge}</h3>
    <div class="sq-tech-list">
      <div class="sq-tech-row">
        <div class="sq-tech-row__label">月營收</div>
        <div class="sq-tech-row__value">${revenueDisplay}</div>
        <div class="sq-tech-row__hint sq-price-neutral">元</div>
      </div>
      <div class="sq-tech-row">
        <div class="sq-tech-row__label">年增率（YoY）</div>
        <div class="sq-tech-row__value ${pctColorClass(yoy)}">${formatSignedPct(yoy)}</div>
        <div class="sq-tech-row__hint ${pctColorClass(yoy)}">${yoy !== null && yoy > 0 ? '年增' : yoy !== null && yoy < 0 ? '年減' : '—'}</div>
      </div>
      <div class="sq-tech-row">
        <div class="sq-tech-row__label">月增率（MoM）</div>
        <div class="sq-tech-row__value ${pctColorClass(mom)}">${formatSignedPct(mom)}</div>
        <div class="sq-tech-row__hint ${pctColorClass(mom)}">${mom !== null && mom > 0 ? '月增' : mom !== null && mom < 0 ? '月減' : '—'}</div>
      </div>
    </div>
    ${isStructuralOrder ? `<div class="sq-revenue-note">年增率超過 ${STRUCTURAL_ORDER_YOY_THRESHOLD}%，屬結構性訂單信號（對位 SK-31 §4 月頻對位）。</div>` : ''}
  </div>`;
}
