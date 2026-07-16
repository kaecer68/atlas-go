import { fmtSafeSigned, fmtSafeNumber } from '../shared/format-metric.js';
import { escapeHtml } from '../shared/app-utils.js';

const FORCE_LABELS = {
  foreign: '外資',
  institutional: '投信',
  dealer: '自營商',
  retail: '散戶',
  government: '政府/公股行庫',
  futures: '期貨',
  'tsm_adr': 'TSM ADR',
};

const TONE_MAP = {
  bullish: 'positive',
  bearish: 'negative',
  neutral: 'neutral',
};

function forceCard(force) {
  const name = force.force || force.Force || '';
  const trend = (force.trend || force.Trend || 'neutral').toLowerCase();
  const zScore = typeof force.z_score === 'number' ? force.z_score : (typeof force.ZScore === 'number' ? force.ZScore : null);
  const rawValue = typeof force.raw_value === 'number' ? force.raw_value : (typeof force.RawValue === 'number' ? force.RawValue : null);
  const tone = TONE_MAP[trend] || 'neutral';
  const directionText = trend === 'bullish' ? '偏多' : trend === 'bearish' ? '偏空' : '觀望';
  const strength = zScore !== null ? Math.min(Math.abs(zScore) / 3, 1) : 0;
  const strengthPct = Math.round(strength * 100);
  const valueText = rawValue !== null ? fmtSafeSigned(rawValue, { decimals: 1, suffix: ' 億', forceSign: true }) : '—';
  const zText = zScore !== null ? fmtSafeNumber(zScore, { decimals: 2 }) : '—';
  const label = FORCE_LABELS[name] || name;

  return `
    <div class="force-card force-card--${tone}">
      <div class="force-card__header">
        <span class="force-card__name">${escapeHtml(label)}</span>
        <span class="force-card__direction force-card__direction--${tone}">${escapeHtml(directionText)}</span>
      </div>
      <div class="force-card__value" title="原始買賣超（億）">${valueText}</div>
      <div class="force-card__strength">
        <div class="force-card__strength-bar" style="width: ${strengthPct}%" aria-label="強度 ${strengthPct}%"></div>
      </div>
      <div class="force-card__meta">Z-score ${zText}</div>
    </div>
  `;
}

export function renderSevenForceBoard(container, summary) {
  if (!container) return;
  const forces = summary && Array.isArray(summary.forces) ? summary.forces : [];
  if (!forces.length) {
    container.innerHTML = '<div class="home-loading-card">尚無 7-Force 資料</div>';
    return;
  }
  container.innerHTML = `<div class="seven-force-board">${forces.map(forceCard).join('')}</div>`;
}
