import { agentName, regimeLabel } from '../names.js';
import { silentGetJSON } from '../shared/app-utils.js';
import { getThemeColor } from '../shared/utils.js';
import { hexToRgba } from '../shared/color-tokens.js';
import { fmtSafeNumber, fmtSafeSignedPct } from '../shared/format-metric.js';

function isValidNumber(v) {
  return typeof v === 'number' && Number.isFinite(v);
}

let evolutionData = null;
let currentView = 'compact';

// ====== Trend Summary ======

function renderTrendSummary(scorecards, sessions) {
  if (!scorecards.length) return '';
  const sharpeValues = scorecards.map(function(a) { return a.sharpe; }).filter(isValidNumber).sort(function(a, b) { return a - b; });
  const mid = Math.floor(sharpeValues.length / 2);
  const medianSharpe = sharpeValues.length % 2 === 0
    ? (sharpeValues[mid - 1] + sharpeValues[mid]) / 2
    : sharpeValues[mid];
  const hitRateValues = scorecards.map(function(a) { return a.hit_rate; }).filter(isValidNumber);
  const avgHitRate = hitRateValues.length ? hitRateValues.reduce(function(s, v) { return s + v; }, 0) / hitRateValues.length : null;
  const healthyCount = scorecards.filter(function(a) { return (a.sharpe || 0) > 0.5 && (a.hit_rate || 0) > 0.3; }).length;
  const weakCount = scorecards.filter(function(a) { return (a.sharpe || 0) < 0.5; }).length;
  const healthPct = Math.round(healthyCount / scorecards.length * 100);
  const weakPct = Math.round(weakCount / scorecards.length * 100);

  var transitions = 0;
  for (var i = 1; i < sessions.length; i++) {
    if (sessions[i].regime !== sessions[i - 1].regime) transitions++;
  }
  var stability = sessions.length > 0 ? Math.round((1 - transitions / sessions.length) * 100) : 100;

  var recentSlice = sessions.slice(-5);
  var recentTransitions = 0;
  for (var j = 1; j < recentSlice.length; j++) {
    if (recentSlice[j].regime !== recentSlice[j - 1].regime) recentTransitions++;
  }
  var regimeWarning = '';
  if (recentTransitions >= 3) {
    regimeWarning = '<span style="display:block;color:var(--warn);margin-top:2px">⚠ 近期 Regime 波動加劇：近 5 場切換 <strong>' + recentTransitions + '</strong> 次，市場方向不明確</span>';
  } else if (recentTransitions === 2 && recentSlice.length >= 4) {
    regimeWarning = '<span style="display:block;color:var(--warn);margin-top:2px">⚡ 近期 Regime 出現波動：近 5 場切換 ' + recentTransitions + ' 次，注意方向轉變</span>';
  }

  var sharpeCls = medianSharpe > 1 ? 'ev-summary-good' : (medianSharpe > 0.5 ? 'ev-summary-warn' : 'ev-summary-bad');
  var hitCls = avgHitRate > 0.6 ? 'ev-summary-good' : (avgHitRate > 0.4 ? 'ev-summary-warn' : 'ev-summary-bad');
  var healthCls = healthPct > 70 ? 'ev-summary-good' : (healthPct > 40 ? 'ev-summary-warn' : 'ev-summary-bad');
  var weakCls = weakPct < 20 ? 'ev-summary-good' : (weakPct < 40 ? 'ev-summary-warn' : 'ev-summary-bad');

  return '<div class="ev-trend-summary">' +
    '<div class="ev-summary-title">📊 演化趨勢摘要</div>' +
    '<div class="ev-summary-grid">' +
      '<div class="ev-summary-card ' + sharpeCls + '">' +
        '<div class="ev-summary-value">' + fmtSafeNumber(medianSharpe, { decimals: 2 }) + '</div>' +
        '<div class="ev-summary-label">Sharpe 中位數</div>' +
        '<div class="ev-summary-tag">' + (medianSharpe > 1 ? '✅ 良好' : (medianSharpe > 0.5 ? '⚠ 一般' : '❌ 偏弱')) + '</div>' +
      '</div>' +
      '<div class="ev-summary-card ' + hitCls + '">' +
        '<div class="ev-summary-value">' + fmtSafeNumber(avgHitRate, { decimals: 0, suffix: '%', percent: true }) + '</div>' +
        '<div class="ev-summary-label">命中率 整體</div>' +
        '<div class="ev-summary-tag">' + (avgHitRate > 0.6 ? '✅ 優秀' : (avgHitRate > 0.4 ? '⚠ 一般' : '❌ 偏低')) + '</div>' +
      '</div>' +
      '<div class="ev-summary-card ' + healthCls + '">' +
        '<div class="ev-summary-value">' + healthyCount + '<span style="font-size:16px">/</span>' + scorecards.length + '</div>' +
        '<div class="ev-summary-label">策略健康度</div>' +
        '<div class="ev-summary-tag">' + (healthPct > 70 ? '✅ 健康' : (healthPct > 40 ? '⚠ 注意' : '❌ 堪慮')) + ' (' + healthPct + '%)</div>' +
      '</div>' +
      '<div class="ev-summary-card ' + weakCls + '">' +
        '<div class="ev-summary-value">' + weakCount + '</div>' +
        '<div class="ev-summary-label">淘汰壓力</div>' +
        '<div class="ev-summary-tag">' + (weakPct < 20 ? '✅ 低壓' : (weakPct < 40 ? '⚠ 關注' : '❌ 高壓')) + ' (' + weakPct + '%)</div>' +
      '</div>' +
    '</div>' +
    '<div class="ev-summary-footer">' +
      'Regime 穩定度 <strong>' + stability + '%</strong> — ' + transitions + ' 次切換 / ' + sessions.length + ' 場次 · ' +
      '<span style="color:var(--muted)">系統正以 mutation → judge → promote 循環持續優化 Agent 策略</span>' +
      regimeWarning +
    '</div>' +
    '</div>';
}

// ====== Page-Level Reading Guide ======

function renderPageGuide(view) {
  var views = {
    compact: '<strong>精簡檢視</strong>：一目了然看趨勢 — 上方 Regime 點矩陣判斷市場環境，中間實驗卡了解進化熱度，下方 Agent 排名與淘汰名單判斷策略健康度。',
    'ai-analysis': '<strong>AI 競爭分析</strong>：結合 Agent 競爭散布圖、Agent 評分表（含命中率、Sharpe、最大回撤）與實驗日誌，全面分析 Agent 的歷史績效與競爭態勢。'
  };
  var viewDesc = views[view] || '';

  return '<details class="ev-reading-guide" id="evReadingGuide">' +
    '<summary><strong>💡 如何解讀本頁</strong> — ' + (view === 'compact' ? '精簡' : 'AI 競爭') + '檢視</summary>' +
    '<div class="ev-guide-body">' +
      '<p><strong>演化透視</strong> 是系統的「達爾文進化儀表板」— 顯示 AI Agent 在回測中的競爭、淘汰與進化過程。</p>' +
      '<p style="margin-bottom:6px">目前模式：' + viewDesc + '</p>' +
      '<h4>核心概念</h4>' +
      '<ul>' +
        '<li><strong>Regime（市場體制）</strong>：系統對當前市場環境的自動分類。多頭（RISK_ON）= 偏向積極配置；空頭（RISK_OFF）= 偏向防禦或減倉；盤整（NEUTRAL）= 中性觀望；過渡（TRANSITIONAL）= 體制正在切換中。</li>' +
        '<li><strong>命中率（Hit Rate）</strong>：AI Agent 推薦產生正向回測報酬的比例。<strong>&gt;60% 為優秀</strong>（綠），30~60% 為一般（黃），&lt;30% 為低（灰）。持續高命中率代表該 Agent 的選股邏輯在當前市場體制下相對有效。</li>' +
        '<li><strong>Sharpe Ratio</strong>：風險調整後報酬指標。<strong>&gt;1.0 為良好</strong>（綠），0~1 為一般（黃），&lt;0 表示經風險調整後整體為負貢獻（紅）。數值越高代表承擔單位風險所獲得的報酬越好。</li>' +
        '<li><strong>Delta（▲/▼）</strong>：候選策略 vs 基線策略的績效差異。正值（▲）表示候選策略優於基線，負值（▼）表示劣於基線。這是系統判斷是否「晉升」候選策略的核心依據。</li>' +
        '<li><strong>最大回撤（Max Drawdown）</strong>：歷史推薦中曾出現的最大累積虧損幅度。數值越接近 0 風險控制越好；絕對值過大時應檢查該 Agent 的停損機制。</li>' +
      '</ul>' +
      '<div class="ev-guide-note">💬 本頁所有數據均來自歷史回測，不代表未來績效。系統透過持續的 mutation（突變）→ judge（評判）→ promote（晉升）循環來優化 Agent 策略。</div>' +
    '</div>' +
    '</details>';
}

export async function loadEvolutionData() {
  const [agents, regime, inbox] = await Promise.all([
    silentGetJSON('/api/dashboard/agent-observatory'),
    silentGetJSON('/api/dashboard/regime-history'),
    silentGetJSON('/api/dashboard/experiment-inbox'),
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
  else if (currentView === 'ai-analysis') renderAiAnalysis();
}

function getData() {
  const d = evolutionData || {};
  return {
    scorecards: (d.agents && d.agents.scorecards) || [],
    sessions: (d.regime && d.regime.sessions) || [],
    judges: (d.inbox && d.inbox.pending_judges) || [],
    promotes: (d.inbox && d.inbox.pending_promotes) || [],
  };
}

// ====== Helpers ======

function regimeClass(regime) {
  const map = { RISK_ON: 'risk-on', RISK_OFF: 'risk-off', NEUTRAL: 'neutral', TRANSITIONAL: 'transitional' };
  return map[regime] || 'neutral';
}

function sharpeClass(v) {
  return (v || 0) > 1 ? 'value-up' : ((v || 0) < 0 ? 'value-down' : 'value-warn');
}

function hitRateBarClass(v) {
  const p = (v || 0) * 100;
  return p > 60 ? 'high' : (p > 30 ? 'mid' : 'low');
}

function escapeHtml(s) {
  if (!s) return '';
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

function latestRegime(sessions) {
  if (!sessions || !sessions.length) return 'NEUTRAL';
  return sessions[sessions.length - 1].regime || 'NEUTRAL';
}

function evolutionState(judgeCount, promoteCount) {
  const total = judgeCount + promoteCount;
  if (total === 0) return 'stable';
  if (judgeCount > 0) return 'pending';
  return 'experimenting';
}

function formatDelta(baseline, candidate) {
  if (!isValidNumber(baseline) || !isValidNumber(candidate)) return { cls: 'neutral', text: '—', value: 0 };
  const delta = ((candidate - baseline) / Math.abs(baseline)) * 100;
  const cls = delta > 1 ? 'positive' : (delta < -1 ? 'negative' : 'neutral');
  return { cls, text: fmtSafeSignedPct(delta, 1), value: delta };
}

// ====== Cold-Start Empty State ======

const COLD_START_SCHEDULER = 'auto_strategy_evolution';

function isColdStart(scorecards, sessions, judges, promotes) {
  return [scorecards, sessions, judges, promotes].every(s => !s || !s.length);
}

function renderColdStartEmpty() {
  return '<div class="empty-state-guidance" style="padding:48px 16px">' +
    '<div class="icon" style="font-size:48px">🧬</div>' +
    '<div class="title" style="font-size:16px;margin-bottom:8px">演化系統尚未啟動</div>' +
    '<div class="desc" style="max-width:480px;margin:0 auto 12px">需等待 <code>' + COLD_START_SCHEDULER + '</code> 排程首次觸發。<br>' +
    '在此之前，Agent 評分、Regime 歷史、實驗記錄均為空。</div>' +
    '<div class="action">💡 可至排程頁面查詢首次執行時間</div>' +
    '</div>';
}

function renderAgentScoreEmpty() {
  return '<div class="empty-state-guidance"><div class="icon">🤖</div>' +
    '<div class="title">無 Agent 評分</div>' +
    '<div class="desc">需等待 <code>' + COLD_START_SCHEDULER + '</code> 排程首次執行</div></div>';
}

// ====== Shared Experiment List ======

function renderExperimentList(judges, promotes, showStatus) {
  const all = judges.concat(promotes);
  if (!all.length) {
    return '<div class="empty-state-guidance"><div class="icon">🧪</div><div class="title">無實驗記錄</div><div class="desc">系統處於穩態運行</div></div>';
  }
  let html = '';
  for (const e of all) {
    const isJudge = judges.includes(e);
    const delta = formatDelta(e.baseline_value, e.candidate_value);
    let statusBadge = '';
    if (showStatus) {
      statusBadge = e.status === 'running'
        ? '<span class="badge warn">進行中</span>'
        : (e.status === 'completed' ? '<span class="badge ok">完成</span>' : '<span class="badge info">' + escapeHtml(e.status || '') + '</span>');
    } else {
      statusBadge = isJudge
        ? '<span class="badge warn">待評判</span>'
        : '<span class="badge ok">待晉升</span>';
    }
    html += '<div class="ev-exp-item">' +
      '<div class="ev-exp-header">' +
        '<div><strong>' + escapeHtml(agentName(e.target_agent_id)) + '</strong> · ' + escapeHtml(e.mutation_type || '實驗') + '</div>' +
        statusBadge +
      '</div>' +
      '<div class="ev-exp-detail">' + escapeHtml(e.mutation_summary || '') + '</div>' +
      '<div class="ev-exp-metrics">' +
        '<span class="baseline">基線: ' + fmtSafeNumber(e.baseline_value, { decimals: 3 }) + '</span>' +
        '<span class="candidate">候選: ' + fmtSafeNumber(e.candidate_value, { decimals: 3 }) + '</span>' +
        '<span class="ev-exp-delta ' + delta.cls + '">' + delta.text + '</span>' +
        (showStatus ? '<span class="ev-exp-id">' + escapeHtml(e.experiment_id || '') + '</span>' : '') +
      '</div></div>';
  }
  html += '<div class="ev-delta-legend">' +
    '<span><span class="ev-exp-delta positive" style="display:inline-block;vertical-align:middle;margin:0">▲ +N%</span> 候選優於基線</span>' +
    '<span><span class="ev-exp-delta negative" style="display:inline-block;vertical-align:middle;margin:0">▼ -N%</span> 候選劣於基線</span>' +
    '<span><span class="ev-exp-delta neutral" style="display:inline-block;vertical-align:middle;margin:0">— 0%</span> 無顯著差異</span>' +
    '</div>';
  return html;
}

// ====== Regime Timeline ======

function renderRegimeTimeline(sessions, maxDots) {
  if (!sessions || !sessions.length) {
    return '<div class="empty-state-guidance"><div class="icon">📡</div><div class="title">無 Regime 歷史</div><div class="desc">系統尚未累積足夠 session 數據</div></div>';
  }
  let display = sessions;
  if (maxDots > 0 && sessions.length > maxDots) {
    const step = Math.floor(sessions.length / maxDots);
    display = [];
    for (let i = 0; i < sessions.length; i += step) {
      display.push(sessions[i]);
      if (display.length >= maxDots) break;
    }
  }

  let statsHtml = '<div class="ev-regime-stats">';
  const counts = { RISK_ON: 0, RISK_OFF: 0, NEUTRAL: 0, TRANSITIONAL: 0 };
  for (const s of display) { counts[s.regime] = (counts[s.regime] || 0) + 1; }
  const total = display.length;
  const statItems = [
    { key: 'RISK_ON', cls: 'risk-on', label: '多頭' },
    { key: 'RISK_OFF', cls: 'risk-off', label: '空頭' },
    { key: 'NEUTRAL', cls: 'neutral', label: '盤整' },
    { key: 'TRANSITIONAL', cls: 'transitional', label: '過渡' },
  ];
  for (const item of statItems) {
    const cnt = counts[item.key] || 0;
    if (cnt === 0) continue;
    const pct = fmtSafeNumber(cnt / total, { percent: true, decimals: 0, suffix: '%' });
    statsHtml += '<div class="ev-regime-stat"><span class="ev-regime-stat-dot ' + item.cls + '"></span>' +
      '<span class="ev-regime-stat-label">' + item.label + '</span>' +
      '<span class="ev-regime-stat-pct ' + item.cls + '">' + pct + '%</span>' +
      '<span class="ev-regime-stat-count">' + cnt + '</span></div>';
  }
  statsHtml += '</div>';

  let dotsHtml = '<div class="ev-regime-dots">';
  for (let i = 0; i < display.length; i++) {
    const s = display[i];
    const cls = regimeClass(s.regime);
    const dateHint = s.session_id ? s.session_id.split('-')[1] : '';
    if (i > 0 && display[i].regime !== display[i - 1].regime) {
      dotsHtml += '<span class="ev-regime-transition"></span>';
    }
    dotsHtml += '<span class="ev-regime-dot ' + cls + '" title="' + regimeLabel(s.regime) + ' (' + dateHint + ')"></span>';
  }
  dotsHtml += '</div>';

  let hintHtml = '<div class="ev-section-hint">🔴 多頭（RISK_ON）= 偏向積極配置 · 🟢 空頭（RISK_OFF）= 防禦或減倉 · 🟡 盤整（NEUTRAL）= 中性觀望 · 🟣 過渡（TRANSITIONAL）= 體制切換中。藍色分隔線標記體制切換點；同色密集區 = 市場穩定期，頻繁變色 = 動盪期。</div>';

  let metaHtml = '<div class="ev-regime-meta">' +
    '<div class="ev-regime-legend">' +
      '<span><span class="dot risk-on"></span> 多頭</span>' +
      '<span><span class="dot risk-off"></span> 空頭</span>' +
      '<span><span class="dot neutral"></span> 盤整</span>' +
      '<span><span class="dot transitional"></span> 過渡</span>' +
    '</div>' +
    '<span class="ev-regime-range">' + display[0].session_id.split('-')[1] + ' → ' + display[display.length - 1].session_id.split('-')[1] + '</span>' +
    '</div>';

  return statsHtml + dotsHtml + hintHtml + metaHtml;
}

// ====== Compact View ======

function renderCompact() {
  const el = document.getElementById('evolutionContent');
  if (!el) return;
  const { scorecards, sessions, judges, promotes } = getData();
  if (isColdStart(scorecards, sessions, judges, promotes)) {
    el.innerHTML = renderPageGuide('compact') + renderColdStartEmpty();
    return;
  }
  var allExps = judges.concat(promotes);

  const sorted = scorecards.slice().sort((a, b) => (b.sharpe || 0) - (a.sharpe || 0));
  const top5 = sorted.slice(0, 5);
  const weakest = sorted.slice(-3).reverse();
  const current = latestRegime(sessions);
  const state = evolutionState(judges.length, promotes.length);
  const expCount = judges.length + promotes.length;

  let regimeIndicator = '<div class="ev-current-regime ' + regimeClass(current) + '">' +
    regimeLabel(current) + '</div>';
  let regimeSection = '<div class="panel wide" style="margin-bottom:12px;padding:14px 16px">' +
    '<div class="ev-section-title">Regime 演化 ' + regimeIndicator + '</div>' +
    '<div class="ev-section-count">' + sessions.length + ' sessions</div>' +
    renderRegimeTimeline(sessions, 60) +
    '</div>';

  let stateHtml = '';
  if (state === 'stable') {
    stateHtml = '<div class="ev-state stable"><span class="ev-state-dot"></span>系統處於穩態運行，所有 Agent 策略已收斂</div>';
  } else if (state === 'pending') {
    stateHtml = '<div class="ev-state pending"><span class="ev-state-dot"></span>有 ' + judges.length + ' 個候選者等待評判，' + promotes.length + ' 個待晉升</div>';
  } else {
    stateHtml = '<div class="ev-state experimenting"><span class="ev-state-dot"></span>系統正在持續進化 Agent 策略，' + expCount + ' 個實驗進行中</div>';
  }

  let expSection = '<div class="panel wide" style="margin-bottom:12px;padding:14px 16px">' +
    '<div class="ev-section-title">實驗活動 <span class="ev-section-count">' + expCount + ' active</span></div>' +
    '<div class="ev-kpi-grid">' +
      '<div class="ev-kpi-card"><div class="ev-kpi-value" style="color:' + (judges.length > 0 ? 'var(--warn)' : 'var(--muted)') + '">' + judges.length + '</div><div class="ev-kpi-label">待評判</div><div class="ev-kpi-hint">需人工確認</div></div>' +
      '<div class="ev-kpi-card"><div class="ev-kpi-value" style="color:' + (promotes.length > 0 ? 'var(--color-success)' : 'var(--muted)') + '">' + promotes.length + '</div><div class="ev-kpi-label">待晉升</div><div class="ev-kpi-hint">可升級為基線</div></div>' +
      '<div class="ev-kpi-card"><div class="ev-kpi-value">' + scorecards.length + '</div><div class="ev-kpi-label">活躍 Agent</div><div class="ev-kpi-hint">策略總數</div></div>' +
    '</div>' +
stateHtml +
    (allExps.length > 0 ? '<div style="margin-top:12px;max-height:180px;overflow:auto">' + renderExperimentList(judges, promotes, false) + '</div>' : '') +
    '</div>';

  let agentRows = '';
  for (let i = 0; i < top5.length; i++) {
    const a = top5[i];
    const barW = Math.max(5, Math.min(100, (a.hit_rate || 0) * 100));
    const barCls = hitRateBarClass(a.hit_rate);
    agentRows += '<div class="ev-agent-row">' +
      '<span class="ev-agent-rank ' + (i < 3 ? 'top' : '') + '">' + (i + 1) + '</span>' +
      '<span class="ev-agent-name">' + escapeHtml(agentName(a.agent_id)) + '</span>' +
      '<span class="ev-agent-layer" data-layer="' + escapeHtml(a.layer || '') + '">' + escapeHtml(a.layer || '-') + '</span>' +
      '<div class="ev-agent-bar-track"><div class="ev-agent-bar-fill ' + barCls + '" style="width:' + barW + '%"></div></div>' +
      '<span class="ev-agent-stat">' + fmtSafeNumber(a.hit_rate, { decimals: 0, suffix: '%', percent: true }) + '</span>' +
      '<span class="ev-agent-stat ' + sharpeClass(a.sharpe) + '">S:' + fmtSafeNumber(a.sharpe, { decimals: 2 }) + '</span>' +
      '</div>';
  }
  let agentSection = '<div class="panel" style="padding:14px 16px">' +
    '<div class="ev-section-title">🏆 Agent 表現 Top 5</div>' +
    (top5.length > 0
      ? '<div class="ev-metric-legend" style="margin-bottom:6px">' +
        '<span><span class="ev-legend-swatch high"></span> 命中率 &gt;60%</span>' +
        '<span><span class="ev-legend-swatch mid"></span> 30-60%</span>' +
        '<span><span class="ev-legend-swatch low"></span> &lt;30%</span>' +
        '<span style="margin-left:8px"><span class="ev-legend-swatch good"></span> Sharpe &gt;1</span>' +
        '<span><span class="ev-legend-swatch warn"></span> 0-1</span>' +
        '<span><span class="ev-legend-swatch bad"></span> &lt;0</span>' +
        '</div>' + agentRows
      : renderAgentScoreEmpty()) +
    '</div>';

  let elimRows = '';
  for (let i = 0; i < weakest.length; i++) {
    const a = weakest[i];
    if ((a.sharpe || 0) > 0.5) continue;
    const barW = Math.max(5, Math.min(100, (a.hit_rate || 0) * 100));
    const barCls = hitRateBarClass(a.hit_rate);
    elimRows += '<div class="ev-agent-row ev-agent-elimination">' +
      '<span class="ev-elim-icon">⚠</span>' +
      '<span class="ev-agent-name">' + escapeHtml(agentName(a.agent_id)) + '</span>' +
      '<span class="ev-agent-layer" data-layer="' + escapeHtml(a.layer || '') + '">' + escapeHtml(a.layer || '-') + '</span>' +
      '<div class="ev-agent-bar-track"><div class="ev-agent-bar-fill ' + barCls + '" style="width:' + barW + '%"></div></div>' +
      '<span class="ev-agent-stat">' + fmtSafeNumber(a.hit_rate, { decimals: 0, suffix: '%', percent: true }) + '</span>' +
      '<span class="ev-agent-stat value-down">S:' + fmtSafeNumber(a.sharpe, { decimals: 2 }) + '</span>' +
      '</div>';
  }
  let elimSection = '<div class="panel ev-elim-panel" style="padding:14px 16px">' +
    '<div class="ev-section-title">⚡ 淘汰候選</div>' +
    '<div class="ev-section-hint" style="margin-bottom:6px">Sharpe &lt;0.5 的 Agent，績效顯著落後。若持續低於門檻，將在下一進化週期被淘汰並由新突變策略取代。</div>' +
    (elimRows ? elimRows : '<div class="empty" style="padding:12px 0">目前無低績效 Agent</div>') +
    '</div>';

  el.innerHTML = renderPageGuide('compact') + renderTrendSummary(scorecards, sessions) + regimeSection + expSection +
    '<div style="display:grid;grid-template-columns:1fr 1fr;gap:12px">' +
      agentSection + elimSection +
    '</div>';
}

// ====== AI Analysis View ======

function renderAiAnalysis() {
  const el = document.getElementById('evolutionContent');
  const catEl = document.getElementById('evolutionCatContent');
  if (!el || !catEl) return;
  catEl.style.display = 'block';

  const { scorecards, sessions, judges, promotes } = getData();
  if (isColdStart(scorecards, sessions, judges, promotes)) {
    el.innerHTML = renderPageGuide('ai-analysis') + renderColdStartEmpty();
    renderCatContent('agents', scorecards, sessions, judges, promotes);
    return;
  }

  // Render categorical content in catEl
  renderCatContent('agents', scorecards, sessions, judges, promotes);

  // Render detailed content in el
  const sorted = scorecards.slice().sort((a, b) => (b.sharpe || 0) - (a.sharpe || 0));

  let tableHtml;
  if (sorted.length > 0) {
    tableHtml = '<table class="ev-score-table"><thead><tr>' +
      '<th>Agent</th><th>技能</th><th style="text-align:center">層</th><th style="text-align:right" title="該 Agent 參與過的歷史回測視窗數。越多表示統計可信度越高。">觀察 ⓘ</th>' +
      '<th style="text-align:right" title="推薦產生正向報酬的比例。>60% 優秀（綠），30-60% 一般（黃），<30% 偏低（灰）。">命中率 ⓘ</th>' +
      '<th style="text-align:right" title="風險調整後報酬。>1 良好（綠），0-1 一般（黃），<0 虧損（紅）。越高代表單位風險報酬越好。">Sharpe ⓘ</th>' +
      '<th style="text-align:right" title="歷史推薦中最大累積虧損。越接近 0 風險控制越好。">最大回撤 ⓘ</th></tr></thead><tbody>';

    for (let i = 0; i < sorted.length; i++) {
    const a = sorted[i];
    const sClass = sharpeClass(a.sharpe);
    const hColor = (a.hit_rate || 0) > 0.5 ? 'var(--metric-good)' : ((a.hit_rate || 0) > 0.3 ? 'var(--warn)' : 'var(--muted)');
    tableHtml += '<tr>' +
      '<td>' + escapeHtml(agentName(a.agent_id)) + '</td>' +
      '<td class="text-muted">' + escapeHtml(a.skill || '-') + '</td>' +
      '<td style="text-align:center"><span class="badge info">' + escapeHtml(a.layer || '-') + '</span></td>' +
      '<td style="text-align:right">' + (a.observations || 0) + '</td>' +
      '<td style="text-align:right;color:' + hColor + '">' + fmtSafeNumber(a.hit_rate, { decimals: 0, suffix: '%', percent: true }) + '</td>' +
      '<td style="text-align:right"><span class="' + sClass + '">' + fmtSafeNumber(a.sharpe, { decimals: 2 }) + '</span></td>' +
      '<td style="text-align:right;color:var(--risk-high)">' + fmtSafeNumber(a.max_drawdown, { decimals: 1, suffix: '%', percent: true }) + '</td>' +
      '</tr>';
    }
    tableHtml += '</tbody></table>';
  } else {
    tableHtml = renderAgentScoreEmpty();
  }

  let scoreSection = '<div class="panel wide" style="margin-bottom:12px;padding:14px 16px">' +
    '<div class="ev-section-title">Agent 評分表 <span class="ev-section-count">' + sorted.length + ' agents</span></div>' +
    tableHtml + '</div>';

  let expSection = '<div class="panel wide" style="padding:14px 16px">' +
    '<div class="ev-section-title">實驗日誌</div>' +
    renderExperimentList(judges, promotes, true) +
    '</div>';

  el.innerHTML = renderPageGuide('ai-analysis') + scoreSection + expSection;
}

function renderScatterPlot(scorecards) {
  const container = document.getElementById('evScatterWrap');
  const canvas = document.getElementById('evScatterCanvas');
  if (!canvas || !container) return;

  const dpr = window.devicePixelRatio || 1;
  const rect = container.getBoundingClientRect();
  const W = rect.width;
  const H = Math.min(280, Math.max(200, W * 0.45));
  canvas.width = W * dpr;
  canvas.height = H * dpr;
  canvas.style.width = W + 'px';
  canvas.style.height = H + 'px';
  const ctx = canvas.getContext('2d');
  ctx.scale(dpr, dpr);

  if (!scorecards || !scorecards.length) {
    var mutedColor = getThemeColor('--muted', '#9ca3af');
    ctx.fillStyle = mutedColor;
    ctx.font = '12px ' + getComputedStyle(document.body).fontFamily;
    ctx.textAlign = 'center';
    ctx.fillText('無 Agent 數據', W / 2, H / 2);
    return;
  }

  const pad = { top: 24, right: 20, bottom: 34, left: 44 };
  const plotW = W - pad.left - pad.right;
  const plotH = H - pad.top - pad.bottom;

  // Scales: hit_rate [0,1] → X, sharpe [-1.5, 3] → Y (inverted)
  const xMin = 0, xMax = 1;
  const yMin = -1.5, yMax = 3;
  function toX(v) { return pad.left + ((v - xMin) / (xMax - xMin)) * plotW; }
  function toY(v) { return pad.top + plotH - ((v - yMin) / (yMax - yMin)) * plotH; }

  
  const layerColors = {
    sector: getThemeColor('--layer-1', '#3b82f6'),
    style: getThemeColor('--layer-2', '#8b5cf6'),
    superinvestor: getThemeColor('--layer-3', '#10b981'),
    context: getThemeColor('--layer-4', '#f59e0b'),
    control: getThemeColor('--layer-5', '#ef4444'),
    unknown: getThemeColor('--muted', '#9ca3af'),
  };

  ctx.fillStyle = getThemeColor('--bg', '#0b0d11');
  ctx.fillRect(0, 0, W, H);

  ctx.strokeStyle = getThemeColor('--border', '#242a33');
  ctx.lineWidth = 0.5;
  ctx.font = '10px monospace';
  ctx.fillStyle = getThemeColor('--muted', '#b8c4d0');
  ctx.textAlign = 'center';
  for (let v = 0; v <= 1; v += 0.25) {
    const x = toX(v);
    ctx.beginPath(); ctx.moveTo(x, pad.top); ctx.lineTo(x, pad.top + plotH); ctx.stroke();
    ctx.fillText(fmtSafeNumber(v, { percent: true, decimals: 0, suffix: '%' }), x, pad.top + plotH + 14);
  }
  ctx.textAlign = 'right';
  for (let v = -1; v <= 3; v += 1) {
    const y = toY(v);
    ctx.beginPath(); ctx.moveTo(pad.left, y); ctx.lineTo(pad.left + plotW, y); ctx.stroke();
    ctx.fillText(fmtSafeNumber(v, { decimals: 0 }), pad.left - 6, y + 4);
  }

  ctx.fillStyle = getThemeColor('--muted', '#b8c4d0');
  ctx.font = '10px sans-serif';
  ctx.textAlign = 'center';
  ctx.fillText('命中率', pad.left + plotW / 2, H - 4);
  ctx.save();
  ctx.translate(10, pad.top + plotH / 2);
  ctx.rotate(-Math.PI / 2);
  ctx.fillText('Sharpe Ratio', 0, 0);
  ctx.restore();

  ctx.strokeStyle = getThemeColor('--color-warning', '#f59e0b');
  ctx.lineWidth = 1;
  ctx.setLineDash([4, 4]);
  const y0 = toY(0);
  ctx.beginPath(); ctx.moveTo(pad.left, y0); ctx.lineTo(pad.left + plotW, y0); ctx.stroke();
  ctx.setLineDash([]);
  ctx.fillStyle = getThemeColor('--color-warning', '#f59e0b');
  ctx.textAlign = 'left';
  ctx.font = '9px sans-serif';
  ctx.fillText('Sharpe = 0', pad.left + plotW - 50, y0 - 5);

  // Vertical reference line at 60% hit rate
  ctx.strokeStyle = getThemeColor('--muted', '#9ca3af');
  ctx.lineWidth = 0.5;
  ctx.setLineDash([3, 5]);
  const x60 = toX(0.6);
  ctx.beginPath(); ctx.moveTo(x60, pad.top); ctx.lineTo(x60, pad.top + plotH); ctx.stroke();
  ctx.setLineDash([]);
  ctx.fillStyle = getThemeColor('--muted', '#9ca3af');
  ctx.textAlign = 'left';
  ctx.font = '8px sans-serif';
  ctx.fillText('60%', x60 + 2, pad.top + 10);

  var mutedColor = getThemeColor('--muted', '#9ca3af');
  var upColor = getThemeColor('--metric-good', '#10b981');
  var downColor = getThemeColor('--metric-bad', '#ef4444');

  ctx.fillStyle = hexToRgba(upColor, 0.04);
  ctx.fillRect(toX(0), toY(3), plotW * (1 / (xMax - xMin)), plotH * ((3 - 0) / (yMax - yMin)));
  ctx.fillStyle = hexToRgba(downColor, 0.04);
  ctx.fillRect(pad.left, toY(0), plotW, plotH * ((0 - yMin) / (yMax - yMin)));

  for (const a of scorecards) {
    const hr = Math.min(xMax, Math.max(xMin, a.hit_rate || 0));
    const sh = Math.min(yMax, Math.max(yMin, a.sharpe || 0));
    const x = toX(hr);
    const y = toY(sh);
    const layer = a.layer || 'unknown';
    const color = layerColors[layer] || layerColors.unknown;

    ctx.beginPath();
    ctx.arc(x, y, 10, 0, Math.PI * 2);
    ctx.fillStyle = hexToRgba(color, 0.15);
    ctx.fill();

    ctx.beginPath();
    ctx.arc(x, y, 5, 0, Math.PI * 2);
    ctx.fillStyle = color;
    ctx.fill();
    ctx.strokeStyle = getThemeColor('--panel', '#13161c');
    ctx.lineWidth = 1;
    ctx.stroke();
  }

  // Quadrant annotations
  ctx.textAlign = 'left';
  ctx.font = 'bold 10px sans-serif';
  // Top-right: good zone
  ctx.fillStyle = upColor;
  ctx.globalAlpha = 0.25;
  ctx.fillText('★ 優秀區', toX(0.65), toY(2.2));
  // Bottom-right: high hit_rate, low sharpe
  ctx.fillStyle = mutedColor;
  ctx.globalAlpha = 0.3;
  ctx.fillText('高勝率低報酬', toX(0.65), toY(0.4));
  // Top-left: low hit_rate, high sharpe  
  ctx.fillText('低勝率高報酬', toX(0.05), toY(2.2));
  // Bottom-left: weak zone
  ctx.fillStyle = downColor;
  ctx.globalAlpha = 0.25;
  ctx.fillText('⚠ 弱勢區', toX(0.05), toY(0.4));
  ctx.globalAlpha = 1;

  const legendItems = [
    { layer: 'sector', label: '產業', color: layerColors.sector },
    { layer: 'style', label: '風格', color: layerColors.style },
    { layer: 'superinvestor', label: '超級投資者', color: layerColors.superinvestor },
    { layer: 'context', label: '宏觀', color: layerColors.context },
    { layer: 'control', label: '控制', color: layerColors.control },
  ];
  const legendX = pad.left + plotW - 8;
  let legendY = pad.top + 8;
  ctx.textAlign = 'right';
  ctx.font = '9px sans-serif';
  for (const item of legendItems) {
    ctx.fillStyle = item.color;
    ctx.fillRect(legendX - 30, legendY - 5, 8, 8);
    ctx.fillStyle = getThemeColor('--muted', '#b8c4d0');
    ctx.fillText(item.label, legendX - 2, legendY + 2);
    legendY += 14;
  }

  var tip = document.getElementById('evScatterTip');
  canvas.onmousemove = function(e) {
    const rect = canvas.getBoundingClientRect();
    const mx = e.clientX - rect.left;
    const my = e.clientY - rect.top;
    let found = null;
    for (const a of scorecards) {
      const hr = Math.min(xMax, Math.max(xMin, a.hit_rate || 0));
      const sh = Math.min(yMax, Math.max(yMin, a.sharpe || 0));
      const dx = mx - toX(hr);
      const dy = my - toY(sh);
      if (Math.sqrt(dx * dx + dy * dy) < 14) { found = a; break; }
    }
    if (found && tip) {
      tip.innerHTML = '<strong>' + agentName(found.agent_id) + '</strong>' +
        '<span class="ev-tip-layer">' + escapeHtml(found.layer || '?') + '</span>' +
        '<div class="ev-tip-row"><span>命中率</span><span style="color:' + (found.hit_rate > 0.6 ? 'var(--metric-good)' : (found.hit_rate > 0.3 ? 'var(--warn)' : 'var(--muted)')) + '">' + fmtSafeNumber(found.hit_rate, { decimals: 0, suffix: '%', percent: true }) + '</span></div>' +
        '<div class="ev-tip-row"><span>Sharpe</span><span style="color:' + (found.sharpe > 1 ? 'var(--metric-good)' : (found.sharpe > 0 ? 'var(--warn)' : 'var(--metric-bad)')) + '">' + fmtSafeNumber(found.sharpe, { decimals: 2 }) + '</span></div>' +
        (found.observations ? '<div class="ev-tip-row"><span>觀察數</span><span>' + found.observations + '</span></div>' : '');
      tip.style.display = 'block';
      tip.style.left = Math.min(mx + 14, rect.width - 130) + 'px';
      tip.style.top = Math.max(my - 40, 4) + 'px';
    } else if (tip) {
      tip.style.display = 'none';
    }
  };
  canvas.onmouseleave = function() { if (tip) tip.style.display = 'none'; };
}

function renderCatContent(tab, scorecards, sessions, judges, promotes) {
  const el = document.getElementById('evolutionCatContent');
  if (!el) return;

  if (tab === 'agents') {
    el.innerHTML = '<div class="ev-section-title">Agent 競爭散布圖 <span class="ev-section-count">X: 命中率 / Y: Sharpe</span></div>' +
      '<div id="evScatterWrap" style="position:relative;width:100%;background:var(--bg);border:1px solid var(--border);border-radius:8px;overflow:hidden">' +
      '<canvas id="evScatterCanvas"></canvas>' +
      '<div id="evScatterTip" class="ev-scatter-tip"></div></div>' +
      '<div class="ev-scatter-guide">' +
        '<strong>📖 如何閱讀散布圖：</strong>每一點代表一個 AI Agent。<strong>X 軸 = 命中率</strong>（推薦成功率），<strong>Y 軸 = Sharpe</strong>（風險調整報酬）。' +
        '<strong>越往右上角越優秀</strong>（高命中 + 高報酬）。水平虛線為 Sharpe = 0（盈虧分界）。不同顏色代表不同策略層（layer）。' +
        '<div class="ev-quadrant-grid">' +
          '<div class="ev-quad good"><strong>★ 右上角：優秀區</strong><br>命中率 &gt;60% 且 Sharpe &gt;1。這些 Agent 在當前市場體制下表現最佳，最不可能被淘汰。</div>' +
          '<div class="ev-quad warn"><strong>左上角：低勝率高報酬</strong><br>命中率偏低但每次命中回報極高。可能是激進型策略，需注意風險控制。</div>' +
          '<div class="ev-quad warn"><strong>右下角：高勝率低報酬</strong><br>命中率高但 Sharpe 偏低。策略偏保守，適合穩健配置。</div>' +
          '<div class="ev-quad bad"><strong>⚠ 左下角：弱勢區</strong><br>命中率 &lt;30% 且 Sharpe &lt;0。這些 Agent 很可能在下一波進化中被淘汰。</div>' +
        '</div>' +
      '</div>';
      requestAnimationFrame(function() { renderScatterPlot(scorecards); });
  }
}

// ====== Global bridges ======
window._evSwitch = function(mode) { switchView(mode); };
