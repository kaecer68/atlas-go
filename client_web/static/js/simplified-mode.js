/**
 * Atlas Dashboard - Simplified Mode Toggle
 *
 * 依 audit_assets/phase2-retail-investor-landing-audit.md §3.2 落地雙軌介面：
 *   進階使用者（顯示所有 HHI、Drawdown、Sharpe 等指標）
 *   ↔ 一般投資人（隱藏進階指標，保留 Risk Badge + Trust Footer）
 *
 * 切換 :root 的 data-simplified 屬性，並由 base/simplified.css 隱藏 .advanced-only 元素。
 * 狀態持久化到 localStorage（key: 'atlas-simplified'）。
 *
 * 為避免 FOUC（Flash of Unstyled Content），index.html <head> 內有一段同步早期腳本
 * 會在 CSS 載入前先設定 data-simplified 屬性。
 */

const STORAGE_KEY = 'atlas-simplified';
const root = typeof document !== 'undefined' ? document.documentElement : null;

export function isEnabled() {
  return Boolean(root && root.getAttribute('data-simplified') === 'true');
}

export function setEnabled(next) {
  if (!root) return;
  if (next) {
    root.setAttribute('data-simplified', 'true');
  } else {
    root.removeAttribute('data-simplified');
  }
  try {
    localStorage.setItem(STORAGE_KEY, next ? '1' : '0');
  } catch (_) {
    // localStorage 不可用（隱私模式 / 禁用）時靜默忽略
  }
}

function updateButton(btn, enabled) {
  if (!btn) return;
  btn.setAttribute('aria-pressed', String(enabled));
  btn.setAttribute('aria-label', enabled ? '切換到簡化模式' : '切換到完整模式');
  btn.textContent = enabled ? '🔬 完整模式' : '🔰 簡化模式';
  btn.title = enabled
    ? '切換到完整模式（顯示所有指標）'
    : '切換到簡化模式（隱藏進階指標）';
}

export function toggle() {
  const next = !isEnabled();
  setEnabled(next);
  const btn = typeof document !== 'undefined'
    ? document.getElementById('simplifiedToggle')
    : null;
  updateButton(btn, next);
}

export function init() {
  if (typeof document === 'undefined') return;
  const btn = document.getElementById('simplifiedToggle');
  if (!btn) return;
  btn.addEventListener('click', toggle);
  updateButton(btn, isEnabled());
}

if (typeof window !== 'undefined') {
  window.toggleSimplifiedMode = toggle;
}
