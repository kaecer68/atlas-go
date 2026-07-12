# Wave 8 Bootstrap — 獨立 opencode CLI 工作區啟動提示詞

> **用途**：為 Wave 8/9（事件驅動擴展）開新的獨立 opencode CLI 工作區時，把本檔內容貼入 system prompt 或第一則 user prompt。
> **建立日期**：2026-06-20
> **對應 repo**：kaecer68/atlas-go
> **對應工作目錄**：`/Users/kaecer/workspace/atlas`

---

## 你正在哪裡

- 工作目錄：`/Users/kaecer/workspace/atlas`（atlas-go repo，**NOT** atlas-go binary）
- 主分支：`main`（v0.0.0.6，PRs #599-#610 merged）
- 目前 VERSION：0.0.0.6
- 必跑 pre-flight：`git status` 確認 main HEAD 為 c97c26d9 或更新

## 另一個 CLI 工作區（**不可搶工作**）

- Worktree：`atlas-feature-llm-phase2-capability-wiring`
- 範圍：`internal/llm/`、`cmd/atlas/main.go` provider wiring、`internal/llm_annotator/`
- 正在修 PR #610（5 項 findings）：capability drift、doc.go 編譯、ForceProvider 繞過 DataClass、redundant GetSecret、iota sentinel

## 你的範圍（嚴格白名單）

### ✅ 可動
- `internal/eventbus/eventbus.go`（僅新增 EventType 常數 + `eventDescriptions` + Payload struct）
- `internal/monitoring/api/events/`（新增事件檔案 + 擴充 `sse_handler.go` buffer）
- `internal/monitoring/service/`（事件 producer）
- `internal/monitoring/dashboard_api.go`（SSE 路由註冊）
- `shared_web/static/js/services/event-source.js`、`admin_web/static/js/event-listeners.js`
- `internal/monitoring/rules.go`
- `docs/roadmap.md` Wave 8/9 段落、`docs/REFERENCE/events/`

### ⛔ 不可動
- `internal/llm/`、`internal/llm_annotator/`、`internal/narrative/`、`internal/spawning/`、`internal/orchestrator/`
- `cmd/atlas/main.go` provider wiring 部分
- `internal/config/` Provider/Config 結構
- `internal/monitoring/service/crossmarket.go`

## 你要交付的事件

### Wave 8 RED（9 個）
1. `RiskGateRejected`
2. `RiskGateOverride`
3. `IndustryCalendarEvent`
4. `TradeSlippage`
5. `LLMAnnotatorCircuitOpen`
6. `LLMAnnotatorFallbackUsed`
7. `LLMAnnotatorQuotaExceeded`
8. `BacktestCompleted`
9. `CalibrationCompleted`

### Wave 9 YELLOW（5 個）
- `ChannelIndividualHealth`
- `FactorWeightRegression`
- `DriftDetector`
- `RegimeChangeConfirmed`
- `IngestionLagSpike`

## 模式規範

1. 每次跨 ≥3 檔修改必跑 `atlas-pre-change-protocol`。
2. 探索優先使用 `gitnexus_query` / `gitnexus_context`。
3. 事件 type 定義 pattern：參考 `internal/eventbus/eventbus.go` EventType 常數 + `eventDescriptions` + 既有 `RiskEventPayload`。
4. SSE handler 為通用 endpoint `/api/events/stream`；新增事件只需加 buffer 區塊 + event listener。
5. SSE 路由註冊：`internal/monitoring/dashboard_api.go:570-573`。
6. Frontend pattern：`EventSourceService` + `event-listeners.js`。
7. 每個事件 = 1 個 atomic commit + 1 個 PR + 對應測試。
8. PR merge：`gh pr create --base main --head <branch>` → 等 CI → `gh pr merge --squash --admin --delete-branch`。
9. 不可 `--no-verify` 跳過 hook。

## 不可犯的錯

- ❌ 不修改 capability enum 名稱
- ❌ 不修改 provider 路由/router/ForceProvider 邏輯
- ❌ 不繞過 EventBus 直接 publish
- ❌ 不寫死 JSONL
- ❌ 不引入 React/Vue

## 啟動步驟

1. `git status` 確認 main 在 v0.0.0.6
2. 跑 `/gsd-progress` 或 `atlas-progress` 看 Wave 7 收尾狀態
3. 提議 Wave 8 atomic PR breakdown（9 事件 × 1 PR = 9 PR）
4. 第一個 PR 建議 `RiskGateRejected`（觸發點最近、refactor 風險最低）

## 權威 Plan 來源

- `docs/archive/2026-07-10-wave-8-plan.md`（歷史 plan，已封存）
- `.omo/plans/wave-9-plan.md`（Wave 9 權威來源）

## 衝突預防

若發現需要修改 `internal/llm/` 或 `cmd/atlas/main.go` provider 區段，**立刻停下**，向使用者回報：你想做什麼、為什麼需要碰該範圍、替代方案。
