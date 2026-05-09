// Inbox / experiment overview page
import { getJSON, formatDate } from '../shared/app-utils.js';
import { agentName } from '../names.js';
import { escapeHtml } from '../shared/utils.js';

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
        const extra = h.status === 'rejected' && h.reject_reason ? `原因: ${escapeHtml(h.reject_reason)}` : '';
        return card(h, extra).replace('${item._actions || \'\'}', histBadge(h.status, h.reject_reason));
      }).join('') : renderEmptyState('無歷史紀錄', '')}
    </div>
  `;

  // Populate promote/revert dropdowns
  const promoteSel = document.getElementById('promoteSelect');
  promoteSel.innerHTML = '<option value="">-- 選擇已接受的實驗 --</option>' + promotes.map(p => `<option value="data/state/experiments/${escapeHtml(p.experiment_id)}.json">${escapeHtml(p.experiment_id)} (${escapeHtml(agentName(p.target_agent_id))})</option>`).join('');
  if (promoteSel.options.length > 1 && promoteSel.selectedIndex === 0) {
    promoteSel.selectedIndex = 1;
  }
}
