import { escapeHtml } from '../shared/app-utils.js';

function isEditable(val) {
  if (typeof val === 'number') return true;
  if (typeof val === 'boolean') return true;
  if (typeof val === 'string' && !isNaN(val) && val.trim() !== '') return true;
  if (typeof val === 'string' && val.trim() !== '') return true;
  if (typeof val === 'object' && val !== null && !Array.isArray(val)) return true;
  return false;
}

function detectParamType(val) {
  if (typeof val === 'boolean') return 'bool';
  if (typeof val === 'number') return 'number';
  if (typeof val === 'string') {
    if (val === 'true' || val === 'false') return 'bool';
    if (!isNaN(val) && val.trim() !== '') return 'number';
    return 'string';
  }
  if (typeof val === 'object') return 'object';
  return 'unknown';
}

function formatValue(key, val, maxLen) {
  if (val === undefined || val === null) return '-';
  if (typeof val === 'object' && !Array.isArray(val)) {
    const entries = Object.entries(val);
    const count = entries.length;
    const preview = entries.slice(0, 2).map(([k, v]) => `${k}: ${typeof v === 'number' ? v.toFixed(2) : v}`).join(', ');
    return `<span class="param-map-collapsed" onclick="window._paramMapExpand(this)" data-key="${escapeHtml(key)}" data-val="${escapeHtml(JSON.stringify(val))}" title="點擊展開">${count} entries{${preview}…}</span>`;
  }
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
  if (td.querySelector('input, select')) return;
  const key = td.dataset.key;
  const oldVal = td.dataset.val;

  let typedOldVal;
  try { typedOldVal = JSON.parse(oldVal); } catch(e) { typedOldVal = oldVal; }
  const pType = detectParamType(typedOldVal);

  if (pType === 'bool') {
    const sel = document.createElement('select');
    sel.style.cssText = 'width:100%;background:var(--panel-l2);color:var(--text);border:1px solid var(--accent);padding:2px 4px;font-size:12px;border-radius:3px;';
    sel.innerHTML = '<option value="true"' + (oldVal === 'true' ? ' selected' : '') + '>true</option>' +
      '<option value="false"' + (oldVal === 'false' ? ' selected' : '') + '>false</option>';
    sel.onchange = function() { sel.blur(); };
    sel.onblur = async function() {
      const newStr = sel.value;
      if (newStr === oldVal) { td.innerHTML = escapeHtml(oldVal); td.classList.add('param-val-editable'); return; }
      td.innerHTML = escapeHtml(newStr) + ' <span style="color:var(--warn);font-size:10px">儲存中…</span>';
      await submitEdit(td, key, oldVal, newStr === 'true');
    };
    td.innerHTML = '';
    td.classList.remove('param-val-editable');
    td.appendChild(sel);
    sel.focus();
    return;
  }

  const input = document.createElement('input');
  input.type = 'text';
  input.value = oldVal;
  input.style.cssText = 'width:100%;background:var(--panel-l2);color:var(--text);border:1px solid var(--accent);padding:2px 4px;font-size:12px;border-radius:3px;';
  input.onkeydown = async function(e) {
    if (e.key === 'Enter') { input.blur(); }
    if (e.key === 'Escape') { td.innerHTML = escapeHtml(oldVal); td.classList.add('param-val-editable'); }
  };
  input.onblur = async function() {
    const raw = input.value.trim();
    if (raw === oldVal) {
      td.innerHTML = escapeHtml(oldVal);
      td.classList.add('param-val-editable');
      return;
    }
    if (pType === 'string') {
      td.innerHTML = escapeHtml(raw) + ' <span style="color:var(--warn);font-size:10px">儲存中…</span>';
      await submitEdit(td, key, oldVal, raw);
      return;
    }
    const newVal = parseFloat(raw);
    if (isNaN(newVal) || newVal === parseFloat(oldVal)) {
      td.innerHTML = escapeHtml(oldVal);
      td.classList.add('param-val-editable');
      return;
    }
    td.innerHTML = escapeHtml(String(newVal)) + ' <span style="color:var(--warn);font-size:10px">儲存中…</span>';
    await submitEdit(td, key, oldVal, newVal);
  };
  td.innerHTML = '';
  td.classList.remove('param-val-editable');
  td.appendChild(input);
  input.focus();
  input.select();
};

async function submitEdit(td, key, oldVal, bodyVal) {
  try {
    const body = {}; body[key] = bodyVal;
    const resp = await fetch('/api/parameters', { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify(body) });
    if (resp.ok) {
      td.innerHTML = escapeHtml(String(bodyVal));
      td.classList.add('param-val-editable');
      td.dataset.val = String(bodyVal);
    } else {
      const err = await resp.json().catch(() => ({}));
      td.innerHTML = escapeHtml(oldVal) + ' <span style="color:var(--down);font-size:10px">' + escapeHtml(err.error || '失敗') + '</span>';
      td.classList.add('param-val-editable');
    }
  } catch {
    td.innerHTML = escapeHtml(oldVal) + ' <span style="color:var(--down);font-size:10px">錯誤</span>';
    td.classList.add('param-val-editable');
  }
}

window._paramMapExpand = function(span) {
  const key = span.dataset.key;
  let map;
  try { map = JSON.parse(span.dataset.val); } catch(e) { return; }
  const entries = Object.entries(map);
  let html = '<table class="params-table map-sub-table"><thead><tr><th>Key</th><th>Value</th></tr></thead><tbody>';
  for (const [k, v] of entries) {
    const subKey = key + '.' + k;
    html += `<tr>
      <td class="param-key">${escapeHtml(k)}</td>
      <td class="param-val-editable" title="點擊編輯" data-key="${escapeHtml(subKey)}" data-val="${escapeHtml(String(v))}" onclick="window._paramEdit(this)">${escapeHtml(typeof v === 'number' ? v.toFixed(4) : String(v))}</td>
    </tr>`;
  }
  html += '</tbody></table>';
  html += `<button class="btn-sm" onclick="window._paramMapEdit(this)" data-key="${escapeHtml(key)}" data-val="${escapeHtml(span.dataset.val)}" style="margin-top:4px">JSON 批量編輯</button>`;
  span.parentElement.innerHTML = html;
};

window._paramMapEdit = function(btn) {
  const key = btn.dataset.key;
  const oldVal = btn.dataset.val;
  const td = btn.parentElement;
  const ta = document.createElement('textarea');
  ta.style.cssText = 'width:100%;min-height:80px;background:var(--panel-l2);color:var(--text);border:1px solid var(--accent);padding:4px;font-size:11px;border-radius:3px;font-family:monospace;';
  ta.value = JSON.stringify(JSON.parse(oldVal), null, 2);
  td.innerHTML = '';
  td.appendChild(ta);
  ta.focus();

  const save = document.createElement('button');
  save.textContent = '儲存';
  save.className = 'btn-sm';
  save.style.cssText = 'margin-top:4px;margin-right:4px';
  save.onclick = async function() {
    let newMap;
    try { newMap = JSON.parse(ta.value); } catch(e) {
      td.innerHTML = '<span style="color:var(--down)">JSON 格式錯誤</span>';
      return;
    }
    if (typeof newMap !== 'object' || Array.isArray(newMap)) {
      td.innerHTML = '<span style="color:var(--down)">必須是物件格式</span>';
      return;
    }
    td.innerHTML = ' <span style="color:var(--warn);font-size:10px">儲存中…</span>';
    try {
      const body = {}; body[key] = newMap;
      const resp = await fetch('/api/parameters', { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify(body) });
      if (resp.ok) {
        const newVal = JSON.stringify(newMap);
        const entries = Object.entries(newMap);
        let html = `<span class="param-map-collapsed" onclick="window._paramMapExpand(this)" data-key="${escapeHtml(key)}" data-val="${escapeHtml(newVal)}" title="點擊展開">${entries.length} entries — 已儲存</span>`;
        td.innerHTML = html;
      } else {
        const err = await resp.json().catch(() => ({}));
        td.innerHTML = '<span style="color:var(--down)">' + escapeHtml(err.error || '儲存失敗') + '</span>';
      }
    } catch {
      td.innerHTML = '<span style="color:var(--down)">網路錯誤</span>';
    }
  };

  const cancel = document.createElement('button');
  cancel.textContent = '取消';
  cancel.className = 'btn-sm';
  cancel.style.cssText = 'margin-top:4px';
  cancel.onclick = function() {
    const entries = Object.entries(JSON.parse(oldVal));
    let html = `<span class="param-map-collapsed" onclick="window._paramMapExpand(this)" data-key="${escapeHtml(key)}" data-val="${escapeHtml(oldVal)}">${entries.length} entries</span>`;
    td.innerHTML = html;
  };

  td.appendChild(save);
  td.appendChild(cancel);
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
