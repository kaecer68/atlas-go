# Gap Audit 3 — 錢潮雷達 → 散戶行動的引導/追蹤

> **盤查日期**: 2026-08-06
> **盤查者**: atlas AI (scout subagent, 4m17s 完成)
> **範圍**: READ-ONLY 盤查,未動任何 code
> **ACI 工具鏈**: `codegraph_explore` (7 個核心檔案 source)、`grep` (watchlist/追蹤/通知/已讀 API in client_web/static),`read` (`subscription/types.go` + `waitlist.go` + `alerting/doc.go` + `monitoring/alert_api.go` + `client_web/main.js` + `index.html` + `docs/investor/`)

---

## 1. 背景

產品定位 §9 誠實聲明:「平台的價值主張是:把『觀測 → 解讀 → 追蹤 → 紀律』系統化,降低散戶的資訊處理成本與情緒干擾。」

問題:目前系統是否只到「觀測 → 解讀 → 顯示」就結束?「追蹤 → 紀律」這兩個階段有沒有任何機制?

---

## 2. 現況發現

### 2.1 機制存在性

| 機制 | 定義 | 存在? | 證據 |
|---|---|:---:|---|
| 提醒/警示 | signal triggered → user notified | ❌ | `<div id="notificationCenter">` 容器存在但 0 注入邏輯;`internal/alerting` 只 output 到 Alertmanager webhook,**未回送 user** |
| 用戶紀錄 | user 標記某訊號已讀/已執行 | ❌ | 0 命中 `user_signal_ack` / `mark_read` / `已讀` / `user_feedback` API |
| 追蹤清單 | user 加入 watchlist,系統主動複檢 | ❌ | 0 命中 user-side watchlist 端點;`docs/investor/query-examples.md:269` 提的 `narrative_get_bundle.watchlist` 為 **spec 紙面欄位,實作缺失** |

### 2.2 完整證據鏈

#### client_web 前端

- `client_web/static/js/main.js:24-44` SHELL_LOADERS:19 個 page id,**0 個**與追蹤/紀律相關
- `client_web/static/js/main.js:160-175` loadModules 12 個 import,**無** `./pages/alerts.js`
- `client_web/static/js/main.js:204-208` `modules.alerts` 永遠 undefined (死碼)
- `client_web/static/index.html:99-130` sidebar nav 16 個連結,**0 個**對應追蹤/紀律
- `client_web/static/index.html:171` notification-center 容器存在,無 JS 注入
- `client_web/static/js/main.js:354-372` live page 載入 11 個 API,**未抓** user_state/preference

#### 內部系統皆為運維語意,不可複用為散戶功能

- `internal/monitoring/alert_store.go` `AlertRecord.Rule` 為 `simulation/experiment/etf_nav/crossmarket/universe_coverage/universe_watchlist/background_task/data_staleness/state_store/mcp_*`(**全運維**)
- `internal/monitoring/alert_api.go:42-50` 9 條路由為 operator alert CRUD
- `internal/repository/postgres_alerts.go:76-83` `acknowledged_by` 寫死為字串,**未與** `subscription.User.ID` 關聯
- `cmd/atlas/main.go:1662-1674` `universe_watchlist` = **D6 模擬標的池** (60 天淘汰),與散戶無關
- `internal/subscription/doc.go:9-13` 明確僅 tier-based access control
- `data/state/strategy_feedback/` 為策略 hit_rate 內部累積,**無 user_id**
- `AcknowledgeAlert` 寫死 `acknowledged_by` 為字串,**未**與 `subscription.User.ID` 關聯

### 2.3 與 §9 誠實聲明的對齊

| 承諾層 | 實作狀態 |
|---|:---:|
| 觀測 | ✅ |
| 解讀 | ✅ |
| 顯示 | ✅ |
| **追蹤** | ❌ |
| **紀律** | ❌ |

---

## 3. 缺口判定

> **REAL GAP,且直接損害核心使命**。非單純缺頁面,而是 4 個相互依賴子系統同時缺失:
> 1. **資料模型**:無 `user_signal_state` / `user_watchlist` / `user_journal` 實體
> 2. **API 層**:無 user 寫側 API(`/api/user/signals/:key/ack`、`/api/user/watchlist`、`/api/user/journal`)
> 3. **推送層**:`notification-center` 為空殼,無 SSE 訂閱
> 4. **UI 層**:無 nav 節點、無卡片「標記已讀」按鈕、無 watchlist 介面

且既有「universe_watchlist / strategy_feedback / alerting」命名近似但語意完全不同(運維語意),易誤導 PM/投資人以為「有 watchlist 啊?」 — 需在後續實作中明確分流。

---

## 4. 對齊核心目的程度

| 核心目的 | 對齊程度 |
|---|:---:|
| 觀測 | ✅ 高 |
| 解讀 | ✅ 高 |
| **追蹤** | ❌ **0** |
| **紀律** | ❌ **0** |
| 降低散戶資訊處理成本 | ✅ 高 |
| **降低散戶情緒干擾** | ❌ **0** |

整體對齊 ~50%。

**最關鍵損害**:§1 一句話定位「幫散戶建立**可紀律執行**的投資判斷流程」中的「**可紀律執行**」無法落地;§4 設計原則「人機協同」目前只在「機器自動」側完成,「人機」側只到「觀察/點擊頁面」,未到「行為覆盤」。

---

## 5. 建議

**必要 (若要在 §9 誠實聲明下維持產品完整度)**:

- **R1** 建立 `internal/userstate` 套件,定義 `UserSignalState` / `UserWatchlist` / `UserJournal` 三個基本實體
- **R2** `internal/subscription` 加 4 條 user 寫側 API
- **R3** 為 `notification-center` 空殼注入 SSE 訂閱 + signal 過濾邏輯
- **R4** `client_web` sidebar 新增「我的追蹤」nav 節點 + page shell
- **R5** 改寫 `query-examples.md` §C7:移除「`watchlist` 欄位」或標註「未實作」(最小範圍,可立即做)

**反建議 (不要做的事)**:

- ❌ 不要把 `universe_watchlist` 改造成 user-facing watchlist — 命名近似會造成嚴重語意污染
- ❌ 不要把 `internal/alerting` system-alert 通路當作 user 通知管道 — 語意層完全不同
- ❌ 不要讓「追蹤/紀律」變成付費才能用 — 違反 §9「降低情緒干擾」初衷

**最小可行範圍 (P0)**:

- R5 (修 `query-examples.md` §C7 標註 watchlist 為未實作) + R1 骨架 (建立 `internal/userstate` 套件 + 1 個實體型別,證明 R2-R4 設計意圖可落地)
- 範圍: 1 MD 修正 + 1 新套件 + 1 個實體型別
- 工作量: 1-2 天
- PR 數: 1

**完整實作 (P1+)**:

- R2-R4 為後續 PR,每個約 3-5 天
- 整個 Gap 3 完整實作預估 2-3 週

---

## Summary

- **findings_summary**: §9 誠實聲明承諾的「觀測→解讀→追蹤→紀律」四段中,後兩段(追蹤、紀律)在 client_web 完全沒有實作;notification-center 是空殼;既有 alerting/watchlist 是運維語意不可複用
- **is_real_gap**: true
- **value_to_core_mission**: high
- **recommended_action**: 最小範圍 R1 + R5 (1-2 天, 1 PR) 立即做,證明設計意圖;完整 R2-R4 為後續 P1+ 排程
