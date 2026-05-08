import { agentName, sectorName, regimeLabel } from '../names.js';
import { getJSON } from '../shared/app-utils.js';

let evolutionData = null;
let currentView = 'compact';

export async function loadEvolutionData() {
  const [agents, regime, inbox] = await Promise.all([
    getJSON('/api/dashboard/agent-observatory').catch(() => null),
    getJSON('/api/dashboard/regime-history').catch(() => null),
    getJSON('/api/dashboard/experiment-inbox').catch(() => null),
  ]);
  evolutionData = { agents, regime, inbox };
  const el = document.getElementById('evolutionContent');
  if (el) el.classList.remove('loading', 'empty');
  switchView(currentView);
}

export function switchView(mode) {
  currentView = mode || currentView;
  document.querySelectorAll('#evolutionViews .view-btn').forEach(b => b.classList.remove('active'));
  const btn = document.getElementById('evView-' + currentView);
  if (btn) btn.classList.add('active');
  const catEl = document.getElementById('evolutionCatContent');
  if (catEl) catEl.style.display = 'none';

  if (currentView === 'compact') renderCompact();
  else if (currentView === 'detailed') renderDetailed();
  else if (currentView === 'categorical') renderCategorical();
}

function getData() {
  const d = evolutionData || {};
  return {
    scorecards: (d.agents && d.agents.weakest_agent_scorecards) || [],
    sessions: (d.regime && d.regime.sessions) || [],
    judges: (d.inbox && d.inbox.pending_judges) || [],
    promotes: (d.inbox && d.inbox.pending_promotes) || [],
  };
}

// ====== 總覽儀表板 ======
function renderCompact() {
  const el = document.getElementById('evolutionContent');
  if (!el) return;
  const { scorecards, sessions, judges, promotes } = getData();
  const sorted = scorecards.slice().sort((a, b) => (b.sharpe || 0) - (a.sharpe || 0));
  const top5 = sorted.slice(0, 5);
  const expCount = judges.length + promotes.length;

  const agentRows = top5.map((a, i) => {
    const barW = Math.max(5, Math.min(100, (a.hit_rate || 0) * 100));
    const sColor = (a.sharpe || 0) > 1 ? 'var(--up)' : ((a.sharpe || 0) < 0 ? 'var(--down)' : 'var(--warn)');
    return '<div style="display:flex;align-items:center;gap:8px;padding:4px 0;border-bottom:1px solid var(--border)">' +
      '<span style="font-size:11px;color:var(--muted);width:18px">' + (i + 1) + '</span>' +
      '<span style="flex:1;font-size:12px">' + agentName(a.agent_id) + '</span>' +
      '<div style="width:80px;height:4px;background:var(--bg);border-radius:2px">' +
        '<div style="width:' + barW + '%;height:100%;background:' + (barW > 50 ? 'var(--up)' : 'var(--warn)') + ';border-radius:2px"></div>' +
      '</div>' +
      '<span style="font-size:11px;color:var(--muted);width:35px;text-align:right">' + ((a.hit_rate || 0) * 100).toFixed(0) + '%</span>' +
      '<span style="font-size:11px;color:' + sColor + ';width:45px;text-align:right">S:' + (a.sharpe || 0).toFixed(2) + '</span>' +
      '</div>';
  }).join('');

  el.innerHTML =
    '<div class="panel wide" style="margin-bottom:12px;padding:10px 14px">' +
      '<div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px">' +
        '<div style="font-size:13px;font-weight:700">Regime 演化</div>' +
        '<div style="font-size:11px;color:var(--muted)">' + sessions.length + ' 個 session</div>' +
      '</div>' +
      renderRegimeTimeline(sessions, 60) +
    '</div>' +
    '<div style="display:grid;grid-template-columns:1fr 1fr;gap:12px">' +
      '<div class="panel" style="padding:10px 14px">' +
        '<div style="font-size:13px;font-weight:700;margin-bottom:8px">Agent 表現 Top 5</div>' +
        agentRows +
      '</div>' +
      '<div class="panel" style="padding:10px 14px">' +
        '<div style="font-size:13px;font-weight:700;margin-bottom:8px">實驗活動</div>' +
        '<div style="display:flex;gap:24px;margin-top:4px">' +
          '<div style="text-align:center"><div style="font-size:20px;font-weight:700;color:' + (judges.length > 0 ? 'var(--warn)' : 'var(--muted)') + '">' + judges.length + '</div><div style="font-size:11px;color:var(--muted)">待評判</div></div>' +
          '<div style="text-align:center"><div style="font-size:20px;font-weight:700;color:' + (promotes.length > 0 ? 'var(--up)' : 'var(--muted)') + '">' + promotes.length + '</div><div style="font-size:11px;color:var(--muted)">待晉升</div></div>' +
          '<div style="text-align:center"><div style="font-size:20px;font-weight:700">' + scorecards.length + '</div><div style="font-size:11px;color:var(--muted)">活躍 Agent</div></div>' +
        '</div>' +
        '<div style="margin-top:12px;font-size:11px;color:var(--muted)">' +
          (expCount > 0 ? '當前有 ' + expCount + ' 個實驗排隊。系統正在持續進化 Agent 策略。' : '當前無待處理實驗。系統處於穩態運行。') +
        '</div>' +
      '</div>' +
    '</div>';
}

// ====== 詳細分析 ======
function renderDetailed() {
  const el = document.getElementById('evolutionContent');
  if (!el) return;
  const { scorecards, sessions, judges, promotes } = getData();
  const sorted = scorecards.slice().sort((a, b) => (b.sharpe || 0) - (a.sharpe || 0));
  const allExps = judges.concat(promotes);

  let scoreboardHtml = '<table style="width:100%;font-size:12px;border-collapse:collapse">' +
    '<thead><tr style="border-bottom:2px solid var(--border)">' +
    '<th style="text-align:left;padding:6px">Agent</th><th style="text-align:left;padding:6px">技能</th>' +
    '<th style="text-align:center;padding:6px">層</th><th style="text-align:right;padding:6px">觀察</th>' +
    '<th style="text-align:right;padding:6px">命中率</th><th style="text-align:right;padding:6px">Sharpe</th>' +
    '<th style="text-align:right;padding:6px">最大回撤</th></tr></thead><tbody>';
  for (let i = 0; i < sorted.length; i++) {
    const a = sorted[i];
    const sColor = (a.sharpe || 0) > 1 ? 'var(--up)' : ((a.sharpe || 0) < 0 ? 'var(--down)' : 'var(--warn)');
    const hColor = (a.hit_rate || 0) > 0.5 ? 'var(--up)' : ((a.hit_rate || 0) > 0.3 ? 'var(--warn)' : 'var(--muted)');
    scoreboardHtml += '<tr style="border-bottom:1px solid var(--border)">' +
      '<td style="padding:6px">' + agentName(a.agent_id) + '</td>' +
      '<td style="padding:6px;color:var(--muted)">' + (a.skill || '-') + '</td>' +
      '<td style="padding:6px;text-align:center"><span class="badge">' + (a.layer || '-') + '</span></td>' +
      '<td style="padding:6px;text-align:right">' + (a.observations || 0) + '</td>' +
      '<td style="padding:6px;text-align:right;color:' + hColor + '">' + ((a.hit_rate || 0) * 100).toFixed(0) + '%</td>' +
      '<td style="padding:6px;text-align:right;color:' + sColor + '">' + (a.sharpe || 0).toFixed(2) + '</td>' +
      '<td style="padding:6px;text-align:right;color:var(--down)">' + ((a.max_drawdown || 0) * 100).toFixed(1) + '%</td></tr>';
  }
  scoreboardHtml += '</tbody></table>';

  let expLogHtml = '';
  if (allExps.length > 0) {
    expLogHtml = '<div style="font-size:12px">';
    for (let j = 0; j < allExps.length; j++) {
      const e = allExps[j];
      const badge = e.status === 'running' ? '<span style="color:var(--warn)">● 進行中</span>' :
        (e.status === 'completed' ? '<span style="color:var(--up)">✓ 完成</span>' : '<span style="color:var(--muted)">○ ' + e.status + '</span>');
      expLogHtml += '<div style="padding:6px 0;border-bottom:1px solid var(--border)">' +
        '<div><strong>' + agentName(e.target_agent_id) + '</strong> · ' + (e.mutation_type || '實驗') + '</div>' +
        '<div style="color:var(--muted);font-size:11px">' + (e.mutation_summary || '') + '</div>' +
        '<div style="display:flex;gap:16px;margin-top:4px;font-size:11px">' + badge +
          '<span>基線: ' + ((e.baseline_value || 0)).toFixed(3) + '</span>' +
          '<span>候選: ' + ((e.candidate_value || 0)).toFixed(3) + '</span>' +
          '<span style="color:var(--muted)">' + (e.experiment_id || '') + '</span></div></div>';
    }
    expLogHtml += '</div>';
  } else {
    expLogHtml = '<div style="padding:20px;text-align:center;color:var(--muted)">當前無實驗記錄。系統處於穩態運行。</div>';
  }

  el.innerHTML =
    '<div class="panel wide" style="margin-bottom:12px;padding:10px 14px">' +
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

// ====== 分類視覺圖 ======
function renderCategorical() {
  const el = document.getElementById('evolutionContent');
  const catEl = document.getElementById('evolutionCatContent');
  if (!el || !catEl) return;
  catEl.style.display = 'block';

  el.innerHTML = '<div id="evolutionTabs" style="display:flex;gap:8px;margin-bottom:16px">' +
    '<button class="cat-tab active" id="catTab-agents" onclick="window._evCatTab(\'agents\')">Agent 競爭</button>' +
    '<button class="cat-tab" id="catTab-regime" onclick="window._evCatTab(\'regime\')">Regime 演化</button>' +
    '<button class="cat-tab" id="catTab-experiments" onclick="window._evCatTab(\'experiments\')">實驗日誌</button>' +
    '</div>';

  const { scorecards, sessions, judges, promotes } = getData();
  renderCatContent('agents', scorecards, sessions, judges, promotes);
}

function renderCatContent(tab, scorecards, sessions, judges, promotes) {
  const el = document.getElementById('evolutionCatContent');
  if (!el) return;

  document.querySelectorAll('#evolutionTabs .cat-tab').forEach(b => b.classList.remove('active'));
  const btn = document.getElementById('catTab-' + tab);
  if (btn) btn.classList.add('active');

  if (tab === 'agents') {
    const sorted = scorecards.slice().sort((a, b) => (b.sharpe || 0) - (a.sharpe || 0));
    const byLayer = {};
    for (const a of sorted) {
      const layer = a.layer || 'unknown';
      if (!byLayer[layer]) byLayer[layer] = [];
      byLayer[layer].push(a);
    }
    const layers = ['sector', 'style', 'superinvestor', 'context', 'control'];
    const layerLabels = { sector: '🏭 產業', style: '🎨 風格', superinvestor: '🧠 超級投資者', context: '🌍 宏觀', control: '⚙️ 控制' };
    let html = '';
    for (const l of layers) {
      const agents = byLayer[l];
      if (!agents || !agents.length) continue;
      html += '<div style="margin-bottom:16px"><div style="font-size:13px;font-weight:700;margin-bottom:8px">' +
        (layerLabels[l] || l) + ' (' + agents.length + ')</div>';
      for (const a of agents) {
        const barLen = Math.max(5, Math.min(100, (a.hit_rate || 0) * 100));
        const barColor = barLen > 60 ? 'var(--up)' : (barLen > 30 ? 'var(--warn)' : 'var(--muted)');
        html += '<div style="display:flex;align-items:center;gap:8px;padding:3px 0">' +
          '<span style="font-size:11px;flex:1">' + agentName(a.agent_id) + '</span>' +
          '<span style="font-size:10px;color:var(--muted);width:40px">命中</span>' +
          '<div style="width:100px;height:5px;background:var(--bg);border-radius:3px">' +
            '<div style="width:' + barLen + '%;height:100%;background:' + barColor + ';border-radius:3px"></div></div>' +
          '<span style="font-size:10px;color:var(--muted);width:30px;text-align:right">' + ((a.hit_rate || 0) * 100).toFixed(0) + '%</span>' +
          '<span style="font-size:10px;color:var(--muted);width:55px;text-align:right">S:' + (a.sharpe || 0).toFixed(2) + '</span></div>';
      }
      html += '</div>';
    }
    el.innerHTML = html || '<div style="padding:20px;color:var(--muted);text-align:center">無 Agent 數據</div>';
  } else if (tab === 'regime') {
    el.innerHTML = '<div style="font-size:13px;font-weight:700;margin-bottom:8px">Regime 演化時間線</div>' +
      '<div style="font-size:11px;color:var(--muted);margin-bottom:12px">共 ' + sessions.length + ' 個 session</div>' +
      renderRegimeTimeline(sessions, -1);
  } else if (tab === 'experiments') {
    const all = judges.concat(promotes);
    let html = '';
    if (all.length > 0) {
      for (const e of all) {
        const isJudge = judges.includes(e);
        html += '<div style="padding:8px 0;border-bottom:1px solid var(--border)">' +
          '<div style="display:flex;justify-content:space-between;align-items:center">' +
            '<div><strong>' + agentName(e.target_agent_id) + '</strong> · ' + (e.mutation_type || '') + '</div>' +
            '<span style="font-size:11px;color:' + (isJudge ? 'var(--warn)' : 'var(--up)') + '">' + (isJudge ? '待評判' : '待晉升') + '</span></div>' +
          '<div style="font-size:11px;color:var(--muted);margin-top:2px">' + (e.mutation_summary || '') + '</div>' +
          '<div style="display:flex;gap:16px;margin-top:2px;font-size:11px">' +
            '<span>基線: ' + (e.baseline_value || 0).toFixed(3) + '</span>' +
            '<span>候選: ' + (e.candidate_value || 0).toFixed(3) + '</span></div></div>';
      }
    } else {
      html = '<div style="padding:20px;color:var(--muted);text-align:center">無實驗記錄</div>';
    }
    el.innerHTML = html;
  }
}

// ====== Regime 時間線 ======
function renderRegimeTimeline(sessions, maxBars) {
  if (!sessions || !sessions.length) {
    return '<div style="padding:8px;color:var(--muted);font-size:11px">無 regime 歷史數據</div>';
  }
  let display = sessions;
  if (maxBars > 0 && sessions.length > maxBars) {
    const step = Math.floor(sessions.length / maxBars);
    display = [];
    for (let i = 0; i < sessions.length; i += step) {
      display.push(sessions[i]);
      if (display.length >= maxBars) break;
    }
  }
  const colors = { RISK_ON: 'var(--up)', RISK_OFF: 'var(--down)', NEUTRAL: 'var(--warn)' };
  const segments = [];
  let cur = display[0].regime, start = 0;
  for (let i = 1; i < display.length; i++) {
    if (display[i].regime !== cur) {
      segments.push({ regime: cur, count: i - start });
      cur = display[i].regime; start = i;
    }
  }
  segments.push({ regime: cur, count: display.length - start });

  const total = display.length;
  let html = '<div style="display:flex;width:100%;height:24px;border-radius:4px;overflow:hidden;margin-bottom:6px">';
  for (const s of segments) {
    const w = (s.count / total * 100).toFixed(2);
    const color = colors[s.regime] || 'var(--muted)';
    const label = s.count > 3 ? '<span style="position:absolute;left:4px;top:50%;transform:translateY(-50%);font-size:9px;font-weight:600;color:var(--bg);white-space:nowrap">' + regimeLabel(s.regime) + '</span>' : '';
    html += '<div title="' + regimeLabel(s.regime) + ': ' + s.count + ' sessions" ' +
      'style="width:' + w + '%;height:100%;background:' + color + ';position:relative">' + label + '</div>';
  }
  html += '</div>';
  html += '<div style="display:flex;gap:16px;font-size:10px;color:var(--muted)">' +
    '<span style="color:var(--up)">🟢 多頭</span>' +
    '<span style="color:var(--down)">🔴 空頭</span>' +
    '<span style="color:var(--warn)">🟡 盤整</span>' +
    '<span style="margin-left:auto">' + display[0].session_id.split('-')[1] + ' → ' + display[display.length - 1].session_id.split('-')[1] + '</span></div>';
  return html;
}

// ====== 全域視圖切換（從 HTML onclick 調用）======
window._evSwitch = function(mode) { switchView(mode); };
window._evCatTab = function(tab) {
  const { scorecards, sessions, judges, promotes } = getData();
  renderCatContent(tab, scorecards, sessions, judges, promotes);
};
