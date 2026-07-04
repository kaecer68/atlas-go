/**
 * Atlas Dashboard — 三級使用者模式管理器
 *
 * 支援 simple / standard / pro 三個層級，取代舊的 simplified boolean toggle。
 * 狀態持久化到 localStorage（key: 'atlas-mode'）。
 *
 * simple:   一般投資人 — 簡化首頁、隱藏進階指標、最少 API 呼叫
 * standard: 預設模式 — 完整首頁、全部指標
 * pro:      專業投資人 — 完整首頁 + 額外風控/量化指標
 */

const STORAGE_KEY = 'atlas-mode';
const VALID_MODES = ['simple', 'standard', 'pro'];
const DEFAULT_MODE = 'standard';

export function getMode() {
  if (typeof document === 'undefined') return DEFAULT_MODE;
  const attr = document.documentElement.getAttribute('data-atlas-mode');
  return VALID_MODES.includes(attr) ? attr : DEFAULT_MODE;
}

export function setMode(mode) {
  if (typeof document === 'undefined') return;
  if (!VALID_MODES.includes(mode)) {
    console.warn('[mode-manager] invalid mode:', mode, '— falling back to', DEFAULT_MODE);
    mode = DEFAULT_MODE;
  }
  document.documentElement.setAttribute('data-atlas-mode', mode);
  try {
    localStorage.setItem(STORAGE_KEY, mode);
  } catch (_) {
    // localStorage 不可用時靜默忽略
  }
}

export function isSimple()  { return getMode() === 'simple'; }
export function isStandard(){ return getMode() === 'standard'; }
export function isPro()     { return getMode() === 'pro'; }

export function modeLabel(mode) {
  const map = { simple: '簡易', standard: '標準', pro: '專業' };
  return map[mode] || map[DEFAULT_MODE];
}

export function nextMode(current) {
  const idx = VALID_MODES.indexOf(current || DEFAULT_MODE);
  return VALID_MODES[(idx + 1) % VALID_MODES.length];
}

export function toggleMode() {
  const current = getMode();
  setMode(nextMode(current));
  updateUI();
}

function updateUI() {
  const btn = document.getElementById('simplifiedToggle');
  if (!btn) return;
  const mode = getMode();
  const labels = { simple: '🔰 簡易', standard: '📊 標準', pro: '🔬 專業' };
  btn.textContent = labels[mode] || labels[DEFAULT_MODE];
  btn.setAttribute('aria-pressed', String(mode !== 'simple'));
  btn.setAttribute('aria-label', `目前模式：${modeLabel(mode)} — 點擊切換`);
}

export function init() {
  if (typeof document === 'undefined') return;

  // Restore from localStorage on cold start
  let stored;
  try { stored = localStorage.getItem(STORAGE_KEY); } catch (_) {}
  if (VALID_MODES.includes(stored)) {
    document.documentElement.setAttribute('data-atlas-mode', stored);
  }

  // Wire up toggle button
  const btn = document.getElementById('simplifiedToggle');
  if (btn) {
    btn.addEventListener('click', toggleMode);
    updateUI();
  }
}

if (typeof window !== 'undefined') {
  window.toggleSimplifiedMode = toggleMode;
}
