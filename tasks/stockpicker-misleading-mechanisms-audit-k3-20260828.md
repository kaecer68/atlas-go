# Stockpicker 誤導機制審計報告（k3, 2026-08-28）

> 審計範圍：Phase 4 選股層開發（2026-08-27~28）連續踩坑 5+1 件的系統性根因。
> 方法：逐檔盤查 + 本地重現（起 CI 同規格 timescaledb:2.14.2-pg15 容器跑 `-tags=integration`）+ 查閱 CI 失敗 log + ssh 驗證 iMac .env 實際值。
> 註：本文件於 2026-08-28 kernel 中斷後由 root agent 依對話記錄重建存檔（原始檔因未 commit 遺失）。

## 1. 總體結論

**這不是模型能力問題，而是「雙資料庫方言 + 三種 store backend + 兩機四種 DSN」的複雜度沒有任何一道防線（文件、測試、CI gate、環境隔離）接住 —— 每一個坑都對應一個具體的機制缺失，且全部被本審計重現或實錘。**

## 2. 誤導機制清單

| # | 機制 | 判定 | 如何導致踩坑 |
|---|------|------|--------------|
| M1 | `internal/db` 無 PG migration 測試；`db_test.go` 只測 error case | **缺失** | migration 000018/000019 的 PG 適法性無人驗證。`TestMigration_UpDownUp` 用 SQLite in-memory 跑 —— SQLite 接受 `window` 欄位名與 `REAL`，測試全綠給出**虛假信心**，production PG 才爆 42601。|
| M2 | `docs/reference/traps.md` Data/Persistence 表 + AGENTS.md 高頻陷阱表 | **缺失** | 全表 0 條 PG 方言陷阱：無「PG 保留字（window/user/order/…）」、無「REAL=浮點4位精度流失」、無「migration 是雙方言 DDL」。|
| M3 | `internal/ledger/AGENTS.md` 是 Stage 5+ placeholder | **過時** | 該模組已是三 backend + PGFirstOutcomeStore 的核心樞紐，文件卻寫「TODO(Stage 6+)」。agent 讀到 placeholder → 判斷「這模組很簡單」。|
| M4 | StoreBackend 選擇機制（store_factory.go / config.go）| **誤導** | ① config 預設 `jsonl`，switch default 靜默落回 JSONL —— 配錯不報錯。② `run-stockpicker-backtest` 自己另寫 `resolveBackend` heuristic 與 store_factory 分歧。③ 同一支 job 內 quotes 走 backend-aware、outcomes 硬編碼 SQLite。|
| M5 | integration CI job（ci-cd.yml）| **誤導 + 壞掉** | 三個獨立缺陷（見 §2.1）。job 壞到必須 `--admin` 合併，且 `release-check` 的 needs **不含 integration**。|
| M6 | PG 整合測試連線字串三國鼎立 | **誤導** | 三組硬編碼 DSN 並存；本地無匹配 PG 時全部 `t.Skipf` 靜默跳過 —— 本地綠燈 ≠ 測過。|
| M7 | 測試非 hermetic：`config.Load()` 讀 `~/.config/atlas-go/.env` | **誤導** | 已重現：MacBook .env 有 `ATLAS_STORE_BACKEND=postgres`，`go test -tags=integration ./...` 讓 cmd/atlas 10 個測試全炸。|
| M8 | 共享 DB 跨測試污染 | **缺失** | 已重現 `TestPostgresLedgerStore_TradesRoundTrip: expected 2 trades, got 4`；所有整合測試共用一個 DB、cleanup 只 DELETE 部分表。|
| M9 | `docs/data-catalog.md` | **過時** | 寫 PG migration 在 `internal/db/migrations/`（實際 `sql/migrations/`，19 組）。`atlas.db` 條目無「本機 dev artifact」標註。|
| M10 | .env 環境分離文件 vs 實作 | **矛盾** | `docs/operations/local-deploy.md` 明寫「iMac .env 指向 prod DB（atlas）」，**已 ssh 實錘 iMac 實際指向 `atlas_dev`**。|
| M11 | 密碼命名說謊 + dev/prod 同實例 | **誤導** | prod DB `atlas`（port 55432）的密碼叫 `atlas_dev_pwd_2026`；`atlas_dev` DB 與 prod **同一個 postgres 實例**、同一 superuser —— 名義隔離，實際零隔離。|
| M12 | migration 執行無目標守衛 | **缺失** | `run-stockpicker-backtest` 對 source 進來的任何 DATABASE_URL 直接跑 migrate up。無 `SELECT current_database()` 自報、無 `-expect-db` 斷言。|

### 2.1 integration CI 三缺陷（全部本地重現）

1. **非 hermetic（M7）**：匯出 `ATLAS_STORE_BACKEND=postgres` 的 user .env → cmd/atlas 10 個測試 fail。
2. **跨測試污染（M8）**：乾淨 PG 容器上 `internal/ledger` 仍炸 `expected 2 trades, got 4`。
3. **假綠燈（M6）**：本機 5432 無 atlas role → 所有 PG 整合測試 SKIP，輸出 `ok`。

## 3. 修正計劃

### P0（本週，擋下一個 stockpicker PR）

| 項 | 改哪裡 | 內容 | 驗收 |
|----|--------|------|------|
| P0-1 PG migration 驗證 gate | `internal/db/migration_pg_test.go`（`//go:build integration`） | 對 CI postgres service 跑全部 19 組 migration：up→down→up；斷言無 dirty、兩表存在。加 `scripts/ci/check-migration-sql.sh`（保留字/REAL 靜態掃描） | 故意放 `window` 欄位 → CI 紅；修正後 → 綠 |
| P0-2 修 integration job 三缺陷 | `ci-cd.yml` + 各整合測試 | ① `ATLAS_ENV_FILE: /dev/null` + config `testing.Testing()` guard；② 統一 DSN 只讀 DATABASE_URL，CI 未設 fail loudly；③ per-test TRUNCATE | integration job 連 3 次全綠；拔 DATABASE_URL → 紅 |
| P0-3 宣告 Postgres-first | traps.md + AGENTS.md | 加「production = Postgres-first，atlas.db 是本機 dev artifact，禁硬編碼 SQLite 路徑」+ PG 方言三行 | 新 agent 能從 AGENTS.md 找到答案 |
| P0-4 重寫 ledger/AGENTS.md | 該檔 | 三 backend 對照、SetPostgresPool 前提、PGFirstOutcomeStore 降級、migration 陷阱 | 移除 placeholder |

### P1（兩週內）

| 項 | 內容 |
|----|------|
| P1-1 store backend fail-loud | store_factory default 分支未知 → error；共用 resolver |
| P1-2 backtest 目標守衛 | `SELECT current_database()` 自報 + `-expect-db` flag + outcomes job-local 標註 |
| P1-3 .env 分離 | local-deploy.md 修正 + source .env 前 echo DATABASE_URL 陷阱 |
| P1-4 密碼改名 | `atlas_dev_pwd_2026` → `atlas_prod_pwd_2026` |
| P1-5 atlas_dev 善後 | DROP 2 空表 + force 18 或保留（決策 D2） |
| P1-6 data-catalog 修正 | migration 路徑改 sql/migrations/；atlas.db 標 dev artifact |

### P2（一個月內）

| 項 | 內容 |
|----|------|
| P2-1 | `.github/instructions/db-migration.instructions.md` |
| P2-2 | integration 納入 release-check needs |
| P2-3 | 整合測試隔離（testcontainers / per-test schema） |
| P2-4 | stale DSN 盤點清除 |

## 4. 決策 D1-D4

- **D1（.env 指向）**：k3 建議指 atlas_dev，prod atlas DSN 移出 .env 只存在 gateway 容器 env。
- **D2（atlas_dev 存廢）**：需拍板；本計劃建議保留作 dev DB + 機制加固（WP5 -expect-db / WP3 guard）。
- **D3（prod 密碼改名）**：需維護窗口。
- **D4（gate 強制力）**：建議直接進 branch protection。

## 附錄：重現實證

| 證據 | 結果 |
|------|------|
| 本地起 timescaledb:2.14.2-pg15 跑 `go test -tags=integration ./...` | cmd/atlas 10 fail（M7）；`internal/ledger` `expected 2 got 4`（M8）|
| 本地無匹配 PG | 全部 `--- SKIP`，package 結果 `PASS`（M6 假綠燈）|
| `ssh kk@kimac grep DATABASE_URL ~/.config/atlas-go/.env` | 指向 `localhost:55432/atlas_dev`，與 local-deploy.md 矛盾（M10 實錘）|
| CI run 33074523440 failed log | `TestDualWriteMetrics_LoadRecent Expected ScreeningTotal 50, got 0`（M8 flaky）|
