/**
 * Atlas Dashboard — Progressive Disclosure 狀態管理器
 *
 * 取代舊的「三級投資人角色模式」(PR #946 引入、PR #967 移除)，改為單一模式 + 區塊摺疊。
 * 狀態持久化到 localStorage（prefix: 'atlas-disclosure-'）。
 *
 * collapsed: 預設 — 僅渲染核心卡片、最少 API 呼叫
 * expanded:  用戶主動展開 — 顯示進階卡片
 *
 * 設計目標：兌現原 mode-manager.js「最少 API 呼叫」承諾。目前透過 CSS 隱藏實現，
 * 真實 API lazy-load 待後端 /api/macro/snapshot 支援 fields 過濾後實作
 * (見 shared_web/static/js/pages/home.js renderMarketPulse 中的 TODO 註解)。
 *
 * State values 為字串型別 ('expanded' | 'collapsed')，向後相容；新增狀態請擴充 VALID_STATES。
 */

const STORAGE_PREFIX = 'atlas-disclosure-';
const VALID_STATES = new Set(['expanded', 'collapsed']);
const DEFAULT_STATE = 'collapsed';

function safeLocalStorage() {
  if (typeof window === 'undefined') return null;
  try {
    return window.localStorage;
  } catch {
    // Storage disabled (private mode, file:// origin, etc.) — fail open
    return null;
  }
}

/**
 * Read the persisted disclosure state for a named section.
 * Returns the stored state, or the provided default if absent/invalid.
 * @param {string} sectionKey - Logical section name (e.g. 'market-pulse')
 * @param {string} [defaultState='collapsed']
 * @returns {string} 'expanded' | 'collapsed' | the provided default
 */
export function getDisclosureState(sectionKey, defaultState = DEFAULT_STATE) {
  const ls = safeLocalStorage();
  if (!ls) return defaultState;
  try {
    const stored = ls.getItem(STORAGE_PREFIX + sectionKey);
    return VALID_STATES.has(stored) ? stored : defaultState;
  } catch {
    return defaultState;
  }
}

/**
 * Persist the disclosure state for a named section.
 * Silently no-ops if localStorage is unavailable or write fails.
 * @param {string} sectionKey
 * @param {string} state - 'expanded' | 'collapsed'
 */
export function setDisclosureState(sectionKey, state) {
  if (!VALID_STATES.has(state)) return;
  const ls = safeLocalStorage();
  if (!ls) return;
  try {
    ls.setItem(STORAGE_PREFIX + sectionKey, state);
  } catch {
    // Quota exceeded or storage disabled — silently ignore
  }
}
