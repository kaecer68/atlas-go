import { agentName, stockName, regimeLabel } from '../names.js';
import { computePipelineSummary, factorBar, renderFactorMini, renderFactorBreakdown, toggleBreakdown } from './dashboard.js';
import { formatDate, getJSON, notify, renderEmptyState } from '../shared/app-utils.js';
import { escapeHtml } from '../shared/utils.js';
import { fmtSafeSignedPct, fmtSafeNumber } from '../shared/format-metric.js';

function fmtPrice(v) {
  return fmtSafeNumber(v, { decimals: 2, useGrouping: true });
}

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

// Delegated click handler for pipeline action buttons (legacy .pipeline-action + new gear-icon override)
if (typeof document !== 'undefined') {
  document.addEventListener('click', function(e) {
    // Legacy direct buttons (放行/否決/補追)
    const btn = e.target.closest('.pipeline-action');
    if (btn) {
      const text = btn.textContent.trim();
      if (text === '放行' || text === '補追' || text === '否決') {
        const symbol = btn.dataset.symbol;
        const agentId = btn.dataset.agentId;
        if (!symbol || !agentId) return;
        e.stopPropagation();
        if (text === '否決') {
          rejectRec(btn, symbol, agentId);
        } else {
          approveRec(btn, symbol, agentId);
        }
        return;
      }
    }
    // Machine-first gear-icon override button
    const overrideBtn = e.target.closest('.pipeline-override-btn');
    if (overrideBtn) {
      handleOverrideClick({ currentTarget: overrideBtn, stopPropagation: () => {} });
      return;
    }
  });
}

export function renderConvictionBreakdown(cb) {
  if (!cb) return '<div class="text-muted text-xs">無計算明細</div>';
  const steps = (cb.steps || []).map(s => {
    const deltaCls = s.delta > 0 ? 'color:var(--up)' : (s.delta < 0 ? 'color:var(--down)' : 'color:var(--muted)');
    const deltaLabel = s.delta > 0 ? '+' + s.delta : String(s.delta);
    const prov = s.source ? `<span class="badge" style="font-size:9px;padding:1px 4px;background:rgba(59,130,246,.12);border:1px solid rgba(59,130,246,.3);color:var(--color-info);margin-left:4px">${s.source}${s.param_ref ? ':' + s.param_ref.split('.').pop() : ''}</span>` : '';
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

// renderPipelineNarrative builds the 敘事歸因 block for a pipeline item.
// Renders the reasoning chain (theme/region/confidence strings) and the
// supporting event badges when present; returns '' when neither exists
// (no narrative attribution available — zero visual disruption).
function renderPipelineNarrative(it) {
  const chain = Array.isArray(it.reasoning_chain) ? it.reasoning_chain : [];
  const events = Array.isArray(it.supporting_events) ? it.supporting_events : [];
  if (!chain.length && !events.length) return '';

  const chainHtml = chain.length
    ? `<div class="pipeline-narrative-chain">${chain.map(c => `<span class="pipeline-narrative-item">${escapeHtml(c)}</span>`).join('')}</div>`
    : '';
  const eventsHtml = events.length
    ? `<div class="pipeline-narrative-events">${events.map(e => `<span class="badge pipeline-narrative-event">${escapeHtml(e)}</span>`).join('')}</div>`
    : '';
  return `<div class="pipeline-narrative"><strong>敘事歸因：</strong>${chainHtml}${eventsHtml}</div>`;
}

export function renderPipeline(data, showAll, sessionId, showScreened) {
  if (showAll === undefined) showAll = false;
  if (showScreened === undefined) showScreened = false;
  const el = document.getElementById('recommendationPipeline');
  if (!el) return;
  if (!data || !data.session_id) { el.innerHTML = renderEmptyState('尚無回測場次資料', '執行回測後將自動顯示'); el.classList.remove('loading'); return; }
  el.classList.remove('loading');

  // Store data for filter access
  window._pipelineData = data;

  const { rawInputs, finalOutputs, filteredCount, guard } = computePipelineSummary(data.guard_outcomes, data.items);
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

  const sessionList = (window.pipelineSessions && window.pipelineSessions.length) ? window.pipelineSessions : [];
  const sessionSelect = sessionList.length ? `
    <select id="pipelineSessionSelect" style="font-size:12px;padding:2px 6px;border-radius:4px;border:1px solid var(--border);background:var(--panel);color:var(--text)" onchange="togglePipelineSession(this)">
      ${sessionList.map(s => `<option value="${escapeHtml(s.session_id)}" ${s.session_id === data.session_id ? 'selected' : ''}>${escapeHtml(s.session_id)} · ${escapeHtml(s.regime)} · ${new Date(s.recorded_at).toLocaleDateString('zh-TW')}</option>`).join('')}
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
            ${screenedItems.map(s => `<tr style="border-left:3px solid var(--border);color:var(--muted)">
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
    const rowStyle = !it.passed_guards ? 'background:rgba(239,68,68,0.06);border-left:3px solid var(--color-danger)' : 'border-left:3px solid var(--color-success)';
    const crowded = (it.reason || '').includes('[crowded:');
    const badge = crowded ? `<span class="badge warn">擁擠</span> ` : '';
    const sideLabel = it.side === 'BUY' ? '<span style="color:var(--up);font-weight:700">買入</span>' : (it.side === 'SELL' ? '<span style="color:var(--down);font-weight:700">賣出</span>' : (it.side === 'REDUCE' ? '<span style="color:var(--warn);font-weight:700">減持</span>' : '-'));
    const priceLabel = typeof it.price === 'number' ? fmtPrice(it.price) : '—';
    const targetPriceLabel = typeof it.target_price === 'number' ? fmtPrice(it.target_price) : '—';
    const stopLossPriceLabel = typeof it.stop_loss_price === 'number' ? fmtPrice(it.stop_loss_price) : '—';
    let frCls = 'color:var(--muted)';
    let frIcon = '➖ ';
    if (it.forward_return > 0) { frCls = 'color:var(--up);font-weight:700'; frIcon = '📈 '; }
    else if (it.forward_return < 0) { frCls = 'color:var(--down);font-weight:700'; frIcon = '📉 '; }
    const layerMap = { sector: '產業主題', style: '風格因子', superinvestor: '超級投資者', context: '總經情境', control: '控制層', macro: '總經情境' };
    const layerName = layerMap[it.layer] || (it.layer || (it.agent_id === 'alpha_discovery' ? '風格因子' : '-'));
    const tags = (it.tags || []).map(t => `<span class="badge" style="font-size:10px;padding:1px 5px;background:var(--bg);border:1px solid var(--border);margin-right:3px">${escapeHtml(t)}</span>`).join('');
    const actionBtns = it.passed_guards
      ? `<button class="pipeline-override-btn" data-symbol="${escapeHtml(it.symbol)}" data-agent-id="${escapeHtml(it.agent_id)}" data-guards="1" style="background:none;border:1px solid var(--border);border-radius:4px;padding:2px 6px;cursor:pointer;font-size:13px;color:var(--muted);line-height:1" title="人工覆寫（機器已放行）">⚙</button>`
      : `<button class="pipeline-override-btn" data-symbol="${escapeHtml(it.symbol)}" data-agent-id="${escapeHtml(it.agent_id)}" data-guards="0" style="background:none;border:1px solid var(--border);border-radius:4px;padding:2px 6px;cursor:pointer;font-size:13px;color:var(--color-warning);line-height:1" title="人工覆寫（機器已阻擋）">⚙</button>`;
    const actionHelp = `<span style="cursor:pointer;color:var(--accent);font-size:12px;margin-left:2px" onclick="event.stopPropagation();openInfoHelp('人工覆寫說明', \`<p><strong>機器優先原則</strong>：系統自動執行所有決策，人工覆寫僅用於對機器決策有不同意見的例外情況。</p><p>點擊 ⚙ 按鈕可選擇：</p><ul><li><strong>放行</strong>：人工背書此推薦，跳過控制層過濾。</li><li><strong>否決</strong>：人工拒絕此推薦，強制排除該組合。</li><li><strong>補追</strong>：對已被控制層擋下的標的進行人工放行。</li></ul><p>覆寫有效期 48 小時，過期後自動回復機器決策。</p>\`)">ℹ️</span>`;
    const narrativeEvents = it.narrative_event_ids || [];
    const narrativeCtx = it.narrative_context;
    const hasNarrative = narrativeEvents.length > 0 || narrativeCtx;
    const narrativeBadge = hasNarrative
      ? `<span class="badge" style="font-size:10px;padding:1px 5px;background:rgba(59,130,246,.15);border:1px solid rgba(59,130,246,.4);color:var(--color-info)">${narrativeCtx ? escapeHtml(narrativeCtx.primary_theme || narrativeCtx.theme || '敘事') : '敘事'} ${narrativeEvents.length > 0 ? narrativeEvents.length : ''}</span>`
      : '<span class="text-muted text-xs">-</span>';
    const industryCtx = it.industry_context;
    const hasIndustry = industryCtx && industryCtx.business_cycle;
    const industryBadge = hasIndustry
      ? `<span class="badge" style="font-size:10px;padding:1px 5px;background:rgba(139,92,246,.15);border:1px solid rgba(139,92,246,.4);color:var(--accent-secondary)">${escapeHtml(industryCtx.business_cycle)} ${industryCtx.cycle_confidence != null ? fmtSafeNumber(industryCtx.cycle_confidence, { percent: true, decimals: 0, suffix: '%' }) : ''}</span>`
      : '';
    const narrativeAttribution = renderPipelineNarrative(it);
    return `<tr class="pipeline-row ${cls}" style="${rowStyle}"><td>${escapeHtml(it.symbol)}</td><td>${escapeHtml(stockName(it.symbol)) || '-'}</td><td>${escapeHtml(agentName(it.agent_id))}（${escapeHtml(it.skill)}）</td><td>${escapeHtml(layerName)}</td><td>${sideLabel}</td><td>${priceLabel}</td><td>${targetPriceLabel}</td><td>${stopLossPriceLabel}</td><td>${it.conviction != null ? it.conviction : '-'}</td><td>${narrativeBadge}${industryBadge ? ' ' + industryBadge : ''}</td><td>${renderFactorMini(it.factor_scores)}</td><td style="${frCls}">${frIcon}${typeof it.forward_return === 'number' ? fmtSafeSignedPct(it.forward_return, 1) : '—'}</td><td><div style="display:flex;gap:3px;flex-wrap:wrap;margin-bottom:4px">${tags}</div>${badge}${it.reason ? escapeHtml(it.reason) : '-'}${it.guard_reason ? '<br><span class="text-muted text-xs">' + escapeHtml(it.guard_reason) + '</span>' : ''}${narrativeAttribution}</td><td>${actionBtns}${actionHelp}</td></tr>`;
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

  // FIX-2: when the pipeline has zero items AND every screened-out candidate
  // was rejected by the momentum filter, surface the real reason instead of
  // the generic control-layer message.
  const momentumRejects = screenedItems.filter(s => (s.criterion || '').indexOf('momentum') !== -1);
  const allRejectedByMomentum = momentumRejects.length > 0 && momentumRejects.length === screenedItems.length;
  const momentumThreshold = allRejectedByMomentum ? (momentumRejects[0].threshold != null ? String(momentumRejects[0].threshold) : '-') : '';
  const emptyMsg = !items.length ? (allRejectedByMomentum
    ? `全部 ${screenedItems.length} 檔候選均因「20 日動能（momentum_20d_min，門檻 ${escapeHtml(momentumThreshold)}）」被篩選層排除，故本場次無推薦標的。可勾選「顯示被篩選層排除的標的」查看明細；市場動能回復後將自動恢復推薦。`
    : (finalOutputs === 0
    ? '本場次經控制層審核後，沒有任何標的被放行進入模擬投組。原因可能是風控長強制阻擋，或所有推薦均被投資長過濾。'
    : `控制層放行 ${finalOutputs} 筆，但投資管線暫無詳細標的資料（可能該場次尚未載入管線數據）。`)) : '';

  const fallbackBanner = data.is_fallback_session ? `
    <div style="margin-bottom:12px;padding:10px 14px;background:rgba(245,158,11,.1);border:1px solid rgba(245,158,11,.3);border-radius:8px;display:flex;align-items:center;gap:10px;flex-wrap:wrap">
      <span style="font-size:13px">⚠️ ${escapeHtml(data.fallback_message || '已自動切換至最近有數據的場次')}</span>
      <button onclick="switchPage('reports')" style="font-size:11px;padding:3px 10px;border-radius:4px;border:1px solid var(--warn);background:rgba(245,158,11,.15);color:var(--warn);cursor:pointer;margin-left:auto">🚀 啟動新回測</button>
    </div>
  ` : '';
  const statusBanner = buildPipelineStatusBanner(data);
  const degradedBanner = data.status === 'degraded' ? statusBanner : '';

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
    ${degradedBanner}
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
  const parts = ['/api/dashboard/recommendation-pipeline'];
  if (checkbox.checked) parts.push('show_all=true');
  if (sessionId) parts.push('session_id=' + encodeURIComponent(sessionId));
  const url = parts[0] + (parts.length > 1 ? '?' + parts.slice(1).join('&') : '');
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


// Machine-first override: gear-icon click opens a compact dropdown with
// contextual options based on guard status (passed vs blocked).
export function handleOverrideClick(e) {
  e.stopPropagation();
  const btn = e.currentTarget;
  const symbol = btn.dataset.symbol;
  const agentId = btn.dataset.agentId;
  const passedGuards = btn.dataset.guards === '1';

  const existing = document.querySelector('.override-popover');
  if (existing) existing.remove();

  const popover = document.createElement('div');
  popover.className = 'override-popover';
  popover.style.cssText = 'position:absolute;z-index:999;background:var(--bg);border:1px solid var(--border);border-radius:6px;padding:8px;min-width:140px;box-shadow:0 4px 12px rgba(0,0,0,0.15)';
  popover.innerHTML = `
    <div style="font-size:11px;color:var(--muted);margin-bottom:6px">${symbol} · ${agentId}</div>
    ${passedGuards
      ? '<button class="override-action" data-action="reject" style="display:block;width:100%;padding:4px 8px;margin-bottom:4px;border-radius:4px;border:1px solid var(--color-danger);background:rgba(239,68,68,.1);color:var(--color-danger);cursor:pointer;font-size:12px;text-align:left">🚫 強制否決</button>'
      : '<button class="override-action" data-action="approve" style="display:block;width:100%;padding:4px 8px;border-radius:4px;border:1px solid var(--color-success);background:rgba(34,197,94,.1);color:var(--color-success);cursor:pointer;font-size:12px;text-align:left">✅ 強制放行</button>'
    }
    <div style="margin-top:6px">
      <input type="text" class="override-reason" placeholder="覆寫原因（必填）" style="width:100%;box-sizing:border-box;padding:3px 6px;font-size:11px;border:1px solid var(--border);border-radius:3px;background:var(--bg);color:var(--text)">
    </div>
    <button class="override-submit" style="display:block;width:100%;margin-top:6px;padding:4px 8px;border-radius:4px;border:none;background:var(--accent);color:#fff;cursor:pointer;font-size:12px">確認送出</button>
    <div class="override-error" style="display:none;margin-top:4px;font-size:11px;color:var(--color-danger)"></div>
  `;

  const btnRect = btn.getBoundingClientRect();
  popover.style.left = (btnRect.left - 130) + 'px';
  popover.style.top = (btnRect.bottom + 4) + 'px';
  document.body.appendChild(popover);

  const popoverWidth = popover.offsetWidth;
  if (btnRect.left - popoverWidth < 0) {
    popover.style.left = (btnRect.right + 4) + 'px';
    if (popover.offsetLeft + popoverWidth > window.innerWidth) {
      popover.style.left = Math.max(4, window.innerWidth - popoverWidth - 4) + 'px';
    }
  }

  popover.querySelector('.override-submit').onclick = async () => {
    const reason = popover.querySelector('.override-reason').value.trim();
    if (reason.length < 4) {
      popover.querySelector('.override-error').style.display = 'block';
      popover.querySelector('.override-error').textContent = '原因至少需 4 個字元';
      return;
    }
    const action = popover.querySelector('.override-action').dataset.action;
    const endpoint = action === 'approve'
      ? '/api/control/approve-recommendation'
      : '/api/control/reject-recommendation';
    try {
      const resp = await fetch(endpoint, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ symbol, agent_id: agentId, reason, operator: 'admin' })
      });
      if (!resp.ok) throw new Error(await resp.text());
      popover.remove();
      window._pipelinePage = window._pipelinePage || 0;
      renderPipeline(window._pipelineData, document.getElementById('pipelineShowAll')?.checked || false, null, false);
    } catch (err) {
      popover.querySelector('.override-error').style.display = 'block';
      popover.querySelector('.override-error').textContent = '送出失敗: ' + err.message;
    }
  };

  const closeHandler = (ev) => {
    if (!popover.contains(ev.target) && ev.target !== btn) {
      popover.remove();
      document.removeEventListener('click', closeHandler);
    }
  };
  setTimeout(() => document.addEventListener('click', closeHandler), 0);
}

if (typeof window !== "undefined") Object.assign(window, {
  applyFilters, clearFilters, toggleFilterPanel, toggleWorkflowScreening, handleOverrideClick
});

// buildPipelineStatusBanner: 依後端 PipelineLoadStatus 與 is_fallback_session
// 組合回傳對應的 banner HTML。正常 (ok) 與未提供 status 時回傳空字串。
// 對應後端 internal/monitoring/service/pipeline.go 的 PipelineStatus* 常數。
export function buildPipelineStatusBanner(data) {
  if (!data) return '';
  const banners = [];
  if (data.is_fallback_session) {
    banners.push(`<div style="margin-bottom:12px;padding:10px 14px;background:rgba(245,158,11,.1);border:1px solid rgba(245,158,11,.3);border-radius:8px;display:flex;align-items:center;gap:10px;flex-wrap:wrap"><span style="font-size:13px">⚠️ ${escapeHtml(data.fallback_message || '已自動切換至最近有數據的場次')}</span><button onclick="switchPage('reports')" style="font-size:11px;padding:3px 10px;border-radius:4px;border:1px solid var(--warn);background:rgba(245,158,11,.15);color:var(--warn);cursor:pointer;margin-left:auto">🚀 啟動新回測</button></div>`);
  }
  switch (data.status) {
    case 'degraded':
      banners.push(`<div style="margin-bottom:12px;padding:10px 14px;background:rgba(245,158,11,.1);border:1px solid rgba(245,158,11,.35);border-radius:8px;display:flex;align-items:center;gap:10px;flex-wrap:wrap"><span class="badge warn">資料不完整</span><span style="font-size:13px">⚠️ ${escapeHtml(data.status_message || '本場次部分資料缺失（控制層審核記錄不可用），推薦清單仍可檢視。')}</span></div>`);
      break;
    case 'minimal':
      banners.push(`<div style="margin-bottom:12px;padding:10px 14px;background:rgba(107,114,128,.1);border:1px solid rgba(107,114,128,.35);border-radius:8px;display:flex;align-items:center;gap:10px;flex-wrap:wrap"><span class="badge">尚無資料</span><span style="font-size:13px">ℹ️ ${escapeHtml(data.status_message || '本場次尚無推薦產出記錄')}</span></div>`);
      break;
    case 'no_session':
      banners.push(`<div style="margin-bottom:12px;padding:10px 14px;background:rgba(59,130,246,.1);border:1px solid rgba(59,130,246,.35);border-radius:8px;display:flex;align-items:center;gap:10px;flex-wrap:wrap"><span class="badge info">尚未執行</span><span style="font-size:13px">ℹ️ ${escapeHtml(data.status_message || '尚未執行任何回測場次，請先執行回測')}</span><button onclick="switchPage('reports')" style="font-size:11px;padding:3px 10px;border-radius:4px;border:1px solid var(--color-info);background:rgba(59,130,246,.15);color:var(--color-info);cursor:pointer;margin-left:auto">🚀 啟動新回測</button></div>`);
      break;
    case 'error':
      banners.push(`<div style="margin-bottom:12px;padding:10px 14px;background:rgba(239,68,68,.1);border:1px solid rgba(239,68,68,.45);border-radius:8px;display:flex;align-items:center;gap:10px;flex-wrap:wrap"><span class="badge err">錯誤</span><span style="font-size:13px">❌ ${escapeHtml(data.status_message || '載入推薦管線資料時發生錯誤')}</span></div>`);
      break;
    case 'ok':
    case undefined:
    case '':
    case null:
      break;
    default:
      banners.push(`<div style="margin-bottom:12px;padding:10px 14px;background:rgba(107,114,128,.1);border:1px solid var(--border);border-radius:8px"><span style="font-size:12px;color:var(--muted)">未知的管線狀態：${escapeHtml(String(data.status))}</span></div>`);
  }
  return banners.join('');
}
