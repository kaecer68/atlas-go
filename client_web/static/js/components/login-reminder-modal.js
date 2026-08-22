// client_web/static/js/components/login-reminder-modal.js
//
// 登入提醒 modal（Track A：不強迫登入，改為閒置提醒）。
//
// 行為：
//   - showLoginReminderModal()：未登入時彈出「溫馨提醒：登入以解鎖完整功能」。
//     「立即登入」→ 跳轉 member.goluck.uk/login?redirect=<current>
//     「再逛一下」→ 關閉 modal，並由 main.js 傳入的 onKeepBrowsing 重設 idle。
//   - localStorage 去重：24 小時內不彈第二次（key = atlasLoginReminderAt）。
//     註：此 flag 只是 UI 提醒去重，不是登入狀態快取（登入狀態仍只走
//     cookie + /api/user/profile，維持 SSO 一致性）。
//   - 已登入時呼叫不會彈出（由 auth.js isLoggedIn 判定）。
//
// 樣式：玻璃面板 + 金色 CTA（見 shared_web/static/css/components/topbar.css
// 的 .login-reminder-* 區段），DOM 由本模組動態建立，不依賴 index.html。

// 註：auth.js 實際位於 shared_web（esbuild shared-plugin 會 fallback）；
// 這裡用顯式 shared 相對路徑，讓 node --test 也能直接 import。
import { getRedirectUrl, isLoggedIn } from '../../../../shared_web/static/js/services/auth.js';

export const REMINDER_STORAGE_KEY = 'atlasLoginReminderAt';
export const REMINDER_INTERVAL_MS = 24 * 60 * 60 * 1000; // 24h 去重
const MODAL_ID = 'loginReminderModal';

/**
 * 純函數：判斷 24h 去重是否允許彈出（測試用）。
 * @param {number} nowMs
 * @param {string|null} lastReminderAt  localStorage 值（ISO string 或 null）
 * @returns {boolean}
 */
export function shouldShowReminder(nowMs, lastReminderAt) {
  if (!lastReminderAt) return true;
  const last = new Date(lastReminderAt).getTime();
  if (Number.isNaN(last)) return true;
  return nowMs - last >= REMINDER_INTERVAL_MS;
}

function _storage() {
  if (typeof localStorage !== 'undefined') return localStorage;
  return null;
}

function _readLastReminder() {
  const s = _storage();
  if (!s) return null;
  try { return s.getItem(REMINDER_STORAGE_KEY); } catch (e) { return null; }
}

function _writeLastReminder(now) {
  const s = _storage();
  if (!s) return;
  try { s.setItem(REMINDER_STORAGE_KEY, new Date(now).toISOString()); } catch (e) { /* 私密模式等忽略 */ }
}

function _removeModal() {
  const el = document.getElementById(MODAL_ID);
  if (el && el.parentNode) el.parentNode.removeChild(el);
}

/**
 * 建立 modal DOM（若不存在）。
 * @param {() => void} [onKeepBrowsing] 「再逛一下」回呼（重設 idle 計時）
 * @returns {HTMLElement|null}
 */
function _buildModal(onKeepBrowsing) {
  _removeModal();
  const overlay = document.createElement('div');
  overlay.id = MODAL_ID;
  overlay.className = 'login-reminder-overlay';
  overlay.setAttribute('role', 'dialog');
  overlay.setAttribute('aria-modal', 'true');
  overlay.setAttribute('aria-labelledby', MODAL_ID + 'Title');
  overlay.innerHTML =
    '<div class="login-reminder-card">' +
      '<button type="button" class="login-reminder-close" aria-label="關閉提醒">×</button>' +
      '<div class="login-reminder-icon" aria-hidden="true">🔐</div>' +
      '<h2 id="' + MODAL_ID + 'Title" class="login-reminder-title">溫馨提醒：登入以解鎖完整功能</h2>' +
      '<p class="login-reminder-desc">登入後可解鎖個人儀表板、組合持倉與策略推薦等會員功能。</p>' +
      '<div class="login-reminder-actions">' +
        '<a class="login-reminder-btn login-reminder-btn--primary" href="' + getRedirectUrl() + '">立即登入</a>' +
        '<button type="button" class="login-reminder-btn login-reminder-btn--ghost">再逛一下</button>' +
      '</div>' +
    '</div>';

  const close = function () { _removeModal(); };
  overlay.addEventListener('click', function (e) {
    if (e.target === overlay) close();
  });
  overlay.querySelector('.login-reminder-close').addEventListener('click', close);
  overlay.querySelector('.login-reminder-btn--ghost').addEventListener('click', function () {
    close();
    if (typeof onKeepBrowsing === 'function') onKeepBrowsing();
  });
  document.body.appendChild(overlay);
  return overlay;
}

/**
 * 彈出登入提醒 modal。
 * 去重 + 已登入防呆：不符合條件時回傳 false（不彈）。
 *
 * @param {object} [options]
 * @param {() => void} [options.onKeepBrowsing] 「再逛一下」回呼（重設 idle 計時）
 * @returns {Promise<boolean>} true = 已彈出
 */
export async function showLoginReminderModal(options = {}) {
  if (typeof document === 'undefined') return false;
  try {
    const loggedIn = await isLoggedIn();
    if (loggedIn) return false;
  } catch (e) {
    // profile check 失敗視同未登入，仍可提醒
  }
  const now = Date.now();
  if (!shouldShowReminder(now, _readLastReminder())) return false;
  _writeLastReminder(now);
  const el = _buildModal(options.onKeepBrowsing);
  if (!el) return false;
  return true;
}

/**
 * 程式化關閉 modal（測試/其他元件用）。
 */
export function closeLoginReminderModal() {
  if (typeof document === 'undefined') return;
  _removeModal();
}
