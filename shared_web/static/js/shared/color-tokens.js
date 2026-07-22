/**
 * 統一的金融色彩函式庫
 * 集中管理所有漲跌/資金流/風險/信號的顏色判斷邏輯
 * 取代分散各處的 pnlColor / convColor / inline color 判斷
 */
/**
 * financialColor — 根據數值和語意類別回傳對應 CSS 變數顏色
 *
 * @param {number|null} value - 數值
 * @param {'pnl'|'trend'|'capital'|'signal'|'risk'} category
 *   - pnl: 損益 (正=profit, 負=loss)
 *   - trend: 漲跌方向 (正=bullish, 負=bearish)
 *   - capital: 資金流向 (正=inflow, 負=outflow)
 *   - signal: 事件信號 (正=bullish, 負=bearish)
 *   - risk: 風險等級 (high/medium/low)
 * @returns {string} CSS var() 值，或空字串
 */
export function financialColor(value, category) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) return '';
  const num = Number(value);
  const tokenMap = {
    pnl:     [ 'var(--pnl-profit)', 'var(--pnl-loss)' ],
    trend:   [ 'var(--trend-bullish)', 'var(--trend-bearish)' ],
    capital: [ 'var(--capital-inflow)', 'var(--capital-outflow)' ],
    signal:  [ 'var(--signal-bullish)', 'var(--signal-bearish)' ],
  };
  const pair = tokenMap[category];
  if (!pair) return '';
  return num >= 0 ? pair[0] : pair[1];
}

/**
 * regimeColor — 將盤勢 regime 字串對應到顏色
 *
 * @param {'RISK_ON'|'RISK_OFF'|'NEUTRAL'|'TRANSITIONAL'} regime
 * @returns {string} CSS var() 值
 */
export function regimeColor(regime) {
  const map = {
    RISK_ON:       'var(--trend-bullish)',
    RISK_OFF:      'var(--trend-bearish)',
    NEUTRAL:       'var(--warn)',
    TRANSITIONAL:  'var(--warn)',
  };
  return map[regime] || 'var(--muted)';
}

/**
 * severityColor — 將事件嚴重度對應到顏色
 *
 * @param {'low'|'medium'|'high'|'critical'} severity
 * @returns {string} CSS var() 值
 */
export function severityColor(severity) {
  const map = {
    low:      'var(--metric-good)',
    medium:   'var(--warn)',
    high:     'var(--risk-high)',
    critical: 'var(--color-danger)',
  };
  return map[severity] || 'var(--muted)';
}

/**
 * confidenceColor — 將信心分數對應到顏色
 *
 * @param {number} confidence - 0..1
 * @returns {string} CSS var() 值
 */
export function confidenceColor(confidence) {
  if (confidence >= 0.7) return 'var(--metric-good)';
  if (confidence >= 0.4) return 'var(--warn)';
  return 'var(--muted)';
}

/**
 * pnlProfitColor / pnlLossColor — 零依賴的純函式包裝
 * 回傳 CSS var() 字串，無需 import getThemeColor
 */
export function pnlProfitColor()  { return 'var(--pnl-profit)'; }
export function pnlLossColor()   { return 'var(--pnl-loss)'; }
export function inflowColor()    { return 'var(--capital-inflow)'; }
export function outflowColor()   { return 'var(--capital-outflow)'; }

// --- Chart palette helpers (added in Phase 2b-A, see docs/operations/color-token-audit-2026-07-22.md) ---
// These wrap the existing CSS variables in shared_web/static/css/base/variables.css
// so JS callers can reference semantic names instead of hardcoded hex.

/** chartAxisColor — chart axis / gridline / minor label color */
export function chartAxisColor()      { return 'var(--muted)'; }

/** chartBackgroundColor — chart panel background (dark theme) */
export function chartBackgroundColor() { return 'var(--panel)'; }

/** chartTextColor — chart text / legend / tooltip */
export function chartTextColor()      { return 'var(--text)'; }

/** mutedTextColor — secondary / de-emphasized text */
export function mutedTextColor()       { return 'var(--status-unknown)'; }

/** accentBlueColor — primary accent blue (replaces #4fc1ff / #3b82f6 / #3498db) */
export function accentBlueColor()      { return 'var(--accent)'; }

/** accentPurpleColor — secondary accent purple (replaces #a855f7 / #8b5cf6 / #9b59b6) */
export function accentPurpleColor()    { return 'var(--accent-secondary)'; }

/** accentTealColor — tertiary teal (replaces #1abc9c; uses --accent-tertiary = #10b981 as fallback) */
export function accentTealColor()      { return 'var(--accent-tertiary)'; }

/** neutralTextColor — neutral gray (replaces #6b7280) */
export function neutralTextColor()     { return 'var(--status-unknown)'; }

/** overlayColor — overlay/shadow base (uses --bg = #0b0d11 as the deepest dark token) */
export function overlayColor()         { return 'var(--bg)'; }

// --- End Phase 2b-A additions ---

/**
 * hexToRgba — 將 hex 顏色轉為 rgba 字串（Canvas 繪圖用）
 * 集中於 color-tokens.js，取代各頁面重複定義的本地版本。
 *
 * @param {string} hex - #RGB、#RRGGBB、或無 # 前綴
 * @param {number} alpha - 透明度，預設 1
 * @returns {string} rgba(r, g, b, alpha) 或無法解析時回傳原字串
 */
export function hexToRgba(hex, alpha = 1) {
  if (!hex) return `rgba(0, 0, 0, ${alpha})`;
  if (!hex.startsWith('#')) return hex;
  let h = hex.slice(1);
  if (h.length === 3) {
    h = h.split('').map(c => c + c).join('');
  }
  const r = parseInt(h.slice(0, 2), 16);
  const g = parseInt(h.slice(2, 4), 16);
  const b = parseInt(h.slice(4, 6), 16);
  return `rgba(${r}, ${g}, ${b}, ${alpha})`;
}
