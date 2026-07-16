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

function label(name) {
  return FORCE_LABELS[name] || name || '未知';
}

function trendOf(force) {
  if (!force) return 'neutral';
  return ((force.trend || force.Trend || 'neutral')).toLowerCase();
}

function bullishForces(forces) {
  return forces.filter(f => trendOf(f) === 'bullish');
}

function bearishForces(forces) {
  return forces.filter(f => trendOf(f) === 'bearish');
}

function topWeighted(forces, n = 2) {
  return [...forces]
    .filter(f => typeof (f.weight || f.Weight) === 'number')
    .sort((a, b) => (b.weight || b.Weight) - (a.weight || a.Weight))
    .slice(0, n);
}

export function renderSevenForceInterpretations(container, summary) {
  if (!container) return;
  const forces = summary && Array.isArray(summary.forces) ? summary.forces : [];
  if (!forces.length) {
    container.innerHTML = '<div class="home-loading-card">尚無 7-Force 解讀</div>';
    return;
  }

  const bullish = bullishForces(forces);
  const bearish = bearishForces(forces);
  const top = topWeighted(forces, 2);
  const interpretations = [];

  if (bullish.length === forces.length) {
    interpretations.push('七大勢力全面偏多，資金共識強。');
  } else if (bearish.length === forces.length) {
    interpretations.push('七大勢力全面偏空，資金共識偏謹慎。');
  } else if (bullish.length >= 4 && bearish.length <= 1) {
    interpretations.push('多數勢力偏多，僅少數勢力分歧。');
  } else if (bearish.length >= 4 && bullish.length <= 1) {
    interpretations.push('多數勢力偏空，僅少數勢力支撐。');
  } else if (bullish.length === 0 && bearish.length === 0) {
    interpretations.push('各勢力觀望，市場方向不明。');
  }

  const hasForeign = bullish.some(f => (f.force || f.Force) === 'foreign');
  const hasInstitutional = bullish.some(f => (f.force || f.Force) === 'institutional');
  const hasRetailBearish = bearish.some(f => (f.force || f.Force) === 'retail');
  const hasDealer = bullish.some(f => (f.force || f.Force) === 'dealer');
  const hasRetailBullish = bullish.some(f => (f.force || f.Force) === 'retail');
  const hasForeignBearish = bearish.some(f => (f.force || f.Force) === 'foreign');
  const hasGovernment = bullish.some(f => (f.force || f.Force) === 'government');
  const hasFutures = bullish.some(f => (f.force || f.Force) === 'futures');
  const hasTsmAdr = bullish.some(f => (f.force || f.Force) === 'tsm_adr');

  if (hasForeign && hasInstitutional) {
    interpretations.push('外資與投信同步偏多，法人齊買。');
  }
  if (hasForeign && hasRetailBearish) {
    interpretations.push('外資偏多但散戶偏空，呈現法人接散戶籌碼結構。');
  }
  if (hasForeign && hasForeignBearish) {
    interpretations.push('外資與投信方向分歧，法人對作。');
  }
  if (hasRetailBullish && hasDealer) {
    interpretations.push('散戶與自營商同步偏多，短線動能活躍。');
  }
  if (hasRetailBullish && hasForeignBearish) {
    interpretations.push('散戶偏多但外資偏空，籌碼與法人反向。');
  }
  if (hasGovernment && hasForeign) {
    interpretations.push('政府/公股行庫與外資同步偏多，官股護盤加上外資回流。');
  }
  if (hasFutures && hasTsmAdr) {
    interpretations.push('外資期貨與 TSM ADR 同步偏多，外資態度積極。');
  }

  if (!interpretations.length) {
    const topNames = top.map(f => label(f.force || f.Force)).join('、');
    interpretations.push(`主要權重集中在 ${escapeHtml(topNames)}，方向以觀望為主。`);
  }

  const items = interpretations.map(t => `<li class="force-interpretation__item">${escapeHtml(t)}</li>`).join('');
  container.innerHTML = `
    <div class="force-interpretation">
      <h4 class="force-interpretation__title">7-Force 組合解讀</h4>
      <ul class="force-interpretation__list">${items}</ul>
    </div>
  `;
}
