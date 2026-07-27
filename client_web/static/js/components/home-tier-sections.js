import { metricCard } from './metric-card.js';
import { fmtSignedPct, formatSigned, formatNumber } from '../shared/format-metric.js';
import { escapeHtml } from '../shared/utils.js';
import { getTier } from '../services/auth.js';
import { silentGetJSON } from '../shared/app-utils.js';

function buildTierSection(title, children) {
  var section = document.createElement('section');
  section.className = 'home-section';
  var header = document.createElement('div');
  header.className = 'home-section__header';
  var h2 = document.createElement('h2');
  h2.textContent = title;
  header.appendChild(h2);
  section.appendChild(header);
  if (Array.isArray(children)) {
    children.forEach(function (child) {
      if (child) section.appendChild(child);
    });
  } else if (children) {
    section.appendChild(children);
  }
  return section;
}

function buildMetricGrid(variant, htmlChunks) {
  var grid = document.createElement('div');
  grid.className = 'home-grid home-grid--' + variant;
  grid.innerHTML = htmlChunks.join('');
  return grid;
}

function buildTierCTA() {
  var section = document.createElement('section');
  section.className = 'home-section tier-cta';
  var header = document.createElement('div');
  header.className = 'home-section__header tier-cta__header';
  var h2 = document.createElement('h2');
  h2.textContent = '解鎖更多分析';
  var sub = document.createElement('span');
  sub.className = 'home-section__subtitle';
  sub.textContent = '註冊獲取「機器人輔助（atlas-mcp）」密鑰';
  var btn = document.createElement('button');
  btn.className = 'btn btn--primary tier-cta__btn';
  btn.type = 'button';
  btn.textContent = '免費註冊';
  btn.addEventListener('click', function () { window.switchPage('register'); });
  header.appendChild(h2);
  header.appendChild(sub);
  header.appendChild(btn);
  section.appendChild(header);
  return section;
}

function buildEventCard(ev) {
  var card = document.createElement('div');
  card.className = 'event-card';
  var name = document.createElement('div');
  name.className = 'event-card__name';
  name.textContent = ev.name || '';
  var meta = document.createElement('div');
  meta.className = 'event-card__meta';
  var impact = ev.expected_flow_impact || '';
  var badge = document.createElement('span');
  badge.className = impact === 'bullish' ? 'tier-badge tier-badge--signal tier-badge--bullish'
    : impact === 'bearish' ? 'tier-badge tier-badge--signal tier-badge--bearish'
    : 'tier-badge tier-badge--neutral';
  badge.textContent = impact || 'neutral';
  meta.appendChild(badge);
  var date = document.createElement('span');
  date.className = 'event-card__date';
  date.textContent = ev.start_date ? ev.start_date.slice(0, 10) : '';
  meta.appendChild(date);
  card.appendChild(name);
  card.appendChild(meta);
  return card;
}

// P0 fix: backend shape is {tier, market, strategies} map, not an array
function buildRecCard(rec) {
  var card = document.createElement('div');
  card.className = 'rec-card';

  var header = document.createElement('div');
  header.className = 'rec-card__header';
  var name = document.createElement('span');
  name.className = 'rec-card__name';
  name.textContent = (rec.strategies && rec.strategies.active) || rec.tier || '';
  header.appendChild(name);
  var tierBadge = document.createElement('span');
  var tierKey = (rec.tier || 'public').replace(/[^a-z]/g, '');
  tierBadge.className = 'rec-card__tier tier-badge tier-badge--' + tierKey;
  tierBadge.textContent = rec.tier || 'public';
  header.appendChild(tierBadge);
  card.appendChild(header);

  if (rec.market && (rec.market.regime_label || rec.market.capital_flow_summary)) {
    var ctx = document.createElement('div');
    ctx.className = 'rec-card__signal';
    var parts = [];
    if (rec.market.regime_label) parts.push(rec.market.regime_label);
    if (rec.market.capital_flow_summary) parts.push(rec.market.capital_flow_summary);
    ctx.textContent = parts.join(' · ');
    card.appendChild(ctx);
  }

  if (rec.market && rec.market.capital_flow_detail) {
    var d = rec.market.capital_flow_detail;
    var detail = document.createElement('div');
    var dClass = 'rec-card__signal';
    var ql = (d.quality_label || '').toLowerCase();
    if (ql === 'strong_inflow' || ql === 'inflow') {
      dClass += ' rec-card__signal--bullish';
    } else if (ql === 'strong_outflow' || ql === 'outflow') {
      dClass += ' rec-card__signal--bearish';
    } else {
      dClass += ' rec-card__signal--neutral';
    }
    detail.className = dClass;
    var dParts = [];
    if (d.quality_label) dParts.push(d.quality_label);
    if (d.dominant_force) dParts.push('主力: ' + d.dominant_force);
    if (d.resonance_dir) dParts.push('共振: ' + d.resonance_dir);
    if (d.date) dParts.push(d.date);
    detail.textContent = dParts.join(' · ');
    card.appendChild(detail);
  }

  if (rec.strategies) {
    if (rec.strategies.entry_signal) {
      var signal = document.createElement('div');
      signal.className = 'rec-card__signal rec-card__signal--bullish';
      signal.textContent = '進場訊號：' + rec.strategies.entry_signal;
      card.appendChild(signal);
    }
    if (rec.strategies.stop_loss) {
      var stop = document.createElement('div');
      stop.className = 'rec-card__signal rec-card__signal--bearish';
      stop.textContent = '停損：' + rec.strategies.stop_loss;
      card.appendChild(stop);
    }
    if (Array.isArray(rec.strategies.ranked) && rec.strategies.ranked.length > 0) {
      var ranked = document.createElement('div');
      ranked.className = 'rec-card__signal rec-card__signal--neutral';
      ranked.textContent = '排名：' + rec.strategies.ranked.join(' > ');
      card.appendChild(ranked);
    }
  }

  return card;
}

const FORCE_LABEL = {
  foreign: '外資現貨',
  futures: '外資期貨',
  tsm_adr: 'TSM ADR',
  institutional: '投信',
  dealer: '自營商',
  government: '公股行庫',
  retail: '散戶',
};

const QUALITY_LABEL = {
  strong_inflow: '強勁流入',
  inflow: '流入',
  neutral: '中性',
  outflow: '流出',
  strong_outflow: '強勁流出',
};

const DIRECTION_LABEL = {
  bullish: '偏多',
  bearish: '偏空',
  mixed: '分歧',
};

const TREND_CLASS = {
  bullish: 'trend-bullish',
  bearish: 'trend-bearish',
  neutral: '',
};

async function renderCapitalFlowDetail(host, btn) {
  try {
    var resp = await fetch('/api/capital-flow/daily', { credentials: 'same-origin' });
    if (!resp.ok) {
      throw new Error('HTTP ' + resp.status);
    }
    var daily = await resp.json();
    host.innerHTML = buildCapitalFlowDetailHTML(daily);
    host.style.display = '';
    btn.textContent = '收合明細';
    btn.setAttribute('aria-expanded', 'true');
    btn.disabled = false;
  } catch (err) {
    host.innerHTML =
      '<div class="empty error">資金流明細載入失敗：' + escapeHtml(err.message || String(err)) + '</div>';
    host.style.display = '';
    btn.textContent = '重試載入明細';
    btn.disabled = false;
  }
}

function buildCapitalFlowDetailHTML(daily) {
  var forces = Array.isArray(daily.forces) ? daily.forces : [];
  var rows = forces.map(function (f) {
    var name = FORCE_LABEL[f.force] || f.force || '—';
    var z = typeof f.z_score === 'number' ? formatSigned(f.z_score, { decimals: 2, forceSign: true }) : '—';
    var raw = typeof f.raw_value === 'number' ? formatNumber(f.raw_value, { decimals: 1 }) : '—';
    var trend = f.trend || 'neutral';
    var cls = TREND_CLASS[trend] || '';
    return (
      '<tr>' +
      '<td>' + escapeHtml(name) + '</td>' +
      '<td class="text-right ' + cls + '">' + z + '</td>' +
      '<td class="text-right">' + raw + '</td>' +
      '<td><span class="tier-badge ' + cls + '">' + escapeHtml(trend) + '</span></td>' +
      '</tr>'
    );
  }).join('');

  var r = daily.resonance || {};
  var aligned = Array.isArray(r.aligned) ? r.aligned.map(function (f) { return FORCE_LABEL[f] || f; }).join('、') : '--';
  var opposing = Array.isArray(r.opposing) && r.opposing.length > 0
    ? r.opposing.map(function (f) { return FORCE_LABEL[f] || f; }).join('、')
    : '無';
  var dirLabel = DIRECTION_LABEL[r.direction] || r.direction || '--';
  var coef = typeof r.coefficient === 'number' ? formatNumber(r.coefficient, { decimals: 2 }) : '—';

  var qualityCls = '';
  var ql = (daily.quality_label || '').toLowerCase();
  if (ql === 'strong_inflow' || ql === 'inflow') qualityCls = 'trend-bullish';
  else if (ql === 'strong_outflow' || ql === 'outflow') qualityCls = 'trend-bearish';
  var qualityText = QUALITY_LABEL[ql] || daily.quality_label || '--';
  var qScore = typeof daily.quality_score === 'number' ? formatNumber(daily.quality_score, { decimals: 2 }) : '—';

  return (
    '<div class="panel capital-flow-panel mt-md">' +
    '  <div class="capital-flow-panel__header">' +
    '    <div><span class="text-muted">市場品質</span> ' +
    '      <span class="tier-badge ' + qualityCls + '">' + escapeHtml(qualityText) + '</span> ' +
    '      <span class="text-muted">（' + qScore + '）</span>' +
    '    </div>' +
    '    <div><span class="text-muted">共振</span> ' + escapeHtml(dirLabel) + ' ' +
    '      <span class="text-muted">（係數 ' + coef + '）</span>' +
    '    </div>' +
    '  </div>' +
    '  <p class="capital-flow-panel__summary">' + escapeHtml(daily.summary || '尚無資料') + '</p>' +
    '  <table class="capital-flow-table">' +
    '    <thead><tr><th>勢力</th><th class="text-right">Z-score</th><th class="text-right">原始值（億）</th><th>趨勢</th></tr></thead>' +
    '    <tbody>' + rows + '</tbody>' +
    '  </table>' +
    '  <div class="capital-flow-panel__meta text-muted">' +
    '    對齊：' + escapeHtml(aligned) + ' ／ 對立：' + escapeHtml(opposing) +
    '  </div>' +
    '</div>'
  );
}

export async function renderHomeTierSections() {
  var tier = await getTier();
  var container = document.getElementById('page-home');
  if (!container) return;

  var existed = document.getElementById('home-tier-sections');
  if (existed) existed.remove();

  var root = document.createElement('div');
  root.id = 'home-tier-sections';

  if (!tier || tier === 'free') {
    container.appendChild(root);
    root.appendChild(buildTierCTA());
    return;
  }

  // E6a: 移除重複的 /api/capital-flow/summary 與 /api/events/prediction（主軌已處理）
  var reportsFetch = tier === 'premium' ? silentGetJSON('/api/reports/latest') : Promise.resolve(null);
  var responses = await Promise.all([
    silentGetJSON('/api/events/calendar'),
    silentGetJSON('/api/recommendations'),
    reportsFetch,
  ]);
  var eventsCal = responses[0];
  var recs = responses[1];
  var report = responses[2];

  if (eventsCal && Array.isArray(eventsCal.events) && eventsCal.events.length > 0) {
    var list = document.createElement('div');
    list.className = 'event-list';
    list.id = 'home-event-calendar';
    eventsCal.events.slice(0, 5).forEach(function (e) {
      list.appendChild(buildEventCard(e));
    });
    root.appendChild(buildTierSection('近期事件', [list]));
  }

  if (recs && (recs.strategies || recs.market)) {
    var recList = document.createElement('div');
    recList.className = 'rec-list';
    recList.appendChild(buildRecCard(recs));
    root.appendChild(buildTierSection('策略推薦', [recList]));
  }

  if (tier === 'premium' && report) {
    var panel = document.createElement('div');
    panel.className = 'panel';
    var pre = document.createElement('pre');
    pre.style.cssText = 'white-space:pre-wrap;font-size:0.92rem;line-height:1.6;margin:0;';
    pre.textContent = report.summary || '尚無報告';
    panel.appendChild(pre);
    root.appendChild(buildTierSection('今日市場報告', [panel]));
  }

  container.appendChild(root);
}