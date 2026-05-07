// 演化透視面板 — Agent 競爭、Regime 時間線、實驗記錄
import { agentName, sectorName, regimeLabel } from '../names.js';
import { getJSON, formatDate, notify } from '../shared/app-utils.js';
import { renderEquityCurve } from '../components/sparkline.js';

let evolutionData = null;
let currentView = 'compact';

export async function loadEvolutionData() {
  const [agents, regime, inbox] = await Promise.all([
    getJSON('/api/dashboard/agent-observatory').catch(() => null),
    getJSON('/api/dashboard/regime-history').catch(() => null),
    getJSON('/api/dashboard/experiment-inbox').catch(() => null),
  ]);
  evolutionData = { agents, regime, inbox };
  var el = document.getElementById('evolutionContent');
  if (el) el.classList.remove('loading', 'empty');
  renderEquityFromData();
  renderCurrentView();
}

function renderCurrentView() {
  document.querySelectorAll('#evolutionViews .view-btn').forEach(function(b) { b.classList.remove('active'); });
  var btn = document.getElementById('evView-' + currentView);
  if (btn) btn.classList.add('active');

  if (currentView === 'compact') renderCompact();
  else if (currentView === 'detailed') renderDetailed();
  else if (currentView === 'categorical') renderCategorical();
}

export function switchView(mode) {
  currentView = mode;
  renderCurrentView();
}

function renderEquityFromData() {
  var d = evolutionData;
  var scorecards = (d.agents && d.agents.weakest_agent_scorecards) || [];
  if (scorecards.length < 2) return;

  var sorted = scorecards.slice().sort(function(a, b) { return (a.last_updated_at || '').localeCompare(b.last_updated_at || ''); });
  var points = sorted.map(function(a, i) {
    return {
      value: a.average_return || 0,
      label: (i + 1).toString()
    };
  });
  renderEquityCurve(points);
}

// 用 Canvas 繪製內嵌權益曲線（不依賴外部 DOM 元素）
function renderInlineEquityCurve(containerId, points) {
  var container = document.getElementById(containerId);
  if (!container || !points || points.length < 2) return;
  var canvas = document.createElement('canvas');
  canvas.style.width = '100%'; canvas.style.height = '200px';
  container.appendChild(canvas);

  var ctx = canvas.getContext('2d');
  var dpr = window.devicePixelRatio || 1;
  var rect = container.getBoundingClientRect();
  var W = rect.width - 20, H = 200;
  canvas.width = W * dpr; canvas.height = H * dpr;
  ctx.scale(dpr, dpr);
  var pad = {top: 10, right: 15, bottom: 25, left: 45};
  var chartW = W - pad.left - pad.right, chartH = H - pad.top - pad.bottom;

  var values = points.map(function(p) { return p.value; });
  var minV = Math.min.apply(null, values), maxV = Math.max.apply(null, values), range = maxV - minV || 1;
  ctx.clearRect(0, 0, W, H);

  // Background
  ctx.fillStyle = '#0d1015';
  ctx.beginPath(); ctx.roundRect(pad.left, pad.top, chartW, chartH, 6); ctx.fill();

  // Grid
  ctx.strokeStyle = 'rgba(255,255,255,0.04)'; ctx.lineWidth = 0.5;
  for (var i = 1; i <= 3; i++) {
    var gy = pad.top + (chartH / 4) * i;
    ctx.beginPath(); ctx.moveTo(pad.left, gy); ctx.lineTo(pad.left + chartW, gy); ctx.stroke();
  }

  // Y-axis
  ctx.fillStyle = 'rgba(184,196,208,0.5)'; ctx.font = '9px system-ui'; ctx.textAlign = 'right';
  for (var i = 0; i <= 4; i++) {
    var ly = pad.top + (chartH / 4) * i;
    ctx.fillText((maxV - (range / 4) * i).toFixed(3), pad.left - 6, ly + 3);
  }

  // Points
  var pts = points.map(function(p, i) {
    return { x: pad.left + (i / (points.length - 1)) * chartW, y: pad.top + (1 - (p.value - minV) / range) * chartH };
  });

  // Gradient fill
  var grad = ctx.createLinearGradient(0, pad.top, 0, pad.top + chartH);
  grad.addColorStop(0, 'rgba(79,193,255,0.22)');
  grad.addColorStop(1, 'rgba(79,193,255,0.01)');
  ctx.beginPath();
  ctx.moveTo(pts[0].x, pad.top + chartH);
  for (var j = 0; j < pts.length; j++) ctx.lineTo(pts[j].x, pts[j].y);
  ctx.lineTo(pts[pts.length - 1].x, pad.top + chartH);
  ctx.closePath();
  ctx.fillStyle = grad; ctx.fill();

  // Line with glow
  ctx.save();
  ctx.shadowColor = 'rgba(79,193,255,0.5)'; ctx.shadowBlur = 5;
  ctx.strokeStyle = '#4fc1ff'; ctx.lineWidth = 2;
  ctx.lineJoin = 'round'; ctx.beginPath();
  pts.forEach(function(p, i) { i === 0 ? ctx.moveTo(p.x, p.y) : ctx.lineTo(p.x, p.y); });
  ctx.stroke();
  ctx.restore();

  // Dots
  if (pts.length <= 20) {
    ctx.fillStyle = '#4fc1ff';
    pts.forEach(function(p) { ctx.beginPath(); ctx.arc(p.x, p.y, 2, 0, Math.PI * 2); ctx.fill(); });
  }
}

function renderCompact() {
  var el = document.getElementById('evolutionContent');
  var catEl = document.getElementById('evolutionCatContent');
  if (catEl) catEl.style.display = 'none';
  if (!el) return;
  var d = evolutionData;

  var scorecards = (d.agents && d.agents.weakest_agent_scorecards) || [];
  var sessions = (d.regime && d.regime.sessions) || [];
  var judges = (d.inbox && d.inbox.pending_judges) || [];
  var promotes = (d.inbox && d.inbox.pending_promotes) || [];
  var sorted = scorecards.slice().sort(function(a, b) { return (b.sharpe || 0) - (a.sharpe || 0); });

  var regimeHtml = renderRegimeTimeline(sessions, 40);

  var top5 = sorted.slice(0, 5);
  var agentSummaryHtml = top5.map(function(a, i) {
    var barW = Math.max(5, Math.min(100, (a.hit_rate || 0) * 100));
    var sharpeColor = (a.sharpe || 0) > 1 ? 'var(--up)' : ((a.sharpe || 0) < 0 ? 'var(--down)' : 'var(--warn)');
    return '<div style="display:flex;align-items:center;gap:8px;padding:4px 0;border-bottom:1px solid var(--border)">' +
      '<span style="font-size:11px;color:var(--muted);width:18px">' + (i + 1) + '</span>' +
      '<span style="flex:1;font-size:12px">' + agentName(a.agent_id) + '</span>' +
      '<div style="width:80px;height:4px;background:var(--bg);border-radius:2px">' +
        '<div style="width:' + barW + '%;height:100%;background:' + (barW > 50 ? 'var(--up)' : 'var(--warn)') + ';border-radius:2px"></div>' +
      '</div>' +
      '<span style="font-size:11px;color:var(--muted);width:35px;text-align:right">' + ((a.hit_rate || 0) * 100).toFixed(0) + '%</span>' +
      '<span style="font-size:11px;color:' + sharpeColor + ';width:45px;text-align:right">S:' + (a.sharpe || 0).toFixed(2) + '</span>' +
      '</div>';
  }).join('');

  var expCount = judges.length + promotes.length;
  var expHtml = '<div style="display:flex;gap:24px;margin-top:4px">' +
    '<div style="text-align:center"><div style="font-size:20px;font-weight:700;color:' + (judges.length > 0 ? 'var(--warn)' : 'var(--muted)') + '">' + judges.length + '</div><div style="font-size:11px;color:var(--muted)">待評判</div></div>' +
    '<div style="text-align:center"><div style="font-size:20px;font-weight:700;color:' + (promotes.length > 0 ? 'var(--up)' : 'var(--muted)') + '">' + promotes.length + '</div><div style="font-size:11px;color:var(--muted)">待晉升</div></div>' +
    '<div style="text-align:center"><div style="font-size:20px;font-weight:700">' + scorecards.length + '</div><div style="font-size:11px;color:var(--muted)">活躍 Agent</div></div>' +
    '</div>';

  el.innerHTML = '<div class="panel wide" style="margin-bottom:12px;padding:10px 14px">' +
    '<div style="font-size:13px;font-weight:700;margin-bottom:8px">Agent 回報分佈</div>' +
    '<div id="inlineEquityCurve"></div>' +
    '</div>' +
    '<div class="panel wide" style="margin-bottom:12px;padding:10px 14px">' +
    '<div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px">' +
      '<div style="font-size:13px;font-weight:700">Regime 演化</div>' +
      '<div style="font-size:11px;color:var(--muted)">' + sessions.length + ' 個 session</div>' +
    '</div>' +
    regimeHtml +
    '</div>' +

  // 繪製內嵌權益曲線
  var sortedForCurve = sorted.slice().sort(function(a, b) {
    return (a.last_updated_at || '').localeCompare(b.last_updated_at || '');
  });
  var curvePoints = sortedForCurve.map(function(a, i) {
    return { value: a.average_return || 0, label: (i + 1).toString() };
  });
  setTimeout(function() { renderInlineEquityCurve('inlineEquityCurve', curvePoints); }, 100);
    '<div style="display:grid;grid-template-columns:1fr 1fr;gap:12px">' +
      '<div class="panel" style="padding:10px 14px">' +
        '<div style="font-size:13px;font-weight:700;margin-bottom:8px">Agent 表現 Top 5</div>' +
        agentSummaryHtml +
      '</div>' +
      '<div class="panel" style="padding:10px 14px">' +
        '<div style="font-size:13px;font-weight:700;margin-bottom:8px">實驗活動</div>' +
        expHtml +
        '<div style="margin-top:12px;font-size:11px;color:var(--muted)">' +
          (judges.length + promotes.length > 0
            ? '當前有 ' + expCount + ' 個實驗排隊。系統正在持續進化 Agent 策略。'
            : '當前無待處理實驗。系統處於穩態運行。') +
        '</div>' +
      '</div>' +
    '</div>';
}

function renderDetailed() {
  var el = document.getElementById('evolutionContent');
  var catEl = document.getElementById('evolutionCatContent');
  if (catEl) catEl.style.display = 'none';
  if (!el) return;
  var d = evolutionData;
  var scorecards = (d.agents && d.agents.weakest_agent_scorecards) || [];
  var sessions = (d.regime && d.regime.sessions) || [];
  var judges = (d.inbox && d.inbox.pending_judges) || [];
  var promotes = (d.inbox && d.inbox.pending_promotes) || [];
  var sorted = scorecards.slice().sort(function(a, b) { return (b.sharpe || 0) - (a.sharpe || 0); });

  var scoreboardHtml = '<table style="width:100%;font-size:12px;border-collapse:collapse">' +
    '<thead><tr style="border-bottom:2px solid var(--border)">' +
    '<th style="text-align:left;padding:6px">Agent</th>' +
    '<th style="text-align:left;padding:6px">技能</th>' +
    '<th style="text-align:center;padding:6px">層</th>' +
    '<th style="text-align:right;padding:6px">觀察</th>' +
    '<th style="text-align:right;padding:6px">命中率</th>' +
    '<th style="text-align:right;padding:6px">Sharpe</th>' +
    '<th style="text-align:right;padding:6px">最大回撤</th>' +
    '</tr></thead><tbody>';
  for (var i = 0; i < sorted.length; i++) {
    var a = sorted[i];
    var sharpeColor = (a.sharpe || 0) > 1 ? 'var(--up)' : ((a.sharpe || 0) < 0 ? 'var(--down)' : 'var(--warn)');
    var hitColor = (a.hit_rate || 0) > 0.5 ? 'var(--up)' : ((a.hit_rate || 0) > 0.3 ? 'var(--warn)' : 'var(--muted)');
    scoreboardHtml += '<tr style="border-bottom:1px solid var(--border)">' +
      '<td style="padding:6px">' + agentName(a.agent_id) + '</td>' +
      '<td style="padding:6px;color:var(--muted)">' + (a.skill || '-') + '</td>' +
      '<td style="padding:6px;text-align:center"><span class="badge">' + (a.layer || '-') + '</span></td>' +
      '<td style="padding:6px;text-align:right">' + (a.observations || 0) + '</td>' +
      '<td style="padding:6px;text-align:right;color:' + hitColor + '">' + ((a.hit_rate || 0) * 100).toFixed(0) + '%</td>' +
      '<td style="padding:6px;text-align:right;color:' + sharpeColor + '">' + (a.sharpe || 0).toFixed(2) + '</td>' +
      '<td style="padding:6px;text-align:right;color:var(--down)">' + ((a.max_drawdown || 0) * 100).toFixed(1) + '%</td>' +
      '</tr>';
  }
  scoreboardHtml += '</tbody></table>';

  var allExps = judges.concat(promotes);
  var expLogHtml = '';
  if (allExps.length > 0) {
    expLogHtml = '<div style="font-size:12px">';
    for (var j = 0; j < allExps.length; j++) {
      var e = allExps[j];
      var statusBadge = e.status === 'running' ? '<span style="color:var(--warn)">● 進行中</span>' :
        (e.status === 'completed' ? '<span style="color:var(--up)">✓ 完成</span>' : '<span style="color:var(--muted)">○ ' + e.status + '</span>');
      expLogHtml += '<div style="padding:6px 0;border-bottom:1px solid var(--border)">' +
        '<div><strong>' + agentName(e.target_agent_id) + '</strong> · ' + (e.mutation_type || '實驗') + '</div>' +
        '<div style="color:var(--muted);font-size:11px">' + (e.mutation_summary || '') + '</div>' +
        '<div style="display:flex;gap:16px;margin-top:4px;font-size:11px">' +
          statusBadge +
          '<span>基線: ' + ((e.baseline_value || 0)).toFixed(3) + '</span>' +
          '<span>候選: ' + ((e.candidate_value || 0)).toFixed(3) + '</span>' +
          '<span style="color:var(--muted)">' + (e.experiment_id || '') + '</span>' +
        '</div></div>';
    }
    expLogHtml += '</div>';
  } else {
    expLogHtml = '<div style="padding:20px;text-align:center;color:var(--muted)">當前無實驗記錄。系統處於穩態運行。</div>';
  }

  el.innerHTML = '<div class="panel wide" style="margin-bottom:12px;padding:10px 14px">' +
    '<div style="font-size:13px;font-weight:700;margin-bottom:8px">Regime 時間線</div>' +
    renderRegimeTimeline(sessions, 80) +
    '</div>' +
    '<div class="panel wide" style="margin-bottom:12px;padding:10px 14px">' +
    '<div style="font-size:13px;font-weight:700;margin-bottom:8px">Agent 評分表</div>' +
    scoreboardHtml +
    '</div>' +
    '<div class="panel wide" style="padding:10px 14px">' +
    '<div style="font-size:13px;font-weight:700;margin-bottom:8px">實驗日誌</div>' +
    expLogHtml +
    '</div>';
}

function renderCategorical() {
  var el = document.getElementById('evolutionContent');
  var catEl = document.getElementById('evolutionCatContent');
  if (!el || !catEl) return;
  catEl.style.display = 'block';

  // 分類視圖：上方顯示 Tab 按鈕，下方顯示對應內容
  el.innerHTML = '<div id="evolutionTabs" style="display:flex;gap:8px;margin-bottom:16px">' +
    '<button class="cat-tab active" id="catTab-agents" onclick="window._evCatTab(\'agents\')">Agent 競爭</button>' +
    '<button class="cat-tab" id="catTab-regime" onclick="window._evCatTab(\'regime\')">Regime 演化</button>' +
    '<button class="cat-tab" id="catTab-experiments" onclick="window._evCatTab(\'experiments\')">實驗日誌</button>' +
    '</div>';

  var d = evolutionData;
  var scorecards = (d.agents && d.agents.weakest_agent_scorecards) || [];
  var sessions = (d.regime && d.regime.sessions) || [];
  var judges = (d.inbox && d.inbox.pending_judges) || [];
  var promotes = (d.inbox && d.inbox.pending_promotes) || [];

  renderCatTabContent('agents', scorecards, sessions, judges, promotes);
}

function renderCatTabContent(tab, scorecards, sessions, judges, promotes) {
  var el = document.getElementById('evolutionCatContent');
  if (!el) return;

  document.querySelectorAll('#evolutionTabs .cat-tab').forEach(function(b) { b.classList.remove('active'); });
  var activeBtn = document.getElementById('catTab-' + tab);
  if (activeBtn) activeBtn.classList.add('active');

  if (tab === 'agents') {
    var sorted = scorecards.slice().sort(function(a, b) { return (b.sharpe || 0) - (a.sharpe || 0); });
    var byLayer = {};
    for (var i = 0; i < sorted.length; i++) {
      var layer = sorted[i].layer || 'unknown';
      if (!byLayer[layer]) byLayer[layer] = [];
      byLayer[layer].push(sorted[i]);
    }
    var layers = ['sector', 'style', 'superinvestor', 'context', 'control'];
    var html = '';
    for (var l = 0; l < layers.length; l++) {
      var agents = byLayer[layers[l]];
      if (!agents || agents.length === 0) continue;
      html += '<div style="margin-bottom:16px"><div style="font-size:13px;font-weight:700;margin-bottom:8px">' +
        (layers[l] === 'sector' ? '🏭 產業' : layers[l] === 'style' ? '🎨 風格' : layers[l] === 'superinvestor' ? '🧠 超級投資者' : layers[l] === 'context' ? '🌍 宏觀' : '⚙️ 控制') +
        ' (' + agents.length + ')</div>';
      for (var j = 0; j < agents.length; j++) {
        var a = agents[j];
        var barLen = Math.max(5, Math.min(100, (a.hit_rate || 0) * 100));
        var barColor = barLen > 60 ? 'var(--up)' : (barLen > 30 ? 'var(--warn)' : 'var(--muted)');
        html += '<div style="display:flex;align-items:center;gap:8px;padding:3px 0">' +
          '<span style="font-size:11px;flex:1">' + agentName(a.agent_id) + '</span>' +
          '<span style="font-size:10px;color:var(--muted);width:40px">命中</span>' +
          '<div style="width:100px;height:5px;background:var(--bg);border-radius:3px">' +
            '<div style="width:' + barLen + '%;height:100%;background:' + barColor + ';border-radius:3px"></div>' +
          '</div>' +
          '<span style="font-size:10px;color:var(--muted);width:30px;text-align:right">' + ((a.hit_rate || 0) * 100).toFixed(0) + '%</span>' +
          '<span style="font-size:10px;color:var(--muted);width:55px;text-align:right">S:' + (a.sharpe || 0).toFixed(2) + '</span>' +
          '</div>';
      }
      html += '</div>';
    }
    el.innerHTML = html || '<div style="padding:20px;color:var(--muted);text-align:center">無 Agent 數據</div>';
  }
  else if (tab === 'regime') {
    el.innerHTML = '<div style="font-size:13px;font-weight:700;margin-bottom:8px">Regime 演化時間線</div>' +
      '<div style="font-size:11px;color:var(--muted);margin-bottom:12px">共 ' + sessions.length + ' 個 session</div>' +
      renderRegimeTimeline(sessions, -1);
  }
  else if (tab === 'experiments') {
    var all = judges.concat(promotes);
    var html = '';
    if (all.length > 0) {
      for (var i = 0; i < all.length; i++) {
        var e = all[i];
        var isJudge = judges.indexOf(e) >= 0;
        html += '<div style="padding:8px 0;border-bottom:1px solid var(--border)">' +
          '<div style="display:flex;justify-content:space-between;align-items:center">' +
            '<div><strong>' + agentName(e.target_agent_id) + '</strong> · ' + (e.mutation_type || '') + '</div>' +
            '<span style="font-size:11px;color:' + (isJudge ? 'var(--warn)' : 'var(--up)') + '">' + (isJudge ? '待評判' : '待晉升') + '</span>' +
          '</div>' +
          '<div style="font-size:11px;color:var(--muted);margin-top:2px">' + (e.mutation_summary || '') + '</div>' +
          '<div style="display:flex;gap:16px;margin-top:2px;font-size:11px">' +
            '<span>基線: ' + (e.baseline_value || 0).toFixed(3) + '</span>' +
            '<span>候選: ' + (e.candidate_value || 0).toFixed(3) + '</span>' +
          '</div></div>';
      }
    } else {
      html = '<div style="padding:20px;color:var(--muted);text-align:center">無實驗記錄</div>';
    }
    el.innerHTML = html;
  }
}

function renderRegimeTimeline(sessions, maxBars) {
  if (!sessions || !sessions.length) return '<div style="padding:8px;color:var(--muted);font-size:11px">無 regime 歷史數據</div>';

  var display = sessions;
  if (maxBars > 0 && sessions.length > maxBars) {
    var step = Math.floor(sessions.length / maxBars);
    display = [];
    for (var i = 0; i < sessions.length; i += step) {
      display.push(sessions[i]);
      if (display.length >= maxBars) break;
    }
  }

  var colors = { RISK_ON: 'var(--up)', RISK_OFF: 'var(--down)', NEUTRAL: 'var(--warn)' };
  var segments = [];
  var currentRegime = display[0].regime;
  var segmentStart = 0;
  for (var i = 1; i < display.length; i++) {
    if (display[i].regime !== currentRegime) {
      segments.push({ regime: currentRegime, count: i - segmentStart, start: segmentStart });
      currentRegime = display[i].regime;
      segmentStart = i;
    }
  }
  segments.push({ regime: currentRegime, count: display.length - segmentStart, start: segmentStart });

  var total = display.length;
  var html = '<div style="display:flex;width:100%;height:24px;border-radius:4px;overflow:hidden;margin-bottom:6px">';
  for (var s = 0; s < segments.length; s++) {
    var seg = segments[s];
    var w = (seg.count / total * 100).toFixed(2);
    var color = colors[seg.regime] || 'var(--muted)';
    html += '<div title="' + regimeLabel(seg.regime) + ': ' + seg.count + ' sessions" ' +
      'style="width:' + w + '%;height:100%;background:' + color + ';transition:all 0.2s;position:relative">' +
      (seg.count > 3 ? '<span style="position:absolute;left:4px;top:50%;transform:translateY(-50%);font-size:9px;font-weight:600;color:var(--bg);white-space:nowrap">' + regimeLabel(seg.regime) + '</span>' : '') +
      '</div>';
  }
  html += '</div>';

  html += '<div style="display:flex;gap:16px;font-size:10px;color:var(--muted)">' +
    '<span style="color:var(--up)">🟢 多頭</span>' +
    '<span style="color:var(--down)">🔴 空頭</span>' +
    '<span style="color:var(--warn)">🟡 盤整</span>' +
    '<span style="margin-left:auto">' + display[0].session_id.split('-')[1] + ' → ' + display[display.length - 1].session_id.split('-')[1] + '</span>' +
    '</div>';

  return html;
}

// 視圖切換入口（從 HTML onclick 調用）
window._evSwitch = switchView;

// 分類 Tab 切換
window._evCatTab = function(tab) {
  var d = evolutionData;
  var scorecards = (d.agents && d.agents.weakest_agent_scorecards) || [];
  var sessions = (d.regime && d.regime.sessions) || [];
  var judges = (d.inbox && d.inbox.pending_judges) || [];
  var promotes = (d.inbox && d.inbox.pending_promotes) || [];
  renderCatTabContent(tab, scorecards, sessions, judges, promotes);
};
