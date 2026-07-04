/**
 * 向後相容層 — 獨立運作，不 import mode-manager.js。
 * 與 mode-manager.js 共用 localStorage('atlas-mode') + data-atlas-mode attribute。
 */

const MODES = ['simple', 'standard', 'pro'];
const LABELS = { simple: '🔰 簡易', standard: '📊 標準', pro: '🔬 專業' };

function getMode() {
  if (typeof document === 'undefined') return 'standard';
  const m = document.documentElement.getAttribute('data-atlas-mode');
  return MODES.includes(m) ? m : 'standard';
}

function setMode(mode) {
  if (typeof document === 'undefined' || !MODES.includes(mode)) return;
  document.documentElement.setAttribute('data-atlas-mode', mode);
  if (mode === 'simple') document.documentElement.setAttribute('data-simplified', 'true');
  else document.documentElement.removeAttribute('data-simplified');
  try { localStorage.setItem('atlas-mode', mode); } catch (_) {}
}

export function isEnabled() { return getMode() === 'simple'; }

function updateButton(btn) {
  if (!btn) return;
  const m = getMode();
  btn.textContent = LABELS[m] || LABELS.standard;
  btn.setAttribute('aria-pressed', String(m !== 'simple'));
}

export function toggle() {
  const cur = getMode();
  const idx = MODES.indexOf(cur);
  setMode(MODES[(idx + 1) % MODES.length]);
  updateButton(document.getElementById('simplifiedToggle'));
}

export function init() {
  if (typeof document === 'undefined') return;
  const btn = document.getElementById('simplifiedToggle');
  if (!btn) return;
  btn.addEventListener('click', toggle);
  updateButton(btn);
}

if (typeof window !== 'undefined') {
  window.toggleSimplifiedMode = toggle;
}
