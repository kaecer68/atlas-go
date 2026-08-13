# Handoff: Outcome 資料來源語義 Audit + 統一 session_id 修復

> **接手對象**：新 CLI / 新 agent session（本 session 上下文過長，移交處理）
> **來源 session**：2026-08-13 SQLite→PG 遷移 worktree（`/Users/kaecer/workspace/atlas/2026-08-13-sqlite-to-postgres-migration`）
> **對應 manifest**：`docs/manifests/2026-08-13-migration-session-outcomes-audit.md`
> **狀態**：Phase A 完成（根因已證實）；Phase B 部分（資料分析完成，設計未定案）；**尚未動任何 code**
> **Created**: 2026-08-13

---

## 一、總目標

修復 PostgreSQL 遷移後 **performance-report trades=0**（SQLite 時代 real_trades=134），使 `LoadSessionOutcomes(session-XXX-daily)` 在 PG 查得到資料，統一 `recommendation_outcomes.session_id` 為 **`session-YYYYMMDD-daily`** 格式（與 `session_summaries.session_id` 一致），不產生 ghost session、不丟資料。

## 二、問題症狀（已證實）

| 指標 | SQLite 時代 | PG 時代 |
|---|---|---|
| performance-report real_trades | **134** | **0** |
| LoadSessionOutcomes(session-20260702-daily) | 45 筆 | **0 筆** |
| recommendation_outcomes.session_id 格式 | session-YYYYMMDD-daily | **日期 YYYY-MM-DD（7,406 筆）+ 空（43）+ 0 筆 session 格式** |

根因鏈（Phase A 已 accepted）：
1. `migrateOutcomesSQLiteData`（PR-5）用 `LoadOutcomes()` — 只讀 SQLite `session_id=''` 的 global，**2,965 筆 session-scoped（session_id!=''）完全沒遷**
2. JSONL `-outcomes` flag 遷入 root JSONL（45,661 筆，window=日期）時，`insertOutcomeBatch` 用 `o.Window`（日期）當 session_id → **誤存成日期格式**
3. PostgresLedgerStore.LoadSessionOutcomes 用 `WHERE session_id=$1`（session-XXX 格式）→ 查不到

## 三、需盤查的核心問題（Phase B 前置 — 不可猜測）

### Q1 — root JSONL 的語義（決定重映射是否安全）
`data/state/recommendation_outcomes.jsonl`（45,661 行）：
- window = 日期字串（2026-05-31 等，44 個不同日期，非純月末）
- session_id = null（全部）
- **它是「全球聚合檔（global aggregate）」還是「跨 session 的交易日資料」？**
- 盤查方式：查 git history 這個檔怎麼被寫入（`internal/ledger/ledger.go` Store.RecordOutcomes 或 scheduler）；看 window 是否對應真實 session 交易日；比對 SQLite global（window 空）是否同義

### Q2 — 重映射安全性
- PG 7,406 筆日期 session_id → 目標 `session-YYYYMMDD-daily`。**已驗證**：25 個日期中 24 個有對應 session 目錄、1 個（2026-07-21）無目錄但 SQLite 有 375 筆佐證。
- 需盤查：重映射後是否所有 session 都有真實資料來源？metadata JSON 內嵌的 `window` 要不要一起改？

### Q3 — 三批資料的重疊/衝突
| 批次 | 來源 | 筆數 | session_id 現況 |
|---|---|---|---|
| A. SQLite session | `outcomes WHERE session_id!=''` | 2,965 | session-XXX（未遷 PG） |
| B. SQLite global | `outcomes WHERE session_id=''` | 3,032 | 空（PG 只有 43，2,989 未遷） |
| C. root JSONL | `recommendation_outcomes.jsonl` | 45,661 | 日期（已遷 PG 7,406） |
| D. session JSONL | `sessions/*/recommendation_outcomes.jsonl` | 4,164（132 檔） | window=交易日 |

- 盤查：A 與 C 是否重疊（同一 session 的資料是否兩邊都有）？A ∪ C 的正確 union 該是什麼？現行 NOT EXISTS guard（`session_id+symbol+agent_id+time`）對 A/B/C 的去重是否正確？

### Q4 — 為什麼 SQLite global 只遷到 43 筆
- SQLite global 3,032 vs PG 空 session_id 43 — 現行 migrateOutcomesSQLiteData 用 LoadOutcomes()，NOT EXISTS guard key 含 session_id，為何只進 43？是否 key 衝突（與 C 批日期格式不同 key）？

### Q5 — 正確的統一 session_id 設計
- 盤查後決定：是否重映射 C 批（日期→session-XXX）+ 補遷 A/B 批（session_id 保留），三批統一成 session-XXX 格式
- 檢查 `PostgresLedgerStore` 的 `RecordOutcomes`/`RecordSessionOutcomes` 用 `o.Window` 當 session_id 是否需改（還是只改 migration）

## 四、盤查完畢後要做的事（Phase C/D）

1. **設計統一 session_id 方案**（依 Q1-Q5 結論）：
   - 方案甲：重映射 C 批（UPDATE 日期→session-XXX）+ 補遷 A 批（保留 session_id）+ 補遷 B 批（session_id 留空）
   - 方案乙：不重映射 C，只補遷 A，並讓 LoadSessionOutcomes 同時匹配日期與 session 格式（技術債）
   - 擇一，記錄 tradeoff
2. **實作**（cmd/migrate-data/main.go，可能 + store 層）：
   - 新增 migration 函式 / flag（如 `-outcomes-sqlite-sessions`、`-remap-outcome-sessions`）
   - 冪等（ON CONFLICT / NOT EXISTS）
3. **重跑遷移**（dev PG，來源 production atlas.db，先備份）
4. **驗證**：
   - PG session_id 統一為 session-XXX 格式，0 筆 ghost（每個 session_id 有真實來源）
   - `LoadSessionOutcomes(session-20260702-daily)` > 0
   - performance-report real_trades > 0（目標接近 SQLite 時代 134）
   - 既有表（quotes/screening/trades/experiments）counts 不變
5. **commit + PR**（走分支→push→gh pr create，PR body 引用 manifest）
6. **更新 manifest** 狀態列

## 五、驗收計劃（完成定義）

- [ ] PG `recommendation_outcomes` 無純日期 session_id（全為 session-XXX 或空）
- [ ] 每個非空 session_id 都有真實 session 來源（無 ghost）
- [ ] `curl :18080/api/dashboard/performance-report?period=all` real_trades > 0
- [ ] `LoadSessionOutcomes(session-20260702-daily)` 回傳 > 0
- [ ] `go test ./internal/ledger/...` 過
- [ ] `make ci-gate` + `make ci-full` 過
- [ ] 遷移冪等（重跑 counts 不變）
- [ ] manifest 全部狀態更新、backlog 處理

## 六、環境事實（勿重猜）

- 主 repo 資料（production 現役）：`/Users/kaecer/workspace/atlas/data/state/atlas.db`（76MB，WAL）
- dev PG：`postgres://atlas:atlas_dev_pwd_2026@127.0.0.1:5432/atlas?sslmode=disable`
- root JSONL：`/Users/kaecer/workspace/atlas/data/state/recommendation_outcomes.jsonl`（45,661 行）
- session JSONL：`/Users/kaecer/workspace/atlas/data/state/sessions/*/recommendation_outcomes.jsonl`（132 非空檔，4,164 行）
- 關鍵檔案：
  - `cmd/migrate-data/main.go`（migrateOutcomesSQLiteData:916、insertOutcomeBatch:355、migrateOutcomesFile:299）
  - `internal/ledger/postgres_ledger.go`（PostgresLedgerStore，LoadSessionOutcomes:124、RecordOutcomes:56、RecordSessionOutcomes:87）
  - `internal/ledger/outcome_store_sqlite.go`（LoadOutcomes/LoadOutcomesFromSessions/scanOutcomes — scanOutcomes 不保留 session_id）
  - `internal/reporting/performance.go`（loadAllOutcomes:369 用 store.LoadSessionOutcomes(s.SessionID)）
  - `internal/repository/dual_write.go`（LoadSessionOutcomes:334 JSONL fallback）
- 注意：`scanOutcomes`（outcome_store_sqlite.go）只填 `Window`（SQLite window 欄位），**不保留 session_id 欄位** → migration 若要保留 session_id 需直接 SQL，不能走 domain 物件

## 七、接手約束

- **禁止猜測修復**：Q1-Q5 未盤查完前不動 code（audit 精神）
- 每 ID 一個 commit（`<type>(manifest): #<ID> <desc>`）
- 走分支→push→gh pr create→merge，禁直接 push main
- 改 code 前跑 `atlas-pre-change-protocol`（Step 0 overlap、Step 1 blast radius）
- 接手前先跑 `make check-binaries`（start-session gate）
- migration 前先備份 atlas.db
- 不自行執行含 docker 的 target（除非 kaecer 明示授權）

## 八、Branch 狀態

- 分支：`fix/20260813-session-outcomes-migration`（已建，目前只有 manifest + handoff commit，無 code change）
- 接手後在此分支繼續，或依 multi-cli-protocol 另建
