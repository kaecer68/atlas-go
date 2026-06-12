// Decision-chain page — single aggregate API → 5 collapsible panels.
// Each panel shows progressive insight: event radar → rules → sectors → stocks → exits.
import { getJSON } from '../shared/app-utils.js';
import { escapeHtml } from '../shared/utils.js';
import { narrativeThemeLabel } from '../shared/constants.js';
import { renderStockCell, stockName } from '../names.js';

let lastData = null;
let currentTime = '--:--';
let refreshInterval = null;

export async function loadDecisionChain() {
  const el = document.getElementById('decisionChain');
  if (!el) return;

  el.classList.add('loading');
  el.innerHTML = '<div class="loading-placeholder" style="padding:20px;text-align:center;color:var(--muted)">載入決策鏈資料中…</div>';

  try {
    lastData = await getJSON('/api/dashboard/decision-chain');
    currentTime = new Date().toLocaleTimeString('zh-TW');
    renderDecisionChain(lastData);
  } catch (err) {
    console.error('Decision chain load failed:', err);
    el.innerHTML = '<div class="error-box" style="padding:20px;color:var(--err);text-align:center">載入失敗：' + escapeHtml(String(err.message || err)) + '</div>';
    el.classList.remove('loading');
  }

  // Auto-refresh every 30 seconds while on this page.
  if (!refreshInterval) {
    refreshInterval = setInterval(() => {
      if (document.getElementById('page-decision')?.classList.contains('active')) {
        refreshDecisionChain();
      }
    }, 30000);
  }
}

async function refreshDecisionChain() {
  try {
    lastData = await getJSON('/api/dashboard/decision-chain');
    currentTime = new Date().toLocaleTimeString('zh-TW');
    renderDecisionChain(lastData);
  } catch (err) {
    console.error('Auto-refresh failed:', err);
  }
}

function renderDecisionChain(data) {
  const el = document.getElementById('decisionChain');
  if (!el) return;
  el.classList.remove('loading');

  el.innerHTML = ''
    + renderPanel('⏱ 即時事件雷達', 'events-radar', renderEventsRadar(data), currentTime)
    + renderPanel('🧠 投資心法庫', 'strategies-panel', renderStrategiesPanel(data), '5 層框架 + 4 指標')
    + renderPanel('🔥 產業熱力圖', 'sector-heatmap', renderSectorHeatmap(data))
    + renderPanel('📋 推薦標的', 'recommendations', renderRecommendations(data))
    + renderPanel('🔔 出場提醒', 'exit-alerts', renderExitAlerts(data));
}

function renderPanel(title, panelId, body, sub) {
  return `<div class="dc-panel" id="dc-${panelId}">
    <div class="dc-panel-header" onclick="document.getElementById('dc-${panelId}').classList.toggle('collapsed')">
      <span class="dc-arrow">▼</span>
      <span class="dc-title">${title}</span>
      <span class="dc-sub">${sub || ''}</span>
    </div>
    <div class="dc-panel-body">${body}</div>
  </div>`;
}

function empty(msg) {
  return `<div class="dc-empty" style="padding:16px;color:var(--muted);text-align:center">${msg || '尚無資料'}</div>`;
}

// --- Panel 1: 即時事件雷達 ---
function renderEventsRadar(data) {
  const events = data && data.events;
  if (!events) return empty('尚無事件資料');

  let premarketHtml = '';
  const pm = events.premarket;
  if (pm) {
    const kvs = [];
    if (pm.us_market && pm.us_market.sox_pct !== undefined) {
      const cl = pm.us_market.sox_pct >= 0 ? 'color:var(--bullish);font-weight:600' : 'color:var(--bearish);font-weight:600';
      kvs.push(`<span>SOX <span style="${cl}">${fmtPct(pm.us_market.sox_pct)}</span></span>`);
    }
    if (pm.fx && pm.fx.usd_twd) {
      // USD/TWD 對台股而言是反向：USD 漲 (=TWD 貶) 對台灣不利 → bearish
      const fxCl = pm.fx.change_pct >= 0 ? 'color:var(--bearish)' : 'color:var(--bullish)';
      kvs.push(`<span>USD/TWD ${pm.fx.usd_twd.toFixed(2)} <span style="${fxCl}">${fmtPct(pm.fx.change_pct)}</span></span>`);
    }
    if (pm.foreign_flow && pm.foreign_flow.net_buy_twd !== undefined) {
      kvs.push(`<span>外資淨買超 ${fmtNTD(pm.foreign_flow.net_buy_twd)}</span>`);
    }
    if (pm.bdi && pm.bdi.value) {
      const bdiCl = pm.bdi.deviation_pct >= 0 ? 'color:var(--bullish);font-weight:600' : 'color:var(--bearish);font-weight:600';
      kvs.push(`<span>BDI ${pm.bdi.value} <span style="${bdiCl}">${fmtPct(pm.bdi.deviation_pct)}</span></span>`);
    }
    if (pm.vix && pm.vix.value !== undefined) {
      // VIX 漲=恐慌=風險升高=對台股負面 → 維持系統狀態語意（紅=danger）
      const vixCl = pm.vix.value >= 25 ? 'color:var(--color-danger);font-weight:600' : pm.vix.value >= 20 ? 'color:var(--color-warning);font-weight:600' : 'color:var(--color-success)';
      kvs.push(`<span>VIX ${pm.vix.value.toFixed(1)} <span style="${vixCl}">${fmtPct(pm.vix.change_pct)}</span></span>`);
    }
    if (pm.stress_index && pm.stress_index.dxy !== undefined) {
      const stressCl = pm.stress_index.vix_level >= 25 ? 'color:var(--color-danger)' : 'color:var(--muted)';
      kvs.push(`<span title="DXY ${pm.stress_index.dxy.toFixed(2)} · Oil ${pm.stress_index.oil_pct ? fmtPct(pm.stress_index.oil_pct) : '-'}" style="cursor:help;${stressCl}">壓力指數</span>`);
    }
    if (kvs.length) {
      premarketHtml = `<div class="dc-badge-row" style="margin-bottom:10px">${kvs.map(k => `<span class="badge info" style="font-size:11px">${k}</span>`).join(' ')}</div>`;
    }
  }

  const todayRows = (events.today || []).map(e => {
    const sev = e.severity || 'low';
    const sevEmoji = sev === 'critical' ? '🔴' : sev === 'high' ? '🟠' : sev === 'medium' ? '🟡' : '🟢';
    return `<div class="dc-event-row">
      <span>${sevEmoji} <strong title="${escapeHtml(e.theme)}">${narrativeThemeLabel(e.theme)}</strong></span>
      <span class="text-muted" style="font-size:11px">Conf ${(e.confidence * 100).toFixed(0)}% · Hit ${(e.hit_rate * 100).toFixed(0)}% · ${escapeHtml(e.status || 'active')}</span>
    </div>`;
  }).join('');

  const recentRows = (events.recent || []).slice(0, 5).map(e => {
    return `<div class="dc-event-row" style="opacity:0.7">
      <span>📌 <strong title="${escapeHtml(e.theme)}">${narrativeThemeLabel(e.theme)}</strong></span>
      <span class="text-muted" style="font-size:11px">${timeAgo(e.timestamp)} · Conf ${(e.confidence * 100).toFixed(0)}%</span>
    </div>`;
  }).join('');

  return `<div class="dc-section">
    ${premarketHtml}
    ${events.today && events.today.length ? '<div class="dc-section-title">📡 今日事件 (' + events.today.length + ')</div>' + todayRows : empty('今日暫無事件')}
    ${recentRows ? '<div class="dc-section-title" style="margin-top:8px">📆 近期事件</div>' + recentRows : ''}
  </div>`;
}

// --- Panel 2: 投資心法庫 ---
function renderStrategiesPanel(data) {
  const strategies = data && data.strategies;
  const ci = data && data.core_indicators;
  const ciStrip = ci ? `
    <div class="dc-section" style="display:flex;gap:6px;flex-wrap:wrap;padding:6px 8px;background:var(--surface-2);border-radius:4px;margin-bottom:6px">
      <span class="badge ${ci.foreign_capital_net_twd > 0 ? 'up' : 'down'}" title="外資現貨 (TWD 億)">外資 ${(ci.foreign_capital_net_twd / 1e8).toFixed(1)}</span>
      <span class="badge ${ci.tsm_adr_pct > 0 ? 'up' : 'down'}" title="TSM ADR (%)">TSM ${ci.tsm_adr_pct.toFixed(2)}%</span>
      <span class="badge ${ci.nvda_pct > 0 ? 'up' : 'down'}" title="NVDA (%)">NVDA ${ci.nvda_pct.toFixed(2)}%</span>
      <span class="badge ${ci.dxy_pct < 0 ? 'up' : 'down'}" title="DXY (% change, 跌=資金回流)">DXY ${ci.dxy_pct.toFixed(2)}%</span>
    </div>` : '';
  if (!strategies || !strategies.length) return ciStrip + empty('尚無投資心法（9 條 seeds 未載入或非活躍）');
  const LAYER_COLORS = { L1: 'var(--color-info)', L2: '#a855f7', L3: 'var(--color-success)', L4: 'var(--color-warning)', L5: 'var(--color-danger)' };
  const rows = strategies.map(s => {
    const hitPct = Math.round((s.hit_rate || 0) * 100);
    const barColor = hitPct >= 70 ? 'var(--status-ok)' : hitPct >= 50 ? 'var(--status-warn)' : 'var(--status-err)';
    const layerColor = LAYER_COLORS[s.layer] || 'var(--muted)';
    const themes = (s.themes || []).slice(0, 2).map(t => `<span class="badge muted">${escapeHtml(t)}</span>`).join(' ');
    return `<div class="dc-rule-row" style="border-left:3px solid ${layerColor};padding-left:6px">
      <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:4px">
        <span class="dc-rule-id" title="${escapeHtml(s.id)}">#${escapeHtml(s.id).slice(0, 28)}</span>
        <div style="display:flex;gap:4px">
          <span class="badge muted">${escapeHtml(s.layer)}</span>
          <span class="badge ${s.status === 'active' ? 'ok' : 'warn'}">${escapeHtml(s.status)}</span>
        </div>
      </div>
      <div class="dc-rule-pattern">${escapeHtml(s.name)}</div>
      <div style="display:flex;align-items:center;gap:8px;margin-top:4px">
        <div class="dc-hitbar" style="flex:1;height:6px;background:var(--border);border-radius:3px">
          <div style="width:${hitPct}%;height:100%;background:${barColor};border-radius:3px;transition:width 0.3s"></div>
        </div>
        <span style="font-size:11px;font-weight:600;min-width:36px;text-align:right">${hitPct}%</span>
      </div>
      <div style="margin-top:2px;font-size:10px;color:var(--muted)">
        ${themes}
        <span class="badge ${s.risk === 'low' ? 'ok' : s.risk === 'high' ? 'err' : 'warn'}">${escapeHtml(s.risk)}</span>
        <span class="badge ${s.direction === 'up' ? 'up' : s.direction === 'down' ? 'down' : 'warn'}">${escapeHtml(s.direction)}</span>
      </div>
    </div>`;
  }).join('');
  return `${ciStrip}<div class="dc-section">${rows}</div>`;
}

// --- Panel 3: 產業熱力圖 ---
function renderSectorHeatmap(data) {
  const sectors = data && data.sector_heatmap;
  if (!sectors || !sectors.length) return empty('尚無產業數據');

  const badges = sectors.map(s => {
    const score = typeof s.confidence_score === 'number' ? s.confidence_score : (s.confidence === 'high' ? 0.8 : s.confidence === 'medium' ? 0.5 : 0.2);
    const bg = s.confidence === 'high' ? 'color-mix(in srgb, var(--color-success) 15%, transparent)' : s.confidence === 'medium' ? 'color-mix(in srgb, var(--color-warning) 15%, transparent)' : 'color-mix(in srgb, var(--muted) 10%, transparent)';
    const border = s.confidence === 'high' ? 'var(--color-success)' : s.confidence === 'medium' ? 'var(--color-warning)' : 'var(--muted)';
    const emoji = s.confidence === 'high' ? '🔥' : s.confidence === 'medium' ? '🟡' : '⚪';
    return `<div class="dc-heat-badge" style="background:${bg};border:1px solid ${border};border-radius:8px;padding:8px 10px;display:flex;align-items:center;gap:8px">
      <span style="font-size:14px">${emoji}</span>
      <div style="flex:1">
        <div style="font-size:12px;font-weight:600">${escapeHtml(s.sector)}</div>
        <div style="font-size:10px;color:var(--muted)">${(s.reasons || []).map(escapeHtml).join(' · ')}</div>
        <div style="margin-top:4px;height:4px;background:var(--border);border-radius:2px;overflow:hidden">
          <div style="width:${Math.round(score * 100)}%;height:100%;background:${border}"></div>
        </div>
      </div>
      <span class="badge ${s.confidence === 'high' ? 'ok' : s.confidence === 'medium' ? 'warn' : 'muted'}" title="信心度 ${(score * 100).toFixed(0)}%">${s.confidence}</span>
    </div>`;
  }).join('');

  return `<div class="dc-heat-grid" style="display:flex;flex-wrap:wrap;gap:8px">${badges}</div>`;
}

// --- Panel 4: 推薦標的 ---
function renderRecommendations(data) {
  const recs = data && data.recommendations;
  if (!recs || !recs.length) return empty('暫無推薦標的');

  const rows = recs.map(r => {
    const confPct = Math.round(r.confidence * 100);
    const confColor = confPct >= 80 ? 'var(--color-success)' : confPct >= 60 ? 'var(--color-warning)' : 'var(--muted)';
    const actionBadge = r.action === 'buy' ? 'ok' : r.action === 'sell' || r.action === 'short' ? 'err' : 'warn';
    const price = typeof r.price === 'number' && r.price > 0 ? r.price.toFixed(2) : '-';
    const target = typeof r.target_price === 'number' && r.target_price > 0 ? r.target_price.toFixed(2) : '-';
    const stop = typeof r.stop_loss_price === 'number' && r.stop_loss_price > 0 ? r.stop_loss_price.toFixed(2) : '-';
    return `<tr>
      <td>${renderStockCell(r.symbol)}</td>
      <td>${escapeHtml(r.name || stockName(r.symbol))}</td>
      <td><span class="badge ${actionBadge}">${escapeHtml(r.action)}</span></td>
      <td style="font-size:11px;text-align:right">${price}</td>
      <td style="font-size:11px;text-align:right">${target}</td>
      <td style="font-size:11px;text-align:right">${stop}</td>
      <td><span style="color:${confColor};font-weight:600">${confPct}%</span></td>
      <td style="font-size:11px;color:var(--muted)">${(r.reasons || []).map(escapeHtml).join(', ')}</td>
    </tr>`;
  }).join('');

  return `<div class="dc-table-wrap"><table class="dc-table">
    <thead><tr><th>代號</th><th>名稱</th><th>方向</th><th>參考價</th><th>目標價</th><th>止損價</th><th>置信度</th><th>原因</th></tr></thead>
    <tbody>${rows}</tbody>
  </table></div>`;
}

// --- Panel 5: 出場提醒 ---
function renderExitAlerts(data) {
  const alerts = data && data.exit_alerts;
  if (!alerts || !alerts.length) return empty('目前沒有需要出場提醒的持倉');

  const rows = alerts.map(a => {
    const pnlSign = a.pnl_pct >= 0 ? '+' : '';
    return `<div class="dc-exit-row">
      <span>🔔 ${renderStockCell(a.symbol)} ${escapeHtml(a.name && a.name !== a.symbol ? a.name : '')}</span>
      <span class="badge ${a.pnl_pct >= 10 ? 'up' : a.pnl_pct <= -5 ? 'down' : 'warn'}">
        ${pnlSign}${a.pnl_pct.toFixed(1)}%
      </span>
      <span style="font-size:11px;color:var(--muted);flex:1;text-align:right">${escapeHtml(a.suggestion)}</span>
    </div>`;
  }).join('');

  return `<div class="dc-section">${rows || empty('目前沒有需要出場提醒的持倉')}</div>`;
}

// --- Helpers ---
function fmtPct(v) {
  if (v === undefined || v === null || isNaN(v)) return '-';
  return (v >= 0 ? '+' : '') + v.toFixed(1) + '%';
}

function fmtNTD(v) {
  if (v === undefined || v === null) return '-';
  const abs = Math.abs(v);
  if (abs >= 1e8) return (v / 1e8).toFixed(2) + '億';
  if (abs >= 1e4) return (v / 1e4).toFixed(2) + '萬';
  return v.toFixed(0);
}

function timeAgo(ts) {
  if (!ts) return '';
  const diff = Date.now() - new Date(ts).getTime();
  const mins = Math.floor(diff / 60000);
  if (mins < 60) return mins + '分鐘前';
  const hours = Math.floor(mins / 60);
  if (hours < 24) return hours + '小時前';
  return Math.floor(hours / 24) + '天前';
}

// Refresh individual panel
export async function refreshPanel(panelId) {
  try {
    lastData = await getJSON('/api/dashboard/decision-chain');
    currentTime = new Date().toLocaleTimeString('zh-TW');
    renderPartialPanel(panelId, lastData);
  } catch (err) {
    console.error('Panel refresh failed:', err);
  }
}

function renderPartialPanel(panelId, data) {
  const body = document.querySelector('#dc-' + panelId + ' .dc-panel-body');
  if (!body) return;
  switch (panelId) {
    case 'events-radar': body.innerHTML = renderEventsRadar(data); break;
    case 'strategies-panel': body.innerHTML = renderStrategiesPanel(data); break;
    case 'sector-heatmap': body.innerHTML = renderSectorHeatmap(data); break;
    case 'recommendations': body.innerHTML = renderRecommendations(data); break;
    case 'exit-alerts': body.innerHTML = renderExitAlerts(data); break;
  }
}
