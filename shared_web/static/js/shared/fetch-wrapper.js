// shared_web/static/js/shared/fetch-wrapper.js
//
// Atlas 401 攔截器 — 提供 client_web 與 admin_web 共用的 fetch 包裝。
// 攔截 HTTP 401 → 觸發 onUnauthorized（預設 invalidateAuth）+ 自動跳轉登入頁。
//
// 使用：
//   import { install401Interceptor } from '../shared/fetch-wrapper.js';
//   import { invalidateAuth } from './services/auth.js';
//   install401Interceptor({
//     loginPageId: 'login',          // client 用 login；admin 用 login
//     excludedPages: ['login', 'register'],
//     onUnauthorized: invalidateAuth, // shared_web 預設行為
//     switchPage: window.switchPage,  // 從 outer scope 傳入避免循環依賴
//   });
//
// 重複呼叫 install401Interceptor 是冪等的：第二次呼叫回傳 noop uninstall，
// 不會重複包裝 window.fetch。

/**
 * @typedef {Object} InterceptorOptions
 * @property {string} [loginPageId='login']        401 觸發時要跳轉的 page id
 * @property {string[]} [excludedPages=[]]          已是登入相關頁時不再跳轉
 * @property {() => void} [onUnauthorized]          401 觸發時的副作用（預設 invalidateAuth）
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
    excludedPages = [],
    onUnauthorized,
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
          if (
            excludedPages.indexOf(currentPage) === -1 &&
            typeof switchPage === 'function'
          ) {
            // eslint-disable-next-line no-console
            console.warn(
              '[auth] 401 detected, redirecting to ' + loginPageId,
            );
            switchPage(loginPageId);
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
