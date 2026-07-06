// 演化透視面板 — Agent 競爭、Regime 時間線、實驗記錄
import { agentName, sectorName, regimeLabel } from '../names.js';

import { silentGetJSON, formatDate, notify } from '../shared/app-utils.js';
import { getThemeColor } from '../shared/utils.js';
import { hexToRgba } from '../shared/color-tokens.js';


import { renderEquityCurve, renderComparisonChart, renderRadarChart, renderRegimeVolumeChart } from '../components/sparkline.js';

let evolutionData = null;
let currentView = 'compact';
let loaded = false;

export async function loadEvolutionData() {
  const [agents, regime, inbox] = await Promise.all([
    silentGetJSON('/api/dashboard/agent-observatory'),
    silentGetJSON('/api/dashboard/regime-history'),
    silentGetJSON('/api/dashboard/experiment-inbox'),
  ]);
  evolutionData = { agents, regime, inbox };
  let el = document.getElementById('evolutionContent');
  if (el) el.classList.remove('loading', 'empty');
  // renderEquityFromData();
  renderCurrentView();
}

function renderCurrentView() {
  document.querySelectorAll('#evolutionViews .view-btn').forEach(function(b) { b.classList.remove('active'); });
  let btn = document.getElementById('evView-' + currentView);
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
  let d = evolutionData;
  let scorecards = (d.agents && d.agents.scorecards) || [];
  if (scorecards.length < 2) return;

  let sorted = scorecards.slice().sort(function(a, b) { return (a.last_updated_at || '').localeCompare(b.last_updated_at || ''); });
  let points = sorted.map(function(a, i) {
    return {
      value: a.average_return || 0,
      label: (i + 1).toString()
    };
  });
  renderEquityCurve(points);
}

// 用 Canvas 繪製內嵌權益曲線（不依賴外部 DOM 元素）
function renderInlineEquityCurve(containerId, points) {
  let container = document.getElementById(containerId);
  if (!container || !points || points.length < 2) return;
  let canvas = document.createElement('canvas');
  canvas.style.width = '100%'; canvas.style.height = '200px';
  container.appendChild(canvas);

  let tooltip = document.createElement('div');
  tooltip.className = 'chart-tooltip';
  tooltip.style.cssText = 'position:absolute;pointer-events:none;background:var(--panel-l2,#1a2030);border:1px solid var(--border,#333);border-radius:6px;padding:6px 10px;font-size:11px;color:var(--text,#f0f4f8);z-index:10;opacity:0;transition:opacity 0.15s;white-space:nowrap;';
  container.style.position = 'relative';
  container.appendChild(tooltip);

  let ctx = canvas.getContext('2d');
  let dpr = window.devicePixelRatio || 1;
  let rect = container.getBoundingClientRect();
  let W = rect.width - 20, H = 200;
  canvas.width = W * dpr; canvas.height = H * dpr;
  ctx.scale(dpr, dpr);
  let pad = {top: 10, right: 15, bottom: 25, left: 45};
  let chartW = W - pad.left - pad.right, chartH = H - pad.top - pad.bottom;

  let values = points.map(function(p) { return p.value; });
  let minV = Math.min.apply(null, values), maxV = Math.max.apply(null, values), range = maxV - minV || 1;
  ctx.clearRect(0, 0, W, H);

  // Background
  const panelBg = getThemeColor('--panel-l2') || getThemeColor('--panel') || '#0d1015';
  ctx.fillStyle = panelBg;
  ctx.beginPath(); ctx.roundRect(pad.left, pad.top, chartW, chartH, 6); ctx.fill();

  // Grid
  ctx.strokeStyle = hexToRgba(getThemeColor('--text') || '#f0f4f8', 0.04); ctx.lineWidth = 0.5;
  for (let i = 1; i <= 3; i++) {
    let gy = pad.top + (chartH / 4) * i;
    ctx.beginPath(); ctx.moveTo(pad.left, gy); ctx.lineTo(pad.left + chartW, gy); ctx.stroke();
  }

  // Y-axis
  ctx.fillStyle = hexToRgba(getThemeColor('--muted') || '#b8c4d0', 0.5); ctx.font = '9px system-ui'; ctx.textAlign = 'right';
  for (let i = 0; i <= 4; i++) {
    let ly = pad.top + (chartH / 4) * i;
    ctx.fillText((maxV - (range / 4) * i).toFixed(3), pad.left - 6, ly + 3);
  }

  // Points
  let pts = points.map(function(p, i) {
    return { x: pad.left + (i / (points.length - 1)) * chartW, y: pad.top + (1 - (p.value - minV) / range) * chartH };
  });

  // Gradient fill
  const accentColor = getThemeColor('--accent') || '#4fc1ff';
  let grad = ctx.createLinearGradient(0, pad.top, 0, pad.top + chartH);
  grad.addColorStop(0, hexToRgba(accentColor, 0.22));
  grad.addColorStop(1, hexToRgba(accentColor, 0.01));
  ctx.beginPath();
  ctx.moveTo(pts[0].x, pad.top + chartH);
  for (let j = 0; j < pts.length; j++) ctx.lineTo(pts[j].x, pts[j].y);
  ctx.lineTo(pts[pts.length - 1].x, pad.top + chartH);
  ctx.closePath();
  ctx.fillStyle = grad; ctx.fill();

  // Line with glow
  ctx.save();
  ctx.shadowColor = hexToRgba(accentColor, 0.5); ctx.shadowBlur = 5;
  ctx.strokeStyle = accentColor; ctx.lineWidth = 2;
  ctx.lineJoin = 'round'; ctx.beginPath();
  pts.forEach(function(p, i) { i === 0 ? ctx.moveTo(p.x, p.y) : ctx.lineTo(p.x, p.y); });
  ctx.stroke();
  ctx.restore();

  // Dots
  if (pts.length <= 20) {
    ctx.fillStyle = accentColor;
    pts.forEach(function(p) { ctx.beginPath(); ctx.arc(p.x, p.y, 2, 0, Math.PI * 2); ctx.fill(); });
  }

  let tooltipVisible = false;
  canvas.addEventListener('mousemove', function(e) {
    let canvasRect = canvas.getBoundingClientRect();
    let mx = e.clientX - canvasRect.left;
    let my = e.clientY - canvasRect.top;

    // 找到最近的資料點
    let nearestIdx = 0;
    let nearestDist = Infinity;
    pts.forEach(function(p, i) {
      let dist = Math.sqrt(Math.pow(p.x - mx, 2) + Math.pow(p.y - my, 2));
      if (dist < nearestDist) {
        nearestDist = dist;
        nearestIdx = i;
      }
    });

    // 計算 tooltip 位置（避免超出 canvas 範圍）
    let tpX = pts[nearestIdx].x + 12;
    let tpY = pts[nearestIdx].y - 10;
    if (tpX + 120 > W) tpX = pts[nearestIdx].x - 130;
    if (tpY < 10) tpY = 10;
    tooltip.style.left = tpX + 'px';
    tooltip.style.top = tpY + 'px';

    // 格式化 X 軸標籤：嘗試顯示日期，若無則顯示「第N筆」
    let label = points[nearestIdx].label || '';
    let isDateLabel = /^\d{4}[-/]\d{2}[-/]\d{2}/.test(label) || /^\d{2}[-/]\d{2}[-/]\d{4}/.test(label);
    let xLabel = isDateLabel ? label : '第' + (nearestIdx + 1) + '筆';

    // 格式化金額（NT$）
    let val = points[nearestIdx].value;
    let formattedValue = 'NT$ ' + (val >= 0 ? '+' : '') + val.toFixed(4);

    tooltip.innerHTML = '<div style="color:var(--muted)">' + xLabel + '</div><div style="font-weight:600;color:var(--pnl-profit)">' + formattedValue + '</div>';
    tooltip.style.opacity = '1';
    tooltipVisible = true;
  });

  canvas.addEventListener('mouseout', function() {
    tooltip.style.opacity = '0';
    tooltipVisible = false;
  });
}

function renderCompact() {
  let el = document.getElementById('evolutionContent');
  let catEl = document.getElementById('evolutionCatContent');
  if (catEl) catEl.style.display = 'none';
  if (!el) return;
  let d = evolutionData;

  let scorecards = (d.agents && d.agents.scorecards) || [];
  let sessions = (d.regime && d.regime.sessions) || [];
  let judges = (d.inbox && d.inbox.pending_judges) || [];
  let promotes = (d.inbox && d.inbox.pending_promotes) || [];
  let sorted = scorecards.slice().sort(function(a, b) { return (b.sharpe || 0) - (a.sharpe || 0); });

  let regimeHtml = renderRegimeTimeline(sessions, 40);

  let top5 = sorted.slice(0, 5);
  let agentSummaryHtml = top5.map(function(a, i) {
    let barW = Math.max(5, Math.min(100, (a.hit_rate || 0) * 100));
    let sharpeColor = (a.sharpe || 0) > 1 ? 'var(--up)' : ((a.sharpe || 0) < 0 ? 'var(--down)' : 'var(--warn)');
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

  let expCount = judges.length + promotes.length;
  let expHtml = '<div style="display:flex;gap:24px;margin-top:4px">' +
    '<div style="text-align:center"><div style="font-size:20px;font-weight:700;color:' + (judges.length > 0 ? 'var(--warn)' : 'var(--muted)') + '">' + judges.length + '</div><div style="font-size:11px;color:var(--muted)">待評判</div></div>' +
    '<div style="text-align:center"><div style="font-size:20px;font-weight:700;color:' + (promotes.length > 0 ? 'var(--color-success)' : 'var(--muted)') + '">' + promotes.length + '</div><div style="font-size:11px;color:var(--muted)">待晉升</div></div>' +
    '<div style="text-align:center"><div style="font-size:20px;font-weight:700">' + scorecards.length + '</div><div style="font-size:11px;color:var(--muted)">活躍 Agent</div></div>' +
    '</div>';

  // 繪製內嵌權益曲線（需在 innerHTML 設定前提取資料）
  let sortedForCurve = sorted.slice().sort(function(a, b) {
    return (a.last_updated_at || '').localeCompare(b.last_updated_at || '');
  });
  let curvePoints = sortedForCurve.map(function(a, i) {
    return { value: a.average_return || 0, label: (i + 1).toString() };
  });

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

  setTimeout(function() { renderInlineEquityCurve('inlineEquityCurve', curvePoints); }, 100);
}

function renderDetailed() {
  let el = document.getElementById('evolutionContent');
  let catEl = document.getElementById('evolutionCatContent');
  if (catEl) catEl.style.display = 'none';
  if (!el) return;
  let d = evolutionData;
  let scorecards = (d.agents && d.agents.scorecards) || [];
  let sessions = (d.regime && d.regime.sessions) || [];
  let judges = (d.inbox && d.inbox.pending_judges) || [];
  let promotes = (d.inbox && d.inbox.pending_promotes) || [];
  let sorted = scorecards.slice().sort(function(a, b) { return (b.sharpe || 0) - (a.sharpe || 0); });

  let scoreboardHtml = '<table style="width:100%;font-size:12px;border-collapse:collapse">' +
    '<thead><tr style="border-bottom:2px solid var(--border)">' +
    '<th style="text-align:left;padding:6px">Agent</th>' +
    '<th style="text-align:left;padding:6px">技能</th>' +
    '<th style="text-align:center;padding:6px">層</th>' +
    '<th style="text-align:right;padding:6px">觀察</th>' +
    '<th style="text-align:right;padding:6px">命中率</th>' +
    '<th style="text-align:right;padding:6px">Sharpe</th>' +
    '<th style="text-align:right;padding:6px">最大回撤</th>' +
    '</tr></thead><tbody>';
  for (let i = 0; i < sorted.length; i++) {
    let a = sorted[i];
    let sharpeColor = (a.sharpe || 0) > 1 ? 'var(--up)' : ((a.sharpe || 0) < 0 ? 'var(--down)' : 'var(--warn)');
    let hitColor = (a.hit_rate || 0) > 0.5 ? 'var(--up)' : ((a.hit_rate || 0) > 0.3 ? 'var(--warn)' : 'var(--muted)');
    scoreboardHtml += '<tr style="border-bottom:1px solid var(--border)">' +
      '<td style="padding:6px">' + agentName(a.agent_id) + '</td>' +
      '<td style="padding:6px;color:var(--muted)">' + (a.skill || '-') + '</td>' +
      '<td style="padding:6px;text-align:center"><span class="badge">' + (a.layer || '-') + '</span></td>' +
      '<td style="padding:6px;text-align:right">' + (a.observations || 0) + '</td>' +
      '<td style="padding:6px;text-align:right;color:' + hitColor + '">' + ((a.hit_rate || 0) * 100).toFixed(0) + '%</td>' +
      '<td style="padding:6px;text-align:right;color:' + sharpeColor + '">' + (a.sharpe || 0).toFixed(2) + '</td>' +
      '<td style="padding:6px;text-align:right;color:var(--color-danger)">' + ((a.max_drawdown || 0) * 100).toFixed(1) + '%</td>' +
      '</tr>';
  }
  scoreboardHtml += '</tbody></table>';

  let allExps = judges.concat(promotes);
  let expLogHtml = '';
  if (allExps.length > 0) {
    expLogHtml = '<div style="font-size:12px">';
    for (let j = 0; j < allExps.length; j++) {
      let e = allExps[j];
      let statusBadge = e.status === 'running' ? '<span style="color:var(--warn)">● 進行中</span>' :
        (e.status === 'completed' ? '<span style="color:var(--color-success)">✓ 完成</span>' : '<span style="color:var(--muted)">○ ' + e.status + '</span>');
      expLogHtml += '<div style="padding:6px 0;border-bottom:1px solid var(--border)">' +
        '<div><strong>' + agentName(e.target_agent_id) + '</strong> · ' + (e.mutation_type || '實驗') + '</div>' +
        '<div style="color:var(--muted);font-size:11px">' + (e.mutation_summary || '') + '</div>' +
        '<div style="display:flex;gap:16px;margin-top:4px;font-size:11px">' +
          statusBadge +
          '<span>基線: ' + ((e.baseline_value || 0)).toFixed(3) + '</span>' +
          '<span>候選: ' + ((e.candidate_value || 0)).toFixed(3) + '</span>' +
          '<span>' + (window.fmtNTD ? window.fmtNTD(e.baseline_monetary_ntd) : '—') + ' → ' + (window.fmtNTD ? window.fmtNTD(e.candidate_monetary_ntd) : '—') + '</span>' +
          '<span style="color:var(--muted)">' + (e.experiment_id || '') + '</span>' +
        '</div></div>';
    }
    expLogHtml += '</div>';
  } else {
    expLogHtml = '<div style="padding:20px;text-align:center;color:var(--muted)">當前無實驗記錄。系統處於穩態運行。</div>';
  }

  let comparisonHtml = '<div id="comparisonChartContainer" style="width:100%;height:220px;"></div>';

  el.innerHTML = '<div class="panel wide" style="margin-bottom:12px;padding:10px 14px">' +
    '<div style="font-size:13px;font-weight:700;margin-bottom:8px">策略比較 (Baseline vs Candidate 貨幣趨勢)</div>' +
    comparisonHtml +
    '</div>' +
    '<div class="panel wide" style="margin-bottom:12px;padding:10px 14px">' +
    '<div style="font-size:13px;font-weight:700;margin-bottom:8px">Agent 評分表</div>' +
    scoreboardHtml +
    '</div>' +
    '<div class="panel wide" style="padding:10px 14px">' +
    '<div style="font-size:13px;font-weight:700;margin-bottom:8px">實驗日誌</div>' +
    expLogHtml +
    '</div>';

  if (allExps.length >= 2) {
    let baseData = allExps.map((e, i) => ({ label: `Exp ${i+1}`, value: e.baseline_monetary_ntd || e.baseline_value || 0 }));
    let candData = allExps.map((e, i) => ({ label: `Exp ${i+1}`, value: e.candidate_monetary_ntd || e.candidate_value || 0 }));
    let datasets = [
      { label: 'Baseline', data: baseData, color: 'var(--muted)', glow: 'rgba(107,114,128,0.5)' },
      { label: 'Candidate', data: candData, color: 'var(--accent)', glow: 'rgba(79,193,255,0.5)' }
    ];
    setTimeout(() => renderComparisonChart('comparisonChartContainer', datasets, { height: 220 }), 50);
  } else {
    document.getElementById('comparisonChartContainer').innerHTML = '<div style="padding:40px;text-align:center;color:var(--muted)">實驗資料不足，無法繪製比較圖</div>';
  }
}

function renderCategorical() {
  let el = document.getElementById('evolutionContent');
  let catEl = document.getElementById('evolutionCatContent');
  if (!el || !catEl) return;
  catEl.style.display = 'block';

  // 分類視圖：上方顯示 Tab 按鈕，下方顯示對應內容
  el.innerHTML = '<div id="evolutionTabs" style="display:flex;gap:8px;margin-bottom:16px">' +
    '<button class="cat-tab active" id="catTab-agents" onclick="window._evCatTab(\'agents\')">Agent 競爭</button>' +
    '<button class="cat-tab" id="catTab-regime" onclick="window._evCatTab(\'regime\')">Regime 演化</button>' +
    '<button class="cat-tab" id="catTab-experiments" onclick="window._evCatTab(\'experiments\')">實驗日誌</button>' +
    '</div>';

  let d = evolutionData;
  let scorecards = (d.agents && d.agents.scorecards) || [];
  let sessions = (d.regime && d.regime.sessions) || [];
  let judges = (d.inbox && d.inbox.pending_judges) || [];
  let promotes = (d.inbox && d.inbox.pending_promotes) || [];

  renderCatTabContent('agents', scorecards, sessions, judges, promotes);
}

function renderCatTabContent(tab, scorecards, sessions, judges, promotes) {
  let el = document.getElementById('evolutionCatContent');
  if (!el) return;

  document.querySelectorAll('#evolutionTabs .cat-tab').forEach(function(b) { b.classList.remove('active'); });
  let activeBtn = document.getElementById('catTab-' + tab);
  if (activeBtn) activeBtn.classList.add('active');

  if (tab === 'agents') {
    let sorted = scorecards.slice().sort(function(a, b) { return (b.sharpe || 0) - (a.sharpe || 0); });
    let byLayer = {};
    for (let i = 0; i < sorted.length; i++) {
      let layer = sorted[i].layer || 'unknown';
      if (!byLayer[layer]) byLayer[layer] = [];
      byLayer[layer].push(sorted[i]);
    }
    
    let radarHtml = '';
    if (sorted.length > 0) {
      let top = sorted[0];
      let rMetrics = [
        Math.max(0, Math.min(1, (top.sharpe || 0) / 3)),
        top.hit_rate || 0,
        Math.max(0, 1 - (top.max_drawdown || 0)),
        Math.max(0, Math.min(1, (top.observations || 0) / 100)),
        (top.hit_rate || 0) * 0.8 + 0.2
      ];
      radarHtml = '<div style="margin-bottom:24px"><div style="font-size:13px;font-weight:700;margin-bottom:8px">最強 Agent 能力雷達 (' + agentName(top.agent_id) + ')</div>' +
        '<div id="radarChartContainer" style="width:100%;height:200px"></div></div>';
      setTimeout(() => renderRadarChart('radarChartContainer', rMetrics, ['Sharpe', 'Hit Rate', 'Resilience', 'Observations', 'Consistency']), 50);
    }

    let layers = ['sector', 'style', 'superinvestor', 'context', 'control'];
    let html = radarHtml;
    for (let l = 0; l < layers.length; l++) {
      let agents = byLayer[layers[l]];
      if (!agents || agents.length === 0) continue;
      html += '<div style="margin-bottom:16px"><div style="font-size:13px;font-weight:700;margin-bottom:8px">' +
        (layers[l] === 'sector' ? '🏭 產業' : layers[l] === 'style' ? '🎨 風格' : layers[l] === 'superinvestor' ? '🧠 超級投資者' : layers[l] === 'context' ? '🌍 宏觀' : '⚙️ 控制') +
        ' (' + agents.length + ')</div>';
      for (let j = 0; j < agents.length; j++) {
        let a = agents[j];
        let barLen = Math.max(5, Math.min(100, (a.hit_rate || 0) * 100));
        let barColor = barLen > 60 ? 'var(--up)' : (barLen > 30 ? 'var(--warn)' : 'var(--muted)');
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
    el.innerHTML = '<div style="font-size:13px;font-weight:700;margin-bottom:8px">Regime 演化時間線 (含波動/成交量)</div>' +
      '<div style="font-size:11px;color:var(--muted);margin-bottom:12px">共 ' + sessions.length + ' 個 session</div>' +
      '<div id="regimeVolumeContainer"></div>';
    
    // Regime as intensity proxy (backend has no per-session volume yet).
    const _regimeToIntensity = { RISK_ON: 1.0, RISK_OFF: 0.4, NEUTRAL: 0.7 };
    let volumes = sessions.map(s => _regimeToIntensity[s.regime] || 0.5);
    setTimeout(() => renderRegimeVolumeChart('regimeVolumeContainer', sessions, volumes), 50);
  }
  else if (tab === 'experiments') {
    let all = judges.concat(promotes);
    let html = '';
    if (all.length > 0) {
      for (let i = 0; i < all.length; i++) {
        let e = all[i];
        let isJudge = judges.indexOf(e) >= 0;
        html += '<div style="padding:8px 0;border-bottom:1px solid var(--border)">' +
          '<div style="display:flex;justify-content:space-between;align-items:center">' +
            '<div><strong>' + agentName(e.target_agent_id) + '</strong> · ' + (e.mutation_type || '') + '</div>' +
            '<span style="font-size:11px;color:' + (isJudge ? 'var(--warn)' : 'var(--color-success)') + '">' + (isJudge ? '待評判' : '待晉升') + '</span>' +
          '</div>' +
          '<div style="font-size:11px;color:var(--muted);margin-top:2px">' + (e.mutation_summary || '') + '</div>' +
          '<div style="display:flex;gap:16px;margin-top:2px;font-size:11px">' +
            '<span>基線: ' + (e.baseline_value || 0).toFixed(3) + '</span>' +
            '<span>候選: ' + (e.candidate_value || 0).toFixed(3) + '</span>' +
            '<span>' + (window.fmtNTD ? window.fmtNTD(e.baseline_monetary_ntd) : '—') + ' → ' + (window.fmtNTD ? window.fmtNTD(e.candidate_monetary_ntd) : '—') + '</span>' +
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

  let display = sessions;
  if (maxBars > 0 && sessions.length > maxBars) {
    let step = Math.floor(sessions.length / maxBars);
    display = [];
    for (let i = 0; i < sessions.length; i += step) {
      display.push(sessions[i]);
      if (display.length >= maxBars) break;
    }
  }

  let colors = { RISK_ON: 'var(--up)', RISK_OFF: 'var(--down)', NEUTRAL: 'var(--warn)' };
  let segments = [];
  let currentRegime = display[0].regime;
  let segmentStart = 0;
  for (let i = 1; i < display.length; i++) {
    if (display[i].regime !== currentRegime) {
      segments.push({ regime: currentRegime, count: i - segmentStart, start: segmentStart });
      currentRegime = display[i].regime;
      segmentStart = i;
    }
  }
  segments.push({ regime: currentRegime, count: display.length - segmentStart, start: segmentStart });

  let total = display.length;
  let html = '<div style="display:flex;width:100%;height:24px;border-radius:4px;overflow:hidden;margin-bottom:6px">';
  for (let s = 0; s < segments.length; s++) {
    let seg = segments[s];
    let w = (seg.count / total * 100).toFixed(2);
    let color = colors[seg.regime] || 'var(--muted)';
    html += '<div title="' + regimeLabel(seg.regime) + ': ' + seg.count + ' sessions" ' +
      'style="width:' + w + '%;height:100%;background:' + color + ';transition:all 0.2s;position:relative">' +
      (seg.count > 3 ? '<span style="position:absolute;left:4px;top:50%;transform:translateY(-50%);font-size:9px;font-weight:600;color:var(--bg);white-space:nowrap">' + regimeLabel(seg.regime) + '</span>' : '') +
      '</div>';
  }
  html += '</div>';

  html += '<div style="display:flex;gap:16px;font-size:10px;color:var(--muted)">' +
    '<span style="color:var(--up)">🔴 多頭</span>' +
    '<span style="color:var(--down)">🟢 空頭</span>' +
    '<span style="color:var(--warn)">🟡 盤整</span>' +
    '<span style="margin-left:auto">' + display[0].session_id.split('-')[1] + ' → ' + display[display.length - 1].session_id.split('-')[1] + '</span>' +
    '</div>';

  return html;
}

// 視圖切換入口（從 HTML onclick 調用）
window._evSwitch = switchView;

// 分類 Tab 切換
window._evCatTab = function(tab) {
  let d = evolutionData;
  let scorecards = (d.agents && d.agents.scorecards) || [];
  let sessions = (d.regime && d.regime.sessions) || [];
  let judges = (d.inbox && d.inbox.pending_judges) || [];
  let promotes = (d.inbox && d.inbox.pending_promotes) || [];
  renderCatTabContent(tab, scorecards, sessions, judges, promotes);
};
