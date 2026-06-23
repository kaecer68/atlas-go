// 投資心法庫頁面 (Strategy Techniques Library) —
// 取代舊的 eventlogic-rules.js，作為 Atlas 5 層框架（L1 全球流動性 / L2
// 外資行為 / L3 產業催化 / L4 匯率籌碼 / L5 地緣政治）與 4 核心短線指標
//（外資現貨 / TSM ADR / NVDA / DXY）的投資心法看板。
//
// 心法來源:
//   - 6 條既有規則遷移自 internal/eventlogic
//   - 3 條新增 L5 地緣政治心法（taiwan-strait / china-slowdown / us-tariff）
//   - 詳見 internal/strategy_techniques + data/seeds/strategy_techniques.json
//
// 自我修正: Wave 4 補 detector + corrector + 混合歸因（規則化 + LLM 加註）
// 視覺規範: 紅漲綠跌 (--up/--down) 為市場方向、--risk-high/low 為風險等級

import { escapeHtml } from '../shared/app-utils.js';
import { classifyFetchError } from '../shared/fetch-error.js';

const STATE = {
  strategies: [],
  layers: [],         // [{layer, count}] 5 層
  activeLayer: 'all', // 'all' | 'L1' | 'L2' | 'L3' | 'L4' | 'L5'
  coreIndicators: null,
  dataStatus: 'idle', // 'idle' | 'ok' | 'partial' | 'empty' | 'failed'
  errors: {},         // { [url]: classifyFetchError result }
  attributionCache: {},
};

const LAYER_META = {
  L1: { name: 'L1 全球流動性', color: 'var(--color-info)',    desc: 'Fed 利率、DXY、US10Y' },
  L2: { name: 'L2 外資行為',   color: '#a855f7',               desc: '外資現貨買賣超、期貨淨多空' },
  L3: { name: 'L3 產業催化',   color: 'var(--color-success)',  desc: '台積電法說、輝達、費半' },
  L4: { name: 'L4 匯率籌碼',   color: 'var(--color-warning)',  desc: 'USD_TWD、融資、大戶動向' },
  L5: { name: 'L5 地緣政治',   color: 'var(--color-danger)',   desc: '台海、關稅、中美科技戰' },
};

const RISK_BADGE = {
  low:    { class: 'ok',    label: '低風險' },
  medium: { class: 'warn',  label: '中風險' },
  high:   { class: 'err',   label: '高風險' },
};

const STATUS_BADGE = {
  active:   { class: 'ok',    label: '活躍' },
  degraded: { class: 'warn',  label: '降級' },
  expired:  { class: 'err',   label: '過期' },
};

const DIRECTION_GLYPH = {
  up:       { class: 'text-up',   label: '↑' },
  down:     { class: 'text-down', label: '↓' },
  volatile: { class: 'text-warn', label: '⚡' },
};

const LAYER_FILTERS = ['all', 'L1', 'L2', 'L3', 'L4', 'L5'];

async function fetchJSON(url, opts) {
  let r;
  try {
    r = await fetch(url, opts);
  } catch (networkErr) {
    throw networkErr;
  }
  if (r.status === 410) {
    const body = await r.json().catch(() => ({}));
    const e = new Error(body.error || 'moved');
    e.status = 410;
    throw e;
  }
  if (!r.ok) {
    const e = new Error(`HTTP ${r.status} ${r.statusText || ''}`.trim());
    e.status = r.status;
    throw e;
  }
  return r.json();
}

export async function renderStrategiesPage(root) {
  root.innerHTML = `
    <details class="help-details" open>
      <summary><strong>💡 什麼是投資心法庫？</strong></summary>
      本頁是 <strong>Atlas 5 層框架</strong>（L1~L5）的投資心法看板，搭配
      <strong>4 核心短線指標</strong>（外資現貨 / TSM ADR / NVDA / DXY），
      作為快速掌握投資策略的入口。每條心法皆標註 5 層歸屬、適用情境、命中率與
      失效歸因歷史；命中並非保證，請視為「投資組合的參考依據」而非「交易訊號」。
    </details>
    <div id="strategiesContent" class="empty loading">載入中…</div>
  `;
  const slot = root.querySelector('#strategiesContent');
  try {
    await loadStrategiesData();
    slot.classList.remove('loading');
    slot.innerHTML = renderSkeleton();
    render();
  } catch (e) {
    slot.classList.remove('loading');
    const classified = classifyFetchError(e, 'strategies page');
    slot.innerHTML = `
      <div class="empty error">
        <div>${escapeHtml(classified.message)}</div>
        ${classified.hint ? `<small class="text-muted">${escapeHtml(classified.hint)}</small>` : ''}
      </div>
    `;
  }

  function renderSkeleton() {
    return `
      <div class="kpi-grid" id="kpiStrip"></div>
      <div class="kpi-grid mt-sm" id="coreIndicatorStrip"></div>
      <div class="filter-tabs mt-md" id="layerTabs"></div>
      <div class="strategy-grid mt-md" id="strategyCards"></div>
    `;
  }

  function renderPartialBanner() {
    var status = STATE.dataStatus;
    if (status === 'ok' || status === 'idle') return '';
    var errorEntries = Object.entries(STATE.errors || {});

    if (status === 'failed') {
      var firstErr = errorEntries[0];
      var hint = firstErr && firstErr[1] ? firstErr[1].hint : '';
      var firstKind = firstErr && firstErr[1] ? firstErr[1].kind : '';
      var message = firstErr && firstErr[1] ? firstErr[1].message : '後端資料來源全部失敗';
      return `
        <div class="error-banner" role="alert">
          <div><strong>載入失敗</strong>：${escapeHtml(message)}${firstKind ? ' (' + escapeHtml(firstKind) + ')' : ''}</div>
          ${hint ? `<small class="text-muted">${escapeHtml(hint)}</small>` : ''}
        </div>
      `;
    }
    if (status === 'empty') {
      return `
        <div class="error-banner error-banner--warning" role="status">
          <div><strong>資料庫為空</strong>：後端回傳 0 筆心法</div>
          <small class="text-muted">可能原因：seed 載入但篩選後為空、或 schema 欄位改名。請聯絡管理員確認 <code>data/seeds/strategy_techniques.json</code>。</small>
        </div>
      `;
    }
    if (status === 'partial') {
      var failedUrls = errorEntries.map(function(e) { return e[0]; }).join(', ');
      return `
        <div class="error-banner error-banner--warning" role="status">
          <div><strong>部分資料載入失敗</strong>：${escapeHtml(failedUrls)}</div>
          <small class="text-muted">已顯示可取得的資料；失敗區塊以「--」標示。</small>
        </div>
      `;
    }
    return '';
  }

  function render() {
    var banner = renderPartialBanner();
    if (banner) slot.insertAdjacentHTML('afterbegin', banner);
    renderKPIs();
    renderCoreIndicators();
    renderLayerTabs();
    renderStrategyCards();
    renderModalPlaceholders();
    bindGlobalHandlers();
  }

  function renderKPIs() {
    const total = STATE.strategies.length;
    const active = STATE.strategies.filter(s => s.status === 'active').length;
    const layersCovered = STATE.layers.filter(l => l.count > 0).length;
    const avgHitRate = total === 0 ? 0 :
      STATE.strategies.reduce((sum, s) => sum + (s.hit_rate || 0), 0) / total;
    document.getElementById('kpiStrip').innerHTML = `
      <div class="kpi-card"><div class="kpi-label">總心法數</div>
        <div class="kpi-value">${total}</div></div>
      <div class="kpi-card"><div class="kpi-label">活躍心法</div>
        <div class="kpi-value">${active}</div></div>
      <div class="kpi-card"><div class="kpi-label">5 層覆蓋</div>
        <div class="kpi-value">${layersCovered}/5</div></div>
      <div class="kpi-card"><div class="kpi-label">平均命中率</div>
        <div class="kpi-value">${(avgHitRate * 100).toFixed(1)}%</div></div>
    `;
  }

  function renderCoreIndicators() {
    const c = STATE.coreIndicators;
    const items = [
      { label: '外資現貨 (TWD 億)', value: c ? c.foreign_capital_net_twd : 0,
        fmt: v => (v / 1e8).toFixed(1) },
      { label: 'TSM ADR (%)',   value: c ? c.tsm_adr_pct  : 0, fmt: v => v.toFixed(2) + '%' },
      { label: 'NVDA (%)',      value: c ? c.nvda_pct     : 0, fmt: v => v.toFixed(2) + '%' },
      { label: 'DXY (%)',       value: c ? c.dxy_pct      : 0, fmt: v => v.toFixed(2) + '%' },
    ];
    document.getElementById('coreIndicatorStrip').innerHTML = items.map(it => `
      <div class="kpi-card">
        <div class="kpi-label">${escapeHtml(it.label)}</div>
        <div class="kpi-value ${(it.value > 0 ? 'text-up' : it.value < 0 ? 'text-down' : '')}">
          ${escapeHtml(it.fmt(it.value))}
        </div>
      </div>
    `).join('');
  }

  function renderLayerTabs() {
    const tabs = LAYER_FILTERS.map(layer => {
      const count = layer === 'all'
        ? STATE.strategies.length
        : (STATE.layers.find(l => l.layer === layer)?.count || 0);
      const meta = LAYER_META[layer];
      const label = layer === 'all' ? '全部' : (meta ? meta.name : layer);
      const active = STATE.activeLayer === layer ? 'active' : '';
      return `<button class="view-btn ${active}" data-layer="${layer}">${escapeHtml(label)} (${count})</button>`;
    }).join('');
    document.getElementById('layerTabs').innerHTML = tabs;
  }

  function renderStrategyCards() {
    const filtered = STATE.activeLayer === 'all'
      ? STATE.strategies
      : STATE.strategies.filter(s => s.layer === STATE.activeLayer);
    if (filtered.length === 0) {
      document.getElementById('strategyCards').innerHTML =
        '<div class="empty">此層尚無心法，點擊下方「＋ 新增心法」開始建立</div>';
      return;
    }
    document.getElementById('strategyCards').innerHTML = filtered.map(renderCard).join('');
  }

  function renderCard(s) {
    const layer = LAYER_META[s.layer] || { name: s.layer, color: 'var(--muted)' };
    const status = STATUS_BADGE[s.status] || { class: '', label: s.status };
    const risk = RISK_BADGE[s.risk] || { class: '', label: s.risk };
    const dir = DIRECTION_GLYPH[s.direction] || { class: '', label: s.direction };
    const themes = (s.themes || []).map(t => `<span class="badge ok">${escapeHtml(t)}</span>`).join(' ');
    const sectors = (s.affected_sectors || []).map(t =>
      `<span class="text-xs-muted">${escapeHtml(t)}</span>`).join(' · ');
    const attribution = (s.attribution || []).slice(0, 2).map(a =>
      `<div class="text-xs-muted">• ${escapeHtml(a)}</div>`).join('');
    return `
      <div class="panel strategy-card" data-id="${escapeHtml(s.id)}" style="border-left:4px solid ${layer.color}">
        <div class="flex-between">
          <div>
            <span class="text-xs-muted">${escapeHtml(layer.name)}</span>
            ${status ? `<span class="badge ${status.class}">${escapeHtml(status.label)}</span>` : ''}
          </div>
          <span class="${dir.class}">${escapeHtml(dir.label)}</span>
        </div>
        <h3 class="m-0 mt-xs">${escapeHtml(s.name)}</h3>
        <p class="text-sm text-muted mt-xs">${escapeHtml(s.summary)}</p>
        <div class="mt-xs">${themes}</div>
        ${sectors ? `<div class="mt-xs">${sectors}</div>` : ''}
        <div class="flex-between mt-sm">
          <div>
            <span class="kpi-label">命中率</span>
            <span class="text-${s.hit_rate >= 0.6 ? 'up' : s.hit_rate < 0.4 ? 'down' : 'muted'}">
              ${(s.hit_rate * 100).toFixed(0)}%
            </span>
          </div>
          <div>
            ${risk ? `<span class="badge ${risk.class}">${escapeHtml(risk.label)}</span>` : ''}
          </div>
        </div>
        ${attribution ? `<div class="mt-xs attribution-preview">${attribution}</div>` : ''}
        <div class="control-group mt-sm">
          <button data-action="ai-annotate" data-id="${escapeHtml(s.id)}">🤖 AI 歸因</button>
          <button data-action="validate" data-id="${escapeHtml(s.id)}">✓ 驗證</button>
        </div>
      </div>
    `;
  }

  function renderModalPlaceholders() {
    if (!document.getElementById('attributionModal')) {
      const m = document.createElement('div');
      m.className = 'modal-overlay';
      m.id = 'attributionModal';
      m.setAttribute('role', 'dialog');
      m.setAttribute('aria-modal', 'true');
      m.innerHTML = `
        <div class="modal" style="width:min(640px,94vw)">
          <h3>🤖 AI 失效歸因</h3>
          <div id="attributionContent">載入中…</div>
          <div class="control-group mt-14 justify-end">
            <button data-action="close-attribution">關閉</button>
          </div>
        </div>
      `;
      document.body.appendChild(m);
    }
  }

  function bindGlobalHandlers() {
    document.querySelectorAll('#layerTabs [data-layer]').forEach(btn => {
      btn.addEventListener('click', () => {
        STATE.activeLayer = btn.dataset.layer;
        renderLayerTabs();
        renderStrategyCards();
      });
    });
    document.querySelectorAll('[data-action="ai-annotate"]').forEach(btn => {
      btn.addEventListener('click', () => aiAnnotate(btn.dataset.id));
    });
    document.querySelectorAll('[data-action="validate"]').forEach(btn => {
      btn.addEventListener('click', () => validateStrategy(btn.dataset.id));
    });
    const closeBtn = document.querySelector('[data-action="close-attribution"]');
    if (closeBtn && !closeBtn._bound) {
      closeBtn._bound = true;
      closeBtn.addEventListener('click', () => closeAttribution());
    }
  }
}

async function loadStrategiesData() {
  const results = await Promise.allSettled([
    fetchJSON('/api/strategies'),
    fetchJSON('/api/strategies/layers'),
    fetchJSON('/api/dashboard/decision-chain'),
  ]);

  const errors = {};
  let strategies = [];
  let layers = [];
  let chain = null;
  let coreFailures = 0;

  if (results[0].status === 'fulfilled') {
    strategies = (results[0].value && results[0].value.strategies) || [];
  } else {
    errors['/api/strategies'] = classifyFetchError(results[0].reason, '/api/strategies');
    coreFailures++;
  }

  if (results[1].status === 'fulfilled') {
    layers = (results[1].value && results[1].value.layers) || [];
  } else {
    errors['/api/strategies/layers'] = classifyFetchError(results[1].reason, '/api/strategies/layers');
    coreFailures++;
  }

  if (results[2].status === 'fulfilled') {
    chain = results[2].value;
  } else {
    errors['/api/dashboard/decision-chain'] = classifyFetchError(results[2].reason, '/api/dashboard/decision-chain');
  }

  STATE.strategies = strategies;
  STATE.layers = layers;
  STATE.coreIndicators = chain && chain.core_indicators ? chain.core_indicators : null;
  STATE.errors = errors;

  if (coreFailures === 0) {
    STATE.dataStatus = strategies.length === 0 ? 'empty' : 'ok';
  } else if (coreFailures === 2) {
    STATE.dataStatus = 'failed';
  } else {
    STATE.dataStatus = 'partial';
  }
}

async function aiAnnotate(id) {
  const modal = document.getElementById('attributionModal');
  const body = document.getElementById('attributionContent');
  modal.style.display = 'flex';
  body.innerHTML = '<div class="empty">🤖 正在呼叫 AI 歸因（最長 30 秒）…</div>';

  let staticData = null;
  try {
    staticData = await fetchJSON(`/api/strategies/${encodeURIComponent(id)}/attribution`);
  } catch (e) {
    // 靜態歸因失敗不阻擋 AI 路徑
  }

  try {
    const r = await fetchJSON(`/api/strategies/${encodeURIComponent(id)}/annotate`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({}),
    });
    renderAIAttribution(id, r, staticData);
  } catch (e) {
    const msg = e && e.message ? e.message : String(e);
    if (staticData) {
      renderAIAttribution(id, {
        annotation: '',
        mode: 'rule_based',
        note: 'AI 不可用，顯示靜態歸因 (' + msg + ')',
      }, staticData);
    } else {
      body.innerHTML = `<div class="empty error">AI 歸因失敗：${escapeHtml(msg)}</div>`;
    }
  }
}

function renderAIAttribution(id, aiResp, staticData) {
  const body = document.getElementById('attributionContent');
  const ai = (aiResp && aiResp.annotation) ? aiResp.annotation : '';
  const isFromLLM = ai.length > 0;
  const mode = (aiResp && aiResp.mode) || (isFromLLM ? 'llm_annotated' : 'rule_based');

  let html = `<div class="mb-xs text-sm text-muted">心法：<strong>${escapeHtml(id)}</strong></div>`;

  if (isFromLLM) {
    html += `<div class="mb-sm"><span class="badge ok">🤖 LLM 即時生成</span></div>`;
    html += `<div style="background:var(--bg-card-2);padding:12px;border-radius:4px;line-height:1.6">${escapeHtml(ai)}</div>`;
  } else {
    const note = (aiResp && aiResp.note) ? aiResp.note : 'LLM 未配置或回傳空';
    html += `<div class="mb-sm"><span class="badge warn">📜 規則化歸因</span> <span class="text-xs-muted">${escapeHtml(note)}</span></div>`;
  }

  const staticItems = (staticData && staticData.attribution) || [];
  if (staticItems.length > 0) {
    html += `<div class="mt-sm"><h4 class="text-sm m-0">靜態歸因（備查）</h4>`;
    html += `<ul style="padding-left:18px;line-height:1.7;margin-top:6px">${staticItems.map(a => `<li>${escapeHtml(a)}</li>`).join('')}</ul></div>`;
  }

  html += `<div class="mt-sm text-xs-muted">attribution_mode=${escapeHtml(mode)}</div>`;
  body.innerHTML = html;
}

function closeAttribution() {
  const modal = document.getElementById('attributionModal');
  if (modal) modal.style.display = 'none';
}

async function validateStrategy(id) {
  try {
    const r = await fetchJSON(`/api/strategies/${encodeURIComponent(id)}/validate`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({}),
    });
    alert(`驗證結果：${r.status || JSON.stringify(r)}`);
  } catch (e) {
    alert('驗證失敗：' + e.message);
  }
}

window._strategiesSetLayer = layer => { STATE.activeLayer = layer; };
window._strategiesRefresh  = async () => { await loadStrategiesData(); };
