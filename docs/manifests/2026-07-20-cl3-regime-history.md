# Audit Manifest: CL-3 regime_get_history 端點 — Wiki-vs-Reality 修正

> **Audit source**: [[atlas-wiki/queries/atlas-mcp-capital-flow-history-truth-seeking-2026-07-19]] §6.4 + §4 CL-3（過時，需以本 manifest 為準）
> **Goal**: 修正 `regime_get_history` MCP 工具的回傳資料：當前是 simulation session 摘要（沒時序）+ 永遠 score=0；修正後是 regime_history 表真實時序 + janus composite score（含 is_synthetic 旗標）
> **Scope**: MEDIUM — service 修法（3 個 wire 點 + builder pattern）+ 新增 `/api/janus/regime-score` HTTP endpoint + 修 MCP wrapper formula 不一致
> **Created**: 2026-07-20
> **Status**: planning

---

## ⚠️ Wiki-vs-Reality 重大差距（必讀）

**本 manifest 與 [[atlas-wiki/queries/atlas-mcp-capital-flow-history-truth-seeking-2026-07-19]] §6.4 + §4 的描述不一致**。本 manifest 為準，wiki 描述已過時（2026-07-19 寫，當前 codebase 2026-07-20 已部分 ship）：

| 維度 | Wiki §6.4 + §4 描述（過時） | 當前現實 |
|------|--------------------------|---------|
| 時序存儲 | 「沒有『每個交易日一個 regime score』的時序存儲」 | **`regime_history` SQLite 表已 ship 且有 90 筆真實資料**（2026-04-01 到 2026-06-29，all `is_synthetic=0`） |
| Store 介面 | 「需新建 `RegimeObservationStore`」 | **`HistoricalStore` interface + SQLite impl 已 ship**（`internal/ledger/historical_store.go`，commit 來自 stage4-loader 階段） |
| 寫入路徑 | 「JANUS 6h 排程要每天寫」 | **`stage4-loader` 已 backfill**（`cmd/atlas-stage4-loader/main.go:380`），runtime writer 未實作（依賴 PRISM training pipeline） |
| MCP 真實 score | `/api/janus/regime-score` 是 score 來源 | **這個 route 不存在**（grep 完全沒結果），`fetchRegimeRealScore` 呼叫 404 → 永遠 fallback |
| Service 實作 | LoadSessionSummaries 是 simulation summary | **對，但這就是 bug** — `PipelineService.LoadRegimeHistory` 應該讀 `HistoricalStore` 但 `HistoricalStore` 沒注入 |
| Score 設計哲學 | （wiki 沒提） | **「honest unknown vs misleading 0」** — `RegimePoint.Score *int` 用 `omitempty`，不知道就不報 |

**結論**：本 manifest 不開新 store（已存在），而是修 wiring gap（service 讀錯 store + HistoricalStore 沒注入）+ 補 MCP endpoint（原本就不存在）+ 修 MCP formula 不一致。

---

## 證據鏈摘要

| 證據層 | 來源 | 結論 |
|--------|------|------|
| `regime_history` 表資料 | `data/state/atlas.db` query（python sqlite3） | 90 筆真實時序資料：min 2026-04-01, max 2026-06-29；NEUTRAL 24 / RISK_OFF 26 / RISK_ON 23 / TRANSITIONAL 17；all `is_synthetic=0` |
| Stage4-loader 寫入 | `cmd/atlas-stage4-loader/main.go:380` `store.UpsertRegime(...)` | 確認 regime_history 表是真實 backfill 來源 |
| HistoricalStore interface | `internal/ledger/historical_store.go:90-95` | `LoadRegimeHistory(ctx, limit)` + `LoadRegimeHistoryAll(ctx, limit)` 已實作 |
| `LoadRegimeHistory` 測試 | `internal/ledger/historical_store_test.go:95` `TestSQLiteHistoricalStore_LoadRegimeHistory_OrderedDesc` | 既有完整測試 |
| Service 錯路徑 | `internal/monitoring/service/pipeline.go:1073-1108` | `PipelineService.LoadRegimeHistory` 用 `store.LoadSessionSummaries()`（讀 filesystem sessions）不是 `LoadRegimeHistory`（讀 SQLite regime_history） |
| HistoricalStore 沒注入 | `cmd/atlas/main.go:678-680` | `_ = historicalStore // SystemCore has no HistoricalStore field yet; wiring deferred to follow-up PR` — **本 manifest 補這個 deferred wiring** |
| PipelineService ctor | `internal/monitoring/service/pipeline.go:50` | `func NewPipelineService(workDir, ledgerDir string, store ledger.OutcomeStore) *PipelineService` — 沒 HistoricalStore 參數 |
| PipelineService builder 模式 | `pipeline.go` 內無 builder | 但 `dashboard_api.go:637` 已有 `.WithNarrativeProvider(...)` chain — pattern 已存在 |
| MCP wrapper score | `cmd/atlas-mcp/server/tools.go:184-217` | `fetchRegimeRealScore` 呼叫 `/api/janus/regime-score`（404）+ fallback `fetchRegimeCompositeScore` |
| Janus engine 真實 API | `internal/janus/engine.go` `GetCurrentRegimeScore() (float64, bool)` | 已實作，回 (score, isSynthetic)；只是沒 HTTP endpoint expose |
| Janus canonical formula | `internal/janus/composite_score_test.go:18` | `score = tanh(foreignFlow/5e9) * 30 - max(0, VIX-20) * 1.5` |
| MCP formula 不一致 | `cmd/atlas-mcp/server/tools.go:225` | 用 `/5` 而非 `/5e9` — 公式不同（**bug**：可能因 /5 永遠 saturate） |
| `/api/janus` routes | grep 完全沒結果 | janus 純 in-memory engine，沒有 HTTP handler framework |
| `janusEngine` in main.go | `cmd/atlas/main.go:420` | `janusEngine = janus.NewEngine()` 已建立 — 可作為 `/api/janus/regime-score` 依賴 |
| NewPipelineService callers | grep | 2 prod + 43 test = 45 total |

**根因判定**（4 個 wiring gap）：
1. **service.go:1073**：`PipelineService.LoadRegimeHistory` 讀錯 store（`LoadSessionSummaries` vs `LoadRegimeHistory`）
2. **service.go:48**：`NewPipelineService` 沒 HistoricalStore 注入
3. **main.go:678-680**：HistoricalStore 沒注入 SystemCore + PipelineService
4. **tools.go:184-217**：MCP wrapper 呼叫不存在 endpoint + formula 不一致

**判定信心**：high（5 層獨立證據 + 程式碼 + DB query + git log + grep 全部對齊）。

---

## Blast Radius

### A: `WithHistoricalStore` builder（45 callers 不破壞）

- Risk: **CRITICAL → MEDIUM**（mitigated by builder pattern）
- 2 prod callers (`cmd/atlas/stage3_tasks.go:51`, `internal/monitoring/dashboard_api.go:637`) 需要 chain `.WithHistoricalStore(historicalStore)`
- 43 test callers 不動（builder 是 optional method）
- Modules affected: 2（monitoring + scheduler via stage3_tasks）

### B: 新增 `/api/janus/regime-score` HTTP endpoint

- Risk: **LOW**
- 在 `cmd/atlas/main.go` 加 inline handler（避免新 package，與既有 janus wiring 風格一致）
- Route 註冊：`mux.Handle("GET /api/janus/regime-score", apishared.Get(...))`
- `isPublicPath` 已在 `/api/janus/*` 涵蓋範圍 — 不需加 whitelist
- MCP wrapper 改 URL — 既有 caller 0（MCP wrapper 是唯一 user）

### B': 修 MCP wrapper formula `/5` → `/5e9`（刪除重複實作）

- Risk: **LOW**
- `fetchRegimeRealScore` 修為真實呼叫 `/api/janus/regime-score`（B 修完後 endpoint 存在）
- 刪除 `fetchRegimeCompositeScore`（重複實作 + 公式錯誤）
- 不破壞既有行為（fallback 路徑刪除，但 primary path 已可用）

---

## Invariant Tracker

### 線 A：Service 修法（PipelineService 改讀 regime_history）

| ID | Problem | Root Cause | Files | Acceptance | Status | Docs Impact | Notes |
|----|---------|-----------|-------|------------|--------|-------------|-------|
| **A01** | `PipelineService.LoadRegimeHistory` 讀 filesystem sessions 而非 SQLite regime_history | `service.go:1073` 用 `store.LoadSessionSummaries()`；service 沒有 HistoricalStore 注入 | `internal/monitoring/service/pipeline.go`：PipelineService struct 加 `historicalStore ledger.HistoricalStore` field；加 `WithHistoricalStore(hs) *PipelineService` builder；`LoadRegimeHistory` 改為優先用 `historicalStore.LoadRegimeHistory(limit)`，若 hs==nil fallback 到 `LoadSessionSummaries`（向後相容） | (1) `go build ./...` 全綠；(2) 既有 `TestLoadRegimeHistory_*` 全綠（向後相容 hs=nil 路徑）；(3) 新增 `TestLoadRegimeHistory_HistoricalStore_OK` PASS；(4) `go vet ./internal/monitoring/service/...` clean | pending | spec §18.6 新章節「Historical Regime Observation Store wiring」 | nil-safe fallback 保證既有 43 個 test 不破壞 |
| **A02** | 既有 prod callers 不傳 HistoricalStore | `cmd/atlas/stage3_tasks.go:51` + `internal/monitoring/dashboard_api.go:637` | chain `.WithHistoricalStore(historicalStore)` | (1) `go build ./...` 全綠；(2) 既有 handler 測試 PASS；(3) integration: GET `/api/dashboard/regime-history?limit=30` 回 sessions 真實 regime（不是 simulation session metadata） | pending | 無 | 2 個 prod caller 修改 |
| **A03** | `cmd/atlas/main.go:678-680` HistoricalStore 沒注入 SystemCore / PipelineService | main.go 已 init HistoricalStore 但丟棄（`_ = historicalStore`） | `cmd/atlas/main.go`：找到 `PipelineService` 的 wire 點，傳入 historicalStore | (1) main.go build OK；(2) 不再有 `_ = historicalStore` 丟棄 pattern | pending | 無 | 同一 PR 內順手補 |

### 線 B：新增 `/api/janus/regime-score` endpoint + 修 MCP wrapper

| ID | Problem | Root Cause | Files | Acceptance | Status | Docs Impact | Notes |
|----|---------|-----------|-------|------------|--------|-------------|-------|
| **B01** | `/api/janus/regime-score` endpoint 不存在；`fetchRegimeRealScore` 永遠 404 fallback | janus engine 純 in-memory，沒 HTTP handler framework；MCP wrapper 假設存在 | `cmd/atlas/main.go`：加 inline handler 呼叫 `janusEngine.GetCurrentRegimeScore()` 回 `{score, is_synthetic}`；route 註冊 `/api/janus/regime-score` | (1) `go build ./...` 全綠；(2) `curl /api/janus/regime-score` 回 200 + JSON；(3) `is_synthetic=true` 當 PRISM training 沒 populate；(4) `cmd/atlas-mcp` 測試全綠 | pending | spec §18.6 加新 sub-section `/api/janus/regime-score` 契約 | main.go ad-hoc handler，與既有 janus wiring 風格一致 |
| **B02** | MCP `fetchRegimeCompositeScore` 公式 `/5` 與 janus canonical `/5e9` 不一致 | tools.go:225 hardcoded `/5` | `cmd/atlas-mcp/server/tools.go`：刪除 `fetchRegimeCompositeScore`（公式錯誤 + 與 janus 重複實作）；`fetchRegimeRealScore` 改 URL 驗證 + `is_synthetic` field 帶入 | (1) `go build ./...` 全綠；(2) `TestHandleMacroGetRegimeHistory_*` 改 mock response 含 `is_synthetic` 後仍 PASS；(3) score 與 janus engine 一致 | pending | spec §18.6 加公式一致性 note | 刪除重複實作，避免日後 drift |

### 線 C：Stage4 backfill → runtime writer（**不做**，入 Backlog）

| ID | Problem | Discovery | Notes |
|----|---------|-----------|-------|
| **BL-CL3b** | `regime_history` 表只有 stage4 backfill 資料（截至 6/29），runtime 沒有新寫入 | 2026-07-20 audit 發現 | 需 PRISM training pipeline 整合後才能寫。**本 PR 不處理**，入下輪 Backlog |
| **BL-CL4** | `universe_get_sessions` 只回 session metadata，缺 per-strategy 5 主體力值 | 2026-07-20（沿用 wiki §6.4） | 中等優先，獨立工作 |
| **BL-CL6** | `recorded_at` ≠ `filename date` 語意混淆 | 2026-07-20（沿用 wiki §6.4） | 低優，與 H05 評估 |
| **BL-MS-01** | SnapshotDir 長期 retention | 2026-07-20 | 短期 <10K 檔不影響 |
| **BL-CF-01** | CapitalFlow store 歷史 backfill（CL-5 提到） | 2026-07-20 | 待 B4 驗證 + Provider API |

---

## Phase Tracker

### Phase A — Audit（已完成）

| Task | Status | Evidence |
|------|--------|----------|
| 真相盤查 wiki §6.4 + §4 CL-3 | done | atlass-wiki/queries/atlas-mcp-capital-flow-history-truth-seeking-2026-07-19.md §6.4 + §4 |
| Wiki-vs-reality 差距揭露 | done | DB query 確認 90 筆資料 + 程式碼 audit |
| 設計 scope 拍板 | done | Option B（A + 加 `/api/janus/regime-score`），1-2 天 |

### Phase B — Plan（已完成）

| Task | Status | Evidence |
|------|--------|----------|
| ID → 檔案/驗收對映 | done | 本 manifest Invariant Tracker（A01-A03, B01-B02） |
| 預期 commit 順序 | planned | spec+manifest → A01 → A02 → A03 → B01 → B02 → meta |
| Builder pattern 確認 | done | `WithNarrativeProvider` chain 已存在，`WithHistoricalStore` 沿用 |

### Phase C — Implement（pending）

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Spec 擴充 §18.6 + manifest | doc | pending | 待 commit |
| PipelineService struct + WithHistoricalStore + LoadRegimeHistory 改讀 | A01 | pending | 待 commit |
| 2 個 prod caller chain .WithHistoricalStore | A02 | pending | 待 commit |
| main.go HistoricalStore 注入 PipelineService | A03 | pending | 待 commit |
| 新增 `/api/janus/regime-score` handler + route | B01 | pending | 待 commit |
| MCP wrapper 刪除 fetchRegimeCompositeScore + 修 fetchRegimeRealScore | B02 | pending | 待 commit |

### Phase D — Close Out（pending）

| Task | Status | Evidence |
|------|--------|----------|
| 驗收：`go build ./...` + `go test ./...` + markdown link + MCP docs consistency 全綠 | pending | - |
| Push branch + PR + CI 綠 | pending | PR # |
| Post-merge cleanup | pending | - |

---

## 預期 Commit 順序

| # | Format | 內容 |
|---|--------|------|
| 1 | `docs(manifest): #A01 #A02 #A03 #B01 #B02 extend §18 spec + initial manifest` | 純文件 |
| 2 | `feat(manifest): #A01 add WithHistoricalStore builder + LoadRegimeHistory HistoricalStore path` | service.go |
| 3 | `feat(manifest): #A02 #A03 wire HistoricalStore into stage3_tasks + dashboard_api + main.go` | 3 個 wire 點 |
| 4 | `feat(manifest): #B01 add /api/janus/regime-score endpoint` | main.go inline handler + route |
| 5 | `fix(manifest): #B02 remove duplicate fetchRegimeCompositeScore + fix MCP formula` | tools.go |
| 6 | `docs(manifest): mark A01-A03 + B01-B02 done + Phase C complete` | meta |

PR body 必須引用：`See docs/manifests/2026-07-20-cl3-regime-history.md`

---

## 設計決策（不開放重選）

| 決策 | 結論 | 證據 |
|------|------|------|
| **Scope 採 wiki-vs-reality 修正**，不採 wiki §6.4 建議的「建新 store」 | wiki 描述與現實嚴重脫節 | DB query + 程式碼 audit |
| **`WithHistoricalStore` builder pattern** | 不破壞既有 43 個 test caller | grep 結果（2 prod + 43 test） |
| **HistoricalStore 為 optional，nil-safe fallback** | `PipelineService.LoadRegimeHistory` 若 hs==nil 走舊路徑 | 向後相容 + 漸進式採用 |
| **`/api/janus/regime-score` 放 main.go inline** | 不開新 package | janus 純 in-memory engine，無 HTTP framework；main.go 已有 `janusEngine` 變數 |
| **刪除 `fetchRegimeCompositeScore`** | 公式錯誤（`/5` vs `/5e9`）+ 與 janus 重複 | trap：「同一件事不可有三種算法」 |
| **`is_synthetic` flag 透傳** | 真實 score vs macro-derived 是不同語意 | `RegimePoint.Score` 設計哲學 honest unknown |
| **不實作 runtime regime writer** | 需 PRISM training pipeline（1 季度工作） | BL-CL3b 範圍 |

---

## 不可動的事項

- 不改 `regime_history` 表 schema（不動 SQLite migration）
- 不動 `internal/janus/Engine` API（用既有 `GetCurrentRegimeScore`）
- 不實作 runtime regime writer（BL-CL3b 範圍）
- 不動既有的 wiki 文件（deprecation 在另一個 PR 處理）
- 不直接 push main（必須 worktree + PR + CI 綠）

---

## Session-End State

- **Done this session**:
  - Phase A 真相盤查（wiki-vs-reality 揭露）
  - Phase B manifest 撰寫（本文件）
- **Remaining**:
  1. Spec 擴充 §18.6
  2. Phase C 程式實作（A01-A03 + B01-B02）
  3. Phase D close out
- **Branch / PR**: `feat/cl3-regime-history` / 未開 PR
- **Worktree**: `.worktrees/feat-cl3-regime-history`（965dd399 base）

---

## Change Log

| Date | Version | Change | Author |
|------|---------|--------|--------|
| 2026-07-20 | 1.0 | Initial manifest（3 IDs A01-A03, 2 IDs B01-B02; wiki-vs-reality 揭露為核心） | OpenCode CLI Agent (Sisyphus) |

---

## 附錄：真實 callers 列表

`NewPipelineService` 45 callers（2 prod + 43 test）：

**Prod callers**：
- `cmd/atlas/stage3_tasks.go:51` — 修改
- `internal/monitoring/dashboard_api.go:637` — 修改

**Test callers**（不動）：
- `internal/monitoring/api/pipeline/handlers_test.go:172`
- `internal/monitoring/service/pipeline_test.go` × 22
- `internal/monitoring/service/recommendation_outcome_test.go` × 5
- `internal/monitoring/service/forward_return_synthetic_test.go:80`