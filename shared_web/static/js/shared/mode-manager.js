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
const DEFAULT_MODE = 'simple';

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
  const map = { simple: '簡單', standard: '標準', pro: '專業' };
  return map[mode] || map[DEFAULT_MODE];
}

export function selectMode(mode) {
  if (!VALID_MODES.includes(mode)) mode = DEFAULT_MODE;
  setMode(mode);
  updateUI();
}

function updateUI() {
  const btns = document.querySelectorAll('.mode-btn');
  const current = getMode();
  btns.forEach(b => {
    const match = b.dataset.mode === current;
    b.classList.toggle('active', match);
    b.setAttribute('aria-pressed', String(match));
  });
}

export function init() {
  if (typeof document === 'undefined') return;

  let stored;
  try { stored = localStorage.getItem(STORAGE_KEY); } catch (_) {}
  const mode = VALID_MODES.includes(stored) ? stored : DEFAULT_MODE;
  document.documentElement.setAttribute('data-atlas-mode', mode);

  document.querySelectorAll('.mode-btn').forEach(btn => {
    btn.addEventListener('click', () => selectMode(btn.dataset.mode));
  });
  updateUI();
}

if (typeof window !== 'undefined') {
  window.toggleSimplifiedMode = () => {}; // Legacy no-op — replaced by selectMode
}
