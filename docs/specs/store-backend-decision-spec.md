# 儲存後端決策規格（store backend decision）— #2107

> **文件角色**：atlas-go 所有 CLI／背景 job 決定「資料寫去哪個儲存後端」的**單一規則來源**。
> 適用範圍：`cmd/**` CLI、`internal/**` 中會自行開 store 的元件（calibration、channelsecrets、symbolindustry…）。
> 相關：`docs/reference/traps.md`（Storage backend 列）、`docs/specs/futures-bars-firstparty-spec.md` §5、
> `docs/decisions/2026-08-23-performance-report-ssot.md`（**PG 為單一真相的裁決**，2026-08-23）。
>
> ⚠️ **引用更正（2026-10-12）**：本行原引用 `docs/reference/constitution.md` 當「Postgres-first」的依據，
> 但該檔案對 `postgres`／`sqlite` **0 命中**（實測）—— 那是一條**不存在的條文**。真實依據是上面那份
> 2026-08-23 裁決（其 §20-22 已把「多後端 schema 與語意不統一」列為共同根因）。

## 1. 事故形狀（為什麼有這份規格）

生產（`docker-compose.yml` 設 `ATLAS_STORE_BACKEND=postgres`）下，有多處程式碼仍會
**開 sqlite 並寫檔**，症狀是 `data/state/atlas.db` 被刪除後又被重建（2026-09-29 14:49
實測重建為 4 KB、只有 schema）⇒ 同一份資料分裂成兩處，查詢層看到空表，
writer 卻以為寫成功。

根因不是「有人寫錯一行」，而是**預設值本身就是錯的**：

| 形狀 | 問題 |
|---|---|
| `-db` 預設 `data/state/atlas.db`、Postgres 要另外給 `-pg` | 沒帶 flag ⇒ 寫 sqlite，即使環境已宣告 postgres |
| `ledger.OpenSQLiteDB(path)` 是 **opens-or-creates**（且帶 WAL pragma） | 只要被呼叫就會建檔，即使只是想讀 |
| `if backend == "postgres" && pool != nil { ... }` 之後 fallthrough 到 sqlite | pool 為 nil（DSN 缺失／`db.Init` 失敗／順序錯）⇒ 靜默降級 |

## 2. 硬性規則

1. **後端決策一律跟隨宣告**：讀 `ATLAS_STORE_BACKEND`（`config.Config.StoreBackend`），
   經 `ledger.ResolveStoreBackend()` 正規化。**禁止**「預設 sqlite、postgres 需明示 flag」的形狀。
2. **顯式 flag 可覆寫**宣告：`-pg`（postgres）、`-db <path>`（sqlite）；`cmd/backfill-outcome-period`
   另有 `-jsonl <dir>`（改寫 JSONL outcome 檔）。顯式 flag 之間互斥（同時給 ⇒ error，不得靜默擇一）。
3. **postgres 後端自行開池**：`atlasdb.Init(ctx, dsn, <workdir>/sql/migrations)`（ping ＋ 套用 migrations）
   → 需要注入時 `ledger.SetPostgresPool(pool)` → **再**建 store。DSN 來源：`-pg-dsn` 優先，其次 `$DATABASE_URL`。
4. **無 DSN ⇒ 明確錯誤、不降級**：不退回 sqlite／jsonl；訊息必須同時指出後端名、`-pg-dsn` 與 `DATABASE_URL`
   （原事故的 `requires SetPostgresPool` 無法診斷，是本規格明文禁止的錯誤訊息形狀）。
5. **宣告的後端沒有實作 ⇒ 明確錯誤**：`HistoricalStore` 只有 sqlite／postgres 實作，
   宣告 `jsonl` 時必須報錯並提示逃生門（`-db`／`-pg`／`-jsonl`），不得靜默改寫 sqlite。
6. **唯讀用途不得使用 opens-or-creates 的開啟方式**：只讀 sqlite 必須先 `os.Stat` 再用
   `mode=ro` 開啟（慣例：`internal/stocktools/win_rate.go`、`cmd/atlas-mcp/server/tools_stock_winrate.go`）。
7. **未宣告（`ATLAS_STORE_BACKEND` 未設）的語意**：`config.Load()` 的預設是 `jsonl`，
   因此規則 5 會在「沒設環境變數又要寫關聯表」時直接報錯 —— 這是刻意的（fail loud，不是靜默選一個後端）。

## 3. A 類：四支 backfill CLI 的決策表（#2107 落地）

| CLI | 決策函式 | 顯式覆寫 | 宣告 postgres 的行為 |
|---|---|---|---|
| `cmd/backfill-period-history` | `backendFor(cfg, appCfg)` | `-pg` ／ `-db` | `openSink` 開池 ＋ `SetPostgresPool` ＋ `ledger.NewHistoricalStore` |
| `cmd/backfill-period-history-range` | `backendFor` | `-pg` ／ `-db` | 同上 |
| `cmd/backfill-event-calendar` | `backendFor` | `-pg` ／ `-db` | 同上 |
| `cmd/backfill-outcome-period` | `resolveMode` | `-pg` ／ `-db` ／ `-jsonl` | `runPostgres` 開池後直接以 pool 跑 SQL；`-jsonl` 模式的 period_history 來源改走 PG（`periodHistoryStore`） |

共同契約：

* 決策順序：**顯式 flag > 宣告**；未知宣告值 ⇒ error（`ResolveStoreBackend` 既有行為）。
* `-pg` 與 `-db` 同時給 ⇒ error（`checkBackendFlags` / `checkModeFlags`）。
* 額外 seam：`var initPostgresPool = atlasdb.Init`，供單元測試在不連 PostgreSQL 的情況下
  驗證「開池 → 注入 → 建 store」的順序（與 `cmd/backfill-futures-bars` 的 `storeDeps` 等價）。

## 4. 釘子測試（強制）

每個 CLI 都必須有「宣告 postgres ⇒ 不得落到 sqlite」的釘子，且必須**附反假陽性對照**
（同一個環境下 sqlite 後端必須真的可用），否則「紅燈」可能只是環境壞掉：

| CLI | 釘子測試（無 build tag，`go test ./...` 即紅） |
|---|---|
| period-history | `TestOpenSink_DeclaredPostgresWithoutDSNFailsLoudly` ＋ `TestOpenSink_ExplicitDBOverridesDeclaredPostgres`（對照） |
| period-history-range | `TestOpenSink_DeclaredPostgresWithoutDSNFailsLoudly` ＋ `TestOpenSink_ExplicitDBOverridesDeclaredPostgres` |
| event-calendar | 同上 |
| outcome-period | `TestRun_DeclaredPostgresWithoutDSNFailsLoudly` ＋ `TestRun_ExplicitDBOverridesDeclaredPostgres` |

另需：

* `TestOpenSink_PostgresOpensAndInjectsPool`（或 `resolveMode` 的決策表測試）：池必須真的被開、被注入。
* `TestOpenSink_DeclaredJSONLFailsLoudly`：宣告 jsonl ⇒ 明確錯誤且不建 sqlite artifact。
* `//go:build integration` 端到端：真 PostgreSQL 寫入驗證（`main_integration_test.go`）。

**突變紅燈**（驗收證據）：把決策函式改回「硬編 sqlite」後，上表的釘子必須全部 FAIL；
原始輸出需附在 PR 的 Verification 段。

## 5. C 類：其他自行開 store 的元件（#2107 稽核處置）

| 檔案 | 資料 | 判定 | 處置 |
|---|---|---|---|
| `internal/calibration/predictor_calibrator.go` | `prediction_backtest`（7 日校準） | **應 backend-aware 的資料 store**（reader 曾用 opens-or-creates） | 本 PR：改為 `mode=ro` 唯讀開既有檔、**絕不建檔**（`openSQLiteReadOnly`）。**未修**：reader 仍讀 job-local sqlite，而 writer 已在 PG ⇒ 生產恆為 no-op；改讀 `HistoricalStore` 會改變生產校準結果，需另行核准（見 PR 說明）。 |
| `internal/subscription/store.go` | `users` / `subscription_events`（`users.db`） | 應 backend-aware，但**套件內零 postgres 實作**、`Store` 是具體 struct | **不在本 PR 修**（非最小修法）。#2109 另案：`users.db` 建在容器可寫層。 |
| `internal/stocktools/win_rate.go` | `stock_win_rate`（讀） | 同上（HTTP 讀取端） | **2026-10-13（C6b）改為 PG 優先**：四個端點（`/api/stock/win_rate`、`/api/stock/condition_winrate`、`/api/stock/industry_winrate`、`/api/stock/family-expectancy`）在宣告 `postgres` 且 `DATABASE_URL` 有值時讀 PG（`default_transaction_read_only=on`、`stdlib.OpenDB`、惰性連線、PG 可達即權威且 PG 空 ≠ 故障），PG 不可用才回退這條 SQLite 讀取路徑（`mode=ro`、不建檔）；兩條路徑執行同一份 `$N` SQL（`internal/stocktools/win_rate_backend.go`）。**原本列為後續項的**「SQLite **寫入**端仍由 stockpicker job 落地」**已由 C7（同一天）關閉**（見下表 `internal/stockpicker/daily_update.go`）。 |
| `cmd/atlas-mcp/server/tools_stock_winrate.go` | 同上（MCP 讀取端） | 同上（唯讀、`Stat` 後才開） | **2026-10-13（C6）改為 PG 優先**：`DATABASE_URL` 有值且宣告 `postgres` ⇒ 兩個 win-rate tool（`stock_get_win_rate`、`stock_picker_scan`）讀 PG（`default_transaction_read_only=on`、`stdlib.OpenDB`、惰性連線），PG 不可用才回退這條 SQLite 讀取路徑；兩條路徑執行同一份 `$N` SQL。**原本列為後續項的**「`internal/stocktools/win_rate.go`（HTTP）仍 SQLite-only ⇒ HTTP 與 MCP 可能不同步」**已由 C6b（同一天）關閉**：四個 HTTP 端點同規則 PG 優先。**原本列為後續項的**「SQLite 寫入端」**已由 C7 關閉**。 |
| `internal/channelsecrets/store.go` | `channel_secrets` | 應 backend-aware 的資料 store；缺陷是**靜默降級**（`postgres && pool != nil` 條件） | 本 PR：pool 為 nil 時改為明確 error（與其他 ledger factory 同形狀）。呼叫端 `initChannelKeyManager` 已是 fail-open（log ＋ 回 nil ⇒ 管理端點 503），不會拖垮啟動。 |
| `internal/symbolindustry/store.go` | `symbol_industry` | **正確範例**（已 backend-aware） | 不動。 |
| `internal/stockpicker/daily_update.go` | `stock_signal_outcomes` / `stock_win_rate` / `stock_win_rate_by_regime`（**寫**） | C6／C6b 只切換了讀取端 ⇒ 這條寫入路徑仍以 `data/state/atlas.db`（SQLite）為主、把 PG 當鏡射（`ATLAS_STOCKPICKER_DUAL_WRITE`）；`atlas.db` 因此仍是每日更新的寫入目標。 | **2026-10-13（C7）改為 PG 主寫入**：`backend=postgres` 且 `DualWrite` 開（或注入 PG handle）時，`SignalOutcomeStore`／`WinRateStore` 直接建在 PG handle 上（聚合的**讀**也走同一 handle），SQLite 改為 opt-in 鏡射 `ATLAS_STOCKPICKER_SQLITE_MIRROR`（**預設 false**；開啟才寫、才會開啟檔案）。鏡射關閉時 `data/state/atlas.db` 既不開啟也不建立。`ATLAS_STOCKPICKER_DUAL_WRITE=false` 保留原語意（kill switch）＝整條 run 退回 SQLite 主寫入、零 PG 寫入；`sqlite`／`jsonl` backend 行為不變。⚠️ **未改（部署前必辦）**：`internal/orchestrator/stockpicker_winrate_executor.go`（`stockpicker-winrate-01`，`configs/agents.json` `enabled:true`）仍以 `stocktools.OpenWinRateDB` 唯讀開 `data/state/atlas.db` —— 該 reader 未遷移前，生產部署須把 `ATLAS_STOCKPICKER_SQLITE_MIRROR=true`，或先完成它的 PG 化。 |

## 6. 已知行為變更（本規格落地時）

1. 四支 CLI 在「未設 `ATLAS_STORE_BACKEND`」的開發機上不再預設寫 sqlite：
   要 sqlite 請顯式 `-db`，要 PG 請 `-pg` 或宣告 `postgres`。這是規則 1＋5 的直接後果。
2. `cmd/backfill-outcome-period` 宣告 `postgres` 時，預設模式從「寫 sqlite」變成「寫 PG」；
   `-jsonl` 模式的 `period_history` 來源也從 sqlite 變成 PG（避免在生產建出 sqlite artifact）。

## 7. 未驗證／不確定

* 「誰重建了生產的 `data/state/atlas.db`」在本規格中未斷言單一來源；本 PR 只關掉
  **程式碼層已知會 opens-or-creates 的呼叫點**，並對生產不執行任何 CLI（護欄）。
* `internal/subscription` 的 backend-aware 化（含 `ATLAS_SUBSCRIPTION_DB_PATH` 這個
  文件有記載、程式碼未讀的 env）需另案處理。
