import { fmtSafeNumber, fmtSafeSignedPct } from '../shared/format-metric.js';

function isValidNumber(v) {
  return typeof v === 'number' && Number.isFinite(v);
}

export async function renderPnLAttribution(container, getJSON) {
  // 失敗已由 app-utils choke point 回報 (reportDegraded)，此處 .catch 僅為 null 預設值。
  const data = await getJSON('/api/dashboard/pnl-attribution').catch(() => null);
  if (!data || !data.agent_attribution) {
    container.innerHTML = '<div style="padding:20px;text-align:center;color:var(--muted)">暫無歸因資料</div>';
    return;
  }

  const fmtP = (v) => fmtSafeSignedPct(v, 2);
  const fmtF = (v) => fmtSafeNumber(v, { decimals: 4 });

  const agentRows = (data.agent_attribution || [])
    .sort((a, b) => (b.avg_return || 0) - (a.avg_return || 0))
    .map(a => {
      const layerColors = { macro: 'var(--layer-4)', sector: 'var(--layer-1)', style: 'var(--layer-2)' };
      const color = layerColors[a.layer] || 'var(--muted)';
      return `<tr><td>${a.agent_name || a.agent_id}</td><td style="color:${color}">${a.layer || '-'}</td><td style="text-align:right">${fmtP(a.avg_return)}</td><td style="text-align:right">${a.count || 0}</td></tr>`;
    }).join('');

  const sectorRows = (data.sector_attribution || [])
    .sort((a, b) => (b.avg_return || 0) - (a.avg_return || 0))
    .map(s => `<tr><td>${s.sector_label || s.sector}</td><td style="text-align:right">${fmtP(s.avg_return)}</td><td style="text-align:right">${s.count || 0}</td></tr>`).join('');

  const fa = data.factor_attribution || {};
  const factorRows = ['momentum', 'value', 'quality', 'agent']
    .map(k => {
      const f = fa[k] || {};
      return `<tr><td style="font-weight:600">${k}</td><td style="text-align:right">${fmtF(f.avg_score)}</td><td style="text-align:right">${fmtP(f.avg_return)}</td><td style="text-align:right">${fmtF(f.contribution)}</td></tr>`;
    }).join('');

  container.innerHTML = `
    <div class="panel-content">
      <div class="section-title">Agent 貢獻</div>
      <div class="table-wrapper"><table class="text-sm"><thead><tr><th>策略來源</th><th>層級</th><th>平均報酬</th><th>次數</th></tr></thead><tbody>${agentRows}</tbody></table></div>
      <div class="section-title" style="margin-top:16px">產業貢獻</div>
      <div class="table-wrapper"><table class="text-sm"><thead><tr><th>產業</th><th>平均報酬</th><th>次數</th></tr></thead><tbody>${sectorRows}</tbody></table></div>
      <div class="section-title" style="margin-top:16px">因子貢獻</div>
      <div class="table-wrapper"><table class="text-sm"><thead><tr><th>因子</th><th>平均分數</th><th>平均報酬</th><th>貢獻度</th></tr></thead><tbody>${factorRows}</tbody></table></div>
    </div>`;
}