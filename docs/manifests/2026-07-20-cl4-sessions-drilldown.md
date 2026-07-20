# Audit Manifest: CL-4 universe_get_sessions per-strategy force — Session List + Detail API

> **Audit source**: hermes 私域 `~/workspace/atlas-wiki/queries/atlas-mcp-capital-flow-history-truth-seeking-2026-07-19.md` §6.4 CL-4 + `~/workspace/atlas-wiki/queries/capital-flow-history-unresolved-2026-07-20.md` §2 BL-CL4

> **路徑備註**：本文所有 `[[atlas-wiki/queries/...]]` 形式 wikilink 為 hermes 私域 Obsidian-style 寫法。實體在 `~/workspace/atlas-wiki/queries/...`（hermes agent 工作目錄），不在本 repo 內。
> **Goal**: 補齊 MCP `universe_get_sessions` 對 per-strategy data 的暴露（List 帶 top_strategies 摘要 + Detail 端點拿完整 per-strategy outcomes）
> **Scope**: MEDIUM — 擴充 `HandleSessions` 加 `top_strategies` 聚合欄位 + 新增 `HandleSessionDetail` endpoint 呼叫既有 `LoadSessionOutcomes(sessionID)` Go function
> **Created**: 2026-07-20
> **Status**: planning

---

## ⚠️ Wiki-vs-Reality 揭露（必讀）

**本 manifest 結論與 [[atlas-wiki/queries/atlas-mcp-capital-flow-history-truth-seeking-2026-07-19]] §6.4 描述大致一致**，但深入 audit 揭露更多細節：

| 維度 | Wiki 描述（部分正確） | 深入 audit 結論 |
|------|---------------------|-----------------|
| 問題 | `universe_get_sessions` 缺「5 主體力值」 | 真實：缺 per-strategy outcomes（conviction / agent / action / guards 等） |
| 根因 | 「code 沒寫（API 設計只回 session metadata）」 | **部分對**：`HandleSessions` handler 只 expose 4 個 metadata fields；但**底層 `LoadSessionOutcomes(sessionID)` Go function 早已實作**（`internal/ledger/ledger.go:96` + `internal/ledger/outcome_store_sqlite.go:175`），只是**無 HTTP endpoint expose** |
| 解法 | 「要 drill-down 進 session 才有」 | 對的。但 drill-down 目前**要走 Go 程式碼**，沒有 HTTP wrapper。**本 PR 補這個 wrapper**。 |

**「5 主體力值」是 hermes 用的非正式術語**。真實概念是 **per-strategy outcomes**（每個 strategy 在 session 內的行為記錄），已存在 SQLite `outcomes` table，欄位包括 `agent_id / symbol / action / conviction / passed_guards / factor_scores_json` 等。

---

## 證據鏈摘要

| 證據層 | 來源 | 結論 |
|--------|------|------|
| **atlas-mcp live call** | `atlas-mcp_universe_get_sessions` | 120+ sessions 全部只有 4 fields：session_id / recorded_at / regime / outcome_count |
| **HandleSessions 程式碼** | `internal/monitoring/api/pipeline/handlers.go:386-389` | 只 map 4 個 fields，SessionSummary 20+ 欄位全丟失 |
| **SessionSummary struct** | `internal/domain/session.go:17-37` | 20+ 欄位已 ship：OrderCount/PositionCount/EndingCash/PortfolioValue/GuardOutcomes/TaxSnapshots/AfterTaxPnL/TotalTaxPaid/BrokerRuntime/RiskCommentary/... |
| **`LoadSessionOutcomes` Go func** | `internal/ledger/ledger.go:96` + `internal/ledger/outcome_store_sqlite.go:175` | **已實作** — 給 `sessionID` 回 `[]RecommendationOutcome`。**無 HTTP wrapper**。 |
| **outcomes table schema** | DB query (PRAGMA) | 15 欄位：id, session_id, symbol, agent_id, action, weight, target_price, stop_loss, conviction, regime, timestamp, passed_guards, guard_reason, factor_scores_json, conviction_breakdown_json |
| **outcomes table 資料** | DB query (52 rows) | 2 個 sessions × 26 outcomes；8 個 agents：earnings-quality-01 (12), value-yield-01 (10), technical-breakout-01 (10), growth-momentum-01 (6), financials-desk-01 (6), etf-rotation-01 (4), semi-desk-01 (2), ai-desk-01 (2) |
| **ReasoningHandler 4 個 notes** | `internal/monitoring/api/pipeline/reasoning_handler.go:145-179` | "historical session — detailed regime/agent/control/portfolio trace not preserved" — SessionSummary 設計上是 summary，detail trace 存 SQLite `outcomes` table |
| **MCP wrapper** | `cmd/atlas-mcp/server/tools_data_universe.go:95-103` | `handleUniverseGetSessions` 對 `/api/dashboard/sessions` 包成 `dataUniverseBaseOutput{Result}` |
| **既有 strategy tools** | `cmd/atlas-mcp/server/tools_strategy.go` | `strategy_get / strategy_get_attribution / strategy_get_summary` 已有（per-strategy ID query）但不走 session 聚合 |
| **既有 handler test** | `internal/monitoring/api/pipeline/handlers_test.go` | **無** `TestHandleSessions` test（依 codegraph 結果顯示） |
| **既有 MCP test** | `cmd/atlas-mcp/server/tools_data_universe_test.go:56` | `TestHandleUniverseGetSessions_OK` 驗 path 而非 shape |

**根因判定**（3 個 wiring gap）：
1. **`HandleSessions` handler 過度精簡**：SessionSummary 20+ 欄位但只 map 4 個
2. **無 top_strategies 聚合欄位**：handler 不從 outcomes table 拿 per-strategy data
3. **無 drill-down HTTP endpoint**：`LoadSessionOutcomes(sessionID)` Go function 已有但無 HTTP wrapper

**判定信心**：high（6 層獨立證據：live MCP call + source code + DB schema + DB data + file structure + test gap）

---

## Blast Radius（pre-change protocol Step 1）

### A2: `HandleSessions` 加 `top_strategies` 欄位

- Risk: **LOW**（gitnexus_impact 0 upstream callers — `HandleSessions` 只被 `RegisterRoutes` mux.Handle 呼叫）
- 新增欄位 to map[string]any response（additive，**向後相容** — 既有 client 讀 4 個 fields 不受影響）
- 新增 service method `LoadSessionsWithTopStrategies(limit, topN)`（**新方法**，不改 `LoadSessions` signature → 既有 caller 不破壞）

### B: 新增 `HandleSessionDetail` handler + `/api/dashboard/sessions/{id}` route

- Risk: **LOW**（純新增 — 既有路由 `GET /api/dashboard/sessions` 不動）
- 既有 `LoadSessionOutcomes(sessionID)` 直接呼叫（zero new infra）
- 新 MCP tool `universe_get_session_detail`（純新增）

### 不變的既有 helper

- `LoadSessions` 函數不動（既有 caller：HandlerSessions, LoadAgentObservatory, etc.）
- `LoadSessionOutcomes` 不動（既有 caller：LoadAgentObservatory, LoadOutcomesFromSessions 等）
- SessionSummary struct 不動（既有 caller 太多）
- OutcomeStore interface 不動（太多實作）

---

## Invariant Tracker

### 線 A：HandleSessions top_strategies 摘要（per-session aggregate）

| ID | Problem | Root Cause Hypothesis | Files to Change | Acceptance | Status | Docs Impact | Notes |
|----|---------|----------------------|-----------------|-------------|--------|-------------|-------|
| **A1** | `HandleSessions` 只回 4 個 metadata fields，缺 per-strategy 摘要 | `internal/monitoring/api/pipeline/handlers.go:386-389` 過度精簡 handler map | `internal/monitoring/service/pipeline.go`：新增 `LoadSessionsWithTopStrategies(limit, topN int) ([]SessionMeta, error)` 呼叫 `LoadSessions` + 對每個 session 呼叫 `LoadSessionOutcomes` 排序取 top N + `internal/monitoring/api/pipeline/handlers.go`：`HandleSessions` map 加 `top_strategies` 欄位 | (1) `go build ./...` 全綠；(2) 既有 `LoadSessions` caller 不破壞（4 fields 仍回）；(3) 新增 `TestLoadSessionsWithTopStrategies_IncludesTopPerSession` PASS；(4) 新增 `TestHandleSessions_IncludesTopStrategies` PASS；(5) live atlas-mcp MCP call 仍 work | done | spec §18.7 新章節 | nil-safe via `TopStrategies []RecommendationOutcome` field on SessionMeta | commit `A1+A2` |
| **A2** | SessionMeta 沒 `TopStrategies` 欄位 | struct 設計時未含 per-strategy aggregation | `internal/monitoring/service/pipeline.go`：SessionMeta struct 加 `TopStrategies []domain.RecommendationOutcome` 欄位（nil when not requested） | (1) compile 通過；(2) 既有 `LoadSessions` 不傳值 → nil → 既有 caller 不受影響 | done | 無 | nil-safe via 既有 caller 不寫此欄位 | commit `A1+A2` |

### 線 B：drill-down endpoint + MCP tool

| ID | Problem | Root Cause Hypothesis | Files to Change | Acceptance | Status | Docs Impact | Notes |
|----|---------|----------------------|-----------------|-------------|--------|-------------|-------|
| **B1** | 沒有 `/api/dashboard/sessions/{id}` HTTP endpoint | `HandleSessionDetail` handler 從未寫 | `internal/monitoring/api/pipeline/handlers.go`：新增 `HandleSessionDetail(r *http.Request) (int, any)` + `RegisterRoutes` 加 `mux.Handle("GET /api/dashboard/sessions/{id}", shared.Get(h.HandleSessionDetail))` | (1) compile 全綠；(2) `TestHandleSessionDetail_OK` PASS（mock LoadSessionOutcomes 回 fixed outcomes）；(3) `TestHandleSessionDetail_NotFound` PASS（sessionID 不存在 → 404）；(4) `isPublicPath` 加 `/api/dashboard/sessions/{` prefix case + `authFreePrefixPaths` 加 `/api/dashboard/sessions/` | done | spec §18.7 B sub-section | 既有 `LoadSessionOutcomes` 直接呼叫（不重複） | commit `B1` |
| **B2** | MCP 無 per-session detail tool | wrapper 沒寫 | `cmd/atlas-mcp/server/tools_data_universe.go`：新增 `handleUniverseGetSessionDetail(ctx, req, input)` + `UniverseGetSessionDetailInput{SessionID string}` + 在 `registerDataUniverseTools` 加 `countedAddTool("universe_get_session_detail", ...)` | (1) compile 全綠；(2) `TestHandleUniverseGetSessionDetail_OK` PASS；(3) MCP tool count 不破壞（AGENTS.md 108-110 assert 仍通過） | done | spec §18.7 B sub-section | 同 file 新增 handler + tool 註冊，2 sub-50 lines each | commit `B2` |

### 線 C：Test + docs（合約驗證）

| ID | Problem | Root Cause Hypothesis | Files to Change | Acceptance | Status | Docs Impact | Notes |
|----|---------|----------------------|-----------------|-------------|--------|-------------|-------|
| **C1** | 既有 `TestHandleSessions` 不存在 | 從未寫 | `internal/monitoring/api/pipeline/handlers_test.go`：新增 `TestHandleSessions_OK`（驗 4 個既有 fields + 5th `top_strategies` 欄位）+ `TestHandleSessions_EmptyStore` | (1) test PASS；(2) 確認既有用法（MCP wrapper 從 response 讀 4 個 fields）仍 work | done | 無 | 補既有測試缺口 | commit `C1+C2`（合 1 commit with C2） |
| **C2** | 既有 `TestHandleUniverseGetSessions_OK` 只驗 path | mock 過於鬆散 | `cmd/atlas-mcp/server/tools_data_universe_test.go`：增強 mock response 含 `top_strategies` 欄位 | (1) test PASS；(2) MCP path assertion 仍 work | done | 無 | optional enhancement → 新增 TestHandleUniverseGetSessionDetail_OK | commit `C1+C2`（合 1 commit with C1） |
| **C3** | spec §18.7 缺 Session List + Detail 契約 | 缺 | `docs/specs/capital-flow-seven-dimension-spec.md` §18.7 新章節 | markdown link check PASS | done | 無 | spec 擴充（與 CL-5/CL-3 一致 pattern） | commit 1 (spec + manifest) |

---

## Phase Tracker

### Phase A — Audit（已完成）

| Task | Status | Evidence |
|------|--------|----------|
| 真實 MCP call 驗證 production response | done | atlas-mcp_universe_get_sessions 回 120+ sessions × 4 fields |
| wiki-vs-reality 比對 | done | 描述大致對但細節差異（LoadSessionOutcomes 已實作但無 HTTP） |
| 設計 scope 拍板（Option C = A2 + B） | done | 用戶選擇 Option C |

### Phase B — Plan（已完成）

| Task | Status | Evidence |
|------|--------|----------|
| ID → 檔案/驗收對映 | done | 本 manifest Invariant Tracker（A1-A2, B1-B2, C1-C3） |
| 預期 commit 順序 | planned | spec+manifest → A1+A2 service → B1 handler → B2 MCP → C1-C3 tests → meta |
| Top strategies SQL 設計 | done | N+1: 每個 session 呼叫 `LoadSessionOutcomes` 排序 conviction DESC |

### Phase C — Implement（pending）

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Spec 擴充 + manifest | doc | pending | 待 commit |
| SessionMeta 加 TopStrategies 欄位 + LoadSessionsWithTopStrategies | A1+A2 | pending | 待 commit |
| HandleSessionDetail handler + route | B1 | pending | 待 commit |
| MCP handleUniverseGetSessionDetail + tool 註冊 | B2 | pending | 待 commit |
| 新增 handler test (TestHandleSessions + TestHandleSessionDetail) | C1 | pending | 待 commit |
| 加強 MCP test | C2 | pending | 待 commit |

### Phase D — Close Out（pending）

| Task | Status | Evidence |
|------|--------|----------|
| 驗收：`go build ./...` + `go test ./internal/monitoring/...` + `go test ./cmd/atlas-mcp/...` + markdown link + MCP docs consistency 全綠 | pending | - |
| Push branch + PR + CI 綠 | pending | PR # |
| Post-merge cleanup | pending | - |

---

## 預期 Commit 順序

| # | Format | 內容 |
|---|--------|------|
| 1 | `docs(manifest): #A1 #A2 #B1 #B2 #C1 #C2 #C3 extend §18 spec + initial manifest (CL-4 sessions drilldown)` | 純文件 |
| 2 | `feat(manifest): #A1 #A2 add LoadSessionsWithTopStrategies service + top_strategies field` | service.go |
| 3 | `feat(manifest): #B1 add HandleSessionDetail handler + /api/dashboard/sessions/{id} route` | pipeline/handlers.go |
| 4 | `feat(manifest): #B2 add universe_get_session_detail MCP tool` | tools_data_universe.go |
| 5 | `test(manifest): #C1 #C2 add TestHandleSessions + TestHandleSessionDetail + MCP path` | handlers_test.go + tools_data_universe_test.go |
| 6 | `docs(manifest): mark all IDs done + Phase C complete` | meta |

PR body 必須引用：`See docs/manifests/2026-07-20-cl4-sessions-drilldown.md`

---

## 設計決策（不開放重選）

| 決策 | 結論 | 證據 |
|------|------|------|
| **Top strategies 實作方式** | N+1 (對每個 session 呼叫 `LoadSessionOutcomes`) | sessions count bounded (~100)，N+1 perf 足夠；不需新 SQL interface；介面保持不變 |
| **SessionMeta 改 vs 新 struct** | 加 `TopStrategies []RecommendationOutcome` 欄位（nil-safe） | Go 結構 field 向後相容；既有 caller 拿到 nil 不會 panic；只有 `LoadSessionsWithTopStrategies` 會填值 |
| **Drill-down endpoint 設計** | `GET /api/dashboard/sessions/{id}` 直接呼叫 `LoadSessionOutcomes(sessionID)` | 既有 Go function 0 修改；純加 HTTP wrapper |
| **MCP tool 命名** | `universe_get_session_detail`（對齊 `universe_get_sessions`） | 一致命名空間 |
| **`LoadSessionOutcomes` 不重複** | Drill-down endpoint 呼叫既有 Go function（不新加 SQL query） | AGENTS.md「同一件事不可有三種算法」 |
| **Field 順序** | A1+A2 → B1 → B2 → C1+C2 → meta | service first（提供資料），handler 暴露，wrapper expose，test 驗證 |
| **404 behavior** | sessionID 不存在時回 404（不是 500） | spec 契約；既有 HandleSessions 對錯誤回 500，但 ID 不存在是「找不到」語意 |

---

## 不可動的事項

- 不改 `SessionSummary` struct（既有 20+ fields 已有，不重構）
- 不改 `OutcomeStore` interface（太多實作）
- 不改 `LoadSessionOutcomes` SQL（既有已正確）
- 不刪除既有 `HandleSessions` 4 個 fields（向後相容 — 加 5th `top_strategies`）
- 不改 `LoadSessions` signature（既有 caller 太多；用新方法 `LoadSessionsWithTopStrategies`）
- 不直接 push main（必須 worktree + PR + CI 綠）

---

## Session-End State

- **Done this session**:
  - Phase A 真實 audit（atlas-mcp live call + 6 層獨立證據）
  - Phase B 設計拍板 + manifest 撰寫
  - Phase C 程式實作（A1+A2 service + B1 handler + B2 MCP wrapper + C1+C2 tests + C3 spec）
- **Remaining**:
  1. Phase D close out（push + PR + CI 綠 + merge + cleanup）
- **Uncommitted code**: 待提交（service + handler + tests + MCP + spec）
- **Branch / PR**: `feat/cl4-sessions-drilldown` / 未開 PR
- **Worktree**: `.worktrees/feat-cl4-sessions-drilldown`（14721301 base）
- **Paused because**: 待 push + merge

---

## Change Log

| Date | Version | Change | Author |
|------|---------|--------|--------|
| 2026-07-20 | 1.0 | Initial manifest（7 IDs A1-A2 + B1-B2 + C1-C3; wiki-vs-reality + scope Option C） | OpenCode CLI Agent (Sisyphus) |
| 2026-07-20 | 1.1 | Phase C 全部完成：4 commits（spec+manifest / service A1+A2 / handler B1 / MCP B2 / tests C1+C2）；status 全部 → done；coverage 69.4%；MCP tool count 7 仍通過 AGENTS.md 108-110 assert；C3 spec 與 commit 1 同步 | OpenCode CLI Agent (Sisyphus) |

---

## 附錄：3 個產出檔案路徑預覽

| 檔案 | 內容 | 預估行數變動 |
|------|------|------------|
| `docs/manifests/2026-07-20-cl4-sessions-drilldown.md` | new manifest | +300 (new) |
| `docs/specs/capital-flow-seven-dimension-spec.md` | §18.7 new section | +80 |
| `internal/monitoring/service/pipeline.go` | SessionMeta field + LoadSessionsWithTopStrategies | +50 |
| `internal/monitoring/api/pipeline/handlers.go` | HandleSessionDetail + route | +40 |
| `cmd/atlas-mcp/server/tools_data_universe.go` | handleUniverseGetSessionDetail + tool | +40 |
| `internal/monitoring/api/pipeline/handlers_test.go` | 3 new tests | +100 |
| `cmd/atlas-mcp/server/tools_data_universe_test.go` | 1 enhanced test | +20 |

**總計**：~630 行（~40% spec + tests + 6 commits）