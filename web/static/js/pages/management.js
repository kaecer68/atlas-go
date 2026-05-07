// Management pages: inbox, experiments, alerts, industry, controls, approvals
import { agentName, sectorName, eventName } from '../names.js';

export function renderInbox(data) {
  const el = document.getElementById('experimentInbox');
  if (!data) { el.innerHTML = renderEmptyState('尚無實驗資料', '執行「go run ./cmd/run-experiment -brief &lt;file&gt;」後將自動顯示'); el.classList.remove('loading'); return; }
  el.classList.remove('loading');
  const pending = data.pending_judges || [];
  const promotes = data.pending_promotes || [];
  const history = data.recent_history || [];

  const card = (item, extra) => `
    <div class="inbox-card">
      <div class="title">${item.experiment_id}</div>
      <div class="meta">${agentName(item.target_agent_id)} · ${item.mutation_type} · 基線 ${fmt(item.baseline_value)} / 候選 ${fmt(item.candidate_value)}</div>
      ${item.mutation_summary ? `<div style="margin:3px 0;font-size:11px;color:var(--muted)">${item.mutation_summary}</div>` : ''}
      ${extra ? `<div style="margin:4px 0;font-size:11px;color:var(--muted)">${extra}</div>` : ''}
      <div class="actions">${item._actions || ''}</div>
    </div>
  `;

  const judgeActions = (id) => `
    <button onclick="judgeExperiment('${id}')">評判</button>
    <button onclick="viewDiff('${id}')">差異</button>
  `;
  const promoteActions = (id) => `
    <button class="primary" onclick="openPromote('data/state/experiments/${id}.json')">晉升</button>
    <button onclick="viewDiff('${id}')">差異</button>
  `;
  const histBadge = (s, reason) => `<span class="badge ${s==='accepted'?'ok':(s==='rejected'?'err':'warn')}">${s==='accepted'?'已接受':(s==='rejected'?'已拒絕':s)}</span>${reason ? ` <span title="${reason.replace(/"/g,'&quot;')}" style="cursor:help;border-bottom:1px dotted var(--muted)">ℹ️</span>` : ''}`;

  el.innerHTML = `
    <div class="inbox-col">
      <h3>待評判 (${pending.length})</h3>
      ${pending.length ? pending.map(p => card(p).replace('${item._actions || \'\'}', judgeActions(p.experiment_id))).join('') : renderEmptyState('無待評判實驗', '執行實驗後將自動顯示')}
    </div>
    <div class="inbox-col">
      <h3>待晉升 (${promotes.length})</h3>
      ${promotes.length ? promotes.map(p => card(p).replace('${item._actions || \'\'}', promoteActions(p.experiment_id))).join('') : renderEmptyState('無待晉升實驗', '評判通過後將自動顯示')}
    </div>
    <div class="inbox-col">
      <h3>近期歷史 (${history.length})</h3>
      ${history.length ? history.map(h => {
        const extra = h.status === 'rejected' && h.reject_reason ? `原因: ${h.reject_reason}` : '';
        return card(h, extra).replace('${item._actions || \'\'}', histBadge(h.status, h.reject_reason));
      }).join('') : renderEmptyState('無歷史紀錄', '')}
    </div>
  `;

  // Populate promote/revert dropdowns
  const promoteSel = document.getElementById('promoteSelect');
  promoteSel.innerHTML = '<option value="">-- 選擇已接受的實驗 --</option>' + promotes.map(p => `<option value="data/state/experiments/${p.experiment_id}.json">${p.experiment_id} (${agentName(p.target_agent_id)})</option>`).join('');
  if (promoteSel.options.length > 1 && promoteSel.selectedIndex === 0) {
    promoteSel.selectedIndex = 1;
  }
}
export async function loadOverrides() {
  try {
    const data = await getJSON('/api/control/active-overrides');
    const paused = data.paused_agents || [];
    const banned = data.banned_sectors || [];
    const container = document.getElementById('overrideBadges');
    const pausedBadges = paused.map(a => `<span class="badge err">⏸ ${a}</span>`).join('');
    const bannedBadges = banned.map(s => `<span class="badge warn">🚫 ${s}</span>`).join('');
    container.innerHTML = pausedBadges + bannedBadges || '<span class="text-muted text-sm">目前無生效覆寫</span>';
  } catch (e) { console.error(e); }
}

export async function loadAuditLog() {
  const el = document.getElementById('auditLog');
  try {
    const data = await getJSON('/api/control/audit-log');
    el.classList.remove('loading');
    const items = (data.interventions || []).slice(0, 20);
    if (!items.length) { el.innerHTML = renderEmptyState('無紀錄', ''); return; }
    const actionMap = {
      'pause_agent': '暫停 Agent',
      'resume_agent': '恢復 Agent',
      'set_model_weight': '設定模型權重',
      'sector_ban': '封鎖產業',
      'sector_unban': '解除封鎖',
      'approve_rec': '批准推薦',
      'reject_rec': '拒絕推薦'
    };
    el.innerHTML = `<table><thead><tr><th>時間</th><th>操作者</th><th>動作</th><th>對象</th><th>原因</th></tr></thead><tbody>
      ${items.map(it => `<tr><td>${formatDate(it.recorded_at)}</td><td>${it.operator || '-'}</td><td>${actionMap[it.type] || it.type}</td><td>${agentName(it.target_agent_id) || sectorName(it.target_sector) || it.target_symbol || it.target_model_id || '-'}</td><td>${it.reason || '-'}</td></tr>`).join('')}
    </tbody></table>`;
  } catch (e) { el.innerHTML = '<div class="empty">載入失敗</div>'; }
}

export async function loadExperimentHistory() {
  const el = document.getElementById('experimentHistory');
  try {
    const data = await getJSON('/api/experiment/history');
    el.classList.remove('loading');
    const items = (data.history || []).slice(0, 20);
    if (!items.length) { el.innerHTML = renderEmptyState('無紀錄', ''); return; }
    el.innerHTML = `<table><thead><tr><th>時間</th><th>版本</th><th>實驗</th><th>AI</th><th>狀態</th></tr></thead><tbody>
      ${items.map(it => `<tr><td>${formatDate(it.promoted_at)}</td><td>v${it.version_after || '-'}</td><td>${it.experiment_id || ''}</td><td>${agentName(it.target_agent_id) || it.target_agent_id || ''}</td><td><span class="badge ${it.status==='accepted'?'ok':'warn'}">${it.status==='accepted'?'已接受':(it.status==='rejected'?'已拒絕':it.status)}</span></td></tr>`).join('')}
    </tbody></table>`;
    const revertSel = document.getElementById('revertSelect');
    revertSel.innerHTML = '<option value="">-- 選擇要回滾的版本 --</option>' + items.map((it, i) => `<option value="${it.experiment_id}">v${it.version_after || i} - ${it.experiment_id}</option>`).join('');
  } catch (e) { el.innerHTML = '<div class="empty">載入失敗</div>'; }
}

// --- Actions ---
export async function judgeExperiment(id) {
  try {
    const res = await postJSON('/api/experiment/judge', { experiment_id: id });
    notify(`評判完成：${id} → ${res.status==='accepted'?'已接受':res.status}`, res.status==='accepted'?'ok':'info');
    loadAll();
  } catch (e) { notify('評判失敗：' + e.message, 'err'); }
}

export async function viewDiff(id) {
  try {
    const data = await getJSON('/api/experiment/diff?experiment_id=' + encodeURIComponent(id));
    document.getElementById('diffBaseline').textContent = data.baseline_prompt || '(empty)';
    document.getElementById('diffCandidate').textContent = data.candidate_prompt || '(empty)';
    document.getElementById('diffModal').classList.add('show');
  } catch (e) { notify('載入差異比對失敗：' + e.message, 'err'); }
}
export function closeModal() { document.getElementById('diffModal').classList.remove('show'); }

export let pendingPromotePath = '';
export function openPromote(path) {
  pendingPromotePath = path;
  document.getElementById('promotePreview').innerHTML = `<code>${path}</code>`;
  document.getElementById('promoteModal').classList.add('show');
}
export function closePromoteModal() { document.getElementById('promoteModal').classList.remove('show'); pendingPromotePath = ''; }

export function openKpiHelp(key) {
  const titleMap = {
    narrative: '敘事脈絡',
    regime: '市場狀態',
    weakest: '表現最差 AI',
    experiment: '實驗狀態',
    crowding: '擁擠標的'
  };
  const contentMap = {
    narrative: `<p><strong>這是什麼？</strong><br>顯示當前回測窗口中，由總經數據（利率、匯率、資金流向等）驅動的最重要敘事主題，以及外資出逃指數。它回答了「現在市場的主要故事是什麼」。</p>
<p><strong>為什麼重要？</strong><br>敘事脈絡決定了倉位規模與產業配置傾向。若出現「AI 資本支出激增」或「地緣政治緊張」等事件，會直接影響下游 Agent 的推薦權重與控制層的過濾條件。</p>
<p><strong>該注意什麼？</strong><br>點擊卡片上的「開啟宏觀敘事 →」可前往完整 6 大面板。若外資出逃指數處於「橙燈」（50-69分）或「紅燈」（70分以上），代表外資正在明顯撤離，建議同步檢視【相對趨勢】頁的總經雷達，並考慮降低整體曝險。</p>`,
    regime: `<p><strong>這是什麼？</strong><br>基於最新回測資料計算出的市場體制（RISK_ON／NEUTRAL／RISK_OFF）。這是 Context Layer 對當前環境的綜合判斷。<br><br>
<strong>請注意名稱含義：</strong>
<ul style="margin:6px 0;padding-left:18px">
<li><code>RISK_ON</code> = <span class="text-up">風險偏好（積極）</span>：市場願意承擔風險，通常是加倉機會。</li>
<li><code>NEUTRAL</code> = <span class="text-warn">中性</span>：方向不明，系統會自動縮減單一倉位上限至 85%。</li>
<li><code>RISK_OFF</code> = <span class="text-down">風險趨避（保守）</span>：市場傾向避險，這才是需要特別警惕的狀態。</li>
</ul></p>
<p><strong>為什麼重要？</strong><br>體制決定了整個投資組合的基調。RISK_ON 時可積極參與成長與動能策略；NEUTRAL 時應控制單一標的比重並提高篩選標準；RISK_OFF 時應優先考慮降低曝險、轉向防禦性配置或提高現金比重。</p>
<p><strong>該注意什麼？</strong><br>當 regime 從 RISK_ON 快速轉向 RISK_OFF，通常伴隨外資出逃指數飆升（紅燈）。此時請立即前往【相對趨勢】頁檢查總經雷達，並檢視【投資管線】是否有過多推薦被控制層阻擋——這是市場風險情緒惡化的早期信號。</p>`,
    weakest: `<p><strong>這是什麼？</strong><br>根據最新回測窗口的 Scorecard，Sharpe-like 指標最低的 Agent。它是下一輪突變實驗（Mutation）的首選候選對象。</p>
<p><strong>為什麼重要？</strong><br>Atlas-Go 的演化循環（Evolution Loop）就是持續識別最弱 Agent，並對其進行 prompt、規則或約束條件的突變改進。這不代表該 Agent 一定會被淘汰，而是指出當前最需要迭代的策略缺口。</p>
<p><strong>該注意什麼？</strong><br>在提出突變前，建議先進入【AI 觀測台】頁檢查該 Agent 的觀察數（observations）是否充足，以及失敗是否集中在特定 regime。若樣本過少，改善結論的置信度會降低。</p>`,
    experiment: `<p><strong>這是什麼？</strong><br>顯示當前待評判（judge）和待晉升（promote）的實驗數量。它是整個 mutation → judge → promote → revert 閉環的可視化入口。</p>
<p><strong>為什麼重要？</strong><br>若存在「待評判」實驗，表示已有突變運行完成但尚未決定接受或拒絕；若存在「待晉升」實驗，表示已有實驗通過門檻但尚未寫入基線政策（baseline_policy.json）。</p>
<p><strong>該注意什麼？</strong><br>停滯的實驗會阻塞下一輪改進循環。建議定期進入【模擬交易】頁進行評判與晉升。若晉升後發現績效反轉，可透過【控制與稽核】頁回滾基線版本。</p>`,
    crowding: `<p><strong>這是什麼？</strong><br>當同一標的同時被 ≥3 個 Agent 推薦，或 Style Layer 的標的池重疊過高時，CIO 層會觸發擁擠懲罰（conviction × 0.7）。此卡片列出當前被多重疊覆蓋的標的。</p>
<p><strong>為什麼重要？</strong><br>擁擠是風格趨同的信號，往往預示短期波動放大或回調風險。這幫助操作者識別「大家都愛」的熱門標的是否已過度集中。</p>
<p><strong>該注意什麼？</strong><br>高重疊不一定立刻危險，但如果疊加外資出逃指數紅燈（>70分）或 NEUTRAL regime，應特別警惕。可考慮在【投資管線】頁手動拒絕部分高擁擠標的，或進一步降低該風格 Agent 的 Darwinian 權重。</p>`
  };
  document.getElementById('infoTitle').textContent = titleMap[key] || '說明';
  document.getElementById('infoContent').innerHTML = contentMap[key] || '';
  document.getElementById('infoModal').classList.add('show');
}
export function closeInfoModal() { document.getElementById('infoModal').classList.remove('show'); }
export function openInfoHelp(title, htmlContent) {
  document.getElementById('infoTitle').textContent = title || '說明';
  document.getElementById('infoContent').innerHTML = htmlContent || '';
  document.getElementById('infoModal').classList.add('show');
}

export async function confirmPromote() {
  if (!pendingPromotePath) return;
  try {
    const res = await postJSON('/api/experiment/promote', { result_path: pendingPromotePath });
    notify(`晉升成功：基線 v${res.version}`, 'ok');
    closePromoteModal();
    loadAll();
  } catch (e) { notify('晉升失敗：' + e.message, 'err'); }
}

export async function promoteExperiment() {
  const path = document.getElementById('promoteSelect').value;
  if (!path) { notify('請先選擇一個實驗', 'warn'); return; }
  openPromote(path);
}

export async function revertExperiment() {
  const id = document.getElementById('revertSelect').value;
  const reason = document.getElementById('revertReason').value.trim();
  if (!id) { notify('請選擇要回滾的版本', 'warn'); return; }
  if (!confirm('確定要回滾到 ' + id + ' 嗎？')) return;
  try {
    await postJSON('/api/experiment/revert', { type: 'experiment', experiment_id: id, reason: reason || '儀表板回滾' });
    notify('回滾成功', 'ok');
    loadAll();
  } catch (e) { notify('回滾失敗: ' + e.message, 'err'); }
}


export async function approveRec(btn, symbol, agentID) {
  const cell = btn.parentElement;
  cell.querySelectorAll('button').forEach(b => b.disabled = true);
  btn.textContent = '…';
  try {
    await postJSON('/api/control/approve-recommendation', { symbol, agent_id: agentID, reason: '儀表板人工批准', operator: 'human' });
    btn.textContent = '✓';
    notify(`已批准 ${symbol}（${agentID}）`, 'ok');
    loadAll();
  } catch (e) {
    cell.querySelectorAll('button').forEach(b => b.disabled = false);
    btn.textContent = '✓';
    notify('批准失敗：' + e.message, 'err');
  }
}
export async function rejectRec(btn, symbol, agentID) {
  const cell = btn.parentElement;
  cell.querySelectorAll('button').forEach(b => b.disabled = true);
  btn.textContent = '…';
  try {
    await postJSON('/api/control/reject-recommendation', { symbol, agent_id: agentID, reason: '儀表板人工拒絕', operator: 'human' });
    btn.textContent = '✕';
    notify(`已拒絕 ${symbol}（${agentID}）`, 'info');
    loadAll();
  } catch (e) {
    cell.querySelectorAll('button').forEach(b => b.disabled = false);
    btn.textContent = '✕';
    notify('拒絕失敗：' + e.message, 'err');
  }
}
export async function pauseAgent() {
  const agent_id = document.getElementById('agentSelect').value;
  if (!agent_id) return;
  await postJSON('/api/control/pause-agent', { agent_id, reason: '儀表板人工暫停', operator: 'human' });
  loadOverrides(); notify('已暫停 Agent');
}
export async function resumeAgent() {
  const agent_id = document.getElementById('agentSelect').value;
  if (!agent_id) return;
  await postJSON('/api/control/resume-agent', { agent_id, reason: '儀表板人工恢復', operator: 'human' });
  loadOverrides(); notify('已恢復 Agent');
}
export async function banSector() {
  const sector = document.getElementById('sectorSelect').value;
  await postJSON('/api/control/sector-ban', { sector, banned: true, reason: '儀表板人工封鎖', operator: 'human' });
  loadOverrides(); notify('已封鎖產業');
}
export async function unbanSector() {
  const sector = document.getElementById('sectorSelect').value;
  await postJSON('/api/control/sector-ban', { sector, banned: false, reason: '儀表板人工解除封鎖', operator: 'human' });
  loadOverrides(); notify('已解除產業封鎖');
}

// --- Boot ---
export function populateAgentSelect() {
export function renderAlerts(data) {
  const el = document.getElementById('alertsPanel');
  if (!el) return;
  if (!data || !data.alerts || data.alerts.length === 0) {
    el.innerHTML = '<div class="empty" style="padding:20px 0;line-height:1.8">' +
      '<div style="font-size:14px;margin-bottom:8px">目前沒有警報</div>' +
      '<div style="font-size:12px;color:var(--muted)">' +
      '警報由系統監控模組觸發，當以下條件發生時會產生警報：<br>' +
      '• 資料通道延遲超過閾值<br>' +
      '• 系統健康度異常<br>' +
      '• 交易風險超過限制<br>' +
      '目前系統運行正常，暫無需要關注的警報。' +
      '</div></div>';
    el.classList.remove('loading');
    return;
  }
  el.classList.remove('loading');
  const severityMap = { critical: '嚴重', warning: '警告', info: '資訊' };
  const rows = data.alerts.map(a => {
    const sevClass = a.severity === 'critical' ? 'err' : a.severity === 'warning' ? 'warn' : 'info';
    const ackBtn = a.acknowledged ? '<span class="badge ok">已確認</span>' : `<button class="pipeline-action" onclick="acknowledgeAlert('${a.id}')">確認</button>`;
    return `<tr><td>${new Date(a.timestamp).toLocaleString('zh-TW')}</td><td><span class="badge ${sevClass}">${severityMap[a.severity] || a.severity}</span></td><td>${a.rule}</td><td>${a.message}</td><td>${a.value !== undefined ? a.value.toFixed(2) : '-'}</td><td>${ackBtn}</td></tr>`;
  }).join('');
  el.innerHTML = `<div style="display:flex;justify-content:flex-end;margin-bottom:6px"><button onclick="exportTableToCSV('alertsTable','alerts_export.csv')" style="font-size:11px;padding:3px 10px;border-radius:4px;border:1px solid var(--border);background:var(--bg);color:var(--text);cursor:pointer">📥 匯出 CSV</button></div><table id="alertsTable"><thead><tr><th>時間</th><th>嚴重度</th><th>規則</th><th>訊息</th><th>數值</th><th>操作</th></tr></thead><tbody>${rows}</tbody></table>`;
}

export async function acknowledgeAlert(alertId) {
  try {
    await postJSON('/api/alerts/acknowledge', { alert_id: alertId, user: 'human' });
    notify('警報已確認', 'success');
    loadAlerts();
  } catch (e) {
    notify('確認失敗: ' + e.message, 'error');
  }
}

export async function loadAlerts() {
  try {
    const data = await getJSON('/api/alerts');
    renderAlerts(data);
  } catch (e) {
    console.error(e);
  }
}

export async function loadMetrics() {
  try {
    const data = await getJSON('/api/dashboard/metrics?type=all');
    const screeningRate = data && data.screening_rate != null ? (data.screening_rate * 100).toFixed(1) + '%' : '-';
    const screeningRateEl = document.getElementById('screeningRate');
    if (screeningRateEl) screeningRateEl.textContent = screeningRate;
    const alertsTriggeredEl = document.getElementById('alertsTriggered');
    if (alertsTriggeredEl) alertsTriggeredEl.textContent = data && data.alerts_triggered != null ? data.alerts_triggered : '-';
    const capitalPhaseEl = document.getElementById('capitalPhase');
    if (capitalPhaseEl) {
      const cp = await getJSON('/api/dashboard/capital-phase').catch(() => null);
      capitalPhaseEl.textContent = cp && cp.phase ? cp.phase : 'Simulation';
    }
    updateMetricsTrend(data);
  } catch (err) {
    console.error('loadMetrics error:', err);
  }
}

export function updateMetricsTrend(data) {
  const trendDiv = document.getElementById('metricsTrend');
  if (!trendDiv) return;
  let html = '<div class="grid cols-2">';
  if (data && data.alerts_by_type && Object.keys(data.alerts_by_type).length > 0) {
    html += '<div class="panel"><h3>警報類型分佈</h3><table><thead><tr><th>類型</th><th>次數</th></tr></thead><tbody>';
    for (const [type, count] of Object.entries(data.alerts_by_type)) {
      html += `<tr><td>${type}</td><td>${count}</td></tr>`;
    }
    html += '</tbody></table></div>';
  } else {
    html += '<div class="panel"><h3>警報類型分佈</h3><div class="empty" style="font-size:12px;color:var(--muted);padding:10px 0">目前尚無警報觸發記錄。當系統偵測到異常時，此處將顯示警報分類統計。</div></div>';
  }
  html += '<div class="panel"><h3>篩選統計</h3><table><thead><tr><th>項目</th><th>數值</th></tr></thead><tbody>';
  html += `<tr><td>總數</td><td>${data && data.screening_total != null ? data.screening_total : '-'}</td></tr>`;
  html += `<tr><td>通過</td><td>${data && data.screening_passed != null ? data.screening_passed : '-'}</td></tr>`;
  html += `<tr><td>拒絕</td><td>${data && data.screening_total != null && data.screening_passed != null ? data.screening_total - data.screening_passed : '-'}</td></tr>`;
  html += '</tbody></table></div>';
  html += '</div>';
  trendDiv.innerHTML = html;
}

export async function loadIndustryData() {
  try {
    const [classification, overview, seasonality, calendar] = await Promise.all([
      getJSON('/api/dashboard/industry-classification').catch(() => null),
      getJSON('/api/dashboard/industry-overview').catch(() => null),
      getJSON('/api/dashboard/industry-seasonality').catch(() => null),
      getJSON('/api/dashboard/industry-seasonality-calendar').catch(() => null),
    ]);
    renderIndustryMap(classification);
    renderIndustryCycle(overview);
    renderIndustryLinkage(overview);
    if (seasonality && calendar) {
      seasonality.calendar = calendar;
    }
    renderIndustrySeasonality(seasonality);
  } catch (e) { console.error('loadIndustryData error:', e); }
}

export function renderIndustryMap(data) {
  const el = document.getElementById('industryMap');
  if (!data || !data.industries) { el.innerHTML = renderEmptyState('尚無產業資料', ''); el.classList.remove('loading'); return; }
  el.classList.remove('loading');
  const industries = data.industries;
  let html = '<div style="display:flex;flex-wrap:wrap;gap:10px">';
  industries.forEach(ind => {
    const weightPct = Math.round((ind.weight || 0) * 100);
    html += `<div style="flex:1;min-width:140px;background:var(--bg);border:1px solid var(--border);border-radius:8px;padding:10px;cursor:pointer" onclick="showIndustryDetail('${ind.id}')">`;
    html += `<div style="font-weight:700;font-size:14px;margin-bottom:4px">${ind.name}</div>`;
    html += `<div style="font-size:12px;color:var(--muted)">權重 ${weightPct}%</div>`;
    html += `<div style="margin-top:6px;height:4px;background:var(--border);border-radius:2px;overflow:hidden">`;
    html += `<div style="width:${weightPct}%;height:100%;background:var(--accent)"></div></div>`;
    html += `</div>`;
  });
  html += '</div>';
  el.innerHTML = html;
}

export function renderIndustryCycle(data) {
  const el = document.getElementById('industryCycle');
  if (!data || !data.industries) { el.innerHTML = renderEmptyState('尚無週期資料', ''); el.classList.remove('loading'); return; }
  el.classList.remove('loading');
  const industries = data.industries;
  const cycleColors = {
    recovery: '#10b981',
    expansion: '#3b82f6',
    mature: '#f59e0b',
    recession: '#ef4444'
  };
  const cycleNames = {
    recovery: '復甦',
    expansion: '擴張',
    mature: '成熟',
    recession: '衰退'
  };
  let html = '<div style="display:flex;flex-wrap:wrap;gap:10px">';
  industries.forEach(ind => {
    const color = cycleColors[ind.cycle_phase] || '#666';
    const name = cycleNames[ind.cycle_phase] || ind.cycle_phase;
    html += `<div style="flex:1;min-width:140px;background:var(--bg);border:1px solid var(--border);border-radius:8px;padding:10px">`;
    html += `<div style="font-weight:700;font-size:14px;margin-bottom:4px">${ind.name}</div>`;
    html += `<div style="display:flex;align-items:center;gap:6px;margin:4px 0">`;
    html += `<span style="width:10px;height:10px;border-radius:50%;background:${color};display:inline-block"></span>`;
    html += `<span style="font-size:12px">${name}</span>`;
    html += `</div>`;
    html += `<div style="font-size:11px;color:var(--muted)">信心度 ${Math.round((ind.cycle_confidence || 0) * 100)}%</div>`;
    html += `</div>`;
  });
  html += '</div>';
  el.innerHTML = html;
}

export function renderIndustryLinkage(data) {
  const el = document.getElementById('industryLinkage');
  if (!data || !data.industries) { el.innerHTML = renderEmptyState('尚無產業關聯資料', ''); el.classList.remove('loading'); return; }
  el.classList.remove('loading');
  const industries = data.industries;

  // Calculate historical averages across all industries
  let totalSystemicImportance = 0;
  let totalPropagationSpeed = 0;
  let maxSystemic = 0;
  let maxPropagation = 0;
  let count = 0;

  industries.forEach(ind => {
    const score = ind.linkage_score || {};
    const si = score.systemic_importance || 0;
    const sp = score.shock_propagation_speed || 0;
    totalSystemicImportance += si;
    totalPropagationSpeed += sp;
    if (si > maxSystemic) maxSystemic = si;
    if (sp > maxPropagation) maxPropagation = sp;
    count++;
  });

  const avgSystemic = count > 0 ? totalSystemicImportance / count : 0;
  const avgPropagation = count > 0 ? totalPropagationSpeed / count : 0;

  let html = '<div style="font-size:11px;color:var(--muted);margin-bottom:10px;padding:8px;background:var(--bg);border-radius:6px">' +
    '<strong>數據說明：</strong>「系統重要性」衡量該產業在整體經濟中的關鍵程度（0-1）；「連動分數」反映衝擊傳導速度，數值越高表示該產業受外部衝擊影響越快擴散至其他產業。' +
    '</div>';

  // Summary stats
  html += '<div style="display:flex;gap:8px;margin-bottom:12px">';
  html += `<div style="flex:1;background:var(--bg);border:1px solid var(--border);border-radius:8px;padding:10px;text-align:center">`;
  html += `<div style="font-size:11px;color:var(--muted)">平均系統重要性</div>`;
  html += `<div style="font-size:16px;font-weight:700">${avgSystemic.toFixed(2)}</div>`;
  html += `</div>`;
  html += `<div style="flex:1;background:var(--bg);border:1px solid var(--border);border-radius:8px;padding:10px;text-align:center">`;
  html += `<div style="font-size:11px;color:var(--muted)">平均連動分數</div>`;
  html += `<div style="font-size:16px;font-weight:700">${avgPropagation.toFixed(2)}</div>`;
  html += `</div>`;
  html += `<div style="flex:1;background:var(--bg);border:1px solid var(--border);border-radius:8px;padding:10px;text-align:center">`;
  html += `<div style="font-size:11px;color:var(--muted)">最高系統重要性</div>`;
  html += `<div style="font-size:16px;font-weight:700">${maxSystemic.toFixed(2)}</div>`;
  html += `</div>`;
  html += '</div>';

  html += '<table><thead><tr><th>產業</th><th>系統重要性</th><th>連動分數</th><th>相對強度</th></tr></thead><tbody>';
  industries.forEach(ind => {
    const score = ind.linkage_score || {};
    const si = score.systemic_importance || 0;
    const sp = score.shock_propagation_speed || 0;
    const siRelative = avgSystemic > 0 ? (si / avgSystemic) : 1;
    const spRelative = avgPropagation > 0 ? (sp / avgPropagation) : 1;
    const overallStrength = (siRelative + spRelative) / 2;

    let strengthLabel = '平均';
    let strengthColor = 'var(--muted)';
    if (overallStrength > 1.3) { strengthLabel = '高'; strengthColor = 'var(--up)'; }
    else if (overallStrength < 0.7) { strengthLabel = '低'; strengthColor = 'var(--down)'; }

    html += `<tr><td>${ind.name}</td><td>${si.toFixed(2)}</td><td>${sp.toFixed(2)}</td><td style="color:${strengthColor}">${strengthLabel}</td></tr>`;
  });
  html += '</tbody></table>';
  el.innerHTML = html;
}

export let seasonalityViewMode = 'list'; // 'list' or 'calendar'

export function renderIndustrySeasonality(data) {
  const el = document.getElementById('industrySeasonality');
  el.classList.remove('loading');

  const allPatterns = data && data.all_patterns ? data.all_patterns : [];
  const activePatterns = data && data.active_patterns ? data.active_patterns : [];

  let html = '<div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:10px">';
  html += '<div style="font-size:11px;color:var(--muted)">顯示所有歷史季節性模式與統計數據</div>';
  html += '<div style="display:flex;gap:4px">';
  html += `<button onclick="seasonalityViewMode='list';renderIndustrySeasonality(window.seasonalityData)" style="background:${seasonalityViewMode==='list'?'var(--accent)':'var(--bg)'};color:${seasonalityViewMode==='list'?'#fff':'var(--text)'};border:1px solid var(--border);border-radius:4px;padding:3px 10px;font-size:11px;cursor:pointer">列表</button>`;
  html += `<button onclick="seasonalityViewMode='calendar';renderIndustrySeasonality(window.seasonalityData)" style="background:${seasonalityViewMode==='calendar'?'var(--accent)':'var(--bg)'};color:${seasonalityViewMode==='calendar'?'#fff':'var(--text)'};border:1px solid var(--border);border-radius:4px;padding:3px 10px;font-size:11px;cursor:pointer">日曆</button>`;
  html += '</div></div>';

  if (seasonalityViewMode === 'calendar') {
    html += renderSeasonalityCalendar(data);
  } else {
    html += renderSeasonalityList(allPatterns, activePatterns, data);
  }

  el.innerHTML = html;
  window.seasonalityData = data;
}

export function renderSeasonalityList(allPatterns, activePatterns, data) {
  if (!allPatterns || allPatterns.length === 0) {
    return renderEmptyState('無季節性模式資料', '');
  }

  const activeIds = new Set(activePatterns.map(p => p.id));
  const today = new Date().toLocaleDateString('zh-TW');

  let html = '<table style="font-size:12px"><thead><tr><th>模式</th><th>期間</th><th>歷史準確度</th><th>典型報酬</th><th>調整因子</th><th>狀態</th></tr></thead><tbody>';
  allPatterns.forEach(p => {
    const isActive = activeIds.has(p.id);
    const statusBadge = isActive
      ? '<span class="badge ok">進行中</span>'
      : '<span class="badge info">非活躍</span>';
    const accuracy = Math.round((p.historical_accuracy || 0) * 100);
    const returnPct = ((p.typical_return || 0) * 100).toFixed(1);
    const adjustment = (p.adjustment_factor || 1.0).toFixed(2);
    const period = `${p.start_month}/${p.start_day} ~ ${p.end_month}/${p.end_day}`;

    html += `<tr style="${isActive ? 'background:rgba(79,193,255,0.05)' : ''}">`;
    html += `<td><strong>${p.name}</strong><br><span style="font-size:11px;color:var(--muted)">${p.description || ''}</span></td>`;
    html += `<td>${period}</td>`;
    html += `<td>${accuracy}%</td>`;
    html += `<td>${returnPct}%</td>`;
    html += `<td>${adjustment}x</td>`;
    html += `<td>${statusBadge}</td>`;
    html += '</tr>';
  });
  html += '</tbody></table>';

  if (activePatterns.length === 0) {
    html += `<div style="margin-top:10px;padding:10px;background:var(--bg);border-radius:6px;font-size:12px;color:var(--muted)">
      今天是 ${today}，目前無活躍模式。上表列出所有追蹤中的季節性模式供參考。
    </div>`;
  }

  return html;
}

export function renderSeasonalityCalendar(data) {
  if (!data || !data.calendar) {
    return renderEmptyState('無日曆資料', '');
  }

  const months = ['1月', '2月', '3月', '4月', '5月', '6月', '7月', '8月', '9月', '10月', '11月', '12月'];
  const calendar = data.calendar;

  let html = '<div style="display:grid;grid-template-columns:repeat(3,1fr);gap:8px">';
  calendar.months.forEach(m => {
    const monthName = months[m.month - 1];
    html += `<div style="background:var(--bg);border:1px solid var(--border);border-radius:8px;padding:10px">`;
    html += `<div style="font-weight:700;font-size:13px;margin-bottom:6px">${monthName}</div>`;
    if (m.patterns && m.patterns.length > 0) {
      m.patterns.forEach(p => {
        const accuracy = Math.round((p.historical_accuracy || 0) * 100);
        html += `<div style="font-size:11px;margin:3px 0;padding:4px;background:var(--panel);border-radius:4px">`;
        html += `<strong>${p.name}</strong> <span style="color:var(--muted)">(${accuracy}%)</span>`;
        html += `</div>`;
      });
    } else {
      html += `<div style="font-size:11px;color:var(--muted)">無相關模式</div>`;
    }
    html += `</div>`;
  });
  html += '</div>';

  return html;
}

export function showIndustryDetail(industryId) {
  notify(`產業詳細分析功能開發中: ${industryId}`, 'info');
