# Audit Manifest: 2026-08-10 Fugle Unified Access & Data Governance

> **Audit source**: 三次對話問題統一盤查 — (1) 資料鏈 degraded 修復（A01-A05/B01-B05，7 PR 已 merged）; (2) B06 breaker API 評估; (3) stock-quote OHLC 殘缺 + Fugle 破表 + 非交易日報錯盤查
> **Goal**: 一次性收斂 Fugle（及一般外部 provider）的消費架構到單一存取層；校正 rate/quota 至真實值；統一完整性驗證與錯誤分類；補齊文件漏洞（為何每次盤查只看到一部分）
> **Created**: 2026-08-10
> **Status**: planned

---

## 一、三次對話問題統一梳理

| 對話 | 表象 | 已修（PR） | 遺留的系統性問題 |
|------|------|-----------|-----------------|
| 1. 資料鏈 degraded | tw_vol/twse_etf/auto_cycle_update 三鏈失敗 | A01-A05（#1502）、B01-B05（#1503-1507） | 錯誤分類只在部分 provider 實作；假日機制分散未全面接入 |
| 2. B06 評估 | 資料通道 breaker 無 API 暴露 | 決定不做（#1508，記觸發條件） | 盤查發現 breaker 只在 gateway 層 — 非 gateway 消費路徑無 breaker |
| 3. stock-quote 盤查 | OHLC 殘缺 200 / Fugle 破表 / 非交易日 503 | 未修（本 manifest） | HandleQuote 無完整性驗證；rate/quota 閾值高於免費實際；假日機制未接 HandleQuote |

**統一視角**：三個對話是**同一個元問題**的多個表象 — **外部資料源（Fugle）消費沒有單一存取層**。rate/quota 已統一（shared singleton + QuotaRegistry），但 **breaker / health 記錄 / 完整性驗證 / 錯誤分類分散在三套架構**，每次修復只覆蓋其中一層 → 重複出問題（Fugle 相關 commit 46 筆）。

## 二、全局盤查發現（ACI 校驗）

### 2.1 Fugle 五個消費層 — 治理機制對照

| 層 | 路徑 | rate | quota gate | breaker | health 記錄 | 完整性驗證 | 錯誤分類 |
|----|------|------|-----------|---------|-------------|-----------|---------|
| L1 | gateway fugle channel（`adapter_fugle.go`）| ✓ shared | ✓ | ✓ apigateway | ✓ channel_health | ✗ | 部分（warn on quota）|
| L2 | stocktools HandleQuote（`handler.go:128`）| ✓ shared | ✓ | ✗ | ✗ | ✗ **→ 殘缺 200** | ✗（401 隱形）|
| L3 | stocktools HandleTechnical（`handler.go:293`）| ✓ shared | ✓ | ✗ | ✗ | 弱（len<2）| ✗ |
| L4 | hybrid provider（`hybrid_provider.go:116`）| ✓ shared | ✓ | ✓ providerBreaker | ✗ | **✓ hasInvalidQuotes** | 部分 |
| L5 | warmup（`dashboard_api.go:911`）| ✓ shared | ✓ | ✗ | ✗ | ✗ | ✗ |

- **rate/quota 統一 ✓**：`GetSharedFugleClient` singleton + `DailyQuotaTracker`（file-based 跨 process）+ `GlobalQuotaRegistry`（#1451）
- **breaker 三套**：apigateway `CircuitBreakerManager`（只蓋 L1）/ hybrid `providerBreaker`（只蓋 L4）/ 無（L2/L3/L5）
- **health 只有 L1 記錄**：L2/L3/L5 的 Fugle 失敗完全不留 channel_health → **dashboard 看不到 stocktools 路的 Fugle 問題**
- **完整性驗證只有 L4 有**：`hasInvalidQuotes`（Last==0 && OHLC==0 → invalid）— **L2 缺同一檢查**

### 2.2 Fugle 破表機制缺陷（統一管理為何失效）

| ID | 缺陷 | 證據 |
|----|------|------|
| F1 | 日額度 gate = `fugleDailyLimit 86400`（理論上限 60×1440），免費實際遠低 → **gate 永不先擋，永遠先被 Fugle upstream 拒絕** | `fugle_client.go:38` 註解自承「conservative upper bound」 |
| F2 | 分鐘 rate 設 60/min，但內部實測免費 ~39/min 就 429 → **設定值高於實際承受** | `fugle_client.go:33` burst 註解「measured 429 at ~39 live calls, 2026-08-03」 |
| F3 | Fugle 破表後鎖定回 **401**（非 429）→ `doGet` 401 不走 429 retry 分支 → **無 rate_limit_429 log、完全隱形**；今日額度重算 key 恢復 → 偽裝成「key 有效但曾失效」 | 08-09 文件實測 401；`doGet:239` 僅 429 進 retry |
| F4 | 高峰來源集中單 bucket：warmup ~32 calls 啟動 + HandleTechnical on-demand + HandleQuote + gateway liveness 併發 → 超過實際 ~39/min | `dashboard_api.go:833`、DefaultSymbols 41 個 |

### 2.3 非交易日報錯 — 假日機制接入缺口

`isTaiwanTradingDay`/`isTaiwanHoliday`（B05，`marketdata/calendar.go:103-131`）**只接入 marketdata provider 層**：

- ✅ 已接：`finmind_client.go:480`、`taiwan_index_cache.go:47`、`capitalflow/service.go:244`、`industry/event_calendar.go:923`
- ❌ 未接：**stocktools HandleQuote/HandleTechnical**（`handler.go:108/255`）、gateway TWSE 相關排程、daily-replay-sync

非交易日 HandleQuote → Fugle 失敗/回前一交易日 → fallback TWSE → `STOCK_DAY_ALL` 空 → **503「symbol not found」誤導**。

### 2.4 文件漏洞（為何每次盤查只看到一部分）

| ID | 漏洞 | 後果 |
|----|------|------|
| D1 | CONSTITUTION 第一條「所有外部 API 走 `apigateway.Fetch`」— 但 L2/L3/L4/L5 **違反**（直接呼叫 FugleClient），**無 CI 檢查、無違規清單** | 盤查者以為全走 gateway，實際只有 L1 → 漏看 L2-L5 |
| D2 | CONSTITUTION 記載 fugle `60/min (1s/1b)` — 與實際承受 ~39/min 不符，文件未反映 | 每次設定照文件 60 → 破表 |
| D3 | 無「外部資料源存取架構」文件（消費層 × 治理機制對照）| 每次盤查重新發現架構分裂（本 manifest §2.1 首次完整記錄）|
| D4 | `known_issues.go` 無 Fugle 記載（僅 twse_etf 等）| 08-09 的 401/破表無登錄 → 下次重蹈 |
| D5 | 401 = 破表鎖定語義無記載（#1449 曾修正「key 誤解」但方向錯 — 問題不是 key 是破表）| 診斷誤導 |
| D6 | 資料源優先級「TWSE → Fubon → FinMind → Fugle」vs HandleQuote「Fugle 優先」無例外說明 | 盤查者困惑（Fugle 唯一免費即時源，需明示例外）|

## 三、問題模型（統一）

**元問題：外部資料源消費無單一存取層 — 治理機制（rate/quota/breaker/health/驗證/錯誤分類）跨消費層不一致，修復只能逐層打地鼠。**

具體問題（按優先級）：
1. **P0-F**：Fugle rate/quota 閾值高於免費實際（F1/F2）→ 破表必然
2. **P0-F**：401 隱形（F3）→ 破表無訊號
3. **P0-Q**：HandleQuote 無完整性驗證（L2）→ 殘缺 200
4. **P1**：假日機制未接 quote 路徑 → 非交易日 503
5. **P1**：L2/L3/L5 無 breaker/health → Fugle 問題不入 channel_health
6. **P2**：文件漏洞 D1-D6 → 盤查盲點重複

## 四、一次性解決方案（分階段）

### Phase A — Fugle 真實參數校正（小，先止血）
- A1: `fugleDailyLimit` 改免費實際日額度（保守估計，需 kaecer 確認；未確認前先改 2000/day 保守值 + 註解標記）
- A2: 分鐘 rate 60 → 40/min 保守（burst 3）；`parameters.json` `fugle_rate_limit` 同步 60→40
- A3: `doGet` 401 納入 quota 語義：401 時檢查是否 quota 相關（Fugle 鎖定）→ 記 `rate_limit_401` warn log + 進 breaker（L1）+ health warn；非 quota 401 才算 auth error
- 測試：quota gate 在 <2000 觸發；rate 40/min 邊界

### Phase B — 完整性驗證統一（共用函數）
- B1: 把 `hybrid_provider.hasInvalidQuotes` 提升為 marketdata 共用函數 `ValidateQuoteCompleteness(q)`（Last==0 或 OHLC 全 0 → incomplete）
- B2: HandleQuote 用共用函數：Fugle 200 但 incomplete → 視為失敗 → fallback TWSE；TWSE 也 incomplete → 回 200 + `complete:false` 標記（非殘缺 200）
- B3: `domain.Quote` 加 `Complete bool` 欄位（json `complete`）或 handler response 層加欄位
- 測試：Fugle closePrice-only 回應 → fallback/標記；TWSE 完整 → 200+complete:true

### Phase C — 假日機制全面接入 quote 路徑
- C1: HandleQuote/HandleTechnical 前置 `isTaiwanTradingDay(now)` 檢查
- C2: 非交易日 → 標記 `trading_day:false` + 照常回前一交易日資料（不清零 OHLC）
- C3: 統一交易日查詢入口（marketdata 已存在 `isTaiwanTradingDay`，stocktools 引用）
- 測試：週日 2330 → 200 + trading_day:false + 前一交易日完整 OHLC

### Phase D — 存取層收斂（大，架構性）
- D1: 建立 `marketdata.FugleAccess`（單一存取層）：封裝 rate/quota gate/breaker（單一 FSM）/health 記錄/錯誤分類（typed: Quota/RateLimited/Auth/Upstream/Incomplete）
- D2: L1-L5 全部改走 FugleAccess（gateway adapter、stocktools、hybrid、warmup）→ breaker/health/驗證單一事實來源
- D3: 所有 Fugle 失敗寫 channel_health（stocktools 路徑也進 dashboard）
- D4: 401/429/503 統一分類透出（MCP/API 層可見「fugle quota」vs「twse 無資料」）
- 測試：各層經 FugleAccess 的 breaker 共享測試

### Phase E — 文件治理（防再犯）
- E1: 建立 `docs/reference/data-source-architecture.md`：每 provider（Fugle/FinMind/TWSE/Yahoo）消費層 × 治理機制對照表（本 manifest §2.1 為雛形）
- E2: `known_issues.go` 登錄：Fugle 免費破表 401 事件、rate 實際值、HandleQuote 殘缺歷史
- E3: CONSTITUTION 更新：fugle 實際限制（40/min）、401 語義、L2-L5 為「已知例外（已收斂至 FugleAccess）」
- E4: CI 檢查：grep 直接呼叫 `FugleClient.GetQuote/GetHistoricalCandles` 必須經 FugleAccess（仿 check_constitution.sh）
- E5: HandleQuote「Fugle 優先」例外說明（Fugle = 唯一免費即時源）寫入 CONSTITUTION 資料源優先級章節

## 五、驗證方式

- Phase A/B/C：`go test ./internal/marketdata/... ./internal/stocktools/...` + 新增測試（每項列出）
- Phase D：全層測試 + `make ci-gate/ci-full`
- 部署後：MCP `stock_get_quote` 2330（交易日完整）/ 660（policy 語義）/ 非交易日（trading_day:false）；`channel_health` 看 stocktools 路 Fugle 失敗入健康；`data_get_channels` 看 fugle 狀態
- E：文件 review + CI 新檢查過 gate

## 六、Backlog（非本 manifest 範圍）

| ID | 問題 | 狀態 |
|----|------|------|
| B06 | 資料通道 breaker API 暴露 | 已評估不做（#1508），記觸發條件 |

## Change Log

| Date | Version | Change | Author |
|------|---------|--------|--------|
| 2026-08-10 | 1.0 | Initial manifest — 三次對話統一盤查 + Fugle 存取層問題模型 + 分階段方案 | agent |
