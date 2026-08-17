import { agentName, stockName, regimeLabel, stressLabel, sectorName } from '../names.js';
import { narrativeThemeLabel } from '../shared/constants.js';
import { getJSON, notify, sortNarrativeEvents } from '../shared/app-utils.js';
import { escapeHtml, fmtInt } from '../shared/utils.js';
import { fmtSafeNumber, fmtSafePct, fmtSafeSignedPct, fmtSafeDrawdown } from '../shared/format-metric.js';


// Main overview dashboard
export function renderOverview(data, agentsData, inbox, overlap, narrativeEvents, stress, dataChannels, capitalPhase) {
  const gridMarket = document.getElementById('overviewMarket');
  const gridRisk = document.getElementById('overviewRisk');
  const gridSystem = document.getElementById('overviewSystem');
  if (!gridMarket || !gridRisk || !gridSystem) return;
  [gridMarket, gridRisk, gridSystem].forEach(g => g.classList.remove('loading'));
  const health = data || {};
  const cards = agentsData || {};
  const scorecards = cards.scorecards || [];
  const weakestEntry = scorecards[0];
  const weakest = weakestEntry ? weakestEntry.agent_id : '-';
  const weakSharpe = fmtSafeNumber(weakestEntry && weakestEntry.sharpe, { decimals: 3 });

  const warnings = health.warnings || [];
  const crowdingWarnings = warnings.filter(w => w.toLowerCase().includes('crowded trade') || w.toLowerCase().includes('high overlap'));

  const pendingJudges = (inbox && inbox.pending_judges) ? inbox.pending_judges.length : 0;
  const pendingPromotes = (inbox && inbox.pending_promotes) ? inbox.pending_promotes.length : 0;
  const experimentText = (pendingJudges || pendingPromotes) ? `待評判 ${pendingJudges} · 待晉升 ${pendingPromotes}` : '全部完成';

  const regime = (data && data.regime) || '-';
  const regimeColor = regime === 'RISK_ON' ? 'var(--up)' : (regime === 'RISK_OFF' ? 'var(--down)' : (regime === 'NEUTRAL' ? 'var(--warn)' : 'inherit'));

  const nev = (narrativeEvents && narrativeEvents.events) || [];
  // 以「情緒絕對值 × 信心度 × 命中率」排序，取最強烈的事件作為代表
  const sortedEvents = sortNarrativeEvents(nev.slice());
  const topEvent = sortedEvents[0];
  const stressScore = fmtSafeNumber(stress && stress.score, { decimals: 1 });
  const stressRegime = stress ? stressLabel(stress.regime || '-') : '-';
  const narrativeTitle = topEvent ? escapeHtml(narrativeThemeLabel(topEvent.theme)) : '無活躍事件';
  const narrativeSub = `外資出逃指數 ${stressScore}分（${stressRegime}）· ${nev.length} 個事件`;

  let crowdingHtml = '';
  if (crowdingWarnings.length) {
    crowdingHtml = crowdingWarnings.map(w => {
      const text = w.replace('擁擠交易：', '').replace('重疊過高：', '');
      return `<div class="my-xs text-sm text-warn">⚠ ${text}</div>`;
    }).join('');
  } else {
    crowdingHtml = '<div class="my-xs text-sm text-success">✓ 無擁擠標的</div>';
  }

  // Data channel alerts - unified from /api/dashboard/data-channels (40 channels;
  // system-health only tracked 22, leaving an 18-channel blind spot e.g. twse_etf).
  // dataChannels is the 7th renderOverview parameter; fetchWithRetry returns null
  // on failure, so guard against null to avoid crashing the overview.
  const dcData = dataChannels || {};
  const sysChannels = dcData.channels || [];
  const dcAlerts = dcData.alerts || [];
  const errorChannels = sysChannels.filter(c => c.status === 'error');
  const warnChannels = sysChannels.filter(c => c.status === 'warn');
  const partialChannels = sysChannels.filter(c => c.status === 'partial');
  const inactiveChannels = sysChannels.filter(c => c.status === 'inactive');
  const totalAlerts = errorChannels.length + warnChannels.length + partialChannels.length;
  const channelName = c => c.channel_id || '未知通道';
  const alertRows = dcAlerts.map(a => {
    const isErr = a.status === 'error';
    return `<div style="margin:2px 0;font-size:12px;color:${isErr ? 'var(--color-danger)' : 'var(--warn)'}">${isErr ? '⚠' : '◌'} ${escapeHtml(a.channel_id)} ${isErr ? '發生異常' : '資料待更新'}</div>`;
  });
  const inactiveHtml = inactiveChannels.length > 0
    ? `<div class="my-xs text-sm text-muted">◌ ${inactiveChannels.map(c => escapeHtml(channelName(c))).join('、')} 未啟用</div>`
    : '';
  const alertHtml = dataChannels == null
    ? '<div class="my-xs text-sm text-muted">◌ 通道狀態載入失敗</div>'
    : (alertRows.length > 0
      ? alertRows.join('') + inactiveHtml
      : (inactiveHtml || '<div class="my-xs text-sm text-success">✓ 所有通道正常</div>'));

  const phaseMap = { simulation: '模擬', paper: '模擬', live: '實盤', full: '全倉' };
  const phaseColor = capitalPhase ? (capitalPhase.can_advance ? 'var(--color-success)' : 'var(--warn)') : 'inherit';
  const phaseHtml = capitalPhase
    ? `<div class="my-xs text-sm text-muted">第 ${capitalPhase.days_in_phase} 天 · Sharpe ${fmtSafeNumber(capitalPhase.rolling_sharpe, { decimals: 2, useGrouping: true })}</div>`
    : '<div class="my-xs text-sm text-muted">-</div>';

  gridMarket.innerHTML = `
    <div class="kpi-card clickable" onclick="openKpiHelp('narrative')"><div class="kpi-label">敘事脈絡</div><div class="kpi-value text-lg">${narrativeTitle}</div><div class="kpi-hint">${narrativeSub}</div></div>
    <div class="kpi-card clickable" onclick="openKpiHelp('regime')"><div class="kpi-label">市場狀態</div><div class="kpi-value" style="color:${regimeColor}">${regimeLabel(regime)}</div></div>
  `;
  gridRisk.innerHTML = `
    <div class="kpi-card clickable" onclick="openKpiHelp('weakest')"><div class="kpi-label">待改進 AI 策略</div><div class="kpi-value">${agentName(weakest)}</div><div class="kpi-hint">Sharpe-like：<span style="${parseFloat(weakSharpe) < 0 ? 'color:var(--color-danger);font-weight:600' : ''}">${weakSharpe}</span></div></div>
    <div class="kpi-card ${crowdingWarnings.length ? 'alert-err' : ''} clickable" onclick="openKpiHelp('crowding')"><div class="kpi-label">擁擠標的</div><div class="kpi-value text-lg">${crowdingWarnings.length ? crowdingWarnings.length + ' 筆' : '正常'}</div>${crowdingHtml}</div>
    <div class="kpi-card ${totalAlerts > 0 ? 'alert-err' : ''} clickable" onclick="switchPage('datachannels')"><div class="kpi-label">信息通道預警</div><div class="kpi-value text-lg">${errorChannels.length > 0 ? errorChannels.length + ' 筆異常' : (warnChannels.length > 0 ? warnChannels.length + ' 筆待更新' : (partialChannels.length > 0 ? partialChannels.length + ' 筆部分異常' : '正常'))}</div>${alertHtml}</div>
  `;
  gridSystem.innerHTML = `
    <div class="kpi-card ${!health.replay_data_path_ok ? 'alert-err' : ''} clickable" onclick="${!health.replay_data_path_ok ? "openKpiHelp('replay-missing')" : "switchPage('datachannels')"}"><div class="kpi-label">資料時間</div><div class="kpi-value text-lg">${health.replay_data_latest_date || '未匯入'}</div><div class="kpi-hint">${health.replay_data_path_ok ? `最新回放數據<br>最後模擬：${health.last_window_id || '?'} / ${formatDate(health.last_window_generated_at)}` : '⚠️ 回放資料尚未匯入<br><small style="color:var(--color-danger)">點此查看匯入方式 →</small>'}</div></div>
    <div class="kpi-card ${health.baseline_version === '未知' ? 'alert-err' : ''}"><div class="kpi-label">基線版本</div><div class="kpi-value">${health.baseline_version || '?'}</div><div class="kpi-hint">${health.baseline_version === '未知' ? '⚠️ 基線策略未載入<br><small style="color:var(--muted)">確認 baseline_policy.json 存在</small>' : '目前生效的政策'}</div></div>
    <div class="kpi-card clickable" onclick="openKpiHelp('experiment')"><div class="kpi-label">實驗狀態</div><div class="kpi-value text-lg">${experimentText}</div><div class="kpi-hint">待處理項目</div></div>
    <div class="kpi-card clickable" onclick="switchPage('parameters')"><div class="kpi-label">資金階段</div><div class="kpi-value" style="color:${phaseColor};font-size:18px">${capitalPhase ? phaseMap[capitalPhase.phase] || capitalPhase.phase : '-'}</div>${phaseHtml}</div>
    <div class="kpi-card ${health.cycle_stale ? 'alert-err' : ''}"><div class="kpi-label">產業週期數據</div><div class="kpi-value text-lg">${health.cycle_stale ? '⚠️ 數據過期' : '正常'}</div><div class="kpi-hint">${health.cycle_stale ? '請檢查資料通道狀態' : '定時更新中'}</div></div>
  `;

  // Session sync alert — rendered below the KPI cards
  var sessionSyncEl = document.getElementById('sessionSyncAlert');
  if (!sessionSyncEl) {
    sessionSyncEl = document.createElement('div');
    sessionSyncEl.id = 'sessionSyncAlert';
    sessionSyncEl.style.margin = '8px 0';
    document.querySelector('.kpi-grid')?.after(sessionSyncEl);
  }
  var sessions = window.pipelineSessions || [];
  if (sessions.length) {
    var latest = sessions[0];
    var latestDate = new Date(latest.recorded_at);
    var today = new Date();
    var diffDays = Math.floor((today - latestDate) / (1000 * 60 * 60 * 24));
    if (diffDays > 1) {
      sessionSyncEl.innerHTML = '<div style="padding:8px 16px;border-radius:4px;font-size:13px;background:rgba(245,158,11,0.15);border:1px solid var(--color-warning);color:var(--color-warning);display:flex;align-items:center;gap:8px">' +
        '⚠️ 最新場次為 ' + diffDays + ' 天前（' + latestDate.toLocaleDateString('zh-TW') + '），可能已非當日同步' +
        '</div>';
    } else {
      sessionSyncEl.innerHTML = '<div style="padding:8px 16px;border-radius:4px;font-size:13px;background:rgba(16,185,129,0.15);border:1px solid var(--color-success);color:var(--color-success);display:flex;align-items:center;gap:8px">' +
        '✅ 場次已同步 · 最新：' + latestDate.toLocaleDateString('zh-TW') +
        '</div>';
    }
  }
}


// --- Utilities ---

function formatDate(d) {
  if (!d) return '-';
  const date = new Date(d);
  if (isNaN(date.getTime()) || date.getFullYear() < 2000) return '-';
  return date.toLocaleString('zh-TW');
}


export function renderEmptyState(message, action, hint) {
  return `<div class="empty-state-guidance">
    <div class="icon">📭</div>
    <div class="title">${message}</div>
    ${hint ? `<div class="desc">${hint}</div>` : ''}
    ${action ? `<div class="action">${action}</div>` : ''}
  </div>`;
}

export function renderSkeleton(lines=4) {
  let html = '';
  for (let i = 0; i < lines; i++) {
    const w = Math.random() * 30 + 50;
    html += `<div class="skeleton skeleton-line" style="width:${w}%"></div>`;
  }
  return `<div class="skeleton-block"></div><div style="padding:8px">${html}</div>`;
}


export function renderSchedulerStatus(tasks, liveness) {
  const el = document.getElementById('schedulerStatusContent');
  if (!el) return;
  if (!Array.isArray(tasks) || tasks.length === 0) {
    el.classList.remove('loading');
    el.innerHTML = '<div class="empty">目前無背景排程</div>';
    return;
  }
  // Optional second arg: /api/dashboard/task-liveness rows (cross-restart
  // persisted state). Keyed by task name for merging.
  const livenessByName = {};
  if (liveness && Array.isArray(liveness.tasks)) {
    liveness.tasks.forEach(l => { livenessByName[l.name] = l; });
  }
  const rows = tasks.map(t => {
    const l = livenessByName[t.name] || {};
    const enabledBadge = t.enabled
      ? '<span class="tier-badge tier-badge--bullish">啟用</span>'
      : '<span class="tier-badge tier-badge--neutral">停用</span>';
    const staleBadge = l.stale
      ? '<span class="tier-badge tier-badge--warn" title="' + escapeHtml(l.stale_reason || '逾期：超過 3x 間隔未執行') + '">逾期</span>'
      : '';
    const failCls = (l.consecutive_failures || t.consecutive_failures || 0) > 0 ? 'text-warn' : 'text-muted';
    const failText = (l.consecutive_failures || t.consecutive_failures || 0) > 0
      ? `${l.consecutive_failures || t.consecutive_failures} 次連續失敗`
      : '正常';
    const lastSuccess = l.last_success_at ? formatDate(l.last_success_at) : '<span class="text-muted">—</span>';
    return `<tr>
      <td>${escapeHtml(t.name || '-')}</td>
      <td><code>${escapeHtml(t.channel_id || '-')}</code></td>
      <td>${enabledBadge} ${staleBadge}</td>
      <td class="text-mono">${escapeHtml(t.interval || '-')}</td>
      <td>${formatDate(t.last_run)}</td>
      <td>${lastSuccess}</td>
      <td>${formatDate(t.next_run)}</td>
      <td class="${failCls}">${failText}</td>
    </tr>`;
  }).join('');
  el.classList.remove('loading');
  el.innerHTML = `
    <div class="table-scroll mt-sm">
      <table class="ranker-table">
        <thead>
          <tr>
            <th>任務</th>
            <th>Channel</th>
            <th>狀態</th>
            <th>間隔</th>
            <th>上次執行</th>
            <th>上次成功<span class="text-muted" title="來自 task_liveness（跨重啟持久化）">*</span></th>
            <th>下次執行</th>
            <th>連續失敗</th>
          </tr>
        </thead>
        <tbody>${rows}</tbody>
      </table>
    </div>
  `;
}

export function computePipelineSummary(guardOutcomes, items) {
  const guard = guardOutcomes || [];
  const itemList = items || [];

  if (guard.length) {
    const firstGuard = guard[0];
    const lastGuard = guard[guard.length - 1];
    const rawInputs = firstGuard ? (firstGuard.input_count || 0) : 0;
    const finalOutputs = lastGuard ? (lastGuard.output_count || 0) : 0;
    const filteredCount = Math.max(0, rawInputs - finalOutputs);
    return { rawInputs, finalOutputs, filteredCount, guard };
  }

  // Fallback: 對齊 backend pipeline.go:621-623 的 legacy PassedGuards 處理，避免孤兒 session 顯示 0/0。
  const rawInputsFromItems = itemList.length;
  const finalOutputsFromItems = itemList.filter(it => it.passed_guards !== false).length;
  const filteredCountFromItems = Math.max(0, rawInputsFromItems - finalOutputsFromItems);
  return {
    rawInputs: rawInputsFromItems,
    finalOutputs: finalOutputsFromItems,
    filteredCount: filteredCountFromItems,
    guard: [],
  };
}

export function renderMacroRadar(data, pipelineData) {
  const el = document.getElementById('macroRadar');
  if (!el) return;
  if (!data || !data.session_id) { el.innerHTML = renderEmptyState('尚無回測資料', '執行回測後將自動顯示'); el.classList.remove('loading'); return; }
  el.classList.remove('loading');
  const { rawInputs, finalOutputs, filteredCount, guard } = computePipelineSummary(data.guard_outcomes, data.items);
  const regimeColor = data.regime === 'RISK_ON' ? 'var(--up)' : (data.regime === 'RISK_OFF' ? 'var(--down)' : (data.regime === 'NEUTRAL' ? 'var(--warn)' : 'inherit'));
  const recordedAt = data.recorded_at ? formatDate(data.recorded_at) : '-';
  const items = (pipelineData && pipelineData.items) || [];
  const hasPipelinePage = !!document.getElementById('page-pipeline');

  let controlSummary = '';
  if (guard.length) {
    const lines = guard.map(g => {
      const inputCount = g.input_count || 0;
      const outputCount = g.output_count || 0;
      const filtered = inputCount - outputCount;
      if (!g.passed) return `<span class="text-danger">● ${agentName(g.guard_id)} 強制阻擋全部推薦（${inputCount} → 0）</span>`;
      if (filtered > 0) return `<span class="text-warn">● ${agentName(g.guard_id)} 過濾了 ${filtered} 筆推薦（${inputCount} → ${outputCount}）</span>`;
      return `<span class="text-success">● ${agentName(g.guard_id)} 未過濾任何推薦，全部放行</span>`;
    });
    controlSummary = `<div style="margin:8px 0;font-size:12px;line-height:1.6;background:var(--bg);padding:8px 10px;border-radius:6px">${lines.join('<br>')}</div>`;
  }

  const passedItems = items.filter(it => it.passed_guards);
  const topItems = passedItems.slice(0, 5);
  let symbolTable = '';
  if (topItems.length) {
    symbolTable = `
      <div style="margin-top:10px;font-size:13px;font-weight:700">最新回測認為可進場的標的（${finalOutputs} 檔中的前 ${topItems.length} 檔）</div>
      <table style="margin-top:6px;font-size:12px">
        <thead><tr><th>標的</th><th>公司名稱</th><th>推薦 AI</th><th>信念 <span class="cursor-pointer text-accent" onclick="event.stopPropagation();openInfoHelp('信念說明', \`<p><strong>信念（Conviction）是什麼？</strong></p><p>這是 AI Agent 對該標的推薦信心分數，範圍通常為 <strong>0 ~ 100</strong>。</p><ul style='margin:6px 0;padding-left:18px;line-height:1.8'><li><strong>&gt;70</strong>：高信念，AI 認為該標的強烈符合策略條件且風險可控。</li><li><strong>40~70</strong>：中等信念，條件部分符合，但可能存在不確定性。</li><li><strong>&lt;40</strong>：低信念，條件邊緣符合，容易被控制層過濾。</li></ul><p>當多個 AI 同時推薦同一標的時，控制層可能會對信念較低的推薦進行懲罰或過濾。</p>\`)">ℹ️</span></th><th>遠期報酬 <span class="cursor-pointer text-accent" onclick="event.stopPropagation();openInfoHelp('遠期報酬說明', \`<p><strong>遠期報酬是什麼？</strong></p><p>這是系統在回測模擬中，假設「以當日收盤價買入該標的並持有固定天數」後計算出的收益率。</p><p>數值可正可負，例如 <strong>+8.5%</strong> 代表回測中持有期間上漲，<strong>-20.9%</strong> 代表下跌。幅度大小取決於市場波動與持有期間長度。</p><p><strong>這不是未來保證</strong>，而是用來驗證 AI 推薦品質的歷史參考值。持續為正的遠期報酬代表該 AI 的選股邏輯在當前市場體制下相對有效。</p>\`)">ℹ️</span></th></tr></thead>
        <tbody>
          ${topItems.map(it => {
            const retCls = it.forward_return > 0 ? 'up' : (it.forward_return < 0 ? 'down' : '');
            return `<tr><td>${escapeHtml(it.symbol)}</td><td>${escapeHtml(stockName(it.symbol)) || '-'}</td><td>${escapeHtml(agentName(it.agent_id))}</td><td>${it.conviction != null ? escapeHtml(String(it.conviction)) : '-'}</td><td class="${retCls}">${fmtSafeSignedPct(it.forward_return)}</td></tr>`;
          }).join('')}
        </tbody>
      </table>
      ${passedItems.length > 5 ? `<div style="margin-top:4px;font-size:12px;color:var(--muted)">尚有 ${passedItems.length - 5} 檔標的${hasPipelinePage ? `，請前往<a href="#" onclick="switchPage('pipeline');return false;" style="color:var(--accent);text-decoration:underline">【投資管線】</a>查看完整清單與操作建議 →` : ''}</div>` : ''}
    `;
  } else if (finalOutputs > 0) {
    symbolTable = `<div style="margin-top:10px;font-size:12px;color:var(--warn)">控制層放行 ${finalOutputs} 筆，但投資管線暫無詳細標的資料（可能該場次尚未載入管線數據）。</div>`;
  } else if (guard.length) {
    symbolTable = renderEmptyState('本場次無被推薦標的進入模擬投組', 'AI 推薦可能全部被控制層過濾');
  }

  el.innerHTML = `
    <div class="mb-sm text-muted text-sm">
      以下為 <strong>最新回測場次</strong>（${recordedAt}）。這不是即時持倉，而是系統以當時資料與基線策略模擬後，控制層對 AI 推薦的最終處置。
    </div>
    <div class="metric"><div class="label">回測場次</div><div class="value">${data.session_id}</div></div>
    <div class="metric"><div class="label">市場狀態</div><div class="value" style="color:${regimeColor}">${regimeLabel(data.regime || '-')}</div></div>
    <div class="metric"><div class="label">推薦處置</div><div class="value">${rawInputs} 筆推薦 → 最終放行 ${finalOutputs} 筆</div></div>
    ${filteredCount > 0 ? `<div style="font-size:12px;color:var(--warn);margin:4px 0">其中 ${filteredCount} 筆因風控條件未通過而被過濾（詳見下方控制層紀錄）</div>` : ''}
    ${controlSummary}
    ${symbolTable}
    ${guard.length ? '<table style="margin-top:8px;font-size:12px"><thead><tr><th>AI</th><th>結果</th><th>處理數量</th><th>說明</th></tr></thead><tbody>' + guard.map(g => {
      const inputCount = g.input_count || 0;
      const outputCount = g.output_count || 0;
      const filtered = inputCount - outputCount;
      let actionText = '放行';
      let actionClass = 'ok';
      if (!g.passed) { actionText = '阻擋'; actionClass = 'err'; }
      else if (filtered > 0) { actionText = '過濾'; actionClass = 'warn'; }
      return `<tr><td>${escapeHtml(agentName(g.guard_id)) || '-'}</td><td><span class="badge ${actionClass}">${actionText}</span></td><td>${inputCount} → ${outputCount}</td><td>${g.reason ? escapeHtml(g.reason) : '-'}</td></tr>`;
    }).join('') + '</tbody></table>' : renderEmptyState('本場次無控制層紀錄', '')}
  `;
}

export function renderAgentObservatory(data, overlapData) {
  const el = document.getElementById('agentObservatory');
  if (!el) return;
  if (!data) { el.innerHTML = renderEmptyState('尚無資料', ''); el.classList.remove('loading'); return; }
  const cards = data.scorecards || [];
  if (!cards.length) { el.innerHTML = renderEmptyState('尚無 Agent 績效資料', ''); el.classList.remove('loading'); return; }
  el.classList.remove('loading');
  // cards are sorted by Sharpe ascending (weakest first) by BuildScorecards
  const weakest = (cards[0] && cards[0].sharpe != null) ? cards[0].agent_id : '';
  const helpIcon = (title, html) => `<span class="cursor-pointer text-accent text-sm ml-xs" onclick="event.stopPropagation();openInfoHelp('${title}', \`${html.replace(/"/g, '&quot;')}\`)">ℹ️</span>`;

  let criteriaHtml = '';
  if (overlapData && overlapData.agents) {
    const stockPickingLayers = ['sector', 'style', 'superinvestor'];
    const agentsWithCriteria = overlapData.agents.filter(a => stockPickingLayers.includes(a.layer));
    if (agentsWithCriteria.length) {
      criteriaHtml = '<div style="margin-top:14px"><div style="font-size:13px;font-weight:700;margin-bottom:8px">篩選條件</div>' +
        agentsWithCriteria.map(a => {
          const sc = a.screening_criteria || {};
          const badges = [];
          if (sc.pe && sc.pe.max != null) badges.push(`P/E≤${sc.pe.max}`);
          if (sc.pe && sc.pe.min != null && sc.pe.max == null) badges.push(`P/E≥${sc.pe.min}`);
          if (sc.pb && sc.pb.max != null) badges.push(`P/B≤${sc.pb.max}`);
          if (sc.pb && sc.pb.min != null && sc.pb.max == null) badges.push(`P/B≥${sc.pb.min}`);
          if (sc.dividend_yield && sc.dividend_yield.min != null) badges.push(`股息≥${sc.dividend_yield.min}%`);
          if (sc.volume_intraday && sc.volume_intraday.min != null) badges.push(`Vol≥${fmtSafeNumber(sc.volume_intraday.min / 10000, { decimals: 0 })}萬`);
          if (sc.momentum_20d && sc.momentum_20d.min != null) badges.push(`動能≥${sc.momentum_20d.min}`);
          if (sc.min_total_factor_score != null) badges.push(`因子≥${sc.min_total_factor_score}`);
          if (!badges.length) return '';
          return `<div style="margin:6px 0;font-size:12px"><strong>${escapeHtml(agentName(a.agent_id))}</strong> <span class="text-muted">${badges.map(b => `<span class="badge info cursor-help" title="${escapeHtml(b)}">${escapeHtml(b)}</span>`).join(' ')}</span></div>`;
        }).filter(s => s).join('') + '</div>';
    }
  }

  el.innerHTML = `<table>
    <thead><tr><th>策略來源</th><th>層級</th><th>窗口數 ${helpIcon('窗口數說明', '<p>該 Agent 參與過多少個回測窗口。</p><p>窗口數越多，統計信心度越高；窗口數過少時，績效數字可能僅供參考。</p>')}</th><th>觀察數</th><th>命中率 ${helpIcon('命中率說明', '<p>推薦產生正向隔日回測報酬的比例。</p><p>持續 >50% 代表該 Agent 的選股邏輯在當前市場體制下相對有效。</p>')}</th>        <th>Sharpe ${helpIcon('Sharpe 說明', '<p>風險調整後報酬指標。基於標準差計算，已修正先前使用 variance 的錯誤。</p><p>越高代表單位風險帶來的報酬越好；<0 表示經風險調整後整體為負貢獻（虧損）。</p>')}</th><th>95% CI</th><th>平均報酬</th><th>最大回撤 ${helpIcon('最大回撤說明', '<p>歷史推薦中曾出現的最大累積虧損幅度。</p><p>數值越接近 0，代表風險控制越好；絕對值過大時應檢查該 Agent 的停損機制。</p>')}</th><th>IS Sharpe ${helpIcon('IS Sharpe 說明', '<p>樣本內 (In-Sample) 風險調整後報酬，使用前 80% 的時間序列計算。</p><p>與 OOS Sharpe 比較可判斷策略是否過度擬合。</p>')}</th><th>OOS Sharpe ${helpIcon('OOS Sharpe 說明', '<p>樣本外 (Out-of-Sample) 風險調整後報酬，使用後 20% 的時間序列計算。</p><p>若 IS 為正但 OOS 為負，可能存在過度擬合風險。</p>')}</th><th>IS/OOS ${helpIcon('IS/OOS 說明', '<p>IS Sharpe 與 OOS Sharpe 的絕對值比率。</p><p>比率 > 2.0 或 IS>0 但 OOS≤0 時觸發過適度 (Overfit) 警告。</p>')}</th></tr></thead>
    <tbody>
      ${cards.map(c => {
        const isWeak = c.agent_id === weakest;
        const sigWarn = (c.windows || 0) < 20 ? '<span title="窗口數不足，統計信心有限" style="color:var(--warn);font-size:10px">⚠️</span>' : '';
        const ciLow = fmtSafeNumber(c.confidence_low, { decimals: 3 });
        const ciHigh = fmtSafeNumber(c.confidence_high, { decimals: 3 });
        const trendVal = c.rolling_sharpe_trend;
        let trendIcon = '';
        if (trendVal != null && Math.abs(trendVal) > 0.001) {
          if (trendVal > 0) {
            trendIcon = `<span title="趨勢向上 (rolling_sharpe_trend=${fmtSafeNumber(trendVal, { decimals: 4 })})" style="color:var(--up);font-size:12px">↗</span>`;
          } else {
            trendIcon = `<span title="趨勢向下 (rolling_sharpe_trend=${fmtSafeNumber(trendVal, { decimals: 4 })})" style="color:var(--down);font-size:12px">↘</span>`;
          }
        } else if (trendVal != null) {
          trendIcon = `<span title="趨勢平穩 (rolling_sharpe_trend=${fmtSafeNumber(trendVal, { decimals: 4 })})" style="color:var(--muted);font-size:12px">→</span>`;
        }
        const oosWarn = c.oos_sample_warning ? `<span title="${escapeHtml(c.oos_sample_warning)}" style="color:var(--warn);font-size:10px">⚠️</span>` : '';
        const overfitBadge = c.overfit_warning ? `<span title="${escapeHtml(c.overfit_reason || '')}" style="color:var(--color-danger);font-size:10px;margin-left:4px">⚠️</span>` : '';
        const isSharpeStr = fmtSafeNumber(c.is_sharpe, { decimals: 3 });
        const oosSharpeStr = fmtSafeNumber(c.oos_sharpe, { decimals: 3 });
        const isOosStr = fmtSafeNumber(c.is_oos_ratio, { decimals: 2 });
        return `<tr class="${isWeak ? 'weak' : ''}">
          <td>${agentName(c.agent_id) || ''} ${trendIcon} ${sigWarn}</td>
          <td>${c.layer || '-'}</td>
          <td>${fmtInt(c.windows)}</td>
          <td>${fmtInt(c.observations)}</td>
          <td>${fmtSafePct(c.hit_rate)}</td>
          <td style="${c.sharpe != null && c.sharpe < 0 ? 'color:var(--color-danger)' : ''}">${fmtSafeNumber(c.sharpe, { decimals: 3 })}</td>
          <td class="text-muted text-xs">[${ciLow}, ${ciHigh}]</td>
          <td>${fmtSafeSignedPct(c.average_return)}</td>
          <td>${fmtSafeDrawdown(c.max_drawdown)}</td>
          <td style="${c.is_sharpe != null && c.is_sharpe < 0 ? 'color:var(--color-danger)' : ''}">${isSharpeStr}${oosWarn}</td>
          <td style="${c.oos_sharpe != null && c.oos_sharpe < 0 ? 'color:var(--color-danger)' : ''}">${oosSharpeStr}${overfitBadge}</td>
          <td>${isOosStr}</td>
        </tr>`;
      }).join('')}
    </tbody>
  </table>
  ${weakest ? `<div style="margin-top:8px;font-size:12px;color:var(--color-danger)">待改進策略來源：<strong>${agentName(weakest)}</strong></div>` : ''}
  ${data.recorded_at ? `<div style="margin-top:4px;font-size:11px;color:var(--muted)">數據時間：${new Date(data.recorded_at).toLocaleString('zh-TW')}</div>` : ''}
  ${criteriaHtml}`;
}


export function renderUniverseOverlap(data) {
  const el = document.getElementById('universeOverlap');
  if (!el) return;
  if (!data || !data.agents) { el.innerHTML = renderEmptyState('尚無資料', ''); el.classList.remove('loading'); return; }
  el.classList.remove('loading');
  const agents = data.agents || [];
  const matrix = data.matrix || {};
  const warnings = data.warnings || [];
  const styleAgents = agents.filter(a => a.layer === 'style');
  const uoHelp = (title, html) => `<span class="cursor-pointer text-accent text-sm ml-xs" onclick="event.stopPropagation();openInfoHelp('${title}', \`${html.replace(/"/g, '&quot;')}\`)">ℹ️</span>`;
  el.innerHTML = `
    <div style="margin-bottom:10px;font-size:12px;color:var(--text);line-height:1.6">
      <strong>如何解讀本區塊：</strong>以下顯示各策略來源（Agent）設定的關注標的池，以及 <strong>Style 層</strong> 之間的標的重疊程度。當兩個 Style Agent 同時關注 >=3 檔相同標的時，CIO 層會自動施加信念懲罰（擁擠交易警告）。
    </div>
    ${warnings.length ? `<div class="mb-sm">${warnings.map(w => `<span class="badge warn">⚠ ${w}</span>`).join('')}</div>` : ''}
    <div class="two-col-grid">
      <div>
        <div class="mb-xs text-sm"><strong>策略來源標的池</strong></div>
        <table>
          <thead><tr><th>策略來源 ${uoHelp('策略來源', '<p>Agent 的中文名稱，對應 <code>configs/agents.json</code> 中的 <code>name</code> 欄位。</p><p>同一策略技能未來可能由多個 Agent 競爭執行。</p>')}</th><th>層級 ${uoHelp('層級', '<p>Agent 在 Atlas 分層架構中的所屬層級。</p><ul style="margin:6px 0;padding-left:18px;line-height:1.8"><li><strong>產業主題</strong>（sector）：決定板塊配置。</li><li><strong>風格因子</strong>（style）：決定成長／價值／動能等風格傾向。</li><li><strong>超級投資者</strong>（superinvestor）：模擬特定投資大師的選股邏輯。</li><li><strong>總經情境</strong>（context）：根據總經 regime 產生方向性建議，不直接選股。</li><li><strong>控制層</strong>（control）：風控長／投資長，負責過濾與後置風控。</li></ul>')}</th><th>數量 ${uoHelp('數量', '<p>該 Agent 關注的股票檔數。</p><p>若 Agent 未在設定中指定標的池，會自動 fallback 至系統預設的 24 檔台股。</p><p>數量過少可能導致策略覆蓋不足；過多則可能失去聚焦。</p>')}</th><th>標的 ${uoHelp('標的', '<p>該 Agent 會進行評估與推薦的具體股票代碼清單。</p><p>此清單來自 <code>configs/agents.json</code> 的 <code>universe</code> 欄位。</p>')}</th></tr></thead>
          <tbody>
            ${agents.map(a => `<tr><td>${agentName(a.agent_id) || a.agent_id}</td><td>${a.layer || '-'}</td><td>${a.universe ? a.universe.length : 0}</td><td class="text-muted text-xs">${(a.universe || []).join(', ')}</td></tr>`).join('')}
          </tbody>
        </table>
      </div>
      <div>
        <div class="mb-xs text-sm"><strong>Style 層重疊矩陣</strong></div>
        ${styleAgents.length ? `<table>
          <thead><tr><th>策略來源 ${uoHelp('策略來源（列）', '<p>矩陣中的每一列代表一個 Style 層 Agent。</p><p>交叉格子顯示的是「該列 Agent」與「該欄 Agent」同時關注的相同標的數量。</p>')}</th>${styleAgents.map(b => `<th>${agentName(b.agent_id)} ${uoHelp(agentName(b.agent_id), '<p>Style 層 Agent 之一。</p><p>列與欄的交叉數字代表兩個 Agent 標的池的重疊檔數。</p>')}</th>`).join('')}</tr></thead>
          <tbody>
            ${styleAgents.map(a => `<tr><td>${agentName(a.agent_id)}</td>${styleAgents.map(b => `<td style="${a.agent_id === b.agent_id ? 'background:var(--bg)' : (matrix[a.agent_id] && matrix[a.agent_id][b.agent_id] >= 3 ? 'color:var(--warn);font-weight:700' : '')}">${a.agent_id === b.agent_id ? '-' : (matrix[a.agent_id] && matrix[a.agent_id][b.agent_id] || 0)}</td>`).join('')}</tr>`).join('')}
          </tbody>
        </table>` : renderEmptyState('無風格層 Agent', '請在 configs/agents.json 中設定 style 層 Agent')}
      </div>
    </div>
  `;
}
export function factorBar(score, minVal, maxVal) {
  if (score == null || isNaN(score)) return '<span class="text-muted">-</span>';
  const range = maxVal - minVal;
  const pct = Math.max(0, Math.min(100, ((score - minVal) / range) * 100));
  let color = 'var(--warn)';
  if (score >= (minVal + range * 0.6)) color = 'var(--up)';
  else if (score <= (minVal + range * 0.4)) color = 'var(--down)';
  return `<div class="factor-bar-bg" title="${fmtSafeNumber(score, { decimals: 3 })}"><div style="width:${pct}%;height:100%;background:${color}"></div></div>`;
}
export function renderFactorMini(fs) {
  if (!fs || fs.total == null || isNaN(fs.total)) return '<span class="text-muted">-</span>';
  const t = fs.total;
  let color = 'var(--warn)';
  if (t >= 0.5) color = 'var(--up)';
  else if (t <= 0) color = 'var(--down)';
  const pct = Math.max(0, Math.min(100, ((t + 1) / 2) * 100));
  return `<div class="factor-mini"><div class="factor-mini-bar"><div style="width:${pct}%;background:${color}"></div></div><span class="factor-mini-val" style="${t >= 0.5 ? 'color:var(--up)' : (t <= 0 ? 'color:var(--down)' : '')}">${fmtSafeNumber(t, { decimals: 2 })}</span></div>`;
}
export function renderFactorBreakdown(breakdown) {
  if (!breakdown) return '<div class="text-muted text-xs">無計算明細</div>';
  const item = (label, it) => {
    if (!it) return '';
    const inputs = it.raw_inputs ? Object.entries(it.raw_inputs).map(([k, v]) => `${k}: ${typeof v === 'number' ? fmtSafeNumber(v, { decimals: 3 }) : v}`).join(', ') : '';
    const fallback = it.is_fallback ? '<span style="color:var(--warn);font-size:10px">fallback</span> ' : '';
    const weight = it.weight ? `<span style="color:var(--muted);font-size:10px">權重 ${fmtSafeNumber(it.weight, { decimals: 2 })}</span> ` : '';
    return `<div style="margin:4px 0;padding:4px 6px;background:var(--bg);border-radius:4px">
      <div class="text-xs font-semibold">${label} ${fallback}${weight}= <span class="text-accent">${it.score != null ? fmtSafeNumber(it.score, { decimals: 3 }) : '-'}</span></div>
      <div style="font-size:10px;color:var(--muted);margin-top:2px">公式: ${it.formula || '-'}</div>
      ${inputs ? `<div class="text-muted text-xs">原始輸入: ${inputs}</div>` : ''}
    </div>`;
  };
  return `<div class="py-sm">
    ${item('動能', breakdown.momentum)}
    ${item('價值', breakdown.value)}
    ${item('品質', breakdown.quality)}
    ${item('Agent', breakdown.agent)}
    ${item('總分', breakdown.total)}
  </div>`;
}
export function toggleBreakdown(key) {
  const row = document.getElementById('breakdown-' + key);
  const btn = document.getElementById('btn-' + key);
  if (!row || !btn) return;

  const isHidden = row.style.display === 'none' || row.classList.contains('hidden') || getComputedStyle(row).display === 'none';

  if (isHidden) {
    row.style.display = 'table-row';
    row.classList.remove('hidden');
    btn.textContent = '收起';
  } else {
    row.style.display = 'none';
    btn.textContent = '展開';
  }
}
if (typeof window !== 'undefined') window.toggleBreakdown = toggleBreakdown;
export function renderAIEvolution(inbox, phase3, darwinianStatus, darwinianTrend, agents, macro, stress) {
  const el = document.getElementById('aiEvolution');
  if (!el) return;
  el.classList.remove('loading');
  const items = (inbox && inbox.items) ? inbox.items : [];
  const pending = items.filter(i => i.status === 'pending' || i.status === 'planned');
  const latest = items.slice(0, 3);

  const prismCompleted = phase3 && phase3.prism_completed_results != null ? phase3.prism_completed_results : (phase3 && phase3.PRISMCompletedResults != null ? phase3.PRISMCompletedResults : '-');

  let agentList = [];
  if (darwinianStatus && darwinianStatus.agents) {
    agentList = Object.keys(darwinianStatus.agents).map(id => Object.assign({agent_id: id}, darwinianStatus.agents[id]));
  }
  const topAgent = agentList.length > 0 ? agentList.reduce((a, b) => (b.weight || 0) > (a.weight || 0) ? b : a) : null;
  const avgWeight = agentList.length > 0 ? fmtFloat(agentList.reduce((s, a) => s + (a.weight || 0), 0) / agentList.length) : '-';

  const regime = (macro && macro.regime) || 'NEUTRAL';
  const regimeColor = regime === 'RISK_ON' ? 'var(--up)' : (regime === 'RISK_OFF' ? 'var(--down)' : 'var(--warn)');

  const stressVal = (stress && typeof stress.score === 'number') ? stress.score : null;
  const stressLabel = stressVal >= 70 ? '危機' : (stressVal >= 50 ? '高壓' : (stressVal >= 30 ? '警戒' : '低壓'));
  const stressColor = stressVal >= 70 ? 'var(--color-danger)' : (stressVal >= 50 ? 'var(--warn)' : 'var(--color-success)');

  const scorecards = (agents && agents.scorecards) ? agents.scorecards : [];
  const healthyCount = scorecards.filter(a => (a.sharpe || 0) > 0.5 && (a.hit_rate || 0) > 0.3).length;
  const healthPct = scorecards.length > 0 ? Math.round(healthyCount / scorecards.length * 100) : 0;
  const healthColor = healthPct > 70 ? 'var(--color-success)' : (healthPct > 40 ? 'var(--warn)' : 'var(--color-danger)');

  el.innerHTML = `
    <div style="display:grid;grid-template-columns:repeat(6,1fr);gap:10px;margin-bottom:12px">
      <div class="panel-card" style="text-align:center">
        <div class="text-sm text-muted mb-xs">市場狀態</div>
        <div class="text-xl font-bold" style="color:${regimeColor}">${regimeLabel(regime)}</div>
        <div class="text-xs text-muted mt-xs">Regime 訊號</div>
      </div>
      <div class="panel-card" style="text-align:center">
        <div class="text-sm text-muted mb-xs">外資出逃指數</div>
        <div class="text-xl font-bold" style="color:${stressColor}">${fmtSafeNumber(stressVal, { decimals: 1 })}</div>
        <div class="text-xs text-muted mt-xs">${stressVal != null ? stressLabel : '無資料'}</div>
      </div>
      <div class="panel-card" style="text-align:center">
        <div class="text-sm text-muted mb-xs">策略健康度</div>
        <div class="text-xl font-bold" style="color:${healthColor}">${healthPct}%</div>
        <div class="text-xs text-muted mt-xs">${healthyCount}/${scorecards.length} Agent 達標</div>
      </div>
      <div class="panel-card" style="text-align:center">
        <div class="text-sm text-muted mb-xs">Darwinian 權重</div>
        <div class="text-xl font-bold">${avgWeight}</div>
        <div class="text-xs text-muted mt-xs">平均權重</div>
      </div>
      <div class="panel-card" style="text-align:center">
        <div class="text-sm text-muted mb-xs">待評判實驗</div>
        <div class="text-xl font-bold" style="color:${pending.length > 0 ? 'var(--warn)' : 'var(--muted)'}">${pending.length}</div>
        <div class="text-xs text-muted mt-xs">共 ${items.length} 個歷史實驗</div>
      </div>
    </div>
    ${topAgent ? `
    <div style="display:flex;gap:12px;align-items:center;padding:8px 12px;background:var(--bg);border-radius:8px;margin-bottom:12px;border-left:3px solid var(--up)">
      <span style="font-size:12px;color:var(--muted)">🏆 最強 Agent</span>
      <span style="font-size:13px;font-weight:700">${escapeHtml(agentName(topAgent.agent_id))}</span>
      <span style="font-size:11px;color:var(--muted)">權重 ${fmtSafeNumber(topAgent.weight, { decimals: 2, useGrouping: true })} · 命中率 ${fmtSafePct(topAgent.hit_rate)}</span>
    </div>` : ''}
    ${latest.length ? `
    <div>
      <div class="text-sm font-bold mb-xs">最近實驗</div>
      <div style="display:flex;gap:8px;flex-wrap:wrap">
        ${latest.map(it => `<span class="badge info">${it.experiment_id} · ${it.mutation_type || it.target_agent_id || '實驗'}</span>`).join(' ')}
      </div>
    </div>` : ''}
  `;
}

