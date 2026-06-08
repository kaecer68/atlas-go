import { getJSON } from '../shared/app-utils.js';
import { escapeHtml } from '../shared/utils.js';

function renderSessionBar(sessions, currentId) {
  if (!sessions.length) {
    return '<div class="help-panel">尚無可用場次，請先執行回測或模擬</div>';
  }
  var opts = sessions.map(function(s) {
    var sel = s.session_id === currentId ? ' selected' : '';
    return '<option value="' + escapeHtml(s.session_id) + '"' + sel + '>' + escapeHtml(s.session_id) + ' \u00B7 ' + escapeHtml(s.regime) + ' \u00B7 ' + new Date(s.recorded_at).toLocaleDateString('zh-TW') + '</option>';
  }).join('');

  // Sync status: compare latest session date with today
  var latest = sessions[0];
  var syncHtml = '';
  if (latest) {
    var latestDate = new Date(latest.recorded_at);
    var today = new Date();
    var diffDays = Math.floor((today - latestDate) / (1000 * 60 * 60 * 24));
    if (diffDays > 0) {
      syncHtml = '<div class="rt-sync-warn">' +
        '\u26A0\uFE0F 最新場次為 ' + diffDays + ' 天前（' + latestDate.toLocaleDateString('zh-TW') + '），可能已非當日同步' +
        '</div>';
    } else {
      syncHtml = '<div class="rt-sync-ok">' +
        '\u2705 場次已同步（' + latestDate.toLocaleDateString('zh-TW') + '）' +
        '</div>';
    }
  }

  return syncHtml +
    '<div class="rt-session-bar">' +
    '<span>場次：</span>' +
    '<select id="reasoningTraceSessionSelect" class="rt-session-select" onchange="toggleReasoningTraceSession(this)">' +
    opts +
    '</select>' +
    '</div>' +
    '<div id="reasoningTraceTimeline"></div>';
}

export async function toggleReasoningTraceSession(select) {
  if (!select || !select.value) return;
  window._currentSessionId = select.value;
  await loadReasoningTrace(select.value);
}

export async function loadReasoningTrace(sessionId) {
  var container = document.getElementById('page-reasoning-trace');
  if (!container) return;

  // Fetch sessions directly from API — no dependency on loadAll() timing
  var sessions = window.pipelineSessions;
  if (!sessions || !sessions.length) {
    try {
      var resp = await getJSON('/api/dashboard/sessions');
      sessions = (resp && resp.sessions) ? resp.sessions : [];
      window.pipelineSessions = sessions;
    } catch (e) {
      sessions = [];
    }
  }
  if (!sessions) { sessions = []; }

  // Auto-select latest session when none specified
  if (!sessionId && sessions.length) {
    sessionId = sessions[0].session_id;
    window._currentSessionId = sessionId;
  }

  if (!sessionId) {
    container.innerHTML = '<div class="help-panel">尚無可用場次<br><small class="text-muted">請先執行回測或模擬以產生場次資料</small></div>';
    return;
  }

  container.innerHTML = '<div class="loading">載入中\u2026</div>';
  var timeline;

  try {
    var data = await getJSON('/api/dashboard/reasoning-trace?session_id=' + encodeURIComponent(sessionId));
    var html = renderSessionBar(sessions, sessionId);
    container.innerHTML = html;
    timeline = document.getElementById('reasoningTraceTimeline');
    if (timeline) renderReasoningTimeline(data, timeline);
  } catch (e) {
    console.error('Failed to load reasoning trace:', e);
    var html2 = renderSessionBar(sessions, sessionId);
    container.innerHTML = html2;
    timeline = document.getElementById('reasoningTraceTimeline');
    if (timeline) timeline.innerHTML = '<div class="help-panel text-down">載入失敗: ' + escapeHtml(e.message) + '</div>';
  }
}

export function renderReasoningTimeline(data, timelineEl) {
  var timeline = timelineEl;
  if (!timeline) {
    timeline = document.getElementById('reasoningTraceTimeline');
    if (!timeline) return;
  }

  if (!data || !data.traces || data.traces.length === 0) {
    timeline.innerHTML = '<div class="help-panel">目前沒有決策追蹤資料</div>';
    return;
  }

  var phases = {
    'regime_detection': { label: '盤勢判定', color: '#3b82f6' },
    'agent_recommendation': { label: '代理推薦', color: '#22c55e' },
    'control_filter': { label: '控制層過濾', color: '#f97316' },
    'portfolio_build': { label: '組合構建', color: '#a855f7' }
  };

  var html = '<div><h2 style="font-size: 18px; margin-bottom: 20px;">決策追蹤 (Session: ' + escapeHtml(data.session_id) + ')</h2>';
  html += '<div style="position: relative; border-left: 2px solid var(--border); margin-left: 10px; padding-left: 20px;">';

  data.traces.forEach(function(trace, idx) {
    var p = phases[trace.phase] || { label: trace.phase, color: '#9ca3af' };
    var pct = Math.round((trace.confidence || 0) * 100);
    var fb = trace.is_fallback ? '<span class="rt-fallback-badge">備援</span>' : '';
    var rawId = 'rt-raw-' + idx;

    html +=
      '<div class="rt-trace-node">' +
        '<div class="rt-trace-dot" style="background:' + p.color + '"></div>' +
        '<div class="card rt-trace-card">' +
          '<div class="flex-between mb-sm">' +
            '<h3 class="rt-trace-title" style="color:' + p.color + '">' + escapeHtml(p.label) + ' ' + fb + '</h3>' +
            '<span class="text-sm text-muted">' + escapeHtml(trace.component) + ' / ' + escapeHtml(trace.action) + '</span>' +
          '</div>' +
          '<div class="rt-confidence-bar">' +
            '<span class="text-sm text-muted" style="min-width:60px">信心水準</span>' +
            '<div class="rt-confidence-track">' +
              '<div class="rt-confidence-fill" style="width:' + pct + '%;background:' + p.color + '"></div>' +
            '</div>' +
            '<span class="text-sm text-muted" style="min-width:40px;text-align:right">' + pct + '%</span>' +
          '</div>' +
          (trace.explanation ?
            '<div class="rt-explanation">' +
              escapeHtml(trace.explanation) +
            '</div>' : '') +
          '<details class="rt-raw-details" data-raw="' + escapeHtml(JSON.stringify(trace.raw_data || {})) + '">' +
            '<summary class="rt-raw-summary">原始資料 (Raw JSON)</summary>' +
            '<pre class="rt-raw-pre" id="' + rawId + '"></pre>' +
          '</details>' +
        '</div>' +
      '</div>';
  });

  // Lazy-load raw JSON on expand to avoid DOM bloat.
  setTimeout(function() {
    timeline.querySelectorAll('.rt-raw-details').forEach(function(details) {
      details.addEventListener('toggle', function() {
        if (details.open) {
          var pre = details.querySelector('.rt-raw-pre');
          if (pre && !pre.textContent) {
            try {
              pre.textContent = JSON.stringify(JSON.parse(details.dataset.raw), null, 2);
            } catch (e) {
              pre.textContent = details.dataset.raw;
            }
          }
        }
      });
    });
  }, 0);

  html += '</div></div>';
  timeline.innerHTML = html;
}

if (typeof window !== 'undefined') {
  window.toggleReasoningTraceSession = toggleReasoningTraceSession;
}
