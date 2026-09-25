/**
 * narrative-hit-rate.js
 *
 * narrative 模組「命中率」對外文案的**唯一權威 helper**（issue #1944 Batch 4, item I23）。
 *
 * 背景：`internal/narrative` 有多個對外欄位名叫 hit rate
 * （`InvestmentModel.HitRate`、`CausalTemplate.HistoricalHitRate`），但 shipped 值
 * 100% 是 `internal/narrative/templates.go` / `knowledge_base.go` 裡的**手寫先驗常數**；
 * 唯一的量測路徑（replay 評估）只在本次 process 記憶體重算且不持久化。
 * 因此前端不得把這些數字寫成「歷史命中率」——那是把先驗常數冒充成量測值。
 *
 * 後端已為每個這類欄位加上 sibling `hit_rate_source`
 * （權威定義：`internal/narrative/hitrate_provenance.go`）。本模組把該字串映射為
 * 對外文字，三個頁面共用；未知／缺欄位一律回誠實的 fallback「來源不明」，
 * 且 `unavailable_*` / `not_populated` 的 0 值絕不呈現為「0%」（0 代表未知，不是零準確率）。
 *
 * 使用方式（頁面）：
 *   import { hitRateDisplayInfo, pickHitRateValue, pickHitRateSource } from '../shared/narrative-hit-rate.js';
 *   const info = hitRateDisplayInfo(pickHitRateValue(m), pickHitRateSource(m));
 *   ... info.text / info.badge / info.note / info.sourceLabel
 */

// ── 來源標籤（對應 internal/narrative/hitrate_provenance.go 的常數）────────
export const HIT_RATE_SOURCES = Object.freeze({
  HANDWRITTEN_PRIOR: 'handwritten_prior',
  REPLAY_EVAL_IN_MEMORY: 'replay_eval_in_memory',
  UNAVAILABLE_NO_SAMPLES: 'unavailable_no_samples',
  UNAVAILABLE_NO_TEMPLATE: 'unavailable_no_template',
  NOT_POPULATED: 'not_populated',
  MIXED: 'mixed',
});

/** 欄位主標籤：刻意不含「歷史」「回測」「實測」字樣。 */
export const HIT_RATE_FIELD_LABEL = '先驗命中率';

/** 主標籤的 tooltip：說明這個數字是什麼、不是什麼。 */
export const HIT_RATE_FIELD_TOOLTIP =
  'narrative 模組的手寫先驗估計，未經校準；這是預設假設值，不是績效證明。';

/** 缺欄位／未知來源時的誠實 fallback。 */
export const HIT_RATE_UNKNOWN_SOURCE_LABEL = '來源不明';

// 完整說明（tooltip 用）
const SOURCE_LABELS = Object.freeze({
  handwritten_prior: '手寫先驗常數（未經校準）',
  replay_eval_in_memory: '本次執行記憶體重算（未持久化，重啟即失效）',
  unavailable_no_samples: '無法量測（無可用樣本）',
  unavailable_no_template: '無法量測（該主題無對應模板）',
  not_populated: '未知（尚無 producer 填入，0 不代表 0% 準確率）',
  mixed: '來源混合（非單一量測值）',
});

// 短標籤（badge 用）
const SOURCE_BADGES = Object.freeze({
  handwritten_prior: '手寫先驗，未校準',
  replay_eval_in_memory: '本次重算，未持久化',
  unavailable_no_samples: '無可用樣本',
  unavailable_no_template: '無對應模板',
  not_populated: '尚無來源填入',
  mixed: '來源混合',
});

/** 這些來源的數值「不是量測結果」，不得當成百分比準確率呈現。 */
const UNMEASURABLE_SOURCES = Object.freeze([
  'unavailable_no_samples',
  'unavailable_no_template',
  'not_populated',
]);

/** 這些來源的數值不能當成單一數字讀（需逐項檢視貢獻者）。 */
const NON_SINGLE_SOURCES = Object.freeze(['mixed']);

const NO_DATA_TEXT = '無資料';
const UNMEASURABLE_TEXT = '無法量測';
const MIXED_TEXT = '來源混合';
const MISSING_TEXT = '—';

/**
 * 來源字串 → 完整說明文字。未知／缺欄位回「來源不明」。
 * @param {*} source `hit_rate_source` 的值
 * @returns {string}
 */
export function hitRateSourceLabel(source) {
  const key = normalizeSource(source);
  return SOURCE_LABELS[key] || HIT_RATE_UNKNOWN_SOURCE_LABEL;
}

/**
 * 來源字串 → 短標籤（badge 文字）。未知／缺欄位回「來源不明」。
 * @param {*} source
 * @returns {string}
 */
export function hitRateSourceBadge(source) {
  const key = normalizeSource(source);
  return SOURCE_BADGES[key] || HIT_RATE_UNKNOWN_SOURCE_LABEL;
}

/**
 * 是否為已知的來源標籤（後端 contract 內的值）。缺欄位／未知字串皆回 false。
 * @param {*} source
 * @returns {boolean}
 */
export function isKnownHitRateSource(source) {
  const key = normalizeSource(source);
  return Object.prototype.hasOwnProperty.call(SOURCE_LABELS, key);
}

/**
 * 該來源的數值是否「不可量測」。`unavailable_no_samples` /
 * `unavailable_no_template` / `not_populated` 皆回 true；
 * `mixed` 因不是單一量測值也回 true（顯示端不應給百分比）。
 * @param {*} source
 * @returns {boolean}
 */
export function isHitRateUnmeasurable(source) {
  const key = normalizeSource(source);
  return UNMEASURABLE_SOURCES.indexOf(key) !== -1 || NON_SINGLE_SOURCES.indexOf(key) !== -1;
}

/**
 * 從 API 物件取出命中率數值（三頁共用同一優先序，避免各頁各自解讀）。
 * 模板用 `historical_hit_rate`，模型用 `hit_rate`。
 * @param {Object|null|undefined} record
 * @returns {*} 原始值（未轉型）
 */
export function pickHitRateValue(record) {
  if (!record || typeof record !== 'object') return null;
  if (record.hit_rate !== null && record.hit_rate !== undefined) return record.hit_rate;
  if (record.historical_hit_rate !== null && record.historical_hit_rate !== undefined) {
    return record.historical_hit_rate;
  }
  return null;
}

/**
 * 從 API 物件取出 `hit_rate_source`（缺欄位回 undefined → helper 給誠實 fallback）。
 * @param {Object|null|undefined} record
 * @returns {*}
 */
export function pickHitRateSource(record) {
  if (!record || typeof record !== 'object') return undefined;
  return record.hit_rate_source;
}

/**
 * 把 0/1 比例或字串數值轉為數字；無法解讀者回 null。
 * 支援 legacy 的 "hits/total" 字串（分母 0 → 代表無資料）。
 * @param {*} raw
 * @returns {{value: number|null, noData: boolean}}
 */
function coerceHitRateValue(raw) {
  if (typeof raw === 'number') {
    if (!Number.isFinite(raw)) return { value: null, noData: false };
    return { value: raw, noData: false };
  }
  if (typeof raw === 'string') {
    const text = raw.trim();
    if (text === '') return { value: null, noData: false };
    const fraction = /^(-?\d+(?:\.\d+)?)\s*\/\s*(\d+(?:\.\d+)?)$/.exec(text);
    if (fraction) {
      const num = Number(fraction[1]);
      const den = Number(fraction[2]);
      if (!Number.isFinite(num) || !Number.isFinite(den)) return { value: null, noData: false };
      if (den === 0) return { value: null, noData: true };
      return { value: num / den, noData: false };
    }
    const n = Number(text);
    if (Number.isFinite(n)) return { value: n, noData: false };
  }
  return { value: null, noData: false };
}

function normalizeSource(source) {
  if (typeof source !== 'string') return '';
  return source.trim().toLowerCase();
}

function formatPct(value, decimals) {
  const d = Number.isInteger(decimals) && decimals >= 0 ? decimals : 1;
  const rounded = Math.round(value * 100 * (10 ** d)) / (10 ** d);
  return `${rounded.toFixed(d)}%`;
}

/**
 * 命中率顯示資訊 —— 三個頁面唯一入口。
 *
 * 回傳物件欄位：
 *   - text        主顯示文字（例如 `65.0%`、`無法量測`、`無資料`、`—`）
 *   - badge       短來源標籤（例如 `手寫先驗，未校準`、`來源不明`）
 *   - note        完整來源說明（tooltip 用）
 *   - sourceLabel 同 note 的別名（完整來源說明）
 *   - kind        `prior` | `measured_in_memory` | `unmeasured` | `mixed` | `no_data` | `missing_value` | `unknown_source`
 *   - hasNumber   text 是否為真實數值（false 者不得當成準確率讀）
 *   - value       轉型後的數值（1.0 = 100%），無數值為 null
 *   - unmeasured  是否「不可量測」（unavailable_* / not_populated / mixed）
 *
 * 誠實規則：
 *   1. `handwritten_prior` 不會被描述成歷史／回測／已量測。
 *   2. `unavailable_*` / `not_populated` 的 0 值顯示「無法量測」，絕不出現 `0.0%`。
 *   3. 缺 `hit_rate_source` 的 0 值顯示「無資料」（0 與未知不可區分）；缺來源的非 0 值
 *      顯示數值但標明「來源不明」，不靜默當成量測值。
 *
 * @param {*} rawValue 原始命中率（可為 number / 字串 / null）
 * @param {*} source `hit_rate_source` 原始值
 * @param {{decimals?: number}} [options]
 * @returns {{text: string, badge: string, note: string, sourceLabel: string, kind: string, hasNumber: boolean, value: number|null, unmeasured: boolean}}
 */
export function hitRateDisplayInfo(rawValue, source, options = {}) {
  const decimals = options && options.decimals !== undefined ? options.decimals : 1;
  const known = isKnownHitRateSource(source);
  const sourceLabel = hitRateSourceLabel(source);
  const badge = hitRateSourceBadge(source);
  const coerced = coerceHitRateValue(rawValue);
  const base = {
    badge,
    note: sourceLabel,
    sourceLabel,
    hasNumber: false,
    value: coerced.value,
    unmeasured: isHitRateUnmeasurable(source),
  };

  // 來源為 unavailable_* / not_populated / mixed：一律不呈現數值。
  if (base.unmeasured) {
    const isMixed = NON_SINGLE_SOURCES.indexOf(normalizeSource(source)) !== -1;
    return Object.assign({}, base, {
      text: isMixed ? MIXED_TEXT : UNMEASURABLE_TEXT,
      kind: isMixed ? 'mixed' : 'unmeasured',
    });
  }

  // legacy "0/0" 或無法解讀的分數字串：無資料，不是 0%。
  if (coerced.noData) {
    return Object.assign({}, base, { text: NO_DATA_TEXT, kind: 'no_data' });
  }

  if (coerced.value === null) {
    return Object.assign({}, base, { text: MISSING_TEXT, kind: 'missing_value' });
  }

  if (!known && coerced.value === 0) {
    // 缺來源 + 0：0 與「未量測」不可區分，不得畫成 0% 準確率。
    return Object.assign({}, base, { text: NO_DATA_TEXT, kind: 'no_data' });
  }

  const kind = known
    ? (normalizeSource(source) === 'replay_eval_in_memory' ? 'measured_in_memory' : 'prior')
    : 'unknown_source';
  return Object.assign({}, base, {
    text: formatPct(coerced.value, decimals),
    kind,
    hasNumber: true,
    value: coerced.value,
  });
}

/**
 * 便利函式：只要主顯示文字。
 * @param {*} rawValue
 * @param {*} source
 * @param {{decimals?: number}} [options]
 * @returns {string}
 */
export function hitRateDisplay(rawValue, source, options = {}) {
  return hitRateDisplayInfo(rawValue, source, options).text;
}

/**
 * 數值文字顏色。**只有「本次記憶體量測」才用績效色階**；
 * 先驗常數與來源不明維持 muted，避免用綠／紅暗示這是已量測的績效。
 * @param {{kind: string, hasNumber: boolean, value: number|null}} info
 * @returns {string} CSS color（var(...)）
 */
export function hitRateTextColor(info) {
  if (!info || !info.hasNumber || typeof info.value !== 'number') return 'var(--muted)';
  if (info.kind !== 'measured_in_memory') return 'var(--muted)';
  if (info.value >= 0.7) return 'var(--color-success)';
  if (info.value >= 0.5) return 'var(--color-warning)';
  return 'var(--color-danger)';
}
