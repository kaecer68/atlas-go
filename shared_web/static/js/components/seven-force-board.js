import { fmtSafeSigned, fmtSafeNumber } from '../shared/format-metric.js';
import { escapeHtml } from '../shared/app-utils.js';

// 7-force backward-compatible 中文 label map (spec §4 D-CF-05；alias only)。
// 真正的分層由下列 DIMENSION_GROUPS 提供。
const FORCE_LABELS = {
  foreign: '外資',
  institutional: '投信',
  dealer: '自營商',
  retail: '散戶',
  government: '政府/公股行庫',
  futures: '期貨',
  'tsm_adr': 'TSM ADR',
};

// 七維錢潮雷達：3+2+2 分層（spec §4 D-CF-04）。
//   - official_actor: 三大法人（T86 第一方資料；共振模型只看這層）
//   - behavioral_proxy: 行為代理（官股/散戶，非 T86 第一方）
//   - market_signal:   領先／跨市場訊號（futures + tsm_adr，跨單位/跨市場）
//
// E07 backend 把 futures（positioning_indicator）與 tsm_adr（cross_market_signal）
// 拆為兩個 dimension_role；本 UI 依 spec §4 D-CF-04 對齊到同一 tier，讓散戶一眼
// 看見「訊號層」≠ 「法人層」。
const DIMENSION_GROUPS = [
  {
    role: 'official_actor',
    title: '官方法人',
    blurb: '三項為 T86 第一方資料；共振模型只看這層',
    keys: ['foreign', 'institutional', 'dealer'],
  },
  {
    role: 'behavioral_proxy',
    title: '行為代理',
    blurb: '官股與散戶；資料口徑與法人不同，缺資料不補 0',
    keys: ['government', 'retail'],
  },
  {
    role: 'market_signal',
    title: '領先／跨市場訊號',
    blurb: '期貨 OI 與 TSM ADR；非台股資金主體，只供方向參考',
    keys: ['futures', 'tsm_adr'],
  },
];

const TONE_MAP = {
  bullish: 'positive',
  bearish: 'negative',
  neutral: 'neutral',
};

function nameOf(force) {
  return force.force || force.Force || '';
}

function dataAvailableOf(force) {
  // E07 ForceScore 帶 DataAvailable；PascalCase / snake_case 皆支援。
  if (force.data_available === false) return false;
  if (force.DataAvailable === false) return false;
  return true;
}

function trendOf(force) {
  return ((force.trend || force.Trend || 'neutral')).toLowerCase();
}

function zScoreOf(force) {
  if (typeof force.z_score === 'number') return force.z_score;
  if (typeof force.ZScore === 'number') return force.ZScore;
  return null;
}

function rawValueOf(force) {
  if (typeof force.raw_value === 'number') return force.raw_value;
  if (typeof force.RawValue === 'number') return force.RawValue;
  return null;
}

function forceCard(force) {
  const name = nameOf(force);
  const available = dataAvailableOf(force);
  const trend = trendOf(force);
  const zScore = zScoreOf(force);
  const rawValue = rawValueOf(force);
  const tone = TONE_MAP[trend] || 'neutral';
  // 缺資料 → 「資料不足」badge（**不**顯示「觀望」）；
  // 其他情形才以 trend 顯示「偏多／偏空／觀望」。
  const directionText = !available
    ? '資料不足'
    : trend === 'bullish'
      ? '偏多'
      : trend === 'bearish'
        ? '偏空'
        : '觀望';
  const strength = (available && zScore !== null) ? Math.min(Math.abs(zScore) / 3, 1) : 0;
  const strengthPct = Math.round(strength * 100);
  const valueText = (available && rawValue !== null)
    ? fmtSafeSigned(rawValue, { decimals: 1, suffix: ' 億', forceSign: true })
    : '—';
  const zText = (available && zScore !== null) ? fmtSafeNumber(zScore, { decimals: 2 }) : '—';
  const label = FORCE_LABELS[name] || name;

  return `
    <div class="force-card force-card--${tone}${available ? '' : ' force-card--unavailable'}">
      <div class="force-card__header">
        <span class="force-card__name">${escapeHtml(label)}</span>
        <span class="force-card__direction force-card__direction--${tone}${available ? '' : ' force-card__direction--unavailable'}">${escapeHtml(directionText)}</span>
      </div>
      <div class="force-card__value" title="原始買賣超（億）">${valueText}</div>
      <div class="force-card__strength">
        <div class="force-card__strength-bar" style="width: ${strengthPct}%" aria-label="強度 ${strengthPct}%"></div>
      </div>
      <div class="force-card__meta">Z-score ${zText}</div>
    </div>
  `;
}

// 把 forces 依「E07 dimension_role」分層；legacy 沒 dimension_role 時按 force
// 鍵 fallback 回 7-force，向後相容（spec §7.1 / CF-INV-01）。
function groupForces(forces) {
  const buckets = new Map();
  for (const g of DIMENSION_GROUPS) buckets.set(g.role, []);

  for (const f of forces) {
    const role = f.dimension_role || f.DimensionRole;
    const name = nameOf(f);
    let bucket;
    if (role && buckets.has(role)) {
      // 把 E07 兩種 role (positioning_indicator / cross_market_signal) 對齊到同一 tier。
      bucket = role === 'positioning_indicator' || role === 'cross_market_signal'
        ? 'market_signal'
        : role;
    } else if (FORCE_LABELS[name]) {
      // legacy fallback：依 force 鍵分流到對應 tier。
      bucket = DIMENSION_GROUPS.find(g => g.keys.includes(name))?.role || null;
    } else {
      bucket = null;
    }
    if (bucket) buckets.get(bucket).push(f);
  }
  return buckets;
}

export function renderSevenForceBoard(container, summary) {
  if (!container) return;
  const forces = summary && Array.isArray(summary.forces) ? summary.forces : [];
  if (!forces.length) {
    container.innerHTML = '<div class="home-loading-card">尚無七維錢潮雷達資料</div>';
    return;
  }

  const buckets = groupForces(forces);
  const sections = DIMENSION_GROUPS.map(g => {
    const tier = (buckets.get(g.role) || []).map(forceCard).join('');
    return `
      <section class="seven-force-tier" data-tier="${g.role}">
        <header class="seven-force-tier__head">
          <h3 class="seven-force-tier__title">${escapeHtml(g.title)}</h3>
          <p class="seven-force-tier__blurb">${escapeHtml(g.blurb)}</p>
        </header>
        <div class="seven-force-tier__cards">${tier}</div>
      </section>
    `;
  }).join('');

  container.innerHTML = `<div class="seven-force-board">${sections}</div>`;
}
