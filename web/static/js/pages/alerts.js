// Alert management page
import { getJSON } from '../shared/app-utils.js';
import { escapeHtml } from '../shared/utils.js';

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
    return;
  }
  el.classList.remove('loading');
  const severityMap = { critical: '嚴重', warning: '警告', info: '資訊' };
  const rows = data.alerts.map(a => {
    const sevClass = a.severity === 'critical' ? 'err' : a.severity === 'warning' ? 'warn' : 'info';
    const ackBtn = a.acknowledged ? '<span class="badge ok">已確認</span>' : `<button class="pipeline-action" onclick="acknowledgeAlert('${escapeHtml(a.id)}')">確認</button>`;
    return `<tr><td>${new Date(a.timestamp).toLocaleString('zh-TW')}</td><td><span class="badge ${sevClass}">${escapeHtml(severityMap[a.severity]) || escapeHtml(a.severity)}</span></td><td>${escapeHtml(a.rule)}</td><td>${escapeHtml(a.message)}</td><td>${a.value !== undefined ? a.value.toFixed(2) : '-'}</td><td>${ackBtn}</td></tr>`;
  }).join('');
  el.innerHTML = `<div style="display:flex;justify-content:flex-end;margin-bottom:6px"><button onclick="exportTableToCSV('alertsTable','alerts_export.csv')" style="font-size:11px;padding:3px 10px;border-radius:4px;border:1px solid var(--border);background:var(--bg);color:var(--text);cursor:pointer">📥 匯出 CSV</button></div><table id="alertsTable"><thead><tr><th>時間</th><th>嚴重度</th><th>規則</th><th>訊息</th><th>數值</th><th>操作</th></tr></thead><tbody>${rows}</tbody></table>`;
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

export async function loadAlerts() {
  try {
    const data = await getJSON('/api/alerts');
    renderAlerts(data);
  } catch (e) {
    console.error(e);
  }
}

if (typeof window !== "undefined") Object.assign(window, { loadAlerts });
