import { escapeHtml } from '../main.js';
import { agentName as getAgentName } from '../shared/constants.js';

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
  if (trend && trend.points && trend.points.length > 1) {
    const latest = trend.points[trend.points.length - 1].values;
    const previous = trend.points[trend.points.length - 2].values;
    
    for (const agent in latest) {
      const cur = latest[agent] || 1.0;
      const prev = previous[agent] || 1.0;
      if (cur > prev + 0.01) trends[agent] = 'up';
      else if (cur < prev - 0.01) trends[agent] = 'down';
      else trends[agent] = 'flat';
    }
  }

  const sortedAgents = [...agentList].sort((a, b) => b.weight - a.weight);

  let html = `
    <div class="table-wrapper">
      <table>
        <thead>
          <tr>
            <th width="60">排名</th>
            <th>Agent</th>
            <th>權重 (Weight)</th>
            <th>趨勢</th>
            <th>狀態</th>
          </tr>
        </thead>
        <tbody>
  `;

  sortedAgents.forEach((agent, index) => {
    const weight = agent.weight || 1.0;
    let statusBadge = '<span class="badge ok">Normal</span>';
    if (weight > 2.0) statusBadge = '<span class="badge info" title="高影響力">Alpha</span>';
    if (weight < 0.5) statusBadge = '<span class="badge warn" title="低影響力，可能被淘汰">At Risk</span>';

    const trendDir = trends[agent.agent_id] || 'flat';
    let trendHtml = '<span class="text-muted">→</span>';
    if (trendDir === 'up') trendHtml = '<span class="text-up">↑</span>';
    if (trendDir === 'down') trendHtml = '<span class="text-down">↓</span>';

    html += `
      <tr>
        <td>#${index + 1}</td>
        <td><strong>${escapeHtml(getAgentName(agent.agent_id))}</strong></td>
        <td>${weight.toFixed(3)}</td>
        <td>${trendHtml}</td>
        <td>${statusBadge}</td>
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

  let html = '';
  inbox.items.forEach(item => {
    const brief = item.brief || {};
    html += `
      <div class="inbox-card">
        <div class="title">${escapeHtml(item.experiment_id)}</div>
        <div class="meta">Agent: ${escapeHtml(getAgentName(brief.agent_id || 'Unknown'))}</div>
        <div class="meta">Mutation: ${escapeHtml(brief.mutation_type || 'Unknown')}</div>
        <div style="font-size:11px; color:var(--muted); margin-top:8px;">
          Created: ${new Date(item.created_at).toLocaleString()}
        </div>
      </div>
    `;
  });

  container.innerHTML = html;
}
