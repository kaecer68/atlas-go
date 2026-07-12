/**
 * format-metric.js
 * 統一格式化投資人介面中的金融數值，避免語義錯誤（如最大回撤顯示為 +0.0%）。
 */

/**
 * 將數值格式化為帶正負號的字串。
 * @param {number|null|undefined} value
 * @param {Object} options
 * @param {number} [options.decimals=2]
 * @param {string} [options.suffix='']
 * @param {boolean} [options.forceSign=false] true 時強制加上 + 號
 * @param {boolean} [options.invertSign=false]  true 時反轉正負號（用於回撤、損失等）
 * @returns {string}
 */
function isValidNumber(value) {
  return typeof value === 'number' && Number.isFinite(value);
}

export function formatSigned(value, options = {}) {
  const { decimals = 2, suffix = '', forceSign = false, invertSign = false } = options;
  if (!isValidNumber(value)) return `—${suffix ? ' ' + suffix : ''}`;
  let v = invertSign ? -value : value;
  // 先依指定精度四捨五入，再決定正負號，避免 -0.0001 顯示成 -0.0。
  const factor = 10 ** decimals;
  v = Math.round(v * factor) / factor;
  const sign = v > 0 ? '+' : v < 0 ? '−' : '';
  const displaySign = forceSign || v !== 0 ? sign : '';
  const abs = Math.abs(v).toFixed(decimals);
  return `${displaySign}${abs}${suffix}`;
}

/**
 * 格式化最大回撤（Max Drawdown）。
 * 慣例：以負數或絕對值呈現，絕對不會出現 + 號。
 * @param {number|null|undefined} value 0.15 表示 15%
 * @param {Object} options
 * @param {boolean} [options.asAbsolute=false] true 時回傳絕對值百分比
 * @returns {string}
 */
export function formatMaxDrawdown(value, options = {}) {
  const { asAbsolute = false } = options;
  if (!isValidNumber(value)) return '—';
  const pct = value * 100;
  const abs = Math.abs(pct).toFixed(1);
  if (pct === 0) return `0.0%`;
  return asAbsolute ? `${abs}%` : `−${abs}%`;
}

/**
 * 格式化 HHI（Herfindahl-Hirschman Index）。
 * 不使用 %，改以純數值或低/中/高分級。
 * @param {number|null|undefined} value
 * @returns {{value: string, level: 'low'|'medium'|'high'|null}}
 */
export function formatHHI(value) {
  if (!isValidNumber(value)) {
    return { value: '—', level: null };
  }
  let level = 'low';
  if (value > 0.25) level = 'high';
  else if (value > 0.15) level = 'medium';
  return { value: value.toFixed(3), level };
}

/**
 * 簡單的數字/百分比格式化。
 * @param {number|null|undefined} value
 * @param {Object} options
 * @param {number} [options.decimals=2]
 * @param {string} [options.suffix='']
 * @param {boolean} [options.percent=false] true 時將 value 視為 0.15 並乘 100
 * @returns {string}
 */
export function formatNumber(value, options = {}) {
  const { decimals = 2, suffix = '', percent = false, useGrouping = false } = options;
  if (!isValidNumber(value)) return `—${suffix ? ' ' + suffix : ''}`;
  let v = percent ? value * 100 : value;
  // 同樣先四捨五入，避免 -0.0001 在四捨五入後變成 -0.0。
  const factor = 10 ** decimals;
  v = Math.round(v * factor) / factor;
  const formatted = useGrouping
    ? v.toLocaleString('en-US', { minimumFractionDigits: decimals, maximumFractionDigits: decimals })
    : v.toFixed(decimals);
  return `${formatted}${suffix}`;
}

/**
 * 依據風險等級回傳本地化標籤。
 * @param {'low'|'medium'|'high'|string|null|undefined} level
 * @returns {string}
 */
export function riskLevelLabel(level) {
  const map = { low: '低', medium: '中', high: '高' };
  return map[level] || '未知';
}

/**
 * 將 API 回傳的數值安全地轉為數字。
 * @param {*} v
 * @returns {number|null}
 */
export function toNumber(v) {
  if (v === null || v === undefined || v === '') return null;
  const n = Number(v);
  return Number.isNaN(n) ? null : n;
}

// 簡潔別名，方便頁面層 import。
export const fmtSignedPct = (value, decimals = 1) => {
  // 對極小百分比自動增加精度，避免 -0.011% 四捨五入後變成 -0.0%。
  let effectiveDecimals = decimals;
  if (isValidNumber(value) && value !== 0) {
    const abs = Math.abs(value);
    for (let d = decimals; d <= 3; d += 1) {
      const rounded = Math.round(abs * (10 ** d)) / (10 ** d);
      if (rounded !== 0) {
        effectiveDecimals = d;
        break;
      }
    }
  }
  return formatSigned(value, { decimals: effectiveDecimals, suffix: '%', forceSign: true });
};
export const fmtDrawdown = (value) => formatMaxDrawdown(value);
export const fmtHHI = (value) => formatHHI(value);
