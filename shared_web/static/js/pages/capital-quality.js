// shared_web/static/js/pages/capital-quality.js
//
// Stage 6 PR#2：admin_web「資料品質」頁。
// 來源：
//   - GET /api/health/aggregate   ：4-tier 健康聚合（PR#1 新增）
//   - GET /api/dashboard/data-channels：每個 channel 的最後更新狀態
// 顯示 tier 卡片 + 各通道健康狀態表。

import { silentGetJSON, escapeHtml, renderEmptyState } from '../shared/app-utils.js';

export async function loadCapitalQuality() {
  await Promise.all([loadHealthAggregate(), loadDataChannelsHealth()]);
}

export async function loadHealthAggregate() {
  const data = await silentGetJSON('/api/health/aggregate');
  renderHealthTiers(data);
}

export function renderHealthTiers(data) {
  const el = document.getElementById('capitalHealthTiersContent');
  if (!el) return;
  if (!data || !data.tiers) {
    el.classList.remove('loading');
    el.innerHTML = renderEmptyState('健康聚合回傳為空');
    return;
  }
  var tierCards = Object.keys(data.tiers).map(function (name) {
    var tier = data.tiers[name] || {};
    var cls = tier.ok ? 'tier-badge tier-badge--bullish' : 'tier-badge tier-badge--bearish';
    var status = tier.ok ? '✓ 健康' : '⚠ 異常';
    var latency = typeof tier.latency_ms === 'number' ? tier.latency_ms + ' ms' : '—';
    var reason = tier.reason ? '<div class="tier-reason">' + escapeHtml(tier.reason) + '</div>' : '';
    return '<div class="tier-card ' + cls + '">'
      + '<h4>' + escapeHtml(name) + '</h4>'
      + '<div class="tier-status">' + status + '</div>'
      + '<div class="tier-latency">' + latency + '</div>'
      + reason
      + '</div>';
  }).join('');
  el.classList.remove('loading');
  var overallCls = data.overall && data.overall.ok ? 'ok' : 'fail';
  var overallLabel = data.overall && data.overall.ok ? '整體：健康' : '整體：異常';
  el.innerHTML = '<div class="tier-grid">' + tierCards + '</div>'
    + '<div class="health-overall"><span class="' + overallCls + '">' + escapeHtml(overallLabel) + '</span></div>';
}

export async function loadDataChannelsHealth() {
  var data = await silentGetJSON('/api/dashboard/data-channels');
  renderDataChannelsHealth(data);
}

export function renderDataChannelsHealth(data) {
  var el = document.getElementById('capitalDataChannelsContent');
  if (!el) return;
  var channels = data && Array.isArray(data.channels) ? data.channels : [];
  if (channels.length === 0) {
    el.classList.remove('loading');
    el.innerHTML = renderEmptyState('尚無資料通道');
    return;
  }
  var rows = channels.map(function (c) {
    var ok = c.healthy === true || c.status === 'healthy' || c.status === 'fresh';
    return '<tr>'
      + '<td><code>' + escapeHtml(c.name || c.id || c.source_id || '-') + '</code></td>'
      + '<td>' + escapeHtml(c.last_update || c.last_fetch_at || c.updated_at || '-') + '</td>'
      + '<td><span class="' + (ok ? 'ok' : 'fail') + '">' + (ok ? '✓' : '⚠') + '</span></td>'
      + '</tr>';
  }).join('');
  el.classList.remove('loading');
  el.innerHTML = '<div class="table-scroll mt-sm">'
    + '<table class="ranker-table">'
    +   '<thead><tr><th>通道</th><th>最後更新</th><th>狀態</th></tr></thead>'
    +   '<tbody>' + rows + '</tbody>'
    + '</table>'
    + '</div>';
}
