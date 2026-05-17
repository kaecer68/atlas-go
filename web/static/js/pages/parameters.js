import { escapeHtml } from '../shared/app-utils.js';

export function renderParametersPage(params, categories, auditLog) {
  const contentDiv = document.getElementById('parametersContent');
  if (!contentDiv) return;

  if (!params || Object.keys(params).length === 0) {
    contentDiv.innerHTML = '<div class="empty" style="text-align:center;padding:40px">無法載入參數配置或目前無任何參數。</div>';
  } else {
    let html = '<div class="two-col-grid">';
    
    const uncategorized = new Set(Object.keys(params));
    
    if (categories && Object.keys(categories).length > 0) {
      for (const [catName, keys] of Object.entries(categories)) {
        html += `<div class="panel"><h3>${escapeHtml(catName)}</h3><table><tbody>`;
        let hasKeys = false;
        for (const key of keys) {
          if (key in params) {
            let displayVal = params[key];
            if (typeof displayVal === 'object') displayVal = JSON.stringify(displayVal);
            else displayVal = String(displayVal);
            
            html += `<tr>
              <td style="word-break:break-all;width:55%;color:var(--muted)">${escapeHtml(key)}</td>
              <td style="font-family:var(--font-mono);font-size:12px;color:var(--accent)">${escapeHtml(displayVal)}</td>
            </tr>`;
            uncategorized.delete(key);
            hasKeys = true;
          }
        }
        if (!hasKeys) html += `<tr><td colspan="2" class="empty">無參數</td></tr>`;
        html += `</tbody></table></div>`;
      }
    }
    
    if (uncategorized.size > 0) {
      html += `<div class="panel"><h3>其他參數</h3><table><tbody>`;
      for (const key of uncategorized) {
        let displayVal = params[key];
        if (typeof displayVal === 'object') displayVal = JSON.stringify(displayVal);
        else displayVal = String(displayVal);
        
        html += `<tr>
          <td style="word-break:break-all;width:55%;color:var(--muted)">${escapeHtml(key)}</td>
          <td style="font-family:var(--font-mono);font-size:12px;color:var(--accent)">${escapeHtml(displayVal)}</td>
        </tr>`;
      }
      html += `</tbody></table></div>`;
    }
    
    html += '</div>';
    contentDiv.innerHTML = html;
    contentDiv.classList.remove('empty', 'loading');
  }
  
  const logDiv = document.getElementById('parametersAuditLog');
  if (!logDiv) return;
  
  if (!auditLog || !Array.isArray(auditLog) || auditLog.length === 0) {
    logDiv.innerHTML = '<div class="empty" style="text-align:center;padding:20px">尚無參數變更紀錄。</div>';
  } else {
    let html = '<div class="table-wrapper"><table><thead><tr><th>時間</th><th>參數</th><th>原值</th><th>新值</th><th>原因</th><th>操作者</th></tr></thead><tbody>';
    const logsToShow = auditLog.slice(0, 20);
    
    for (const log of logsToShow) {
      const timestamp = log.timestamp || log.Timestamp || log.changed_at || log.ChangedAt || log.time || log.Time || '-';
      const key = log.key || log.Key || log.parameter || log.Parameter || log.name || log.Name || '-';
      const oldVal = log.old_value !== undefined ? log.old_value : (log.OldValue !== undefined ? log.OldValue : (log.old !== undefined ? log.old : '-'));
      const newVal = log.new_value !== undefined ? log.new_value : (log.NewValue !== undefined ? log.NewValue : (log.new !== undefined ? log.new : '-'));
      const reason = log.reason || log.Reason || log.comment || log.Comment || '-';
      const user = log.user || log.User || log.operator || log.Operator || log.author || log.Author || 'system';
      
      const formatTime = t => {
        if (t === '-') return t;
        try { 
          const d = new Date(t);
          if (isNaN(d.getTime())) return t;
          return d.toLocaleString('zh-TW'); 
        } catch(e) { return t; }
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
    logDiv.classList.remove('empty', 'loading');
  }
}
