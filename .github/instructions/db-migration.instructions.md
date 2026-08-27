---
applyTo: "sql/migrations/**"
description: "適用於 atlas-go 的雙方言 SQL migration。確保每一組 up/down 同時在 SQLite 測試與 production PostgreSQL 合法，避免保留字與 float 精度陷阱。"
---

# DB Migration 守則

## 適用範圍

編輯 `sql/migrations/*.up.sql` / `*.down.sql` 時套用。本檔案是 `docs/reference/traps.md` §Data/Persistence 的執行層補充。

## 雙方言 DDL 規則

每一組 migration **必須同時合法**於：

1. **SQLite**（本地測試、`go test ./internal/ledger/...`）
2. **PostgreSQL 15 + TimescaleDB**（production、docker compose）

禁止事項：

- ❌ 寫 PG-only 語法（`CREATE EXTENSION`、`gen_random_uuid()` 等）在 migration 主體；extension 啟用應在 docker entrypoint / bootstrap 腳本處理。
- ❌ 寫 SQLite-only pragma / `AUTOINCREMENT` / `IF NOT EXISTS` 以外的方言語法。
- ✅ `CREATE TABLE IF NOT EXISTS`、標準 SQL-92 DDL、兩者皆懂的型別（`TEXT`、`INTEGER`、`DOUBLE PRECISION`、`TIMESTAMPTZ`）是安全的。
- ✅ 需要 PG 專屬功能時，請在 migration 頂端註解說明，並確認 SQLite 測試會跳過或 mock 該功能。

## 保留字清單與處理

PostgreSQL 保留字不得作為未加引號的表名或欄位名。常見陷阱：

| 保留字 | 範例 | 建議 |
|--------|------|------|
| `window` | 原想命名 `window` | 改為 `rolling_window` 等語意名稱 |
| `user` | `user` 表 / `user_id` | 使用 `users`、`operator`、`account_id` |
| `order` | `order` 表 | 使用 `orders`、`trade_id`、`sequence` |
| `select` | `select` 欄位 | 使用 `selection`、`filter`、`chosen` |
| `group` | `group` 欄位 | 使用 `category`、`bucket`、`cluster` |
| `primary` / `foreign` / `references` | 欄位名 | 避免；必要時加雙引號 `"primary"` |

處理方式二選一：

1. **改名（推薦）**：用不衝突的語意名稱，如 000019 `stock_win_rate.rolling_window`。
2. **加雙引號**：`"window"` — 但此後所有 SQL 都必須帶引號且區分大小寫，容易出錯，不推薦大規模使用。

驗證保留字：

```bash
# 在 PostgreSQL 中查詢保留字
SELECT word FROM pg_get_keywords() WHERE catcode = 'R';
```

## DOUBLE PRECISION 規定

任何需要 IEEE 754 double（8-byte）精度的欄位，必須使用 `DOUBLE PRECISION`：

- ✅ `price DOUBLE PRECISION`
- ✅ `score DOUBLE PRECISION`
- ✅ `forward_return DOUBLE PRECISION`
- ❌ `price REAL`（PG 會變成 float4，無聲精度損失）

SQLite 會把 `DOUBLE PRECISION` 接受並存成 8-byte `REAL`，所以兩方言結果一致。參考實例：`000018_stock_signal_outcomes.up.sql`。

## 必跑 PG Migration Gate

PR 在合併前必須通過以下兩項：

1. **Integration tag 測試**：以 `ATLAS_STORE_BACKEND=postgres` 跑通受影響套件的 integration test，確認新 migration 在 PostgreSQL 實際可跑。
2. **check-migration-sql.sh**：執行 migration 雙方言靜態檢查（保留字、非法型別、PG-only 語法、SQLite-only 語法）。

```bash
# 本地預檢範例
ATLAS_STORE_BACKEND=postgres go test ./internal/ledger/... -tags=integration -run TestIntegration
./scripts/ci/check-migration-sql.sh
```

> `check-migration-sql.sh` 為 CI gate；若本地不存在，請先確認 CI workflow 已包含 `sql/migrations` 路徑的 lint。

## 參考文件

- `docs/reference/traps.md` §Data/Persistence：PostgreSQL 保留字、REAL vs DOUBLE PRECISION、雙方言 DDL 詳細說明
- `internal/ledger/AGENTS.md`：三 backend 樞紐與 `SetPostgresPool` wiring
- `docs/data-catalog.md`：Database 章節與 migration 路徑
