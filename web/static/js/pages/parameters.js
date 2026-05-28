import { escapeHtml } from '../shared/app-utils.js';

function isEditable(val) {
  return typeof val === 'number' || (typeof val === 'string' && !isNaN(val) && val.trim() !== '');
}

function formatValue(key, val, maxLen) {
  if (val === undefined || val === null) return '-';
  let s = val;
  if (typeof s === 'object') s = JSON.stringify(s);
  else s = String(s);
  if (!isEditable(val) && s.length > maxLen) {
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
  const editable = isEditable(val) ? ' class="param-val-editable" title="點擊編輯" data-key="' + escapeHtml(key) + '" data-val="' + escapeHtml(String(val)) + '"' : '';

  return `<tr>
    <td class="param-key">${escapeHtml(key)}</td>
    <td${editable} onclick="window._paramEdit(this)">${formatValue(key, val, maxLen)}</td>
    <td class="param-meta">${escapeHtml(source)}</td>
    <td class="param-meta">${hasEvo}</td>
    <td class="param-meta">${escapeHtml(lastCal)}</td>
  </tr>`;
}

window._paramEdit = function(td) {
  if (td.querySelector('input')) return;
  const key = td.dataset.key;
  const oldVal = td.dataset.val;
  const input = document.createElement('input');
  input.type = 'text';
  input.value = oldVal;
  input.style.cssText = 'width:100%;background:var(--panel-l2);color:var(--text);border:1px solid var(--accent);padding:2px 4px;font-size:12px;border-radius:3px;';
  input.onkeydown = async function(e) {
    if (e.key === 'Enter') { input.blur(); }
    if (e.key === 'Escape') { td.innerHTML = escapeHtml(oldVal); td.classList.add('param-val-editable'); }
  };
  input.onblur = async function() {
    const newVal = parseFloat(input.value);
    if (isNaN(newVal) || newVal === parseFloat(oldVal)) {
      td.innerHTML = escapeHtml(oldVal);
      td.classList.add('param-val-editable');
      return;
    }
    td.innerHTML = escapeHtml(String(newVal)) + ' <span style="color:var(--warn);font-size:10px">儲存中…</span>';
    try {
      const body = {}; body[key] = newVal;
      const resp = await fetch('/api/parameters', { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify(body) });
      if (resp.ok) {
        td.innerHTML = escapeHtml(String(newVal));
        td.classList.add('param-val-editable');
        td.dataset.val = String(newVal);
      } else {
        td.innerHTML = escapeHtml(oldVal) + ' <span style="color:var(--down);font-size:10px">失敗</span>';
        td.classList.add('param-val-editable');
      }
    } catch {
      td.innerHTML = escapeHtml(oldVal) + ' <span style="color:var(--down);font-size:10px">錯誤</span>';
      td.classList.add('param-val-editable');
    }
  };
  td.innerHTML = '';
  td.classList.remove('param-val-editable');
  td.appendChild(input);
  input.focus();
  input.select();
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
        <th style="width:33%">參數</th><th style="width:33%">值</th><th style="width:12%">來源</th><th style="width:8%">進化</th><th style="width:10%">校準</th><th style="width:4%"></th>
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
      <th style="width:33%">參數</th><th style="width:33%">值</th><th style="width:12%">來源</th><th style="width:8%">進化</th><th style="width:10%">校準</th><th style="width:4%"></th>
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
