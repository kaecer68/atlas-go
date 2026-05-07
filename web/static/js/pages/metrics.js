// Metrics monitoring page
import { getJSON } from '../shared/app-utils.js';

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
      const cp = await getJSON('/api/dashboard/capital-phase').catch(() => null);
      capitalPhaseEl.textContent = cp && cp.phase ? cp.phase : 'Simulation';
    }
    updateMetricsTrend(data);
  } catch (err) {
    console.error('loadMetrics error:', err);
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
