// Shared utility functions extracted from index.html
export function fmt(v) {
  return typeof v === 'number' ? v.toLocaleString('en-US', {minimumFractionDigits: 0, maximumFractionDigits: 0}) : '-';
}

export function fmtPct(v) {
  return typeof v === 'number' ? (v >= 0 ? '+' : '') + (v * 100).toFixed(1) + '%' : '-';
}

export function fmtFloat(v) {
  return typeof v === 'number' ? v.toLocaleString('en-US', {minimumFractionDigits: 2, maximumFractionDigits: 2}) : '-';
}

export function fmtInt(v) {
  return typeof v === 'number' ? v.toLocaleString('en-US') : '-';
}

export function pnlColor(v) {
  return typeof v === 'number' ? (v >= 0 ? 'var(--up)' : 'var(--down)') : '';
}

export function pnlSign(v) {
  return typeof v === 'number' ? (v >= 0 ? '+' : '') : '';
}

export function convColor(v) {
  return typeof v === 'number' ? (v >= 0.7 ? 'var(--up)' : (v >= 0.4 ? 'var(--warn)' : 'var(--muted)')) : 'var(--muted)';
}

export function escapeHtml(str) {
  if (!str) return '';
  return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

export function emptyState(msg, hint) {
  return `<div style="padding:20px;text-align:center;color:var(--muted)">${msg || '尚無資料'}</div>`;
}
