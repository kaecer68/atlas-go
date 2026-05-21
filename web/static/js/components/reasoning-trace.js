import { getJSON, escapeHtml } from '../main.js';

function renderSessionBar(sessions, currentId) {
  if (!sessions.length) {
    return '<div class="help-panel">尚無可用場次，請先執行回測或模擬</div>';
  }
  var opts = sessions.map(function(s) {
    var sel = s.session_id === currentId ? ' selected' : '';
    return '<option value="' + escapeHtml(s.session_id) + '"' + sel + '>' + escapeHtml(s.session_id) + ' \u00B7 ' + escapeHtml(s.regime) + ' \u00B7 ' + new Date(s.recorded_at).toLocaleDateString('zh-TW') + '</option>';
  }).join('');
  var latest = sessions[0];
  var syncHtml = '';
  if (latest) {
    var latestDate = new Date(latest.recorded_at);
    var today = new Date();
    var diffDays = Math.floor((today - latestDate) / (1000 * 60 * 60 * 24));
    if (diffDays > 1) {
      syncHtml = '<div style="margin:8px 0;padding:8px 12px;border-radius:4px;font-size:12px;background:#fef3cd;border:1px solid #fde68a;color:#854d0e">' +
        '\u26A0\uFE0F 最新場次為 ' + diffDays + ' 天前（' + latestDate.toLocaleDateString('zh-TW') + '），可能已非當日同步' +
        '</div>';
    } else {
      syncHtml = '<div style="margin:8px 0;padding:8px 12px;border-radius:4px;font-size:12px;background:#ecfdf5;border:1px solid #a7f3d0;color:#065f46">' +
        '\u2705 場次已同步（' + latestDate.toLocaleDateString('zh-TW') + '）' +
        '</div>';
    }
  }
  return syncHtml +
    '<div style="margin-bottom:16px;font-size:12px;display:flex;align-items:center;gap:8px">' +
    '<span>場次：</span>' +
    '<select id="reasoningTraceSessionSelect" style="font-size:12px;padding:4px 8px;border-radius:4px;border:1px solid var(--border);background:var(--panel);color:var(--text)" onchange="toggleReasoningTraceSession(this)">' +
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

  var sessions = window.pipelineSessions;
  if (!sessions || !sessions.length) {
    try {
      var resp = await getJSON('/api/dashboard/sessions');
      sessions = (resp && resp.sessions) ? resp.sessions : [];
      window.pipelineSessions = sessions;
    } catch (e) { sessions = []; }
  }

  if (!sessionId && sessions.length) {
    sessionId = sessions[0].session_id;
    window._currentSessionId = sessionId;
  }

  if (!sessionId) {
    container.innerHTML = '<div class="help-panel">尚無可用場次<br><small class="text-muted">請先執行回測或模擬以產生場次資料</small></div>';
    return;
  }

  container.innerHTML = '<div class="loading">載入中\u2026</div>';

  try {
    var data = await getJSON('/api/dashboard/reasoning-trace?session_id=' + encodeURIComponent(sessionId));
    container.innerHTML = renderSessionBar(sessions, sessionId);
    var timeline = document.getElementById('reasoningTraceTimeline');
    if (timeline) renderReasoningTimeline(data, timeline);
  } catch (e) {
    console.error('Failed to load reasoning trace:', e);
    container.innerHTML = renderSessionBar(sessions, sessionId);
    var t2 = document.getElementById('reasoningTraceTimeline');
    if (t2) t2.innerHTML = '<div class="help-panel text-down">載入失敗: ' + escapeHtml(e.message) + '</div>';
  }
}

export function renderReasoningTimeline(data, timelineEl) {
  var timeline = timelineEl || document.getElementById('reasoningTraceTimeline');
  if (!timeline) return;

  if (!data || !data.traces || data.traces.length === 0) {
    timeline.innerHTML = '<div class="help-panel">目前沒有決策追蹤資料</div>';
    return;
  }

  var phases = {
    regime_detection: { label: '盤勢判定', color: '#3b82f6' },
    agent_recommendation: { label: '代理推薦', color: '#22c55e' },
    control_filter: { label: '控制層過濾', color: '#f97316' },
    portfolio_build: { label: '組合構建', color: '#a855f7' }
  };

  var html = '<div><h2 style="font-size: 18px; margin-bottom: 20px;">決策追蹤 (Session: ' + escapeHtml(data.session_id) + ')</h2>';
  html += '<div style="position: relative; border-left: 2px solid var(--border); margin-left: 10px; padding-left: 20px;">';

  data.traces.forEach(function(trace) {
    var p = phases[trace.phase] || { label: trace.phase, color: '#9ca3af' };
    var pct = Math.round((trace.confidence || 0) * 100);
    var fb = trace.is_fallback ? '<span style="margin-left:8px;padding:2px 6px;font-size:11px;font-weight:bold;background:#facc15;color:#854d0e;border-radius:4px">備援</span>' : '';

    html +=
      '<div style="position:relative;margin-bottom:24px">' +
        '<div style="position:absolute;width:12px;height:12px;border-radius:50%;left:-27px;top:4px;background:' + p.color + ';box-shadow:0 0 0 3px var(--bg)"></div>' +
        '<div class="card" style="padding:16px">' +
          '<div class="flex-between mb-sm">' +
            '<h3 style="margin:0;font-size:15px;color:' + p.color + '">' + escapeHtml(p.label) + ' ' + fb + '</h3>' +
            '<span class="text-sm text-muted">' + escapeHtml(trace.component) + ' / ' + escapeHtml(trace.action) + '</span>' +
          '</div>' +
          '<div class="mb-sm" style="display:flex;align-items:center;gap:10px">' +
            '<span class="text-sm text-muted" style="min-width:60px">信心水準</span>' +
            '<div style="flex-grow:1;height:6px;background:var(--panel-l3);border-radius:3px;overflow:hidden">' +
              '<div style="height:100%;width:' + pct + '%;background:' + p.color + '"></div>' +
            '</div>' +
            '<span class="text-sm text-muted" style="min-width:40px;text-align:right">' + pct + '%</span>' +
          '</div>' +
          (trace.explanation ?
            '<div class="mb-md" style="padding:12px;background:rgba(59,130,246,.1);border:1px solid rgba(59,130,246,.2);border-radius:4px;font-size:13px;color:var(--text);white-space:pre-line;line-height:1.5">' + escapeHtml(trace.explanation) + '</div>' : '') +
          '<details>' +
            '<summary style="cursor:pointer;font-size:12px;color:var(--muted);user-select:none">原始資料 (Raw JSON)</summary>' +
            '<pre style="margin-top:8px;padding:10px;background:var(--panel-l2);border-radius:4px;overflow-x:auto;font-size:11px;font-family:var(--font-mono);color:var(--muted)">' + escapeHtml(JSON.stringify(trace.raw_data || {}, null, 2)) + '</pre>' +
          '</details>' +
        '</div>' +
      '</div>';
  });

  html += '</div></div>';
  timeline.innerHTML = html;
}

if (typeof window !== 'undefined') {
  window.toggleReasoningTraceSession = toggleReasoningTraceSession;
}
