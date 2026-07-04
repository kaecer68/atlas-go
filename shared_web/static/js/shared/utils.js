// Shared utility functions for Atlas dashboard
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
  return typeof v === 'number' ? (v >= 0 ? 'var(--pnl-profit)' : 'var(--pnl-loss)') : '';
}

export function pnlSign(v) {
  return typeof v === 'number' ? (v >= 0 ? '+' : '') : '';
}

export function convColor(v) {
  return typeof v === 'number' ? (v >= 0.7 ? 'var(--metric-good)' : (v >= 0.4 ? 'var(--warn)' : 'var(--muted)')) : 'var(--muted)';
}

export function getThemeColor(varName, fallbackHex) {
  const v = getComputedStyle(document.documentElement).getPropertyValue(varName).trim();
  if (v) return v;
  if (fallbackHex) return fallbackHex;
  console.warn('[getThemeColor] CSS variable not found:', varName);
  return '#000000';
}

export function escapeHtml(str) {
  if (!str) return '';
  return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

export function fmtNTD(v) {
  if (typeof v !== 'number' || isNaN(v)) return 'NT$—';
  const sign = v < 0 ? '-' : '';
  const abs = Math.abs(v);
  const formatted = abs.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  return `NT$${sign}${formatted}`;
}

export function emptyState(msg, hint) {
  return `<div style="padding:20px;text-align:center;color:var(--muted)">${msg || '尚無資料'}</div>`;
}
