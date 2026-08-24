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

// ── Degraded (降級) 狀態追蹤 ────────────────────────────────────────────────
// P1-B：API 失敗不再靜默。所有 GET 失敗（可恢復類型：網路/逾時/5xx/429）會在
// choke point 記錄進全域 degraded 狀態並派發 'atlas:degraded' CustomEvent，
// UI 據此顯示「部分資料源暫時不可用」badge；同 endpoint 後續 fetch 成功時
// 自動清除（恢復後 badge 自動消失）。
//
// dedupe：同一 endpoint 在 DEGRADED_REPORT_WINDOW_MS 內重複失敗只通知一次，
// 避免 30s auto-refresh 每輪都刷事件。
//
// 刻意「不上報後端」：失敗迴路風險（backend 都掛了還把失敗 POST 回去）與
// 前端降級可視性無關，這裡只做前端狀態 + CustomEvent。
// ---------------------------------------------------------------------------
export let DEGRADED_REPORT_WINDOW_MS = 5 * 60 * 1000; // 同 endpoint 5 分鐘內只報一次

/**
 * 覆寫 dedupe window（測試用；ESM import 側無法直接改 let 綁定）。
 * @param {number} ms
 */
export function setDegradedReportWindowMs(ms) {
  DEGRADED_REPORT_WINDOW_MS = ms;
}

const degradedEndpoints = new Set();    // 目前降級中的 endpoint（成功後清除）
const lastReportedAt = new Map();       // endpoint -> 最後一次「已通知」時間（dedupe）
const degradedListeners = new Set();    // onDegradedChange 訂閱者（badge 等 UI）

function notifyDegradedChanged() {
  const endpoints = getDegradedEndpoints();
  degradedListeners.forEach(function (fn) {
    try { fn(endpoints); } catch (e) { /* listener 異常不影響主流程 */ }
  });
  if (typeof window !== 'undefined' && typeof window.dispatchEvent === 'function') {
    try {
      window.dispatchEvent(new CustomEvent('atlas:degraded', { detail: endpoints }));
    } catch (e) { /* 非瀏覽器環境（node --test）沒有 CustomEvent 也能跑 */ }
  }
}

/**
 * 記錄一個 endpoint 進入降級狀態（dedupe：同 endpoint 於 window 內只通知一次）。
 * @param {string} endpoint
 */
export function reportDegraded(endpoint) {
  if (!endpoint) return;
  const now = Date.now();
  const wasDegraded = degradedEndpoints.has(endpoint);
  degradedEndpoints.add(endpoint);
  const last = lastReportedAt.get(endpoint) || 0;
  if (!wasDegraded || now - last >= DEGRADED_REPORT_WINDOW_MS) {
    lastReportedAt.set(endpoint, now);
    notifyDegradedChanged();
  }
}

/**
 * endpoint 恢復（後續 fetch 成功）時清除降級狀態；有變化才通知。
 * @param {string} endpoint
 */
export function clearDegraded(endpoint) {
  if (!endpoint) return;
  if (degradedEndpoints.delete(endpoint)) {
    notifyDegradedChanged();
  }
}

/**
 * 目前降級中的 endpoint 清單（複本，勿直接改內部 Set）。
 * @returns {string[]}
 */
export function getDegradedEndpoints() {
  return Array.from(degradedEndpoints).sort();
}

/**
 * 訂閱 degraded 狀態變化。回傳取消訂閱函式。
 * @param {(endpoints: string[]) => void} listener
 * @returns {() => void}
 */
export function onDegradedChange(listener) {
  degradedListeners.add(listener);
  return function () { degradedListeners.delete(listener); };
}

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
 * Attach X-API-Key header whenever a key is available（GET/HEAD 靜默帶 key，
 * 解決 config / metrics / deployment / report 等受保護 GET 端點的 401）。
 * 只有 mutating（POST/PUT/DELETE/PATCH）在無 key 時才 prompt；GET/HEAD 無
 * key 一律靜默不帶、不 prompt（未授權者維持誠實空態）。
 *
 * 安全性：client_web 從不寫 ATLAS_API_KEY → getAtlasApiKey() 回空 → 不帶
 * header → client_web 零影響。
 * @param {Record<string, string>} headers
 * @param {string} method
 */
async function attachApiKey(headers, method) {
  let key = getAtlasApiKey();
  if (!key && MUTATING_METHODS.includes(method)) {
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
  try {
    const data = await fetchWithRetry('GET', url, undefined, options);
    clearDegraded(url);
    return data;
  } catch (err) {
    // 可恢復失敗（網路/逾時/5xx/429）→ 記錄降級；4xx 為確定性錯誤不誤報。
    if (isRetryable(err)) reportDegraded(url);
    throw err;
  }
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
 * @param {Object} [options] 額外透傳給底層 fetch 的選項（如 { retry: 0 }）
 * @returns {Promise<any|null>}
 */
export async function getJSONWithTimeout(url, ms, options) {
  ms = ms || 10000;
  // 把 timeout 往下傳：底層 fetch 在 deadline 到達時真的被 abort（而非放任 8s
  // 預設 timer 懸掛）。
  //
  // settled guard：timeout 分支與底層 abort 路徑「誰先到誰贏」。若底層已先
  // settle（成功或已由 getJSON 回報降級），timeout 分支不得再補一發
  // reportDegraded——否則 dedupe window 內會出現「測試/頁面結束後才冒出來」
  // 的延遲上報。
  let settled = false;
  return new Promise(function(resolve) {
    silentGetJSON(url, Object.assign({ timeoutMs: ms }, options || {})).then(
      function(data) { if (settled) return; settled = true; resolve(data); },
      function()   { if (settled) return; settled = true; resolve(null); }
    );
    setTimeout(function() {
      if (settled) return;
      settled = true;
      console.warn('[timeout]', url);
      reportDegraded(url);
      resolve(null);
    }, ms);
  });
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
  // 2026-08-24 UI audit P3：loading 狀態改用 skeleton 線條（避免多卡同時
  // 「⏳ 載入中」長時間空白；載入完成由各 renderer 覆蓋 innerHTML）。
  if (reason === 'loading') {
    return `<div class="missing-state ${state.className}" style="padding:12px 16px;border-radius:6px;background:color-mix(in srgb,var(--bg) 90%,transparent);border:1px dashed var(--border)">
      ${label ? `<div style="font-weight:600;color:var(--text);margin-bottom:8px">${escapeHtml(label)}</div>` : ''}
      <div class="skeleton-line"></div><div class="skeleton-line"></div><div class="skeleton-line" style="width:60%"></div>
    </div>`;
  }
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
