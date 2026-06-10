// Alert management page — Phase 2A lifecycle support
import { getJSON, postJSON, notify } from '../shared/app-utils.js';
import { escapeHtml } from '../shared/utils.js';
import { exportTableToCSV } from './backtest.js';

const STATUS_MAP = {
  triggered: '觸發中',
  acknowledged: '已確認',
  resolved: '已解決',
  silenced: '已抑制',
};

const STATUS_CLASS = {
  triggered: 'err',
  acknowledged: 'warn',
  resolved: 'ok',
  silenced: 'muted',
};

const SEVERITY_MAP = { critical: '嚴重', warning: '警告', info: '資訊' };
const SEVERITY_CLASS = { critical: 'err', warning: 'warn', info: 'info' };

let currentAlerts = [];
let currentFilter = 'all';

function formatTime(ts) {
  if (!ts) return '-';
  return new Date(ts).toLocaleString('zh-TW');
}

function buildStats(alerts) {
  const stats = { triggered: 0, acknowledged: 0, resolved: 0, silenced: 0, total: alerts.length };
  for (const a of alerts) {
    const s = a.status || 'triggered';
    if (stats[s] !== undefined) stats[s]++;
  }
  return stats;
}

function renderStats(stats) {
  const el = document.getElementById('alertSummary');
  if (!el) return;
  el.innerHTML = `
    <span class="badge ${STATUS_CLASS.triggered}">觸發中: <strong>${stats.triggered}</strong></span>
    <span class="badge ${STATUS_CLASS.acknowledged}">已確認: <strong>${stats.acknowledged}</strong></span>
    <span class="badge ${STATUS_CLASS.resolved}">已解決: <strong>${stats.resolved}</strong></span>
    <span class="badge ${STATUS_CLASS.silenced}">已抑制: <strong>${stats.silenced}</strong></span>
    <span class="badge" style="background:var(--bg);color:var(--muted);border:1px solid var(--border)">總計: <strong>${stats.total}</strong></span>
  `;
}

function updateFilterButtons() {
  document.querySelectorAll('.alert-filter-btn').forEach(btn => {
    btn.classList.toggle('active', btn.dataset.filter === currentFilter);
  });
}

export function renderAlerts(data) {
  const el = document.getElementById('alertsPanel');
  if (!el) return;

  if (!data || !data.alerts || data.alerts.length === 0) {
    el.innerHTML = '<div class="empty" style="padding:20px 0;line-height:1.8">' +
      '<div style="font-size:14px;margin-bottom:8px">目前沒有警報</div>' +
      '<div style="font-size:12px;color:var(--muted)">' +
      '警報由系統監控模組觸發，當以下條件發生時會產生警報：<br>' +
      '• 資料通道待更新超過閾值<br>' +
      '• 系統健康度異常<br>' +
      '• 交易風險超過限制<br>' +
      '目前系統運行正常，暫無需要關注的警報。' +
      '</div></div>';
    el.classList.remove('loading');
    renderStats({ triggered: 0, acknowledged: 0, resolved: 0, silenced: 0, total: 0 });
    return;
  }

  el.classList.remove('loading');
  currentAlerts = data.alerts;

  const stats = buildStats(currentAlerts);
  renderStats(stats);

  // Apply status filter
  let filtered = currentAlerts;
  if (currentFilter !== 'all') {
    filtered = currentAlerts.filter(a => (a.status || 'triggered') === currentFilter);
  }

  if (filtered.length === 0) {
    el.innerHTML = `<div class="empty" style="padding:20px 0;text-align:center;color:var(--muted)">此狀態下沒有警報</div>`;
    return;
  }

  const rows = filtered.map(a => {
    const sevClass = SEVERITY_CLASS[a.severity] || 'info';
    const statusClass = STATUS_CLASS[a.status] || 'err';
    const statusLabel = STATUS_MAP[a.status] || (a.status || '觸發中');

    // Dedup info: count > 1 shows recurrence badge
    const dedupBadge = (a.count > 1)
      ? `<span class="badge warn" title="首次: ${formatTime(a.first_seen)}\n最後: ${formatTime(a.last_seen)}">重複 ×${a.count}</span>`
      : '';

    // Acknowledge info: distinguish auto-handler from human
    const ackInfo = a.acknowledged
      ? (a.acknowledged_by === 'auto-handler'
        ? `<span class="badge" style="background:var(--bg);color:var(--muted);border:1px solid var(--border)">自動確認</span>`
        : `<span class="badge ok">已確認</span>`)
      : '';

    // Action buttons
    let actions = '';
    if (!a.acknowledged) {
      actions += `<button class="pipeline-action" onclick="acknowledgeAlert('${escapeHtml(a.id)}')">確認</button>`;
    }
    if (a.acknowledged && a.status !== 'resolved') {
      actions += `<button class="pipeline-action" style="background:var(--color-success);color:#fff;border-color:var(--color-success)" onclick="resolveAlert('${escapeHtml(a.id)}')">解決</button>`;
    }
    if (a.status === 'resolved') {
      actions += `<span class="badge ok">已解決</span>`;
    }
    if (!actions) actions = '-';

    // Timeline row: show first_seen / last_seen when count > 1
    const timeline = (a.count > 1 && (a.first_seen || a.last_seen))
      ? `<div style="font-size:11px;color:var(--muted);margin-top:2px">${formatTime(a.first_seen)} → ${formatTime(a.last_seen)}</div>`
      : '';

    return `<tr>
      <td>${formatTime(a.timestamp)}${timeline}</td>
      <td><span class="badge ${sevClass}">${escapeHtml(SEVERITY_MAP[a.severity]) || escapeHtml(a.severity)}</span></td>
      <td><span class="badge ${statusClass}">${escapeHtml(statusLabel)}</span>${dedupBadge}</td>
      <td>${escapeHtml(a.rule)}</td>
      <td>${escapeHtml(a.message)}</td>
      <td>${typeof a.value === 'number' ? a.value.toFixed(2) : '-'}</td>
      <td>${ackInfo}</td>
      <td>${actions}</td>
    </tr>`;
  }).join('');

  el.innerHTML = `<div style="display:flex;justify-content:flex-end;margin-bottom:6px"><button onclick="exportTableToCSV('alertsTable','alerts_export.csv')" style="font-size:11px;padding:3px 10px;border-radius:4px;border:1px solid var(--border);background:var(--bg);color:var(--text);cursor:pointer">📥 匯出 CSV</button></div>
  <table id="alertsTable"><thead><tr>
    <th>時間</th><th>嚴重度</th><th>狀態</th><th>規則</th><th>訊息</th><th>數值</th><th>確認</th><th>操作</th>
  </tr></thead><tbody>${rows}</tbody></table>`;
}

export async function acknowledgeAlert(alertId) {
  try {
    await postJSON('/api/alerts/acknowledge', { alert_id: alertId, user: 'human' });
    notify('警報已確認', 'success');
    loadAlerts();
  } catch (e) {
    notify('確認失敗: ' + e.message, 'error');
  }
}

export async function resolveAlert(alertId) {
  try {
    await postJSON('/api/alerts/resolve', { alert_id: alertId, user: 'human' });
    notify('警報已解決', 'success');
    loadAlerts();
  } catch (e) {
    notify('解決失敗: ' + e.message, 'error');
  }
}

export async function loadAlerts() {
  try {
    const data = await getJSON('/api/alerts');
    renderAlerts(data);
  } catch (e) {
    console.error(e);
  }
}

export async function showUnacknowledgedOnly() {
  try {
    const data = await getJSON('/api/alerts/unacknowledged');
    renderAlerts(data);
  } catch (e) {
    console.error(e);
  }
}

export function setAlertFilter(filter) {
  currentFilter = filter;
  updateFilterButtons();
  renderAlerts({ alerts: currentAlerts });
}

if (typeof window !== "undefined") Object.assign(window, {
  loadAlerts, acknowledgeAlert, resolveAlert, showUnacknowledgedOnly, setAlertFilter
});
