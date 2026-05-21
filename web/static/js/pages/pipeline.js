import { agentName, stockName, sectorName, regimeLabel } from '../names.js';
import { computePipelineSummary, factorBar, renderFactorMini, renderFactorBreakdown, toggleBreakdown } from './dashboard.js';
import { formatDate, getJSON, notify, renderEmptyState } from '../shared/app-utils.js';
import { escapeHtml } from '../shared/utils.js';

function negStyle(val) {
  const num = typeof val === 'number' ? val : parseFloat(val);
  if (isNaN(num)) return '';
  return num < 0 ? ' style="color:var(--down);font-weight:700"' : '';
}
function negSpan(val, fmt) {
  const num = typeof val === 'number' ? val : parseFloat(val);
  if (isNaN(num)) return fmt || String(val);
  const style = num < 0 ? ' style="color:var(--down);font-weight:700"' : '';
  return `<span${style}>${fmt != null ? fmt : num}</span>`;
}

// Delegated click handler for pipeline approve/reject buttons (replaces inline onclick)
if (typeof document !== 'undefined') {
  document.addEventListener('click', function(e) {
    const btn = e.target.closest('.pipeline-action');
    if (!btn) return;
    const text = btn.textContent.trim();
    if (text !== '放行' && text !== '補追' && text !== '否決') return;
    const symbol = btn.dataset.symbol;
    const agentId = btn.dataset.agentId;
    if (!symbol || !agentId) return;
    e.stopPropagation();
    if (text === '否決') {
      rejectRec(btn, symbol, agentId);
    } else {
      approveRec(btn, symbol, agentId);
    }
  });
}

export function renderConvictionBreakdown(cb) {
  if (!cb) return '<div class="text-muted text-xs">無計算明細</div>';
  const steps = (cb.steps || []).map(s => {
    const deltaCls = s.delta > 0 ? 'color:var(--up)' : (s.delta < 0 ? 'color:var(--down)' : 'color:var(--muted)');
    const deltaLabel = s.delta > 0 ? '+' + s.delta : String(s.delta);
    const prov = s.source ? `<span class="badge" style="font-size:9px;padding:1px 4px;background:rgba(59,130,246,.12);border:1px solid rgba(59,130,246,.3);color:#3b82f6;margin-left:4px">${s.source}${s.param_ref ? ':' + s.param_ref.split('.').pop() : ''}</span>` : '';
    return `<div style="display:flex;gap:8px;align-items:flex-start;margin:3px 0;padding:3px 6px;background:var(--bg);border-radius:4px">
      <div class="w-90 text-xs text-muted">${s.rule}${prov}</div>
      <div class="w-40 text-xs font-semibold" style="${deltaCls}">${deltaLabel}</div>
      <div style="flex:1;font-size:10px;color:var(--muted)">${s.reason || '-'}</div>
    </div>`;
  }).join('');
  return `<div class="py-sm">
    <div style="font-size:11px;margin-bottom:6px">基礎分: <strong>${cb.base}</strong>　門檻: <strong>${cb.floor}</strong>　最終: <strong class="text-accent">${cb.final}</strong></div>
    ${steps}
  </div>`;
}
export function renderDecisionChain(pipeline, macro, agentsData, stress, events, chains, models, inbox, phase3, taxSnapshot) {
  const el = document.getElementById('decisionChain');
  el.classList.remove('loading');
  const regime = (pipeline && pipeline.regime) || (macro && macro.regime) || '-';
  const eventList = (events && events.events) ? events.events.slice(0, 5) : [];
  const chainList = (chains && chains.chains) ? chains.chains.slice(0, 5) : [];
  const modelList = (models && models.models) ? models.models.slice(0, 5) : [];
  const stressVal = (stress && typeof stress.score === 'number') ? stress.score : null;
  const items = (pipeline && pipeline.items) ? pipeline.items : [];
  const screened = (pipeline && pipeline.screened_items) ? pipeline.screened_items : [];
  const guards = (pipeline && pipeline.guard_outcomes) ? pipeline.guard_outcomes : [];
  const agentCards = (agentsData && agentsData.weakest_agent_scorecards) ? agentsData.weakest_agent_scorecards : [];

  const layerCard = (num, title, content, color) => `
    <div style="display:flex;gap:12px;margin-bottom:14px;align-items:flex-start">
      <div style="flex-shrink:0;width:32px;height:32px;border-radius:50%;background:${color};display:flex;align-items:center;justify-content:center;color:#fff;font-weight:700;font-size:14px">${num}</div>
      <div style="flex:1;background:var(--panel);border:1px solid var(--border);border-radius:10px;padding:12px 14px">
        <div style="font-size:13px;font-weight:700;margin-bottom:8px;color:var(--text)">${title}</div>
        <div style="font-size:12px;color:var(--muted);line-height:1.7">${content}</div>
      </div>
    </div>
  `;

  const macroEventRows = eventList.map(e => {
    const hitRate = typeof e.hit_rate === 'number' ? (e.hit_rate * 100).toFixed(0) + '%' : '-';
    const source = e.confidence_source || '-';
    return `<tr>
      <td>${escapeHtml(e.theme)}</td>
      <td>${(e.confidence * 100).toFixed(0)}%</td>
      <td class="text-muted text-xs">${source}</td>
      <td>${hitRate}</td>
    </tr>`;
  }).join('');

  const macroContent = `
    <div style="display:flex;gap:10px;flex-wrap:wrap;margin-bottom:8px">
      <span class="badge info">市場狀態：${regimeLabel(regime)}</span>
      ${stressVal != null ? `<span class="badge ${stressVal >= 70 ? 'err' : (stressVal >= 50 ? 'warn' : 'ok')}">外資出逃指數：${stressVal.toFixed(1)}分（${stressVal >= 70 ? '紅燈-危機' : (stressVal >= 50 ? '黃燈-高壓' : (stressVal >= 30 ? '黃燈-警戒' : '綠燈-低壓'))}）</span>` : ''}
    </div>
    <div class="mb-sm">
      <div class="text-sm font-bold mb-xs">宏觀敘事事件（${eventList.length} 個）</div>
      ${eventList.length ? `<div class="scroll-box">
        <div class="table-wrapper">
        <table class="text-sm">
          <thead><tr><th>主題</th><th>Confidence</th><th>來源</th><th>歷史命中率</th></tr></thead>
          <tbody>${macroEventRows}</tbody>
        </table>
        </div>
      </div>` : renderEmptyState('暫無宏觀敘事事件', '')}
    </div>
    <div style="margin-bottom:10px;padding:10px 12px;background:var(--bg);border-radius:8px;border-left:3px solid #3b82f6">
      <div class="text-sm font-bold mb-xs">宏觀算法說明</div>
      <div class="text-xs text-muted lh-relaxed">
        宏觀事件 confidence 來源已升級為實證計算：偏差偵測（deviation_based_v1）、融資歷史百分位（margin_history_percentile）、日曆季節性（calendar_seasonal）等。<br>
        歷史命中率（HitRate）取自對應因果模板的回測驗證，範圍 0.55–0.81，代表該主題在歷史回測中的預測準確度。
      </div>
    </div>
    <div>活躍因果傳導鏈：${chainList.length} 條　投資模型：${modelList.length} 個</div>
  `;

  const sectorAgents = items.filter(it => it.layer === 'sector' || it.layer === 'macro');
  const styleAgents = items.filter(it => it.layer === 'style' || it.layer === 'superinvestor');

  const sectorAlgoNote = `
  <div style="margin-bottom:12px;padding:10px 12px;background:var(--bg);border-radius:8px;border-left:3px solid #8b5cf6">
    <div class="text-sm font-bold mb-xs">行業趨勢算法說明</div>
    <div class="text-xs text-muted lh-relaxed">
      所有產業桌（Sector）與風格桌（Style）的 conviction 皆從基礎分 60 開始。<br>
      基礎分會根據當日價量訊號調整：收盤價 > 開盤價加價格加分；成交量超過門檻加成交量分（上限 75）。<br>
      接著依據各 Agent 的 prompt 關鍵詞與 control 參數進行逐條加減分，最後與 floor 比較決定是否產出推薦。
    </div>
  </div>`;

  const sectorRow = (it, idx, prefix) => {
    const key = `${prefix}-${idx}-${escapeHtml(it.symbol)}-${escapeHtml(it.agent_id)}`;
    const cb = it.conviction_breakdown;
    return `<tr>
      <td>${escapeHtml(it.symbol)}</td>
      <td>${escapeHtml(stockName(it.symbol)) || '-'}</td>
      <td>${escapeHtml(agentName(it.agent_id))}</td>
      <td>${it.conviction != null ? negSpan(it.conviction) : '-'}</td>
      <td><button class="pipeline-action" onclick="toggleBreakdown('${key}')" id="btn-${key}">展開</button></td>
    </tr>
    <tr id="breakdown-${key}" class="hidden"><td colspan="5" style="background:#0d1015;border-left:3px solid #8b5cf6">${renderConvictionBreakdown(cb)}</td></tr>`;
  };

  const sectorTable = (title, list, prefix) => {
    if (!list.length) return `<div class="mb-sm"><div class="text-sm font-bold mb-xs">${title}</div>${renderEmptyState('暫無資料', '')}</div>`;
    return `<div class="mb-sm">
      <div class="text-sm font-bold mb-xs">${title}（${list.length} 筆）</div>
      <div class="scroll-box">
        <div class="table-wrapper">
        <table class="text-sm">
          <thead><tr><th>標的</th><th>公司名稱</th><th>策略來源</th><th>信念</th><th>明細</th></tr></thead>
          <tbody>${list.map((it, idx) => sectorRow(it, idx, prefix)).join('')}</tbody>
        </table>
        </div>
      </div>
    </div>`;
  };

  const sectorContent = `
    ${sectorAlgoNote}
    ${sectorTable('產業主題推薦', sectorAgents, 'sector')}
    ${sectorTable('風格因子推薦', styleAgents, 'style')}
    <div class="mb-sm"><strong>活躍因果鏈：</strong></div>
    <ul style="margin:0;padding-left:18px;line-height:1.8">
      ${chainList.map(c => `<li>${escapeHtml(c.template_id)}（分數 ${c.score?.toFixed(2) || '-' }）</li>`).join('')}
      ${!chainList.length ? '<li>暫無活躍因果鏈</li>' : ''}
    </ul>
  `;

  let agentCriteriaHtml = '';
  if (agentCards.length) {
    agentCriteriaHtml = agentCards.map(c => {
      const overlapAgent = (agentsData && agentsData.agents) ? agentsData.agents.find(a => a.agent_id === c.agent_id) : null;
      const sc = (overlapAgent && overlapAgent.screening_criteria) || {};
      const badges = [];
      if (sc.pe && sc.pe.max != null) badges.push(`P/E≤${sc.pe.max}`);
      if (sc.pe && sc.pe.min != null && sc.pe.max == null) badges.push(`P/E≥${sc.pe.min}`);
      if (sc.pb && sc.pb.max != null) badges.push(`P/B≤${sc.pb.max}`);
      if (sc.pb && sc.pb.min != null && sc.pb.max == null) badges.push(`P/B≥${sc.pb.min}`);
      if (sc.dividend_yield && sc.dividend_yield.min != null) badges.push(`股息≥${sc.dividend_yield.min}%`);
      if (sc.volume_intraday && sc.volume_intraday.min != null) badges.push(`成交量≥${(sc.volume_intraday.min/10000).toFixed(0)}萬`);
      if (sc.momentum_20d && sc.momentum_20d.min != null) badges.push(`動能≥${sc.momentum_20d.min}`);
      if (sc.min_total_factor_score != null) badges.push(`因子≥${sc.min_total_factor_score}`);
      return `<div class="my-xs"><strong>${agentName(c.agent_id)}</strong> ${badges.length ? badges.map(b => `<span class="badge" style="font-size:10px;padding:1px 5px">${b}</span>`).join(' ') : '<span class="text-muted">無篩選條件</span>'}</div>`;
    }).join('');
  } else {
    agentCriteriaHtml = renderEmptyState('暫無 Agent 績效資料', '');
  }

  const factorCells = (fs, rowKey) => {
    fs = fs || {};
    const btn = `<button class="pipeline-action" onclick="toggleBreakdown('${rowKey}')" id="btn-${rowKey}">展開</button>`;
    return `<td>${factorBar(fs.momentum, -1, 1)}</td><td class="text-xs" style="${fs.momentum != null && fs.momentum < 0 ? 'color:var(--down);font-weight:700' : ''}">${fs.momentum != null ? fs.momentum.toFixed(2) : '-'}</td>
            <td>${factorBar(fs.value, -1, 1)}</td><td class="text-xs" style="${fs.value != null && fs.value < 0 ? 'color:var(--down);font-weight:700' : ''}">${fs.value != null ? fs.value.toFixed(2) : '-'}</td>
            <td>${factorBar(fs.quality, -1, 1)}</td><td class="text-xs" style="${fs.quality != null && fs.quality < 0 ? 'color:var(--down);font-weight:700' : ''}">${fs.quality != null ? fs.quality.toFixed(2) : '-'}</td>
            <td>${factorBar(fs.agent, 0, 1)}</td><td class="text-xs" style="${fs.agent != null && fs.agent < 0 ? 'color:var(--down);font-weight:700' : ''}">${fs.agent != null ? fs.agent.toFixed(2) : '-'}</td>
            <td>${factorBar(fs.total, -1, 1)}</td><td class="text-xs" style="${fs.total != null && fs.total < 0 ? 'color:var(--down);font-weight:700' : ''}">${fs.total != null ? fs.total.toFixed(3) : '-'}</td>
            <td>${btn}</td>`;
  };

  const recRows = items.map((it, idx) => {
    const rowKey = `rec-${idx}-${escapeHtml(it.symbol)}-${escapeHtml(it.agent_id)}`;
    return `<tr><td>${escapeHtml(it.symbol)}</td><td>${escapeHtml(stockName(it.symbol)) || '-'}</td><td>${escapeHtml(agentName(it.agent_id))}</td>${factorCells(it.factor_scores, rowKey)}</tr>
            <tr id="breakdown-${rowKey}" class="hidden"><td colspan="14" class="detail-panel">${renderFactorBreakdown(it.factor_scores && it.factor_scores.breakdown)}</td></tr>`;
  }).join('');
  const screenedRows = screened.map((s, idx) => {
    const rowKey = `scr-${idx}-${escapeHtml(s.symbol)}-${escapeHtml(s.agent_id)}`;
    return `<tr class="text-muted"><td>${escapeHtml(s.symbol)}</td><td>${escapeHtml(stockName(s.symbol)) || '-'}</td><td>${escapeHtml(agentName(s.agent_id))}</td><td colspan="2">${s.criterion_label ? escapeHtml(s.criterion_label) : (s.criterion ? escapeHtml(s.criterion) : '-')}</td>${factorCells(s.factor_scores, rowKey)}</tr>
            <tr id="breakdown-${rowKey}" class="hidden"><td colspan="16" class="detail-panel">${renderFactorBreakdown(s.factor_scores && s.factor_scores.breakdown)}</td></tr>`;
  }).join('');

  const algoNote = `
  <div style="margin-bottom:12px;padding:10px 12px;background:var(--bg);border-radius:8px;border-left:3px solid var(--accent)">
    <div class="text-sm font-bold mb-xs">算法透明化說明</div>
    <div class="text-xs text-muted lh-relaxed">
      動能：20日收益率 ÷ 0.30（無歷史資料則用當日漲跌 ÷ 0.10），clamp 到 [-1, 1]。<br>
      價值：P/E 得分 = 1 - (PE-5)/45；P/B 得分 = 1 - (PB-0.5)/4.5；clamp 到 [-1, 1] 後取平均。<br>
      品質：股息率 ÷ 5%；波動率得分 = 1 - Vol20d/0.05；兩者平均。<br>
      Agent：該標的所有 Agent 推薦的 Conviction × 權重 ÷ 100 的加權平均。<br>
      總分：動能 0.30 + 價值 0.25 + 品質 0.25 + Agent 0.20。
    </div>
  </div>`;

  const stockContent = `
    ${algoNote}
    <div class="mb-sm">${agentCriteriaHtml}</div>
    <div class="mb-sm">
      <div class="text-sm font-bold mb-xs">被推薦個股因子明細（${items.length} 檔）</div>
      ${items.length ? `<div style="max-height:260px;overflow:auto">
        <div class="table-wrapper">
        <table class="text-sm">
          <thead><tr><th>標的</th><th>公司名稱</th><th>策略來源</th><th colspan="2">動能</th><th colspan="2">價值</th><th colspan="2">品質</th><th colspan="2">Agent</th><th colspan="2">總分</th><th>明細</th></tr></thead>
          <tbody>${recRows}</tbody>
        </table>
        </div>
      </div>` : renderEmptyState('暫無被推薦個股', '')}
    </div>
    <div>
      <div class="text-sm font-bold mb-xs">被篩除個股因子明細（${screened.length} 檔）</div>
      ${screened.length ? `<div class="scroll-box">
        <div class="table-wrapper">
        <table class="text-sm">
          <thead><tr><th>標的</th><th>公司名稱</th><th>策略來源</th><th colspan="2">未通過條件</th><th colspan="2">動能</th><th colspan="2">價值</th><th colspan="2">品質</th><th colspan="2">Agent</th><th colspan="2">總分</th><th>明細</th></tr></thead>
          <tbody>${screenedRows}</tbody>
        </table>
        </div>
      </div>` : renderEmptyState('本場次暫無篩除紀錄', '')}
    </div>
  `;

  const guardContent = guards.map(g => {
    const inputCount = g.input_count || 0;
    const outputCount = g.output_count || 0;
    const filtered = inputCount - outputCount;
    let cls = 'ok';
    let txt = `放行 ${outputCount} 筆`;
    if (!g.passed) { cls = 'err'; txt = `阻擋（輸出 ${outputCount} 筆）`; }
    else if (filtered > 0) { cls = 'warn'; txt = `過濾 ${filtered} 筆後放行 ${outputCount} 筆`; }
    return `<div class="my-xs"><span class="badge ${cls}">${escapeHtml(agentName(g.guard_id))}</span> ${txt}（輸入 ${inputCount} → 輸出 ${outputCount}）</div>`;
  }).join('') || renderEmptyState('暫無控制層紀錄', '');

  const perfRows = items.slice(0, 20).map(it => {
    const fr = it.forward_return;
    const frCls = fr > 0 ? 'color:var(--up);font-weight:700' : (fr < 0 ? 'color:var(--down);font-weight:700' : 'color:var(--muted)');
    const frIcon = fr > 0 ? '📈 ' : (fr < 0 ? '📉 ' : '➖ ');
    const hit = it.hit != null ? (it.hit ? '<span class="badge ok">命中</span>' : '<span class="badge err">失誤</span>') : '<span class="badge">待驗證</span>';
    return `<tr>
      <td>${escapeHtml(it.symbol)}</td>
      <td>${escapeHtml(stockName(it.symbol)) || '-'}</td>
      <td>${escapeHtml(agentName(it.agent_id))}</td>
      <td>${it.price ? it.price.toFixed(2) : '-'}</td>
      <td>${it.target_price > 0 ? it.target_price.toFixed(2) : '-'}</td>
      <td>${it.stop_loss_price > 0 ? it.stop_loss_price.toFixed(2) : '-'}</td>
      <td style="${frCls}">${frIcon}${fr != null ? (fr*100).toFixed(1)+'%' : '-'}</td>
      <td>${hit}</td>
    </tr>`;
  }).join('');

  const perfContent = items.length ? `
    <div style="max-height:360px;overflow:auto">
    <div class="table-wrapper">
    <table class="text-sm">
      <thead><tr><th>標的</th><th>公司名稱</th><th>策略來源</th><th>建議價</th><th>目標價</th><th>停損價</th><th>隔日報酬</th><th>命中</th></tr></thead>
      <tbody>${perfRows}</tbody>
    </table>
    </div>
    </div>
    </div>
    </div>
    <div style="margin-top:8px;color:var(--muted);font-size:11px">顯示前 20 筆控制層放行標的。完整明細請至「投資管線」頁面。</div>
  ` : renderEmptyState('本場次尚無控制層放行標的', 'AI 推薦可能全部被控制層過濾');

  const taxContent = taxSnapshot && taxSnapshot.snapshots && taxSnapshot.snapshots.length > 0 ? `
    <div class="mb-sm">
      <div class="text-sm font-bold mb-xs">稅務摘要</div>
      <div style="display:grid;grid-template-columns:repeat(4,1fr);gap:10px">
        <div class="kpi-card" class="p-sm">
          <div class="kpi-label">稅前損益</div>
          <div class="kpi-value" class="text-base" style="${(taxSnapshot.before_tax_pnl || 0) < 0 ? 'color:var(--down)' : ''}">${(taxSnapshot.before_tax_pnl || 0).toFixed(0)}</div>
        </div>
        <div class="kpi-card" class="p-sm">
          <div class="kpi-label">總稅額</div>
          <div class="kpi-value" style="font-size:16px;color:var(--down)">${(taxSnapshot.total_tax_paid || 0).toFixed(0)}</div>
        </div>
        <div class="kpi-card" class="p-sm">
          <div class="kpi-label">稅後損益</div>
          <div class="kpi-value" style="font-size:16px;color:${(taxSnapshot.after_tax_pnl || 0) >= 0 ? 'var(--up)' : 'var(--down)'}">${(taxSnapshot.after_tax_pnl || 0).toFixed(0)}</div>
        </div>
        <div class="kpi-card" class="p-sm">
          <div class="kpi-label">有效稅率</div>
          <div class="kpi-value" class="text-base">${taxSnapshot.before_tax_pnl != 0 ? ((taxSnapshot.total_tax_paid / Math.abs(taxSnapshot.before_tax_pnl)) * 100).toFixed(1) : 0}%</div>
        </div>
      </div>
    </div>
    <div class="scroll-box">
      <div class="table-wrapper">
      <table class="text-sm">
        <thead><tr><th>標的</th><th>公司名稱</th><th>交易稅</th><th>股息稅</th><th>總稅額</th><th>稅後損益</th></tr></thead>
        <tbody>${taxSnapshot.snapshots.map(s => `<tr><td>${escapeHtml(s.symbol)}</td><td>${escapeHtml(stockName(s.symbol)) || '-'}</td><td>${s.transaction_tax.toFixed(0)}</td><td>${s.dividend_tax.toFixed(0)}</td><td>${s.total_tax.toFixed(0)}</td><td style="${s.after_tax_pnl < 0 ? 'color:var(--down);font-weight:700' : ''}">${s.after_tax_pnl.toFixed(0)}</td></tr>`).join('')}</tbody>
      </table>
      <div style="margin-top:8px;font-size:11px;color:var(--muted);padding:6px 10px;background:var(--bg);border-radius:6px;border-left:3px solid var(--warn)">
        ${taxSnapshot.note ? `ℹ️ ${escapeHtml(taxSnapshot.note)}` : 'ℹ️ 稅務資料已計算'}
      </div>
      </div>
    </div>
  ` : renderEmptyState('暫無稅務資料', '');

  el.innerHTML = `
    ${layerCard('1', '宏觀環境', macroContent, 'var(--layer-1)')}
    ${layerCard('2', '行業趨勢', sectorContent, 'var(--layer-2)')}
    ${layerCard('3', '個股篩選', stockContent, 'var(--layer-3)')}
    ${layerCard('4', '控制決策', guardContent, 'var(--layer-4)')}
    ${layerCard('5', '績效追蹤', perfContent, 'var(--layer-5)')}
    ${layerCard('6', '稅務影響', taxContent, 'var(--layer-6)')}
  `;
}

// --- Filter panel state ---
export let filterState = {
  peMin: null, peMax: null,
  pbMin: null, pbMax: null,
  dyMin: null, dyMax: null,
  retMin: null, retMax: null
};
export let isFilterActive = false;

export function toggleFilterPanel() {
  const panel = document.getElementById('filterPanel');
  const toggle = document.getElementById('filterToggle');
  if (!panel || !toggle) return;
  panel.classList.toggle('open');
  toggle.classList.toggle('active', panel.classList.contains('open'));
}

export function applyFilters() {
  const peMin = parseFloat(document.getElementById('peMin')?.value);
  const peMax = parseFloat(document.getElementById('peMax')?.value);
  const pbMin = parseFloat(document.getElementById('pbMin')?.value);
  const pbMax = parseFloat(document.getElementById('pbMax')?.value);
  const dyMin = parseFloat(document.getElementById('dyMin')?.value);
  const dyMax = parseFloat(document.getElementById('dyMax')?.value);
  const retMin = parseFloat(document.getElementById('retMin')?.value);
  const retMax = parseFloat(document.getElementById('retMax')?.value);

  filterState = { peMin: isNaN(peMin) ? null : peMin, peMax: isNaN(peMax) ? null : peMax,
                  pbMin: isNaN(pbMin) ? null : pbMin, pbMax: isNaN(pbMax) ? null : pbMax,
                  dyMin: isNaN(dyMin) ? null : dyMin, dyMax: isNaN(dyMax) ? null : dyMax,
                  retMin: isNaN(retMin) ? null : retMin, retMax: isNaN(retMax) ? null : retMax };

  isFilterActive = Object.values(filterState).some(v => v !== null);

  const badge = document.getElementById('filterBadge');
  if (badge) badge.innerHTML = isFilterActive ? '<span class="filter-active-badge">篩選中</span>' : '';

  // Re-render pipeline with filters (reset to first page)
  window._pipelinePage = 0;
  const data = window._pipelineData;
  if (data) renderPipeline(data, false, null, false);

  const count = document.getElementById('filterResultCount');
  if (count) {
    const filtered = countFilteredItems(data?.items || []);
    count.textContent = filtered > 0 ? `符合條件：${filtered} 筆` : '';
  }

  notify('篩選條件已套用', 'success');
}

export function clearFilters() {
  ['peMin','peMax','pbMin','pbMax','dyMin','dyMax','retMin','retMax'].forEach(id => {
    const el = document.getElementById(id);
    if (el) el.value = '';
  });
  filterState = { peMin: null, peMax: null, pbMin: null, pbMax: null, dyMin: null, dyMax: null, retMin: null, retMax: null };
  isFilterActive = false;
  const badge = document.getElementById('filterBadge');
  if (badge) badge.innerHTML = '';
  const count = document.getElementById('filterResultCount');
  if (count) count.textContent = '';
  window._pipelinePage = 0;
  const data = window._pipelineData;
  if (data) renderPipeline(data, false, null, false);
  notify('篩選條件已清除', 'info');
}

export function countFilteredItems(items) {
  if (!items || items.length === 0) return 0;
  return items.filter(item => passesFilter(item)).length;
}

export function passesFilter(item) {
  if (!isFilterActive) return true;
  if (!item.metrics) return true;
  const m = item.metrics;
  const pe = m.price_to_earnings ?? null;
  const pb = m.price_to_book ?? null;
  const dy = m.dividend_yield ?? null;
  const ret = m.backtest_return ?? null;

  if (filterState.peMin !== null && pe !== null && pe < filterState.peMin) return false;
  if (filterState.peMax !== null && pe !== null && pe > filterState.peMax) return false;
  if (filterState.pbMin !== null && pb !== null && pb < filterState.pbMin) return false;
  if (filterState.pbMax !== null && pb !== null && pb > filterState.pbMax) return false;
  if (filterState.dyMin !== null && dy !== null && dy < filterState.dyMin) return false;
  if (filterState.dyMax !== null && dy !== null && dy > filterState.dyMax) return false;
  if (filterState.retMin !== null && ret !== null && ret < filterState.retMin) return false;
  if (filterState.retMax !== null && ret !== null && ret > filterState.retMax) return false;
  return true;
}

export let pipelineSessions = [];

export function renderPipeline(data, showAll, sessionId, showScreened) {
  if (showAll === undefined) showAll = false;
  if (showScreened === undefined) showScreened = false;
  const el = document.getElementById('recommendationPipeline');
  if (!data || !data.session_id) { el.innerHTML = renderEmptyState('尚無回測場次資料', '執行回測後將自動顯示'); el.classList.remove('loading'); return; }
  el.classList.remove('loading');

  // Store data for filter access
  window._pipelineData = data;

  const { rawInputs, finalOutputs, filteredCount, guard } = computePipelineSummary(data.guard_outcomes);
  let items = data.items || [];
  const screenedItems = data.screened_items || [];
  const recordedAt = data.recorded_at ? formatDate(data.recorded_at) : '-';

  // Apply filters if active
  if (isFilterActive) {
    const before = items.length;
    items = items.filter(passesFilter);
    const after = items.length;
    const count = document.getElementById('filterResultCount');
    if (count) count.textContent = `符合條件：${after} / ${before} 筆`;
  }

  const sessionSelect = pipelineSessions.length ? `
    <select id="pipelineSessionSelect" style="font-size:12px;padding:2px 6px;border-radius:4px;border:1px solid var(--border);background:var(--panel);color:var(--text)" onchange="togglePipelineSession(this)">
      ${pipelineSessions.map(s => `<option value="${escapeHtml(s.session_id)}" ${s.session_id === data.session_id ? 'selected' : ''}>${escapeHtml(s.session_id)} · ${escapeHtml(s.regime)} · ${new Date(s.recorded_at).toLocaleDateString('zh-TW')}</option>`).join('')}
    </select>
  ` : `<span class="text-muted text-sm">載入場次中…</span>`;

  const guardBadges = guard.length ? `<div class="mb-sm">${guard.map(g => {
    const inputCount = g.input_count || 0;
    const outputCount = g.output_count || 0;
    const filtered = inputCount - outputCount;
    let label = '放行';
    let cls = 'ok';
    if (!g.passed) { label = '阻擋'; cls = 'err'; }
    else if (filtered > 0) { label = `過濾 ${filtered} 筆`; cls = 'warn'; }
    return `<span class="badge ${cls}">${escapeHtml(agentName(g.guard_id))}：${label}（${inputCount}→${outputCount}）</span>`;
  }).join('')}</div>` : '';

  const screeningBadge = screenedItems.length ? `（${screenedItems.length} 檔被篩除）` : '';
  const workflowScreening = document.getElementById('workflowScreening');
  if (workflowScreening) {
    workflowScreening.textContent = '篩選層' + screeningBadge;
    if (showScreened) workflowScreening.classList.add('active');
    else workflowScreening.classList.remove('active');
  }

  const screenedCountLabel = screenedItems.length ? `（${screenedItems.length} 檔）` : '（無資料）';
  const showScreenedCheckbox = `
    <div class="mb-sm text-sm">
      <label class="cursor-pointer select-none">
        <input type="checkbox" id="pipelineShowScreened" ${showScreened ? 'checked' : ''} onchange="togglePipelineShowScreened(this)">
        <span class="ml-xs">顯示被篩選層排除的標的 ${screenedCountLabel}</span>
      </label>
    </div>
  `;

  const screenedSection = showScreened ? (screenedItems && screenedItems.length ? `
    <div class="mt-sm">
      <h3 style="font-size:13px;color:var(--muted);margin-bottom:8px">被篩選層排除的標的（${screenedItems.length} 檔）</h3>
      <div class="scroll-box-lg">
        <div class="table-wrapper">
        <table class="text-sm">
          <thead><tr><th>標的</th><th>公司名稱</th><th>策略來源</th><th>未通過條件</th><th>門檻</th><th>實際值</th><th>因子分數</th></tr></thead>
          <tbody>
            ${screenedItems.map(s => `<tr style="border-left:3px solid #666;color:var(--muted)">
              <td>${escapeHtml(s.symbol)}</td>
              <td>${escapeHtml(stockName(s.symbol)) || '-'}</td>
              <td>${escapeHtml(agentName(s.agent_id))}</td>
              <td>${s.criterion_label ? escapeHtml(s.criterion_label) : (s.criterion ? escapeHtml(s.criterion) : '-')}</td>
              <td>${s.threshold != null ? escapeHtml(String(s.threshold)) : '-'}</td>
              <td>${s.actual_value != null ? escapeHtml(String(s.actual_value)) : '-'}</td>
              <td>${renderFactorMini(s.factor_scores)}</td>
            </tr>`).join('')}
          </tbody>
        </table>
        </div>
      </div>
    </div>
  ` : `<div style="margin-top:12px;padding:12px;background:var(--bg);border-radius:8px;border-left:3px solid var(--border);color:var(--muted);font-size:12px">
    <strong>本場次暫無篩選層排除紀錄</strong><br>
    若為較舊場次，可能尚未記錄篩選資料。請執行新的回測後再查看。
  </div>`) : '';

  const PAGE_SIZE = 50;
  const totalItems = items.length;
  let page = window._pipelinePage || 0;
  const startIdx = page * PAGE_SIZE;
  const endIdx = Math.min(startIdx + PAGE_SIZE, totalItems);
  const pagedItems = items.slice(startIdx, endIdx);
  const totalPages = Math.ceil(totalItems / PAGE_SIZE);

  const buildTableRows = (itemList) => itemList.map(it => {
    const cls = it.forward_return > 0 ? 'positive' : (it.forward_return < 0 ? 'negative' : '');
    const rowStyle = !it.passed_guards ? 'background:rgba(239,68,68,0.06);border-left:3px solid var(--down)' : 'border-left:3px solid var(--up)';
    const crowded = (it.reason || '').includes('[crowded:');
    const badge = crowded ? `<span class="badge warn">擁擠</span> ` : '';
    const sideLabel = it.side === 'BUY' ? '<span style="color:var(--up);font-weight:700">買入</span>' : (it.side === 'SELL' ? '<span style="color:var(--down);font-weight:700">賣出</span>' : '-');
    const priceLabel = it.price ? it.price.toFixed(2) : '-';
    const targetPriceLabel = it.target_price > 0 ? it.target_price.toFixed(2) : '-';
    const stopLossPriceLabel = it.stop_loss_price > 0 ? it.stop_loss_price.toFixed(2) : '-';
    let frCls = 'color:var(--muted)';
    let frIcon = '➖ ';
    if (it.forward_return > 0) { frCls = 'color:var(--up);font-weight:700'; frIcon = '📈 '; }
    else if (it.forward_return < 0) { frCls = 'color:var(--down);font-weight:700'; frIcon = '📉 '; }
    const layerMap = { sector: '產業主題', style: '風格因子', superinvestor: '超級投資者', context: '總經情境', control: '控制層', macro: '總經情境' };
    const layerName = layerMap[it.layer] || (it.layer || (it.agent_id === 'alpha_discovery' ? '風格因子' : '-'));
    const tags = (it.tags || []).map(t => `<span class="badge" style="font-size:10px;padding:1px 5px;background:var(--bg);border:1px solid var(--border);margin-right:3px">${escapeHtml(t)}</span>`).join('');
    const actionBtns = it.passed_guards
      ? `<button class="pipeline-action" data-symbol="${escapeHtml(it.symbol)}" data-agent-id="${escapeHtml(it.agent_id)}">放行</button> <button class="pipeline-action" data-symbol="${escapeHtml(it.symbol)}" data-agent-id="${escapeHtml(it.agent_id)}">否決</button>`
      : `<button class="pipeline-action" data-symbol="${escapeHtml(it.symbol)}" data-agent-id="${escapeHtml(it.agent_id)}">補追</button>`;
    const actionHelp = `<span style="cursor:pointer;color:var(--accent);font-size:12px;margin-left:4px" onclick="event.stopPropagation();openInfoHelp('人工覆寫說明', \`<p><strong>放行</strong>：人工背書此推薦，系統後續回測不會將它濾除。</p><p><strong>否決</strong>：人工拒絕此推薦，系統後續回測會強制排除該 (標的, Agent) 組合。</p><p><strong>補追</strong>：對已被控制層擋下的標的進行人工放行。</p>\`)">ℹ️</span>`;
    const narrativeEvents = it.narrative_event_ids || [];
    const narrativeCtx = it.narrative_context;
    const hasNarrative = narrativeEvents.length > 0 || narrativeCtx;
    const narrativeBadge = hasNarrative
      ? `<span class="badge" style="font-size:10px;padding:1px 5px;background:rgba(59,130,246,.15);border:1px solid rgba(59,130,246,.4);color:#3b82f6">${narrativeCtx ? escapeHtml(narrativeCtx.primary_theme || narrativeCtx.theme || '敘事') : '敘事'} ${narrativeEvents.length > 0 ? narrativeEvents.length : ''}</span>`
      : '<span class="text-muted text-xs">-</span>';
    const industryCtx = it.industry_context;
    const hasIndustry = industryCtx && industryCtx.business_cycle;
    const industryBadge = hasIndustry
      ? `<span class="badge" style="font-size:10px;padding:1px 5px;background:rgba(139,92,246,.15);border:1px solid rgba(139,92,246,.4);color:#8b5cf6">${escapeHtml(industryCtx.business_cycle)} ${industryCtx.cycle_confidence != null ? (industryCtx.cycle_confidence * 100).toFixed(0) + '%' : ''}</span>`
      : '';
    return `<tr class="pipeline-row ${cls}" style="${rowStyle}"><td>${escapeHtml(it.symbol)}</td><td>${escapeHtml(stockName(it.symbol)) || '-'}</td><td>${escapeHtml(agentName(it.agent_id))}（${escapeHtml(it.skill)}）</td><td>${escapeHtml(layerName)}</td><td>${sideLabel}</td><td>${priceLabel}</td><td>${targetPriceLabel}</td><td>${stopLossPriceLabel}</td><td>${it.conviction != null ? it.conviction : '-'}</td><td>${narrativeBadge}${industryBadge ? ' ' + industryBadge : ''}</td><td>${renderFactorMini(it.factor_scores)}</td><td style="${frCls}">${frIcon}${(it.forward_return*100).toFixed(1)}%</td><td><div style="display:flex;gap:3px;flex-wrap:wrap;margin-bottom:4px">${tags}</div>${badge}${it.reason ? escapeHtml(it.reason) : '-'}${it.guard_reason ? '<br><span class="text-muted text-xs">' + escapeHtml(it.guard_reason) + '</span>' : ''}</td><td>${actionBtns}${actionHelp}</td></tr>`;
  }).join('');

  const paginationControls = totalItems > PAGE_SIZE ? `
    <div class="table-pagination" style="margin-top:10px">
      <span>顯示 <strong>${startIdx + 1}-${endIdx}</strong> / 共 <strong>${totalItems}</strong> 筆</span>
      <div style="display:flex;gap:6px;align-items:center">
        <button onclick="window._pipelinePage=0;renderPipeline(window._pipelineData,false,null,false)" ${page===0?'disabled':''}>« 首頁</button>
        <button onclick="window._pipelinePage=${page-1};renderPipeline(window._pipelineData,false,null,false)" ${page===0?'disabled':''}>‹ 上一頁</button>
        <span style="padding:0 8px">第 ${page + 1} / ${totalPages} 頁</span>
        <button onclick="window._pipelinePage=${page+1};renderPipeline(window._pipelineData,false,null,false)" ${page>=totalPages-1?'disabled':''}>下一頁 ›</button>
        <button onclick="window._pipelinePage=${totalPages-1};renderPipeline(window._pipelineData,false,null,false)" ${page>=totalPages-1?'disabled':''}>末頁 »</button>
      </div>
    </div>
  ` : '';

  const pipelineTable = items.length ? `
    <div class="scroll-box-lg">
    <div class="table-wrapper">
    <table id="pipelineTable">
      <thead><tr><th>標的</th><th>公司名稱</th><th>策略來源 <span class="cursor-pointer text-accent" onclick="event.stopPropagation();openInfoHelp('策略來源說明', \`<p><strong>策略來源是什麼？</strong></p><p>顯示格式為 <code>Agent ID（策略技能）</code>。</p><p>Agent ID 是系統內部的實例名稱；括號內為該 Agent 執行的策略技能。同一策略未來可能由多個 Agent 競爭執行。</p>\`)">ℹ️</span></th><th>來源層 <span class="cursor-pointer text-accent" onclick="event.stopPropagation();openInfoHelp('來源層說明', \`<p><strong>來源層決定這筆推薦來自 Atlas 的哪一層代理。</strong></p><ul style='margin:6px 0;padding-left:18px;line-height:1.8'><li><strong>產業主題</strong>：sector layer，決定板塊配置。</li><li><strong>風格因子</strong>：style layer，決定成長／價值／動能等風格傾向。</li><li><strong>超級投資者</strong>：superinvestor layer，模擬特定投資大師的選股邏輯。</li><li><strong>總經情境</strong>：context layer，根據總經 regime 產生的方向性建議。</li></ul>\`)">ℹ️</span></th><th>方向</th><th>收盤價</th><th>目標價</th><th>停損價</th><th>信念 <span class="cursor-pointer text-accent" onclick="event.stopPropagation();openInfoHelp('信念說明', \`<p><strong>信念（Conviction）是什麼？</strong></p><p>這是 AI Agent 對該標的推薦信心分數，範圍通常為 <strong>0 ~ 100</strong>。</p><ul style='margin:6px 0;padding-left:18px;line-height:1.8'><li><strong>&gt;70</strong>：高信念，AI 認為該標的強烈符合策略條件且風險可控。</li><li><strong>40~70</strong>：中等信念，條件部分符合，但可能存在不確定性。</li><li><strong>&lt;40</strong>：低信念，條件邊緣符合，容易被控制層過濾。</li></ul><p>當多個 AI 同時推薦同一標的時，控制層可能會對信念較低的推薦進行懲罰或過濾。</p>\`)">ℹ️</span></th><th>敘事/產業影響 <span class="cursor-pointer text-accent" onclick="event.stopPropagation();openInfoHelp('敘事與產業影響說明', \`<p><strong>敘事影響（Narrative Influence）</strong></p><p>顯示該推薦受到哪些宏觀敘事事件的影響，以及產業週期調整資訊。</p><ul style='margin:6px 0;padding-left:18px;line-height:1.8'><li><strong>藍色標籤</strong>：關聯的宏觀敘事主題（如 AI_capex_surge、US_rates_up 等）。</li><li><strong>紫色標籤</strong>：產業週期階段與週期評分。</li><li>數字表示關聯的敘事事件數量。</li></ul><p>這些資訊來自後台的 NarrativeConvictionModulator 與 IndustryCycleModulator，反映宏觀環境對個股推薦的動態調整。</p>\`)">ℹ️</span></th><th>因子總分 <span class="cursor-pointer text-accent" onclick="event.stopPropagation();openInfoHelp('因子總分說明', \`<p><strong>因子總分（Factor Total Score）是什麼？</strong></p><p>這是多因子模型的加權綜合分數，範圍約為 <strong>-1.0 ~ 1.0</strong>。</p><ul style='margin:6px 0;padding-left:18px;line-height:1.8'><li><strong>&gt;0.5</strong>：強烈正向，多因子同時看多。</li><li><strong>0 ~ 0.5</strong>：偏多但力道不足，部分因子可能是負分。</li><li><strong>&lt;0</strong>：整體偏空，建議謹慎。</li></ul><p>公式：動能 0.30 + 價值 0.25 + 品質 0.25 + Agent 0.20。進一步細項請至「決策鏈」頁面查看。</p>\`)">ℹ️</span></th><th>隔日回測報酬 <span class="cursor-pointer text-accent" onclick="event.stopPropagation();openInfoHelp('隔日回測報酬說明', \`<p><strong>隔日回測報酬是什麼？</strong></p><p>這是回測模擬中「以當日收盤價進場，隔日收盤價出場」的報酬率。</p><p>數值可正可負，例如 <strong>+3.2%</strong> 代表隔日上漲，<strong>-1.5%</strong> 代表隔日下跌。</p><p><strong>這不是未來保證</strong>，而是用來驗證該 Agent 在當時市場體制下的選股品質。持續為正代表策略相對有效。</p>\`)">ℹ️</span></th><th>價量標籤 + 推薦理由</th><th>操作</th></tr></thead>
      <tbody id="pipelineTableBody">
        ${buildTableRows(pagedItems)}
      </tbody>
    </table>
    </div>
    </div>
    ${paginationControls}
  ` : '';

  const emptyMsg = !items.length ? (finalOutputs === 0
    ? '本場次經控制層審核後，沒有任何標的被放行進入模擬投組。原因可能是風控長強制阻擋，或所有推薦均被投資長過濾。'
    : `控制層放行 ${finalOutputs} 筆，但投資管線暫無詳細標的資料（可能該場次尚未載入管線數據）。`) : '';

  const fallbackBanner = data.is_fallback_session ? `
    <div style="margin-bottom:12px;padding:10px 14px;background:rgba(245,158,11,.1);border:1px solid rgba(245,158,11,.3);border-radius:8px;display:flex;align-items:center;gap:10px;flex-wrap:wrap">
      <span style="font-size:13px">⚠️ ${escapeHtml(data.fallback_message || '已自動切換至最近有數據的場次')}</span>
      <button onclick="switchPage('reports')" style="font-size:11px;padding:3px 10px;border-radius:4px;border:1px solid var(--warn);background:rgba(245,158,11,.15);color:var(--warn);cursor:pointer;margin-left:auto">🚀 啟動新回測</button>
    </div>
  ` : '';

  const emptyStateWithAction = !items.length ? `
    <div class="empty-state-guidance" style="padding:32px 16px">
      <div class="icon" style="font-size:40px;margin-bottom:12px">📊</div>
      <div class="title" style="font-size:15px;margin-bottom:8px">尚無回測場次資料</div>
      <div class="desc" style="font-size:13px;margin-bottom:16px">執行回測後將自動顯示推薦明細與控制層結果</div>
      <button onclick="switchPage('reports')" style="font-size:13px;padding:8px 16px;border-radius:6px;border:1px solid var(--accent);background:rgba(79,193,255,.15);color:var(--accent);cursor:pointer">前往【最新回測】啟動回測 →</button>
    </div>
  ` : '';

  el.innerHTML = `
    ${fallbackBanner}
    <div style="margin-bottom:8px;font-size:12px;display:flex;align-items:center;gap:8px;flex-wrap:wrap"><span>場次：</span>${sessionSelect} · 市場狀態：<strong>${regimeLabel(data.regime || '-')}</strong> · ${recordedAt}</div>
    <div class="mb-sm text-muted text-sm">
      本場次共有 ${rawInputs} 筆 AI 推薦，經控制層後放行 ${finalOutputs} 筆${filteredCount > 0 ? `（過濾 ${filteredCount} 筆）` : ''}。
    </div>
    ${guardBadges}
    <div style="display:flex;justify-content:flex-end;margin-bottom:6px">
      <button onclick="exportTableToCSV('pipelineTable','pipeline_${data.session_id}.csv')" style="font-size:11px;padding:3px 10px;border-radius:4px;border:1px solid var(--border);background:var(--bg);color:var(--text);cursor:pointer">📥 匯出 CSV</button>
    </div>
    <div class="mb-sm text-sm">
      <label class="cursor-pointer select-none">
        <input type="checkbox" id="pipelineShowAll" ${showAll ? 'checked' : ''} onchange="togglePipelineShowAll(this)">
        <span class="ml-xs">顯示全部被過濾項目（含未通過控制層）</span>
      </label>
    </div>
    ${showScreenedCheckbox}
    ${emptyMsg ? `<div style="font-size:12px;color:var(--warn);margin-bottom:8px">${emptyMsg}</div>` : ''}
    ${pipelineTable}
    ${emptyStateWithAction}
    ${screenedSection}
  `;
}
export async function togglePipelineShowScreened(checkbox) {
  const sessionSelect = document.getElementById('pipelineSessionSelect');
  const sessionId = sessionSelect ? sessionSelect.value : '';
  const showAllCheckbox = document.getElementById('pipelineShowAll');
  const showAll = showAllCheckbox ? showAllCheckbox.checked : false;
  let url = '/api/dashboard/recommendation-pipeline';
  const params = [];
  if (sessionId) params.push('session_id=' + encodeURIComponent(sessionId));
  if (showAll) params.push('show_all=true');
  if (params.length) url += '?' + params.join('&');
  const data = await getJSON(url);
  renderPipeline(data, showAll, sessionId, checkbox.checked);
}
export async function togglePipelineShowAll(checkbox) {
  const sessionSelect = document.getElementById('pipelineSessionSelect');
  const sessionId = sessionSelect ? sessionSelect.value : '';
  const showScreenedCheckbox = document.getElementById('pipelineShowScreened');
  const showScreened = showScreenedCheckbox ? showScreenedCheckbox.checked : false;
  const url = (checkbox.checked ? '/api/dashboard/recommendation-pipeline?show_all=true' : '/api/dashboard/recommendation-pipeline') + (sessionId ? '&session_id=' + encodeURIComponent(sessionId) : '');
  const data = await getJSON(url);
  renderPipeline(data, checkbox.checked, sessionId, showScreened);
}
export async function togglePipelineSession(select) {
  const showAllCheckbox = document.getElementById('pipelineShowAll');
  const showAll = showAllCheckbox ? showAllCheckbox.checked : false;
  const showScreenedCheckbox = document.getElementById('pipelineShowScreened');
  const showScreened = showScreenedCheckbox ? showScreenedCheckbox.checked : false;
  const url = '/api/dashboard/recommendation-pipeline?session_id=' + encodeURIComponent(select.value) + (showAll ? '&show_all=true' : '');
  const data = await getJSON(url);
  renderPipeline(data, showAll, select.value, showScreened);
}

export function toggleWorkflowScreening() {
  const showScreenedCheckbox = document.getElementById('pipelineShowScreened');
  if (showScreenedCheckbox) {
    showScreenedCheckbox.checked = !showScreenedCheckbox.checked;
    togglePipelineShowScreened(showScreenedCheckbox);
  }
}

if (typeof window !== "undefined") Object.assign(window, {
  applyFilters, clearFilters, toggleFilterPanel, toggleWorkflowScreening
});
