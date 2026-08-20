// client_web/static/js/services/login-gate.js
//
// 會員制 (GUEST_MODE=false) 登入 gate 的頁面分類 + 判定。
//
// 邊界（業主澄清）：匿名 landing = atlas-index（另一站點，促註冊漏斗）；
// atlas client 功能前台需登入（免費旁聽也要帳號）。因此 client 內「公開可看」
// 的只剩登入/註冊頁（與 404 系統頁），其餘所有功能頁都要登入，未登入直接
// 導向 login。
//
// 本模組保持純函數（無 DOM / 無 fetch side-effect），方便單測。

/** 不需登入即可看的頁面（client 內）。 */
export const PUBLIC_PAGES = ['login', 'register', 'errors/404'];

/**
 * 判定某個 page id 是否需登入。
 * @param {string} pageId
 * @returns {boolean} true 表示需要登入
 */
export function pageRequiresLogin(pageId) {
  return PUBLIC_PAGES.indexOf(pageId) === -1;
}

/**
 * Gate：頁面需登入且未登入 → 導向 login。
 * 回傳 true 表示「已擋下並導向 login」，false 表示可繼續。
 *
 * @param {string} pageId
 * @param {() => Promise<boolean>} isLoggedInFn
 * @param {(id: string) => void} switchPageFn
 * @returns {Promise<boolean>}
 */
export async function runLoginGate(pageId, isLoggedInFn, switchPageFn) {
  if (!pageRequiresLogin(pageId)) return false;
  const loggedIn = await isLoggedInFn();
  if (loggedIn) return false;
  switchPageFn('login');
  return true;
}
