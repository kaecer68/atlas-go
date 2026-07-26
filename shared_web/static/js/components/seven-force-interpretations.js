import { escapeHtml } from '../shared/app-utils.js';

// 與 seven-force-board.js 共用 7-force 中文 label map，向後相容。
const FORCE_LABELS = {
  foreign: '外資',
  institutional: '投信',
  dealer: '自營商',
  retail: '散戶',
  government: '政府/公股行庫',
  futures: '期貨',
  'tsm_adr': 'TSM ADR',
};

// 預期 dimension_role 集合（spec §4 D-CF-04 / §6）。
//
//  - 「官方actor」三大法人（foreign/institutional/dealer）— 共識投票者（CF-INV-09）
//  - 「行為代理」（government/retail）— 不進共識敘事（CF-INV-01）
//  - 「領先／跨市場」（futures + tsm_adr）— 跨單位訊號，**不**影響 actor 共識
const OFFICIAL_ACTOR_KEYS = new Set(['foreign', 'institutional', 'dealer']);
const BEHAVIORAL_PROXY_KEYS = new Set(['government', 'retail']);

function nameOf(force) {
  return force.force || force.Force || '';
}

function dataAvailableOf(force) {
  if (force.data_available === false) return false;
  if (force.DataAvailable === false) return false;
  return true;
}

function trendOf(force) {
  return ((force.trend || force.Trend || 'neutral')).toLowerCase();
}

// E07「dimension_role」分流；legacy fallback：依 force 鍵判斷是否為 official_actor。
function officialActors(forces) {
  return forces.filter(f => {
    const role = f.dimension_role || f.DimensionRole;
    if (role) return role === 'official_actor';
    return OFFICIAL_ACTOR_KEYS.has(nameOf(f));
  });
}

function behavioralProxies(forces) {
  return forces.filter(f => {
    const role = f.dimension_role || f.DimensionRole;
    if (role) return role === 'behavioral_proxy';
    return BEHAVIORAL_PROXY_KEYS.has(nameOf(f));
  });
}

function foreignPositioning(forces) {
  return forces.filter(f => {
    const role = f.dimension_role || f.DimensionRole;
    const name = nameOf(f);
    if (role) return role === 'positioning_indicator' || (role === 'official_actor' && name === 'foreign');
    return name === 'futures' || name === 'foreign';
  });
}

function crossMarket(forces) {
  return forces.filter(f => {
    const role = f.dimension_role || f.DimensionRole;
    const name = nameOf(f);
    if (role) return role === 'cross_market_signal';
    return name === 'tsm_adr';
  });
}

function bullish(forces) {
  return forces.filter(f => trendOf(f) === 'bullish');
}

function bearish(forces) {
  return forces.filter(f => trendOf(f) === 'bearish');
}

function label(name) {
  return FORCE_LABELS[name] || name || '未知';
}

function weightOf(force) {
  return typeof force.weight === 'number'
    ? force.weight
    : (typeof force.Weight === 'number' ? force.Weight : null);
}

function topWeighted(forces, n = 2) {
  return [...forces]
    .filter(f => weightOf(f) !== null)
    .sort((a, b) => weightOf(b) - weightOf(a))
    .slice(0, n);
}

export function renderSevenForceInterpretations(container, summary) {
  if (!container) return;
  const forces = summary && Array.isArray(summary.forces) ? summary.forces : [];
  if (!forces.length) {
    container.innerHTML = '';
    return;
  }

  const interpretations = [];

  // ============ Group 1: 機構共識（official_actor 三大法人） ============
  //
  // 三大法人（共識）— 共識模型只看這層（CF-INV-09）。
  // futures / tsm_adr **不**計入此敘事（CF-INV-01）。
  const actorForces = officialActors(forces).filter(dataAvailableOf);
  const bullActors = bullish(actorForces);
  const bearActors = bearish(actorForces);
  if (actorForces.length) {
    if (bullActors.length === actorForces.length) {
      interpretations.push('三大法人（共識）偏多，資金方向一致。');
    } else if (bearActors.length === actorForces.length) {
      interpretations.push('三大法人（共識）偏空，資金方向一致。');
    } else if (bullActors.length === 0 && bearActors.length === 0) {
      interpretations.push('三大法人皆觀望，方向不明。');
    } else if (bullActors.length >= 2 && bearActors.length === 0) {
      interpretations.push('三大法人多數偏多，僅少數分歧。');
    } else if (bearActors.length >= 2 && bullActors.length === 0) {
      interpretations.push('三大法人多數偏空，僅少數分歧。');
    } else if (bullActors.length > bearActors.length) {
      interpretations.push('三大法人分歧，多數偏多。');
    } else if (bearActors.length > bullActors.length) {
      interpretations.push('三大法人分歧，多數偏空。');
    } else {
      // bull == bear (e.g., 1 vs 1)
      interpretations.push('三大法人對作，方向分歧。');
    }
  }

  // ============ Group 2: 行為代理（behavioral_proxy）============
  //
  // 缺資料的 force 不敘事（CF-INV 行為代理完整性），
  // 未通過 data gate 的 government/retail 不影響「法人接散戶」等組合敘事。
  const proxies = behavioralProxies(forces).filter(dataAvailableOf);
  const bullProxies = bullish(proxies);
  const bearProxies = bearish(proxies);
  const hasRetailBullish = bullProxies.some(f => nameOf(f) === 'retail');
  const hasRetailBearish = bearProxies.some(f => nameOf(f) === 'retail');
  const hasGovernmentBullish = bullProxies.some(f => nameOf(f) === 'government');
  const hasGovernmentBearish = bearProxies.some(f => nameOf(f) === 'government');

  // 法人主流方向（用 actor consensus 摘要）
  const actorBullish = bullActors.length > bearActors.length;
  const actorBearish = bearActors.length > bullActors.length;
  const actorMixed = bullActors.length > 0 && bearActors.length > 0;

  if (hasRetailBearish && actorBullish) {
    interpretations.push('法人偏多但散戶偏空，呈現法人接散戶籌碼結構。');
  }
  if (hasRetailBullish && actorBearish) {
    interpretations.push('散戶偏多但法人偏空，籌碼與法人反向。');
  }
  if (hasRetailBullish && actorBullish) {
    interpretations.push('散戶與法人同步偏多，動能共識一致。');
  }
  if (hasGovernmentBullish && actorBullish) {
    interpretations.push('政府/公股行庫與法人同步偏多，官股護盤加上外資回流。');
  }
  if (hasGovernmentBearish && actorBullish && !actorMixed) {
    // 官股反向但法人完全偏多：值得敘事
    // 政府 unavailable → 已被 filter 掉，故不會敘事
    interpretations.push('官股反向但法人偏多，行為代理分歧。');
  }
  // 行為代理 fallback：散戶/官股都缺資料 → 不主動敘事
  // （spec §4 D-CF-04：缺資料不補 0）

  // ============ Group 3: 外資 positioning（futures OI）============
  //
  // 領先訊號：futures 不影響 actor consensus，但若 futures 方向與法人一致，
  // 提供「領先支持」敘事。
  const positioning = foreignPositioning(forces).filter(dataAvailableOf);
  const hasFuturesBullish = positioning.some(f => nameOf(f) === 'futures' && trendOf(f) === 'bullish');
  const hasFuturesBearish = positioning.some(f => nameOf(f) === 'futures' && trendOf(f) === 'bearish');
  if (hasFuturesBullish && actorBullish && !actorMixed) {
    interpretations.push('期貨未平倉領先訊號支持法人偏多方向。');
  } else if (hasFuturesBearish && actorBearish && !actorMixed) {
    interpretations.push('期貨未平倉領先訊號支持法人偏空方向。');
  } else if (hasFuturesBullish && actorBearish && !actorMixed) {
    interpretations.push('期貨領先偏多，但法人偏空，方向可能即將反轉。');
  } else if (hasFuturesBearish && actorBullish && !actorMixed) {
    interpretations.push('期貨領先偏空，但法人偏多，方向可能即將反轉。');
  }

  // ============ Group 4: 跨市場訊號（TSM ADR）============
  //
  // ADR 是跨市場價格訊號，不影響 actor 共識（CF-INV-01）。
  const xs = crossMarket(forces).filter(dataAvailableOf);
  const hasAdrBullish = xs.some(f => nameOf(f) === 'tsm_adr' && trendOf(f) === 'bullish');
  const hasAdrBearish = xs.some(f => nameOf(f) === 'tsm_adr' && trendOf(f) === 'bearish');
  const hasForeignBullish = bullActors.some(f => nameOf(f) === 'foreign');
  const hasForeignBearish = bearActors.some(f => nameOf(f) === 'foreign');

  // TSM ADR × foreign 在 E07 仍是「跨市場同步」訊號（保留原 spec §9.1 行為）
  if (hasAdrBullish && hasFuturesBullish) {
    interpretations.push('期貨與 TSM ADR 同步偏多，外資跨市場態度積極。');
  }
  if (hasAdrBullish && hasForeignBullish) {
    interpretations.push('TSM ADR 與外資同步偏多，跨市場與現貨一致。');
  }
  if (hasAdrBearish && hasForeignBearish) {
    interpretations.push('TSM ADR 與外資同步偏空，跨市場與現貨一致。');
  }

  // ============ 透明披露：主要權重聲明（spec §9.1 允許 actor 共識敘事） ============
  //
  // 作為最後一條透明聲明列出前 2 高 weight 的 force。
  // 在 E07 weight=0/deprecated 的前提下，這條敘事成為「模型未校準前的透明標記」，
  // 不影響上面分層共識敘事的強弱。
  {
    const top = topWeighted(forces, 2);
    if (top.length) {
      const topNames = top.map(f => label(nameOf(f))).join('、');
      interpretations.push(`主要權重集中在 ${escapeHtml(topNames)}，方向以觀望為主。`);
    }
  }

  const items = interpretations.map(t => `<li class="force-interpretation__item">${escapeHtml(t)}</li>`).join('');
  container.innerHTML = `
    <div class="force-interpretation">
      <h4 class="force-interpretation__title">七維錢潮雷達（3+2+2 分層解讀）</h4>
      <ul class="force-interpretation__list">${items}</ul>
    </div>
  `;
}
