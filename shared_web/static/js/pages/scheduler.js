// Scheduler task management page — shows all BackgroundTaskManager tasks with
// cross-restart liveness (task_liveness table) + live runtime status.
//
// Data sources:
//   /api/dashboard/task-liveness  — persisted last run / last success /
//                                   consecutive failures / stale flag
//                                   (survives restarts; includes cron pings)
//   /api/scheduler/status         — live runtime state (enabled, interval,
//                                   next run, channel id)
// Both are merged by task name; tasks only in the runtime status (never
// written to liveness yet) are synthesized from the live state.

export function renderSchedulerPage(tasks, getJSON) {
  var el = document.getElementById('schedulerContent');
  if (!el) return;
  if (!tasks || tasks.length === 0) {
    el.innerHTML = '<div class="empty">無排程任務</div>';
    return;
  }

  var ok = 0, err = 0, stale = 0, disabled = 0, rows = '';
  tasks.sort(function(a, b) { return a.name.localeCompare(b.name); });

  for (var i = 0; i < tasks.length; i++) {
    var t = tasks[i];
    var statusClass = 'ok';
    var statusText = '正常';
    if (t.stale) {
      statusClass = 'warn';
      statusText = '逾期';
      stale++;
    }
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
    var lastRunStr = t.last_run_at ? formatTime(t.last_run_at) : '—';
    var lastSuccessStr = t.last_success_at ? formatTime(t.last_success_at) : '—';
    var nextRunStr = t.next_run_at ? formatTime(t.next_run_at) : '—';
    var staleBadge = t.stale
      ? '<span class="badge warn" title="' + escapeHtml(t.stale_reason || '') + '">逾期</span>'
      : '';
    var sourceBadge = t.source === 'cron'
      ? '<span class="badge info" title="cron 容器 ping 寫入，無 BTM 間隔">cron</span>'
      : '';

    rows += '<tr>' +
      '<td><strong>' + escapeHtml(t.name) + '</strong>' + sourceBadge + '</td>' +
      '<td>' + (t.channel_id
        ? '<span class="badge info">' + escapeHtml(t.channel_id) + '</span>'
        : '<span class="text-muted" style="font-size:11px" title="' + escapeHtml(t.description || '系統任務') + '">' + escapeHtml(t.description || '系統任務') + '</span>') + '</td>' +
      '<td>' + intervalStr + '</td>' +
      '<td>' + lastRunStr + '</td>' +
      '<td>' + lastSuccessStr + '</td>' +
      '<td>' + (t.consecutive_failures > 0
        ? '<span class="' + (t.consecutive_failures >= 3 ? 'text-danger' : 'text-warn') + '">' + t.consecutive_failures + '</span>'
        : '<span class="text-muted">0</span>') + '</td>' +
      '<td>' + staleBadge + (t.last_error ? '<span class="text-muted" style="font-size:11px" title="' + escapeHtml(t.last_error) + '">⚠</span>' : '') + '</td>' +
      '<td>' + nextRunStr + '</td>' +
      '<td><span class="badge ' + statusClass + '">' + statusText + '</span></td>' +
    '</tr>';
  }

  var total = tasks.length;
  el.innerHTML =
    '<div class="rc-summary mt-md">' +
      '<div class="rc-summary__left">' +
        '<button onclick="loadSchedulerPage()">刷新狀態</button>' +
        '<span class="text-muted text-sm" style="margin-left:10px">來源：task_liveness（跨重啟持久化）＋ /api/scheduler/status（即時）</span>' +
      '</div>' +
      '<div class="rc-summary__gauge">' +
        // 2026-08-24 UI audit P2：狀態條依比例分段（正常綠/異常紅/逾期黃），
        // 不再「任一異常就整條變紅」造成 86/97 正常卻全紅的誤讀。
        '<div class="rc-summary__bar">' +
          '<div class="rc-summary__fill" style="width:' + (total > 0 ? (ok / total * 100) : 0) + '%;background:var(--color-success)"></div>' +
          (err > 0 ? '<div class="rc-summary__fill" style="width:' + (err / total * 100) + '%;background:var(--color-danger)"></div>' : '') +
          (stale > 0 ? '<div class="rc-summary__fill" style="width:' + (stale / total * 100) + '%;background:var(--color-warning)"></div>' : '') +
        '</div>' +
        '<span class="rc-summary__score">' + total + '</span>' +
        '<span class="text-muted text-sm" style="margin-left:8px">總任務</span>' +
      '</div>' +
      '<div class="flex" style="gap:16px;margin-top:8px">' +
        '<div class="text-sm"><span class="text-success">●</span> 正常 <span>' + ok + '</span></div>' +
        '<div class="text-sm"><span class="text-danger">●</span> 異常 <span>' + err + '</span></div>' +
        '<div class="text-sm"><span class="text-warn">●</span> 逾期 <span>' + stale + '</span></div>' +
        '<div class="text-sm"><span class="text-muted">●</span> 已停用 <span>' + disabled + '</span></div>' +
      '</div>' +
    '</div>' +
    '<div style="overflow-x:auto;margin-top:12px">' +
    '<table>' +
      '<thead><tr>' +
        '<th style="min-width:180px">任務名稱</th>' +
        '<th>通道</th>' +
        '<th>間隔</th>' +
        '<th>上次執行</th>' +
        '<th>上次成功</th>' +
        '<th>連敗</th>' +
        '<th>逾期/錯誤</th>' +
        '<th>下次執行</th>' +
        '<th>狀態</th>' +
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

// Merge persisted liveness rows with live runtime status by task name.
function mergeLivenessAndStatus(livenessTasks, statusTasks) {
  var byName = {};
  (statusTasks || []).forEach(function(s) {
    byName[s.name] = {
      name: s.name,
      channel_id: s.channel_id,
      enabled: s.enabled,
      interval: s.interval,
      next_run_at: s.next_run,
      last_run_at: s.last_run,
      consecutive_failures: s.consecutive_failures || 0,
      last_error: s.last_error || '',
      stale: false,
      source: 'btm'
    };
  });
  (livenessTasks || []).forEach(function(l) {
    var base = byName[l.name] || {};
    byName[l.name] = {
      name: l.name,
      description: base.description || '',
      channel_id: base.channel_id,
      enabled: base.enabled,
      interval: l.interval || base.interval,
      next_run_at: l.next_run_at || base.next_run_at,
      last_run_at: l.last_run_at,
      last_success_at: l.last_success_at,
      last_error: l.last_error || '',
      consecutive_failures: l.consecutive_failures || 0,
      last_duration_ms: l.last_duration_ms,
      stale: !!l.stale,
      stale_reason: l.stale_reason || '',
      source: l.source || base.source || 'btm'
    };
  });
  return Object.keys(byName).map(function(k) { return byName[k]; });
}

// 2026-09-03 UI 審計：停用/啟用按鈕移除——內部管線不應由 UI 切換運行狀態
//（誤點即停用關鍵監控；ml_retrain/calibration_cycle 的停用屬 code/參數
// 層決策，UI toggle 反而會與其打架）。POST /api/scheduler/toggle 端點
// 保留給 CLI/維運腳本。postJSON import 一併移除。

// Global page reload function: fetches persisted liveness + live status.
export function loadSchedulerPage() {
  var el = document.getElementById('schedulerContent');
  if (el) el.innerHTML = '<div class="empty loading">載入中…</div>';
  Promise.all([
    fetch('/api/dashboard/task-liveness').then(function(r) { return r.ok ? r.json() : null; }),
    fetch('/api/scheduler/status').then(function(r) { return r.ok ? r.json() : null; })
  ]).then(function(results) {
    var liveness = results[0];
    var status = results[1];
    if (!liveness && !status) {
      throw new Error('liveness 與 status API 皆不可用');
    }
    var tasks = mergeLivenessAndStatus(
      liveness && liveness.tasks ? liveness.tasks : [],
      Array.isArray(status) ? status : []
    );
    renderSchedulerPage(tasks);
  }).catch(function(err) {
    if (el) el.innerHTML = '<div class="empty" style="color:var(--color-danger)">載入失敗: ' + err.message + '</div>';
  });
};

// 註：window-level 接線由 admin main.js loadModules（modules.scheduler block）
// 統一處理（PR-7 補 'scheduler' key 後走正常 path）。此處不再自我註冊，避免
// 與 loadModules 雙重接線。本模組僅被 admin_web 動態 import。
