import { escapeHtml } from './utils.js';

export async function getJSON(url) {
  var res = await fetch(url);
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
  var res = await fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
  if (!res.ok) throw new Error(url + ': ' + res.status);
  return res.json();
}

export function notify(msg, type) { console.log('[' + (type || 'info') + '] ' + msg); }

export { escapeHtml };

export async function putJSON(url, body) {
  var res = await fetch(url, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
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

// Sort narrative events by composite strength (|sentiment| * confidence * hit_rate).
// Higher values indicate more significant events.  Mutates the input array.
export function sortNarrativeEvents(events) {
  return events.sort(function(a, b) {
    var strengthA = Math.abs(a.sentiment || 0) * (a.confidence || 1) * (a.hit_rate || 0.5);
    var strengthB = Math.abs(a.sentiment || 0) * (b.confidence || 1) * (b.hit_rate || 0.5);
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
