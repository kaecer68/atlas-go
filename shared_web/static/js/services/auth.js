/**
 * auth.js — shared authentication service for admin_web and client_web.
 *
 * Dual-token mode:
 *   1. Cookie-based JWT (HttpOnly, auto-sent via credentials: 'include')
 *   2. Memory-cached token (from POST /api/auth/login response, for Authorization header)
 *
 * Usage:
 *   import { login, register, logout, isLoggedIn, getTier, getClaims, getToken, initAuth } from '../services/auth.js';
 */

import { postJSON, getJSON } from '../shared/app-utils.js';

let _token = null;
let _claims = null;
let _authChecked = false;
let _authValid = false;

const PROFILE_URL = '/api/user/profile';
const LOGIN_URL = '/api/auth/login';
const REGISTER_URL = '/api/auth/register';

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
 * Logout: clear local state. The HttpOnly cookie is cleared by the server if needed.
 */
export function logout() {
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
    if (profile && profile.email) {
      _authValid = true;
      if (profile.tier) {
        if (!_claims) _claims = {};
        _claims.tier = profile.tier;
      }
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
 */
export async function initAuth() {
  const loggedIn = await isLoggedIn();
  if (!loggedIn) {
    const token = readCookie('token');
    if (token) {
      const claims = parseJWT(token);
      if (claims && claims.exp) {
        const expired = Date.now() >= claims.exp * 1000;
        if (expired) {
          // Token expired — clear stale cookie and local state
          _token = null;
          _claims = null;
          _authValid = false;
          _authChecked = true;
        }
      }
    }
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

// ─── Re-export for convenience ───
export { postJSON, getJSON };
