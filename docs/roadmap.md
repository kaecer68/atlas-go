# Roadmap

## Phase 1: Foundation

- initialize standalone repository
- add GitHub workflow and templates
- define canonical domain types
- implement simple simulation engine
- support mock and replay market data providers

## Phase 2: Taiwan Replay MVP

- TWSE and TPEX daily adapters
- daily replay runner
- scorecard and performance ledger
- first sector and macro agent prompts
- sessionized replay artifacts

## Phase 3: OpenClaw Training Loop ✅

- agent registry
- prompt versioning
- rolling evaluation windows
- keep-or-revert experiment flow
- ledger-backed weakest-agent selection
- multi-session backtest windows
- intelligent mutation type selection (prompt_tightening, risk_rule_change, portfolio_constraint_revision)
- interactive mutation proposal scripts

## Phase 4: Near-Real-Time Paper Trading ✅

- Fugle snapshots ✅
- TWSE OpenAPI integration ✅
- Hybrid Provider (Fugle + TWSE fallback) ✅
- Live State Store ✅
- Event-driven orchestration ✅
- Monitoring and alerting ✅

## Phase 5: Portfolio Intelligence ✅

- Multi-factor portfolio optimization ✅
- Risk-adjusted position sizing ✅
- Dynamic regime-based allocation ✅
- Agent weighting system ✅
- Style rotation detection ✅
- Post-trade analysis ✅

## Wave 7.5 (2026-06) ✅

Shipped in v0.0.0.6:

- Risk gate safety wiring with explicit promotion limits and rejection reasons.
- Orphan config rejection for misplaced `parameters.json` files.
- Alertmanager webhook receiver hardening and field contract validation.
- Calibration metadata preservation across auto-rollback scenarios.
- Channel health improvements with per-channel failure reasons and fetch log persistence.
- `AutoJudgePromoter` wired into the atlas scheduler.
- `EventPromotionRecorded` event type delivered to dashboard clients via SSE with catch-up support.
- `GET /api/dashboard/fetch-log` endpoint for recent channel fetch events.

## Wave 8 (2026-06, in planning)

Detailed plan: [`docs/wave-8-plan.md`](wave-8-plan.md) · Bootstrap prompt for new CLI: [`.opencode/prompts/wave-8-bootstrap.md`](../.opencode/prompts/wave-8-bootstrap.md)

目標：把 b29 audit 識別的 4 個 RED + 5 個 YELLOW 事件缺口實作成統一事件流（type 定義 + producer + SSE handler + frontend component）。

### 前置決策（必拍板才能開工）

- **PD-1**：事件 payload schema 版本化（每個事件加 `schema_version` 欄位）
- **PD-2**：JSONL 審計軌跡（9 個新事件同步寫入 AnnotationStore，batch flush 100 筆或 1 秒）
- **PD-3**：效能預算與去重（單節點 max 500 events/sec；高頻事件 producer 端 dedup）

### Wave 8 RED（9 個事件，1 PR = 1 事件）

1. `RiskGateRejected`
2. `RiskGateOverride`
3. `IndustryCalendarEvent`
4. `TradeSlippage`
5. `LLMAnnotatorCircuitOpen`
6. `LLMAnnotatorFallbackUsed`
7. `LLMAnnotatorQuotaExceeded`
8. `BacktestCompleted`
9. `CalibrationCompleted`

### Wave 9 YELLOW（5 個，排隊等 Wave 8 收尾）

- `ChannelIndividualHealth`、`FactorWeightRegression`、`DriftDetector`、`RegimeChangeConfirmed`、`IngestionLagSpike`

### 依賴

- 上游：Wave 7.5 v0.0.0.6 ✅、Phase 2 CLI #610 修復（in-flight）
- 下游：Phase 4 Production Trading、refactor issue #611

### 邊界（嚴格）

- ✅ 可動：`internal/monitoring/api/events/`、`internal/monitoring/service/`、`web/services/sse.js`、`web/components/event-list.js`、`monitoring/rules/`、`docs/`
- ⛔ 不可動：`internal/llm/`、`internal/llm_annotator/`、`internal/narrative/`、`internal/spawning/`、`internal/orchestrator/`、`cmd/atlas/main.go` provider 區段

---

## Phase 6: Production Trading (Next)

- Decision chain transparency (FactorScores breakdown, ConvictionBreakdown, MacroEvent confidence) ✅
- NT$ currency formatter + after-tax equity curve + cost KPI cards ✅ (Phase 3)
- Experiment monetary NT$ display on evolution panel ✅ (Phase 3)
- Evolution panel chart-based UI upgrade (dual-curve chart, monetary trend visualization)
- Real broker integration
- Live order management
- Risk circuit breakers
- Performance reporting

## P-Infra: Infrastructure Foundation (Future)

Infrastructure improvements required before or during production hardening. Triggered alongside database migration (JSONL → SQLite/PostgreSQL).

| 項目 | 用途 | 不改善的風險 |
|------|------|-------------|
| **Structured logging** (slog/zap) | 360 處 `log.Printf` → JSON structured logs，支援 ELK/Loki 查詢、alerting | Production incident 排查從 30 分變 2 小時 |
| **Context propagation** | 127 處 `context.Background()` → 傳遞 parent context，配合 graceful shutdown | Live mode graceful shutdown 不完整，goroutine 洩漏 |
| **Data Access Layer** | 定義 `OutcomeStore`/`QuoteStore`/`SessionStore` interface，支援 JSONL→SQLite 遷移 | 將來換 DB 幾乎要重寫整個 ledger 層 |

**觸發時機**：當以下任一條件滿足時啟動
- 數據量增長導致 JSONL 效能瓶頸
- 需要多 consumer（如 dashboard、backtest）共用同一筆資料
- Production 環境需要 ELK/Loki 等 structured logging 工具

## Execution Roadmap (2026 Q2-Q4)

### Short Term (2-6 weeks)

- Freeze proposal/commit/event contracts and add validation checks
- Implement decision state machine with explicit transition guards
- Persist trace fields (`proposal_id`, `commit_id`, `approval_id`) in ledger artifacts
- Build guard pipeline v2 with auditable reject reasons
- Deliver first dashboard APIs for macro radar, agent observatory, and forecast-vs-reality

Exit criteria:
- Contract tests pass and are versioned
- Replay runs remain deterministic
- Every experiment record can be reconstructed end-to-end

### Mid Term (6-12 weeks)

- Add macro event ingestion pipeline with factor timeline snapshots
- Add parallel simulation scenarios (base, stress, shock)
- Wire human approval/revert flow into command path
- Introduce weekly governance review with acceptance scorecards

Exit criteria:
- Scenario comparisons are stable across repeated runs
- Human-in-the-loop checkpoints are enforced before promotion
- Guard outcomes are visible in monitoring panels

### Long Term (3-6 months)

- Integrate production broker adapter with strict circuit breakers
- Add SLO-driven operations dashboards and incident runbooks
- Run staged drills for rollback and degraded-data operation
- Promote to production with controlled capital ramp

Exit criteria:
- Staging drills pass with documented evidence
- Rollback completes within defined operational window
- Risk and audit controls satisfy production checklist
