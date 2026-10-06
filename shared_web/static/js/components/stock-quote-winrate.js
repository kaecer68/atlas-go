import { renderEmptyState, renderSkeleton } from '../shared/app-utils.js';

// 個股勝率 section (PR 3c, Part 2) — renders the persisted Phase-4
// stockpicker win-rate aggregates from GET /api/stock/win_rate.
//
// Contract with the backend (mirrors MCP stock_get_win_rate):
//   - 200 + found=true  → conditions[] with per-condition win-rate stats
//   - 200 + found=false → "暫無勝率資料" empty state (no data is NOT an error)
//   - 503               → transient unavailable box (store not configured)
//
// Calibration status semantics (stockpicker/winrate.go):
//   - eligible    → 樣本充足，可參考 (green badge)
//   - calibrating → 樣本不足，僅供觀察 (gray badge + note)
//   - degraded    → IS/OOS 背離或過擬合，已降級 (amber badge)

function isValidNumber(v) {
  return typeof v === 'number' && Number.isFinite(v);
}

// escapeHtml is a minimal HTML escaper for API-provided strings (condition
// ids / symbols come from the URL query; defense-in-depth against XSS even
// though the handler echoes them as JSON).
function escapeHtml(s) {
  return String(s ?? '')
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

// formatPct renders a 0..1 win rate as a percentage, e.g. 0.619 → "61.9%".
function formatPct(v, digits = 1) {
  if (!isValidNumber(v)) return '—';
  return `${(v * 100).toFixed(digits)}%`;
}

// formatSignedPct renders avg_forward_return (decimal) as signed percent.
function formatSignedPct(v) {
  if (!isValidNumber(v)) return '—';
  const sign = v > 0 ? '+' : '';
  return `${sign}${(v * 100).toFixed(2)}%`;
}

function statusBadge(status) {
  switch (status) {
    case 'eligible':
      return '<span class="sq-winrate-badge sq-winrate-badge--eligible">可參考</span>';
    case 'calibrating':
      return '<span class="sq-winrate-badge sq-winrate-badge--calibrating">校準中</span>';
    case 'degraded':
      return '<span class="sq-winrate-badge sq-winrate-badge--degraded">已降級</span>';
    default:
      return '';
  }
}

// Demoted families (2026-10-06). Source of truth: internal/config/stockpicker_edge.go
// (config.IsDemotedStockpickerCondition / DemotedStockpickerConditionIDs).
// These two families lose money after the 0.585% round-trip cost (5-day
// net-cost expectancy -0.517% t=-11.4 and -0.987% t=-21.3 measured on
// stock_signal_outcomes, n=73,819), so no recommendation surface may present
// them as usable. This card keeps MEASURING them — the aggregates stay visible
// per the demotion's "measurement preserved" rule — but it must not label them
// "可參考": the badge drops to a measurement-only wording and an explicit note
// names the expectancy. Keep in sync with the Go list.
const DEMOTED_CONDITIONS = new Map([
  ['foreign-3d-net-buy', '−0.517%（t=−11.4，n=31,982）'],
  ['momentum-20d-positive', '−0.987%（t=−21.3，n=31,107）'],
]);

// demotedBadge replaces the referenceable wording for a demoted condition while
// keeping the calibration classes (the calibration status itself is still true —
// what changed is whether the family may back advice).
function demotedBadge(status) {
  const inner = status === 'eligible' ? '僅供量測' : '';
  const base = statusBadge(status);
  if (!inner) return base;
  return base.replace('>可參考<', `>${inner}<`).replace('sq-winrate-badge--eligible', 'sq-winrate-badge--eligible sq-winrate-badge--demoted');
}

// AVOID-semantics conditions (頂背離): a LOW forward win rate after trigger
// CONFIRMS the signal — the condition fires as an avoid/exit warning, not a
// buy signal. Rendered with an inverted badge + inverted return coloring so
// users don't misread a "good avoid signal" as a "bad condition" (k3 review
// F2). Keep in sync with stockpicker.IsAvoidCondition.
const AVOID_CONDITIONS = new Set(['price-volume-top-divergence']);

function renderCondition(cond) {
  const rawId = cond.condition_id || cond.source || '';
  const id = escapeHtml(rawId);
  const bareId = rawId.replace(/^stockpicker-/, '');
  const isAvoid = AVOID_CONDITIONS.has(bareId);
  const demotedReason = DEMOTED_CONDITIONS.get(bareId);
  const badge = demotedReason ? demotedBadge(cond.calibration_status) : statusBadge(cond.calibration_status);
  const calibrating = cond.calibration_status === 'calibrating';
  const observeNote = calibrating
    ? '<div class="sq-winrate-note">樣本數不足，僅供觀察</div>'
    : '';
  const avoidNote = isAvoid
    ? '<div class="sq-winrate-note sq-winrate-note--avoid">反向指標（頂背離）：觸發後勝率越低、前瞻報酬越負，代表「迴避訊號」越有效</div>'
    : '';
  const demotedNote = demotedReason
    ? `<div class="sq-winrate-note sq-winrate-note--demoted">已移出推薦與排名：5 日淨成本期望值 ${escapeHtml(demotedReason)}，扣掉 0.585% 來回成本後無 edge。數值僅供量測參考。</div>`
    : '';

  const range = (cond.data_start && cond.data_end)
    ? `${escapeHtml(cond.data_start)} ~ ${escapeHtml(cond.data_end)}`
    : '—';

  return `<div class="sq-winrate-condition">
    <div class="sq-winrate-condition__head">
      <span class="sq-winrate-condition__id">${id}</span>
      ${badge}
    </div>
    <div class="sq-tech-list">
      <div class="sq-tech-row">
        <div class="sq-tech-row__label">勝率</div>
        <div class="sq-tech-row__value">${formatPct(cond.win_rate)}</div>
        <div class="sq-tech-row__hint sq-price-neutral">${isValidNumber(cond.observations) ? `${cond.observations} 次` : '—'}</div>
      </div>
      <div class="sq-tech-row">
        <div class="sq-tech-row__label">Wilson</div>
        <div class="sq-tech-row__value">${formatPct(cond.wilson_lower, 0)} ~ ${formatPct(cond.wilson_upper, 0)}</div>
        <div class="sq-tech-row__hint sq-price-neutral">95% CI</div>
      </div>
      <div class="sq-tech-row">
        <div class="sq-tech-row__label">平均報酬</div>
        <div class="sq-tech-row__value ${isAvoid
          ? (cond.avg_forward_return < 0 ? 'sq-price-up' : cond.avg_forward_return > 0 ? 'sq-price-down' : 'sq-price-neutral')
          : (cond.avg_forward_return > 0 ? 'sq-price-up' : cond.avg_forward_return < 0 ? 'sq-price-down' : 'sq-price-neutral')}">${formatSignedPct(cond.avg_forward_return)}</div>
        <div class="sq-tech-row__hint sq-price-neutral">${range}${isAvoid ? ' · 反向語義' : ''}</div>
      </div>
    </div>
    ${observeNote}
    ${avoidNote}
    ${demotedNote}
  </div>`;
}

export function renderWinRate(state, winRateResult, coverage) {
  if (state === 'loading') {
    return `<div class="sq-card"><h3 class="sq-card__title">個股勝率</h3>${renderSkeleton(3)}</div>`;
  }
  if (state === 'error' || winRateResult.status === 'error') {
    // Store-not-configured 503 is transient/infra — render a neutral
    // "稍後再試" so the page doesn't look broken (same as monthly_revenue).
    return `<div class="sq-card"><h3 class="sq-card__title">個股勝率</h3>
      <div class="sq-error-box">勝率資料暫時無法取得，請稍後再試。</div>
    </div>`;
  }

  const data = winRateResult.data;
  if (!data || !data.found || !data.conditions || data.conditions.length === 0) {
    // 200 + found=false: no stored win-rate data. Distinguish WHY when the
    // bundle coverage says the symbol is out of TWSE scope (TPEX/ETF): the
    // stockpicker backtest universe is quote-symbols with enough bars +
    // T86 flows (TWSE-scoped), so out-of-scope symbols will NEVER have
    // data — say so instead of a bare "暫無" that reads like a glitch.
    // Note: real found=true data is rendered even when coverage says
    // NOT_COVERED (e.g. 0050 has quotes+flows despite no fundamentals).
    if (coverage && !coverage.covered) {
      return `<div class="sq-card"><h3 class="sq-card__title">個股勝率</h3>
        <div class="sq-scope-notice">此標的暫未納入勝率量測（量測範圍：具備足夠日線與法人籌碼資料的 TWSE 上市普通股）<br><small>${escapeHtml(coverage.reason || '')}</small></div>
      </div>`;
    }
    return `<div class="sq-card"><h3 class="sq-card__title">個股勝率</h3>${renderEmptyState('暫無勝率資料（此個股觸發次數不足或歷史資料涵蓋較短）')}</div>`;
  }

  const conditions = data.conditions.map(renderCondition).join('');
  return `<div class="sq-card">
    <h3 class="sq-card__title">個股勝率</h3>
    ${conditions}
    <div class="sq-card__source">滾動視窗：${escapeHtml(data.rolling_window || '120d')} · 資料期間為觸發日範圍</div>
  </div>`;
}
