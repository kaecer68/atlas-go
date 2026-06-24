# Wave 9 Bootstrap — 獨立 opencode CLI 工作區啟動提示詞

> **用途**：當你需要為 Wave 9（YELLOW 觀測性擴展）開新的獨立 opencode CLI 工作區時，把整份檔案內容貼入新 CLI 的 system prompt 或第一則 user prompt。
> **建立日期**：2026-06-22
> **對應 repo**：kaecer68/atlas-go
> **對應工作目錄**：`/Users/kaecer/workspace/atlas`
> **取代**：`.opencode/prompts/wave-8-bootstrap.md`（Wave 8 收尾後，LLM Phase 2/3/4 已合併，CLI 協調需求消失）

---

## 你正在哪裡

- 工作目錄：`/Users/kaecer/workspace/atlas`（atlas-go repo，**NOT** atlas-go binary）
- 主分支：`main`（已含 Wave 8 全收尾的 v0.0.0.7，PRs #619-#631 全部 merged）
- 目前 VERSION：0.0.0.7
- 必跑 pre-flight：`git log --oneline -1 origin/main` 確認 HEAD 包含 commit bfd95e43（PR #631 merge commit）

## 另一個 CLI 工作區在做什麼（**目前無 — Wave 8 已收尾**）

- 之前的 Wave 8 + LLM Phase 2/3/4 + PR #630 follow-up A 全部已合併至 main
- 沒有「搶工作」的平行 CLI 工作區
- **唯一需注意的平行工作流**：Issue **#611**（OPEN，refactor 9 個 oversized 檔案，6-9 週）— **但 Wave 9 forward-compat 設計保證不衝突**

## 你的範圍（**嚴格白名單 + forward-compat 約束**）

### ✅ 可動

- `internal/eventbus/eventbus.go`（**僅新增 EventType 常數 + eventDescriptions + Payload struct**，不碰 dispatcher 邏輯）
- `internal/eventbus/eventbus_test.go`（新增測試）
- `internal/monitoring/api/events/`（新增事件 .go 檔）
- `internal/monitoring/service/`（**新增** debouncer、drift_detector、channel_health_synthesizer 等套件；**不修改** `crossmarket.go` 與 `pipeline.go`）
- `internal/monitoring/handlers.go`（SSE endpoints）
- `internal/apigateway/background.go`（**只加 Prometheus histogram struct field**，不修改 method 邏輯）
- `web/static/js/services/event-source.js`、`web/static/js/event-listeners.js`（frontend SSE 訂閱）
- `monitoring/rules/`（alertmanager rules）
- `docs/events/`、`docs/roadmap.md`、`docs/wave-9-plan.md`

### ⛔ 不可動（#611 refactor 範圍，避免 boundary 衝突）

**#611 Wave 1（P0 4 檔）**：
- `internal/config/parameters_defaults.go`
- `internal/config/parameters.go`
- `cmd/atlas/main.go`
- `internal/narrative/knowledge_base.go`

**#611 Wave 2（P1 5 檔）**：
- `internal/orchestrator/system.go`
- `internal/orchestrator/executors.go`
- `internal/portfolio/optimizer.go`
- `internal/portfolio/factor_engine.go`
- `internal/monitoring/service/pipeline.go`

### ⚠️ 可讀不可改（forward-compat 約束）

- `internal/portfolio/factor_weight_engine.go`（**只讀** `OnRegimeChange` hook，不修改內部）
- `internal/portfolio/regime.go`（**只讀** `RegimeAllocator.GetCurrentRegime()`）
- `internal/monitoring/gateway_adapter.go`（**只讀** `ChannelErrors()` public method）

## 你要交付的事件（從 Wave 8 plan §2.2 而來）

### Wave 9 YELLOW（5 個觀測性擴展，1 PR = 1 事件）

1. `ChannelIndividualHealth{channel_id, error_count, last_error, severity}`（per-channel, not aggregated）
2. `RegimeChangeConfirmed{old_regime, new_regime, confirmed_at, stability_seconds}`（regime 轉換穩定 30 秒後 emit）
3. `FactorWeightRegression{regime, factor_diffs, regression_score, threshold}`（factor drift signal）
4. `DriftDetector{symbol, actual_weight, target_weight, drift_score}`（post-rebalance drift score）
5. `IngestionLagSpike{channel_id, p99_latency_seconds, threshold_seconds}`（ingestion → processing > 5s）

> **Forward-compat 設計原則**：5 個事件**全部只讀既有 public API**，debouncer 與 drift 計算完全在 monitoring service 層，#611 完成後 Wave 9 程式碼不需重做。

## 模式規範

1. **強制跑 atlas-pre-change-protocol**（AGENTS.md §2）— 每次跨 ≥3 檔的修改必跑
2. 探索工具：`gitnexus_query` / `gitnexus_context`（優先於直接 grep；Wave 9 規劃期間曾用 `codegraph_context`，已於 v0.0.0.8 切換為 GitNexus）
3. 事件 type 定義 pattern：參考 `internal/eventbus/eventbus.go` 既有 EventType + `eventDescriptions` map（line 366-371 為最近新增的範例）
4. 既有 SSE handler pattern：`internal/monitoring/api/events/sse_handler.go`（通用 SSE endpoint，`/api/events/stream`，內建 narrative/promotion/health-alert/risk-gate buffer，新增事件只需加 buffer 區塊 + event listener）
5. SSE 路由註冊：參考 `internal/monitoring/dashboard_api.go:570-573`
6. Frontend pattern：`web/static/js/services/event-source.js`（`EventSourceService`）+ `web/static/js/event-listeners.js`
7. 每個事件 = 1 個 atomic commit + 1 個 PR + 對應測試
8. **Wave 9.0 infrastructure PR 必須先做**：5 個 EventType 常數 slot 先就位、3 個 PD-W9 框架先建立，避免後續 PR 改動太散
9. PR merge pattern：`gh pr create --base main --head <branch>` → 等 CI → `gh pr merge --squash --admin --delete-branch`
10. 不可 `--no-verify` 跳過 hook

## 不可犯的錯

- ❌ **不修改** `internal/portfolio/factor_engine.go` 或 `optimizer.go`（#611 Wave 2 範圍）
- ❌ **不修改** `internal/monitoring/service/pipeline.go`（#611 Wave 2 範圍）
- ❌ **不修改** `internal/config/parameters*.go` 或 `cmd/atlas/main.go`（#611 Wave 1 範圍）
- ❌ **不修改** `internal/narrative/knowledge_base.go` 或 `internal/orchestrator/{system,executors}.go`（#611 範圍）
- ❌ **不修改** `factor_weight_engine.go` / `regime.go` / `gateway_adapter.go` 內部（只讀 public API）
- ❌ **不繞過 EventBus 直接 publish**（所有事件走統一 `eventbus.Publish`）
- ❌ **不寫死 JSONL**（事件持久化用既有 AnnotationStore）
- ❌ **不引入 React/Vue**（維持 vanilla JS）

## #611 衝突預防

若你不小心發現自己需要修改 `internal/portfolio/`、`internal/monitoring/service/pipeline.go`、`internal/orchestrator/`、`internal/narrative/knowledge_base.go` 等 #611 範圍檔案，**立刻停下**，向使用者回報：
- 你想做什麼
- 為什麼需要碰那個範圍
- 是否有 forward-compat 替代方案

讓使用者決定是否要協調 #611 工作流合併或調整 scope。

## 啟動步驟

1. `git status` 確認 main 在 v0.0.0.7（HEAD 應為 bfd95e43 或更新）
2. 跑 `git log --oneline origin/main | head -15` 確認 Wave 8 + LLM + PR #630 follow-up 都已合併
3. 讀 `.omo/plans/wave-9-plan.md` 作為權威 plan 來源（本檔僅為啟動入口）
4. 提議 Wave 9 atomic PR breakdown（5 事件 + Wave 9.0 infra + Wave 9.6 frontend/docs = 7 PR）
5. 第一個 PR = Wave 9.0 infrastructure（**必須先做**，5 個 EventType slot + 3 個 PD-W9 框架先就位）

## 開始

跑完上述啟動步驟後，輸出你的 Wave 9 plan 與第一個 PR 的 scope，**不要直接動工**——等使用者 review plan 後再執行。

---

## 附錄：必讀的既有模式檔案

| 模式 | 檔案 | 用途 |
|------|------|------|
| 事件 type 定義 | `internal/eventbus/eventbus.go` line 78-88（Wave 8 新增的 7 個 EventType）+ line 366-371（eventDescriptions） | Wave 9 沿用相同 pattern |
| 事件 payload schema | `internal/eventbus/eventbus.go:175-179`（`RiskGateEventPayload`） | Wave 8 完整 schema 範例 |
| RiskGate 三向 routing | `internal/eventbus/eventbus.go:855-880`（`PublishRiskGateEvent`） | Wave 8.2 收尾的三向 split 範例 |
| SSE handler（通用） | `internal/monitoring/api/events/sse_handler.go:1-320` | 通用 SSE endpoint，內建 5 種 buffer（narrative/promotion/health-alert/risk-gate/SubscribeAll） |
| SSE 路由註冊 | `internal/monitoring/dashboard_api.go:570-573` | `mux.HandleFunc("/api/events/stream", sseHandler.ServeHTTP)` |
| Public API for ChannelIndividualHealth | `internal/monitoring/gateway_adapter.go:32-39`（`ChannelErrors()`） | 既有 public method，Wave 9.1 直接訂閱 |
| Public hook for FactorWeightRegression | `internal/portfolio/factor_weight_engine.go:179`（`OnRegimeChange`） | 既有 hook，Wave 9.3 訂閱 weights 變化 |
| Public API for RegimeChangeConfirmed | `internal/eventbus/eventbus.go`（`EventRegimeChange` + `PublishRegimeChange`） | 既有 EventRegimeChange，Wave 9.2 外部 debouncer |
| BackgroundTaskManager | `internal/apigateway/background.go` | Wave 9.5 加 histogram metrics，只加 struct field |
| Frontend SSE | `web/static/js/services/event-source.js:1-151` | `EventSourceService` 類，支援 exponential backoff |
| Frontend event 監聽 | `web/static/js/event-listeners.js` | `DOMContentLoaded` 事件委派，新增 SSE 事件監聽需在此註冊 |
| AGENTS.md 規範 | `AGENTS.md` §2 atlas-pre-change-protocol、§3 模組大小 | 必跑 + 不可破壞 |
| 既有 plan | `.omo/plans/wave-9-plan.md`（**本檔為權威來源**，wave-9-bootstrap.md 僅為啟動入口） | 7 PR atomic breakdown + PD-W9-1/2/3 + forward-compat boundary |
| Issue #611 | github.com/kaecer68/atlas-go/issues/611（OPEN） | refactor 9 檔，Wave 9 forward-compat 保證不衝突 |
| Wave 8 plan | `.omo/plans/wave-8-plan.md` | Wave 8 PD-1/PD-2/PD-3 沿用至 Wave 9 |

## 衝突預防機制（複述強調）

任何對 ⛔ 不可動清單的修改都**必須先停下**回報，不論理由多充分。forward-compat 是 Wave 9 設計的核心約束，破壞等於放棄 Wave 9 與 #611 的解耦。