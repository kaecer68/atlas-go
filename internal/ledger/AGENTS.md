# internal/ledger/AGENTS.md — Ledger persistence truth

> 本文件說明 `internal/ledger` 的三 backend 樞紐：`jsonl`（legacy 檔案導向）、`sqlite`（本機 dev 關聯式）、`postgres`（production SSoT）。
> 任何修改 `store_factory.go`、`*_store.go`、migration 或新增 store 的行為前，請先讀本文件與 `docs/reference/traps.md` §Data/Persistence。

---

## 1. 三 backend 對照表

| backend | `ATLAS_STORE_BACKEND` | 角色 | 何時使用 | 注意事項 |
|---------|----------------------|------|---------|---------|
| **jsonl** | `jsonl`（預設） | legacy 檔案導向 store | 無 backend 設置時的 fallback；快速本機測試；不依賴 DB | 無索引、無事務、跨 process 不友好；`Store` 以 `baseDir` 為根目錄寫入 `*.jsonl` |
| **sqlite** | `sqlite` | 本機 dev 關聯式 store | 開發/單機測試；驗證關聯式 schema 但不想起 PG | 檔案 = `data/state/atlas.db`（`ATLAS_SQLITE_PATH`）；WAL mode；多 process 共用仍有風險 |
| **postgres** | `postgres` | production SSoT | docker compose / iMac production；多 writer、多 reader | 需先 `SetPostgresPool(*pgxpool.Pool)`，否則 factory 報錯 |

production 部署（`ATLAS_STORE_BACKEND=postgres`）以 PostgreSQL 為權威來源；`data/state/atlas.db` 在 prod 上只是空殼或不存在，禁止把 SQLite 路徑當成固定資料來源。

---

## 2. `SetPostgresPool` wiring 前提

所有 `New*Store(cfg)` factory 在 `StoreBackend == "postgres"` 時，都會檢查 package-level `postgresPool` 是否已注入：

```go
// internal/ledger/store_factory.go
func SetPostgresPool(pool *pgxpool.Pool) { postgresPool = pool }
```

呼叫時序：

1. bootstrap / main 先從 `config.DatabaseURL` 建立 `*pgxpool.Pool`
2. 呼叫 `ledger.SetPostgresPool(pool)`
3. 之後才呼叫 `ledger.NewFullStore(cfg)` / `NewOutcomeStore(cfg)` / `NewQuoteStore(cfg)` 等

若順序顛倒，`NewFullStore` 會回：

```
fullstore: postgres backend requires SetPostgresPool before NewFullStore
```

**不要**在 library 層自行 `pgxpool.New`；pool 生命週期由 bootstrap 統一管理，確保 graceful shutdown 正確關閉。

---

## 3. `PGFirstOutcomeStore` 降級語意

`NewReportOutcomeStore(cfg)` 在 `StoreBackend == "postgres"` 時回傳 `PGFirstOutcomeStore`。

- **讀取路徑**：永遠先讀 PostgreSQL；只有當 PG **出錯** 時才降級到 JSONL ledger，並標記 `degraded=true`。
- **空結果 ≠ 降級**：PG 成功但無資料是權威結果（SSoT 可能尚未有資料），不會 fallback。
- **寫入路徑**：全部交給 PG，不透過 JSONL。
- **報告用途**：`reporting.GenerateReport` 可透過 `Degraded()` / `SourceBackend()` / `FallbackCount()` 在輸出中標註資料來源，避免 silently mix backend。

設計來源：`docs/decisions/2026-08-23-performance-report-ssot.md`。

---

## 4. `DetectorScanStore` / `HistoricalStore` 無 JSONL fallback 的原因

| Store | 原因 |
|-------|------|
| **DetectorScanStore** | MCP `template_detector_status` 需要 `LIMIT + ORDER BY scan_id DESC` 高效查詢掃描歷史。JSONL 無索引也無法快速排序，因此 contract 規定必須使用關聯式表（`detector_scan_log`）。僅支援 `sqlite` 與 `postgres`。 |
| **HistoricalStore** | `history_regime`、`history_stress`、`history_event_calendar` 等工具需要對 `date` 做 range / ORDER BY / 去 synthetic 過濾。這些查詢在 JSONL 上效率與正確性都無法保證，因此僅支援 `sqlite` 與 `postgres`。 |

對這兩個 store，`StoreBackend == "jsonl"` 會直接回 `fmt.Errorf("... backend %q not supported")`。新增 store 時請先評估查詢模式：需要索引 + 排序的就走關聯式 only。

---

## 5. Migration 陷阱

- `sql/migrations/*.up.sql` 同時會被 SQLite 測試與 production PG 執行，必須是**雙方言合法 DDL**。
- PostgreSQL 保留字（`window` / `user` / `order` / `select` 等）必須加引號或改名；實例見 000019 `stock_win_rate.rolling_window`。
- float 語意欄請用 `DOUBLE PRECISION`：SQLite 存成 8-byte double，PostgreSQL 也避免單精度 `REAL` 的精度損失。
- 完整規則與保留字對照見 `docs/reference/traps.md` §Data/Persistence。

---

## 快速檢查清單

- [ ] 修改 factory 時同步更新 `NewFullStore` / `NewOutcomeStore` / `NewSessionStore` / `NewQuoteStore` / `NewDetectorScanStore` / `NewHistoricalStore` 六個入口
- [ ] `StoreBackend == "postgres"` 的 code path 有檢查 `postgresPool == nil`
- [ ] 新增 table 同時新增 `sql/migrations/NNN_*.up.sql` 與 `*.down.sql`，並確認雙方言合法
- [ ] 浮點欄位使用 `DOUBLE PRECISION`，不是 `REAL`
- [ ] 欄位名避開 PG 保留字，無法避開則用雙引號並考慮測試可讀性
