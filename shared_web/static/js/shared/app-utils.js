import { escapeHtml } from './utils.js';

export async function getJSON(url) {
  const res = await fetch(url, { credentials: 'include' });
  if (!res.ok) throw new Error(url + ': ' + res.status);
  return res.json();
}

export async function silentGetJSON(url) {
  try {
    return await getJSON(url);
  } catch (err) {
    console.warn('API ' + url + ': ' + err.message);
    return null;
  }
}

export async function postJSON(url, body) {
  const res = await fetch(url, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  });
  if (!res.ok) throw new Error(url + ': ' + res.status);
  return res.json();
}

export function notify(msg, type) { console.log('[' + (type || 'info') + '] ' + msg); }

export { escapeHtml };

export async function putJSON(url, body) {
  const res = await fetch(url, {
    method: 'PUT',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  });
  if (!res.ok) throw new Error(url + ': ' + res.status);
  return res.json();
}

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
