import { silentGetJSON, notify } from '../shared/app-utils.js';

export async function loadDataChannels() {
  const data = await silentGetJSON('/api/dashboard/data-channels');
  renderDataChannels(data);
  loadMacroDataHealth();
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
    const res = await fetch('/api/channels/ingest', { method: 'POST' });
    const data = await res.json().catch(() => ({ error: 'Unknown error' }));
    if (!res.ok) {
      notify('資料更新失敗: ' + (data.error || res.statusText), 'err');
      return;
    }
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
    btn.textContent = '立即更新 Macro + Geo 資料';
  }
}

export async function enableAllChannels() {
  console.log('[Management] Enable all channels');
  try {
    const data = await silentGetJSON('/api/dashboard/data-channels');
    const channels = data.channels || [];
    for (const c of channels) {
      if (c.status === 'inactive') {
        await fetch(`/api/dashboard/channels/${c.channel_id}/toggle`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ enabled: true })
        });
      }
    }
    notify('已啟用所有通道', 'info');
    refreshChannelStatus();
  } catch (e) {
    notify('啟用通道失敗: ' + e.message, 'err');
  }
}

export async function disableAllChannels() {
  console.log('[Management] Disable all channels');
  try {
    const data = await silentGetJSON('/api/dashboard/data-channels');
    const channels = data.channels || [];
    for (const c of channels) {
      if (c.status !== 'inactive') {
        await fetch(`/api/dashboard/channels/${c.channel_id}/toggle`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ enabled: false })
        });
      }
    }
    notify('已停用所有通道', 'warn');
    refreshChannelStatus();
  } catch (e) {
    notify('停用通道失敗: ' + e.message, 'err');
  }
}

export function refreshChannelStatus() {
  console.log('[Management] Refresh channel status');
  loadDataChannels();
  notify('狀態已刷新', 'info');
}

export async function triggerChannelFetch(channelID) {
  console.log('[Management] Trigger fetch for', channelID);
  try {
    const res = await fetch(`/api/dashboard/channels/${channelID}/trigger`, { method: 'POST' });
    if (res.ok) {
      notify(`${channelID} 抓取已觸發`, 'info');
    } else {
      notify(`${channelID} 觸發失敗: ${res.statusText}`, 'err');
    }
  } catch (e) {
    notify(`${channelID} 觸發失敗: ${e.message}`, 'err');
  }
}

export async function toggleChannel(channelID, enable) {
  console.log('[Management] Toggle', channelID, 'to', enable);
  try {
    const res = await fetch(`/api/dashboard/channels/${channelID}/toggle`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled: enable })
    });
    if (res.ok) {
      notify(`${channelID} 已${enable ? '啟用' : '停用'}`, 'info');
    } else {
      notify(`${channelID} 切換失敗: ${res.statusText}`, 'err');
    }
  } catch (e) {
    notify(`${channelID} 切換失敗: ${e.message}`, 'err');
  }
}

const idMap = {
  'yahoo': 'dcApiKeyYahoo',
  'finmind': 'dcApiKeyFinmind',
  'fubon': 'dcApiKeyFubon',
  'fugle': 'dcApiKeyFugle',
  'tej': 'dcApiKeyTej'
};

export async function updateApiKey(provider) {
  const input = document.getElementById(idMap[provider] || `apikey-${provider}`);
  if (!input) return;
  const key = input.value.trim();
  if (!key) {
    notify('請輸入 API Key', 'warn');
    return;
  }
  console.log('[Management] Update API key for', provider);
  try {
    const res = await fetch('/api/dashboard/api-keys/update', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ provider, api_key: key })
    });
    if (res.ok) {
      notify(`${provider} API Key 已更新`, 'info');
      input.value = '';
    } else {
      notify('更新失敗: ' + res.statusText, 'err');
    }
  } catch (e) {
    notify('更新失敗: ' + e.message, 'err');
  }
}

let fetchLogs = [
  { time: '14:32:01', channel: 'us_yahoo', result: 'ok', latency: '234ms' },
  { time: '14:30:00', channel: 'twse_margin', result: 'ok', latency: '456ms' },
  { time: '14:28:00', channel: 'finmind', result: 'error', latency: '1.2s' },
  { time: '14:25:00', channel: 'geopolitical', result: 'ok', latency: '189ms' },
  { time: '14:22:00', channel: 'export_statistics', result: 'ok', latency: '567ms' },
  { time: '14:20:00', channel: 'twse_capital_flow', result: 'ok', latency: '345ms' },
  { time: '14:18:00', channel: 'tsmc_revenue', result: 'warn', latency: '2.1s' },
  { time: '14:15:00', channel: 'jpy_yahoo', result: 'ok', latency: '198ms' },
  { time: '14:12:00', channel: 'tej', result: 'ok', latency: '423ms' },
  { time: '14:10:00', channel: 'janus_regime', result: 'ok', latency: '12ms' }
];

export function loadFetchLogs() {
  const el = document.getElementById('fetchLogs');
  if (!el) return;

  let html = '<div class="dc-table-wrap"><table class="dc-table" style="table-layout:fixed"><thead><tr><th class="w-20">時間</th><th class="w-35">通道</th><th class="w-20">結果</th><th class="w-25">延遲</th></tr></thead><tbody>';
  fetchLogs.forEach(log => {
    const resultIcon = log.result === 'ok' ? '✓' : (log.result === 'warn' ? '⚠' : '✗');
    const resultColor = log.result === 'ok' ? 'var(--up)' : (log.result === 'warn' ? 'var(--warn)' : 'var(--down)');
    html += `<tr>
      <td class="text-muted text-xs">${log.time}</td>
      <td>${log.channel}</td>
      <td style="color:${resultColor};font-weight:600">${resultIcon} ${log.result}</td>
      <td class="text-muted text-xs">${log.latency}</td>
    </tr>`;
  });
  html += '</tbody></table></div>';
  el.innerHTML = html;
}

export function renderDataChannels(data) {
  const el = document.getElementById('dataChannels');
  if (!data || !data.channels) { el.innerHTML = renderEmptyState('尚無資料通道資料', ''); el.classList.remove('loading'); return; }
  el.classList.remove('loading');
  const channels = data.channels || [];
  const statusLight = s => {
    const color = s === 'ok' ? 'var(--status-ok)' : (s === 'warn' ? 'var(--status-warn)' : (s === 'error' ? 'var(--status-err)' : 'var(--status-unknown)'));
    return `<span style="display:inline-block;width:10px;height:10px;border-radius:50%;background:${color};box-shadow:0 0 6px ${color};margin-right:6px;vertical-align:middle"></span>`;
  };
  const statusClass = s => s === 'ok' ? 'up' : (s === 'warn' ? 'warn' : (s === 'error' ? 'down' : 'muted'));

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
  html += `<div class="control-summary" style="display:flex;gap:16px;margin-bottom:16px;flex-wrap:wrap">
    <div class="metric" style="background:var(--panel);padding:10px 16px;border-radius:8px;border:1px solid var(--border)">
      <div class="label">總通道</div>
      <div class="value">${total}</div>
    </div>
    <div class="metric" style="background:var(--panel);padding:10px 16px;border-radius:8px;border:1px solid var(--border)">
      <div class="label">異常</div>
      <div class="value" style="color:var(--down)">${errorCount}</div>
    </div>
    <div class="metric" style="background:var(--panel);padding:10px 16px;border-radius:8px;border:1px solid var(--border)">
      <div class="label">待更新</div>
      <div class="value" style="color:var(--warn)">${warnCount}</div>
    </div>
    <div class="metric" style="background:var(--panel);padding:10px 16px;border-radius:8px;border:1px solid var(--border)">
      <div class="label">正常</div>
      <div class="value" style="color:var(--up)">${total - errorCount - warnCount}</div>
    </div>
  </div>`;

  Object.keys(byCountry).forEach(country => {
    html += `<div style="margin:12px 0"><div style="font-size:14px;font-weight:700;color:var(--accent);margin-bottom:6px">${country}</div>`;
    html += '<div class="dc-table-wrap"><table class="dc-table" style="table-layout:fixed"><thead><tr><th class="w-8">燈號</th><th class="w-12">平台名稱</th><th class="w-10">API 格式</th><th class="w-20">資料路徑</th><th class="w-12">本地儲存</th><th class="w-15">狀態</th><th class="w-13">操作</th><th class="w-10">最後更新</th></tr></thead><tbody>';
    byCountry[country].forEach(c => {
      const errorHint = c.last_error ? `<div style="font-size:11px;color:var(--down);margin-top:2px">⚠ ${escapeHtml(c.last_error)}</div>` : '';
      const toggleBtn = `<button class="text-xs" onclick="toggleChannel('${c.channel_id}', this.dataset.enabled !== 'true')" data-enabled="${c.status !== 'inactive'}" style="padding:2px 8px;border-radius:4px;background:var(--border);border:1px solid var(--border);cursor:pointer">${c.status === 'inactive' ? '啟用' : '停用'}</button>`;
      const triggerBtn = `<button class="text-xs" onclick="triggerChannelFetch('${c.channel_id}')" style="padding:2px 8px;border-radius:4px;background:var(--primary);border:1px solid var(--primary);color:#fff;cursor:pointer;margin-left:4px">觸發</button>`;
      html += `<tr>
        <td class="text-center">${statusLight(c.status)}</td>
        <td>${c.platform}</td>
        <td>${c.api_format}</td>
        <td class="text-muted text-xs">${c.path}</td>
        <td class="text-muted text-xs">${c.storage}</td>
        <td><span class="badge ${statusClass(c.status)}">${c.status_text}</span>${errorHint}</td>
        <td>${toggleBtn}${triggerBtn}</td>
        <td class="text-xs">${c.updated_at}</td>
      </tr>`;
    });
    html += '</tbody></table></div></div>';
  });

  if (data.alerts && data.alerts.length) {
    const statusLabel = s => s === 'error' ? '異常' : (s === 'warn' ? '待更新' : '異常');
    const statusColor = s => s === 'error' ? 'var(--down)' : 'var(--warn)';
    html += `<div style="margin-top:14px;padding:10px 12px;background:rgba(239,68,68,0.08);border-left:3px solid var(--down);border-radius:6px">
      <div style="font-size:13px;font-weight:700;color:var(--down);margin-bottom:6px">需要關注的通道</div>
      ${data.alerts.map(a => `<div style="font-size:12px;margin:3px 0"><strong>${escapeHtml(a.channel_id)}</strong>：<span style="color:${statusColor(a.status)}">${a.error || statusLabel(a.status)}</span></div>`).join('')}
    </div>`;
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
  if (!data || !data.indicators) { el.innerHTML = ''; return; }
  el.classList.remove('loading');

  const statusLight = s => {
    const color = s === 'ok' ? 'var(--status-ok)' : (s === 'warn' ? 'var(--status-warn)' : (s === 'error' ? 'var(--status-err)' : 'var(--status-unknown)'));
    return `<span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${color};box-shadow:0 0 4px ${color};margin-right:6px;vertical-align:middle"></span>`;
  };

  const indicators = data.indicators;
  const okCount = indicators.filter(i => i.status === 'ok').length;
  const warnCount = indicators.filter(i => i.status === 'warn').length;
  const errCount = indicators.filter(i => i.status === 'error').length;

  const headerColor = errCount > 0 ? 'var(--down)' : (warnCount > 0 ? 'var(--warn)' : 'var(--up)');
  const headerText = errCount > 0 ? '有資料異常需處理' : (warnCount > 0 ? '部分指標待更新' : '全部指標正常');

  let html = `<div style="margin:16px 0;padding:12px 16px;background:var(--panel);border-radius:8px;border:1px solid var(--border)">
    <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:10px">
      <div>
        <span style="font-size:14px;font-weight:700;color:${headerColor}">📊 總經指標資料品質</span>
        <span style="font-size:12px;color:var(--muted);margin-left:8px">${headerText}</span>
      </div>
      <div style="display:flex;gap:12px;font-size:12px">
        <span style="color:var(--up)">🟢 ${okCount} 正常</span>
        <span style="color:var(--warn)">🟡 ${warnCount} 待更新</span>
        <span style="color:var(--down)">🔴 ${errCount} 異常</span>
      </div>
    </div>
    <div class="dc-table-wrap"><table class="dc-table" style="table-layout:fixed"><thead><tr>
      <th class="w-35">指標</th><th class="w-20">數值</th><th class="w-20">日變動%</th><th class="w-25">狀態</th>
    </tr></thead><tbody>";

  indicators.forEach(ind => {
    const valStr = typeof ind.value === 'number' ? ind.value.toFixed(3) : '-';
    const chgStr = typeof ind.change_pct === 'number' ? ind.change_pct.toFixed(4) + '%' : '-';
    const statusColor = ind.status === 'ok' ? 'var(--up)' : (ind.status === 'warn' ? 'var(--warn)' : 'var(--down)');
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
