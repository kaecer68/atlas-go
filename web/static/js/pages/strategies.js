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

const STATE = {
  strategies: [],
  layers: [],         // [{layer, count}] 5 層
  activeLayer: 'all', // 'all' | 'L1' | 'L2' | 'L3' | 'L4' | 'L5'
  coreIndicators: null,
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
  const r = await fetch(url, opts);
  if (r.status === 410) {
    const body = await r.json().catch(() => ({}));
    throw new Error(body.error || 'moved');
  }
  if (!r.ok) throw new Error(`${url} -> ${r.status}`);
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
  slot.classList.remove('loading');
  slot.innerHTML = renderSkeleton();
  try {
    await loadStrategiesData();
    render();
  } catch (e) {
    slot.innerHTML = `<div class="empty error">載入失敗：${escapeHtml(e.message)}</div>`;
  }

  function renderSkeleton() {
    return `
      <div class="kpi-grid" id="kpiStrip"></div>
      <div class="kpi-grid mt-sm" id="coreIndicatorStrip"></div>
      <div class="filter-tabs mt-md" id="layerTabs"></div>
      <div class="strategy-grid mt-md" id="strategyCards"></div>
    `;
  }

  function render() {
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
          <button data-action="view-attribution" data-id="${escapeHtml(s.id)}">📜 歸因</button>
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
          <h3>📜 失效歸因歷史</h3>
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
    document.querySelectorAll('[data-action="view-attribution"]').forEach(btn => {
      btn.addEventListener('click', () => openAttribution(btn.dataset.id));
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
  const [strategiesResp, layersResp, chainResp] = await Promise.all([
    fetchJSON('/api/strategies'),
    fetchJSON('/api/strategies/layers'),
    fetchJSON('/api/dashboard/decision-chain').catch(() => null),
  ]);
  STATE.strategies = strategiesResp.strategies || [];
  STATE.layers = layersResp.layers || [];
  STATE.coreIndicators = chainResp ? (chainResp.core_indicators || null) : null;
}

async function openAttribution(id) {
  const modal = document.getElementById('attributionModal');
  const body = document.getElementById('attributionContent');
  modal.style.display = 'flex';
  if (STATE.attributionCache[id]) {
    renderAttribution(id, STATE.attributionCache[id]);
    return;
  }
  body.textContent = '載入中…';
  try {
    const r = await fetchJSON(`/api/strategies/${encodeURIComponent(id)}/attribution`);
    STATE.attributionCache[id] = r;
    renderAttribution(id, r);
  } catch (e) {
    body.innerHTML = `<div class="empty error">${escapeHtml(e.message)}</div>`;
  }
}

function renderAttribution(id, data) {
  const body = document.getElementById('attributionContent');
  const items = (data.attribution || []);
  if (items.length === 0) {
    body.innerHTML = `<div class="empty">${escapeHtml(id)} 目前無失效歸因記錄</div>`;
    return;
  }
  body.innerHTML = `
    <div class="mb-xs text-sm text-muted">心法：<strong>${escapeHtml(id)}</strong></div>
    <ul style="padding-left:18px;line-height:1.7">
      ${items.map(a => `<li>${escapeHtml(a)}</li>`).join('')}
    </ul>
  `;
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
