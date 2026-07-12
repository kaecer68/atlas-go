# Wave 9 Bootstrap — 獨立 opencode CLI 工作區啟動提示詞

> **用途**：為 Wave 9（YELLOW 觀測性擴展）開新的獨立 opencode CLI 工作區時，把本檔內容貼入 system prompt 或第一則 user prompt。
> **建立日期**：2026-06-22
> **對應 repo**：kaecer68/atlas-go
> **對應工作目錄**：`/Users/kaecer/workspace/atlas`
> **取代**：`.opencode/prompts/wave-8-bootstrap.md`

---

## 你正在哪裡

- 工作目錄：`/Users/kaecer/workspace/atlas`（atlas-go repo，**NOT** atlas-go binary）
- 主分支：`main`（v0.0.0.7，PRs #619-#631 merged）
- 目前 VERSION：0.0.0.7
- 必跑 pre-flight：`git log --oneline -1 origin/main` 確認 HEAD 包含 bfd95e43

## 平行工作流

- Wave 8 + LLM Phase 2/3/4 + PR #630 follow-up A 已全部合併至 main。
- 沒有搶工作的平行 CLI 工作區。
- **唯一注意**：Issue #611（OPEN，refactor 9 個 oversized 檔案，6-9 週）— Wave 9 forward-compat 設計保證不衝突。

## 你的範圍（嚴格白名單 + forward-compat 約束）

### ✅ 可動
- `internal/eventbus/eventbus.go`（僅新增 EventType 常數 + `eventDescriptions` + Payload struct）
- `internal/eventbus/eventbus_test.go`
- `internal/monitoring/api/events/`（新增事件 .go 檔）
- `internal/monitoring/service/`（**新增** debouncer、drift_detector、channel_health_synthesizer；**不修改** `crossmarket.go` 與 `pipeline.go`）
- `internal/monitoring/handlers.go`（SSE endpoints）
- `internal/apigateway/background.go`（**只加** Prometheus histogram struct field）
- `shared_web/static/js/services/event-source.js`、`admin_web/static/js/event-listeners.js`
- `monitoring/rules/`
- `docs/REFERENCE/events/`、`docs/roadmap.md`、`docs/wave-9-plan.md`

### ⛔ 不可動（#611 refactor 範圍）
- P0 4 檔：`internal/config/parameters_defaults.go`、`internal/config/parameters.go`、`cmd/atlas/main.go`、`internal/narrative/knowledge_base.go`
- P1 5 檔：`internal/orchestrator/system.go`、`internal/orchestrator/executors.go`、`internal/portfolio/optimizer.go`、`internal/portfolio/factor_engine.go`、`internal/monitoring/service/pipeline.go`

### ⚠️ 可讀不可改
- `internal/portfolio/factor_weight_engine.go`（只讀 `OnRegimeChange` hook）
- `internal/portfolio/regime.go`（只讀 `RegimeAllocator.GetCurrentRegime()`）
- `internal/monitoring/gateway_adapter.go`（只讀 `ChannelErrors()`）

## 你要交付的事件

### Wave 9 YELLOW（5 個，1 PR = 1 事件）
1. `ChannelIndividualHealth{channel_id, error_count, last_error, severity}`
2. `RegimeChangeConfirmed{old_regime, new_regime, confirmed_at, stability_seconds}`
3. `FactorWeightRegression{regime, factor_diffs, regression_score, threshold}`
4. `DriftDetector{symbol, actual_weight, target_weight, drift_score}`
5. `IngestionLagSpike{channel_id, p99_latency_seconds, threshold_seconds}`

> **Forward-compat**：5 個事件全部只讀既有 public API，debouncer 與 drift 計算完全在 monitoring service 層，#611 完成後 Wave 9 程式碼不需重做。

## 模式規範

1. 每次跨 ≥3 檔修改必跑 `atlas-pre-change-protocol`。
2. 探索優先使用 `gitnexus_query` / `gitnexus_context`。
3. 事件 type 定義 pattern：沿用 `internal/eventbus/eventbus.go` 既有 EventType + `eventDescriptions`。
4. SSE handler 為通用 endpoint `/api/events/stream`；新增事件只需加 buffer 區塊 + event listener。
5. SSE 路由註冊：`internal/monitoring/dashboard_api.go:570-573`。
6. Frontend pattern：`EventSourceService` + `event-listeners.js`。
7. 每個事件 = 1 個 atomic commit + 1 個 PR + 對應測試。
8. **Wave 9.0 infrastructure PR 必須先做**：5 個 EventType 常數 slot + 3 個 PD-W9 框架先就位。
9. PR merge：`gh pr create --base main --head <branch>` → 等 CI → `gh pr merge --squash --admin --delete-branch`。
10. 不可 `--no-verify` 跳過 hook。

## 不可犯的錯

- ❌ 不修改 `internal/portfolio/factor_engine.go` 或 `optimizer.go`
- ❌ 不修改 `internal/monitoring/service/pipeline.go`
- ❌ 不修改 `internal/config/parameters*.go` 或 `cmd/atlas/main.go`
- ❌ 不修改 `internal/narrative/knowledge_base.go` 或 `internal/orchestrator/{system,executors}.go`
- ❌ 不修改 `factor_weight_engine.go` / `regime.go` / `gateway_adapter.go` 內部
- ❌ 不繞過 EventBus 直接 publish
- ❌ 不寫死 JSONL
- ❌ 不引入 React/Vue

## 啟動步驟

1. `git status` 確認 main 在 v0.0.0.7
2. `git log --oneline origin/main | head -15` 確認 Wave 8 + LLM + PR #630 已合併
3. 讀 `.omo/plans/wave-9-plan.md` 作為權威 plan 來源
4. 提議 Wave 9 atomic PR breakdown（5 事件 + Wave 9.0 infra + Wave 9.6 frontend/docs = 7 PR）
5. 第一個 PR = Wave 9.0 infrastructure（5 個 EventType slot + 3 個 PD-W9 框架先就位）

## 權威 Plan 來源

- `.omo/plans/wave-9-plan.md`（**本 plan 為權威來源**）
- `docs/archive/2026-07-10-wave-8-plan.md`（歷史 plan，已封存）
- github.com/kaecer68/atlas-go/issues/611

## 衝突預防

任何對 ⛔ 不可動清單的修改都**必須先停下**回報，不論理由多充分。
