# Audit Manifest: CL-5 capital-flow/history 缺 status 報告 + capacity 60→252 — 設計與實作

> **Audit source**: [[atlas-wiki/queries/atlas-mcp-capital-flow-history-truth-seeking-2026-07-19]] §6.4 CL-5 + [[atlas-wiki/queries/capital-flow-history-unresolved-2026-07-20]] §2 BL-CL5
> **承接**: docs/manifests/2026-07-20-capital-flow-history-audit.md（CL-1 修復後未解決）+ docs/manifests/2026-07-20-cl2-macro-snapshot-history.md（前輪 CL-2 已 ship）
> **Goal**: 補齊 `/api/capital-flow/history` 對缺失 dimension 的 silent omission（CF-INV-17 程式實作）+ 把 production capacity 從 60 提升到 252（spec H-CF-05 gate）
> **Scope**: MEDIUM — handler 行為擴充（opt-in `?include_meta=true` 向後相容 H02 frontend）+ 3 處 capacity 常數提升 + 既有 `TestHandleHistory` 確保不破壞。明確不做：spec §18.3 描述的 point-in-time endpoint `/api/capital-flow/historical-snapshot/{trading_date}`（入 Backlog）；production store 歷史資料 backfill（無 Provider 資料源；需另立 manifest）。
> **Created**: 2026-07-20
> **Status**: planning

---

## Scope 取捨說明（必讀）

CL-5 在 wiki 不同段落有兩個 scope 描述：

| 來源 | 描述 |
|------|------|
| **Wiki §2 BL-CL5 + 用戶原始指令** | HandleHistory 程式實作 + capacity 60→252 提升 |
| **Wiki §6.4 + spec §18.3** | 新增 `/api/capital-flow/historical-snapshot/{trading_date}` point-in-time endpoint |

**本 manifest 採解讀 A（按用戶字面「HandleHistory 程式實作」）**：修改既有 `HandleHistory` handler 行為 + capacity 提升。Point-in-time endpoint（解讀 B）入 Backlog 下下輪做。

**H02 frontend 向後相容考量**：`shared_web/static/js/pages/capital-history.js`（H02 retail-experience，commit 04622ab1）會讀 `currentData[d.key]` 預期 array。改 response shape 會破壞 UI。設計採 **opt-in `?include_meta=true` wrapper**：

- 預設行為（不傳 `include_meta`）：回傳既有 flat shape `{foreign: [...], government: []}` — frontend 不破壞
- 開啟 `?include_meta=true`：回傳 wrapper `{samples: {...}, meta: {status, missing_dimensions, ...}}` — hermes/MCP 拿到缺失資訊

---

## 證據鏈摘要

| 證據層 | 來源 | 結論 |
|--------|------|------|
| Handler 程式碼 | `internal/capitalflow/handler.go:95-143` | `HandleHistory` 對每個 dimension 呼叫 `store.History(...)`，error 或空資料 → `result[dim] = []RollingSample{}`（silent omission） |
| Handler days 參數 | `internal/capitalflow/handler.go:96-110` | `days := 60`，`if n > 60 { n = 60 }` — handler 層 hardcoded 上限 60 |
| Service 常數 | `internal/capitalflow/service.go:24-32` | `defaultHistoryLimit = 60` — in-memory store capacity default |
| Production wiring | `cmd/atlas/main.go:733` | `capitalflow.NewFileRollingSampleStore(..., 60)` — production store capacity hardcoded 60 |
| H-CF-05 spec gate | `docs/specs/capital-flow-seven-dimension-spec.md §10` | 「分層模型優於七項平權模型」需 ≥252 交易日；目前 60 未達 gate |
| H02 frontend | `shared_web/static/js/pages/capital-history.js:128` | `const samples = currentData[d.key] || []` — 預期每個 dimension 是 array |
| spec §18.3 CF-INV-17 | `docs/specs/capital-flow-seven-dimension-spec.md:463-471` | 「對 `data_available=false` 的維度回 `null` + `data_available: false`，禁止補 0」— 程式實作待補 |
| AGENTS.md 警告 | `internal/capitalflow/AGENTS.md` | 「**PublicBank 欄位歷史較短** \| 公股行庫資料 TWSE 約 2018+ 才完整；早期資料空值（data_available=false），**不補 0**」— 與 CL-5 對齊 |
| MCP wrapper | `cmd/atlas-mcp/server/` | grep 結果：**無任何 tool 呼叫 `/api/capital-flow/history`**（MCP 唯一 consumer 是 H02 frontend） |
| B4 store 狀態 | `data/state/capital_flow_rolling.json` | post-A01 但 15:30 前只 7/17 一筆（capacity 60 vs 252 不影響當前資料量） |

**根因判定**：
1. HandleHistory silent omission：handler 對 store.History error 與空資料一視同仁回 `[]`，無 metadata 揭露缺失原因（CF-INV-17 未實作）
2. Capacity 60 限制：`defaultHistoryLimit` 與 production `NewFileRollingSampleStore(..., 60)` 兩處 hardcoded，未達 spec H-CF-05 gate 的 252

**判定信心**：high（4 層獨立證據互不依賴 + AGENTS.md 與 spec 雙重確認）。

---

## Blast Radius（pre-change protocol Step 1）

### A01 capacity 60→252

- Risk: **LOW**
- 3 個改動點：`cmd/atlas/main.go:733`（60→252）+ `internal/capitalflow/service.go:24`（`defaultHistoryLimit`）+ `internal/capitalflow/handler.go:96-110`（days default 與 cap）
- 既有 TestHandleHistory 需更新 `if got != 60 → 252` 對應期望值
- Production 行為：handler 允許 query 252 天，但 store 內只有 7/17 一筆（不會自動回填歷史）— 後續需另立 backfill manifest

### A02 handler include_meta opt-in

- Risk: **LOW**（opt-in 設計）
- 1 個新 query param `include_meta`（default false）
- 既有 `?days=N` 行為不變（向後相容 H02 frontend）
- 新增 `?days=N&include_meta=true` 路徑：回傳 wrapper
- 既有 TestHandleHistory 不需改（預設行為不變）
- 新增 `TestHandleHistory_WithMeta` 驗證 wrapper shape

### 不變的既有 helper

- `store.History(ctx, dim, beforeDate, limit)` — **保留**，僅 caller 把 limit 從 60 提到 252
- `applyUpsert` capacity 邏輯（rolling_store.go:316-317）— **保留**，僅 caller 傳入的 capacity 從 60 提到 252
- `cmd/atlas/wire_recommender_test.go` 用 `NewMemoryRollingSampleStore(60)` — **保留**（test fixture 顯式 capacity 60 是 OK 的，與 production 252 不同）

---

## Invariant Tracker

### 線 A：CL-5 HandleHistory 程式實作 + capacity 提升

| ID | Problem | Root Cause Hypothesis | Files to Change | Acceptance Criteria | Status | Documentation Impact | Notes |
|----|---------|----------------------|-----------------|---------------------|--------|----------------------|-------|
| **A01** | Production rolling store capacity hardcoded 60，未達 spec H-CF-05 gate 的 ≥252 | **accepted**：`defaultHistoryLimit=60`（service.go）+ `NewFileRollingSampleStore(..., 60)`（main.go）+ handler `days` 上限 60 三處 hardcoded | `cmd/atlas/main.go:733`（60→252）+ `internal/capitalflow/service.go:24`（`defaultHistoryLimit=60`→252）+ `internal/capitalflow/handler.go:96-110`（`days:=60`→252 + `n>60` cap→252）+ 既有 `TestHandleHistory` 更新期望值（`?days=999` cap 60→252）+ spec §18.5 新章節 | (1) `go build ./...` 全綠；(2) `TestHandleHistory` 更新後 PASS（含 `?days=999` cap 252 case）；(3) `TestWireRecommenderDeps_*` 全綠（test fixture capacity 60 保留無關）；(4) `bash scripts/ci/check_atlas_mcp_docs_consistency.sh` 全綠；(5) spec §18.5 新章節引用 spec §10 H-CF-05 | pending | **spec §18.5 新章節**（capacity gate） | 影響：3 個常數變更 + 1 個 spec sub-section；無行為破壞 |
| **A02** | `HandleHistory` 對缺失 dimension silent omission；spec §18.3 CF-INV-17 寫了但**程式實作**待補 | **accepted**：handler 對 store.History error 或空資料一視同仁回 `[]RollingSample{}`，無 metadata 揭露缺失原因；H02 frontend 依賴既有 flat shape 不能破壞 | `internal/capitalflow/handler.go`（新增 `?include_meta=true` query param；開啟時回傳 wrapper `{samples: {...}, meta: {...}}`；不開時維持既有 flat shape）+ spec §18.3 新增 opt-in sub-section（`?include_meta=true` 設計）+ 既有 `TestHandleHistory` 新增 wrapper case + 新增 4 個 test：`TestHandleHistory_IncludeMeta_OK`、`TestHandleHistory_IncludeMeta_Partial`、`TestHandleHistory_IncludeMeta_Complete`、`TestHandleHistory_BackwardCompat_NoMeta` | (1) `go build ./...` 全綠；(2) 既有 `TestHandleHistory` 仍 PASS（無 meta 預設行為不變）；(3) 4 個新 test 全 PASS；(4) `shared_web/static/js/pages/capital-history.js` 不需修改（向後相容）；(5) `bash scripts/ci/check_atlas_mcp_docs_consistency.sh` 全綠 | pending | **spec §18.3 擴充 opt-in sub-section** | 影響：handler +1 條件分支 + 4 個新 test；既有 frontend 0 變更 |

---

## Phase Tracker

### Phase A — Audit（已完成）

| Task | Status | Evidence |
|------|--------|----------|
| Wiki §6.4 CL-5 真相盤查 | done | atlass-wiki/queries/atlas-mcp-capital-flow-history-truth-seeking-2026-07-19.md §6.4 |
| Wiki §2 BL-CL5 確認 | done | atlass-wiki/queries/capital-flow-history-unresolved-2026-07-20.md §2 |
| spec §18.3 CF-INV-17 確認 | done | docs/specs/capital-flow-seven-dimension-spec.md:463-471 |
| H02 frontend blast radius 確認 | done | shared_web/static/js/pages/capital-history.js:128 |
| AGENTS.md silent omission 警告對齊 | done | internal/capitalflow/AGENTS.md「PublicBank 欄位歷史較短」 |
| 設計選項拍板（opt-in include_meta） | done | 避免破壞 H02 frontend + 對齊 spec §18.3 |

### Phase B — Plan（已完成）

| Task | Status | Evidence |
|------|--------|----------|
| ID → 檔案/驗收對映 | done | 本 manifest Invariant Tracker（A01-A02） |
| 預期 commit 順序 | planned | A02（spec 擴充）→ A01（capacity code）→ A01+A02 meta commit |
| Spec outline | done | spec §18.3 opt-in sub-section + §18.5 capacity gate sub-section |

### Phase C — Implement（pending）

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Spec 擴充 §18.3 opt-in sub-section + §18.5 capacity gate | A01+A02 | pending | 待 commit |
| Service `defaultHistoryLimit` 60→252 | A01 | pending | 待 commit |
| Main.go `NewFileRollingSampleStore(..., 60)` → 252 | A01 | pending | 待 commit |
| Handler `days` default + cap 60→252 | A01 | pending | 待 commit |
| Handler `?include_meta=true` opt-in wrapper | A02 | pending | 待 commit |
| 既有 `TestHandleHistory` 更新期望值（cap 60→252） | A01 | pending | 待 commit |
| 4 個新 `TestHandleHistory_*` test（含 BackwardCompat_NoMeta） | A02 | pending | 待 commit |

### Phase D — Close Out（pending）

| Task | Status | Evidence |
|------|--------|----------|
| 驗收：`go build ./...` + `go test ./internal/capitalflow/...` + markdown link check + doc consistency check 全綠 | pending | - |
| Worktree branch push + PR 開啟 + CI 綠 | pending | PR # |
| Post-merge cleanup per `docs/multi-cli-protocol.md` | pending | - |
| Backlog 維持 BL-CL3/BL-CL4/BL-CL6 + spec §18.3 point-in-time endpoint | pending | - |

---

## 預期 Commit 順序

| # | Format | 內容 |
|---|--------|------|
| 1 | `docs(manifest): #A01 #A02 extend §18 spec with include_meta + capacity 252` | 純 spec 文件擴充 |
| 2 | `feat(manifest): #A01 raise capital-flow history capacity 60→252` | 3 處常數變更 + 既有 TestHandleHistory 更新期望值 |
| 3 | `feat(manifest): #A02 add HandleHistory ?include_meta opt-in wrapper (CF-INV-17)` | handler 條件分支 + 4 個新 test |

PR body 必須引用：`See docs/manifests/2026-07-20-cl5-capital-flow-handlehistory.md`

---

## Backlog（明確不做，入後續輪次）

| ID | Problem | Discovery Time | Proposed Round |
|----|---------|----------------|----------------|
| **BL-CL5b** | spec §18.3 描述的 point-in-time endpoint `/api/capital-flow/historical-snapshot/{trading_date}` 完全沒實作 | 2026-07-20 | 下下輪（需先補 B4 store 歷史資料才能驗證） |
| **BL-CL3** | regime observation store + JANUS 整合 | 2026-07-20 | 長期 1 季度 |
| **BL-CL4** | universe session drill-down | 2026-07-20 | 低優 |
| **BL-CL6** | `recorded_at` vs filename date 語意分離 | 2026-07-20 | 評估 |
| **BL-MS-01** | SnapshotDir 長期 retention | 2026-07-20 | 短期 <10K 檔不影響 |
| **BL-CF-01** | production store 歷史資料 backfill（capacity 提到 252 但實際只有 7/17 一筆；需要 Provider 提供歷史 API 或 replay 機制） | 2026-07-20 | 待評估 Provider 能力 |

---

## 設計決策（已從證據推出，不開放重選）

| 決策 | 結論 | 證據 |
|------|------|------|
| **Scope 取捨** | 解讀 A（HandleHistory + capacity）；point-in-time endpoint 入 Backlog | 用戶原始指令字面「HandleHistory 程式實作」；解讀 B 需 B4 通過才能驗證 |
| **Response shape 向後相容** | 預設維持 flat shape；新增 `?include_meta=true` opt-in wrapper | H02 frontend 已在生產（commit 04622ab1）；silent break = 線上事故 |
| **`include_meta` parameter name** | `include_meta`（不用 `with_meta` 或 `meta=true`） | spec §18.3 暗示「status / missing_dimensions」是 metadata；語意最明確 |
| **`meta.status` 枚舉** | `"complete"`（七維度全有）｜`"partial"`（部分有）｜`"missing"`（全空） | 與 spec §18.3 既有枚舉一致；避免引入新枚舉 |
| **`meta.missing_dimensions` 列表** | 只列「完全沒資料」的 dimension（result[dim] 為空 slice） | 對齊 wiki §6.4「silent omission」根因；不過度揭露「某天缺」的細節 |
| **Capacity 60→252** | 252 trading days = 一年（扣假日） | spec §10 H-CF-05 要求；業界標準 252 |
| **常數提升位置** | `cmd/atlas/main.go`（production）+ `service.go`（in-memory default）+ `handler.go`（cap 上限）三處都改 | 三處獨立但語意一致；不留 footgun |
| **Test 策略** | 既有 TestHandleHistory 更新期望值（cap 60→252）+ 新增 4 個 include_meta test | 既有 test 行為契約（cap 60）已變更；不更新會誤導 |

---

## Commit Discipline

- Format: `<type>(manifest): #<ID> <short description>`
- 一 ID 一 commit（A01 + A02 範圍小但語意獨立，分開 commit 保留 ID 結構）
- 驗收標準未過不得 commit
- PR body 必須引用：`See docs/manifests/2026-07-20-cl5-capital-flow-handlehistory.md`
- 不直接推 main；CI 綠才 merge；merge 後依 `docs/multi-cli-protocol.md` 清理分支
- 修改 function/class 前先跑 gitnexus_impact（pre-change protocol，已記錄於 §Blast Radius）

---

## 風險與緩解

| 風險 | 等級 | 緩解 |
|------|------|------|
| H02 frontend 被破壞（line 128 `currentData[d.key]`） | HIGH | opt-in `?include_meta` 設計；預設行為不變；既有 test 仍驗證舊 shape |
| Production capacity 提升但實際只 1 筆資料 | LOW | 252 只是上限；當前 `applyUpsert` trim 在超過 capacity 才觸發，1 筆不會 trim；後續 backfill 另立 manifest |
| `?include_meta=true` 被濫用導致 response size 暴增 | LOW | meta 物件是固定大小（≤7 dimension keys），不會隨 days 變大 |
| MCP wrapper 沒用 `?include_meta` 拿不到 missing 資訊 | LOW | hermes 是當前唯一觀察者；下一步可加 MCP `cf_history_with_meta` 工具，本輪先不做 |
| H-CF-05 gate 仍無法通過（缺歷史資料） | MEDIUM | capacity 提升是必要條件非充分條件；backfill 問題入 BL-CF-01 |
| spec §18.3 既有 sub-section 描述「未來 endpoint」與本 manifest opt-in wrapper 設計語意混淆 | MEDIUM | 在 spec §18.3 明確標註「CF-INV-17 程式實作：本 manifest 採 opt-in 設計；point-in-time endpoint 入 BL-CL5b 下下輪做」 |

---

## 不可動的事項（mission 紀律）

- 不改 H02 frontend `shared_web/static/js/pages/capital-history.js`（向後相容）
- 不改 MCP wrapper（沒有 history tool）
- 不動 spec §18.4（CL-6 範圍）+ spec §18.1/§18.2（CF-INV-15/16 已落地）
- 不補假資料（缺失日 silent omission 改報，但不補 0 — 對齊 AGENTS.md 警告）
- 不動 production store 歷史資料（BL-CF-01 範圍）
- 不直接 push main（必須 worktree + PR + CI 綠）

---

## Session-End State

- **Done this session**: 
  - Phase A 真相盤查（沿用 wiki §6.4 + §2 BL-CL5）
  - Phase B manifest 撰寫（本文件）
  - Pre-change protocol Step 0-7（發現 H02 frontend blast radius，設計 opt-in 避免破壞）
- **Remaining**: 
  1. spec 擴充 §18.3 opt-in sub-section + §18.5 capacity gate（A01+A02 一步）
  2. Phase C 程式實作（A01 capacity + A02 include_meta）
  3. Phase D close out（push + PR + CI 綠 + merge + cleanup）
- **Uncommitted code**: none（本 manifest 為純設計文件）
- **Branch / PR**: `feat/cl5-capital-flow-handlehistory` / 未開 PR
- **Worktree**: `.worktrees/feat-cl5-capital-flow-handlehistory`（5459a646 base）
- **Paused because**: 等業主 review scope 取捨（H02 frontend 向後相容 + capacity 252 是真實需求）

---

## 給下一個 session 的具體行動指令

1. **立即可做**：擴充 `docs/specs/capital-flow-seven-dimension-spec.md` §18.3 與 §18（新增 §18.5）— commit `docs(manifest): #A01 #A02 ...`
2. **接著**：3 處 capacity 常數提升（A01 code）— commit `feat(manifest): #A01 ...`
3. **最後**：handler `?include_meta` opt-in wrapper（A02 code + 4 個 test）— commit `feat(manifest): #A02 ...`
4. **Phase D**：push + PR + CI 綠 + merge + cleanup（依 multi-cli-protocol §Post-merge cleanup）

---

## Change Log

| Date | Version | Change | Author |
|------|---------|--------|--------|
| 2026-07-20 | 1.0 | Initial manifest（2 IDs A01-A02，承接 wiki §6.4 CL-5 + wiki §2 BL-CL5） | OpenCode CLI Agent (Sisyphus) |

---

## 附錄：3 個產出檔案路徑預覽

| 檔案 | 內容 | 預估行數變動 |
|------|------|------------|
| `docs/specs/capital-flow-seven-dimension-spec.md` | §18.3 新增 opt-in sub-section（`?include_meta=true` 設計）+ §18.5 新章節（capacity gate 252 對齊 H-CF-05） | +80 |
| `cmd/atlas/main.go` | `NewFileRollingSampleStore(..., 60)` → 252 | +1 / -1 |
| `internal/capitalflow/service.go` | `defaultHistoryLimit = 60` → 252 + comment 更新引用 §10 H-CF-05 | +3 / -2 |
| `internal/capitalflow/handler.go` | `days := 60` → 252 + cap `n > 60` → 252 + `?include_meta=true` 條件分支 | +30 / -3 |
| `internal/capitalflow/handler_test.go` | 既有 TestHandleHistory 更新 cap 期望值 + 4 個新 test | +100 |

**總計**：~210 行，分散在 5 個檔案，3 個 atomic commits。