# admin_web 孤兒頁面清理、半導體景氣指標視覺化與 orphan API 啟用

> 對應 PR 脈絡：#976–#982（Wave 11 Phase 1–7 已 merge）  
> 設計日期：2026-07-07  
> 分支：`feat/admin-web-cleanup-semiconductor-orphan-apis`

## 背景與目標

Wave 11 將 admin_web 側邊欄精簡為 7 個核心頁面，但 DOM 與路由表中仍殘留大量孤兒頁面（narrative、industry、controls、synergy、prism、reasoning-trace、agents）。
本次工作要：

1. 刪除 admin_web 中無側邊欄入口、且非本次必要功能的孤兒頁面。
2. 在現有頁面中新增半導體景氣指標視覺化。
3. 啟用已存在但前端未呼叫的 orphan API。

設計原則：**不開新頁面、不新增 backend endpoint、不影響 client_web**。

---

## 1. 分支與範圍

- **新分支**：`feat/admin-web-cleanup-semiconductor-orphan-apis`（從 `origin/main` 切出）
- **修改範圍**：
  - `admin_web/static/index.html`
  - `admin_web/static/js/main.js`
  - `admin_web/static/js/event-listeners.js`
  - `shared_web/static/js/pages/risk.js`
  - `shared_web/static/js/pages/backtest.js`
  - `shared_web/static/js/pages/experiments.js`
- **不改**：client_web、backend Go、shared_web 共用的 page module 檔案本身（只調整 admin_web 是否 import/呼叫）。
- **櫃買/OTC**：本次暫不實作，因後端 `OTCIndexProvider` 尚未註冊到 macro snapshot，留待後續有資料管道時再補。

---

## 2. admin_web 孤兒頁面清理

### 2.1 保留頁面（10 個）

側邊欄現有 7 個：

| 頁面 | 用途 |
|------|------|
| `home` | 系統總覽 |
| `live` | 風控營運台 |
| `alerts` | 系統警報 |
| `evolution_panel` | 策略演化 |
| `experiments` | 模擬交易 |
| `datachannels` | 資料通道 |
| `parameters` | 參數管理 |

額外保留 3 個（因要啟用 orphan API 而需有頁面承載）：

| 頁面 | 原因 |
|------|------|
| `reports` | 承載 `/api/backtest/signals`、`/api/backtest/snapshots`、`/api/dashboard/forecast-vs-reality` 詳細表格 |
| `metrics` | 系統指標監控，有完整 module |
| `config` | 部署配置唯讀檢視，有完整 module |

### 2.2 刪除頁面（7 個）

從 `admin_web/static/index.html`、`admin_web/static/js/main.js`、`admin_web/static/js/event-listeners.js` 移除：

- `narrative`
- `industry`
- `controls`
- `synergy`
- `prism`
- `reasoning-trace`
- `agents`

### 2.3 具體修改

#### `admin_web/static/index.html`

- 移除上述 7 個 `<div class="page" id="page-xxx">` 區塊。
- 側邊欄 `<nav>` 維持不變（只保留現有 7 個入口）。

#### `admin_web/static/js/main.js`

- `titles` 物件：只保留 10 個頁面的標題對應。
- `loadModules()`：
  - 保留 import：`dashboard`、`risk`、`backtest`、`inbox`、`experiments`、`alerts`、`metrics`、`datachannels`、`parameters`、`deployConfig`、`evolution_panel`、`narrative`（`live` 頁面的 narrative strip 仍需要）。
  - 移除對 `industry`、`synergy`、`prism` 的 import（若原本有）。
- `loadAll()`：
  - 移除被刪頁面相關的 API 呼叫（如 `/api/synergy/...` 等）。
  - 保留 `/api/narrative/events`、`/api/narrative/chains`、`/api/narrative/models` 等供 `live` 頁面的 narrative strip 使用。
  - 移除對應被刪頁面的 render 呼叫。
- `loadPageData()`：
  - 移除 `industry`、`controls`、`synergy`、`prism`、`reasoning-trace`、`agents` 的分支。
  - 保留 `narrative` 分支（因 `live` 頁面仍會用到 narrative strip，但不再作為獨立頁面）。
- **清理死連結**：
  - `home` 頁面「資金階段」卡片原本 `switchPage('controls')`，改為連到 `parameters` 或移除連結。
  - `home` 頁面「產業週期數據」卡片原本 `switchPage('synergy')`，改為純文字提示或移除連結。
  - `home` 頁面「敘事脈絡」卡片原本有「開啟宏觀敘事 →」連到 `narrative`，移除該連結。
  - `sessionSyncAlert` 原本連到 `reasoning-trace`，改為連到 `home` 或移除。
  - `shared_web/static/js/pages/dashboard.js` 中 `renderMacroRadar` 的「前往【投資管線】」連到 `pipeline`，因 admin_web 無 `pipeline` 頁面，改為純文字或移除連結。

#### `admin_web/static/js/event-listeners.js`

- 移除 `#page-industry`、 `#page-synergy`、 `#page-controls`、 `#page-prism`、 `#page-reasoning-trace`、 `#page-agents` 等被刪頁面相關的 event listener。
- 保留 `#page-evolution_panel` 的 view switch（因該頁面仍保留）。

---

## 3. 半導體景氣指標視覺化（整合進 `live`）

在 `live` 頁面「總經雷達」旁新增「市場廣度 / 半導體景氣」panel。

### 3.1 資料來源

| 指標 | API | 欄位 |
|------|-----|------|
| SOX 費城半導體指數 | `GET /api/macro/snapshot/latest` | `sox_index.value`、`sox_index.change_pct` |
| 台灣半導體週期 | `GET /api/dashboard/industry-cycle?industry=semiconductor` | `business_cycle`、`inventory_cycle`、`capex_cycle`、`confidence`、`trend` |

### 3.2 呈現方式

- 兩張 KPI 卡片：
  - **SOX 指數**：數值 + 漲跌幅（紅綠標示）。
  - **半導體週期燈號**：`business_cycle` 對應擴張/復甦/成熟/衰退，使用顏色燈號。
- 一張小表格：
  - Inventory cycle、capex cycle、confidence、trend。
- 資料缺失時使用 `renderEmptyState('尚無資料')` 佔位，不中斷頁面。

### 3.3 修改檔案

- `shared_web/static/js/pages/risk.js`：新增 `renderSemiconductorSentiment(snapshot, industryCycle)`。
- `admin_web/static/js/main.js`：在 `live` 頁面的 `loadPageData` 中，把 `/api/macro/snapshot/latest` 與 `/api/dashboard/industry-cycle?industry=semiconductor` 加入 Promise.all，並傳給 `renderSemiconductorSentiment`。

---

## 4. Orphan API 啟用

### 4.1 API 對應表

| API | 頁面 | 呈現內容 |
|-----|------|---------|
| `GET /api/dashboard/drawdown` | `live` | 在風控指標區新增「最大回撤 / VaR95」卡片 |
| `GET /api/backtest/signals` | `reports` | 在回測報告下方新增「回測信號」區塊：active_signals、VaR95/99、Sharpe、Drawdown% |
| `GET /api/backtest/snapshots` | `reports`（可選） | 若時間允許，顯示最近快照清單；否則先啟用 signals |
| `GET /api/dashboard/forecast-vs-reality` | `experiments` + `reports` | `experiments` 顯示命中率/實驗命中彙總卡片；`reports` 顯示 symbol predictions 詳細表格 |

### 4.2 修改檔案

- `shared_web/static/js/pages/risk.js`
  - 新增 `renderDrawdownPanel(data)`。
- `shared_web/static/js/pages/backtest.js`
  - 新增 `renderBacktestSignals(data)`、`renderForecastVsRealityTable(data)`。
- `shared_web/static/js/pages/experiments.js`
  - 新增 `renderForecastVsRealitySummary(data)`。
- `admin_web/static/js/main.js`
  - 在 `loadAll()` 與 `loadPageData()` 的對應頁面加入 API 呼叫。

---

## 5. 錯誤處理

- 所有新增 API 呼叫使用 `getJSONWithTimeout` 或 `silentGetJSON`，單一 API 失敗不阻塞整頁。
- 資料缺失時統一使用 `renderEmptyState` 佔位。
- console 僅輸出 error，不彈 alert。

---

## 6. 驗證與測試

### 6.1 Build

```bash
cd admin_web && npm run build
cd ../client_web && npm run build
```

兩邊都必須成功，且 `admin_web/dist/` 產出正確。

### 6.2 Smoke Test

```bash
cd admin_web && npm run test:smoke
```

確認 `live`、`reports`、`experiments` 三頁載入無 404/500，console 無未處理錯誤。

### 6.3 手動驗證清單

- [ ] 側邊欄只有 7 個入口。
- [ ] 直接輸入 `/admin/narrative` 等已刪除頁面 URL 會 fallback 到 `home` 或 404（依現有 `switchPage` 行為）。
- [ ] `live` 頁面出現 SOX 與半導體週期 panel。
- [ ] `reports` 頁面出現「回測信號」與「預測 vs 實際」區塊。
- [ ] `experiments` 頁面出現「預測命中追蹤」彙總卡片。

---

## 7. 後續可擴展

- 當 `OTCIndexProvider` 被註冊到 macro snapshot 後，可在 `live` 的「市場廣度」panel 直接加入 `otc_index` 卡片，無需再開新頁面。
- 若未來要讓投資人看到半導體景氣，可將同一份 render 函數套用到 `client_web` 的「產業地圖」或「市場總覽」。
