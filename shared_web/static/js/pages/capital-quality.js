// shared_web/static/js/pages/capital-quality.js
//
// Stage 6 PR#2：admin_web「資料品質」頁。
// 來源：
//   - GET /api/health/aggregate   ：4-tier 健康聚合（可選，失敗不阻塞）
//   - GET /api/dashboard/data-channels：各通道狀態與最近更新時間
//   - GET /api/alerts             ：嚴重警報整合
// 顯示：摘要卡片 + 通道表格（依新鮮度著色）+ 錯誤展開 + critical alerts。

import { silentGetJSON, escapeHtml, renderEmptyState, renderMissingState, renderErrorState, formatDate } from '../shared/app-utils.js';
import { fmtSafeNumber } from '../shared/format-metric.js';

const STALE_WARNING_MS = 2 * 60 * 60 * 1000;
const STALE_CRITICAL_MS = 6 * 60 * 60 * 1000;
const RETRY_ID = 'capital-quality';

export async function loadCapitalQuality() {
  const el = document.getElementById('capitalQualityContent');
  if (!el) return;
  el.classList.remove('loading');
  el.innerHTML = renderMissingState('資料品質', 'loading');

  try {
    const [healthAgg, channelsData, alertsData] = await Promise.all([
      silentGetJSON('/api/health/aggregate').catch(function () { return null; }),
      silentGetJSON('/api/dashboard/data-channels').catch(function () { return null; }),
      silentGetJSON('/api/alerts').catch(function () { return null; }),
    ]);
    renderCapitalQuality({ healthAgg, channelsData, alertsData });
  } catch (err) {
    console.error('[capital-quality] load failed', err);
    el.classList.remove('loading');
    el.innerHTML = renderErrorState('資料品質', RETRY_ID);
    const btn = el.querySelector('[data-retry="' + RETRY_ID + '"]');
    if (btn) btn.addEventListener('click', loadCapitalQuality);
  }
}

export function renderCapitalQuality(payload) {
  const el = document.getElementById('capitalQualityContent');
  if (!el) return;

  const channels = payload && payload.channelsData && Array.isArray(payload.channelsData.channels)
    ? payload.channelsData.channels
    : [];
  const channelAlerts = payload && payload.channelsData && Array.isArray(payload.channelsData.alerts)
    ? payload.channelsData.alerts
    : [];
  const alerts = payload && payload.alertsData && Array.isArray(payload.alertsData.alerts)
    ? payload.alertsData.alerts
    : channelAlerts;
  const healthAgg = payload && payload.healthAgg ? payload.healthAgg : null;

  if (channels.length === 0 && !healthAgg) {
    el.classList.remove('loading');
    el.innerHTML = renderErrorState('資料品質', RETRY_ID);
    const btn = el.querySelector('[data-retry="' + RETRY_ID + '"]');
    if (btn) btn.addEventListener('click', loadCapitalQuality);
    return;
  }

  const now = Date.now();
  const rows = channels.map(function (c, idx) {
    const cid = c.channel_id || c.id || c.source_id || 'unknown';
    const displayName = humanizeChannelId(cid);
    const platform = c.platform || displayName;
    const lastUpdate = formatChannelTime(c.updated_at || c.last_fetch_at);
    const errorText = c.last_error || c.error || '';

    // #1785-followup: by-design non-live channels (G01/G02 stubs, disabled
    // channels) must not render as 未知/過期 — they are archived awaiting
    // enablement, same semantics as the home scheduler ARCHIVED_TASKS.
    const archivedReason = ARCHIVED_CHANNELS[cid]
      || (c.enabled === false && (errorText || '已停用'));

    const freshness = computeFreshness(c.updated_at || c.last_fetch_at, now, cid);
    const statusClass = archivedReason ? 'archived' : (errorText ? 'critical' : freshness.className);
    const statusLabel = archivedReason
      ? '歸檔·等待啟用'
      : (c.status_text || c.status || '-');
    const freshnessLabel = archivedReason
      ? archivedReason
      : freshness.label;

    return (
      '<tr class="cq-row ' + statusClass + '" data-idx="' + idx + '">'
      + '<td>'
      +   '<div><strong>' + escapeHtml(platform) + '</strong></div>'
      +   '<div class="text-muted text-xs">' + escapeHtml(displayName) + ' · ' + escapeHtml(c.country || '-') + '</div>'
      + '</td>'
      + '<td>' + escapeHtml(statusLabel) + '</td>'
      + '<td>' + escapeHtml(lastUpdate) + '</td>'
      + '<td><span class="badge ' + (statusClass === 'ok' ? 'ok' : statusClass === 'warn' ? 'warn' : statusClass === 'archived' ? 'muted' : 'err') + '">' + escapeHtml(String(freshnessLabel)) + '</span></td>'
      + '</tr>'
      + (errorText
        ? '<tr class="cq-error" id="cq-error-' + idx + '"><td colspan="4">'
          + '<strong>錯誤詳情</strong><br>' + escapeHtml(errorText)
          + '</td></tr>'
        : '')
    );
  }).join('');

  // #1787 lifecycle: a resolved alert is a closed condition — listing it
  // under 嚴重警報 made the page show fixed errors forever.
  const criticalAlerts = alerts.filter(function (a) {
    return a
      && (a.severity === 'CRITICAL' || a.severity === 'ERROR' || a.severity === 'HIGH')
      && a.status !== 'resolved' && a.status !== 'silenced';
  }).slice(0, 10);

  const summary = buildSummary(channels, criticalAlerts);

  el.classList.remove('loading');
  el.innerHTML = (
    summary
    + '<div class="cq-stale-legend">'
    +   '<span><span class="cq-dot ok"></span> 新鮮 (&lt;2h)</span>'
    +   '<span><span class="cq-dot warn"></span> 待更新 (2h–6h)</span>'
    +   '<span><span class="cq-dot critical"></span> 過期 (&gt;6h) 或異常</span>'
    +   '<span style="color:var(--muted)">日頻通道以交易日計（收盤後更新不計過期）</span>'
    + '</div>'
    + '<div class="cq-table table-scroll">'
    +   '<table class="ranker-table">'
    +     '<thead><tr><th>通道</th><th>狀態</th><th>最後更新</th><th>新鮮度</th></tr></thead>'
    +     '<tbody>' + (rows || '<tr><td colspan="4" class="text-muted">尚無通道</td></tr>') + '</tbody>'
    +   '</table>'
    + '</div>'
    + renderCriticalAlerts(criticalAlerts)
  );

  el.querySelectorAll('.cq-row').forEach(function (row) {
    row.addEventListener('click', function () {
      const idx = row.getAttribute('data-idx');
      const errorRow = document.getElementById('cq-error-' + idx);
      if (errorRow) errorRow.classList.toggle('open');
    });
  });
}

function buildSummary(channels, criticalAlerts) {
  const now = Date.now();
  let ok = 0, warn = 0, critical = 0, archived = 0;
  channels.forEach(function (c) {
    const cid = c.channel_id || c.id || c.source_id || 'unknown';
    const errorText = c.last_error || c.error || '';
    if (ARCHIVED_CHANNELS[cid] || (c.enabled === false && (errorText || '已停用'))) {
      archived++;
      return;
    }
    if (errorText) {
      critical++;
    } else {
      const fresh = computeFreshness(c.updated_at || c.last_fetch_at, now, cid);
      if (fresh.className === 'ok') ok++;
      else if (fresh.className === 'warn') warn++;
      else critical++;
    }
  });

  return (
    '<div class="cq-summary">'
    + '<div class="cq-summary__card ok">'
    +   '<div class="cq-summary__label">正常通道</div>'
    +   '<div class="cq-summary__value" style="color:var(--color-success)">' + ok + '</div>'
    + '</div>'
    + '<div class="cq-summary__card warn">'
    +   '<div class="cq-summary__label">待更新通道</div>'
    +   '<div class="cq-summary__value" style="color:var(--warn)">' + warn + '</div>'
    + '</div>'
    + '<div class="cq-summary__card critical">'
    +   '<div class="cq-summary__label">異常 / 過期</div>'
    +   '<div class="cq-summary__value" style="color:var(--color-danger)">' + critical + '</div>'
    + '</div>'
    + '<div class="cq-summary__card">'
    +   '<div class="cq-summary__label">嚴重警報</div>'
    +   '<div class="cq-summary__value" style="color:var(--color-danger)">' + criticalAlerts.length + '</div>'
    + '</div>'
    + '<div class="cq-summary__card">'
    +   '<div class="cq-summary__label">歸檔·等待啟用</div>'
    +   '<div class="cq-summary__value" style="color:var(--muted)">' + archived + '</div>'
    + '</div>'
    + '</div>'
  );
}

function renderCriticalAlerts(alerts) {
  if (!alerts.length) {
    return '<div class="cq-alerts"><div class="text-muted text-sm">目前無嚴重警報</div></div>';
  }
  const rows = alerts.map(function (a) {
    return (
      '<div class="cq-alert">'
      + '<span class="cq-alert__severity ' + (a.severity === 'CRITICAL' ? 'text-danger' : 'text-warn') + '">' + escapeHtml(a.severity) + '</span>'
      + '<span class="cq-alert__message">' + escapeHtml(a.message || a.rule || '-') + '</span>'
      + '<span class="cq-alert__time">' + escapeHtml(formatAlertTime(a.timestamp)) + '</span>'
      + '</div>'
    );
  }).join('');
  return (
    '<div class="cq-alerts panel wide">'
    + '<h3 class="m-0" style="font-size:14px;margin-bottom:10px">🔥 嚴重警報（最近 ' + alerts.length + ' 筆）</h3>'
    + rows
    + '</div>'
  );
}

// By-design archived channels (G01/G02 stubs, disabled channels).
// Same semantics as the home dashboard ARCHIVED_TASKS: these are NOT
// failures — render them as 歸檔·等待啟用 with the reason instead of
// 未知/過期, so the page distinguishes "known not-yet-live" from "broken".
// G01/G02 已接線（FinMind TaiwanStockHoldingSharesPer / TaiwanDailyShortSaleBalances，
// PR 2026-09-01）——tdcc 走週期 freshness 規則、twse_sbl 走日頻規則，離開歸檔清單。
const ARCHIVED_CHANNELS = {
  twse_etf: '上游 TWT44U 移除 · 改 Fubon PCF 替代源',
  // 上游 BFI84U/ODDLOT 報表 2026-08 移除；known_issues
  // twse_oddlot_upstream_60d 追蹤，短期由 twse_capital_flow 代理。
  twse_oddlot: '上游報表移除 · 暫由 twse_capital_flow 代理',
  // #1758 決策：key 過期且三類資料均有 FinMind 免費替代。
  tej: '暫不開通（#1758）· FinMind 替代',
};
// 週期通道：資料每週更新一次，過期判定用「資料日期 < 最近已收盤週五」。
const WEEKLY_TW_CHANNELS = new Set(['tdcc_equity_dispersion']);

// Taiwan-market daily channels: data lands after market close (13:30+),
// often in the evening batch. A fixed 6h threshold flags them 過期 every
// morning — freshness for these must be trading-day-aware: fresh if the
// last update falls on/after the most recent settled trading day's close.
const DAILY_TW_CHANNELS = new Set([
  'twse_oddlot', 'government_broker', 'government_flow',
  'tdcc_equity_dispersion', 'twse_sbl', 'twse_etf',
]);

// The Taipei calendar date (YYYY-MM-DD) of the most recent weekday whose
// 13:30 close has passed. Holiday-blind by design — same known follow-up
// as the gov_flow coverage expectation (see #1787 comments).
function lastSettledTradingDay(nowMs) {
  const shifted = new Date(nowMs + 8 * 3600 * 1000); // Taipei wall clock as UTC
  for (let i = 0; i < 7; i++) {
    const day = new Date(shifted.getTime());
    day.setUTCDate(day.getUTCDate() - i);
    const wd = day.getUTCDay();
    if (wd === 0 || wd === 6) continue;
    // 13:30 Taipei = 05:30 UTC
    const settleUTC = Date.UTC(day.getUTCFullYear(), day.getUTCMonth(), day.getUTCDate(), 5, 30, 0);
    if (nowMs >= settleUTC) {
      return day.toISOString().slice(0, 10);
    }
  }
  return '';
}

function taipeiDateKey(tsMs) {
  return new Date(tsMs + 8 * 3600 * 1000).toISOString().slice(0, 10);
}

// The most recent completed Friday (Taipei date key): weekly data dated on
// or after it covers the current week. A Friday's own data counts once
// published (i.e. from Friday evening / Saturday onward).
function lastCompletedFriday(nowMs) {
  const shifted = new Date(nowMs + 8 * 3600 * 1000);
  for (let i = 1; i <= 7; i++) {
    const day = new Date(shifted.getTime());
    day.setUTCDate(day.getUTCDate() - i);
    if (day.getUTCDay() === 5) { // Friday
      // Friday 13:30 close: data "exists" for this week from Fri 13:30 TW.
      const publishUTC = Date.UTC(day.getUTCFullYear(), day.getUTCMonth(), day.getUTCDate(), 5, 30, 0);
      if (nowMs >= publishUTC) return day.toISOString().slice(0, 10);
      // Friday morning: the week isn't complete yet — use the prior Friday.
      const prev = new Date(day.getTime() - 7 * 24 * 3600 * 1000);
      return prev.toISOString().slice(0, 10);
    }
  }
  return '';
}

function computeFreshness(value, now, channelId) {
  const ts = parseTimestamp(value);
  if (!ts) {
    return { className: 'critical', label: '未知' };
  }
  const age = now - ts;
  if (age < STALE_WARNING_MS) return { className: 'ok', label: '新鮮' };
  if (channelId && DAILY_TW_CHANNELS.has(channelId)) {
    // Trading-day-aware: stale only if the update predates the most recent
    // settled trading day. Comparing calendar DATES (not hours) — some
    // daily channels land their batch in the morning
    // (government_broker 10:46 observed), so hour-level cutoffs would
    // falsely expire them every morning.
    const settledDay = lastSettledTradingDay(now);
    if (settledDay && taipeiDateKey(ts) >= settledDay) {
      return { className: 'warn', label: '今日待更新' };
    }
    return { className: 'critical', label: '過期' };
  }
  if (channelId && WEEKLY_TW_CHANNELS.has(channelId)) {
    // Weekly channels (集保分散表 data dated Friday): fresh if the last
    // update covers the most recent completed week.
    const fridayKey = lastCompletedFriday(now);
    if (fridayKey && taipeiDateKey(ts) >= fridayKey) {
      return { className: 'warn', label: '本週待更新' };
    }
    return { className: 'critical', label: '過期' };
  }
  if (age < STALE_CRITICAL_MS) return { className: 'warn', label: '待更新' };
  return { className: 'critical', label: '過期' };
}

function parseTimestamp(value) {
  if (!value || typeof value !== 'string') return null;
  const d = new Date(value);
  if (!isNaN(d.getTime()) && d.getFullYear() > 2000) return d.getTime();
  return null;
}

function formatChannelTime(value) {
  if (!value || typeof value !== 'string') return '—';
  if (value.startsWith('上次失敗:')) return value;
  const d = new Date(value);
  if (isNaN(d.getTime()) || d.getFullYear() < 2000) return value;
  return d.toLocaleString('zh-TW');
}

function formatAlertTime(value) {
  if (!value) return '-';
  const d = new Date(value);
  if (isNaN(d.getTime())) return String(value);
  return d.toLocaleString('zh-TW');
}

function humanizeChannelId(id) {
  if (!id) return '未知';
  const map = {
    us_yahoo: 'Yahoo Finance 美國',
    us_spx: 'S&P 500',
    us_ndx: 'NASDAQ 100',
    us_dji: '道瓊工業指數',
    sox_index: '費城半導體指數',
    us_nvda: 'NVIDIA',
    us_aapl: 'Apple',
    us_msft: 'Microsoft',
    tsm_adr: '台積電 ADR',
    finmind: 'FinMind',
    fugle: 'Fugle 富果',
    fubon: '富邦證券',
    tej: 'TEJ 台灣經濟新報',
    twse_replay: 'TWSE 回放',
    twse_capital_flow: 'TWSE 三大法人',
    twse_margin: 'TWSE 融資融券',
    export_statistics: '海關進出口統計',
    tsmc_revenue: '台積電月營收',
    geopolitical: '地緣政治風險',
    geopolitical_taiwan: '台灣地緣政治',
    frankfurter_fx: 'Frankfurter 匯率',
    janus_regime: 'JANUS 盤勢偵測',
  };
  return map[id] || id.replace(/_/g, ' ');
}
