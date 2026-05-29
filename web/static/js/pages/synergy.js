import { escapeHtml } from '../main.js';
import { agentName as getAgentName } from '../shared/constants.js';

const MUTATION_TYPE_MAP = {
  'prompt_tightening': '策略收緊',
  'prompt_relaxation': '策略放寬',
  'constraint_tightening': '限制收緊',
  'constraint_relaxation': '限制放寬',
  'risk_rule_update': '風控更新',
  'risk_rule_change': '風控調整',
  'portfolio_constraint_revision': '投組治理',
  'portfolio_constraint': '投組約束',
  'governance_routing': '治理路由',
  'volume_filter': '成交量篩選',
  'conviction_adjustment': '信念調整',
  'parameter_sweep': '參數掃描',
  'promote_spawned': '晉升候選',
};

function mutationName(type) {
  return MUTATION_TYPE_MAP[type] || type || '';
}

function mutationDescription(type, skill) {
  const skillName = getAgentName(skill) || skill || '';
  const m = {
    'prompt_tightening': `${skillName} 的選股條件已被系統自動收緊，以提高推薦品質。`,
    'prompt_relaxation': `${skillName} 的選股條件已被系統自動放寬，以增加機會覆蓋。`,
    'risk_rule_change': `${skillName} 的風險閾值已被系統自動調整，以優化風險回報。`,
    'risk_rule_update': `${skillName} 的風控規則已被系統更新。`,
    'portfolio_constraint_revision': `${skillName} 的投組治理限制已被重新審視。`,
    'portfolio_constraint': `${skillName} 的投組部位限制已被調整。`,
    'governance_routing': `${skillName} 的執行路由已被調整。`,
    'volume_filter': `${skillName} 的成交量篩選門檻已被調整。`,
    'conviction_adjustment': `${skillName} 的信念值計算參數已被調整。`,
    'parameter_sweep': `${skillName} 的參數已被系統掃描優化。`,
    'promote_spawned': `${skillName} 新生成代理表現優異，已被晉升。`,
    'constraint_tightening': `${skillName} 的限制條件已被收緊。`,
    'constraint_relaxation': `${skillName} 的限制條件已被放寬。`,
  };
  return m[type] || `系統已對 ${skillName} 進行自動優化調整。`;
}

function fmtPct(v) {
  if (v == null || isNaN(v)) return '-';
  return (v * 100).toFixed(1) + '%';
}

function fmtVal(v) {
  if (v == null || isNaN(v)) return '-';
  if (Math.abs(v) < 0.001) return v.toExponential(2);
  return v.toFixed(4);
}

function fmtMoney(v) {
  if (v == null || isNaN(v) || v === 0) return '';
  return 'NT$' + Math.round(v).toLocaleString('zh-TW');
}

function statusBadge(status) {
  switch (status) {
    case 'accepted': return '<span class="badge ok">已接受</span>';
    case 'rejected': return '<span class="badge err">已拒絕</span>';
    case 'expired': return '<span class="badge warn">已過期</span>';
    case 'running': return '<span class="badge info">執行中</span>';
    case 'planned': return '<span class="badge info">已規劃</span>';
    default: return '<span class="badge warn">待處理</span>';
  }
}

export function renderSynergyPage(darwinianStatus, darwinianTrend, inbox) {
  renderLeaderboard(darwinianStatus, darwinianTrend);
  renderCandidates(inbox);
}

function renderLeaderboard(status, trend) {
  const container = document.getElementById('synergyLeaderboard');
  if (!container) return;

  const agentList = agentListFromStatus(status);
  if (!agentList.length) {
    container.className = 'empty';
    container.innerHTML = '<div class="empty">尚無 Darwinian 權重資料</div>';
    return;
  }

  container.className = '';

  const trends = {};
  if (trend && trend.points && trend.points.length > 0) {
    const grouped = {};
    for (const p of trend.points) {
      if (!p.agent_id) continue;
      if (!grouped[p.agent_id]) grouped[p.agent_id] = [];
      grouped[p.agent_id].push(p.weight || 1.0);
    }
    for (const aid in grouped) {
      const weights = grouped[aid];
      if (weights.length < 2) { trends[aid] = 'flat'; continue; }
      const cur = weights[0];
      const prev = weights[1];
      if (cur > prev + 0.01) trends[aid] = 'up';
      else if (cur < prev - 0.01) trends[aid] = 'down';
      else trends[aid] = 'flat';
    }
  }

  const sortedAgents = [...agentList].sort((a, b) => b.weight - a.weight);

  const lastComputed = status && status.last_computed ? status.last_computed : '';
  const metaHtml = lastComputed
    ? `<div style="font-size:11px;color:var(--muted);margin-bottom:8px">最後計算：${escapeHtml(lastComputed)} · 共 ${agentList.length} 個 Agent · 權重範圍 [0.3, 2.5]</div>`
    : '';

  let html = `
    ${metaHtml}
    <div class="table-wrapper">
      <table>
        <thead>
          <tr>
            <th width="40">排名</th>
            <th>Agent</th>
            <th>權重</th>
            <th>Sharpe</th>
            <th>命中率</th>
            <th>信號數</th>
            <th>勝/敗</th>
            <th>均報酬</th>
            <th>趨勢</th>
            <th>狀態</th>
          </tr>
        </thead>
        <tbody>
  `;

  sortedAgents.forEach((agent, index) => {
    const weight = agent.weight || 1.0;
    let statusBadgeHtml = '<span class="badge ok">正常</span>';
    if (weight >= 2.5) statusBadgeHtml = '<span class="badge info" title="已達權重上限，高影響力">最強 Alpha</span>';
    else if (weight > 2.0) statusBadgeHtml = '<span class="badge info" title="高影響力">Alpha</span>';
    else if (weight <= 0.3) statusBadgeHtml = '<span class="badge err" title="已達權重下限，面臨淘汰">淘汰邊緣</span>';
    else if (weight < 0.5) statusBadgeHtml = '<span class="badge warn" title="低影響力，可能被淘汰">高風險</span>';

    const trendDir = trends[agent.agent_id] || 'flat';
    let trendHtml = '<span class="text-muted">→</span>';
    if (trendDir === 'up') trendHtml = '<span class="text-up">↑</span>';
    if (trendDir === 'down') trendHtml = '<span class="text-down">↓</span>';

    const sharpe = agent.rolling_sharpe || 0;
    const totalSignals = agent.total_signals || 0;
    const sharpeHtml = sharpe > 0
      ? `<span style="color:var(--up)">${sharpe.toFixed(2)}</span>`
      : (sharpe < 0 ? `<span style="color:var(--down)">${sharpe.toFixed(2)}</span>` : '<span class="text-muted">N/A</span>');

    const hitRate = agent.hit_rate || 0;
    const hitRateHtml = hitRate >= 0.6
      ? `<span style="color:var(--up)">${fmtPct(hitRate)}</span>`
      : (hitRate >= 0.4 ? fmtPct(hitRate) : `<span style="color:var(--down)">${fmtPct(hitRate)}</span>`);

    const avgReturn = agent.avg_return || 0;
    const avgReturnHtml = avgReturn > 0
      ? `<span style="color:var(--up)">${fmtVal(avgReturn)}</span>`
      : (avgReturn < 0 ? `<span style="color:var(--down)">${fmtVal(avgReturn)}</span>` : fmtVal(avgReturn));

    html += `
      <tr>
        <td>#${index + 1}</td>
        <td><strong>${escapeHtml(getAgentName(agent.agent_id))}</strong></td>
        <td>${weight.toFixed(3)}</td>
        <td>${sharpeHtml}</td>
        <td>${hitRateHtml}</td>
        <td>${agent.total_signals || 0}</td>
        <td>${agent.win_count || 0}/${agent.loss_count || 0}</td>
        <td>${avgReturnHtml}</td>
        <td>${trendHtml}</td>
        <td>${statusBadgeHtml}</td>
      </tr>
    `;
  });

  html += `
        </tbody>
      </table>
    </div>
  `;

  container.innerHTML = html;
}

function agentListFromStatus(status) {
  if (!status || !status.agents) return [];
  return Object.keys(status.agents).map(function(id) {
    return Object.assign({agent_id: id}, status.agents[id]);
  });
}

function renderCandidates(inbox) {
  const container = document.getElementById('synergyInbox');
  if (!container) return;

  if (!inbox || !inbox.items || inbox.items.length === 0) {
    container.innerHTML = '<div class="empty" style="grid-column:1/-1;text-align:center">目前沒有新的實驗候選者</div>';
    return;
  }

  const versionHtml = inbox.baseline_version ? `<div style="font-size:11px;color:var(--muted);margin-bottom:4px">基線版本：v${inbox.baseline_version}</div>` : '';
  const explainHtml = '<div style="font-size:10px;color:var(--muted);margin-bottom:8px;line-height:1.4">系統每日自動選出表現最弱的 Agent 作為實驗候選(planned)。每 7 天執行測試後更新結果。下方為每個 Agent 的最新候選。</div>';

  // Sort items into sections: planned, tested (accepted/rejected), history
  const planned = [], tested = [], history = [];
  inbox.items.forEach(item => {
    const s = item.status || '';
    if (s === 'planned' || s === 'running') planned.push(item);
    else if (s === 'accepted' || s === 'rejected') tested.push(item);
    else history.push(item);
  });

  let html = versionHtml + explainHtml;

  function renderSection(title, items, color) {
    if (!items.length) return '';
    let h = `<div style="margin-bottom:12px"><div style="font-weight:700;color:${color};margin-bottom:6px;font-size:13px">${title}（${items.length}）</div>`;
    items.forEach(item => renderCard(item));
    h += '</div>';
    return h;
  }

  let cardHtml = '';
  function renderCard(item) {
    const agentId = item.target_agent_id || '';
    const mutation = item.mutation_type || '';
    const summary = item.mutation_summary || '';
    const status = item.status || '';

    const bv = item.baseline_value;
    const cv = item.candidate_value;
    let compareHtml = '';
    if (status === 'planned') {
      compareHtml = `<div class="meta" style="font-size:11px">基線 SharpeLike ${fmtVal(bv)} → 候選 <span style="color:var(--warn)">待測試</span></div>`;
    } else if (bv != null && cv != null && (bv !== 0 || cv !== 0)) {
      const better = cv > bv;
      compareHtml = `<div class="meta" style="font-size:11px">基線 ${fmtVal(bv)} → 候選 <span style="color:${better ? 'var(--up)' : 'var(--down)'}">${fmtVal(cv)}</span></div>`;
    }

    const bMoney = item.baseline_monetary_ntd;
    const cMoney = item.candidate_monetary_ntd;
    let moneyHtml = '';
    if (bMoney && cMoney) {
      moneyHtml = `<div class="meta" style="font-size:11px">基線 ${fmtMoney(bMoney)} → 候選 ${fmtMoney(cMoney)}</div>`;
    }

    let rejectHtml = '';
    if (status === 'rejected' && item.reject_reason) {
      rejectHtml = `<div style="font-size:10px;color:var(--down);margin-top:2px">拒絕原因：${escapeHtml(item.reject_reason)}</div>`;
    }

    cardHtml += `
      <div class="inbox-card">
        <div class="title">${escapeHtml(item.experiment_id)}</div>
        <div class="meta">${escapeHtml(getAgentName(agentId))} ${statusBadge(status)}</div>
        <div class="meta"><strong>${escapeHtml(mutationName(mutation))}</strong></div>
        <div style="font-size:11px;color:var(--muted);margin:4px 0;line-height:1.5">${escapeHtml(mutationDescription(mutation, item.skill))}</div>
        ${compareHtml}
        ${moneyHtml}
        ${summary ? `<div style="font-size:10px;color:var(--muted);margin-top:4px;word-break:break-all">${escapeHtml(summary)}</div>` : ''}
        ${rejectHtml}
      </div>
    `;
  }

  html += renderSection('📋 待測試（系統已選出，等待 auto_experiment 執行）', planned, 'var(--warn)');
  html += renderSection('✅ 已測試（含結果比較）', tested, 'var(--up)');
  html += renderSection('📁 歷史記錄', history, 'var(--muted)');

  container.innerHTML = html;
}
