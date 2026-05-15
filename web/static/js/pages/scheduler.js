// Scheduler task management page — shows all BackgroundTaskManager tasks with status and toggle controls.

export function renderSchedulerPage(tasks, getJSON) {
  var el = document.getElementById('schedulerContent');
  if (!el) return;
  if (!tasks || tasks.length === 0) {
    el.innerHTML = '<div class="empty">無排程任務</div>';
    return;
  }

  var ok = 0, err = 0, disabled = 0, rows = '';
  tasks.sort(function(a, b) { return a.name.localeCompare(b.name); });

  for (var i = 0; i < tasks.length; i++) {
    var t = tasks[i];
    var statusClass = 'ok';
    var statusText = '正常';
    if (!t.enabled) {
      statusClass = 'info';
      statusText = '已停用';
      disabled++;
    } else if (t.consecutive_failures > 0) {
      statusClass = t.consecutive_failures >= 3 ? 'err' : 'warn';
      statusText = '失効' + t.consecutive_failures + (t.consecutive_failures > 1 ? ' 次' : ' 次');
      if (statusClass === 'err') err++;
      else ok++;
    } else {
      ok++;
    }

    var intervalStr = formatDuration(t.interval);
    var lastRunStr = t.last_run ? formatTime(t.last_run) : '—';
    var marketInfo = t.market_hours_only ? '<span class="badge warn">盤中限定</span>' : '';
    if (t.market_open_time) marketInfo += ' <span class="text-muted" style="font-size:11px">' + t.market_open_time + '-' + t.market_close_time + '</span>';

    rows += '<tr>' +
      '<td><strong>' + escapeHtml(t.name) + '</strong></td>' +
      '<td>' + (t.channel_id ? '<span class="badge info">' + escapeHtml(t.channel_id) + '</span>' : '<span class="text-muted">—</span>') + '</td>' +
      '<td>' + intervalStr + '</td>' +
      '<td>' + lastRunStr + '</td>' +
      '<td><span class="badge ' + statusClass + '">' + statusText + '</span></td>' +
      '<td>' + marketInfo + '</td>' +
      '<td><button class="' + (t.enabled ? 'danger' : 'primary') + '" style="font-size:11px;padding:3px 10px" onclick="toggleSchedulerTask(\'' + escapeHtml(t.name) + '\',' + (!t.enabled) + ')">' +
        (t.enabled ? '停用' : '啟用') +
      '</button></td>' +
    '</tr>';
  }

  var total = tasks.length;
  el.innerHTML =
    '<div class="rc-summary mt-md">' +
      '<div class="rc-summary__left">' +
        '<button onclick="loadSchedulerPage()">刷新狀態</button>' +
      '</div>' +
      '<div class="rc-summary__gauge">' +
        '<div class="rc-summary__bar"><div class="rc-summary__fill" style="width:' + (total > 0 ? (ok / total * 100) : 0) + '%;background:' + (err > 0 ? 'var(--color-danger)' : 'var(--color-success)') + '"></div></div>' +
        '<span class="rc-summary__score">' + total + '</span>' +
        '<span class="text-muted text-sm" style="margin-left:8px">總任務</span>' +
      '</div>' +
      '<div class="flex" style="gap:16px;margin-top:8px">' +
        '<div class="text-sm"><span class="text-up">●</span> 正常 <span>' + ok + '</span></div>' +
        '<div class="text-sm"><span class="text-warn">●</span> 異常 <span>' + err + '</span></div>' +
        '<div class="text-sm"><span class="text-muted">●</span> 已停用 <span>' + disabled + '</span></div>' +
      '</div>' +
    '</div>' +
    '<div style="overflow-x:auto;margin-top:12px">' +
    '<table>' +
      '<thead><tr>' +
        '<th style="min-width:180px">任務名稱</th>' +
        '<th>通道</th>' +
        '<th>間隔</th>' +
        '<th>最後執行</th>' +
        '<th>狀態</th>' +
        '<th>設定</th>' +
        '<th style="width:80px">操作</th>' +
      '</tr></thead>' +
      '<tbody>' + rows + '</tbody>' +
    '</table></div>';
}

function formatDuration(d) {
  if (!d) return '—';
  if (typeof d === 'string') {
    var match = d.match(/(\d+)h(\d+)m(\d+)s/);
    if (match) {
      var parts = [];
      if (+match[1]) parts.push(match[1] + 'h');
      if (+match[2]) parts.push(match[2] + 'm');
      if (+match[3]) parts.push(match[3] + 's');
      return parts.join(' ') || d;
    }
    return d;
  }
  return String(d);
}

function formatTime(iso) {
  if (!iso) return '—';
  try {
    var d = new Date(iso);
    return d.toLocaleString('zh-TW', { hour12: false, month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
  } catch(e) {
    return iso;
  }
}

// Global toggle function called from HTML onclick
window.toggleSchedulerTask = function(name, enabled) {
  fetch('/api/scheduler/toggle', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name: name, enabled: enabled })
  }).then(function(r) {
    if (!r.ok) throw new Error('HTTP ' + r.status);
    return r.json();
  }).then(function(resp) {
    var el = document.getElementById('schedulerContent');
    if (el) el.innerHTML = '<div class="empty loading">載入中…</div>';
    loadSchedulerPage();
  }).catch(function(err) {
    console.error('toggle task ' + name + ':', err.message);
    alert('切換失敗: ' + err.message);
  });
};

// Global page reload function
window.loadSchedulerPage = function() {
  var el = document.getElementById('schedulerContent');
  if (el) el.innerHTML = '<div class="empty loading">載入中…</div>';
  fetch('/api/scheduler/status').then(function(r) { return r.json(); }).then(function(tasks) {
    renderSchedulerPage(tasks);
  }).catch(function(err) {
    if (el) el.innerHTML = '<div class="empty" style="color:var(--down)">載入失敗: ' + err.message + '</div>';
  });
};
