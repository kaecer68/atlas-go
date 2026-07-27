# E4 方法論頁面審查與修復紀錄

> **審查對象**: PR #1397 `feat(methodology): add 七時期 UI + 因果傳導鏈頁面 (E4)`
> **審查依據**: 本地提示詞 `E4_方法論頁面_實作提示詞_v3.md`（位於 `/Users/kaecer/Downloads/`）、`docs/ATLAS_METHODOLOGY.md`
> **審查分支**: `fix/20260727-methodology-review`
> **審查日期**: 2026-07-27

---

## 1. Phase 0 結論：源頭落點

| 類型 | 源頭路徑 | 說明 |
|------|---------|------|
| JS 頁面 | `shared_web/static/js/page-shells/methodology.js` | `client_web/static/js/page-shells/` 下無同名檔案，esbuild shared plugin 會從 app tree fallback 到 shared tree；與 `crossmarket.js`、`capital_board.js` 落點一致。 |
| 路由註冊 | `client_web/static/js/main.js` 的 `SHELL_LOADERS` + 側邊欄 `<nav>` | 標準 page-shell 模式，已正確註冊 `methodology` 並在「更多」區塊加入側邊欄連結。 |
| CSS 頁面 | `shared_web/static/css/pages/methodology.css` | 經 `shared_web/static/css/main.css` `@import`。 |
| CSS tokens | `shared_web/static/css/base/variables.css` | 已新增 `--regime-*` 七時期色票（深/淺兩段）。 |

---

## 2. 發現的問題與修復項目

### P0 — 功能性問題

| # | 問題 | 影響 | 修復方式 |
|---|------|------|---------|
| P0-1 | `getCrossField()` 以 `!raw.symbol` 與 `value <= 0` 作為資料失效判斷 | 可能把 API 正常回傳（如 symbol 短暫缺失、或極端行情下的合法數值）誤判為「資料獲取失敗」 | 改為僅以 `value == null` / `Number.isNaN(value)` 判斷；symbol 僅作輔助提示，不阻塞顯示。 |
| P0-2 | `main.js` 的 `loadPageData()` 未處理 `pageId === 'methodology'` | 停留在方法論頁面時，30 秒 auto-refresh 不會更新；從其他頁面切換回來也走不到 lazy loader | 新增 `else if (pageId === 'methodology')` 分支，呼叫 `mod.methodology.loadMethodologyData()`。 |
| P0-3 | 三態歷史色帶在 API 無資料時未隱藏 | 提示詞要求「整區塊隱藏」，實作顯示 empty 狀態訊息 | `renderRegimeHistory()` 在 sessions 為空或解析失敗時，將 `#md-regime-host` 設為 `display:none`。 |

### P1 — 體驗與提示詞對齊

| # | 問題 | 影響 | 修復方式 |
|---|------|------|---------|
| P1-1 | Modal 中「對決策的含義」未獨立成區塊 | 提示詞要求 Modal 包含：憲章引用、指標清單、對決策的含義 | 在 Modal 內新增「對決策的含義」區塊，內容取自 `LAYER_IMPLICATION`（憲章原文，禁止自由發揮）。 |
| P1-2 | 頁首標題為「方法論」而非「ATLAS 方法論：因果傳導鏈」 | 與提示詞要求不一致 | 更新 `template` 中 `.md-page-header` 的標題與副標題。 |
| P1-3 | 策略矩陣使用 `all_weather`/`value`/`growth`/`momentum` 等程式名稱 | 提示詞要求使用「跟隨聰明錢啟動 / 事件套利 / 資金對抗後爆發 / 防禦 / 現金」等散戶友善用詞 | 新增 `STRATEGY_DISPLAY_NAME` 映射，在不改變後端契約的前提下，於策略推薦區與矩陣表格顯示中文用詞。 |

### P2 — 已知限制（不影響上線，需文件化）

| # | 項目 | 說明 |
|---|------|------|
| L1 | 第二/四/五/六層部分指標「資料源未接入」 | 依提示詞誠實佔位：第二層（出口訂單、設備進口、台積電月營收）、第四層（加權指數、成交量、當沖佔比）、第五層（壽險/銀行、公司派/內部人）、第六層（融資維持率、Google Trends）。 |
| L2 | 七時期歷史軸待後端 `period_history` 提供 | 目前僅能消費 `/api/regime/history` 的三態（RISK_ON/OFF/NEUTRAL/TRANSITIONAL）畫色帶，已誠實標示。 |
| L3 | PR #1397 連帶改動後端 Go 檔案 | 為使前端 TypeScript 型別生成器讀取 `internal/dailyreport` 型別，PR 修改了 `cmd/gentags/main.go`、`internal/config/config.go`、`cmd/experimental/staging-drill-strategy-techniques/main.go`、`internal/orchestrator/plugin_adapters_test.go`。雖屬必要的生成器/測試支撐，但已違反提示詞「本 PR 純前端」之鐵律；本次不再回退（已合併），僅紀錄於此。 |

---

## 3. 未改動但確認無誤的項目

- 八層結構、名稱、順序與憲章第二章一致，未合併或改名。
- 時期色票與 CSS 變數僅存於 `variables.css`，methodology.js / CSS 無硬編碼色碼（陰影 rgba 黑色為通用設計 token，不屬色票）。
- 資料來源對齊提示詞：第〇層混源、第一層 `/api/cross-market/status`、第三~六層 `/api/capital-flow/summary|daily`、第七層 `report.events`。
- 各 API 獨立 error/empty/tier-gate：因 `silentGetJSON()` 會吞錯誤回傳 `null`，`init()` 內 `Promise.all` 不會單點失敗拖垮整頁。
- Tier 行為：僅 premium 載入 `/api/reports/latest` 與 `/api/capital-flow/summary`，非 premium 顯示「升級查看即時數值」，結構圖與憲章內容全 tier 可見。

---

## 4. 驗收狀態

- [x] `npm run build`（client_web）通過
- [ ] 訪問 `/client/methodology` 人工驗證（需本地後端或 smoke test）
- [ ] 逐一模擬 API 斷線確認對應區塊 error 態
- [ ] `data-theme=light` 視覺檢查
- [ ] Playwright smoke test 通過（若存在）

---

## 5. 版本歷史

| 版本 | 日期 | 變更摘要 |
|------|------|---------|
| v1.0 | 2026-07-27 | 建立審查紀錄，標註 P0/P1 修復項目與已知限制 |
