# Stockpicker 誤導機制修復執行計劃（deepseek-v4-pro, 2026-08-28）

> 依據：`tasks/stockpicker-misleading-mechanisms-audit-k3-20260828.md`（k3 審計報告）。
> 角色：把 k3 的 P0/P1/P2 + D1~D4 轉成可派工的 Work Package。
> 註：本文件於 2026-08-28 kernel 中斷後由 root agent 依對話記錄重建存檔（原始檔因未 commit 遺失）。

## 1. 執行策略摘要（一句話）

**「先把紅燈接上（P0：PG migration gate + integration job 修好 + Postgres-first 文件），再把誤導源拔掉（P1：fail-loud + 目標守衛 + .env/密碼去混淆），最後把防線制度化（P2：instructions + gate 常態化 + 測試隔離），iMac prod 操作一律走 hermes/root 並過 agent-guard。」**

## 2. Work Package 清單

> 派工模型縮寫：kimi=kimi-for-coding（文件/契約）；ds4pro=deepseek-v4-pro（跨檔/CI）；ds4flash=deepseek-v4-flash（Go 修改）；hermes=hermes-dispatch（iMac prod）；root=root agent 自己。

### WP1 — 文件陷阱表補 PG 方言（P0-3）
- 改 `docs/reference/traps.md` §Data/Persistence + `AGENTS.md` 高頻陷阱表
- 派工：kimi | 驗收：grep Postgres-first 命中、三行 PG 方言、traps.md ≤330 行、AGENTS.md ≤155 行

### WP2 — 重寫 internal/ledger/AGENTS.md（P0-4）
- 移除 placeholder，寫五段：三 backend 對照 / SetPostgresPool 前提 / PGFirstOutcomeStore 降級 / 無 JSONL fallback 原因 / migration 陷阱指向 traps.md
- 派工：kimi

### WP3 — PG migration 驗證 gate + integration job 三缺陷修復（P0-1 + P0-2）
- 新增 `internal/db/migration_pg_test.go`（integration tag）+ `scripts/ci/check-migration-sql.sh` + ci-cd.yml 修復（ATLAS_ENV_FILE guard / DSN 統一 / per-test TRUNCATE）+ `internal/testdb`（DATABASE_URL-only helper）
- 派工：ds4pro（唯一改 ci-cd.yml 的 WP）| 驗收：故意紅 / CI 連 3 次全綠 / 拔 DATABASE_URL 紅 / M7 單測 / M8 修復

### WP4 — store backend fail-loud + 共用 resolver（P1-1a）
- store_factory default 分支未知 → error；`ResolveStoreBackend` 共用
- 派工：ds4flash

### WP5 — backtest job 目標守衛 + resolver DRY（P1-1b + P1-2）
- `run-stockpicker-backtest/main.go`：`SELECT current_database()` 自報 + `-expect-db` flag + resolveBackend 改呼叫共用 resolver + outcomes job-local 標註
- 派工：ds4flash | 依賴：**PR 2a merge + WP4 merge**（本 WP 的 worktree 基於 PR 2a merge 後的 main 開出）

### WP6 — .env 分離落地（P1-3 = D1）
- local-deploy.md 修正（iMac .env → dev atlas_dev；prod DSN 移出）+ traps.md source-.env 陷阱 + iMac .env 移除 prod DSN
- 派工：文件 kimi + iMac 操作 hermes/root（過 agent-guard）

### WP7 — prod 密碼/命名去混淆（P1-4 = D3）
- `ALTER USER atlas WITH PASSWORD 'atlas_prod_pwd_2026'` + compose（prod/crons）+ SKILL + 兩機 .env 同步
- 派工：hermes/root（維護窗口）| 需拍板

### WP8 — atlas_dev 污染善後（P1-5 = D2）
- 三選一：保留（本計劃建議）/ 清理 force 18 / DROP DATABASE
- 派工：hermes/root（過 agent-guard）| 需拍板

### WP9 — data-catalog.md 修正（P1-6）
- migration 路徑改 sql/migrations/；atlas.db 標 dev artifact；quotes 標 PG SSoT
- 派工：kimi

### WP10 — db-migration.instructions.md（P2-1）
- 新增 `.github/instructions/db-migration.instructions.md`（雙方言規則 / 保留字 / DOUBLE PRECISION / PG gate）
- 派工：kimi

### WP11 — integration 測試隔離（P2-3）
- testcontainers-go 或 per-test schema 根除共享 DB 污染
- 派工：ds4pro | 依賴：WP3 merge

### WP12 — stale DSN 盤點清除（P2-4）
- manifest/SKILL 的 stale DSN 標 historical
- 派工：root/kimi | 依賴：WP7 完成（新密碼定案）

## 3. 執行順序（依賴圖）

```
Wave 0（並行）：WP1 WP2 WP9 WP10（kimi docs）| WP3（ds4pro）| WP4（ds4flash）
Wave 1（等前置）：WP5（PR2a + WP4）| WP11（WP3）| WP6+WP7（維護窗口）| WP8（驗證子步驟立即）
Wave 2（收尾）：WP12（WP7）
```

## 4. D1-D4 決策執行方式

| 決策 | k3 建議 | 本計劃處置 | 需拍板 |
|------|---------|-----------|--------|
| D1 | .env → atlas_dev；prod DSN 移出 | WP6 依建議執行 | 否 |
| D2 | 清理保留 或 DROP | 需拍板；建議保留+加固 | **是** |
| D3 | 密碼改名 | WP7 維護窗口 | **是**（用戶已同意） |
| D4 | gate 直接進 branch protection | 分兩階段：WP3 第一階段 + 穩定後第二階段 | 建議直接進 |
