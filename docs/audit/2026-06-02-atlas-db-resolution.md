# atlas.db Resolution — P2.1

**日期**: 2026-06-02  
**決策**: 移除（Remove with documentation）

---

## 現狀

| 項目 | 值 |
|------|-----|
| 檔案路徑 | `data/state/atlas.db` |
| 類型 | SQLite 3.x |
| 大小 | 172KB |
| Tables | `outcomes`, `screening_rejects`, `experiments`, `session_summaries`, `human_interventions`, `quotes` |
| 總行數 | 52 rows（6 tables 合計） |
| Git 追蹤 | ❌（data/state/ gitignored） |

## Schema（完整）

```sql
CREATE TABLE outcomes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    symbol TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    action TEXT, weight REAL,
    target_price REAL, stop_loss REAL,
    conviction REAL, regime TEXT,
    timestamp TEXT, passed_guards INTEGER,
    guard_reason TEXT,
    factor_scores_json TEXT,
    conviction_breakdown_json TEXT
);

CREATE TABLE screening_rejects (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL, symbol TEXT NOT NULL,
    reason TEXT, timestamp TEXT,
    factor_scores_json TEXT
);

CREATE TABLE experiments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    experiment_id TEXT UNIQUE NOT NULL,
    session_id TEXT,
    mutation_brief_json TEXT, result_json TEXT,
    accepted INTEGER, timestamp TEXT
);

CREATE TABLE session_summaries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT UNIQUE NOT NULL,
    total_recs INTEGER, passed_guards INTEGER,
    timestamp TEXT
);

CREATE TABLE human_interventions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT, symbol TEXT, agent_id TEXT,
    action TEXT, reason TEXT, timestamp TEXT
);

CREATE TABLE quotes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    symbol TEXT NOT NULL, date TEXT NOT NULL,
    open REAL, high REAL, low REAL, close REAL,
    volume INTEGER
);
```

## 程式碼參考

1. **`cmd/migrate-jsonl-to-sqlite/main.go`**（232 lines）
   - CLI 工具，從 JSONL 目錄遷移至 SQLite
   - **預設來源目錄**: `data/ledger` — **此目錄不存在**
   - **預設目標**: `data/state/atlas.db`
   - 僅在 `-dry-run=false` 時寫入
   - 使用 `ledger.OpenSQLiteDB()` 和 `ledger.InitSchema()`

2. **`internal/config/config.go`**
   - `AtlasDBPath` 欄位，預設值指向 `data/state/atlas.db`
   - 無主動讀取邏輯（僅儲存路徑參考）

3. **`internal/ledger/sqlite.go`** (推定存在)
   - `OpenSQLiteDB()`, `InitSchema()`, `SQLiteOutcomeStore` — 由 migrate 工具使用

## PostgreSQL 重疊分析

| SQLite Table | PostgreSQL Table | 重疊度 | 備註 |
|-------------|-----------------|--------|------|
| `outcomes` | `recommendation_outcomes` (hypertable) | 100% | PG 有 TimescaleDB compression + retention |
| `screening_rejects` | `screening_rejects` | 100% | PG 有 migration 管理 |
| `experiments` | — | 部分 | PG 無直接對應表 |
| `session_summaries` | `session_summaries` | 100% | |
| `human_interventions` | `human_interventions` | 100% | |
| `quotes` | — | 無 | PG 無 quotes 表 |

## 決策：移除

**理由**：

1. **功能重疊**: 6 個表中 4 個與 PostgreSQL 完全重疊，且 PG 版本有更好的查詢性能、TimescaleDB compression、retention policy
2. **資料量極小**: 僅 52 行，無實用價值
3. **遷移工具失效**: `cmd/migrate-jsonl-to-sqlite` 的預設來源目錄 `data/ledger` 已不存在，工具無法正常運作
4. **未被主動讀取**: 僅 `config.go` 儲存路徑，無任何執行期讀取邏輯
5. **gitignored**: 不影響版本控制歷史

**執行**（P2.2 合併處理）：

```
1. 刪除 data/state/atlas.db（本地檔案，gitignored）
2. 移除 cmd/migrate-jsonl-to-sqlite/（死碼）
3. 移除 internal/config/config.go 中的 AtlasDBPath（或標記 deprecated）
4. 移除 internal/ledger/sqlite.go 中的相關程式碼（若有）
5. 更新 docs/data-catalog.md — 標記為已移除
6. 更新 docs/data-architecture.md — 移除 atlas.db 層級
```

**不執行原因**（若選擇保留）：
- 無。沒有保留的合理理由。

---

## 相關文件

- `docs/data-catalog.md` — 資料目錄
- `docs/data-architecture.md` §層級 18 — 架構文件
