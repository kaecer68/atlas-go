import { escapeHtml } from '../shared/app-utils.js';

const SENSITIVE_KEYS = ['password', 'secret', 'api_key', 'token', 'signor_key', 'fubon_personal', 'personal_id'];

function isSensitive(key) {
  const k = key.toLowerCase();
  return SENSITIVE_KEYS.some(s => k.includes(s));
}

function maskValue(val) {
  if (val === '' || val === null || val === undefined) return '-';
  const s = String(val);
  if (s.length <= 8) return '••••';
  return s.slice(0, 3) + '••••' + s.slice(-3);
}

function renderSection(title, entries) {
  if (entries.length === 0) return '';
  let html = `<div class="panel"><h3>${escapeHtml(title)}</h3>
    <table class="params-table"><thead><tr>
      <th style="width:30%">設定鍵</th><th style="width:55%">值</th><th style="width:15%">類型</th>
    </tr></thead><tbody>`;

  for (const [key, val] of entries) {
    const isSen = isSensitive(key);
    const display = isSen ? maskValue(val) : (val === '' || val === null || val === undefined ? '-' : escapeHtml(String(val)));
    const type = typeof val;
    html += `<tr>
      <td class="param-key">${escapeHtml(key)}${isSen ? ' 🔒' : ''}</td>
      <td class="param-val" style="font-family:var(--font-mono);font-size:11px">${display}</td>
      <td class="param-meta">${type === 'number' ? 'number' : type === 'boolean' ? 'bool' : 'string'}</td>
    </tr>`;
  }
  html += '</tbody></table></div>';
  return html;
}

export function renderConfigPage(cfg, error) {
  const contentDiv = document.getElementById('configContent');
  if (!contentDiv) return;

  if (!cfg || Object.keys(cfg).length === 0) {
    // 2026-08-24 UI audit P2：錯誤狀態補上原因 + 重試按鈕（原先只有一行灰字）。
    const reason = error ? escapeHtml(String(error.message || error).slice(0, 120)) : '';
    contentDiv.innerHTML =
      '<div style="text-align:center;padding:48px 24px;max-width:480px;margin:0 auto">' +
        '<div style="font-size:2rem;margin-bottom:10px">⚠️</div>' +
        '<div style="font-weight:700;margin-bottom:6px">無法載入部署配置</div>' +
        (reason ? '<div style="font-size:12px;color:var(--muted);margin-bottom:4px">' + reason + '</div>' : '') +
        '<div style="font-size:12px;color:var(--muted);margin-bottom:16px">讀取失敗可能為後端暫時無回應，請稍後重試。</div>' +
        '<button class="alert-action-btn" onclick="window.loadConfigPage()">🔄 重試</button>' +
      '</div>';
    contentDiv.classList.remove('empty', 'loading');
    return;
  }

  const entries = Object.entries(cfg);

  // Separate by category
  const paths = [];
  const keys = [];
  const flags = [];
  const ports = [];
  const other = [];

  for (const [k, v] of entries) {
    const kl = k.toLowerCase();
    if (kl.includes('_path') || kl.includes('_dir') || kl.includes('_file')) paths.push([k, v]);
    else if (kl.includes('_port') || kl === 'port') ports.push([k, v]);
    else if (kl.includes('_key') || kl.includes('_token') || kl.includes('_secret') || kl.includes('_password') || kl.includes('_id')) keys.push([k, v]);
    else if (typeof v === 'boolean') flags.push([k, v]);
    else other.push([k, v]);
  }

  let html = '';
  html += renderSection('🔑 API 金鑰與憑證', keys);
  html += renderSection('📁 路徑設定', paths);
  html += renderSection('🔌 連接埠', ports);
  html += renderSection('🚩 功能開關', flags);
  html += renderSection('📋 其他設定', other);

  contentDiv.innerHTML = html;
  contentDiv.classList.remove('empty', 'loading');
}
