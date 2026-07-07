import { metricCard } from './metric-card.js';
import { fmtSignedPct } from '../shared/format-metric.js';
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
  header.className = 'home-section__header';
  var h2 = document.createElement('h2');
  h2.textContent = '解鎖更多分析';
  var sub = document.createElement('span');
  sub.className = 'home-section__subtitle';
  sub.textContent = '註冊即可獲得 7 天免費 Premium 試用';
  header.appendChild(h2);
  header.appendChild(sub);
  section.appendChild(header);
  var actions = document.createElement('div');
  actions.className = 'tier-cta__actions';
  var btn = document.createElement('button');
  btn.className = 'btn btn--primary';
  btn.type = 'button';
  btn.textContent = '免費註冊';
  btn.addEventListener('click', function () { window.switchPage('register'); });
  actions.appendChild(btn);
  section.appendChild(actions);
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

  // P1: parallelize all 5 independent API calls via Promise.all (saves ~4 RTT).
  var reportsFetch = tier === 'premium' ? silentGetJSON('/api/reports/latest') : Promise.resolve(null);
  var responses = await Promise.all([
    silentGetJSON('/api/capital-flow/summary'),
    silentGetJSON('/api/events/prediction'),
    silentGetJSON('/api/events/calendar'),
    silentGetJSON('/api/recommendations'),
    reportsFetch,
  ]);
  var capitalFlow = responses[0];
  var events = responses[1];
  var eventsCal = responses[2];
  var recs = responses[3];
  var report = responses[4];

  if (capitalFlow && Array.isArray(capitalFlow.forces) && capitalFlow.forces.length > 0) {
    var cards = capitalFlow.forces.slice(0, 4).map(function (f) {
      var val = f.z_score ? fmtSignedPct(f.z_score / 10) : '--';
      var cls = f.z_score > 0.5 ? 'trend-bullish' : f.z_score < -0.5 ? 'trend-bearish' : '';
      return metricCard({ label: f.name || f.source, value: val, trend: cls, sub: f.direction || '' });
    });
    root.appendChild(buildTierSection('資金流向', [buildMetricGrid(4, cards)]));
  }

  if (events && Array.isArray(events.predictions) && events.predictions.length > 0) {
    var preds = events.predictions.slice(0, 5).map(function (p) {
      var cls = p.direction === 'inflow' ? 'trend-bullish' : p.direction === 'outflow' ? 'trend-bearish' : '';
      var label = p.direction === 'inflow' ? '流入' : p.direction === 'outflow' ? '流出' : '中性';
      return metricCard({ label: label, value: (p.confidence * 100).toFixed(0) + '%', trend: cls, sub: '' });
    });
    root.appendChild(buildTierSection('5 日資金流預測', [buildMetricGrid(5, preds)]));
  }

  if (eventsCal && Array.isArray(eventsCal.events) && eventsCal.events.length > 0) {
    var list = document.createElement('div');
    list.className = 'event-list';
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