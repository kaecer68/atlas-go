// shared_web/static/js/shared/disclosure-state.js
// Persistent expand/collapse state for "展開更多" / "收合" disclosure patterns.
//
// Generic across any section that needs cross-session state preservation
// (e.g. market pulse cards, portfolio details, narrative chains).
// State values are intentionally string-typed ('expanded' | 'collapsed') for
// forward compatibility — add new states by extending the valid set.

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
