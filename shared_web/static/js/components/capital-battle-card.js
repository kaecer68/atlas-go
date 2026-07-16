import { escapeHtml } from '../shared/app-utils.js';

const DIRECTION_LABELS = {
  bullish: '偏多',
  bearish: '偏空',
  neutral: '觀望',
};

const FORCE_LABELS = {
  foreign: '外資',
  institutional: '投信',
  dealer: '自營商',
  retail: '散戶',
};

function forceTrend(force) {
  if (!force) return 'neutral';
  return String(force.trend || force.Trend || 'neutral').toLowerCase();
}

export function renderCapitalBattleCard(container, summary) {
  if (!container) return;
  const forces = summary && Array.isArray(summary.forces) ? summary.forces : [];
  const byName = {};
  for (const f of forces) {
    const name = f.force || f.Force || '';
    byName[name] = f;
  }

  const foreign = byName.foreign;
  const institutional = byName.institutional;
  const dealer = byName.dealer;
  const retail = byName.retail;

  const rows = [
    { key: 'foreign', force: foreign },
    { key: 'institutional', force: institutional },
    { key: 'dealer', force: dealer },
    { key: 'retail', force: retail },
  ];

  let institutionalBullish = 0;
  let institutionalBearish = 0;
  for (const { key, force } of rows) {
    if (key === 'retail') continue;
    const t = forceTrend(force);
    if (t === 'bullish') institutionalBullish++;
    else if (t === 'bearish') institutionalBearish++;
  }

  const retailTrend = forceTrend(retail);
  const institutionalDir = institutionalBullish > institutionalBearish ? '偏多' : institutionalBearish > institutionalBullish ? '偏空' : '觀望';
  const retailDir = DIRECTION_LABELS[retailTrend] || '觀望';

  let narrative = '';
  if (institutionalDir === '偏多' && retailDir === '偏空') {
    narrative = '法人進 / 散戶出';
  } else if (institutionalDir === '偏空' && retailDir === '偏多') {
    narrative = '法人出 / 散戶進';
  } else if (institutionalDir === '偏多') {
    narrative = '法人與散戶同向偏多';
  } else if (institutionalDir === '偏空') {
    narrative = '法人與散戶同向偏空';
  } else {
    narrative = '法人與散戶方向分歧或觀望';
  }

  const rowsHtml = rows.map(({ key, force }) => {
    const trend = forceTrend(force);
    const tone = trend === 'bullish' ? 'positive' : trend === 'bearish' ? 'negative' : 'neutral';
    const label = FORCE_LABELS[key] || key;
    const dir = DIRECTION_LABELS[trend] || '觀望';
    return `
      <div class="capital-battle__row capital-battle__row--${tone}">
        <span class="capital-battle__force">${escapeHtml(label)}</span>
        <span class="capital-battle__direction">${escapeHtml(dir)}</span>
      </div>
    `;
  }).join('');

  container.innerHTML = `
    <div class="capital-battle__card">
      <div class="capital-battle__title">法人 vs 散戶對殺</div>
      <div class="capital-battle__narrative">${escapeHtml(narrative)}</div>
      <div class="capital-battle__grid">${rowsHtml}</div>
    </div>
  `;
}
