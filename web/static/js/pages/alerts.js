// Alert management page — Phase 2A lifecycle support with pagination & professional styling
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

const PAGE_SIZE = 20;

let currentAlerts = [];
let currentFilter = 'all';
let currentPage = 0;

function formatTime(ts) {
  if (!ts) return '-';
  return new Date(ts).toLocaleString('zh-TW');
}

function formatDate(ts) {
  if (!ts) return '-';
  return new Date(ts).toLocaleDateString('zh-TW');
}

function formatTimeShort(ts) {
  if (!ts) return '-';
  return new Date(ts).toLocaleTimeString('zh-TW', { hour: '2-digit', minute: '2-digit' });
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
    <span class="badge ${STATUS_CLASS.triggered}">觸發中 ${stats.triggered}</span>
    <span class="badge ${STATUS_CLASS.acknowledged}">已確認 ${stats.acknowledged}</span>
    <span class="badge ${STATUS_CLASS.resolved}">已解決 ${stats.resolved}</span>
    <span class="badge ${STATUS_CLASS.silenced}">已抑制 ${stats.silenced}</span>
    <span class="badge" style="background:var(--bg);color:var(--muted);border:1px solid var(--border)">總計 ${stats.total}</span>
  `;
}

function updateFilterButtons() {
  document.querySelectorAll('#alertFilters .view-btn').forEach(btn => {
    btn.classList.toggle('active', btn.dataset.filter === currentFilter);
  });
}

function getFilteredAlerts() {
  if (currentFilter === 'all') return currentAlerts;
  return currentAlerts.filter(a => (a.status || 'triggered') === currentFilter);
}

function renderPagination(totalItems, page, onPageChange) {
  const totalPages = Math.ceil(totalItems / PAGE_SIZE);
  if (totalPages <= 1) return '';
  return `
    <div class="table-pagination" style="margin-top:10px">
      <span>顯示 <strong>${Math.min(page * PAGE_SIZE + 1, totalItems)}-${Math.min((page + 1) * PAGE_SIZE, totalItems)}</strong> / 共 <strong>${totalItems}</strong> 筆</span>
      <div style="display:flex;gap:6px;align-items:center">
        <button onclick="window._alertPage=0;window._alertRender()" ${page===0?'disabled':''}>« 首頁</button>
        <button onclick="window._alertPage=${page-1};window._alertRender()" ${page===0?'disabled':''}>‹ 上一頁</button>
        <span style="padding:0 8px">第 ${page + 1} / ${totalPages} 頁</span>
        <button onclick="window._alertPage=${page+1};window._alertRender()" ${page>=totalPages-1?'disabled':''}>下一頁 ›</button>
        <button onclick="window._alertPage=${totalPages-1};window._alertRender()" ${page>=totalPages-1?'disabled':''}>末頁 »</button>
      </div>
    </div>
  `;
}

function renderAlertTable() {
  const el = document.getElementById('alertsPanel');
  if (!el) return;

  if (!currentAlerts || currentAlerts.length === 0) {
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
  const stats = buildStats(currentAlerts);
  renderStats(stats);

  const filtered = getFilteredAlerts();

  if (filtered.length === 0) {
    el.innerHTML = `<div class="empty" style="padding:20px 0;text-align:center;color:var(--muted)">此狀態下沒有警報</div>`;
    return;
  }

  const totalPages = Math.ceil(filtered.length / PAGE_SIZE);
  if (currentPage >= totalPages) currentPage = Math.max(0, totalPages - 1);
  const startIdx = currentPage * PAGE_SIZE;
  const pagedItems = filtered.slice(startIdx, startIdx + PAGE_SIZE);

  const rows = pagedItems.map(a => {
    const sevClass = SEVERITY_CLASS[a.severity] || 'info';
    const statusClass = STATUS_CLASS[a.status] || 'err';
    const statusLabel = STATUS_MAP[a.status] || (a.status || '觸發中');

    // Time display: if recurring, show date + time range
    let timeCell = formatTime(a.timestamp);
    if (a.count > 1 && (a.first_seen || a.last_seen)) {
      const first = formatDate(a.first_seen);
      const last = formatTimeShort(a.last_seen);
      timeCell = `<div>${formatDate(a.timestamp)} ${formatTimeShort(a.timestamp)}</div>`
        + `<div style="font-size:11px;color:var(--muted)">${first} → ${last}</div>`;
    }

    // Dedup badge merged into status cell
    const dedupBadge = (a.count > 1)
      ? ` <span class="badge warn" style="font-size:10px;padding:1px 5px">×${a.count}</span>`
      : '';

    // Disposition tracking: who and when
    let disposition = '-';
    if (a.status === 'resolved' && a.resolved_by) {
      disposition = `<span class="badge ok" style="font-size:10px;padding:1px 5px">${escapeHtml(a.resolved_by)} ${formatDate(a.resolved_at)}</span>`;
    } else if (a.acknowledged) {
      const by = a.acknowledged_by === 'auto-handler' ? '系統' : escapeHtml(a.acknowledged_by);
      disposition = `<span class="badge warn" style="font-size:10px;padding:1px 5px">${by} ${formatDate(a.acknowledged_at)}</span>`;
    }

    // Action buttons
    let actions = '';
    if (!a.acknowledged) {
      actions += `<button class="pipeline-action" onclick="acknowledgeAlert('${escapeHtml(a.id)}')">確認</button>`;
    }
    if (a.acknowledged && a.status !== 'resolved') {
      actions += `<button class="pipeline-action" onclick="resolveAlert('${escapeHtml(a.id)}')">解決</button>`;
    }
    if (a.status === 'resolved') {
      actions += `<span style="color:var(--color-success);font-size:12px">✓ 已解決</span>`;
    }
    if (!actions) actions = '-';

    return `<tr>
      <td style="white-space:nowrap">${timeCell}</td>
      <td><span class="badge ${statusClass}" style="font-size:11px;padding:2px 7px">${escapeHtml(statusLabel)}</span>${dedupBadge}</td>
      <td><span class="badge ${sevClass}" style="font-size:11px;padding:2px 7px">${escapeHtml(SEVERITY_MAP[a.severity]) || escapeHtml(a.severity)}</span></td>
      <td style="font-size:12px">${escapeHtml(a.rule)}</td>
      <td style="font-size:12px;max-width:280px;overflow:hidden;text-overflow:ellipsis" title="${escapeHtml(a.message)}">${escapeHtml(a.message)}</td>
      <td style="white-space:nowrap">${disposition}</td>
      <td style="white-space:nowrap">${actions}</td>
    </tr>`;
  }).join('');

  const paginationHtml = renderPagination(filtered.length, currentPage, (p) => {
    currentPage = p;
    renderAlertTable();
  });

  el.innerHTML = `<div style="display:flex;justify-content:flex-end;margin-bottom:6px">
    <button onclick="exportTableToCSV('alertsTable','alerts_export.csv')" class="pipeline-action" style="font-size:11px">📥 匯出 CSV</button>
  </div>
  <div class="table-wrapper">
  <table id="alertsTable" style="font-size:12px">
    <thead><tr>
      <th>時間</th><th>狀態</th><th>嚴重度</th><th>規則</th><th>訊息</th><th>處置記錄</th><th>操作</th>
    </tr></thead>
    <tbody>${rows}</tbody>
  </table>
  </div>
  ${paginationHtml}`;
}

export function renderAlerts(data) {
  const el = document.getElementById('alertsPanel');
  if (!el) return;
  if (!data || !data.alerts) {
    currentAlerts = [];
  } else {
    currentAlerts = data.alerts;
  }
  currentPage = 0;
  renderAlertTable();
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
  currentPage = 0;
  updateFilterButtons();
  renderAlertTable();
}

// Expose render helper for pagination callbacks
if (typeof window !== "undefined") {
  window._alertPage = 0;
  window._alertRender = () => {
    currentPage = window._alertPage || 0;
    renderAlertTable();
  };
  Object.assign(window, {
    loadAlerts, acknowledgeAlert, resolveAlert, showUnacknowledgedOnly, setAlertFilter
  });
}
