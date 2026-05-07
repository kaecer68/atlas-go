export async function getJSON(url) {
  var res = await fetch(url);
  if (!res.ok) throw new Error(url + ': ' + res.status);
  return res.json();
}

export async function postJSON(url, body) {
  var res = await fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
  return res.json();
}

export function notify(msg, type) { console.log('[' + (type || 'info') + '] ' + msg); }

export function escapeHtml(text) {
  if (!text) return '';
  return String(text).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

export function formatDate(d) { return d ? new Date(d).toLocaleString('zh-TW') : '-'; }

export function renderEmptyState(msg, hint) {
  return '<div style="padding:20px;text-align:center;color:var(--muted)">' + (msg || '尚無資料') + (hint ? '<br><small>' + hint + '</small>' : '') + '</div>';
}

export function renderSkeleton(lines) {
  return Array(lines || 4).fill('<div class="skeleton-line"></div>').join('');
}
