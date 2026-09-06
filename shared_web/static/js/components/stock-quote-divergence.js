import { renderEmptyState, renderSkeleton } from '../shared/app-utils.js';

// 量價背離 section — renders the price/volume divergence reading from
// GET /api/stock/volume_divergence (mirrors MCP stock_get_volume_divergence).
//
// Contract with the backend:
//   - 200 → full reading: top_divergence / bottom_divergence flags plus the
//     raw evidence (close vs window high/low %, vol_ma5, vol_ma20) and a
//     zh-TW interpretation string
//   - 503 → transient/insufficient-data box (fewer than 20 bars, store not
//     configured) — degrade gracefully like monthly_revenue
//
// Semantics (domain.DetectVolumeDivergence):
//   頂背離 = 股價接近近 N 日新高 且 5 日均量 < 20 日均量（上漲動能可能衰竭）
//   底背離 = 股價接近近 N 日新低 且 5 日均量 < 20 日均量（賣壓可能竭盡）

function isValidNumber(v) {
  return typeof v === 'number' && Number.isFinite(v);
}

// escapeHtml mirrors stock-quote-winrate.js — defense-in-depth for
// API-provided strings (symbol / interpretation).
function escapeHtml(s) {
  return String(s ?? '')
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

function formatInt(v) {
  if (!isValidNumber(v)) return '—';
  return Math.round(v).toLocaleString('zh-TW');
}

function formatPct(v, digits = 1) {
  if (!isValidNumber(v)) return '—';
  return `${v.toFixed(digits)}%`;
}

// verdict renders the headline badge for the divergence reading.
function verdict(data) {
  if (data.top_divergence) {
    return '<div class="sq-divergence-badge sq-divergence-badge--bearish">⚠️ 頂背離（偏空警訊）</div>';
  }
  if (data.bottom_divergence) {
    return '<div class="sq-divergence-badge sq-divergence-badge--bullish">🔎 底背離（賣壓竭盡）</div>';
  }
  return '<div class="sq-divergence-badge sq-divergence-badge--none">無明顯背離</div>';
}

export function renderDivergence(state, divergenceResult) {
  if (state === 'loading' || !divergenceResult) {
    return `<div class="sq-card"><h3 class="sq-card__title">量價背離</h3>${renderSkeleton(3)}</div>`;
  }
  if (state === 'error' || divergenceResult.status === 'error') {
    // 503 (insufficient bars / store not configured) is transient — render
    // a neutral box so the page doesn't look broken (same as win_rate).
    return `<div class="sq-card"><h3 class="sq-card__title">量價背離</h3>
      <div class="sq-error-box">量價背離資料暫時無法取得，請稍後再試。</div>
    </div>`;
  }

  const data = divergenceResult.data;
  if (!data) {
    return `<div class="sq-card"><h3 class="sq-card__title">量價背離</h3>${renderEmptyState('暫無量價背離資料')}</div>`;
  }
  // Out-of-scope: the handler answers 200 + coverage_note=NOT_COVERED +
  // volume_divergence:{empty:true} for TPEX/ETF symbols (same contract as
  // chips/fundamentals/technical). Render the neutral scope notice — NOT a
  // fake "無明顯背離" reading with empty numbers.
  if (data.coverage_note === 'NOT_COVERED' || (data.volume_divergence && data.volume_divergence.empty)) {
    return `<div class="sq-card"><h3 class="sq-card__title">量價背離</h3>
      <div class="sq-scope-notice">此股票代號不在量價背離涵蓋範圍（涵蓋 TWSE 上市普通股，約 1070 隻）<br><small>${escapeHtml(data.reason || '')}</small></div>
    </div>`;
  }

  const staleNote = data.trading_day === false
    ? '<div class="sq-winrate-note">今日非交易日，數據為最近交易日收盤結果</div>'
    : '';

  return `<div class="sq-card">
    <h3 class="sq-card__title">量價背離</h3>
    ${verdict(data)}
    <div class="sq-tech-list">
      <div class="sq-tech-row">
        <div class="sq-tech-row__label">收盤 vs ${escapeHtml(String(data.window_days ?? 30))}日高點</div>
        <div class="sq-tech-row__value">${formatPct(data.close_below_high_pct)}</div>
        <div class="sq-tech-row__hint sq-price-neutral">低於高點幅度</div>
      </div>
      <div class="sq-tech-row">
        <div class="sq-tech-row__label">收盤 vs ${escapeHtml(String(data.window_days ?? 30))}日低點</div>
        <div class="sq-tech-row__value">${formatPct(data.close_above_low_pct)}</div>
        <div class="sq-tech-row__hint sq-price-neutral">高於低點幅度</div>
      </div>
      <div class="sq-tech-row">
        <div class="sq-tech-row__label">5日均量 / 20日均量</div>
        <div class="sq-tech-row__value">${formatInt(data.vol_ma5)} / ${formatInt(data.vol_ma20)}</div>
        <div class="sq-tech-row__hint ${data.volume_declining ? 'sq-price-down' : 'sq-price-neutral'}">${data.volume_declining ? '量能遞減' : '量能未遞減'}</div>
      </div>
    </div>
    <p class="sq-divergence-interpretation">${escapeHtml(data.interpretation || '')}</p>
    ${staleNote}
    <div class="sq-card__source">區間：近 ${escapeHtml(String(data.bars_used ?? data.window_days ?? 30))} 個交易日 · ${escapeHtml(data.latest_date || '')}</div>
  </div>`;
}
