// Metrics monitoring page
import { getJSON, silentGetJSON } from '../shared/app-utils.js';

export async function loadMetrics() {
  try {
    const data = await getJSON('/api/dashboard/metrics?type=all');
    const screeningRate = data && data.screening_rate != null ? (data.screening_rate * 100).toFixed(1) + '%' : '-';
    const screeningRateEl = document.getElementById('screeningRate');
    if (screeningRateEl) screeningRateEl.textContent = screeningRate;
    const alertsTriggeredEl = document.getElementById('alertsTriggered');
    if (alertsTriggeredEl) alertsTriggeredEl.textContent = data && data.alerts_triggered != null ? data.alerts_triggered : '-';
    const capitalPhaseEl = document.getElementById('capitalPhase');
    if (capitalPhaseEl) {
      const cp = await silentGetJSON('/api/dashboard/capital-phase');
      capitalPhaseEl.textContent = cp && cp.phase ? cp.phase : 'Simulation';
    }
    updateMetricsTrend(data);
  } catch (err) {
    console.error('loadMetrics error:', err);
  }

  try {
    const storageData = await silentGetJSON('/api/metrics/storage');
    if (storageData && storageData.total_deleted != null) {
      const storageDeletedEl = document.getElementById('storageDeleted');
      if (storageDeletedEl) storageDeletedEl.textContent = storageData.total_deleted;
      renderStorageCleanup(storageData);
    }
  } catch (err) {
    console.error('loadStorageMetrics error:', err);
  }
}

export function updateMetricsTrend(data) {
  const trendDiv = document.getElementById('metricsTrend');
  if (!trendDiv) return;
  let html = '<div class="grid cols-2">';
  if (data && data.alerts_by_type && Object.keys(data.alerts_by_type).length > 0) {
    html += '<div class="panel"><h3>警報類型分佈</h3><table><thead><tr><th>類型</th><th>次數</th></tr></thead><tbody>';
    for (const [type, count] of Object.entries(data.alerts_by_type)) {
      html += `<tr><td>${type}</td><td>${count}</td></tr>`;
    }
    html += '</tbody></table></div>';
  } else {
    html += '<div class="panel"><h3>警報類型分佈</h3><div class="empty" style="font-size:12px;color:var(--muted);padding:10px 0">目前尚無警報觸發記錄。當系統偵測到異常時，此處將顯示警報分類統計。</div></div>';
  }
  html += '<div class="panel"><h3>篩選統計</h3><table><thead><tr><th>項目</th><th>數值</th></tr></thead><tbody>';
  html += `<tr><td>總數</td><td>${data && data.screening_total != null ? data.screening_total : '-'}</td></tr>`;
  html += `<tr><td>通過</td><td>${data && data.screening_passed != null ? data.screening_passed : '-'}</td></tr>`;
  html += `<tr><td>拒絕</td><td>${data && data.screening_total != null && data.screening_passed != null ? data.screening_total - data.screening_passed : '-'}</td></tr>`;
  html += '</tbody></table></div>';
  html += '</div>';
  trendDiv.innerHTML = html;
}

export function renderStorageCleanup(data) {
  const panel = document.getElementById('storageCleanupPanel');
  const detail = document.getElementById('storageCleanupDetail');
  if (!panel || !detail) return;

  if (!data || !data.policies || data.policies.length === 0) {
    detail.innerHTML = '<div class="empty">暫無清理記錄</div>';
    return;
  }

  panel.style.display = 'block';
  let html = '<table><thead><tr><th>目錄</th><th>保留天數</th><th>已刪除</th><th>已保留</th><th>最舊保留</th></tr></thead><tbody>';
  for (const p of data.policies) {
    html += `<tr><td>${p.dir || '-'}</td><td>${p.max_age_days || '-'}</td><td>${p.deleted != null ? p.deleted : '-'}</td><td>${p.kept != null ? p.kept : '-'}</td><td>${p.oldest_kept || '-'}</td></tr>`;
  }
  html += '</tbody></table>';
  html += `<div style="margin-top:8px;font-size:12px;color:var(--muted)">總計：刪除 ${data.total_deleted != null ? data.total_deleted : '-'} 個檔案，保留 ${data.total_kept != null ? data.total_kept : '-'} 個檔案</div>`;
  detail.innerHTML = html;
}
