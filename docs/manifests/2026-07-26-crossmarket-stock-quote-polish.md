# Audit Manifest: 美台連動 + 個股快查 UI/資料 修正

> **Audit source**: 使用者 (2026-07-26) — http://localhost:18080/client/stock-quote & /client/crossmarket
> **Goal**: 修復 stock-quote 輸入框/按鈕風格、置中修正、us10y/vix 通道失敗、crossmarket 產業標籤中英混雜與直書格式
> **Scope**: shared_web 前端 CSS/JS + internal/marketdata + internal/apigateway + internal/monitoring/api/risk + internal/industry
> **Created**: 2026-07-26
> **Status**: in-progress

---

## 證據蒐集 (Investigation complete)

### 症狀 A: stock-quote 風格不一致
- **頁面 HTML**: `client_web/static/js/page-shells/stock-quote.js` (line 12-13) 注入 `#sq-symbol-input` / `#sq-search-btn`
- **當前 CSS**: `shared_web/static/css/pages/stock-quote.css`
  - `.sq-search-input` 使用 `var(--text-lg)` + `padding: var(--spacing-4) var(--spacing-5)` (line 28-39) — 巨大輸入框
  - `.sq-search-btn` 使用 `font-size: calc(var(--text-sm) * 0.85)` (line 52) — 比輸入框小
  - 邊框變數名混用 `--border` (line 32) 與 `--border-color` (line 100)
- **平台標準**: `shared_web/static/css/components/controls.css:3`
  - `.control-group input` 使用 `padding: 5px 9px; font-size: var(--text-sm); border: 1px solid var(--border); border-radius: 6px`
  - 焦點態 `border-color: var(--accent); box-shadow: 0 0 0 2px rgba(79,193,255,.15)`
- **結論**: 風格偏離平台 input/button token

### 症狀 B: stock-quote 個股舉例/免責聲明未置中
- **當前結構**: `client_web/static/js/page-shells/stock-quote.js` (line 11-14) 為 `.sq-search-bar` flex 容器,輸入/按鈕各自 `text-align` 預設
- **個股舉例 pills**: 該 pill (`#sq-symbol-display` 或新註入的 `.sq-search-pills` section) 預設 `align-items: center` (CSS line 66) 但 `text-align` 為 `start`
- **免責聲明**: 渲染於 `client_web/static/js/pages/stock-quote.js` 的 disclaimer 區塊,需 `text-align: center`
- **結論**: 兩者皆缺 `text-align: center`

### 症狀 C: crossmarket us10y + vix 通道回傳失敗
- **API 現況**: `curl /api/cross-market/status` → `failed: ['us10y', 'vix']`, `data_status: 'degraded'`
- **真實原因 (非猜測)**:
  1. `data/state/channel_health.json`: `us_yahoo: error — circuit breaker open for channel us_yahoo`
  2. `docker logs atlas-go` 在 07:37:24 / 07:42:26 紀錄:
     `partial_fetch_failures errors="[HG=F: zero latest price ... CL=F ... GC=F ... SI=F ... DX-Y.NYB ...]"` (5 個指標 off-hours 回傳 0)
  3. `YahooFinanceMacroProvider.FetchSnapshot` (`internal/marketdata/yahoo_macro_provider.go:113-121`) — 任何 indicator 失敗就 `errors.Join`,即使其他成功也包 error
  4. `YahooMacroChannelAdapter.Fetch` (`internal/apigateway/adapter_yahoo_macro.go:29-46`) — 將 `FetchSnapshot` 的 error 整包回傳給 gateway
  5. Gateway 對 us_yahoo 計入 CB 失敗 (3 次後開啟,5 分鐘復原)
- **歷史 refactor 證據**: `git log` 顯示
  - 233c0c72 P1 us10y zero-value guard (引入嚴格零值拒絕)
  - 685096cb P2 usCache param-keyed 升級 (整合 cache layer)
- **斷裂點**: adapter 沒有區分「**全失敗** vs **部分失敗**」。當 Snapshot 有 `RecordedAt > 0` 已是有效資料,但 adapter 仍把 partial error 視為整體失敗送進 CB → 整個 us_yahoo 通道(內含 us10y/vix/dxy/oil/gold/usd_twd/silver/copper 9 個指標)被 lock 5 分鐘
- **結論**: 真因是 `adapter_yahoo_macro.go` 的 error 處理不夠細 — 與 `HealthCheck` (line 51-75) 已經實作的「partial 仍視為 ok」邏輯不一致

### 症狀 D: crossmarket 產業相關性矩陣中英混雜
- **API 現況**: `curl /api/dashboard/correlation-matrix`:
  ```
  labels: ["AI 供應鏈","消費","defensive","電子零組件","能源","etf_rotation","金融","晶圓代工","high_dividend","工業","leo_satellite","礦業/貴金屬","pcb","機器人","半導體","伺服器組裝","航運","small_cap","tech","thermal"]
  ```
  → 20 個 sector 中 8 個仍是英文 ID
- **根因**: `internal/monitoring/api/risk/handlers.go:255-275` 的 `industryLabel` 是一個手寫 13 筆的 hardcoded map,未對齊 `configs/parameters.json` 的 `industry.classification_tree.segments`(已含 20+ sector 含中文 name 與 name_en)
- **產業標籤統一來源**: `configs/parameters.json` (ID ↔ Name 中文 ↔ NameEN 英文)

### 症狀 E: crossmarket 矩陣上方說明文字格式不一
- **現況**: `shared_web/static/js/pages/crossmarket.js:192` 使用 `writing-mode:vertical-rl;transform:rotate(180deg)` 直書 header
- **renderCorrelation** (line 135-164) `<td>` 內 inline style `font-weight:700;font-family:var(--font-mono)` 為橫書+粗體
- **renderCrisis** (line 212-245) 為 panel 內部文字橫書
- **色階說明** (line 208) 為橫書
- **結論**: 多種風格並存,需統一為「直書中文 + 字型大小一致」,但矩陣儲存格直書合理,**只把 header 與圖例統一格式**即可

---

## Invariant Tracker

| ID | 問題 | 根因(有證據) | 變更檔案 | 驗收 | 狀態 | 備註 |
|----|------|------------|---------|------|------|------|
| A1 | stock-quote 輸入框/按鈕風格不一致 | `pages/stock-quote.css:28-57` 偏離 controls.css token | `shared_web/static/css/pages/stock-quote.css` | 比對 controls.css 風格 | pending | 影響樣式 |
| A2 | 個股舉例/免責未置中 | shell 與 pages stock-quote.js 缺 `text-align: center` | `shared_web/static/css/pages/stock-quote.css` + `client_web/static/js/pages/stock-quote.js` | 視覺置中 | pending | 文字排版 |
| B1 | us10y/vix 失敗 | `adapter_yahoo_macro.go:29-46` partial 失敗仍報 error → CB open | `internal/apigateway/adapter_yahoo_macro.go` | curl /api/cross-market/status us10y 有值 | pending | 接線斷裂 |
| B2 | 回歸測試 | 缺 partial-success 測試 | `internal/apigateway/adapter_yahoo_macro_test.go` | go test 通過 | pending | 防回歸 |
| C1 | 產業中英混雜 | `risk/handlers.go:255` hardcoded 13 筆 map,未對齊 configs | `internal/monitoring/api/risk/handlers.go` + 用 `internal/industry` 的 `IndustryRegistry.NameByID` | 20 sector 全中文 | pending | 統一翻譯 |
| C2 | 直書格式不一 | crossmarket.js 多種 inline style 並存 | `shared_web/static/js/pages/crossmarket.js` | header 直書統一,字體大小一致 | pending | UI 一致 |

---

## Phase Tracker

### Phase A — Audit (read-only)

| Task | Status | Evidence |
|------|--------|----------|
| 重現症狀 A (stock-quote) | accepted | curl + page-shells/stock-quote.js line 11-13 |
| 重現症狀 B (置中) | accepted | CSS 缺 `text-align: center`,頁面 HTML 結構 |
| 重現症狀 C (us10y) | accepted | docker logs 顯示 zero-latest 5 指標, channel_health.json CB open, adapter_yahoo_macro.go:29-46 邏輯 |
| 重現症狀 D (中英) | accepted | curl /api/dashboard/correlation-matrix labels 8/20 為英文 ID |
| 重現症狀 E (直書) | accepted | crossmarket.js inline style 混用 |

### Phase B — Plan

| Task | Status | Evidence |
|------|--------|----------|
| 對齊 controls.css token | pending | A1 |
| 文字置中 | pending | A2 |
| adapter partial-success 邏輯 | pending | B1 需依 Snapshot.RecordedAt 區分 |
| 補回歸測試 | pending | B2 |
| IndustryRegistry 取中文 | pending | C1 需查 internal/industry 是否已有 NameByID |
| 矩陣 header 統一 | pending | C2 |

### Phase C — Implement

| Task | Status | Evidence |
|------|--------|----------|
| 修改 stock-quote.css | pending | - |
| 修改 stock-quote.js (頁面) | pending | - |
| 修改 adapter_yahoo_macro.go | pending | - |
| 新增 test | pending | - |
| 修改 risk/handlers.go | pending | - |
| 修改 crossmarket.js (header style) | pending | - |

### Phase D — Verify

| Task | Status | Evidence |
|------|--------|----------|
| 視覺檢查 3 頁面 | pending | - |
| curl 驗證 us10y 有值 | pending | - |
| curl 驗證 labels 全中文 | pending | - |
| ci-gate 通過 | pending | - |
