# admin_web 孤兒頁面清理、半導體景氣指標與 orphan API 啟用 — 實作計畫

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 清理 admin_web 孤兒頁面，於既有頁面新增半導體景氣指標視覺化，並啟用 drawdown、forecast-vs-reality、backtest signals 等 orphan API。

**Architecture:** 不改 backend、不開新頁面；所有視覺化與 API 啟用都嵌入現有 `live`、`reports`、`experiments` 頁面與共用 `shared_web/static/js/pages/*` module；admin_web 僅保留 10 個必要頁面。

**Tech Stack:** Vanilla ES modules (admin_web/client_web/shared_web), esbuild, Playwright smoke tests.

## Global Constraints

- 所有 AI 回覆使用繁體中文。
- 不改 backend Go 與 client_web。
- 不新增孤兒頁面；不開新 CSS/JS 檔案，功能嵌入既有 module。
- 共用 `shared_web/static/js/pages/*` module 不刪除（client_web 仍使用）。
- JS 色彩使用 `shared_web/static/js/shared/color-tokens.js` 的 `financialColor`/`regimeColor`；inline style 使用 `var(--...)`，不寫死 hex。
- 單一 API 失敗不阻塞整頁；缺失資料使用 `renderEmptyState`。
- 每個 task 結束後 commit。

---

## File Map

| 檔案 | 責任 |
|------|------|
| `admin_web/static/index.html` | admin_web DOM；刪除孤兒 page div。 |
| `admin_web/static/js/main.js` | admin_web 路由、模組載入、API 排程；精簡 titles/loadModules/loadPageData/loadAll。 |
| `admin_web/static/js/event-listeners.js` | admin_web 事件繫結；移除被刪頁面監聽器。 |
| `shared_web/static/js/pages/dashboard.js` | 首頁渲染與 macro radar；清理死連結。 |
| `shared_web/static/js/pages/risk.js` | `live` 頁面風控渲染；新增半導體景氣 panel 與 drawdown 卡片。 |
| `shared_web/static/js/pages/backtest.js` | `reports` 頁面回測渲染；新增 backtest signals 與 forecast-vs-reality 詳細表格。 |
| `shared_web/static/js/pages/experiments.js` | `experiments` 頁面；新增 forecast-vs-reality 彙總卡片。 |

---

## Task 1: 清理 `admin_web/static/index.html`

**Files:**
- Modify: `admin_web/static/index.html`

**Interfaces:**
- Consumes: 設計規格中的刪除清單。
- Produces: 只剩 10 個 page div 的 DOM。

- [x] **Step 1: 刪除孤兒 page div**

移除以下區塊（保留註解可一併清理）：

```html
<!-- 宏觀敘事 -->
<div class="page" id="page-narrative">...</div>

<!-- 投資管線 / 決策鏈 / 決策追蹤 註解 -->

<!-- 組合持倉 / AI 觀測台 -->
<div class="page" id="page-agents">...</div>

<!-- 控制與稽核 -->
<div class="page" id="page-controls">...</div>

<!-- 信息通道 已存在，但 synergy/prism/industry/reasoning-trace 要刪 -->
<div class="page" id="page-industry">...</div>
<div class="page" id="page-synergy">...</div>
<div class="page" id="page-prism">...</div>
<div class="page" id="page-reasoning-trace">...</div>
```

保留：`page-home`、`page-live`、`page-alerts`、`page-evolution_panel`、`page-experiments`、`page-datachannels`、`page-parameters`、`page-reports`、`page-metrics`、`page-config`。

- [x] **Step 2: 驗證 HTML 結構**

Run: `cd admin_web && npm run build`
Expected: build success, no esbuild errors.

- [x] **Step 3: Commit**

```bash
git add admin_web/static/index.html
git commit -m "feat(admin_web): 移除孤兒頁面 DOM"
```

---

## Task 2: 精簡 `admin_web/static/js/main.js`

**Files:**
- Modify: `admin_web/static/js/main.js`

**Interfaces:**
- Consumes: Task 1 保留的頁面清單。
- Produces: `titles`、 `loadModules`、 `loadAll`、 `loadPageData` 只包含必要頁面。

- [x] **Step 1: 更新 `titles`**

只保留：

```js
const titles = {
  home: '系統總覽', live: '風控營運台', alerts: '系統警報',
  evolution_panel: '策略演化', experiments: '模擬交易',
  datachannels: '資料通道', parameters: '參數管理',
  reports: '最新回測', metrics: '指標監控', config: '部署配置'
};
```

- [x] **Step 2: 更新 `loadModules()` imports**

移除 `narrative`、`industry`、`synergy`、`prism` 的 import；保留 `narrative`（供 live narrative strip）。

保留 import 清單：

```js
import('./pages/dashboard.js'),
import('./pages/risk.js'),
import('./pages/narrative.js'),
import('./pages/backtest.js'),
import('./pages/inbox.js'),
import('./pages/experiments.js'),
import('./pages/alerts.js'),
import('./pages/metrics.js'),
import('./pages/datachannels.js'),
import('./pages/parameters.js'),
import('./pages/deploy-config.js'),
import('./pages/evolution_panel.js'),
```

對應 `keys`：

```js
var keys = ['dash', 'risk', 'narr', 'back', 'inbox', 'experiments', 'alerts', 'metrics', 'datachannels', 'parameters', 'deployConfig', 'evolution_panel'];
```

- [x] **Step 3: 更新 `loadAll()`**

移除對 `m.narr.renderNarrativePage` 的呼叫（narrative 頁面已刪）。
保留 `m.narr.renderLiveNarrativeStrip`（live 頁面仍需）。
移除對 `m.industry.loadIndustryData`、 `m.synergy.renderSynergyPage`、 `m.prism.loadPrismData`、 `m.back.renderBacktestReport`（改為 reports 頁面 lazy 載入）等被刪頁面相關的 render。

- [x] **Step 4: 更新 `loadPageData()`**

移除 `narrative`、`industry`、`controls`、`synergy`、`prism`、`reasoning-trace`、`agents` 分支。
保留 `live`、`experiments`、`reports`、`datachannels`、`alerts`、`metrics`、`evolution_panel`、`portfolio`、`parameters`、`config`、`strategies`、`crossmarket`、`prism`（若原本有）中的必要項目。

- [x] **Step 5: Build 驗證**

Run: `cd admin_web && npm run build`
Expected: success.

- [x] **Step 6: Commit**

```bash
git add admin_web/static/js/main.js
git commit -m "feat(admin_web): 精簡路由與模組載入，只保留必要頁面"
```

---

## Task 3: 清理 `admin_web/static/js/event-listeners.js`

**Files:**
- Modify: `admin_web/static/js/event-listeners.js`

**Interfaces:**
- Consumes: Task 1–2 刪除的頁面。
- Produces: 事件監聽器只對應現有頁面。

- [x] **Step 1: 移除被刪頁面監聽器**

刪除與 `#page-industry`、 `#page-controls`、 `#page-synergy`、 `#page-prism`、 `#page-reasoning-trace`、 `#page-agents` 相關的 querySelector 區塊。
保留 `#page-evolution_panel`、 `#page-datachannels`、 `#page-alerts`、 `#page-reports`、 modals 等現有監聽器。

- [x] **Step 2: Build 驗證**

Run: `cd admin_web && npm run build`
Expected: success.

- [x] **Step 3: Commit**

```bash
git add admin_web/static/js/event-listeners.js
git commit -m "feat(admin_web): 移除孤兒頁面事件監聽器"
```

---

## Task 4: 清理共用 dashboard.js 中的死連結

**Files:**
- Modify: `shared_web/static/js/pages/dashboard.js`

**Interfaces:**
- Consumes: Task 1–3 刪除的頁面清單。
- Produces: `renderOverview` 與 `renderMacroRadar` 不再連向不存在頁面。

- [x] **Step 1: 更新 `renderOverview` 連結**

- 「敘事脈絡」卡片：移除「開啟宏觀敘事 →」連結。
- 「資金階段」卡片：`switchPage('controls')` → `switchPage('parameters')`。
- 「產業週期數據」卡片：移除 `switchPage('synergy')` 連結，改為純文字提示。

- [x] **Step 2: 更新 `renderMacroRadar` 連結**

移除「前往【投資管線】」連結（admin_web 無 pipeline 頁面），改為純文字：

```js
`尚有 ${passedItems.length - 5} 檔標的，詳細清單請切換至投資人平臺。`
```

- [x] **Step 3: 更新 `sessionSyncAlert`**

`switchPage('reasoning-trace')` → `switchPage('home')` 或移除連結。

- [x] **Step 4: Build 驗證**

Run: `cd admin_web && npm run build && cd ../client_web && npm run build`
Expected: both success.

- [x] **Step 5: Commit**

```bash
git add shared_web/static/js/pages/dashboard.js
git commit -m "fix(admin_web): 清理 dashboard 中被刪頁面的死連結"
```

---

## Task 5: 於 `live` 頁面新增半導體景氣指標 panel

**Files:**
- Modify: `shared_web/static/js/pages/risk.js`
- Modify: `admin_web/static/js/main.js`

**Interfaces:**
- Consumes: `/api/macro/snapshot/latest` (`sox_index`) 與 `/api/dashboard/industry-cycle?industry=semiconductor`。
- Produces: `renderSemiconductorSentiment(snapshot, industryCycle)` 渲染至 `#semiconductorSentimentPanel`。

- [x] **Step 1: 在 `admin_web/static/index.html` 的 `page-live` 新增容器**

在 `live` 頁面適當位置（例如 `macroRadar` panel 之後）新增：

```html
<div class="panel wide" id="semiconductorSentimentPanel" style="display:none;">
  <h2>📊 市場廣度 / 半導體景氣</h2>
  <div id="semiconductorSentimentContent" class="empty loading">載入中…</div>
</div>
```

- [x] **Step 2: 於 `shared_web/static/js/pages/risk.js` 新增渲染函數**

```js
import { financialColor } from '../shared/color-tokens.js';

export function renderSemiconductorSentiment(snapshot, industryCycle) {
  const panel = document.getElementById('semiconductorSentimentPanel');
  const el = document.getElementById('semiconductorSentimentContent');
  if (!panel || !el) return;
  panel.style.display = '';
  el.classList.remove('loading');

  const sox = snapshot && snapshot.sox_index ? snapshot.sox_index : null;
  const cycle = industryCycle || {};

  const soxValue = sox && typeof sox.value === 'number' ? sox.value.toFixed(2) : '-';
  const soxChange = sox && typeof sox.change_pct === 'number' ? sox.change_pct : null;
  const soxChangeStr = soxChange !== null ? (soxChange > 0 ? '+' : '') + soxChange.toFixed(2) + '%' : '-';
  const soxColor = financialColor(soxChange, 'trend');

  const cycleMap = {
    expansion: { label: '擴張', color: 'var(--color-success)' },
    recovery: { label: '復甦', color: 'var(--color-info)' },
    maturity: { label: '成熟', color: 'var(--warn)' },
    recession: { label: '衰退', color: 'var(--color-danger)' }
  };
  const business = cycleMap[cycle.business_cycle] || { label: cycle.business_cycle || '-', color: 'var(--muted)' };

  el.innerHTML = `
    <div class="kpi-grid" style="grid-template-columns:repeat(auto-fit,minmax(160px,1fr));gap:12px">
      <div class="panel" style="text-align:center">
        <div class="kpi-label">費城半導體指數 (SOX)</div>
        <div class="kpi-value" style="color:${soxColor}">${soxValue}</div>
        <div class="kpi-hint">${soxChangeStr}</div>
      </div>
      <div class="panel" style="text-align:center">
        <div class="kpi-label">半導體週期燈號</div>
        <div class="kpi-value" style="color:${business.color};font-size:20px">${business.label}</div>
        <div class="kpi-hint">confidence ${typeof cycle.confidence === 'number' ? (cycle.confidence * 100).toFixed(0) + '%' : '-'}</div>
      </div>
    </div>
    <table style="margin-top:12px;font-size:12px;width:100%">
      <thead><tr><th>庫存週期</th><th>資本支出週期</th><th>趨勢</th><th>更新時間</th></tr></thead>
      <tbody>
        <tr>
          <td>${cycle.inventory_cycle || '-'}</td>
          <td>${cycle.capex_cycle || '-'}</td>
          <td>${cycle.trend || '-'}</td>
          <td>${cycle.updated_at ? new Date(cycle.updated_at).toLocaleString('zh-TW') : '-'}</td>
        </tr>
      </tbody>
    </table>
  `;
}
```

- [x] **Step 3: 於 `admin_web/static/js/main.js` 的 `live` 載入分支加入 API 呼叫**

```js
else if (pageId === 'live') {
  try {
    var liveResults = await Promise.all([
      getJSONWithTimeout('/api/dashboard/live-status'),
      getJSONWithTimeout('/api/dashboard/recommendation-pipeline'),
      getJSONWithTimeout('/api/dashboard/risk-exposure'),
      getJSONWithTimeout('/api/dashboard/macro-radar'),
      getJSONWithTimeout('/api/narrative/events'),
      getJSONWithTimeout('/api/taiwan/stress-index'),
      getJSONWithTimeout('/api/narrative/chains'),
      getJSONWithTimeout('/api/narrative/models'),
      getJSONWithTimeout('/api/dashboard/capital-phase'),
      getJSONWithTimeout('/api/dashboard/risk-calibration'),
      getJSONWithTimeout('/api/macro/snapshot/latest'),
      getJSONWithTimeout('/api/dashboard/industry-cycle?industry=semiconductor'),
    ]);
    if (m.risk.renderLiveStatus) m.risk.renderLiveStatus(liveResults[0]);
    if (m.risk.renderRiskCards) m.risk.renderRiskCards(liveResults[2], liveResults[1], liveResults[8]);
    if (m.risk.renderRiskCalibration) m.risk.renderRiskCalibration(liveResults[9]);
    if (m.risk.renderRiskCommentary) m.risk.renderRiskCommentary();
    if (m.dash.renderMacroRadar) m.dash.renderMacroRadar(liveResults[3], liveResults[1]);
    if (m.narr.renderLiveNarrativeStrip) m.narr.renderLiveNarrativeStrip(liveResults[4], liveResults[5], liveResults[7], liveResults[6]);
    if (m.risk.renderSemiconductorSentiment) m.risk.renderSemiconductorSentiment(liveResults[10], liveResults[11]);
  } catch(e) { console.error(e); }
}
```

- [x] **Step 4: Build 驗證**

Run: `cd admin_web && npm run build`
Expected: success.

- [x] **Step 5: Commit**

```bash
git add admin_web/static/index.html admin_web/static/js/main.js shared_web/static/js/pages/risk.js
git commit -m "feat(live): 新增半導體景氣指標 panel"
```

---

## Task 6: 於 `live` 頁面啟用 `/api/dashboard/drawdown`

**Files:**
- Modify: `shared_web/static/js/pages/risk.js`
- Modify: `admin_web/static/js/main.js`

**Interfaces:**
- Consumes: `/api/dashboard/drawdown`。
- Produces: `renderDrawdownPanel(data)` 渲染至 `#drawdownPanel`。

- [x] **Step 1: 在 `page-live` 新增容器**

於 `riskCardsPanel` 後新增：

```html
<div class="panel wide" id="drawdownPanel" style="display:none;">
  <h2>📉 回撤模擬</h2>
  <div id="drawdownContent" class="empty loading">載入中…</div>
</div>
```

- [x] **Step 2: 於 `risk.js` 新增渲染函數**

```js
export function renderDrawdownPanel(data) {
  const panel = document.getElementById('drawdownPanel');
  const el = document.getElementById('drawdownContent');
  if (!panel || !el) return;
  panel.style.display = '';
  el.classList.remove('loading');

  if (!data || data.status === 'not_available') {
    el.innerHTML = renderEmptyState('尚無回撤模擬資料', '系統執行回測後將自動產生');
    return;
  }

  const fmtPct = v => (typeof v === 'number' && !isNaN(v)) ? (v * 100).toFixed(1) + '%' : '—';
  el.innerHTML = `
    <div class="kpi-grid" style="grid-template-columns:repeat(auto-fit,minmax(160px,1fr));gap:12px">
      <div class="panel" style="text-align:center">
        <div class="kpi-label">最大回撤</div>
        <div class="kpi-value" style="color:var(--color-danger)">${fmtPct(data.max_drawdown)}</div>
      </div>
      <div class="panel" style="text-align:center">
        <div class="kpi-label">VaR 95%</div>
        <div class="kpi-value" style="color:var(--color-danger)">${fmtPct(data.var_95)}</div>
      </div>
    </div>
    ${Array.isArray(data.worst_path) && data.worst_path.length ? `
    <div style="margin-top:12px;font-size:12px">
      <strong>最差路徑（前 5 筆）：</strong>
      ${data.worst_path.slice(0,5).map(p => `<span class="badge err">${(p*100).toFixed(1)}%</span>`).join(' ')}
    </div>` : ''}
  `;
}
```

- [x] **Step 3: 於 `admin_web/static/js/main.js` 加入 drawdown API**

在 `live` 頁面的 Promise.all 中加入：

```js
getJSONWithTimeout('/api/dashboard/drawdown'),
```

並傳給 `renderDrawdownPanel`。

- [x] **Step 4: Build 驗證與 Commit**

Run: `cd admin_web && npm run build`
Expected: success.

```bash
git add admin_web/static/index.html admin_web/static/js/main.js shared_web/static/js/pages/risk.js
git commit -m "feat(live): 啟用 /api/dashboard/drawdown 視覺化"
```

---

## Task 7: 於 `reports` 頁面啟用 `/api/backtest/signals`

**Files:**
- Modify: `shared_web/static/js/pages/backtest.js`
- Modify: `admin_web/static/js/main.js`

**Interfaces:**
- Consumes: `/api/backtest/signals`。
- Produces: `renderBacktestSignals(data)` 渲染至 `#backtestSignals`。

- [x] **Step 1: 在 `page-reports` 新增容器**

於 `backtestReport` panel 之後新增：

```html
<div class="panel wide">
  <h2>📡 回測信號</h2>
  <div id="backtestSignals" class="empty loading">載入中…</div>
</div>
```

- [x] **Step 2: 於 `backtest.js` 新增渲染函數**

```js
export function renderBacktestSignals(data) {
  const el = document.getElementById('backtestSignals');
  if (!el) return;
  el.classList.remove('loading');

  if (!data || !data.active_signals) {
    el.innerHTML = renderEmptyState('尚無回測信號', '執行回測後將自動產生');
    return;
  }

  const fmtPct = v => (typeof v === 'number' && !isNaN(v)) ? (v * 100).toFixed(1) + '%' : '—';
  const signals = data.active_signals || [];

  el.innerHTML = `
    <div class="kpi-grid" style="grid-template-columns:repeat(auto-fit,minmax(140px,1fr));gap:12px;margin-bottom:12px">
      <div class="panel" style="text-align:center"><div class="kpi-label">活躍信號</div><div class="kpi-value">${signals.length}</div></div>
      <div class="panel" style="text-align:center"><div class="kpi-label">VaR 95%</div><div class="kpi-value" style="color:var(--color-danger)">${fmtPct(data.var_95)}</div></div>
      <div class="panel" style="text-align:center"><div class="kpi-label">VaR 99%</div><div class="kpi-value" style="color:var(--color-danger)">${fmtPct(data.var_99)}</div></div>
      <div class="panel" style="text-align:center"><div class="kpi-label">Sharpe (short)</div><div class="kpi-value">${data.sharpe_short != null ? data.sharpe_short.toFixed(2) : '—'}</div></div>
      <div class="panel" style="text-align:center"><div class="kpi-label">Sharpe (long)</div><div class="kpi-value">${data.sharpe_long != null ? data.sharpe_long.toFixed(2) : '—'}</div></div>
      <div class="panel" style="text-align:center"><div class="kpi-label">回撤</div><div class="kpi-value" style="color:var(--color-danger)">${fmtPct(data.drawdown_pct)}</div></div>
    </div>
    ${signals.length ? `<div style="font-size:12px"><strong>活躍信號：</strong>${signals.map(s => `<span class="badge info">${escapeHtml(s)}</span>`).join(' ')}</div>` : ''}
  `;
}
```

- [x] **Step 3: 於 `admin_web/static/js/main.js` 的 `reports` 分支載入 signals**

```js
else if (pageId === 'reports') {
  try {
    var sig = await getJSONWithTimeout('/api/backtest/signals');
    if (m.back.renderBacktestSignals) m.back.renderBacktestSignals(sig);
    if (m.back.renderBacktestReport) m.back.renderBacktestReport();
  } catch(e) { console.error(e); }
}
```

- [x] **Step 4: Build 驗證與 Commit**

Run: `cd admin_web && npm run build`
Expected: success.

```bash
git add admin_web/static/index.html admin_web/static/js/main.js shared_web/static/js/pages/backtest.js
git commit -m "feat(reports): 啟用 /api/backtest/signals 視覺化"
```

---

## Task 8: 於 `experiments` 與 `reports` 啟用 `/api/dashboard/forecast-vs-reality`

**Files:**
- Modify: `shared_web/static/js/pages/experiments.js`
- Modify: `shared_web/static/js/pages/backtest.js`
- Modify: `admin_web/static/js/main.js`

**Interfaces:**
- Consumes: `/api/dashboard/forecast-vs-reality`。
- Produces: `renderForecastVsRealitySummary(data)`（experiments）、`renderForecastVsRealityTable(data)`（reports）。

- [x] **Step 1: 於 `experiments.js` 新增 summary 函數**

```js
export function renderForecastVsRealitySummary(data) {
  const container = document.getElementById('experimentForecastSummary');
  if (!container) return;
  container.classList.remove('loading');

  const preds = data && Array.isArray(data.symbol_predictions) ? data.symbol_predictions : [];
  const hits = preds.filter(p => p.hit).length;
  const hitRate = preds.length ? (hits / preds.length * 100).toFixed(1) + '%' : '—';

  container.innerHTML = `
    <div class="panel" style="text-align:center">
      <div class="kpi-label">預測命中追蹤</div>
      <div class="kpi-value">${hitRate}</div>
      <div class="kpi-hint">${hits} / ${preds.length} 筆命中</div>
    </div>
  `;
}
```

- [x] **Step 2: 於 `backtest.js` 新增 table 函數**

```js
export function renderForecastVsRealityTable(data) {
  const el = document.getElementById('forecastVsReality');
  if (!el) return;
  el.classList.remove('loading');

  const preds = data && Array.isArray(data.symbol_predictions) ? data.symbol_predictions : [];
  if (!preds.length) {
    el.innerHTML = renderEmptyState('尚無預測命中資料', '執行回測後將自動產生');
    return;
  }

  el.innerHTML = `
    <table style="font-size:12px;width:100%">
      <thead><tr><th>標的</th><th>Agent</th><th>方向</th><th>信念</th><th>遠期報酬</th><th>命中</th><th>時間</th></tr></thead>
      <tbody>
        ${preds.slice(0,20).map(p => {
          const retCls = p.forward_return > 0 ? 'up' : (p.forward_return < 0 ? 'down' : '');
          return `<tr>
            <td>${escapeHtml(p.symbol)}</td>
            <td>${escapeHtml(p.agent_id)}</td>
            <td>${escapeHtml(p.side)}</td>
            <td>${p.conviction != null ? p.conviction : '-'}</td>
            <td class="${retCls}">${p.forward_return != null ? (p.forward_return * 100).toFixed(1) + '%' : '-'}</td>
            <td>${p.hit ? '✅' : '❌'}</td>
            <td>${p.recorded_at ? new Date(p.recorded_at).toLocaleString('zh-TW') : '-'}</td>
          </tr>`;
        }).join('')}
      </tbody>
    </table>
    ${preds.length > 20 ? `<div style="margin-top:8px;font-size:12px;color:var(--muted)">尚有 ${preds.length - 20} 筆...</div>` : ''}
  `;
}
```

- [x] **Step 3: 更新 DOM 容器**

`admin_web/static/index.html`：
- `page-experiments` 的 inbox/workflow 區塊後新增：
  ```html
  <div class="panel wide"><h2>🔮 預測命中追蹤</h2><div id="experimentForecastSummary" class="empty loading">載入中…</div></div>
  ```
- `page-reports` 的 backtestSignals 區塊後新增：
  ```html
  <div class="panel wide"><h2>🔮 預測 vs 實際</h2><div id="forecastVsReality" class="empty loading">載入中…</div></div>
  ```

- [x] **Step 4: 更新 `admin_web/static/js/main.js`**

`experiments` 分支：

```js
else if (pageId === 'experiments') {
  try {
    var [inbox, fvr] = await Promise.all([
      silentGetJSON('/api/dashboard/experiment-inbox'),
      getJSONWithTimeout('/api/dashboard/forecast-vs-reality')
    ]);
    if (m.inbox.renderInbox) m.inbox.renderInbox(inbox);
    if (m.experiments.renderForecastVsRealitySummary) m.experiments.renderForecastVsRealitySummary(fvr);
    if (m.experiments.loadAuditLog) m.experiments.loadAuditLog();
    if (m.experiments.loadExperimentHistory) m.experiments.loadExperimentHistory();
  } catch(e) { console.error(e); }
}
```

`reports` 分支：

```js
else if (pageId === 'reports') {
  try {
    var [sig, fvr] = await Promise.all([
      getJSONWithTimeout('/api/backtest/signals'),
      getJSONWithTimeout('/api/dashboard/forecast-vs-reality')
    ]);
    if (m.back.renderBacktestSignals) m.back.renderBacktestSignals(sig);
    if (m.back.renderForecastVsRealityTable) m.back.renderForecastVsRealityTable(fvr);
    if (m.back.renderBacktestReport) m.back.renderBacktestReport();
  } catch(e) { console.error(e); }
}
```

- [x] **Step 5: Build 驗證與 Commit**

Run: `cd admin_web && npm run build`
Expected: success.

```bash
git add admin_web/static/index.html admin_web/static/js/main.js shared_web/static/js/pages/experiments.js shared_web/static/js/pages/backtest.js
git commit -m "feat(experiments/reports): 啟用 /api/dashboard/forecast-vs-reality"
```

---

## Task 9: 全端 Build 與 Smoke Test

**Files:**
- All modified files.

**Interfaces:**
- Consumes: Tasks 1–8 的修改。
- Produces: 可工作的 admin_web 與未被破壞的 client_web。

- [x] **Step 1: Build admin_web 與 client_web**

```bash
cd admin_web && npm run build
cd ../client_web && npm run build
```

Expected: both exit 0.

- [x] **Step 2: admin_web smoke test**

```bash
cd admin_web && npm run test:smoke
```

Expected: pass, no 404/500 on `live`、`reports`、`experiments`。

- [x] **Step 3: 手動檢查清單**

- [x] 側邊欄只有 7 個入口。
- [x] `live` 頁面出現「市場廣度 / 半導體景氣」panel 與「回撤模擬」panel。
- [x] `reports` 頁面出現「回測信號」與「預測 vs 實際」區塊。
- [x] `experiments` 頁面出現「預測命中追蹤」卡片。
- [x] `home` 頁面點擊「資金階段」卡片可正常切換到 `parameters`。
- [x] 被刪除頁面 URL（如 `/admin/narrative`）會 fallback 到 `home` 或 404。

- [x] **Step 4: Commit 測試結果/調整**

```bash
git add -A
git commit -m "test(admin_web): smoke test 與 build 驗證"
```

---

## Task 10: 開新 PR

**Files:** N/A

- [x] **Step 1: Push 分支**

```bash
git push -u origin feat/admin-web-cleanup-semiconductor-orphan-apis
```

- [x] **Step 2: 開 PR**

Title: `feat(admin_web): 孤兒頁面清理、半導體景氣指標視覺化與 orphan API 啟用`

Body 需包含：
- 關聯 PR：#976–#982
- 修改摘要：清理 7 個孤兒頁面、新增半導體景氣 panel、啟用 3 個 orphan API
- 測試方式：`npm run build`、`npm run test:smoke`
- 文件：`.omo/plans/2026-07-07-admin-web-cleanup-semiconductor-orphan-apis-implementation.md`

- [x] **Step 3: 回報使用者**

PR URL: `https://github.com/kaecer68/atlas-go/pull/<number>`

---

## Self-Review

1. **Spec coverage**: 設計規格中的清理清單、半導體指標、orphan API 啟用、build/smoke 驗證都已對應到 task。
2. **Placeholder scan**: 無 TBD/TODO；所有步驟含具體程式碼與命令。
3. **Type consistency**: `renderSemiconductorSentiment(snapshot, industryCycle)`、`renderDrawdownPanel(data)`、`renderBacktestSignals(data)`、`renderForecastVsRealitySummary(data)`、`renderForecastVsRealityTable(data)` 命名與參數前後一致。
4. **Risk**: GitNexus 顯示三個 orphan API handler 的 upstream impact 皆為 0，risk LOW；共用 dashboard.js/risk.js/backtest.js 的簽名未變更，不影響 client_web。

---

## 實作備註（2026-07-08）

與原始計畫的差異與驗證結果：

1. **narrative 模組保留方式**：設計規格 2.2 將 narrative 列為刪除頁面，但 2.3 又提到保留 narrative 分支。實作採用折衷：從 `titles`、`loadPageData` 與 sidebar 中移除 narrative 獨立頁面，但保留 `import('./pages/narrative.js')` 與 `m.narr.renderLiveNarrativeStrip`，供 `live` 頁面的總經敘事脈絡 strip 使用。

2. **DOM 容器 ID**：實際使用 `#semiconductorSentiment`、`#drawdownPanel`、`#backtestSignals`、`#forecastVsRealityTable`、`#forecastVsRealitySummary`，與計畫草稿中的部分命名不同。

3. **shared_web 防禦性修復**：
   - `experiments.js` 的 `loadExperimentHistory` 與 `inbox.js` 的 `renderInbox` 在操作 `#revertSelect` / `#promoteSelect` 前增加 `if (el)` 檢查，因為 admin_web 的 DOM 中沒有這些元素（原本屬於 controls 頁面），否則會在 smoke test 觸發 `TypeError: Cannot set properties of null`。
   - `experiments.js` 補上 `renderEmptyState` import。

4. **Smoke test 已知問題**：`admin_web/smoke/known-issues.json` 新增 `SimHealthPanel: failed to fetch traces: Error: HTTP 401`，因為 smoke runner 未登入，traces API 回傳 401 是預期行為。

5. **發現的 backend 問題**：`/api/dashboard/forecast-vs-reality` 在當前環境回傳 HTTP 500，錯誤為 `json: cannot unmarshal string into Go struct field PromptExperimentResult.notes of type []string`。frontend 已使用 `silentGetJSON` 與 `renderEmptyState` 優雅處理失敗，不阻塞頁面；此問題需 backend 資料修復，不在本次 frontend 範圍。

6. **驗證結果**：
   - `cd admin_web && npm run build` ✅
   - `cd client_web && npm run build` ✅
   - `SMOKE_PAGES="home,live,reports,experiments,parameters,alerts,datachannels,evolution_panel" npm run test:smoke` ✅（0 unknown console errors）
