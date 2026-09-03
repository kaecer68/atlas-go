/**
 * format-metric.js
 * 統一格式化投資人介面中的金融數值，避免語義錯誤（如最大回撤顯示為 +0.0%）。
 */

function isValidNumber(value) {
  return typeof value === 'number' && Number.isFinite(value);
}

/**
 * 統一判斷數值是否為「缺失資料」。
 * 後端缺失資料通常以 null / undefined / NaN / 空字串表示；
 * 此函數讓所有頁面對「無效值」有一致的定義。
 * @param {*} value
 * @returns {boolean}
 */
export function isEmptyMetric(value) {
  return value === null || value === undefined || value === '' || (typeof value === 'number' && Number.isNaN(value));
}

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
export function formatSigned(value, options = {}) {
  const { decimals = 2, suffix = '', forceSign = false, invertSign = false } = options;
  if (!isValidNumber(value)) return `—${suffix ? ' ' + suffix : ''}`;
  let v = invertSign ? -value : value;
  // 先依指定精度四捨五入，再決定正負號，避免 -0.0001 顯示成 -0.0。
  const factor = 10 ** decimals;
  v = Math.round(v * factor) / factor;
  // 消除 IEEE -0
  if (v === 0) v = 0;
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
 * @param {boolean} [options.useGrouping=false] true 時加入千分位
 * @returns {string}
 */
export function formatNumber(value, options = {}) {
  const { decimals = 2, suffix = '', percent = false, useGrouping = false } = options;
  if (!isValidNumber(value)) return `—${suffix ? ' ' + suffix : ''}`;
  let v = percent ? value * 100 : value;
  // 同樣先四捨五入，避免 -0.0001 在四捨五入後變成 -0.0。
  const factor = 10 ** decimals;
  v = Math.round(v * factor) / factor;
  if (v === 0) v = 0;
  const formatted = useGrouping
    ? v.toLocaleString('en-US', { minimumFractionDigits: decimals, maximumFractionDigits: decimals })
    : v.toFixed(decimals);
  // B3 (risk-console Phase 1)：percent:true 即代表百分比語意，
  // 未明確給 suffix 時自動補上 '%'，避免 live/portfolio 頁數字無單位。
  const effSuffix = percent && suffix === '' ? '%' : suffix;
  return `${formatted}${effSuffix}`;
}

/**
 * 格式化幣別（預設 NTD）。
 * @param {number|null|undefined} value
 * @param {Object} options
 * @param {number} [options.decimals=0]
 * @param {string} [options.prefix='NT$']
 * @param {boolean} [options.useGrouping=true]
 * @returns {string}
 */
export function fmtCurrency(value, options = {}) {
  const { decimals = 0, prefix = 'NT$', useGrouping = true } = options;
  if (!isValidNumber(value)) return '—';
  const formatted = value.toLocaleString('en-US', {
    minimumFractionDigits: decimals,
    maximumFractionDigits: decimals,
    useGrouping,
  });
  return `${prefix}${formatted}`;
}

/**
 * 格式化百分比（無正負號）。value 為小數，例如 0.15 → 15.0%。
 * @param {number|null|undefined} value
 * @param {number} [decimals=1]
 * @returns {string}
 */
export function fmtPct(value, decimals = 1) {
  return formatNumber(value, { percent: true, decimals, suffix: '%' });
}

/**
 * 格式化大數：自動縮放為 萬 / 億，並保留千分位。
 * @param {number|null|undefined} value
 * @returns {string}
 */
export function fmtLargeNumber(value) {
  if (!isValidNumber(value)) return '—';
  const abs = Math.abs(value);
  if (abs >= 1e8) {
    return (value / 1e8).toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 }) + ' 億';
  }
  if (abs >= 1e4) {
    return (value / 1e4).toLocaleString('en-US', { minimumFractionDigits: 1, maximumFractionDigits: 1 }) + ' 萬';
  }
  return value.toLocaleString('en-US');
}

/**
 * 帶正負號百分比。value 為已乘 100 的百分點，例如 0.35 → +0.4%。
 * 對極小值自動提升精度，避免出現 -0.0%。
 * @param {number|null|undefined} value
 * @param {number} [decimals=1]
 * @returns {string}
 */
export const fmtSignedPct = (value, decimals = 1) => {
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

// ============================================================================
// Safe formatting wrappers (P2 structural refactor)
// These aliases make the "validity check before format" convention explicit.
// They behave like their non-safe counterparts but are the preferred entry
// point for page code so that missing data never renders as 0.0 / 0.0%.
// ============================================================================

/**
 * 安全格式化帶正負號數值（非百分比）。無效值回傳 '—'。
 * @param {number|null|undefined} value
 * @param {Object} options 同 formatSigned
 * @returns {string}
 */
export function fmtSafeSigned(value, options = {}) {
  if (isEmptyMetric(value)) return '—';
  return formatSigned(value, options);
}

/**
 * 安全格式化一般數值。無效值回傳 '—'，真正的 0 顯示為 '0.00'。
 * @param {number|null|undefined} value
 * @param {Object} options 同 formatNumber
 * @returns {string}
 */
export function fmtSafeNumber(value, options = {}) {
  if (isEmptyMetric(value)) return '—';
  return formatNumber(value, options);
}

/**
 * 安全格式化百分比。無效值回傳 '—'，真正的 0 顯示為 '0.0%'。
 * @param {number|null|undefined} value
 * @param {number} [decimals=1]
 * @returns {string}
 */
export function fmtSafePct(value, decimals = 1) {
  if (isEmptyMetric(value)) return '—';
  return fmtPct(value, decimals);
}

/**
 * 安全格式化帶正負號百分比。無效值回傳 '—'，真正的 0 顯示為 '0.0%'。
 * @param {number|null|undefined} value
 * @param {number} [decimals=1]
 * @returns {string}
 */
export function fmtSafeSignedPct(value, decimals = 1) {
  if (isEmptyMetric(value)) return '—';
  return fmtSignedPct(value, decimals);
}

/**
 * 安全格式化最大回撤。無效值回傳 '—'，真正的 0 顯示為 '0.0%'。
 * @param {number|null|undefined} value
 * @returns {string}
 */
export function fmtSafeDrawdown(value) {
  if (isEmptyMetric(value)) return '—';
  return fmtDrawdown(value);
}

/**
 * 安全格式化幣別。無效值回傳 '—'，真正的 0 顯示為 'NT$0'。
 * @param {number|null|undefined} value
 * @param {Object} options 同 fmtCurrency
 * @returns {string}
 */
export function fmtSafeCurrency(value, options = {}) {
  if (isEmptyMetric(value)) return '—';
  return fmtCurrency(value, options);
}

/**
 * 安全格式化大數。無效值回傳 '—'。
 * @param {number|null|undefined} value
 * @returns {string}
 */
export function fmtSafeLargeNumber(value) {
  if (isEmptyMetric(value)) return '—';
  return fmtLargeNumber(value);
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
