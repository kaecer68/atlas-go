// Evolution Transparency Panel — Agent competition, regime timeline, experiment log
import { agentName, sectorName, regimeLabel } from '../names.js';
import { getJSON, formatDate, notify } from '../shared/app-utils.js';
import { renderEquityCurve, renderAgentScoreboard, renderRegimeContext, renderAllocationGuidance } from '../components/sparkline.js';

let evolutionData = null;
let currentView = 'compact';

export async function loadEvolutionData() {
  const [agents, regime, inbox] = await Promise.all([
    getJSON('/api/dashboard/agent-observatory').catch(() => null),
    getJSON('/api/dashboard/regime-history').catch(() => null),
    getJSON('/api/dashboard/experiment-inbox').catch(() => null),
  ]);
  evolutionData = { agents, regime, inbox };
  switchView(currentView);
}

export function switchView(mode) {
  currentView = mode || currentView;
  document.querySelectorAll('#evolutionViews .view-btn').forEach(b => b.classList.remove('active'));
  var btn = document.getElementById('evView-' + currentView);
  if (btn) btn.classList.add('active');

  if (currentView === 'compact') renderCompact();
  else if (currentView === 'detailed') renderDetailed();
  else if (currentView === 'categorical') renderCategorical();
}

function renderCompact() {
  var el = document.getElementById('evolutionContent');
  if (!el) return;
  var d = evolutionData;

  var scorecards = (d.agents && d.agents.weakest_agent_scorecards) || [];
  var sessions = (d.regime && d.regime.sessions) || [];
  var judges = (d.inbox && d.inbox.pending_judges) || [];
  var promotes = (d.inbox && d.inbox.pending_promotes) || [];

  // Sort scorecards by sharpe descending
  var sorted = scorecards.slice().sort(function(a, b) { return (b.sharpe || 0) - (a.sharpe || 0); });

  // Regime mini timeline
  var regimeHtml = renderRegimeTimeline(sessions, 40);

  // Top agents summary
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

  // Experiment activity summary
  var expCount = judges.length + promotes.length;
  var expHtml = '<div style="display:flex;gap:24px;margin-top:4px">' +
    '<div style="text-align:center"><div style="font-size:20px;font-weight:700;color:' + (judges.length > 0 ? 'var(--warn)' : 'var(--muted)') + '">' + judges.length + '</div><div style="font-size:11px;color:var(--muted)">待评判</div></div>' +
    '<div style="text-align:center"><div style="font-size:20px;font-weight:700;color:' + (promotes.length > 0 ? 'var(--up)' : 'var(--muted)') + '">' + promotes.length + '</div><div style="font-size:11px;color:var(--muted)">待晋升</div></div>' +
    '<div style="text-align:center"><div style="font-size:20px;font-weight:700">' + scorecards.length + '</div><div style="font-size:11px;color:var(--muted)">活跃 Agent</div></div>' +
    '</div>';

  el.innerHTML = '<div class="panel wide" style="margin-bottom:12px;padding:10px 14px">' +
    '<div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px">' +
      '<div style="font-size:13px;font-weight:700">Regime 演化</div>' +
      '<div style="font-size:11px;color:var(--muted)">' + sessions.length + ' 个 session</div>' +
    '</div>' +
    regimeHtml +
    '</div>' +
    '<div style="display:grid;grid-template-columns:1fr 1fr;gap:12px">' +
      '<div class="panel" style="padding:10px 14px">' +
        '<div style="font-size:13px;font-weight:700;margin-bottom:8px">Agent 表现 Top 5</div>' +
        agentSummaryHtml +
      '</div>' +
      '<div class="panel" style="padding:10px 14px">' +
        '<div style="font-size:13px;font-weight:700;margin-bottom:8px">实验活动</div>' +
        expHtml +
        '<div style="margin-top:12px;font-size:11px;color:var(--muted)">' +
          (judges.length + promotes.length > 0
            ? '当前有 ' + expCount + ' 个实验排队。系统正在持续进化 Agent 策略。'
            : '当前无待处理实验。系统处于稳态运行。') +
        '</div>' +
      '</div>' +
    '</div>';
}

function renderDetailed() {
  var el = document.getElementById('evolutionContent');
  if (!el) return;
  var d = evolutionData;
  var scorecards = (d.agents && d.agents.weakest_agent_scorecards) || [];
  var sessions = (d.regime && d.regime.sessions) || [];
  var judges = (d.inbox && d.inbox.pending_judges) || [];
  var promotes = (d.inbox && d.inbox.pending_promotes) || [];

  var sorted = scorecards.slice().sort(function(a, b) { return (b.sharpe || 0) - (a.sharpe || 0); });

  // Full agent scoreboard
  var scoreboardHtml = '<table style="width:100%;font-size:12px;border-collapse:collapse">' +
    '<thead><tr style="border-bottom:2px solid var(--border)">' +
    '<th style="text-align:left;padding:6px">Agent</th>' +
    '<th style="text-align:left;padding:6px">技能</th>' +
    '<th style="text-align:center;padding:6px">层</th>' +
    '<th style="text-align:right;padding:6px">观察</th>' +
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

  // Experiment log
  var allExps = judges.concat(promotes);
  var expLogHtml = '';
  if (allExps.length > 0) {
    expLogHtml = '<div style="font-size:12px">';
    for (var j = 0; j < allExps.length; j++) {
      var e = allExps[j];
      var statusBadge = e.status === 'running' ? '<span style="color:var(--warn)">● 进行中</span>' :
        (e.status === 'completed' ? '<span style="color:var(--up)">✓ 完成</span>' : '<span style="color:var(--muted)">○ ' + e.status + '</span>');
      expLogHtml += '<div style="padding:6px 0;border-bottom:1px solid var(--border)">' +
        '<div><strong>' + agentName(e.target_agent_id) + '</strong> · ' + (e.mutation_type || '实验') + '</div>' +
        '<div style="color:var(--muted);font-size:11px">' + (e.mutation_summary || '') + '</div>' +
        '<div style="display:flex;gap:16px;margin-top:4px;font-size:11px">' +
          statusBadge +
          '<span>基线: ' + ((e.baseline_value || 0)).toFixed(3) + '</span>' +
          '<span>候选: ' + ((e.candidate_value || 0)).toFixed(3) + '</span>' +
          '<span style="color:var(--muted)">' + (e.experiment_id || '') + '</span>' +
        '</div></div>';
    }
    expLogHtml += '</div>';
  } else {
    expLogHtml = '<div style="padding:20px;text-align:center;color:var(--muted)">当前无实验记录。系统处于稳态运行。</div>';
  }

  el.innerHTML = '<div class="panel wide" style="margin-bottom:12px;padding:10px 14px">' +
    '<div style="font-size:13px;font-weight:700;margin-bottom:8px">Regime 时间线</div>' +
    renderRegimeTimeline(sessions, 80) +
    '</div>' +
    '<div class="panel wide" style="margin-bottom:12px;padding:10px 14px">' +
    '<div style="font-size:13px;font-weight:700;margin-bottom:8px">Agent 评分表</div>' +
    scoreboardHtml +
    '</div>' +
    '<div class="panel wide" style="padding:10px 14px">' +
    '<div style="font-size:13px;font-weight:700;margin-bottom:8px">实验日志</div>' +
    expLogHtml +
    '</div>';
}

function renderCategorical() {
  var el = document.getElementById('evolutionContent');
  if (!el) return;
  var d = evolutionData;
  var scorecards = (d.agents && d.agents.weakest_agent_scorecards) || [];
  var sessions = (d.regime && d.regime.sessions) || [];
  var judges = (d.inbox && d.inbox.pending_judges) || [];
  var promotes = (d.inbox && d.inbox.pending_promotes) || [];

  // Show the currently active tab
  renderCategoricalTab('agents', scorecards, sessions, judges, promotes);
}

export function renderCategoricalTab(tab, scorecards, sessions, judges, promotes) {
  scorecards = scorecards || ((evolutionData && evolutionData.agents && evolutionData.agents.weakest_agent_scorecards) || []);
  sessions = sessions || ((evolutionData && evolutionData.regime && evolutionData.regime.sessions) || []);
  judges = judges || ((evolutionData && evolutionData.inbox && evolutionData.inbox.pending_judges) || []);
  promotes = promotes || ((evolutionData && evolutionData.inbox && evolutionData.inbox.pending_promotes) || []);

  var el = document.getElementById('evolutionCatContent');
  if (!el) return;

  // Update tab buttons
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
        (layers[l] === 'sector' ? '🏭 产业' : layers[l] === 'style' ? '🎨 风格' : layers[l] === 'superinvestor' ? '🧠 超级投资者' : layers[l] === 'context' ? '🌍 宏观' : '⚙️ 控制') +
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
    el.innerHTML = html || '<div style="padding:20px;color:var(--muted);text-align:center">无 Agent 数据</div>';
  }
  else if (tab === 'regime') {
    el.innerHTML = '<div style="font-size:13px;font-weight:700;margin-bottom:8px">Regime 演化时间线</div>' +
      '<div style="font-size:11px;color:var(--muted);margin-bottom:12px">共 ' + sessions.length + ' 个 session</div>' +
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
            '<span style="font-size:11px;color:' + (isJudge ? 'var(--warn)' : 'var(--up)') + '">' + (isJudge ? '待评判' : '待晋升') + '</span>' +
          '</div>' +
          '<div style="font-size:11px;color:var(--muted);margin-top:2px">' + (e.mutation_summary || '') + '</div>' +
          '<div style="display:flex;gap:16px;margin-top:2px;font-size:11px">' +
            '<span>基线: ' + (e.baseline_value || 0).toFixed(3) + '</span>' +
            '<span>候选: ' + (e.candidate_value || 0).toFixed(3) + '</span>' +
          '</div></div>';
      }
    } else {
      html = '<div style="padding:20px;color:var(--muted);text-align:center">无实验记录</div>';
    }
    el.innerHTML = html;
  }
}

function renderRegimeTimeline(sessions, maxBars) {
  if (!sessions || !sessions.length) return '<div style="padding:8px;color:var(--muted);font-size:11px">无 regime 历史数据</div>';

  var display = sessions;
  if (maxBars > 0 && sessions.length > maxBars) {
    // Sample evenly
    var step = Math.floor(sessions.length / maxBars);
    display = [];
    for (var i = 0; i < sessions.length; i += step) {
      display.push(sessions[i]);
      if (display.length >= maxBars) break;
    }
  }

  var colors = { RISK_ON: 'var(--up)', RISK_OFF: 'var(--down)', NEUTRAL: 'var(--warn)' };

  // Find regime change points
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
  var html = '<div style="display:flex;width:100%;height:22px;border-radius:4px;overflow:hidden;margin-bottom:4px">';
  for (var s = 0; s < segments.length; s++) {
    var seg = segments[s];
    var w = (seg.count / total * 100).toFixed(2);
    html += '<div title="' + regimeLabel(seg.regime) + ': ' + seg.count + ' sessions" style="width:' + w + '%;height:100%;background:' + (colors[seg.regime] || 'var(--muted)') + '"></div>';
  }
  html += '</div>';

  // Legend
  html += '<div style="display:flex;gap:16px;font-size:10px;color:var(--muted)">' +
    '<span>🟢 RISK_ON</span><span>🔴 RISK_OFF</span><span>🟡 NEUTRAL</span>' +
    '<span style="margin-left:auto">' + display[0].session_id.split('-')[1] + ' → ' + display[display.length - 1].session_id.split('-')[1] + '</span>' +
    '</div>';

  return html;
}

window._evSwitch = function(mode) {
  if (mode === 'categorical') {
    document.getElementById('evolutionContent').style.display = 'block';
    document.getElementById('evolutionCatContent').style.display = 'block';
    // Show tab buttons
    var tabsHtml = '<div id="evolutionTabs" style="display:flex;gap:8px;margin-bottom:12px">' +
      '<button class="cat-tab active" id="catTab-agents" onclick="window._evCatTab(\'agents\')">Agent 竞争</button>' +
      '<button class="cat-tab" id="catTab-regime" onclick="window._evCatTab(\'regime\')">Regime 演化</button>' +
      '<button class="cat-tab" id="catTab-experiments" onclick="window._evCatTab(\'experiments\')">实验日志</button>' +
      '</div>';
    document.getElementById('evolutionContent').innerHTML = tabsHtml;
    renderCategoricalTab('agents');
  } else {
    document.getElementById('evolutionCatContent').style.display = 'none';
    switchView(mode);
  }
};

window._evCatTab = function(tab) { renderCategoricalTab(tab); };
