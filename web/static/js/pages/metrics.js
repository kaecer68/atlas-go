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
    updateDerivedKpis(data);
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
    await loadThresholdViolations();
  } catch (err) {
    console.error('loadThresholdViolations error:', err);
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

  try {
    await loadDataQuality();
  } catch (err) {
    console.error('loadDataQuality error:', err);
  }

  const updatedEl = document.getElementById('metricsLastUpdated');
  if (updatedEl) {
    const now = new Date();
    const hh = now.getHours().toString().padStart(2, '0');
    const mm = now.getMinutes().toString().padStart(2, '0');
    const ss = now.getSeconds().toString().padStart(2, '0');
    updatedEl.textContent = `最後更新 ${hh}:${mm}:${ss}`;
  }
}

export function updateDerivedKpis(data) {
  updateScreeningTotal(data);
  updatePendingAlerts(data);
  updateAckRate(data);
}

function updateScreeningTotal(data) {
  const countEl = document.getElementById('screeningTotalCount');
  const labelEl = document.getElementById('screeningTotalLabel');
  if (!countEl || !labelEl) return;
  const total = data && data.screening_total != null ? data.screening_total : null;
  if (total == null) {
    countEl.textContent = '-';
    labelEl.textContent = '尚無資料';
    countEl.style.color = 'var(--muted)';
    return;
  }
  countEl.textContent = String(total);
  labelEl.textContent = total > 0 ? '已執行' : '尚無紀錄';
  countEl.style.color = total > 0 ? 'var(--color-success)' : 'var(--muted)';
}

function updatePendingAlerts(data) {
  const countEl = document.getElementById('pendingAlertsCount');
  const labelEl = document.getElementById('pendingAlertsLabel');
  if (!countEl || !labelEl) return;
  const triggered = data && data.alerts_triggered != null ? data.alerts_triggered : 0;
  const acknowledged = data && data.alerts_acknowledged != null ? data.alerts_acknowledged : 0;
  const pending = Math.max(0, triggered - acknowledged);
  countEl.textContent = String(pending);
  labelEl.textContent = pending > 0 ? '待處理警報' : '已全數處理';
  countEl.style.color = pending > 0 ? 'var(--risk-high)' : 'var(--risk-low)';
}

function updateAckRate(data) {
  const countEl = document.getElementById('ackRateCount');
  const labelEl = document.getElementById('ackRateLabel');
  if (!countEl || !labelEl) return;
  const triggered = data && data.alerts_triggered != null ? data.alerts_triggered : 0;
  const acknowledged = data && data.alerts_acknowledged != null ? data.alerts_acknowledged : 0;
  if (triggered <= 0) {
    countEl.textContent = '—';
    labelEl.textContent = '尚無警報';
    countEl.style.color = 'var(--muted)';
    return;
  }
  const rate = (acknowledged / triggered) * 100;
  countEl.textContent = rate.toFixed(1) + '%';
  if (rate >= 80) {
    labelEl.textContent = '回應良好';
    countEl.style.color = 'var(--metric-good)';
  } else if (rate < 50) {
    labelEl.textContent = '回應不足';
    countEl.style.color = 'var(--metric-bad)';
  } else {
    labelEl.textContent = '待加強';
    countEl.style.color = 'var(--color-warning)';
  }
}

export async function updateMetricsTrend(data) {
  const trendDiv = document.getElementById('metricsTrend');
  if (!trendDiv) return;

  let trendData = null;
  try {
    trendData = await silentGetJSON('/api/dashboard/metrics/trend?metric=screening_rate&period=24h');
  } catch (e) {
    console.error('Failed to fetch trend data', e);
  }

  let html = '';

  html += '<div class="panel mb-md"><h3>指標趨勢 (24h)</h3>';
  if (!trendData || !trendData.trend || trendData.trend.length === 0) {
    html += '<div class="empty" style="font-size:12px;color:var(--muted);padding:20px 0;text-align:center;">目前尚無 24 小時內的趨勢資料，請稍後再查看。</div>';
  } else {
    const points = trendData.trend;
    const width = 800;
    const height = 120;
    const padding = { top: 10, right: 10, bottom: 20, left: 40 };
    const innerWidth = width - padding.left - padding.right;
    const innerHeight = height - padding.top - padding.bottom;

    const values = points.map(p => p.value * 100);
    const minVal = Math.min(...values, 0);
    const maxVal = Math.max(...values, 100);
    const range = maxVal - minVal || 1;

    const firstVal = values[0];
    const lastVal = values[values.length - 1];
    let strokeColor = 'var(--muted)';
    if (lastVal > firstVal) strokeColor = 'var(--trend-bullish)';
    else if (lastVal < firstVal) strokeColor = 'var(--trend-bearish)';

    let pathD = '';
    points.forEach((p, i) => {
      const x = padding.left + (i / (points.length - 1 || 1)) * innerWidth;
      const y = padding.top + innerHeight - ((values[i] - minVal) / range) * innerHeight;
      if (i === 0) pathD += `M ${x} ${y}`;
      else pathD += ` L ${x} ${y}`;
    });

    html += `<svg viewBox="0 0 ${width} ${height}" style="width:100%;height:120px;display:block;overflow:visible;">`;
    
    html += `<text x="${padding.left - 5}" y="${padding.top + 4}" fill="var(--muted)" font-size="10" text-anchor="end">${maxVal.toFixed(0)}%</text>`;
    html += `<text x="${padding.left - 5}" y="${padding.top + innerHeight + 4}" fill="var(--muted)" font-size="10" text-anchor="end">${minVal.toFixed(0)}%</text>`;
    
    const tickCount = Math.min(6, points.length);
    for (let i = 0; i < tickCount; i++) {
      const idx = Math.floor(i * (points.length - 1) / (tickCount - 1 || 1));
      const p = points[idx];
      const x = padding.left + (idx / (points.length - 1 || 1)) * innerWidth;
      const date = new Date(p.timestamp);
      const timeStr = `${date.getHours().toString().padStart(2, '0')}:${date.getMinutes().toString().padStart(2, '0')}`;
      html += `<text x="${x}" y="${height - 2}" fill="var(--muted)" font-size="10" text-anchor="middle">${timeStr}</text>`;
    }
    
    html += `<path d="${pathD}" fill="none" stroke="${strokeColor}" stroke-width="2" stroke-linejoin="round" stroke-linecap="round" />`;
    html += `</svg>`;
  }
  html += '</div>';

  html += '<div class="grid cols-2">';
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

export async function loadThresholdViolations() {
  const panel = document.getElementById('thresholdViolationsKpi');
  const countEl = document.getElementById('thresholdViolationsCount');
  const labelEl = document.getElementById('thresholdViolationsLabel');
  if (!panel || !countEl || !labelEl) return;

  try {
    const data = await silentGetJSON('/api/dashboard/metrics/thresholds');
    const violations = data && Array.isArray(data.violations) ? data.violations : [];
    const count = violations.length;

    let severity = 'success';
    if (violations.some(v => v.severity === 'critical')) severity = 'critical';
    else if (violations.some(v => v.severity === 'warning')) severity = 'warning';

    countEl.textContent = String(count);
    labelEl.textContent = severity === 'critical' ? '嚴重違規' :
                          severity === 'warning' ? '需要關注' : '全部正常';
    panel.dataset.severity = severity;
    countEl.style.color = severity === 'critical' ? 'var(--color-danger)' :
                          severity === 'warning' ? 'var(--color-warning)' :
                          'var(--color-success)';
  } catch (err) {
    console.error('[metrics] threshold violations load failed:', err);
    countEl.textContent = '—';
    labelEl.textContent = '載入失敗';
    panel.dataset.severity = 'unknown';
    countEl.style.color = 'var(--muted)';
  }
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

export async function loadDataQuality() {
  const scoreEl = document.getElementById('dataQualityScore');
  const detailEl = document.getElementById('dataQualityDetail');
  if (!scoreEl || !detailEl) return;

  const report = await silentGetJSON('/api/dashboard/data-quality');
  if (!report) {
    detailEl.className = 'empty';
    detailEl.textContent = '無法載入資料品質檢查';
    scoreEl.textContent = '-';
    return;
  }

  // Backend serializes time.Duration as nanoseconds despite the duration_ms JSON tag.
  // Convert ns → ms/µs/ns for human-readable display.
  const dataQualityDuration = (v) => {
    if (v == null) return '-';
    const ns = Number(v);
    if (!Number.isFinite(ns)) return '-';
    if (ns >= 1000000) return (ns / 1000000).toFixed(2) + ' ms';
    if (ns >= 1000) return (ns / 1000).toFixed(2) + ' µs';
    return ns + ' ns';
  };

  // CheckStatus enum → system-status color (per web/AGENTS.md).
  const statusColor = (status) => {
    if (status === 'ok') return 'var(--color-success)';
    if (status === 'warning') return 'var(--color-warning)';
    if (status === 'critical') return 'var(--color-danger)';
    return 'var(--muted)';
  };
  const statusLabel = (status) => {
    if (status === 'ok') return '正常';
    if (status === 'warning') return '警告';
    if (status === 'critical') return '嚴重';
    return '略過';
  };

  const overall = report.overall || 'unknown';
  const score = report.score != null ? report.score : '-';
  scoreEl.textContent = `整體：${statusLabel(overall)} · 評分：${score}`;
  scoreEl.style.color = statusColor(overall);

  const checks = Array.isArray(report.checks) ? report.checks : [];
  if (checks.length === 0) {
    detailEl.className = 'empty';
    detailEl.textContent = '目前沒有資料品質檢查記錄';
    return;
  }

  let html = '<table class="data-table"><thead><tr><th>檢查項目</th><th>狀態</th><th>訊息</th><th>耗時</th></tr></thead><tbody>';
  for (const c of checks) {
    const name = c && c.name ? c.name : '-';
    const status = c && c.status ? c.status : 'skipped';
    const message = c && c.message ? c.message : '';
    const duration = dataQualityDuration(c && c.duration);
    const color = statusColor(status);
    const label = statusLabel(status);
    html += `<tr><td>${name}</td><td style="color:${color};font-weight:600">${label}</td><td>${message}</td><td>${duration}</td></tr>`;
  }
  html += '</tbody></table>';
  detailEl.className = '';
  detailEl.innerHTML = html;
}

const metricsRefreshBtn = document.getElementById('metricsRefreshBtn');
if (metricsRefreshBtn && !metricsRefreshBtn.dataset.metricsWired) {
  metricsRefreshBtn.addEventListener('click', () => { loadMetrics(); });
  metricsRefreshBtn.dataset.metricsWired = '1';
}
