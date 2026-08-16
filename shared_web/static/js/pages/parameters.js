import { escapeHtml } from '../shared/app-utils.js';
import { fmtSafeNumber } from '../shared/format-metric.js';

function formatValue(key, val, maxLen) {
  if (val === undefined || val === null) return '-';
  if (typeof val === 'object' && !Array.isArray(val)) {
    const entries = Object.entries(val);
    const count = entries.length;
    const preview = entries.slice(0, 2).map(([k, v]) => `${k}: ${typeof v === 'number' ? fmtSafeNumber(v, { decimals: 2 }) : v}`).join(', ');
    return `<span class="param-map-collapsed" onclick="window._paramMapExpand(this)" data-key="${escapeHtml(key)}" data-val="${escapeHtml(JSON.stringify(val))}" title="點擊展開">${count} entries{${preview}…}</span>`;
  }
  let s = val;
  if (typeof s === 'object') s = JSON.stringify(s);
  else s = String(s);
  if (s.length > maxLen) {
    return `<span class="param-val-trunc" title="${escapeHtml(s)}" onclick="this.classList.toggle('expanded')">${escapeHtml(s.slice(0, maxLen))}…</span>`;
  }
  return escapeHtml(s);
}

function renderRow(key, val, meta, maxLen, isUncategorized) {
  const source = typeof meta.source === 'string' ? meta.source : '-';
  const hasEvo = (meta.todo && meta.todo.length > 0) ? '✓' : '—';
  const lastCal = meta.last_calibrated
    ? new Date(meta.last_calibrated).toLocaleDateString('zh-TW', {year:'numeric',month:'2-digit',day:'2-digit'})
    : 'never';
  return `<tr>
    <td class="param-key">${escapeHtml(key)}</td>
    <td>${formatValue(key, val, maxLen)}</td>
    <td class="param-meta">${escapeHtml(source)}</td>
    <td class="param-meta">${hasEvo}</td>
    <td class="param-meta">${escapeHtml(lastCal)}</td>
  </tr>`;
}

window._paramMapExpand = function(span) {
  const key = span.dataset.key;
  let map;
  try { map = JSON.parse(span.dataset.val); } catch(e) { return; }
  const entries = Object.entries(map);
  let html = '<table class="params-table map-sub-table"><thead><tr><th>Key</th><th>Value</th></tr></thead><tbody>';
  for (const [k, v] of entries) {
    html += `<tr>
      <td class="param-key">${escapeHtml(k)}</td>
      <td>${escapeHtml(typeof v === 'number' ? fmtSafeNumber(v, { decimals: 4 }) : String(v))}</td>
    </tr>`;
  }
  html += '</tbody></table>';
  span.parentElement.innerHTML = html;
};

export function renderParametersPage(params, categoriesResp, auditLog, metadata) {
  const contentDiv = document.getElementById('parametersContent');
  if (!contentDiv) return;

  if (!params || Object.keys(params).length === 0) {
    contentDiv.innerHTML = '<div class="empty" style="text-align:center;padding:40px">無法載入參數配置或目前無任何參數。</div>';
    contentDiv.classList.remove('empty', 'loading');
    renderAuditLog(auditLog);
    return;
  }

  const uncategorized = new Set(Object.keys(params));
  const cats = (categoriesResp && categoriesResp.categories) ? categoriesResp.categories : [];
  const catKeys = (categoriesResp && categoriesResp.keys) ? categoriesResp.keys : {};
  const MAX_VAL_LEN = 80;

  let html = '<div class="parameters-grid">';

  for (const cat of cats) {
    html += `<div class="panel"><h3>${escapeHtml(cat.name)}</h3>
      <table class="params-table"><thead><tr>
        <th style="width:40%">參數</th><th style="width:22%">值</th><th style="width:12%">來源</th><th style="width:10%">進化</th><th style="width:10%">校準</th><th style="width:6%"></th>
      </tr></thead><tbody>`;
    let hasKeys = false;
    const keys = catKeys[cat.id] || [];

    for (const key of keys) {
      if (key in params) {
        const meta = (metadata && metadata[key]) ? metadata[key] : {};
        html += renderRow(key, params[key], meta, MAX_VAL_LEN);
        uncategorized.delete(key);
        hasKeys = true;
      }
    }
    if (!hasKeys) html += `<tr><td colspan="6" class="empty">無參數</td></tr>`;
    html += `</tbody></table></div>`;
  }

  if (uncategorized.size > 0) {
    html += `<div class="panel"><h3>其他參數</h3><table class="params-table"><thead><tr>
      <th style="width:40%">參數</th><th style="width:22%">值</th><th style="width:12%">來源</th><th style="width:10%">進化</th><th style="width:10%">校準</th><th style="width:6%"></th>
    </tr></thead><tbody>`;
    for (const key of uncategorized) {
      const meta = (metadata && metadata[key]) ? metadata[key] : {};
      html += renderRow(key, params[key], meta, MAX_VAL_LEN);
    }
    html += `</tbody></table></div>`;
  }

  html += '</div>';
  contentDiv.innerHTML = html;
  contentDiv.classList.remove('empty', 'loading');

  renderAuditLog(auditLog);
}

function renderAuditLog(auditLog) {
  const logDiv = document.getElementById('parametersAuditLog');
  if (!logDiv) return;

  const changes = (auditLog && auditLog.changes) ? auditLog.changes : null;
  if (!changes || !Array.isArray(changes) || changes.length === 0) {
    logDiv.innerHTML = '<div class="empty" style="text-align:center;padding:20px">尚無參數變更紀錄。</div>';
  } else {
    let html = '<div class="table-wrapper"><table><thead><tr><th>時間</th><th>參數</th><th>原值</th><th>新值</th><th>原因</th><th>操作者</th></tr></thead><tbody>';
    const logsToShow = changes.slice(0, 20);

    for (const log of logsToShow) {
      const timestamp = log.timestamp || log.Timestamp || log.changed_at || log.ChangedAt || log.time || log.Time || '-';
      const key = log.key || log.Key || log.parameter || log.Parameter || log.name || log.Name || '-';
      const oldVal = log.old_value !== undefined ? log.old_value : (log.OldValue !== undefined ? log.OldValue : (log.old !== undefined ? log.old : '-'));
      const newVal = log.new_value !== undefined ? log.new_value : (log.NewValue !== undefined ? log.NewValue : (log.new !== undefined ? log.new : '-'));
      const reason = log.reason || log.Reason || log.comment || log.Comment || '-';
      const user = log.user || log.User || log.operator || log.Operator || log.author || log.Author || 'system';

      const formatTime = t => {
        if (t === '-') return t;
        try { const d = new Date(t); if (isNaN(d.getTime())) return t; return d.toLocaleString('zh-TW'); } catch(e) { return t; }
      };
      const formatVal = v => {
        if (v === '-') return v;
        if (typeof v === 'object') return JSON.stringify(v);
        return String(v);
      };

      html += `<tr>
        <td style="white-space:nowrap">${escapeHtml(formatTime(timestamp))}</td>
        <td style="word-break:break-all">${escapeHtml(key)}</td>
        <td style="font-family:var(--font-mono);font-size:11px;color:var(--muted);max-width:200px;overflow:hidden;text-overflow:ellipsis">${escapeHtml(formatVal(oldVal))}</td>
        <td style="font-family:var(--font-mono);font-size:11px;color:var(--color-success);max-width:200px;overflow:hidden;text-overflow:ellipsis">${escapeHtml(formatVal(newVal))}</td>
        <td>${escapeHtml(reason)}</td>
        <td style="white-space:nowrap">${escapeHtml(user)}</td>
      </tr>`;
    }
    html += '</tbody></table></div>';
    logDiv.innerHTML = html;
  }
  logDiv.classList.remove('empty', 'loading');
}
