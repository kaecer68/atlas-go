// client_web/static/js/services/login-gate.js
//
// 登入 gate（2026-08-21 業主決策：不強迫登入 + 自願登入 + 閒置提醒）。
//
// 邊界（業主澄清後的新政策）：
//   - atlas-index 是匿名 landing（促註冊漏斗）。
//   - atlas client 不再全站強制登入：公開內容（今日判讀/看板/報價/宏觀等）
//     允許未登入瀏覽，top bar 顯示自願 Login 按鈕。
//   - 只有需要會員/個人資料的頁面（組合持倉、升級 Premium）才啟用 gate，
//     未登入導向 login。
//   - my-signals 有自己的 401 逃脫路徑（「此功能需要登入」+ 回公開內容），
//     故不列入 gated，維持 UX audit P0 修復。
//   - API 401 資料保護仍由 install401Interceptor 負責（main.js 把公開頁
//     加入 excludedPages，避免 initAuth 的 profile check 把匿名訪客踢走）。
//
// 本模組保持純函數（無 DOM / 無 fetch side-effect），方便單測。

/** 不需登入即可瀏覽的頁面（client 內）。 */
export const PUBLIC_PAGES = [
  'login', 'register', 'errors/404',
  // 公開內容頁（後端對應 endpoint 皆為 shared.Get 公開 API）
  'home', 'capital_board', 'stock-quote', 'narrative', 'crossmarket',
  'industry', 'retail_sentiment', 'pipeline', 'strategies', 'methodology',
  'mcp', 'performance-report', 'my-signals',
];

/** 明確需要會員資料 / 付費才可進入的頁面 → gate 強制登入。 */
export const GATED_PAGES = ['portfolio', 'premium'];

/**
 * 判定某個 page id 是否需登入。
 * @param {string} pageId
 * @returns {boolean} true 表示需要登入
 */
export function pageRequiresLogin(pageId) {
  return GATED_PAGES.indexOf(pageId) !== -1;
}

/**
 * Gate：gated 頁面且未登入 → 導向 login。
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
