import { escapeHtml } from '../shared/utils.js';
import { renderStockCell } from '../names.js';

export async function loadSwarmData() {
  const [status, consensus, anomalies, scenarios, strategies] = await Promise.all([
    fetch('/api/dashboard/swarm-status').then(r => r.json()).catch(() => null),
    fetch('/api/dashboard/swarm-consensus').then(r => r.json()).catch(() => null),
    fetch('/api/dashboard/swarm-anomalies').then(r => r.json()).catch(() => null),
    fetch('/api/dashboard/swarm-scenarios').then(r => r.json()).catch(() => null),
    fetch('/api/dashboard/swarm-strategies').then(r => r.json()).catch(() => null),
  ]);
  renderStatus(status);
  renderConsensus(consensus);
  renderAnomalies(anomalies);
  renderScenarios(scenarios);
  renderStrategies(strategies);
}

function renderStatus(status) {
  const el = document.getElementById('swarm-status');
  if (!el) return;
  if (!status || status.error) {
    el.innerHTML = '<div class="empty" style="padding:24px;text-align:center;background:var(--panel-l2);border-radius:8px">' +
      '<div style="font-size:32px;margin-bottom:8px">🐟</div>' +
      '<div style="color:var(--text);font-weight:600;margin-bottom:4px">等待 Swarm 模擬資料</div>' +
      '<div style="color:var(--muted);font-size:13px">MiroFish Swarm 每 30 分鐘自動執行一次背景模擬。<br>首次啟動後請稍待，或手動觸發模擬。</div>' +
      '</div>';
    return;
  }
  const totalFish = status.total_fish != null ? status.total_fish : '—';
  const confidence = status.consensus_confidence != null ? (status.consensus_confidence * 100).toFixed(1) + '%' : '—';
  const topAccuracy = status.top_accuracy != null ? (status.top_accuracy * 100).toFixed(1) + '%' : '—';
  const anomalyCount = status.anomaly_count || 0;
  const anomalyColor = anomalyCount === 0 ? 'var(--color-success)' : anomalyCount <= 3 ? 'var(--color-warning)' : 'var(--color-danger)';
  const lastRun = status.recorded_at ? new Date(status.recorded_at).toLocaleString() : '—';
  const generations = status.generations_evolved != null ? status.generations_evolved : '—';
  const trainingCount = status.training_scenarios != null ? status.training_scenarios : '—';

  el.innerHTML = `
    <div class="kpi-card"><div class="kpi-label">魚群數量</div><div class="kpi-value">${totalFish}</div></div>
    <div class="kpi-card"><div class="kpi-label">共識信心度</div><div class="kpi-value">${confidence}</div></div>
    <div class="kpi-card"><div class="kpi-label">最佳魚準確率</div><div class="kpi-value">${topAccuracy}</div></div>
    <div class="kpi-card" style="border-left:3px solid ${anomalyColor}"><div class="kpi-label">異常偵測</div><div class="kpi-value" style="color:${anomalyColor}">${anomalyCount}</div></div>
    <div class="kpi-card"><div class="kpi-label">最近執行</div><div class="kpi-value" style="font-size:14px">${escapeHtml(lastRun)}</div></div>
    <div class="kpi-card"><div class="kpi-label">演化世代</div><div class="kpi-value">${generations}</div></div>
    <div class="kpi-card"><div class="kpi-label">訓練資料筆數</div><div class="kpi-value">${trainingCount}</div></div>
  `;
}

function renderConsensus(consensus) {
  const el = document.getElementById('swarm-consensus');
  if (!el) return;
  if (!consensus || !Array.isArray(consensus) || consensus.length === 0) {
    el.innerHTML = '<div class="empty" style="padding:20px;text-align:center;color:var(--muted)">尚無共識資料</div>';
    return;
  }
  let rows = '';
  for (const item of consensus) {
    const dir = (item.consensus_direction || 'neutral').toLowerCase();
    const dirIcon = dir === 'bullish' ? '📈' : dir === 'bearish' ? '📉' : '↔️';
    const dirColor = dir === 'bullish' ? 'var(--color-success)' : dir === 'bearish' ? 'var(--color-danger)' : 'var(--muted)';
    const dirLabel = dir === 'bullish' ? '看多' : dir === 'bearish' ? '看空' : '中立';
    const conf = item.average_confidence != null ? (item.average_confidence * 100).toFixed(1) + '%' : '—';
    rows += `<tr>
      <td>${item.symbol ? renderStockCell(item.symbol) : '—'}</td>
      <td><span style="color:${dirColor};font-weight:600">${dirIcon} ${dirLabel}</span></td>
      <td>${conf}</td>
      <td>${item.bullish_count || 0}</td>
      <td>${item.bearish_count || 0}</td>
      <td>${item.neutral_count || 0}</td>
    </tr>`;
  }
  el.innerHTML = `<div class="table-wrapper"><table><thead><tr><th>標的</th><th>共識方向</th><th>信心度</th><th>看多</th><th>看空</th><th>中立</th></tr></thead><tbody>${rows}</tbody></table></div>`;
}

function renderAnomalies(anomalies) {
  const el = document.getElementById('swarm-anomalies');
  if (!el) return;
  if (!anomalies || !Array.isArray(anomalies) || anomalies.length === 0) {
    el.innerHTML = '<div class="empty" style="padding:20px;text-align:center;color:var(--color-success)">✅ 無異常偵測</div>';
    return;
  }
  let items = '';
  for (const a of anomalies) {
    const sev = a.severity || 0;
    const sevColor = sev > 0.7 ? 'var(--color-danger)' : sev > 0.4 ? 'var(--color-warning)' : 'var(--muted)';
    const sevLabel = sev > 0.7 ? '高' : sev > 0.4 ? '中' : '低';
    items += `<div style="padding:10px 14px;border-left:3px solid ${sevColor};margin-bottom:8px;background:var(--panel-l2);border-radius:6px">
      <div style="display:flex;justify-content:space-between;align-items:center">
        <span style="font-weight:600;color:${sevColor}">${escapeHtml(a.type || 'Unknown')}</span>
        <span style="font-size:11px;color:var(--muted)">${sevLabel}</span>
      </div>
      <div style="margin-top:4px;font-size:13px;color:var(--text)">${escapeHtml(a.description || '')}</div>
      <div style="margin-top:2px;font-size:11px;color:var(--muted)">影響: ${(a.symbols || []).join(', ') || '—'}</div>
    </div>`;
  }
  el.innerHTML = items;
}

function renderScenarios(scenarios) {
  const el = document.getElementById('swarm-scenarios');
  if (!el) return;
  if (!scenarios || !Array.isArray(scenarios) || scenarios.length === 0) {
    el.innerHTML = '<div class="empty" style="padding:20px;text-align:center;color:var(--muted)">尚無情境資料</div>';
    return;
  }
  const regimeLabels = { risk_on: '風險偏好', risk_off: '風險迴避', crisis: '危機', complacent: '自滿', transition: '轉換' };
  let rows = '';
  for (const s of scenarios) {
    const rLabel = regimeLabels[s.regime] || s.regime || '—';
    rows += `<tr>
      <td style="font-weight:600">${escapeHtml(s.name || '')}</td>
      <td><span class="badge">${escapeHtml(rLabel)}</span></td>
      <td>${s.volatility != null ? s.volatility.toFixed(4) : '—'}</td>
      <td>${s.trend != null ? s.trend.toFixed(6) : '—'}</td>
    </tr>`;
  }
  el.innerHTML = `<div class="table-wrapper"><table><thead><tr><th>情境</th><th>盤勢</th><th>波動率</th><th>趨勢</th></tr></thead><tbody>${rows}</tbody></table></div>`;
}

function renderStrategies(strategies) {
  const el = document.getElementById('swarm-strategies');
  if (!el) return;
  if (!strategies || !Array.isArray(strategies) || strategies.length === 0) {
    el.innerHTML = '<div class="empty" style="padding:20px;text-align:center;color:var(--muted)">尚無策略推薦資料</div>';
    return;
  }
  let rows = '';
  for (const s of strategies) {
    const perf = s.performance || {};
    const successRate = perf.success_rate != null ? (perf.success_rate * 100).toFixed(1) + '%' : '—';
    const avgImprovement = perf.avg_improvement != null ? perf.avg_improvement.toFixed(4) : '—';
    const convergenceRate = perf.convergence_rate != null ? (perf.convergence_rate * 100).toFixed(1) + '%' : '—';
    rows += `<tr>
      <td style="font-weight:600">${escapeHtml(s.name || '')}</td>
      <td><span class="badge">${escapeHtml(s.type || '—')}</span></td>
      <td>${s.score != null ? s.score.toFixed(4) : '—'}</td>
      <td>${successRate}</td>
      <td>${avgImprovement}</td>
      <td>${convergenceRate}</td>
    </tr>`;
  }
  el.innerHTML = `<div class="table-wrapper"><table><thead><tr><th>策略名稱</th><th>類型</th><th>分數</th><th>成功率</th><th>平均改善</th><th>收斂率</th></tr></thead><tbody>${rows}</tbody></table></div>`;
}

window.loadSwarmData = loadSwarmData;
