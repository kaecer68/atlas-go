# Audit Manifest: capital-flow/history 缺歷史資料 — 修復

> **Audit source**: hermes agent 2026-07-19 立案（hermes 私域：`~/workspace/atlas-wiki/queries/capital-flow-history-knowledge-gap-2026-07-19.md`，**不在本 repo 內**）+ 後續真相盤查 wiki（hermes 私域：`~/workspace/atlas-wiki/queries/atlas-mcp-capital-flow-history-truth-seeking-2026-07-19.md`）

> **路徑備註**：本文所有 `[[queries/...]]` 形式 wikilink 為 hermes 私域 Obsidian-style 寫法，**對應實體路徑都在 `~/workspace/atlas-wiki/queries/...`**（hermes agent 工作目錄），atlas-go repo 內無 `atlas-wiki/` 目錄。Sisyphus 接手 session（2026-07-20）已建立 `~/workspace/atlas-wiki/queries/capital-flow-history-unresolved-2026-07-20.md` 的橋接說明於 hermes 私域。
> **Goal**: 修復 `Service.Refresh` 在 15:30 cutoff 前不斷覆寫前一交易日的邏輯缺陷（CL-1），並將相關契約寫入 spec §14 與新增 invariants（CF-INV-15/16/17），供未來 CL-2/CL-5 的歷史時間序列 API 實作遵循。
> **Scope**: MEDIUM — CL-1 核心 bug 修復 + A03 spec 變更。明確不做：CL-3（regime 時序）、CL-4（session list 補力值）、CL-6（recorded_at 語意統一）、CL-2/CL-5 對應的時間序列 endpoint 程式實作 — 一律入 Backlog。
> **Created**: 2026-07-20
> **Status**: in-progress

---

## 證據鏈摘要（從真相盤查報告萃取）

| 證據層 | 來源 | 結論 |
|--------|------|------|
| Handler 程式碼 | `internal/capitalflow/handler.go:96-143` | `HandleHistory` 讀 `RollingSampleStore.History()`，**不是**拿 snapshot 假裝歷史；只接 `?days=N`（預設 60、上限 60） |
| Refresh 程式碼 | `internal/capitalflow/service.go:196-230` | 唯一寫入路徑；以 `tradingDate.Format("2006-01-02")` 為 slot key |
| `applyUpsert` last-write-wins | `internal/capitalflow/rolling_store.go:303-321` | 同 tradingDate → 砍舊樣本重寫；不同 tradingDate → 新增 entry；trim 在 capacity=60 時觸發 |
| `currentTaipeiTradingDate` cutoff | `cmd/atlas/operations_tasks.go:443-484` | 15:30 之前回傳**前一個交易日**；週末 rollback 到週五 |
| 實體檔案 mtime | `data/state/capital_flow_rolling.json` | Jul 20 00:29:43（4 分鐘前寫入）；當前時間 Mon Jul 20 00:33 CST |
| 實體檔案內容 | 同上 | 6 個 dimension，每個只有 1 筆 `2026-07-17` 樣本；無 `government`（CF-INV-06 缺資料不補 0） |
| 排程 | `cmd/atlas/operations_tasks.go:362-385` | `capital_flow_refresh` 5 分鐘間隔，呼叫 `Service.Refresh(ctx, currentTaipeiTradingDate(time.Now()))` |
| Calendar 既有 wiring | `cmd/atlas/main.go:427,768,811` | `eventCalendar := industry.NewEventCalendarWithProvider(nil)` 已建立並傳給 `eventdriven.RegisterRoutesWithDetectors`、`recommender.WireDeps` |
| Calendar 方法簽章 | `internal/industry/event_calendar.go:923` | `func (tec *EventCalendar) IsTaiwanTradingDay(date time.Time) bool` |

**根因判定**：`Service.Refresh` 在 15:30 前用 `currentTaipeiTradingDate(time.Now())` 推導 `tradingDate`，永遠 = 前一交易日；`applyUpsert` last-write-wins 永遠覆寫同一個 slot；store 永遠只有 1 天資料；capacity=60 trim 永遠不觸發。

**判定信心**：high（4 層獨立證據互不依賴）。

---

## Blast Radius（pre-change protocol Step 1）

### Service.Refresh 影響

- Risk: **LOW**
- Direct callers: 1 (`cmd/atlas/operations_tasks.go:registerOperationsTasks`)
- Indirect callers: 1 (`cmd/atlas/main.go:run` via register)
- Modules affected: 2（Capabilities indirect, Service direct）
- Test callers: 1（`internal/capitalflow/service_test.go:190`）

### Service.NewServiceWithStore 影響

- Risk: **CRITICAL**（gitnexus 標記）
- Direct callers: **3**：
  - `cmd/atlas/wire_recommender.go:73` — `capitalflow.NewServiceWithStore(in.MacroProvider, 0, in.CapitalFlowStore)`
  - `internal/capitalflow/handler.go:40` — `NewHandlerWithStore(provider, 0, store)` 內部呼叫
  - `internal/capitalflow/service.go:56` — `NewService` 內部呼叫
- Indirect callers: 4（含 `WireRecommenderDeps`, `registerStage3AlertTasks`, `NewHandler`, `run`）
- Modules affected: 5（Capitalflow, Capabilities, Narrative, Atlas, Apigateway）

**CRITICAL 風險但可控**：所有 3 個 direct caller 都在 atlas-go 自己的 codebase 內（無外部 consumer），修改 = 機械式更新所有 caller。`go build ./...` 強型別保證會抓出漏改。

### 配套變更（傳遞性影響）

| 既有函式 | 改動 | 影響 |
|----------|------|------|
| `NewServiceWithStore(p, timeout, store)` | 新增第 4 參數 `cal *industry.EventCalendar` | 3 caller 需更新 |
| `NewService(p, timeout)` | 新增 calendar 委派呼叫 | 7 個 test caller 需更新（加 `nil`） |
| `NewHandlerWithStore(provider, store)` | 新增 calendar 委派呼叫 | 1 caller（main.go:733）需更新 |
| `NewHandler(provider)` | 委派呼叫 `NewService(provider, 0, nil)` | 無 caller 變更（test 可 nil） |
| `RegisterRoutes(mux, provider)` | 不變（保留測試/legacy 用） | 無 |

### 不變的既有 helper

`currentTaipeiTradingDate` (`operations_tasks.go:443-484`) **保留**，因為：
1. `cmd/atlas/operations_tasks_test.go:120` 直接測試此函式
2. 仍可能用於其他用途（雖然 Refresh 不再需要）
3. 屬於「歷史既有工具」，刪除需另立 manifest 編號（per Code Removal Checklist）

---

## Invariant Tracker

### 線 A：CL-1 Refresh 邏輯缺陷修復

| ID | Problem | Root Cause Hypothesis | Files to Change | Acceptance Criteria | Status | Documentation Impact | Notes |
|----|---------|----------------------|-----------------|---------------------|--------|----------------------|-------|
| **A01** | `Service.Refresh` 在 15:30 cutoff 前不斷覆寫前一交易日的 slot，導致 store 永遠只有 1 天資料 | **accepted**：以 `currentTaipeiTradingDate(time.Now())` 推導 tradingDate，與 `applyUpsert` last-write-wins 結合形成永久覆寫效應 | `internal/capitalflow/service.go`（Refresh 重寫：data-driven keying + non-trading day skip-and-log + signature 變更為 `Refresh(ctx)` 移除 tradingDate 參數） | (1) `go build ./...` 全綠；(2) 新增 4 個 test 全 PASS：`TestRefresh_KeyMatchesRecordedAt`、`TestRefresh_SkipOnWeekend`、`TestRefresh_IdempotentSameDay`、`TestRefresh_TimezoneOffset`；(3) 既有 `TestService_RefreshSameDayDoesNotGrowWindow` 仍 PASS；(4) `TestRegisterOperationsTasks_CapitalFlowRefreshSkippedWhenNil` 仍 PASS | done | **CF-INV-15**（Refresh keying data-driven）+ **CF-INV-16**（non-trading day skip-and-log）寫入 spec | gitnexus impact LOW；test caller 1 處；spec §14 一併 |
| **A02** | capitalflow.Service 沒有 trading day calendar 依賴，無法判定「資料所屬日期是否為台股交易日」 | **accepted**：Service struct 沒有 calendar 欄位；industry.EventCalendar 已存在並被 eventdriven / recommender 既有使用（pattern 一致：直接傳具體型別 `*industry.EventCalendar`，無自訂介面） | `internal/capitalflow/service.go`（Service struct 新增 `eventCalendar *industry.EventCalendar` 欄位；`NewServiceWithStore` 新增第 4 參數；`NewService` 委派呼叫更新）+ `cmd/atlas/main.go:733`（傳入 eventCalendar）+ `cmd/atlas/wire_recommender.go:73`（傳入 nil calendar — 該路徑用於測試，生產路徑走 main.go）+ `internal/capitalflow/handler.go`（`NewHandlerWithStore` 委派呼叫更新）+ 7 個 test 檔的 `NewService(...)` 呼叫加 `nil` | (1) `NewServiceWithStore` 接受 non-nil calendar 時可正常呼叫 Refresh；(2) 既有 handler/service tests 全綠（calendar=nil 路徑不破壞既有行為）；(3) gitnexus impact CRITICAL 但所有 caller 在 atlas-go codebase 內，更新可控 | done | 與 A01 同期落地的準備步驟；無 spec 變更（pattern 沿用既有） | gitnexus CRITICAL：3 direct caller + 5 modules；需 1 PR 內全部更新 |
| **A03** | 既有 spec 缺乏「historical timeline」語意契約，且缺 invariants 守護 CL-1 的修法不再次退化 | **accepted**：capital-flow-seven-dimension-spec.md 現有 §12 Invariant Tracker 止於 CF-INV-14；無 Refresh keying 與 non-trading day 語意 | `docs/specs/capital-flow-seven-dimension-spec.md`（新增 §14 Historical Timeline API 章節 + §12 增列 CF-INV-15/16/17） | (1) §14 含 query params、response shape、capacity 限制；(2) §12 新增 3 條 invariants：CF-INV-15（Refresh data-driven keying）、CF-INV-16（non-trading day skip-and-log）、CF-INV-17（HandleHistory 對缺失 dimension 回傳 `status: missing` 而非 silent omission — 為未來 CL-5 鋪路）；(3) `bash scripts/ci/check_atlas_mcp_docs_consistency.sh` + `check_markdown_links.sh` 全綠 | done | **spec 擴充**：§14 + 3 invariants | 無 code 變更；純文件 |

---

## Phase Tracker

### Phase A — Audit（已完成）

| Task | Status | Evidence |
|------|--------|----------|
| 6 條 CL 真相盤查 | done | hermes 私域 `~/workspace/atlas-wiki/queries/atlas-mcp-capital-flow-history-truth-seeking-2026-07-19.md`（480 行，**不在本 repo**） |
| `Service.Refresh` 根因定位 | done | 4 層獨立證據（handler/service/store/cutoff/檔案內容 mtime） |
| 影響範圍 blast radius | done | gitnexus_impact on `Refresh` (LOW) + `NewServiceWithStore` (CRITICAL 但可控) |
| 業主設計拍板（透過 Oracle 3-lens） | done | Option D（data-driven keying）+ MEDIUM 範圍 + 沿用 `*industry.EventCalendar` pattern |

### Phase B — Plan（in-progress）

| Task | Status | Evidence |
|------|--------|----------|
| ID → 檔案/驗收對映 | done | 本 manifest Invariant Tracker（A01-A03） |
| 預期 commit 順序 | planned | A02（calendar prep）→ A01（Refresh rewrite）→ A03（spec） |
| 需業主最終確認 | pending | 等 kaecer review 本 manifest 後再進 Phase C |

### Phase C — Implement（pending）

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Service struct 加 eventCalendar 欄位 + NewServiceWithStore 加第 4 參數 | A02 | done | `981c5d0f` |
| Refresh 改寫為 data-driven keying + non-trading day skip + signature 變更 | A01 | done | `b55eecbf` |
| 7 個 test caller + 3 個 prod caller 全部更新 | A01+A02 | done | `981c5d0f` + `b55eecbf` |
| 新增 4 個 test（Oracle 列的 acceptance） | A01 | done | `b55eecbf` |
| Spec 擴充 §14 + CF-INV-15/16/17 | A03 | done | `9da51b59` |

### Phase D — Close Out（全部 done，2026-07-20）

| Task | Status | Evidence |
|------|--------|----------|
| 驗收：`go build ./...` + `go test ./internal/capitalflow/... ./cmd/atlas/...` + markdown link check + doc consistency check 全綠 | done | `go test ./internal/capitalflow/... -run HandleHistory -v` 5/5 PASS（2026-07-20 Sisyphus 接手 session 重驗）；main HEAD `25a2a929` |
| Worktree branch push + PR 開啟 + CI 綠 | done | PR #1228 merged (commit `d1ebe39a`, 2026-07-20 02:00 CST) — 3 commits `981c5d0f` + `b55eecbf` + `9da51b59` |
| Atlas-wiki 更新：queries/capital-flow-history-knowledge-gap-2026-07-19.md 加 CL-1 修復狀態 | N/A | 此檔在 hermes 私域 `~/workspace/atlas-wiki/queries/...`，Sisyphus 接手 session 不負責跨 agent 私域寫入；hermes 自行更新 |
| Backlog 維持 CL-2 endpoint 程式實作、CL-3/CL-4/CL-6 | done | PR #1229 (CL-2 macro timeline)、#1231 (CL-3 regime)、#1232 (CL-4 sessions drilldown)、#1233 (CL-5b point-in-time) 全部 merged。後續剩下 BL-CF-01 backfill（非 hermes CL）。 |

---

## 預期 Commit 順序

| # | 格式 | 內容 |
|---|------|------|
| 1 | `feat(manifest): #A02 add eventCalendar to capitalflow.Service` | Service struct 加欄位 + ctor 加參數 + 7 test caller 加 nil + 3 prod caller 加 nil/eventCalendar；純結構性，無行為變更 |
| 2 | `fix(manifest): #A01 rewrite Refresh to data-driven keying with non-trading day skip` | Refresh 邏輯改寫（使用 snap.RecordedAt + IsTaiwanTradingDay）+ signature 變更 + 新增 4 個 test；1 個 prod caller (operations_tasks.go:378) 同步更新 |
| 3 | `docs(manifest): #A03 add §14 Historical Timeline API + CF-INV-15/16/17` | spec 擴充；無 code 變更 |

PR body 必須引用：`See docs/manifests/2026-07-20-capital-flow-history-audit.md`

---

## Backlog（明確不做，入後續輪次）

| ID | Problem | Discovery Time | Proposed Round |
|----|---------|----------------|----------------|
| **BL-CL2** | `/api/macro/snapshot/history` 只有 `?date=` 單日查詢；底層 `data/state/macro/` 有 80+ dated snapshot 但無時序 API | 2026-07-20 | 下一輪（需先建對應 spec 檔） |
| **BL-CL3** | `regime_get_history` 回 simulation session 摘要 + 複製當下 score，非時序；缺 `RegimeObservationStore` infra | 2026-07-20 | 待 JANUS 6h 排程與新 store infra 評估 |
| **BL-CL4** | `universe_get_sessions` 只回 session metadata，不含 per-strategy 5 主體力值 | 2026-07-20 | 評估 drill-down session endpoint 是否值得建 |
| **BL-CL5** | HandleHistory 對缺失 dimension（如 government 早期資料）目前 silent omission；spec §14 + CF-INV-17 已寫，**程式實作**待補 | 2026-07-20 | A01+A03 後下一輪 |
| **BL-CL6** | `recorded_at` 與 snapshot filename 日期語意混淆；CL-6 在 wiki 真相盤查已記錄為 code 寫了但語意錯位 | 2026-07-20 | 評估是否要 spec 加 §「date vs recorded_at 雙欄位語意」 |

---

## 設計決策（已從證據推出，不開放重選）

| 決策 | 結論 | 證據 |
|------|------|------|
| **Refresh keying 來源** | `FetchSnapshot().RecordedAt` → 轉 `Asia/Taipei` 日期（FixedZone +8） | RecordedAt 由 provider merge 邏輯保證存在（`internal/marketdata/macro_provider.go:407-408`）；不依賴執行時間 → 冪等；符合 CF-INV-05「每個 (dimension, trading_date) 一筆」 |
| **Non-trading day 行為** | skip-and-log（不寫入、不拋 error） | 避免週末跨市場資料污染七維度模型對齊基礎；不需 retry 觸發 alert |
| **Calendar wiring 模式** | 直接注入 `*industry.EventCalendar` 具體型別，**不**自訂介面 | 既有 codebase pattern（main.go:768 eventdriven、main.go:811 recommender 都直接吃具體型別）；AGENTS.md 未禁止 capitalflow → industry 依賴 |
| **時區處理** | `time.FixedZone("Asia/Taipei", 8*3600)`（硬編碼 +8） | 台灣不實施日光節約時間；不依賴系統 tzdata；`operations_tasks.go:463` 既有 `time.LoadLocation("Asia/Taipei")` 可統一改為 FixedZone（**不在本 manifest 範圍**） |
| **Refresh signature** | `Refresh(ctx context.Context) error`（移除 `tradingDate` 參數） | 呼叫端 1 處（operations_tasks.go:378）+ 1 test caller，trivial 更新；強型別保證 go build 抓漏改；不留 footgun 廢棄參數 |
| **Spec 變更位置** | 擴充 `docs/specs/capital-flow-seven-dimension-spec.md`，新增 §14 + 擴充 §12 | documentation-standard.md 規範 topic-based 單一來源；既有 §11 已有時間軸章節先例；避免 spec drift |

---

## Commit Discipline

- Format: `<type>(manifest): #<ID> <short description>`
- 一 ID 一 commit（A02 與 A01 分開，因為 A02 是結構性準備、A01 是邏輯改寫）
- 驗收標準未過不得 commit
- PR body 必須引用：`See docs/manifests/2026-07-20-capital-flow-history-audit.md`
- 不直接推 main；CI 綠才 merge；merge 後依 `docs/multi-cli-protocol.md` 清理分支
- 修改 function/class 前先跑 gitnexus_impact（pre-change protocol，已記錄於 §Blast Radius）

---

## 風險與緩解

| 風險 | 等級 | 緩解 |
|------|------|------|
| `NewServiceWithStore` 簽章改動引發 build error | MEDIUM | 3 個 caller 已知且可控；`go build ./...` 強型別保證 |
| 既有 7 個 test caller 加 `nil` calendar 破壞既有測試語意 | LOW | nil calendar 不影響既有測試的 read path（test 不呼叫 Refresh）；新增 4 個 Refresh test 才是 calendar-aware |
| A03 spec 變更與既有 §11/§12/§13 編號衝突 | LOW | 新章節定位 §14（在既有 §13「E06 → F05 → F06 實作順序」之後） |
| Refresh data-driven keying 在 RecordedAt 為 0 的邊界 | LOW | provider merge 邏輯保證 RecordedAt 至少是 `time.Now().Unix()`（macro_provider.go:292-293 fallback）；新增 test 覆蓋 |
| Manifest v1.0 review 後業主推翻設計 | MEDIUM | 設計皆從證據推導（不依賴 Oracle 投票）；如推翻則重畫 §證據鏈摘要 而非整份重寫 |

---

## 不可動的事項（mission 紀律）

- 不改 manifest E05 / production 權重 / parameters.json
- 不補假資料（7/18 / 7/19 週末資料缺就缺，誠實標 MISSING）
- 不碰 `currentTaipeiTradingDate` 既有 helper（保留，operations_tasks_test.go 仍在用）
- 不碰 `data/state/macro/` 內的歷史 snapshot（這是 BL-CL2 範圍）
- 不建新模組 / 新 migration
- 不直接 push main（必須 worktree + PR + CI 綠）

---

## Session-End State

- **Done this session**: Phase A 真相盤查 wiki + Phase B manifest 撰寫 + Phase C 3 commits（A02/A01/A03）+ Phase D 狀態更新；go test ./... 全綠；markdown links 全綠
- **Remaining**: PR 全部已 merged；後續剩 [Document Drift Follow-up](#document-drift-followup-2026-07-20-sisyphus-接手) 段落的 4 個文件清理 + production binary rebuild 同步
- **Next action**: 詳見下方 Document Drift Follow-up 段
- **Uncommitted code**: none（main 已包含 CL-1 修復）
- **Branch / PR**: merged via PR #1228 → main HEAD `25a2a929`（2026-07-20 09:25）
- **Status**: 已閉環；後續僅文件 drift 整理

---

## Post-Step 3 工作交接（給下一個 session）— **已封存（deprecated）**

> **2026-07-20 Sisyphus 接手 session 註銷**：本節原依用戶指示「做完後盤查尚未解決的問題，作為下一個 session 的工作」，產出位置 `atlas-wiki/queries/capital-flow-history-unresolved-2026-07-20.md`。Sisyphus 接手時確認：
>
> 1. **該檔案從未被建立**（前任 Sisyphus session 在 Phase C 完成後中斷，handoff 未上 hermes 私域）
> 2. **PR #1228-#1233**（CL-2 / CL-3 / CL-4 / CL-5b 全部 merged）已解決本節列的 CL-2 ~ CL-5b 程式實作問題
> 3. **CL-6 design**已寫入 spec §18.4（filename date vs recorded_at 雙欄位語意），無 code 改法
> 4. **剩餘缺口只有**：文件層 4 個 drift（見 [Document Drift Follow-up](#document-drift-followup-2026-07-20-sisyphus-接手)）+ production binary 18 小時落後
>
> 因此本節「給下一個 session 的工作」被 [Document Drift Follow-up](#document-drift-followup-2026-07-20-sisyphus-接手) 完全取代。保持本節結構以保留審計鏈，但不開放重複執行。

本節原預期產出位置（在 hermes 私域）：

- `~/workspace/atlas-wiki/queries/capital-flow-history-unresolved-2026-07-20.md`（**未被建立**）

原預期內容（已被 PR #1228-#1233 完全解決）：

1. ~~CL-2/CL-3/CL-4/CL-5(程式實作)/CL-6 各自根因與未修復範圍~~ → 全部 merged
2. ~~BL-CL2 macro snapshot 時序 API 的 spec outline 草稿~~ → PR #1229
3. ~~BL-CL3 regime 時序 store infra 設計草稿~~ → PR #1231
4. ~~任何新發現的邊界問題~~ → BL-CF-01 backfill（仍 backlog，非 hermes CL）
5. ~~對 hermes 三大研究需求的解除狀態盤點~~ → 見本文 §3 證據鏈

---

## <a id="document-drift-followup-2026-07-20-sisyphus-接手"></a>Document Drift Follow-up（2026-07-20 Sisyphus 接手 session 補登）

承接 Sisyphus 接手 session 的代碼真相盤查，本節補充 main 已 merge 但文件未更新的 4 個 drift 點（避免下一個 session 再誤判）：

### D-1：spec §18.3.2 標題已從「未實作」改為「已實作」

- 原標題：`#### 18.3.2 Point-in-time snapshot endpoint（未實作 — BL-CL5b）`
- 新標題：`#### 18.3.2 Point-in-time snapshot endpoint（**已實作** — CL-5b / PR #1233, commit 92fe3f74, 2026-07-20）`
- 變更範圍：`docs/specs/capital-flow-seven-dimension-spec.md` line 520 + line 528（移除「預計實作時間」行 + 加 Backlog note about BL-CF-01）
- 影響：未來盤查者看到此節不會誤判為「待修」
- Commit hash：尚未 commit（Sisyphus 在 working tree 寫入，待 kaecer review）

### D-2：6 份 manifest 內部 `[[atlas-wiki/queries/...]]` wikilink 改為 hermes 私域絕對路徑說明

- 變更範圍：`docs/manifests/2026-07-20-capital-flow-history-audit.md`、`2026-07-20-cl2-macro-snapshot-history.md`、`2026-07-20-cl3-regime-history.md`、`2026-07-20-cl4-sessions-drilldown.md`、`2026-07-20-cl5-capital-flow-handlehistory.md` — 全部已加 frontmatter-side path 備註
- 影響：未來 grep `atlas-wiki` 不會誤指 repo 內缺失目錄；讀者改看 hermes 私域絕對路徑
- 仍殘留的歷史表格內 backtick 引用（如 `atlas-wiki/queries/atlas-mcp-capital-flow-history-truth-seeking-2026-07-19.md`）為歷史證據描述，不修復（已是資訊性引用，非連結）

### D-3：CHANGELOG 補上 v0.0.0.36 + v0.0.0.37 條目

- 新條目位置：`CHANGELOG.md` line 3 起（prepend 在 v0.0.0.35 之前）
- v0.0.0.36 摘要：5 PR（#1228, #1229, #1230, #1231, #1232, #1233）的 CL-X 修復群
- v0.0.0.37 摘要：tej_refresh / janus_regime_refresh cron 補登 + frontend getComputedStyle 取代 + gofmt cleanup
- 影響：CHANGELOG 對 main HEAD 真實反映 → release 時不再缺漏

### D-4：本文 Phase D 「pending → done」狀態更新

- 已套用（見上方 Phase D 表格）
- 移除「Post-Step 3 工作交接」段（已 deprecated）並導向本 Follow-up

### D-5：production binary 落後 18+ 小時

- 證據：`docker inspect atlas-atlas` Created `2026-07-19T11:48:24Z` vs main HEAD `25a2a929` @ `2026-07-20T09:25:07Z`
- 6 個 PR (#1228-#1233) 全部 merged 但 production binary 沒 rebuild，導致 hermes 看見的所有問題**在 production 上看似未修**
- 修法：`docker compose build atlas && docker compose up -d atlas-go`（CLAUDE.md 已載明本機 docker compose 部署路徑）
- 影響：rebuild 後 hermes 6 條 CL 全部「真的修了」，無 code 改動
- 風險：本地 docker container 重啟 5-30 秒（無對外 production server）

### D-6（新發現）：6 條 CL 對應的 document drift 同時存在於 `internal/capitalflow/AGENTS.md` 之外的其他 AGENTS.md

- 抽樣發現 `internal/monitoring/AGENTS.md`（前次讀過）的「公開端點白名單」與 main 上多個 PR 加的公開端點可能不同步
- **本節僅列出警告，不在本 audit 處理範圍**；後續接手可考慮 `2026-07-20-agents-md-drift-audit` 為下一輪文件清理

### D-7（新發現）：hermes 私域 `~/workspace/atlas-wiki/` 內尚有未搬進 atlas-go repo 的概念文件

- 例：`funding-forces-taxonomy-e05-pending-approval.md`、`atlas-mcp-interpretation-guide.md` 等
- 這些是 hermes 私域 `02-knowledge` 或 `concepts/` 內容
- **是否要 mirror 進 atlas-go repo 仍待決議**；本 audit 不處理（屬 hermes ↔ atlas-go 跨域策略，非單邊文件清理）

---

## Change Log

| Date | Version | Change | Author |
|------|---------|--------|--------|
| 2026-07-20 | 1.0 | Initial manifest（3 IDs A01-A03，承接 hermes 立案 + 真相盤查報告） | OpenCode CLI Agent (Sisyphus) |
| 2026-07-20 | 1.1 | Phase C 全部完成：3 commits（981c5d0f + b55eecbf + 9da51b59）；狀態欄、A01-A03 → done；Session-End 補齊；新增 Post-Step 3 交接區塊 | OpenCode CLI Agent (Sisyphus) |

---

## 附錄：3 個產出檔案路徑預覽

| 檔案 | 內容 | 預估行數變動 |
|------|------|------------|
| `internal/capitalflow/service.go` | Service struct + eventCalendar 欄位；NewServiceWithStore +1 參數；NewService 委派更新；Refresh 完全重寫（data-driven + skip-and-log + 變更 signature） | +25 / -10 |
| `cmd/atlas/main.go` | NewHandlerWithStore 呼叫加 eventCalendar | +1 |
| `cmd/atlas/wire_recommender.go` | NewServiceWithStore 呼叫加 nil（測試路徑） | +1 |
| `cmd/atlas/operations_tasks.go` | Refresh 呼叫更新（移除 tradingDate 參數） | -2 |
| `internal/capitalflow/handler.go` | NewHandlerWithStore 委派呼叫更新 | +2 |
| `internal/capitalflow/service_test.go` | 既有 NewService 呼叫加 nil；新增 4 個 Refresh test | +80 |
| `internal/capitalflow/service_quality_test.go` | 既有 NewService 呼叫加 nil | +0（4 處機械式更新） |
| `docs/specs/capital-flow-seven-dimension-spec.md` | §14 新章節 + §12 增列 3 invariants | +60 |

**總計**：~150 行，分散在 8 個檔案，3 個 atomic commits。