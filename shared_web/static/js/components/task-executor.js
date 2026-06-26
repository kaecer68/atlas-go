// Task executor UI components

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
