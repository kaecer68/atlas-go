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
import { fmtSafeNumber, fmtSafePct, fmtSafeSignedPct, fmtSafeDrawdown, fmtSafeSigned } from '../shared/format-metric.js';

const STATE = {
  strategies: [],
  layers: [],         // [{layer, count}] 5 層
  activeLayer: 'all', // 'all' | 'L1' | 'L2' | 'L3' | 'L4' | 'L5'
  coreIndicators: null,
  dataStatus: 'idle', // 'idle' | 'ok' | 'partial' | 'empty' | 'failed'
  errors: {},         // { [url]: classifyFetchError result }
  indicatorsError: null, // decision-chain 失敗但核心 OK 時的單獨錯誤
  attributionCache: {},
};

const FETCH_TIMEOUT_MS = 30000;

const LAYER_META = {
  L1: { name: 'L1 全球流動性', color: 'var(--layer-1)', desc: 'Fed 利率、DXY、US10Y' },
  L2: { name: 'L2 外資行為',   color: 'var(--layer-2)', desc: '外資現貨買賣超、期貨淨多空' },
  L3: { name: 'L3 產業催化',   color: 'var(--layer-3)', desc: '台積電法說、輝達、費半' },
  L4: { name: 'L4 匯率籌碼',   color: 'var(--layer-4)', desc: 'USD_TWD、融資、大戶動向' },
  L5: { name: 'L5 地緣政治',   color: 'var(--layer-5)', desc: '台海、關稅、中美科技戰' },
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

const CATEGORY_BADGE = {
  defensive:  { class: 'ok',   label: '防禦型' },
  aggressive: { class: 'warn', label: '攻擊型' },
  tactical:   { class: 'info', label: '事件型' },
};

const DIRECTION_GLYPH = {
  up:       { class: 'text-up',   label: '↑' },
  down:     { class: 'text-down', label: '↓' },
  volatile: { class: 'text-warn', label: '⚡' },
};

const LAYER_FILTERS = ['all', 'L1', 'L2', 'L3', 'L4', 'L5'];

async function fetchJSON(url, opts) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), FETCH_TIMEOUT_MS);
  let r;
  try {
    r = await fetch(url, { ...opts, signal: controller.signal });
  } finally {
    clearTimeout(timer);
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

export function processStrategiesResults(results) {
  if (!Array.isArray(results) || results.length < 3) {
    throw new Error(`processStrategiesResults 需要長度 ≥ 3 的 results 陣列，實際收到 ${results && results.length}`);
  }
  const errors = {};
  const schemaErr = (url, missing) => ({
    kind: 'schema',
    message: `回應缺少 ${missing} 欄位`,
    recoverable: false,
    hint: '請檢查後端 schema 是否變更',
  });
  let strategies = [];
  let layers = [];
  let chain = null;
  let coreFailures = 0;
  let indicatorsError = null;

  if (results[0].status === 'fulfilled') {
    const value = results[0].value;
    if (value && Array.isArray(value.strategies)) {
      strategies = value.strategies;
    } else {
      errors['/api/strategies'] = schemaErr('/api/strategies', 'strategies');
      coreFailures++;
    }
  } else {
    errors['/api/strategies'] = classifyFetchError(results[0].reason, '/api/strategies');
    coreFailures++;
  }

  if (results[1].status === 'fulfilled') {
    const value = results[1].value;
    if (value && Array.isArray(value.layers)) {
      layers = value.layers;
    } else {
      errors['/api/strategies/layers'] = schemaErr('/api/strategies/layers', 'layers');
      coreFailures++;
    }
  } else {
    errors['/api/strategies/layers'] = classifyFetchError(results[1].reason, '/api/strategies/layers');
    coreFailures++;
  }

  if (results[2].status === 'fulfilled') {
    chain = results[2].value;
  } else {
    const err = classifyFetchError(results[2].reason, '/api/dashboard/decision-chain');
    errors['/api/dashboard/decision-chain'] = err;
    if (coreFailures === 0) {
      indicatorsError = err;
    }
  }

  let dataStatus;
  if (coreFailures === 0) {
    dataStatus = strategies.length === 0 ? 'empty' : 'ok';
  } else if (coreFailures >= 2) {
    dataStatus = 'failed';
  } else {
    dataStatus = 'partial';
  }

  return {
    strategies,
    layers,
    coreIndicators: chain && chain.core_indicators ? chain.core_indicators : null,
    errors,
    indicatorsError,
    dataStatus,
  };
}

export function renderPartialBanner(state) {
  const status = state.dataStatus;
  const errorEntries = Object.entries(state.errors || {});

  if ((status === 'ok' || status === 'idle') && state.indicatorsError) {
    const ie = state.indicatorsError;
    return `
      <div class="error-banner error-banner--warning" role="status">
        <div><strong>短線指標無法顯示</strong>：${escapeHtml(ie.message)}</div>
        ${ie.hint ? `<small class="text-muted">${escapeHtml(ie.hint)}</small>` : ''}
      </div>
    `;
  }
  if (status === 'ok' || status === 'idle') return '';

  if (status === 'failed') {
    const items = errorEntries.map(([url, info]) =>
      `<li><code>${escapeHtml(url)}</code>：${escapeHtml(info.message)}${info.hint ? ` — ${escapeHtml(info.hint)}` : ''}</li>`
    ).join('');
    return `
      <div class="error-banner" role="alert">
        <div><strong>載入失敗</strong>：核心資料來源全數失敗</div>
        <ul>${items}</ul>
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
    const items = errorEntries.map(([url, info]) =>
      `<li><code>${escapeHtml(url)}</code>：${escapeHtml(info.message)}${info.hint ? `<br><small class="text-muted">${escapeHtml(info.hint)}</small>` : ''}</li>`
    ).join('');
    return `
      <div class="error-banner error-banner--warning" role="status">
        <div><strong>部分資料載入失敗</strong>：</div>
        <ul>${items}</ul>
        <small class="text-muted">已顯示可取得的資料；失敗區塊以「--」標示。</small>
      </div>
    `;
  }
  return '';
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
      <section class="strategy-ranker mt-md" id="rankerStrip" aria-label="策略排名">
        <div class="strategy-ranker__header">
          <h3>策略排名（依回測表現）</h3>
          <span class="text-muted" id="rankerMeta"></span>
        </div>
        <div class="strategy-ranker__list" id="rankerList"></div>
      </section>
      <div class="filter-tabs mt-md" id="layerTabs"></div>
      <div class="strategy-grid mt-md" id="strategyCards"></div>
    `;
  }

  function render() {
    // Re-render 路徑：先清掉舊 banner，否則 insertAdjacentHTML 會堆積多個
    const oldBanner = slot.querySelector('.error-banner');
    if (oldBanner) oldBanner.remove();
    const banner = renderPartialBanner(STATE);
    if (banner) slot.insertAdjacentHTML('afterbegin', banner);
    renderKPIs();
    renderCoreIndicators();
    renderLayerTabs();
    renderStrategyCards();
    renderStrategyRanker();
    renderModalPlaceholders();
    bindGlobalHandlers();
  }

  function renderStrategyRanker() {
    const meta = document.getElementById('rankerMeta');
    const list = document.getElementById('rankerList');
    if (!meta || !list) return;
    meta.textContent = '載入中…';
    list.innerHTML = '<div class="text-muted">策略排名載入中…</div>';
    fetchJSON('/api/strategy-ranker/rank')
      .then((rows) => {
        if (!Array.isArray(rows) || rows.length === 0) {
          meta.textContent = '';
          list.innerHTML = '<div class="empty">目前沒有活躍策略排名資料</div>';
          return;
        }
        meta.textContent = '共 ' + rows.length + ' 個活躍策略 · 依綜合評分排序';
        list.innerHTML = renderRankerRows(rows);
      })
      .catch((err) => {
        const classified = classifyFetchError(err, '/api/strategy-ranker/rank');
        meta.textContent = '';
        list.innerHTML =
          '<div class="error-banner error-banner--warning" role="status">' +
            '<div><strong>策略排名無法顯示</strong>：' + escapeHtml(classified.message) + '</div>' +
            (classified.hint ? '<small class="text-muted">' + escapeHtml(classified.hint) + '</small>' : '') +
          '</div>';
      });
  }

  function renderRankerRows(rows) {
    return '<table class="ranker-table"><thead><tr>' +
      '<th>#</th><th>策略</th><th>分數</th>' +
      '<th class="text-right">年化報酬</th><th class="text-right">Sharpe</th>' +
      '<th class="text-right">最大回撤</th><th class="text-right">勝率</th>' +
      '<th class="text-right">樣本天數</th>' +
      '</tr></thead><tbody>' +
      rows.map(formatRankerRow).join('') +
      '</tbody></table>';
  }

  function formatRankerRow(r) {
    const rank = typeof r.rank === 'number' ? r.rank : '--';
    const name = r.strategy_name || r.strategy_id || '--';
    const score = typeof r.score === 'number' ? r.score : null;
    const annRet = typeof r.annualized_return === 'number' ? r.annualized_return : null;
    const sharpe = typeof r.sharpe_ratio === 'number' ? r.sharpe_ratio : null;
    const maxDD = typeof r.max_drawdown === 'number' ? r.max_drawdown : null;
    const winRate = typeof r.win_rate === 'number' ? r.win_rate : null;
    const sample = typeof r.sample_days === 'number' ? r.sample_days : null;
    const tierKey = (r.tier || 'free').replace(/[^a-z]/g, '') || 'free';
    const scoreClass = score == null ? '' :
      score >= 80 ? 'ranker-score ranker-score--high' :
      score >= 60 ? 'ranker-score ranker-score--mid' :
      'ranker-score ranker-score--low';
    const retClass = annRet == null ? '' :
      annRet > 0 ? 'text-up' : annRet < 0 ? 'text-down' : '';
    return '<tr>' +
      '<td><span class="ranker-rank">#' + escapeHtml(String(rank)) + '</span></td>' +
      '<td><div class="ranker-name">' + escapeHtml(name) +
        '<span class="tier-badge tier-badge--' + tierKey + '">' + escapeHtml(r.tier || 'free') + '</span>' +
      '</div></td>' +
      '<td><span class="' + scoreClass + '">' + (score == null ? '--' : fmtSafeNumber(score, { decimals: 0 })) + '</span></td>' +
      '<td class="text-right ' + retClass + '">' + (annRet == null ? '--' : fmtSafeSignedPct(annRet, 2)) + '</td>' +
      '<td class="text-right">' + (sharpe == null ? '--' : fmtSafeNumber(sharpe, { decimals: 2 })) + '</td>' +
      // 2026-08-24 UI audit P3：最大回撤以「-X%」呈現（fmtSafeDrawdown），
      // 避免正數幅度被顯示成「+X%」造成語意混淆。
      '<td class="text-right">' + (maxDD == null ? '--' : fmtSafeDrawdown(maxDD)) + '</td>' +
      '<td class="text-right">' + (winRate == null ? '--' : fmtSafePct(winRate, 1)) + '</td>' +
      '<td class="text-right">' + (sample == null ? '--' : sample) + '</td>' +
      '</tr>';
  }

  function renderKPIs() {
    const kpiStrip = document.getElementById('kpiStrip');
    if (!kpiStrip) return;
    const total = STATE.strategies.length;
    const active = STATE.strategies.filter(s => s.status === 'active').length;
    const layersCovered = STATE.layers.filter(l => l.count > 0).length;
    const validHitRates = STATE.strategies
      .map(s => s.hit_rate)
      .filter(v => typeof v === 'number' && Number.isFinite(v));
    const avgHitRate = validHitRates.length > 0
      ? validHitRates.reduce((sum, v) => sum + v, 0) / validHitRates.length
      : null;
    kpiStrip.innerHTML = `
      <div class="kpi-card"><div class="kpi-label">總心法數</div>
        <div class="kpi-value">${total}</div></div>
      <div class="kpi-card"><div class="kpi-label">活躍心法</div>
        <div class="kpi-value">${active}</div></div>
      <div class="kpi-card"><div class="kpi-label">5 層覆蓋</div>
        <div class="kpi-value">${layersCovered}/5</div></div>
      <div class="kpi-card"><div class="kpi-label">平均命中率</div>
        <div class="kpi-value">${avgHitRate === null ? '—' : fmtSafePct(avgHitRate, 1)}</div></div>
    `;
  }

  function renderCoreIndicators() {
    const coreIndicatorStrip = document.getElementById('coreIndicatorStrip');
    if (!coreIndicatorStrip) return;
    const c = STATE.coreIndicators;
    const failed = c === null;
    const isValid = v => typeof v === 'number' && Number.isFinite(v);
    const items = [
      { label: '外資現貨 (TWD 億)', key: 'foreign_capital_net_twd',
        fmt: v => fmtSafeSigned(v, { decimals: 1, suffix: ' 億', forceSign: true }) },
      { label: 'TSM ADR (%)',   key: 'tsm_adr_pct',  fmt: v => fmtSafeSignedPct(v, 2) },
      { label: 'NVDA (%)',      key: 'nvda_pct',     fmt: v => fmtSafeSignedPct(v, 2) },
      { label: 'DXY (%)',       key: 'dxy_pct',      fmt: v => fmtSafeSignedPct(v, 2) },
    ];
    coreIndicatorStrip.innerHTML = items.map(it => {
      const raw = c ? c[it.key] : null;
      const hasValue = isValid(raw);
      const display = failed || !hasValue ? '—' : it.fmt(raw);
      const cls = failed ? 'kpi-value kpi-value--error' :
        `kpi-value ${(hasValue ? (raw > 0 ? 'text-up' : raw < 0 ? 'text-down' : '') : '')}`;
      return `
      <div class="kpi-card">
        <div class="kpi-label">${escapeHtml(it.label)}</div>
        <div class="${cls}">
          ${escapeHtml(display)}
        </div>
      </div>
    `;
    }).join('');
  }

  function renderLayerTabs() {
    const layerTabs = document.getElementById('layerTabs');
    if (!layerTabs) return;
    const tabs = LAYER_FILTERS.map(layer => {
      const count = layer === 'all'
        ? STATE.strategies.length
        : (STATE.layers.find(l => l.layer === layer)?.count || 0);
      const meta = LAYER_META[layer];
      const label = layer === 'all' ? '全部' : (meta ? meta.name : layer);
      const active = STATE.activeLayer === layer ? 'active' : '';
      return `<button class="view-btn ${active}" data-layer="${layer}">${escapeHtml(label)} (${count})</button>`;
    }).join('');
    layerTabs.innerHTML = tabs;
  }

  function renderStrategyCards() {
    const strategyCards = document.getElementById('strategyCards');
    if (!strategyCards) return;
    const filtered = STATE.activeLayer === 'all'
      ? STATE.strategies
      : STATE.strategies.filter(s => s.layer === STATE.activeLayer);
    if (filtered.length === 0) {
      strategyCards.innerHTML =
        '<div class="empty">此層尚無心法，點擊下方「＋ 新增心法」開始建立</div>';
      return;
    }
    strategyCards.innerHTML = filtered.map(renderCard).join('');
  }

  function renderCard(s) {
    const layer = LAYER_META[s.layer] || { name: s.layer, color: 'var(--muted)' };
    const status = STATUS_BADGE[s.status] || { class: '', label: s.status };
    const risk = RISK_BADGE[s.risk] || { class: '', label: s.risk };
    const dir = DIRECTION_GLYPH[s.direction] || { class: '', label: s.direction };
    const cat = CATEGORY_BADGE[s.category] || null;
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
            ${cat ? `<span class="badge ${cat.class}">${escapeHtml(cat.label)}</span>` : ''}
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
              ${fmtSafePct(s.hit_rate, 0)}
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

  const next = processStrategiesResults(results);
  STATE.strategies = next.strategies;
  STATE.layers = next.layers;
  STATE.coreIndicators = next.coreIndicators;
  STATE.errors = next.errors;
  STATE.indicatorsError = next.indicatorsError;
  STATE.dataStatus = next.dataStatus;
}

async function aiAnnotate(id) {
  const modal = document.getElementById('attributionModal');
  const body = document.getElementById('attributionContent');
  modal.style.display = 'flex';
  body.innerHTML = '<div class="empty">🤖 正在呼叫 AI 歸因（最長 30 秒）…</div>';

  let staticData = null;
  let staticError = null;
  try {
    staticData = await fetchJSON(`/api/strategies/${encodeURIComponent(id)}/attribution`);
  } catch (e) {
    staticError = classifyFetchError(e, `/api/strategies/${id}/attribution`);
  }

  try {
    const r = await fetchJSON(`/api/strategies/${encodeURIComponent(id)}/annotate`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({}),
    });
    renderAIAttribution(id, r, staticData, staticError);
  } catch (e) {
    const classified = classifyFetchError(e, `/api/strategies/${id}/annotate`);
    if (staticData) {
      renderAIAttribution(id, {
        annotation: '',
        mode: 'rule_based',
        note: 'AI 不可用（' + classified.message + '），顯示靜態歸因',
      }, staticData, staticError);
    } else {
      body.innerHTML = `<div class="empty error">AI 歸因失敗：${escapeHtml(classified.message)}</div>`;
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

let _render = null;

if (typeof window !== 'undefined') {
  window._strategiesSetLayer = layer => {
    STATE.activeLayer = layer;
    if (_render) _render();
  };
  window._strategiesRefresh  = async () => {
    await loadStrategiesData();
    if (_render) _render();
  };
}
