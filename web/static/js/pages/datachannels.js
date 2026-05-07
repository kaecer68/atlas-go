// Data channels page
import { getJSON, notify } from '../shared/app-utils.js';

export async function loadDataChannels() {
  const data = await getJSON('/api/dashboard/data-channels').catch(() => null);
  renderDataChannels(data);
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

  // Group by country
  const byCountry = {};
  channels.forEach(c => {
    if (!byCountry[c.country]) byCountry[c.country] = [];
    byCountry[c.country].push(c);
  });

  let html = '';
  Object.keys(byCountry).forEach(country => {
    html += `<div style="margin:12px 0"><div style="font-size:14px;font-weight:700;color:var(--accent);margin-bottom:6px">${country}</div>`;
    html += '<table class="text-sm"><thead><tr><th class="w-28">燈號</th><th>平台名稱</th><th>API 格式</th><th>資料路徑</th><th>本地儲存</th><th>狀態</th><th>最後更新</th></tr></thead><tbody>';
    byCountry[country].forEach(c => {
      const errorHint = c.last_error ? `<div style="font-size:11px;color:var(--down);margin-top:2px">⚠ ${escapeHtml(c.last_error)}</div>` : '';
      html += `<tr>
        <td class="text-center">${statusLight(c.status)}</td>
        <td>${c.platform}</td>
        <td>${c.api_format}</td>
        <td class="text-muted text-xs">${c.path}</td>
        <td class="text-muted text-xs">${c.storage}</td>
        <td><span class="badge ${statusClass(c.status)}">${c.status_text}</span>${errorHint}</td>
        <td class="text-xs">${c.updated_at}</td>
      </tr>`;
    });
    html += '</tbody></table></div>';
  });

  if (data.alerts && data.alerts.length) {
    html += `<div style="margin-top:14px;padding:10px 12px;background:rgba(239,68,68,0.08);border-left:3px solid var(--down);border-radius:6px">
      <div style="font-size:13px;font-weight:700;color:var(--down);margin-bottom:6px">需要關注的通道</div>
      ${data.alerts.map(a => `<div style="font-size:12px;margin:3px 0"><strong>${escapeHtml(a.channel_id)}</strong>：${escapeHtml(a.error || '狀態異常')} <span class="text-muted">（${a.fetch_at ? new Date(a.fetch_at).toLocaleString('zh-TW') : '-'}）</span></div>`).join('')}
    </div>`;
  }

  html += `<div class="mt-sm text-muted text-sm">報告生成時間：${data.generated || '-'}</div>`;
  el.innerHTML = html;
}
