import { escapeHtml } from '../shared/utils.js';
import { renderStockCell } from '../names.js';

// i18n: 情境 ID → 中文名
const SCENARIO_NAME_ZH = {
  covid_crash_2020: 'COVID-19 市場崩盤 2020',
  fed_rate_hikes_2022: 'Fed 強勢升息 2022',
  ai_semiconductor_bubble: 'AI 半導體泡沫',
  taiwan_geopolitical: '台海地緣政治緊張',
  normal_market: '正常市場狀態',
  stagflation_2023: '停滯性通膨 2023',
  hualien_earthquake_2024: '花蓮地震 2024',
  china_lockdown_2022: '中國封城 2022',
  taiwan_level3_2021: '台灣三級警戒 2021',
  em_contagion_2018: '新興市場傳染風險 2018',
  liquidity_crunch_2008: '金融海嘯流動性緊縮 2008',
};

const REGIME_LABEL_ZH = {
  risk_on: '風險偏好',
  risk_off: '風險迴避',
  crisis: '危機',
  complacent: '自滿',
  transition: '轉換',
};

const ANOMALY_TYPE_ZH = {
  high_disagreement: '高度分歧',
};

const STRATEGY_TYPE_ZH = {
  momentum: '動量',
  adaptive: '自適應',
  curriculum: '課程式',
  ensemble: '集成',
  evolutionary: '演化式',
};

// 自動生成的策略名稱翻譯規則 (前綴匹配)
const STRATEGY_NAME_RULES = [
  { test: /^Crossover_strategy_momentum_.*_v(\d+)$/, label: m => `動量交叉策略 v${m[1]}` },
];

function translateScenarioName(id, fallback) {
  if (!id) return fallback || '—';
  return SCENARIO_NAME_ZH[id] || fallback || id;
}

function translateAnomalyType(t) {
  if (!t) return '未分類異常';
  return ANOMALY_TYPE_ZH[t] || t;
}

function translateStrategyType(t) {
  if (!t) return '—';
  return STRATEGY_TYPE_ZH[t] || t;
}

function translateStrategyName(name) {
  if (!name) return '—';
  for (const rule of STRATEGY_NAME_RULES) {
    const m = name.match(rule.test);
    if (m) return rule.label(m);
  }
  return name;
}

// 金融語意色彩輔助函數
// 趨勢: 正→bullish, 負→bearish, 0→muted
function trendColor(t) {
  if (t == null) return 'var(--muted)';
  if (t > 0) return 'var(--trend-bullish)';
  if (t < 0) return 'var(--trend-bearish)';
  return 'var(--muted)';
}

// 風險/波動: >0.3 高, <0.15 低, 其餘中性
function riskColor(v) {
  if (v == null) return 'var(--muted)';
  if (v > 0.3) return 'var(--risk-high)';
  if (v < 0.15) return 'var(--risk-low)';
  return 'var(--muted)';
}

// 績效指標: >0.6 good, <0.4 bad, 其餘中性
function metricColor(m) {
  if (m == null) return 'var(--muted)';
  if (m > 0.6) return 'var(--metric-good)';
  if (m < 0.4) return 'var(--metric-bad)';
  return 'var(--muted)';
}

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
  const confColor = metricColor(status.consensus_confidence);
  const accColor = metricColor(status.top_accuracy);
  const lastRun = status.recorded_at ? new Date(status.recorded_at).toLocaleString() : '—';
  const generations = status.generations_evolved != null ? status.generations_evolved : '—';
  const trainingCount = status.training_scenarios != null ? status.training_scenarios : '—';

  el.innerHTML = `
    <div class="kpi-card"><div class="kpi-label">魚群數量</div><div class="kpi-value">${totalFish}</div></div>
    <div class="kpi-card" style="border-left:3px solid ${confColor}"><div class="kpi-label">共識信心度</div><div class="kpi-value" style="color:${confColor}">${confidence}</div></div>
    <div class="kpi-card" style="border-left:3px solid ${accColor}"><div class="kpi-label">最佳魚準確率</div><div class="kpi-value" style="color:${accColor}">${topAccuracy}</div></div>
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
    const dirColor = dir === 'bullish' ? 'var(--bullish)' : dir === 'bearish' ? 'var(--bearish)' : 'var(--muted)';
    const dirLabel = dir === 'bullish' ? '看多' : dir === 'bearish' ? '看空' : '中立';
    const conf = item.average_confidence != null ? (item.average_confidence * 100).toFixed(1) + '%' : '—';
    const confColor = metricColor(item.average_confidence);
    rows += `<tr>
      <td>${item.symbol ? renderStockCell(item.symbol) : '—'}</td>
      <td><span style="color:${dirColor};font-weight:600">${dirIcon} ${dirLabel}</span></td>
      <td style="color:${confColor}">${conf}</td>
      <td>${item.bullish_count || 0}</td>
      <td>${item.bearish_count || 0}</td>
      <td>${item.neutral_count || 0}</td>
    </tr>`;
  }
  el.innerHTML = `<div class="table-wrapper"><table>
    <colgroup><col style="width:20%"><col style="width:22%"><col style="width:13%"><col style="width:15%"><col style="width:15%"><col style="width:15%"></colgroup>
    <thead><tr><th>標的</th><th>共識方向</th><th>信心度</th><th>看多</th><th>看空</th><th>中立</th></tr></thead>
    <tbody>${rows}</tbody>
  </table></div>`;
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
    const typeLabel = translateAnomalyType(a.type);
    const symbolsStr = (a.symbols || []).join(', ');
    items += `<div style="padding:10px 14px;border-left:3px solid ${sevColor};margin-bottom:8px;background:var(--panel-l2);border-radius:6px">
      <div style="display:flex;justify-content:space-between;align-items:center">
        <span style="font-weight:600;color:${sevColor}">${escapeHtml(typeLabel)}</span>
        <span style="font-size:11px;color:var(--muted)">${sevLabel}</span>
      </div>
      <div style="margin-top:4px;font-size:13px;color:var(--text)">${escapeHtml(a.description || '')}</div>
      <div style="margin-top:2px;font-size:11px;color:var(--muted)">影響: ${escapeHtml(symbolsStr) || '—'}</div>
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
  let rows = '';
  for (const s of scenarios) {
    const rLabel = REGIME_LABEL_ZH[s.regime] || s.regime || '—';
    const nameZh = translateScenarioName(s.id, s.name);
    const volColor = riskColor(s.volatility);
    const trColor = trendColor(s.trend);
    rows += `<tr>
      <td style="font-weight:600">${escapeHtml(nameZh)}</td>
      <td><span class="badge">${escapeHtml(rLabel)}</span></td>
      <td style="color:${volColor}">${s.volatility != null ? s.volatility.toFixed(4) : '—'}</td>
      <td style="color:${trColor}">${s.trend != null ? s.trend.toFixed(6) : '—'}</td>
    </tr>`;
  }
  el.innerHTML = `<div class="table-wrapper"><table>
    <colgroup><col style="width:40%"><col style="width:25%"><col style="width:17%"><col style="width:18%"></colgroup>
    <thead><tr><th>情境</th><th>盤勢</th><th>波動率</th><th>趨勢</th></tr></thead>
    <tbody>${rows}</tbody>
  </table></div>`;
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
    const successRate = s.score != null ? s.score : null;
    const successRateStr = successRate != null ? (successRate * 100).toFixed(1) + '%' : '—';
    const avgImprovement = perf.avg_improvement != null ? perf.avg_improvement : null;
    const avgImprovementStr = avgImprovement != null ? avgImprovement.toFixed(4) : '—';
    const convergenceRate = perf.convergence_rate != null ? perf.convergence_rate : null;
    const convergenceRateStr = convergenceRate != null ? (convergenceRate * 100).toFixed(1) + '%' : '—';
    const stabilityScore = perf.stability_score != null ? perf.stability_score : null;
    const stabilityScoreStr = stabilityScore != null ? stabilityScore.toFixed(4) : '—';
    const impColor = avgImprovement == null
      ? 'var(--muted)'
      : avgImprovement > 0 ? 'var(--metric-good)' : avgImprovement < 0 ? 'var(--metric-bad)' : 'var(--muted)';
    const nameZh = translateStrategyName(s.name);
    const typeZh = translateStrategyType(s.type);
    rows += `<tr>
      <td style="font-weight:600">${escapeHtml(nameZh)}</td>
      <td><span class="badge">${escapeHtml(typeZh)}</span></td>
      <td style="color:${metricColor(s.score)}">${s.score != null ? s.score.toFixed(4) : '—'}</td>
      <td style="color:${metricColor(successRate)}">${successRateStr}</td>
      <td style="color:${impColor}">${avgImprovementStr}</td>
      <td style="color:${metricColor(convergenceRate)}">${convergenceRateStr}</td>
      <td style="color:${metricColor(stabilityScore)}">${stabilityScoreStr}</td>
    </tr>`;
  }
  el.innerHTML = `<div class="table-wrapper"><table>
    <colgroup><col style="width:25%"><col style="width:14%"><col style="width:12%"><col style="width:12%"><col style="width:12%"><col style="width:12%"><col style="width:13%"></colgroup>
    <thead><tr><th>策略名稱</th><th>類型</th><th>分數</th><th>成功率</th><th>平均改善</th><th>收斂率</th><th>穩定性</th></tr></thead>
    <tbody>${rows}</tbody>
  </table></div>`;
}

window.loadSwarmData = loadSwarmData;
