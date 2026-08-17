import { escapeHtml } from './utils.js';

// Stage 6 PR#1：統一 fetch 工具加上 timeout + AbortController + 1 retry。
// 設計：原本 `getJSON(url)` 是 bare fetch，無 timeout、無 retry、無錯誤分類。
// 改成 `fetchWithRetry(method, url, body, options)` 共用底層，4 個公開 wrapper
// 各自帶預設設定。silent* 系列仍吞掉錯誤回傳 null（前端 UI 別阻塞）。
//
// 預設值：
//   - 8000ms timeout（足夠撐到 backend 八大主要 endpoint 的 P95）
//   - 1 retry（總共 2 次 attempt）
//   - 500ms backoff
//   - 觸發 retry 的條件：timeout / network error / 5xx / 429
//
// 對既有 20+ consumers 完全向後相容：呼叫端沒傳 options 仍能正常用，
// 只不過自動套上新預設（這正是想要的全域改善）。

export const DEFAULT_TIMEOUT_MS = 8000;
export const DEFAULT_RETRY = 1;
export const DEFAULT_RETRY_BACKOFF_MS = 500;

function delay(ms) {
  return new Promise(function (resolve) { setTimeout(resolve, ms); });
}

function isRetryable(err) {
  if (!err) return false;
  if (err.name === 'AbortError') return true;
  if (err.name === 'TypeError' && /fetch/i.test(err.message)) return true;
  if (err.status && err.status >= 500) return true;
  if (err.status === 429) return true;
  return false;
}

const MUTATING_METHODS = ['POST', 'PUT', 'DELETE', 'PATCH'];

/**
 * Read the Atlas API key from localStorage, if available.
 * Returns an empty string when localStorage is unavailable or no key is stored.
 * @returns {string}
 */
export function getAtlasApiKey() {
  try {
    return localStorage.getItem('ATLAS_API_KEY') || '';
  } catch (_) {
    return '';
  }
}

/**
 * Store the Atlas API key in localStorage.
 * @param {string} key
 */
export function setAtlasApiKey(key) {
  try {
    if (key) {
      localStorage.setItem('ATLAS_API_KEY', key);
    } else {
      localStorage.removeItem('ATLAS_API_KEY');
    }
  } catch (_) {
    // localStorage may be unavailable in some environments; ignore.
  }
}

/**
 * Prompt the admin user for the ATLAS_API_KEY when a mutating request is made
 * without one. Returns the entered key (empty if cancelled / unavailable).
 * @returns {Promise<string>}
 */
async function promptForApiKey() {
  if (typeof window === 'undefined') return '';
  if (typeof window.__atlasPromptForApiKey === 'function') {
    return window.__atlasPromptForApiKey();
  }
  if (typeof window.prompt === 'function') {
    return window.prompt('ATLAS_API_KEY required for write operations:') || '';
  }
  return '';
}

/**
 * Attach X-API-Key header for mutating methods when a key is stored.
 * If no key is stored, prompt once and store it.
 * @param {Record<string, string>} headers
 * @param {string} method
 */
async function attachApiKey(headers, method) {
  if (!MUTATING_METHODS.includes(method)) return;
  let key = getAtlasApiKey();
  if (!key) {
    key = await promptForApiKey();
    if (key) setAtlasApiKey(key);
  }
  if (key) {
    headers['X-API-Key'] = key;
  }
}

async function fetchOnce(method, url, body, signal) {
  const opts = { method: method, credentials: 'include', signal: signal };
  const headers = {};
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json';
    opts.body = JSON.stringify(body);
  }
  await attachApiKey(headers, method);
  if (Object.keys(headers).length > 0) {
    opts.headers = headers;
  }
  const res = await fetch(url, opts);
  if (!res.ok) {
    const e = new Error(url + ': ' + res.status);
    e.status = res.status;
    throw e;
  }
  return res.json();
}

async function fetchWithRetry(method, url, body, options) {
  const opts = options || {};
  const timeoutMs = opts.timeoutMs == null ? DEFAULT_TIMEOUT_MS : opts.timeoutMs;
  const retry = opts.retry == null ? DEFAULT_RETRY : opts.retry;
  const backoffMs = opts.retryBackoffMs == null ? DEFAULT_RETRY_BACKOFF_MS : opts.retryBackoffMs;
  let lastErr;
  for (let attempt = 0; attempt <= retry; attempt++) {
    const controller = new AbortController();
    const timer = setTimeout(function () { controller.abort(); }, timeoutMs);
    try {
      return await fetchOnce(method, url, body, controller.signal);
    } catch (err) {
      lastErr = err;
      if (attempt < retry && isRetryable(err)) {
        await delay(backoffMs);
        continue;
      }
      throw err;
    } finally {
      clearTimeout(timer);
    }
  }
  throw lastErr;
}

export async function getJSON(url, options) {
  return fetchWithRetry('GET', url, undefined, options);
}

export async function silentGetJSON(url, options) {
  try {
    return await getJSON(url, options);
  } catch (err) {
    console.warn('API ' + url + ': ' + err.message);
    return null;
  }
}

/**
 * Fetch JSON with a hard timeout to prevent a single slow endpoint from
 * blocking the whole dashboard. Returns null on timeout or error.
 * @param {string} url
 * @param {number} [ms=10000]
 * @returns {Promise<any|null>}
 */
export async function getJSONWithTimeout(url, ms) {
  ms = ms || 10000;
  return Promise.race([
    silentGetJSON(url),
    new Promise(function(resolve) {
      setTimeout(function() { console.warn('[timeout]', url); resolve(null); }, ms);
    })
  ]);
}

export async function postJSON(url, body, options) {
  return fetchWithRetry('POST', url, body, options);
}

export async function putJSON(url, body, options) {
  return fetchWithRetry('PUT', url, body, options);
}

export async function delJSON(url, options) {
  return fetchWithRetry('DELETE', url, undefined, options);
}

export function notify(msg, type) { console.log('[' + (type || 'info') + '] ' + msg); }

export { escapeHtml };

export function formatDate(d) {
  if (!d) return '-';
  const date = new Date(d);
  if (isNaN(date.getTime()) || date.getFullYear() < 2000) return '-';
  return date.toLocaleString('zh-TW');
}

export function renderEmptyState(msg, hint) {
  return '<div style="padding:20px;text-align:center;color:var(--muted)">' + escapeHtml(msg || '尚無資料') + (hint ? '<br><small>' + escapeHtml(hint) + '</small>' : '') + '</div>';
}

export function renderSkeleton(lines) {
  return Array(lines || 4).fill('<div class="skeleton-line"></div>').join('');
}

/**
 * 統一資料缺失狀態元件，區分載入中 / 無資料 / 資料待更新 / API 錯誤。
 * @param {string} label 欄位或區塊名稱
 * @param {'loading'|'no-data'|'stale-data'|'api-error'} reason
 * @returns {string}
 */
export function renderMissingState(label, reason) {
  const states = {
    loading: { icon: '⏳', text: '載入中', className: 'missing-state--loading' },
    'no-data': { icon: '—', text: '無資料', className: 'missing-state--no-data' },
    'stale-data': { icon: '⌛', text: '資料待更新', className: 'missing-state--stale' },
    'api-error': { icon: '⚠️', text: 'API 錯誤', className: 'missing-state--error' },
  };
  const state = states[reason] || states['no-data'];
  return `<div class="missing-state ${state.className}" style="padding:12px 16px;text-align:center;border-radius:6px;background:color-mix(in srgb,var(--bg) 90%,transparent);border:1px dashed var(--border);color:var(--muted)">
    <div style="font-size:16px;margin-bottom:4px">${state.icon}</div>
    ${label ? `<div style="font-weight:600;color:var(--text);margin-bottom:2px">${escapeHtml(label)}</div>` : ''}
    <div style="font-size:12px">${escapeHtml(state.text)}</div>
  </div>`;
}

/**
 * 統一錯誤狀態元件，附帶重試按鈕。Stage 6 錢潮頁面使用。
 * @param {string} label 欄位或區塊名稱
 * @param {string} retryId 用於 querySelector 綁定重試事件的唯一標識
 * @returns {string}
 */
export function renderErrorState(label, retryId) {
  return `<div class="missing-state missing-state--error" style="padding:16px;text-align:center;border-radius:6px;background:color-mix(in srgb,var(--bg) 90%,transparent);border:1px dashed var(--border);color:var(--muted)">
    <div style="font-size:18px;margin-bottom:4px">⚠️</div>
    ${label ? `<div style="font-weight:600;color:var(--text);margin-bottom:2px">${escapeHtml(label)}</div>` : ''}
    <div style="font-size:12px;margin-bottom:10px">資料暫時無法載入</div>
    <button class="btn btn--secondary retry-btn" data-retry="${escapeHtml(retryId || '')}" type="button">重試</button>
  </div>`;
}

// Sort narrative events by composite strength (|sentiment| * confidence * hit_rate).
// Higher values indicate more significant events.  Mutates the input array.
export function sortNarrativeEvents(events) {
  return events.sort(function(a, b) {
    var strengthA = Math.abs(a.sentiment || 0) * (a.confidence || 1) * (a.hit_rate || 0.5);
    var strengthB = Math.abs(b.sentiment || 0) * (b.confidence || 1) * (b.hit_rate || 0.5);
    return strengthB - strengthA;
  });
}

export function parseSessionsList(payload) {
  if (payload === null || payload === undefined) {
    return { sessions: [], data_status: 'fetch_failed' };
  }
  if (!Array.isArray(payload.sessions)) {
    return { sessions: [], data_status: 'malformed' };
  }
  return { sessions: payload.sessions, data_status: 'ok' };
}
