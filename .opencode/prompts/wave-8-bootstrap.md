# Wave 8 Bootstrap — 獨立 opencode CLI 工作區啟動提示詞

> **用途**：當你需要為 Wave 8/9（事件驅動擴展）開新的獨立 opencode CLI 工作區時，把整份檔案內容貼入新 CLI 的 system prompt 或第一則 user prompt。
> **建立日期**：2026-06-20
> **對應 repo**：kaecer68/atlas-go
> **對應工作目錄**：`/Users/kaecer/workspace/atlas`

---

## 你正在哪裡

- 工作目錄：`/Users/kaecer/workspace/atlas`（atlas-go repo，**NOT** atlas-go binary）
- 主分支：`main`（已含 Wave 7 全收尾的 v0.0.0.6，PRs #599-#610 全部 merged）
- 目前 VERSION：0.0.0.6
- 必跑 pre-flight：`git status` 確認 main 在 v0.0.0.6（HEAD 應為 c97c26d9 或更新）

## 另一個 CLI 工作區在做什麼（**不可搶工作**）

- Worktree 名稱：`atlas-feature-llm-phase2-capability-wiring`（in-flight）
- 範圍：`internal/llm/`、`cmd/atlas/main.go`（provider wiring 部分）、provider/router/ForceProvider/QuotaLogic、`internal/llm_annotator/` 的業務邏輯
- 正在修 PR #610（PR #609 review 的 5 項 findings）：
  1. capability 名稱 6 處 drift
  2. doc.go example 不編譯
  3. ForceProvider 繞過 DataClass gate
  4. redundant GetSecret lookup
  5. iota+1 sentinel 不安全
- 後續 Phase 2：capability→model 路由、prompt construction、AnnotationStore 持久化

## 你的範圍（**嚴格白名單**）

### ✅ 可動
- `internal/eventbus/eventbus.go`（**僅新增 EventType 常數 + eventDescriptions + Payload struct**，不碰 dispatcher 邏輯）
- `internal/monitoring/api/events/`（新增事件檔案 + 擴充 sse_handler.go buffer 區段）
- `internal/monitoring/api/events/llm_*.go`（**僅限事件類型定義與 payload schema**，不碰 llm_annotator 內部）
- `internal/monitoring/service/`（事件 producer）
- `internal/monitoring/dashboard_api.go`（SSE 路由註冊，line 570 附近）
- `web/static/js/services/event-source.js`、`web/static/js/event-listeners.js`（frontend SSE 訂閱）
- `internal/monitoring/rules.go`（alert rules）
- `docs/roadmap.md`（Wave 8/9 段落）、`docs/events/`（事件類型 doc）

### ⛔ 不可動
- `internal/llm/`、`internal/llm_annotator/`、`internal/narrative/`、`internal/spawning/`、`internal/orchestrator/`
- `cmd/atlas/main.go`（provider wiring 部分；其餘背景任務註冊可新增）
- `internal/config/` 的 Provider/Config 結構
- `internal/monitoring/service/crossmarket.go`（Wave 7 已結案，勿動）

## 你要交付的事件（從 b29 audit 而來）

### Wave 8 RED（9 個高優先）

1. `RiskGateRejected{order_id, gate_level, reason, equity_state}`
2. `RiskGateOverride{order_id, operator, original_reason, override_reason}`
3. `IndustryCalendarEvent{industry_code, event_type, event_date, impact}`
4. `TradeSlippage{order_id, expected, actual, slippage_bps, venue}`
5. `LLMAnnotatorCircuitOpen{provider, capability, opened_at, error_class}`
6. `LLMAnnotatorFallbackUsed{primary_provider, fallback_provider, capability, reason}`
7. `LLMAnnotatorQuotaExceeded{provider, capability, quota_type, reset_at}`
8. `BacktestCompleted{backtest_id, strategy, sharpe, mdd, duration_ms}`
9. `CalibrationCompleted{run_id, success, drifted_metrics[], duration_ms}`

### Wave 9 YELLOW（5 個觀測性擴展）

- `ChannelIndividualHealth`（per-channel, not aggregated）
- `FactorWeightRegression`（factor drift signal）
- `DriftDetector`（post-rebalance drift score）
- `RegimeChangeConfirmed`（regime 轉換穩定後 30 秒 emit）
- `IngestionLagSpike`（ingestion → processing > 5s）

## 模式規範

1. **強制跑 atlas-pre-change-protocol**（AGENTS.md §2）— 每次跨 ≥3 檔的修改必跑
2. 探索工具：`gitnexus_query` / `gitnexus_context`（優先於直接 grep；Wave 8 期間曾用 `codegraph_context`，已於 v0.0.0.8 切換為 GitNexus）
3. 事件 type 定義 pattern：參考 `internal/eventbus/eventbus.go:15-77`（EventType 常數）+ 同檔 line 245 `eventDescriptions`（描述 map）+ line 157 `RiskEventPayload`（既有 risk event schema）
4. SSE handler pattern：參考 `internal/monitoring/api/events/sse_handler.go`（通用 SSE endpoint，`/api/events/stream`，內建 narrative/promotion/health-alert 三種 buffer，新增事件只需加 buffer 區塊 + event listener，不需新 endpoint）
5. SSE 路由註冊：參考 `internal/monitoring/dashboard_api.go:570-573`（`mux.HandleFunc("/api/events/stream", sseHandler.ServeHTTP)`）
6. Frontend pattern：參考 `web/static/js/services/event-source.js`（`EventSourceService` 類，含 exponential backoff）+ `web/static/js/event-listeners.js`（事件監聽器註冊）
7. 每個事件 = 1 個 atomic commit + 1 個 PR + 對應測試
8. PR merge pattern：`gh pr create --base main --head <branch>` → 等 CI → `gh pr merge --squash --admin --delete-branch`
9. 不可 `--no-verify` 跳過 hook（除非 hook 已自動允許）

## 不可犯的錯

- ❌ **不修改 capability enum 名稱**（Phase 2 CLI 在管；引用 `internal/llm/` 的 capability 常數時必須 import，不可自行 hardcode 字串）
- ❌ **不修改 provider 路由/router/ForceProvider 邏輯**
- ❌ **不繞過 EventBus 直接 publish**（所有事件走統一 `eventbus.Publish`）
- ❌ **不寫死 JSONL**（事件持久化用既有 AnnotationStore，或新增 EventStore 而非自己 marshal）
- ❌ **不引入 React/Vue**（維持 141 web/ 檔案的結構化 vanilla JS）

## 啟動步驟

1. `git status` 確認 main 在 v0.0.0.6（HEAD 應為 c97c26d9 或更新）
2. 跑 `/gsd-progress` 或 `atlas-progress` 看 Wave 7 收尾狀態
3. 提議 Wave 8 atomic PR breakdown（9 事件 × 1 PR = 9 PR，分批 ship）
4. 第一個 PR = Wave 8 第一個事件類型（建議 `RiskGateRejected`，理由：現有 handler 觸點最近、refactor 風險最低）

## 開始

跑完上述啟動步驟後，輸出你的 Wave 8 plan 與第一個 PR 的 scope，**不要直接動工**——等使用者 review plan 後再執行。

---

## 附錄：必讀的既有模式檔案

| 模式 | 檔案 | 用途 |
|------|------|------|
| 事件 type 定義 | `internal/eventbus/eventbus.go:15-77`（EventType 常數）+ `eventDescriptions`（line 245） | 20 種既有 event type，新增事件需在此加常數 |
| 事件 payload schema | `internal/eventbus/eventbus.go:157-213`（`RiskEventPayload`、`HealthAlertPayload`） | 既有 payload struct pattern |
| 既有 risk event 發佈 | `internal/eventbus/eventbus.go:572-584`（`PublishRiskEvent`） | 既有 risk event publisher 範例 |
| SSE handler（通用） | `internal/monitoring/api/events/sse_handler.go:1-320` | 通用 SSE endpoint，內建 narrative/promotion/health-alert 三種 buffer |
| SSE 路由註冊 | `internal/monitoring/dashboard_api.go:570-573` | `mux.HandleFunc("/api/events/stream", sseHandler.ServeHTTP)` |
| Producer 範例 | `internal/monitoring/service/crossmarket.go:241-243` | degraded callback emit（注意：這是 callback 不是 EventBus，Wave 8 producer 應走 eventBus.Publish） |
| Risk Gate Producer 注入點 | `internal/risk/gate.go:165-168`（`publish`）+ `Subscribe` 方法 | RiskGate 的 callback 機制，producer 可透過 Subscribe 注入 |
| Frontend SSE | `web/static/js/services/event-source.js:1-151` | `EventSourceService` 類，支援 exponential backoff、EventSource 原生 |
| Frontend event 監聽 | `web/static/js/event-listeners.js` | `DOMContentLoaded` 事件委派（~80 handlers），新增 SSE 事件監聽需在此註冊 |
| AGENTS.md 規範 | `AGENTS.md` §2 atlas-pre-change-protocol、§3 模組大小 | 必跑 + 不可破壞 |
| 既有 audit | `docs/wave-8-plan.md`（b29 audit 缺口追蹤） | Wave 7 audit 與事件缺口依據 |
| 既有 prompt | `.opencode/prompts/wave-8-bootstrap.md`（本檔） | 新 CLI 啟動參考 |
| **新增** Plan 檔案 | `docs/wave-8-plan.md:1-188`（9 事件 + PD-1/2/3 + 9-PR breakdown） | **本 plan 為權威來源**，bootstrap prompt 僅為啟動入口 |

## 衝突預防機制

若你不小心發現自己需要修改 `internal/llm/` 或修改 `cmd/atlas/main.go` 的 provider 區段，**立刻停下**，向使用者回報：
- 你想做什麼
- 為什麼需要碰那個範圍
- 替代方案

讓使用者決定是否要協調另一個 CLI 工作區合併或調整 scope。