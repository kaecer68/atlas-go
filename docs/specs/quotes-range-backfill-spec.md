# quotes 日線 range 回補規格（`backfill-quotes-range`）

> **狀態**：2026-09-29 實作（#2126 後續）
> **對應缺口**：`quotes` 表在 **2026-06-25 之前只有 114 檔**（FinMind 全市場回填自該日起）；
> 這是**所有 per-stock 研究的標籤地基**問題，不只是 SBL 的問題。
> **相關**：`docs/specs/store-backend-decision-spec.md`（後端決策 SSOT）、
> `docs/specs/futures-bars-firstparty-spec.md`（同形狀的 range 回補 CLI 範本）。

## 1. 為什麼不能用逐日（per-day）回補

`internal/marketdata` 原本只有逐日入口：

```go
GetStockPrice(ctx, symbol, date) // → fetchDataset("TaiwanStockPrice", symbol, date, date)
```

TaiwanStockPrice **支援日期區間**（一次請求回整段），所以逐日形式的成本是：

| 形式 | 呼叫次數 | 免費層（600/hr ≈ 6s/req） | 付費層（約 1.8s/req） |
|------|----------|---------------------------|------------------------|
| per-day：`GetStockPrice` | 檔數 × 交易日數（1,600 × 80 ≈ **128,000**） | **≈ 64 小時**（不可行） | ≈ 64 小時（不可行） |
| **range：`GetStockPriceRange`** | **檔數 × 1**（≈ **1,600**） | **≈ 2.7 小時** | **≈ 48 分** |

⇒ 唯一可行路徑是 range 形式。這與 `twse_sbl_provider.go` / `tdcc_provider.go` 早已使用
`FetchDatasetRaw(ctx, dataset, dataID, start, end)` 的做法一致。

## 2. 新增 API：`(*FinMindClient).GetStockPriceRange`

```go
func (c *FinMindClient) GetStockPriceRange(ctx context.Context, symbol, startDate, endDate string) ([]domain.DailyBar, error)
```

實作在 `internal/marketdata/finmind_stock_price_range.go`，內部只呼叫一次
`FetchDatasetRaw(ctx, "TaiwanStockPrice", symbol, startDate, endDate)`。

契約（逐條都有測試釘住）：

| 條款 | 內容 | 為什麼 |
|------|------|--------|
| **一次呼叫覆蓋整段** | [start, end] 只發一次 HTTP | 成本形狀（檔數 × 1）；突變回逐日 ⇒ `TestGetStockPriceRange_SingleRequestCarriesTheWholeWindow` 紅（實測 115 次呼叫） |
| **兩層日期界** | ①請求帶 `start_date`/`end_date`；②回傳列在客戶端再過濾一次，窗口外的列一律丟棄 | 本平台已實證 FinMind 有 dataset 會忽略窗口（`twse_sbl_provider.go`：full-market query 只回 START 那天的列）。寫入端寧可少寫，不可把窗口外資料寫進 `quotes` |
| **逐檔比對 `stock_id`** | 列上的 `stock_id`（正規化後）與請求代碼不符 ⇒ 丟棄 | 上游若忽略 `data_id`，別的股票的價格不會污染這條序列 |
| **同日去重、日期遞增** | 同日重複列取後出現者 | 上游以日期遞增回傳，後者為最新修訂 |
| **`Date` 為 UTC 午夜** | `time.Parse("2006-01-02", date)`（UTC），不是台北午夜 | 見 §4；否則 sqlite 與 postgres 會落在不同天 |
| **`Symbol` 保留呼叫端原字串** | `2330.TW` 就寫 `2330.TW` | quotes 的鍵由呼叫端決定，library 不猜形式 |
| **空結果 = `ErrNoDataForSymbol`** | 整段窗口零列 ⇒ 具型別錯誤（訊息含各類丟棄計數） | 對一段幾十天的窗口而言，空結果不是「今天還沒收盤」而是「代碼在該區間沒有資料」（下市／代碼錯誤），呼叫端據此與傳輸錯誤分流 |

`schema_fingerprint.go` 既有對 `TaiwanStockPrice` 的欄位指紋檢查由 `fetchDataset` 一併繼承
（上游欄位變更會在 warn log 立刻現形）。

## 3. 新增 CLI：`cmd/backfill-quotes-range`

```
backfill-quotes-range \
  -start 2026-03-02 -end 2026-06-24 \
  [-symbols 2330,2317 | 空 = 用 store 內既有標的鍵] \
  [-backend jsonl|sqlite|postgres] [-pg-dsn "$DATABASE_URL"] [-workdir <repo root>] \
  [-dry-run] [-force]
```

### 3.1 後端決策（完全沿用 #2107／#2118，無例外）

1. `-backend` 顯式覆寫，否則跟隨 `ATLAS_STORE_BACKEND`（`ledger.ResolveStoreBackend`，未知值直接錯誤）。
2. 解析結果為 `postgres` ⇒ 取 DSN（`-pg-dsn`，預設 `$DATABASE_URL`）；**沒有 DSN ⇒ 明確錯誤中止**，絕不退回 sqlite。
3. `atlasdb.Init`（ping ＋ 套用 `<workdir>/sql/migrations`）開池 → `ledger.SetPostgresPool` 注入 → 才建 store。
4. 非 postgres 後端完全不碰 DSN（也不開池）。

釘子測試：`TestResolveStore_EnvPostgresNeverFallsBackToSQLite`、
`TestResolveStore_PostgresWithoutDSNFailsWithDiagnosticMessage`（訊息必須提到 `postgres`／`-pg-dsn`／`DATABASE_URL`）、
`TestResolveStore_PostgresWithDSNOpensAndInjectsPool`、
`TestRun_DeclaredPostgresWithoutDSNWritesNoSQLiteFile`（**檔案級**證據：本機 sqlite 檔不得被建出來）。
反假陽性對照：`TestResolveStore_ExplicitBackendWins`（同一組 wiring 下合法的 sqlite 設定必須成功）。

### 3.2 資料紀律

| 規則 | 行為 |
|------|------|
| **只補缺少的交易日** | 先算「窗口內交易日」（`marketdata.IsTaiwanTradingDay`），扣掉 store 已有的日期；**沒有缺日就不呼叫上游**（重跑自動續傳，配額中止後可直接重跑） |
| **既有列不被覆寫** | 只有 `-force` 才重抓整個窗口並覆寫（`-force` 會以空 `name` 覆寫，這是 upsert 語意） |
| **只寫交易日** | 上游若回了休市日（週末/假日）的列，一律丟棄 —— 休市日的列會污染下游 `ForwardReturn` 的重複偵測（2026-08-23 replay 事故；同一道守門已在 `cmd/backfill-quotes`／`cmd/daily-replay-sync`／`cmd/cron-quote-backfill`） |
| **日期一律 UTC 午夜** | 見 §4 |
| **source 標籤** | 寫入列的 `source = "finmind_quotes_range"`（與逐日路徑的 `finmind_backfill` 區分，供事後稽核） |
| **標的鍵形式** | 省略 `-symbols` ⇒ 沿用 store 內既有鍵（原樣，不轉換）；顯式清單的裸代碼（`2330`）補成 `<code>.TW`（生產 FinMind 路徑的形式） |
| **失敗分類** | 配額用完／IP 被封／breaker open ⇒ **立即中止**（避免把剩餘檔數燒在保證失敗的請求上；2026-09-26 教訓）；`no_data` ⇒ 警告並繼續（1,600 檔裡有一檔下市不該讓整批看起來失敗）；其他錯誤 ⇒ 計數、繼續、最後以非零 exit code 回報 |

### 3.3 輸出（可稽核）

```
backfill-quotes-range: plan symbols=1600 trading_days=80 window=2026-03-02..2026-06-24 backend=postgres source=finmind_quotes_range dry_run=false force=false (range form: ≤1600 upstream calls; per-day form would need ≤128000)
[  1/1600] 2330.TW fetch 2026-03-02..2026-06-24 fetched=80 wrote=80
...
backfill-quotes-range: symbols=1600 requests=1599 skipped_complete=1 rows_fetched=127920 rows_written=127920 skipped_not_wanted=0 no_data=1 failures=0 backend=postgres source=finmind_quotes_range window=2026-03-02..2026-06-24 dry_run=false force=false
```

## 4. 日期語意：為什麼是 UTC 午夜

三個 quotes store 對日期的格式化方式**不一致**：

| store | 寫入 | 區間查詢界 |
|-------|------|------------|
| `PostgresQuoteStore` | `q.Date.UTC().Format("2006-01-02")` | `start.UTC()` / `end.UTC()` |
| `SQLiteQuoteStore` | `q.Date.Format("2006-01-02")`（時間自帶時區） | `start.Format(...)` |
| `JSONLQuoteStore` | `q.Date.UTC().Format(...)`（去重鍵） | — |

⇒ 餵「台北午夜」的 `time.Time` 時，sqlite 寫 `2026-03-02`、postgres 寫 `2026-03-01`
（台北午夜 = 前一日 16:00Z）。因此本 CLI 的 `Date` 與 `-start`／`-end` 一律是
**交易所日曆日、以 UTC 午夜表示**：

- `time.Parse("2006-01-02", s)`（**不是** `ParseInLocation(..., TaiwanLocation())`）。
- 預設迄日：先取 `time.Now().In(marketdata.TaiwanLocation())` 的日曆日再轉 UTC 午夜
  （容器 TZ-unset 時 UTC 的「今天」在台北 00:00–08:00 之間會少一天）。

相關跨模組陷阱已登記於 `docs/reference/traps.md` §Data / Persistence。

## 5. 執行 playbook（運維／a2a-dev）

```bash
# 0) 先估成本（不寫入；仍會呼叫上游）
backfill-quotes-range -start 2026-03-02 -end 2026-06-24 -workdir ~/workspace/atlas -dry-run

# 1) 生產（postgres）：由宣告決定後端，不帶 -backend；-dry-run 先跑一次看 requests 數
ATLAS_STORE_BACKEND=postgres DATABASE_URL="$DATABASE_URL" \
  backfill-quotes-range -start 2026-03-02 -end 2026-06-24 -workdir ~/workspace/atlas

# 2) 中止（配額／SIGINT）後直接重跑同一條指令即可續傳：已完整的標的 requests=0
```

- **節奏**：由 FinMind 客戶端**共用** rate limiter 決定（`FINMIND_RATE_LIMIT_PER_HOUR`；
  未設 = 免費 600/hr）。CLI **不另外加 sleep**（兩層節流會把 2.7 小時變成 5 小時以上）。
- **配額**：共用每日上限見 `marketdata.FinMindDailyLimit()`（狀態檔 `data/state/finmind_daily_quota.json`，
  跨 process flock）。1,600 次呼叫遠低於上限，但若當天已被別的 backfill 用完，本 CLI 會**立即中止**而不是空轉。
- **驗收查詢**（PG）：

```sql
-- 回補窗口內的覆蓋率（每個交易日應接近當時的標的數）
SELECT date, count(*) FROM quotes
WHERE date BETWEEN '2026-03-02' AND '2026-06-24'
GROUP BY date ORDER BY date;

-- 本 CLI 寫入的列（provenance）
SELECT count(*) FROM quotes WHERE source = 'finmind_quotes_range';
```

## 6. 測試清單

| 類型 | 測試 |
|------|------|
| range 抓取解析 | `internal/marketdata/finmind_stock_price_range_test.go`：單次請求與查詢參數、窗口外列、他人列、同日去重、空結果 typed error、上游錯誤不被誤判為 no-data、UTC 午夜（含負控制） |
| CLI 單元 | `cmd/backfill-quotes-range/main_test.go`：只補缺、已完整不呼叫上游、一次呼叫/檔、dry-run 不寫、force 覆寫、配額中止、no_data 不失敗、部分失敗回非零、取消、後端決策五條釘子與反假陽性 |
| PG 端到端 | `cmd/backfill-quotes-range/main_integration_test.go`（`//go:build integration`）：生產形狀（宣告 postgres）走真 flag 解析 → 真 wiring → 真寫入；第二次執行 `requests=0` 且列數不變 |
| 突變紅燈（已實證） | ①拿掉客戶端日期界 ⇒ 窗口外列被寫入（紅）②改回逐日 ⇒ 115 次呼叫（紅）③CLI 送空窗口 ⇒ 呼叫簽章不符（紅）④無 DSN 靜默降級 sqlite ⇒ 三條釘子紅 |

## 7. 邊界與不確定

- **未回補 2026-06-25 之後的既有列**：本 CLI 只處理「指定的窗口」；窗口內已存在的列預設不動
  （`-force` 才會覆寫，且會以空 `name` 覆寫 —— upsert 語意）。
- **台北時間的日曆日 vs UTC 表示**：`IsTaiwanTradingDay` 用傳入時間的 `Weekday()`；UTC 午夜與
  台北日曆日的星期相同，故不受影響。
- **未證明**「Range 查詢一定不會回休市日的列」：本 CLI **不依賴**這個假設，而是自行過濾（見 §3.2）；
  過濾掉的列數會出現在摘要中，若長期非零即代表該假設需要重新檢視。
- **付費窗口**：FinMind 付費層的速率僅影響執行時間，不影響正確性。
- **`name` 欄位**：TaiwanStockPrice 不含股票名稱，本 CLI 寫入的列 `name` 為空字串；
  預設的「不動既有列」語意可避免覆蓋既有名稱。
