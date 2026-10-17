# PG ↔ SQLite schema parity 規格（SSOT）

> **Created**: 2026-10-16（FU-20261012-07 / FU-20261012-09 收斂）
> **守門**：`scripts/ci/check_schema_parity.sh` → `internal/ledger/schema_parity_test.go`
> **契約測試**：`tests/scripts/test-check-schema-parity.sh`
> **allowlist**：`scripts/ci/schema-parity-allowlist.json`
> **相關**：[`store-backend-decision-spec.md`](store-backend-decision-spec.md)（後端決策）、
> [`repository-dual-write-spec.md`](repository-dual-write-spec.md)（雙寫）

## 1. 問題（2026-10-12 生產 503 的類別）

同一張表有**兩份 schema 定義**：

| 後端 | 定義位置 |
|------|----------|
| PostgreSQL | `sql/migrations/*.up.sql`（`CREATE TABLE` ＋ 之後的 `ALTER TABLE … ADD COLUMN`）|
| SQLite | `internal/ledger/sqlite_core.go` 的 `CREATE TABLE IF NOT EXISTS` ＋ `additiveMigrations` 清單；少數欄位由各 store 首次使用時 `ALTER TABLE … ADD COLUMN` 補（`summary_json`）|

`CREATE TABLE IF NOT EXISTS` **不會**替既有的資料庫補欄位。因此「新增一個欄位」實際要動 **三** 個
地方（PG migration／SQLite `CREATE TABLE`／SQLite `additiveMigrations`），任何一處漏掉都不會在
單元測試或啟動時報錯，只會在**讀該欄位的第一個請求**上炸：

- 2026-10-12：`#2213` 補了 PG 的 `stock_win_rate.break_even_cost_rate`，SQLite 檔案沒有 ⇒
  `/api/stock/win_rate` 對每個請求回 HTTP 503（`no such column`），由 `#2218` 修。
- 同一類別的另一半是**兩個後端各用不同欄名描述同一件事**（PG `time` / SQLite `timestamp`、
  PG `target_symbol` / SQLite `symbol`、PG `criterion` / SQLite `reason`），使「用 PG 形狀寫的
  查詢一旦改走 SQLite（或反之）」必然失敗。

## 2. 量測結果（權威，2026-10-16，lane `fix/20261016-schema-parity` 基準 `844f590f2`）

守門輸出（`bash scripts/ci/check_schema_parity.sh`）：

```
schema-parity: 23 shared tables, 18 PostgreSQL-only tables (allowlisted 18), 1 SQLite-only tables (allowlisted 1)
schema-parity: shared table human_interventions — 16 PostgreSQL columns, 16 SQLite columns
schema-parity: shared table screening_rejects   — 14 PostgreSQL columns, 14 SQLite columns
schema-parity: shared table session_summaries   — 24 PostgreSQL columns, 25 SQLite columns
```

### 2.1 修復前（FU-20261012-09 的「44 個欄位不一致」→ 實測 **47**）

| 表 | PG-only 欄 | SQLite-only 欄 | 小計 |
|----|-----------|----------------|------|
| `human_interventions` | 10（`time`／`intervention_id`／`type`／`target_agent_id`／`target_model_id`／`target_sector`／`target_symbol`／`value`／`reason`／`operator`） | 4（`symbol`／`agent_id`／`action`／`timestamp`） | 14 |
| `screening_rejects` | 8（`time`／`agent_id`／`skill`／`criterion`／`criterion_label`／`threshold`／`actual_value`／`factor_scores`） | 3（`reason`／`timestamp`／`factor_scores_json`） | 11 |
| `session_summaries` | 18（`time`／`regime`／`order_count`／`position_count`／`ending_cash`／`portfolio_value`／`outcome_count`／`broker_runtime`／`next_experiment_agent_id`／`proposal_id`／`commit_id`／`approval_id`／`guard_outcomes`／`tax_snapshots`／`after_tax_pnl`／`total_tax_paid`／`parameters_version`／`risk_commentary`） | 4（`total_recs`／`passed_guards`／`rejected`／`timestamp`） | 22 |
| **合計** | **36** | **11** | **47** |

**為什麼是 47 而不是 F-item 的 44**：2026-10-12 版守門的 `ALTER TABLE` 解析器每個敘述只抓
**第一個** `ADD COLUMN`。`sql/migrations/000007_add_session_summary_tax_params.up.sql` 是

```sql
ALTER TABLE session_summaries
    ADD COLUMN IF NOT EXISTS tax_snapshots JSONB DEFAULT '[]',
    ADD COLUMN IF NOT EXISTS after_tax_pnl DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS total_tax_paid DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS parameters_version TEXT;
```

⇒ 只看到 `tax_snapshots`，於是 `after_tax_pnl`／`total_tax_paid`／`parameters_version` 三個
**PG 有、SQLite 沒有**的欄位從未進入比對（少 3 欄）。舊工具是把「有差異的表」整張表登錄
進 allowlist，所以連這 3 欄的存在都不會有人發現。

### 2.2 修復後

| 表 | 修復前（PG／SQLite 欄數） | 修復後 |
|----|--------------------------|--------|
| `human_interventions` | 12 ／ 6 | **16 ／ 16** |
| `screening_rejects` | 11 ／ 6 | **14 ／ 14** |
| `session_summaries` | 20 ／ 6 | **24 ／ 25**（差 1 欄見 §2.3） |

⇒ 欄位集合差異 **0**（除 allowlist 的 1 欄），型別差異 **0**。

### 2.3 剩下刻意登錄的差異（`scripts/ci/schema-parity-allowlist.json`）

| 類別 | 條目 | 理由（摘要） |
|------|------|--------------|
| `pg_only_tables` | 18 張（`alerts`／`metrics`／`users`／`workspaces`／`task_*`／`otel_traces`…） | 其他子系統的 store 專用表，SQLite ledger 從未實作，無雙寫路徑 |
| `sqlite_only_tables` | 1 張（`outcomes`） | SQLite ledger 的舊 outcome 表；PG 對應表是 `stock_signal_outcomes`／`recommendation_outcomes`（不同 schema、不同寫入端） |
| `column_differences` | 1 欄（`session_summaries.summary_json`） | SQLite 各 store 首次使用時延遲 ALTER 加的整包 summary JSON；PG 用 `broker_runtime`／`guard_outcomes`／`tax_snapshots` 存同一份內容 |
| `type_differences` | 0 | — |
| `nullability_differences` | 9 欄 | 7 欄是本次補的 PG 欄在 SQLite 只能可為 NULL（`ALTER TABLE` 無法在沒有 DEFAULT 時加 `NOT NULL`，且既有資料列沒有值）；2 欄是 SQLite 舊 DDL 比 PG 嚴格（`human_interventions.session_id`／`symbol`） |

allowlist **不許腐爛**：某筆差異不再出現（已修好或敘述過期）⇒ 守門紅燈（`STALE`）。修好就刪掉該筆。

## 3. 守門（gate）

`scripts/ci/check_schema_parity.sh`（`make ci-gate` ＋ GitHub Actions `governance` job）跑
`TestSchemaParity_PGMigrationsVsSQLite`，逐表比對三件事：

1. **欄位集合**（兩側雙向）；
2. **型別家族**——含 SQLite 無法表達的跨後端等價（`uuid↔INTEGER`、`jsonb↔TEXT`、
   `timestamptz⁄date↔TEXT`、`boolean↔INTEGER`、`double precision↔REAL`），清單在
   `typeFamiliesCompatible`；
3. **可空性**（某後端接受另一後端拒絕的資料列）。

SQLite 側不是解析 Go 文字，而是**實測**：`InitSchema` 跑在 in-memory DB 上，再以
`PRAGMA table_info` 取欄位；此外掃描套件內 store 的 `ALTER TABLE … ADD COLUMN` 字面值
（`summary_json` 這類延遲補欄），否則模型會少掉真實資料庫有的欄。

退出碼：`0` 一致；`1` 有差異或 allowlist stale；`2` 執行錯誤（go 不在、migration／allowlist
檔不存在、0 份 migration）——**fail loud，不當成通過**。

環境變數（契約測試用來餵 fixture，正式路徑不用設）：

| 變數 | 預設 | 用途 |
|------|------|------|
| `ATLAS_SCHEMA_PARITY_MIGRATIONS_DIR` | `sql/migrations` | 替代的 migration 目錄 |
| `ATLAS_SCHEMA_PARITY_SQLITE_DDL` | （未設 ⇒ 用 `InitSchema`） | 以檔案中的 DDL 建 SQLite schema |
| `ATLAS_SCHEMA_PARITY_ALLOWLIST` | `scripts/ci/schema-parity-allowlist.json` | 替代的 allowlist |

## 4. 新增一個欄位時要做的事（不容許只改一邊）

1. **PG**：`sql/migrations/0000NN_*.up.sql` 加 `ALTER TABLE … ADD COLUMN IF NOT EXISTS`（多欄請
   每個 `ADD COLUMN` 各自一行，勿依賴解析器能讀懂縮寫）。
2. **SQLite（新資料庫）**：`internal/ledger/sqlite_core.go` 的 `CREATE TABLE IF NOT EXISTS` 定義。
3. **SQLite（既有資料庫）**：`additiveMigrations` 加一個 `addColumnIfMissing` 呼叫
   （`addSchemaParityColumns` 是這批的範例）。**少了這一步就是 2026-10-12 的 P0。**
4. `make ci-gate`（或 `bash scripts/ci/check_schema_parity.sh`）必須綠。
5. 若刻意不一致：登錄 `scripts/ci/schema-parity-allowlist.json` 並寫理由。

## 5. 誠實邊界（已知限制）

- **schema 一致 ≠ 語意一致**。收斂後兩個後端仍有各自的**欄名世代**（PG 寫 `time`／`criterion`，
  SQLite 寫 `timestamp`／`reason`），各寫各的子集，另一側的欄位留 NULL。要真正單一形狀必須等
  「單一 PG」收斂（Q2 C1→C7），本規格不假裝已完成。
- 守門只看 `internal/ledger` 的 SQLite schema。`internal/symbolindustry`、`internal/channelsecrets`、
  `internal/subscription` 也在自己的 SQLite 檔內建了與 PG 同名的表，屬同一類別但未納入本守門。
- SQLite 端的「延遲 ALTER」仍是 schema 的第三個定義點（`summary_json`）；把它搬進 `InitSchema`
  可少一個定義點，屬後續工作（會改動三處 store 的啟動行為）。
- 本守門不驗證**資料**層（欄位有值、型別正確），也不驗證 PG 端 migration 是否已套用到生產
  （migration runner 另行負責）。本次沒有需要回填的資料：新增欄位全為 nullable。
