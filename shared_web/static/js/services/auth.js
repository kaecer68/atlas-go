/**
 * auth.js — shared authentication service for admin_web and client_web.
 *
 * Dual-token mode:
 *   1. Cookie-based JWT (HttpOnly, auto-sent via credentials: 'include')
 *   2. Memory-cached token (from POST /api/auth/login response, for Authorization header)
 *
 * Guest mode (GUEST_MODE = true):
 *   When the backend is started without ATLAS_REQUIRE_USER_AUTH, every
 *   visitor is treated as an anonymous TierFree "guest". login/register
 *   are still callable, but the UI hides the auth sidebar entries and
 *   isLoggedIn() returns true for everyone. Flip back to false when
 *   commercialising.
 *
 * Usage:
 *   import { login, register, logout, isLoggedIn, getTier, getClaims, getToken, initAuth, renderTopBar, getRedirectUrl } from '../services/auth.js';
 */

import { postJSON, getJSON } from '../shared/app-utils.js';

const GUEST_MODE = false;

let _token = null;
let _claims = null;
let _authChecked = false;
let _authValid = false;

const PROFILE_URL = '/api/user/profile';
const LOGIN_URL = '/api/auth/login';
const REGISTER_URL = '/api/auth/register';

// ─── Top-bar brand / member-site constants (Track A: voluntary login UX) ───
// MEMBER_BASE_URL 是 go-member 會員中心域名；BRAND_LOGO_URL 共用
// atlas-index 的 ATLAS-GO 螃蟹 logo（root-relative，production 由 caddy
// 導到 atlas-index 靜態檔）。兩者集中在此，避免散落 hardcode。
export const MEMBER_BASE_URL = 'https://member.goluck.uk';
export const BRAND_LOGO_URL = '/assets/img/ATLAS-GO.png';

/**
 * Login with email + password. Stores JWT cookie (HttpOnly) and caches token in memory.
 * Returns { user } on success, throws on failure.
 */
export async function login(email, password) {
  const res = await postJSON(LOGIN_URL, { email, password });
  if (res.token) {
    _token = res.token;
    _claims = parseJWT(res.token);
    _authValid = true;
  }
  _authChecked = true;
  return res;
}

/**
 * Register a new user account.
 */
export async function register(email, password) {
  const res = await postJSON(REGISTER_URL, { email, password });
  if (res.token) {
    _token = res.token;
    _claims = parseJWT(res.token);
    _authValid = true;
  }
  _authChecked = true;
  return res;
}

/**
 * Logout: clear local state and ask the server to clear the HttpOnly cookie.
 */
export async function logout() {
  try {
    await postJSON('/api/auth/logout', {});
  } catch (e) {
    // Best-effort server cookie clear; local state is cleared regardless.
  }
  _token = null;
  _claims = null;
  _authValid = false;
  _authChecked = true;
}

/**
 * Check whether the user has a valid session by calling GET /api/user/profile.
 * Caches the result to avoid repeated HTTP calls.
 */
export async function isLoggedIn() {
  if (_authChecked) return _authValid;
  try {
    const profile = await getJSON(PROFILE_URL);
    const email = profile.email || (profile.user && profile.user.email);
    const tier = profile.effective_tier || profile.tier || (profile.user && profile.user.tier);
    if (profile && email) {
      _authValid = true;
      if (!_claims) _claims = {};
      _claims.email = email;
      if (tier) _claims.tier = tier;
    }
  } catch (e) {
    _authValid = false;
    _claims = null;
    _token = null;
  }
  _authChecked = true;
  return _authValid;
}

/**
 * Get the current user's subscription tier.
 * Returns 'free', 'registered', 'premium', or null if not logged in.
 */
export async function getTier() {
  if (!_authChecked) await isLoggedIn();
  if (!_authValid) return null;
  if (_claims && _claims.tier) return _claims.tier;
  return 'free';
}

/**
 * Get the cached JWT claims object.
 */
export function getClaims() {
  return _claims;
}

/**
 * Get the memory-cached token for Authorization header.
 */
export function getToken() {
  return _token;
}

/**
 * Force re-check of auth state (e.g., after tier change detected in API response).
 */
export function invalidateAuth() {
  _authChecked = false;
  _authValid = false;
}

/**
 * Initialize auth state. Called once on page load.
 * Checks cookie-based JWT validity, handles expired tokens.
 *
 * In GUEST_MODE, every visitor is auto-promoted to anonymous TierFree
 * when no JWT is present so the rest of the app can run unmodified.
 */
// Track A: go-member 登入回跳 — URL 帶 ?token= 時先交換成 HttpOnly session
// cookie（/api/auth/sso），再清掉 URL 上的 token 避免殘留。go-member 的
// email/OAuth 登入流程都會把 token 附在 redirect 上。
async function consumeSSOTokenIfPresent() {
  if (typeof window === 'undefined' || !window.location) return;
  const params = new URLSearchParams(window.location.search);
  const token = params.get('token');
  if (!token || token.length < 20) return;
  const redirect = params.get('redirect') || window.location.pathname + window.location.search.replace(/[?&]token=[^&]*/, '').replace(/[?&]redirect=[^&]*/, '');
  try {
    const data = await getJSON('/api/auth/sso?token=' + encodeURIComponent(token) + '&redirect=' + encodeURIComponent(redirect));
    if (data && data.ok === 'true') {
      // cookie 已設，跳到乾淨 URL（不含 token）
      window.location.href = data.redirect || redirect;
      return;
    }
  } catch (e) {
    // sso 失敗視同未登入，不阻斷瀏覽
  }
  try {
    window.history.replaceState({}, '', redirect);
  } catch (e) {}
}

export async function initAuth() {
  await consumeSSOTokenIfPresent();
  const loggedIn = await isLoggedIn();
  if (!loggedIn) {
    const token = readCookie('token');
    if (token) {
      const claims = parseJWT(token);
      if (claims && claims.exp) {
        const expired = Date.now() >= claims.exp * 1000;
        if (expired) {
          _token = null;
          _claims = null;
          _authValid = false;
          _authChecked = true;
        }
      }
    }
  }
  if (GUEST_MODE && !_authValid) {
    _authValid = true;
    _claims = { tier: 'free', email: '', sub: 0 };
    _token = 'guest';
    _authChecked = true;
  }
  return _authValid;
}

// ─── Internal helpers ───

function parseJWT(token) {
  try {
    const payload = token.split('.')[1];
    if (!payload) return null;
    return JSON.parse(atob(payload.replace(/-/g, '+').replace(/_/g, '/')));
  } catch (_) {
    return null;
  }
}

function readCookie(name) {
  const match = document.cookie.match(new RegExp('(^| )' + name + '=([^;]+)'));
  return match ? decodeURIComponent(match[2]) : null;
}

// ─── Sidebar nav state ───

/**
 * Update sidebar auth navigation based on current login state.
 * Call after initAuth() completes or after login/logout.
 *
 * In GUEST_MODE the entire "帳戶" sidebar section is hidden — the
 * login/register/profile/premium/logout nav block is meaningless when
 * every visitor is anonymous.
 */
export async function renderNavState() {
  const loggedIn = await isLoggedIn();
  const tier = await getTier();

  if (GUEST_MODE) {
    const accountSection = document.getElementById('navAccountSection');
    if (accountSection) accountSection.classList.add('hidden');
    return;
  }

  const guestItems = document.querySelectorAll('.nav-guest');
  const userItems = document.querySelectorAll('.nav-user');
  const tierBadge = document.getElementById('navTierBadge');
  const tierLabel = document.getElementById('navTierLabel');

  guestItems.forEach(function(el) { el.classList.toggle('hidden', loggedIn); });
  userItems.forEach(function(el) { el.classList.toggle('hidden', !loggedIn); });

  if (tierBadge && tier) {
    tierBadge.textContent = tier === 'premium' ? 'Premium' : tier === 'registered' ? '已註冊' : '免費';
    tierBadge.className = 'tier-badge tier-' + tier;
  }
  if (tierLabel && tier) {
    tierLabel.textContent = tierLabelText(tier);
    tierLabel.className = 'tier-label tier-' + tier;
    tierLabel.classList.remove('hidden');
  }

  // Track A：top bar 的 logo + 動態 menu 隨登入狀態一起更新（單一入口）。
  await renderTopBar();
}

/**
 * Tier → 中文/英文 label（sidebar badge 與 top bar user chip 共用）。
 * @param {string|null} tier
 * @returns {string}
 */
function tierLabelText(tier) {
  if (tier === 'premium') return 'Premium';
  if (tier === 'registered') return '已註冊';
  return '免費';
}

// ─── Top bar（Track A：自願登入 + 雙模式 menu）───

/**
 * 行銷（未登入）模式 menu：對齊 atlas-index 的 4 個主導覽。
 * 連結指向 atlas-index landing 的對應錨點（production 同源 root）或
 * go-member 註冊入口。
 */
const MARKETING_MENU = [
  { label: 'Why Atlas', href: '/#why-atlas' },
  { label: '會員方案', href: '/#pricing-detail' },
  { label: '社群學習', href: MEMBER_BASE_URL + '/login' },
  { label: '問答提示', href: '/#faq' },
];

/**
 * 會員（已登入）模式 menu：對齊 go-member dashboard 的 4 個分頁。
 * go-member dashboard 用 location.hash 切 tab（#membership / #vip / #bot）。
 */
const MEMBER_MENU = [
  { label: '控制台', href: MEMBER_BASE_URL + '/dashboard' },
  { label: '會員權益', href: MEMBER_BASE_URL + '/dashboard#membership' },
  { label: '升級 VIP Room', href: MEMBER_BASE_URL + '/dashboard#vip' },
  { label: '私人 AI 機器人', href: MEMBER_BASE_URL + '/dashboard#bot' },
];

/**
 * 組「自願登入」連結：member.goluck.uk/login?redirect=<目前完整 URL>。
 * go-member 登入成功後會以 redirect 值導回 atlas（需絕對 URL）。
 *
 * @param {string} [currentUrl] 測試/覆寫用；預設由 window.location 計算。
 * @returns {string}
 */
export function getRedirectUrl(currentUrl) {
  let current = currentUrl;
  if (current === undefined) {
    const win = typeof window !== 'undefined' ? window : null;
    if (win && win.location && win.location.pathname) {
      current = win.location.origin + win.location.pathname + win.location.search;
    } else {
      current = 'https://atlas.goluck.uk/client/home';
    }
  }
  return MEMBER_BASE_URL + '/login?redirect=' + encodeURIComponent(current);
}

/**
 * 渲染 top bar 的動態 menu 容器（#topbarMenu）：
 *   - 未登入 → marketing menu + 金色 Login 按鈕
 *   - 已登入 → member menu + user email + tier badge + 登出
 * 在 renderNavState() 末尾自動呼叫；也可獨立呼叫（如登出後重渲染）。
 *
 * 不做任何 localStorage 快取登入狀態（保持 SSO 一致性）。
 */
export async function renderTopBar() {
  const loggedIn = await isLoggedIn();
  const tier = await getTier();
  const container = typeof document !== 'undefined' ? document.getElementById('topbarMenu') : null;
  if (!container) return;

  if (loggedIn) {
    const claims = getClaims() || {};
    const email = claims.email || '會員';
    const tierText = tierLabelText(tier);
    const tierCls = tier === 'premium' ? 'premium' : tier === 'registered' ? 'registered' : 'free';
    container.innerHTML =
      '<div class="topbar__menu topbar__menu--member">' +
        '<nav aria-label="會員導覽"><ul class="topbar__menu-list">' +
        MEMBER_MENU.map(function (item) {
          return '<li><a class="topbar__menu-link" href="' + item.href + '">' + item.label + '</a></li>';
        }).join('') +
        '</ul></nav>' +
        '<div class="topbar__user">' +
          '<span class="topbar__user-email" title="' + escapeEmail(email) + '">' + escapeEmail(email) + '</span>' +
          '<span class="topbar__tier-badge topbar__tier-badge--' + tierCls + '">' + tierText + '</span>' +
          '<button type="button" class="topbar__logout-btn" id="topbarLogoutBtn">登出</button>' +
        '</div>' +
      '</div>';
    const logoutBtn = container.querySelector('#topbarLogoutBtn');
    if (logoutBtn) {
      logoutBtn.addEventListener('click', async function () {
        await logout();
        await renderNavState();
      });
    }
    return;
  }

  // 未登入：marketing menu + 自願 Login 按鈕
  const loginHref = getRedirectUrl();
  container.innerHTML =
    '<div class="topbar__menu topbar__menu--marketing">' +
      '<nav aria-label="主導覽"><ul class="topbar__menu-list">' +
      MARKETING_MENU.map(function (item) {
        return '<li><a class="topbar__menu-link" href="' + item.href + '">' + item.label + '</a></li>';
      }).join('') +
      '</ul></nav>' +
      '<a class="topbar__login-btn" href="' + loginHref + '">' +
        '<svg class="topbar__login-icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">' +
          '<path d="M15 3h4a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-4"></path>' +
          '<polyline points="10 17 15 12 10 7"></polyline>' +
          '<line x1="15" y1="12" x2="3" y2="12"></line>' +
        '</svg>' +
        '<span>Login</span>' +
      '</a>' +
    '</div>';
}

/** 最小 HTML escape（email 只可能含 @ . - _ +；防 attribute injection）。 */
function escapeEmail(email) {
  return String(email)
    .replace(/&/g, '&amp;')
    .replace(/"/g, '&quot;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
}

// ─── Re-export for convenience ───
export { postJSON, getJSON };
