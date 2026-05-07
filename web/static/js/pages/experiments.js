// Experiments and human intervention controls
import { agentName, sectorName } from '../names.js';
import { getJSON, notify, formatDate } from '../shared/app-utils.js';

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

if (typeof window !== "undefined") Object.assign(window, {
  closeModal, closeInfoModal, closePromoteModal, openKpiHelp, openInfoHelp,
  confirmPromote, promoteExperiment, revertExperiment,
  pauseAgent, resumeAgent, banSector, unbanSector,
  judgeExperiment, viewDiff
});
  const select = document.getElementById('agentSelect');
  if (!select) return;
}
