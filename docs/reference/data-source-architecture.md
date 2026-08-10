# 外部資料源存取架構（Data Source Access Architecture）

> **目的**：單一事實來源 — 外部資料源（Fugle / FinMind / TWSE / Yahoo）的消費層、治理機制（rate / quota / breaker / health / 完整性驗證 / 錯誤分類）一覽。任何修改外部資料源抓取的程式碼，或盤查「為什麼某 provider 又出問題」，**先讀本文件**，避免重複「每次只盤查到一部分」的歷史（見 `.omo/manifests/2026-08-10-fugle-unified-access.md` 盤查記錄）。

---

## 1. Fugle 存取架構（2026-08-10 收斂後）

Fugle 有 **5 個消費層**，全部共用同一個 `GetSharedFugleClient` singleton（`internal/marketdata/fugle_client.go`）— 因此 **rate limiter、daily quota gate、client 級 breaker 在單一 client 內統一**，所有消費層共享同一治理狀態：

| 層 | 消費路徑 | API 方法 | rate | quota gate | breaker | health 記錄 | 完整性驗證 |
|----|---------|---------|------|-----------|---------|-------------|-----------|
| L1 | gateway fugle channel（`internal/apigateway/adapter_fugle.go`）| `GetQuote` | ✅ client 共享 | ✅ | ✅ client 級 + apigateway 級 | ✅ channel_health | 由下游 |
| L2 | stocktools `HandleQuote`（`internal/stocktools/handler.go:108`）| `GetQuote` | ✅ client 共享 | ✅ | ✅ client 級 | ⚠️ slog（不入 channel_health）| ✅ `QuoteComplete`（殘缺 → fallback/`complete:false`）|
| L3 | stocktools `HandleTechnical` on-demand（`handler.go:255`）| `GetHistoricalCandles` | ✅ client 共享 | ✅ | ✅ client 級 | ⚠️ slog | 弱（len<2）|
| L4 | hybrid provider（`internal/marketdata/hybrid_provider.go:116`）| `GetQuotes` | ✅ client 共享 | ✅ | ✅ client 級 + providerBreaker | ⚠️ slog | ✅ `QuoteComplete`（hasInvalidQuotes）|
| L5 | quote warmup（`internal/monitoring/dashboard_api.go:911`）| `GetHistoricalCandles` | ✅ client 共享 | ✅ | ✅ client 級 | ⚠️ slog | 無（raw bars）|

**治理機制三層（依 scope）**：
1. **client 級**（FugleClient 內）：rate limiter（`rate.NewLimiter`）→ daily quota gate（`DailyQuotaTracker` + `GlobalQuotaRegistry`）→ **breaker**（`providerBreaker`，threshold 3 / recovery 5min / half-open 2）→ 401/429 typed errors。**所有消費層共享**（manifest Phase D）。
2. **apigateway 級**（僅 L1）：`CircuitBreakerManager` + `UnifiedHealthStore`（channel_health / system-health / wave9 metrics）。
3. **hybrid 級**（僅 L4）：`providerBreaker` 的 fubon/fugle FSM + fallback 鏈（fubon → finmind → fugle → twse）。

**設計決策（2026-08-10）**：
- 不為每個消費層建立獨立 breaker — 共享 client 的單一 breaker 已讓 L1-L5 統一熔斷（`ErrFugleBreakerOpen` 短路，不再逐層打上游）。
- L2/L3/L5 的失敗記錄在 slog（`stocktools: fugle quote failed/incomplete ...`），**不入 channel_health** — channel_health 是系統級健康視圖（L1 代表 Fugle channel）；per-symbol 需求失敗會透出 503/complete:false/typed error 給 API 消費者。若未來需要 dashboard 可見 stocktools 路徑失敗，再擴充 health recorder 注入。

## 2. Fugle 問題語義與防範（2026-08-10 修復後）

| 上游訊號 | 本地處理 | 防範 |
|---------|---------|------|
| **401**（免費破表鎖定 / key 無效 — **免費 tier 破表回 401 非 429**）| `ErrFugleUnauthorized` typed + `rate_limit_401` warn log + breaker failure | 不再隱形（歷史：401 不走 429 retry 分支 → 無 log → 偽裝成「key 有效但曾失效」）|
| **429**（sliding window）| Retry-After / backoff ×3 → 耗盡後 breaker failure | `rate_limit_429` log + 本地 rate 30/min < 實測破表點 ~39/min |
| **quota gate**（本地日額度）| `ErrFugleQuotaExhausted` → channel warn / fallback | `fugleDailyLimit = 2000`（歷史 86,400 理論值 = gate 虛設，永不先擋）|
| **breaker open** | `ErrFugleBreakerOpen` 短路，不發 HTTP | 連續 3 失敗 → 5min recovery → half-open 探測 |
| **殘缺 200**（closePrice-only：Last>0 但 OHLC 全 0）| `QuoteComplete` 判定 → HandleQuote fallback TWSE；全殘缺 → `complete:false` | `domain.Quote.complete` 明確訊號（歷史：靜默回殘缺 200）|

**剩餘風險（TODO）**：Fugle 免費 tier 官方日額度未確認（`fugleDailyLimit` 2000 為保守估計，見 `fugle_client.go` 註解）。確認後校正。

## 3. 交易日機制接入點（假日/非交易日防範）

`isTaiwanTradingDay`（`internal/marketdata/calendar.go`，含 2023-2030 農曆假日表）**必須接入**所有「以當日為 asOf 的行情取得路徑」：

| 已接入 | 路徑 |
|--------|------|
| ✅ | `finmind_client.go` FinMindProvider.GetQuotes（非交易日拒絕）|
| ✅ | `taiwan_index_cache.go` tw_vol freshness（盤前/假日寬限）|
| ✅ | `capitalflow/service.go`（非交易日跳過）|
| ✅ | `industry/event_calendar.go` EventCalendar.IsTaiwanTradingDay |
| ✅ | **stocktools HandleQuote / HandleTechnical**（2026-08-10 接入：非交易日 → `trading_day:false` 標記，**不**因 TWSE 空快照回誤導 503）|

**規則**：新增任何「當日行情」路徑，必須標記 `trading_day`（`domain.Quote.TradingDay` / technical response 欄位），不得讓非交易日誤導為「今天沒開盤」或「symbol 不存在」。

## 4. 資料源優先級與例外

CONSTITUTION 資料源優先級：**TWSE → Fubon → FinMind → Fugle**。

**已知例外（2026-08-10 定案）**：`/api/stock/quote`（HandleQuote）以 **Fugle 優先** — Fugle 是唯一免費提供即時盤中報價的來源；TWSE OpenAPI 只有盤後資料。此例外寫入 CONSTITUTION 資料源優先級章節。其餘路徑（回填/重放/批次）維持 CONSTITUTION 優先級。

## 5. 防再犯規則（修改外部資料源程式碼前必讀）

1. 本文件 §1 表格更新任何消費層/治理機制變更 — 不允許「新增消費層但不入表」。
2. Fugle 新 API 方法必須走 `doGet` 或明確加上 rate + quota + breaker（`GetHistoricalCandles` 歷史教訓：只過 rateLimiter 導致 warmup 不受日額度 gate 保護）。
3. provider 新錯誤必須 typed（`ErrFugle*`），401/429/503/quota 語義不得混用。
4. 非交易日路徑必須標記 `trading_day`。
5. 所有「殘缺資料」判定走 `QuoteComplete` 共用函數，不允許各層內聯不同規則。

---

**相關文件**：`internal/apigateway/CONSTITUTION.md`（資料源憲法）、`docs/reference/traps.md`、`.omo/manifests/2026-08-10-fugle-unified-access.md`（盤查與修復 manifest，本地）。
