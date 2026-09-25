# Universe 報價鏈可靠性規格（issue #1986）

> **狀態**：active（2026-09-25）
> **對應程式碼**：`internal/marketdata/{quote_batch.hybrid_provider,coverage_completing_provider,tpex_daily_close}.go`、`internal/monitoring/universe_scheduler.go`、`cmd/atlas/bootstrap_helpers.go`
> **相關**：#1965（母體實作）、#1977（per-stock 母體 gate）、#1979（I25 + chunk 化）、#1985（母體失效告警三條）、#1943（第一方 symbol_industry）、#1954（FinMind 保留配額）

## 1. 問題陳述（生產實證）

2026-09-25 07:10Z 生產母體建構（Mac Mini，`uniiverse_snapshot.json` 原文）：

```json
"symbols_built": 1599, "symbols_filtered": 1599, "symbols_ranked": 150, "symbols_excluded": 130,
"quotes_status": "partial", "quotes_returned": 1301, "quotes_requested": 1599,
"quotes_chunks": 32, "quotes_chunks_failed": 2,
"ranked_fallback_reason": "quote_fetch_partial", "ranked_trustworthy": false
```

`ranked_trustworthy=false` 使排名不能作為 D6 基準。生產 `ATLAS_MARKET_DATA_PROVIDER=hybrid`，
`fubon-proxy` 在該窗口**全程健康**（access log：28 個 50 檔 chunk 全部 HTTP 200，無 ERROR）。
但是同一份 proxy log 的**呼叫間隔**是 4–6 秒（健康 chunk）與 59–61 秒（落入 fallback 的 chunk）交替，
且 32 個 chunk 只有 28 次 proxy 呼叫。同日的佇列配額檔：`finmind_daily_quota.json`
`calls_today=14361`（上限 14,400，剩 39），`fugle_daily_quota.json` `calls_today=473`，
`FUGLE_TIER` 未設定 ⇒ Fugle 免費層 `rate_limit=30`（30 req/min）。

## 2. 根因（實測證實）

`HybridProvider` 舊行為是**整批語意**：

```go
// 舊：批次中任一 quote 不完整 ⇒ 丟掉整批，改問下一個 arm
if err == nil && len(quotes) > 0 && !p.hasInvalidQuotes(quotes) { return quotes, nil }
```

而 fallback 鏈的兩個中間 arm 都是**逐檔一次 HTTP**：

| arm | 每次呼叫成本 | 速率上限 | 50 檔 chunk 的理論下界 |
|---|---|---|---|
| fubon-proxy `/quotes` | 1 HTTP（proxy 內部逐檔迴圈） | `FubonIntradayLimit` | ~5 秒（實測） |
| FinMind `GetQuotes` | **每檔 1 HTTP** | 600/hr（free）或 6,000/hr（sponsor） | 300 秒 / 30 秒 |
| Fugle `GetQuotes` | **每檔 1 HTTP** | 30 req/min（tier 未設定 = free） | 100 秒 |
| TWSE `STOCK_DAY_ALL` | 1 HTTP（**全市場**） | 共用 3 req/5s | ~2 秒 |

⇒ 一個缺檔 symbol 會讓整個 50 檔 chunk 走進「逐檔 fallback」，成本遠超 60 秒的 chunk timeout。
另外 `recordFailure()` 原本在「批次不完整」時就呼叫，所以**三個這種 chunk 就會打開 fubon breaker 5 分鐘**
——正好對應 proxy log 只有 28 次呼叫而快照記 32 個 chunk。

**逐項實測（本 PR 的迴歸測試，`internal/marketdata/hybrid_quote_chain_test.go`）**：

* 50 檔中 1 檔缺 → 舊行為 fallback 送 50 次逐檔請求；新行為送 **1 次**。
* 50 檔全缺（primary arm 掛掉）→ 新行為**完全不呼叫**逐檔 arm，直接交由全市場 arm 收斂。
* 6 輪各有 1 檔不可用 → fubon breaker 保持 closed，6 輪都問了 fubon。

## 3. 設計

### 3.1 殘量鏈（`HybridProvider.GetQuotesBatch`）

鏈上的每個 arm **只被問「尚未解決的殘量」**（`QuoteBatch.Missing`）。單一 arm 的答案不再被整批丟棄：

* 可用 quote（`QuoteComplete` 且價格/量非負）→ 記為 `ok`。
* arm 有發佈該檔但無可用價格，且該 arm 是**全市場**來源（TWSE 全市場表）→ `no_data`。
* arm 有發佈該檔但無可用價格，且該 arm 是**逐檔**來源 → 不給 `no_data`（一次全零的盤中回應與「arm 壞掉」不可區分），
  該檔留待下一個 arm。
* arm 成功但沒有該檔 → 全市場 arm 記 `not_covered`；逐檔 arm 記 `error`。

### 3.2 逐檔 arm 的護欄（實測決定，非推論）

| 常數 | 值 | 依據 |
|---|---|---|
| `hybridNarrowResidualMax` | 8 | 最慢的可派遣層是 Fugle free（30 req/min ≈ 2 s/request）⇒ 8 檔 ≈ 16 s，與下一個常數同數量級；且 TWSE 全市場 arm 永遠在鏈尾，殘量真正的收斂者是它，不是逐檔 arm。 |
| `hybridNarrowArmTimeout` | 10 s | 逐檔 arm 的硬性 deadline；最壞情況 10 + 10 = 20 s，遠低於 60 s chunk timeout。 |
| `DefaultQuoteChunkSize` | 50（不變） | 逐檔成本已由上面兩個常數封頂，chunk 尺寸不再是成本函數；維持 50 使 1,599 檔 = 32 個呼叫，且與 #1979 已驗證的形狀一致。 |
| `DefaultQuoteChunkTimeout` | 60 s（不變） | 最壞情況實測上界：fubon（自身 client timeout）+ 10 s + 10 s + TWSE（重試排程）≈ 45 s < 60 s；見 §3.3 的量測。 |
| `DefaultQuoteChunkPause` | 100 ms（不變） | 32 chunk ⇒ 3.2 秒總延遲，可忽略。 |

`SetNarrowFallbackPolicy` 可覆寫前兩者（測試與未來不同 chunk 預算的呼叫端）。

### 3.3 最壞情況為何仍在 timeout 內

鏈上每個 arm 都有上界：fubon 受自身 HTTP client timeout 限制；兩個逐檔 arm 各受 `hybridNarrowArmTimeout`（10 s）限制，
且殘量 > 8 時根本不呼叫；TWSE 全市場 arm 受 `twseRateLimitWaitAllowance` + 重試排程限制。
實測（`TestHybridQuoteChain_SkipsPerSymbolArmOnLargeResidual`）在 primary arm 全滅的最壞情況下，
整個 chunk 在 **20 秒**內結束（該測試的上界斷言）。加上 chunk pause 後，單一 chunk 仍遠低於 60 s。

### 3.4 「來源沒這檔」vs「取得失敗」（#1986 要求 3）

`marketdata.QuoteOutcome`：

| outcome | 語意 | 是否良性 |
|---|---|---|
| `ok` | 有可用 quote | 是 |
| `no_data` | 來源權威地回答「該檔當日無可交易資料」（暫停交易、當日無成交） | 是 |
| `not_covered` | 該檔不在**任何成功取得**的全市場表的發佈範圍內 | 是 |
| `error` | 取得失敗（傳輸、逾時、rate limit、配額、breaker） | 否 |
| `not_attempted` | 沒有任何 arm 問過它 | 否 |

`UniverseBuildResult` 新增可稽核欄位（寫入 `universe_snapshot.json`）：

```
quotes_missing_no_data, quotes_missing_not_covered, quotes_missing_fetch_error, quotes_missing_not_attempted
```

不變式：`quotes_returned + 上面四者 == quotes_requested`（測試斷言）。

**信任閘門**：`ranked_trustworthy = false` 若且唯若 `chunks_failed > 0` 或 `fetch_error + not_attempted > 0`。
`no_data` / `not_covered` **不會**讓排名不可信——那兩者是市場或來源範圍的事實，不是量測失敗。
風險由既有的母體覆蓋告警（#1985）承擔，`quotes_missing_*` 讓比率可被稽核。

**保守預設**：不能分類的 provider（只實作 `GetQuotes`）其缺檔一律記為 `error`，
因此沒有實作 `PartialBatchProvider` 的 provider 不會意外把缺口當成市場事實。

### 3.5 全市場覆蓋層（`CoverageCompletingProvider`）

以 `marketdata.CoverageCompletingProvider` 包住 `GatewayBackedProvider`，只在**母體管線**使用
（`cmd/atlas/bootstrap_helpers.go newUniverseQuoteProvider`）。它把殘量交給第一方全市場表：

* `TPExDailyCloseClient`（`tpex_mainboard_daily_close_quotes`，1 次 request 取得整個上櫃市場）。
  沿用 TWSE/TPEx 共用的 token bucket 與自己的 circuit breaker，走 `apiclient` 既有的 retry 排程；
  快取在覆蓋層（`marketWideCacheTTL = 5 分鐘`），因此 32 個 chunk 只下載一次。
* 全部來源都成功回答且都沒有該檔 ⇒ `not_covered`；有任一來源失敗 ⇒ 把 `not_covered` **降級為 `error`**
  （「不在任何覆蓋場域」只有在每個場域都回答時才是事實）。

**為什麼是包裹層而不是 `HybridProvider` 的新 arm**：`HybridProvider` 同時在 live trading 路徑上，
把盤後的上櫃收盤表接到那條鏈，會讓 live trading 對「盤中抓不到」的 symbol 拿到前一日收盤價。
母體管線是日終排名工作，才是正確的消費者。

**實測覆蓋率（1,599 檔母體，2026-09-24 資料）**：

| 來源 | 覆蓋 |
|---|---|
| TWSE `STOCK_DAY_ALL` | 904 |
| TPEx 每日收盤行情 | 689 |
| 合計 | **1,593 / 1,599（99.6 %）** |
| 兩者皆無 | 6 |

## 4. 配額與 rate limiter 紀律（#1986 要求 4）

* 不繞過 `GatewayBackedProvider` 的 50 req/s + burst 10。
* FinMind 逐檔 arm：殘量 ≤ 8 才會被呼叫 ⇒ 單一 chunk 最多 8 次；配額護欄（#1954 的 1,500 保留值）不動。
* Fugle 逐檔 arm：同上，且沿用 `GetSharedFugleClient`（單一 rate/quota/breaker）。
* TPEx 與 TWSE 共用同一個 token bucket（同一營運單位、同一份 OpenAPI 速率政策）。
* TPEx 表在覆蓋層快取 5 分鐘 ⇒ 一次母體建構最多下載 1 次（~4 MB，gzip 後 ~1.6 秒）。

## 5. 驗收證據

見 PR 說明：1,599 檔母體（第一方 `symbol_industry` 實際清單，`cmd/atlas/testdata/universe_scale_symbols.txt`）
的真實 `quotes_*` 輸出、缺報價原因分類，以及休市日／交易日的語意說明。

## 6. 未解 / 已知限制

1. **TPEx 表的列被截斷時**（`TWSEClient` 會濾掉欄位不足的列）該檔會被歸為 `not_covered` 而不是 `no_data`。
   客戶端的截斷列計數是另一個議題。
2. **Fugle 逐檔 arm 沒有 `no_data` 分類**：只有 FinMind 與全市場表能宣告 `no_data`；
   Fugle 的缺檔一律記為 `error`（保守）。
3. **`not_covered` 的比例沒有專屬告警**：目前由 #1985 的覆蓋告警承擔；`quotes_missing_not_covered`
   是給比率型規則的輸入，尚未接線。
4. **`not_covered` 不會被細化為 `no_data`**：primary 鏈已宣告 `not_covered` 時，覆蓋層即使在表上找到
   該檔（但無可用價格）也不會覆寫成 `no_data`。兩者皆良性，閘門不受影響；拆分僅影響稽核粒度。
