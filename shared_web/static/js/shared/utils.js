// Shared utility functions for Atlas dashboard

import { formatNumber } from './format-metric.js';

const MISSING = '—';

function isValidNumber(v) {
  return typeof v === 'number' && Number.isFinite(v);
}

export function fmt(v) {
  return isValidNumber(v) ? v.toLocaleString('en-US', {minimumFractionDigits: 0, maximumFractionDigits: 0}) : MISSING;
}

export function fmtPct(v) {
  const s = formatNumber(v, { percent: true, decimals: 1, suffix: '%' });
  if (!isValidNumber(v) || v === 0) return s;
  return (v > 0 ? '+' : '') + s;
}

export function fmtFloat(v) {
  return isValidNumber(v) ? v.toLocaleString('en-US', {minimumFractionDigits: 2, maximumFractionDigits: 2}) : MISSING;
}

export function fmtInt(v) {
  return isValidNumber(v) ? v.toLocaleString('en-US') : MISSING;
}

export function pnlColor(v) {
  if (!isValidNumber(v)) return '';
  if (v === 0) return 'var(--muted)';
  return v > 0 ? 'var(--pnl-profit)' : 'var(--pnl-loss)';
}

export function pnlSign(v) {
  if (!isValidNumber(v) || v === 0) return '';
  return v > 0 ? '+' : '−';
}

export function convColor(v) {
  if (!isValidNumber(v)) return 'var(--muted)';
  if (v >= 0.7) return 'var(--metric-good)';
  if (v >= 0.4) return 'var(--warn)';
  return 'var(--muted)';
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
  // B8 (risk-console Phase 1)：台幣無角分實務 — 0 位小數（與 fmtCurrency 一致）。
  if (!isValidNumber(v)) return 'NT$—';
  const sign = v < 0 ? '-' : '';
  const abs = Math.abs(v);
  const formatted = abs.toLocaleString('en-US', { minimumFractionDigits: 0, maximumFractionDigits: 0 });
  return `NT$${sign}${formatted}`;
}

export function emptyState(msg, hint) {
  return `<div style="padding:20px;text-align:center;color:var(--muted)">${msg || '尚無資料'}</div>`;
}
