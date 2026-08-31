import { silentGetJSON, notify, postJSON } from '../shared/app-utils.js';
import { fmtSafeNumber, fmtSafeSignedPct } from '../shared/format-metric.js';
import { confirmAction } from '../components/confirm-modal.js';

export async function loadDataChannels() {
  const data = await silentGetJSON('/api/dashboard/data-channels');
  renderDataChannels(data);
  await Promise.all([loadMacroDataHealth(), loadFetchLogs(), loadDataPipeline()]);
}

export async function loadDataPipeline() {
  const data = await silentGetJSON('/api/dashboard/data-pipeline');
  renderDataPipeline(data);
}

export function renderDataPipeline(data) {
  const el = document.getElementById('dataPipelineContent');
  if (!el) return;
  const sources = (data && Array.isArray(data.sources)) ? data.sources : [];
  if (sources.length === 0) {
    el.classList.remove('loading');
    el.innerHTML = '<div class="empty">目前無資料管線狀態</div>';
    return;
  }
  // 對齊後端契約 (data_pipeline.go): ok/warn/error/unknown。
  // fresh/stale/paused 保留為相容 key (歷史前端值)；未知狀態一律顯示「未知」,
  // 不再誤標為「延遲」。
  const STATUS_BADGE = {
    ok:      { class: 'tier-badge tier-badge--bullish', label: '最新' },
    warn:    { class: 'tier-badge tier-badge--warn',    label: '延遲' },
    error:   { class: 'tier-badge tier-badge--bearish', label: '異常' },
    unknown: { class: 'tier-badge tier-badge--neutral', label: '未知' },
    fresh:   { class: 'tier-badge tier-badge--bullish', label: '最新' },
    stale:   { class: 'tier-badge tier-badge--warn',    label: '延遲' },
    paused:  { class: 'tier-badge tier-badge--neutral', label: '暫停' },
  };
  const rows = sources.map(s => {
    const meta = STATUS_BADGE[s.status] || STATUS_BADGE.unknown;
    return `<tr>
      <td><code>${escapeHtml(s.source_id || '-')}</code></td>
      <td>${escapeHtml(s.producer || '-')}</td>
      <td>${escapeHtml(s.consumer || '-')}</td>
      <td>${escapeHtml(s.last_produced || '-')}</td>
      <td>${escapeHtml(s.last_consumed || '-')}</td>
      <td>${escapeHtml(s.lag_human || '-')}</td>
      <td><span class="${meta.class}">${escapeHtml(meta.label)}</span></td>
    </tr>`;
  }).join('');
  el.classList.remove('loading');
  el.innerHTML = `
    <div class="table-scroll mt-sm">
      <table class="ranker-table">
        <thead>
          <tr>
            <th>Source</th>
            <th>Producer</th>
            <th>Consumer</th>
            <th>最後產出</th>
            <th>最後消費</th>
            <th>延遲</th>
            <th>狀態</th>
          </tr>
        </thead>
        <tbody>${rows}</tbody>
      </table>
    </div>
    <div class="text-muted text-sm mt-xs">最後更新：${escapeHtml(data.generated || '-')}</div>
  `;
}

export async function loadMacroDataHealth() {
  const data = await silentGetJSON('/api/dashboard/macro-data-health');
  renderMacroDataHealth(data);
}

export async function triggerChannelsIngest() {
  const btn = document.getElementById('btnIngestChannels');
  if (!btn) return;
  btn.disabled = true;
  btn.textContent = '更新中…';
  try {
    const data = await postJSON('/api/channels/ingest');
    const parts = [];
    if (data.macro_ok) parts.push('Macro ✓');
    else parts.push('Macro ✗' + (data.macro_error ? ': ' + data.macro_error : ''));
    if (data.geo_ok) parts.push('Geo ✓');
    else parts.push('Geo ✗' + (data.geo_error ? ': ' + data.geo_error : ''));
    notify('資料更新完成: ' + parts.join(' | '), data.macro_ok || data.geo_ok ? 'info' : 'err');
    loadDataChannels();
  } catch (e) {
    notify('資料更新失敗: ' + e.message, 'err');
  } finally {
    btn.disabled = false;
    btn.textContent = '立即更新 Macro 資料';
  }
}

export async function refreshChannelStatus() {
  console.log('[Management] Refresh channel status');
  await loadDataChannels();
  notify('狀態已刷新', 'info');
}

let fetchLogs = [];

function renderFetchLogsTable(logs) {
  let html = '<div class="dc-table-wrap"><table class="dc-table" style="table-layout:fixed"><thead><tr><th class="w-20">時間</th><th class="w-35">通道</th><th class="w-20">結果</th><th class="w-25">延遲</th></tr></thead><tbody>';
  logs.forEach(log => {
    const resultIcon = log.result === 'ok' ? '✓' : (log.result === 'warn' ? '⚠' : '✗');
    const resultColor = log.result === 'ok' ? 'var(--color-success)' : (log.result === 'warn' ? 'var(--warn)' : 'var(--color-danger)');
    html += `<tr>
      <td class="text-muted text-xs">${log.time}</td>
      <td>${log.channel}</td>
      <td style="color:${resultColor};font-weight:600">${resultIcon} ${log.result}</td>
      <td class="text-muted text-xs">${log.latency}</td>
    </tr>`;
  });
  html += '</tbody></table></div>';
  return html;
}

export async function loadFetchLogs() {
  const el = document.getElementById('dcFetchLog');
  if (!el) return;

  el.classList.add('loading');
  try {
    const data = await silentGetJSON('/api/dashboard/channel-fetch-log?limit=10');
    if (!data || !Array.isArray(data.entries) || data.entries.length === 0) {
      const emptyMsg = (data && data.empty_state) ? data.empty_state : '尚無 fetch 紀錄 (CLI 工具下次抓取時會自動寫入)';
      el.innerHTML = `<div class="empty"><div class="text-muted">${emptyMsg}</div></div>`;
      fetchLogs = [];
      return;
    }
    fetchLogs = data.entries;
    el.innerHTML = renderFetchLogsTable(fetchLogs);
  } catch (e) {
    el.innerHTML = `<div class="empty"><div class="text-muted">無法載入 fetch 紀錄: ${e.message || e}</div></div>`;
  } finally {
    el.classList.remove('loading');
  }
}

// splitDisabledChannels partitions channels into operator-active rows and
// intentionally-disabled rows (channels.json enabled=false). Disabled rows are
// hidden from the table + summary counts (#1758 決策: intentionally-off
// channels like tej/twse_etf must not render as「異常」or clutter the page) but
// stay discoverable via the one-line summary so operators can re-enable them.
export function splitDisabledChannels(channels) {
  const active = [];
  const disabled = [];
  (channels || []).forEach(c => {
    if (c.enabled === false) disabled.push(c);
    else active.push(c);
  });
  return { active, disabled };
}

export function renderDataChannels(data) {
  const el = document.getElementById('dataChannels');
  if (!data || !data.channels) { el.innerHTML = renderEmptyState('尚無資料通道資料', ''); el.classList.remove('loading'); return; }
  el.classList.remove('loading');
  const allChannels = data.channels || [];
  const { active: channels, disabled: disabledChannels } = splitDisabledChannels(allChannels);
  const disabledIds = new Set(disabledChannels.map(c => c.channel_id));
  const statusLight = s => {
    const color = s === 'ok' ? 'var(--status-ok)' : (s === 'warn' ? 'var(--status-warn)' : (s === 'error' ? 'var(--status-err)' : 'var(--status-unknown)'));
    return `<span style="display:inline-block;width:10px;height:10px;border-radius:50%;background:${color};box-shadow:0 0 6px ${color};margin-right:6px;vertical-align:middle"></span>`;
  };
  const statusClass = s => s === 'ok' ? 'ok' : (s === 'warn' ? 'warn' : (s === 'error' ? 'err' : 'muted'));

  // Calculate summary stats
  const total = channels.length;
  const errorCount = channels.filter(c => c.status === 'error').length;
  const warnCount = channels.filter(c => c.status === 'warn').length;

  // Group by country
  const byCountry = {};
  channels.forEach(c => {
    if (!byCountry[c.country]) byCountry[c.country] = [];
    byCountry[c.country].push(c);
  });

  let html = '';

  // Control panel summary
  html += `<div class="control-summary">
    <div class="metric" style="background:var(--panel);padding:10px 16px;border-radius:8px;border:1px solid var(--border)">
      <div class="label">總通道</div>
      <div class="value">${total}</div>
    </div>
    <div class="metric" style="background:var(--panel);padding:10px 16px;border-radius:8px;border:1px solid var(--border)">
      <div class="label">異常</div>
      <div class="value" style="color:var(--color-danger)">${errorCount}</div>
    </div>
    <div class="metric" style="background:var(--panel);padding:10px 16px;border-radius:8px;border:1px solid var(--border)">
      <div class="label">待更新</div>
      <div class="value" style="color:var(--warn)">${warnCount}</div>
    </div>
    <div class="metric" style="background:var(--panel);padding:10px 16px;border-radius:8px;border:1px solid var(--border)">
      <div class="label">正常</div>
      <div class="value" style="color:var(--color-success)">${total - errorCount - warnCount}</div>
    </div>
  </div>`;

  const sevIcon = { info: 'ℹ', warn: '⚠', error: '❌', critical: '🚫' };
  const sevColor = { info: 'var(--muted)', warn: 'var(--warn)', error: 'var(--color-danger)', critical: 'var(--color-danger)' };

  Object.keys(byCountry).forEach(country => {
    html += `<div style="margin:12px 0"><div style="font-size:14px;font-weight:700;color:var(--accent);margin-bottom:6px">${country}</div>`;
    html += '<div class="dc-table-wrap"><table class="dc-table" style="table-layout:fixed"><thead><tr><th class="w-8">燈號</th><th class="w-12">平台名稱</th><th class="w-10">API 格式</th><th class="w-20">資料路徑</th><th class="w-12">本地儲存</th><th class="w-15">狀態</th><th class="w-10">最後更新</th></tr></thead><tbody>';
    byCountry[country].forEach(c => {
      const sev = c.error_severity || 'warn';
      const errorHint = c.last_error ? `<div style="font-size:11px;color:${sevColor[sev] || sevColor.warn};margin-top:2px">${sevIcon[sev] || sevIcon.warn} ${escapeHtml(c.last_error)}</div>` : '';
      // disabled state is read from the new `enabled` field exposed by the
      // backend (data/state/channels.json) — the old code keyed off c.status
      // which conflates health with the operator's enable toggle.
      const isEnabled = c.enabled !== false;
      const statusBadge = !isEnabled
        ? `<span class="badge status-disabled">已停用</span>`
        : `<span class="badge ${statusClass(c.status)}">${c.status_text}</span>`;
      html += `<tr class="${!isEnabled ? 'dc-row-disabled' : ''}">
        <td class="text-center">${statusLight(c.status)}</td>
        <td>${c.platform}</td>
        <td>${c.api_format}</td>
        <td class="text-muted text-xs">${c.path}</td>
        <td class="text-muted text-xs">${c.storage}</td>
        <td>${statusBadge}${errorHint}</td>
        <td class="text-xs">${c.updated_at}</td>
      </tr>`;
    });
    html += '</tbody></table></div></div>';
  });

  // 需要關注的通道 — exclude intentionally-disabled channels (#1758): an
  // operator-disabled channel is a decision, not an incident.
  const visibleAlerts = (data.alerts || []).filter(a => !disabledIds.has(a.channel_id));
  if (visibleAlerts.length) {
    const statusLabel = s => s === 'error' ? '異常' : (s === 'warn' ? '待更新' : '異常');
    const statusColor = s => s === 'error' ? 'var(--color-danger)' : 'var(--warn)';
    html += `<div style="margin-top:14px;padding:10px 12px;background:color-mix(in srgb, var(--color-danger) 8%, transparent);border-left:3px solid var(--color-danger);border-radius:6px">
      <div style="font-size:13px;font-weight:700;color:var(--color-danger);margin-bottom:6px">需要關注的通道</div>
      ${visibleAlerts.map(a => `<div style="font-size:12px;margin:3px 0"><strong>${escapeHtml(a.channel_id)}</strong>：<span style="color:${statusColor(a.status)}">${a.error || statusLabel(a.status)}</span></div>`).join('')}
    </div>`;
  }

  if (disabledChannels.length) {
    html += `<div class="mt-sm text-muted text-sm">已停用通道（${disabledChannels.length}）：${disabledChannels.map(c => escapeHtml(c.channel_id)).join('、')} — 重新啟用請調整 data/state/channels.json 或對應環境變數後重啟</div>`;
  }

  html += `<div class="mt-sm text-muted text-sm">報告生成時間：${data.generated || '-'}</div>`;
  el.innerHTML = html;
}

function renderEmptyState(title, subtitle) {
  return `<div class="empty">${title}${subtitle ? `<div class="text-muted">${subtitle}</div>` : ''}</div>`;
}

export function renderMacroDataHealth(data) {
  const el = document.getElementById('macroDataHealth');
  if (!el) return;
  el.classList.remove('loading');
  if (!data || !data.indicators) { el.innerHTML = '<div class="empty">無資料</div>'; return; }

  const statusLight = s => {
    const color = s === 'ok' ? 'var(--status-ok)' : (s === 'warn' ? 'var(--status-warn)' : (s === 'error' ? 'var(--status-err)' : 'var(--status-unknown)'));
    return `<span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${color};box-shadow:0 0 4px ${color};margin-right:6px;vertical-align:middle"></span>`;
  };

  const indicators = data.indicators;
  const okCount = indicators.filter(i => i.status === 'ok').length;
  const warnCount = indicators.filter(i => i.status === 'warn').length;
  const errCount = indicators.filter(i => i.status === 'error').length;

  const headerColor = errCount > 0 ? 'var(--color-danger)' : (warnCount > 0 ? 'var(--warn)' : 'var(--color-success)');
  const headerText = errCount > 0 ? '有資料異常需處理' : (warnCount > 0 ? '部分指標待更新' : '全部指標正常');

  let html = `<div style="margin:16px 0;padding:12px 16px;background:var(--panel);border-radius:8px;border:1px solid var(--border)">
    <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:10px">
      <div>
        <span style="font-size:14px;font-weight:700;color:${headerColor}">📊 總經指標資料品質</span>
        <span style="font-size:12px;color:var(--muted);margin-left:8px">${headerText}</span>
      </div>
      <div style="display:flex;gap:12px;font-size:12px">
        <span style="color:var(--color-success)">🟢 ${okCount} 正常</span>
        <span style="color:var(--warn)">🟡 ${warnCount} 待更新</span>
        <span style="color:var(--color-danger)">🔴 ${errCount} 異常</span>
      </div>
    </div>
    <div class="dc-table-wrap"><table class="dc-table" style="table-layout:fixed"><thead><tr>
      <th class="w-35">指標</th><th class="w-20">數值</th><th class="w-20">日變動%</th><th class="w-25">狀態</th>
    </tr></thead><tbody>`;

  indicators.forEach(ind => {
    const valStr = fmtSafeNumber(ind.value, { decimals: 3 });
    const chgStr = fmtSafeSignedPct(ind.change_pct, 2);
    const statusColor = ind.status === 'ok' ? 'var(--color-success)' : (ind.status === 'warn' ? 'var(--warn)' : 'var(--color-danger)');
    const symbolHint = ind.symbol ? '' : ' <span class="text-muted text-xs">(無來源)</span>';
    html += `<tr>
      <td>${ind.name}${symbolHint}</td>
      <td>${valStr}</td>
      <td>${chgStr}</td>
      <td><span style="color:${statusColor};font-weight:600;font-size:12px">${statusLight(ind.status)}${ind.status_text}</span></td>
    </tr>`;
  });

  html += '</tbody></table></div></div>';
  el.innerHTML = html;
}

function escapeHtml(text) {
  const div = document.createElement('div');
  div.textContent = text;
  return div.innerHTML;
}
