# AGENTS.md — internal/db

**成熟度**: evolving
**模組職責**: PostgreSQL 連線池初始化與 migration 管理。

---

## 核心型別

| 型別 | 檔案 | 功能 |
|------|------|------|
| `Init` | `db.go` | 建立 pgxpool、Ping 驗證、執行 migration（golang-migrate） |

---

## 本模組特有陷阱

| 陷阱 | 說明 |
|------|------|
| **DATABASE_URL 為必要** | `Init()` 在 `databaseURL` 為空時會讀取環境變數 `DATABASE_URL`，兩者皆空則回傳錯誤。 |
| **Migration 路徑為 file:// 協議** | `runMigrations()` 使用 `file://` + 絕對路徑，傳入相對路徑可能導致找不到 migration 檔案。 |
| **Driver URL 前綴轉換** | `runMigrations()` 將 `postgres://` 替換為 `pgx5://`，供 golang-migrate 識別。 |
| **Migration 失敗會關閉 pool** | 若 `runMigrations()` 回傳錯誤，已建立的 `pgxpool` 會被 `Close()`，不會洩漏連線。 |
| **ErrNoChange 視為成功** | `m.Up()` 回傳 `migrate.ErrNoChange` 時不會回傳錯誤。 |

---

## 測試

- 涵蓋空 DATABASE_URL、無效 env fallback、unreachable host ping 失敗、無效 migration source path、migration-up error 等錯誤路徑（完整連線與 migration 流程需外部 PostgreSQL）。

(End of file - total 28 lines)
