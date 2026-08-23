// shared_web/static/js/shared/fetch-wrapper.js
//
// Atlas 401 攔截器 — 提供 client_web 與 admin_web 共用的 fetch 包裝。
// 攔截 HTTP 401 → 依請求類型分流：
//   - mutating 401（POST/PUT/DELETE/PATCH，通常是無效/缺 X-API-Key）
//     → 觸發 onApiKeyRequired()（admin 開 apiKeyModal 輸入管理員 API Key）
//   - 其餘 401（user-auth 失效）→ 觸發 onUnauthorized（預設 invalidateAuth）
//     + 自動跳轉登入頁（client_web 保留此路徑）
// 未提供 onApiKeyRequired 時（client_web），mutating 401 亦走 onUnauthorized
// + 跳轉登入頁，維持既有行為。
//
// 使用：
//   import { install401Interceptor } from '../shared/fetch-wrapper.js';
//   import { invalidateAuth } from './services/auth.js';
//   install401Interceptor({
//     loginPageId: 'login',          // client 用 login；admin 用 login
//     excludedPages: ['login', 'register'],
//     onUnauthorized: invalidateAuth, // shared_web 預設行為
//     onApiKeyRequired: showApiKeyPrompt, // admin：mutating 401 開 apiKeyModal
//     switchPage: window.switchPage,  // 從 outer scope 傳入避免循環依賴
//   });
//
// 重複呼叫 install401Interceptor 是冪等的：第二次呼叫回傳 noop uninstall，
// 不會重複包裝 window.fetch。

const MUTATING_METHODS = ['POST', 'PUT', 'DELETE', 'PATCH'];

/**
 * @typedef {Object} InterceptorOptions
 * @property {string} [loginPageId='login']        401 觸發時要跳轉的 page id（無 loginRedirectUrl 時）
 * @property {string|(() => string)} [loginRedirectUrl]  401 時導向的外部登入 URL（優於 switchPage(loginPageId)；可用函式延遲計算）
 * @property {string[]} [excludedPages=[]]          已是登入相關頁時不再跳轉
 * @property {() => void} [onUnauthorized]          401 觸發時的副作用（預設 invalidateAuth）
 * @property {() => void} [onApiKeyRequired]        mutating 401（缺/無效 X-API-Key）時觸發（admin 開 apiKeyModal）
 * @property {typeof fetch} [fetchImpl]             自訂 fetch（測試用，預設 window.fetch）
 * @property {Window} [windowObj]                   自訂 window（測試用，預設 globalThis.window）
 * @property {(id: string) => void} [switchPage]    跳轉 fn（測試用或 lazy 注入）
 */

/**
 * Install 401 interceptor on window.fetch.
 * @param {InterceptorOptions} [options={}]
 * @returns {() => void} uninstall function
 */
export function install401Interceptor(options = {}) {
  const {
    loginPageId = 'login',
    loginRedirectUrl,
    excludedPages = [],
    onUnauthorized,
    onApiKeyRequired,
    fetchImpl,
    windowObj,
    switchPage,
  } = options;

  const win = windowObj || (typeof window !== 'undefined' ? window : null);
  if (!win) {
    throw new Error('install401Interceptor: window not available');
  }
  if (typeof win.fetch !== 'function' && !fetchImpl) {
    throw new Error('install401Interceptor: window.fetch not available');
  }
  const baseFetch = fetchImpl || win.fetch.bind(win);
  if (!baseFetch) {
    throw new Error('install401Interceptor: base fetch not available');
  }

  // Idempotency guard: if already installed, return noop uninstall.
  if (win.__atlasFetch401Installed) {
    return function uninstallNoop() {};
  }
  win.__atlasFetch401Installed = true;

  const origFetch = win.fetch;

  win.fetch = function patchedFetch(input, init) {
    return origFetch.call(win, input, init).then(function (res) {
      if (res && res.status === 401) {
        try {
          // 401 分流：mutating 請求（POST/PUT/DELETE/PATCH）的 401 通常是
          // 無效/缺 X-API-Key → 有 onApiKeyRequired 時觸發它（admin 開
          // apiKeyModal），不再跳登入頁；其餘 401（user-auth 失效）維持
          // 既有 onUnauthorized + switchPage(login)（client_web 路徑）。
          var method = 'GET';
          if (init && typeof init.method === 'string') {
            method = init.method.toUpperCase();
          } else if (input && typeof input.method === 'string') {
            method = input.method.toUpperCase();
          }
          var isMutating = MUTATING_METHODS.indexOf(method) !== -1;
          if (isMutating && typeof onApiKeyRequired === 'function') {
            onApiKeyRequired();
          } else {
            if (typeof onUnauthorized === 'function') {
              onUnauthorized();
            }
            // Compute current page id from path, supporting both /client/ and /admin/.
            var currentPage = 'home';
            if (win.location && win.location.pathname) {
              currentPage = win.location.pathname
                .replace(/^\/(client|admin)\/?/, '')
                .replace(/\?.*$/, '') || 'home';
            }
            if (excludedPages.indexOf(currentPage) === -1) {
              // 外部登入 URL（如 member.goluck.uk）優於站內 switchPage —
              // client 的 login/register 頁已移除，401 一律導 go-member。
              if (loginRedirectUrl != null) {
                var redirectUrl = typeof loginRedirectUrl === 'function'
                  ? loginRedirectUrl()
                  : loginRedirectUrl;
                // eslint-disable-next-line no-console
                console.warn('[auth] 401 detected, redirecting to ' + redirectUrl);
                if (win.location) win.location.href = redirectUrl;
              } else if (typeof switchPage === 'function') {
                // eslint-disable-next-line no-console
                console.warn(
                  '[auth] 401 detected, redirecting to ' + loginPageId,
                );
                switchPage(loginPageId);
              }
            }
          }
        } catch (e) {
          // eslint-disable-next-line no-console
          console.warn('[auth] 401 handler error:', e);
        }
      }
      return res;
    });
  };

  return function uninstall() {
    if (win.__atlasFetch401Installed) {
      win.fetch = origFetch;
      win.__atlasFetch401Installed = false;
    }
  };
}

/**
 * Check whether the interceptor is already installed.
 * @param {Window} [windowObj]
 * @returns {boolean}
 */
export function is401InterceptorInstalled(windowObj) {
  const win = windowObj || (typeof window !== 'undefined' ? window : null);
  return !!(win && win.__atlasFetch401Installed);
}
